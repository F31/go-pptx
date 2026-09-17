package style

// 本文件实现 TABLE-01 的表格样式子集（根包 table_style.go 下沉）：
//
//   - 表格级样式开关（a:tblPr：firstRow/bandRow/firstCol/lastCol/
//     lastRow/bandCol）为三态（on/off/def），与 tableStyleId 分离；
//   - 单元格样式来源分三层：单元格显式覆盖（a:tcPr）→ 表格样式
//     （tableStyles.xml 按区域部分命中）→ 未定义（unresolved）；
//   - 区域部分按 ECMA tblStyle 的 12 个 band/first/last 标志（含
//     whole table）参与命中，重叠时按固定优先级；
//   - 未知内置样式 ID（文件未给出定义且库无内置映射）返回
//     unresolved，不假定 tableStyles.xml 含全部可计算定义。
//
// 宿主依赖（表格几何、tableStyles.xml 加载、主题链文档）经 CellStyleCtx
// 注入；单元格显式样式与本包解析分离。

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- 三态开关 ----------

// StyleToggle 是表格样式区域开关的三态（ST_OnOffStyleType）。
type StyleToggle int8

const (
	// ToggleDefault 表示未指定（沿用样式定义）。
	ToggleDefault StyleToggle = iota
	// ToggleOn 表示显式启用。
	ToggleOn
	// ToggleOff 表示显式关闭。
	ToggleOff
)

func (s StyleToggle) String() string {
	switch s {
	case ToggleOn:
		return "on"
	case ToggleOff:
		return "off"
	default:
		return "def"
	}
}

// TableStyleFlags 是表格级区域开关（a:tblPr 属性）。
type TableStyleFlags struct {
	FirstRow StyleToggle
	BandRow  StyleToggle
	LastRow  StyleToggle
	FirstCol StyleToggle
	BandCol  StyleToggle
	LastCol  StyleToggle
}

// ParseToggle 解析 ST_OnOffStyleType 属性值（v2.0 起在 internal 定义，
// 根包 TableShape.StyleFlags 经 alias/委托调用）。
func ParseToggle(v string, ok bool) StyleToggle {
	if !ok {
		return ToggleDefault
	}
	switch v {
	case "on", "1", "true":
		return ToggleOn
	case "off", "0", "false":
		return ToggleOff
	}
	return ToggleDefault
}

// TablePrNode 返回表格的 a:tblPr；缺失返回 nil。
func TablePrNode(doc *xmlstore.XMLDocument, tbl *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	return xmlstore.ChildOfKind(doc, tbl, ooxmlns.DrawingML, "tblPr", 0)
}

// ---------- 区域部分与优先级矩阵 ----------

// StylePart 是表格样式的区域部分（ECMA tblStyle 的部分）。
type StylePart int

const (
	// PartWholeTable 是整表兜底（优先级最低）。
	PartWholeTable StylePart = iota
	// 条带（bandRow/bandCol 开关控制）。
	PartBand1H
	PartBand2H
	PartBand1V
	PartBand2V
	// 首末行列。
	PartFirstRow
	PartLastRow
	PartFirstCol
	PartLastCol
	// 角单元格（优先级最高）。
	PartNWCell
	PartNECell
	PartSWCell
	PartSECell
)

// String 返回区域部分名（诊断与测试可读）。
func (p StylePart) String() string {
	switch p {
	case PartWholeTable:
		return "wholeTbl"
	case PartBand1H:
		return "band1H"
	case PartBand2H:
		return "band2H"
	case PartBand1V:
		return "band1V"
	case PartBand2V:
		return "band2V"
	case PartFirstRow:
		return "firstRow"
	case PartLastRow:
		return "lastRow"
	case PartFirstCol:
		return "firstCol"
	case PartLastCol:
		return "lastCol"
	case PartNWCell:
		return "nwCell"
	case PartNECell:
		return "neCell"
	case PartSWCell:
		return "swCell"
	case PartSECell:
		return "seCell"
	}
	return "StylePart(" + strconv.Itoa(int(p)) + ")"
}

// stylePartPriority 是区域命中优先级（高 → 低）：角单元格 → 首末
// 行列 → 条带 → 整表。重叠时取优先级最高且样式库给出定义的部分。
var stylePartPriority = []StylePart{
	PartNWCell, PartNECell, PartSWCell, PartSECell,
	PartFirstRow, PartLastRow, PartFirstCol, PartLastCol,
	PartBand1H, PartBand2H, PartBand1V, PartBand2V,
	PartWholeTable,
}

// partsForCell 返回单元格按优先级排列的候选区域部分（仅含开关允许
// 的部分；整表始终在末尾兜底）。bandSize 为 1（ECMA 默认条带大小）。
func partsForCell(row, col, rows, cols int, f TableStyleFlags) []StylePart {
	set := map[StylePart]bool{}
	add := func(p StylePart) { set[p] = true }
	if rows > 0 {
		if row == 0 {
			add(PartFirstRow)
		}
		if row == rows-1 && rows > 1 {
			add(PartLastRow)
		}
		if f.BandRow != ToggleOff {
			if (row % 2) == 0 {
				add(PartBand1H)
			} else {
				add(PartBand2H)
			}
		}
	}
	if cols > 0 {
		if col == 0 {
			add(PartFirstCol)
		}
		if col == cols-1 && cols > 1 {
			add(PartLastCol)
		}
		if f.BandCol != ToggleOff {
			if (col % 2) == 0 {
				add(PartBand1V)
			} else {
				add(PartBand2V)
			}
		}
	}
	// 角单元格：首/末行与首/末列交叉（需 2×2 以上）。
	if rows > 1 && cols > 1 {
		if row == 0 && col == 0 {
			add(PartNWCell)
		}
		if row == 0 && col == cols-1 {
			add(PartNECell)
		}
		if row == rows-1 && col == 0 {
			add(PartSWCell)
		}
		if row == rows-1 && col == cols-1 {
			add(PartSECell)
		}
	}
	add(PartWholeTable)
	out := make([]StylePart, 0, len(set))
	for _, p := range stylePartPriority {
		if set[p] {
			out = append(out, p)
		}
	}
	return out
}

// ---------- 表格样式库（tableStyles.xml）----------

// tblStyleNode 返回指定 styleId 的 a:tblStyle；未定义返回 nil。
func tblStyleNode(doc *xmlstore.XMLDocument, styleID string) *xmlstore.NodeRecord {
	if doc == nil || styleID == "" {
		return nil
	}
	for _, lid := range doc.Elements(ooxmlns.DrawingML, "tblStyleLst") {
		lst := doc.Node(lid)
		for _, cid := range lst.Children {
			c := doc.Node(cid)
			if c == nil || c.Namespace != ooxmlns.DrawingML || c.Local() != "tblStyle" {
				continue
			}
			if v, ok := c.Attr("", "styleId"); ok && v == styleID {
				return c
			}
		}
	}
	return nil
}

// tblStylePartNode 返回样式内指定区域部分的节点（如 a:band1H）。
func tblStylePartNode(doc *xmlstore.XMLDocument, style *xmlstore.NodeRecord, part StylePart) *xmlstore.NodeRecord {
	if doc == nil || style == nil {
		return nil
	}
	return xmlstore.ChildOfKind(doc, style, ooxmlns.DrawingML, part.String(), 0)
}

// tcStyleNode 返回区域部分内的 a:tcStyle。
func tcStyleNode(doc *xmlstore.XMLDocument, partNode *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if doc == nil || partNode == nil {
		return nil
	}
	return xmlstore.ChildOfKind(doc, partNode, ooxmlns.DrawingML, "tcStyle", 0)
}

// ---------- 单元格样式解析 ----------

// FillKind 是单元格填充类型（首版子集）。
type FillKind int

const (
	// FillUnspecified 表示未给出填充定义（未知或未解析）。
	FillUnspecified FillKind = iota
	// FillNone 表示显式无填充（a:noFill）。
	FillNone
	// FillSolid 表示纯色填充（a:solidFill）。
	FillSolid
	// FillGradient 表示渐变填充（首版不解析颜色）。
	FillGradient
	// FillPattern 表示图案填充（首版不解析）。
	FillPattern
	// FillPicture 表示图片填充（首版不解析）。
	FillPicture
	// FillGroup 表示继承组填充（首版不解析）。
	FillGroup
)

func (k FillKind) String() string {
	switch k {
	case FillNone:
		return "none"
	case FillSolid:
		return "solid"
	case FillGradient:
		return "gradient"
	case FillPattern:
		return "pattern"
	case FillPicture:
		return "picture"
	case FillGroup:
		return "group"
	}
	return "unspecified"
}

// CellFill 是单元格填充的解析结果。
type CellFill struct {
	// Kind 是填充类型；FillUnspecified 表示未解析。
	Kind FillKind
	// Color 是纯色填充的颜色（仅 FillSolid 且有定义时解析）。
	Color ResolvedColor
}

// CellBorder 是单元格单条边框的解析结果。
type CellBorder struct {
	// Specified 表示该边框有显式定义（a:lnL/lnR/lnT/lnB）。
	Specified bool
	// Width 是线宽（a:ln@w，EMU）；未设置返回 0。
	Width model.EMU
	// Color 是线条颜色的解析结果。
	Color ResolvedColor
}

// CellText 是单元格文本相关属性（首版：对齐与内边距）。
type CellText struct {
	// Anchor 是垂直对齐（a:tcPr@anchor：t/ctr/b/just/dist）。
	Anchor string
	// AnchorCenter 表示水平垂直均居中（a:tcPr@anchorCtr）。
	AnchorCenter bool
	// MarginLeft/Right/Top/Bottom 是内边距（EMU）。
	MarginLeft   model.EMU
	MarginRight  model.EMU
	MarginTop    model.EMU
	MarginBottom model.EMU
}

// CellBorders 是单元格四边边框。
type CellBorders struct {
	Left   ResolvedValue[CellBorder]
	Right  ResolvedValue[CellBorder]
	Top    ResolvedValue[CellBorder]
	Bottom ResolvedValue[CellBorder]
}

// EffectiveCellStyle 是单元格的逐属性样式解析结果（§9.1：填充、边框、
// 文字各自独立状态，而非只返回一个 Color）。
type EffectiveCellStyle struct {
	// Fill 是填充解析结果（含来源链）。
	Fill ResolvedValue[CellFill]
	// Borders 是四条边框的解析结果。
	Borders CellBorders
	// Text 是文本属性解析结果（对齐/边距）。
	Text ResolvedValue[CellText]
	// Part 是样式的生效区域部分（未命中样式库时为 PartWholeTable）。
	Part StylePart
	// StyleID 是表格样式 ID（未设置或未知时为空/已知但未定义仍返回原值）。
	StyleID string
	// StyleResolved 表示样式库给出了该样式 ID 的定义（tableStyles.xml
	// 缺失或未定义该 ID 时为 false → 视为 unresolved，不臆测内置映射）。
	StyleResolved bool
}

// CellStyleCtx 是单元格样式解析的注入面（门面以 root 状态构造）。
type CellStyleCtx struct {
	// Env 是单元格所属 Part 的样式环境。
	Env *Env
	// Docs 提供主题链文档（schemeClr 解析需要）。
	Docs DocFunc
	// StyleDoc 是 tableStyles.xml 文档；nil 表示不可用（unresolved）。
	StyleDoc *xmlstore.XMLDocument
	// Part 是单元格所属 Part（诊断归属；可空）。
	Part string
	// Geom 返回单元格所在表格的行列数与区域开关（由表格宿主注入；
	// 读取失败返回保守值）。
	Geom func(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) (rows, cols int, flags TableStyleFlags)
}

// ResolveEffectiveCellStyle 解析单元格的有效样式（§9.1）。
//
// 来源优先级：单元格显式覆盖（a:tcPr）→ 表格样式库命中区域部分
// （tableStyles.xml）→ 未定义（Resolved=false + 诊断，不臆造内置样式
// 映射）。错误仅由门面在定位/环境层面返回；样式级解析不足通过字段与
// 诊断表达。
func ResolveEffectiveCellStyle(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord,
	row, col int, ctx CellStyleCtx) (EffectiveCellStyle, []diag.Diagnostic) {
	var out EffectiveCellStyle
	var diags []diag.Diagnostic
	env := ctx.Env
	part := ctx.Part

	// 表格尺寸与开关（用于区域命中）。
	rows, cols, flags := cellGeom(doc, tc, ctx.Geom)

	// 样式库：tableStyles.xml 中该 styleId 的定义（缺失 → unresolved）。
	styleID := ""
	styleResolved := false
	var styleNode *xmlstore.NodeRecord
	if ctx.StyleDoc != nil {
		styleDoc := ctx.StyleDoc
		// 取表格 styleId：需要表格句柄（由 tc 上溯到 a:tbl）。
		if tbl := AncestorOf(doc, tc, ooxmlns.DrawingML, "tbl"); tbl != nil {
			if pr := TablePrNode(doc, tbl); pr != nil {
				styleID, _ = pr.Attr("", "tableStyleId")
			}
		}
		if styleID != "" {
			styleNode = tblStyleNode(styleDoc, styleID)
			styleResolved = styleNode != nil
			if !styleResolved {
				diags = append(diags, diag.Diagnostic{
					Code: "STYLE_UNRESOLVED", Severity: diag.SeverityWarning, Part: part,
					Message: "table style id not defined in tableStyles.xml: " + styleID,
				})
			}
		}
	}
	out.StyleID = styleID
	out.StyleResolved = styleResolved
	out.Part = PartWholeTable

	// 逐属性：填充、四边框、文本属性。
	tcPr := xmlstore.ChildOfKind(doc, tc, ooxmlns.DrawingML, "tcPr", 0)
	candidates := partsForCell(row, col, rows, cols, flags)
	out.Fill = resolveFill(doc, env, ctx.Docs, tcPr, ctx.StyleDoc, styleNode, candidates, row, col, part, &diags, &out.Part)
	out.Borders.Left = resolveBorder(doc, env, ctx.Docs, tcPr, ctx.StyleDoc, styleNode, candidates, "lnL", part, &diags)
	out.Borders.Right = resolveBorder(doc, env, ctx.Docs, tcPr, ctx.StyleDoc, styleNode, candidates, "lnR", part, &diags)
	out.Borders.Top = resolveBorder(doc, env, ctx.Docs, tcPr, ctx.StyleDoc, styleNode, candidates, "lnT", part, &diags)
	out.Borders.Bottom = resolveBorder(doc, env, ctx.Docs, tcPr, ctx.StyleDoc, styleNode, candidates, "lnB", part, &diags)
	out.Text = resolveTextStyle(doc, tcPr, ctx.StyleDoc, styleNode, candidates, &diags)
	return out, diags
}

// AncestorOf 沿父链查找首个匹配命名空间的祖先元素（缺失返回 nil）。
func AncestorOf(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, ns, local string) *xmlstore.NodeRecord {
	cur := n
	for cur != nil && cur.Parent != xmlstore.NoNode {
		par := doc.Node(cur.Parent)
		if par == nil {
			return nil
		}
		if par.Namespace == ns && par.Local() == local {
			return par
		}
		cur = par
	}
	return nil
}

// cellGeom 调用注入的表格几何；注入面缺失时返回保守值。
func cellGeom(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord, geom func(*xmlstore.XMLDocument, *xmlstore.NodeRecord) (int, int, TableStyleFlags)) (rows, cols int, flags TableStyleFlags) {
	if geom == nil {
		return 0, 0, flags
	}
	return geom(doc, tc)
}

// fillIn 返回容器中首个填充属性元素（noFill/solidFill/...）。
// 单元格 a:tcPr 直接携带填充属性；样式库 a:tcStyle 以 a:fill 包裹
// （CT_TableStyleCellStyle），此处先直查再进入 a:fill。
func fillIn(doc *xmlstore.XMLDocument, container *xmlstore.NodeRecord) (*xmlstore.NodeRecord, FillKind) {
	if container == nil {
		return nil, FillUnspecified
	}
	if node, kind := fillPropIn(doc, container); node != nil {
		return node, kind
	}
	if f := xmlstore.ChildOfKind(doc, container, ooxmlns.DrawingML, "fill", 0); f != nil {
		return fillPropIn(doc, f)
	}
	return nil, FillUnspecified
}

// fillPropIn 在填充属性容器中查找首个填充元素。
func fillPropIn(doc *xmlstore.XMLDocument, container *xmlstore.NodeRecord) (*xmlstore.NodeRecord, FillKind) {
	for _, cid := range container.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "noFill":
			return c, FillNone
		case "solidFill":
			return c, FillSolid
		case "gradFill":
			return c, FillGradient
		case "pattFill":
			return c, FillPattern
		case "blipFill":
			return c, FillPicture
		case "grpFill":
			return c, FillGroup
		}
	}
	return nil, FillUnspecified
}

// resolveFill 解析填充：单元格显式 a:tcPr 优先，其次按区域优先级查
// 样式库的 a:tcStyle。
func resolveFill(doc *xmlstore.XMLDocument, env *Env, docs DocFunc, tcPr *xmlstore.NodeRecord,
	styleDoc *xmlstore.XMLDocument, styleNode *xmlstore.NodeRecord,
	candidates []StylePart, row, col int, part string, diags *[]diag.Diagnostic, effPart *StylePart) ResolvedValue[CellFill] {

	if tcPr != nil {
		if node, kind := fillIn(doc, tcPr); node != nil {
			cf := CellFill{Kind: kind}
			if kind == FillSolid {
				cf.Color = resolveColorSpec(doc, node, env, docs, part, diags)
			}
			return ResolvedValue[CellFill]{Value: cf, Resolved: true,
				Trace: []StyleStep{{Source: SourceCellExplicit, Detail: "cell a:tcPr fill"}}}
		}
	}
	if styleDoc != nil && styleNode != nil {
		for _, p := range candidates {
			ts := tcStyleNode(styleDoc, tblStylePartNode(styleDoc, styleNode, p))
			if ts == nil {
				continue
			}
			if node, kind := fillIn(styleDoc, ts); node != nil {
				cf := CellFill{Kind: kind}
				if kind == FillSolid {
					cf.Color = resolveColorSpec(styleDoc, node, env, docs, part, diags)
				}
				*effPart = p
				return ResolvedValue[CellFill]{Value: cf, Resolved: true,
					Trace: []StyleStep{{Source: SourceTableStyle, Detail: "tblStyle " + p.String()}}}
			}
		}
	}
	*diags = append(*diags, diag.Diagnostic{
		Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
		Message: fmt.Sprintf("cell (%d,%d) fill unresolved", row, col),
	})
	return ResolvedValue[CellFill]{}
}

// resolveBorder 解析单条边框（a:tcPr 或样式库 a:tcBdr 内 a:lnX）。
func resolveBorder(doc *xmlstore.XMLDocument, env *Env, docs DocFunc, tcPr *xmlstore.NodeRecord,
	styleDoc *xmlstore.XMLDocument, styleNode *xmlstore.NodeRecord,
	candidates []StylePart, side, part string, diags *[]diag.Diagnostic) ResolvedValue[CellBorder] {

	// 单元格显式：a:tcPr/a:lnL 等。
	if tcPr != nil {
		if ln := xmlstore.ChildOfKind(doc, tcPr, ooxmlns.DrawingML, side, 0); ln != nil {
			cb := CellBorder{Specified: true}
			if v, ok := ln.Attr("", "w"); ok {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
					cb.Width = model.EMU(n)
				}
			}
			if fill, kind := fillIn(doc, ln); fill != nil && kind == FillSolid {
				cb.Color = resolveColorSpec(doc, fill, env, docs, part, diags)
			}
			return ResolvedValue[CellBorder]{Value: cb, Resolved: true,
				Trace: []StyleStep{{Source: SourceCellExplicit, Detail: "cell a:tcPr/" + side}}}
		}
	}
	// 样式库：a:tcStyle/a:tcBdr/a:lnX。
	if styleDoc == nil || styleNode == nil {
		return ResolvedValue[CellBorder]{}
	}
	for _, p := range candidates {
		ts := tcStyleNode(styleDoc, tblStylePartNode(styleDoc, styleNode, p))
		if ts == nil {
			continue
		}
		bdr := xmlstore.ChildOfKind(styleDoc, ts, ooxmlns.DrawingML, "tcBdr", 0)
		if bdr == nil {
			continue
		}
		if ln := xmlstore.ChildOfKind(styleDoc, bdr, ooxmlns.DrawingML, side, 0); ln != nil {
			cb := CellBorder{Specified: true}
			if v, ok := ln.Attr("", "w"); ok {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
					cb.Width = model.EMU(n)
				}
			}
			if fill, kind := fillIn(styleDoc, ln); fill != nil && kind == FillSolid {
				cb.Color = resolveColorSpec(styleDoc, fill, env, docs, part, diags)
			}
			return ResolvedValue[CellBorder]{Value: cb, Resolved: true,
				Trace: []StyleStep{{Source: SourceTableStyle, Detail: "tblStyle " + p.String() + "/tcBdr/" + side}}}
		}
	}
	return ResolvedValue[CellBorder]{}
}

// resolveTextStyle 解析单元格文本属性（对齐与内边距）。
func resolveTextStyle(doc *xmlstore.XMLDocument, tcPr *xmlstore.NodeRecord,
	styleDoc *xmlstore.XMLDocument, styleNode *xmlstore.NodeRecord,
	candidates []StylePart, diags *[]diag.Diagnostic) ResolvedValue[CellText] {

	read := func(pr *xmlstore.NodeRecord) (CellText, bool) {
		var ct CellText
		if pr == nil {
			return ct, false
		}
		ok := false
		if v, has := pr.Attr("", "anchor"); has {
			ct.Anchor = v
			ok = true
		}
		if v, has := pr.Attr("", "anchorCtr"); has && (v == "1" || v == "true") {
			ct.AnchorCenter = true
			ok = true
		}
		for name, dst := range map[string]*model.EMU{"marL": &ct.MarginLeft, "marR": &ct.MarginRight,
			"marT": &ct.MarginTop, "marB": &ct.MarginBottom} {
			if v, has := pr.Attr("", name); has {
				if n, err := strconv.ParseInt(v, 10, 32); err == nil {
					*dst = model.EMU(n)
					ok = true
				}
			}
		}
		return ct, ok
	}
	if ct, ok := read(tcPr); ok {
		return ResolvedValue[CellText]{Value: ct, Resolved: true,
			Trace: []StyleStep{{Source: SourceCellExplicit, Detail: "cell a:tcPr text"}}}
	}
	if styleDoc == nil || styleNode == nil {
		return ResolvedValue[CellText]{}
	}
	for _, p := range candidates {
		ts := tcStyleNode(styleDoc, tblStylePartNode(styleDoc, styleNode, p))
		if ts == nil {
			continue
		}
		if ct, ok := read(ts); ok {
			return ResolvedValue[CellText]{Value: ct, Resolved: true,
				Trace: []StyleStep{{Source: SourceTableStyle, Detail: "tblStyle " + p.String() + " text"}}}
		}
	}
	return ResolvedValue[CellText]{}
}

// resolveColorSpec 解析填充/线条容器内的颜色（a:srgbClr 直出、
// a:schemeClr 经 clrMap 与主题）。未知形态返回 unresolved 诊断。
func resolveColorSpec(doc *xmlstore.XMLDocument, fill *xmlstore.NodeRecord,
	env *Env, docs DocFunc, part string, diags *[]diag.Diagnostic) ResolvedColor {

	var clr *xmlstore.NodeRecord
	kind := ""
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "srgbClr":
			clr, kind = c, "srgbClr"
		case "schemeClr":
			clr, kind = c, "schemeClr"
		case "sysClr":
			if v, ok := c.Attr("", "lastClr"); ok {
				return ResolvedColor{Spec: ColorSpec{RGB: strings.ToUpper(v)},
					RGB: strings.ToUpper(v), Resolved: true,
					Trace: []StyleStep{{Source: SourceCellExplicit, Detail: "sysClr lastClr"}}}
			}
		}
		if clr != nil {
			break
		}
	}
	if clr == nil {
		*diags = append(*diags, diag.Diagnostic{
			Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
			Message: "fill has no resolvable color",
		})
		return ResolvedColor{}
	}
	// 变换（lumMod 等）不解析 → 部分解析标记。
	hasTransform := false
	for _, cid := range clr.Children {
		if c := doc.Node(cid); c != nil && c.Namespace == ooxmlns.DrawingML {
			hasTransform = true
			break
		}
	}
	switch kind {
	case "srgbClr":
		v, ok := clr.Attr("", "val")
		if !ok {
			return ResolvedColor{}
		}
		rc := ResolvedColor{Spec: ColorSpec{RGB: strings.ToUpper(v)}, RGB: strings.ToUpper(v),
			Resolved: true, Trace: []StyleStep{{Source: SourceCellExplicit, Detail: "srgbClr"}}}
		if hasTransform {
			rc.Resolved = false
			*diags = append(*diags, diag.Diagnostic{
				Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning, Part: part,
				Message: "color transformations are not applied by TABLE-01",
			})
		}
		return rc
	case "schemeClr":
		v, ok := clr.Attr("", "val")
		if !ok {
			return ResolvedColor{}
		}
		spec := ColorSpec{Scheme: v}
		scheme := v
		if ClrMapIndirect(scheme) {
			// MasterClrMap 恒非 nil：直接查表。
			if mapped, ok := MasterClrMap(masterDocOf(docs, env))[scheme]; ok {
				scheme = mapped
			}
		}
		rgb, partial := SchemeRGB(themeDocOf(docs, env), scheme)
		rc := ResolvedColor{Spec: spec, RGB: rgb, Resolved: rgb != "" && !partial && !hasTransform,
			Trace: []StyleStep{{Source: SourceTheme, Detail: "clrScheme " + v + " → #" + rgb}}}
		if rgb == "" || partial || hasTransform {
			rc.Resolved = false
			*diags = append(*diags, diag.Diagnostic{
				Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning, Part: part,
				Message: "scheme color not fully resolved: " + v,
			})
		}
		return rc
	}
	return ResolvedColor{}
}
