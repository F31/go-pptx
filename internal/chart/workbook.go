package chart

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 命名空间常量（与根包同步）。const 不能跨包 alias，必须在内部维护一份。
//
// 修改时务必同步根包 internal/opc 与 template.go；任何不一致会破坏 xlsx
// 字节（B1 黄金语料哈希 regression）。
const (
	nsSpreadsheetML = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsPkgRels       = "http://schemas.openxmlformats.org/package/2006/relationships"
	nsContentTypes  = "application/vnd.openxmlformats-package.relationships+xml"
)

// xlsx 关系与内容类型常量（与 internal/opc.RelOfficeDocument 等同步）。
const (
	xlsxRelTypeWorksheet     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"
	xlsxRelTypeOfficeDoc     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	xlsxContentTypeWorkbookM = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	xlsxContentTypeWorksheet = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
)

// BuildChartWorkbookXML 组装 xlsx 包（内存 ZIP，条目固定顺序）。
func BuildChartWorkbookXML(book ChartDataBook, sheetName string) ([]byte, error) {
	sheetXML, err := BuildChartSheetXML(book, sheetName)
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
			`<Override PartName="/xl/workbook.xml" ContentType="` + xlsxContentTypeWorkbookM + `"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="` + xlsxContentTypeWorksheet + `"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rId1" Type="` + xlsxRelTypeOfficeDoc + `" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		// 注意：<sheet r:id="..."/> 依赖 r 前缀，xmlns:r 必须声明——缺失会
		// 产出未定义前缀的非法 XML，且改变 xlsx 字节（B1 金样比对失败）。
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<workbook xmlns="` + nsSpreadsheetML + `" xmlns:r="` + nsOfficeDocument + `">` +
			`<sheets><sheet name="` + sheetName + `" sheetId="1" r:id="rId1"/></sheets>` +
			`</workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rId1" Type="` + xlsxRelTypeWorksheet + `" Target="worksheets/sheet1.xml"/>` +
			`</Relationships>`},
		{"xl/worksheets/sheet1.xml", sheetXML},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range parts {
		f, err := zw.Create(part.name)
		if err != nil {
			return nil, &BuildError{Op: "DefaultWorkbookBuilder.Build", Message: err.Error(), Sentinel: ErrMalformedPackage}
		}
		if _, err := f.Write([]byte(part.content)); err != nil {
			return nil, &BuildError{Op: "DefaultWorkbookBuilder.Build", Message: err.Error(), Sentinel: ErrMalformedPackage}
		}
	}
	if err := zw.Close(); err != nil {
		return nil, &BuildError{Op: "DefaultWorkbookBuilder.Build", Message: err.Error(), Sentinel: ErrMalformedPackage}
	}
	return buf.Bytes(), nil
}

// BuildChartSheetXML 生成数据工作表（行 1 系列名，行 2.. 类别 + 数值）。
func BuildChartSheetXML(book ChartDataBook, sheetName string) (string, error) {
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
			return "", &BuildError{Op: "DefaultWorkbookBuilder.Build", Message: "series name: " + err.Error(), Sentinel: ErrInvalidArgument}
		}
		sb.WriteString(`<c r="` + WorkbookColumn(i+2) + `1" t="inlineStr"><is><t xml:space="preserve">` + name + `</t></is></c>`)
	}
	sb.WriteString(`</row>`)
	// 行 2..n+1：A = 类别，B.. = 数值。
	for r, cat := range book.Categories {
		row := r + 2
		catEsc, err := esc(cat)
		if err != nil {
			return "", &BuildError{Op: "DefaultWorkbookBuilder.Build", Message: "category: " + err.Error(), Sentinel: ErrInvalidArgument}
		}
		sb.WriteString(`<row r="` + strconv.Itoa(row) + `">`)
		sb.WriteString(`<c r="A` + strconv.Itoa(row) + `" t="inlineStr"><is><t xml:space="preserve">` + catEsc + `</t></is></c>`)
		for i, ser := range book.Series {
			if r < len(ser.Values) {
				sb.WriteString(`<c r="` + WorkbookColumn(i+2) + strconv.Itoa(row) + `"><v>` + ChartNumber(ser.Values[r]) + `</v></c>`)
			}
		}
		sb.WriteString(`</row>`)
	}
	sb.WriteString(`</sheetData></worksheet>`)
	return sb.String(), nil
}
