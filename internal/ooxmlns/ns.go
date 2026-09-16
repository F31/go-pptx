// Package ooxmlns 集中 OOXML / OPC 命名空间 URI 常量，供根包与各 internal
// 子包共用，避免同一 URI 在多处重复定义。
//
// 例外：包级关系（package/2006/relationships）与内容类型
// （package/2006/content-types）URI 由 internal/opc 暴露
// （opc.NsRelationships / opc.NsContentTypes），此处不重复。
package ooxmlns

const (
	// PresentationML 是 p: 命名空间。
	PresentationML = "http://schemas.openxmlformats.org/presentationml/2006/main"
	// DrawingML 是 a: 命名空间。
	DrawingML = "http://schemas.openxmlformats.org/drawingml/2006/main"
	// OfficeDocument 是 r: 关系引用命名空间。
	OfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	// ExtendedProps 是 docProps/app.xml 命名空间。
	ExtendedProps = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
	// CoreProps 是 docProps/core.xml 命名空间。
	CoreProps = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	// CustomProps 是 docProps/custom.xml 命名空间。
	CustomProps = "http://schemas.openxmlformats.org/officeDocument/2006/custom-properties"
	// VTypes 是自定义属性值类型命名空间。
	VTypes = "http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"
	// DC 是 Dublin Core 元素命名空间。
	DC = "http://purl.org/dc/elements/1.1/"
	// DCTerms 是 Dublin Core 术语命名空间。
	DCTerms = "http://purl.org/dc/terms/"
	// XSI 是 XML Schema 实例命名空间。
	XSI = "http://www.w3.org/2001/XMLSchema-instance"
	// SpreadsheetML 是 x: 命名空间（图表工作簿）。
	SpreadsheetML = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	// ChartML 是 c: 命名空间。
	ChartML = "http://schemas.openxmlformats.org/drawingml/2006/chart"
)
