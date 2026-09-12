package chart

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// CHART-02 fragment / validator helpers。
// 严格白名单：白名单外任何输入都返回 *ValidationError；根包 wrapper
// 把它转换为 *pptx.OperationError 并保持 errors.Is(err, ErrXxx) 链有效。

// 数据标签位置白名单（PowerPoint §9.2 客户端允许值全集）。
var dataLabelPositionAllowed = map[string]bool{
	"ctr":  true,
	"inr":  true,
	"inb":  true,
	"outT": true, "outB": true, "outL": true, "outR": true,
	"t": true, "b": true, "l": true, "r": true,
}

// 误差线方向白名单。
var errBarDirectionAllowed = map[string]bool{
	"plus": true, "minus": true, "both": true,
}

// 轴位置白名单（axPos val）。
var axisPosAllowed = map[string]bool{
	"b": true, "l": true, "t": true, "r": true,
}

// ---------- Value validators ----------

// ValidateChartData 校验受限图表数据（AddChart 与 SetData 共用）。
//
// 嵌套校验：每个 Series 含 ErrorBars/Trendline 时再走 ValidateErrorBars /
// ValidateTrendline；顶层 DataLabel/Axes 同理。所有失败统一包为
// *ValidationError，根包 wrapper 负责 Annotate 加注解（保留 errors.Is 链）。
func ValidateChartData(cd ChartData, op string) error {
	if cd.Type != ChartBar && cd.Type != ChartLine && cd.Type != ChartPie {
		return &ValidationError{Op: op, Message: "unknown chart type", Sentinel: ErrInvalidArgument}
	}
	if len(cd.Categories) == 0 {
		return &ValidationError{Op: op, Message: "at least one category is required", Sentinel: ErrInvalidArgument}
	}
	if len(cd.Series) == 0 {
		return &ValidationError{Op: op, Message: "at least one series is required", Sentinel: ErrInvalidArgument}
	}
	for i, ser := range cd.Series {
		if len(ser.Values) != len(cd.Categories) {
			return &ValidationError{
				Op: op,
				Message: fmt.Sprintf("series %d has %d values, want %d (one per category)",
					i, len(ser.Values), len(cd.Categories)),
				Sentinel: ErrInvalidArgument,
			}
		}
		for j, v := range ser.Values {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return &ValidationError{
					Op:       op,
					Message:  fmt.Sprintf("series %d value %d is not finite", i, j),
					Sentinel: ErrInvalidArgument,
				}
			}
		}
		if cd.Type == ChartPie {
			if ser.ErrorBars != nil {
				return &ValidationError{
					Op: op, Message: "series " + strconv.Itoa(i) + ": pie chart does not support error bars",
					Sentinel: ErrInvalidArgument,
				}
			}
			if ser.Trendline != nil {
				return &ValidationError{
					Op: op, Message: "series " + strconv.Itoa(i) + ": pie chart does not support trendlines",
					Sentinel: ErrInvalidArgument,
				}
			}
		}
		if ser.ErrorBars != nil {
			if err := ValidateErrorBars(*ser.ErrorBars, op); err != nil {
				return annotateChart(err, "series "+strconv.Itoa(i)+".ErrorBars")
			}
		}
		if ser.Trendline != nil {
			if err := ValidateTrendline(*ser.Trendline, op); err != nil {
				return annotateChart(err, "series "+strconv.Itoa(i)+".Trendline")
			}
		}
	}
	if cd.DataLabel != nil {
		if err := ValidateDataLabel(*cd.DataLabel, op); err != nil {
			return annotateChart(err, "ChartDataLabel")
		}
	}
	if cd.Axes != nil {
		if err := ValidateAxisOptions(*cd.Axes, op); err != nil {
			return annotateChart(err, "ChartAxisOptions")
		}
	}
	return nil
}

// annotateChart 给 ValidationError 加上下文注解（不引入 root 依赖）。
//
// 返回新的 *ValidationError，保留 Unwrap 链（errors.Is 仍能命中 inner.Sentinel）。
func annotateChart(inner error, ctx string) error {
	if inner == nil {
		return nil
	}
	if ve, ok := inner.(*ValidationError); ok {
		return &ValidationError{
			Op: ve.Op, Message: ctx + ": " + ve.Message, Sentinel: ve.Sentinel,
		}
	}
	return fmt.Errorf("%s: %w", ctx, inner)
}

// ValidateDataLabel 校验图表级数据标签。
func ValidateDataLabel(d ChartDataLabel, op string) error {
	if !d.Show {
		return nil
	}
	if d.Position != "" && !dataLabelPositionAllowed[d.Position] {
		return &ValidationError{
			Op: op, Message: "ChartDataLabel position not in whitelist: " + d.Position,
			Sentinel: ErrInvalidArgument,
		}
	}
	return nil
}

// ValidateErrorBars 校验单系列误差线。
func ValidateErrorBars(e ChartErrorBars, op string) error {
	switch e.Type {
	case ChartErrFixed:
		if e.Value < 0 || math.IsInf(e.Value, 0) || math.IsNaN(e.Value) {
			return &ValidationError{
				Op: op, Message: "ErrorBars fixed value must be a finite non-negative number",
				Sentinel: ErrInvalidArgument,
			}
		}
	case ChartErrPercentage:
		if e.Value < 0 || e.Value > 1000 || math.IsInf(e.Value, 0) || math.IsNaN(e.Value) {
			return &ValidationError{
				Op: op, Message: "ErrorBars percentage must be in [0,1000]",
				Sentinel: ErrInvalidArgument,
			}
		}
	case ChartErrStandardDeviation, ChartErrStandardError:
		// 无 Value 校验
	default:
		return &ValidationError{
			Op: op, Message: "ErrorBars type out of range", Sentinel: ErrInvalidArgument,
		}
	}
	if e.Direction != "" && !errBarDirectionAllowed[e.Direction] {
		return &ValidationError{
			Op: op, Message: "ErrorBars direction not in whitelist: " + e.Direction,
			Sentinel: ErrInvalidArgument,
		}
	}
	return nil
}

// ValidateTrendline 校验单系列趋势线。
func ValidateTrendline(t ChartTrendline, op string) error {
	switch t.Type {
	case ChartTrendLinear, ChartTrendLogarithmic, ChartTrendExponential,
		ChartTrendPolynomial, ChartTrendPower, ChartTrendMovingAverage:
	default:
		return &ValidationError{
			Op: op, Message: "Trendline type out of range", Sentinel: ErrInvalidArgument,
		}
	}
	if t.Type == ChartTrendPolynomial {
		if t.Order < 2 || t.Order > 6 {
			return &ValidationError{
				Op: op, Message: "Trendline polynomial Order must be in [2,6]",
				Sentinel: ErrInvalidArgument,
			}
		}
	}
	if t.Type == ChartTrendMovingAverage && t.Period < 2 {
		return &ValidationError{
			Op: op, Message: "Trendline movingAverage Period must be >= 2",
			Sentinel: ErrInvalidArgument,
		}
	}
	return nil
}

// ValidateAxisOptions 校验轴扩展。
func ValidateAxisOptions(a ChartAxisOptions, op string) error {
	if a.ValueLogBase < 0 || (a.ValueLogBase != 0 && (a.ValueLogBase < 2 || a.ValueLogBase > 32)) {
		return &ValidationError{
			Op: op, Message: "Axes ValueLogBase must be 0 or in [2,32]",
			Sentinel: ErrInvalidArgument,
		}
	}
	if a.Position != "" && !axisPosAllowed[a.Position] {
		return &ValidationError{
			Op: op, Message: "Axes Position not in whitelist: " + a.Position,
			Sentinel: ErrInvalidArgument,
		}
	}
	if a.Min.Set && a.Max.Set && a.Min.Value > a.Max.Value {
		return &ValidationError{
			Op: op, Message: "Axes Min must be <= Max", Sentinel: ErrInvalidArgument,
		}
	}
	return nil
}

// ---------- Fragment builders (CHART-02) ----------

// BuildChartDataLabelFragment 构造图表级 c:dLbls。
// Show=false（外层短路）；Position 留空 = 写 <c:dLbls> 但不带 dLblPos。
func BuildChartDataLabelFragment(d ChartDataLabel) (string, error) {
	if !d.Show {
		return "", nil
	}
	frag := `<c:dLbls>`
	if d.Position != "" {
		frag += `<c:dLblPos val="` + d.Position + `"/>`
	}
	frag += `<c:showLegendKey val="0"/><c:showVal val="1"/><c:showCatName val="0"/>` +
		`<c:showSerName val="0"/><c:showPercent val="0"/><c:showBubbleSize val="0"/></c:dLbls>`
	return frag, nil
}

// BuildErrBarsFragment 构造 c:errBars。
// OOXML 序：errDir, errBarType, errValType（cusstdErr 等），val?, plus?, minus?, noEndCap。
// R 档简化：errDir + errBarType + val (Fixed/Percentage) + noEndCap。
func BuildErrBarsFragment(e ChartErrorBars) (string, error) {
	direction := e.Direction
	if direction == "" {
		direction = "both"
	}
	var frag strings.Builder
	frag.WriteString(`<c:errBars>`)
	frag.WriteString(`<c:errDir val="` + direction + `"/>`)
	frag.WriteString(`<c:errBarType val="` + e.Type.String() + `"/>`)
	if e.Type == ChartErrFixed || e.Type == ChartErrPercentage {
		frag.WriteString(`<c:val val="` + ChartNumber(e.Value) + `"/>`)
	}
	if e.NoEndCap {
		frag.WriteString(`<c:noEndCap val="1"/>`)
	}
	frag.WriteString(`</c:errBars>`)
	return frag.String(), nil
}

// BuildTrendlineFragment 构造 c:trendline。
// OOXML 序：name?, spPr?, trendlineType, dispEq?, dispRSqr?, trendlineLbl?, intercept?
func BuildTrendlineFragment(t ChartTrendline) (string, error) {
	var frag strings.Builder
	frag.WriteString(`<c:trendline>`)
	if t.Name != "" {
		esc, err := xmlstore.EscapeText(t.Name)
		if err != nil {
			return "", &ValidationError{
				Op: "trendline", Message: "name: " + err.Error(), Sentinel: ErrInvalidArgument,
			}
		}
		frag.WriteString(`<c:name val="` + esc + `"/>`)
	}
	frag.WriteString(`<c:trendlineType val="` + t.Type.String() + `"/>`)
	if t.DisplayEq {
		frag.WriteString(`<c:dispEq val="1"/>`)
	}
	if t.DisplayRSq {
		frag.WriteString(`<c:dispRSqr val="1"/>`)
	}
	if t.SetIntercept {
		frag.WriteString(`<c:intercept val="` + ChartNumber(t.Intercept) + `"/>`)
	}
	// MovingAverage.period 与 Polynomial.order：OOXML CT_Trendline 不含，
	// R 档简化不写（客户端用默认值）；保留分支为后续 CHART-02 增强预留。
	if t.Type == ChartTrendMovingAverage && t.Period > 0 {
		_ = t.Period
	}
	if t.Type == ChartTrendPolynomial && t.Order > 0 {
		_ = t.Order
	}
	frag.WriteString(`</c:trendline>`)
	return frag.String(), nil
}

// BuildCatOrDateAxFragment 构造类别轴（catAx 或 dateAx）。
func BuildCatOrDateAxFragment(a ChartAxisOptions, catAxID, valAxID int) string {
	tag := "c:catAx"
	if a.CategoryAsDate {
		tag = "c:dateAx"
	}
	axPos := "b"
	if a.Position != "" {
		axPos = a.Position
	}
	return `<` + tag + `><c:axId val="` + strconv.Itoa(catAxID) + `"/>` +
		`<c:scaling><c:orientation val="minMax"/></c:scaling>` +
		`<c:delete val="0"/><c:axPos val="` + axPos + `"/>` +
		`<c:crossAx val="` + strconv.Itoa(valAxID) + `"/></` + tag + `>`
}

// BuildValAxFragment 构造值轴（valAx）+ logBase/Min/Max 扩展。
func BuildValAxFragment(a ChartAxisOptions, axID, crossAxID int) string {
	var frag strings.Builder
	frag.WriteString(`<c:valAx><c:axId val="` + strconv.Itoa(axID) + `"/>`)
	frag.WriteString(`<c:scaling>`)
	if a.ValueLogBase != 0 {
		frag.WriteString(`<c:logBase val="` + strconv.Itoa(a.ValueLogBase) + `"/>`)
	}
	frag.WriteString(`<c:orientation val="minMax"/>`)
	frag.WriteString(`</c:scaling>`)
	axPos := "l"
	if a.Position != "" {
		axPos = a.Position
	}
	frag.WriteString(`<c:delete val="0"/><c:axPos val="` + axPos + `"/>`)
	if a.Min.Set {
		frag.WriteString(`<c:min val="` + ChartNumber(a.Min.Value) + `"/>`)
	}
	if a.Max.Set {
		frag.WriteString(`<c:max val="` + ChartNumber(a.Max.Value) + `"/>`)
	}
	frag.WriteString(`<c:crossAx val="` + strconv.Itoa(crossAxID) + `"/>`)
	frag.WriteString(`</c:valAx>`)
	return frag.String()
}
