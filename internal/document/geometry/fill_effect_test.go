package geometry

// 填充/效果解析的测试（根包 geom_fill.go / geom_effect.go 下沉后随迁）：
// ParseShapeFill 全形态（solid/grad/patt/blip/grp/noFill/未知/缺 spPr）、
// ParseShapeEffects（阴影/发光/柔边/反射/fillOverlay/unknown/scene3d/sp3d）。

import (
	"testing"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

const dd = ooxmlns.DrawingML
const pp = ooxmlns.PresentationML

func fillTheme(t *testing.T) (*style.Env, style.DocFunc) {
	t.Helper()
	theme := mustIndex(t, `<a:theme xmlns:a="`+dd+`"><a:themeElements><a:clrScheme>`+
		`<a:accent1><a:srgbClr val="4472C4"/></a:accent1>`+
		`</a:clrScheme></a:themeElements></a:theme>`)
	env := &style.Env{Kind: style.StyleKindSlide, Theme: "/t.xml"}
	return env, func(p opc.PartName) *xmlstore.XMLDocument {
		if p == env.Theme {
			return theme
		}
		return nil
	}
}

func spWrap(spPr string) string {
	return `<p:sp xmlns:p="` + pp + `" xmlns:a="` + dd + `">` + spPr + `</p:sp>`
}

func TestParseShapeFill(t *testing.T) {
	env, docs := fillTheme(t)
	cases := []struct {
		name string
		spPr string
		want *FillInfo
		dg   string
	}{
		{"solid", `<p:spPr><a:fill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:fill></p:spPr>`,
			&FillInfo{Kind: style.FillSolid}, ""},
		{"noFill", `<p:spPr><a:fill><a:noFill/></a:fill></p:spPr>`, &FillInfo{Kind: style.FillNone}, ""},
		{"grpFill", `<p:spPr><a:fill><a:grpFill/></a:fill></p:spPr>`, &FillInfo{Kind: style.FillGroup}, ""},
		{"blip", `<p:spPr><a:fill><a:blipFill/></a:fill></p:spPr>`, &FillInfo{Kind: style.FillPicture}, ""},
		{"unknown-kind", `<p:spPr><a:fill><a:weirdFill/></a:fill></p:spPr>`, &FillInfo{Kind: style.FillUnspecified, Raw: "weirdFill"}, "fill.unknown_kind"},
		{"missing-fill", `<p:spPr/>`, nil, "fill.spPr.missing"},
	}
	for _, tc := range cases {
		raw := tc.spPr
		if tc.name != "missing-fill" {
			raw = spWrap(tc.spPr)
		}
		doc := mustIndex(t, raw)
		out, diags := ParseShapeFill(doc, doc.Root(), env, docs, "/p")
		if tc.want == nil {
			if out.Kind != style.FillUnspecified || len(diags) == 0 {
				t.Errorf("%s: %+v %v", tc.name, out, diags)
			}
			continue
		}
		if out.Kind != tc.want.Kind || out.Raw != tc.want.Raw {
			t.Errorf("%s: kind=%v raw=%q want %+v", tc.name, out.Kind, out.Raw, tc.want)
		}
		if tc.name == "solid" && !out.Color.Resolved || tc.name == "solid" && out.Color.RGB != "4472C4" {
			t.Errorf("%s: color=%+v", tc.name, out.Color)
		}
	}
	// 多填充冲突 → 仅首个生效 + Unknown。
	doc := mustIndex(t, spWrap(`<p:spPr><a:fill><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill><a:noFill/></a:fill></p:spPr>`))
	out, _ := ParseShapeFill(doc, doc.Root(), env, docs, "/p")
	if out.Kind != style.FillSolid || len(out.Unknown) != 1 || out.Unknown[0] != "noFill" {
		t.Fatalf("conflict fill = %+v", out)
	}
	if out.Color.RGB != "FF0000" {
		t.Fatalf("solid color = %+v", out.Color)
	}
}

func TestParseGradFill(t *testing.T) {
	env, docs := fillTheme(t)
	doc := mustIndex(t, spWrap(`<p:spPr><a:fill><a:gradFill flip="xy" rotWithShape="0">`+
		`<a:gsLst><a:gs pos="0"><a:schemeClr val="accent1"/></a:gs>`+
		`<a:gs pos="100000"><a:srgbClr val="000000"/></a:gs></a:gsLst>`+
		`<a:lin ang="5400000" scaled="0"/></a:gradFill></a:fill></p:spPr>`))
	out, _ := ParseShapeFill(doc, doc.Root(), env, docs, "/p")
	if out.Kind != style.FillGradient || out.Gradient == nil {
		t.Fatalf("grad = %+v", out)
	}
	g := out.Gradient
	if g.Flip != "xy" || g.RotateWithShape.Value || g.Angle != 5400000 || g.Scaled.Value {
		t.Fatalf("grad attrs = %+v", g)
	}
	if len(g.Stops) != 2 || g.Stops[0].Position != 0 || !g.Stops[0].Color.Resolved || g.Stops[0].Color.RGB != "4472C4" {
		t.Fatalf("stops = %+v", g.Stops)
	}
	if g.Stops[1].Color.RGB != "000000" {
		t.Fatalf("stop1 color = %+v", g.Stops[1].Color)
	}
	// path + fillToRect。
	doc2 := mustIndex(t, spWrap(`<p:spPr><a:fill><a:gradFill>`+
		`<a:gsLst><a:gs pos="xyz"><a:srgbClr val="111111"/></a:gs></a:gsLst>`+
		`<a:path path="circle"><a:fillToRect l="0" r="50000" t="25000" b="75000"/></a:path>`+
		`<a:tileRect/><a:unknownGrad/></a:gradFill></a:fill></p:spPr>`))
	out2, diags2 := ParseShapeFill(doc2, doc2.Root(), env, docs, "/p")
	if out2.Gradient.PathType != "circle" || out2.Gradient.PathRight != 50000 || out2.Gradient.PathTop != 25000 {
		t.Fatalf("path grad = %+v", out2.Gradient)
	}
	if out2.Gradient.Stops[0].Position != -1 {
		t.Fatalf("invalid pos stop = %+v", out2.Gradient.Stops[0])
	}
	if got := findDiag(diags2, "fill.gradient.invalid_pos"); !got {
		t.Fatalf("want invalid_pos diag, got %+v", diags2)
	}
	if len(out2.Gradient.Unknown) != 1 || out2.Gradient.Unknown[0] != "unknownGrad" {
		t.Fatalf("grad unknown = %+v", out2.Gradient.Unknown)
	}
	_ = findDiag
}

func TestParsePattFill(t *testing.T) {
	env, docs := fillTheme(t)
	doc := mustIndex(t, spWrap(`<p:spPr><a:fill><a:pattFill prst="pct20">`+
		`<a:fgClr><a:srgbClr val="112233"/></a:fgClr><a:bgClr><a:schemeClr val="accent1"/></a:bgClr>`+
		`<a:weird/></a:pattFill></a:fill></p:spPr>`))
	out, _ := ParseShapeFill(doc, doc.Root(), env, docs, "/p")
	if out.Kind != style.FillPattern || out.Pattern.Preset != "pct20" {
		t.Fatalf("patt = %+v", out.Pattern)
	}
	if out.Pattern.Foreground.RGB != "112233" || out.Pattern.Background.RGB != "4472C4" {
		t.Fatalf("patt colors = %+v", out.Pattern)
	}
	if len(out.Pattern.Unknown) != 1 {
		t.Fatalf("patt unknown = %+v", out.Pattern.Unknown)
	}
}

func TestParseBlipFill(t *testing.T) {
	env, docs := fillTheme(t)
	doc := mustIndex(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`" xmlns:r="`+ooxmlns.OfficeDocument+`"><p:spPr><a:fill><a:blipFill><a:blip r:embed="rId7" dpi="150" rotWithShape="1"/>`+
		`<a:srcRect l="10000" r="20000" t="30000" b="40000"/><a:tile/><a:weird/></a:blipFill></a:fill></p:spPr></p:sp>`)
	out, diags := ParseShapeFill(doc, doc.Root(), env, docs, "/p")
	if out.Blip == nil || out.Blip.RId != "rId7" || out.Blip.Dpi != 150 || !out.Blip.RotateWithShape.Value {
		t.Fatalf("blip = %+v", out.Blip)
	}
	if out.Blip.SrcLeft != 10000 || out.Blip.SrcBottom != 40000 {
		t.Fatalf("srcRect = %+v", out.Blip)
	}
	if len(diags) != 1 || diags[0].Code != "fill.blip.unknown_child" {
		t.Fatalf("diags = %+v", diags)
	}
}

func TestParseShapeEffects(t *testing.T) {
	env, docs := fillTheme(t)
	doc := mustIndex(t, spWrap(`<p:spPr><a:effectLst>`+
		`<a:outerShdw blurRad="50000" dist="100000" dir="5400000" sx="1000" sy="2000" algn="tl" blend="mult"><a:schemeClr val="accent1"/></a:outerShdw>`+
		`<a:softEdge rad="80000" hideSelf="1"/>`+
		`<a:glow rad="60000"><a:srgbClr val="FF00FF"/></a:glow>`+
		`<a:reflection blurRad="10000" stA="40000" stPos="2000" endA="0" endPos="8000" dir="0" fadeDir="0"/>`+
		`<a:fillOverlay blend="screen"><a:srgbClr val="00FF00"/></a:fillOverlay>`+
		`<a:blur blurRad="1000"/>`+
		`<a:innerShdw blurRad="30000"><a:srgbClr val="000000"/></a:innerShdw>`+
		`</a:effectLst><a:scene3d><a:camera prst="perspectiveContrastingRightFacing" zoom="200000"><a:rot lat="5400000" lng="0" rev="0"/></a:camera>`+
		`<a:lightRig rig="threePt" dir="t"><a:rot lng="0" rev="0"/></a:lightRig>`+
		`<a:backdrop plane="1"/>`+
		`<a:sp3d prstMaterial="plastic" extrusionH="100000" contourW="50000">`+
		`<a:bevelT prst="relaxedInset" w="10000" h="20000"/>`+
		`<a:bevelB prst="circle" w="3000" h="4000"/></a:sp3d></a:scene3d></p:spPr>`))
	out, diags := ParseShapeEffects(doc, doc.Root(), env, docs, "/p")
	if out.Container != "effectLst" {
		t.Fatalf("container = %q", out.Container)
	}
	if len(out.Effects) != 7 {
		t.Fatalf("effects = %d: %+v", len(out.Effects), out.Effects)
	}
	sh := out.Effects[0]
	if sh.Kind != EffectOuterShadow || sh.BlurRadius != 50000 || sh.Distance != 100000 || sh.Angle != 5400000 ||
		sh.OffsetX != 1000 || sh.OffsetY != 2000 || sh.BlendMode != "mult" || !sh.Color.Resolved || sh.Color.RGB != "4472C4" {
		t.Fatalf("outerShdw = %+v", sh)
	}
	if len(sh.Unknown) != 1 || sh.Unknown[0] != "algn=tl" {
		t.Fatalf("outerShdw unknown = %+v", sh.Unknown)
	}
	if out.Effects[1].Kind != EffectSoftEdge || out.Effects[1].StandardDeviation != 80000 || !out.Effects[1].Hidden {
		t.Fatalf("softEdge = %+v", out.Effects[1])
	}
	if out.Effects[2].Kind != EffectGlow || out.Effects[2].Color.RGB != "FF00FF" {
		t.Fatalf("glow = %+v", out.Effects[2])
	}
	refl := out.Effects[3]
	if refl.Kind != EffectReflection || refl.StartOpacity != 0.4 || refl.EndOpacity != 0 || refl.Distance != 2000 || refl.OffsetY != 8000 {
		t.Fatalf("reflection = %+v", refl)
	}
	if out.Effects[4].Kind != EffectFillOverlay || out.Effects[4].BlendMode != "screen" || out.Effects[4].Color.RGB != "00FF00" {
		t.Fatalf("fillOverlay = %+v", out.Effects[4])
	}
	if out.Effects[5].Kind != EffectUnknown {
		t.Fatalf("blur (unknown) = %+v", out.Effects[5])
	}
	if out.Scene3D == nil || out.Scene3D.Camera.Preset != "perspectiveContrastingRightFacing" || out.Scene3D.Camera.Zoom.Value != 200000 ||
		len(out.Scene3D.Camera.Rot) != 3 || out.Scene3D.Camera.Rot[0] != 5400000 ||
		out.Scene3D.LightRig.Rig != "threePt" || out.Scene3D.LightRig.Dir[0] != 0 || !out.Scene3D.BackdropPlane.Value {
		t.Fatalf("scene3d = %+v", out.Scene3D)
	}
	if out.Shape3D == nil || out.Shape3D.ExtrusionH != 100000 || out.Shape3D.ContourW != 50000 || out.Shape3D.PresetMaterial != "plastic" ||
		out.Shape3D.TopBevel.Preset != "relaxedInset" || out.Shape3D.TopBevel.Width != 10000 || out.Shape3D.BotBevel.Preset != "circle" {
		t.Fatalf("sp3d = %+v", out.Shape3D)
	}
	_ = diags
}

func TestParseShapeEffectsMissing(t *testing.T) {
	env, docs := fillTheme(t)
	doc := mustIndex(t, spWrap(``))
	out, diags := ParseShapeEffects(doc, doc.Root(), env, docs, "/p")
	if len(diags) == 0 || diags[0].Code != "effects.spPr.missing" {
		t.Fatalf("diags = %+v", diags)
	}
	if out.Container != "" {
		t.Fatalf("container = %q", out.Container)
	}
}

func TestParseOptionalBoolAndHelpers(t *testing.T) {
	if v := parseOptionalBool("1"); !v.Set || !v.Value {
		t.Fatal("1")
	}
	if v := parseOptionalBool("FALSE"); !v.Set || v.Value {
		t.Fatal("FALSE")
	}
	if v := parseOptionalBool("x"); v.Set {
		t.Fatal("x should be unset")
	}
	doc := mustIndex(t, `<a:solidFill xmlns:a="`+dd+`"><a:srgbClr val="010203"/></a:solidFill>`)
	if c := firstDrawingChild(doc, doc.Root()); c == nil || c.Local() != "srgbClr" {
		t.Fatal("firstDrawingChild")
	}
	if p := percentAttr(doc, doc.Root(), "missing"); p != -1 {
		t.Fatal("percentAttr missing")
	}
	if style.ColorChild(doc, doc.Root()) == nil {
		t.Fatal("ColorChild")
	}
}

func findDiag(diags []diag.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
