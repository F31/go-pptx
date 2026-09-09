package pptx

// 本文件实现 CHART-02（方案 §9.2 后续 / §2.3 矩阵）：图表数据标签、
// 误差线、趋势线、类别轴日期模式与值轴对数/极值扩展。仅 R 档白名单——
// 超出即显式 ErrInvalidArgument，绝不静默降级。
//
// 受限范围（方案 §9.2「先支持创建明确限定的…复杂图表…先保留，不伪
// 称可编辑」的延续）：
//   - 趋势线 6 种类型（linear/log/exponential/polynomial/power/moving
//     average）；polynomial Order ∈ [2,6]；movingAverage Period ≥2。
//   - 误差线 4 种（standardDeviation / standardError / fixed / percent）；
//     Direction ∈ plus/minus/both；NoEndCap=false 写 <c:noEndCap val="0"/>。
//   - 图表级数据标签 c:dLbls：Show=true 即整图组启用；Position ∈
//     白名单（详见 ChartDataLabel 说明）。
//   - 对数轴：ChartAxisOptions.ValueLogBase ∈ {0,2..32}；0 = 不写。
//   - 日期类别轴：ChartAxisOptions.CategoryAsDate=true → 图表组末用
//     <c:dateAx> 替代 <c:catAx>；<c:catAx> 不混用。
//   - 值轴 Min/Max：Optional[float64]，Set=true 即写 <c:min>/<c:max>。

// ---------- 公共类型 ----------

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
type ChartAxisOptions struct {
	CategoryAsDate bool
	ValueLogBase   int
	Min, Max       Optional[float64]
	Position       string // b / l / t / r，空保留默认
}
