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

// ---------- 表格样式 ----------

// StyleToggle 是表格样式区域开关的三态（ST_OnOffStyleType）。
type StyleToggle = style.StyleToggle

// 三态开关常量（alias 到 internal/document/style）。
const (
	// ToggleDefault 表示未指定（沿用样式定义）。
	ToggleDefault = style.ToggleDefault
	// ToggleOn 表示显式启用。
	ToggleOn = style.ToggleOn
	// ToggleOff 表示显式关闭。
	ToggleOff = style.ToggleOff
)

// TableStyleFlags 是表格级区域开关（a:tblPr 属性）。
type TableStyleFlags = style.TableStyleFlags

// StylePart 是表格样式的区域部分（ECMA tblStyle 的部分）。
type StylePart = style.StylePart

// 区域部分常量（alias 到 internal/document/style）。
const (
	// PartWholeTable 是整表兜底（优先级最低）。
	PartWholeTable = style.PartWholeTable
	PartBand1H     = style.PartBand1H
	PartBand2H     = style.PartBand2H
	PartBand1V     = style.PartBand1V
	PartBand2V     = style.PartBand2V
	PartFirstRow   = style.PartFirstRow
	PartLastRow    = style.PartLastRow
	PartFirstCol   = style.PartFirstCol
	PartLastCol    = style.PartLastCol
	PartNWCell     = style.PartNWCell
	PartNECell     = style.PartNECell
	PartSWCell     = style.PartSWCell
	PartSECell     = style.PartSECell
)

// FillKind 是单元格填充类型（首版子集）。
type FillKind = style.FillKind

// 填充类型常量（alias 到 internal/document/style）。
const (
	// FillUnspecified 表示未给出填充定义（未知或未解析）。
	FillUnspecified = style.FillUnspecified
	// FillNone 表示显式无填充（a:noFill）。
	FillNone = style.FillNone
	// FillSolid 表示纯色填充（a:solidFill）。
	FillSolid = style.FillSolid
	// FillGradient 表示渐变填充（首版不解析颜色）。
	FillGradient = style.FillGradient
	// FillPattern 表示图案填充（首版不解析）。
	FillPattern = style.FillPattern
	// FillPicture 表示图片填充（首版不解析）。
	FillPicture = style.FillPicture
	// FillGroup 表示继承组填充（首版不解析）。
	FillGroup = style.FillGroup
)

// CellFill 是单元格填充的解析结果。
type CellFill = style.CellFill

// CellBorder 是单元格单条边框的解析结果。
type CellBorder = style.CellBorder

// CellText 是单元格文本相关属性（首版：对齐与内边距）。
type CellText = style.CellText

// CellBorders 是单元格四边边框。
type CellBorders = style.CellBorders

// EffectiveCellStyle 是单元格的逐属性样式解析结果。
type EffectiveCellStyle = style.EffectiveCellStyle
