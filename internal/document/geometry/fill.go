package geometry

// 本文件是 GEOM-02 的**填充解析**（根包 geom_fill.go 下沉）：solidFill/
// gradFill（gsLst + 线性/路径）/pattFill/blipFill/grpFill/noFill →
// FillInfo。颜色解析经 style.ResolveColor（Env + DocFunc 注入，与根包
// 与文档存储解耦）。

import (
	"strconv"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// Optional 是共享可空值包装，定义在 internal/document/model。
type Optional[T any] = model.Optional[T]

// FillInfo 是形状填充（spPr/a:fill 内首元素）的解析结果。
// Kind=FillSolid 时 Color 有效；Kind=FillGradient 时 Gradient 有效；
// Kind=FillPattern 时 Pattern 有效；Kind=FillPicture 时 Blip 有效。
type FillInfo struct {
	// Kind 是填充类别（FillKind 定义在 internal/document/style）。
	Kind style.FillKind
	// Color 是 solidFill 的颜色（Kind=FillSolid 时有效）。
	Color style.ParsedColor
	// Gradient 是渐变填充详情（Kind=FillGradient 时有效）。
	Gradient *GradientFill
	// Pattern 是图案填充详情（Kind=FillPattern 时有效）。
	Pattern *PatternFill
	// Blip 是图片填充占位（Kind=FillPicture 时有效）。
	Blip *BlipFillInfo
	// Raw 是底层填充元素 Local 名（Kind=FillUnspecified 且非未声明时记录）。
	Raw string
	// Unknown 是同一 a:fill 容器内其余未识别兄弟元素名。
	Unknown []string
	// Diagnostics 是解析期诊断条目。
	Diagnostics []diag.Diagnostic
}

// GradientFill 是 a:gradFill 解析结果。
type GradientFill struct {
	// Flip / TileAlign / RotateWithShape 是 a:gradFill 上的三个属性
	//（默认分别为 nil/"ctr"/true）。
	Flip            string
	TileAlign       string
	RotateWithShape Optional[bool]
	// PathType 是 a:path@path 的"shape"/"rect"/"circle"之一；缺省"shape"。
	PathType string
	// PathCenter 是 a:path@a:fillToRect 的 l/r/t/b 各通道（千分比 0..100000）。
	PathLeft, PathRight, PathTop, PathBottom int32
	// Angle 是线性渐变角度（a:lin@ang，1/60000 度；线性渐变时有效）。
	Angle int64
	// Scaled 是 a:lin@scaled（默认 true）。
	Scaled Optional[bool]
	// Stops 是按文档序的 a:gs 内 a:stop 列表。
	Stops []GradientStop
	// Unknown 是未识别的子元素名。
	Unknown []string
}

// GradientStop 是 a:gs/a:stop 一个停止点。
type GradientStop struct {
	// Position 是 a:gs@pos 的千分比值（0..100000）。
	Position int32
	// Color 是该停止点的颜色。
	Color style.ParsedColor
}

// PatternFill 是 a:pattFill 解析结果。
type PatternFill struct {
	// Preset 是 a:patt@prst 的预设图案名（如 "pct5"、"ltHorz"、"dkHorz"）。
	Preset string
	// Foreground / Background 是前景/背景颜色（缺一即视为部分解析）。
	Foreground style.ParsedColor
	Background style.ParsedColor
	// Unknown 是未识别的子元素名。
	Unknown []string
}

// BlipFillInfo 是 a:blipFill 的占位结构（图片关系未解析）。
type BlipFillInfo struct {
	// RId 是 a:blip@r:embed 关系 ID；缺关系时为空。
	RId string
	// Dpi 是 a:blip@dpi（资源 DPI）；缺省 0。
	Dpi int32
	// RotateWithShape 是 a:blip@rotWithShape 的值。
	RotateWithShape Optional[bool]
	// SourceRect 是 a:srcRect 的 l/r/t/b（千分比 0..100000）。
	SrcLeft, SrcRight, SrcTop, SrcBottom int32
}

// fillContainer 在形状元素下查找 spPr/a:fill 容器。spPr 不存在返回 nil。
func fillContainer(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	sp := xmlstore.ChildOfKind(doc, el, ooxmlns.PresentationML, "spPr", 0)
	if sp == nil {
		return nil
	}
	return xmlstore.ChildOfKind(doc, sp, ooxmlns.DrawingML, "fill", 0)
}

// ParseShapeFill 解析 spPr/a:fill 内的首个有意义的填充元素。env/docs
// 提供主题链（schemeClr 展开）；不可达时颜色留 unresolved，不臆造。
func ParseShapeFill(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord,
	env *style.Env, docs style.DocFunc, part string) (FillInfo, []diag.Diagnostic) {
	var diags []diag.Diagnostic
	fc := fillContainer(doc, el)
	if fc == nil {
		// 形状无 spPr 或 spPr 无 a:fill：返回 unknown + Warning。
		diags = append(diags, diag.Diagnostic{
			Code: "fill.spPr.missing", Severity: diag.SeverityWarning,
			Message: "shape has no spPr/fill; fill not applicable",
		})
		return FillInfo{Kind: style.FillUnspecified, Diagnostics: diags}, diags
	}
	out := FillInfo{Diagnostics: diags}
	first := true
	for _, cid := range fc.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
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
			out.Kind = style.FillNone
		case "solidFill":
			out.Kind = style.FillSolid
			out.Color = style.ResolveColor(doc, style.ColorChild(doc, c), env, docs, part, &diags)
		case "gradFill":
			out.Kind = style.FillGradient
			out.Gradient = parseGradFill(doc, c, env, docs, part, &diags)
		case "pattFill":
			out.Kind = style.FillPattern
			out.Pattern = parsePattFill(doc, c, env, docs, part, &diags)
		case "blipFill":
			out.Kind = style.FillPicture
			out.Blip = parseBlipFill(doc, c, &diags)
		case "grpFill":
			out.Kind = style.FillGroup
		default:
			out.Kind = style.FillUnspecified
			out.Raw = c.Local()
			diags = append(diags, diag.Diagnostic{
				Code: "fill.unknown_kind", Severity: diag.SeverityWarning,
				Message: "unknown fill kind: " + c.Local(),
			})
		}
	}
	out.Diagnostics = diags
	return out, diags
}

// parseGradFill 解析 a:gradFill + a:gsLst/a:gs/a:stop + a:lin/a:path。
func parseGradFill(doc *xmlstore.XMLDocument, grad *xmlstore.NodeRecord,
	env *style.Env, docs style.DocFunc, part string, diags *[]diag.Diagnostic) *GradientFill {
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
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "gsLst":
			for _, gid := range c.Children {
				gs := doc.Node(gid)
				if gs == nil || gs.Namespace != ooxmlns.DrawingML || gs.Local() != "gs" {
					continue
				}
				stop := GradientStop{Position: -1}
				if v, ok := gs.Attr("", "pos"); ok {
					if n, e := strconv.ParseInt(v, 10, 32); e == nil {
						stop.Position = int32(n)
					} else {
						*diags = append(*diags, diag.Diagnostic{
							Code: "fill.gradient.invalid_pos", Severity: diag.SeverityWarning,
							Message: "invalid gs@pos: " + v,
						})
					}
				}
				// gs 内首个 a:DrawingML 子元素即为颜色。
				clr := firstDrawingChild(doc, gs)
				stop.Color = style.ResolveColor(doc, clr, env, docs, part, diags)
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
			if fillToRect := xmlstore.ChildOfKind(doc, c, ooxmlns.DrawingML, "fillToRect", 0); fillToRect != nil {
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
func parsePattFill(doc *xmlstore.XMLDocument, patt *xmlstore.NodeRecord,
	env *style.Env, docs style.DocFunc, part string, diags *[]diag.Diagnostic) *PatternFill {
	out := &PatternFill{Preset: "pct5"}
	if v, ok := patt.Attr("", "prst"); ok {
		out.Preset = v
	}
	for _, cid := range patt.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		clr := style.ColorChild(doc, c)
		col := style.ResolveColor(doc, clr, env, docs, part, diags)
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
func parseBlipFill(doc *xmlstore.XMLDocument, bf *xmlstore.NodeRecord, diags *[]diag.Diagnostic) *BlipFillInfo {
	out := &BlipFillInfo{RotateWithShape: NewOptional(true)}
	for _, cid := range bf.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "blip":
			out.RId, _ = c.Attr(ooxmlns.OfficeDocument, "embed")
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
			*diags = append(*diags, diag.Diagnostic{
				Code: "fill.blip.unknown_child", Severity: diag.SeverityInfo,
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
		if c != nil && c.Namespace == ooxmlns.DrawingML {
			return c
		}
	}
	return nil
}

// NewOptional 共享可空值包装构造（模型在 internal/document/model）。
func NewOptional[T any](v T) Optional[T] { return model.NewOptional(v) }
