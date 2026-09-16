// Package style 定义样式/格式域的纯值类型与只读解析（段落属性、间距、
// 项目符号、制表位等）。不依赖根包。
package style

import (
	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// Spacing 是间距值（百分比为千分比或磅值）。
type Spacing struct {
	// Kind 为 "pct"（千分比，100000=100%）或 "pts"（磅）。
	Kind string
	// Value 是数值。
	Value int32
}

// BulletKind 是项目符号类型。
type BulletKind int

const (
	// BulletNone 表示无项目符号（a:buNone）。
	BulletNone BulletKind = iota
	// BulletChar 表示字符项目符号（a:buChar）。
	BulletChar
	// BulletAutoNum 表示自动编号（a:buAutoNum）。
	BulletAutoNum
	// BulletBlip 表示图片项目符号（a:buBlip）。
	BulletBlip
	// BulletUnknown 表示未识别/缺失符号定义。
	BulletUnknown
)

func (k BulletKind) String() string {
	switch k {
	case BulletNone:
		return "none"
	case BulletChar:
		return "char"
	case BulletAutoNum:
		return "autonum"
	case BulletBlip:
		return "blip"
	}
	return "unknown"
}

// Bullet 是段落项目符号的解析结果。
type Bullet struct {
	Kind BulletKind
	// Char 是符号字符（a:buChar@char）。
	Char string
	// Font 是符号字体（a:buFont@typeface）。
	Font string
	// AutoNumType 是编号类型（a:buAutoNum@type）。
	AutoNumType string
	// StartAt 是起始编号（a:buAutoNum@startAt）。
	StartAt int32
	// SizePct / SizePts 是符号相对尺寸（千分比 / 磅）。
	SizePct int32
	SizePts float64
}

// TabStop 是制表位（a:tab）。
type TabStop struct {
	// Position 是位置（EMU）。
	Position model.EMU
	// Align 是对齐（l/ctr/r/dec）。
	Align string
}

// ParagraphProps 是段落属性（a:pPr）的解析结果。
type ParagraphProps struct {
	// Specified 表示存在 a:pPr（缺省继承时为 false）。
	Specified bool
	Level     int32
	Align     string
	// Indent 是首行/悬挂缩进（EMU，负值表示悬挂）。
	Indent      model.EMU
	MarginLeft  model.EMU
	MarginRight model.EMU
	LineSpacing *Spacing
	SpaceBefore *Spacing
	SpaceAfter  *Spacing
	Bullet      *Bullet
	Tabs        []TabStop
	// DefaultTabSize 是默认制表宽度（EMU）。
	DefaultTabSize model.EMU
	// RTL 表示从右到左段落。
	RTL bool
	// EastAsianLineBreak / LatinLineBreak / HangingPunct 是换行与标点规则。
	EastAsianLineBreak bool
	LatinLineBreak     bool
	HangingPunct       bool
	// FontAlign 是字体对齐（auto/t/ctr/b/base）。
	FontAlign string
	// Unknown 是未识别的 a:pPr 属性/子元素名。
	Unknown []string
}

// ParseParagraphProps 解析 a:pPr 节点为 ParagraphProps（已知属性/子元素；
// 未知项记入 Unknown，不臆造取值）。
func ParseParagraphProps(doc *xmlstore.XMLDocument, pr *xmlstore.NodeRecord) ParagraphProps {
	out := ParagraphProps{Specified: true}
	known := map[string]bool{
		"lvl": true, "algn": true, "indent": true, "marL": true, "marR": true,
		"rtl": true, "eaLnBrk": true, "latinLnBrk": true, "hangingPunct": true,
		"fontAlgn": true, "defTabSz": true,
	}
	for i := range pr.Attrs {
		a := &pr.Attrs[i]
		if a.Namespace != "" {
			out.Unknown = append(out.Unknown, a.RawName)
			continue
		}
		switch a.RawName {
		case "lvl":
			out.Level = xmlstore.IntAttr(a.Value)
		case "algn":
			out.Align = a.Value
		case "indent":
			out.Indent = model.EMU(xmlstore.IntAttr(a.Value))
		case "marL":
			out.MarginLeft = model.EMU(xmlstore.IntAttr(a.Value))
		case "marR":
			out.MarginRight = model.EMU(xmlstore.IntAttr(a.Value))
		case "rtl", "eaLnBrk", "latinLnBrk", "hangingPunct":
			v := a.Value == "1" || a.Value == "true"
			switch a.RawName {
			case "rtl":
				out.RTL = v
			case "eaLnBrk":
				out.EastAsianLineBreak = v
			case "latinLnBrk":
				out.LatinLineBreak = v
			default:
				out.HangingPunct = v
			}
		case "fontAlgn":
			out.FontAlign = a.Value
		case "defTabSz":
			out.DefaultTabSize = model.EMU(xmlstore.IntAttr(a.Value))
		default:
			if !known[a.RawName] {
				out.Unknown = append(out.Unknown, a.RawName)
			}
		}
	}
	for _, cid := range pr.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "lnSpc":
			out.LineSpacing = parseSpacing(doc, c)
		case "spcBef":
			out.SpaceBefore = parseSpacing(doc, c)
		case "spcAft":
			out.SpaceAfter = parseSpacing(doc, c)
		case "tabLst":
			for _, tid := range c.Children {
				t := doc.Node(tid)
				if t.Namespace != ooxmlns.DrawingML || t.Local() != "tab" {
					continue
				}
				pos, _ := t.Attr("", "pos")
				algn, _ := t.Attr("", "algn")
				out.Tabs = append(out.Tabs, TabStop{Position: model.EMU(xmlstore.IntAttr(pos)), Align: algn})
			}
		case "buNone":
			out.Bullet = &Bullet{Kind: BulletNone}
		case "buChar":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Kind = BulletChar
			out.Bullet.Char, _ = c.Attr("", "char")
		case "buAutoNum":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Kind = BulletAutoNum
			out.Bullet.AutoNumType, _ = c.Attr("", "type")
			if v, ok := c.Attr("", "startAt"); ok {
				out.Bullet.StartAt = xmlstore.IntAttr(v)
			}
		case "buBlip":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Kind = BulletBlip
		case "buFont":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Font, _ = c.Attr("", "typeface")
		case "buSzPct":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			if v, ok := c.Attr("", "val"); ok {
				out.Bullet.SizePct = xmlstore.IntAttr(v)
			}
		case "buSzPts":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			if v, ok := c.Attr("", "val"); ok {
				out.Bullet.SizePts = float64(xmlstore.IntAttr(v)) / 100
			}
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out
}

// parseSpacing 解析 a:lnSpc/a:spcBef/a:spcAft 的 spcPct/spcPts。
func parseSpacing(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *Spacing {
	if pct := xmlstore.ChildOfKind(doc, n, ooxmlns.DrawingML, "spcPct", 0); pct != nil {
		v, _ := pct.Attr("", "val")
		return &Spacing{Kind: "pct", Value: xmlstore.IntAttr(v)}
	}
	if pts := xmlstore.ChildOfKind(doc, n, ooxmlns.DrawingML, "spcPts", 0); pts != nil {
		v, _ := pts.Attr("", "val")
		return &Spacing{Kind: "pts", Value: xmlstore.IntAttr(v)}
	}
	return nil
}
