package ir

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

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

func TestFromPresentation_PageHiddenProjection(t *testing.T) {
	// 验证 IR 默认 IncludeHidden=true 时 Page.Hidden 三态：
	//   - 新建未显式标记的 page → &false（缺省即可见，已读到）；
	//   - IncludeHidden=false → nil（opt-out）。
	// sldId@show="0" 真隐藏路径由 slide_test.go::TestSlide_Hidden_AfterMark 覆盖
	// （Slide.Hidden() 是 Page.Hidden 的唯一来源；IR 不重复读 OOXML）。
	t.Run("default_yields_pointer_false", func(t *testing.T) {
		p := irTestDeck(t)
		defer p.Close()
		doc, err := FromPresentation(p, DefaultOptions())
		if err != nil {
			t.Fatalf("FromPresentation: %v", err)
		}
		if len(doc.Pages) == 0 {
			t.Fatal("irTestDeck produced 0 pages")
		}
		h := doc.Pages[0].Hidden
		if h == nil {
			t.Fatal("Page.Hidden should be non-nil when IncludeHidden=true (default)")
		}
		if *h {
			t.Error("fresh deck has no hidden pages; *Hidden should be false")
		}
	})
	t.Run("opt_out_yields_nil", func(t *testing.T) {
		p := irTestDeck(t)
		defer p.Close()
		opts := DefaultOptions()
		opts.IncludeHidden = false
		doc, err := FromPresentation(p, opts)
		if err != nil {
			t.Fatalf("FromPresentation: %v", err)
		}
		for i, pg := range doc.Pages {
			if pg.Hidden != nil {
				t.Errorf("page %d: Page.Hidden should be nil when IncludeHidden=false, got %v", i, *pg.Hidden)
			}
		}
	})
}

func TestOptions_Defaults(t *testing.T) {
	o := DefaultOptions()
	if !o.IncludeNotes {
		t.Error("IncludeNotes default should be true")
	}
	if !o.IncludeTimingNode {
		t.Error("IncludeTimingNode default should be true")
	}
	if !o.IncludeHidden {
		t.Error("IncludeHidden default should be true (FEAT-003)")
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

func TestFromPresentation_TextShapeProjection(t *testing.T) {
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layouts, err := p.Layouts()
	if err != nil || len(layouts) == 0 {
		t.Fatalf("Layouts: %v len=%d", err, len(layouts))
	}
	slide, err := p.AddSlide(layouts[0])
	if err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	tb, err := slide.AddTextBox(pptx.TextBoxSpec{
		Name:   "IR Text",
		X:      10,
		Y:      20,
		Width:  300,
		Height: 120,
		Text:   "line one\nline two",
	})
	if err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}
	if err := tb.SetAltText("alt text"); err != nil {
		t.Fatalf("SetAltText: %v", err)
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
	if got.Kind != "textbox" {
		t.Fatalf("kind = %q, want textbox", got.Kind)
	}
	if got.Name != "IR Text" || got.Text != "line one\nline two" {
		t.Fatalf("shape summary = %+v", got)
	}
	if got.AltText != "alt text" || got.Decorative {
		t.Fatalf("accessibility = alt %q decorative %v", got.AltText, got.Decorative)
	}
	if got.Bounds == nil || got.Bounds.X != 10 || got.Bounds.Y != 20 || got.Bounds.Width != 300 || got.Bounds.Height != 120 {
		t.Fatalf("bounds = %+v", got.Bounds)
	}
	if got.NodePath == "" {
		t.Fatal("nodePath empty")
	}
}

func TestFromPresentation_CorePropertiesProjection(t *testing.T) {
	p := irTestDeck(t)
	defer p.Close()
	when := time.Date(2026, 9, 11, 3, 4, 5, 0, time.UTC)
	if err := p.SetCoreProperties(pptx.CorePropertiesPatch{
		Title:    pptx.NewOptional("季度报告"),
		Author:   pptx.NewOptional("jinfeng105"),
		Modified: pptx.NewOptional(when),
	}); err != nil {
		t.Fatalf("SetCoreProperties: %v", err)
	}
	doc, err := FromPresentation(p, DefaultOptions())
	if err != nil {
		t.Fatalf("FromPresentation: %v", err)
	}
	if doc.Core == nil {
		t.Fatal("core projection missing")
	}
	if doc.Core.Title != "季度报告" || doc.Core.Creator != "jinfeng105" {
		t.Fatalf("core = %+v", doc.Core)
	}
	if doc.Core.Modified == nil || !doc.Core.Modified.Equal(when) {
		t.Fatalf("core.Modified = %+v, want %v", doc.Core.Modified, when)
	}
	if doc.DocumentID == "" {
		t.Fatal("documentFingerprint should be non-empty when modified is set")
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", doc.Diagnostics)
	}
}

func TestSortDiagsOrdersSeverityCodePart(t *testing.T) {
	diags := Diagnostics{
		{Severity: SevInfo, Code: "B", Part: "/b"},
		{Severity: SevError, Code: "C", Part: "/c"},
		{Severity: SevWarning, Code: "B", Part: "/z"},
		{Severity: SevWarning, Code: "A", Part: "/a"},
	}
	sortDiags(diags)
	got := []string{
		string(diags[0].Severity) + ":" + diags[0].Code + ":" + diags[0].Part,
		string(diags[1].Severity) + ":" + diags[1].Code + ":" + diags[1].Part,
		string(diags[2].Severity) + ":" + diags[2].Code + ":" + diags[2].Part,
		string(diags[3].Severity) + ":" + diags[3].Code + ":" + diags[3].Part,
	}
	want := []string{"error:C:/c", "warning:A:/a", "warning:B:/z", "info:B:/b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
