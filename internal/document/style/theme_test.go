package style

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

func idx(t *testing.T, s string) *xmlstore.XMLDocument {
	t.Helper()
	d, err := xmlstore.Index([]byte(s))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	return d
}

func TestMasterClrMap(t *testing.T) {
	if got := MasterClrMap(nil); len(got) != 0 {
		t.Fatalf("nil doc = %v", got)
	}
	d := idx(t, `<p:sldMaster xmlns:p="`+ooxmlns.PresentationML+`"><p:clrMap bg1="lt1" tx1="dk1"/><p:other/></p:sldMaster>`)
	m := MasterClrMap(d)
	if m["bg1"] != "lt1" || m["tx1"] != "dk1" || len(m) != 2 {
		t.Fatalf("map = %v", m)
	}
}

func TestTextStyleNodeAndDefRPr(t *testing.T) {
	if TextStyleNode(nil, ClassTitle) != nil {
		t.Fatal("nil doc should return nil")
	}
	d := idx(t, `<p:sldMaster xmlns:p="`+ooxmlns.PresentationML+`" xmlns:a="`+ooxmlns.DrawingML+`"><p:txStyles>`+
		`<p:titleStyle><a:lvl1pPr><a:defRPr/></a:lvl1pPr></p:titleStyle>`+
		`<p:bodyStyle/>`+
		`</p:txStyles></p:sldMaster>`)
	ts := TextStyleNode(d, ClassTitle)
	if ts == nil || ts.Local() != "titleStyle" {
		t.Fatalf("titleStyle = %+v", ts)
	}
	// bodyStyle 无 lvlNpPr。
	bs := TextStyleNode(d, ClassBody)
	if bs == nil || bs.Local() != "bodyStyle" {
		t.Fatalf("bodyStyle = %+v", bs)
	}
	// 不存在的 class。
	if TextStyleNode(d, ClassNotes) != nil {
		t.Fatal("notesStyle should be absent")
	}
	// defRPr 命中 / 缺失 / nil style。
	if DefRPrAtLevel(d, ts, 0) == nil {
		t.Fatal("lvl1 defRPr should exist")
	}
	if DefRPrAtLevel(d, ts, 3) != nil {
		t.Fatal("lvl4 should be absent")
	}
	if DefRPrAtLevel(d, nil, 0) != nil {
		t.Fatal("nil style should return nil")
	}
}

func TestLstStyleOf(t *testing.T) {
	d := idx(t, `<p:sp xmlns:p="`+ooxmlns.PresentationML+`" xmlns:a="`+ooxmlns.DrawingML+`"><p:txBody><a:lstStyle/><a:p/></p:txBody></p:sp>`)
	if ls := LstStyleOf(d, d.Root()); ls == nil || ls.Local() != "lstStyle" {
		t.Fatalf("lstStyle = %+v", ls)
	}
	d2 := idx(t, `<p:sp xmlns:p="`+ooxmlns.PresentationML+`"><p:other/></p:sp>`)
	if LstStyleOf(d2, d2.Root()) != nil {
		t.Fatal("no txBody should return nil")
	}
}

func themeDocXML(major, minor string) string {
	return `<a:theme xmlns:a="` + ooxmlns.DrawingML + `"><a:themeElements><a:fontScheme>` +
		`<a:majorFont><a:latin typeface="` + major + `"/></a:majorFont>` +
		`<a:minorFont><a:latin typeface="` + minor + `"/></a:minorFont>` +
		`</a:fontScheme></a:themeElements></a:theme>`
}

func TestThemeFontFace(t *testing.T) {
	if _, ok := ThemeFontFace(nil, true, "latin"); ok {
		t.Fatal("nil doc")
	}
	d := idx(t, themeDocXML("Calibri Light", "Calibri"))
	if f, ok := ThemeFontFace(d, true, "latin"); !ok || f != "Calibri Light" {
		t.Fatalf("major = %q %v", f, ok)
	}
	if f, ok := ThemeFontFace(d, false, "latin"); !ok || f != "Calibri" {
		t.Fatalf("minor = %q %v", f, ok)
	}
	if _, ok := ThemeFontFace(d, true, "ea"); ok {
		t.Fatal("missing ea should be false")
	}
	empty := idx(t, `<a:theme xmlns:a="`+ooxmlns.DrawingML+`"/>`)
	if _, ok := ThemeFontFace(empty, true, "latin"); ok {
		t.Fatal("no themeElements")
	}
}

func TestExpandTypeface(t *testing.T) {
	// 无前缀：原样返回，无需主题。
	if f, st, ok := ExpandTypeface(nil, "Arial", "latin"); !ok || f != "Arial" {
		t.Fatalf("plain = %q %v", f, ok)
	} else if st != (StyleStep{}) {
		t.Fatalf("plain step = %+v", st)
	}
	// 有前缀但无主题：失败。
	if _, _, ok := ExpandTypeface(nil, "+mj-lt", "latin"); ok {
		t.Fatal("theme ref without doc should fail")
	}
	// 有前缀 + 主题：展开并附 SourceTheme。
	d := idx(t, themeDocXML("Calibri Light", "Calibri"))
	f, st, ok := ExpandTypeface(d, "+mj-lt", "latin")
	if !ok || f != "Calibri Light" || st.Source != SourceTheme {
		t.Fatalf("expand = %q %+v %v", f, st, ok)
	}
	fmt2, st2, ok2 := ExpandTypeface(d, "+mn-lt", "latin")
	if !ok2 || fmt2 != "Calibri" || st2.Source != SourceTheme {
		t.Fatalf("minor expand = %q %+v %v", fmt2, st2, ok2)
	}
	// 前缀指向不存在的族。
	if _, _, ok := ExpandTypeface(d, "+mj-ea", "ea"); ok {
		t.Fatal("missing ea family should fail")
	}
}

func TestFontSchemeName(t *testing.T) {
	if FontSchemeName(true) != "majorFont" || FontSchemeName(false) != "minorFont" {
		t.Fatal("fontSchemeName wrong")
	}
}

func TestStyleSourceString(t *testing.T) {
	for _, tc := range []struct {
		s    StyleSource
		want string
	}{
		{SourceRun, "run"},
		{SourceParagraphDefault, "paragraph-default"},
		{SourceListStyle, "list-style"},
		{SourceTheme, "theme"},
		{SourceFallback, "fallback"},
		{SourceCellExplicit, "cell-explicit"},
		{SourceTableStyle, "table-style"},
		{StyleSource(99), "unknown"},
	} {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("StyleSource(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
