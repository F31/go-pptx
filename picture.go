package pptx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 IMAGE-01 的图片形状（方案 §8/§20.2 图片子集）：
//
//   - PNG/JPEG 添加（Slide.AddPicture）与替换（PictureShape.ReplaceImage）；
//   - 四种放置模式（PictureFitMode）：原尺寸（96 dpi 换算 EMU）、拉伸、
//     保持比例（Contain）、裁剪填充（Cover，经 a:srcRect 表达，不重采样）；
//   - 默认保留源媒体字节（不隐式重采样），SVG/EMF/WMF 等未支持（明确报错）；
//   - 媒体暂存与共享引用保护：内容哈希去重限定为已确认安全的图片类型；
//     替换时旧媒体仅在其不再被任何关系引用时删除，绝不破坏共享引用。
//
// 无障碍元数据（§8.1）：PictureSpec 携带 AltText/IsDecorative；IsDecorative
// 写入 p:cNvPr@decorative（规范装饰标记），AltText 写入 @descr；两者语义
// 不同，不用空 descr 代替装饰性声明。读写实现与 AutoShape 共用
// shapes.go 的通用形状句柄基元（shapeNode）。

// emuPerPixel96 是 96 dpi 下 1 像素的 EMU 值（914400/96）。
const emuPerPixel96 = int64(9525)

// PictureFitMode 决定图片放入给定框的几何策略（首版不重采样）。
type PictureFitMode int

const (
	// FitOriginalSize 按像素原尺寸（96 dpi）放置；忽略 Width/Height。
	FitOriginalSize PictureFitMode = iota
	// FitStretch 拉伸至 Width×Height（允许变形，同 PowerPoint 拉伸图片）。
	FitStretch
	// FitContain 在 Width×Height 框内保持宽高比完整容纳（不裁剪），
	// 图形按比例缩小并居中于给定框。
	FitContain
	// FitCover 裁剪填充：源图等比放大覆盖 Width×Height 框后居中裁剪
	// 超出部分（a:srcRect），画面不变形。
	FitCover
)

func (m PictureFitMode) String() string {
	switch m {
	case FitOriginalSize:
		return "FitOriginalSize"
	case FitStretch:
		return "FitStretch"
	case FitContain:
		return "FitContain"
	case FitCover:
		return "FitCover"
	}
	return fmt.Sprintf("PictureFitMode(%d)", int(m))
}

// PictureSpec 描述新增图片（单位：EMU；1 in = 914400，1 pt = 12700）。
// 位置 (X,Y) 是 spTree 内左上角；Width/Height 对 FitStretch/FitContain/
// FitCover 必需（>0），FitOriginalSize 忽略。零值 Fit 视为 FitOriginalSize。
type PictureSpec struct {
	X, Y         int64
	Width        int64
	Height       int64
	Fit          PictureFitMode
	AltText      string
	IsDecorative bool
}

// pictureGeometry 计算放置几何与裁剪。
// 返回 offX/offY/extCx/extCy（EMU）与 srcRect 片段（空串表示无裁剪）。
func pictureGeometry(kind imageKind, spec PictureSpec) (int64, int64, int64, int64, string, error) {
	if kind.width <= 0 || kind.hgt <= 0 {
		return 0, 0, 0, 0, "", &OperationError{
			Op: "AddPicture", Message: "image has invalid zero dimension", Err: ErrInvalidArgument,
		}
	}
	wE := int64(kind.width) * emuPerPixel96
	hE := int64(kind.hgt) * emuPerPixel96
	needWH := func() error {
		if spec.Width <= 0 || spec.Height <= 0 {
			return &OperationError{
				Op:      "AddPicture",
				Message: fmt.Sprintf("PictureSpec.Width/Height required for fit %s", spec.Fit),
				Err:     ErrInvalidArgument,
			}
		}
		return nil
	}
	switch spec.Fit {
	case FitOriginalSize:
		return spec.X, spec.Y, wE, hE, "", nil
	case FitStretch:
		if err := needWH(); err != nil {
			return 0, 0, 0, 0, "", err
		}
		return spec.X, spec.Y, spec.Width, spec.Height, "", nil
	case FitContain:
		if err := needWH(); err != nil {
			return 0, 0, 0, 0, "", err
		}
		s := math.Min(float64(spec.Width)/float64(wE), float64(spec.Height)/float64(hE))
		cx := int64(math.Round(float64(wE) * s))
		cy := int64(math.Round(float64(hE) * s))
		ox := spec.X + (spec.Width-cx)/2
		oy := spec.Y + (spec.Height-cy)/2
		return ox, oy, cx, cy, "", nil
	case FitCover:
		if err := needWH(); err != nil {
			return 0, 0, 0, 0, "", err
		}
		s := math.Max(float64(spec.Width)/float64(wE), float64(spec.Height)/float64(hE))
		rw := float64(spec.Width) / s // 显示所需源区宽度（EMU）
		rh := float64(spec.Height) / s
		fl := (float64(wE) - rw) / (2 * float64(wE))
		ft := (float64(hE) - rh) / (2 * float64(hE))
		src := srcRectFragment(fl, ft)
		return spec.X, spec.Y, spec.Width, spec.Height, src, nil
	}
	return 0, 0, 0, 0, "", &OperationError{
		Op: "AddPicture", Message: "unknown fit mode " + spec.Fit.String(), Err: ErrInvalidArgument,
	}
}

// srcRectFragment 生成 a:srcRect 片段；两侧均无裁剪返回空串。
// 百分比单位为千分之一（ST_Percentage：1 = 0.001%）。
func srcRectFragment(l, t float64) string {
	pct := func(v float64) int64 {
		if v < 0 {
			return 0
		}
		return int64(math.Round(v * 100000))
	}
	pl, pt, pr, pb := pct(l), pct(t), pct(l), pct(t)
	if pl == 0 && pt == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<a:srcRect l="`)
	sb.WriteString(intString64(pl))
	sb.WriteString(`" t="`)
	sb.WriteString(intString64(pt))
	sb.WriteString(`" r="`)
	sb.WriteString(intString64(pr))
	sb.WriteString(`" b="`)
	sb.WriteString(intString64(pb))
	sb.WriteString(`"/>`)
	return sb.String()
}

func intString64(v int64) string { return strconv.FormatInt(v, 10) }

// ---------- PictureShape ----------

// PictureShape 是页面图片（p:pic）的受控句柄。
//
// 句柄不持有资源；每次操作以"原始字节 → 当前 revision 索引"沿稳定
// 元素路径（nodeStep）重定位。路径目标被删除返回 ErrStaleHandle；
// 文档关闭返回 ErrClosed（与 Slide/Text 句柄语义一致，§5）。
type PictureShape struct {
	shapeNode
}

// Kind 返回形状类别（恒为 ShapePicture）。
func (s *PictureShape) Kind() ShapeKind { return ShapePicture }

// SetAltText 设置替代文本；同时清除装饰性标记（二者语义互斥，§8.1）。
// text 为空清除 @descr。
func (s *PictureShape) SetAltText(text string) error {
	return s.setAltText("PictureShape.SetAltText", text)
}

// SetDecorative 设置装饰性标记；为 true 时清除替代文本（§8.1），
// false 时仅清除标记、保留既有 @descr。
func (s *PictureShape) SetDecorative(decorative bool) error {
	return s.setDecorative("PictureShape.SetDecorative", decorative)
}

// ---------- AddPicture ----------

// AddPicture 读取 src（PNG/JPEG），按 spec 放置并返回图片句柄。
//
// 媒体字节在返回前被完整读取与校验（暂存），后续源文件变化不影响
// 保存；内容经 SHA-256 去重：与包内既有同类型图片字节相同时复用既有
// 媒体 Part，不产生重复资源。图片追加到 spTree 末尾（最上层 z-order）。
// AltText/IsDecorative 语义见 §8.1。
func (s *Slide) AddPicture(ctx context.Context, src MediaSource, spec PictureSpec) (*PictureShape, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}
	data, err := readMedia(ctx, src)
	if err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}
	kind, err := probeImage(data, src.DeclaredType())
	if err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}
	ox, oy, cx, cy, srcRect, err := pictureGeometry(kind, spec)
	if err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}

	p := s.p
	doc, tree, err := s.slideTree()
	if err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}
	id := nextShapeID(doc, tree)

	// 媒体去重/暂存 + 关系分配（同事务内与 slide XML 一并提交）。
	mediaName, err := p.stageMedia(data, kind)
	if err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}
	rid, err := relForMedia(p, s.part, mediaName)
	if err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}

	frag, err := buildPicFragment(id, ox, oy, cx, cy, rid, srcRect, spec.AltText, spec.IsDecorative)
	if err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}
	ap, err := xmlstore.AppendChild(tree, []byte(frag))
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddPicture")
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{ap})
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddPicture")
	}
	if err := p.stagePatch(s.part, out); err != nil {
		return nil, Annotate(err, "Slide.AddPicture")
	}
	p.commit()
	return s.lastPicHandle(), nil
}

// slideTree 定位页面 spTree（首个 p:spTree）。
func (s *Slide) slideTree() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return nil, nil, err
	}
	ids := doc.Elements(nsPresentationML, "spTree")
	if len(ids) == 0 {
		return nil, nil, &OperationError{
			Op: "slide", Part: string(s.part),
			Message: "slide has no p:spTree", Err: ErrMalformedPackage,
		}
	}
	return doc, doc.Node(ids[0]), nil
}

// nextShapeID 递归扫描 spTree 内所有 cNvPr@id 取 max+1（≥2）。
func nextShapeID(doc *xmlstore.XMLDocument, tree *xmlstore.NodeRecord) int64 {
	maxID := int64(1)
	var walk func(n *xmlstore.NodeRecord)
	walk = func(n *xmlstore.NodeRecord) {
		if n.Namespace == nsPresentationML && n.Local() == "cNvPr" {
			if v, ok := n.Attr("", "id"); ok {
				if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > maxID {
					maxID = id
				}
			}
		}
		for _, cid := range n.Children {
			walk(doc.Node(cid))
		}
	}
	walk(tree)
	return maxID + 1
}

// buildPicFragment 构造 p:pic 片段（blipFill + 可选 srcRect + spPr 矩形）。
func buildPicFragment(id, ox, oy, cx, cy int64, rid, srcRect, altText string, decorative bool) (string, error) {
	var sb strings.Builder
	sb.WriteString(`<p:pic><p:nvPicPr><p:cNvPr id="`)
	sb.WriteString(intString64(id))
	sb.WriteString(`" name="Picture `)
	sb.WriteString(intString64(id))
	sb.WriteString(`"`)
	if decorative {
		sb.WriteString(` decorative="1"`)
	} else if altText != "" {
		esc, err := xmlstore.EscapeAttrValue(altText, '"')
		if err != nil {
			return "", Annotate(mapXMLError(err), "AddPicture")
		}
		sb.WriteString(` descr="`)
		sb.WriteString(esc)
		sb.WriteString(`"`)
	}
	sb.WriteString(`/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>`)
	sb.WriteString(`<p:blipFill>`)
	sb.WriteString(srcRect)
	sb.WriteString(`<a:blip r:embed="`)
	sb.WriteString(rid)
	sb.WriteString(`"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`)
	sb.WriteString(`<p:spPr><a:xfrm><a:off x="`)
	sb.WriteString(intString64(ox))
	sb.WriteString(`" y="`)
	sb.WriteString(intString64(oy))
	sb.WriteString(`"/><a:ext cx="`)
	sb.WriteString(intString64(cx))
	sb.WriteString(`" cy="`)
	sb.WriteString(intString64(cy))
	sb.WriteString(`"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`)
	return sb.String(), nil
}

// lastPicHandle 在提交后定位 spTree 末尾 p:pic 并返回句柄。
func (s *Slide) lastPicHandle() *PictureShape {
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return &PictureShape{shapeNode: shapeNode{p: s.p, part: s.part}}
	}
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for i := len(tree.Children) - 1; i >= 0; i-- {
			c := doc.Node(tree.Children[i])
			if c.Namespace == nsPresentationML && c.Local() == "pic" {
				return &PictureShape{shapeNode: shapeNode{p: s.p, part: s.part, path: recordPath(doc, c.ID)}}
			}
		}
	}
	return &PictureShape{shapeNode: shapeNode{p: s.p, part: s.part}}
}

// ---------- ReplaceImage（共享引用保护） ----------

// ReplaceImage 替换本图片引用的源媒体（保留当前形状几何与裁剪/替代
// 文本等属性，不改变 p:pic 结构）。
//
// 共享引用保护：新内容与旧内容相同时为 no-op；否则经内容哈希去重
// 复用既有媒体 Part（或新增）；旧媒体仅在替换后不再被任何关系引用时
// 才删除，绝不破坏其它形状的共享引用。裁剪（a:srcRect）在替换时清除
// ——换图后按当前框拉伸，避免旧裁剪比例作用于新图。
func (s *PictureShape) ReplaceImage(ctx context.Context, src MediaSource) error {
	doc, pic, err := s.locate()
	if err != nil {
		return Annotate(err, "PictureShape.ReplaceImage")
	}
	blip := picBlipOf(doc, pic)
	if blip == nil {
		return &OperationError{
			Op: "PictureShape.ReplaceImage", Part: string(s.part),
			Message: "p:pic has no a:blip with r:embed", Err: ErrUnsupportedEdit,
		}
	}
	oldRID, ok := blip.Attr(nsOfficeDocument, "embed")
	if !ok || oldRID == "" {
		return &OperationError{
			Op: "PictureShape.ReplaceImage", Part: string(s.part),
			Message: "p:pic has no r:embed", Err: ErrUnsupportedEdit,
		}
	}
	oldPart, ok := s.p.mediaTargetOf(s.part, oldRID)
	if !ok {
		return &OperationError{
			Op: "PictureShape.ReplaceImage", Part: string(s.part),
			Message: "r:embed " + oldRID + " has no internal image relationship",
			Err:     ErrNotFound,
		}
	}

	data, err := readMedia(ctx, src)
	if err != nil {
		return Annotate(err, "PictureShape.ReplaceImage")
	}
	kind, err := probeImage(data, src.DeclaredType())
	if err != nil {
		return Annotate(err, "PictureShape.ReplaceImage")
	}
	sum := sha256.Sum256(data)
	// 内容未变：no-op（含声明类型一致但字节相同）。
	if oldBytes, e := s.p.partBytes(oldPart); e == nil && sha256.Sum256(oldBytes) == sum {
		return nil
	}

	p := s.p
	newPart, err := p.stageMedia(data, kind)
	if err != nil {
		return Annotate(err, "PictureShape.ReplaceImage")
	}
	if newPart == oldPart {
		return nil // 去重命中自身（内容相同）——已由上行 no-op 覆盖，防御。
	}

	// 关系变更基于同一份已提交关系流计算，合并为一次补丁：
	//   新目标 rId（已有同类型同目标关系则复用）与旧 rId 移除互不覆盖。
	relBase, err := relsXML(p, s.part)
	if err != nil {
		return Annotate(err, "PictureShape.ReplaceImage")
	}
	relsOut := relBase
	newRID := ""
	if cur, ok, err := p.relsOf(s.part); err != nil {
		return Annotate(err, "PictureShape.ReplaceImage")
	} else if ok {
		for _, rel := range cur {
			if rel.Mode == opc.TargetInternal && rel.Type == relImage && rel.TargetPart == newPart {
				newRID = rel.ID
				break
			}
		}
	}
	if newRID == "" {
		rid := nextRID(relsOut)
		relsOut = insertRel(relsOut,
			`<Relationship Id="`+rid+`" Type="`+relImage+`" Target="../media/`+slideName(newPart)+`"/>`)
		newRID = rid
	}

	// 旧关系清理：本页内该 rId 无其它使用者才移除对应关系条目。
	oldRelRemoved := false
	if newRID != oldRID && countBlipsEmbed(doc, oldRID, blip.ID) == 0 {
		prev := relsOut
		relsOut = removeRelEntry(relsOut, oldRID)
		oldRelRemoved = !bytes.Equal(relsOut, prev)
	}
	if !bytes.Equal(relsOut, relBase) {
		if err := stageRelsBytes(p, s.part, relsOut); err != nil {
			return Annotate(err, "PictureShape.ReplaceImage")
		}
	}
	// 旧媒体删除：仅当关系条目被移除且全局归零（保守口径：关系条目仍
	// 指向即保留，不解析未知 Part 的消费方，绝不破坏共享引用）。
	if oldRelRemoved && p.mediaTargetRefs(oldPart) == 1 {
		if err := p.stageDelete(oldPart); err != nil {
			return Annotate(err, "PictureShape.ReplaceImage")
		}
	}

	// blip r:embed → newRID；清除旧裁剪（换图按当前框拉伸，见方法注释）。
	embedPatch, err := xmlstore.SetAttrValuePatch(doc, blip, nsOfficeDocument, "embed", newRID)
	if err != nil {
		return Annotate(mapXMLError(err), "PictureShape.ReplaceImage")
	}
	var extra []xmlstore.SpanPatch
	if sr := picSrcRectOf(doc, pic); sr != nil {
		extra = append(extra, xmlstore.SpanPatch{
			Start: sr.Source.Start, End: sr.Source.End, Replacement: []byte(""),
		})
	}
	all := append([]xmlstore.SpanPatch{embedPatch}, extra...)
	out, err := xmlstore.ApplyPatches(doc.Original(), all)
	if err != nil {
		return Annotate(mapXMLError(err), "PictureShape.ReplaceImage")
	}
	if err := p.stagePatch(s.part, out); err != nil {
		return Annotate(err, "PictureShape.ReplaceImage")
	}
	p.commit()
	return nil
}

// picBlipOf 返回图片填充中的 a:blip（首个）。
func picBlipOf(doc *xmlstore.XMLDocument, pic *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	fill := childOfKind(doc, pic, nsPresentationML, "blipFill", 0)
	if fill == nil {
		return nil
	}
	return childOfKind(doc, fill, nsDrawingML, "blip", 0)
}

// picSrcRectOf 返回图片填充中的 a:srcRect（首个）。
func picSrcRectOf(doc *xmlstore.XMLDocument, pic *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	fill := childOfKind(doc, pic, nsPresentationML, "blipFill", 0)
	if fill == nil {
		return nil
	}
	return childOfKind(doc, fill, nsDrawingML, "srcRect", 0)
}

// countBlipsEmbed 统计整个页面中 r:embed==rid 的 blip 数量；skipID 排除
// 指定节点（当前正被替换的 pic）。
func countBlipsEmbed(doc *xmlstore.XMLDocument, rid string, skipID xmlstore.NodeID) int {
	cnt := 0
	var walk func(nd *xmlstore.NodeRecord)
	walk = func(nd *xmlstore.NodeRecord) {
		if nd.Namespace == nsDrawingML && nd.Local() == "blip" {
			if v, ok := nd.Attr(nsOfficeDocument, "embed"); ok && v == rid && nd.ID != skipID {
				cnt++
			}
		}
		for _, cid := range nd.Children {
			walk(doc.Node(cid))
		}
	}
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		walk(doc.Node(tid))
	}
	return cnt
}

// mediaTargetOf 返回 Part 关系流中 rid 指向的内部目标。
func (p *Presentation) mediaTargetOf(part opc.PartName, rid string) (opc.PartName, bool) {
	rels, ok, err := p.relsOf(part)
	if err != nil || !ok {
		return "", false
	}
	for _, rel := range rels {
		if rel.ID == rid && rel.Mode == opc.TargetInternal {
			return rel.TargetPart, true
		}
	}
	return "", false
}

// mediaTargetRefs 统计整个文档中指向 target 的内部关系条目数（含新增
// Part 的关系流；已删除 Part 排除）。保守口径：只要还有关系条目指向
// 媒体就不删除——不解析未知/二进制 Part 的消费方，避免破坏共享引用。
func (p *Presentation) mediaTargetRefs(target opc.PartName) int {
	count := 0
	seen := map[opc.PartName]bool{}
	consider := func(name opc.PartName) {
		if seen[name] || strings.HasSuffix(string(name), ".rels") {
			return
		}
		seen[name] = true
		if p.deletedParts[name] {
			return
		}
		rels, ok, err := p.relsOf(name)
		if err != nil || !ok {
			return
		}
		for _, rel := range rels {
			if rel.Mode == opc.TargetInternal && rel.TargetPart == target {
				count++
			}
		}
	}
	for _, name := range p.pk.PartNames() {
		consider(name)
	}
	for name := range p.addedParts {
		consider(name)
	}
	return count
}

// removeRelEntry 删除关系流中 Id==id 的 Relationship 元素（规范中关系
// 为空元素；取 <Relationship .../> 文本块）。找不到返回原样。
func removeRelEntry(rels []byte, id string) []byte {
	s := string(rels)
	needle := `Id="` + id + `"`
	i := strings.Index(s, needle)
	if i < 0 {
		return rels
	}
	start := strings.LastIndex(s[:i], "<Relationship")
	if start < 0 {
		return rels
	}
	end := strings.Index(s[start:], ">")
	if end < 0 {
		return rels
	}
	end += start + 1
	if end < start || end > len(s) {
		return rels
	}
	return []byte(s[:start] + s[end:])
}

// ---------- 媒体暂存与去重 ----------

// stageMedia 把图片字节暂存为媒体 Part：先按内容哈希（SHA-256）+ 类型
// 去重复用既有 Part，否则分配 /ppt/media/imageN.ext 新增。字节在暂存时
// 拷贝（修改事务完成前固化，§21.1）。
func (p *Presentation) stageMedia(data []byte, kind imageKind) (opc.PartName, error) {
	sum := sha256.Sum256(data)
	if name, ok := p.findExistingMedia(sum, kind.ct); ok {
		return name, nil
	}
	name := p.newMediaName(kind.ext)
	if err := p.stageAdd(name, data, kind.ct); err != nil {
		return "", err
	}
	return name, nil
}

// findExistingMedia 在包内与已提交新增 Part 中查找同哈希同类型媒体。
func (p *Presentation) findExistingMedia(sum [32]byte, ct string) (opc.PartName, bool) {
	for _, name := range p.pk.PartNames() {
		s := string(name)
		if !strings.HasPrefix(s, "/ppt/media/") {
			continue
		}
		if c, ok := p.pk.ContentType(name); !ok || !imageTypeEqual(c, ct) {
			continue
		}
		b, err := p.partBytes(name)
		if err != nil {
			continue
		}
		if sha256.Sum256(b) == sum {
			return name, true
		}
	}
	for name, ap := range p.addedParts {
		s := string(name)
		if !strings.HasPrefix(s, "/ppt/media/") {
			continue
		}
		if !imageTypeEqual(ap.ContentType, ct) {
			continue
		}
		b, err := p.partBytes(name)
		if err != nil {
			continue
		}
		if sha256.Sum256(b) == sum {
			return name, true
		}
	}
	return "", false
}

// newMediaName 分配下一个 /ppt/media/imageN.ext（序号跨扩展全局递增，
// 避免 image1.png/image1.jpg 并存冲突）。需同时避开包内与已提交新增名。
func (p *Presentation) newMediaName(ext string) opc.PartName {
	max := 0
	consider := func(name opc.PartName) {
		s := string(name)
		const prefix = "/ppt/media/image"
		if !strings.HasPrefix(s, prefix) {
			return
		}
		rest := s[len(prefix):]
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		if digits == 0 {
			return
		}
		if n, err := strconv.Atoi(rest[:digits]); err == nil && n > max {
			max = n
		}
	}
	for _, name := range p.pk.PartNames() {
		consider(name)
	}
	for name := range p.addedParts {
		consider(name)
	}
	return opc.PartName("/ppt/media/image" + strconv.Itoa(max+1) + "." + ext)
}

// stageRelsBytes 写入 Part 的关系流：关系流 Part 已存在（包内或会话内
// 新增）走补丁；不存在则按新增注册（扩展名 .rels 由 Default 覆盖）。
func stageRelsBytes(p *Presentation, part opc.PartName, content []byte) error {
	rp := relsPart(part)
	if p.pk.HasPart(rp) || p.addedParts[rp].Content != nil {
		return p.stagePatch(rp, content)
	}
	return p.stageAdd(rp, content, "")
}

// relForMedia 返回 slide 关系流中指向 media 的 rId：已有同类型同目标
// 关系直接复用（同页多图共享一个媒体引用），否则新增关系条目并暂存
// 补丁。返回的 rId 在调用方同一事务提交后生效。
func relForMedia(p *Presentation, slide, media opc.PartName) (string, error) {
	rels, ok, err := p.relsOf(slide)
	if err != nil {
		return "", err
	}
	if ok {
		for _, rel := range rels {
			if rel.Mode == opc.TargetInternal && rel.Type == relImage && rel.TargetPart == media {
				return rel.ID, nil
			}
		}
	}
	xml, err := relsXML(p, slide)
	if err != nil {
		return "", err
	}
	rid := nextRID(xml)
	entry := `<Relationship Id="` + rid + `" Type="` + relImage +
		`" Target="../media/` + slideName(media) + `"/>`
	updated := insertRel(xml, entry)
	if err := stageRelsBytes(p, slide, updated); err != nil {
		return "", err
	}
	return rid, nil
}
