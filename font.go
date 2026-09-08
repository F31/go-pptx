package pptx

import "fmt"

// Optional[T] 是"未设置/显式设置"的双态包装（方案 §5.2）。
//
// Set=false 表示无本地覆盖（继承）；Set=true 时必须按 Value 理解——
// 例如 FontStyle.Bold 的 Set=true, Value=false 表示显式取消粗体，而非
// "未设置"。读取状态与写入 patch 语义见 FontStyle 文档。
type Optional[T any] struct {
	Value T
	Set   bool
}

// NewOptional 构造显式设置值。
func NewOptional[T any](v T) Optional[T] { return Optional[T]{Value: v, Set: true} }

// FontSize 是字号（单位 pt；XML 存储为百分之一 pt 的 a:sz val，
// 单位集中换算属 GEOM-01，本类型先承担强类型职责）。
type FontSize float64

// Pts 以点数构造 FontSize。
func Pts(v float64) FontSize { return FontSize(v) }

// FontProperty 标识可单独重置的字符格式属性（ResetFontProperty 参数）。
type FontProperty int

const (
	// FontPropBold 粗体。
	FontPropBold FontProperty = iota
	// FontPropItalic 斜体。
	FontPropItalic
	// FontPropSize 字号。
	FontPropSize
	// FontPropColor 颜色。
	FontPropColor
	// FontPropLatin Latin 字体。
	FontPropLatin
	// FontPropEastAsian 东亚字体。
	FontPropEastAsian
	// FontPropComplexScript 复杂文种字体。
	FontPropComplexScript
)

// ColorSpec 是字体颜色的原始规格（方案 §6.1：颜色保留原始 ColorSpec）。
//
// 二选一：Scheme 非空表示 a:schemeClr（主题色引用，如 "accent1"）；
// 否则 RGB 是 sRGB 十六进制 RRGGBB（无 '#' 前缀）。两者都为空表示
// "无颜色信息"（读取到无法安全表示的颜色形态时保持 Set=false，不臆测；
// STYLE-01 扩展颜色变换等解析）。
type ColorSpec struct {
	Scheme string
	RGB    string
}

func (c ColorSpec) String() string {
	if c.Scheme != "" {
		return "scheme:" + c.Scheme
	}
	if c.RGB != "" {
		return "#" + c.RGB
	}
	return ""
}

// Valid 报告 ColorSpec 是否携带可写出的颜色。
func (c ColorSpec) Valid() bool { return c.Scheme != "" || c.RGB != "" }

// FontStyle 是字符格式的 patch 类型（方案 §5.2/§20.2）。
//
// 写入（SetFont）：仅应用 Set=true 的字段，未设置字段保持不变；恢复
// 继承必须调用 ResetFontProperty，不能用 Set=false 表达"恢复"。
// 读取（ExplicitFont）：Set=true 表示原 XML 存在该本地属性，Value 为
// 其值；Set=false 表示原 XML 没有该本地属性（继承链解析属 STYLE-01）。
type FontStyle struct {
	Bold   Optional[bool]
	Italic Optional[bool]
	Size   Optional[FontSize]
	Color  Optional[ColorSpec]
	// Latin/EastAsian/ComplexScript 字体名（a:latin/a:ea/a:cs typeface）。
	Latin         Optional[string]
	EastAsian     Optional[string]
	ComplexScript Optional[string]
}

func (f FontStyle) String() string {
	return fmt.Sprintf("bold=%v italic=%v size=%v color=%v latin=%v ea=%v cs=%v",
		f.Bold, f.Italic, f.Size, f.Color, f.Latin, f.EastAsian, f.ComplexScript)
}
