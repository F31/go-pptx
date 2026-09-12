package chart

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// ADR-017 第三批归属测试：build / parse / validate / workbook 四组实现搬到
// internal/chart 后，根包只经公共 API 间接执行它们，per-package 覆盖率不
// 归属。本文件直接对本包实现做往返与分支断言，恢复归属并锁死 byte 级
// 确定性（B1 金样比对的前提）。

const (
	testSheetName = "Sheet1"
	testCatAxID   = 100000001
	testValAxID   = 100000002
)

func baseChartData(typ ChartType) ChartData {
	return ChartData{
		Type:       typ,
		Title:      `A&B <营收> «Q»`,
		Categories: []string{"Q1", "Q2", "Q3"},
		Series: []ChartSeries{
			{Name: "系列<1>", Values: []float64{10, 20.5, 15}},
			{Name: "2026", Values: []float64{5, 25, 35}},
		},
	}
}

// TestBuildParseRoundTrip 对三类图表做 build → Index → Parse 往返，
// 断言类型/标题（实体解码）/类别/系列名/系列值全部保真，且生成的
// chartSpace 通过本包 canonical 判定（自洽性）。
func TestBuildParseRoundTrip(t *testing.T) {
	for _, typ := range []ChartType{ChartBar, ChartLine, ChartPie} {
		t.Run(typ.String(), func(t *testing.T) {
			want := baseChartData(typ)
			xml, err := BuildChartSpaceXML(want, testSheetName, testCatAxID, testValAxID)
			if err != nil {
				t.Fatalf("BuildChartSpaceXML: %v", err)
			}
			// 确定性：同输入两次构建字节一致（B1 金样前提）。
			xml2, err := BuildChartSpaceXML(want, testSheetName, testCatAxID, testValAxID)
			if err != nil {
				t.Fatalf("BuildChartSpaceXML(2): %v", err)
			}
			if xml != xml2 {
				t.Fatal("BuildChartSpaceXML not deterministic")
			}

			doc, err := xmlstore.Index([]byte(xml))
			if err != nil {
				t.Fatalf("Index: %v", err)
			}
			root := doc.Root()
			if root.Namespace != nsChartML || root.Local() != "chartSpace" {
				t.Fatalf("root = %q %q, want c:chartSpace", root.Namespace, root.Local())
			}
			if !IsCanonical(doc, root) {
				t.Fatal("generated chartSpace is not canonical")
			}

			got, err := ParseChartSpace(doc, root)
			if err != nil {
				t.Fatalf("ParseChartSpace: %v", err)
			}
			if got.Type != want.Type || got.Title != want.Title {
				t.Errorf("type/title = %v/%q, want %v/%q", got.Type, got.Title, want.Type, want.Title)
			}
			if strings.Join(got.Categories, ",") != strings.Join(want.Categories, ",") {
				t.Errorf("categories = %v, want %v", got.Categories, want.Categories)
			}
			if len(got.Series) != 2 {
				t.Fatalf("series count = %d, want 2", len(got.Series))
			}
			for i := range want.Series {
				if got.Series[i].Name != want.Series[i].Name {
					t.Errorf("series[%d].Name = %q, want %q", i, got.Series[i].Name, want.Series[i].Name)
				}
				if len(got.Series[i].Values) != len(want.Series[i].Values) {
					t.Fatalf("series[%d] values len = %d, want %d", i, len(got.Series[i].Values), len(want.Series[i].Values))
				}
				for j := range want.Series[i].Values {
					if got.Series[i].Values[j] != want.Series[i].Values[j] {
						t.Errorf("series[%d].Values[%d] = %v, want %v", i, j, got.Series[i].Values[j], want.Series[i].Values[j])
					}
				}
			}
		})
	}
}

// TestBuildParseExtras 覆盖 CHART-02 扩展往返：数据标签 / 误差线 / 趋势线 /
// 值轴 logBase 与 Min-Max / 日期类别轴。
func TestBuildParseExtras(t *testing.T) {
	cd := baseChartData(ChartBar)
	cd.DataLabel = &ChartDataLabel{Show: true, Position: "outT"}
	cd.Axes = &ChartAxisOptions{ValueLogBase: 10, Min: NewOptional(0.0), Max: NewOptional(1000.0)}
	cd.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrFixed, Value: 2.5, Direction: "both", NoEndCap: true}
	cd.Series[0].Trendline = &ChartTrendline{
		Type: ChartTrendPolynomial, Order: 3, DisplayEq: true, DisplayRSq: true,
		Name: "拟合", SetIntercept: true, Intercept: 1.5,
	}

	xml, err := BuildChartSpaceXML(cd, testSheetName, testCatAxID, testValAxID)
	if err != nil {
		t.Fatalf("BuildChartSpaceXML: %v", err)
	}
	doc, err := xmlstore.Index([]byte(xml))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	got, err := ParseChartSpace(doc, doc.Root())
	if err != nil {
		t.Fatalf("ParseChartSpace: %v", err)
	}
	if got.DataLabel == nil || !got.DataLabel.Show || got.DataLabel.Position != "outT" {
		t.Errorf("DataLabel = %+v, want Show/outT", got.DataLabel)
	}
	if got.Axes == nil || got.Axes.ValueLogBase != 10 ||
		!got.Axes.Min.Set || got.Axes.Min.Value != 0 ||
		!got.Axes.Max.Set || got.Axes.Max.Value != 1000 {
		t.Errorf("Axes = %+v, want logBase=10 min=0 max=1000", got.Axes)
	}
	eb := got.Series[0].ErrorBars
	if eb == nil || eb.Type != ChartErrFixed || eb.Value != 2.5 || eb.Direction != "both" || !eb.NoEndCap {
		t.Errorf("ErrorBars = %+v", eb)
	}
	tr := got.Series[0].Trendline
	if tr == nil || tr.Type != ChartTrendPolynomial || !tr.DisplayEq || !tr.DisplayRSq ||
		tr.Name != "拟合" || !tr.SetIntercept || tr.Intercept != 1.5 {
		t.Errorf("Trendline = %+v", tr)
	}

	// 日期类别轴：CategoryAsDate=true → 生成 c:dateAx，读回应为 true。
	dateCD := baseChartData(ChartLine)
	dateCD.Axes = &ChartAxisOptions{CategoryAsDate: true}
	xml, err = BuildChartSpaceXML(dateCD, testSheetName, testCatAxID, testValAxID)
	if err != nil {
		t.Fatalf("BuildChartSpaceXML(date): %v", err)
	}
	if !strings.Contains(xml, "<c:dateAx>") {
		t.Errorf("CategoryAsDate did not emit c:dateAx:\n%s", xml)
	}
	doc, err = xmlstore.Index([]byte(xml))
	if err != nil {
		t.Fatalf("Index(date): %v", err)
	}
	got, err = ParseChartSpace(doc, doc.Root())
	if err != nil {
		t.Fatalf("ParseChartSpace(date): %v", err)
	}
	if got.Axes == nil || !got.Axes.CategoryAsDate {
		t.Errorf("Axes.CategoryAsDate = %+v, want true", got.Axes)
	}
}

// TestParseChartSpaceErrors 覆盖缺 c:chart / 缺 c:plotArea / 无受限图表组。
func TestParseChartSpaceErrors(t *testing.T) {
	cases := []struct {
		name string
		xml  string
		want error
	}{
		{"missing-chart", `<c:chartSpace xmlns:c="` + testChartNS + `"/>`, ErrUnsupportedFormat},
		{"missing-plotArea", `<c:chartSpace xmlns:c="` + testChartNS + `"><c:chart/></c:chartSpace>`, ErrUnsupportedFormat},
		{"no-supported-group",
			`<c:chartSpace xmlns:c="` + testChartNS + `"><c:chart><c:plotArea><c:areaChart/></c:plotArea></c:chart></c:chartSpace>`,
			ErrUnsupportedFormat},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := xmlstore.Index([]byte(tc.xml))
			if err != nil {
				t.Fatalf("Index: %v", err)
			}
			_, err = ParseChartSpace(doc, doc.Root())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want errors.Is %v", err, tc.want)
			}
			var be *BuildError
			if !errors.As(err, &be) {
				t.Fatalf("err type = %T, want *BuildError", err)
			}
			if be.Error() == "" || be.Unwrap() != tc.want {
				t.Errorf("BuildError Error()=%q Unwrap()=%v", be.Error(), be.Unwrap())
			}
		})
	}
}

// TestValidateChartDataBranches 覆盖 ValidateChartData 全部分支（含子校验
// 的注解路径），断言统一以 ErrInvalidArgument 暴露。
func TestValidateChartDataBranches(t *testing.T) {
	valid := baseChartData(ChartBar)
	if err := ValidateChartData(valid, "AddChart"); err != nil {
		t.Fatalf("valid rejected: %v", err)
	}

	mutate := func(f func(cd *ChartData)) ChartData {
		cd := baseChartData(ChartBar)
		f(&cd)
		return cd
	}
	cases := []struct {
		name string
		cd   ChartData
	}{
		{"unknown-type", mutate(func(cd *ChartData) { cd.Type = ChartType(99) })},
		{"no-categories", mutate(func(cd *ChartData) { cd.Categories = nil })},
		{"no-series", mutate(func(cd *ChartData) { cd.Series = nil })},
		{"value-count-mismatch", mutate(func(cd *ChartData) { cd.Series[0].Values = []float64{1} })},
		{"nan-value", mutate(func(cd *ChartData) { cd.Series[0].Values[0] = math.NaN() })},
		{"inf-value", mutate(func(cd *ChartData) { cd.Series[0].Values[0] = math.Inf(1) })},
		{"pie-error-bars", func() ChartData {
			cd := baseChartData(ChartPie)
			cd.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrFixed, Value: 1, Direction: "both"}
			return cd
		}()},
		{"pie-trendline", func() ChartData {
			cd := baseChartData(ChartPie)
			cd.Series[0].Trendline = &ChartTrendline{Type: ChartTrendLinear}
			return cd
		}()},
		{"series-errorbars-invalid", mutate(func(cd *ChartData) {
			cd.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrFixed, Value: -1, Direction: "both"}
		})},
		{"series-trendline-invalid", mutate(func(cd *ChartData) {
			cd.Series[0].Trendline = &ChartTrendline{Type: ChartTrendPolynomial, Order: 1}
		})},
		{"datalabel-invalid", mutate(func(cd *ChartData) {
			cd.DataLabel = &ChartDataLabel{Show: true, Position: "diagonal"}
		})},
		{"axes-invalid", mutate(func(cd *ChartData) { cd.Axes = &ChartAxisOptions{ValueLogBase: 1} })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateChartData(tc.cd, "AddChart")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err = %v, want ErrInvalidArgument", err)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err type = %T, want *ValidationError", err)
			}
			if ve.Error() == "" || ve.Unwrap() == nil {
				t.Errorf("ValidationError Error()=%q Unwrap()=%v", ve.Error(), ve.Unwrap())
			}
		})
	}
}

// TestValidateSubValidators 直接覆盖四个子校验器的边界。
func TestValidateSubValidators(t *testing.T) {
	// DataLabel：Show=false 短路；合法位置通过；白名单外拒绝。
	if err := ValidateDataLabel(ChartDataLabel{Show: false, Position: "bogus"}, "op"); err != nil {
		t.Errorf("Show=false must skip validation: %v", err)
	}
	if err := ValidateDataLabel(ChartDataLabel{Show: true, Position: "ctr"}, "op"); err != nil {
		t.Errorf("valid position rejected: %v", err)
	}
	if err := ValidateDataLabel(ChartDataLabel{Show: true, Position: "x"}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("bad position = %v, want ErrInvalidArgument", err)
	}
	// 空 Position 视为默认，通过。
	if err := ValidateDataLabel(ChartDataLabel{Show: true}, "op"); err != nil {
		t.Errorf("empty position rejected: %v", err)
	}

	// ErrorBars：四类型 + 非法类型 + 非法方向 + percentage 越界。
	for _, ok := range []ChartErrorBars{
		{Type: ChartErrStandardDeviation},
		{Type: ChartErrStandardError, Direction: "plus"},
		{Type: ChartErrFixed, Value: 0, Direction: "minus"},
		{Type: ChartErrPercentage, Value: 1000, Direction: "both"},
	} {
		if err := ValidateErrorBars(ok, "op"); err != nil {
			t.Errorf("valid errBars %+v rejected: %v", ok, err)
		}
	}
	if err := ValidateErrorBars(ChartErrorBars{Type: ChartErrorType(9)}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("bad type = %v", err)
	}
	if err := ValidateErrorBars(ChartErrorBars{Type: ChartErrFixed, Value: -0.5}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("negative fixed = %v", err)
	}
	if err := ValidateErrorBars(ChartErrorBars{Type: ChartErrPercentage, Value: 1001}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("percentage > 1000 = %v", err)
	}
	if err := ValidateErrorBars(ChartErrorBars{Type: ChartErrFixed, Value: 1, Direction: "sideways"}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("bad direction = %v", err)
	}

	// Trendline：合法 6 类型；poly 阶数越界；movingAvg 周期不足。
	for _, tl := range []ChartTrendline{
		{Type: ChartTrendLinear},
		{Type: ChartTrendLogarithmic},
		{Type: ChartTrendExponential},
		{Type: ChartTrendPolynomial, Order: 6},
		{Type: ChartTrendPower},
		{Type: ChartTrendMovingAverage, Period: 2},
	} {
		if err := ValidateTrendline(tl, "op"); err != nil {
			t.Errorf("valid trendline %+v rejected: %v", tl, err)
		}
	}
	if err := ValidateTrendline(ChartTrendline{Type: ChartTrendType(42)}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("bad trend type = %v", err)
	}
	if err := ValidateTrendline(ChartTrendline{Type: ChartTrendPolynomial, Order: 7}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("poly order 7 = %v", err)
	}
	if err := ValidateTrendline(ChartTrendline{Type: ChartTrendMovingAverage, Period: 1}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("movingAvg period 1 = %v", err)
	}

	// AxisOptions：logBase 0/2..32 合法；1 与 33 拒绝；Min>Max 拒绝。
	if err := ValidateAxisOptions(ChartAxisOptions{ValueLogBase: 2, Position: "l"}, "op"); err != nil {
		t.Errorf("valid axes rejected: %v", err)
	}
	if err := ValidateAxisOptions(ChartAxisOptions{ValueLogBase: 33}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("logBase 33 = %v", err)
	}
	if err := ValidateAxisOptions(ChartAxisOptions{Position: "up"}, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("bad axPos = %v", err)
	}
	badRange := ChartAxisOptions{Min: NewOptional(10.0), Max: NewOptional(1.0)}
	if err := ValidateAxisOptions(badRange, "op"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("Min>Max = %v", err)
	}
}

// TestFragmentBuilders 直接覆盖 5 个 fragment 构造器（含空/短路分支）。
func TestFragmentBuilders(t *testing.T) {
	if frag, err := BuildChartDataLabelFragment(ChartDataLabel{Show: false}); err != nil || frag != "" {
		t.Errorf("Show=false frag = %q %v, want empty", frag, err)
	}
	frag, err := BuildChartDataLabelFragment(ChartDataLabel{Show: true, Position: "outB"})
	if err != nil {
		t.Fatalf("DataLabel frag: %v", err)
	}
	if !strings.Contains(frag, `<c:dLblPos val="outB"/>`) || !strings.Contains(frag, `<c:showVal val="1"/>`) {
		t.Errorf("DataLabel frag = %q", frag)
	}

	// errBars：默认方向 both；固定值写 c:val；noEndCap 写 1。
	frag, err = BuildErrBarsFragment(ChartErrorBars{Type: ChartErrFixed, Value: 3.25, NoEndCap: true})
	if err != nil {
		t.Fatalf("ErrBars frag: %v", err)
	}
	if !strings.Contains(frag, `<c:errDir val="both"/>`) ||
		!strings.Contains(frag, `<c:errBarType val="fixed"/>`) ||
		!strings.Contains(frag, `<c:val val="3.25"/>`) ||
		!strings.Contains(frag, `<c:noEndCap val="1"/>`) {
		t.Errorf("ErrBars frag = %q", frag)
	}
	// stdDev 不写 val。
	frag, err = BuildErrBarsFragment(ChartErrorBars{Type: ChartErrStandardDeviation, Direction: "minus"})
	if err != nil {
		t.Fatalf("ErrBars(stdDev) frag: %v", err)
	}
	if strings.Contains(frag, "<c:val ") {
		t.Errorf("stdDev must not write c:val: %q", frag)
	}

	// trendline：名称转义 + dispEQ/RSq + intercept。
	frag, err = BuildTrendlineFragment(ChartTrendline{
		Type: ChartTrendLinear, Name: "a&b", DisplayEq: true, DisplayRSq: true,
		SetIntercept: true, Intercept: 2.5,
	})
	if err != nil {
		t.Fatalf("Trendline frag: %v", err)
	}
	if !strings.Contains(frag, `<c:name val="a&amp;b"/>`) ||
		!strings.Contains(frag, `<c:trendlineType val="linear"/>`) ||
		!strings.Contains(frag, `<c:dispEq val="1"/>`) ||
		!strings.Contains(frag, `<c:dispRSqr val="1"/>`) ||
		!strings.Contains(frag, `<c:intercept val="2.5"/>`) {
		t.Errorf("Trendline frag = %q", frag)
	}
	// 无名 → 不写 c:name。
	frag, err = BuildTrendlineFragment(ChartTrendline{Type: ChartTrendMovingAverage, Period: 3})
	if err != nil {
		t.Fatalf("Trendline(no name) frag: %v", err)
	}
	if strings.Contains(frag, "<c:name ") {
		t.Errorf("empty name must not write c:name: %q", frag)
	}

	// 类别轴：默认 catAx + axPos b；CategoryAsDate → dateAx；Position 覆盖。
	if got := BuildCatOrDateAxFragment(ChartAxisOptions{}, 7, 8); !strings.Contains(got, "<c:catAx>") ||
		!strings.Contains(got, `<c:axPos val="b"/>`) ||
		!strings.Contains(got, `<c:axId val="7"/>`) ||
		!strings.Contains(got, `<c:crossAx val="8"/>`) {
		t.Errorf("catAx frag = %q", got)
	}
	if got := BuildCatOrDateAxFragment(ChartAxisOptions{CategoryAsDate: true, Position: "t"}, 7, 8); !strings.Contains(got, "<c:dateAx>") ||
		!strings.Contains(got, `<c:axPos val="t"/>`) {
		t.Errorf("dateAx frag = %q", got)
	}

	// 值轴：logBase 在 orientation 前；Min/Max 按 Set 写入；Position 覆盖。
	got := BuildValAxFragment(ChartAxisOptions{
		ValueLogBase: 10, Min: NewOptional(0.0), Max: NewOptional(100.0), Position: "r",
	}, 7, 8)
	// ChartNumber 用 strconv 'g',-1（HEAD 历史行为）→ 0.0 输出 "0"、100.0 输出 "100"。
	if !strings.Contains(got, `<c:logBase val="10"/>`) ||
		!strings.Contains(got, `<c:min val="0"/>`) ||
		!strings.Contains(got, `<c:max val="100"/>`) ||
		!strings.Contains(got, `<c:axPos val="r"/>`) {
		t.Errorf("valAx frag = %q", got)
	}
	if i, j := strings.Index(got, "<c:logBase"), strings.Index(got, "<c:orientation"); i < 0 || j < 0 || i > j {
		t.Errorf("logBase must precede orientation: %q", got)
	}
	// 零值 → 不写 logBase/min/max。
	if got := BuildValAxFragment(ChartAxisOptions{}, 7, 8); strings.Contains(got, "logBase") ||
		strings.Contains(got, "<c:min ") || strings.Contains(got, "<c:max ") {
		t.Errorf("zero axes must omit extensions: %q", got)
	}
}

// TestBuildChartWorkbookBytes 覆盖 xlsx 包装配：条目顺序、内容、确定性、
// 关系命名空间声明（xmlns:r 缺失会使 r:id 成为未定义前缀）。
func TestBuildChartWorkbookBytes(t *testing.T) {
	book := ChartDataBook{
		SheetName:  testSheetName,
		Categories: []string{"Q1", "Q2"},
		Series: []ChartSeries{
			{Name: "A&B", Values: []float64{1, 2.5}},
			{Name: "B", Values: []float64{3, 4}},
		},
	}
	out, err := BuildChartWorkbookXML(book, testSheetName)
	if err != nil {
		t.Fatalf("BuildChartWorkbookXML: %v", err)
	}
	out2, err := BuildChartWorkbookXML(book, testSheetName)
	if err != nil {
		t.Fatalf("BuildChartWorkbookXML(2): %v", err)
	}
	if !bytes.Equal(out, out2) {
		t.Fatal("workbook build not deterministic")
	}

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	wantOrder := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"xl/worksheets/sheet1.xml",
	}
	if len(zr.File) != len(wantOrder) {
		t.Fatalf("entries = %d, want %d", len(zr.File), len(wantOrder))
	}
	for i, want := range wantOrder {
		if zr.File[i].Name != want {
			t.Errorf("entry[%d] = %q, want %q", i, zr.File[i].Name, want)
		}
	}
	read := func(name string) string {
		t.Helper()
		for _, f := range zr.File {
			if f.Name != name {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return string(b)
		}
		t.Fatalf("entry %s not found", name)
		return ""
	}
	// workbook.xml 必须声明 r 前缀（<sheet r:id="..."/> 依赖它）。
	wbXML := read("xl/workbook.xml")
	if !strings.Contains(wbXML, `xmlns:r="`+nsOfficeDocument+`"`) {
		t.Errorf("xl/workbook.xml missing xmlns:r declaration:\n%s", wbXML)
	}
	if !strings.Contains(wbXML, `<sheet name="`+testSheetName+`" sheetId="1" r:id="rId1"/>`) {
		t.Errorf("xl/workbook.xml sheet entry unexpected:\n%s", wbXML)
	}
	sheet := read("xl/worksheets/sheet1.xml")
	for _, want := range []string{
		`<c r="B1" t="inlineStr"><is><t xml:space="preserve">A&amp;B</t></is></c>`,
		`<c r="C1" t="inlineStr"><is><t xml:space="preserve">B</t></is></c>`,
		`<c r="A2" t="inlineStr"><is><t xml:space="preserve">Q1</t></is></c>`,
		`<c r="B2"><v>1</v></c>`,
		`<c r="C2"><v>3</v></c>`,
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("sheet1.xml missing %s:\n%s", want, sheet)
		}
	}
}

// TestXmlUnescapeEntities 覆盖命名实体、十进制/十六进制数字引用与未知实体保留。
func TestXmlUnescapeEntities(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"a&amp;b", "a&b"},
		{"&lt;x&gt;", "<x>"},
		{"&quot;q&quot;", `"q"`},
		{"&apos;", "'"},
		{"&#65;&#66;", "AB"},
		{"&#x4E2D;", "中"},
		{"&#X41;", "A"},
		{"&unknown;", "&unknown;"}, // 未知命名实体原样保留
		{"trailing&", "trailing&"}, // 无分号的 & 原样保留
		{"&#xZZ;", "&#xZZ;"},       // 非法十六进制原样保留
		{"&#;", "&#;"},             // 空数字引用原样保留
	}
	for _, tc := range cases {
		if got := xmlUnescape(tc.in); got != tc.want {
			t.Errorf("xmlUnescape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
