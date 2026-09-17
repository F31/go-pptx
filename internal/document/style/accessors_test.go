package style

// 访问器解析的测试（v2.0 下沉后随迁）：ParseRunProps / ParseShapeLine /
// ParseStyleMatrixRefs 与主题矩阵引用链、phClr 替换、字体槽位解析。

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/diag"
	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/opc"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

const matrixThemeXML = `<a:theme xmlns:a="` + dd + `"><a:themeElements>` +
	`<a:clrScheme><a:accent1><a:srgbClr val="4472C4"/></a:accent1></a:clrScheme>` +
	`<a:fontScheme><a:majorFont><a:latin typeface="Calibri Light"/><a:ea typeface="DengXian"/></a:majorFont>` +
	`<a:minorFont><a:latin typeface="Calibri"/><a:cs typeface="Arial"/></a:minorFont></a:fontScheme>` +
	`<a:fmtScheme>` +
	`<a:fillStyleLst><a:solidFill><a:schemeClr val="accent1"/></a:solidFill>` +
	`<a:solidFill><a:schemeClr val="phClr"><a:tint val="50000"/></a:schemeClr></a:solidFill></a:fillStyleLst>` +
	`<a:lnStyleLst><a:ln><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:ln></a:lnStyleLst>` +
	`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
	`</a:fmtScheme></a:themeElements></a:theme>`

const matrixShapeXML = `<p:sp xmlns:p="` + pp + `" xmlns:a="` + dd + `"><p:spPr><a:style>` +
	`<a:fillRef idx="1"><a:schemeClr val="accent1"/></a:fillRef>` +
	`<a:lnRef idx="1"><a:schemeClr val="accent1"/></a:lnRef>` +
	`<a:effectRef idx="1"/><a:fontRef idx="major"/>` +
	`</a:style></p:spPr></p:sp>`

func matrixEnv(t *testing.T) (*Env, DocFunc, *xmlstore.XMLDocument) {
	t.Helper()
	theme := idx(t, matrixThemeXML)
	env := &Env{Kind: StyleKindSlide, Master: "/m.xml", Theme: "/t.xml"}
	parts := map[opc.PartName]*xmlstore.XMLDocument{env.Theme: theme}
	docs := func(p opc.PartName) *xmlstore.XMLDocument { return parts[p] }
	return env, docs, theme
}

func TestParseStyleMatrixRefs(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	sp := idx(t, matrixShapeXML)
	refs, diags := ParseStyleMatrixRefs(sp, sp.Root(), env, docs, "/p")
	if len(refs) != 4 {
		t.Fatalf("refs = %d: %+v", len(refs), refs)
	}
	fill := refs[0]
	if fill.Kind != RefFill || fill.Index != 1 || !fill.Resolved {
		t.Fatalf("fill = %+v", fill)
	}
	if fill.ThemeEntry != "schemeClr" {
		t.Fatalf("fill.ThemeEntry = %q", fill.ThemeEntry)
	}
	if !fill.ThemeColor.Resolved || fill.ThemeColor.RGB != "4472C4" {
		t.Fatalf("fill.ThemeColor = %+v", fill.ThemeColor)
	}
	ln := refs[1]
	if ln.Kind != RefLine || !ln.Resolved || ln.ThemeEntry != "solidFill" {
		t.Fatalf("ln = %+v", ln)
	}
	if eff := refs[2]; eff.Kind != RefEffect || !eff.Resolved || eff.ThemeEntry != "effectLst" {
		t.Fatalf("effectRef = %+v", eff)
	}
	font := refs[3]
	if font.Kind != RefFont || font.FontSlot != FontSlotMajor || font.ThemeTypeface != "Calibri Light" || !font.Resolved {
		t.Fatalf("fontRef = %+v", font)
	}
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
}

func TestParseStyleMatrixRefsPhClr(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	xml := `<p:sp xmlns:p="` + pp + `" xmlns:a="` + dd + `"><p:spPr><a:style>` +
		`<a:fillRef idx="2"><a:schemeClr val="accent1"/></a:fillRef></a:style></p:spPr></p:sp>`
	sp := idx(t, xml)
	refs, _ := ParseStyleMatrixRefs(sp, sp.Root(), env, docs, "/p")
	if len(refs) != 1 {
		t.Fatalf("refs = %d", len(refs))
	}
	r := refs[0]
	if !r.Resolved || !r.ThemeColor.Resolved {
		t.Fatalf("phClr fill = %+v", r)
	}
	// 基色 accent1=4472C4 代入 phClr + tint 50% → A1B8E1。
	if r.ThemeColor.RGB != "A1B8E1" {
		t.Fatalf("phClr RGB = %q, want A1B8E1", r.ThemeColor.RGB)
	}
}

func TestParseStyleMatrixRefsMissing(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	xml := `<p:sp xmlns:p="` + pp + `" xmlns:a="` + dd + `"><p:spPr><a:style>` +
		`<a:fillRef idx="9"/><a:fontRef idx="bogus"/></a:style></p:spPr></p:sp>`
	sp := idx(t, xml)
	refs, diags := ParseStyleMatrixRefs(sp, sp.Root(), env, docs, "/p")
	if len(refs) != 2 || refs[0].Resolved || refs[1].Resolved {
		t.Fatalf("refs = %+v", refs)
	}
	if refs[1].FontSlot != FontSlotUnknown || refs[1].ThemeTypeface != "" {
		t.Fatalf("bogus fontRef slot = %+v", refs[1])
	}
	if len(diags) == 0 {
		t.Fatal("expected STYLE_UNRESOLVED diags")
	}
}

func TestParseStyleMatrixRefsNoStyle(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	sp := idx(t, `<p:sp xmlns:p="`+pp+`"><p:spPr/></p:sp>`)
	if refs, _ := ParseStyleMatrixRefs(sp, sp.Root(), env, docs, "/p"); len(refs) != 0 {
		t.Fatalf("refs = %+v", refs)
	}
	if refs, _ := ParseStyleMatrixRefs(sp, idx(t, `<p:sp xmlns:p="`+pp+`"/>`).Root(), env, docs, "/p"); len(refs) != 0 {
		t.Fatalf("no spPr refs = %+v", refs)
	}
}

func TestThemeFontTypeface(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	for idxRaw, want := range map[string][2]any{
		"major": {FontSlotMajor, "Calibri Light"},
		"1":     {FontSlotMajor, "Calibri Light"},
		"minor": {FontSlotMinor, "Calibri"},
		"2":     {FontSlotMinor, "Calibri"},
	} {
		slot, tf := themeFontTypeface(docs, env, idxRaw)
		if slot != want[0] || tf != want[1] {
			t.Errorf("idx=%q slot=%v typeface=%q want %v/%v", idxRaw, slot, tf, want[0], want[1])
		}
	}
	if slot, _ := themeFontTypeface(docs, env, ""); slot != FontSlotUnknown {
		t.Errorf("empty slot = %v", slot)
	}
	if slot, _ := themeFontTypeface(docs, &Env{}, ""); slot != FontSlotUnknown {
		t.Errorf("no-theme slot = %v", slot)
	}
}

func TestParseRunProps(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	rPr := idx(t, `<a:rPr xmlns:a="`+dd+`" baseline="-25000" spc="300" cap="all" u="sng" lang="en-US" altLang="de-DE" kern="1200" dirty="1" spellErr="true" bogusAttr="x">`+
		`<a:highlight><a:schemeClr val="accent1"/></a:highlight>`+
		`<a:sym font="Wingdings" char="F0FF"/>`+
		`<a:solidFill><a:srgbClr val="FF0000"/></a:solidFill>`+
		`<a:weird/><a:extLst/></a:rPr>`)
	out, diags := ParseRunProps(rPr, rPr.Root(), env, docs, "/p")
	if out.Baseline != -25000 || out.Spacing != 3.0 || out.Caps != "all" || out.Underline != "sng" {
		t.Fatalf("attrs = %+v", out)
	}
	if out.Language != "en-US" || out.AltLanguage != "de-DE" || out.Kern != 12.0 {
		t.Fatalf("lang/kern = %+v", out)
	}
	if !out.Dirty || !out.SpellError {
		t.Fatalf("flags = %+v", out)
	}
	if !out.Highlight.Resolved || out.Highlight.RGB != "4472C4" {
		t.Fatalf("highlight = %+v", out.Highlight)
	}
	if out.Symbol == nil || out.Symbol.Font != "Wingdings" || out.Symbol.Char != "F0FF" {
		t.Fatalf("symbol = %+v", out.Symbol)
	}
	// solidFill/extLst 是已知子元素；bogusAttr 与 weird 入 Unknown。
	if len(out.Unknown) != 2 {
		t.Fatalf("unknown = %v, diags=%v", out.Unknown, diags)
	}
}

func TestParseRunPropsNil(t *testing.T) {
	if out, _ := ParseRunProps(nil, nil, nil, nil, "/p"); out.Baseline != 0 || out.Symbol != nil {
		t.Fatalf("nil rPr = %+v", out)
	}
}

func TestParseShapeLine(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	shape := idx(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:spPr><a:ln w="12700" cap="rnd" cmpd="dbl" algn="ctr">`+
		`<a:prstDash val="dash"/><a:miter lim="80000"/>`+
		`<a:headEnd type="triangle" w="med" len="lg"/>`+
		`<a:solidFill><a:schemeClr val="accent1"/></a:solidFill>`+
		`<a:weird/></a:ln></p:spPr></p:sp>`)
	out, diags := ParseShapeLine(shape, shape.Root(), env, docs, "/p")
	if !out.Specified || out.Width != 12700 || out.Cap != "rnd" || out.Compound != "dbl" || out.Align != "ctr" {
		t.Fatalf("line = %+v", out)
	}
	if out.Dash != "dash" || out.Join != "miter" || out.MiterLimit != 80000 {
		t.Fatalf("dash/join = %+v", out)
	}
	if !out.HeadEnd.Specified || out.HeadEnd.Type != "triangle" || out.HeadEnd.Width != "med" || out.HeadEnd.Length != "lg" {
		t.Fatalf("headEnd = %+v", out.HeadEnd)
	}
	if !out.Color.Resolved || out.Color.RGB != "4472C4" {
		t.Fatalf("color = %+v", out.Color)
	}
	if len(out.Unknown) != 1 || out.Unknown[0] != "weird" {
		t.Fatalf("unknown = %v, diags=%v", out.Unknown, diags)
	}
}

func TestParseShapeLineVariants(t *testing.T) {
	env, docs, _ := matrixEnv(t)
	custom := idx(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:spPr><a:ln>`+
		`<a:custDash/><a:bevel/><a:tailEnd type="oval"/>`+
		`<a:gradFill><a:gsLst/></a:gradFill></a:ln></p:spPr></p:sp>`)
	out, _ := ParseShapeLine(custom, custom.Root(), env, docs, "/p")
	if out.Dash != "custom" || out.Join != "bevel" || !out.TailEnd.Specified || out.TailEnd.Type != "oval" {
		t.Fatalf("variants = %+v", out)
	}
	if len(out.Unknown) != 1 || out.Unknown[0] != "gradFill" {
		t.Fatalf("gradFill should be unknown: %v", out.Unknown)
	}
	noSp := idx(t, `<p:sp xmlns:p="`+pp+`"/>`)
	if out, _ := ParseShapeLine(noSp, noSp.Root(), env, docs, "/p"); out.Specified {
		t.Fatal("no spPr should be unspecified")
	}
	noLn := idx(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:spPr/></p:sp>`)
	if out, _ := ParseShapeLine(noLn, noLn.Root(), env, docs, "/p"); out.Specified {
		t.Fatal("no ln should be unspecified")
	}
}

func TestDocFetchAndColorChild(t *testing.T) {
	if themeDocOf(nil, nil) != nil || masterDocOf(nil, nil) != nil {
		t.Fatal("nil env/docs should yield nil")
	}
	doc := idx(t, `<a:solidFill xmlns:a="`+dd+`"><a:srgbClr val="FF0000"/></a:solidFill>`)
	clr := ColorChild(doc, doc.Root())
	if clr == nil || clr.Local() != "srgbClr" {
		t.Fatal("color child expected")
	}
	if ColorChild(doc, nil) != nil {
		t.Fatal("nil fill")
	}
	env, docs, _ := matrixEnv(t)
	if themeDocOf(docs, env) == nil || masterDocOf(docs, env) != nil {
		t.Fatalf("themeDocOf = %v", themeDocOf(docs, env))
	}
}

func TestStyleEntryColorChild(t *testing.T) {
	theme := idx(t, matrixThemeXML)
	// fmtScheme → fillStyleLst 条目 a:solidFill → schemeClr（下探两层）。
	elems := xmlstore.ChildOfKind(theme, theme.Root(), ooxmlns.DrawingML, "themeElements", 0)
	fillStyle := xmlstore.ChildOfKind(theme, elems, ooxmlns.DrawingML, "fmtScheme", 0)
	fillLst := xmlstore.ChildOfKind(theme, fillStyle, ooxmlns.DrawingML, "fillStyleLst", 0)
	solid := xmlstore.ChildOfKind(theme, fillLst, ooxmlns.DrawingML, "solidFill", 0)
	clr := styleEntryColorChild(theme, solid)
	if clr == nil || clr.Local() != "schemeClr" {
		t.Fatalf("nested clr = %v", clr)
	}
	// 条目本身是颜色元素。
	if got := styleEntryColorChild(theme, clr); got != clr {
		t.Fatal("self color element")
	}
	if !isColorElement("sysClr") || isColorElement("solidFill") {
		t.Fatal("isColorElement")
	}
	none := idx(t, `<a:solidFill xmlns:a="`+dd+`"/>`)
	if got := styleEntryColorChild(none, none.Root()); got != nil {
		t.Fatal("no color found")
	}
}

func TestApplyPhClrTransformsUnresolvedBase(t *testing.T) {
	doc := idx(t, `<a:schemeClr val="phClr" xmlns:a="`+dd+`"/>`)
	var diags []diag.Diagnostic
	out, ok := applyPhClrTransforms(doc, doc.Root(), ParsedColor{}, &diags)
	if ok || out.RGB != "" {
		t.Fatalf("unresolved base should fail: %+v", out)
	}
}

func TestApplyPhClrTransformsUnknownTransform(t *testing.T) {
	doc := idx(t, `<a:schemeClr val="phClr" xmlns:a="`+dd+`"><a:bogusMod val="50000"/></a:schemeClr>`)
	var diags []diag.Diagnostic
	out, ok := applyPhClrTransforms(doc, doc.Root(), ParsedColor{RGB: "FF0000", Alpha: 0.5}, &diags)
	if !ok || out.RGB == "" || out.Resolved {
		t.Fatalf("out = %+v (unknown transform must stay unresolved)", out)
	}
	if out.Alpha != 0.5 {
		t.Fatalf("alpha should carry ref alpha: %v", out.Alpha)
	}
	if len(diags) != 1 || diags[0].Code != "STYLE_PARTIAL" {
		t.Fatalf("diags = %+v", diags)
	}
}
