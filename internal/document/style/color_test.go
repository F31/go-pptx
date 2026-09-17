package style

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/diag"
	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// ecmaColorTransformSet 是 ECMA-376 Part 1 EG_ColorTransform 全集 28 种。
var ecmaColorTransformSet = []string{
	"lumMod", "lumOff", "satMod", "satOff", "hueMod", "hueOff",
	"redMod", "redOff", "greenMod", "greenOff", "blueMod", "blueOff",
	"alphaMod", "alphaOff",
	"hue", "sat", "lum", "red", "green", "blue", "alpha",
	"comp", "inv", "gray",
	"gamma", "invGamma",
	"tint", "shade",
}

func TestTransformSetComplete(t *testing.T) {
	if len(ecmaColorTransformSet) != 28 {
		t.Fatalf("test fixture set size = %d, want 28", len(ecmaColorTransformSet))
	}
	for _, k := range ecmaColorTransformSet {
		if !knownTransformKinds[k] {
			t.Errorf("knownTransformKinds missing %q", k)
		}
	}
	set := map[string]bool{}
	for _, k := range ecmaColorTransformSet {
		set[k] = true
	}
	for k := range knownTransformKinds {
		if !set[k] {
			t.Errorf("knownTransformKinds has extra entry %q", k)
		}
	}
}

func colorIdx(t *testing.T, s string) *xmlstore.XMLDocument {
	t.Helper()
	d, err := xmlstore.Index([]byte(s))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	return d
}

func TestColorSpecStringValid(t *testing.T) {
	if (ColorSpec{Scheme: "accent1"}).String() != "scheme:accent1" {
		t.Fatal("scheme String")
	}
	if (ColorSpec{RGB: "FF0000"}).String() != "#FF0000" {
		t.Fatal("rgb String")
	}
	if (ColorSpec{}).String() != "" {
		t.Fatal("empty String")
	}
	if !(ColorSpec{RGB: "FF0000"}).Valid() || (ColorSpec{}).Valid() {
		t.Fatal("Valid")
	}
}

func TestIsHexRGBAndClrMapIndirect(t *testing.T) {
	for _, ok := range []string{"000000", "FFFFFF", "ABC123"} {
		if !IsHexRGB(ok) {
			t.Errorf("IsHexRGB(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "FFF", "fffffff", "GGGGGG", "ff0000"} {
		if IsHexRGB(bad) {
			t.Errorf("IsHexRGB(%q) = true", bad)
		}
	}
	for _, s := range []string{"bg1", "bg2", "tx1", "tx2"} {
		if !ClrMapIndirect(s) {
			t.Errorf("ClrMapIndirect(%q) = false", s)
		}
	}
	if ClrMapIndirect("accent1") {
		t.Fatal("accent1 should be direct")
	}
}

func TestApplyColorTransformsAllKnown(t *testing.T) {
	for _, k := range ecmaColorTransformSet {
		out, alpha, unknown := ApplyColorTransforms("336699", []ColorTransform{{Kind: k, Value: 50000}})
		if len(unknown) != 0 {
			t.Errorf("%s reported unknown", k)
		}
		if !IsHexRGB(out) || alpha < 0 || alpha > 1 {
			t.Errorf("%s out=%q alpha=%v", k, out, alpha)
		}
	}
	out, _, unknown := ApplyColorTransforms("336699", []ColorTransform{{Kind: "bogus"}})
	if len(unknown) != 1 || unknown[0] != "bogus" || out != "336699" {
		t.Fatalf("unknown: out=%q unknown=%v", out, unknown)
	}
	if out, _, _ := ApplyColorTransforms("zzz", nil); out != "zzz" {
		t.Fatalf("bad base = %q", out)
	}
}

func TestApplyColorTransformsValues(t *testing.T) {
	if out, _, _ := ApplyColorTransforms("000000", []ColorTransform{{Kind: "inv"}}); out != "FFFFFF" {
		t.Fatalf("inv = %q", out)
	}
	if out, _, _ := ApplyColorTransforms("FF0000", []ColorTransform{{Kind: "gray"}}); out != "4C4C4C" {
		t.Fatalf("gray = %q", out)
	}
	out, alpha, _ := ApplyColorTransforms("FFFFFF", []ColorTransform{{Kind: "alpha", Value: 50000}})
	if out != "FFFFFF" || alpha != 0.5 {
		t.Fatalf("alpha out=%q alpha=%v", out, alpha)
	}
	if out, _, _ := ApplyColorTransforms("FFFFFF", []ColorTransform{{Kind: "shade", Value: 50000}}); out == "FFFFFF" {
		t.Fatalf("shade no-op: %q", out)
	}
	if out, _, _ := ApplyColorTransforms("808080", []ColorTransform{{Kind: "gamma", Value: 0}}); out != "808080" {
		t.Fatalf("gamma0 = %q", out)
	}
}

func themeWithClr() string {
	return `<a:theme xmlns:a="` + ooxmlns.DrawingML + `"><a:themeElements><a:clrScheme>` +
		`<a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1>` +
		`<a:lt1><a:sysClr val="window"/></a:lt1>` +
		`<a:accent1><a:srgbClr val="4472C4"/></a:accent1>` +
		`<a:accent2><a:sysClr val="weird"/></a:accent2>` +
		`<a:accent3><a:schemeClr val="x"/></a:accent3>` +
		`<a:accent4><a:srgbClr val="112233"><a:alpha val="50000"/></a:srgbClr></a:accent4>` +
		`</a:clrScheme></a:themeElements></a:theme>`
}

func TestSchemeRGB(t *testing.T) {
	if _, p := SchemeRGB(nil, "accent1"); !p {
		t.Fatal("nil theme should be partial")
	}
	d := colorIdx(t, themeWithClr())
	if rgb, p := SchemeRGB(d, "accent1"); rgb != "4472C4" || p {
		t.Fatalf("accent1 = %q %v", rgb, p)
	}
	if rgb, p := SchemeRGB(d, "dk1"); rgb != "000000" || p {
		t.Fatalf("dk1 lastClr = %q %v", rgb, p)
	}
	if rgb, p := SchemeRGB(d, "lt1"); rgb != "FFFFFF" || p {
		t.Fatalf("lt1 fallback = %q %v", rgb, p)
	}
	if _, p := SchemeRGB(d, "accent2"); !p {
		t.Fatal("unknown sysClr should be partial")
	}
	if _, p := SchemeRGB(d, "accent3"); !p {
		t.Fatal("non srgb/sys should be partial")
	}
	if rgb, p := SchemeRGB(d, "accent4"); rgb != "112233" || !p {
		t.Fatalf("alpha partial = %q %v", rgb, p)
	}
	if _, p := SchemeRGB(d, "nope"); !p {
		t.Fatal("unknown scheme should be partial")
	}
	empty := colorIdx(t, `<a:theme xmlns:a="`+ooxmlns.DrawingML+`"/>`)
	if _, p := SchemeRGB(empty, "accent1"); !p {
		t.Fatal("no themeElements should be partial")
	}
}

func nodeAt(t *testing.T, inner, themeXML, masterXML string) (*xmlstore.XMLDocument, *xmlstore.NodeRecord, *xmlstore.XMLDocument, *xmlstore.XMLDocument) {
	t.Helper()
	doc := colorIdx(t, `<a:root xmlns:a="`+ooxmlns.DrawingML+`">`+inner+`</a:root>`)
	var td, md *xmlstore.XMLDocument
	if themeXML != "" {
		td = colorIdx(t, themeXML)
	}
	if masterXML != "" {
		md = colorIdx(t, masterXML)
	}
	return doc, doc.Node(doc.Root().Children[0]), td, md
}

func TestParseColorNodeKinds(t *testing.T) {
	parse := func(t *testing.T, inner, themeXML, masterXML string) (ParsedColor, []diag.Diagnostic) {
		t.Helper()
		doc, node, td, md := nodeAt(t, inner, themeXML, masterXML)
		var diags []diag.Diagnostic
		return ParseColorNode(doc, node, td, md, "/p", &diags), diags
	}

	var diags []diag.Diagnostic
	if out := ParseColorNode(colorIdx(t, `<a:root xmlns:a="`+ooxmlns.DrawingML+`"/>`), nil, nil, nil, "/p", &diags); out.Alpha != 1 {
		t.Fatal("nil clr")
	}

	out, _ := parse(t, `<a:srgbClr val="ff0000"><a:lumMod val="50000"/></a:srgbClr>`, "", "")
	if out.Spec.RGB != "FF0000" || !out.Resolved {
		t.Fatalf("srgb = %+v", out)
	}

	out, dg := parse(t, `<a:srgbClr val="zz"/>`, "", "")
	if out.Resolved || len(dg) == 0 {
		t.Fatalf("bad srgb = %+v %v", out, dg)
	}

	out, _ = parse(t, `<a:scrgbClr r="100000" g="0" b="0"/>`, "", "")
	if out.Spec.RGB != "FF0000" {
		t.Fatalf("scrgb = %+v", out)
	}

	out, _ = parse(t, `<a:hslClr hue="0" sat="100000" lum="50000"/>`, "", "")
	if !IsHexRGB(out.Spec.RGB) {
		t.Fatalf("hsl = %+v", out)
	}

	out, _ = parse(t, `<a:prstClr val="red"/>`, "", "")
	if out.Spec.RGB != "red" || out.RGB != "FF0000" {
		t.Fatalf("prst = %+v", out)
	}
	if _, dg = parse(t, `<a:prstClr val="nope"/>`, "", ""); len(dg) == 0 {
		t.Fatal("unknown preset should diagnose")
	}

	out, _ = parse(t, `<a:schemeClr val="accent1"/>`, themeWithClr(), "")
	if out.RGB != "4472C4" || !out.Resolved {
		t.Fatalf("scheme = %+v", out)
	}

	master := `<p:sldMaster xmlns:p="` + ooxmlns.PresentationML + `"><p:clrMap bg1="accent1"/></p:sldMaster>`
	out, _ = parse(t, `<a:schemeClr val="bg1"/>`, themeWithClr(), master)
	if out.RGB != "4472C4" {
		t.Fatalf("scheme indirect = %+v", out)
	}

	if _, dg = parse(t, `<a:schemeClr val="accent1"/>`, "", ""); len(dg) == 0 {
		t.Fatal("scheme without theme should diagnose")
	}

	out, _ = parse(t, `<a:sysClr val="x" lastClr="abcdef"/>`, "", "")
	if out.Spec.Scheme != "x" || out.RGB != "ABCDEF" {
		t.Fatalf("sysClr = %+v", out)
	}
	if _, dg = parse(t, `<a:sysClr val="nope"/>`, "", ""); len(dg) == 0 {
		t.Fatal("unknown sysClr should diagnose")
	}
	if _, dg = parse(t, `<a:bogusClr/>`, "", ""); len(dg) == 0 {
		t.Fatal("unsupported element should diagnose")
	}
	if _, dg = parse(t, `<a:srgbClr val="FF0000"><a:bogus val="1"/></a:srgbClr>`, "", ""); len(dg) == 0 {
		t.Fatal("unknown transform should diagnose")
	}
}
