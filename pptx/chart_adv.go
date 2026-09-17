package pptx

import (
	chartinternal "github.com/F31/go-pptx/internal/chart"
)

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

// ---------- 公共类型（type alias，ADR-017 第二批） ----------

// ChartDataLabel 描述图表级数据标签 c:dLbls（R 档）。
type ChartDataLabel = chartinternal.ChartDataLabel

// ChartErrorType 枚举 c:errBars 合法 errBarType 值。
type ChartErrorType = chartinternal.ChartErrorType

// ChartErrorBars 描述单个系列的误差线 c:errBars（R 档白名单）。
type ChartErrorBars = chartinternal.ChartErrorBars

// ChartTrendType 枚举 c:trendline 合法 trendlineType 值。
type ChartTrendType = chartinternal.ChartTrendType

// ChartTrendline 描述单个系列的趋line c:trendline。
type ChartTrendline = chartinternal.ChartTrendline

// ChartAxisOptions 描述轴扩展（R 档）。
type ChartAxisOptions = chartinternal.ChartAxisOptions

// iota 枚举值别名（根包公共 API 保留 ChartErr*/ChartTrend* 名字）。
const (
	// ChartErrStandardDeviation 对应 c:errBarType val="stdDev"。
	ChartErrStandardDeviation = chartinternal.ChartErrStandardDeviation
	// ChartErrStandardError 对应 c:errBarType val="stdErr"。
	ChartErrStandardError = chartinternal.ChartErrStandardError
	// ChartErrFixed 对应 c:errBarType val="fixed" + val=Value。
	ChartErrFixed = chartinternal.ChartErrFixed
	// ChartErrPercentage 对应 c:errBarType val="percentage" + val=Value。
	ChartErrPercentage = chartinternal.ChartErrPercentage

	// ChartTrendLinear 对应 c:trendlineType val="linear"。
	ChartTrendLinear = chartinternal.ChartTrendLinear
	// ChartTrendLogarithmic 对应 c:trendlineType val="log"。
	ChartTrendLogarithmic = chartinternal.ChartTrendLogarithmic
	// ChartTrendExponential 对应 c:trendlineType val="exp"。
	ChartTrendExponential = chartinternal.ChartTrendExponential
	// ChartTrendPolynomial 对应 c:trendlineType val="poly"。
	ChartTrendPolynomial = chartinternal.ChartTrendPolynomial
	// ChartTrendPower 对应 c:trendlineType val="power"。
	ChartTrendPower = chartinternal.ChartTrendPower
	// ChartTrendMovingAverage 对应 c:trendlineType val="movingAvg"。
	ChartTrendMovingAverage = chartinternal.ChartTrendMovingAverage
)

// String 方法已搬到 internal/chart 包（ADR-017 第二批：方法必须定义在
// 类型所在包，alias 上不能定义新方法）。
