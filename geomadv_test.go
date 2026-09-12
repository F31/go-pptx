package pptx

import (
	"bytes"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// geomDeckWith 与 layoutDeckWith 同型，但仅定制 slide1.xml。返回的
// Presentation 已 Open，可直接走 Shape API。
func geomDeckWith(t *testing.T, spXML string) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	const nsP = nsPresentationML
	const nsA = nsDrawingML
	slide := []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsA + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		spXML +
		`</p:spTree></p:cSld>` +
		`</p:sld>`)
	parts["/ppt/slides/slide1.xml"] = slide
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rIdS1" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rIdS1"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	parts["/[Content_Types].xml"] = append(
		parts["/[Content_Types].xml"][:len(parts["/[Content_Types].xml"])-len("</Types>")],
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`)...)
	zipBytes, err := buildPackageZip(parts)
	if err != nil {
		t.Fatalf("buildPackageZip: %v", err)
	}
	p, err := OpenReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	return p
}

// findShapeByName 在第一张幻灯片上按 p:cNvPr@name 查找形状。
func findShapeByName(t *testing.T, p *Presentation, name string) Shape {
	t.Helper()
	slide, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := slide.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	for _, sh := range shapes {
		if sh.Name() == name {
			return sh
		}
	}
	t.Fatalf("shape %q not found on slide 0", name)
	return nil
}

// ---------- GeometryInfo 测试 ----------

// GEOM-02 验收 §1：prstGeom 直读 Preset。
func TestGeom_PresetGeom_BasicShape(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Rect"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1000000" cy="500000"/></a:xfrm>` +
		`<a:prstGeom prst="rect"/></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Rect")
	g, diags, err := sh.Geometry()
	if err != nil {
		t.Fatalf("Geometry: %v", err)
	}
	if g.Kind != GeometryPreset {
		t.Errorf("Kind = %v, want GeometryPreset", g.Kind)
	}
	if g.Preset != "rect" {
		t.Errorf("Preset = %q, want rect", g.Preset)
	}
	if len(g.Adjusts) != 0 {
		t.Errorf("Adjusts should be empty for plain rect, got %d", len(g.Adjusts))
	}
	if len(diags) != 0 {
		t.Errorf("diags should be empty for clean preset, got %v", diags)
	}
}

// GEOM-02 验收 §2：prstGeom 全部 adjust 解析（avLst/gd）。
func TestGeom_PresetGeom_WithAdjusts(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="RoundRect"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1000000" cy="500000"/></a:xfrm>` +
		`<a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val 16667"/>` +
		`<a:gd name="adj1" fmla="val 50000"/></a:avLst></a:prstGeom></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "RoundRect")
	g, _, err := sh.Geometry()
	if err != nil {
		t.Fatalf("Geometry: %v", err)
	}
	if g.Preset != "roundRect" {
		t.Errorf("Preset = %q", g.Preset)
	}
	if len(g.Adjusts) != 2 {
		t.Fatalf("Adjusts len = %d, want 2", len(g.Adjusts))
	}
	if g.Adjusts[0].Name != "adj" || g.Adjusts[0].Fmla != "val 16667" {
		t.Errorf("Adjusts[0] = %+v", g.Adjusts[0])
	}
	if g.Adjusts[1].Name != "adj1" || g.Adjusts[1].Fmla != "val 50000" {
		t.Errorf("Adjusts[1] = %+v", g.Adjusts[1])
	}
}

// GEOM-02 验收 §3：custGeom 路径与 guide 公式全集。
func TestGeom_CustomGeom_PathAndGuides(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Custom"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="2000000" cy="2000000"/></a:xfrm>` +
		`<a:custGeom>` +
		`<a:gdLst><a:gd name="myGuide" fmla="val 12345"/></a:gdLst>` +
		`<a:pathLst>` +
		`<a:path w="2000000" h="2000000" fill="solid" stroke="solid">` +
		`<a:moveTo><a:pt x="0" y="0"/></a:moveTo>` +
		`<a:lnTo><a:pt x="1000" y="2000"/></a:lnTo>` +
		`<a:cubicBezTo><a:pt x="1" y="2"/><a:pt x="3" y="4"/><a:pt x="5" y="6"/></a:cubicBezTo>` +
		`<a:quadBezTo><a:pt x="7" y="8"/><a:pt x="9" y="10"/></a:quadBezTo>` +
		`<a:close/>` +
		`</a:path>` +
		`</a:pathLst></a:custGeom></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Custom")
	g, _, err := sh.Geometry()
	if err != nil {
		t.Fatalf("Geometry: %v", err)
	}
	if g.Kind != GeometryCustom {
		t.Errorf("Kind = %v, want GeometryCustom", g.Kind)
	}
	if len(g.Guides) != 1 || g.Guides[0].Name != "myGuide" {
		t.Errorf("Guides = %+v", g.Guides)
	}
	if len(g.Paths) != 1 {
		t.Fatalf("Paths len = %d", len(g.Paths))
	}
	path := g.Paths[0]
	if path.Width != 2000000 || path.Height != 2000000 {
		t.Errorf("path size = (%d,%d)", path.Width, path.Height)
	}
	if path.Fill != "solid" || path.Stroke != "solid" {
		t.Errorf("path fill/stroke = (%q,%q)", path.Fill, path.Stroke)
	}
	if len(path.Commands) != 5 {
		t.Fatalf("Commands len = %d, want 5", len(path.Commands))
	}
	if path.Commands[0].Kind != "moveTo" || path.Commands[0].Points[0].X != 0 || path.Commands[0].Points[0].Y != 0 {
		t.Errorf("moveTo = %+v", path.Commands[0])
	}
	if path.Commands[1].Kind != "lnTo" || path.Commands[1].Points[0].X != 1000 {
		t.Errorf("lnTo = %+v", path.Commands[1])
	}
	if path.Commands[2].Kind != "cubicBezTo" || len(path.Commands[2].Points) != 3 {
		t.Errorf("cubicBezTo = %+v", path.Commands[2])
	}
	if path.Commands[3].Kind != "quadBezTo" || len(path.Commands[3].Points) != 2 {
		t.Errorf("quadBezTo = %+v", path.Commands[3])
	}
	if path.Commands[4].Kind != "close" {
		t.Errorf("close = %+v", path.Commands[4])
	}
}

// GEOM-02 验收 §4：缺 spPr 的形状返回 unknown + Warning 诊断，不报错。
func TestGeom_NoSpPr_Warning(t *testing.T) {
	sp := `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="2" name="Group"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/></p:grpSpSp>`
	// 修正上面缺失结尾标签
	sp = `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="2" name="Group"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/></p:grpSp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Group")
	g, diags, err := sh.Geometry()
	if err != nil {
		t.Fatalf("Geometry: %v", err)
	}
	if g.Kind != GeometryUnknown {
		t.Errorf("Kind = %v, want GeometryUnknown", g.Kind)
	}
	if len(diags) == 0 || diags[0].Code != "geom.spPr.missing" {
		t.Errorf("expected geom.spPr.missing diagnostic, got %v", diags)
	}
}

// GEOM-02 验收 §5：spPr 存在但无 prstGeom/custGeom → unknown，无诊断。
func TestGeom_SpPrWithoutGeometry(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Plain"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Plain")
	g, diags, err := sh.Geometry()
	if err != nil {
		t.Fatalf("Geometry: %v", err)
	}
	if g.Kind != GeometryUnknown {
		t.Errorf("Kind = %v, want GeometryUnknown", g.Kind)
	}
	if len(diags) != 0 {
		t.Errorf("expected no diags for plain spPr, got %v", diags)
	}
}

// ---------- FillInfo 测试 ----------

// GEOM-02 验收 §6：solidFill 颜色解析。
func TestFill_Solid(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Solid"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Solid")
	f, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillSolid {
		t.Errorf("Kind = %v, want FillSolid", f.Kind)
	}
	if f.Color.Spec.RGB != "FF0000" {
		t.Errorf("Color.Spec.RGB = %q", f.Color.Spec.RGB)
	}
}

// GEOM-02 验收 §7：noFill 类别。
func TestFill_None(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="NoFill"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:noFill/></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "NoFill")
	f, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillNone {
		t.Errorf("Kind = %v, want FillNone", f.Kind)
	}
}

// GEOM-02 验收 §8：linear gradient（lin + gsLst）。
func TestFill_LinearGradient(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Grad"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:gradFill flip="none" tileAlign="ctr" rotWithShape="1">` +
		`<a:gsLst><a:gs pos="0"><a:srgbClr val="FF0000"/></a:gs>` +
		`<a:gs pos="100000"><a:srgbClr val="0000FF"/></a:gs></a:gsLst>` +
		`<a:lin ang="5400000" scaled="1"/></a:gradFill></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Grad")
	f, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillGradient || f.Gradient == nil {
		t.Fatalf("Kind/Gradient = %v / %v", f.Kind, f.Gradient)
	}
	g := f.Gradient
	if g.Angle != 5400000 {
		t.Errorf("Angle = %d", g.Angle)
	}
	if !g.Scaled.Set || !g.Scaled.Value {
		t.Errorf("Scaled = %+v", g.Scaled)
	}
	if len(g.Stops) != 2 {
		t.Fatalf("Stops len = %d", len(g.Stops))
	}
	if g.Stops[0].Position != 0 || g.Stops[0].Color.Spec.RGB != "FF0000" {
		t.Errorf("Stops[0] = %+v", g.Stops[0])
	}
	if g.Stops[1].Position != 100000 || g.Stops[1].Color.Spec.RGB != "0000FF" {
		t.Errorf("Stops[1] = %+v", g.Stops[1])
	}
}

// GEOM-02 验收 §9：path gradient（rect fillToRect）。
func TestFill_PathGradient(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="PathGrad"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:gradFill><a:gsLst><a:gs pos="0"><a:srgbClr val="FFFFFF"/></a:gs>` +
		`<a:gs pos="100000"><a:srgbClr val="000000"/></a:gs></a:gsLst>` +
		`<a:path path="rect"><a:fillToRect l="50000" r="50000" t="50000" b="50000"/></a:path>` +
		`</a:gradFill></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "PathGrad")
	f, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillGradient || f.Gradient == nil {
		t.Fatalf("Kind/Gradient = %v / %v", f.Kind, f.Gradient)
	}
	g := f.Gradient
	if g.PathType != "rect" {
		t.Errorf("PathType = %q", g.PathType)
	}
	if g.PathLeft != 50000 || g.PathRight != 50000 || g.PathTop != 50000 || g.PathBottom != 50000 {
		t.Errorf("fillToRect = (%d,%d,%d,%d)", g.PathLeft, g.PathTop, g.PathRight, g.PathBottom)
	}
}

// GEOM-02 验收 §10：pattern fill (fg/bg + preset)。
func TestFill_Pattern(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Patt"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:pattFill prst="ltHorz">` +
		`<a:fgClr><a:srgbClr val="FF0000"/></a:fgClr>` +
		`<a:bgClr><a:srgbClr val="000000"/></a:bgClr>` +
		`</a:pattFill></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Patt")
	f, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillPattern || f.Pattern == nil {
		t.Fatalf("Kind/Pattern = %v / %v", f.Kind, f.Pattern)
	}
	pa := f.Pattern
	if pa.Preset != "ltHorz" {
		t.Errorf("Preset = %q", pa.Preset)
	}
	if pa.Foreground.Spec.RGB != "FF0000" {
		t.Errorf("Foreground = %+v", pa.Foreground)
	}
	if pa.Background.Spec.RGB != "000000" {
		t.Errorf("Background = %+v", pa.Background)
	}
}

// GEOM-02 验收 §11：blip fill（保留 rId + srcRect + dpi）。
func TestFill_Blip(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Blip"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:blipFill><a:blip r:embed="rIdImg1" dpi="96" rotWithShape="1"/>` +
		`<a:srcRect l="10000" r="20000" t="30000" b="40000"/>` +
		`<a:stretch/></a:blipFill></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Blip")
	f, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillPicture || f.Blip == nil {
		t.Fatalf("Kind/Blip = %v / %v", f.Kind, f.Blip)
	}
	bl := f.Blip
	if bl.RId != "rIdImg1" {
		t.Errorf("RId = %q", bl.RId)
	}
	if bl.Dpi != 96 {
		t.Errorf("Dpi = %d", bl.Dpi)
	}
	if !bl.RotateWithShape.Set || !bl.RotateWithShape.Value {
		t.Errorf("RotateWithShape = %+v", bl.RotateWithShape)
	}
	if bl.SrcLeft != 10000 || bl.SrcRight != 20000 || bl.SrcTop != 30000 || bl.SrcBottom != 40000 {
		t.Errorf("srcRect = (%d,%d,%d,%d)", bl.SrcLeft, bl.SrcTop, bl.SrcRight, bl.SrcBottom)
	}
}

// GEOM-02 验收 §12：grpFill 类别。
func TestFill_Group(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Grp"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:grpFill/></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Grp")
	f, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillGroup {
		t.Errorf("Kind = %v, want FillGroup", f.Kind)
	}
}

// GEOM-02 验收 §13：a:fill 容器不存在 → FillUnspecified + Warning。
func TestFill_NoFillContainer_Warning(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="NoFillSp"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "NoFillSp")
	f, diags, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillUnspecified {
		t.Errorf("Kind = %v, want FillUnspecified", f.Kind)
	}
	if len(diags) == 0 || diags[0].Code != "fill.spPr.missing" {
		t.Errorf("expected fill.spPr.missing, got %v", diags)
	}
}

// GEOM-02 验收 §14：未知填充类别 → FillUnspecified + Raw + 诊断。
func TestFill_UnknownKind_RawAndDiag(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Unk"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:weirdFill/></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Unk")
	f, diags, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if f.Kind != FillUnspecified || f.Raw != "weirdFill" {
		t.Errorf("Kind/Raw = %v / %q", f.Kind, f.Raw)
	}
	if !containsDiagCode(diags, "fill.unknown_kind") {
		t.Errorf("expected fill.unknown_kind, got %v", diags)
	}
}

// GEOM-02 验收 §15：gradFill 缺停止点 pos → 不臆造；提供诊断。
func TestFill_GradientInvalidPos_Warning(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="BadStop"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:gradFill><a:gsLst><a:gs pos="abc"><a:srgbClr val="FF0000"/></a:gs>` +
		`<a:gs pos="100000"><a:srgbClr val="0000FF"/></a:gs></a:gsLst>` +
		`<a:lin ang="0"/></a:gradFill></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "BadStop")
	_, diags, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if !containsDiagCode(diags, "fill.gradient.invalid_pos") {
		t.Errorf("expected fill.gradient.invalid_pos, got %v", diags)
	}
}

// ---------- EffectInfo 测试 ----------

// GEOM-02 验收 §16：outerShdw + blurRad/dist/dir/algn。
func TestEffect_OuterShadow(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Shadow"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:effectLst><a:outerShdw blurRad="40000" dist="20000" dir="5400000" algn="tl" sx="100000" sy="100000">` +
		`<a:srgbClr val="000000"><a:alpha val="50000"/></a:srgbClr></a:outerShdw></a:effectLst></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Shadow")
	e, _, err := sh.Effects()
	if err != nil {
		t.Fatalf("Effects: %v", err)
	}
	if e.Container != "effectLst" {
		t.Errorf("Container = %q", e.Container)
	}
	if len(e.Effects) != 1 || e.Effects[0].Kind != EffectOuterShadow {
		t.Fatalf("Effects = %+v", e.Effects)
	}
	eff := e.Effects[0]
	if eff.BlurRadius != 40000 || eff.Distance != 20000 || eff.Angle != 5400000 {
		t.Errorf("outerShdw attrs = (blur=%d dist=%d dir=%d)", eff.BlurRadius, eff.Distance, eff.Angle)
	}
	if eff.OffsetX != 100000 || eff.OffsetY != 100000 {
		t.Errorf("sx/sy = (%d,%d)", eff.OffsetX, eff.OffsetY)
	}
	if eff.Color.Spec.RGB != "000000" {
		t.Errorf("Color = %+v", eff.Color)
	}
	if eff.Alpha != 0.5 {
		t.Errorf("Alpha = %v", eff.Alpha)
	}
}

// GEOM-02 验收 §17：glow + softEdge（不同子集）。
func TestEffect_GlowAndSoftEdge(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Glow"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:effectLst><a:glow rad="80000"><a:srgbClr val="FF8800"/></a:glow>` +
		`<a:softEdge rad="50000"/></a:effectLst></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Glow")
	e, _, err := sh.Effects()
	if err != nil {
		t.Fatalf("Effects: %v", err)
	}
	if len(e.Effects) != 2 {
		t.Fatalf("Effects len = %d", len(e.Effects))
	}
	if e.Effects[0].Kind != EffectGlow || e.Effects[0].StandardDeviation != 80000 {
		t.Errorf("glow = %+v", e.Effects[0])
	}
	if e.Effects[0].Color.Spec.RGB != "FF8800" {
		t.Errorf("glow color = %+v", e.Effects[0].Color)
	}
	if e.Effects[1].Kind != EffectSoftEdge || e.Effects[1].StandardDeviation != 50000 {
		t.Errorf("softEdge = %+v", e.Effects[1])
	}
}

// GEOM-02 验收 §18：reflection 多属性全集。
func TestEffect_Reflection(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Reflect"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:effectLst><a:reflection blurRad="10000" stA="30000" stPos="20000" endA="10000" endPos="90000" dir="5400000" fadeDir="1000000"/>` +
		`<a:fillOverlay><a:srgbClr val="00FF00"/></a:fillOverlay></a:effectLst></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Reflect")
	e, _, err := sh.Effects()
	if err != nil {
		t.Fatalf("Effects: %v", err)
	}
	if len(e.Effects) != 2 {
		t.Fatalf("Effects len = %d", len(e.Effects))
	}
	r := e.Effects[0]
	if r.Kind != EffectReflection {
		t.Fatalf("Kind = %v, want Reflection", r.Kind)
	}
	if r.StandardDeviation != 10000 || r.Distance != 20000 || r.OffsetY != 90000 ||
		r.Direction != 5400000 || r.FadeDirection != 1000000 {
		t.Errorf("reflection attrs = %+v", r)
	}
	if r.StartOpacity != 0.3 || r.EndOpacity != 0.1 {
		t.Errorf("opacity = (%v, %v)", r.StartOpacity, r.EndOpacity)
	}
	// fillOverlay 第二条。
	if e.Effects[1].Kind != EffectFillOverlay || e.Effects[1].Color.Spec.RGB != "00FF00" {
		t.Errorf("fillOverlay = %+v", e.Effects[1])
	}
}

// GEOM-02 验收 §19：scene3d + sp3d 全字段。
func TestEffect_SceneAndShape3D(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="D3"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:scene3d><a:camera prst="orthographicFront" zoom="0.5">` +
		`<a:rot lat="0" lng="0" rev="0"/></a:camera>` +
		`<a:lightRig rig="threePt" dir="t"><a:rot lat="0" lng="0" rev="0"/></a:lightRig>` +
		`<a:backdrop plane="0"/>` +
		`<a:sp3d prstMaterial="warmMatte" extrusionH="100000" contourW="20000">` +
		`<a:bevelT prst="round" w="1000" h="2000"/>` +
		`<a:bevelB prst="cross" w="3000" h="4000"/>` +
		`</a:sp3d>` +
		`</a:scene3d></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "D3")
	e, _, err := sh.Effects()
	if err != nil {
		t.Fatalf("Effects: %v", err)
	}
	if e.Scene3D == nil || e.Scene3D.Camera.Preset != "orthographicFront" {
		t.Errorf("Scene3D.Camera = %+v", e.Scene3D)
	}
	if e.Scene3D.LightRig.Rig != "threePt" {
		t.Errorf("LightRig = %+v", e.Scene3D.LightRig)
	}
	if !e.Scene3D.BackdropPlane.Set || e.Scene3D.BackdropPlane.Value {
		t.Errorf("BackdropPlane = %+v", e.Scene3D.BackdropPlane)
	}
	if e.Shape3D == nil || e.Shape3D.PresetMaterial != "warmMatte" {
		t.Errorf("Shape3D = %+v", e.Shape3D)
	}
	if e.Shape3D.ExtrusionH != 100000 || e.Shape3D.ContourW != 20000 {
		t.Errorf("Shape3D ext/cont = (%d,%d)", e.Shape3D.ExtrusionH, e.Shape3D.ContourW)
	}
	if e.Shape3D.TopBevel.Preset != "round" || e.Shape3D.TopBevel.Width != 1000 || e.Shape3D.TopBevel.Height != 2000 {
		t.Errorf("TopBevel = %+v", e.Shape3D.TopBevel)
	}
	if e.Shape3D.BotBevel.Preset != "cross" || e.Shape3D.BotBevel.Width != 3000 || e.Shape3D.BotBevel.Height != 4000 {
		t.Errorf("BotBevel = %+v", e.Shape3D.BotBevel)
	}
}

// GEOM-02 验收 §20：效果容器缺失 → empty effect list, no error。
func TestEffect_NoEffectList(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="NoFx"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "NoFx")
	e, _, err := sh.Effects()
	if err != nil {
		t.Fatalf("Effects: %v", err)
	}
	if len(e.Effects) != 0 {
		t.Errorf("Effects should be empty, got %d", len(e.Effects))
	}
	if e.Container != "" {
		t.Errorf("Container should be empty, got %q", e.Container)
	}
}

// GEOM-02 验收 §21：FillInfo 解析全程不返回 error；调用方按 Diagnostic 处理。
func TestFill_ReturnsNoError(t *testing.T) {
	sp := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Any"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100000" cy="100000"/></a:xfrm>` +
		`<a:fill><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:fill></p:spPr></p:sp>`
	p := geomDeckWith(t, sp)
	defer p.Close()
	sh := findShapeByName(t, p, "Any")
	_, _, err := sh.Fill()
	if err != nil {
		t.Fatalf("Fill must not return error: %v", err)
	}
}

// ---------- 共享辅助 ----------

// containsDiagCode 检查 diags 中是否存在指定 code 的诊断条目。
func containsDiagCode(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

// ---------- 纯函数（零覆盖消除，2026-09-13 第 5 轮） ----------

// TestGeometryKindString 覆盖 GeometryKind.String 三分支。
func TestGeometryKindString(t *testing.T) {
	for _, tc := range []struct {
		in   GeometryKind
		want string
	}{
		{GeometryPreset, "preset"},
		{GeometryCustom, "custom"},
		{GeometryUnknown, "unknown"},
		{GeometryKind(99), "unknown"}, // 越界值回落 unknown
	} {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("GeometryKind(%d).String() = %q, want %q", int(tc.in), got, tc.want)
		}
	}
}

// TestEffectKindString 覆盖 EffectKind.String 全部已知类别 + unknown 回落。
func TestEffectKindString(t *testing.T) {
	for _, tc := range []struct {
		in   EffectKind
		want string
	}{
		{EffectOuterShadow, "outerShdw"},
		{EffectInnerShadow, "innerShdw"},
		{EffectGlow, "glow"},
		{EffectSoftEdge, "softEdge"},
		{EffectReflection, "reflection"},
		{EffectFillOverlay, "fillOverlay"},
		{EffectBlur, "blur"},
		{EffectUnknown, "unknown"},
		{EffectKind(99), "unknown"}, // 越界值回落 unknown
	} {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("EffectKind(%d).String() = %q, want %q", int(tc.in), got, tc.want)
		}
	}
}
