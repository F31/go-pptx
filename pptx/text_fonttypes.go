package pptx

import (
	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/document/style"
)

import ()

// Optional[T] 是"未设置/显式设置"的双态包装（方案 §5.2）。
//
// ADR-017 第二批：实现搬到 internal/chart.Optional[T]；本类型为 alias，
// 公共 API 表面零变化（NewOptional / 字段集自动共享）。
//
// Set=false 表示无本地覆盖（继承）；Set=true 时必须按 Value 理解——
// 例如 FontStyle.Bold 的 Set=true, Value=false 表示显式取消粗体，而非
// "未设置"。读取状态与写入 patch 语义见 FontStyle 文档。
type Optional[T any] = model.Optional[T]

// NewOptional 构造显式设置值。
func NewOptional[T any](v T) Optional[T] { return model.NewOptional(v) }

// FontSize 是字号（单位 pt；XML 存储为百分之一 pt 的 a:sz val，
// 单位集中换算属 GEOM-01，本类型先承担强类型职责）。
//
// v2.0：定义在 internal/document/style，此处以 alias 暴露。
type FontSize = style.FontSize

// Pts 以点数构造 FontSize。
func Pts(v float64) FontSize { return style.Pts(v) }

// FontProperty 标识可单独重置的字符格式属性（ResetFontProperty 参数）。
type FontProperty = style.FontProperty

// 字符格式属性常量（alias 到 internal/document/style）。
const (
	// FontPropBold 粗体。
	FontPropBold = style.FontPropBold
	// FontPropItalic 斜体。
	FontPropItalic = style.FontPropItalic
	// FontPropSize 字号。
	FontPropSize = style.FontPropSize
	// FontPropColor 颜色。
	FontPropColor = style.FontPropColor
	// FontPropLatin Latin 字体。
	FontPropLatin = style.FontPropLatin
	// FontPropEastAsian 东亚字体。
	FontPropEastAsian = style.FontPropEastAsian
	// FontPropComplexScript 复杂文种字体。
	FontPropComplexScript = style.FontPropComplexScript
)

// ColorSpec 是字体颜色的原始规格（v2.0：定义在 internal/document/style）。
type ColorSpec = style.ColorSpec

// FontStyle 是字符格式的 patch 类型（方案 §5.2/§20.2）。
type FontStyle = style.FontStyle
