package style

import "testing"

const (
	nsP = "http://schemas.openxmlformats.org/presentationml/2006/main"
	nsA = "http://schemas.openxmlformats.org/drawingml/2006/main"
)

func TestPhKeyOf(t *testing.T) {
	// 显式 type/idx。
	d := mustIndex(t, `<p:sp xmlns:p="`+nsP+`"><p:nvSpPr><p:nvPr><p:ph type="title" idx="1"/></p:nvPr></p:nvSpPr></p:sp>`)
	k, ok := PhKeyOf(d, d.Root())
	if !ok || k.Typ != "title" || k.Idx != 1 {
		t.Fatalf("PhKeyOf = %+v %v", k, ok)
	}
	// 缺省 type="obj" / idx=0。
	d2 := mustIndex(t, `<p:sp xmlns:p="`+nsP+`"><p:nvSpPr><p:nvPr><p:ph/></p:nvPr></p:nvSpPr></p:sp>`)
	k2, ok2 := PhKeyOf(d2, d2.Root())
	if !ok2 || k2.Typ != "obj" || k2.Idx != 0 {
		t.Fatalf("default PhKeyOf = %+v %v", k2, ok2)
	}
	// idx 非数字 → 回落 0。
	dBad := mustIndex(t, `<p:sp xmlns:p="`+nsP+`"><p:nvSpPr><p:nvPr><p:ph idx="x"/></p:nvPr></p:nvSpPr></p:sp>`)
	if got, ok := PhKeyOf(dBad, dBad.Root()); !ok || got.Idx != 0 {
		t.Fatalf("bad idx PhKeyOf = %+v %v", got, ok)
	}
	// 无 nvSpPr / 无 nvPr / 无 ph → ok=false。
	for _, doc := range []string{
		`<p:sp xmlns:p="` + nsP + `"/>`,
		`<p:sp xmlns:p="` + nsP + `"><p:nvSpPr/></p:sp>`,
		`<p:sp xmlns:p="` + nsP + `"><p:nvSpPr><p:nvPr/></p:nvSpPr></p:sp>`,
	} {
		dd := mustIndex(t, doc)
		if _, ok := PhKeyOf(dd, dd.Root()); ok {
			t.Fatalf("expected not placeholder: %s", doc)
		}
	}
}

func TestFindPlaceholderShape(t *testing.T) {
	d := mustIndex(t, `<p:sld xmlns:p="`+nsP+`"><p:cSld><p:spTree>`+
		`<p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr></p:sp>`+
		`<p:sp><p:nvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr></p:sp>`+
		`</p:spTree></p:cSld></p:sld>`)
	got := FindPlaceholderShape(d, PhKey{Typ: "body", Idx: 1})
	if got == nil || got.Local() != "sp" {
		t.Fatalf("FindPlaceholderShape = %+v", got)
	}
	if FindPlaceholderShape(d, PhKey{Typ: "nope", Idx: 0}) != nil {
		t.Fatal("miss should be nil")
	}
}

func TestAncestorShapeAndRunPara(t *testing.T) {
	d := mustIndex(t, `<p:sld xmlns:p="`+nsP+`" xmlns:a="`+nsA+`"><p:cSld><p:spTree><p:sp>`+
		`<p:txBody><a:p><a:r/></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`)
	runs := d.Elements(nsA, "r")
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	run := d.Node(runs[0])
	sp := AncestorShape(d, run)
	if sp == nil || sp.Local() != "sp" {
		t.Fatalf("AncestorShape = %+v", sp)
	}
	para := RunPara(d, run)
	if para == nil || para.Local() != "p" {
		t.Fatalf("RunPara = %+v", para)
	}
	// 无 sp 祖先的孤立 run。
	orphan := mustIndex(t, `<a:r xmlns:a="`+nsA+`"/>`)
	if AncestorShape(orphan, orphan.Root()) != nil {
		t.Fatal("orphan AncestorShape should be nil")
	}
}

func TestParaLevel(t *testing.T) {
	cases := []struct {
		xml  string
		want int
	}{
		{`<a:p xmlns:a="` + nsA + `"/>`, 0},
		{`<a:p xmlns:a="` + nsA + `"><a:pPr/></a:p>`, 0},
		{`<a:p xmlns:a="` + nsA + `"><a:pPr lvl="3"/></a:p>`, 3},
		{`<a:p xmlns:a="` + nsA + `"><a:pPr lvl="99"/></a:p>`, 8},
		{`<a:p xmlns:a="` + nsA + `"><a:pPr lvl="x"/></a:p>`, 0},
	}
	for _, tc := range cases {
		d := mustIndex(t, tc.xml)
		if got := ParaLevel(d, d.Root()); got != tc.want {
			t.Fatalf("ParaLevel(%s) = %d, want %d", tc.xml, got, tc.want)
		}
	}
}

func TestClassOf(t *testing.T) {
	cases := []struct {
		k    PhKey
		phOk bool
		kind string
		want TextClass
	}{
		{PhKey{Typ: "title"}, true, StyleKindSlide, ClassTitle},
		{PhKey{Typ: "ctrTitle"}, true, StyleKindSlide, ClassTitle},
		{PhKey{Typ: "body"}, true, StyleKindSlide, ClassBody},
		{PhKey{Typ: "obj"}, true, StyleKindSlide, ClassBody},
		{PhKey{}, false, StyleKindSlide, ClassOther},
		{PhKey{Typ: "body"}, true, StyleKindNotes, ClassNotes},
	}
	for _, tc := range cases {
		if got := ClassOf(tc.k, tc.phOk, tc.kind); got != tc.want {
			t.Fatalf("ClassOf(%+v,%v,%q) = %q, want %q", tc.k, tc.phOk, tc.kind, got, tc.want)
		}
	}
}
