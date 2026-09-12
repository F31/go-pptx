package pptx

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/chart"
	"github.com/F31/go-pptx/internal/editplan"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 CHART-01（方案 §9.2）：受限三类（柱/折/饼）图表的创建、
// 读取与受限数据更新。
//
// 图表是复合对象：slide 的 p:graphicFrame（引用）+ 独立 chart Part
// （c:chartSpace，含数据缓存）+ 嵌入工作簿 Part（可编辑数据源）。
// 数据更新必须同时维护缓存与工作簿（AT-10），只改显示文本是禁止的。
//
// 受限范围（超出即显式 ErrUnsupportedEdit/ErrUnsupportedFormat，不静默
// 降级）：
//   - 图类型：柱状（clustered col）/折线/饼图，不支持组合、双轴、
//     面积/散点等（E 逐类立项，方案 2.3 矩阵）；
//   - 数据更新 SetData 仅支持本库生成的规范布局图表：结构校验失败
//     （外部图表、客户端往返重写、附加了数据标签/趋势线等）即整体
//     拒绝，不做部分合并；
//   - SetData 重建 chart Part 与嵌入工作簿（保持 Part 名与关系不变），
//     图表类型不可变更（类型变更 = 删除重建，不在受限编辑范围）。
//
// 嵌入工作簿经 ChartWorkbookBuilder 可替换适配接口生成（默认纯 Go
// 最小 xlsx，见 chartbook.go）。

// ---------- 常量 ----------

const (
	// nsChartML 是 DrawingML 图表命名空间（c: 前缀）。
	nsChartML = "http://schemas.openxmlformats.org/drawingml/2006/chart"

	relChart    = opc.RelTypePrefix + "chart"
	relPackage  = opc.RelTypePrefix + "package"
	ctChartPart = "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"
	ctWorkbook  = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// chartGraphicURI 是 a:graphicData@uri 的图表标识（与 nsChartML 同值）。
//
// ADR-017 第一批：实现搬到 internal/chart.GraphicURI；调用方直接用 chart.GraphicURI。
// 保留此文件级常量仅供本地文档/历史对照使用。
const chartGraphicURI = nsChartML

// chartSheetName 是工作簿数据表名（图表引用固定指向它）。
//
// ADR-017 第一批：实现搬到 internal/chart.SheetName；调用方直接用 chart.SheetName。
const chartSheetName = "Sheet1"

// 规范布局的轴 ID（chart Part 内部作用域，两值固定）。
//
// ADR-017 第一批：实现搬到 internal/chart.CatAxID / chart.ValAxID；调用方直接用之。
const (
	chartCatAxID = 100000001
	chartValAxID = 100000002
)

// ---------- 公共类型 ----------

// ChartType 是受限图表类型（CHART-01 范围：柱/折/饼）。
type ChartType int

const (
	// ChartBar 是簇状柱状图（barDir=col，单值轴）。
	ChartBar ChartType = iota
	// ChartLine 是折线图（标准分组，无平滑）。
	ChartLine
	// ChartPie 是饼图。
	ChartPie
)

func (t ChartType) String() string {
	switch t {
	case ChartBar:
		return "bar"
	case ChartLine:
		return "line"
	case ChartPie:
		return "pie"
	}
	return fmt.Sprintf("ChartType(%d)", int(t))
}

// plotElement 返回类型对应的 c: 图表组元素名。
func (t ChartType) plotElement() string {
	switch t {
	case ChartBar:
		return "barChart"
	case ChartLine:
		return "lineChart"
	case ChartPie:
		return "pieChart"
	}
	return ""
}

// chartTypeFromPlot 由图表组元素名解析类型。
func chartTypeFromPlot(local string) (ChartType, bool) {
	switch local {
	case "barChart":
		return ChartBar, true
	case "lineChart":
		return ChartLine, true
	case "pieChart":
		return ChartPie, true
	}
	return 0, false
}

// ChartSpec 描述新增图表（单位：EMU）。Width/Height 必需（>0）。
//
// 扩展字段（CHART-02，R 档白名单）：
//   - DataLabel：nil 或 Show=false 即不写图表级 <c:dLbls>（PowerPoint
//     默认隐藏）；Show=true 时按 Position 白名单生成。
//   - Axes：nil = 默认（categorical catAx + 线性 valAx）；非空可启用对
//     数轴 / 日期类别轴 / 值轴 Min/Max。
//
// Series 内 ErrorBars/Trendline 由各 ChartSeries 自身携带，按 OOXML 序
// 排在 c:tx 与 c:cat 之间。
type ChartSpec struct {
	Type       ChartType
	Title      string
	Categories []string
	Series     []ChartSeries
	X, Y       int64
	Width      int64
	Height     int64
	DataLabel  *ChartDataLabel   // CHART-02：可选图表级数据标签
	Axes       *ChartAxisOptions // CHART-02：可选轴扩展
}

// ChartSeries 是一个数据系列（名称 + 与类别等长的数值）。
type ChartSeries struct {
	// Name 是系列名（显示于图例与工作簿行 1）。
	Name string
	// Values 是数值（长度必须等于类别数）。
	Values []float64
	// ErrorBars：可选系列级误差线（CHART-02，CHART-01 不暴露）。
	ErrorBars *ChartErrorBars
	// Trendline：可选系列级趋势线（CHART-02，CHART-01 不暴露）。
	Trendline *ChartTrendline
}

// ChartData 是图表数据的读写快照：Data() 的返回值与 SetData 的入参。
//
// 扩展字段（CHART-02）随读回的 Data()/SetData 往返：每条 Series 含指针
// ErrorBars/Trendline，未设时为 nil；顶层 DataLabel/Axes 同理。
type ChartData struct {
	Type       ChartType
	Title      string
	Categories []string
	Series     []ChartSeries
	DataLabel  *ChartDataLabel
	Axes       *ChartAxisOptions
}

// Stable: ChartShape 是页面图表（p:graphicFrame 引用 chart Part）的读侧
// 句柄，类比 TextFrame/Paragraph/TextRun 同级别核心入口型。0 公开字段
// （shapeNode 嵌入）；句柄失效语义由 shapeNode + STALE-GUARD 锁定。
// v1.x 内承诺：
//
//   - 类型签名不变；不新增/重命名/移除公开方法
//   - 现有方法签名与返回类型不变（Kind / SetAltText / SetDecorative /
//     ChartData 等）
//   - 仅允许追加新方法
//   - 句柄身份语义不变
//   - 实现 Shape 接口
type ChartShape struct {
	shapeNode
}

// Kind 返回形状类别（恒为 ShapeChart）。
func (c *ChartShape) Kind() ShapeKind { return ShapeChart }

// SetAltText 设置替代文本；同时清除装饰性标记（二者语义互斥，§8.1）。
func (c *ChartShape) SetAltText(text string) error {
	return c.setAltText("ChartShape.SetAltText", text)
}

// SetDecorative 设置装饰性标记；为 true 时清除替代文本（§8.1）。
func (c *ChartShape) SetDecorative(decorative bool) error {
	return c.setDecorative("ChartShape.SetDecorative", decorative)
}

// ---------- 图表定位 ----------

// chartOfGraphic 返回图形框内 a:graphicData（图表 URI）下的 c:chart
// 引用元素；非图表图形框返回 nil。
func chartOfGraphic(doc *xmlstore.XMLDocument, frame *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	g := childOfKind(doc, frame, nsDrawingML, "graphic", 0)
	if g == nil {
		return nil
	}
	gd := childOfKind(doc, g, nsDrawingML, "graphicData", 0)
	if gd == nil {
		return nil
	}
	if uri, ok := gd.Attr("", "uri"); !ok || uri != chartGraphicURI {
		return nil
	}
	return childOfKind(doc, gd, nsChartML, "chart", 0)
}

// chartPartOf 解析句柄当前指向的 chart Part 名（slide 关系 → 目标）。
func (c *ChartShape) chartPartOf() (opc.PartName, error) {
	doc, frame, err := c.locate()
	if err != nil {
		return "", Annotate(err, "ChartShape")
	}
	ch := chartOfGraphic(doc, frame)
	if ch == nil {
		return "", &OperationError{
			Op: "ChartShape", Part: string(c.part),
			Message: "graphicFrame is not a chart (no chart graphicData)", Err: ErrUnsupportedFormat,
		}
	}
	rid, ok := ch.Attr(nsOfficeDocument, "id")
	if !ok || rid == "" {
		return "", &OperationError{
			Op: "ChartShape", Part: string(c.part),
			Message: "c:chart has no r:id", Err: ErrMalformedPackage,
		}
	}
	target, ok := c.p.mediaTargetOf(c.part, rid)
	if !ok {
		return "", &OperationError{
			Op: "ChartShape", Part: string(c.part),
			Message: "r:id " + rid + " has no internal chart relationship",
			Err:     ErrNotFound,
		}
	}
	return target, nil
}

// chartWorkbookPartOf 返回 chart Part 关系流中 package 关系指向的嵌入
// 工作簿 Part 名（不存在返回 false）。
func (p *Presentation) chartWorkbookPartOf(chartPart opc.PartName) (opc.PartName, bool) {
	rels, ok, err := p.relsOf(chartPart)
	if err != nil || !ok {
		return "", false
	}
	for _, rel := range rels {
		if rel.Mode == opc.TargetInternal && rel.Type == relPackage {
			return rel.TargetPart, true
		}
	}
	return "", false
}

// ---------- AddChart ----------

// AddChart 在页面上创建受限图表（柱/折/饼）：新建 chart Part（含数据
// 缓存）、嵌入工作簿 Part 与两侧关系，并在 spTree 末尾插入引用图表的
// p:graphicFrame（最上层 z-order）。
//
// 数据约束：至少 1 个系列与 1 个类别；每个系列数值长度等于类别数；
// 数值必须有限（NaN/Inf 拒绝）。Width/Height 必需（EMU）。
// 工作簿字节经当前 ChartWorkbookBuilder 生成（默认最小 xlsx）。
func (s *Slide) AddChart(ctx context.Context, spec ChartSpec) (*ChartShape, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.AddChart")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, Annotate(err, "Slide.AddChart")
	}
	cd := ChartData{
		Type:       spec.Type,
		Title:      spec.Title,
		Categories: spec.Categories,
		Series:     spec.Series,
		DataLabel:  spec.DataLabel,
		Axes:       spec.Axes,
	}
	if err := validateChartData(cd, "Slide.AddChart"); err != nil {
		return nil, err
	}
	if spec.Width <= 0 || spec.Height <= 0 {
		return nil, &OperationError{
			Op: "Slide.AddChart", Message: "ChartSpec.Width/Height must be > 0 (EMU)", Err: ErrInvalidArgument,
		}
	}
	p := s.p

	// 1) Part 名（序号避开包内既有与会话内新增）。
	chartPart := opc.PartName(fmt.Sprintf("/ppt/charts/chart%d.xml",
		nextPartSeq(p, "/ppt/charts/chart", ".xml")))
	wbPart := opc.PartName(fmt.Sprintf("/ppt/embeddings/Microsoft_Excel_Worksheet%d.xlsx",
		nextPartSeq(p, "/ppt/embeddings/Microsoft_Excel_Worksheet", ".xlsx")))

	// 2) 工作簿与 chart XML。
	wb, err := p.chartWorkbookBytes(ChartDataBook{
		SheetName:  chartSheetName,
		Categories: spec.Categories,
		Series:     spec.Series,
	})
	if err != nil {
		return nil, Annotate(err, "Slide.AddChart")
	}
	chartXML, err := buildChartSpaceXML(cd)
	if err != nil {
		return nil, Annotate(err, "Slide.AddChart")
	}

	// 3) 规划 chart Part、工作簿与 chart 关系流（package → 工作簿）。
	chartRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + relPackage + `" Target="../embeddings/` + slideName(wbPart) + `"/>` +
		`</Relationships>`
	ops := []editplan.Operation{
		editplan.Add(chartPart, []byte(xmlDecl+chartXML), ctChartPart),
		editplan.Add(wbPart, wb, ctWorkbook),
		relsPlanOp(p, chartPart, []byte(chartRels)),
	}

	// 4) slide 关系 → chart Part。
	rels, err := relsXML(p, s.part)
	if err != nil {
		return nil, Annotate(err, "Slide.AddChart")
	}
	rid := nextRID(rels)
	rels = insertRel(rels, `<Relationship Id="`+rid+`" Type="`+relChart+
		`" Target="../charts/`+slideName(chartPart)+`"/>`)
	ops = append(ops, relsPlanOp(p, s.part, rels))

	// 5) spTree 末尾追加 p:graphicFrame。
	doc, tree, err := s.slideTree()
	if err != nil {
		return nil, Annotate(err, "Slide.AddChart")
	}
	id := nextShapeID(doc, tree)
	frag := buildChartFrameFragment(id, spec.X, spec.Y, spec.Width, spec.Height, rid)
	ap, err := xmlstore.AppendChild(tree, []byte(frag))
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddChart")
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{ap})
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Slide.AddChart")
	}
	ops = append(ops, editplan.Patch(s.part, out))
	if err := applyMultiPartPlan(p, editplan.NewMultiPartPlan(ops...)); err != nil {
		return nil, Annotate(err, "Slide.AddChart")
	}
	return s.lastChartHandle(), nil
}

// buildChartFrameFragment 构造引用 chart Part 的 p:graphicFrame 片段
// （c: 前缀内联声明；a:/r: 由页面根元素作用域解析）。
func buildChartFrameFragment(id, x, y, cx, cy int64, rid string) string {
	return chart.BuildChartFrameFragment(id, x, y, cx, cy, rid)
}

// lastChartHandle 在提交后定位 spTree 末尾引用图表的 p:graphicFrame。
func (s *Slide) lastChartHandle() *ChartShape {
	p := s.p
	doc, err := p.docOf(s.part)
	if err != nil {
		return &ChartShape{shapeNode: shapeNode{p: p, part: s.part}}
	}
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for i := len(tree.Children) - 1; i >= 0; i-- {
			c := doc.Node(tree.Children[i])
			if c.Namespace == nsPresentationML && c.Local() == "graphicFrame" &&
				chartOfGraphic(doc, c) != nil {
				return &ChartShape{shapeNode: shapeNode{p: p, part: s.part, path: recordPath(doc, c.ID), idHint: shapeNodeIDFromRecord(doc, c.ID)}}
			}
		}
	}
	return &ChartShape{shapeNode: shapeNode{p: p, part: s.part}}
}

// ---------- 数据校验 ----------

// validateChartData 校验受限图表数据（AddChart 与 SetData 共用）。
func validateChartData(cd ChartData, op string) error {
	if cd.Type != ChartBar && cd.Type != ChartLine && cd.Type != ChartPie {
		return &OperationError{Op: op, Message: "unknown chart type", Err: ErrInvalidArgument}
	}
	if len(cd.Categories) == 0 {
		return &OperationError{Op: op, Message: "at least one category is required", Err: ErrInvalidArgument}
	}
	if len(cd.Series) == 0 {
		return &OperationError{Op: op, Message: "at least one series is required", Err: ErrInvalidArgument}
	}
	for i, ser := range cd.Series {
		if len(ser.Values) != len(cd.Categories) {
			return &OperationError{
				Op: op,
				Message: fmt.Sprintf("series %d has %d values, want %d (one per category)",
					i, len(ser.Values), len(cd.Categories)),
				Err: ErrInvalidArgument,
			}
		}
		for j, v := range ser.Values {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return &OperationError{
					Op:      op,
					Message: fmt.Sprintf("series %d value %d is not finite", i, j),
					Err:     ErrInvalidArgument,
				}
			}
		}
		if cd.Type == ChartPie {
			if ser.ErrorBars != nil {
				return &OperationError{
					Op: op, Message: "series " + strconv.Itoa(i) + ": pie chart does not support error bars",
					Err: ErrInvalidArgument,
				}
			}
			if ser.Trendline != nil {
				return &OperationError{
					Op: op, Message: "series " + strconv.Itoa(i) + ": pie chart does not support trendlines",
					Err: ErrInvalidArgument,
				}
			}
		}
		if ser.ErrorBars != nil {
			if err := validateChartErrorBars(*ser.ErrorBars, op); err != nil {
				return Annotate(err, "series "+strconv.Itoa(i)+".ErrorBars")
			}
		}
		if ser.Trendline != nil {
			if err := validateChartTrendline(*ser.Trendline, op); err != nil {
				return Annotate(err, "series "+strconv.Itoa(i)+".Trendline")
			}
		}
	}
	if cd.DataLabel != nil {
		if err := validateChartDataLabel(*cd.DataLabel, op); err != nil {
			return Annotate(err, "ChartDataLabel")
		}
	}
	if cd.Axes != nil {
		if err := validateChartAxisOptions(*cd.Axes, op); err != nil {
			return Annotate(err, "ChartAxisOptions")
		}
	}
	return nil
}

// ---------- chart XML 生成（规范布局） ----------

// buildChartSpaceXML 生成规范布局的 c:chartSpace（受限三类）。
// 引用约定与工作簿布局配套（chartbook.go 顶部注释）。
func buildChartSpaceXML(cd ChartData) (string, error) {
	esc := func(s string) (string, error) { return xmlstore.EscapeText(s) }
	n := len(cd.Categories)
	lastRow := n + 1
	catRef := chartSheetName + `!$A$2:$A$` + strconv.Itoa(lastRow)

	var sb strings.Builder
	sb.WriteString(`<c:chartSpace xmlns:c="` + nsChartML + `" xmlns:a="` + nsDrawingML +
		`" xmlns:r="` + nsOfficeDocument + `">`)
	sb.WriteString(`<c:chart>`)
	if cd.Title != "" {
		t, err := esc(cd.Title)
		if err != nil {
			return "", &OperationError{Op: "chart", Message: "title: " + err.Error(), Err: ErrInvalidArgument}
		}
		sb.WriteString(`<c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:r>` +
			`<a:rPr lang="en-US"/><a:t>` + t + `</a:t></a:r></a:p></c:rich></c:tx>` +
			`<c:overlay val="0"/></c:title>`)
	}
	sb.WriteString(`<c:autoTitleDeleted val="0"/>`)
	sb.WriteString(`<c:plotArea><c:layout/>`)

	writeSer := func() error {
		for i, ser := range cd.Series {
			col := chartWorkbookColumn(i + 2)
			nameRef := chartSheetName + `!$` + col + `$1`
			valRef := chartSheetName + `!$` + col + `$2:$` + col + `$` + strconv.Itoa(lastRow)
			sb.WriteString(`<c:ser><c:idx val="` + strconv.Itoa(i) + `"/><c:order val="` + strconv.Itoa(i) + `"/>`)
			name, err := esc(ser.Name)
			if err != nil {
				return &OperationError{Op: "chart", Message: "series name: " + err.Error(), Err: ErrInvalidArgument}
			}
			sb.WriteString(`<c:tx><c:strRef><c:f>` + nameRef + `</c:f>` +
				`<c:strCache><c:ptCount val="1"/><c:pt idx="0"><c:v>` + name + `</c:v></c:pt></c:strCache>` +
				`</c:strRef></c:tx>`)
			// CHART-02 系列级扩展：trendline + errBars，写在 c:tx 与 c:cat 之间。
			if ser.Trendline != nil {
				frag, err := buildTrendlineFragment(*ser.Trendline)
				if err != nil {
					return Annotate(err, "series "+strconv.Itoa(i)+".Trendline")
				}
				sb.WriteString(frag)
			}
			if ser.ErrorBars != nil {
				frag, err := buildErrBarsFragment(*ser.ErrorBars)
				if err != nil {
					return Annotate(err, "series "+strconv.Itoa(i)+".ErrorBars")
				}
				sb.WriteString(frag)
			}
			sb.WriteString(`<c:cat><c:strRef><c:f>` + catRef + `</c:f>` +
				`<c:strCache><c:ptCount val="` + strconv.Itoa(n) + `"/>`)
			for j, cat := range cd.Categories {
				cv, err := esc(cat)
				if err != nil {
					return &OperationError{Op: "chart", Message: "category: " + err.Error(), Err: ErrInvalidArgument}
				}
				sb.WriteString(`<c:pt idx="` + strconv.Itoa(j) + `"><c:v>` + cv + `</c:v></c:pt>`)
			}
			sb.WriteString(`</c:strCache></c:strRef></c:cat>`)
			sb.WriteString(`<c:val><c:numRef><c:f>` + valRef + `</c:f>` +
				`<c:numCache><c:formatCode>General</c:formatCode>` +
				`<c:ptCount val="` + strconv.Itoa(n) + `"/>`)
			for j, v := range ser.Values {
				sb.WriteString(`<c:pt idx="` + strconv.Itoa(j) + `"><c:v>` + chartNumber(v) + `</c:v></c:pt>`)
			}
			sb.WriteString(`</c:numCache></c:numRef></c:val>`)
			if cd.Type == ChartLine {
				sb.WriteString(`<c:smooth val="0"/>`)
			}
			sb.WriteString(`</c:ser>`)
		}
		return nil
	}

	// dLblsFragment 是 chart-level c:dLbls（按 OOXML 序插在图表组的
	// 适当位置：barChart 在 ser* 与 gapWidth 之间；lineChart 在 ser* 与
	// marker 之间；pieChart 在 ser* 与 firstSliceAng 之间）。
	dLblsFragment := ""
	if cd.DataLabel != nil && cd.DataLabel.Show {
		frag, err := buildChartDataLabelFragment(*cd.DataLabel)
		if err != nil {
			return "", Annotate(err, "ChartDataLabel")
		}
		dLblsFragment = frag
	}

	switch cd.Type {
	case ChartBar:
		sb.WriteString(`<c:barChart><c:barDir val="col"/><c:grouping val="clustered"/><c:varyColors val="0"/>`)
		if err := writeSer(); err != nil {
			return "", err
		}
		if dLblsFragment != "" {
			sb.WriteString(dLblsFragment)
		}
		sb.WriteString(`<c:gapWidth val="150"/>` +
			`<c:axId val="` + strconv.Itoa(chartCatAxID) + `"/>` +
			`<c:axId val="` + strconv.Itoa(chartValAxID) + `"/></c:barChart>`)
	case ChartLine:
		sb.WriteString(`<c:lineChart><c:grouping val="standard"/><c:varyColors val="0"/>`)
		if err := writeSer(); err != nil {
			return "", err
		}
		if dLblsFragment != "" {
			sb.WriteString(dLblsFragment)
		}
		sb.WriteString(`<c:marker val="1"/>` +
			`<c:axId val="` + strconv.Itoa(chartCatAxID) + `"/>` +
			`<c:axId val="` + strconv.Itoa(chartValAxID) + `"/></c:lineChart>`)
	case ChartPie:
		sb.WriteString(`<c:pieChart><c:varyColors val="1"/>`)
		if err := writeSer(); err != nil {
			return "", err
		}
		if dLblsFragment != "" {
			sb.WriteString(dLblsFragment)
		}
		sb.WriteString(`<c:firstSliceAng val="0"/></c:pieChart>`)
	}
	if cd.Type == ChartBar || cd.Type == ChartLine {
		// cat/date 类别轴 + val 值轴；按 ChartAxisOptions 切换。
		axOpts := ChartAxisOptions{}
		if cd.Axes != nil {
			axOpts = *cd.Axes
		}
		sb.WriteString(buildCatOrDateAxFragment(axOpts))
		sb.WriteString(buildValAxFragment(axOpts, chartValAxID, chartCatAxID))
	}
	sb.WriteString(`</c:plotArea>`)
	sb.WriteString(`<c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/>`)
	sb.WriteString(`</c:chart></c:chartSpace>`)
	return sb.String(), nil
}

// ---------- 图表数据读取 ----------

// Data 返回图表当前数据快照（类型/标题/类别/系列，读自 chart Part 的
// 数据缓存）。多图表组（组合图等超出受限范围的布局）返回首个受限
// 图表组的数据；无受限图表组返回 ErrUnsupportedFormat。
func (c *ChartShape) Data() (ChartData, error) {
	if err := c.alive(); err != nil {
		return ChartData{}, Annotate(err, "ChartShape.Data")
	}
	part, err := c.chartPartOf()
	if err != nil {
		return ChartData{}, Annotate(err, "ChartShape.Data")
	}
	doc, err := c.p.docOf(part)
	if err != nil {
		return ChartData{}, Annotate(err, "ChartShape.Data")
	}
	root := doc.Root()
	if root == nil || root.Namespace != nsChartML || root.Local() != "chartSpace" {
		return ChartData{}, &OperationError{
			Op: "ChartShape.Data", Part: string(part),
			Message: "chart part root is not c:chartSpace", Err: ErrMalformedPackage,
		}
	}
	return parseChartSpace(doc, root)
}

// alive 复用 shapeNode 的存活检查（独立方法名避免与 Data 内多次定位混淆）。
func (c *ChartShape) alive() error {
	if c.p == nil || c.p.closed {
		return Annotate(ErrClosed, "ChartShape")
	}
	if _, _, err := c.locate(); err != nil {
		return err
	}
	return nil
}

// nodeText 读取元素文本内容（实体解码；自闭合元素返回空串）。
func nodeText(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) string {
	if n == nil || n.SelfClosing() {
		return ""
	}
	return xmlUnescape(string(doc.Original()[n.OpenEnd:n.CloseStart]))
}

// parseChartSpace 从 chartSpace 节点解析受限图表数据。
func parseChartSpace(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord) (ChartData, error) {
	chart := childOfKind(doc, root, nsChartML, "chart", 0)
	if chart == nil {
		return ChartData{}, &OperationError{Message: "c:chartSpace has no c:chart", Err: ErrMalformedPackage}
	}
	cd := ChartData{}
	// 标题：c:title/c:tx/c:rich 内全部 a:t 拼接；无 title 留空。
	if title := childOfKind(doc, chart, nsChartML, "title", 0); title != nil {
		var sb strings.Builder
		var walk func(n *xmlstore.NodeRecord)
		walk = func(n *xmlstore.NodeRecord) {
			if n.Namespace == nsDrawingML && n.Local() == "t" {
				sb.WriteString(nodeText(doc, n))
			}
			for _, cid := range n.Children {
				walk(doc.Node(cid))
			}
		}
		walk(title)
		cd.Title = sb.String()
	}
	plotArea := childOfKind(doc, chart, nsChartML, "plotArea", 0)
	if plotArea == nil {
		return ChartData{}, &OperationError{Message: "c:chart has no c:plotArea", Err: ErrMalformedPackage}
	}
	var plot *xmlstore.NodeRecord
	for _, cid := range plotArea.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			continue
		}
		if _, ok := chartTypeFromPlot(cn.Local()); ok {
			if plot == nil {
				plot = cn
			}
			// 组合图（多个图表组）：读取首个受限组，其余忽略（GoDoc 已声明）。
		}
	}
	if plot == nil {
		return ChartData{}, &OperationError{
			Message: "chart has no supported plot (bar/line/pie)", Err: ErrUnsupportedFormat,
		}
	}
	cd.Type, _ = chartTypeFromPlot(plot.Local())
	// 图表级数据标签（CHART-02）。
	if dl := childOfKind(doc, plot, nsChartML, "dLbls", 0); dl != nil {
		cd.DataLabel = parseChartDLbls(doc, dl)
	}
	for _, cid := range plot.Children {
		ser := doc.Node(cid)
		if ser.Namespace != nsChartML || ser.Local() != "ser" {
			continue
		}
		cs := ChartSeries{}
		cs.Name = chartSerName(doc, ser)
		if cat, ok := chartSerCategories(doc, ser); ok && len(cd.Categories) == 0 {
			cd.Categories = cat
		}
		cs.Values = chartSerValues(doc, ser)
		// CHART-02：系列级 trendline + errBars。
		if tr := childOfKind(doc, ser, nsChartML, "trendline", 0); tr != nil {
			cs.Trendline = parseChartTrendline(doc, tr)
		}
		if eb := childOfKind(doc, ser, nsChartML, "errBars", 0); eb != nil {
			cs.ErrorBars = parseChartErrBars(doc, eb)
		}
		cd.Series = append(cd.Series, cs)
	}
	// 类别/日期轴/值轴扩展（CHART-02）。
	cd.Axes = parseChartAxes(doc, plotArea)
	return cd, nil
}

// parseChartDLbls 读回图表级 c:dLbls。
// 简化 R 档：只读 showVal 与 dLblPos（其余 show* 视为显示标志位，未启
// 用时保持默认）。
func parseChartDLbls(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *ChartDataLabel {
	out := &ChartDataLabel{Show: true}
	for _, cid := range n.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			continue
		}
		switch cn.Local() {
		case "dLblPos":
			out.Position, _ = cn.Attr("", "val")
		case "showVal":
			v, _ := cn.Attr("", "val")
			out.Show = v == "1" || v == "true"
		}
	}
	return out
}

// parseChartTrendline 读回 c:trendline。
func parseChartTrendline(doc *xmlstore.XMLDocument, tr *xmlstore.NodeRecord) *ChartTrendline {
	out := &ChartTrendline{}
	if n := childOfKind(doc, tr, nsChartML, "name", 0); n != nil {
		out.Name, _ = n.Attr("", "val")
	}
	if n := childOfKind(doc, tr, nsChartML, "trendlineType", 0); n != nil {
		v, _ := n.Attr("", "val")
		out.Type = chartTrendTypeFromName(v)
	}
	if n := childOfKind(doc, tr, nsChartML, "dispEq", 0); n != nil {
		v, _ := n.Attr("", "val")
		out.DisplayEq = v == "1" || v == "true"
	}
	if n := childOfKind(doc, tr, nsChartML, "dispRSqr", 0); n != nil {
		v, _ := n.Attr("", "val")
		out.DisplayRSq = v == "1" || v == "true"
	}
	if n := childOfKind(doc, tr, nsChartML, "intercept", 0); n != nil {
		v, _ := n.Attr("", "val")
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			out.SetIntercept = true
			out.Intercept = f
		}
	}
	return out
}

// parseChartErrBars 读回 c:errBars。
func parseChartErrBars(doc *xmlstore.XMLDocument, eb *xmlstore.NodeRecord) *ChartErrorBars {
	out := &ChartErrorBars{Direction: "both"}
	if n := childOfKind(doc, eb, nsChartML, "errBarType", 0); n != nil {
		v, _ := n.Attr("", "val")
		out.Type = chartErrorTypeFromName(v)
	}
	if n := childOfKind(doc, eb, nsChartML, "errDir", 0); n != nil {
		if v, _ := n.Attr("", "val"); v != "" {
			out.Direction = v
		}
	}
	if n := childOfKind(doc, eb, nsChartML, "val", 0); n != nil {
		if v, _ := n.Attr("", "val"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				out.Value = f
			}
		}
	}
	if n := childOfKind(doc, eb, nsChartML, "noEndCap", 0); n != nil {
		if v, _ := n.Attr("", "val"); v == "1" || v == "true" {
			out.NoEndCap = true
		}
	}
	return out
}

// parseChartAxes 读回 catAx/dateAx/valAx 扩展配置。
func parseChartAxes(doc *xmlstore.XMLDocument, plotArea *xmlstore.NodeRecord) *ChartAxisOptions {
	out := &ChartAxisOptions{}
	// cat/date/val 检测
	for _, lk := range []string{"catAx", "dateAx", "valAx"} {
		ax := childOfKind(doc, plotArea, nsChartML, lk, 0)
		if ax == nil {
			continue
		}
		switch lk {
		case "dateAx":
			out.CategoryAsDate = true
		case "valAx":
			if n := childOfKind(doc, ax, nsChartML, "scaling", 0); n != nil {
				if lb := childOfKind(doc, n, nsChartML, "logBase", 0); lb != nil {
					v, _ := lb.Attr("", "val")
					if p, err := strconv.Atoi(v); err == nil {
						out.ValueLogBase = p
					}
				}
			}
			if n := childOfKind(doc, ax, nsChartML, "min", 0); n != nil {
				if v, _ := n.Attr("", "val"); v != "" {
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						out.Min = NewOptional(f)
					}
				}
			}
			if n := childOfKind(doc, ax, nsChartML, "max", 0); n != nil {
				if v, _ := n.Attr("", "val"); v != "" {
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						out.Max = NewOptional(f)
					}
				}
			}
		}
		// axPos
		if p, ok := ax.Attr("", "axPos"); ok {
			// axPos 是命名属性，无命名空间。
			out.Position = p
		}
	}
	return out
}

// chartTrendTypeFromName 由 trendlineType val 反查枚举。
func chartTrendTypeFromName(v string) ChartTrendType {
	switch v {
	case "linear":
		return ChartTrendLinear
	case "log":
		return ChartTrendLogarithmic
	case "exp":
		return ChartTrendExponential
	case "poly":
		return ChartTrendPolynomial
	case "power":
		return ChartTrendPower
	case "movingAvg":
		return ChartTrendMovingAverage
	}
	return ChartTrendLinear
}

// chartErrorTypeFromName 由 errBarType val 反查枚举。
func chartErrorTypeFromName(v string) ChartErrorType {
	switch v {
	case "stdDev":
		return ChartErrStandardDeviation
	case "stdErr":
		return ChartErrStandardError
	case "fixed":
		return ChartErrFixed
	case "percentage":
		return ChartErrPercentage
	}
	return ChartErrStandardError
}

// chartSerName 解析系列名：c:tx 下 strRef/strCache 或直接 c:v。
func chartSerName(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord) string {
	tx := childOfKind(doc, ser, nsChartML, "tx", 0)
	if tx == nil {
		return ""
	}
	if v := childOfKind(doc, tx, nsChartML, "v", 0); v != nil {
		return nodeText(doc, v)
	}
	if ref := childOfKind(doc, tx, nsChartML, "strRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "strCache", 0); cache != nil {
			if pts := chartCachePoints(doc, cache); len(pts) > 0 {
				return pts[0]
			}
		}
	}
	return ""
}

// chartSerCategories 解析类别（首个系列的 cat 决定全局类别）。
// 支持 strRef/strCache、numRef/numCache 与字面量 strLit/numLit。
func chartSerCategories(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord) ([]string, bool) {
	cat := childOfKind(doc, ser, nsChartML, "cat", 0)
	if cat == nil {
		return nil, false
	}
	if ref := childOfKind(doc, cat, nsChartML, "strRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "strCache", 0); cache != nil {
			return chartCachePoints(doc, cache), true
		}
	}
	if ref := childOfKind(doc, cat, nsChartML, "numRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "numCache", 0); cache != nil {
			return chartCachePoints(doc, cache), true
		}
	}
	if lit := childOfKind(doc, cat, nsChartML, "strLit", 0); lit != nil {
		return chartCachePoints(doc, lit), true
	}
	if lit := childOfKind(doc, cat, nsChartML, "numLit", 0); lit != nil {
		return chartCachePoints(doc, lit), true
	}
	return nil, false
}

// chartSerValues 解析系列数值（numRef/numCache 或 numLit）。
func chartSerValues(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord) []float64 {
	val := childOfKind(doc, ser, nsChartML, "val", 0)
	if val == nil {
		return nil
	}
	var pts []string
	if ref := childOfKind(doc, val, nsChartML, "numRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "numCache", 0); cache != nil {
			pts = chartCachePoints(doc, cache)
		}
	} else if lit := childOfKind(doc, val, nsChartML, "numLit", 0); lit != nil {
		pts = chartCachePoints(doc, lit)
	}
	out := make([]float64, 0, len(pts))
	for _, p := range pts {
		if v, err := strconv.ParseFloat(p, 64); err == nil {
			out = append(out, v)
		} else {
			out = append(out, 0)
		}
	}
	return out
}

// chartCachePoints 按 idx 顺序读取缓存（strCache/numCache/strLit/numLit
// 的 c:pt/c:v）。
func chartCachePoints(doc *xmlstore.XMLDocument, cache *xmlstore.NodeRecord) []string {
	type pt struct {
		idx int
		v   string
	}
	var pts []pt
	for _, cid := range cache.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML || cn.Local() != "pt" {
			continue
		}
		idx := 0
		if s, ok := cn.Attr("", "idx"); ok {
			if v, err := strconv.Atoi(s); err == nil {
				idx = v
			}
		}
		v := childOfKind(doc, cn, nsChartML, "v", 0)
		if v == nil {
			continue
		}
		pts = append(pts, pt{idx: idx, v: nodeText(doc, v)})
	}
	if len(pts) == 0 {
		return nil
	}
	max := 0
	for _, p := range pts {
		if p.idx > max {
			max = p.idx
		}
	}
	out := make([]string, max+1)
	for i := range out {
		out[i] = ""
	}
	for _, p := range pts {
		if p.idx >= 0 && p.idx <= max {
			out[p.idx] = p.v
		}
	}
	return out
}

// ---------- 受限数据更新（SetData） ----------

// SetData 更新图表数据：同时重建 chart Part 的数据缓存与嵌入工作簿
// （AT-10 一致性要求），Part 名与关系保持不变。
//
// 受限范围（超出返回 ErrUnsupportedEdit，不产生部分写入）：
//   - 仅支持本库规范布局（外部生成、被客户端往返重写或附加了数据
//     标签/趋势线/自定义样式等的图表整体拒绝）；
//   - 图表类型不可变更（与当前不一致即拒绝）；
//   - 必须存在嵌入工作簿（规范布局创建时总是携带）。
//
// 本方法目前为实验能力（方案 §9.2：工作簿一致性实现落地前的过渡
// 语义——缓存与工作簿由同一数据快照重建，保证两侧一致）。
func (c *ChartShape) SetData(cd ChartData) error {
	const op = "ChartShape.SetData"
	if err := c.alive(); err != nil {
		return Annotate(err, op)
	}
	if err := validateChartData(cd, op); err != nil {
		return err
	}
	p := c.p
	part, err := c.chartPartOf()
	if err != nil {
		return Annotate(err, op)
	}
	doc, err := p.docOf(part)
	if err != nil {
		return Annotate(err, op)
	}
	root := doc.Root()
	if root == nil || root.Namespace != nsChartML || root.Local() != "chartSpace" {
		return &OperationError{Op: op, Part: string(part), Message: "chart part root is not c:chartSpace", Err: ErrMalformedPackage}
	}
	if !chartIsCanonical(doc, root) {
		return &OperationError{
			Op: op, Part: string(part),
			Message: "chart layout is not the library-canonical form (foreign or modified chart); refused to avoid partial merge",
			Err:     ErrUnsupportedEdit,
		}
	}
	cur, err := parseChartSpace(doc, root)
	if err != nil {
		return Annotate(err, op)
	}
	if cur.Type != cd.Type {
		return &OperationError{
			Op: op, Part: string(part),
			Message: fmt.Sprintf("chart type change (%s → %s) is not a supported edit; remove and re-add the chart", cur.Type, cd.Type),
			Err:     ErrUnsupportedEdit,
		}
	}
	wbPart, ok := p.chartWorkbookPartOf(part)
	if !ok {
		return &OperationError{
			Op: op, Part: string(part),
			Message: "chart has no embedded workbook relationship", Err: ErrUnsupportedEdit,
		}
	}
	chartXML, err := buildChartSpaceXML(cd)
	if err != nil {
		return Annotate(err, op)
	}
	wb, err := p.chartWorkbookBytes(ChartDataBook{
		SheetName:  chartSheetName,
		Categories: cd.Categories,
		Series:     cd.Series,
	})
	if err != nil {
		return Annotate(err, op)
	}
	plan := editplan.NewMultiPartPlan(
		editplan.Patch(part, []byte(xmlDecl+chartXML)),
		editplan.Patch(wbPart, wb),
	)
	if err := applyMultiPartPlan(p, plan); err != nil {
		return Annotate(err, op)
	}
	return nil
}

// chartIsCanonical 校验 chartSpace 是否为本库生成的规范布局。
// 任何白名单之外的元素（客户端重写引入的 spPr/dLbls/trendline 等）
// 均判定为非规范，SetData 拒绝而非部分合并（受限编辑语义）。
func chartIsCanonical(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord) bool {
	// chartSpace 子元素：仅 c:chart。
	chart := childOfKind(doc, root, nsChartML, "chart", 0)
	if chart == nil || len(root.Children) != 1 {
		return false
	}
	chartKids := map[string]int{}
	for _, cid := range chart.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			return false
		}
		if !strIn(cn.Local(), "title", "autoTitleDeleted", "plotArea", "plotVisOnly", "dispBlanksAs") {
			return false
		}
		chartKids[cn.Local()]++
	}
	if chartKids["plotArea"] != 1 || chartKids["title"] > 1 || chartKids["autoTitleDeleted"] > 1 ||
		chartKids["plotVisOnly"] > 1 || chartKids["dispBlanksAs"] > 1 {
		return false
	}
	// 标题子树：仅 c:tx/c:overlay + a: 富文本骨架。
	if title := childOfKind(doc, chart, nsChartML, "title", 0); title != nil {
		if !canonicalTitleSubtree(doc, title) {
			return false
		}
	}
	plotArea := childOfKind(doc, chart, nsChartML, "plotArea", 0)
	if plotArea == nil {
		return false
	}
	// plotArea：至多一个 c:layout，恰好一个受限图表组，柱/折线需成对轴
	// （catAx/dateAx/valAx）。允许 dateAx 替代 catAx（CHART-02 R 档）。
	var plot *xmlstore.NodeRecord
	axCount := map[string]int{}
	for _, cid := range plotArea.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			return false
		}
		switch cn.Local() {
		case "layout":
			if len(cn.Children) != 0 {
				return false // 规范布局是 <c:layout/>（空）
			}
		case "catAx", "dateAx", "valAx":
			axCount[cn.Local()]++
		case "barChart", "lineChart", "pieChart":
			if plot != nil {
				return false // 多图表组（组合图）非规范
			}
			plot = cn
		default:
			return false
		}
	}
	if plot == nil {
		return false
	}
	typ, _ := chartTypeFromPlot(plot.Local())
	axIDCount := 0
	serCount := 0
	for _, cid := range plot.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			return false
		}
		switch cn.Local() {
		case "ser":
			serCount++
			if !canonicalSer(doc, cn, typ) {
				return false
			}
		case "axId":
			axIDCount++
		default:
			allowed := false
			switch typ {
			case ChartBar:
				allowed = strIn(cn.Local(), "barDir", "grouping", "varyColors", "gapWidth", "dLbls")
			case ChartLine:
				allowed = strIn(cn.Local(), "grouping", "varyColors", "marker", "dLbls")
			case ChartPie:
				allowed = strIn(cn.Local(), "varyColors", "firstSliceAng", "dLbls")
			}
			if !allowed {
				return false
			}
		}
	}
	if serCount == 0 {
		return false
	}
	switch typ {
	case ChartBar, ChartLine:
		// 类别（catAx）或日期（dateAx）恰好 1，valAx 恰好 1。
		catAxes := axCount["catAx"] + axCount["dateAx"]
		if axIDCount != 2 || catAxes != 1 || axCount["valAx"] != 1 {
			return false
		}
	case ChartPie:
		if axIDCount != 0 || axCount["catAx"] != 0 || axCount["dateAx"] != 0 || axCount["valAx"] != 0 {
			return false
		}
	}
	// valAx 可携带 logBase / min / max（CHART-02 R 档）；任何未列出子元素
	// 仍走整体拒绝（不部分合并）。
	valAx := childOfKind(doc, plotArea, nsChartML, "valAx", 0)
	if valAx != nil && !canonicalAxExtensions(doc, valAx, true) {
		return false
	}
	for _, lk := range []string{"catAx", "dateAx"} {
		ax := childOfKind(doc, plotArea, nsChartML, lk, 0)
		if ax != nil && !canonicalAxExtensions(doc, ax, false) {
			return false
		}
	}
	return true
}

// canonicalAxExtensions 校验 catAx/dateAx/valAx 可识别子元素集合。
// allowLogBase=true 时允许 c:logBase（仅值轴）。
func canonicalAxExtensions(doc *xmlstore.XMLDocument, ax *xmlstore.NodeRecord, allowLogBase bool) bool {
	for _, cid := range ax.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			return false
		}
		if !strIn(cn.Local(), "axId", "scaling", "delete", "axPos", "crossAx", "min", "max") {
			return false
		}
		if cn.Local() == "scaling" {
			for _, sid := range cn.Children {
				sn := doc.Node(sid)
				if sn.Namespace != nsChartML {
					return false
				}
				if sn.Local() == "logBase" && !allowLogBase {
					return false
				}
				if !strIn(sn.Local(), "orientation", "logBase") {
					return false
				}
			}
		}
	}
	return true
}

// canonicalSer 校验系列子元素是否规范（按类型的白名单）。
// CHART-02：扩展接受 trendline / errBars。
func canonicalSer(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord, typ ChartType) bool {
	for _, cid := range ser.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			return false
		}
		ok := strIn(cn.Local(), "idx", "order", "tx", "cat", "val", "trendline", "errBars")
		if typ == ChartLine && cn.Local() == "smooth" {
			ok = true
		}
		if !ok {
			return false
		}
		if cn.Local() == "trendline" && !canonicalTrendline(doc, cn) {
			return false
		}
		if cn.Local() == "errBars" && !canonicalErrBars(doc, cn) {
			return false
		}
	}
	return true
}

// canonicalTrendline 校验 trendline 子元素白名单。
func canonicalTrendline(doc *xmlstore.XMLDocument, tr *xmlstore.NodeRecord) bool {
	for _, cid := range tr.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			return false
		}
		if !strIn(cn.Local(), "name", "trendlineType", "dispEq", "dispRSqr", "intercept") {
			return false
		}
	}
	return true
}

// canonicalErrBars 校验 errBars 子元素白名单。
func canonicalErrBars(doc *xmlstore.XMLDocument, eb *xmlstore.NodeRecord) bool {
	for _, cid := range eb.Children {
		cn := doc.Node(cid)
		if cn.Namespace != nsChartML {
			return false
		}
		if !strIn(cn.Local(), "errDir", "errBarType", "val", "noEndCap") {
			return false
		}
	}
	return true
}

// canonicalTitleSubtree 校验标题子树（c:tx/c:overlay + a: 富文本骨架）。
func canonicalTitleSubtree(doc *xmlstore.XMLDocument, title *xmlstore.NodeRecord) bool {
	for _, cid := range title.Children {
		cn := doc.Node(cid)
		switch {
		case cn.Namespace == nsChartML && cn.Local() == "overlay":
			continue
		case cn.Namespace == nsChartML && cn.Local() == "tx":
			if !canonicalRichText(doc, cn) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// canonicalRichText 校验 c:tx 内的 a: 富文本骨架。
func canonicalRichText(doc *xmlstore.XMLDocument, tx *xmlstore.NodeRecord) bool {
	rich := childOfKind(doc, tx, nsChartML, "rich", 0)
	if rich == nil || len(tx.Children) != 1 {
		return false
	}
	var walk func(n *xmlstore.NodeRecord) bool
	walk = func(n *xmlstore.NodeRecord) bool {
		if n.Namespace != nsDrawingML ||
			!strIn(n.Local(), "bodyPr", "lstStyle", "p", "pPr", "r", "rPr", "defRPr", "t", "br") {
			return false
		}
		for _, cid := range n.Children {
			if !walk(doc.Node(cid)) {
				return false
			}
		}
		return true
	}
	for _, cid := range rich.Children {
		if !walk(doc.Node(cid)) {
			return false
		}
	}
	return true
}

// strIn 报告 s 是否等于列表中任一值。
func strIn(s string, list ...string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}
