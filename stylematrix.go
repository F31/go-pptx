package pptx

import (
	"fmt"
)

// 本文件是 M3 的**主题样式矩阵**：a:fmtScheme 与 styleMatrixReference
// 引用链解析 → StyleMatrixRef。

// ---------- 主题样式矩阵引用链 ----------

// MatrixRefKind 是样式矩阵引用的目标类型。
type MatrixRefKind int

const (
	// RefFill 是填充样式引用（a:fillRef）。
	RefFill MatrixRefKind = iota
	// RefLine 是线条样式引用（a:lnRef）。
	RefLine
	// RefEffect 是效果样式引用（a:effectRef）。
	RefEffect
	// RefFont 是字体样式引用（a:fontRef）；STYLE-02 新增，解析到主题
	// a:fontScheme 的 majorFont/minorFont。
	RefFont
)

func (k MatrixRefKind) String() string {
	switch k {
	case RefFill:
		return "fillRef"
	case RefLine:
		return "lnRef"
	case RefEffect:
		return "effectRef"
	case RefFont:
		return "fontRef"
	}
	return "unknown"
}

// StyleMatrixRef 是形状样式矩阵引用（a:spPr/a:style 内的 fillRef/
// lnRef/effectRef/fontRef）的解析结果（R 档：解析并输出诊断，不提供写入）。
type StyleMatrixRef struct {
	// Kind 是引用类型；STYLE-02 起含 RefFont。
	Kind MatrixRefKind
	// Index 是引用下标（1 基；ECMA idx 从 1 起）。
	Index int32
	// Color 是引用内颜色（含变换）。
	Color ParsedColor
	// ThemeEntry 是主题 fmtScheme 中对应条目的元素名（如 solidFill）。
	ThemeEntry string
	// ThemeColor 是主题条目解析出的可呈现颜色（STYLE-02）。
	// 主题条目以 phClr 声明时，基色取 Color 并套用主题条目自身的变换；
	// 无法解析时 RGB 为空、Resolved=false。
	ThemeColor ParsedColor
	// ThemeTypeface 仅 RefFont 有效：主题 fontScheme majorFont/minorFont
	// 的字体名（latin 优先，退化 ea/cs）；未解析时为空（STYLE-02）。
	ThemeTypeface string
	// FontSlot 仅 RefFont 有效：idx 映射到的主题字体槽位。
	FontSlot ThemeFontSlot
	// Resolved 表示引用与主题条目均已解析。
	Resolved bool
}

// StyleMatrixRefs 返回形状的样式矩阵引用链（a:spPr/a:style）。
// 主题 fmtScheme 缺失或下标越界时输出诊断并保持 Resolved=false。
func (s *shapeNode) StyleMatrixRefs() ([]StyleMatrixRef, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return nil, nil, Annotate(err, "shape.StyleMatrixRefs")
	}
	sp := childOfKind(doc, el, nsPresentationML, "spPr", 0)
	if sp == nil {
		return nil, nil, nil
	}
	st := childOfKind(doc, sp, nsDrawingML, "style", 0)
	if st == nil {
		return nil, nil, nil
	}
	var diags []Diagnostic
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return nil, nil, Annotate(err, "shape.StyleMatrixRefs")
	}
	var out []StyleMatrixRef
	for _, cid := range st.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		var kind MatrixRefKind
		switch c.Local() {
		case "fillRef":
			kind = RefFill
		case "lnRef":
			kind = RefLine
		case "effectRef":
			kind = RefEffect
		case "fontRef":
			kind = RefFont // STYLE-02
		default:
			continue
		}
		ref := StyleMatrixRef{Kind: kind}
		idxRaw := ""
		if v, ok := c.Attr("", "idx"); ok {
			idxRaw = v
			ref.Index = intAttr(v)
		}
		ref.Color = s.p.parseColorNode(doc, env, string(s.part), colorChildOf(doc, c), &diags)
		ref.ThemeEntry, ref.Resolved = s.p.themeMatrixEntry(env, kind, ref.Index)
		if kind == RefFont {
			// a:fontRef：idx 为 "major"/"minor" 或 1/2，解析到主题字体槽位。
			ref.FontSlot, ref.ThemeTypeface = s.p.themeFontTypeface(env, idxRaw)
			ref.Resolved = ref.FontSlot != FontSlotUnknown && ref.ThemeTypeface != ""
		} else if kind == RefFill || kind == RefLine {
			// STYLE-02：把主题条目进一步解析为可呈现颜色（phClr 代入）。
			// 仅填充/线条条目含颜色；effectRef 无颜色元素，不产生诊断。
			tdoc, entryNode, ok := s.p.themeMatrixEntryNode(env, kind, ref.Index)
			if ok {
				tc, resolved := s.p.themeEntryColor(tdoc, env, string(s.part), entryNode, ref.Color, &diags)
				ref.ThemeColor = tc
				if !resolved {
					diags = append(diags, Diagnostic{
						Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: string(s.part),
						Message: "theme entry color not resolved for " + kind.String() + " idx=" + idxRaw,
					})
				}
			}
		}
		if !ref.Resolved {
			diags = append(diags, Diagnostic{
				Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: string(s.part),
				Message: fmt.Sprintf("%s idx=%d not found in theme fmtScheme", kind, ref.Index),
			})
		}
		out = append(out, ref)
	}
	return out, diags, nil
}

// themeMatrixEntry 返回主题 fmtScheme 中对应下标条目的填充元素名。
func (p *Presentation) themeMatrixEntry(env *styleEnv, kind MatrixRefKind, idx int32) (entry string, resolved bool) {
	tdoc := p.themeDoc(env)
	if tdoc == nil || idx < 1 {
		return "", false
	}
	elems := childOfKind(tdoc, tdoc.Root(), nsDrawingML, "themeElements", 0)
	if elems == nil {
		return "", false
	}
	fm := childOfKind(tdoc, elems, nsDrawingML, "fmtScheme", 0)
	if fm == nil {
		return "", false
	}
	listName := ""
	switch kind {
	case RefFill:
		listName = "fillStyleLst"
	case RefLine:
		listName = "lnStyleLst"
	case RefEffect:
		listName = "effectStyleLst"
	}
	lst := childOfKind(tdoc, fm, nsDrawingML, listName, 0)
	if lst == nil {
		return "", false
	}
	seen := int32(0)
	for _, cid := range lst.Children {
		c := tdoc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		seen++
		if seen != idx {
			continue
		}
		// 条目：fillStyleLst 为填充元素；lnStyleLst 为 a:ln；effectStyleLst 为 a:effectStyle。
		for _, gid := range c.Children {
			g := tdoc.Node(gid)
			if g.Namespace == nsDrawingML {
				return g.Local(), true
			}
		}
		return c.Local(), true
	}
	return "", false
}
