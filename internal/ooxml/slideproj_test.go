package ooxml

import (
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/ooxml/schema"
	"github.com/F31/go-pptx/internal/opc"
)

func TestRelsPartName(t *testing.T) {
	cases := []struct {
		part opc.PartName
		want opc.PartName
	}{
		{"/", "/_rels/.rels"},
		{"/ppt/slides/slide1.xml", "/ppt/slides/_rels/slide1.xml.rels"},
		{"/a.xml", "/_rels/a.xml.rels"},
	}
	for _, c := range cases {
		if got := RelsPartName(c.part); got != c.want {
			t.Errorf("RelsPartName(%q) = %q, want %q", c.part, got, c.want)
		}
	}
}

const testRelNS = "http://schemas.openxmlformats.org/package/2006/relationships"
const testDocRelNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

func miniRelsS(body string) string {
	return `<Relationships xmlns="` + testRelNS + `">` + body + `</Relationships>`
}

func TestNotesPartOf(t *testing.T) {
	src := opc.PartName("/ppt/slides/slide1.xml")
	rels := miniRelsS(
		`<Relationship Id="rId1" Type="` + testDocRelNS + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
			`<Relationship Id="rId2" Type="` + testDocRelNS + `/notesSlide" Target="../notesSlides/notesSlide1.xml"/>`)
	if got, ok := NotesPartOf([]byte(rels), src); !ok || got != opc.PartName("/ppt/notesSlides/notesSlide1.xml") {
		t.Fatalf("NotesPartOf = %q, %v", got, ok)
	}
	// 无 notesSlide 关系 → 未命中。
	if _, ok := NotesPartOf([]byte(miniRelsS(
		`<Relationship Id="rId1" Type="`+testDocRelNS+`/slideLayout" Target="x.xml"/>`)), src); ok {
		t.Error("no notesSlide rel should miss")
	}
	// 外部 notesSlide 关系 → 未命中。
	if _, ok := NotesPartOf([]byte(miniRelsS(
		`<Relationship Id="rId2" Type="`+testDocRelNS+`/notesSlide" Target="https://example.com/n.xml" TargetMode="External"/>`)), src); ok {
		t.Error("external notesSlide rel should miss")
	}
	// 空字节 / 畸形关系流 → 未命中。
	if _, ok := NotesPartOf(nil, src); ok {
		t.Error("empty rels should miss")
	}
	if _, ok := NotesPartOf([]byte("<Relationships broken"), src); ok {
		t.Error("malformed rels should miss")
	}
}

func TestNotesText(t *testing.T) {
	body := `<p:notes xmlns:p="` + pmlMainNS + `" xmlns:a="` + dmlMainNS + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes Placeholder 2"/>` +
		`<p:cNvSpPr txBox="1"/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>line one</a:t></a:r></a:p>` +
		`<a:p><a:r><a:t>line two</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:notes>`
	text, err := NotesText([]byte(body))
	if err != nil {
		t.Fatalf("NotesText: %v", err)
	}
	if text != "line one\nline two" {
		t.Errorf("text = %q", text)
	}
	// 无正文占位符 → 空串。
	empty := `<p:notes xmlns:p="` + pmlMainNS + `" xmlns:a="` + dmlMainNS + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/><p:sp><p:nvSpPr><p:cNvPr id="2" name="Other"/><p:cNvSpPr txBox="1"/>` +
		`<p:nvPr><p:ph type="hdr" idx="0"/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>header</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:notes>`
	if text, err := NotesText([]byte(empty)); err != nil || text != "" {
		t.Errorf("no-body = %q, %v", text, err)
	}
	// 畸形 → 错误。
	if _, err := NotesText([]byte("<p:notes")); err == nil {
		t.Error("malformed notes should error")
	}
}

func TestNotesBodyTextFrame(t *testing.T) {
	if notesBodyTextFrame(nil) != nil {
		t.Error("nil notes should yield nil")
	}
	if notesBodyTextFrame(&schema.P_CT_NotesSlide{}) != nil {
		t.Error("notes with nil cSld should yield nil")
	}
}

func TestSlideHasTiming(t *testing.T) {
	slide := func(timing string) string {
		return `<p:sld xmlns:p="` + pmlMainNS + `" xmlns:a="` + dmlMainNS + `">` +
			`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
			timing + `</p:sld>`
	}
	yes, err := SlideHasTiming([]byte(slide(`<p:timing><p:tnLst><p:par><p:cTn id="1"/></p:par></p:tnLst></p:timing>`)))
	if err != nil || !yes {
		t.Errorf("timing present: %v %v", yes, err)
	}
	no, err := SlideHasTiming([]byte(slide("")))
	if err != nil || no {
		t.Errorf("timing absent: %v %v", no, err)
	}
	if _, err := SlideHasTiming([]byte("<p:sld")); err == nil {
		t.Error("malformed should error")
	}
}

func TestSlideTimingRaw(t *testing.T) {
	slide := func(timing string) string {
		return `<p:sld xmlns:p="` + pmlMainNS + `" xmlns:a="` + dmlMainNS + `">` +
			`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
			timing + `</p:sld>`
	}
	raw, ok, err := SlideTimingRaw([]byte(slide(`<p:timing><p:tnLst><p:par><p:cTn id="7" dur="indefinite"/></p:par></p:tnLst></p:timing>`)))
	if err != nil || !ok {
		t.Fatalf("SlideTimingRaw: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(string(raw), "cTn") || !strings.Contains(string(raw), "7") {
		t.Errorf("raw = %s", raw)
	}
	raw, ok, err = SlideTimingRaw([]byte(slide("<p:timing/>")))
	if err != nil || !ok || len(raw) != 0 {
		t.Errorf("self-closing: raw=%q ok=%v err=%v", raw, ok, err)
	}
	if _, ok, _ := SlideTimingRaw([]byte(slide(""))); ok {
		t.Error("absent timing should report ok=false")
	}
	if _, _, err := SlideTimingRaw([]byte("<p:sld")); err == nil {
		t.Error("malformed should error")
	}
}

func TestSlideHidden(t *testing.T) {
	pres := func(sldId string) string {
		return `<p:presentation xmlns:p="` + pmlMainNS + `">` +
			`<p:sldIdLst>` + sldId + `</p:sldIdLst>` +
			`<p:sldSz cx="100" cy="100"/><p:notesSz cx="100" cy="100"/></p:presentation>`
	}
	visible := `<p:sldId id="256"/>`
	hidden := `<p:sldId id="256" show="0"/>`
	otherVisible := `<p:sldId id="300" show="0"/>`
	if h, err := SlideHidden([]byte(pres(hidden)), 256); err != nil || !h {
		t.Errorf("hidden case: h=%v err=%v", h, err)
	}
	if h, err := SlideHidden([]byte(pres(visible)), 256); err != nil || h {
		t.Errorf("visible case: h=%v err=%v", h, err)
	}
	if h, err := SlideHidden([]byte(pres(otherVisible)), 256); err != nil || h {
		t.Errorf("no-matching-id case: h=%v err=%v", h, err)
	}
	if _, err := SlideHidden([]byte("<p:presentation"), 256); err == nil {
		t.Error("malformed should error")
	}
	if _, err := parseUint32("abc"); err == nil {
		t.Error("bad uint parse should error")
	}
	if n, err := parseUint32("7"); err != nil || n != 7 {
		t.Errorf("parseUint32(7) = %d, %v", n, err)
	}
}

func TestSlideHiddenNoSldIdLst(t *testing.T) {
	pres := `<p:presentation xmlns:p="` + pmlMainNS + `">` +
		`<p:sldSz cx="100" cy="100"/><p:notesSz cx="100" cy="100"/></p:presentation>`
	if h, err := SlideHidden([]byte(pres), 256); err != nil || h {
		t.Errorf("no sldIdLst: h=%v err=%v", h, err)
	}
}
