package pptx

import (
	"strings"

	chartinternal "github.com/F31/go-pptx/internal/chart"
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
//
// Experimental: 这是适配层接口，1.0 内可能新增方法或重命名；首选 API
// 路径（受限默认实现）请使用 DefaultWorkbookBuilder。1.0 后将依据
// 真实集成经验收敛为稳定签名——届时本类型冻结，新需求走扩展接口。
type ChartWorkbookBuilder interface {
	Build(book ChartDataBook) ([]byte, error)
}

// ChartDataBook 是传递给工作簿适配器的数据快照。
//
// Experimental: 1.0 内可能新增字段（多 sheet / 公式 / 数据透视等）。
// 当前形态适配 DefaultWorkbookBuilder 的最小 xlsx 输出。ChartSeries 数组
// 内的顺序与 Categories 对应关系视为 v1.0 契约的一部分。
//
// ADR-017 第三批：定义搬到 internal/chart；根包用同名 type alias 引用，
// 保证 DefaultWorkbookBuilder.Build(book ChartDataBook) 公共 API 表面零变化。
type ChartDataBook = chartinternal.ChartDataBook

// DefaultWorkbookBuilder 是默认工作簿适配器：纯标准库生成最小 xlsx。
//
// 生成内容：[Content_Types].xml、_rels/.rels、xl/workbook.xml、
// xl/_rels/workbook.xml.rels、xl/worksheets/sheet1.xml（inlineStr 文本
// 单元格 + 数值单元格），无宏、无公式、无第三方素材。
//
// Experimental: 受限最小实现，1.0 内可能扩展（公式、命名范围、自定义
// sheet 名）。SheetName 当前硬性约束为 "Sheet1"，与图表 c:f 引用固定。
type DefaultWorkbookBuilder struct{}

// Build 生成与 book 一致的最小 xlsx 包字节（确定性输出）。
func (DefaultWorkbookBuilder) Build(book ChartDataBook) ([]byte, error) {
	sheet := book.SheetName
	if strings.TrimSpace(sheet) == "" {
		sheet = chartSheetName
	}
	if sheet != chartSheetName {
		return nil, &OperationError{
			Op:      "DefaultWorkbookBuilder.Build",
			Message: "sheet name must be " + chartSheetName + " (chart references are fixed to it)",
			Err:     ErrInvalidArgument,
		}
	}
	out, err := chartinternal.BuildChartWorkbookXML(book, chartSheetName)
	if err != nil {
		if be, ok := err.(*chartinternal.BuildError); ok {
			return nil, &OperationError{Op: be.Op, Message: be.Message, Err: be.Sentinel}
		}
		return nil, Annotate(err, "DefaultWorkbookBuilder.Build")
	}
	return out, nil
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
