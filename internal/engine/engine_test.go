package engine

import (
	"context"
	"testing"

	"github.com/F31/go-pptx/pptx"
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
