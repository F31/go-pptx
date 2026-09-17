package text

import (
	"strings"

	"github.com/F31/go-pptx/v2/internal/document/model"
	"github.com/F31/go-pptx/v2/internal/document/style"
	"github.com/F31/go-pptx/v2/internal/errs"
	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/textutil"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// BuildFontPatches 计算应用 st 到 run 的补丁集（不修改 doc）。
func BuildFontPatches(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord, prefix string, st style.FontStyle) ([]xmlstore.SpanPatch, error) {
	rPr := xmlstore.ChildOfKind(doc, run, ooxmlns.DrawingML, "rPr", 0)
	if rPr == nil {
		// 需要新建 rPr（属性或子元素），插入 run 内容开头。
		frag, err := BuildRPrFragment(prefix, st)
		if err != nil {
			return nil, err
		}
		anchor := textutil.FirstChildOf(doc, run)
		if anchor != nil {
			p, err := xmlstore.InsertBefore(anchor, []byte(frag))
			return []xmlstore.SpanPatch{p}, err
		}
		p, err := xmlstore.AppendChild(run, []byte(frag))
		return []xmlstore.SpanPatch{p}, err
	}
	return PatchExistingRPr(doc, run, rPr, prefix, st)
}

// PatchExistingRPr 在既有 rPr 上应用 st（属性就地改、子元素增删换）。
func PatchExistingRPr(doc *xmlstore.XMLDocument, run, rPr *xmlstore.NodeRecord, prefix string, st style.FontStyle) ([]xmlstore.SpanPatch, error) {
	// 自闭合 rPr（仅属性）：整体重建为带子元素的展开形态，避免属性补丁
	// 与内容补丁在 '/>' 处重叠冲突。
	if rPr.SelfClosing() {
		expanded, err := ExpandSelfClosingRPr(doc, rPr, prefix, st)
		if err != nil {
			return nil, err
		}
		return []xmlstore.SpanPatch{{
			Start: rPr.Source.Start, End: rPr.Source.End, Replacement: []byte(expanded),
		}}, nil
	}
	var patches []xmlstore.SpanPatch

	// 属性类：b/i/sz。
	if st.Bold.Set {
		p, err := SetRPrAttr(doc, rPr, "b", BoolVal(st.Bold.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}
	if st.Italic.Set {
		p, err := SetRPrAttr(doc, rPr, "i", BoolVal(st.Italic.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}
	if st.Size.Set {
		p, err := SetRPrAttr(doc, rPr, "sz", SizeCentipoints(st.Size.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}

	// 子元素类：color/latin/ea/cs。
	if st.Color.Set {
		if !st.Color.Value.Valid() {
			return nil, &errs.OperationError{Message: "empty ColorSpec", Err: errs.ErrInvalidArgument}
		}
		fill := FillChildOf(doc, rPr)
		frag := SolidFillFragment(prefix, st.Color.Value)
		if fill != nil {
			// 既有填充（solidFill 或其它族）被显式颜色替换。
			patches = append(patches, xmlstore.SpanPatch{
				Start: fill.Source.Start, End: fill.Source.End, Replacement: []byte(frag),
			})
		} else {
			p, err := InsertRPrChild(doc, rPr, frag, FillRank)
			if err != nil {
				return nil, err
			}
			patches = append(patches, p)
		}
	}
	for _, fc := range []struct {
		set  model.Optional[string]
		kind string
		rank int
	}{
		{st.Latin, "latin", LatinRank},
		{st.EastAsian, "ea", EaRank},
		{st.ComplexScript, "cs", CsRank},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return nil, errs.Annotate(err, "SetFont")
		}
		frag := "<" + prefix + ":" + fc.kind + ` typeface="` + esc + `"/>`
		existing := xmlstore.ChildOfKind(doc, rPr, ooxmlns.DrawingML, fc.kind, 0)
		if existing != nil {
			patches = append(patches, xmlstore.SpanPatch{
				Start: existing.Source.Start, End: existing.Source.End, Replacement: []byte(frag),
			})
		} else {
			p, err := InsertRPrChild(doc, rPr, frag, fc.rank)
			if err != nil {
				return nil, err
			}
			patches = append(patches, p)
		}
	}
	return patches, nil
}

// SetRPrAttr 设置 rPr 属性值；属性不存在时在开标签内追加。
// 返回 nil 表示值未变化无需补丁。
func SetRPrAttr(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, name, value, prefix string) (*xmlstore.SpanPatch, error) {
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

// InsertRPrChild 把 fragment 插入 rPr 的 schema 序位置。rPr 含无法
// 判定族别的未知子元素且目标排在其后时返回 ErrUnsupportedEdit
// （不做可能破坏顺序的猜测）。
func InsertRPrChild(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, frag string, targetRank int) (xmlstore.SpanPatch, error) {
	insertPos := -1
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		rank, ok := RPrChildRank(c.Namespace, c.Local())
		if !ok {
			// 未知子元素：若其后还有已知/目标插入点将破坏顺序 → 拒绝。
			return xmlstore.SpanPatch{}, &errs.OperationError{
				Op:      "TextRun.SetFont",
				Message: "cannot safely order new run property around unknown child <" + c.Name() + ">",
				Err:     errs.ErrUnsupportedEdit,
			}
		}
		if rank >= targetRank {
			insertPos = c.Source.Start
			break
		}
	}
	if insertPos < 0 {
		// 追加到末尾（rPr 闭标签前）。
		insertPos = rPr.CloseStart
	}
	return xmlstore.SpanPatch{Start: insertPos, End: insertPos, Replacement: []byte(frag)}, nil
}

// ExpandSelfClosingRPr 把自闭合 rPr（保留全部原属性）展开为带 st
// 子元素的形态。
func ExpandSelfClosingRPr(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, prefix string, st style.FontStyle) (string, error) {
	qp := rPr.QName.Prefix
	if qp == "" {
		qp = "a"
	}
	// 原开标签内属性（去掉 '<pfx:rPr' 与 '/>'），逐条重建。
	var sb strings.Builder
	sb.WriteString("<" + qp + ":rPr")
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if IsNSDeclAttr(a.RawName) {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(a.Value, '"')
		if err != nil {
			return "", errs.Annotate(err, "SetFont")
		}
		sb.WriteString(" " + a.RawName + "=\"" + esc + "\"")
	}
	// 应用 st 属性（去重）。
	styleAttrs := map[string]string{}
	if st.Bold.Set {
		styleAttrs["b"] = BoolVal(st.Bold.Value)
	}
	if st.Italic.Set {
		styleAttrs["i"] = BoolVal(st.Italic.Value)
	}
	if st.Size.Set {
		styleAttrs["sz"] = SizeCentipoints(st.Size.Value)
	}
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		delete(styleAttrs, a.RawName)
	}
	// 剩余 st 属性按固定序输出（b/i/sz）。
	ordered := []string{"b", "i", "sz"}
	for _, k := range ordered {
		if v, ok := styleAttrs[k]; ok {
			sb.WriteString(" " + k + "=\"" + v + "\"")
		}
	}
	if !st.AnyChildSet() {
		sb.WriteString("/>")
		return sb.String(), nil
	}
	sb.WriteString(">")
	frag, err := RPrChildrenFragment(prefix, st)
	if err != nil {
		return "", err
	}
	sb.WriteString(frag)
	sb.WriteString("</" + qp + ":rPr>")
	return sb.String(), nil
}

// RPrChildrenFragment 生成 rPr 子元素（color/latin/ea/cs），供自闭合展开。
func RPrChildrenFragment(prefix string, st style.FontStyle) (string, error) {
	var children []string
	if st.Color.Set {
		if !st.Color.Value.Valid() {
			return "", &errs.OperationError{Message: "empty ColorSpec", Err: errs.ErrInvalidArgument}
		}
		children = append(children, SolidFillFragment(prefix, st.Color.Value))
	}
	for _, fc := range []struct {
		set  model.Optional[string]
		kind string
	}{
		{st.Latin, "latin"},
		{st.EastAsian, "ea"},
		{st.ComplexScript, "cs"},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return "", errs.Annotate(err, "SetFont")
		}
		children = append(children, "<"+prefix+":"+fc.kind+` typeface="`+esc+`"/>`)
	}
	return strings.Join(children, ""), nil
}
