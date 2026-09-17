package pptx

import (
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 M3 的**段落属性全集**：a:lnSpc、buChar/buAutoNum/buBlip、
// a:tabLst → ParagraphProps。
//
// v2.0：类型与解析定义在 internal/document/style，此处以 alias 暴露；
// Paragraph.Props 为薄委托。

// Spacing 是间距值（百分比为千分比或磅值）。
type Spacing = style.Spacing

// BulletKind 是项目符号类型。
type BulletKind = style.BulletKind

// 项目符号类型常量（alias 到 internal/document/style）。
const (
	// BulletNone 表示无项目符号（a:buNone）。
	BulletNone = style.BulletNone
	// BulletChar 表示字符项目符号（a:buChar）。
	BulletChar = style.BulletChar
	// BulletAutoNum 表示自动编号（a:buAutoNum）。
	BulletAutoNum = style.BulletAutoNum
	// BulletBlip 表示图片项目符号（a:buBlip）。
	BulletBlip = style.BulletBlip
	// BulletUnknown 表示未识别/缺失符号定义。
	BulletUnknown = style.BulletUnknown
)

// Bullet 是段落项目符号的解析结果。
type Bullet = style.Bullet

// TabStop 是制表位（a:tab）。
type TabStop = style.TabStop

// ParagraphProps 是段落属性（a:pPr）的解析结果。
type ParagraphProps = style.ParagraphProps

// Props 返回段落属性（a:pPr）的解析结果（§2.3 矩阵"段落属性全集"
// 起步解析）。未知属性与子元素经诊断与 Unknown 输出，不臆造取值。
func (p *Paragraph) Props() (ParagraphProps, []Diagnostic, error) {
	doc, para, err := p.locatePara()
	if err != nil {
		return ParagraphProps{}, nil, Annotate(err, "Paragraph.Props")
	}
	pr := xmlstore.ChildOfKind(doc, para, ooxmlns.DrawingML, "pPr", 0)
	if pr == nil {
		return ParagraphProps{}, nil, nil
	}
	return style.ParseParagraphProps(doc, pr), nil, nil
}
