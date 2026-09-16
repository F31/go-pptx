package pptx

import (
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 TEXT-01 的**字符格式写入路径**：TextRun 的 a:rPr 读取/增量
// patch（ExplicitFont/SetFont/ResetFontProperty）与 rPr 片段的展开/构造
// 辅助（buildFontPatches/patchExistingRPr/setRPrAttr/insertRPrChild/
// expandSelfClosingRPr/rPrChildrenFragment）。
// a:rPr 的解析与片段构建见 textfontparse.go；通用补丁/转义见 textutil.go。

// ExplicitFont 返回 Run 的本地字符格式（a:rPr 中显式出现的属性）；
// 未出现的属性 Set=false（样式链解析属 STYLE-01）。
func (r *TextRun) ExplicitFont() (FontStyle, error) {
	doc, run, err := r.locateRun()
	if err != nil {
		return FontStyle{}, Annotate(err, "TextRun.ExplicitFont")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return FontStyle{}, nil
	}
	return parseLocalFont(doc, rPr), nil
}

// SetFont 以 patch 语义应用 style（方案 §20.2）：仅 Set=true 的字段被
// 写入，其余不变；恢复继承请用 ResetFontProperty。
func (r *TextRun) SetFont(style FontStyle) error {
	if !style.anySet() {
		return nil
	}
	doc, run, err := r.locateRun()
	if err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	prefix := run.QName.Prefix
	if prefix == "" {
		prefix = "a"
	}
	patches, err := r.buildFontPatches(doc, run, prefix, style)
	if err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	if len(patches) == 0 {
		return nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.SetFont")
	}
	if err := applySinglePartPatch(r.p, r.part, out); err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	return nil
}

// ResetFontProperty 移除本地格式中的单个属性族并恢复继承；若 rPr 因
// 此变为空元素则一并移除 rPr。未知属性不受影响。
func (r *TextRun) ResetFontProperty(prop FontProperty) error {
	doc, run, err := r.locateRun()
	if err != nil {
		return Annotate(err, "TextRun.ResetFontProperty")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return nil // 无本地格式，恢复继承是 no-op
	}
	attrNames := map[FontProperty]string{}
	childNames := map[FontProperty]string{
		FontPropColor:         "solidFill",
		FontPropLatin:         "latin",
		FontPropEastAsian:     "ea",
		FontPropComplexScript: "cs",
	}
	attrs := map[FontProperty]string{FontPropBold: "b", FontPropItalic: "i", FontPropSize: "sz"}
	attrName, isAttr := attrs[prop]
	childName, isChild := childNames[prop]
	_ = attrNames

	var patches []xmlstore.SpanPatch
	if isAttr {
		for i := range rPr.Attrs {
			a := &rPr.Attrs[i]
			if a.Namespace == "" && a.RawName == attrName {
				patches = append(patches, removeAttrPatch(doc, rPr, i))
			}
		}
	}
	if isChild {
		for _, cid := range rPr.Children {
			c := doc.Node(cid)
			if c.Namespace == nsDrawingML && c.Local() == childName {
				patches = append(patches, removeElementPatch(doc, c))
			}
		}
	}
	if len(patches) == 0 {
		return nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.ResetFontProperty")
	}
	// rPr 若已无属性无子元素则整体删除（连同注释外），由 removeEmptyRPr 处理。
	out, err = r.removeEmptyRPr(out)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.ResetFontProperty")
	}
	if err := applySinglePartPatch(r.p, r.part, out); err != nil {
		return Annotate(err, "TextRun.ResetFontProperty")
	}
	return nil
}

// removeEmptyRPr 重新解析 out；若 run 的 rPr 已无属性与子元素则移除之
// （连同前导空白）。结构编辑不影响路径解析（路径按元素序）。
func (r *TextRun) removeEmptyRPr(out []byte) ([]byte, error) {
	doc, err := xmlstore.Index(out)
	if err != nil {
		return nil, err
	}
	runNode := resolvePath(doc, r.pathToRun())
	if runNode == nil {
		return out, nil
	}
	rPr := childOfKind(doc, runNode, nsDrawingML, "rPr", 0)
	if rPr == nil || len(rPr.Attrs) > 0 || len(rPr.Children) > 0 {
		return out, nil
	}
	patches := []xmlstore.SpanPatch{removeElementPatch(doc, rPr)}
	return xmlstore.ApplyPatches(out, patches)
}

// pathToRun 返回从根到本 run 的完整路径：句柄路径（到 txBody）+ p 序号
// + r 序号。removeEmptyRPr 在补丁后的重解析文档上以此稳定定位 run。
func (r *TextRun) pathToRun() []nodeStep {
	steps := make([]nodeStep, 0, len(r.path)+2)
	steps = append(steps, r.path...)
	steps = append(steps,
		nodeStep{ns: nsDrawingML, local: "p", nth: r.paraIdx},
		nodeStep{ns: nsDrawingML, local: "r", nth: r.runIdx},
	)
	return steps
}

// buildFontPatches 计算应用 style 到 run 的补丁集（不修改 doc）。
func (r *TextRun) buildFontPatches(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord, prefix string, style FontStyle) ([]xmlstore.SpanPatch, error) {
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		// 需要新建 rPr（属性或子元素），插入 run 内容开头。
		frag, err := buildRPrFragment(prefix, style)
		if err != nil {
			return nil, err
		}
		anchor := firstChildOf(doc, run)
		if anchor != nil {
			p, err := xmlstore.InsertBefore(anchor, []byte(frag))
			return []xmlstore.SpanPatch{p}, err
		}
		p, err := xmlstore.AppendChild(run, []byte(frag))
		return []xmlstore.SpanPatch{p}, err
	}
	return patchExistingRPr(doc, run, rPr, prefix, style)
}

// patchExistingRPr 在既有 rPr 上应用 style（属性就地改、子元素增删换）。
func patchExistingRPr(doc *xmlstore.XMLDocument, run, rPr *xmlstore.NodeRecord, prefix string, style FontStyle) ([]xmlstore.SpanPatch, error) {
	// 自闭合 rPr（仅属性）：整体重建为带子元素的展开形态，避免属性补丁
	// 与内容补丁在 '/>' 处重叠冲突。
	if rPr.SelfClosing() {
		expanded, err := expandSelfClosingRPr(doc, rPr, prefix, style)
		if err != nil {
			return nil, err
		}
		return []xmlstore.SpanPatch{{
			Start: rPr.Source.Start, End: rPr.Source.End, Replacement: []byte(expanded),
		}}, nil
	}
	var patches []xmlstore.SpanPatch

	// 属性类：b/i/sz。
	if style.Bold.Set {
		p, err := setRPrAttr(doc, rPr, "b", boolVal(style.Bold.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}
	if style.Italic.Set {
		p, err := setRPrAttr(doc, rPr, "i", boolVal(style.Italic.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}
	if style.Size.Set {
		p, err := setRPrAttr(doc, rPr, "sz", sizeCentipoints(style.Size.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}

	// 子元素类：color/latin/ea/cs。
	if style.Color.Set {
		if !style.Color.Value.Valid() {
			return nil, &OperationError{Message: "empty ColorSpec", Err: ErrInvalidArgument}
		}
		fill := fillChildOf(doc, rPr)
		frag := solidFillFragment(prefix, style.Color.Value)
		if fill != nil && fill.Local() == "solidFill" {
			// 就地替换 solidFill（区间一致，无顺序问题）。
			patches = append(patches, xmlstore.SpanPatch{
				Start: fill.Source.Start, End: fill.Source.End, Replacement: []byte(frag),
			})
		} else if fill != nil {
			// 其它填充类型被显式颜色替换。
			patches = append(patches, xmlstore.SpanPatch{
				Start: fill.Source.Start, End: fill.Source.End, Replacement: []byte(frag),
			})
		} else {
			p, err := insertRPrChild(doc, rPr, frag, fillRank)
			if err != nil {
				return nil, err
			}
			patches = append(patches, p)
		}
	}
	for _, fc := range []struct {
		set  Optional[string]
		kind string
		rank int
	}{
		{style.Latin, "latin", latinRank},
		{style.EastAsian, "ea", eaRank},
		{style.ComplexScript, "cs", csRank},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return nil, Annotate(mapXMLError(err), "SetFont")
		}
		frag := "<" + prefix + ":" + fc.kind + ` typeface="` + esc + `"/>`
		existing := childOfKind(doc, rPr, nsDrawingML, fc.kind, 0)
		if existing != nil {
			patches = append(patches, xmlstore.SpanPatch{
				Start: existing.Source.Start, End: existing.Source.End, Replacement: []byte(frag),
			})
		} else {
			p, err := insertRPrChild(doc, rPr, frag, fc.rank)
			if err != nil {
				return nil, err
			}
			patches = append(patches, p)
		}
	}
	return patches, nil
}

// setRPrAttr 设置 rPr 属性值；属性不存在时在开标签内追加。
// 返回 nil 表示值未变化无需补丁。
func setRPrAttr(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, name, value, prefix string) (*xmlstore.SpanPatch, error) {
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			if a.Value == value {
				return nil, nil
			}
			p := xmlstore.SpanPatch{Start: a.ValueStart, End: a.ValueEnd, Replacement: []byte(value)}
			return &p, nil
		}
	}
	// 追加到开标签末尾（'>' 或 '/>' 之前；自闭合时插到 '/' 前）。
	pos := rPr.OpenEnd - 1
	if rPr.SelfClosing() {
		pos = rPr.OpenEnd - 2
	}
	repl := []byte(" " + name + "=\"" + value + "\"")
	p := xmlstore.SpanPatch{Start: pos, End: pos, Replacement: repl}
	return &p, nil
}

// insertRPrChild 把 fragment 插入 rPr 的 schema 序位置。rPr 含无法
// 判定族别的未知子元素且目标排在其后时返回 ErrUnsupportedEdit
// （不做可能破坏顺序的猜测）。
func insertRPrChild(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, frag string, targetRank int) (xmlstore.SpanPatch, error) {
	insertPos := -1
	for i, cid := range rPr.Children {
		c := doc.Node(cid)
		rank, ok := rPrChildRank(c.Namespace, c.Local())
		if !ok {
			// 未知子元素：若其后还有已知/目标插入点将破坏顺序 → 拒绝。
			return xmlstore.SpanPatch{}, &OperationError{
				Op:      "TextRun.SetFont",
				Part:    "",
				Message: "cannot safely order new run property around unknown child <" + c.Name() + ">",
				Err:     ErrUnsupportedEdit,
			}
		}
		if rank >= targetRank {
			insertPos = c.Source.Start
			break
		}
		_ = i
	}
	if insertPos < 0 {
		// 追加到末尾（rPr 闭标签前）。
		insertPos = rPr.CloseStart
	}
	return xmlstore.SpanPatch{Start: insertPos, End: insertPos, Replacement: []byte(frag)}, nil
}

// expandSelfClosingRPr 把自闭合 rPr（保留全部原属性）展开为带 style
// 子元素的形态。
func expandSelfClosingRPr(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, prefix string, style FontStyle) (string, error) {
	qp := rPr.QName.Prefix
	if qp == "" {
		qp = "a"
	}
	// 原开标签内属性（去掉 '<pfx:rPr' 与 '/>'），逐条重建。
	var sb strings.Builder
	sb.WriteString("<" + qp + ":rPr")
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if isNSDeclAttr(a.RawName) {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(a.Value, '"')
		if err != nil {
			return "", Annotate(mapXMLError(err), "SetFont")
		}
		sb.WriteString(" " + a.RawName + "=\"" + esc + "\"")
	}
	// 应用 style 属性（去重）。
	styleAttrs := map[string]string{}
	if style.Bold.Set {
		styleAttrs["b"] = boolVal(style.Bold.Value)
	}
	if style.Italic.Set {
		styleAttrs["i"] = boolVal(style.Italic.Value)
	}
	if style.Size.Set {
		styleAttrs["sz"] = sizeCentipoints(style.Size.Value)
	}
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if v, ok := styleAttrs[a.RawName]; ok {
			delete(styleAttrs, a.RawName)
			_ = v
		}
	}
	// 剩余 style 属性按固定序输出（b/i/sz）。
	ordered := []string{"b", "i", "sz"}
	for _, k := range ordered {
		if v, ok := styleAttrs[k]; ok {
			sb.WriteString(" " + k + "=\"" + v + "\"")
		}
	}
	if !style.anyChildSet() {
		sb.WriteString("/>")
		return sb.String(), nil
	}
	sb.WriteString(">")
	frag, err := rPrChildrenFragment(prefix, style)
	if err != nil {
		return "", err
	}
	sb.WriteString(frag)
	sb.WriteString("</" + qp + ":rPr>")
	return sb.String(), nil
}

// rPrChildrenFragment 生成 rPr 子元素（color/latin/ea/cs），供自闭合展开。
func rPrChildrenFragment(prefix string, style FontStyle) (string, error) {
	var children []string
	if style.Color.Set {
		if !style.Color.Value.Valid() {
			return "", &OperationError{Message: "empty ColorSpec", Err: ErrInvalidArgument}
		}
		children = append(children, solidFillFragment(prefix, style.Color.Value))
	}
	for _, fc := range []struct {
		set  Optional[string]
		kind string
	}{
		{style.Latin, "latin"},
		{style.EastAsian, "ea"},
		{style.ComplexScript, "cs"},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return "", Annotate(mapXMLError(err), "SetFont")
		}
		children = append(children, "<"+prefix+":"+fc.kind+` typeface="`+esc+`"/>`)
	}
	return strings.Join(children, ""), nil
}
