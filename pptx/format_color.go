package pptx

import (
	"github.com/F31/go-pptx/v2/internal/document/style"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是 M3 的**颜色解析与颜色变换全集**的根包接线。
// v2.0：类型与解析定义在 internal/document/style，此处以 alias 暴露；
// parseColorNode 为薄委托。

// ColorTransform 是单个颜色变换（ECMA EG_ColorTransform）。
type ColorTransform = style.ColorTransform

// ParsedColor 是颜色元素（a:srgbClr/a:schemeClr/…）的解析结果。
type ParsedColor = style.ParsedColor

// parseColorNode 解析颜色元素节点及其变换序列。env 用于 schemeClr 的
// 主题展开；不可用时（如无主题）RGB 留空并输出诊断。
func (p *Presentation) parseColorNode(doc *xmlstore.XMLDocument, env *style.Env, part string,
	clr *xmlstore.NodeRecord, diags *[]Diagnostic) ParsedColor {
	return style.ParseColorNode(doc, clr, p.themeDoc(env), p.masterDoc(env), part, diags)
}
