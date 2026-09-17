package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/F31/go-pptx/v2/internal/ir"
	"github.com/F31/go-pptx/v2/pptx"
)

func deck(t *testing.T) *pptx.Presentation {
	t.Helper()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	layouts, err := p.Layouts()
	if err != nil || len(layouts) == 0 {
		t.Fatalf("Layouts: %v len=%d", err, len(layouts))
	}
	if _, err := p.AddSlide(layouts[0]); err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	return p
}

func TestInspect(t *testing.T) {
	p := deck(t)
	defer p.Close()
	r, err := Inspect(p)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if r.Pages != 1 || r.Document == nil || !r.ReadOnly {
		t.Fatalf("InspectResult = %+v", r)
	}
	if r.SDKVersion != pptx.SDKVersion {
		t.Errorf("SDKVersion = %q", r.SDKVersion)
	}
}

func TestCountSeverities(t *testing.T) {
	e, w := countSeverities([]pptx.Diagnostic{
		{Severity: pptx.SeverityError},
		{Severity: pptx.SeverityWarning},
		{Severity: pptx.SeverityInfo},
		{Severity: pptx.SeverityError},
	})
	if e != 2 || w != 1 {
		t.Errorf("countSeverities = %d,%d want 2,1", e, w)
	}
	if e, w := countSeverities(nil); e != 0 || w != 0 {
		t.Errorf("nil = %d,%d", e, w)
	}
}

func TestInspectClosedError(t *testing.T) {
	p := deck(t)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := Inspect(p); err == nil {
		t.Error("Inspect on closed presentation should error")
	}
}

func TestValidate(t *testing.T) {
	p := deck(t)
	defer p.Close()
	r := Validate(context.Background(), p)
	if r.Level != "structural" {
		t.Errorf("Level = %q", r.Level)
	}
	if r.ErrorCount < 0 || r.WarnCount < 0 {
		t.Errorf("negative counts: %+v", r)
	}
	if r.Mode == "" {
		t.Errorf("Mode empty: %+v", r)
	}
}

func TestProjectIRNil(t *testing.T) {
	if _, err := ProjectIR(nil, ir.DefaultOptions()); err == nil {
		t.Fatal("nil presentation should error")
	}
}

func TestProjectIRClosed(t *testing.T) {
	p := deck(t)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := ProjectIR(p, ir.DefaultOptions()); err == nil {
		t.Fatal("ProjectIR on closed presentation should error")
	}
}

// irCoreBadDeck 构造一个 docProps/core.xml 损坏的最小包，使
// CoreProperties() 读取失败（覆盖 projectCore/documentFingerprint 的
// 错误分支）。
func testCoreBadDeck(t *testing.T) *pptx.Presentation {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	put := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	nsRels := "http://schemas.openxmlformats.org/package/2006/relationships"
	nsCT := "http://schemas.openxmlformats.org/package/2006/content-types"
	nsR := "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsP := "http://schemas.openxmlformats.org/presentationml/2006/main"
	put("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Types xmlns="`+nsCT+`">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`+
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`+
		`</Types>`)
	put("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="`+nsRels+`">`+
		`<Relationship Id="rId1" Type="`+nsR+`/officeDocument" Target="ppt/presentation.xml"/>`+
		`<Relationship Id="rId2" Type="`+nsR+`/core-properties" Target="docProps/core.xml"/>`+
		`</Relationships>`)
	put("docProps/core.xml", `<cp:coreProperties`) // 损坏
	put("ppt/presentation.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<p:presentation xmlns:r="`+nsR+`" xmlns:p="`+nsP+`">`+
		`<p:sldIdLst/><p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>`+
		`</p:presentation>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	p, err := pptx.OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestProjectIRCoreReadError(t *testing.T) {
	p := testCoreBadDeck(t)
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "IR_CORE_READ" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want IR_CORE_READ diagnostic, got %+v", doc.Diagnostics)
	}
	if doc.DocumentID != "" {
		t.Errorf("DocumentID = %q, want empty on core read failure", doc.DocumentID)
	}
}

func TestConvertDiagnostics(t *testing.T) {
	if got := convertDiagnostics(nil); got != nil {
		t.Errorf("nil = %v", got)
	}
	out := convertDiagnostics([]pptx.Diagnostic{
		{Code: "X", Severity: pptx.SeverityError, Part: "/ppt/a.xml", Message: "m"},
		{Code: "Y", Severity: pptx.SeverityWarning},
	})
	if len(out) != 2 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].Code != "X" || out[0].Severity != ir.SevError || out[0].Part != "/ppt/a.xml" || out[0].Message != "m" {
		t.Errorf("out[0] = %+v", out[0])
	}
	if out[1].Severity != ir.SevWarning {
		t.Errorf("out[1] = %+v", out[1])
	}
}

func TestCapability(t *testing.T) {
	r, err := Capability("in.pptx", 123)
	if err != nil {
		t.Fatalf("Capability: %v", err)
	}
	if r.Manifest.Source.Input != "in.pptx" || r.Manifest.Source.Size != 123 {
		t.Errorf("source = %+v", r.Manifest.Source)
	}
	if len(r.IndentJSON) == 0 || len(r.Manifest.Dimensions) != 6 {
		t.Errorf("manifest = %+v", r.Manifest)
	}
	// 无源：Source.Input 留空。
	r2, err := Capability("", 0)
	if err != nil {
		t.Fatalf("Capability empty: %v", err)
	}
	if r2.Manifest.Source.Input != "" {
		t.Errorf("empty source input = %q", r2.Manifest.Source.Input)
	}
}
