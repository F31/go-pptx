package pptx

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件补齐设计 §20.2 的形状创建与管理 API：AddTextBox / AddAutoShape /
// RemoveShape / MoveShape 与 TextFrame.AddParagraph。
//
// 创建路径复用既有保真补丁机制（AppendChild/InsertBefore + ApplyPatches +
// stagePatch + commit），不引入第二套写路径；移除与移动参照 MoveSlide 的
// "提取字节 + 双补丁升序提交"模式。

// Stable: TextShape 是文本框句柄，是 AutoShape 的类型别名——不引入第二
// 套句柄实现（设计 §20.2）。TextShape 与 AutoShape 共享全部 Stable 语义
// （类型签名不变；不新增/重命名/移除公开方法；现有方法签名与返回类型不变；
// 仅允许追加新方法；句柄身份语义不变）。TextFrame/AltText 等全部能力直接
// 可用。AddTextBox 返回 *TextShape 与 AddAutoShape 返回 *AutoShape 行为
// 完全一致——前者按 cNvSpPr@txBox="1" 标记，后者按其他 OOXML 自选图形
// 标记，但二者底层共用 p:sp 元素（STALE-GUARD 句柄身份语义统一）。
type TextShape = AutoShape

// TextBoxSpec 描述新建文本框（§20.2）。坐标与尺寸为 EMU（914400/inch）。
type TextBoxSpec struct {
	// X, Y 是左上角偏移（可为负，允许画布外放置）。
	X, Y int64
	// Width, Height 是尺寸；必须 > 0（ErrInvalidArgument）。
	Width, Height int64
	// Name 是形状名；空时默认 "TextBox <id>"。
	Name string
	// Text 是初始正文（纯文本）：'\n' 分行生成段落（与 SetPlainText
	// 同语义）；空串生成一个空段落。
	Text string
	// AltText 与 IsDecorative 语义见 §8.1（二者互斥）。
	AltText      string
	IsDecorative bool
}

// AutoShapeSpec 描述新建自选图形（§20.2）。
type AutoShapeSpec struct {
	X, Y   int64
	Width  int64
	Height int64
	// Geometry 是 preset 几何名（a:prstGeom@prst，如 "rect"/"roundRect"/
	// "ellipse"）；必须非空且为安全 token（ErrInvalidArgument）。
	Geometry string
	Name     string
	// Text 是初始正文（同 TextBoxSpec；空串生成空 txBody 段落）。
	Text         string
	AltText      string
	IsDecorative bool
}

// AddTextBox 在页面追加文本框（z-order 最上层），返回其句柄。
//
// 追加到 spTree 末尾；AltText/IsDecorative 互斥语义见 §8.1。
func (s *Slide) AddTextBox(spec TextBoxSpec) (*TextShape, error) {
	const op = "Slide.AddTextBox"
	if err := s.alive(); err != nil {
		return nil, Annotate(err, op)
	}
	if spec.Width <= 0 || spec.Height <= 0 {
		return nil, &OperationError{Op: op, Message: "width/height must be > 0", Err: ErrInvalidArgument}
	}
	frag, err := buildSpFragment(spKindTextBox, spec.X, spec.Y, spec.Width, spec.Height, "", spec.Name, spec.Text, spec.AltText, spec.IsDecorative)
	if err != nil {
		return nil, Annotate(err, op)
	}
	return s.appendSpFragment(op, frag)
}

// AddAutoShape 在页面追加自选图形（z-order 最上层），返回其句柄。
func (s *Slide) AddAutoShape(spec AutoShapeSpec) (*AutoShape, error) {
	const op = "Slide.AddAutoShape"
	if err := s.alive(); err != nil {
		return nil, Annotate(err, op)
	}
	if spec.Width <= 0 || spec.Height <= 0 {
		return nil, &OperationError{Op: op, Message: "width/height must be > 0", Err: ErrInvalidArgument}
	}
	if !validPresetName(spec.Geometry) {
		return nil, &OperationError{Op: op, Message: "geometry must be a non-empty preset name", Err: ErrInvalidArgument}
	}
	frag, err := buildSpFragment(spKindAutoShape, spec.X, spec.Y, spec.Width, spec.Height, spec.Geometry, spec.Name, spec.Text, spec.AltText, spec.IsDecorative)
	if err != nil {
		return nil, Annotate(err, op)
	}
	return s.appendSpFragment(op, frag)
}

// appendSpFragment 把 p:sp 片段追加到 spTree 末尾并提交，返回新形状句柄。
func (s *Slide) appendSpFragment(op string, frag string) (*AutoShape, error) {
	doc, tree, err := s.slideTree()
	if err != nil {
		return nil, Annotate(err, op)
	}
	id := nextShapeID(doc, tree)
	frag = strings.Replace(frag, `__SHAPE_ID__`, intString64(id), 1)
	ap, err := xmlstore.AppendChild(tree, []byte(frag))
	if err != nil {
		return nil, Annotate(mapXMLError(err), op)
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{ap})
	if err != nil {
		return nil, Annotate(mapXMLError(err), op)
	}
	if err := s.p.stagePatch(s.part, out); err != nil {
		return nil, Annotate(err, op)
	}
	s.p.commit()
	return s.spHandleByID(id), nil
}

// spHandleByID 在提交后按 cNvPr@id 定位顶层 p:sp 并返回句柄。
// 定位失败返回零值句柄（后续操作将得到 ErrStaleHandle，不 panic）。
func (s *Slide) spHandleByID(id int64) *AutoShape {
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return &AutoShape{shapeNode: shapeNode{p: s.p, part: s.part}}
	}
	tree := firstSpTree(doc)
	if tree == nil {
		return &AutoShape{shapeNode: shapeNode{p: s.p, part: s.part}}
	}
	for _, cid := range tree.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != nsPresentationML || c.Local() != "sp" {
			continue
		}
		if nid, ok := shapeCNvPrID(doc, c); ok && nid == id {
			return &AutoShape{shapeNode: shapeNode{p: s.p, part: s.part, path: recordPath(doc, c.ID), idHint: ShapeID(nid)}}
		}
	}
	return &AutoShape{shapeNode: shapeNode{p: s.p, part: s.part}}
}

// ---------- RemoveShape / MoveShape ----------

// RemoveShape 删除页面顶层形状（§20.2）。
//
// 保守口径：
//   - 仅顶层形状；组内子形状不经此 API 删除（返回 ErrNotFound）。
//   - 若页面动画时序（p:timing）引用该形状 ID（任何 @spid），拒绝删除
//     并返回 ErrUnsupportedEdit——不解析 timing 消费方，绝不留下悬空引用。
//   - 图片/音视频的媒体 Part 与关系不删除（共享资源，引用归零清理属
//     资源 GC 语义，与 RemoveSlide 同口径）。
func (s *Slide) RemoveShape(id ShapeID) error {
	const op = "Slide.RemoveShape"
	if err := s.alive(); err != nil {
		return Annotate(err, op)
	}
	doc, tree, err := s.slideTree()
	if err != nil {
		return Annotate(err, op)
	}
	var target *xmlstore.NodeRecord
	for _, cid := range tree.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != nsPresentationML || !isShapeElement(c.Local()) {
			continue
		}
		if nid, ok := shapeCNvPrID(doc, c); ok && nid == int64(id) {
			target = c
			break
		}
	}
	if target == nil {
		return &OperationError{Op: op, Message: "shape id " + strconv.FormatInt(int64(id), 10) + " not found", Err: ErrNotFound}
	}
	if timingReferencesShape(doc, int64(id)) {
		return &OperationError{
			Op: op, Part: string(s.part),
			Message: "shape is referenced by slide timing (spid); remove the timing node first",
			Err:     ErrUnsupportedEdit,
		}
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{deletePatch(doc, target, "remove-shape")})
	if err != nil {
		return Annotate(mapXMLError(err), op)
	}
	if err := s.p.stagePatch(s.part, out); err != nil {
		return Annotate(err, op)
	}
	s.p.commit()
	return nil
}

// MoveShape 调整顶层形状 z-order：zIndex 是移动后的最终位置（形状序，
// 与 Slide.Shapes() 的下标一致；范围 [0, 形状数)）。当前位置即目标时
// 空操作（nil）。
func (s *Slide) MoveShape(id ShapeID, zIndex int) error {
	const op = "Slide.MoveShape"
	if err := s.alive(); err != nil {
		return Annotate(err, op)
	}
	doc, tree, err := s.slideTree()
	if err != nil {
		return Annotate(err, op)
	}
	entries := topLevelShapes(doc, tree)
	n := len(entries)
	if zIndex < 0 || zIndex >= n {
		return &OperationError{
			Op: op, Message: "zIndex " + strconv.Itoa(zIndex) + " out of range [0," + strconv.Itoa(n) + ")",
			Err: ErrOutOfRange,
		}
	}
	i := -1
	for k := range entries {
		if nid, _ := shapeCNvPrID(doc, entries[k]); nid == int64(id) {
			i = k
			break
		}
	}
	if i < 0 {
		return &OperationError{Op: op, Message: "shape id " + strconv.FormatInt(int64(id), 10) + " not found", Err: ErrNotFound}
	}
	if i == zIndex {
		return nil
	}
	moved := entries[i]
	frag := doc.Slice(moved.Source)
	var patches []xmlstore.SpanPatch
	if zIndex < i {
		// 前移：插到目标位置元素（原列表）之前。
		patches = append(patches, xmlstore.SpanPatch{
			Start: entries[zIndex].Source.Start, End: entries[zIndex].Source.Start,
			Replacement: frag, Desc: "move-shape-insert",
		})
	} else {
		// 后移：插到原 zIndex+1 元素之前；无则形状列表末尾。
		if zIndex+1 < n {
			patches = append(patches, xmlstore.SpanPatch{
				Start: entries[zIndex+1].Source.Start, End: entries[zIndex+1].Source.Start,
				Replacement: frag, Desc: "move-shape-insert",
			})
		} else {
			last := entries[n-1].Source.End
			patches = append(patches, xmlstore.SpanPatch{
				Start: last, End: last, Replacement: frag, Desc: "move-shape-insert",
			})
		}
	}
	patches = append(patches, deletePatch(doc, moved, "move-shape-delete"))
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), op)
	}
	if err := s.p.stagePatch(s.part, out); err != nil {
		return Annotate(err, op)
	}
	s.p.commit()
	return nil
}

// ---------- TextFrame.AddParagraph ----------

// ParagraphSpec 描述追加到正文的段落（§20.2）。
type ParagraphSpec struct {
	// Text 是段落文本（纯文本，不做 \n 分行——分行需多次 AddParagraph）。
	Text string
	// Style 是首个 Run 的字符格式（patch 语义同 Paragraph.AddRun；
	// 未设置任何字段时不写 a:rPr）。
	Style FontStyle
}

// AddParagraph 在正文末尾追加段落，返回其句柄（§20.2）。
//
// 段落追加为最后一个 a:p（在既有段落之后，不影响 bodyPr/lstStyle）。
// Text 非空时生成一个 Run；Style 为零值表示不写字符格式。
func (t *TextFrame) AddParagraph(spec ParagraphSpec) (*Paragraph, error) {
	const op = "TextFrame.AddParagraph"
	doc, body, err := t.locate()
	if err != nil {
		return nil, Annotate(err, op)
	}
	if body.SelfClosing() {
		return nil, &OperationError{Op: op, Part: string(t.part), Message: "txBody is self-closing", Err: ErrUnsupportedEdit}
	}
	var sb strings.Builder
	sb.WriteString("<a:p>")
	if spec.Text != "" || spec.Style.anySet() {
		sb.WriteString("<a:r>")
		if spec.Style.anySet() {
			frag, err := buildRPrFragment("a", spec.Style)
			if err != nil {
				return nil, Annotate(err, op)
			}
			sb.WriteString(frag)
		}
		if spec.Text != "" {
			esc, err := xmlstore.EscapeText(spec.Text)
			if err != nil {
				return nil, Annotate(mapXMLError(err), op)
			}
			sb.WriteString("<a:t>" + esc + "</a:t>")
		}
		sb.WriteString("</a:r>")
	}
	sb.WriteString("</a:p>")
	ap, err := xmlstore.AppendChild(body, []byte(sb.String()))
	if err != nil {
		return nil, Annotate(mapXMLError(err), op)
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{ap})
	if err != nil {
		return nil, Annotate(mapXMLError(err), op)
	}
	if err := t.p.stagePatch(t.part, out); err != nil {
		return nil, Annotate(err, op)
	}
	t.p.commit()
	n := countKind(doc, body, nsDrawingML, "p")
	return &Paragraph{textNode: textNode{p: t.p, part: t.part, path: t.path}, idx: n}, nil
}

// ---------- 内部辅助 ----------

type spKind int

const (
	spKindTextBox spKind = iota
	spKindAutoShape
)

// buildSpFragment 构造 p:sp 片段。geom：文本框恒 "rect"；自选图形为
// preset 名。id 为 __SHAPE_ID__ 占位，由 appendSpFragment 分配后替换。
func buildSpFragment(kind spKind, x, y, cx, cy int64, geom, name, text, altText string, decorative bool) (string, error) {
	txBox := ""
	defName := "Shape "
	prst := geom
	if kind == spKindTextBox {
		txBox = ` txBox="1"`
		defName = "TextBox "
		prst = "rect"
	}
	if name == "" {
		name = defName + "__SHAPE_ID__"
	}
	nv, err := xmlstore.EscapeAttrValue(name, '"')
	if err != nil {
		return "", &OperationError{Op: "AddShape", Message: "invalid name", Err: mapXMLError(err)}
	}
	descr := ""
	if decorative {
		descr = ` decorative="1"`
	} else if altText != "" {
		esc, err := xmlstore.EscapeAttrValue(altText, '"')
		if err != nil {
			return "", &OperationError{Op: "AddShape", Message: "invalid alt text", Err: mapXMLError(err)}
		}
		descr = ` descr="` + esc + `"`
	}

	var body strings.Builder
	body.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="__SHAPE_ID__" name="`)
	body.WriteString(nv)
	body.WriteString(`"` + descr + `/>`)
	body.WriteString(`<p:cNvSpPr` + txBox + `/><p:nvPr/></p:nvSpPr>`)
	body.WriteString(`<p:spPr><a:xfrm><a:off x="`)
	body.WriteString(intString64(x))
	body.WriteString(`" y="`)
	body.WriteString(intString64(y))
	body.WriteString(`"/><a:ext cx="`)
	body.WriteString(intString64(cx))
	body.WriteString(`" cy="`)
	body.WriteString(intString64(cy))
	body.WriteString(`"/></a:xfrm><a:prstGeom prst="`)
	body.WriteString(prst)
	body.WriteString(`"><a:avLst/></a:prstGeom></p:spPr>`)
	// txBody：bodyPr + lstStyle + 段落（Text 按 '\n' 分行，同 SetPlainText
	// 语义；空文本生成一个空段落，保证 txBody 至少一个 a:p）。
	body.WriteString(`<p:txBody><a:bodyPr/><a:lstStyle/>`)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		body.WriteString(`<a:p>`)
		if line != "" {
			esc, err := xmlstore.EscapeText(line)
			if err != nil {
				return "", &OperationError{Op: "AddShape", Message: "invalid text", Err: mapXMLError(err)}
			}
			body.WriteString(`<a:r><a:t>` + esc + `</a:t></a:r>`)
		}
		body.WriteString(`</a:p>`)
	}
	body.WriteString(`</p:txBody></p:sp>`)
	return body.String(), nil
}

// validPresetName 校验 preset 几何名是安全 token：非空、无空白与
// XML 特殊字符（不白名单——schema 有 180+ preset，调用方自担拼写）。
func validPresetName(s string) bool {
	if s == "" {
		return false
	}
	return !strings.ContainsAny(s, " \t\r\n<>&\"'")
}

// isShapeElement 判断 spTree 顶层子元素是否为可管理形状
// （排除 nvGrpSpPr/grpSpPr 簿记元素；非 p 命名空间已在上游过滤）。
func isShapeElement(local string) bool {
	switch local {
	case "nvGrpSpPr", "grpSpPr":
		return false
	}
	return true
}

// topLevelShapes 返回 spTree 顶层形状元素（文档序，与 Shapes() 一致）。
func topLevelShapes(doc *xmlstore.XMLDocument, tree *xmlstore.NodeRecord) []*xmlstore.NodeRecord {
	out := []*xmlstore.NodeRecord{}
	for _, cid := range tree.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != nsPresentationML || !isShapeElement(c.Local()) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// firstSpTree 返回首个 p:spTree（无则 nil）。
func firstSpTree(doc *xmlstore.XMLDocument) *xmlstore.NodeRecord {
	ids := doc.Elements(nsPresentationML, "spTree")
	if len(ids) == 0 {
		return nil
	}
	return doc.Node(ids[0])
}

// shapeCNvPrID 返回形状元素的 cNvPr@id（嵌套于 nv*Pr 内）。
func shapeCNvPrID(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) (int64, bool) {
	var found *xmlstore.NodeRecord
	var walk func(n *xmlstore.NodeRecord)
	walk = func(n *xmlstore.NodeRecord) {
		if found != nil {
			return
		}
		if n.Namespace == nsPresentationML && n.Local() == "cNvPr" {
			found = n
			return
		}
		for _, cid := range n.Children {
			walk(doc.Node(cid))
			if found != nil {
				return
			}
		}
	}
	walk(el)
	if found == nil {
		return 0, false
	}
	v, ok := found.Attr("", "id")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// timingReferencesShape 判断页面 p:timing 子树中是否有 @spid == id 的引用。
func timingReferencesShape(doc *xmlstore.XMLDocument, id int64) bool {
	want := strconv.FormatInt(id, 10)
	for _, tid := range doc.Elements(nsPresentationML, "timing") {
		timing := doc.Node(tid)
		var hit bool
		var walk func(n *xmlstore.NodeRecord)
		walk = func(n *xmlstore.NodeRecord) {
			if hit || n == nil {
				return
			}
			if v, ok := n.AttrLocal("spid"); ok && v == want {
				hit = true
				return
			}
			for _, cid := range n.Children {
				walk(doc.Node(cid))
				if hit {
					return
				}
			}
		}
		walk(timing)
		if hit {
			return true
		}
	}
	return false
}
