package pptx

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// formatSlideBody 含一组覆盖各深度项的形状：
//   - sp 含完整 a:ln（cap/cmpd/algn/prstDash/round/headEnd/tailEnd + solidFill+变换）
//   - sp 含完整 a:pPr（level/algn/indent/marL/marR/lnSpc+spcPct/spcBef+spcPts/spcAft+spcPts/tabLst+tab/buChar/buFont）
//   - sp 含 a:rPr 高级（baseline/spc/kern/cap/strike/u/lang/altLang/highlight+sym）
//   - sp 含 a:spPr/a:style + fillRef/lnRef/effectRef（引用 idx）
//   - sp 含 prstClr（unknown）+ 未识别变换
const formatSlideBody = `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:cSld><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	// id=2 完整 a:ln
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Shape 1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm>` +
	`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
	`<a:ln w="25400" cap="rnd" cmpd="dbl" algn="ctr">` +
	`<a:solidFill><a:srgbClr val="FF0000"><a:lumMod val="60000"/><a:lumOff val="20000"/></a:srgbClr></a:solidFill>` +
	`<a:prstDash val="dash"/><a:round/>` +
	`<a:headEnd type="triangle" w="med" len="lg"/>` +
	`<a:tailEnd type="arrow" w="sm" len="med"/>` +
	`<a:futureLineChild/>` +
	`</a:ln>` +
	`</p:spPr>` +
	`<p:txBody><a:bodyPr/><a:p><a:r><a:t>ln shape</a:t></a:r></a:p></p:txBody></p:sp>` +
	// id=3 含 a:pPr 全属性
	`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Shape 2"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p>` +
	`<a:pPr lvl="1" algn="ctr" indent="-100" marL="200" marR="300" rtl="1" eaLnBrk="1" fontAlgn="b" defTabSz="91440">` +
	`<a:lnSpc><a:spcPct val="150000"/></a:lnSpc>` +
	`<a:spcBef><a:spcPts val="600"/></a:spcBef>` +
	`<a:spcAft><a:spcPts val="400"/></a:spcAft>` +
	`<a:tabLst><a:tab pos="914400" algn="ctr"/><a:tab pos="1828800" algn="r"/></a:tabLst>` +
	`<a:buFont typeface="Wingdings"/><a:buChar char="&#xF0A7;"/>` +
	`</a:pPr>` +
	`<a:r><a:t>pPr shape</a:t></a:r>` +
	`</a:p></p:txBody></p:sp>` +
	// id=4 含 a:rPr 高级
	`<p:sp><p:nvSpPr><p:cNvPr id="4" name="Shape 3"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p>` +
	`<a:r><a:rPr baseline="30000" spc="200" cap="small" strike="dblStrike" u="sng" lang="en-US" altLang="zh-CN" kern="120" dirty="1" spellErr="1">` +
	`<a:highlight><a:srgbClr val="FFFF00"/></a:highlight>` +
	`<a:sym font="Wingdings" char="F0AB"/>` +
	`</a:rPr><a:t>rPr shape</a:t></a:r>` +
	`</a:p></p:txBody></p:sp>` +
	// id=5 含 a:style 矩阵引用
	`<p:sp><p:nvSpPr><p:cNvPr id="5" name="Shape 4"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr>` +
	`<a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm>` +
	`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
	`<a:style>` +
	`<a:lnRef idx="2"><a:schemeClr val="accent1"><a:shade val="75000"/></a:schemeClr></a:lnRef>` +
	`<a:fillRef idx="1"><a:schemeClr val="accent2"><a:tint val="50000"/></a:schemeClr></a:fillRef>` +
	`<a:effectRef idx="3"/>` +
	`</a:style>` +
	`<a:ln w="12700"><a:solidFill><a:srgbClr val="000000"/></a:solidFill></a:ln>` +
	`</p:spPr>` +
	`<p:txBody><a:bodyPr/><a:p><a:r><a:t>style ref</a:t></a:r></a:p></p:txBody></p:sp>` +
	// id=6 含 prstClr + 未识别变换（触发 STYLE_PARTIAL 而非 UNRESOLVED）
	`<p:sp><p:nvSpPr><p:cNvPr id="6" name="Shape 5"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm>` +
	`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
	`<a:ln w="12700">` +
	`<a:solidFill><a:prstClr val="black"><a:lumMod val="50000"/><a:futureTransform val="0"/></a:prstClr></a:solidFill>` +
	`</a:ln></p:spPr></p:sp>` +
	// id=7 含 a:scrgbClr 颜色（千分比 0..100000）
	// r=100000 → 255=FF；g=50000 → 127=7F；b=00000 → 0=00
	`<p:sp><p:nvSpPr><p:cNvPr id="7" name="Shape 6"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm>` +
	`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
	`<a:ln w="12700">` +
	`<a:solidFill><a:scrgbClr r="100000" g="50000" b="00000"/></a:solidFill>` +
	`</a:ln></p:spPr></p:sp>` +
	// id=8 含 a:hslClr 颜色（hue 1/60000 度；sat/lum 千分比 0..100000）
	// hue=0 sat=100000 lum=50000 → RGB FF0000
	`<p:sp><p:nvSpPr><p:cNvPr id="8" name="Shape 7"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm>` +
	`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
	`<a:ln w="12700">` +
	`<a:solidFill><a:hslClr hue="0" sat="100000" lum="50000"/></a:solidFill>` +
	`</a:ln></p:spPr></p:sp>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
	`</p:sld>`

// formatParts 构造含 formatSlideBody 与主题 fmtScheme（3 项 fill/ln/effect）的单页包 parts。
func formatParts() map[opc.PartName][]byte {
	parts := minimalTemplateParts()
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl + formatSlideBody)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
	parts["/[Content_Types].xml"] = []byte(string(parts["/[Content_Types].xml"])[:len(parts["/[Content_Types].xml"])-len("</Types>")] +
		`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`)
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`<Relationship Id="rId2" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	// 给主题 fmtScheme 三个槽位（idx=1/2/3）以便 fillRef/lnRef/effectRef 命中。
	theme := parts["/ppt/theme/theme1.xml"]
	theme = bytes.Replace(theme,
		[]byte("<a:fmtScheme "),
		[]byte(`<a:fmtScheme name="go-pptx">`+
			`<a:fillStyleLst><a:solidFill><a:schemeClr val="accent2"/></a:solidFill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill><a:solidFill><a:schemeClr val="accent3"/></a:solidFill></a:fillStyleLst>`+
			`<a:lnStyleLst><a:ln/><a:ln w="9525"/><a:ln w="19050"/></a:lnStyleLst>`+
			`<a:effectStyleLst><a:effectStyle/><a:effectStyle/><a:effectStyle/></a:effectStyleLst>`),
		1)
	parts["/ppt/theme/theme1.xml"] = theme
	return parts
}

// formatDeck 打开含 formatSlideBody 的单页文档。
func formatDeck(t *testing.T) *Presentation {
	t.Helper()
	return openFixture(t, buildPackageZipPanic(formatParts()))
}

// findShapeByID 返回 slide 上指定 id 的顶层形状。
func findShapeByID(t *testing.T, shapes []Shape, id ShapeID) Shape {
	t.Helper()
	for _, s := range shapes {
		if s.ID() == id {
			return s
		}
	}
	t.Fatalf("shape id=%d not found", id)
	return nil
}

// mustSlides 提取 Slides 错误为测试失败。
func mustSlides(t *testing.T, p *Presentation) []*Slide {
	t.Helper()
	s, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	return s
}

// mustSlideShapes 提取 Shapes 错误为测试失败。
func mustSlideShapes(t *testing.T, s *Slide) []Shape {
	t.Helper()
	sh, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	return sh
}

// firstParagraph 返回 TextFrame 的首段（测试便捷）。
func firstParagraph(t *testing.T, tf *TextFrame) *Paragraph {
	t.Helper()
	paras, err := tf.Paragraphs()
	if err != nil {
		t.Fatalf("Paragraphs: %v", err)
	}
	if len(paras) == 0 {
		t.Fatal("no paragraphs")
	}
	return paras[0]
}

// ---------- 颜色变换全集 ----------

func TestColorTransformLumModLumOff(t *testing.T) {
	// lumMod 60%：R/G/B *= 0.6 → FF*0.6=153（截断），00*0.6=0
	// lumOff 20%：R/G/B += 255*0.2=51
	// R: 153+51=204=CC；G: 0+51=51=33；B: 0+51=51=33
	out, _, unk := applyColorTransforms("FF0000", []ColorTransform{
		{Kind: "lumMod", Value: 60000}, {Kind: "lumOff", Value: 20000},
	})
	if len(unk) != 0 {
		t.Fatalf("unknown = %v", unk)
	}
	if out != "CC3333" {
		t.Errorf("lumMod+lumOff RGB = %s, want CC3333", out)
	}
}

func TestColorTransformTintShade(t *testing.T) {
	tint, _, _ := applyColorTransforms("800000", []ColorTransform{{Kind: "tint", Value: 50000}})
	// tint: x = x + (255-x)*val/100000（整数除法）
	// R: 128+(255-128)*50000/100000 = 128+63 = 191 = BF
	// G: 0+(255-0)*50000/100000 = 127 = 7F
	// B: 0+127 = 7F
	if tint != "BF7F7F" {
		t.Errorf("tint RGB = %s, want BF7F7F", tint)
	}
	shade, _, _ := applyColorTransforms("808080", []ColorTransform{{Kind: "shade", Value: 50000}})
	// shade: x = x*(100000-val)/100000
	// 128*(50000)/100000 = 64 = 40
	if shade != "404040" {
		t.Errorf("shade RGB = %s, want 404040", shade)
	}
}

func TestColorTransformAlpha(t *testing.T) {
	_, alpha, _ := applyColorTransforms("FF0000", []ColorTransform{{Kind: "alpha", Value: 50000}})
	if alpha != 0.5 {
		t.Errorf("alpha = %v, want 0.5", alpha)
	}
	// alphaMod=50% × 1 → 0.5；alphaOff=20% + 0.5 → 0.7
	_, alpha2, _ := applyColorTransforms("FF0000", []ColorTransform{
		{Kind: "alphaMod", Value: 50000}, {Kind: "alphaOff", Value: 20000},
	})
	if alpha2 < 0.69 || alpha2 > 0.71 {
		t.Errorf("alphaMod+Off = %v, want ≈0.7", alpha2)
	}
}

func TestColorTransformUnknown(t *testing.T) {
	// 未知变换前置 + lumMod 50%
	// R: 255*50000/100000 = 127 = 7F（整数除法截断）
	out, _, unk := applyColorTransforms("FF0000", []ColorTransform{
		{Kind: "futureTransform", Value: 0}, {Kind: "lumMod", Value: 50000},
	})
	if len(unk) != 1 || unk[0] != "futureTransform" {
		t.Fatalf("unknown = %v, want [futureTransform]", unk)
	}
	if out != "7F0000" {
		t.Errorf("partial apply RGB = %s, want 7F0000", out)
	}
}

func TestColorTransformPreservesUnchanged(t *testing.T) {
	out, _, _ := applyColorTransforms("AABBCC", nil)
	if out != "AABBCC" {
		t.Errorf("no-op transform RGB = %s, want AABBCC", out)
	}
}

// ---------- 线条系统 ----------

func TestShapeLineFull(t *testing.T) {
	p := formatDeck(t)
	shapes, err := mustSlides(t, p)[0].Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	line := findShapeByID(t, shapes, 2).Line
	lineShape, diags, err := line()
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if !lineShape.Specified {
		t.Error("line should be specified")
	}
	if lineShape.Width != 25400 {
		t.Errorf("width = %d, want 25400", lineShape.Width)
	}
	if lineShape.Cap != "rnd" || lineShape.Compound != "dbl" || lineShape.Align != "ctr" {
		t.Errorf("cap/cmpd/algn = %q/%q/%q", lineShape.Cap, lineShape.Compound, lineShape.Align)
	}
	if lineShape.Dash != "dash" {
		t.Errorf("dash = %q, want dash", lineShape.Dash)
	}
	if lineShape.Join != "round" {
		t.Errorf("join = %q, want round", lineShape.Join)
	}
	if !lineShape.HeadEnd.Specified || lineShape.HeadEnd.Type != "triangle" ||
		lineShape.HeadEnd.Width != "med" || lineShape.HeadEnd.Length != "lg" {
		t.Errorf("headEnd = %+v", lineShape.HeadEnd)
	}
	if !lineShape.TailEnd.Specified || lineShape.TailEnd.Type != "arrow" ||
		lineShape.TailEnd.Width != "sm" || lineShape.TailEnd.Length != "med" {
		t.Errorf("tailEnd = %+v", lineShape.TailEnd)
	}
	if lineShape.Color.RGB != "CC3333" {
		t.Errorf("color RGB = %s, want CC3333 (FF0000 lumMod=60%% lumOff=20%%)", lineShape.Color.RGB)
	}
	if !lineShape.Color.Resolved {
		t.Errorf("color Resolved = false, true")
	}
	if len(lineShape.Unknown) == 0 || lineShape.Unknown[0] != "futureLineChild" {
		t.Errorf("unknown = %v, want [futureLineChild]", lineShape.Unknown)
	}
	_ = diags // 未知 a:ln 子子不产生诊断（保留在 Unknown 字段）
}

func TestShapeLineMissing(t *testing.T) {
	p := formatDeck(t)
	shapes, _ := mustSlides(t, p)[0].Shapes()
	line, _, err := findShapeByID(t, shapes, 3).Line()
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if line.Specified {
		t.Error("shape id=3 has no line; expected Specified=false")
	}
}

// ---------- 特殊色空间（HslClr / ScrgbClr）端到端 ----------
//
// formatSlideBody 的 id=7 shape 的线条颜色用 a:scrgbClr（千分比 0..100000）：
//
//	r=100000 → 255=FF；g=50000 → 127=7F；b=00000 → 0=00
//
// 这是 format.go scrgbChannels (320 行起) 走到的实测路径——此前该函数
// 覆盖率为 0%（无人触发），roots 覆盖率贴线之一。
func TestShapeLineScrgbClr(t *testing.T) {
	p := formatDeck(t)
	shapes := mustSlideShapes(t, mustSlides(t, p)[0])
	line, diags, err := findShapeByID(t, shapes, 7).Line()
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if !line.Specified {
		t.Fatal("shape id=7 should have a line (scrgbClr)")
	}
	if line.Color.RGB != "FF7F00" {
		t.Errorf("scrgbClr RGB = %s, want FF7F00 (r=100%% g=50%% b=0%%)",
			line.Color.RGB)
	}
	if !line.Color.Resolved {
		t.Error("scrgbClr color Resolved = false, want true")
	}
	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %v", diags)
	}
}

// formatSlideBody 的 id=8 shape 的线条颜色用 a:hslClr：
//
//	hue=0 (1/60000 度 = 0°) sat=100000 (100%) lum=50000 (50%)
//	  → hslToRGB(0, 100000, 50000) → 纯红 (FF, 00, 00)
//
// 这是 format.go hslChannels (341 行起) 走到的实测路径——此前该函数
// 覆盖率为 0%。
func TestShapeLineHSLClr(t *testing.T) {
	p := formatDeck(t)
	shapes := mustSlideShapes(t, mustSlides(t, p)[0])
	line, diags, err := findShapeByID(t, shapes, 8).Line()
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if !line.Specified {
		t.Fatal("shape id=8 should have a line (hslClr)")
	}
	if line.Color.RGB != "FF0000" {
		t.Errorf("hslClr RGB = %s, want FF0000 (hue=0° sat=100%% lum=50%%)",
			line.Color.RGB)
	}
	if !line.Color.Resolved {
		t.Error("hslClr color Resolved = false, want true")
	}
	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %v", diags)
	}
}

// ---------- 段落属性全集 ----------

func TestParagraphPropsAll(t *testing.T) {
	p := formatDeck(t)
	tf, err := findShapeByID(t, mustSlideShapes(t, mustSlides(t, p)[0]), 3).(*AutoShape).TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	props, diags, err := firstParagraph(t, tf).Props()
	if err != nil {
		t.Fatalf("Props: %v", err)
	}
	if !props.Specified {
		t.Error("pPr not specified")
	}
	if props.Level != 1 || props.Align != "ctr" || props.Indent != -100 {
		t.Errorf("level/align/indent = %d/%q/%d", props.Level, props.Align, props.Indent)
	}
	if props.MarginLeft != 200 || props.MarginRight != 300 {
		t.Errorf("marL/marR = %d/%d", props.MarginLeft, props.MarginRight)
	}
	if !props.RTL || !props.EastAsianLineBreak {
		t.Errorf("rtl/eaLnBrk = %v/%v", props.RTL, props.EastAsianLineBreak)
	}
	if props.FontAlign != "b" || props.DefaultTabSize != 91440 {
		t.Errorf("fontAlgn/defTabSz = %q/%d", props.FontAlign, props.DefaultTabSize)
	}
	if props.LineSpacing == nil || props.LineSpacing.Kind != "pct" || props.LineSpacing.Value != 150000 {
		t.Errorf("lnSpc = %+v", props.LineSpacing)
	}
	if props.SpaceBefore == nil || props.SpaceBefore.Kind != "pts" || props.SpaceBefore.Value != 600 {
		t.Errorf("spcBef = %+v", props.SpaceBefore)
	}
	if props.SpaceAfter == nil || props.SpaceAfter.Kind != "pts" || props.SpaceAfter.Value != 400 {
		t.Errorf("spcAft = %+v", props.SpaceAfter)
	}
	if len(props.Tabs) != 2 || props.Tabs[0].Position != 914400 || props.Tabs[0].Align != "ctr" {
		t.Errorf("tabs = %+v", props.Tabs)
	}
	if props.Bullet == nil || props.Bullet.Kind != BulletChar {
		t.Fatalf("bullet = %+v", props.Bullet)
	}
	if props.Bullet.Font != "Wingdings" || props.Bullet.Char == "" {
		t.Errorf("bullet font/char = %q/%q", props.Bullet.Font, props.Bullet.Char)
	}
	if hasDiag(diags) {
		t.Logf("paragraph diags (informational): %+v", diags)
	}
}

func TestParagraphPropsMissing(t *testing.T) {
	p := formatDeck(t)
	tf, _ := findShapeByID(t, mustSlideShapes(t, mustSlides(t, p)[0]), 2).(*AutoShape).TextFrame()
	props, _, err := firstParagraph(t, tf).Props()
	if err != nil {
		t.Fatalf("Props: %v", err)
	}
	if props.Specified {
		t.Error("paragraph without pPr; expected Specified=false")
	}
}

// ---------- Run 高级属性 ----------

func TestRunAdvancedProps(t *testing.T) {
	p := formatDeck(t)
	tf, _ := findShapeByID(t, mustSlideShapes(t, mustSlides(t, p)[0]), 4).(*AutoShape).TextFrame()
	runs, _ := firstParagraph(t, tf).Runs()
	if len(runs) == 0 {
		t.Fatal("no runs")
	}
	props, _, err := runs[0].AdvancedProps()
	if err != nil {
		t.Fatalf("AdvancedProps: %v", err)
	}
	if props.Baseline != 30000 {
		t.Errorf("baseline = %d, want 30000", props.Baseline)
	}
	if props.Spacing != 2.0 {
		t.Errorf("spc = %v, want 2.0", props.Spacing)
	}
	if props.Kern != 1.2 {
		t.Errorf("kern = %v, want 1.2", props.Kern)
	}
	if props.Caps != "small" {
		t.Errorf("cap = %q", props.Caps)
	}
	if props.Strike != "dblStrike" {
		t.Errorf("strike = %q", props.Strike)
	}
	if props.Underline != "sng" {
		t.Errorf("u = %q", props.Underline)
	}
	if props.Language != "en-US" || props.AltLanguage != "zh-CN" {
		t.Errorf("lang/altLang = %q/%q", props.Language, props.AltLanguage)
	}
	if !props.Dirty || !props.SpellError {
		t.Errorf("dirty/spellErr = %v/%v", props.Dirty, props.SpellError)
	}
	if props.Highlight.RGB != "FFFF00" {
		t.Errorf("highlight = %s, want FFFF00", props.Highlight.RGB)
	}
	if props.Symbol == nil || props.Symbol.Font != "Wingdings" || props.Symbol.Char != "F0AB" {
		t.Errorf("sym = %+v", props.Symbol)
	}
}

func TestRunAdvancedPropsEmpty(t *testing.T) {
	p := formatDeck(t)
	tf, _ := findShapeByID(t, mustSlideShapes(t, mustSlides(t, p)[0]), 2).(*AutoShape).TextFrame()
	runs, _ := firstParagraph(t, tf).Runs()
	if len(runs) == 0 {
		t.Fatal("no runs")
	}
	props, _, err := runs[0].AdvancedProps()
	if err != nil {
		t.Fatalf("AdvancedProps: %v", err)
	}
	if props.Baseline != 0 || props.Symbol != nil || props.Highlight.RGB != "" {
		t.Errorf("empty run props = %+v", props)
	}
}

// ---------- 主题样式矩阵引用链 ----------

func TestStyleMatrixRefsResolved(t *testing.T) {
	p := formatDeck(t)
	shapes, _ := mustSlides(t, p)[0].Shapes()
	refs, _, err := findShapeByID(t, shapes, 5).StyleMatrixRefs()
	if err != nil {
		t.Fatalf("StyleMatrixRefs: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("refs = %d, want 3", len(refs))
	}
	wantKinds := []MatrixRefKind{RefLine, RefFill, RefEffect}
	wantIdx := []int32{2, 1, 3}
	for i, r := range refs {
		if r.Kind != wantKinds[i] {
			t.Errorf("refs[%d].Kind = %v, want %v", i, r.Kind, wantKinds[i])
		}
		if r.Index != wantIdx[i] {
			t.Errorf("refs[%d].Index = %d, want %d", i, r.Index, wantIdx[i])
		}
		if !r.Resolved {
			t.Errorf("refs[%d].Resolved = false", i)
		}
	}
	if refs[0].Color.RGB == "" {
		t.Errorf("lnRef color empty: %+v", refs[0].Color)
	}
	if refs[1].Color.RGB == "" {
		t.Errorf("fillRef color empty: %+v", refs[1].Color)
	}
}

func TestStyleMatrixRefsMissingIdx(t *testing.T) {
	parts := formatParts()
	// 注入 idx=99 的引用期望 unresolved。
	xml := strings.Replace(formatSlideBody,
		`<a:effectRef idx="3"/>`,
		`<a:effectRef idx="99"/>`, 1)
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl + xml)
	p := openFixture(t, buildPackageZipPanic(parts))
	shapes, _ := mustSlides(t, p)[0].Shapes()
	refs, _, err := findShapeByID(t, shapes, 5).StyleMatrixRefs()
	if err != nil {
		t.Fatalf("StyleMatrixRefs: %v", err)
	}
	var bad *StyleMatrixRef
	for i := range refs {
		if refs[i].Kind == RefEffect && refs[i].Index == 99 && !refs[i].Resolved {
			bad = &refs[i]
			break
		}
	}
	if bad == nil {
		t.Errorf("expected unresolved effectRef idx=99, got %+v", refs)
	}
}

// ---------- 颜色变换与诊断 ----------

func TestColorNodePresetUnknownDiagnostic(t *testing.T) {
	p := formatDeck(t)
	shapes, _ := mustSlides(t, p)[0].Shapes()
	line, diags, err := findShapeByID(t, shapes, 6).Line()
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if !line.Specified {
		t.Errorf("expected specified (a:ln 存在)")
	}
	// futureTransform 应触发 STYLE_PARTIAL（未知变换），不影响 lumMod。
	if !hasDiagCode(diags, "STYLE_PARTIAL") {
		t.Errorf("expected STYLE_PARTIAL, got %+v", diags)
	}
}

// ---------- 句柄生命周期 ----------

func TestFormatHandlesStaleAndClosed(t *testing.T) {
	p := formatDeck(t)
	shapes, _ := mustSlides(t, p)[0].Shapes()
	shape := findShapeByID(t, shapes, 2)
	// 关闭后所有深度读取应返回 ErrClosed。
	p.Close()
	if _, _, err := shape.Line(); !errors.Is(err, ErrClosed) {
		t.Errorf("Line after close = %v, want ErrClosed", err)
	}
}

// ---------- helpers ----------

func hasDiag(ds []Diagnostic) bool { return len(ds) > 0 }

// diagHasCode 由 style_test 提供，本处不重复定义。
