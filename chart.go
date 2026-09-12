package pptx

import (
	"context"
	"fmt"

	chartinternal "github.com/F31/go-pptx/internal/chart"
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
// ADR-017 第一批：实现搬到 internal/chart.GraphicURI；调用方直接用 chartinternal.GraphicURI。
// 保留此文件级常量仅供本地文档/历史对照使用。
const chartGraphicURI = nsChartML

// chartSheetName 是工作簿数据表名（图表引用固定指向它）。
//
// ADR-017 第一批：实现搬到 internal/chart.SheetName；调用方直接用 chartinternal.SheetName。
const chartSheetName = "Sheet1"

// 规范布局的轴 ID（chart Part 内部作用域，两值固定）。
//
// ADR-017 第一批：实现搬到 internal/chart.CatAxID / chartinternal.ValAxID；调用方直接用之。
const (
	chartCatAxID = 100000001
	chartValAxID = 100000002
)

// ---------- 公共类型 ----------

// ---------- 公共类型（type alias，ADR-017 第二批） ----------

// ChartType 是受限图表类型（CHART-01 范围：柱/折/饼）。
//
// ADR-017 第二批：实现搬到 internal/chart.ChartType；本类型为 alias，
// 调用方零修改，公共 API 表面零变化（field/method set 自动共享）。
type ChartType = chartinternal.ChartType

// ChartSpec 描述新增图表（单位：EMU）。Width/Height 必需（>0）。
type ChartSpec = chartinternal.ChartSpec

// ChartSeries 是一个数据系列（名称 + 与类别等长的数值）。
type ChartSeries = chartinternal.ChartSeries

// ChartData 是图表数据的读写快照。
type ChartData = chartinternal.ChartData

// iota 枚举值别名（根包公共 API 保留 ChartBar/ChartLine/ChartPie 名字）。
const (
	// ChartBar 是簇状柱状图（barDir=col，单值轴）。
	ChartBar = chartinternal.ChartBar
	// ChartLine 是折线图（标准分组，无平滑）。
	ChartLine = chartinternal.ChartLine
	// ChartPie 是饼图。
	ChartPie = chartinternal.ChartPie
)

// chartTypeFromPlot 由图表组元素名解析类型。
func chartTypeFromPlot(local string) (ChartType, bool) {
	return chartinternal.ChartTypeFromPlot(local)
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
	return chartinternal.BuildChartFrameFragment(id, x, y, cx, cy, rid)
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
//
// ADR-017 第三批：实现搬到 internal/chart.ValidateChartData；根包 wrapper
// 见 chartfrag.go（保留薄包装以使 root 调用方零修改；错误由 wrapper 加
// Annotate 注解，保持与原 pptx.OperationError 行为一致）。

// ---------- chart XML 生成（规范布局） ----------
//
// ADR-017 第三批：buildChartSpaceXML 实现搬到 internal/chart.BuildChartSpaceXML；
// 根包保留薄包装，调用方零修改。错误 *BuildError 由 wrapper 转 *OperationError。

func buildChartSpaceXML(cd ChartData) (string, error) {
	out, err := chartinternal.BuildChartSpaceXML(cd, chartSheetName, chartCatAxID, chartValAxID)
	if err == nil {
		return out, nil
	}
	if be, ok := err.(*chartinternal.BuildError); ok {
		return "", &OperationError{Op: be.Op, Message: be.Message, Err: be.Sentinel}
	}
	return "", Annotate(err, "chart")
}

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
//
// ADR-017 第三批：通用 helper 留在根包（chart 内部同名 helper 在
// internal/chart/parse.go 内自持一份；xlsx 单元格解析共用此通用版）。
func nodeText(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) string {
	if n == nil || n.SelfClosing() {
		return ""
	}
	return xmlUnescape(string(doc.Original()[n.OpenEnd:n.CloseStart]))
}

// parseChartSpace 从 chartSpace 节点解析受限图表数据。
func parseChartSpace(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord) (ChartData, error) {
	cd, err := chartinternal.ParseChartSpace(doc, root)
	if err != nil {
		return cd, convertValidationError(err, "ParseChartSpace")
	}
	return cd, nil
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
	return chartinternal.IsCanonical(doc, root)
}
