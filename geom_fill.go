package pptx

import (
	"github.com/F31/go-pptx/internal/document/style"
	"strconv"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 GEOM-02 的**填充解析**：solidFill/gradFill（gsLst + 线性/路径）/
// pattFill/blipFill/grpFill/noFill → FillInfo。
// 几何解析见 geomparse.go，效果解析见 effectparse.go。

// ---------- Fill 解析 ----------

// parseShapeFill 解析 spPr/a:fill 内的首个有意义的填充元素。
func parseShapeFill(p *Presentation, doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord, part string) (FillInfo, []Diagnostic, error) {
	var diags []Diagnostic
	fc := fillContainer(doc, el)
	if fc == nil {
		// 形状无 spPr 或 spPr 无 a:fill：返回 unknown + Warning。
		diags = append(diags, Diagnostic{
			Code: "fill.spPr.missing", Severity: SeverityWarning,
			Message: "shape has no spPr/fill; fill not applicable",
		})
		return FillInfo{Kind: FillUnspecified, Diagnostics: diags}, diags, nil
	}
	// 准备 styleEnv 供 schemeClr 展开。
	var env *style.Env
	if p != nil {
		if e, err := p.styleEnv(opc.PartName(part)); err == nil {
			env = e
		} else {
			diags = append(diags, Diagnostic{
				Code: "fill.theme.unavailable", Severity: SeverityInfo,
				Message: "theme unavailable for fill color resolution: " + err.Error(),
			})
		}
	}
	out := FillInfo{Diagnostics: diags}
	first := true
	for _, cid := range fc.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		// 仅第一个语义填充元素生效；其余视为冲突并记 Unknown。
		if !first {
			out.Unknown = append(out.Unknown, c.Local())
			continue
		}
		first = false
		switch c.Local() {
		case "noFill":
			out.Kind = FillNone
		case "solidFill":
			out.Kind = FillSolid
			out.Color = p.parseColorNode(doc, env, part, style.ColorChild(doc, c), &diags)
		case "gradFill":
			out.Kind = FillGradient
			out.Gradient = parseGradFill(p, doc, c, part, env, &diags)
		case "pattFill":
			out.Kind = FillPattern
			out.Pattern = parsePattFill(p, doc, c, part, env, &diags)
		case "blipFill":
			out.Kind = FillPicture
			out.Blip = parseBlipFill(doc, c, &diags)
		case "grpFill":
			out.Kind = FillGroup
		default:
			out.Kind = FillUnspecified
			out.Raw = c.Local()
			diags = append(diags, Diagnostic{
				Code: "fill.unknown_kind", Severity: SeverityWarning,
				Message: "unknown fill kind: " + c.Local(),
			})
		}
	}
	out.Diagnostics = diags
	return out, diags, nil
}

// parseGradFill 解析 a:gradFill + a:gsLst/a:gs/a:stop + a:lin/a:path。
func parseGradFill(p *Presentation, doc *xmlstore.XMLDocument, grad *xmlstore.NodeRecord,
	part string, env *style.Env, diags *[]Diagnostic) *GradientFill {
	g := &GradientFill{Flip: "none", TileAlign: "ctr", RotateWithShape: NewOptional(true)}
	if v, ok := grad.Attr("", "flip"); ok {
		g.Flip = v
	}
	if v, ok := grad.Attr("", "tileAlign"); ok {
		g.TileAlign = v
	}
	if v, ok := grad.Attr("", "rotWithShape"); ok {
		g.RotateWithShape = parseOptionalBool(v)
	}
	for _, cid := range grad.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "gsLst":
			for _, gid := range c.Children {
				gs := doc.Node(gid)
				if gs.Namespace != nsDrawingML || gs.Local() != "gs" {
					continue
				}
				stop := GradientStop{Position: -1}
				if v, ok := gs.Attr("", "pos"); ok {
					if n, e := strconv.ParseInt(v, 10, 32); e == nil {
						stop.Position = int32(n)
					} else {
						*diags = append(*diags, Diagnostic{
							Code: "fill.gradient.invalid_pos", Severity: SeverityWarning,
							Message: "invalid gs@pos: " + v,
						})
					}
				}
				// gs 内首个 a:DrawingML 子元素即为颜色。
				clr := firstDrawingChild(doc, gs)
				stop.Color = p.parseColorNode(doc, env, part, clr, diags)
				g.Stops = append(g.Stops, stop)
			}
		case "lin":
			if v, ok := c.Attr("", "ang"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					g.Angle = n
				}
			}
			if v, ok := c.Attr("", "scaled"); ok {
				g.Scaled = parseOptionalBool(v)
			}
		case "path":
			g.PathType, _ = c.Attr("", "path")
			if g.PathType == "" {
				g.PathType = "shape"
			}
			if fillToRect := childOfKind(doc, c, nsDrawingML, "fillToRect", 0); fillToRect != nil {
				g.PathLeft = percentAttr(doc, fillToRect, "l")
				g.PathRight = percentAttr(doc, fillToRect, "r")
				g.PathTop = percentAttr(doc, fillToRect, "t")
				g.PathBottom = percentAttr(doc, fillToRect, "b")
			}
		case "tileRect":
			// 路径下 a:tileRect（l/t/r/b）暂作 R 档细节可选保留；此处只跳过。
		default:
			g.Unknown = append(g.Unknown, c.Local())
		}
	}
	return g
}

// parsePattFill 解析 a:pattFill + fg/bg 颜色。
func parsePattFill(p *Presentation, doc *xmlstore.XMLDocument, patt *xmlstore.NodeRecord,
	part string, env *style.Env, diags *[]Diagnostic) *PatternFill {
	out := &PatternFill{Preset: "pct5"}
	if v, ok := patt.Attr("", "prst"); ok {
		out.Preset = v
	}
	for _, cid := range patt.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		clr := style.ColorChild(doc, c)
		col := p.parseColorNode(doc, env, part, clr, diags)
		switch c.Local() {
		case "fgClr":
			out.Foreground = col
		case "bgClr":
			out.Background = col
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out
}

// parseBlipFill 解析 a:blipFill 内 a:blip（rId/dpi/rotWithShape）与 a:srcRect。
func parseBlipFill(doc *xmlstore.XMLDocument, bf *xmlstore.NodeRecord, diags *[]Diagnostic) *BlipFillInfo {
	out := &BlipFillInfo{RotateWithShape: NewOptional(true)}
	for _, cid := range bf.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "blip":
			out.RId, _ = c.Attr(nsOfficeDocument, "embed")
			if v, ok := c.Attr("", "dpi"); ok {
				if n, e := strconv.ParseInt(v, 10, 32); e == nil {
					out.Dpi = int32(n)
				}
			}
			if v, ok := c.Attr("", "rotWithShape"); ok {
				out.RotateWithShape = parseOptionalBool(v)
			}
		case "srcRect":
			out.SrcLeft = percentAttr(doc, c, "l")
			out.SrcRight = percentAttr(doc, c, "r")
			out.SrcTop = percentAttr(doc, c, "t")
			out.SrcBottom = percentAttr(doc, c, "b")
		case "tile", "stretch":
			// 合法子元素，不记 Unknown。
		default:
			*diags = append(*diags, Diagnostic{
				Code: "fill.blip.unknown_child", Severity: SeverityInfo,
				Message: "unknown blipFill child: " + c.Local(),
			})
		}
	}
	return out
}

// percentAttr 解析百分比属性（千分比字符串），失败返回 -1 表示"无值"。
func percentAttr(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, name string) int32 {
	v, ok := n.Attr("", name)
	if !ok {
		return -1
	}
	num, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return -1
	}
	return int32(num)
}

// firstDrawingChild 返回容器内首个 a:DrawingML 命名空间下的子元素。
func firstDrawingChild(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML {
			return c
		}
	}
	return nil
}
