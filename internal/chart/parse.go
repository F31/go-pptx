package chart

import (
	"strconv"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// parse + canonical helpers。所有函数零依赖根包类型，与 root 包同名私有
// helper 同源——把 chart 内部遍历逻辑下移到 internal/chart，让 chart Part
// 字面输出与读取双侧逻辑集中。
//
// ADR-017 第三批：实现搬到 internal/chart；根包保留薄包装。

// childOfKind 返回 parent 下第 nth 个（0 基）ns/local 匹配的子元素。
//
// 等同根包 text.go childOfKind 的最小复刻（13 行）。维护时务必同步
// 根包实现（语义锁死：nth=0 取首个匹配，nth>=匹配数返回 nil）。
func childOfKind(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, ns, local string, nth int) *xmlstore.NodeRecord {
	seen := 0
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == ns && c.Local() == local {
			if seen == nth {
				return c
			}
			seen++
		}
	}
	return nil
}

// childText 返回第 nth 个 ns/local 匹配子元素的 val 属性文本。
func childText(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, ns, local string, nth int) string {
	n := childOfKind(doc, parent, ns, local, nth)
	if n == nil {
		return ""
	}
	v, _ := n.Attr("", "val")
	return v
}

// strIn 是 chart 内部的字符串白名单判定辅助（与 root 包同名私有 helper
// 同源）。导出给本包内调用，不污染公共 API 表面。
func strIn(s string, list ...string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// nodeText 读取元素文本内容（实体解码；自闭合元素返回空串）。
//
// 关键：c:v / a:t / spreadsheetml t 的值都在**标签之间**而非属性里，
// 必须取 Original()[OpenEnd:CloseStart]；用 Attr("val") 会永远读空。
func nodeText(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) string {
	if n == nil || n.SelfClosing() {
		return ""
	}
	return xmlUnescape(string(doc.Original()[n.OpenEnd:n.CloseStart]))
}

// NodeText 拼接子树内全部 a:t 文本（用于 c:title/c:tx/c:rich）。
func NodeText(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) string {
	if n == nil {
		return ""
	}
	var sb []byte
	var walk func(n *xmlstore.NodeRecord)
	walk = func(n *xmlstore.NodeRecord) {
		if n.Namespace == nsDrawingML && n.Local() == "t" {
			sb = append(sb, nodeText(doc, n)...)
		}
		for _, cid := range n.Children {
			walk(doc.Node(cid))
		}
	}
	walk(n)
	return string(sb)
}

// ParseChartSpace 读回受限图表数据（CHART-01/CHART-02 R 档）。
func ParseChartSpace(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord) (ChartData, error) {
	chart := childOfKind(doc, root, nsChartML, "chart", 0)
	if chart == nil {
		return ChartData{}, &BuildError{Op: "ParseChartSpace", Message: "missing c:chart", Sentinel: ErrUnsupportedFormat}
	}
	cd := ChartData{}
	// 标题
	if title := childOfKind(doc, chart, nsChartML, "title", 0); title != nil {
		if tx := childOfKind(doc, title, nsChartML, "tx", 0); tx != nil {
			if rich := childOfKind(doc, tx, nsChartML, "rich", 0); rich != nil {
				cd.Title = NodeText(doc, rich)
			}
		}
	}
	// plotArea 下的图表组元素决定图表类型
	plotArea := childOfKind(doc, chart, nsChartML, "plotArea", 0)
	if plotArea == nil {
		return ChartData{}, &BuildError{Op: "ParseChartSpace", Message: "missing c:plotArea", Sentinel: ErrUnsupportedFormat}
	}
	var plot *xmlstore.NodeRecord
	for _, c := range plotArea.Children {
		cn := doc.Node(c)
		if cn.Namespace != nsChartML {
			continue
		}
		if _, ok := ChartTypeFromPlot(cn.Local()); ok {
			if plot == nil {
				plot = cn
			}
			// 组合图（多个图表组）：读取首个受限组，其余忽略。
		}
	}
	if plot == nil {
		return ChartData{}, &BuildError{Op: "ParseChartSpace", Message: "no supported chart group (bar/line/pie)", Sentinel: ErrUnsupportedFormat}
	}
	cd.Type, _ = ChartTypeFromPlot(plot.Local())
	// plot 与原 chart.go 行为锁死：仅读首个受限组，组合图其余 group 忽略。
	// 图表级 c:dLbls 是**图表组**的子元素（barChart/lineChart/pieChart），
	// 不是 c:plotArea 的直接子元素（与 build 侧对称）。
	if dl := childOfKind(doc, plot, nsChartML, "dLbls", 0); dl != nil {
		cd.DataLabel = ParseChartDLbls(doc, dl)
	}
	// 系列
	for _, c := range plot.Children {
		cn := doc.Node(c)
		if cn.Namespace != nsChartML {
			continue
		}
		if cn.Local() != "ser" {
			continue
		}
		cs := ChartSeries{Name: SerName(doc, cn)}
		// 首个成功解析出类别的系列决定全局类别（后续系列不覆盖）。
		if cat, ok := SerCategories(doc, cn); ok && len(cd.Categories) == 0 {
			cd.Categories = cat
		}
		cs.Values = SerValues(doc, cn)
		if tr := childOfKind(doc, cn, nsChartML, "trendline", 0); tr != nil {
			cs.Trendline = ParseChartTrendline(doc, tr)
		}
		if eb := childOfKind(doc, cn, nsChartML, "errBars", 0); eb != nil {
			cs.ErrorBars = ParseChartErrBars(doc, eb)
		}
		cd.Series = append(cd.Series, cs)
	}
	cd.Axes = ParseChartAxes(doc, plotArea)
	return cd, nil
}

// ParseChartDLbls 读回图表级 c:dLbls。
func ParseChartDLbls(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *ChartDataLabel {
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

// ParseChartTrendline 读回 c:trendline。
func ParseChartTrendline(doc *xmlstore.XMLDocument, tr *xmlstore.NodeRecord) *ChartTrendline {
	out := &ChartTrendline{}
	out.Name = childText(doc, tr, nsChartML, "name", 0)
	if v := childText(doc, tr, nsChartML, "trendlineType", 0); v != "" {
		out.Type = TrendTypeFromName(v)
	}
	if v := childText(doc, tr, nsChartML, "dispEq", 0); v != "" {
		out.DisplayEq = v == "1" || v == "true"
	}
	if v := childText(doc, tr, nsChartML, "dispRSqr", 0); v != "" {
		out.DisplayRSq = v == "1" || v == "true"
	}
	if v := childText(doc, tr, nsChartML, "intercept", 0); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			out.SetIntercept = true
			out.Intercept = f
		}
	}
	return out
}

// ParseChartErrBars 读回 c:errBars。
func ParseChartErrBars(doc *xmlstore.XMLDocument, eb *xmlstore.NodeRecord) *ChartErrorBars {
	out := &ChartErrorBars{Direction: "both"}
	if n := childOfKind(doc, eb, nsChartML, "errBarType", 0); n != nil {
		v, _ := n.Attr("", "val")
		out.Type = ErrorTypeFromName(v)
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

// ParseChartAxes 读回 catAx/dateAx/valAx 扩展配置。
func ParseChartAxes(doc *xmlstore.XMLDocument, plotArea *xmlstore.NodeRecord) *ChartAxisOptions {
	out := &ChartAxisOptions{}
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
			if v := childText(doc, ax, nsChartML, "min", 0); v != "" {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					out.Min = NewOptional(f)
				}
			}
			if v := childText(doc, ax, nsChartML, "max", 0); v != "" {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					out.Max = NewOptional(f)
				}
			}
		}
		if p, ok := ax.Attr("", "axPos"); ok {
			out.Position = p
		}
	}
	return out
}

// serName / serCategories / serValues / serCachePoints 读回系列数据的助手，
// 与 root 包 chart.go 中同名私有 helper 同源。

// SerName 读 c:ser/c:tx 下系列名（c:v 字面优先，strCache 首个 pt 兜底）。
func SerName(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord) string {
	tx := childOfKind(doc, ser, nsChartML, "tx", 0)
	if tx == nil {
		return ""
	}
	if v := childOfKind(doc, tx, nsChartML, "v", 0); v != nil {
		return nodeText(doc, v)
	}
	if ref := childOfKind(doc, tx, nsChartML, "strRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "strCache", 0); cache != nil {
			if pts := CachePoints(doc, cache); len(pts) > 0 {
				return pts[0]
			}
		}
	}
	return ""
}

// SerCategories 读 c:ser/c:cat 下类别字符串；返回 nil 表示仅有 c:val 数值。
//
// 分支优先级与根包历史行为锁死：strRef/strCache → numRef/numCache →
// strLit → numLit。
func SerCategories(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord) ([]string, bool) {
	cat := childOfKind(doc, ser, nsChartML, "cat", 0)
	if cat == nil {
		return nil, false
	}
	if ref := childOfKind(doc, cat, nsChartML, "strRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "strCache", 0); cache != nil {
			return CachePoints(doc, cache), true
		}
	}
	if ref := childOfKind(doc, cat, nsChartML, "numRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "numCache", 0); cache != nil {
			return CachePoints(doc, cache), true
		}
	}
	if lit := childOfKind(doc, cat, nsChartML, "strLit", 0); lit != nil {
		return CachePoints(doc, lit), true
	}
	if lit := childOfKind(doc, cat, nsChartML, "numLit", 0); lit != nil {
		return CachePoints(doc, lit), true
	}
	return nil, false
}

// SerValues 读 c:ser/c:val 下数值序列（numRef/numCache 优先，否则 numLit）。
//
// 非数值点补 0（保持与类别等长）——与根包历史行为锁死。
func SerValues(doc *xmlstore.XMLDocument, ser *xmlstore.NodeRecord) []float64 {
	val := childOfKind(doc, ser, nsChartML, "val", 0)
	if val == nil {
		return nil
	}
	var pts []string
	if ref := childOfKind(doc, val, nsChartML, "numRef", 0); ref != nil {
		if cache := childOfKind(doc, ref, nsChartML, "numCache", 0); cache != nil {
			pts = CachePoints(doc, cache)
		}
	} else if lit := childOfKind(doc, val, nsChartML, "numLit", 0); lit != nil {
		pts = CachePoints(doc, lit)
	}
	out := make([]float64, 0, len(pts))
	for _, p := range pts {
		if f, err := strconv.ParseFloat(p, 64); err == nil {
			out = append(out, f)
		} else {
			out = append(out, 0)
		}
	}
	return out
}

// CachePoints 读 c:strCache/numCache/strLit/numLit 的 c:pt/c:v，
// 按 pt@idx 顺序回填（缺号位补空串）。
func CachePoints(doc *xmlstore.XMLDocument, cache *xmlstore.NodeRecord) []string {
	type point struct {
		idx int
		v   string
	}
	var pts []point
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
		pts = append(pts, point{idx: idx, v: nodeText(doc, v)})
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
	for _, p := range pts {
		if p.idx >= 0 && p.idx <= max {
			out[p.idx] = p.v
		}
	}
	return out
}

// ChartTypeFromPlot 由图表组元素名解析类型。
func ChartTypeFromPlot(local string) (ChartType, bool) {
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

// TrendTypeFromName 由 trendlineType val 反查枚举。
func TrendTypeFromName(v string) ChartTrendType {
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

// ErrorTypeFromName 由 errBarType val 反查枚举。
func ErrorTypeFromName(v string) ChartErrorType {
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
