package geometry

// 本文件是 GEOM-02 的**效果解析**（根包 geom_effect.go 下沉）：
// a:effectLst/a:effectDag（阴影/发光/反射/柔边）、a:scene3d/a:sp3d →
// EffectInfo。颜色解析经 style.ResolveColor（Env + DocFunc 注入）。

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// EffectKind 是效果类别（a:effectLst 或 a:effectDag 顶层元素）。
type EffectKind int

const (
	// EffectNone 表示无效果（effectLst 元素存在但无内容）。
	EffectNone EffectKind = iota
	// EffectOuterShadow 是 a:outerShdw。
	EffectOuterShadow
	// EffectInnerShadow 是 a:innerShdw。
	EffectInnerShadow
	// EffectGlow 是 a:glow。
	EffectGlow
	// EffectSoftEdge 是 a:softEdge。
	EffectSoftEdge
	// EffectReflection 是 a:reflection。
	EffectReflection
	// EffectFillOverlay 是 a:fillOverlay。
	EffectFillOverlay
	// EffectBlur 是 a:blur（a14 扩展）。
	EffectBlur
	// EffectUnknown 是未识别/缺失类别。
	EffectUnknown
)

func (k EffectKind) String() string {
	switch k {
	case EffectOuterShadow:
		return "outerShdw"
	case EffectInnerShadow:
		return "innerShdw"
	case EffectGlow:
		return "glow"
	case EffectSoftEdge:
		return "softEdge"
	case EffectReflection:
		return "reflection"
	case EffectFillOverlay:
		return "fillOverlay"
	case EffectBlur:
		return "blur"
	}
	return "unknown"
}

// EffectInfo 是形状效果（spPr/a:effectLst 或 a:effectDag）的解析结果。
// Scene3D 与 Shape3D 仅在 spPr/a:scene3d 与 a:scene3d/a:sp3d 出现时填入。
type EffectInfo struct {
	// Container 是效果容器名（"effectLst" / "effectDag" / ""）。
	Container string
	// Effects 是按文档序的全部效果条目。
	Effects []Effect
	// Scene3D 表示 a:scene3d 解析结果（nil 表示未声明）。
	Scene3D *Scene3DInfo
	// Shape3D 表示 a:scene3d/a:sp3d 解析结果（nil 表示未声明）。
	Shape3D *Shape3DInfo
	// Diagnostics 是解析期诊断条目。
	Diagnostics []diag.Diagnostic
}

// Effect 是单个效果条目。
type Effect struct {
	// Kind 是效果类别。
	Kind EffectKind
	// Raw 是底层元素 Local 名（未识别时记录）。
	Raw string
	// Color 是阴影/发光/反射的颜色（softEdge 无颜色）；其它效果为空。
	Color style.ParsedColor
	// BlurRadius / OffsetX / OffsetY 是阴影/柔边的 EMU 半径与偏移。
	BlurRadius, OffsetX, OffsetY model.EMU
	// Angle 是阴影方向（1/60000 度）。
	Angle int64
	// Alpha 是阴影/反射等透明度（0..1）；缺省 1。
	Alpha float64
	// StandardDeviation 是高斯模糊标准差 EMU（glow/softEdge/reflection）。
	StandardDeviation model.EMU
	// Distance 是反射距离（EMU）。
	Distance model.EMU
	// Direction 是反射方向（a:reflection@dir，1/60000 度）。
	Direction int64
	// FadeDirection 是反射淡化方向（a:reflection@fadeDir，1/60000 度）。
	FadeDirection int64
	// Start / EndOpacity 是反射/阴影的起始/终止透明度（0..1）。
	StartOpacity, EndOpacity float64
	// BlendMode 是效果混合模式（"norm" / "mult" / "screen" 等）。
	BlendMode string
	// Hidden 是某些效果容器内的 @hideSelf="1"。
	Hidden bool
	// Unknown 是未识别的属性或子元素名。
	Unknown []string
}

// Scene3DInfo 是 a:scene3d 顶层解析（仅"声明存在"+关键子元素）。
type Scene3DInfo struct {
	// Camera 是 a:camera 集合（prst/zoom/rot）。
	Camera Camera3D
	// LightRig 是 a:lightRig 集合（rig/dir）。
	LightRig LightRig3D
	// BackdropPlane 是 a:backdrop 平面（可空）。
	BackdropPlane Optional[bool]
}

// Camera3D 是 a:scene3d/a:camera 解析。
type Camera3D struct {
	Preset string
	Zoom   Optional[float64]
	Rot    []int64 // yaw/pitch/roll 各 1/60000 度
}

// LightRig3D 是 a:scene3d/a:lightRig 解析。
type LightRig3D struct {
	Rig string
	Dir []int64 // latitude/longitude/revolution
}

// Shape3DInfo 是 a:sp3d 解析。
type Shape3DInfo struct {
	ExtrusionH         model.EMU
	ContourW           model.EMU
	PresetMaterial     string
	TopBevel, BotBevel Bevel3D
}

// Bevel3D 是 a:bevelT/a:bevelB 解析。
type Bevel3D struct {
	Preset string
	Width  model.EMU
	Height model.EMU
}

// ParseShapeEffects 解析 spPr/a:effectLst|a:effectDag + a:scene3d + a:sp3d。
func ParseShapeEffects(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord,
	env *style.Env, docs style.DocFunc, part string) (EffectInfo, []diag.Diagnostic) {
	var diags []diag.Diagnostic
	sp := spPrOf(doc, el)
	if sp == nil {
		diags = append(diags, diag.Diagnostic{
			Code: "effects.spPr.missing", Severity: diag.SeverityWarning,
			Message: "shape has no spPr; effects not applicable",
		})
		return EffectInfo{Diagnostics: diags}, diags
	}
	out := EffectInfo{}
	for _, cid := range sp.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "effectLst", "effectDag":
			out.Container = c.Local()
			parseEffectList(doc, c, env, docs, part, &out, &diags)
		case "scene3d":
			out.Scene3D = parseScene3D(doc, c, &diags)
			// scene3d 内 a:sp3d 嵌套解析。
			if sp3 := xmlstore.ChildOfKind(doc, c, ooxmlns.DrawingML, "sp3d", 0); sp3 != nil {
				out.Shape3D = parseShape3D(doc, sp3, &diags)
			}
		}
	}
	out.Diagnostics = diags
	return out, diags
}

// parseEffectList 解析 a:effectLst / a:effectDag 内全部效果条目。
func parseEffectList(doc *xmlstore.XMLDocument, cont *xmlstore.NodeRecord,
	env *style.Env, docs style.DocFunc, part string, out *EffectInfo, diags *[]diag.Diagnostic) {
	for _, cid := range cont.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		eff := Effect{Raw: c.Local(), BlendMode: "norm", Color: style.ParsedColor{Alpha: 1}}
		switch c.Local() {
		case "outerShdw", "innerShdw":
			parseShadow(doc, c, env, docs, part, &eff, diags)
			if c.Local() == "innerShdw" {
				eff.Kind = EffectInnerShadow
			} else {
				eff.Kind = EffectOuterShadow
			}
		case "glow":
			eff.Kind = EffectGlow
			if v, ok := c.Attr("", "rad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = model.EMU(n)
				}
			}
			clr := firstDrawingChild(doc, c)
			eff.Color = style.ResolveColor(doc, clr, env, docs, part, diags)
		case "softEdge":
			eff.Kind = EffectSoftEdge
			if v, ok := c.Attr("", "rad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = model.EMU(n)
				}
			}
		case "reflection":
			eff.Kind = EffectReflection
			if v, ok := c.Attr("", "blurRad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = model.EMU(n)
				}
			}
			if v, ok := c.Attr("", "stA"); ok {
				if f, e := strconv.ParseFloat(v, 64); e == nil {
					eff.StartOpacity = f / 100000.0
				}
			}
			if v, ok := c.Attr("", "stPos"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.Distance = model.EMU(n)
				}
			}
			if v, ok := c.Attr("", "endA"); ok {
				if f, e := strconv.ParseFloat(v, 64); e == nil {
					eff.EndOpacity = f / 100000.0
				}
			}
			if v, ok := c.Attr("", "endPos"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.OffsetY = model.EMU(n)
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
			eff.Color = style.ResolveColor(doc, clr, env, docs, part, diags)
			parseBlendModeAttrs(c, &eff)
		default:
			// a14:blur 等扩展暂归 unknown。
			eff.Kind = EffectUnknown
			*diags = append(*diags, diag.Diagnostic{
				Code: "effects.unknown_kind", Severity: diag.SeverityInfo,
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
func parseShadow(doc *xmlstore.XMLDocument, c *xmlstore.NodeRecord,
	env *style.Env, docs style.DocFunc, part string, eff *Effect, diags *[]diag.Diagnostic) {
	if v, ok := c.Attr("", "blurRad"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.BlurRadius = model.EMU(n)
		}
	}
	if v, ok := c.Attr("", "dist"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.Distance = model.EMU(n)
		}
	}
	if v, ok := c.Attr("", "dir"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.Angle = n
		}
	}
	if v, ok := c.Attr("", "sx"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.OffsetX = model.EMU(n)
		}
	}
	if v, ok := c.Attr("", "sy"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.OffsetY = model.EMU(n)
		}
	}
	if v, ok := c.Attr("", "algn"); ok {
		eff.Unknown = append(eff.Unknown, "algn="+v)
	}
	clr := firstDrawingChild(doc, c)
	eff.Color = style.ResolveColor(doc, clr, env, docs, part, diags)
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
func parseScene3D(doc *xmlstore.XMLDocument, scene *xmlstore.NodeRecord, diags *[]diag.Diagnostic) *Scene3DInfo {
	out := &Scene3DInfo{}
	if cam := xmlstore.ChildOfKind(doc, scene, ooxmlns.DrawingML, "camera", 0); cam != nil {
		out.Camera.Preset, _ = cam.Attr("", "prst")
		if v, ok := cam.Attr("", "zoom"); ok {
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				out.Camera.Zoom = NewOptional(f)
			}
		}
		if rot := xmlstore.ChildOfKind(doc, cam, ooxmlns.DrawingML, "rot", 0); rot != nil {
			out.Camera.Rot = axisTriplet(rot, []string{"lat", "lng", "rev"})
		}
	}
	if rig := xmlstore.ChildOfKind(doc, scene, ooxmlns.DrawingML, "lightRig", 0); rig != nil {
		out.LightRig.Rig, _ = rig.Attr("", "rig")
		if rot := xmlstore.ChildOfKind(doc, rig, ooxmlns.DrawingML, "rot", 0); rot != nil {
			out.LightRig.Dir = axisTriplet(rot, []string{"lat", "lng", "rev"})
		}
	}
	if bd := xmlstore.ChildOfKind(doc, scene, ooxmlns.DrawingML, "backdrop", 0); bd != nil {
		if v, ok := bd.Attr("", "plane"); ok {
			out.BackdropPlane = parseOptionalBool(v)
		}
	}
	return out
}

// parseShape3D 解析 a:sp3d（含 bevelT / bevelB / extrusionH / contourW）。
func parseShape3D(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord, diags *[]diag.Diagnostic) *Shape3DInfo {
	out := &Shape3DInfo{PresetMaterial: "warmMatte"}
	if v, ok := sp.Attr("", "prstMaterial"); ok {
		out.PresetMaterial = v
	}
	if v, ok := sp.Attr("", "extrusionH"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.ExtrusionH = model.EMU(n)
		}
	}
	if v, ok := sp.Attr("", "contourW"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.ContourW = model.EMU(n)
		}
	}
	if b := xmlstore.ChildOfKind(doc, sp, ooxmlns.DrawingML, "bevelT", 0); b != nil {
		out.TopBevel = parseBevel(doc, b)
	}
	if b := xmlstore.ChildOfKind(doc, sp, ooxmlns.DrawingML, "bevelB", 0); b != nil {
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
			out.Width = model.EMU(n)
		}
	}
	if v, ok := b.Attr("", "h"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Height = model.EMU(n)
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
