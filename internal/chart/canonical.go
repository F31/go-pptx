package chart

import (
	"github.com/F31/go-pptx/internal/xmlstore"
)

// Canonical validation helpers（SetData 拒收非规范 chart Part）。
//
// 语义锁死：
//   - 严格白名单：不在白名单的元素 → 非规范（false）；
//   - 客户端重写引入的 spPr/dLbls/trendline 扩展等"超出受限编辑范围"
//     视为非规范，避免部分合并。
//
// ADR-017 第三批：实现搬到 internal/chart，根包保留薄 wrapper。
// 本文件是根包 chart.go 原 canonical* 系列的逐条忠实搬迁——任何白名单
// 差异都会改变 SetData 的拒收边界，禁止"顺手放宽"。

// IsCanonical 判定 chartSpace 是否为受限布局（用于 SetData 拒收）。
//
// chartSpace 子元素：仅 c:chart。
func IsCanonical(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord) bool {
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
		if !CanonicalTitleSubtree(doc, title) {
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
	typ, _ := ChartTypeFromPlot(plot.Local())
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
			if !CanonicalSer(doc, cn, typ) {
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
	if valAx != nil && !CanonicalAxExtensions(doc, valAx, true) {
		return false
	}
	for _, lk := range []string{"catAx", "dateAx"} {
		ax := childOfKind(doc, plotArea, nsChartML, lk, 0)
		if ax != nil && !CanonicalAxExtensions(doc, ax, false) {
			return false
		}
	}
	return true
}

// CanonicalAxExtensions 校验 catAx/dateAx/valAx 可识别子元素集合。
// allowLogBase=true 时允许 c:logBase（仅值轴）。
func CanonicalAxExtensions(doc *xmlstore.XMLDocument, ax *xmlstore.NodeRecord, allowLogBase bool) bool {
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

// CanonicalSer 校验系列子元素是否规范（按类型的白名单）。
// CHART-02：扩展接受 trendline / errBars。
func CanonicalSer(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord, typ ChartType) bool {
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
		if cn.Local() == "trendline" && !CanonicalTrendline(doc, cn) {
			return false
		}
		if cn.Local() == "errBars" && !CanonicalErrBars(doc, cn) {
			return false
		}
	}
	return true
}

// CanonicalTrendline 校验 trendline 子元素白名单。
func CanonicalTrendline(doc *xmlstore.XMLDocument, tr *xmlstore.NodeRecord) bool {
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

// CanonicalErrBars 校验 errBars 子元素白名单。
func CanonicalErrBars(doc *xmlstore.XMLDocument, eb *xmlstore.NodeRecord) bool {
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

// CanonicalTitleSubtree 校验标题子树（c:tx/c:overlay + a: 富文本骨架）。
func CanonicalTitleSubtree(doc *xmlstore.XMLDocument, title *xmlstore.NodeRecord) bool {
	for _, cid := range title.Children {
		cn := doc.Node(cid)
		switch {
		case cn.Namespace == nsChartML && cn.Local() == "overlay":
			continue
		case cn.Namespace == nsChartML && cn.Local() == "tx":
			if !CanonicalRichText(doc, cn) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// CanonicalRichText 校验 c:tx 内的 a: 富文本骨架。
func CanonicalRichText(doc *xmlstore.XMLDocument, tx *xmlstore.NodeRecord) bool {
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
