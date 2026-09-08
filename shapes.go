package pptx

import (
	"errors"
	"strconv"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现页面形状枚举与形状句柄（方案 §20.1/§20.2 页面 API 的
// Shapes/Placeholders 部分 + §8.1 无障碍元数据在 AutoShape 上的读写）。
//
// 形状分类按 spTree 直接子元素（z-order = 文档序）：
//   - p:sp    → *AutoShape（cNvSpPr@txBox="1" 时 Kind 为文本框，其余
//     为自选图形）；含 p:txBody 时可经 TextFrame 读写正文；
//   - p:pic   → *PictureShape（IMAGE-01 既有，补 ID/Name/Kind）；
//   - 其余（p:grpSp/p:cxnSp/p:graphicFrame/未知容器）→ *OpaqueShape，
//     仅暴露通用只读元信息（ID/Name/AltText/IsDecorative），内容编辑
//     随 GEOM/表格/图表工作包；组合子形状不在此展开。
//
// §8.1：AltText/IsDecorative 读写位于 p:cNvPr（@descr/@decorative），
// 所有形状共享。读取不臆测：两者均未声明时返回空串/false。写入语义：
// SetAltText 与装饰性标记互斥（设文本清除 decorative；空串清除 descr），
// SetDecorative(true) 清除 descr 并写 decorative="1"——显式装饰性声明
// 与空替代文本在辅助技术读取语义上不同，不用空 descr 代替装饰声明。

// ShapeKind 描述 spTree 顶层形状的类别。
type ShapeKind int

const (
	// ShapeTextBox 是 p:sp 且 cNvSpPr@txBox="1" 的文本框。
	ShapeTextBox ShapeKind = iota
	// ShapeAutoShape 是其余 p:sp（自选图形/占位符等）。
	ShapeAutoShape
	// ShapePicture 是 p:pic 图片。
	ShapePicture
	// ShapeGroup 是 p:grpSp 组合。
	ShapeGroup
	// ShapeConnector 是 p:cxnSp 连接符。
	ShapeConnector
	// ShapeGraphicFrame 是 p:graphicFrame（表格/图表/对象框等）。
	ShapeGraphicFrame
	// ShapeOpaque 是未知或未支持的形状容器。
	ShapeOpaque
)

func (k ShapeKind) String() string {
	switch k {
	case ShapeTextBox:
		return "textbox"
	case ShapeAutoShape:
		return "autoshape"
	case ShapePicture:
		return "picture"
	case ShapeGroup:
		return "group"
	case ShapeConnector:
		return "connector"
	case ShapeGraphicFrame:
		return "graphic-frame"
	case ShapeOpaque:
		return "opaque"
	}
	return "ShapeKind(" + strconv.Itoa(int(k)) + ")"
}

// Shape 是页面顶层形状的公共接口（§20.2 公共对象的最小公共面）。
// 只读元信息对全部形状可用，不要求理解对象具体内容；组合子树内容
// 编辑与几何/表格等随后续工作包。
//
// GEOM-01（§8）：Bounds/WorldQuad/WorldAABB 对全部形状可用——实现
// 均共享 shapeNode 基元，按元素实际结构解析 xfrm；无 a:xfrm 的形状
// 返回 ErrNotFound。
type Shape interface {
	// ID 返回形状标识（p:cNvPr@id）；无 cNvPr 时返回 0。
	ID() ShapeID
	// Name 返回形状名称（p:cNvPr@name）；未命名返回空串。
	Name() string
	// Kind 返回形状类别。
	Kind() ShapeKind
	// AltText 返回替代文本（p:cNvPr@descr，§8.1）；未声明返回空串。
	AltText() string
	// IsDecorative 返回是否标记为装饰性图形（@decorative="1"，§8.1）。
	IsDecorative() bool
	// Bounds 返回形状本地框（直接父坐标系内 off/ext 轴对齐矩形）。
	Bounds() (Rect, error)
	// WorldQuad 返回形状内容框在世界（页面）坐标系的四角。
	WorldQuad() (Quad, error)
	// WorldAABB 返回 WorldQuad 的轴对齐包围框。
	WorldAABB() (Rect, error)
}

// ---------- 通用形状句柄基元 ----------

// shapeNode 是形状句柄的公共载体：不持有资源，每次操作沿稳定元素
// 路径（nodeStep）在当前 revision 索引上重定位。路径目标被删除返回
// ErrStaleHandle；文档关闭返回 ErrClosed。
type shapeNode struct {
	p    *Presentation
	part opc.PartName
	path []nodeStep
}

// locate 解析句柄路径，返回当前索引与形状元素。
func (s *shapeNode) locate() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	if s.p == nil || s.p.closed {
		return nil, nil, Annotate(ErrClosed, "shape")
	}
	doc, err := s.p.docOf(s.part)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, Annotate(ErrStaleHandle, "shape")
		}
		return nil, nil, err
	}
	n := resolvePath(doc, s.path)
	if n == nil {
		return nil, nil, Annotate(ErrStaleHandle, "shape")
	}
	return doc, n, nil
}

// nvPrContainer 返回形状元素非可视属性的容器子元素名
// （ECMA CT_*：p:nvPicPr/p:nvSpPr/p:nvGrpSpPr/p:nvCxnSpPr/
// p:nvGraphicFramePr）。未知元素返回空串。
func nvPrContainer(local string) string {
	switch local {
	case "pic":
		return "nvPicPr"
	case "sp":
		return "nvSpPr"
	case "grpSp":
		return "nvGrpSpPr"
	case "cxnSp":
		return "nvCxnSpPr"
	case "graphicFrame":
		return "nvGraphicFramePr"
	}
	return ""
}

// elementCNvPr 返回形状元素（p:sp/p:pic/...）的 p:cNvPr。
func elementCNvPr(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	cont := nvPrContainer(el.Local())
	if cont == "" {
		return nil
	}
	c := childOfKind(doc, el, nsPresentationML, cont, 0)
	if c == nil {
		return nil
	}
	return childOfKind(doc, c, nsPresentationML, "cNvPr", 0)
}

// shapePhKey 读取形状元素的占位符键；非占位符返回 ok=false。
// 与 phKeyOf（style.go，仅 p:sp）同规约，泛化到全部形状容器。
func shapePhKey(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) (phKey, bool) {
	cont := nvPrContainer(el.Local())
	if cont == "" {
		return phKey{}, false
	}
	c := childOfKind(doc, el, nsPresentationML, cont, 0)
	if c == nil {
		return phKey{}, false
	}
	nvPr := childOfKind(doc, c, nsPresentationML, "nvPr", 0)
	if nvPr == nil {
		return phKey{}, false
	}
	ph := childOfKind(doc, nvPr, nsPresentationML, "ph", 0)
	if ph == nil {
		return phKey{}, false
	}
	k := phKey{typ: "obj", idx: 0}
	if t, ok := ph.Attr("", "type"); ok {
		k.typ = t
	}
	if s, ok := ph.Attr("", "idx"); ok {
		if v, err := parseUint32(s); err == nil {
			k.idx = v
		}
	}
	return k, true
}

// ID 返回形状标识（p:cNvPr@id）；无 cNvPr 或不可解析返回 0。
func (s *shapeNode) ID() ShapeID {
	doc, el, err := s.locate()
	if err != nil {
		return 0
	}
	c := elementCNvPr(doc, el)
	if c == nil {
		return 0
	}
	if v, ok := c.Attr("", "id"); ok {
		if id, err := parseUint32(v); err == nil {
			return ShapeID(id)
		}
	}
	return 0
}

// Name 返回形状名称（p:cNvPr@name）。
func (s *shapeNode) Name() string {
	doc, el, err := s.locate()
	if err != nil {
		return ""
	}
	c := elementCNvPr(doc, el)
	if c == nil {
		return ""
	}
	v, _ := c.Attr("", "name")
	return v
}

// AltText 返回替代文本（p:cNvPr@descr，§8.1）；未声明返回空串。
func (s *shapeNode) AltText() string {
	doc, el, err := s.locate()
	if err != nil {
		return ""
	}
	c := elementCNvPr(doc, el)
	if c == nil {
		return ""
	}
	v, _ := c.Attr("", "descr")
	return v
}

// IsDecorative 返回是否标记为装饰性图形（p:cNvPr@decorative="1"，§8.1）。
func (s *shapeNode) IsDecorative() bool {
	doc, el, err := s.locate()
	if err != nil {
		return false
	}
	c := elementCNvPr(doc, el)
	if c == nil {
		return false
	}
	v, _ := c.Attr("", "decorative")
	return v == "1"
}

// setAltText 设置替代文本；同时清除装饰性标记（二者语义互斥，§8.1）。
// text 为空清除 @descr。op 用于错误标注（如 "AutoShape.SetAltText"）。
func (s *shapeNode) setAltText(op, text string) error {
	doc, el, err := s.locate()
	if err != nil {
		return Annotate(err, op)
	}
	c := elementCNvPr(doc, el)
	if c == nil {
		return &OperationError{
			Op: op, Part: string(s.part),
			Message: "shape has no p:cNvPr", Err: ErrMalformedPackage,
		}
	}
	var patches []xmlstore.SpanPatch
	if d := removeDecorativePatch(doc, c); d != nil {
		patches = append(patches, *d)
	}
	if text == "" {
		if d := removeDescrPatch(doc, c); d != nil {
			patches = append(patches, *d)
		}
	} else {
		p, err := setDescrAttr(doc, c, text)
		if err != nil {
			return Annotate(err, op)
		}
		patches = append(patches, p)
	}
	if len(patches) == 0 {
		return nil
	}
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

// setDecorative 设置装饰性标记；为 true 时清除替代文本（§8.1），
// false 时仅清除标记、保留既有 @descr。
func (s *shapeNode) setDecorative(op string, decorative bool) error {
	doc, el, err := s.locate()
	if err != nil {
		return Annotate(err, op)
	}
	c := elementCNvPr(doc, el)
	if c == nil {
		return &OperationError{
			Op: op, Part: string(s.part),
			Message: "shape has no p:cNvPr", Err: ErrMalformedPackage,
		}
	}
	var patches []xmlstore.SpanPatch
	if decorative {
		if d := removeDescrPatch(doc, c); d != nil {
			patches = append(patches, *d)
		}
		cur, _ := c.Attr("", "decorative")
		if cur != "1" {
			p, err := addPlainAttrPatch(doc, c, "decorative", "1")
			if err != nil {
				return Annotate(err, op)
			}
			patches = append(patches, p)
		}
	} else {
		if d := removeDecorativePatch(doc, c); d != nil {
			patches = append(patches, *d)
		}
	}
	if len(patches) == 0 {
		return nil
	}
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

// removeDescrPatch 移除 @descr（存在时）。
func removeDescrPatch(doc *xmlstore.XMLDocument, c *xmlstore.NodeRecord) *xmlstore.SpanPatch {
	for i := range c.Attrs {
		a := &c.Attrs[i]
		if a.Namespace == "" && a.RawName == "descr" {
			p := removeAttrPatch(doc, c, i)
			return &p
		}
	}
	return nil
}

// removeDecorativePatch 移除 @decorative（存在时）。
func removeDecorativePatch(doc *xmlstore.XMLDocument, c *xmlstore.NodeRecord) *xmlstore.SpanPatch {
	for i := range c.Attrs {
		a := &c.Attrs[i]
		if a.Namespace == "" && a.RawName == "decorative" {
			p := removeAttrPatch(doc, c, i)
			return &p
		}
	}
	return nil
}

// setDescrAttr 设置 @descr（不存在则追加）。
func setDescrAttr(doc *xmlstore.XMLDocument, c *xmlstore.NodeRecord, text string) (xmlstore.SpanPatch, error) {
	for i := range c.Attrs {
		a := &c.Attrs[i]
		if a.Namespace == "" && a.RawName == "descr" {
			if a.Value == text {
				return xmlstore.SpanPatch{}, nil
			}
			p := xmlstore.SpanPatch{Start: a.ValueStart, End: a.ValueEnd, Replacement: []byte(text)}
			return p, nil
		}
	}
	esc, err := xmlstore.EscapeAttrValue(text, '"')
	if err != nil {
		return xmlstore.SpanPatch{}, Annotate(mapXMLError(err), "setAltText")
	}
	return addPlainAttrPatch(doc, c, "descr", esc)
}

// addPlainAttrPatch 在开标签内追加 " name="value""（值需已转义）。
func addPlainAttrPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, name, value string) (xmlstore.SpanPatch, error) {
	if n.Source.End > len(doc.Original()) {
		return xmlstore.SpanPatch{}, &OperationError{Message: "invalid node span", Err: ErrMalformedPackage}
	}
	pos := n.OpenEnd - 1
	if n.SelfClosing() {
		pos = n.OpenEnd - 2
	}
	return xmlstore.SpanPatch{Start: pos, End: pos, Replacement: []byte(" " + name + "=\"" + value + "\"")}, nil
}

// ---------- Slide.Shapes / Slide.Placeholders ----------
// Shapes 返回页面顶层形状（spTree 直接子元素，z-order = 文档序）。
// 组合/图形框等返回 *OpaqueShape（内容编辑随后续工作包）；组合的
// 子形状不在此展开。顺序解析不要求理解对象全部内容。
func (s *Slide) Shapes() ([]Shape, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.Shapes")
	}
	doc, tree, err := s.slideTree()
	if err != nil {
		return nil, Annotate(err, "Slide.Shapes")
	}
	out := make([]Shape, 0, len(tree.Children))
	for _, cid := range tree.Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML {
			continue // 非 p 命名空间子元素（mc:AlternateContent 等）不展开
		}
		switch c.Local() {
		case "nvGrpSpPr", "grpSpPr":
			continue // spTree 簿记子元素，非形状
		}
		out = append(out, classifyShape(s.p, s.part, doc, c))
	}
	return out, nil
}

// Placeholder 描述页面占位符形状（方案 §6.1 匹配语义：type 缺省规范
// 化为 "obj"、idx 缺省 0，ECMA CT_Placeholder 默认值）。
type Placeholder struct {
	// Type 是占位符类型（规范化值，如 "title"/"body"/"obj"）。
	Type string
	// Index 是占位符索引（同类型多占位符的区分序号）。
	Index uint32
	// Shape 是占位符对应的形状句柄（*AutoShape/*PictureShape/
	// *OpaqueShape）；文本占位符经 *AutoShape.TextFrame 读写正文。
	Shape Shape
}

// Placeholders 返回页面占位符形状（z-order = 文档序）。占位符是
// 形状元素携带 p:ph（容器 nv*Pr/nvPr 内）；图片占位符同样识别。
func (s *Slide) Placeholders() ([]*Placeholder, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.Placeholders")
	}
	doc, tree, err := s.slideTree()
	if err != nil {
		return nil, Annotate(err, "Slide.Placeholders")
	}
	var out []*Placeholder
	for _, cid := range tree.Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML {
			continue
		}
		switch c.Local() {
		case "nvGrpSpPr", "grpSpPr":
			continue
		}
		k, ok := shapePhKey(doc, c)
		if !ok {
			continue
		}
		out = append(out, &Placeholder{
			Type:  k.typ,
			Index: k.idx,
			Shape: classifyShape(s.p, s.part, doc, c),
		})
	}
	return out, nil
}

// classifyShape 按元素类别返回形状句柄。
func classifyShape(p *Presentation, part opc.PartName, doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) Shape {
	path := recordPath(doc, el.ID)
	switch el.Local() {
	case "pic":
		return &PictureShape{shapeNode: shapeNode{p: p, part: part, path: path}}
	case "sp":
		return &AutoShape{shapeNode: shapeNode{p: p, part: part, path: path}}
	case "grpSp":
		return &GroupShape{shapeNode: shapeNode{p: p, part: part, path: path}}
	case "cxnSp":
		return &OpaqueShape{shapeNode: shapeNode{p: p, part: part, path: path}, kind: ShapeConnector}
	case "graphicFrame":
		return &OpaqueShape{shapeNode: shapeNode{p: p, part: part, path: path}, kind: ShapeGraphicFrame}
	}
	return &OpaqueShape{shapeNode: shapeNode{p: p, part: part, path: path}, kind: ShapeOpaque}
}

// ---------- GroupShape ----------

// GroupShape 是页面组合（p:grpSp）的受控句柄。
//
// 组合含自己的几何框（grpSpPr/a:xfrm：off/ext 为父坐标框，chOff/chExt
// 为子坐标映射源）与子形状（组直接子元素，顺序即组内 z-order）。子形状
// 坐标经组映射 G（非等比缩放+平移）到组父坐标后再组合组级翻转旋转
// （Mgroup=T(C)·R·F·T(-C)·G，§8），嵌套组按父矩阵左乘。组句柄的
// Bounds 返回组框（off/ext）；WorldQuad/WorldAABB 返回组框经自身翻转
// 旋转与祖先组链后的世界边界。
type GroupShape struct {
	shapeNode
}

// Kind 返回形状类别（恒为 ShapeGroup）。
func (g *GroupShape) Kind() ShapeKind { return ShapeGroup }

// Children 返回组合的直接子形状（组内 z-order = 文档序）。
// 嵌套组作为子形状返回（再经其 Children 递归）；组不在此展开
// 顶层簿记元素（nvGrpSpPr/grpSpPr）。子形状的本地坐标空间是组的
// 子坐标（ch 空间），经 WorldQuad 才映射到页面坐标。
func (g *GroupShape) Children() ([]Shape, error) {
	doc, el, err := g.locate()
	if err != nil {
		return nil, Annotate(err, "GroupShape.Children")
	}
	var out []Shape
	for _, cid := range el.Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML {
			continue
		}
		switch c.Local() {
		case "nvGrpSpPr", "grpSpPr":
			continue
		}
		out = append(out, classifyShape(g.p, g.part, doc, c))
	}
	return out, nil
}

// ---------- AutoShape ----------

// AutoShape 是页面 p:sp 形状（文本框或自选图形/占位符）的受控句柄。
// 携带 p:txBody 时经 TextFrame 读写正文；§8.1 替代文本可读写。
type AutoShape struct {
	shapeNode
}

// Kind 返回形状类别：cNvSpPr@txBox="1" 为文本框，否则自选图形。
func (a *AutoShape) Kind() ShapeKind {
	doc, el, err := a.locate()
	if err != nil {
		return ShapeAutoShape
	}
	nvSpPr := childOfKind(doc, el, nsPresentationML, "nvSpPr", 0)
	if nvSpPr == nil {
		return ShapeAutoShape
	}
	sp := childOfKind(doc, nvSpPr, nsPresentationML, "cNvSpPr", 0)
	if sp == nil {
		return ShapeAutoShape
	}
	if v, ok := sp.Attr("", "txBox"); ok && v == "1" {
		return ShapeTextBox
	}
	return ShapeAutoShape
}

// TextFrame 返回形状正文（p:txBody）的 TextFrame；形状无正文返回
// ErrNotFound（如纯图形占位符尚未填充文本）。正文路径记录自当前索引，
// 读/写与 TEXT-01 的 TextFrame 语义一致。
func (a *AutoShape) TextFrame() (*TextFrame, error) {
	doc, el, err := a.locate()
	if err != nil {
		return nil, Annotate(err, "AutoShape.TextFrame")
	}
	tx := childOfKind(doc, el, nsPresentationML, "txBody", 0)
	if tx == nil {
		return nil, &OperationError{
			Op: "AutoShape.TextFrame", Part: string(a.part),
			Message: "shape has no p:txBody", Err: ErrNotFound,
		}
	}
	return &TextFrame{textNode: textNode{p: a.p, part: a.part, path: recordPath(doc, tx.ID)}}, nil
}

// SetAltText 设置替代文本；同时清除装饰性标记（§8.1）。空串清除 @descr。
func (a *AutoShape) SetAltText(text string) error {
	return a.setAltText("AutoShape.SetAltText", text)
}

// SetDecorative 设置装饰性标记；为 true 时清除替代文本（§8.1）。
func (a *AutoShape) SetDecorative(decorative bool) error {
	return a.setDecorative("AutoShape.SetDecorative", decorative)
}

// Placeholder 返回占位符类型与索引（规范化值）；非占位符 ok=false。
func (a *AutoShape) Placeholder() (typ string, idx uint32, ok bool) {
	doc, el, err := a.locate()
	if err != nil {
		return "", 0, false
	}
	k, ok := shapePhKey(doc, el)
	if !ok {
		return "", 0, false
	}
	return k.typ, k.idx, true
}

// ---------- OpaqueShape ----------

// OpaqueShape 是暂不支持内容编辑的形状容器（组合/连接符/图形框/
// 未知扩展）的只读句柄：仅提供通用元信息，不臆测内部结构。
type OpaqueShape struct {
	shapeNode
	kind ShapeKind
}

// Kind 返回形状类别（构造时按元素确定）。
func (o *OpaqueShape) Kind() ShapeKind { return o.kind }
