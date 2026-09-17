package pptx

// v2.0 域搬迁：样式 DTO/值类型迁入 internal/document/style，此处以
// alias 暴露（句柄/行为类型仍在根包，见 format_runprops/style_matrix 的
// 薄委托）。

import "github.com/F31/go-pptx/internal/document/style"

// ---------- Run 高级属性 ----------

// RunProps 是 Run 字符属性（a:rPr）中 STYLE-01 之外的高级项解析结果。
type RunProps = style.RunProps

// RunSymbol 是 a:sym 符号引用。
type RunSymbol = style.RunSymbol

// ---------- 线条系统 ----------

// LineStyle 是形状线条（a:ln）的解析结果。
type LineStyle = style.LineStyle

// LineEnd 是线条端点（箭头）。
type LineEnd = style.LineEnd

// ---------- 主题样式矩阵 ----------

// MatrixRefKind 是样式矩阵引用的目标类型。
type MatrixRefKind = style.MatrixRefKind

// 样式矩阵引用目标常量（alias 到 internal/document/style）。
const (
	// RefFill 是填充样式引用（a:fillRef）。
	RefFill = style.RefFill
	// RefLine 是线条样式引用（a:lnRef）。
	RefLine = style.RefLine
	// RefEffect 是效果样式引用（a:effectRef）。
	RefEffect = style.RefEffect
	// RefFont 是字体样式引用（a:fontRef）。
	RefFont = style.RefFont
)

// StyleMatrixRef 是样式矩阵引用（a:fillRef/lnRef/effectRef/fontRef）解析结果。
type StyleMatrixRef = style.StyleMatrixRef

// ThemeFontSlot 是 a:fontRef 指向的主题字体槽位。
type ThemeFontSlot = style.ThemeFontSlot

// 主题字体槽位常量（alias 到 internal/document/style）。
const (
	// FontSlotUnknown 表示 idx 无法映射到 major/minor。
	FontSlotUnknown = style.FontSlotUnknown
	// FontSlotMajor 是 a:fontScheme/a:majorFont。
	FontSlotMajor = style.FontSlotMajor
	// FontSlotMinor 是 a:fontScheme/a:minorFont。
	FontSlotMinor = style.FontSlotMinor
)
