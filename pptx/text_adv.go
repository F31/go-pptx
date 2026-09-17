package pptx

import (
	textpkg "github.com/F31/go-pptx/internal/document/text"
	"github.com/F31/go-pptx/internal/textutil"
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

// BodyProps 描述 a:bodyPr 中可安全写入的子集（TEXT-03 R 档）。
//
// v2.0：类型与解析/构造已下沉 internal/document/text，此处以 alias
// 暴露（句柄 TextFrame.BodyProps/SetBodyProps 仍在本包）。
type BodyProps = textpkg.BodyProps

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
	return textpkg.ParseBodyProps(doc, bp), nil
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
	if p.Vertical.Set && !textpkg.VertAllowed(p.Vertical.Value) {
		return &OperationError{
			Op: "TextFrame.SetBodyProps", Part: string(t.part),
			Message: "Vertical value not in whitelist: " + p.Vertical.Value, Err: ErrInvalidArgument,
		}
	}
	bp := childOfKind(doc, body, nsDrawingML, "bodyPr", 0)
	if bp == nil {
		if !textpkg.BodyPropsAnySet(p) {
			return nil // 无显式字段，bodyPr 也不存在 → no-op
		}
		// 新建 bodyPr（按 schema 序置于 txBody 首位）。
		frag, err := textpkg.BuildBodyPrFragment("a", p)
		if err != nil {
			return Annotate(err, "TextFrame.SetBodyProps")
		}
		var patch xmlstore.SpanPatch
		if body.SelfClosing() {
			start, end := body.Source.Start, body.Source.End
			expanded := "<" + textpkg.NSPrefix(body) + ":txBody>" + frag + "</" + textpkg.NSPrefix(body) + ":txBody>"
			patch = xmlstore.SpanPatch{Start: start, End: end, Replacement: []byte(expanded)}
		} else {
			patch = xmlstore.SpanPatch{Start: body.OpenEnd, End: body.OpenEnd, Replacement: []byte(frag)}
		}
		out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
		if err != nil {
			return Annotate(mapXMLError(err), "TextFrame.SetBodyProps")
		}
		if err := applySinglePartPatch(t.p, t.part, out); err != nil {
			return Annotate(err, "TextFrame.SetBodyProps")
		}
		return nil
	}
	patches, err := textpkg.ApplyBodyPropsPatch(doc, bp, "a", p)
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
	if err := applySinglePartPatch(t.p, t.part, out); err != nil {
		return Annotate(err, "TextFrame.SetBodyProps")
	}
	return nil
}

// ---------- Field（a:fld 白名单字段） ----------

// FieldKind 是 a:fld@type 的白名单（TEXT-03 R 档全集）。
//
// v2.0：类型与校验/构造已下沉 internal/document/text，此处以 alias 暴露。
type FieldKind = textpkg.FieldKind

// 字段类型常量（alias 到 internal/document/text）。
const (
	// FieldSlideNumber 是页码字段（a:fld type="slidenum"）。
	FieldSlideNumber = textpkg.FieldSlideNumber
	// FieldDateTime 是日期/时间字段（a:fld type="datetime"）。
	FieldDateTime = textpkg.FieldDateTime
)

// FieldSpec 描述插入或读取的字段规格。
type FieldSpec = textpkg.FieldSpec

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
	return textutil.XmlUnescape(string(doc.Original()[tNode.OpenEnd:tNode.CloseStart])), nil
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
		pfx := textpkg.NSPrefix(fld)
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
	if err := applySinglePartPatch(f.p, f.part, out); err != nil {
		return Annotate(err, "Field.SetText")
	}
	return nil
}

// Remove 从段落中删除本字段及其 rPr/t 子树。删后句柄不得再调用。
func (f *Field) Remove() error {
	doc, fld, err := f.locateField()
	if err != nil {
		return Annotate(err, "Field.Remove")
	}
	patch := textutil.RemoveElementPatch(doc, fld)
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return Annotate(mapXMLError(err), "Field.Remove")
	}
	if err := applySinglePartPatch(f.p, f.part, out); err != nil {
		return Annotate(err, "Field.Remove")
	}
	f.fldIdx = -1 // 句柄标为无效
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
	if err := textpkg.ValidateFieldSpec(spec); err != nil {
		return nil, Annotate(err, "Paragraph.AppendField")
	}
	frag, err := textpkg.BuildFieldFragment(doc, para, spec)
	if err != nil {
		return nil, Annotate(err, "Paragraph.AppendField")
	}
	// 定位插入点（与 AddRun 同款：endParaRPr 之前）。
	anchor, _ := textpkg.EndParaAnchor(doc, para)
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
	if err := applySinglePartPatch(p.p, p.part, out); err != nil {
		return nil, Annotate(err, "Paragraph.AppendField")
	}
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
	if err := textpkg.ValidateFieldSpec(spec); err != nil {
		return nil, Annotate(err, "Paragraph.InsertField")
	}
	frag, err := textpkg.BuildFieldFragment(doc, para, spec)
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
	if err := applySinglePartPatch(p.p, p.part, out); err != nil {
		return nil, Annotate(err, "Paragraph.InsertField")
	}
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
