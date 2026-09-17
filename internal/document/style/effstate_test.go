package style

import (
	"errors"
	"testing"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/errs"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

const (
	pp = ooxmlns.PresentationML
	dd = ooxmlns.DrawingML
)

func TestFontStyleHelpers(t *testing.T) {
	var f FontStyle
	if f.AnySet() || f.AnyChildSet() {
		t.Fatal("empty should be false")
	}
	f.Bold = model.Optional[bool]{Value: true, Set: true}
	if !f.AnySet() || f.AnyChildSet() {
		t.Fatal("bold not set")
	}
	f2 := FontStyle{Color: model.Optional[ColorSpec]{Value: ColorSpec{RGB: "FF0000"}, Set: true}}
	if !f2.AnySet() || !f2.AnyChildSet() {
		t.Fatal("color should be child-set")
	}
	if f.String() == "" || f2.String() == "" {
		t.Fatal("String should be non-empty")
	}
}

func TestParseLocalFont(t *testing.T) {
	doc, err := xmlstore.Index([]byte(`<a:rPr xmlns:a="` + dd + `" b="1" i="0" sz="1800">` +
		`<a:solidFill><a:schemeClr val="accent1"/></a:solidFill>` +
		`<a:latin typeface="Arial"/>` +
		`<a:ea typeface="SimSun"/>` +
		`<a:cs typeface="Arial"/>` +
		`</a:rPr>`))
	if err != nil {
		t.Fatal(err)
	}
	f := ParseLocalFont(doc, doc.Root())
	if !f.Bold.Set || f.Bold.Value != true {
		t.Fatalf("bold = %+v", f.Bold)
	}
	if !f.Italic.Set || f.Italic.Value != false {
		t.Fatalf("italic = %+v", f.Italic)
	}
	if !f.Size.Set || f.Size.Value != 18 {
		t.Fatalf("size = %+v", f.Size)
	}
	if !f.Color.Set || f.Color.Value.Scheme != "accent1" {
		t.Fatalf("color = %+v", f.Color)
	}
	if !f.Latin.Set || f.Latin.Value != "Arial" {
		t.Fatalf("latin = %+v", f.Latin)
	}
	// sz 非法不设置。
	bad, _ := xmlstore.Index([]byte(`<a:rPr xmlns:a="` + dd + `" sz="xyz"/>`))
	if ParseLocalFont(bad, bad.Root()).Size.Set {
		t.Fatal("bad sz should not set size")
	}
}

const slideDocXML = `<p:sp xmlns:p="` + pp + `" xmlns:a="` + dd + `">
  <p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>
  <p:txBody><a:lstStyle/><a:p><a:pPr lvl="0"/><a:r><a:rPr b="1"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:rPr></a:r></a:p></p:txBody>
</p:sp>`

func effFixture(t *testing.T) (*xmlstore.XMLDocument, *xmlstore.NodeRecord, *Env, DocFunc) {
	t.Helper()
	slide := idx(t, slideDocXML)
	layout := idx(t, `<p:sldLayout xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:cSld><p:spTree/></p:cSld></p:sldLayout>`)
	master := idx(t, `<p:sldMaster xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:clrMap bg1="accent1"/></p:sldMaster>`)
	theme := idx(t, `<a:theme xmlns:a="`+dd+`"><a:themeElements><a:fontScheme>`+
		`<a:majorFont><a:latin typeface="Calibri Light"/></a:majorFont>`+
		`<a:minorFont><a:latin typeface="Calibri"/></a:minorFont>`+
		`<a:clrScheme><a:accent1><a:srgbClr val="4472C4"/></a:accent1></a:clrScheme>`+
		`</a:fontScheme></a:themeElements></a:theme>`)
	env := &Env{Kind: StyleKindSlide,
		Layout: opc.PartName("/ppt/slideLayouts/layout.xml"),
		Master: opc.PartName("/ppt/slideMasters/master.xml"),
		Theme:  opc.PartName("/ppt/theme/theme.xml")}
	parts := map[opc.PartName]*xmlstore.XMLDocument{
		env.Layout: layout, env.Master: master, env.Theme: theme,
	}
	docs := func(p opc.PartName) *xmlstore.XMLDocument { return parts[p] }
	run := xmlstore.ChildOfKind(slide, xmlstore.ChildOfKind(slide, slide.Root(), pp, "txBody", 0), dd, "p", 0)
	run = xmlstore.ChildOfKind(slide, run, dd, "r", 0)
	if run == nil {
		t.Fatal("run not found")
	}
	return slide, run, env, docs
}

func TestResolveEffectiveFontChain(t *testing.T) {
	_, run, env, docs := effFixture(t)
	ctx := ResolveContext{}
	rf, diags, err := ResolveEffectiveFont(ctx, env, docs, idx(t, slideDocXML), run, "/p")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !rf.Bold.Resolved || rf.Bold.Value != true {
		t.Fatalf("bold = %+v", rf.Bold)
	}
	if !rf.Color.Resolved || rf.Color.RGB != "FF0000" {
		t.Fatalf("color = %+v", rf.Color)
	}
	// 字体族 L4 主题 minorFont（ClassBody）。
	if !rf.Latin.Resolved || rf.Latin.Value != "Calibri" {
		t.Fatalf("latin = %+v", rf.Latin)
	}
	// after len(diags) != 0 { } eslint-style: 无主题缺省字体（ea/cs）未决。
	_ = diags
}

func TestResolveEffectiveFontStrict(t *testing.T) {
	slide, run, env, docs := effFixture(t)
	_ = slide
	ctx := ResolveContext{Strict: true}
	_, _, err := ResolveEffectiveFont(ctx, env, docs, idx(t, slideDocXML), run, "/p")
	if !errors.Is(err, errs.ErrUnresolvedStyle) {
		t.Fatalf("strict should return ErrUnresolvedStyle, got %v", err)
	}
}

func TestResolveEffectiveFontFallback(t *testing.T) {
	_, run, env, docs := effFixture(t)
	ctx := ResolveContext{Fallback: FontStyle{
		Size: model.Optional[FontSize]{Value: 22, Set: true},
	}}
	rf, _, err := ResolveEffectiveFont(ctx, env, docs, idx(t, slideDocXML), run, "/p")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !rf.Size.Resolved || !rf.Size.Fallback || rf.Size.Value != 22 {
		t.Fatalf("size = %+v", rf.Size)
	}
}

func TestResolveEffectiveFontPhClrPartial(t *testing.T) {
	slide := idx(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>`+
		`<p:txBody><a:p><a:r><a:rPr><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:rPr></a:r></a:p></p:txBody></p:sp>`)
	run := xmlstore.ChildOfKind(slide, xmlstore.ChildOfKind(slide, slide.Root(), pp, "txBody", 0), dd, "p", 0)
	run = xmlstore.ChildOfKind(slide, run, dd, "r", 0)
	_, raw, env, docs := effFixture(t)
	_ = raw
	rf, diags, err := ResolveEffectiveFont(ResolveContext{}, env, docs, slide, run, "/p")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rf.Color.Resolved {
		t.Fatal("phClr should not be resolved")
	}
	hasPartial := false
	for _, d := range diags {
		if d.Code == "STYLE_PARTIAL" {
			hasPartial = true
		}
	}
	if !hasPartial {
		t.Fatalf("expected STYLE_PARTIAL diag, got %+v", diags)
	}
}

var _ = diag.Diagnostic{} // keep diag import

func phSp(ph string, lst string) string {
	return `<p:sp xmlns:p="` + pp + `" xmlns:a="` + dd + `">` +
		`<p:nvSpPr><p:nvPr><p:ph type="` + ph + `"/></p:nvPr></p:nvSpPr>` +
		`<p:txBody><a:lstStyle>` + lst + `</a:lstStyle></p:txBody></p:sp>`
}

func TestResolveEffectiveFontFullChain(t *testing.T) {
	lstStyle := `<a:lvl1pPr><a:defRPr sz="1600"/></a:lvl1pPr>`
	layout := idx(t, `<p:sldLayout xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:cSld><p:spTree>`+
		phSp("body", lstStyle)+`</p:spTree></p:cSld></p:sldLayout>`)
	master := idx(t, `<p:sldMaster xmlns:p="`+pp+`" xmlns:a="`+dd+`">`+
		`<p:clrMap bg1="accent1"/>`+
		`<p:txStyles><p:bodyStyle><a:lvl1pPr><a:defRPr><a:latin typeface="+mj-lt"/></a:defRPr></a:lvl1pPr></p:bodyStyle></p:txStyles>`+
		`</p:sldMaster>`)
	theme := idx(t, `<a:theme xmlns:a="`+dd+`"><a:themeElements>`+
		`<a:fontScheme><a:majorFont><a:latin typeface="Calibri Light"/><a:ea typeface="DengXian"/></a:majorFont>`+
		`<a:minorFont><a:latin typeface="Calibri"/></a:minorFont></a:fontScheme>`+
		`<a:clrScheme><a:accent1><a:srgbClr val="4472C4"/></a:accent1></a:clrScheme>`+
		`</a:themeElements></a:theme>`)
	slide := idx(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>`+
		`<p:txBody><a:p><a:r><a:rPr b="1"><a:solidFill><a:schemeClr val="bg1"/></a:solidFill></a:rPr></a:r></a:p></p:txBody></p:sp>`)
	run := xmlstore.ChildOfKind(slide, xmlstore.ChildOfKind(slide, slide.Root(), pp, "txBody", 0), dd, "p", 0)
	run = xmlstore.ChildOfKind(slide, run, dd, "r", 0)
	env := &Env{Kind: StyleKindSlide,
		Layout: "/l.xml", Master: "/m.xml", Theme: "/t.xml"}
	parts := map[opc.PartName]*xmlstore.XMLDocument{env.Layout: layout, env.Master: master, env.Theme: theme}
	docs := func(p opc.PartName) *xmlstore.XMLDocument { return parts[p] }

	rf, _, err := ResolveEffectiveFont(ResolveContext{}, env, docs, slide, run, "/p")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !rf.Bold.Resolved || rf.Bold.Value != true {
		t.Fatalf("bold = %+v", rf.Bold)
	}
	// 布局 lstStyle 级 defRPr sz=16（L3 layout 分支 + phDetail）。
	if !rf.Size.Resolved || rf.Size.Value != 16 {
		t.Fatalf("size = %+v", rf.Size)
	}
	// bg1 → clrMap→accent1 → 4472C4。
	if !rf.Color.Resolved || rf.Color.RGB != "4472C4" {
		t.Fatalf("color = %+v", rf.Color)
	}
	// 显式主题引用 +mj-lt 展开为 majorFont latin。
	if !rf.Latin.Resolved || rf.Latin.Value != "Calibri Light" {
		t.Fatalf("latin = %+v", rf.Latin)
	}
}

func TestResolveColorIndirectMissingClrMap(t *testing.T) {
	slide := idx(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>`+
		`<p:txBody><a:p><a:r><a:rPr><a:solidFill><a:schemeClr val="bg1"/></a:solidFill></a:rPr></a:r></a:p></p:txBody></p:sp>`)
	run := xmlstore.ChildOfKind(slide, xmlstore.ChildOfKind(slide, slide.Root(), pp, "txBody", 0), dd, "p", 0)
	run = xmlstore.ChildOfKind(slide, run, dd, "r", 0)
	// master 无 clrMap 的 bg1 条目。
	master := idx(t, `<p:sldMaster xmlns:p="`+pp+`"><p:clrMap bg2="accent1"/></p:sldMaster>`)
	env := &Env{Kind: StyleKindSlide, Master: "/m.xml", Theme: "/t.xml"}
	parts := map[opc.PartName]*xmlstore.XMLDocument{env.Master: master, env.Theme: idx(t, `<a:theme xmlns:a="`+dd+`"><a:themeElements><a:clrScheme><a:accent1><a:srgbClr val="4472C4"/></a:accent1></a:clrScheme></a:themeElements></a:theme>`)}
	docs := func(p opc.PartName) *xmlstore.XMLDocument { return parts[p] }
	rf, diags, err := ResolveEffectiveFont(ResolveContext{}, env, docs, slide, run, "/p")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rf.Color.Resolved {
		t.Fatal("missing clrMap entry should be unresolved")
	}
	found := false
	for _, d := range diags {
		if d.Code == "STYLE_PARTIAL" && containsStr(d.Message, "clrMap has no entry") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected clrMap diagnostic, got %+v", diags)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestResolveEffectiveFontFallbackColor(t *testing.T) {
	slide := idx(t, `<p:sp xmlns:p="`+pp+`" xmlns:a="`+dd+`"><p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>`+
		`<p:txBody><a:p><a:r><a:rPr b="1"/></a:r></a:p></p:txBody></p:sp>`)
	run := xmlstore.ChildOfKind(slide, xmlstore.ChildOfKind(slide, slide.Root(), pp, "txBody", 0), dd, "p", 0)
	run = xmlstore.ChildOfKind(slide, run, dd, "r", 0)
	_, _, env, docs := effFixture(t)
	ctx := ResolveContext{Fallback: FontStyle{
		Color: model.Optional[ColorSpec]{Value: ColorSpec{RGB: "00FF00"}, Set: true},
	}}
	rf, _, err := ResolveEffectiveFont(ctx, env, docs, slide, run, "/p")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !rf.Color.Resolved || !rf.Color.Fallback || rf.Color.RGB != "00FF00" {
		t.Fatalf("color = %+v", rf.Color)
	}
}
