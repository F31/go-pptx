package pptx

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 TEXT-03（方案 V2.6 §2.3 矩阵"文本级 文本框高级项 / 字段
// 全集"，§24 工作包 TEXT-03）：a:bodyPr 的 numCol/vert/anchorCtr 与
// a:fld 白名单字段的受限编辑。
//
// 设计原则（与方案 §7 一致）：
//   - 受限白名单：未知 vert 值、未知字段类型、未识别的 datetime guide
//     全部拒绝并返回 ErrUnsupportedEdit，不做部分合并。
//   - bodyPr 是文本框级元素（位于 txBody 子元素序：bodyPr → lstStyle →
//     p → ...），与段落属性（a:pPr）/Run 属性（a:rPr）独立。R 档只覆
//     盖 numCol/vert/anchorCtr 三项；其它属性（rot/spcFirstLastPara/
//     wrap/wrapSquare/...）保留为 UnsupportedEdit 边界。
//   - 字段是段落级内联节点（与 a:r 平级，但 local="fld"），句柄路径以
//     文档序定位，AddField/RemoveField 与现有 AddRun 走同一套 patch
//     机制。Paragraph.Text() 拼接时字段以缓存显示文本（a:t 内容）嵌
//     入；Paragraph.Runs() 不含字段（既定语义保持，TEXT-02 跨 Run
//     替换亦按 a:fld 视作硬边界）。

// ---------- BodyProps（文本框级高级属性） ----------

// BodyProps 描述 a:bodyPr 中可安全写入的子集（TEXT-03 R 档）：
//   - Columns（numCol）：分栏数，>0；Columns=0 等价于"未设置"，删除
//     本地属性以恢复主题继承；
//   - Vertical（vert）：竖排方向；未设置取空串。合法取值见 vertAllowed；
//   - AnchorCenter（anchorCtr）：文本框内垂直居中（bool）。
//
// 三个字段都遵循"未提及属性 = 不动"语义：调用方仅 Set=true 的字段会
// 被写入或覆盖；其它字段在 XML 中保留原值。
type BodyProps struct {
	Columns      Optional[int]
	Vertical     Optional[string]
	AnchorCenter Optional[bool]
}

// vertAllowed 是 vert 属性的合法取值（OOXML ST_TextVerticalType）。
// 不在表内的值（如自定义字符串）→ ErrInvalidArgument，不写也不返回
// "未知"，避免半生不熟的状态。
var vertAllowed = map[string]bool{
	"":              true, // 空串用作"删除本地属性"
	"horz":          true,
	"vert":          true,
	"vert270":       true,
	"wordArtVert":   true,
	"eaVert":        true,
	"mongolianVert": true,
}

// bodyPropsAnySet 检查 BodyProps 是否含显式设置。
func bodyPropsAnySet(p BodyProps) bool {
	return p.Columns.Set || p.Vertical.Set || p.AnchorCenter.Set
}

// TextFrame 公开 BodyProps：BodyProps() 读取当前本地属性（Set=false
// 表示"未设置/继承"），SetBodyProps(p) 以 patch 语义应用。
//
// BodyProps() 返回的零值表示 bodyPr 不存在或全部属性未设置。
func (t *TextFrame) BodyProps() (BodyProps, error) {
	doc, body, err := t.locate()
	if err != nil {
		return BodyProps{}, Annotate(err, "TextFrame.BodyProps")
	}
	bp := childOfKind(doc, body, nsDrawingML, "bodyPr", 0)
	if bp == nil {
		return BodyProps{}, nil
	}
	return parseBodyProps(doc, bp), nil
}

// SetBodyProps 应用 bodyPr 补丁：Columns=0/Vertical=""/AnchorCenter 未
// 设置 → 对应属性从 XML 删除；否则写入新值；Columns 必须 ≥1 否则
// ErrInvalidArgument；Vertical 不在白名单同样返回 ErrInvalidArgument。
//
// bodyPr 缺失时自动插入（schema 序：bodyPr 必须是 txBody 首个子元素）。
func (t *TextFrame) SetBodyProps(p BodyProps) error {
	doc, body, err := t.locate()
	if err != nil {
		return Annotate(err, "TextFrame.SetBodyProps")
	}
	if p.Columns.Set && p.Columns.Value < 0 {
		return &OperationError{
			Op: "TextFrame.SetBodyProps", Part: string(t.part),
			Message: "Columns must be >= 0 (use 0 to clear)", Err: ErrInvalidArgument,
		}
	}
	if p.Vertical.Set && !vertAllowed[p.Vertical.Value] {
		return &OperationError{
			Op: "TextFrame.SetBodyProps", Part: string(t.part),
			Message: "Vertical value not in whitelist: " + p.Vertical.Value, Err: ErrInvalidArgument,
		}
	}
	bp := childOfKind(doc, body, nsDrawingML, "bodyPr", 0)
	if bp == nil {
		if !bodyPropsAnySet(p) {
			return nil // 无显式字段，bodyPr 也不存在 → no-op
		}
		// 新建 bodyPr（按 schema 序置于 txBody 首位）。
		frag, err := buildBodyPrFragment("a", p)
		if err != nil {
			return Annotate(err, "TextFrame.SetBodyProps")
		}
		var patch xmlstore.SpanPatch
		if body.SelfClosing() {
			start, end := body.Source.Start, body.Source.End
			expanded := "<" + nsPrefix(body) + ":txBody>" + frag + "</" + nsPrefix(body) + ":txBody>"
			patch = xmlstore.SpanPatch{Start: start, End: end, Replacement: []byte(expanded)}
		} else {
			patch = xmlstore.SpanPatch{Start: body.OpenEnd, End: body.OpenEnd, Replacement: []byte(frag)}
		}
		out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
		if err != nil {
			return Annotate(mapXMLError(err), "TextFrame.SetBodyProps")
		}
		if err := t.p.stagePatch(t.part, out); err != nil {
			return Annotate(err, "TextFrame.SetBodyProps")
		}
		t.p.commit()
		return nil
	}
	patches, err := applyBodyPropsPatch(doc, bp, "a", p)
	if err != nil {
		return Annotate(err, "TextFrame.SetBodyProps")
	}
	if len(patches) == 0 {
		return nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "TextFrame.SetBodyProps")
	}
	if err := t.p.stagePatch(t.part, out); err != nil {
		return Annotate(err, "TextFrame.SetBodyProps")
	}
	t.p.commit()
	return nil
}

// parseBodyProps 从 a:bodyPr 抽取 R 档字段（全部以 Set=true 输出）。
func parseBodyProps(doc *xmlstore.XMLDocument, bp *xmlstore.NodeRecord) BodyProps {
	var out BodyProps
	for i := range bp.Attrs {
		a := &bp.Attrs[i]
		if a.Namespace != "" {
			continue
		}
		switch a.RawName {
		case "numCol":
			out.Columns = Optional[int]{Value: int(intAttr(a.Value)), Set: true}
		case "vert":
			out.Vertical = Optional[string]{Value: a.Value, Set: true}
		case "anchorCtr":
			out.AnchorCenter = Optional[bool]{Value: a.Value == "1" || a.Value == "true", Set: true}
		}
	}
	return out
}

// applyBodyPropsPatch 计算对既有 bodyPr 的属性补丁集。
//
// Columns/Vertical/AnchorCenter 三个字段各自独立：
//   - Columns.Set：Columns.Value>0 写入/更新；Columns.Value<=0 删除
//   - Vertical.Set：Vertical.Value 非空写入/更新；空串删除
//   - AnchorCenter.Set：按 Value 写入/更新 true/false
func applyBodyPropsPatch(doc *xmlstore.XMLDocument, bp *xmlstore.NodeRecord, prefix string, p BodyProps) ([]xmlstore.SpanPatch, error) {
	var patches []xmlstore.SpanPatch
	// numCol
	if p.Columns.Set {
		if p.Columns.Value >= 1 {
			v := strconv.Itoa(p.Columns.Value)
			if pat, err := setAttrPatch(bp, "numCol", v, prefix); err != nil {
				return nil, err
			} else if pat != nil {
				patches = append(patches, *pat)
			}
		} else {
			if pat := removeAttrIfExists(doc, bp, "numCol"); pat != nil {
				patches = append(patches, *pat)
			}
		}
	}
	// vert
	if p.Vertical.Set {
		if p.Vertical.Value != "" {
			if pat, err := setAttrPatch(bp, "vert", p.Vertical.Value, prefix); err != nil {
				return nil, err
			} else if pat != nil {
				patches = append(patches, *pat)
			}
		} else {
			if pat := removeAttrIfExists(doc, bp, "vert"); pat != nil {
				patches = append(patches, *pat)
			}
		}
	}
	// anchorCtr
	if p.AnchorCenter.Set {
		v := "0"
		if p.AnchorCenter.Value {
			v = "1"
		}
		if pat, err := setAttrPatch(bp, "anchorCtr", v, prefix); err != nil {
			return nil, err
		} else if pat != nil {
			patches = append(patches, *pat)
		}
	}
	return patches, nil
}

// setAttrPatch 在 bp 上写入/更新属性值；返回 nil 表示值未变化。
//
// 自闭合元素（<foo …/>）的 OpenEnd 指向 '>' 之后；插入位置需取 '/'
// 之前（即 OpenEnd-2）；非自闭合元素 OpenEnd 指向 '>' 之后，插入位
// 置取 '>' 之前（OpenEnd-1）。
func setAttrPatch(n *xmlstore.NodeRecord, name, value, _ string) (*xmlstore.SpanPatch, error) {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			if a.Value == value {
				return nil, nil
			}
			p := xmlstore.SpanPatch{Start: a.ValueStart, End: a.ValueEnd, Replacement: []byte(value)}
			return &p, nil
		}
	}
	pos := n.OpenEnd - 1
	if n.SelfClosing() {
		pos = n.OpenEnd - 2
	}
	repl := []byte(" " + name + `="` + value + `"`)
	p := xmlstore.SpanPatch{Start: pos, End: pos, Replacement: repl}
	return &p, nil
}

// removeAttrIfExists 删除指定本地属性；不存在返回 nil。复用 text.go
// removeAttrPatch（前导空白 + 闭合引号一并删除）。
func removeAttrIfExists(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, name string) *xmlstore.SpanPatch {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			sp := removeAttrPatch(doc, n, i)
			return &sp
		}
	}
	return nil
}

// buildBodyPrFragment 构造完整 bodyPr 片段（自闭合形式）。
func buildBodyPrFragment(prefix string, p BodyProps) (string, error) {
	var sb strings.Builder
	sb.WriteString("<" + prefix + ":bodyPr")
	if p.Columns.Set && p.Columns.Value >= 1 {
		sb.WriteString(` numCol="` + strconv.Itoa(p.Columns.Value) + `"`)
	}
	if p.Vertical.Set && p.Vertical.Value != "" {
		sb.WriteString(` vert="` + p.Vertical.Value + `"`)
	}
	if p.AnchorCenter.Set {
		v := "0"
		if p.AnchorCenter.Value {
			v = "1"
		}
		sb.WriteString(` anchorCtr="` + v + `"`)
	}
	sb.WriteString("/>")
	return sb.String(), nil
}

// nsPrefix 返回节点 QName 的前缀；缺省回落 "a"。
func nsPrefix(n *xmlstore.NodeRecord) string {
	if n.QName.Prefix != "" {
		return n.QName.Prefix
	}
	return "a"
}

// ---------- Field（a:fld 白名单字段） ----------

// FieldKind 是 a:fld@type 的白名单（TEXT-03 R 档全集）。未列入表内的
// 类型（如 user/pageNumber/fileName/title 等含动态行为或高度依赖客
// 户端数据的字段）本库不实现，按 ErrUnsupportedEdit 拒绝整体写入。
//
// slidenum：当前页码（PowerPoint 渲染时重算）；
// datetime：日期/时间（guide 指定格式串；空 guide 视作默认长格式）。
type FieldKind string

const (
	// FieldSlideNumber 是页码字段（a:fld type="slidenum"）。
	FieldSlideNumber FieldKind = "slidenum"
	// FieldDateTime 是日期/时间字段（a:fld type="datetime"）。
	// FieldSpec.Guide 指定格式串（如 "YYYY-MM-DD"、"h:mm AM/PM"），
	// 白名单见 datetimeGuideAllowed；不在表内的 guide → ErrInvalidArgument。
	FieldDateTime FieldKind = "datetime"
)

// datetimeGuideAllowed 是 datetime 字段格式白名单。OOXML 文档定义
// 了一组预置格式（"YYYY-MM-DD"/"hh:mm:ss"/...）；不在表内的字符串按
// 字面保留写回但运行时不会被 PowerPoint 识别——本库采取保守策略，
// 拒绝未识别的 guide。
var datetimeGuideAllowed = map[string]bool{
	"":                true, // 空 guide 由 PowerPoint 取默认
	"YYYY-MM-DD":      true,
	"YYYY/MM/DD":      true,
	"DD-MM-YYYY":      true,
	"DD/MM/YYYY":      true,
	"MM-DD-YYYY":      true,
	"MM/DD/YYYY":      true,
	"hh:mm:ss":        true,
	"h:mm:ss AM/PM":   true,
	"hh:mm":           true,
	"h:mm AM/PM":      true,
	"YYYY-MM":         true,
	"YYYY/MM":         true,
	"MMM YY":          true,
	"MMMM YYYY":       true,
	"MMMM YY":         true,
	"MMM YYYY":        true,
	"DDDD, MMMM YYYY": true,
}

// FieldSpec 描述插入或读取的字段规格。
type FieldSpec struct {
	Kind  FieldKind // 类型
	Guide string    // 仅 datetime 有效；其它类型必填空串
	Text  string    // 缓存显示文本（写入时作为 a:t 初值；读取时是当前缓存）
	Style FontStyle // 可选 rPr（Set=true 字段应用）
}

// Field 是段落级字段（a:fld）的受控句柄。路径与 TextRun 平行：定位至
// 段落内 a:fld 同名单元素的 nth 个。
//
// 字段不可作为 TextRun 句柄使用；Paragraph.Runs() 不含字段（语义见
// TEXT-02 跨 Run 替换与 §20.2 段落属性）。
type Field struct {
	textNode
	paraIdx int
	fldIdx  int // 在所属段落 a:fld 兄弟中的序号（-1 = 无效句柄）
}

// locateField 定位 a:fld 节点。
func (f *Field) locateField() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, para, err := (&Paragraph{textNode: f.textNode, idx: f.paraIdx}).locatePara()
	if err != nil {
		return nil, nil, err
	}
	fld := childOfKind(doc, para, nsDrawingML, "fld", f.fldIdx)
	if fld == nil || f.fldIdx < 0 {
		return nil, nil, Annotate(ErrStaleHandle, "Field")
	}
	return doc, fld, nil
}

// pathToField 返回从根到本 fld 的完整路径：句柄路径 + p 序号 + fld 序号。
func (f *Field) pathToField() []nodeStep {
	steps := make([]nodeStep, 0, len(f.path)+2)
	steps = append(steps, f.path...)
	steps = append(steps,
		nodeStep{ns: nsDrawingML, local: "p", nth: f.paraIdx},
		nodeStep{ns: nsDrawingML, local: "fld", nth: f.fldIdx},
	)
	return steps
}

// Text 返回字段缓存显示文本（a:t 内容解码）。
func (f *Field) Text() (string, error) {
	doc, fld, err := f.locateField()
	if err != nil {
		return "", Annotate(err, "Field.Text")
	}
	tNode := childOfKind(doc, fld, nsDrawingML, "t", 0)
	if tNode == nil {
		return "", nil
	}
	if tNode.SelfClosing() {
		return "", nil
	}
	return xmlUnescape(string(doc.Original()[tNode.OpenEnd:tNode.CloseStart])), nil
}

// Kind 返回字段类型。
func (f *Field) Kind() (FieldKind, error) {
	_, fld, err := f.locateField()
	if err != nil {
		return "", Annotate(err, "Field.Kind")
	}
	v, ok := fld.Attr("", "type")
	if !ok {
		return "", Annotate(ErrStaleHandle, "Field.Kind")
	}
	return FieldKind(v), nil
}

// Guide 返回字段 guide（仅 datetime 类型有值）。
//
// OOXML 标准中 datetime 字段的格式串在 PowerPoint 中以 a:fld 上的扩展
// 属性 `fldGuide="..."` 携带（属于微软扩展形式，非 ST 字段类型）；本
// 库沿用此约定。Reader 兼容旧文档中偶现的 `guid`（属性名兼容）。
func (f *Field) Guide() (string, error) {
	_, fld, err := f.locateField()
	if err != nil {
		return "", Annotate(err, "Field.Guide")
	}
	if v, ok := fld.Attr("", "fldGuide"); ok && v != "" {
		return v, nil
	}
	if v, ok := fld.Attr("", "guid"); ok {
		return v, nil
	}
	return "", nil
}

// SetText 替换字段缓存显示文本（a:t 内容）。
//
// 若 a:fld 仍为自闭合形态（无 rPr/t 子节点），先把 fld 扩展为
// `<a:fld …><a:t>…</a:t></a:fld>` 再写入 t 内容；已有 a:t 就地替换。
func (f *Field) SetText(text string) error {
	doc, fld, err := f.locateField()
	if err != nil {
		return Annotate(err, "Field.SetText")
	}
	esc, err := xmlstore.EscapeText(text)
	if err != nil {
		return Annotate(mapXMLError(err), "Field.SetText")
	}
	var patches []xmlstore.SpanPatch
	tNode := childOfKind(doc, fld, nsDrawingML, "t", 0)
	if tNode != nil {
		patches = append(patches, xmlstore.SpanPatch{
			Start: tNode.OpenEnd, End: tNode.CloseStart, Replacement: []byte(esc),
		})
	} else {
		// 自闭合 fld 必须先把 '/>' 替换为 '>…</a:fld>'，再插入 a:t。
		pfx := nsPrefix(fld)
		expanded := ">{" + pfx + ":t}" + esc + "</" + pfx + ":t></" + pfx + ":fld>"
		patches = append(patches, xmlstore.SpanPatch{
			Start:       fld.Source.End - 2, // '/' 之前
			End:         fld.Source.End,     // '>' 之后
			Replacement: []byte(expanded),
		})
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "Field.SetText")
	}
	if err := f.p.stagePatch(f.part, out); err != nil {
		return Annotate(err, "Field.SetText")
	}
	f.p.commit()
	return nil
}

// Remove 从段落中删除本字段及其 rPr/t 子树。删后句柄不得再调用。
func (f *Field) Remove() error {
	doc, fld, err := f.locateField()
	if err != nil {
		return Annotate(err, "Field.Remove")
	}
	patch := removeElementPatch(doc, fld)
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return Annotate(mapXMLError(err), "Field.Remove")
	}
	if err := f.p.stagePatch(f.part, out); err != nil {
		return Annotate(err, "Field.Remove")
	}
	f.fldIdx = -1 // 句柄标为无效
	f.p.commit()
	return nil
}

// Paragraph.Fields 返回段落内的字段句柄切片（文档序；仅 a:fld）。
func (p *Paragraph) Fields() ([]*Field, error) {
	doc, para, err := p.locatePara()
	if err != nil {
		return nil, Annotate(err, "Paragraph.Fields")
	}
	n := countKind(doc, para, nsDrawingML, "fld")
	out := make([]*Field, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &Field{
			textNode: textNode{p: p.p, part: p.part, path: p.path},
			paraIdx:  p.idx,
			fldIdx:   i,
		})
	}
	return out, nil
}

// AppendField 在段落末尾（a:endParaRPr 之前）追加一个字段，返回新字段
// 句柄。Kind 不在白名单返回 ErrUnsupportedEdit；datetime 且 Guide 不
// 在白名单返回 ErrInvalidArgument。
func (p *Paragraph) AppendField(spec FieldSpec) (*Field, error) {
	doc, para, err := p.locatePara()
	if err != nil {
		return nil, Annotate(err, "Paragraph.AppendField")
	}
	if err := validateFieldSpec(spec); err != nil {
		return nil, Annotate(err, "Paragraph.AppendField")
	}
	frag, err := buildFieldFragment(doc, para, spec)
	if err != nil {
		return nil, Annotate(err, "Paragraph.AppendField")
	}
	// 定位插入点（与 AddRun 同款：endParaRPr 之前）。
	anchor, _ := p.endParaAnchor(doc, para)
	var patch xmlstore.SpanPatch
	if anchor != nil {
		patch, err = xmlstore.InsertBefore(anchor, []byte(frag))
	} else {
		patch, err = xmlstore.AppendChild(para, []byte(frag))
	}
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Paragraph.AppendField")
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Paragraph.AppendField")
	}
	if err := p.p.stagePatch(p.part, out); err != nil {
		return nil, Annotate(err, "Paragraph.AppendField")
	}
	p.p.commit()
	return p.lastField(), nil
}

// InsertField 在段落中 at 所指 Run 之后插入字段；at==nil 等价 AppendField。
// at 句柄失效（Run 已不存在）返回 ErrStaleHandle。
func (p *Paragraph) InsertField(at *TextRun, spec FieldSpec) (*Field, error) {
	if at == nil {
		return p.AppendField(spec)
	}
	doc, para, err := p.locatePara()
	if err != nil {
		return nil, Annotate(err, "Paragraph.InsertField")
	}
	docAt, runAt, err := at.locateRun()
	if err != nil {
		return nil, Annotate(err, "Paragraph.InsertField")
	}
	if docAt != doc || runAt.Parent != para.ID {
		return nil, &OperationError{
			Op: "Paragraph.InsertField", Part: string(p.part),
			Message: "anchor run not in target paragraph", Err: ErrStaleHandle,
		}
	}
	if err := validateFieldSpec(spec); err != nil {
		return nil, Annotate(err, "Paragraph.InsertField")
	}
	frag, err := buildFieldFragment(doc, para, spec)
	if err != nil {
		return nil, Annotate(err, "Paragraph.InsertField")
	}
	// 插入位置：runAt 之后。在 patch 视角即 runAt 的 Source.End 处插入。
	insertPos := runAt.Source.End
	patch := xmlstore.SpanPatch{Start: insertPos, End: insertPos, Replacement: []byte(frag)}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Paragraph.InsertField")
	}
	if err := p.p.stagePatch(p.part, out); err != nil {
		return nil, Annotate(err, "Paragraph.InsertField")
	}
	p.p.commit()
	return p.lastField(), nil
}

// lastField 在提交后重新解析段落并返回最后一个 a:fld 的句柄。
func (p *Paragraph) lastField() *Field {
	doc, para, err := p.locatePara()
	if err != nil {
		return &Field{textNode: p.textNode, paraIdx: p.idx, fldIdx: -1}
	}
	n := countKind(doc, para, nsDrawingML, "fld")
	return &Field{textNode: p.textNode, paraIdx: p.idx, fldIdx: n - 1}
}

// validateFieldSpec 校验 FieldSpec：kind 白名单 + datetime guide 白名单。
func validateFieldSpec(spec FieldSpec) error {
	switch spec.Kind {
	case FieldSlideNumber:
		if spec.Guide != "" {
			return &OperationError{
				Op: "validateFieldSpec", Message: "slidenum field has no guide",
				Err: ErrInvalidArgument,
			}
		}
	case FieldDateTime:
		if !datetimeGuideAllowed[spec.Guide] {
			return &OperationError{
				Op:      "validateFieldSpec",
				Message: "datetime guide not in whitelist: " + spec.Guide,
				Err:     ErrInvalidArgument,
			}
		}
	default:
		return &OperationError{
			Op:      "validateFieldSpec",
			Message: "Unknown field kind: " + string(spec.Kind),
			Err:     ErrUnsupportedEdit,
		}
	}
	if spec.Text != "" {
		if _, err := xmlstore.EscapeText(spec.Text); err != nil {
			return Annotate(mapXMLError(err), "validateFieldSpec")
		}
	}
	return nil
}

// buildFieldFragment 生成 a:fld 片段：总是展开形态（含 a:t），便于
// PowerPoint 在打开时识别为字段并按 guide 重新计算。
func buildFieldFragment(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord, spec FieldSpec) (string, error) {
	prefix := runPrefix(doc, para) // 与 Run/Field 共用前缀约定
	var sb strings.Builder
	sb.WriteString("<" + prefix + ":fld")
	sb.WriteString(` type="` + string(spec.Kind) + `"`)
	if spec.Kind == FieldDateTime && spec.Guide != "" {
		sb.WriteString(` fldGuide="` + spec.Guide + `"`)
	}
	sb.WriteString(">")
	if spec.Style.anySet() {
		f, err := buildRPrFragment(prefix, spec.Style)
		if err != nil {
			return "", err
		}
		sb.WriteString(f)
	}
	esc, err := xmlstore.EscapeText(spec.Text)
	if err != nil {
		return "", err
	}
	sb.WriteString("<" + prefix + ":t>" + esc + "</" + prefix + ":t>")
	sb.WriteString("</" + prefix + ":fld>")
	return sb.String(), nil
}

// ---------- 段落 Text() 字段展示（TEXT-03 必含 §7.1） ----------

// paragraphRunOrField 是段落内 a:r 与 a:fld 的有序序列（XML 文档序）。
type paragraphRunOrField struct {
	isField bool
	run     *xmlstore.NodeRecord
	fld     *xmlstore.NodeRecord
}

// paragraphChildren 解析段落子元素为 a:r / a:fld 有序列表（忽略 pPr /
// endParaRPr / extLst 等非内容元素）。
func paragraphChildren(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) []paragraphRunOrField {
	var out []paragraphRunOrField
	for _, cid := range para.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "r":
			out = append(out, paragraphRunOrField{run: c})
		case "fld":
			out = append(out, paragraphRunOrField{isField: true, fld: c})
		}
	}
	return out
}

// paragraphText 拼接段落 a:r/a:fld 缓存文本；与原 Paragraph.Text 唯一
// 差异是 a:fld 节点以 a:t 缓存文本嵌入（字段位置即显示位置）。
// 此函数在 text.go Paragraph.Text() 内调用，避免循环 import。
func paragraphText(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) string {
	var sb strings.Builder
	for _, item := range paragraphChildren(doc, para) {
		if item.isField {
			tNode := childOfKind(doc, item.fld, nsDrawingML, "t", 0)
			if tNode == nil || tNode.SelfClosing() {
				continue
			}
			sb.WriteString(xmlUnescape(string(doc.Original()[tNode.OpenEnd:tNode.CloseStart])))
			continue
		}
		for _, tid := range item.run.Children {
			tt := doc.Node(tid)
			if tt.Namespace == nsDrawingML && tt.Local() == "t" {
				if tt.SelfClosing() {
					continue
				}
				sb.WriteString(xmlUnescape(string(doc.Original()[tt.OpenEnd:tt.CloseStart])))
			}
		}
	}
	return sb.String()
}
