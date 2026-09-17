package schema

import (
	"strings"
	"testing"
)

const pNS = "http://schemas.openxmlformats.org/presentationml/2006/main"
const aNS = "http://schemas.openxmlformats.org/drawingml/2006/main"

func TestDecodeSlideReadProjection(t *testing.T) {
	x := `<p:sld xmlns:p="` + pNS + `" xmlns:a="` + aNS + `" show="1">` +
		`<p:cSld name="Deck"><p:spTree>` +
		`<p:sp><p:spPr><a:prstGeom prst="rect"/></p:spPr><p:txBody><a:bodyPr/><a:p><a:r><a:t>One</a:t></a:r></a:p></p:txBody></p:sp>` +
		`<p:sp><p:txBody><a:bodyPr/><a:p><a:r><a:t>Two</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:sld>`
	s, err := DecodeSlide([]byte(x))
	if err != nil {
		t.Fatalf("DecodeSlide: %v", err)
	}
	if !s.Show {
		t.Error("Show = false, want true")
	}
	if s.CSld == nil || s.CSld.Name != "Deck" {
		t.Fatalf("CSld = %+v", s.CSld)
	}
	if s.CSld.SpTree == nil || len(s.CSld.SpTree.Sp) != 2 {
		t.Fatalf("Sp len = %d", len(s.CSld.SpTree.Sp))
	}
	sp0 := s.CSld.SpTree.Sp[0]
	if sp0.SpPr == nil || sp0.SpPr.PrstGeom == nil {
		t.Fatalf("sp0 geometry not projected: %+v", sp0.SpPr)
	}
	if sp0.SpPr.PrstGeom.Prst != "rect" {
		t.Errorf("prst = %q", sp0.SpPr.PrstGeom.Prst)
	}
	if sp0.TxBody == nil || len(sp0.TxBody.P) != 1 {
		t.Fatalf("sp0 TxBody.P = %+v", sp0.TxBody)
	}
	if got := sp0.TxBody.P[0].R; len(got) != 1 || got[0].T != "One" {
		t.Errorf("sp0 run = %+v", got)
	}
	if got := s.CSld.SpTree.Sp[1].TxBody.P[0].R[0].T; got != "Two" {
		t.Errorf("sp1 run = %q", got)
	}
}

func TestDecodeLayoutAndMaster(t *testing.T) {
	x := `<p:sldLayout xmlns:p="` + pNS + `" type="blank" preserve="1"><p:cSld name="L"/></p:sldLayout>`
	l, err := DecodeSlideLayout([]byte(x))
	if err != nil {
		t.Fatalf("DecodeSlideLayout: %v", err)
	}
	if l.Type != "blank" || !l.Preserve || l.CSld == nil || l.CSld.Name != "L" {
		t.Fatalf("layout = %+v", l)
	}
	m, err := DecodeSlideMaster([]byte(`<p:sldMaster xmlns:p="` + pNS + `"/>`))
	if err != nil || m == nil {
		t.Fatalf("DecodeSlideMaster: %v %v", m, err)
	}
}

func TestDecodePresentation(t *testing.T) {
	p, err := DecodePresentation([]byte(`<p:presentation xmlns:p="` + pNS + `" firstSlideNum="1"/>`))
	if err != nil {
		t.Fatalf("DecodePresentation: %v", err)
	}
	if p.FirstSlideNum != 1 {
		t.Errorf("FirstSlideNum = %d", p.FirstSlideNum)
	}
}

func TestUnmarshalError(t *testing.T) {
	var v P_CT_Slide
	err := Unmarshal([]byte("<p:sld"), &v)
	if err == nil {
		t.Fatal("want error on malformed XML")
	}
	if !strings.Contains(err.Error(), "ooxml schema") {
		t.Errorf("error prefix missing: %v", err)
	}
}

func TestDecodeErrors(t *testing.T) {
	bad := []byte("<p:sld")
	if _, err := DecodePresentation(bad); err == nil {
		t.Error("DecodePresentation: want error")
	}
	if _, err := DecodeSlide(bad); err == nil {
		t.Error("DecodeSlide: want error")
	}
	if _, err := DecodeSlideLayout(bad); err == nil {
		t.Error("DecodeSlideLayout: want error")
	}
	if _, err := DecodeSlideMaster(bad); err == nil {
		t.Error("DecodeSlideMaster: want error")
	}
}
