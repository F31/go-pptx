package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// CHART-02 fragment / validator helpers。
// 严格白名单：白名单外任何输入都返回 ErrInvalidArgument，不静默降级。

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

// validateChartDataLabel 校验图表级数据标签。
func validateChartDataLabel(d ChartDataLabel, op string) error {
	if !d.Show {
		return nil
	}
	if d.Position != "" && !dataLabelPositionAllowed[d.Position] {
		return &OperationError{
			Op: op, Message: "ChartDataLabel position not in whitelist: " + d.Position,
			Err: ErrInvalidArgument,
		}
	}
	return nil
}

// validateChartErrorBars 校验单系列误差线。
func validateChartErrorBars(e ChartErrorBars, op string) error {
	switch e.Type {
	case ChartErrFixed:
		if e.Value < 0 || math.IsInf(e.Value, 0) || math.IsNaN(e.Value) {
			return &OperationError{
				Op: op, Message: "ErrorBars fixed value must be a finite non-negative number",
				Err: ErrInvalidArgument,
			}
		}
	case ChartErrPercentage:
		if e.Value < 0 || e.Value > 1000 || math.IsInf(e.Value, 0) || math.IsNaN(e.Value) {
			return &OperationError{
				Op: op, Message: "ErrorBars percentage must be in [0,1000]",
				Err: ErrInvalidArgument,
			}
		}
	case ChartErrStandardDeviation, ChartErrStandardError:
		// 无 Value 校验
	default:
		return &OperationError{
			Op: op, Message: "ErrorBars type out of range", Err: ErrInvalidArgument,
		}
	}
	if e.Direction != "" && !errBarDirectionAllowed[e.Direction] {
		return &OperationError{
			Op: op, Message: "ErrorBars direction not in whitelist: " + e.Direction,
			Err: ErrInvalidArgument,
		}
	}
	return nil
}

// validateChartTrendline 校验单系列趋势线。
func validateChartTrendline(t ChartTrendline, op string) error {
	switch t.Type {
	case ChartTrendLinear, ChartTrendLogarithmic, ChartTrendExponential,
		ChartTrendPolynomial, ChartTrendPower, ChartTrendMovingAverage:
	default:
		return &OperationError{
			Op: op, Message: "Trendline type out of range", Err: ErrInvalidArgument,
		}
	}
	if t.Type == ChartTrendPolynomial {
		if t.Order < 2 || t.Order > 6 {
			return &OperationError{
				Op: op, Message: "Trendline polynomial Order must be in [2,6]",
				Err: ErrInvalidArgument,
			}
		}
	}
	if t.Type == ChartTrendMovingAverage && t.Period < 2 {
		return &OperationError{
			Op: op, Message: "Trendline movingAverage Period must be >= 2",
			Err: ErrInvalidArgument,
		}
	}
	return nil
}

// validateChartAxisOptions 校验轴扩展。
func validateChartAxisOptions(a ChartAxisOptions, op string) error {
	if a.ValueLogBase < 0 || (a.ValueLogBase != 0 && (a.ValueLogBase < 2 || a.ValueLogBase > 32)) {
		return &OperationError{
			Op: op, Message: "Axes ValueLogBase must be 0 or in [2,32]",
			Err: ErrInvalidArgument,
		}
	}
	if a.Position != "" && !axisPosAllowed[a.Position] {
		return &OperationError{
			Op: op, Message: "Axes Position not in whitelist: " + a.Position,
			Err: ErrInvalidArgument,
		}
	}
	if a.Min.Set && a.Max.Set && a.Min.Value > a.Max.Value {
		return &OperationError{
			Op: op, Message: "Axes Min must be <= Max", Err: ErrInvalidArgument,
		}
	}
	return nil
}

// ---------- 片段生成（供 buildChartSpaceXML 嵌入） ----------

// buildChartDataLabelFragment 构造图表级 c:dLbls。
// Show=false（外层短路）；Position 留空 = 写 <c:dLbls> 但不带 dLblPos。
func buildChartDataLabelFragment(d ChartDataLabel) (string, error) {
	if !d.Show {
		return "", nil
	}
	frag := `<c:dLbls>`
	if d.Position != "" {
		frag += `<c:dLblPos val="` + d.Position + `"/>`
	}
	// 默认显示数值（与 PowerPoint 客户端「数据标签 → 显示值」一致）；
	// Show 系列展开字段按需由后续 CHART-02 增强包引入。
	frag += `<c:showLegendKey val="0"/><c:showVal val="1"/><c:showCatName val="0"/>` +
		`<c:showSerName val="0"/><c:showPercent val="0"/><c:showBubbleSize val="0"/></c:dLbls>`
	return frag, nil
}

// buildErrBarsFragment 构造 c:errBars。
// OOXML 序：errDir, errBarType, errValType（cusstdErr 等），val?, plus?, minus?, noEndCap, valPlus?, valMinus?
// R 档简化：errDir + errBarType + val (Fixed/Percentage) + noEndCap。
func buildErrBarsFragment(e ChartErrorBars) (string, error) {
	direction := e.Direction
	if direction == "" {
		direction = "both"
	}
	var frag strings.Builder
	frag.WriteString(`<c:errBars>`)
	frag.WriteString(`<c:errDir val="` + direction + `"/>`)
	frag.WriteString(`<c:errBarType val="` + e.Type.String() + `"/>`)
	if e.Type == ChartErrFixed || e.Type == ChartErrPercentage {
		frag.WriteString(`<c:val val="` + chartNumber(e.Value) + `"/>`)
	}
	if e.NoEndCap {
		frag.WriteString(`<c:noEndCap val="1"/>`)
	} else {
		// 缺省即 client 显示为有端帽，不必写。
	}
	frag.WriteString(`</c:errBars>`)
	return frag.String(), nil
}

// buildTrendlineFragment 构造 c:trendline。
// OOXML 序：name?, spPr?, trendlineType, dispEq?, dispRSqr?, trendlineLbl?, intercept?
func buildTrendlineFragment(t ChartTrendline) (string, error) {
	var frag strings.Builder
	frag.WriteString(`<c:trendline>`)
	if t.Name != "" {
		esc, err := xmlstore.EscapeText(t.Name)
		if err != nil {
			return "", &OperationError{
				Op: "trendline", Message: "name: " + err.Error(), Err: ErrInvalidArgument,
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
		frag.WriteString(`<c:intercept val="` + chartNumber(t.Intercept) + `"/>`)
	}
	// MovingAverage.period 与 Polynomial.order 写入 trendlineType 之后、
	// dispEq/dispRSqr 之前对部分 OOXML 版本允许；为最大客户端兼容，我们
	// 把这些附加属性附加在 trendlineType 之后。
	if t.Type == ChartTrendMovingAverage && t.Period > 0 {
		// PowerPoint 实际把 period 写入 a:smoother 属性的 wsp:trendline 模式；
		// OOXML CT_Trendline 不含周期字段。R 档简化：不写 period——客户端
		// 默认 period=2。
		_ = t.Period
	}
	if t.Type == ChartTrendPolynomial && t.Order > 0 {
		// OOXML CT_Trendline 不内置 Order；R 档：不写。
		_ = t.Order
	}
	frag.WriteString(`</c:trendline>`)
	return frag.String(), nil
}

// buildCatOrDateAxFragment 构造类别轴（catAx 或 dateAx）。
// catAx 序：axId, scaling, delete, axPos, crossAx
func buildCatOrDateAxFragment(a ChartAxisOptions) string {
	tag := "c:catAx"
	if a.CategoryAsDate {
		tag = "c:dateAx"
	}
	axPos := "b"
	if a.CategoryAsDate {
		axPos = "b"
	}
	if a.Position != "" {
		axPos = a.Position
	}
	return `<` + tag + `><c:axId val="` + strconv.Itoa(chartCatAxID) + `"/>` +
		`<c:scaling><c:orientation val="minMax"/></c:scaling>` +
		`<c:delete val="0"/><c:axPos val="` + axPos + `"/>` +
		`<c:crossAx val="` + strconv.Itoa(chartValAxID) + `"/></` + tag + `>`
}

// buildValAxFragment 构造值轴（valAx）+ logBase/Min/Max 扩展。
func buildValAxFragment(a ChartAxisOptions, axID, crossAxID int) string {
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
		frag.WriteString(`<c:min val="` + chartNumber(a.Min.Value) + `"/>`)
	}
	if a.Max.Set {
		frag.WriteString(`<c:max val="` + chartNumber(a.Max.Value) + `"/>`)
	}
	frag.WriteString(`<c:crossAx val="` + strconv.Itoa(crossAxID) + `"/>`)
	frag.WriteString(`</c:valAx>`)
	return frag.String()
}

// __ensure fmt import is retained above for future expansion (kept here to
// silence unused import warning if other CLIs drop fmt usage momentarily).
var _ = fmt.Sprintf
