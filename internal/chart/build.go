package chart

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 命名空间常量（与根包 template.go 同步）。const 不能跨包 alias，必须
// 在 internal/chart 内部维护一份。修改根包常量值时务必同步此处；任何
// 不一致都会破坏 chart Part 字面输出（B1 黄金语料哈希 regression）。
const (
	nsChartML        = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	nsDrawingML      = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// BuildChartSpaceXML 生成规范布局的 c:chartSpace（受限三类）。
//
// 参数：
//   - sheetName：图表引用对应工作表名（默认 "Sheet1"；与 ChartSpec / Workbook 共享）；
//   - catAxID / valAxID：图表组内部作用域的轴 ID（与 ChartAxisOptions 引用配套）。
//
// 错误一律包为 *BuildError，根包 wrapper 负责转换为 *pptx.OperationError。
// ADR-017 第三批：实现从根包 chart.go 搬入；调用方零修改（wrapper 保持同名）。
func BuildChartSpaceXML(cd ChartData, sheetName string, catAxID, valAxID int) (string, error) {
	esc := func(s string) (string, error) { return xmlstore.EscapeText(s) }
	n := len(cd.Categories)
	lastRow := n + 1
	catRef := sheetName + `!$A$2:$A$` + strconv.Itoa(lastRow)

	var sb strings.Builder
	sb.WriteString(`<c:chartSpace xmlns:c="` + nsChartML + `" xmlns:a="` + nsDrawingML +
		`" xmlns:r="` + nsOfficeDocument + `">`)
	sb.WriteString(`<c:chart>`)
	if cd.Title != "" {
		t, err := esc(cd.Title)
		if err != nil {
			return "", &BuildError{Op: "chart", Message: "title: " + err.Error(), Sentinel: ErrInvalidArgument}
		}
		sb.WriteString(`<c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:r>` +
			`<a:rPr lang="en-US"/><a:t>` + t + `</a:t></a:r></a:p></c:rich></c:tx>` +
			`<c:overlay val="0"/></c:title>`)
	}
	sb.WriteString(`<c:autoTitleDeleted val="0"/>`)
	sb.WriteString(`<c:plotArea><c:layout/>`)

	writeSer := func() error {
		for i, ser := range cd.Series {
			col := WorkbookColumn(i + 2)
			nameRef := sheetName + `!$` + col + `$1`
			valRef := sheetName + `!$` + col + `$2:$` + col + `$` + strconv.Itoa(lastRow)
			sb.WriteString(`<c:ser><c:idx val="` + strconv.Itoa(i) + `"/><c:order val="` + strconv.Itoa(i) + `"/>`)
			name, err := esc(ser.Name)
			if err != nil {
				return &BuildError{Op: "chart", Message: "series name: " + err.Error(), Sentinel: ErrInvalidArgument}
			}
			sb.WriteString(`<c:tx><c:strRef><c:f>` + nameRef + `</c:f>` +
				`<c:strCache><c:ptCount val="1"/><c:pt idx="0"><c:v>` + name + `</c:v></c:pt></c:strCache>` +
				`</c:strRef></c:tx>`)
			// CHART-02 系列级扩展：trendline + errBars，写在 c:tx 与 c:cat 之间。
			if ser.Trendline != nil {
				frag, err := BuildTrendlineFragment(*ser.Trendline)
				if err != nil {
					return annotateChart(err, "series "+strconv.Itoa(i)+".Trendline")
				}
				sb.WriteString(frag)
			}
			if ser.ErrorBars != nil {
				frag, err := BuildErrBarsFragment(*ser.ErrorBars)
				if err != nil {
					return annotateChart(err, "series "+strconv.Itoa(i)+".ErrorBars")
				}
				sb.WriteString(frag)
			}
			sb.WriteString(`<c:cat><c:strRef><c:f>` + catRef + `</c:f>` +
				`<c:strCache><c:ptCount val="` + strconv.Itoa(n) + `"/>`)
			for j, cat := range cd.Categories {
				cv, err := esc(cat)
				if err != nil {
					return &BuildError{Op: "chart", Message: "category: " + err.Error(), Sentinel: ErrInvalidArgument}
				}
				sb.WriteString(`<c:pt idx="` + strconv.Itoa(j) + `"><c:v>` + cv + `</c:v></c:pt>`)
			}
			sb.WriteString(`</c:strCache></c:strRef></c:cat>`)
			sb.WriteString(`<c:val><c:numRef><c:f>` + valRef + `</c:f>` +
				`<c:numCache><c:formatCode>General</c:formatCode>` +
				`<c:ptCount val="` + strconv.Itoa(n) + `"/>`)
			for j, v := range ser.Values {
				sb.WriteString(`<c:pt idx="` + strconv.Itoa(j) + `"><c:v>` + ChartNumber(v) + `</c:v></c:pt>`)
			}
			sb.WriteString(`</c:numCache></c:numRef></c:val>`)
			if cd.Type == ChartLine {
				sb.WriteString(`<c:smooth val="0"/>`)
			}
			sb.WriteString(`</c:ser>`)
		}
		return nil
	}

	dLblsFragment := ""
	if cd.DataLabel != nil && cd.DataLabel.Show {
		frag, err := BuildChartDataLabelFragment(*cd.DataLabel)
		if err != nil {
			return "", annotateChart(err, "ChartDataLabel")
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
			`<c:axId val="` + strconv.Itoa(catAxID) + `"/>` +
			`<c:axId val="` + strconv.Itoa(valAxID) + `"/></c:barChart>`)
	case ChartLine:
		sb.WriteString(`<c:lineChart><c:grouping val="standard"/><c:varyColors val="0"/>`)
		if err := writeSer(); err != nil {
			return "", err
		}
		if dLblsFragment != "" {
			sb.WriteString(dLblsFragment)
		}
		sb.WriteString(`<c:marker val="1"/>` +
			`<c:axId val="` + strconv.Itoa(catAxID) + `"/>` +
			`<c:axId val="` + strconv.Itoa(valAxID) + `"/></c:lineChart>`)
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
		axOpts := ChartAxisOptions{}
		if cd.Axes != nil {
			axOpts = *cd.Axes
		}
		sb.WriteString(BuildCatOrDateAxFragment(axOpts, catAxID, valAxID))
		sb.WriteString(BuildValAxFragment(axOpts, valAxID, catAxID))
	}
	sb.WriteString(`</c:plotArea>`)
	sb.WriteString(`<c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/>`)
	sb.WriteString(`</c:chart></c:chartSpace>`)
	return sb.String(), nil
}
