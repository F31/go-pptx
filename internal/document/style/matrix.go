package style

// 本文件实现 STYLE-02（根包 style_matrix.go + style_adv.go 下沉）：
//
//	主题样式矩阵（styleMatrixReference 引用链）+ 颜色变换全集
//
// 颜色变换全集见 color.go；本文件承载：
//   - 样式矩阵引用链解析（fillRef/lnRef/effectRef/fontRef）→ StyleMatrixRef；
//   - 主题条目（fillStyleLst/lnStyleLst/effectStyleLst 按 idx）解析；
//   - 主题字体引用（a:fontRef → majorFont/minorFont 字体名）；
//   - phClr 替换（主题条目以 schemeClr@phClr 声明时，代入引用方颜色并
//     套用主题条目自身的变换序列，ECMA §20.1.2.3.23）。
//
// 主题链经 Env + DocFunc 注入；引用方颜色解析经 ParseColorNode。

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

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

// ThemeFontSlot 是 a:fontRef 指向的主题字体槽位。
type ThemeFontSlot int

const (
	// FontSlotUnknown 表示 idx 无法映射到 major/minor（如数字越界）。
	FontSlotUnknown ThemeFontSlot = iota
	// FontSlotMajor 是 a:fontScheme/a:majorFont。
	FontSlotMajor
	// FontSlotMinor 是 a:fontScheme/a:minorFont。
	FontSlotMinor
)

func (s ThemeFontSlot) String() string {
	switch s {
	case FontSlotMajor:
		return "major"
	case FontSlotMinor:
		return "minor"
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

// ParseStyleMatrixRefs 解析形状的样式矩阵引用链（a:spPr/a:style）。
// 主题 fmtScheme 缺失或下标越界时输出诊断并保持 Resolved=false。
func ParseStyleMatrixRefs(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord,
	env *Env, docs DocFunc, part string) ([]StyleMatrixRef, []diag.Diagnostic) {
	sp := xmlstore.ChildOfKind(doc, el, ooxmlns.PresentationML, "spPr", 0)
	if sp == nil {
		return nil, nil
	}
	st := xmlstore.ChildOfKind(doc, sp, ooxmlns.DrawingML, "style", 0)
	if st == nil {
		return nil, nil
	}
	var diags []diag.Diagnostic
	var out []StyleMatrixRef
	for _, cid := range st.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
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
			ref.Index = xmlstore.IntAttr(v)
		}
		ref.Color = ParseColorNode(doc, ColorChild(doc, c), themeDocOf(docs, env), masterDocOf(docs, env), part, &diags)
		ref.ThemeEntry, ref.Resolved = themeMatrixEntry(docs, env, kind, ref.Index)
		if kind == RefFont {
			// a:fontRef：idx 为 "major"/"minor" 或 1/2，解析到主题字体槽位。
			ref.FontSlot, ref.ThemeTypeface = themeFontTypeface(docs, env, idxRaw)
			ref.Resolved = ref.FontSlot != FontSlotUnknown && ref.ThemeTypeface != ""
		} else if kind == RefFill || kind == RefLine {
			// STYLE-02：把主题条目进一步解析为可呈现颜色（phClr 代入）。
			// 仅填充/线条条目含颜色；effectRef 无颜色元素，不产生诊断。
			tdoc, entryNode, ok := themeMatrixEntryNode(docs, env, kind, ref.Index)
			if ok {
				tc, resolved := themeEntryColor(tdoc, env, docs, part, entryNode, ref.Color, &diags)
				ref.ThemeColor = tc
				if !resolved {
					diags = append(diags, diag.Diagnostic{
						Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
						Message: "theme entry color not resolved for " + kind.String() + " idx=" + idxRaw,
					})
				}
			}
		}
		if !ref.Resolved {
			diags = append(diags, diag.Diagnostic{
				Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
				Message: fmt.Sprintf("%s idx=%d not found in theme fmtScheme", kind, ref.Index),
			})
		}
		out = append(out, ref)
	}
	return out, diags
}

// themeMatrixEntry 返回主题 fmtScheme 中对应下标条目的填充元素名。
func themeMatrixEntry(docs DocFunc, env *Env, kind MatrixRefKind, idx int32) (entry string, resolved bool) {
	tdoc := themeDocOf(docs, env)
	if tdoc == nil || idx < 1 {
		return "", false
	}
	elems := xmlstore.ChildOfKind(tdoc, tdoc.Root(), ooxmlns.DrawingML, "themeElements", 0)
	if elems == nil {
		return "", false
	}
	fm := xmlstore.ChildOfKind(tdoc, elems, ooxmlns.DrawingML, "fmtScheme", 0)
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
	lst := xmlstore.ChildOfKind(tdoc, fm, ooxmlns.DrawingML, listName, 0)
	if lst == nil {
		return "", false
	}
	seen := int32(0)
	for _, cid := range lst.Children {
		c := tdoc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		seen++
		if seen != idx {
			continue
		}
		// 条目：fillStyleLst 为填充元素；lnStyleLst 为 a:ln；effectStyleLst 为 a:effectStyle。
		for _, gid := range c.Children {
			g := tdoc.Node(gid)
			if g != nil && g.Namespace == ooxmlns.DrawingML {
				return g.Local(), true
			}
		}
		return c.Local(), true
	}
	return "", false
}

// themeMatrixEntryNode 返回主题 fmtScheme 中对应条目的节点。
// 与 themeMatrixEntry 同链但保留节点引用，便于 STYLE-02 进一步解析颜色。
func themeMatrixEntryNode(docs DocFunc, env *Env, kind MatrixRefKind, idx int32) (*xmlstore.XMLDocument, *xmlstore.NodeRecord, bool) {
	tdoc := themeDocOf(docs, env)
	if tdoc == nil || idx < 1 {
		return nil, nil, false
	}
	elems := xmlstore.ChildOfKind(tdoc, tdoc.Root(), ooxmlns.DrawingML, "themeElements", 0)
	if elems == nil {
		return nil, nil, false
	}
	var listName string
	switch kind {
	case RefFill:
		listName = "fillStyleLst"
	case RefLine:
		listName = "lnStyleLst"
	case RefEffect:
		listName = "effectStyleLst"
	default:
		return nil, nil, false
	}
	scheme := xmlstore.ChildOfKind(tdoc, elems, ooxmlns.DrawingML, "fmtScheme", 0)
	if scheme == nil {
		return nil, nil, false
	}
	lst := xmlstore.ChildOfKind(tdoc, scheme, ooxmlns.DrawingML, listName, 0)
	if lst == nil {
		return nil, nil, false
	}
	seen := int32(0)
	for _, cid := range lst.Children {
		c := tdoc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		seen++
		if seen == idx {
			return tdoc, c, true
		}
	}
	return nil, nil, false
}

// themeFontTypeface 解析 a:fontRef 的 idx（"major"/"minor"/1/2）到主题
// fontScheme 对应槽位的 typeface 名。
//
// ECMA：idx="major" 或 1 → majorFont；idx="minor" 或 2 → minorFont；
// 其他值不作映射（返回 FontSlotUnknown，不臆造）。
func themeFontTypeface(docs DocFunc, env *Env, idxRaw string) (ThemeFontSlot, string) {
	tdoc := themeDocOf(docs, env)
	if tdoc == nil {
		return FontSlotUnknown, ""
	}
	elems := xmlstore.ChildOfKind(tdoc, tdoc.Root(), ooxmlns.DrawingML, "themeElements", 0)
	if elems == nil {
		return FontSlotUnknown, ""
	}
	fs := xmlstore.ChildOfKind(tdoc, elems, ooxmlns.DrawingML, "fontScheme", 0)
	if fs == nil {
		return FontSlotUnknown, ""
	}
	slot := FontSlotUnknown
	switch strings.ToLower(strings.TrimSpace(idxRaw)) {
	case "major", "1":
		slot = FontSlotMajor
	case "minor", "2":
		slot = FontSlotMinor
	}
	if slot == FontSlotUnknown {
		return slot, ""
	}
	listName := "majorFont"
	if slot == FontSlotMinor {
		listName = "minorFont"
	}
	font := xmlstore.ChildOfKind(tdoc, fs, ooxmlns.DrawingML, listName, 0)
	if font == nil {
		return slot, ""
	}
	// latin 优先；退化到 ea（东亚）/cs（复杂文种）。
	for _, want := range []string{"latin", "ea", "cs"} {
		if n := xmlstore.ChildOfKind(tdoc, font, ooxmlns.DrawingML, want, 0); n != nil {
			if tf, ok := n.Attr("", "typeface"); ok && tf != "" {
				return slot, tf
			}
		}
	}
	return slot, ""
}

// themeEntryColor 解析主题 fmtScheme 条目的颜色。
//
// phClr（占位色）语义：主题 fillStyleLst/lnStyleLst 条目通常以
// <a:schemeClr val="phClr"/> 声明"使用引用方提供的颜色"，因此必须把
// refColor（a:fillRef/a:lnRef 内的颜色，含其自身变换结果）代入为基色，
// 再套用主题条目自身的变换序列。ECMA 称此为 phClr 替换（§20.1.2.3.23）。
//
// 非 phClr 条目按常规路径解析（schemeClr/srgbClr/…）。
func themeEntryColor(doc *xmlstore.XMLDocument, env *Env, docs DocFunc, part string,
	entry *xmlstore.NodeRecord, refColor ParsedColor, diags *[]diag.Diagnostic) (ParsedColor, bool) {
	if entry == nil {
		return ParsedColor{Alpha: 1}, false
	}
	clr := styleEntryColorChild(doc, entry)
	if clr == nil {
		return ParsedColor{Alpha: 1}, false
	}
	// phClr：以引用方颜色为基色，套用主题条目变换。
	if clr.Local() == "schemeClr" {
		if v, _ := clr.Attr("", "val"); v == "phClr" {
			return applyPhClrTransforms(doc, clr, refColor, diags)
		}
	}
	out := ParseColorNode(doc, clr, themeDocOf(docs, env), masterDocOf(docs, env), part, diags)
	return out, out.RGB != ""
}

// styleEntryColorChild 在主题样式条目内定位颜色元素：优先直接子层的
// 颜色元素，其次进入 a:solidFill/a:gradFill 等填充容器（lnStyleLst 的
// a:ln 需下探一层到 a:solidFill）。
func styleEntryColorChild(doc *xmlstore.XMLDocument, entry *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if isColorElement(entry.Local()) {
		return entry
	}
	for _, cid := range entry.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		if isColorElement(c.Local()) {
			return c
		}
	}
	// 下探一层（a:solidFill / a:gradFill / a:ln）。
	for _, cid := range entry.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		for _, gid := range c.Children {
			g := doc.Node(gid)
			if g != nil && g.Namespace == ooxmlns.DrawingML && isColorElement(g.Local()) {
				return g
			}
		}
	}
	return nil
}

// isColorElement 报告是否为 DrawingML 颜色元素名。
func isColorElement(local string) bool {
	switch local {
	case "srgbClr", "scrgbClr", "hslClr", "prstClr", "schemeClr", "sysClr":
		return true
	}
	return false
}

// applyPhClrTransforms 以 refColor 为基色套用主题条目的变换序列。
func applyPhClrTransforms(doc *xmlstore.XMLDocument, clr *xmlstore.NodeRecord,
	refColor ParsedColor, diags *[]diag.Diagnostic) (ParsedColor, bool) {
	out := ParsedColor{Alpha: 1, Spec: ColorSpec{Scheme: "phClr"}}
	base := refColor.RGB
	if !IsHexRGB(base) {
		// 引用方颜色本身未解析 → 主题条目也无法给出可呈现值（不臆造）。
		return out, false
	}
	var ts []ColorTransform
	for _, cid := range clr.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		v, _ := c.Attr("", "val")
		val, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			val = 0
		}
		ts = append(ts, ColorTransform{Kind: c.Local(), Value: int32(val)})
	}
	out.Transforms = ts
	// 引用方 alpha 作为起点（phClr 继承引用方的 alpha）。
	startAlpha := refColor.Alpha
	if startAlpha <= 0 {
		startAlpha = 1
	}
	rgb, alpha, unknown := ApplyColorTransforms(base, ts)
	out.RGB = rgb
	out.Alpha = Clamp01(startAlpha * alpha)
	out.Unknown = unknown
	out.Resolved = len(unknown) == 0
	if len(unknown) > 0 {
		*diags = append(*diags, diag.Diagnostic{
			Code:     "STYLE_PARTIAL",
			Severity: diag.SeverityWarning,
			Part:     "",
			Message:  "theme entry has unknown color transforms: " + strings.Join(unknown, ","),
		})
	}
	return out, out.RGB != ""
}
