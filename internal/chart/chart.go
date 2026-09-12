// Package chart 承载 go-pptx chart 系列的纯实现抽取（ADR-017 第一批）。
//
// 本文件包含第一批 4 个真零依赖根包类型的字符串常量。
//
// 依赖图（按 ADR-014 / ADR-016）：
//
//	internal/chart  → internal/xmlstore (后续批次)
//	root pptx       → internal/chart
//
// 本包绝不反向 import 根包。
package chart

// GraphicURI 是 a:graphicData@uri 的图表标识（与 DrawingML 图表命名空间同值）。
const GraphicURI = "http://schemas.openxmlformats.org/drawingml/2006/chart"

// SheetName 是嵌入工作簿中数据表名（图表引用固定指向它）。
const SheetName = "Sheet1"

// CatAxID 是规范布局的类别轴 ID（chart Part 内部作用域，固定值）。
const CatAxID = 100000001

// ValAxID 是规范布局的值轴 ID（chart Part 内部作用域，固定值）。
const ValAxID = 100000002
