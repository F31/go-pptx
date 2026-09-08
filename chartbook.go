package pptx

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 CHART-01 的嵌入工作簿适配（方案 §9.2）：
//
//   - ChartWorkbookBuilder 是可替换适配接口：核心不承担完整 Excel 库
//     开发，默认实现以纯标准库生成与图表数据一致的最小 xlsx；
//   - 工作簿布局（与 buildChartSpaceXML 的引用约定配套，双侧共享
//     chartSheetName / chartWorkbookColumn）：
//
//     行 1：A1 空，B1.. = 系列名（Sheet1!$B$1、$C$1 …）
//     行 2..n+1：A 列 = 类别（Sheet1!$A$2:$A$(n+1)），B.. 列 = 数值
//
//   - 单元格用 inlineStr 表达文本（不依赖 sharedStrings），数值单元格
//     直接 <v>；输出为确定性字节（条目固定顺序，同一数据同一字节）。

// nsSpreadsheetML 是 xlsx 主命名空间（工作簿生成用）。
const nsSpreadsheetML = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

// ChartWorkbookBuilder 是图表嵌入工作簿的可替换适配接口（方案 §9.2）。
//
// Build 返回完整 xlsx 包字节，必须与 book 中的数据保持一致（客户端
// "编辑数据" 将打开该工作簿）。替换实现由调用方保证一致性；返回错误
// 时 AddChart/SetData 整体失败，不产生部分写入。
type ChartWorkbookBuilder interface {
	Build(book ChartDataBook) ([]byte, error)
}

// ChartDataBook 是传递给工作簿适配器的数据快照。
type ChartDataBook struct {
	// SheetName 是数据工作表名（默认 "Sheet1"）；空串按默认处理。
	SheetName string
	// Categories 是类别标签（A 列，行 2..n+1）。
	Categories []string
	// Series 是数据系列（行 1 系列名，行 2..n+1 数值）。
	Series []ChartSeries
}

// DefaultWorkbookBuilder 是默认工作簿适配器：纯标准库生成最小 xlsx。
//
// 生成内容：[Content_Types].xml、_rels/.rels、xl/workbook.xml、
// xl/_rels/workbook.xml.rels、xl/worksheets/sheet1.xml（inlineStr 文本
// 单元格 + 数值单元格），无宏、无公式、无第三方素材。
type DefaultWorkbookBuilder struct{}

// Build 生成与 book 一致的最小 xlsx 包字节（确定性输出）。
func (DefaultWorkbookBuilder) Build(book ChartDataBook) ([]byte, error) {
	sheet := book.SheetName
	if strings.TrimSpace(sheet) == "" {
		sheet = chartSheetName
	}
	if sheet != chartSheetName {
		// 非默认表名会破坏与图表引用（Sheet1!$..）的一致性约定。
		return nil, &OperationError{
			Op:      "DefaultWorkbookBuilder.Build",
			Message: "sheet name must be " + chartSheetName + " (chart references are fixed to it)",
			Err:     ErrInvalidArgument,
		}
	}
	return buildChartWorkbookXML(book)
}

// buildChartWorkbookXML 组装 xlsx 包（内存 ZIP，条目固定顺序）。
func buildChartWorkbookXML(book ChartDataBook) ([]byte, error) {
	sheetXML, err := buildChartSheetXML(book)
	if err != nil {
		return nil, err
	}
	parts := []struct {
		name    string
		content string
	}{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<Types xmlns="` + nsContentTypes + `">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rId1" Type="` + opc.RelOfficeDocument + `" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<workbook xmlns="` + nsSpreadsheetML + `" xmlns:r="` + nsOfficeDocument + `">` +
			`<sheets><sheet name="` + chartSheetName + `" sheetId="1" r:id="rId1"/></sheets>` +
			`</workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rId1" Type="` + opc.RelTypePrefix + `worksheet" Target="worksheets/sheet1.xml"/>` +
			`</Relationships>`},
		{"xl/worksheets/sheet1.xml", sheetXML},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range parts {
		f, err := zw.Create(part.name)
		if err != nil {
			return nil, &OperationError{Op: "DefaultWorkbookBuilder.Build", Message: err.Error(), Err: ErrMalformedPackage}
		}
		if _, err := f.Write([]byte(part.content)); err != nil {
			return nil, &OperationError{Op: "DefaultWorkbookBuilder.Build", Message: err.Error(), Err: ErrMalformedPackage}
		}
	}
	if err := zw.Close(); err != nil {
		return nil, &OperationError{Op: "DefaultWorkbookBuilder.Build", Message: err.Error(), Err: ErrMalformedPackage}
	}
	return buf.Bytes(), nil
}

// buildChartSheetXML 生成数据工作表（行 1 系列名，行 2.. 类别 + 数值）。
func buildChartSheetXML(book ChartDataBook) (string, error) {
	esc := func(s string) (string, error) {
		return xmlstore.EscapeText(s)
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n")
	sb.WriteString(`<worksheet xmlns="` + nsSpreadsheetML + `"><sheetData>`)
	// 行 1：B1.. = 系列名（A1 留空，省略单元格）。
	sb.WriteString(`<row r="1">`)
	for i, ser := range book.Series {
		name, err := esc(ser.Name)
		if err != nil {
			return "", &OperationError{Op: "DefaultWorkbookBuilder.Build", Message: "series name: " + err.Error(), Err: ErrInvalidArgument}
		}
		sb.WriteString(`<c r="` + chartWorkbookColumn(i+2) + `1" t="inlineStr"><is><t xml:space="preserve">` + name + `</t></is></c>`)
	}
	sb.WriteString(`</row>`)
	// 行 2..n+1：A = 类别，B.. = 数值。
	for r, cat := range book.Categories {
		row := r + 2
		catEsc, err := esc(cat)
		if err != nil {
			return "", &OperationError{Op: "DefaultWorkbookBuilder.Build", Message: "category: " + err.Error(), Err: ErrInvalidArgument}
		}
		sb.WriteString(`<row r="` + strconv.Itoa(row) + `">`)
		sb.WriteString(`<c r="A` + strconv.Itoa(row) + `" t="inlineStr"><is><t xml:space="preserve">` + catEsc + `</t></is></c>`)
		for i, ser := range book.Series {
			if r < len(ser.Values) {
				sb.WriteString(`<c r="` + chartWorkbookColumn(i+2) + strconv.Itoa(row) + `"><v>` + chartNumber(ser.Values[r]) + `</v></c>`)
			}
		}
		sb.WriteString(`</row>`)
	}
	sb.WriteString(`</sheetData></worksheet>`)
	return sb.String(), nil
}

// chartWorkbookColumn 把 1 基列号转为列字母（1→A，2→B，27→AA）。
func chartWorkbookColumn(n int) string {
	if n < 1 {
		return ""
	}
	var sb []byte
	for n > 0 {
		n-- // 1 基 → 0 基
		sb = append([]byte{byte('A' + n%26)}, sb...)
		n /= 26
	}
	return string(sb)
}

// chartNumber 输出数值的规范十进制文本（供图表缓存与工作簿共用，
// 保证两侧字节一致；非有限值由上游校验拒绝）。
func chartNumber(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// chartWorkbookBytes 经当前适配器生成工作簿字节。
func (p *Presentation) chartWorkbookBytes(book ChartDataBook) ([]byte, error) {
	b := p.chartWorkbookBuilder
	if b == nil {
		b = DefaultWorkbookBuilder{}
	}
	out, err := b.Build(book)
	if err != nil {
		return nil, Annotate(err, "chart workbook")
	}
	if len(out) == 0 {
		return nil, &OperationError{
			Op: "chart workbook", Message: "workbook builder returned empty package", Err: ErrInvalidArgument,
		}
	}
	return out, nil
}

// SetChartWorkbookBuilder 替换图表嵌入工作簿适配器（nil 恢复默认）。
// 替换实现必须保证 Build 输出与数据一致（见 ChartWorkbookBuilder）。
func (p *Presentation) SetChartWorkbookBuilder(b ChartWorkbookBuilder) error {
	if p.closed {
		return Annotate(ErrClosed, "Presentation.SetChartWorkbookBuilder")
	}
	p.chartWorkbookBuilder = b
	return nil
}
