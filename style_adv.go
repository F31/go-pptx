package pptx

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 STYLE-02（方案 §2.3 + V2.6 §940 工作包收口）：
//
//	主题样式矩阵（styleMatrixReference 引用链）+ 颜色变换全集
//
// 与 M3"起步解析"的关系：M3 在 format.go 提供了 20 种变换与 fillRef/
// lnRef/effectRef 的元素名解析；STYLE-02 收口到
//   - 颜色变换 ECMA EG_ColorTransform 全集 28 种（transform 数学在
//     format.go 的 applyColorTransforms，白名单在 knownTransformKinds）；
//   - 样式矩阵引用链不仅返回主题条目元素名，还把条目解析为可呈现颜色
//     （ThemeColor），并新增 a:fontRef（RefFont）解析到主题
//     a:fontScheme 的 majorFont/minorFont 字体名。
//
// 档位：R 档——解析并输出诊断，不提供写入 API（V2.6 §940 验收口径为
// "引用链与约 20 种颜色变换的解析结果通过语料回归"）。

// ---------- 主题条目 → 可呈现颜色 ----------

// themeEntryColor 解析主题 fmtScheme 条目的颜色。
//
// phClr（占位色）语义：主题 fillStyleLst/lnStyleLst 条目通常以
// <a:schemeClr val="phClr"/> 声明"使用引用方提供的颜色"，因此必须把
// refColor（a:fillRef/a:lnRef 内的颜色，含其自身变换结果）代入为基色，
// 再套用主题条目自身的变换序列。ECMA 称此为 phClr 替换（§20.1.2.3.23）。
//
// 非 phClr 条目按常规路径解析（schemeClr/srgbClr/…）。
func (p *Presentation) themeEntryColor(doc *xmlstore.XMLDocument, env *styleEnv, part string,
	entry *xmlstore.NodeRecord, refColor ParsedColor, diags *[]Diagnostic) (ParsedColor, bool) {
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
			return p.applyPhClrTransforms(doc, clr, refColor, diags)
		}
	}
	out := p.parseColorNode(doc, env, part, clr, diags)
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
		if c.Namespace != nsDrawingML {
			continue
		}
		if isColorElement(c.Local()) {
			return c
		}
	}
	// 下探一层（a:solidFill / a:gradFill / a:ln）。
	for _, cid := range entry.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		for _, gid := range c.Children {
			g := doc.Node(gid)
			if g.Namespace == nsDrawingML && isColorElement(g.Local()) {
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
func (p *Presentation) applyPhClrTransforms(doc *xmlstore.XMLDocument, clr *xmlstore.NodeRecord,
	refColor ParsedColor, diags *[]Diagnostic) (ParsedColor, bool) {
	out := ParsedColor{Alpha: 1, Spec: ColorSpec{Scheme: "phClr"}}
	base := refColor.RGB
	if !isHexRGB(base) {
		// 引用方颜色本身未解析 → 主题条目也无法给出可呈现值（不臆造）。
		return out, false
	}
	var ts []ColorTransform
	for _, cid := range clr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
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
	rgb, alpha, unknown := applyColorTransforms(base, ts)
	// applyColorTransforms 内部 alpha 从 1 起算；此处把引用方 alpha 作为
	// 前置乘子补回（相对量 alphaMod/alphaOff 已含在 ts 内）。
	out.RGB = rgb
	out.Alpha = clamp01(startAlpha * alpha)
	out.Unknown = unknown
	out.Resolved = len(unknown) == 0
	if len(unknown) > 0 {
		*diags = append(*diags, Diagnostic{
			Code:     "STYLE_PARTIAL",
			Severity: SeverityWarning,
			Part:     "",
			Message:  "theme entry has unknown color transforms: " + strings.Join(unknown, ","),
		})
	}
	return out, out.RGB != ""
}

// ---------- 主题字体引用（a:fontRef）----------

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

// themeFontTypeface 解析 a:fontRef 的 idx（"major"/"minor"/1/2）到主题
// fontScheme 对应槽位的 latin 字体名。
//
// ECMA：idx="major" 或 1 → majorFont；idx="minor" 或 2 → minorFont；
// 其他值不作映射（返回 FontSlotUnknown，不臆造）。
func (p *Presentation) themeFontTypeface(env *styleEnv, idxRaw string) (ThemeFontSlot, string) {
	tdoc := p.themeDoc(env)
	if tdoc == nil {
		return FontSlotUnknown, ""
	}
	elems := childOfKind(tdoc, tdoc.Root(), nsDrawingML, "themeElements", 0)
	if elems == nil {
		return FontSlotUnknown, ""
	}
	fs := childOfKind(tdoc, elems, nsDrawingML, "fontScheme", 0)
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
	font := childOfKind(tdoc, fs, nsDrawingML, listName, 0)
	if font == nil {
		return slot, ""
	}
	// latin 优先；退化到 ea（东亚）/cs（复杂文种）。
	for _, want := range []string{"latin", "ea", "cs"} {
		if n := childOfKind(tdoc, font, nsDrawingML, want, 0); n != nil {
			if tf, ok := n.Attr("", "typeface"); ok && tf != "" {
				return slot, tf
			}
		}
	}
	return slot, ""
}

// ---------- 主题条目节点定位（供 StyleMatrixRefs 复用）----------

// themeMatrixEntryNode 返回主题 fmtScheme/fontScheme 中对应条目的节点。
// 与 format.go 的 themeMatrixEntry 同链但保留节点引用，便于 STYLE-02
// 进一步解析颜色/字体。
func (p *Presentation) themeMatrixEntryNode(env *styleEnv, kind MatrixRefKind, idx int32) (*xmlstore.XMLDocument, *xmlstore.NodeRecord, bool) {
	tdoc := p.themeDoc(env)
	if tdoc == nil || idx < 1 {
		return nil, nil, false
	}
	elems := childOfKind(tdoc, tdoc.Root(), nsDrawingML, "themeElements", 0)
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
	scheme := childOfKind(tdoc, elems, nsDrawingML, "fmtScheme", 0)
	if scheme == nil {
		return nil, nil, false
	}
	lst := childOfKind(tdoc, scheme, nsDrawingML, listName, 0)
	if lst == nil {
		return nil, nil, false
	}
	seen := int32(0)
	for _, cid := range lst.Children {
		c := tdoc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		seen++
		if seen == idx {
			return tdoc, c, true
		}
	}
	return nil, nil, false
}
