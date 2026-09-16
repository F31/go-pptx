package pptx

import (
	"strconv"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件组实现 M3"格式深度子集"（方案 2.3 矩阵 M3 行）；本文件承载**线条系统**：
//
//	形状级 线条系统      a:ln（箭头、dash、join）                    E 档
//	文本级 段落属性全集  a:lnSpc、buChar/buAutoNum/buBlip、a:tabLst  E 档
//	文本级 Run 高级属性  baseline、spc、highlight、caps、sym         E 档
//	主题   样式矩阵      a:fmtScheme 与 styleMatrixReference 引用链   R 档
//	主题   颜色变换全集  lumMod/lumOff/shade/tint/satMod 等约 20 种   R 全集
//
// 本轮为"起步解析"：提供结构化读取与诊断输出，不提供写入 API（写入与
// 像素级一致随 M6 的 STYLE-02/GEOM-02/TEXT-03 收口）。每项解析遇到
// 未知形态时保留已知信息并输出诊断，不臆造取值（§6.1 契约）。

// ---------- 线条系统（a:ln） ----------

// LineStyle 是形状线条（a:ln）的解析结果（E 档常用子集）。
type LineStyle struct {
	// Specified 表示形状存在 a:ln（无线条时为 false）。
	Specified bool
	// Width 是线宽（EMU；a:ln@w 缺省 0 表示 hairline，按原值保留）。
	Width EMU
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

// Line 返回形状的线条（spPr/a:ln）；形状无线条返回 Specified=false。
// 组合（grpSp）等无 spPr 的形状返回空 LineStyle 与 nil 错误。
func (s *shapeNode) Line() (LineStyle, []Diagnostic, error) {
	var out LineStyle
	doc, el, err := s.locate()
	if err != nil {
		return out, nil, Annotate(err, "shape.Line")
	}
	sp := childOfKind(doc, el, nsPresentationML, "spPr", 0)
	if sp == nil {
		return out, nil, nil
	}
	ln := childOfKind(doc, sp, nsDrawingML, "ln", 0)
	if ln == nil {
		return out, nil, nil
	}
	var diags []Diagnostic
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return out, nil, Annotate(err, "shape.Line")
	}
	out.Specified = true
	if v, ok := ln.Attr("", "w"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Width = EMU(n)
		}
	}
	out.Cap, _ = ln.Attr("", "cap")
	out.Compound, _ = ln.Attr("", "cmpd")
	out.Align, _ = ln.Attr("", "algn")
	for _, cid := range ln.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
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
			out.Color = s.p.parseColorNode(doc, env, string(s.part), colorChildOf(doc, c), &diags)
			if c.Local() != "solidFill" {
				out.Unknown = append(out.Unknown, c.Local())
			}
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out, diags, nil
}

func parseLineEnd(n *xmlstore.NodeRecord) LineEnd {
	e := LineEnd{Specified: true}
	e.Type, _ = n.Attr("", "type")
	e.Width, _ = n.Attr("", "w")
	e.Length, _ = n.Attr("", "len")
	return e
}

// colorChildOf 返回填充元素内的颜色元素（solidFill→srgbClr 等）。
func colorChildOf(doc *xmlstore.XMLDocument, fill *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML {
			return c
		}
	}
	return nil
}
