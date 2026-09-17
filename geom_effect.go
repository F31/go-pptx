package pptx

import (
	"github.com/F31/go-pptx/internal/document/style"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 GEOM-02 的**效果解析**：a:effectLst/a:effectDag（阴影/发光/反射/
// 柔边）、a:scene3d/a:sp3d → EffectInfo。
// 几何解析见 geomparse.go，填充解析见 fillparse.go。

// ---------- Effects 解析 ----------

// parseShapeEffects 解析 spPr/a:effectLst|a:effectDag + a:scene3d + a:sp3d。
func parseShapeEffects(p *Presentation, doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord, part string) (EffectInfo, []Diagnostic, error) {
	var diags []Diagnostic
	sp := spPrOf(doc, el)
	if sp == nil {
		diags = append(diags, Diagnostic{
			Code: "effects.spPr.missing", Severity: SeverityWarning,
			Message: "shape has no spPr; effects not applicable",
		})
		return EffectInfo{Diagnostics: diags}, diags, nil
	}
	out := EffectInfo{}
	for _, cid := range sp.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "effectLst", "effectDag":
			out.Container = c.Local()
			parseEffectList(p, doc, c, part, &out, &diags)
		case "scene3d":
			out.Scene3D = parseScene3D(doc, c, &diags)
			// scene3d 内 a:sp3d 嵌套解析。
			if sp3 := childOfKind(doc, c, nsDrawingML, "sp3d", 0); sp3 != nil {
				out.Shape3D = parseShape3D(doc, sp3, &diags)
			}
		}
	}
	out.Diagnostics = diags
	return out, diags, nil
}

// parseEffectList 解析 a:effectLst / a:effectDag 内全部效果条目。
func parseEffectList(p *Presentation, doc *xmlstore.XMLDocument, cont *xmlstore.NodeRecord,
	part string, out *EffectInfo, diags *[]Diagnostic) {
	var env *style.Env
	if p != nil {
		if e, err := p.styleEnv(opc.PartName(part)); err == nil {
			env = e
		}
	}
	for _, cid := range cont.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		eff := Effect{Raw: c.Local(), BlendMode: "norm", Color: ParsedColor{Alpha: 1}}
		switch c.Local() {
		case "outerShdw", "innerShdw":
			parseShadow(p, doc, c, part, env, &eff, diags)
			if c.Local() == "innerShdw" {
				eff.Kind = EffectInnerShadow
			} else {
				eff.Kind = EffectOuterShadow
			}
		case "glow":
			eff.Kind = EffectGlow
			if v, ok := c.Attr("", "rad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = EMU(n)
				}
			}
			clr := firstDrawingChild(doc, c)
			eff.Color = p.parseColorNode(doc, env, part, clr, diags)
		case "softEdge":
			eff.Kind = EffectSoftEdge
			if v, ok := c.Attr("", "rad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = EMU(n)
				}
			}
		case "reflection":
			eff.Kind = EffectReflection
			if v, ok := c.Attr("", "blurRad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = EMU(n)
				}
			}
			if v, ok := c.Attr("", "stA"); ok {
				if f, e := strconv.ParseFloat(v, 64); e == nil {
					eff.StartOpacity = f / 100000.0
				}
			}
			if v, ok := c.Attr("", "stPos"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.Distance = EMU(n)
				}
			}
			if v, ok := c.Attr("", "endA"); ok {
				if f, e := strconv.ParseFloat(v, 64); e == nil {
					eff.EndOpacity = f / 100000.0
				}
			}
			if v, ok := c.Attr("", "endPos"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.OffsetY = EMU(n)
				}
			}
			if v, ok := c.Attr("", "dir"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.Direction = n
				}
			}
			if v, ok := c.Attr("", "fadeDir"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.FadeDirection = n
				}
			}
			parseBlendModeAttrs(c, &eff)
		case "fillOverlay":
			eff.Kind = EffectFillOverlay
			clr := firstDrawingChild(doc, c)
			eff.Color = p.parseColorNode(doc, env, part, clr, diags)
			parseBlendModeAttrs(c, &eff)
		default:
			// a14:blur 等扩展暂归 unknown。
			eff.Kind = EffectUnknown
			*diags = append(*diags, Diagnostic{
				Code: "effects.unknown_kind", Severity: SeverityInfo,
				Message: "unrecognised effect element: " + c.Local(),
			})
		}
		// 通用 @hideSelf（a:effectLst 的常见属性）。
		if v, ok := c.Attr("", "hideSelf"); ok {
			eff.Hidden = v == "1"
		}
		out.Effects = append(out.Effects, eff)
	}
}

// parseShadow 解析 outerShdw / innerShdw 的公用字段。
func parseShadow(p *Presentation, doc *xmlstore.XMLDocument, c *xmlstore.NodeRecord,
	part string, env *style.Env, eff *Effect, diags *[]Diagnostic) {
	if v, ok := c.Attr("", "blurRad"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.BlurRadius = EMU(n)
		}
	}
	if v, ok := c.Attr("", "dist"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.Distance = EMU(n)
		}
	}
	if v, ok := c.Attr("", "dir"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.Angle = n
		}
	}
	if v, ok := c.Attr("", "sx"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.OffsetX = EMU(n)
		}
	}
	if v, ok := c.Attr("", "sy"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.OffsetY = EMU(n)
		}
	}
	if v, ok := c.Attr("", "algn"); ok {
		eff.Unknown = append(eff.Unknown, "algn="+v)
	}
	clr := firstDrawingChild(doc, c)
	eff.Color = p.parseColorNode(doc, env, part, clr, diags)
	eff.Alpha = eff.Color.Alpha
	parseBlendModeAttrs(c, eff)
}

// parseBlendModeAttrs 提取 @blend 属性。
func parseBlendModeAttrs(c *xmlstore.NodeRecord, eff *Effect) {
	if v, ok := c.Attr("", "blend"); ok {
		eff.BlendMode = v
	}
}

// parseScene3D 解析 a:scene3d 内 a:camera + a:lightRig。
func parseScene3D(doc *xmlstore.XMLDocument, scene *xmlstore.NodeRecord, diags *[]Diagnostic) *Scene3DInfo {
	out := &Scene3DInfo{}
	if cam := childOfKind(doc, scene, nsDrawingML, "camera", 0); cam != nil {
		out.Camera.Preset, _ = cam.Attr("", "prst")
		if v, ok := cam.Attr("", "zoom"); ok {
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				out.Camera.Zoom = NewOptional(f)
			}
		}
		if rot := childOfKind(doc, cam, nsDrawingML, "rot", 0); rot != nil {
			out.Camera.Rot = axisTriplet(rot, []string{"lat", "lng", "rev"})
		}
	}
	if rig := childOfKind(doc, scene, nsDrawingML, "lightRig", 0); rig != nil {
		out.LightRig.Rig, _ = rig.Attr("", "rig")
		if rot := childOfKind(doc, rig, nsDrawingML, "rot", 0); rot != nil {
			out.LightRig.Dir = axisTriplet(rot, []string{"lat", "lng", "rev"})
		}
	}
	if bd := childOfKind(doc, scene, nsDrawingML, "backdrop", 0); bd != nil {
		if v, ok := bd.Attr("", "plane"); ok {
			out.BackdropPlane = parseOptionalBool(v)
		}
	}
	return out
}

// parseShape3D 解析 a:sp3d（含 bevelT / bevelB / extrusionH / contourW）。
func parseShape3D(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord, diags *[]Diagnostic) *Shape3DInfo {
	out := &Shape3DInfo{PresetMaterial: "warmMatte"}
	if v, ok := sp.Attr("", "prstMaterial"); ok {
		out.PresetMaterial = v
	}
	if v, ok := sp.Attr("", "extrusionH"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.ExtrusionH = EMU(n)
		}
	}
	if v, ok := sp.Attr("", "contourW"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.ContourW = EMU(n)
		}
	}
	if b := childOfKind(doc, sp, nsDrawingML, "bevelT", 0); b != nil {
		out.TopBevel = parseBevel(doc, b)
	}
	if b := childOfKind(doc, sp, nsDrawingML, "bevelB", 0); b != nil {
		out.BotBevel = parseBevel(doc, b)
	}
	return out
}

// parseBevel 解析 a:bevelT/a:bevelB。
func parseBevel(doc *xmlstore.XMLDocument, b *xmlstore.NodeRecord) Bevel3D {
	out := Bevel3D{Preset: "round"}
	if v, ok := b.Attr("", "prst"); ok {
		out.Preset = v
	}
	if v, ok := b.Attr("", "w"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Width = EMU(n)
		}
	}
	if v, ok := b.Attr("", "h"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Height = EMU(n)
		}
	}
	return out
}

// axisTriplet 从 a:rot 元素读取三属性 lat/lng/rev（1/60000 度）。
func axisTriplet(rot *xmlstore.NodeRecord, keys []string) []int64 {
	out := make([]int64, 0, len(keys))
	for _, k := range keys {
		v, ok := rot.Attr("", k)
		if !ok {
			out = append(out, 0)
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

// parseOptionalBool 把 "1"/"0"/"true"/"false" 映射为 Optional[bool]。
// 非合法值返回未设置的 Optional（Set=false）。
func parseOptionalBool(v string) Optional[bool] {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true":
		return NewOptional(true)
	case "0", "false":
		return NewOptional(false)
	}
	return Optional[bool]{}
}
