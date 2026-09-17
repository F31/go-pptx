package geometry

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

func mustIndex(t *testing.T, s string) *xmlstore.XMLDocument {
	t.Helper()
	d, err := xmlstore.Index([]byte(s))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	return d
}

func wrap(spPr string) string {
	return `<p:sp xmlns:p="` + ooxmlns.PresentationML + `" xmlns:a="` + ooxmlns.DrawingML + `">` + spPr + `</p:sp>`
}

func TestParseShapeGeometryPreset(t *testing.T) {
	doc := mustIndex(t, wrap(`<p:spPr><a:prstGeom prst="roundRect"><a:avLst>`+
		`<a:gd name="adj" fmla="val 25000"/>`+
		`<a:notGd/>`+
		`</a:avLst></a:prstGeom><a:xfrm/></p:spPr>`))
	info, diags, err := ParseShapeGeometry(doc, doc.Root())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Kind != GeometryPreset || info.Preset != "roundRect" {
		t.Fatalf("info = %+v", info)
	}
	if len(info.Adjusts) != 1 || info.Adjusts[0].Name != "adj" || info.Adjusts[0].Fmla != "val 25000" {
		t.Fatalf("adjusts = %+v", info.Adjusts)
	}
	if len(info.Unknown) != 1 || info.Unknown[0] != "notGd" {
		t.Fatalf("unknown = %v", info.Unknown)
	}
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
}

func TestParseShapeGeometryCustom(t *testing.T) {
	doc := mustIndex(t, wrap(`<p:spPr><a:custGeom>`+
		`<a:gdLst><a:gd name="g1" fmla="val 1"/><a:other/></a:gdLst>`+
		`<a:pathLst><a:path w="100" h="200" fill="none" stroke="solid">`+
		`<a:moveTo><a:pt x="1" y="2"/></a:moveTo>`+
		`<a:lnTo><a:pt x="3" y="4"/></a:lnTo>`+
		`<a:arcTo><a:pt x="5" y="6"/></a:arcTo>`+
		`<a:cubicBezTo><a:pt x="7" y="8"/><a:pt x="9" y="10"/><a:pt x="11" y="12"/></a:cubicBezTo>`+
		`<a:quadBezTo><a:pt x="13" y="14"/><a:pt x="15" y="16"/></a:quadBezTo>`+
		`<a:close/>`+
		`<a:mystery/>`+
		`</a:path></a:pathLst></a:custGeom></p:spPr>`))
	info, diags, err := ParseShapeGeometry(doc, doc.Root())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Kind != GeometryCustom {
		t.Fatalf("kind = %v", info.Kind)
	}
	if len(info.Guides) != 1 || info.Guides[0].Name != "g1" {
		t.Fatalf("guides = %+v", info.Guides)
	}
	if len(info.Paths) != 1 {
		t.Fatalf("paths = %+v", info.Paths)
	}
	p := info.Paths[0]
	if p.Width != 100 || p.Height != 200 || p.Fill != "none" || p.Stroke != "solid" {
		t.Fatalf("path = %+v", p)
	}
	kinds := []string{"moveTo", "lnTo", "arcTo", "cubicBezTo", "quadBezTo", "close"}
	if len(p.Commands) != len(kinds) {
		t.Fatalf("commands = %+v", p.Commands)
	}
	for i, k := range kinds {
		if p.Commands[i].Kind != k {
			t.Fatalf("command[%d].Kind = %q, want %q", i, p.Commands[i].Kind, k)
		}
	}
	if p.Commands[3].Points[2] != (Point{X: 11, Y: 12}) {
		t.Fatalf("cubicBez last point = %+v", p.Commands[3].Points[2])
	}
	if len(p.Unknown) != 1 || p.Unknown[0] != "mystery" {
		t.Fatalf("path unknown = %v", p.Unknown)
	}
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
}

func TestParseShapeGeometryNoSpPr(t *testing.T) {
	doc := mustIndex(t, wrap(``))
	info, diags, err := ParseShapeGeometry(doc, doc.Root())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Kind != GeometryUnknown {
		t.Fatalf("kind = %v", info.Kind)
	}
	if len(diags) != 1 || diags[0].Code != "geom.spPr.missing" {
		t.Fatalf("diags = %+v", diags)
	}
}

func TestParseShapeGeometrySpPrWithoutGeom(t *testing.T) {
	doc := mustIndex(t, wrap(`<p:spPr><a:ln/></p:spPr>`))
	info, diags, err := ParseShapeGeometry(doc, doc.Root())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Kind != GeometryUnknown || len(diags) != 0 {
		t.Fatalf("info=%+v diags=%+v", info, diags)
	}
}

func TestParseShapeGeometryInvalidPathDims(t *testing.T) {
	doc := mustIndex(t, wrap(`<p:spPr><a:custGeom><a:pathLst>`+
		`<a:path w="x" h="y"><a:moveTo/><a:cubicBezTo/><a:quadBezTo/></a:path>`+
		`</a:pathLst></a:custGeom></p:spPr>`))
	info, diags, err := ParseShapeGeometry(doc, doc.Root())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// w/h 非法 + moveTo/cubic/quad 缺 pt → 5 条 warning。
	codes := map[string]int{}
	for _, d := range diags {
		codes[d.Code]++
	}
	if codes["geom.custGeom.invalid_w"] != 1 || codes["geom.custGeom.invalid_h"] != 1 {
		t.Fatalf("codes = %v", codes)
	}
	if codes["geom.custGeom.missing_pt"] != 3 {
		t.Fatalf("missing_pt = %d, want 3 (%v)", codes["geom.custGeom.missing_pt"], codes)
	}
	if info.Paths[0].Width != 0 || info.Paths[0].Height != 0 {
		t.Fatalf("path = %+v", info.Paths[0])
	}
}

func TestParseShapeGeometryDuplicateGeomSibling(t *testing.T) {
	doc := mustIndex(t, wrap(`<p:spPr>`+
		`<a:prstGeom prst="rect"/>`+
		`<a:custGeom/>`+
		`<a:unknownGeom/>`+
		`</p:spPr>`))
	info, _, err := ParseShapeGeometry(doc, doc.Root())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Kind != GeometryPreset || info.Preset != "rect" {
		t.Fatalf("info = %+v", info)
	}
	if len(info.Unknown) != 1 || info.Unknown[0] != "custGeom" {
		t.Fatalf("unknown = %v", info.Unknown)
	}
}

func TestGeometryKindString(t *testing.T) {
	for _, tc := range []struct {
		k    GeometryKind
		want string
	}{
		{GeometryPreset, "preset"},
		{GeometryCustom, "custom"},
		{GeometryUnknown, "unknown"},
		{GeometryKind(99), "unknown"},
	} {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("GeometryKind(%d).String() = %q, want %q", tc.k, got, tc.want)
		}
	}
}
