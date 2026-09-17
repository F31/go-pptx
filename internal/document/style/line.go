package style

// 本文件是 M3 的**线条系统**解析（根包 format.go 下沉）：spPr/a:ln →
// LineStyle（箭头、dash、join）。主题链经 Env + DocFunc 注入。

import (
	"strconv"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// LineStyle 是形状线条（a:ln）的解析结果（E 档常用子集）。
type LineStyle struct {
	// Specified 表示形状存在 a:ln（无线条时为 false）。
	Specified bool
	// Width 是线宽（EMU；a:ln@w 缺省 0 表示 hairline，按原值保留）。
	Width model.EMU
	// Cap 是端点形状（rnd/sq/flat）。
	Cap string
	// Compound 是复合线型（sng/dbl/thickThin/thinThick/tri）。
	Compound string
	// Align 是笔对齐（ctr/in）。
	Align string
	// Dash 是虚线类型（solid/dot/dash/... 或 custom）。
	Dash string
	// Join 是连接方式（round/bevel/miter）。
	Join string
	// MiterLimit 是斜接限制（a:miter@lim，千分比）。
	MiterLimit int32
	// Color 是线条颜色（含变换）。
	Color ParsedColor
	// HeadEnd / TailEnd 是箭头（a:headEnd/a:tailEnd）。
	HeadEnd LineEnd
	TailEnd LineEnd
	// Unknown 是未识别的线条子元素名。
	Unknown []string
}

// LineEnd 是线条端点（箭头）。
type LineEnd struct {
	// Specified 表示存在对应元素。
	Specified bool
	// Type 是端点类型（none/triangle/stealth/diamond/oval/arrow）。
	Type string
	// Width 与 Length 是尺寸档位（sm/med/lg）。
	Width  string
	Length string
}

// ParseShapeLine 解析形状线条（spPr/a:ln）；无 spPr 或无线条返回空
// LineStyle 与 nil 诊断。
func ParseShapeLine(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord,
	env *Env, docs DocFunc, part string) (LineStyle, []diag.Diagnostic) {
	var out LineStyle
	sp := xmlstore.ChildOfKind(doc, el, ooxmlns.PresentationML, "spPr", 0)
	if sp == nil {
		return out, nil
	}
	ln := xmlstore.ChildOfKind(doc, sp, ooxmlns.DrawingML, "ln", 0)
	if ln == nil {
		return out, nil
	}
	var diags []diag.Diagnostic
	out.Specified = true
	if v, ok := ln.Attr("", "w"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Width = model.EMU(n)
		}
	}
	out.Cap, _ = ln.Attr("", "cap")
	out.Compound, _ = ln.Attr("", "cmpd")
	out.Align, _ = ln.Attr("", "algn")
	for _, cid := range ln.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "prstDash":
			v, _ := c.Attr("", "val")
			out.Dash = v
		case "custDash":
			out.Dash = "custom"
		case "round":
			out.Join = "round"
		case "bevel":
			out.Join = "bevel"
		case "miter":
			out.Join = "miter"
			if v, ok := c.Attr("", "lim"); ok {
				if n, e := strconv.Atoi(v); e == nil {
					out.MiterLimit = int32(n)
				}
			}
		case "headEnd":
			out.HeadEnd = parseLineEnd(c)
		case "tailEnd":
			out.TailEnd = parseLineEnd(c)
		case "noFill", "solidFill", "gradFill", "pattFill":
			out.Color = ParseColorNode(doc, ColorChild(doc, c),
				themeDocOf(docs, env), masterDocOf(docs, env), part, &diags)
			if c.Local() != "solidFill" {
				out.Unknown = append(out.Unknown, c.Local())
			}
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out, diags
}

func parseLineEnd(n *xmlstore.NodeRecord) LineEnd {
	e := LineEnd{Specified: true}
	e.Type, _ = n.Attr("", "type")
	e.Width, _ = n.Attr("", "w")
	e.Length, _ = n.Attr("", "len")
	return e
}
