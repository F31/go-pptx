// 本文件实现 AUDIO-01（方案 §21.1/§21.4）的音频嵌入与元数据：
//   - AddAudio 嵌入媒体 Part（MP3/WAV）+ 关系；图标/后备表示暂以
//     p:pic + audio:file 形式落 OOXML（PowerPoint 兼容）。
//   - AudioProfile 记录库创建音轨（TrackKey/Role/ShapeID/Version/
//     ContentSHA256）落自定义元数据 `/docProps/audio.xml` 与
//     `custom.xml` 同侧的私有命名空间扩展。
//   - AudioShape 句柄（AltText/IsDecorative/Source）。
//
// 已知约束（不在 AUDIO-01 范围）：
//   - AUDIO-02 SetPlayback 与计时树编辑（独立工作包）；
//   - AUDIO-03 PlanTimingSync/ApplyTimingPlan（独立工作包）；
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
	"time"

	"github.com/F31/go-pptx/internal/audioprobe"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- 公共类型 ----------

// AudioRole 是音频在页面上的语义角色（方案 §21.1）。
type AudioRole int

const (
	// AudioRoleNarration 是讲解音轨，参与 PlanTimingSync 翻页计算。
	AudioRoleNarration AudioRole = iota
	// AudioRoleBackground 是背景音，不参与翻页计算。
	AudioRoleBackground
	// AudioRoleEffect 是音效，不参与翻页计算。
	AudioRoleEffect
)

func (r AudioRole) String() string {
	switch r {
	case AudioRoleNarration:
		return "narration"
	case AudioRoleBackground:
		return "background"
	case AudioRoleEffect:
		return "effect"
	}
	return fmt.Sprintf("AudioRole(%d)", int(r))
}

// AudioSpec 描述要嵌入或更新的音频。
//
// TrackKey 是调用方在当前文档内的稳定业务标识（同一 TrackKey 重复
// 合成走幂等路径，方案 §21.1/§21.4 AT-08）。Role/Source/Duration
// 必填；Duration 提供时优先（CallerProvided），缺失则由内部 probe 推导。
type AudioSpec struct {
	TrackKey string
	Role     AudioRole
	Source   MediaSource
	// Duration 可选：调用方已知时长。Set=false 时由 probe 解出；probe
	// 也无法解出返回 ErrDurationUnknown（同步与 AddAudio 均受影响）。
	Duration Optional[time.Duration]
}

// AudioShape 是页面音频形状的受控句柄（实现 Shape 接口）。
type AudioShape struct {
	shapeNode
	role    AudioRole
	profile AudioProfile // 嵌入时的 Profile（AddAudio 写入；UpsertNarration 复用更新）
}

// Kind 返回形状类别：恒为 ShapeAudio（classifyShape 在读取时按
// blipFill 子树 a:audioFile 命中此值）。
func (a *AudioShape) Kind() ShapeKind { return ShapeAudio }

// Role 返回音频角色。
func (a *AudioShape) Role() AudioRole { return a.role }

// AudioSource 返回（嵌入时）经库重组后的媒体源（运行时回放探测）。
// 关闭文档后句柄失效。
func (a *AudioShape) AudioSource() (MediaSource, error) {
	if a.p == nil || a.p.closed {
		return nil, Annotate(ErrClosed, "AudioShape.AudioSource")
	}
	if a.profile.MediaPart == "" {
		return nil, Annotate(ErrNotFound, "AudioShape.AudioSource")
	}
	return sourceRef{part: a.profile.MediaPart, ct: audioCTForExt(extOfMedia(a.profile.MediaPart)), p: a.p}, nil
}

// Profile 返回本 shape 关联的 AudioProfile（库创建音轨）。
func (a *AudioShape) Profile() AudioProfile { return a.profile }

// extOfMedia 从 media part 提取扩展名（"." 之后）。
func extOfMedia(name opc.PartName) string {
	s := string(name)
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return ""
}

// audioCTForExt 给出常见扩展 → Content Type。
func audioCTForExt(ext string) string {
	switch strings.ToLower(ext) {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	}
	return "application/octet-stream"
}

// ---------- AddAudio ----------

// debugAddedParts 列出当前已添加/修改 Part（仅测试用）。
func (p *Presentation) debugAddedParts() []string {
	var out []string
	for name := range p.addedParts {
		out = append(out, string(name))
	}
	for name := range p.overrides {
		out = append(out, "patch:"+string(name))
	}
	return out
}

// AddAudio 在页面上嵌入音频：基于源字节探测类型与时长（若调用方未指定），// 创建媒体 Part 与页面关系，插入 p:pic 形式的音频形状，记录 AudioProfile。
//
// 同一 TrackKey 重复调用返回 ErrUnsupportedEdit（AT-08 语义由
// UpsertNarration 覆盖，AddAudio 仅处理"新增"语义）。
func (s *Slide) AddAudio(ctx context.Context, src MediaSource, spec AudioSpec) (*AudioShape, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	if src == nil {
		return nil, Annotate(ErrInvalidArgument, "Slide.AddAudio src")
	}
	if spec.TrackKey == "" {
		return nil, &OperationError{Op: "Slide.AddAudio", Message: "TrackKey is required", Err: ErrInvalidArgument}
	}
	if strings.ContainsAny(spec.TrackKey, "/\\") {
		return nil, &OperationError{Op: "Slide.AddAudio", Message: "TrackKey contains path separator", Err: ErrInvalidArgument}
	}
	// 拒绝库已有重复 TrackKey（详见 AudioProfile 检查）。
	p := s.p
	existing := p.findAudioProfile(spec.TrackKey)
	if existing != nil {
		return nil, &OperationError{
			Op:      "Slide.AddAudio",
			Message: fmt.Sprintf("TrackKey %q already exists, use UpsertNarration to update", spec.TrackKey),
			Err:     ErrUnsupportedEdit,
		}
	}
	// 1) 暂存并探测媒体。
	data, err := readMedia(ctx, src)
	if err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	if len(data) == 0 {
		return nil, &OperationError{Op: "Slide.AddAudio", Message: "empty audio bytes", Err: ErrInvalidArgument}
	}
	info, err := audioprobe.Probe(audioprobe.ProbeInput{
		Data:          data,
		Declared:      src.DeclaredType(),
		ContainerHint: hintFromName(slideMediaName(spec.TrackKey)),
	})
	if err != nil {
		return nil, Annotate(mapProbeError(err), "Slide.AddAudio")
	}
	ct := audioContentType(info)
	ext := audioExt(info)
	// 2) 时长：CallerProvided 优先；未指定走 probe 结果。
	if spec.Duration.Set {
		// 调用方已提供；保留。
	} else if info.State == audioprobe.DurationKnown {
		spec.Duration = Optional[time.Duration]{Value: time.Duration(info.Duration.Nanoseconds), Set: true}
	} else {
		// 未知时长且调用方未提供：拒绝整个调用，避免无声嵌入。
		return nil, &OperationError{
			Op: "Slide.AddAudio", Message: "audio duration unknown and not provided",
			Err: ErrDurationUnknown,
		}
	}

	// 3) 媒体 Part 暂存（含 SHA-256 去重，content type 决定扩展）。
	sum := sha256.Sum256(data)
	mediaName, err := p.stageAudioMedia(data, sum, ext, ct)
	if err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	// 4) slide 关系指向媒体。
	rid, err := p.addAudioRel(s.part, mediaName, ext, ct)
	if err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	// 5) 插入 p:pic 形式的音频形状。
	doc, tree, err := s.slideTree()
	if err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	id := nextShapeID(doc, tree)
	name := "Audio " + strconv.FormatInt(id, 10)
	frag := buildAudioPicFragment(id, name, rid, ext)
	ap, err := xmlstore.AppendChild(tree, []byte(frag))
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddAudio")
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{ap})
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddAudio")
	}
	if err := p.stagePatch(s.part, out); err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	p.commit()

	// 6) 落 AudioProfile 元数据（私有 Part + custom.xml 引用）。
	ap0 := AudioProfile{
		TrackKey:      spec.TrackKey,
		Role:          spec.Role,
		MediaPart:     mediaName,
		SlidePart:     s.part,
		ShapeID:       ShapeID(id),
		Duration:      spec.Duration.Value,
		Trigger:       PlaybackOnSlideEnter,
		ContentSHA256: hex.EncodeToString(sum[:]),
		Version:       1,
	}
	if err := p.recordAudioProfile(ap0); err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	// 7) 返回句柄：定位 spTree 末尾 p:pic（再次解析已提交字节）。
	as, err := s.lastAudioHandle()
	if err != nil {
		return nil, Annotate(err, "Slide.AddAudio")
	}
	return as, nil
}

// lastAudioHandle 在 spTree 末尾定位刚追加的 audio p:pic 元素。
//
// AddAudio 通过 fragment AppendChild + 单事务提交，doc 缓存会在 commit
// 时失效；这里重新 docOf 拿最新文档，按 z-order 末尾 p:pic 句柄化。
func (s *Slide) lastAudioHandle() (*AudioShape, error) {
	p := s.p
	doc, err := p.docOf(s.part)
	if err != nil {
		return nil, err
	}
	ids := doc.Elements(nsPresentationML, "spTree")
	if len(ids) == 0 {
		return nil, &OperationError{
			Op: "audio", Part: string(s.part),
			Message: "no spTree", Err: ErrMalformedPackage,
		}
	}
	tree := doc.Node(ids[0])
	for i := len(tree.Children) - 1; i >= 0; i-- {
		c := doc.Node(tree.Children[i])
		if c == nil || c.Namespace != nsPresentationML {
			continue
		}
		if c.Local() != "pic" {
			continue
		}
		// 收集 cNvPr id 与 name，构造 handlePath。
		cn := doc.Node(c.Children[0]) // nvPicPr
		if cn == nil {
			continue
		}
		// nvPicPr > cNvPr 子元素（前 0）。
		cnvpr := childOfKind(doc, cn, nsPresentationML, "cNvPr", 0)
		if cnvpr == nil {
			continue
		}
		idv, _ := cnvpr.Attr("", "id")
		namev, _ := cnvpr.Attr("", "name")
		_ = idv
		_ = namev
		return &AudioShape{
			shapeNode: shapeNode{p: p, part: s.part, path: recordPath(doc, c.ID)},
			role:      findRoleForAudio(c, doc),
			profile:   lastProfileByMedia(p),
		}, nil
	}
	return nil, Annotate(ErrNotFound, "audio handle not found")
}

// findRoleForAudio 是当前简化版本：默认叙事（未解析）。
func findRoleForAudio(_ *xmlstore.NodeRecord, _ *xmlstore.XMLDocument) AudioRole {
	return AudioRoleNarration
}

// lastProfileByMedia 返回最近一次 recordAudioProfile 提交的 Profile。
// 简单实现：扫描 /docProps/audio.xml 的最后一个 Profile 节点。
func lastProfileByMedia(p *Presentation) AudioProfile {
	const partName opc.PartName = "/docProps/audio.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return AudioProfile{}
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return AudioProfile{}
	}
	root := doc.Root()
	if len(root.Children) == 0 {
		return AudioProfile{}
	}
	n := doc.Node(root.Children[len(root.Children)-1])
	if n == nil {
		return AudioProfile{}
	}
	tk, _ := n.Attr("", "trackKey")
	role, _ := n.Attr("", "role")
	media, _ := n.Attr("", "media")
	sha, _ := n.Attr("", "sha256")
	shapeID, _ := n.Attr("", "shapeID")
	ver, _ := n.Attr("", "version")
	ap := AudioProfile{
		TrackKey:      tk,
		Role:          roleFromString(role),
		MediaPart:     opc.PartName(media),
		ContentSHA256: sha,
	}
	if id, err := strconv.ParseInt(shapeID, 10, 64); err == nil {
		ap.ShapeID = ShapeID(id)
	}
	if v, err := strconv.Atoi(ver); err == nil {
		ap.Version = v
	}
	return ap
}

// ---------- 媒体 Part / 关系 Helper ----------

// audioContentType 由探测结果映射标准 Content Type。
func audioContentType(info audioprobe.MediaInfo) string {
	switch info.Container {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	}
	return "application/octet-stream"
}

// audioExt 给出媒体 Part 扩展名（与 imageTypeEqual 一类处理）。
func audioExt(info audioprobe.MediaInfo) string {
	switch info.Container {
	case "wav":
		return "wav"
	case "mp3":
		return "mp3"
	}
	return "bin"
}

// hintFromName 从给定的 Part 名得出容器提示（仅后缀）。
func hintFromName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return ""
}

// slideMediaName 暂存时为媒体 Part 生成会话稳定的占位名（用于 hintFromName）。
func slideMediaName(key string) string {
	return "/ppt/media/" + key + ".tmp"
}

// stageAudioMedia 把音频字节暂存为媒体 Part；同 SHA-256 + ContentType 复用。
func (p *Presentation) stageAudioMedia(data []byte, sum [32]byte, ext, ct string) (opc.PartName, error) {
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
			return name, nil
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
			return name, nil
		}
	}
	base := "/ppt/media/audio"
	n := 1
	for {
		candidate := opc.PartName(base + strconv.Itoa(n) + "." + ext)
		if !p.pk.HasPart(candidate) && p.addedParts[candidate].Content == nil {
			if err := p.stageAdd(candidate, data, ct); err != nil {
				return "", err
			}
			return candidate, nil
		}
		n++
	}
}

// addAudioRel 为 slide 创建指向 audio media 的关系，遵循现有图片逻辑（rId
// 复用同目标同类型）。复用现有的 relsXML/insertRel/nextRID 助手。
func (p *Presentation) addAudioRel(slide, media opc.PartName, ext, ct string) (string, error) {
	relType := opc.RelTypePrefix + "audio"
	rels, ok, err := p.relsOf(slide)
	if err != nil {
		return "", err
	}
	if ok {
		for _, rel := range rels {
			if rel.Mode == opc.TargetInternal && rel.Type == relType && rel.TargetPart == media {
				return rel.ID, nil
			}
		}
	}
	xml, err := relsXML(p, slide)
	if err != nil {
		return "", err
	}
	rid := nextRID(xml)
	entry := `<Relationship Id="` + rid + `" Type="` + relType +
		`" Target="../media/` + slideName(media) + `"/>`
	updated := insertRel(xml, entry)
	if err := stageRelsBytes(p, slide, updated); err != nil {
		return "", err
	}
	return rid, nil
}

// ---------- p:pic 形音频片段 ----------

// buildAudioPicFragment 返回最小可识别片段：
// p:pic 含 blipFill + a:audioFile（powerpoint 接受的 audio 形状表达之一）。
func buildAudioPicFragment(id int64, name, rid, ext string) string {
	var sb strings.Builder
	sb.WriteString(`<p:pic>`)
	sb.WriteString(`<p:nvPicPr><p:cNvPr id="`)
	sb.WriteString(strconv.FormatInt(id, 10))
	sb.WriteString(`" name="`)
	xmlEscapeAttr(&sb, name)
	sb.WriteString(`"/><p:cNvPicPr/></p:nvPicPr>`)
	sb.WriteString(`<p:blipFill>`)
	sb.WriteString(`<a:blip r:embed="`)
	sb.WriteString(rid)
	sb.WriteString(`"/>`)
	if ext == "mp3" {
		sb.WriteString(`<a:audioFile contentType="audio/mpeg"/>`)
	} else if ext == "wav" {
		sb.WriteString(`<a:audioFile contentType="audio/wav"/>`)
	} else {
		sb.WriteString(`<a:audioFile/>`)
	}
	sb.WriteString(`</p:blipFill>`)
	// 占位几何：1×1 EMU；调用方一般随后调 Geometry/Move。
	sb.WriteString(`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`)
	sb.WriteString(`</p:pic>`)
	return sb.String()
}

// xmlEscapeAttr 写一个属性值（双重 escape 走 xmlstore 但本片段全文 escape）。
func xmlEscapeAttr(sb *strings.Builder, v string) {
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch c {
		case '"':
			sb.WriteString("&quot;")
		case '&':
			sb.WriteString("&amp;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		default:
			sb.WriteByte(c)
		}
	}
}

// ---------- AudioProfile 元数据 ----------

// AudioProfile 记录库创建的一次音频嵌入（方案 §21.4）。
type AudioProfile struct {
	TrackKey  string
	Role      AudioRole
	MediaPart opc.PartName
	// SlidePart 是音轨所在 slide Part（PlanTimingSync 按页聚合用）。
	SlidePart opc.PartName
	ShapeID   ShapeID
	// Duration 是音轨时长（CallerProvided 或 probe 推导；0 = 未知）。
	Duration time.Duration
	// StartDelay 是播放起始偏移（SetPlayback 写入）。
	StartDelay time.Duration
	// Trigger 是触发方式（SetPlayback 写入；默认 OnSlideEnter）。
	Trigger       PlaybackTrigger
	ContentSHA256 string
	Version       int
}

// profileXMLAttrs 输出 Profile 元素的全部属性（读写两侧共享同一名集）。
func profileXMLAttrs(prof AudioProfile) string {
	return fmt.Sprintf(
		`trackKey=%q role=%q media=%q slide=%q shapeID=%q durMs=%q stMs=%q trigger=%q sha256=%q version=%q`,
		prof.TrackKey, prof.Role.String(), string(prof.MediaPart), string(prof.SlidePart),
		strconv.FormatInt(int64(prof.ShapeID), 10),
		strconv.FormatInt(prof.Duration.Milliseconds(), 10),
		strconv.FormatInt(prof.StartDelay.Milliseconds(), 10),
		prof.Trigger.String(),
		prof.ContentSHA256, strconv.Itoa(prof.Version),
	)
}

// parseAudioProfile 从 Profile 元素节点解析 AudioProfile。
func parseAudioProfile(n *xmlstore.NodeRecord) (AudioProfile, bool) {
	tk, ok := n.Attr("", "trackKey")
	if !ok || tk == "" {
		return AudioProfile{}, false
	}
	ap := AudioProfile{TrackKey: tk}
	if v, _ := n.Attr("", "role"); v != "" {
		ap.Role = roleFromString(v)
	}
	if v, _ := n.Attr("", "media"); v != "" {
		ap.MediaPart = opc.PartName(v)
	}
	if v, _ := n.Attr("", "slide"); v != "" {
		ap.SlidePart = opc.PartName(v)
	}
	if v, _ := n.Attr("", "shapeID"); v != "" {
		if sid, err := strconv.ParseInt(v, 10, 64); err == nil {
			ap.ShapeID = ShapeID(sid)
		}
	}
	if v, _ := n.Attr("", "durMs"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			ap.Duration = time.Duration(ms) * time.Millisecond
		}
	}
	if v, _ := n.Attr("", "stMs"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			ap.StartDelay = time.Duration(ms) * time.Millisecond
		}
	}
	switch v, _ := n.Attr("", "trigger"); v {
	case "onClick":
		ap.Trigger = PlaybackOnClick
	default:
		ap.Trigger = PlaybackOnSlideEnter
	}
	ap.ContentSHA256, _ = n.Attr("", "sha256")
	if v, _ := n.Attr("", "version"); v != "" {
		if ver, err := strconv.Atoi(v); err == nil {
			ap.Version = ver
		}
	}
	return ap, true
}

// recordAudioProfile 把 Profile 落 `/docProps/audio.xml` 自有命名空间扩展。
//
// 设计要点：每次重写整 Part（读 → 修改 → 全量写入）。多 Profile 累积
// 通过扫描现有 Profile 元素后追加新元素实现。Part 首次创建时插入骨架
// 单 Profile；后续每次 stagePatch+commit 把完整内容提交。
func (p *Presentation) recordAudioProfile(prof AudioProfile) error {
	const partName opc.PartName = "/docProps/audio.xml"
	xmlns := `https://schemas.example.org/F31/go-pptx/audio/2026`
	existing, err := p.partBytes(partName)
	if err != nil {
		// Part 不存在 → 直接 stageAdd 全量内容。
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf,
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
				`<AudioProfiles xmlns=%q>`+
				`<Profile %s/>`+
				`</AudioProfiles>`,
			xmlns, profileXMLAttrs(prof),
		)
		if err := p.stageAdd(partName, buf.Bytes(), "application/xml"); err != nil {
			return Annotate(err, "recordAudioProfile")
		}
		p.commit()
		return nil
	}
	// 既有 → 在 </AudioProfiles> 前插入新 Profile，再 stagePatch 整 Part。
	closeTag := []byte("</AudioProfiles>")
	idx := bytes.Index(existing, closeTag)
	if idx < 0 {
		// 既有内容损坏：重置为仅含本条目。
		buf := bytes.NewBuffer(nil)
		fmt.Fprintf(buf,
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
				`<AudioProfiles xmlns=%q>`+
				`<Profile %s/>`+
				`</AudioProfiles>`,
			xmlns, profileXMLAttrs(prof),
		)
		if err := p.stagePatch(partName, buf.Bytes()); err != nil {
			return Annotate(err, "recordAudioProfile")
		}
		p.commit()
		return nil
	}
	ins := append([]byte(nil), existing[:idx]...)
	ins = append(ins, []byte("<Profile "+profileXMLAttrs(prof)+"/>")...)
	ins = append(ins, existing[idx:]...)
	if err := p.stagePatch(partName, ins); err != nil {
		return Annotate(err, "recordAudioProfile")
	}
	p.commit()
	return nil
}

// updateAudioProfile 按 TrackKey 修改 Profile（mutate 就地改写后全量重写
// 该 Part）。不存在返回 ErrNotFound。
func (p *Presentation) updateAudioProfile(trackKey string, mutate func(*AudioProfile)) error {
	const partName opc.PartName = "/docProps/audio.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return Annotate(err, "updateAudioProfile")
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return Annotate(err, "updateAudioProfile")
	}
	root := doc.Root()
	found := false
	out := bytes.NewBuffer(nil)
	out.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<AudioProfiles xmlns="https://schemas.example.org/F31/go-pptx/audio/2026">`)
	for _, id := range root.Children {
		n := doc.Node(id)
		if n == nil {
			continue
		}
		prof, ok := parseAudioProfile(n)
		if !ok {
			continue
		}
		if prof.TrackKey == trackKey {
			mutate(&prof)
			found = true
		}
		out.WriteString("<Profile " + profileXMLAttrs(prof) + "/>")
	}
	out.WriteString(`</AudioProfiles>`)
	if !found {
		return Annotate(ErrNotFound, "updateAudioProfile")
	}
	if err := p.stagePatch(partName, out.Bytes()); err != nil {
		return Annotate(err, "updateAudioProfile")
	}
	p.commit()
	return nil
}

// audioProfilesOfSlide 返回挂在指定 slide Part 上的全部 Profile。
func (p *Presentation) audioProfilesOfSlide(slide opc.PartName) []AudioProfile {
	const partName opc.PartName = "/docProps/audio.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return nil
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return nil
	}
	root := doc.Root()
	var out []AudioProfile
	for _, id := range root.Children {
		n := doc.Node(id)
		if n == nil {
			continue
		}
		if prof, ok := parseAudioProfile(n); ok && prof.SlidePart == slide {
			out = append(out, prof)
		}
	}
	return out
}

// findAudioProfile 按 TrackKey 查已有 Profile。
func (p *Presentation) findAudioProfile(trackKey string) *AudioProfile {
	const partName opc.PartName = "/docProps/audio.xml"
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
		if prof, ok := parseAudioProfile(n); ok && prof.TrackKey == trackKey {
			cp := prof
			return &cp
		}
	}
	return nil
}

func roleFromString(s string) AudioRole {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "narration":
		return AudioRoleNarration
	case "background":
		return AudioRoleBackground
	case "effect":
		return AudioRoleEffect
	}
	return AudioRoleNarration
}

// findAudioByShape 简单实现预留（已使用 AudioShape.profile 字段）。
// 该函数保留是为后续多形状索引做准备。
func (p *Presentation) findAudioByShape(slide opc.PartName, path []nodeStep) *AudioProfile {
	_ = slide
	_ = path
	return nil
}

// ---------- 错误映射 ----------

// DebugAudioXML 返回音频元数据 Part 当前字节（供测试诊断）。
func (p *Presentation) DebugAudioXML() string {
	if p == nil || p.closed {
		return ""
	}
	const partName opc.PartName = "/docProps/audio.xml"
	if b, err := p.partBytes(partName); err == nil {
		return string(b)
	}
	return ""
}

// mapProbeError 把 a audioprobe.ProbeError / ErrMalformedMedia 映射到根包错误码。
func mapProbeError(err error) error {
	if err == nil {
		return nil
	}
	var pe *audioprobe.ProbeError
	if errors.As(err, &pe) {
		switch {
		case errors.Is(pe.Err, audioprobe.ErrMalformedMedia):
			return &OperationError{Op: "audio probe", Message: pe.Message, Err: ErrUnsupportedFormat}
		}
	}
	return err
}

// sourceRef 是 AudioShape.AudioSource 的返回值占位（运行时创建）。
// 真实实现由 AUDIO-02/03 接入，本文件保证接口存在。
type sourceRef struct {
	// 字节（已暂存）+ 媒体名 + contentType。
	part opc.PartName
	ct   string
	p    *Presentation
}

// Open 返回字节流的拷贝读端（Reader 关闭后不影响源）。
// 为简便起见包级使用 io.NopCloser/bytes.NewReader。
func (s sourceRef) Open(ctx context.Context) (io.ReadCloser, error) {
	if s.p == nil || s.p.closed {
		return nil, Annotate(ErrClosed, "audio source")
	}
	b, err := s.p.partBytes(s.part)
	if err != nil {
		return nil, Annotate(err, "audio source")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// DeclaredType 返回 Content Type。
func (s sourceRef) DeclaredType() string { return s.ct }
