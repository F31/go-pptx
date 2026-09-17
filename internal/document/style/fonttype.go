package style

import (
	"fmt"

	"github.com/F31/go-pptx/v2/internal/document/model"
)

// FontSize 是字号（单位 pt；XML 存储为百分之一 pt 的 a:sz val）。
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

// FontStyle 是字符格式的 patch 类型（方案 §5.2/§20.2）。
//
// 写入（SetFont）：仅应用 Set=true 的字段，未设置字段保持不变；恢复
// 继承必须调用 ResetFontProperty，不能用 Set=false 表达"恢复"。
// 读取（ExplicitFont）：Set=true 表示原 XML 存在该本地属性，Value 为
// 其值；Set=false 表示原 XML 没有该本地属性（继承链解析属 STYLE-01）。
type FontStyle struct {
	Bold   model.Optional[bool]
	Italic model.Optional[bool]
	Size   model.Optional[FontSize]
	Color  model.Optional[ColorSpec]
	// Latin/EastAsian/ComplexScript 字体名（a:latin/a:ea/a:cs typeface）。
	Latin         model.Optional[string]
	EastAsian     model.Optional[string]
	ComplexScript model.Optional[string]
}

func (f FontStyle) String() string {
	return fmt.Sprintf("bold=%v italic=%v size=%v color=%v latin=%v ea=%v cs=%v",
		f.Bold, f.Italic, f.Size, f.Color, f.Latin, f.EastAsian, f.ComplexScript)
}

// AnySet 报告 FontStyle 是否含显式字段。
func (f FontStyle) AnySet() bool {
	return f.Bold.Set || f.Italic.Set || f.Size.Set || f.Color.Set ||
		f.Latin.Set || f.EastAsian.Set || f.ComplexScript.Set
}

// AnyChildSet 报告 FontStyle 是否含子元素类字段。
func (f FontStyle) AnyChildSet() bool {
	return f.Color.Set || f.Latin.Set || f.EastAsian.Set || f.ComplexScript.Set
}
