package pptx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 TABLE-01 的表格样式子集（方案 §9.1）：
//
//   - 表格级样式开关（a:tblPr：firstRow/bandRow/firstCol/lastCol/
//     lastRow/bandCol）为三态（on/off/def），与 tableStyleId 分离；
//   - 单元格样式来源分三层：单元格显式覆盖（a:tcPr）→ 表格样式
//     （tableStyles.xml 按区域部分命中）→ 未定义（unresolved）；
//   - 区域部分按 ECMA tblStyle 的 12 个 band/first/last 标志（含
//     whole table）参与命中，重叠时按固定优先级由测试固定；
//   - 未知内置样式 ID（文件未给出定义且库无内置映射）返回
//     unresolved，不假定 tableStyles.xml 含全部可计算定义。

// relTableStyles 是 presentation → tableStyles.xml 的关系类型。
const relTableStyles = opc.RelTypePrefix + "tableStyles"

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

// parseToggle 解析 ST_OnOffStyleType 属性值。
func parseToggle(v string, ok bool) StyleToggle {
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

// tblPrNode 返回表格的 a:tblPr；缺失返回 nil。
func tblPrNode(doc *xmlstore.XMLDocument, tbl *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	return childOfKind(doc, tbl, nsDrawingML, "tblPr", 0)
}

// StyleID 返回表格样式 ID（a:tblPr@tableStyleId）；未设置返回空串。
func (t *TableShape) StyleID() (string, error) {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return "", Annotate(err, "TableShape.StyleID")
	}
	pr := tblPrNode(doc, tbl)
	if pr == nil {
		return "", nil
	}
	v, _ := pr.Attr("", "tableStyleId")
	return v, nil
}

// StyleFlags 返回表格级区域开关的三态值。
func (t *TableShape) StyleFlags() (TableStyleFlags, error) {
	var f TableStyleFlags
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return f, Annotate(err, "TableShape.StyleFlags")
	}
	pr := tblPrNode(doc, tbl)
	if pr == nil {
		return f, nil
	}
	f.FirstRow = parseToggle(pr.Attr("", "firstRow"))
	f.BandRow = parseToggle(pr.Attr("", "bandRow"))
	f.LastRow = parseToggle(pr.Attr("", "lastRow"))
	f.FirstCol = parseToggle(pr.Attr("", "firstCol"))
	f.BandCol = parseToggle(pr.Attr("", "bandCol"))
	f.LastCol = parseToggle(pr.Attr("", "lastCol"))
	return f, nil
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
// 行列 → 条带 → 整表。重叠时取优先级最高且样式库给出定义的部分
// （§9.1"由测试固定重叠优先级"）。
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

// ---------- 表格样式库（tableStyles.xml） ----------

// tblStyleLstDoc 返回 tableStyles.xml 的文档（可选 Part；缺失返回
// nil，不报错）。
func (p *Presentation) tblStyleLstDoc() (*xmlstore.XMLDocument, opc.PartName, error) {
	rels, ok, err := p.relsOf(p.main)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", nil
	}
	for _, rel := range rels {
		if rel.Mode == opc.TargetInternal && rel.Type == relTableStyles {
			doc, err := p.docOf(rel.TargetPart)
			if err != nil {
				if errIsNotFound(err) {
					return nil, "", nil
				}
				return nil, "", err
			}
			return doc, rel.TargetPart, nil
		}
	}
	return nil, "", nil
}

// tblStyleNode 返回指定 styleId 的 a:tblStyle；未定义返回 nil。
func tblStyleNode(doc *xmlstore.XMLDocument, styleID string) *xmlstore.NodeRecord {
	if doc == nil || styleID == "" {
		return nil
	}
	for _, lid := range doc.Elements(nsDrawingML, "tblStyleLst") {
		lst := doc.Node(lid)
		for _, cid := range lst.Children {
			c := doc.Node(cid)
			if c.Namespace != nsDrawingML || c.Local() != "tblStyle" {
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
	return childOfKind(doc, style, nsDrawingML, part.String(), 0)
}

// tcStyleNode 返回区域部分内的 a:tcStyle。
func tcStyleNode(doc *xmlstore.XMLDocument, partNode *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if doc == nil || partNode == nil {
		return nil
	}
	return childOfKind(doc, partNode, nsDrawingML, "tcStyle", 0)
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
	Width EMU
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
	MarginLeft   EMU
	MarginRight  EMU
	MarginTop    EMU
	MarginBottom EMU
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

// CellBorders 是单元格四边边框。
type CellBorders struct {
	Left   ResolvedValue[CellBorder]
	Right  ResolvedValue[CellBorder]
	Top    ResolvedValue[CellBorder]
	Bottom ResolvedValue[CellBorder]
}

// EffectiveCellStyle 解析单元格的有效样式（§9.1）。
//
// 来源优先级：单元格显式覆盖（a:tcPr）→ 表格样式库命中区域部分
// （tableStyles.xml）→ 未定义（Resolved=false + 诊断，不臆造内置样式
// 映射）。错误仅在文档关闭/句柄失效/结构异常时返回；样式级解析不足
// 通过字段与诊断表达。
func (c *Cell) EffectiveCellStyle() (EffectiveCellStyle, []Diagnostic, error) {
	var out EffectiveCellStyle
	doc, tc, err := c.locate()
	if err != nil {
		return out, nil, Annotate(err, "Cell.EffectiveCellStyle")
	}
	var diags []Diagnostic
	env, err := c.p.styleEnv(c.part)
	if err != nil {
		return out, nil, Annotate(err, "Cell.EffectiveCellStyle")
	}

	// 表格尺寸与开关（用于区域命中）。
	rows, cols, flags := c.p.tableGeom(doc, tc)

	// 样式库：tableStyles.xml 中该 styleId 的定义（缺失 → unresolved）。
	styleID := ""
	styleResolved := false
	var styleDoc *xmlstore.XMLDocument
	var styleNode *xmlstore.NodeRecord
	if sdoc, spart, err := c.p.tblStyleLstDoc(); err == nil && sdoc != nil {
		styleDoc = sdoc
		// 取表格 styleId：需要表格句柄（由 tc 上溯到 a:tbl）。
		if tbl := ancestorOf(doc, tc, nsDrawingML, "tbl"); tbl != nil {
			if pr := tblPrNode(doc, tbl); pr != nil {
				styleID, _ = pr.Attr("", "tableStyleId")
			}
		}
		_ = spart
		if styleID != "" {
			styleNode = tblStyleNode(styleDoc, styleID)
			styleResolved = styleNode != nil
			if !styleResolved {
				diags = append(diags, Diagnostic{
					Code: "STYLE_UNRESOLVED", Severity: SeverityWarning, Part: string(c.part),
					Message: "table style id not defined in tableStyles.xml: " + styleID,
				})
			}
		}
	}
	out.StyleID = styleID
	out.StyleResolved = styleResolved
	out.Part = PartWholeTable

	// 逐属性：填充、四边框、文本属性。
	tcPr := childOfKind(doc, tc, nsDrawingML, "tcPr", 0)
	candidates := partsForCell(c.row, c.col, rows, cols, flags)
	out.Fill = c.resolveFill(doc, env, tcPr, styleDoc, styleNode, candidates, &diags, &out.Part)
	out.Borders.Left = c.resolveBorder(doc, env, tcPr, styleDoc, styleNode, candidates, "lnL", &diags)
	out.Borders.Right = c.resolveBorder(doc, env, tcPr, styleDoc, styleNode, candidates, "lnR", &diags)
	out.Borders.Top = c.resolveBorder(doc, env, tcPr, styleDoc, styleNode, candidates, "lnT", &diags)
	out.Borders.Bottom = c.resolveBorder(doc, env, tcPr, styleDoc, styleNode, candidates, "lnB", &diags)
	out.Text = c.resolveTextStyle(doc, tcPr, styleDoc, styleNode, candidates, &diags)
	return out, diags, nil
}

// tableGeom 返回单元格所在表格的行数、列数与样式开关（读取失败时
// 返回保守值，不影响样式解析主流程）。
func (p *Presentation) tableGeom(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) (rows, cols int, flags TableStyleFlags) {
	tbl := ancestorOf(doc, tc, nsDrawingML, "tbl")
	if tbl == nil {
		return 0, 0, flags
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return 0, 0, flags
	}
	if pr := tblPrNode(doc, tbl); pr != nil {
		flags.FirstRow = parseToggle(pr.Attr("", "firstRow"))
		flags.BandRow = parseToggle(pr.Attr("", "bandRow"))
		flags.LastRow = parseToggle(pr.Attr("", "lastRow"))
		flags.FirstCol = parseToggle(pr.Attr("", "firstCol"))
		flags.BandCol = parseToggle(pr.Attr("", "bandCol"))
		flags.LastCol = parseToggle(pr.Attr("", "lastCol"))
	}
	return g.rows(), g.cols, flags
}

// ancestorOf 沿父链查找首个匹配命名空间的祖先元素。
func ancestorOf(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, ns, local string) *xmlstore.NodeRecord {
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
	if f := childOfKind(doc, container, nsDrawingML, "fill", 0); f != nil {
		return fillPropIn(doc, f)
	}
	return nil, FillUnspecified
}

// fillPropIn 在填充属性容器中查找首个填充元素。
func fillPropIn(doc *xmlstore.XMLDocument, container *xmlstore.NodeRecord) (*xmlstore.NodeRecord, FillKind) {
	for _, cid := range container.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
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
func (c *Cell) resolveFill(doc *xmlstore.XMLDocument, env *styleEnv, tcPr *xmlstore.NodeRecord,
	styleDoc *xmlstore.XMLDocument, styleNode *xmlstore.NodeRecord,
	candidates []StylePart, diags *[]Diagnostic, part *StylePart) ResolvedValue[CellFill] {

	if tcPr != nil {
		if node, kind := fillIn(doc, tcPr); node != nil {
			cf := CellFill{Kind: kind}
			if kind == FillSolid {
				cf.Color = c.p.resolveColorSpec(doc, env, node, string(c.part), diags)
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
					cf.Color = c.p.resolveColorSpec(styleDoc, env, node, string(c.part), diags)
				}
				*part = p
				return ResolvedValue[CellFill]{Value: cf, Resolved: true,
					Trace: []StyleStep{{Source: SourceTableStyle, Detail: "tblStyle " + p.String()}}}
			}
		}
	}
	*diags = append(*diags, Diagnostic{
		Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: string(c.part),
		Message: fmt.Sprintf("cell (%d,%d) fill unresolved", c.row, c.col),
	})
	return ResolvedValue[CellFill]{}
}

// resolveBorder 解析单条边框（a:tcPr 或样式库 a:tcBdr 内 a:lnX）。
func (c *Cell) resolveBorder(doc *xmlstore.XMLDocument, env *styleEnv, tcPr *xmlstore.NodeRecord,
	styleDoc *xmlstore.XMLDocument, styleNode *xmlstore.NodeRecord,
	candidates []StylePart, side string, diags *[]Diagnostic) ResolvedValue[CellBorder] {

	// 单元格显式：a:tcPr/a:lnL 等。
	if tcPr != nil {
		if ln := childOfKind(doc, tcPr, nsDrawingML, side, 0); ln != nil {
			cb := CellBorder{Specified: true}
			if v, ok := ln.Attr("", "w"); ok {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
					cb.Width = EMU(n)
				}
			}
			if fill, kind := fillIn(doc, ln); fill != nil && kind == FillSolid {
				cb.Color = c.p.resolveColorSpec(doc, env, fill, string(c.part), diags)
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
		bdr := childOfKind(styleDoc, ts, nsDrawingML, "tcBdr", 0)
		if bdr == nil {
			continue
		}
		if ln := childOfKind(styleDoc, bdr, nsDrawingML, side, 0); ln != nil {
			cb := CellBorder{Specified: true}
			if v, ok := ln.Attr("", "w"); ok {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
					cb.Width = EMU(n)
				}
			}
			if fill, kind := fillIn(styleDoc, ln); fill != nil && kind == FillSolid {
				cb.Color = c.p.resolveColorSpec(styleDoc, env, fill, string(c.part), diags)
			}
			return ResolvedValue[CellBorder]{Value: cb, Resolved: true,
				Trace: []StyleStep{{Source: SourceTableStyle, Detail: "tblStyle " + p.String() + "/tcBdr/" + side}}}
		}
	}
	return ResolvedValue[CellBorder]{}
}

// resolveTextStyle 解析单元格文本属性（对齐与内边距）。
func (c *Cell) resolveTextStyle(doc *xmlstore.XMLDocument, tcPr *xmlstore.NodeRecord,
	styleDoc *xmlstore.XMLDocument, styleNode *xmlstore.NodeRecord,
	candidates []StylePart, diags *[]Diagnostic) ResolvedValue[CellText] {

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
		for name, dst := range map[string]*EMU{"marL": &ct.MarginLeft, "marR": &ct.MarginRight,
			"marT": &ct.MarginTop, "marB": &ct.MarginBottom} {
			if v, has := pr.Attr("", name); has {
				if n, err := strconv.ParseInt(v, 10, 32); err == nil {
					*dst = EMU(n)
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
func (p *Presentation) resolveColorSpec(doc *xmlstore.XMLDocument, env *styleEnv,
	fill *xmlstore.NodeRecord, part string, diags *[]Diagnostic) ResolvedColor {

	var clr *xmlstore.NodeRecord
	kind := ""
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
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
		*diags = append(*diags, Diagnostic{
			Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: part,
			Message: "fill has no resolvable color",
		})
		return ResolvedColor{}
	}
	// 变换（lumMod 等）不解析 → 部分解析标记。
	hasTransform := false
	for _, cid := range clr.Children {
		if c := doc.Node(cid); c.Namespace == nsDrawingML {
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
			*diags = append(*diags, Diagnostic{
				Code: "STYLE_PARTIAL", Severity: SeverityWarning, Part: part,
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
		if clrMapIndirect(scheme) {
			m := masterClrMap(p.masterDoc(env))
			if mapped, ok := m[scheme]; ok {
				scheme = mapped
			}
		}
		rgb, partial := schemeRGB(p.themeDoc(env), scheme)
		rc := ResolvedColor{Spec: spec, RGB: rgb, Resolved: rgb != "" && !partial && !hasTransform,
			Trace: []StyleStep{{Source: SourceTheme, Detail: "clrScheme " + v + " → #" + rgb}}}
		if rgb == "" || partial || hasTransform {
			rc.Resolved = false
			*diags = append(*diags, Diagnostic{
				Code: "STYLE_PARTIAL", Severity: SeverityWarning, Part: part,
				Message: "scheme color not fully resolved: " + v,
			})
		}
		return rc
	}
	return ResolvedColor{}
}
