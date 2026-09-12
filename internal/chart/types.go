// Package chart 承载 go-pptx chart 系列的纯实现抽取（ADR-017）。
//
// 本文件包含第二批值对象定义：11 个公共类型（4 个 chart.go + 7 个 chartadv.go）。
//
// 依赖图（按 ADR-014 / ADR-016）：
//
//	internal/chart  → internal/xmlstore (后续批次)
//	root pptx       → internal/chart (type alias 形式引用所有值对象)
//
// 本包绝不反向 import 根包；值对象定义全部以"无依赖"形态出现。
package chart

// =============== 值对象（type alias move 源） ===============

import "fmt"

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

// String 返回类型的语义字符串（PowerPoint plotElement 与 JSON 序列化共用）。
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

// PlotElement 返回类型对应的 c: 图表组元素名（barChart / lineChart / pieChart）。
//
// 公开名以满足包外访问；root 包通过 type alias 引用本类型。
func (t ChartType) PlotElement() string {
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

// ChartSpec 描述新增图表（单位：EMU）。Width/Height 必需（>0）。
//
//   - X/Y 是页面左上角的 EMU 偏移；
//   - Width/Height 是图表区域尺寸（EMU）；
//   - Title/DataLabel/Axes 是图表级可选扩展；
//   - Series 内 ErrorBars/Trendline 由各 ChartSeries 自身携带，按 OOXML 序
//     排在 c:tx 与 c:cat 之间。
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

// ChartDataLabel 描述图表级数据标签 c:dLbls（R 档）。Show=false 即不
// 写（PowerPoint 默认隐藏）；Show=true 写 <c:dLbls>；Position ∈ 白
// 名单 ctr / inr / inb / outT / outB / outL / outR / t / b / l / r。
type ChartDataLabel struct {
	Show     bool
	Position string // 留空 = PowerPoint 默认；不允许白名单外值
}

// ChartErrorType 枚举 c:errBars 合法 errBarType 值。
type ChartErrorType int

const (
	// ChartErrStandardDeviation 对应 c:errBarType val="stdDev"。
	ChartErrStandardDeviation ChartErrorType = iota
	// ChartErrStandardError 对应 c:errBarType val="stdErr"。
	ChartErrStandardError
	// ChartErrFixed 对应 c:errBarType val="fixed" + val=Value。
	ChartErrFixed
	// ChartErrPercentage 对应 c:errBarType val="percentage" + val=Value。
	ChartErrPercentage
)

// String 返回 errBarType val 字符串。
func (e ChartErrorType) String() string {
	switch e {
	case ChartErrStandardDeviation:
		return "stdDev"
	case ChartErrStandardError:
		return "stdErr"
	case ChartErrFixed:
		return "fixed"
	case ChartErrPercentage:
		return "percentage"
	}
	return ""
}

// ChartErrorBars 描述单个系列的误差线 c:errBars（R 档白名单）。
// Pie 图不支持误差线（生成时静默跳过该字段，写入读取完整性见测试）。
type ChartErrorBars struct {
	Type      ChartErrorType // 必填，固定四值之一
	Value     float64        // Fixed/Percentage 时使用；stdDev/stdErr 忽略
	Direction string         // plus / minus / both（白名单）
	NoEndCap  bool           // <c:noEndCap val="1"/>
}

// ChartTrendType 枚举 c:trendline 合法 trendlineType 值。
type ChartTrendType int

const (
	ChartTrendLinear ChartTrendType = iota
	ChartTrendLogarithmic
	ChartTrendExponential
	ChartTrendPolynomial
	ChartTrendPower
	ChartTrendMovingAverage
)

// String 返回 trendlineType val 字符串。
func (t ChartTrendType) String() string {
	switch t {
	case ChartTrendLinear:
		return "linear"
	case ChartTrendLogarithmic:
		return "log"
	case ChartTrendExponential:
		return "exp"
	case ChartTrendPolynomial:
		return "poly"
	case ChartTrendPower:
		return "power"
	case ChartTrendMovingAverage:
		return "movingAvg"
	}
	return ""
}

// ChartTrendline 描述单个系列的趋line c:trendline。
//   - Polynomial.Order ∈ [2,6]；
//   - MovingAverage.Period ≥2；其余类型忽略；
//   - SetIntercept=true 即写 <c:intercept val="Intercept"/>（exp/poly/
//     power/linear 等可强制截距为某值）；
//   - Name 为趋势线名称，空 = PowerPoint 默认 "Linear (Series n)"。
type ChartTrendline struct {
	Type         ChartTrendType // 必填
	Period       int            // movingAvg 用，≥2
	Order        int            // poly 用，∈[2,6]
	DisplayEq    bool           // dispEq
	DisplayRSq   bool           // dispRSqr
	Name         string         // trendline 名称
	Intercept    float64        // 强制截距
	SetIntercept bool           // true 时写 Intercept
}

// ChartAxisOptions 描述轴扩展（R 档）。零值 = 不写任何扩展（向后兼容
// CHART-01 默认 catAx+valAx）。
//
//   - CategoryAsDate=true → 图表组末以 <c:dateAx> 替代 <c:catAx>（要求
//     类别为可解析日期字符串；语义误用由客户端判定）。
//   - ValueLogBase=0 → 不写；∈ [2,32] → 在 <c:valAx>/<c:scaling> 中追加
//     <c:logBase val="n"/>，实现对数轴。
//   - Min/Max 为值轴上下界（Set=true 时写 <c:min>/<c:max>）。
//   - Position 允许显式指定 axPos val（b/l/t/r）：空 = 由类型决定默认。
//
// 注意：Min/Max 字段引用根包 Optional[T]，由根包负责提供；internal/chart
// 本身不依赖根包，Optional[T] 的导入见 internal/chart/optional.go。
type ChartAxisOptions struct {
	CategoryAsDate bool
	ValueLogBase   int
	Min, Max       Optional[float64]
	Position       string // b / l / t / r，空保留默认
}
