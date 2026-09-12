package pptx

import (
	"bytes"
	"errors"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// timingDeckWith 构造单页 sld，tail 附加在 </p:sld> 之前（用于注入 p:timing）。
func timingDeckWith(t *testing.T, tail string) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	sld := `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		tail + `</p:sld>`
	parts[opc.PartName("/ppt/slides/slide1.xml")] = []byte(xmlDecl + sld)
	parts["/[Content_Types].xml"] = append(
		bytes.TrimSuffix(parts["/[Content_Types].xml"], []byte("</Types>")),
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`)...)
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
	data, err := buildPackageZip(parts)
	if err != nil {
		t.Fatalf("buildPackageZip: %v", err)
	}
	return openFixture(t, data)
}

func TestTimingTreeRawBranches(t *testing.T) {
	// 无 timing：nil 返回。
	p := timingDeckWith(t, "")
	s := mustSlide(t, p)
	if raw, diags, err := s.TimingTreeRaw(); err != nil || raw != nil || diags != nil {
		t.Fatalf("no timing: raw=%v diags=%v err=%v", raw, diags, err)
	}
	if s.HasTiming() {
		t.Fatal("HasTiming = true without timing")
	}

	// 自闭合 timing：返回空字节（非 nil）。
	p2 := timingDeckWith(t, `<p:timing/>`)
	s2 := mustSlide(t, p2)
	raw2, _, err := s2.TimingTreeRaw()
	if err != nil {
		t.Fatalf("self-closing timing: %v", err)
	}
	if raw2 == nil || len(raw2) != 0 {
		t.Fatalf("self-closing raw = %q (len %d), want empty non-nil", raw2, len(raw2))
	}
	if !s2.HasTiming() {
		t.Fatal("HasTiming = false with self-closing timing")
	}

	// 完整 timing：返回原始字节。
	p3 := timingDeckWith(t, `<p:timing><p:tnLst/></p:timing>`)
	s3 := mustSlide(t, p3)
	raw3, _, err := s3.TimingTreeRaw()
	if err != nil {
		t.Fatalf("full timing: %v", err)
	}
	if string(raw3) != `<p:timing><p:tnLst/></p:timing>` {
		t.Fatalf("full raw = %q", raw3)
	}

	// 关闭后句柄失效 → ErrClosed。
	if err := p3.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := s3.TimingTreeRaw(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed TimingTreeRaw: %v, want ErrClosed", err)
	}
}
