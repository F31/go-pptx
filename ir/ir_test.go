package ir

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/F31/go-pptx"
)

// irTestDeck 构建一个最小演示文稿，含三页可演示各种场景。
func irTestDeck(t *testing.T) *pptx.Presentation {
	t.Helper()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	layout, err := p.Layouts()
	if err != nil || len(layout) == 0 {
		t.Fatalf("Layouts: %v len=%d", err, len(layout))
	}
	if _, err := p.AddSlide(layout[0]); err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	if _, err := p.AddSlide(layout[0]); err != nil {
		t.Fatalf("AddSlide 2: %v", err)
	}
	return p
}

func TestFromPresentation_Basic(t *testing.T) {
	p := irTestDeck(t)
	defer p.Close()
	doc, err := FromPresentation(p, DefaultOptions())
	if err != nil {
		t.Fatalf("FromPresentation: %v", err)
	}
	if doc.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", doc.SchemaVersion, SchemaVersion)
	}
	if len(doc.Pages) != 2 {
		t.Errorf("pages = %d, want 2", len(doc.Pages))
	}
	for i, p := range doc.Pages {
		if p.Index != i {
			t.Errorf("page %d index = %d", i, p.Index)
		}
		if p.SlideID == 0 {
			t.Errorf("page %d slideID = 0", i)
		}
		if p.Part == "" {
			t.Errorf("page %d part empty", i)
		}
	}
}

func TestDocument_MarshalRoundtrip(t *testing.T) {
	p := irTestDeck(t)
	defer p.Close()
	doc, err := FromPresentation(p, DefaultOptions())
	if err != nil {
		t.Fatalf("FromPresentation: %v", err)
	}
	data, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(data, []byte(`"schemaVersion":"`+SchemaVersion+`"`)) {
		t.Errorf("marshal missing schemaVersion: %s", data)
	}
	out, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.SchemaVersion != doc.SchemaVersion {
		t.Errorf("roundtrip schemaVersion = %q, want %q", out.SchemaVersion, doc.SchemaVersion)
	}
	if len(out.Pages) != len(doc.Pages) {
		t.Errorf("pages length mismatch: %d vs %d", len(out.Pages), len(doc.Pages))
	}
}

func TestUnmarshal_RejectsMismatchedSchemaVersion(t *testing.T) {
	bad := []byte(`{"schemaVersion":"go-pptx.ir/9999.0","pages":[]}`)
	if _, err := Unmarshal(bad); err == nil || !strings.Contains(err.Error(), "schemaVersion") {
		t.Fatalf("expected schemaVersion error, got %v", err)
	}
	missing := []byte(`{"pages":[]}`)
	if _, err := Unmarshal(missing); err == nil {
		t.Fatal("expected error for missing schemaVersion")
	}
}

func TestFromPresentation_NilPresentation(t *testing.T) {
	if _, err := FromPresentation(nil, DefaultOptions()); err == nil {
		t.Fatal("expected error for nil presentation")
	}
}

func TestOptions_Defaults(t *testing.T) {
	o := DefaultOptions()
	if !o.IncludeNotes {
		t.Error("IncludeNotes default should be true")
	}
	if !o.IncludeTimingNode {
		t.Error("IncludeTimingNode default should be true")
	}
}

// TestFromPresentation_AfterChartEnsuresTypedKind 验证 chart 类型以 String 形式输出。
func TestFromPresentation_AfterChartEnsuresTypedKind(t *testing.T) {
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layout, _ := p.Layouts()
	slide, err := p.AddSlide(layout[0])
	if err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	_, err = slide.AddChart(context.Background(), pptx.ChartSpec{
		Type:       pptx.ChartBar,
		Title:      "Sample",
		Categories: []string{"Q1", "Q2"},
		Series:     []pptx.ChartSeries{{Name: "Series 1", Values: []float64{1, 2}}},
		X:          0, Y: 0, Width: 1000, Height: 1000,
	})
	if err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	doc, err := FromPresentation(p, DefaultOptions())
	if err != nil {
		t.Fatalf("FromPresentation: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(doc.Pages))
	}
	if len(doc.Pages[0].Shapes) != 1 {
		t.Fatalf("shapes = %d, want 1", len(doc.Pages[0].Shapes))
	}
	got := doc.Pages[0].Shapes[0]
	if got.Kind != "chart" {
		t.Errorf("shape kind = %q, want chart", got.Kind)
	}
	if got.ChartType != "bar" {
		t.Errorf("chartType = %q, want bar", got.ChartType)
	}
	if !strings.Contains(got.Text, "Q1") || !strings.Contains(got.Text, "Q2") {
		t.Errorf("shape text missing categories: %q", got.Text)
	}
}
