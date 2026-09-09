package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/go-pptx"
)

// cliDiffDeck 生成一页含指定正文的 PPTX（zip 手术，同 cliBindDeck 的
// 构造方式：本库尚无创建型形状 API）。
func cliDiffDeck(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	layouts, _ := p.Layouts()
	if _, err := p.AddSlide(layouts[0]); err != nil {
		p.Close()
		t.Fatalf("AddSlide: %v", err)
	}
	base := filepath.Join(dir, "base.pptx")
	if _, err := p.Save(context.Background(), base); err != nil {
		p.Close()
		t.Fatalf("Save: %v", err)
	}
	p.Close()

	sld := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
		` xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>` + body + `</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:sld>`
	out := filepath.Join(dir, "deck-"+sanitize(body)+".pptx")
	if err := rewriteSlidePart(base, out, sld); err != nil {
		t.Fatalf("rewriteSlidePart: %v", err)
	}
	return out
}

func sanitize(s string) string {
	r := strings.Map(func(c rune) rune {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			return c
		}
		return '-'
	}, string(s[0:min(len(s), 16)]))
	if r == "" {
		return "x"
	}
	return r
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestCLI_Diff_TextChange(t *testing.T) {
	old := cliDiffDeck(t, "Report: draft")
	new := cliDiffDeck(t, "Report: final")
	code, stdout, stderr := runCLI([]string{"diff", old, new})
	if code != ExitOK {
		t.Fatalf("diff exit = %v, stderr = %s", code, stderr)
	}
	var rep struct {
		SchemaVersion string `json:"schemaVersion"`
		Stats         struct {
			PagesChanged  int `json:"pagesChanged"`
			TextChanges   int `json:"textChanges"`
			ShapesRemoved int `json:"shapesRemoved"`
			ShapesAdded   int `json:"shapesAdded"`
			PagesRemoved  int `json:"pagesRemoved"`
			PagesAdded    int `json:"pagesAdded"`
		} `json:"stats"`
		Entries []struct {
			Kind string `json:"kind"`
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("report JSON: %v\nstdout=%s", err, stdout)
	}
	if rep.SchemaVersion != "go-pptx.diff/1.0" {
		t.Errorf("schemaVersion = %q", rep.SchemaVersion)
	}
	if rep.Stats.PagesChanged == 0 || rep.Stats.TextChanges == 0 {
		t.Errorf("stats = %+v (expect pagesChanged & textChanges > 0)", rep.Stats)
	}
	if rep.Stats.ShapesRemoved != 0 || rep.Stats.ShapesAdded != 0 {
		t.Errorf("same shape should be paired, stats = %+v", rep.Stats)
	}
	if rep.Stats.PagesRemoved != 0 || rep.Stats.PagesAdded != 0 {
		t.Errorf("text-only change must not be page add/remove, stats = %+v", rep.Stats)
	}
	found := false
	for _, e := range rep.Entries {
		if e.Kind == "shape.text_changed" && e.From == "Report: draft" && e.To == "Report: final" {
			found = true
		}
	}
	if !found {
		t.Errorf("text change entry missing: %+v", rep.Entries)
	}
}

func TestCLI_Diff_Identical(t *testing.T) {
	a := cliDiffDeck(t, "same")
	b := cliDiffDeck(t, "same")
	code, stdout, _ := runCLI([]string{"diff", a, b})
	if code != ExitOK {
		t.Fatalf("diff exit = %v", code)
	}
	var rep struct {
		Stats struct {
			TotalChanges int `json:"totalChanges"`
		} `json:"stats"`
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("report JSON: %v", err)
	}
	if len(rep.Entries) != 0 {
		t.Errorf("identical decks should yield no entries, got %d", len(rep.Entries))
	}
}

func TestCLI_Diff_MissingArgUsage(t *testing.T) {
	code, _, stderr := runCLI([]string{"diff", "only-one.pptx"})
	if code != ExitUsageError {
		t.Fatalf("exit = %v, want ExitUsageError, stderr=%s", code, stderr)
	}
}

func TestCLI_Diff_OutputFile(t *testing.T) {
	old := cliDiffDeck(t, "v1")
	new := cliDiffDeck(t, "v2")
	out := filepath.Join(t.TempDir(), "diff.json")
	code, _, stderr := runCLI([]string{"diff", "--output", out, old, new})
	if code != ExitOK {
		t.Fatalf("exit = %v, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output missing: %v", err)
	}
	// 已存在且未 --overwrite：ExitResource。
	code, _, _ = runCLI([]string{"diff", "--output", out, old, new})
	if code != ExitResource {
		t.Fatalf("second run exit = %v, want ExitResource", code)
	}
}
