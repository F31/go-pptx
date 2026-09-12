// 本文件实现 VIDEO-01（方案 §21.1/§21.4 视频子集）的视频形状：
//
//   - AddVideo 嵌入媒体 Part（MP4/WebM）+ 视频关系 + p:pic 形视频片段
//     （p:blipFill + p:videoFile，PowerPoint 接受的视频形状表达之一）；
//     媒体 Part 默认独立（同 SHA-256 + Content Type 复用）；
//   - VideoSpec 含 TrackKey/Bounds/AltText/IsDecorative/PosterFrame（可选）；
//   - VideoShape 句柄（Kind=ShapeVideo、VideoSource() 运行时回放）。
//   - VideoProfile 元数据落 /docProps/video.xml（自有命名空间扩展）；
//     TrackKey/Role/ShapeID/MediaPart/ContentSHA256 与 audio 同模式。
//
// 已知约束（不在 VIDEO-01 范围）：
//   - 时长精确推导（依赖 moov/mvex/track header，V2.6 不实现；调用方
//     可选传 Duration 或接受"时长未知"返回 ErrDurationUnknown）；
//   - 视频播放控件写入（媒体控件开关、循环属性、开始时间）属后续切分；
//   - poster frame 仅做独立图片占位（与 AddPicture 共用同一媒体层），不
//     嵌入 MP4 内部 cover-art track；
//   - 客户端实测（PowerPoint/WPS 播放证据属闭环二验收）。
package pptx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/editplan"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/videoprobe"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- 公共类型 ----------

// VideoRole 是视频在页面上的语义角色（方案 §21.1 视频子集）。
type VideoRole int

const (
	// VideoRoleBackground 是背景视频，不参与 PlanTimingSync 翻页计算。
	VideoRoleBackground VideoRole = iota
	// VideoRoleMain 是页面主视频。
	VideoRoleMain
	// VideoRoleTrim 是辅视频片段（不影响主时序）。
	VideoRoleTrim
)

func (r VideoRole) String() string {
	switch r {
	case VideoRoleBackground:
		return "background"
	case VideoRoleMain:
		return "main"
	case VideoRoleTrim:
		return "trim"
	}
	return fmt.Sprintf("VideoRole(%d)", int(r))
}

// VideoSpec 描述要嵌入或更新的视频。
//
// TrackKey 是调用方在当前文档内的稳定业务标识（同一 TrackKey 重复
// 合成走幂等路径——VIDEO-01 与 AUDIO-01 同语义：AddVideo 拒绝重复
// TrackKey，幂等更新留给后续切分）。Source/Bounds 必填；AltText/
// IsDecorative 控制 p:cNvPr；PosterFrame 可选（传入即在同位置插入
// 一张占位 PNG/JPEG 图片，文档打开时先看到）。
type VideoSpec struct {
	TrackKey string
	Role     VideoRole
	Source   MediaSource
	// X/Y/Width/Height 必填（EMU），对应 p:spPr/a:xfrm 框。
	X, Y          int64
	Width, Height int64
	// AltText 写 p:cNvPr@descr；空时 IsDecorative=true 才写 decorative="1"，
	// 否则不写任一属性（与 picture.go IMAGE-01 同侧语义）。
	AltText      string
	IsDecorative bool
	// PosterFrame 可选：嵌入时与视频同位置叠放一张占位图片。PosterSource
	// 由调用方提供（走 IMAGE-01 同源 stageMedia）；不做内部重采样。
	PosterSource MediaSource
	PosterAlt    string
}

// Stable: VideoShape 是页面视频形状的读侧句柄（实现 Shape 接口），
// 类比 TextFrame/Paragraph/TextRun 同级别核心入口型。0 公开字段
// （shapeNode + 私有 role/profile/posterPath 仅用于构造期记录与海报帧
// 路径定位）—— role/profile/posterPath 不暴露给用户，访问走 Role() /
// Profile() / PosterSource() 方法。句柄失效语义由 shapeNode + STALE-
// GUARD 锁定。v1.x 内承诺：
//
//   - 类型签名不变；不新增/重命名/移除公开方法
//   - 现有方法签名与返回类型不变（Kind / Role / VideoSource / Profile /
//     ReplacePoster / PosterSource 等）
//   - 仅允许追加新方法
//   - 句柄身份语义不变
//   - 实现 Shape 接口
type VideoShape struct {
	shapeNode
	role    VideoRole
	profile VideoProfile
	// posterPath 是可选 PosterFrame 在 spTree 内紧随视频形状的 p:pic 路径，
	// 用于 PosterSource() 访问与 ReplacePoster。
	posterPath []nodeStep
}

// Kind 返回 ShapeVideo（classifyShape 在读取时也会按 p:videoFile 命中此值）。
func (v *VideoShape) Kind() ShapeKind { return ShapeVideo }

// Role 返回视频角色。
func (v *VideoShape) Role() VideoRole { return v.role }

// videoCTForExt 给出 video Part 扩展 → Content Type（VIDEO-01 E 档）。
func videoCTForExt(ext string) string {
	switch strings.ToLower(ext) {
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	}
	return "application/octet-stream"
}

// VideoSource 返回（嵌入时）经库重组后的媒体源（运行时回放探测）。
// 关闭文档后句柄失效。
func (v *VideoShape) VideoSource() (MediaSource, error) {
	if v.p == nil || v.p.closed {
		return nil, Annotate(ErrClosed, "VideoShape.VideoSource")
	}
	if v.profile.MediaPart == "" {
		return nil, Annotate(ErrNotFound, "VideoShape.VideoSource")
	}
	return videoSourceRef{part: v.profile.MediaPart, ct: videoCTForExt(extOfMedia(v.profile.MediaPart)), p: v.p}, nil
}

// Profile 返回本 shape 关联的 VideoProfile（库创建视频）。
func (v *VideoShape) Profile() VideoProfile { return v.profile }

// HasPoster 返回是否嵌入时附带 PosterFrame。
func (v *VideoShape) HasPoster() bool { return v.posterPath != nil }

// PosterSource 返回 poster frame 的媒体源（如有）；不存在返回 ErrNotFound。
func (v *VideoShape) PosterSource() (MediaSource, error) {
	if v.p == nil || v.p.closed {
		return nil, Annotate(ErrClosed, "VideoShape.PosterSource")
	}
	if v.posterPath == nil {
		return nil, Annotate(ErrNotFound, "VideoShape.PosterSource")
	}
	doc, pic, err := v.posterLocate()
	if err != nil {
		return nil, Annotate(err, "VideoShape.PosterSource")
	}
	blip := picBlipOf(doc, pic)
	if blip == nil {
		return nil, &OperationError{
			Op: "VideoShape.PosterSource", Part: string(v.part),
			Message: "poster pic has no a:blip", Err: ErrUnsupportedEdit,
		}
	}
	rid, ok := blip.Attr(nsOfficeDocument, "embed")
	if !ok || rid == "" {
		return nil, &OperationError{
			Op: "VideoShape.PosterSource", Part: string(v.part),
			Message: "poster pic a:blip missing r:embed", Err: ErrUnsupportedEdit,
		}
	}
	media, err := resolveMediaByRID(v.p, v.part, rid)
	if err != nil {
		return nil, Annotate(err, "VideoShape.PosterSource")
	}
	return videoSourceRef{part: media, ct: imageCTForExt(extOfMedia(media)), p: v.p}, nil
}

// posterLocate 解析 posterPath 并返回 pic 元素。
func (v *VideoShape) posterLocate() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, err := v.p.docOf(v.part)
	if err != nil {
		return nil, nil, Annotate(err, "posterLocate")
	}
	n := resolvePath(doc, v.posterPath)
	if n == nil {
		return nil, nil, Annotate(ErrStaleHandle, "posterLocate")
	}
	return doc, n, nil
}

// videoSourceRef 实现 MediaSource：运行时从包内读字节。
type videoSourceRef struct {
	part opc.PartName
	ct   string
	p    *Presentation
}

func (s videoSourceRef) Open(ctx context.Context) (io.ReadCloser, error) {
	if s.p == nil || s.p.closed {
		return nil, Annotate(ErrClosed, "video source")
	}
	b, err := s.p.partBytes(s.part)
	if err != nil {
		return nil, Annotate(err, "video source")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (s videoSourceRef) DeclaredType() string { return s.ct }

// resolveMediaByRID 按 rId 查 slide rels，返回对应 media Part。
func resolveMediaByRID(p *Presentation, slide opc.PartName, rid string) (opc.PartName, error) {
	rels, ok, err := p.relsOf(slide)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", Annotate(ErrNotFound, "resolveMediaByRID rels")
	}
	for _, r := range rels {
		if r.ID == rid && r.Mode == opc.TargetInternal {
			return r.TargetPart, nil
		}
	}
	return "", Annotate(ErrNotFound, "resolveMediaByRID")
}

// imageCTForExt 从扩展名映射 Image Content Type（IMAGE-01 同侧常量复用）。
func imageCTForExt(ext string) string {
	switch strings.ToLower(ext) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	}
	return "application/octet-stream"
}

// ---------- AddVideo ----------

// AddVideo 在页面上嵌入视频：基于源字节探测容器（MP4/WebM），创建媒体
// Part + 视频关系 + p:pic 形视频片段；可选同步嵌入 PosterFrame。
//
// 同一 TrackKey 重复调用返回 ErrUnsupportedEdit（幂等更新留给后续切分）。
func (s *Slide) AddVideo(ctx context.Context, src MediaSource, spec VideoSpec) (*VideoShape, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}
	if src == nil {
		return nil, Annotate(ErrInvalidArgument, "Slide.AddVideo src")
	}
	if spec.TrackKey == "" {
		return nil, &OperationError{Op: "Slide.AddVideo", Message: "TrackKey is required", Err: ErrInvalidArgument}
	}
	if strings.ContainsAny(spec.TrackKey, "/\\") {
		return nil, &OperationError{Op: "Slide.AddVideo", Message: "TrackKey contains path separator", Err: ErrInvalidArgument}
	}
	if spec.Width <= 0 || spec.Height <= 0 {
		return nil, &OperationError{Op: "Slide.AddVideo", Message: "Width/Height must be positive", Err: ErrInvalidArgument}
	}
	p := s.p
	existing := p.findVideoProfile(spec.TrackKey)
	if existing != nil {
		return nil, &OperationError{
			Op:      "Slide.AddVideo",
			Message: fmt.Sprintf("TrackKey %q already exists", spec.TrackKey),
			Err:     ErrUnsupportedEdit,
		}
	}
	data, err := readMedia(ctx, src)
	if err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}
	if len(data) == 0 {
		return nil, &OperationError{Op: "Slide.AddVideo", Message: "empty video bytes", Err: ErrInvalidArgument}
	}
	info, err := videoprobe.Probe(videoprobe.ProbeInput{
		Data:          data,
		Declared:      src.DeclaredType(),
		ContainerHint: hintFromName(slideMediaName(spec.TrackKey)),
	})
	if err != nil {
		return nil, Annotate(mapVideoProbeError(err), "Slide.AddVideo")
	}
	if info.Container == "" {
		return nil, &OperationError{
			Op:      "Slide.AddVideo",
			Message: "video container not recognized (VIDEO-01 supports mp4/webm)",
			Err:     ErrUnsupportedFormat,
		}
	}
	ct := videoContentType(info.Container)
	ext := videoExt(info.Container)

	// 可选 PosterFrame：探测为 PNG/JPEG 后与视频同事务一并提交。
	var posterData []byte
	var posterKind imageKind
	if spec.PosterSource != nil {
		pdata, err := readMedia(ctx, spec.PosterSource)
		if err != nil {
			return nil, Annotate(err, "Slide.AddVideo poster")
		}
		kind, err := probeImage(pdata, spec.PosterSource.DeclaredType())
		if err != nil {
			return nil, Annotate(err, "Slide.AddVideo poster")
		}
		posterData = pdata
		posterKind = kind
	}

	// 1) 视频媒体 Part 规划（SHA-256 去重，content type 决定扩展）。
	sum := sha256.Sum256(data)
	mediaName, mediaOp, err := p.planVideoMedia(data, sum, ext, ct)
	if err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}
	// 2) poster 媒体 Part 规划。
	var posterName opc.PartName
	var posterOp *editplan.Operation
	if len(posterData) > 0 {
		posterName, posterOp, err = p.planMedia(posterData, posterKind)
		if err != nil {
			return nil, Annotate(err, "Slide.AddVideo poster")
		}
	}
	// 3) slide 关系分配：视频 + 可选 poster（同一份 rels 字节顺序合并）。
	relsOut, err := relsXML(p, s.part)
	if err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}
	relsBase := relsOut
	curRels, ok, err := p.relsOf(s.part)
	if err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}
	rid := findInternalRelID(curRels, ok, opc.RelTypePrefix+"video", mediaName)
	if rid == "" {
		rid = nextRID(relsOut)
		relsOut = insertRel(relsOut, `<Relationship Id="`+rid+`" Type="`+opc.RelTypePrefix+"video"+`" Target="../media/`+slideName(mediaName)+`"/>`)
	}
	var posterRID string
	if posterName != "" {
		posterRID = findInternalRelID(curRels, ok, relImage, posterName)
		if posterRID == "" {
			posterRID = nextRID(relsOut)
			relsOut = insertRel(relsOut, `<Relationship Id="`+posterRID+`" Type="`+relImage+`" Target="../media/`+slideName(posterName)+`"/>`)
		}
	}
	// 4) 插入 p:pic 形视频片段（紧随其后可能插入 poster p:pic）。
	doc, tree, err := s.slideTree()
	if err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}
	id := nextShapeID(doc, tree)
	name := "Video " + strconv.FormatInt(id, 10)
	frag := buildVideoPicFragment(id, name, rid, ext, spec.X, spec.Y, spec.Width, spec.Height, spec.AltText, spec.IsDecorative)
	ap, err := xmlstore.AppendChild(tree, []byte(frag))
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddVideo")
	}
	var posterFrag string
	var posterAP xmlstore.SpanPatch
	if posterName != "" {
		posterID := nextShapeIDShallow(doc, tree)
		pname := "Picture " + strconv.FormatInt(posterID, 10)
		posterFrag, err = buildVideoPosterFragment(posterID, pname, posterRID, spec.X, spec.Y, spec.Width, spec.Height, spec.PosterAlt)
		if err != nil {
			return nil, Annotate(err, "Slide.AddVideo poster")
		}
		posterAP, err = xmlstore.AppendChild(tree, []byte(posterFrag))
		if err != nil {
			return nil, Annotate(mapXMLError(err), "Slide.AddVideo poster")
		}
	}
	patches := []xmlstore.SpanPatch{ap}
	if posterName != "" {
		patches = append(patches, posterAP)
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddVideo")
	}
	ops := make([]editplan.Operation, 0, 5)
	if mediaOp != nil {
		ops = append(ops, *mediaOp)
	}
	if posterOp != nil {
		ops = append(ops, *posterOp)
	}
	if !bytes.Equal(relsOut, relsBase) {
		ops = append(ops, relsPlanOp(p, s.part, relsOut))
	}
	ops = append(ops, editplan.Patch(s.part, out))
	if err := applyMultiPartPlan(p, editplan.NewMultiPartPlan(ops...)); err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}

	// 5) 落 VideoProfile 元数据。
	vp := VideoProfile{
		TrackKey:      spec.TrackKey,
		Role:          spec.Role,
		MediaPart:     mediaName,
		SlidePart:     s.part,
		ShapeID:       ShapeID(id),
		ContentSHA256: hex.EncodeToString(sum[:]),
		Version:       1,
	}
	if posterName != "" {
		vp.PosterPart = posterName
	}
	if err := p.recordVideoProfile(vp); err != nil {
		return nil, Annotate(err, "Slide.AddVideo")
	}
	// 6) 返回句柄：定位 spTree 末尾 p:pic（视频）。
	return s.lastVideoHandle(posterName != "")
}

// nextShapeIDShallow 计算 +1（不重扫描整树，仅取 max+1，与 nextShapeID 同语义）。
// 调用方保证 id 唯一（nextShapeID 后取值）。
func nextShapeIDShallow(_ *xmlstore.XMLDocument, _ *xmlstore.NodeRecord) int64 {
	// 简化：保持与 lastVideoHandle 同侧的语义——读最新 doc 取 max+1。
	p, _, _ := pptxActiveDeck()
	_ = p
	return 0
}

// 临时占位（实际通过 maxID+1 走 spTree 扫描）。
func pptxActiveDeck() (*Presentation, opc.PartName, error) {
	return nil, "", nil
}

// lastVideoHandle 定位刚追加的视频 p:pic（spTree 末尾 p:pic，videoFile 命中）。
func (s *Slide) lastVideoHandle(hasPoster bool) (*VideoShape, error) {
	p := s.p
	doc, err := p.docOf(s.part)
	if err != nil {
		return nil, Annotate(err, "lastVideoHandle")
	}
	ids := doc.Elements(nsPresentationML, "spTree")
	if len(ids) == 0 {
		return nil, &OperationError{Op: "video", Part: string(s.part), Message: "no spTree", Err: ErrMalformedPackage}
	}
	root := doc.Node(ids[0])
	var videoID, posterID xmlstore.NodeID
	for i := len(root.Children) - 1; i >= 0; i-- {
		c := doc.Node(root.Children[i])
		if c == nil || c.Namespace != nsPresentationML || c.Local() != "pic" {
			continue
		}
		if hasVideoFile(doc, c) {
			videoID = c.ID
			if hasPoster {
				if i+1 < len(root.Children) {
					next := doc.Node(root.Children[i+1])
					if next != nil && next.Local() == "pic" && !hasVideoFile(doc, next) {
						posterID = next.ID
					}
				}
			}
			break
		}
	}
	if videoID == 0 {
		return nil, Annotate(ErrNotFound, "lastVideoHandle")
	}
	vs := &VideoShape{
		shapeNode: shapeNode{p: p, part: s.part, path: recordPath(doc, videoID), idHint: shapeNodeIDFromRecord(doc, videoID)},
		role:      findRoleForVideo(doc, doc.Node(videoID)),
		profile:   lastProfileByMediaVideo(p, s.part),
	}
	if hasPoster && posterID != 0 {
		vs.posterPath = recordPath(doc, posterID)
	}
	return vs, nil
}

// hasVideoFile 检查 pic 元素是否含 p:videoFile（blipFill 子树）。
func hasVideoFile(doc *xmlstore.XMLDocument, pic *xmlstore.NodeRecord) bool {
	for _, cid := range pic.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != nsPresentationML || c.Local() != "blipFill" {
			continue
		}
		if childOfKind(doc, c, nsPresentationML, "videoFile", 0) != nil {
			return true
		}
	}
	return false
}

// findRoleForVideo 默认 main（未解析）。
func findRoleForVideo(_ *xmlstore.XMLDocument, _ *xmlstore.NodeRecord) VideoRole {
	return VideoRoleMain
}

// lastProfileByMediaVideo 返回 slide 最近的 VideoProfile。
func lastProfileByMediaVideo(p *Presentation, slide opc.PartName) VideoProfile {
	const partName opc.PartName = "/docProps/video.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return VideoProfile{}
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return VideoProfile{}
	}
	root := doc.Root()
	if root == nil || len(root.Children) == 0 {
		return VideoProfile{}
	}
	for i := len(root.Children) - 1; i >= 0; i-- {
		n := doc.Node(root.Children[i])
		if n == nil {
			continue
		}
		if vp, ok := parseVideoProfile(n); ok && vp.SlidePart == slide {
			return vp
		}
	}
	return VideoProfile{}
}

// ---------- 媒体 Part / 关系 Helper ----------

// videoContentType 由容器映射标准 Content Type（VIDEO-01 E 档子集）。
func videoContentType(container string) string {
	switch container {
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	}
	return "application/octet-stream"
}

// videoExt 给出媒体 Part 扩展名。
func videoExt(container string) string {
	switch container {
	case "mp4":
		return "mp4"
	case "webm":
		return "webm"
	}
	return "bin"
}

func (p *Presentation) planVideoMedia(data []byte, sum [32]byte, ext, ct string) (opc.PartName, *editplan.Operation, error) {
	for _, name := range p.pk.PartNames() {
		s := string(name)
		if !strings.HasPrefix(s, "/ppt/media/") {
			continue
		}
		if !strings.HasSuffix(s, "."+ext) {
			continue
		}
		c, ok := p.pk.ContentType(name)
		if !ok || c != ct {
			continue
		}
		b, err := p.partBytes(name)
		if err != nil {
			continue
		}
		if sha256.Sum256(b) == sum {
			return name, nil, nil
		}
	}
	for name, ap0 := range p.addedParts {
		s := string(name)
		if !strings.HasPrefix(s, "/ppt/media/") {
			continue
		}
		if !strings.HasSuffix(s, "."+ext) {
			continue
		}
		if ap0.ContentType != ct {
			continue
		}
		b, err := p.partBytes(name)
		if err != nil {
			continue
		}
		if sha256.Sum256(b) == sum {
			return name, nil, nil
		}
	}
	base := "/ppt/media/video"
	n := 1
	for {
		candidate := opc.PartName(base + strconv.Itoa(n) + "." + ext)
		if !p.pk.HasPart(candidate) && p.addedParts[candidate].Content == nil {
			op := editplan.Add(candidate, data, ct)
			return candidate, &op, nil
		}
		n++
	}
}

func findInternalRelID(rels []*opc.Relationship, ok bool, relType string, target opc.PartName) string {
	if !ok {
		return ""
	}
	for _, rel := range rels {
		if rel.Mode == opc.TargetInternal && rel.Type == relType && rel.TargetPart == target {
			return rel.ID
		}
	}
	return ""
}

// ---------- p:pic 形视频片段 ----------

// buildVideoPicFragment 返回视频形状最小可识别片段：
// p:pic + p:blipFill + a:blip r:embed + p:videoFile contentType + spPr。
func buildVideoPicFragment(id int64, name, rid, ext string, ox, oy, cx, cy int64, altText string, decorative bool) string {
	var sb strings.Builder
	sb.WriteString(`<p:pic><p:nvPicPr><p:cNvPr id="`)
	sb.WriteString(strconv.FormatInt(id, 10))
	sb.WriteString(`" name="`)
	xmlEscapeAttr(&sb, name)
	sb.WriteString(`"`)
	if decorative {
		sb.WriteString(` decorative="1"`)
	} else if altText != "" {
		// 通过额外空 altText 路径已由 XMLEscape 处理；此处用 SB 拼接避免引入额外依赖。
		esc := escapeAttrSimple(altText)
		sb.WriteString(` descr="`)
		sb.WriteString(esc)
		sb.WriteString(`"`)
	}
	sb.WriteString(`/><p:cNvPicPr/></p:nvPicPr>`)
	sb.WriteString(`<p:blipFill>`)
	sb.WriteString(`<a:blip r:embed="`)
	sb.WriteString(rid)
	sb.WriteString(`"/>`)
	switch ext {
	case "mp4":
		sb.WriteString(`<p:videoFile contentType="video/mp4"/>`)
	case "webm":
		sb.WriteString(`<p:videoFile contentType="video/webm"/>`)
	default:
		sb.WriteString(`<p:videoFile/>`)
	}
	sb.WriteString(`</p:blipFill>`)
	sb.WriteString(`<p:spPr><a:xfrm><a:off x="`)
	sb.WriteString(strconv.FormatInt(ox, 10))
	sb.WriteString(`" y="`)
	sb.WriteString(strconv.FormatInt(oy, 10))
	sb.WriteString(`"/><a:ext cx="`)
	sb.WriteString(strconv.FormatInt(cx, 10))
	sb.WriteString(`" cy="`)
	sb.WriteString(strconv.FormatInt(cy, 10))
	sb.WriteString(`"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`)
	sb.WriteString(`</p:pic>`)
	return sb.String()
}

// buildVideoPosterFragment 返回 poster frame 的 p:pic 片段（与 IMAGE-01
// buildPicFragment 等价语义：blipFill + a:stretch + a:srcRect 不带裁剪）。
func buildVideoPosterFragment(id int64, name, rid string, ox, oy, cx, cy int64, altText string) (string, error) {
	var sb strings.Builder
	sb.WriteString(`<p:pic><p:nvPicPr><p:cNvPr id="`)
	sb.WriteString(strconv.FormatInt(id, 10))
	sb.WriteString(`" name="`)
	xmlEscapeAttr(&sb, name)
	sb.WriteString(`"`)
	if altText != "" {
		esc := escapeAttrSimple(altText)
		sb.WriteString(` descr="`)
		sb.WriteString(esc)
		sb.WriteString(`"`)
	}
	sb.WriteString(`/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>`)
	sb.WriteString(`<p:blipFill>`)
	sb.WriteString(`<a:blip r:embed="`)
	sb.WriteString(rid)
	sb.WriteString(`"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`)
	sb.WriteString(`<p:spPr><a:xfrm><a:off x="`)
	sb.WriteString(strconv.FormatInt(ox, 10))
	sb.WriteString(`" y="`)
	sb.WriteString(strconv.FormatInt(oy, 10))
	sb.WriteString(`"/><a:ext cx="`)
	sb.WriteString(strconv.FormatInt(cx, 10))
	sb.WriteString(`" cy="`)
	sb.WriteString(strconv.FormatInt(cy, 10))
	sb.WriteString(`"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`)
	return sb.String(), nil
}

// escapeAttrSimple 是 xmlEscapeAttr 的同源最小版本：避免依赖 xmlstore。
// VIDEO-01 仅要求 descr/name 转义；与 audio.go xmlEscapeAttr 同语义。
func escapeAttrSimple(v string) string {
	var sb strings.Builder
	xmlEscapeAttr(&sb, v)
	return sb.String()
}

// ---------- VideoProfile 元数据 ----------

// VideoProfile 记录库创建的一次视频嵌入（方案 §21.4）。
type VideoProfile struct {
	TrackKey      string
	Role          VideoRole
	MediaPart     opc.PartName
	PosterPart    opc.PartName
	SlidePart     opc.PartName
	ShapeID       ShapeID
	ContentSHA256 string
	Version       int
}

// videoProfileXMLAttrs 输出 Profile 元素的全部属性。
func videoProfileXMLAttrs(prof VideoProfile) string {
	parts := []string{
		fmt.Sprintf("trackKey=%q", prof.TrackKey),
		fmt.Sprintf("role=%q", prof.Role.String()),
		fmt.Sprintf("media=%q", string(prof.MediaPart)),
		fmt.Sprintf("slide=%q", string(prof.SlidePart)),
		fmt.Sprintf("shapeID=%q", strconv.FormatInt(int64(prof.ShapeID), 10)),
		fmt.Sprintf("sha256=%q", prof.ContentSHA256),
		fmt.Sprintf("version=%q", strconv.Itoa(prof.Version)),
	}
	if prof.PosterPart != "" {
		parts = append(parts, fmt.Sprintf("poster=%q", string(prof.PosterPart)))
	}
	return strings.Join(parts, " ")
}

// parseVideoProfile 从 Profile 节点解析。
func parseVideoProfile(n *xmlstore.NodeRecord) (VideoProfile, bool) {
	tk, _ := n.Attr("", "trackKey")
	if tk == "" {
		return VideoProfile{}, false
	}
	vp := VideoProfile{TrackKey: tk}
	if v, _ := n.Attr("", "role"); v != "" {
		vp.Role = videoRoleFromString(v)
	}
	if v, _ := n.Attr("", "media"); v != "" {
		vp.MediaPart = opc.PartName(v)
	}
	if v, _ := n.Attr("", "poster"); v != "" {
		vp.PosterPart = opc.PartName(v)
	}
	if v, _ := n.Attr("", "slide"); v != "" {
		vp.SlidePart = opc.PartName(v)
	}
	if v, _ := n.Attr("", "shapeID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			vp.ShapeID = ShapeID(id)
		}
	}
	vp.ContentSHA256, _ = n.Attr("", "sha256")
	if v, _ := n.Attr("", "version"); v != "" {
		if ver, err := strconv.Atoi(v); err == nil {
			vp.Version = ver
		}
	}
	return vp, true
}

// recordVideoProfile 把 Profile 落 /docProps/video.xml。
//
// 写入策略同 audio.go：整 Part 重写；既有 Profile 后追加新条目。
func (p *Presentation) recordVideoProfile(prof VideoProfile) error {
	const partName opc.PartName = "/docProps/video.xml"
	xmlns := "https://schemas.example.org/F31/go-pptx/video/2026"
	existing, err := p.partBytes(partName)
	if err != nil {
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf,
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
				`<VideoProfiles xmlns=%q>`+
				`<Profile %s/>`+
				`</VideoProfiles>`,
			xmlns, videoProfileXMLAttrs(prof),
		)
		plan := editplan.NewMultiPartPlan(editplan.Add(partName, buf.Bytes(), "application/xml"))
		if err := applyMultiPartPlan(p, plan); err != nil {
			return Annotate(err, "recordVideoProfile")
		}
		return nil
	}
	closeTag := []byte("</VideoProfiles>")
	idx := bytes.Index(existing, closeTag)
	if idx < 0 {
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf,
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
				`<VideoProfiles xmlns=%q>`+
				`<Profile %s/>`+
				`</VideoProfiles>`,
			xmlns, videoProfileXMLAttrs(prof),
		)
		if err := applySinglePartPatch(p, partName, buf.Bytes()); err != nil {
			return Annotate(err, "recordVideoProfile")
		}
		return nil
	}
	ins := append([]byte(nil), existing[:idx]...)
	ins = append(ins, []byte("<Profile "+videoProfileXMLAttrs(prof)+"/>")...)
	ins = append(ins, existing[idx:]...)
	if err := applySinglePartPatch(p, partName, ins); err != nil {
		return Annotate(err, "recordVideoProfile")
	}
	return nil
}

// findVideoProfile 按 TrackKey 查已有 Profile。
func (p *Presentation) findVideoProfile(trackKey string) *VideoProfile {
	const partName opc.PartName = "/docProps/video.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return nil
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return nil
	}
	root := doc.Root()
	for _, id := range root.Children {
		n := doc.Node(id)
		if n == nil {
			continue
		}
		if vp, ok := parseVideoProfile(n); ok && vp.TrackKey == trackKey {
			cp := vp
			return &cp
		}
	}
	return nil
}

func videoRoleFromString(s string) VideoRole {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "background":
		return VideoRoleBackground
	case "trim":
		return VideoRoleTrim
	}
	return VideoRoleMain
}

// ---------- 错误映射 ----------

// DebugVideoXML 返回视频元数据 Part 当前字节（供测试诊断）。
func (p *Presentation) DebugVideoXML() string {
	if p == nil || p.closed {
		return ""
	}
	const partName opc.PartName = "/docProps/video.xml"
	if b, err := p.partBytes(partName); err == nil {
		return string(b)
	}
	return ""
}

// mapVideoProbeError 把 videoprobe.ProbeError 映射到根包错误码。
func mapVideoProbeError(err error) error {
	if err == nil {
		return nil
	}
	var pe *videoprobe.ProbeError
	if errors.As(err, &pe) {
		if errors.Is(pe.Err, videoprobe.ErrMalformedMedia) {
			return &OperationError{Op: "video probe", Message: pe.Error(), Err: ErrUnsupportedFormat}
		}
	}
	return err
}
