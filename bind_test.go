package pptx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ---------- TPL-01：模板数据绑定引擎（ADR 013） ----------

// bindShapeParas 返回页面全部 AutoShape 的段落文本（文档序）。
func bindShapeParas(t *testing.T, p *Presentation) []string {
	t.Helper()
	s := mustSlide(t, p)
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	var out []string
	for _, sh := range shapes {
		as, ok := sh.(*AutoShape)
		if !ok {
			continue
		}
		tf, err := as.TextFrame()
		if err != nil || tf == nil {
			continue
		}
		paras, err := tf.Paragraphs()
		if err != nil {
			t.Fatalf("Paragraphs: %v", err)
		}
		for _, para := range paras {
			txt, err := para.Text()
			if err != nil {
				t.Fatalf("Text: %v", err)
			}
			out = append(out, txt)
		}
	}
	return out
}

// multiParaCell 构造含多个段落的 a:tc。
func multiParaCell(paras ...string) string {
	inner := ""
	for _, p := range paras {
		inner += `<a:p><a:r><a:t>` + p + `</a:t></a:r></a:p>`
	}
	return `<a:tc><a:txBody><a:bodyPr/>` + inner + `</a:txBody><a:tcPr/></a:tc>`
}

// bindTableDeck 构造含一个表格（表头行 + 模板行）的单页文档。
func bindTableDeck(t *testing.T, rows string) *Presentation {
	t.Helper()
	return tableDeck(t, tableFrame("9", "", []string{"3000000", "3000000"}, rows), "")
}

func TestBindInlinePlaceholder(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>Hello {{ name }}!</a:t></a:r></a:p>`)
	rep, err := p.Bind(map[string]any{"name": "Go"})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.Placeholders != 1 || rep.Substituted != 1 {
		t.Errorf("report = %+v", rep)
	}
	if got := bindShapeParas(t, p); len(got) != 1 || got[0] != "Hello Go!" {
		t.Fatalf("paras = %q", got)
	}
	if len(rep.Parts) != 1 || rep.Parts[0] != "/ppt/slides/slide1.xml" {
		t.Errorf("parts = %v", rep.Parts)
	}
}

func TestBindPlaceholderSplitAcrossRuns(t *testing.T) {
	// 占位符被拆到两个 Run（ADR 013：复用跨 Run 保真替换）。
	p := slideWithBody(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:rPr lang="en-US" sz="1800" b="1"/><a:t>Hi {{ na</a:t></a:r>`+
		`<a:r><a:rPr b="0"/><a:t>me }}</a:t></a:r>`+
		`</a:p>`)
	rep, err := p.Bind(map[string]any{"name": "pptx"})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.Substituted != 1 {
		t.Errorf("report = %+v", rep)
	}
	if got := bindShapeParas(t, p); len(got) != 1 || got[0] != "Hi pptx" {
		t.Fatalf("paras = %q", got)
	}
}

func TestBindNestedAndIndexedPaths(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>{{ user.name }} / {{ items.1 }} / {{ n }}</a:t></a:r></a:p>`)
	if _, err := p.Bind(map[string]any{
		"user":  map[string]any{"name": "Ann"},
		"items": []any{"zero", "one"},
		"n":     42,
	}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	got := bindShapeParas(t, p)
	if len(got) != 1 || got[0] != "Ann / one / 42" {
		t.Fatalf("paras = %q", got)
	}
}

func TestBindStrictMissingKeyFails(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>Hi {{ missing }}</a:t></a:r></a:p>`)
	before, err := p.Revision(), error(nil)
	_ = before
	_ = err
	_, bindErr := p.Bind(map[string]any{"name": "Go"})
	if !errors.Is(bindErr, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", bindErr)
	}
	// 无部分写入：文档保持原样，revision 不变。
	if got := bindShapeParas(t, p); len(got) != 1 || got[0] != "Hi {{ missing }}" {
		t.Fatalf("paras after failed bind = %q", got)
	}
}

func TestBindLooseMissingKeyKeepsPlaceholder(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>Hi {{ missing }}</a:t></a:r></a:p>`)
	rep, err := p.Bind(map[string]any{}, WithBindStrict(false))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if len(rep.Diagnostics) == 0 || rep.Diagnostics[0].Code != "bind.key_missing" {
		t.Errorf("diagnostics = %+v", rep.Diagnostics)
	}
	if got := bindShapeParas(t, p); len(got) != 1 || got[0] != "Hi {{ missing }}" {
		t.Fatalf("paras = %q", got)
	}
}

func TestBindUnsupportedValueType(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>{{ v }}</a:t></a:r></a:p>`)
	_, err := p.Bind(map[string]any{"v": struct{ A int }{1}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestBindConditionalTrue(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>Title</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{#if show}}</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>Secret</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{/if}}</a:t></a:r></a:p>`)
	rep, err := p.Bind(map[string]any{"show": true})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.ParagraphsRemoved != 2 {
		t.Errorf("removed = %d, want 2 (markers only)", rep.ParagraphsRemoved)
	}
	got := bindShapeParas(t, p)
	if len(got) != 2 || got[0] != "Title" || got[1] != "Secret" {
		t.Fatalf("paras = %q", got)
	}
}

func TestBindConditionalFalse(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>Title</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{#if show}}</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>Secret</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{/if}}</a:t></a:r></a:p>`)
	rep, err := p.Bind(map[string]any{"show": false})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.ParagraphsRemoved != 3 {
		t.Errorf("removed = %d, want 3", rep.ParagraphsRemoved)
	}
	got := bindShapeParas(t, p)
	if len(got) != 1 || got[0] != "Title" {
		t.Fatalf("paras = %q", got)
	}
}

func TestBindConditionalEmptyBodyKeepsOneParagraph(t *testing.T) {
	// 文本体仅含条件块且条件为假：保留一个空段落，避免产出无段落
	// 的 txBody（PowerPoint 修复提示风险）。
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>{{#if show}}</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>Secret</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{/if}}</a:t></a:r></a:p>`)
	if _, err := p.Bind(map[string]any{"show": false}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	got := bindShapeParas(t, p)
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("paras = %q, want exactly one empty paragraph", got)
	}
}

func TestBindUnclosedIfRejected(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>{{#if show}}</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>Secret</a:t></a:r></a:p>`)
	_, err := p.Bind(map[string]any{"show": true})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestBindRowLoopExpandsRows(t *testing.T) {
	p := bindTableDeck(t,
		tableRow("", tableCell("", "Name")+tableCell("", "Qty"))+
			tableRow("370840",
				multiParaCell("{{#each rows}}", "{{name}}", "{{/each}}")+
					tableCell("", "{{qty}}")))
	rep, err := p.Bind(map[string]any{"rows": []any{
		map[string]any{"name": "Alpha", "qty": 1},
		map[string]any{"name": "Beta", "qty": 2},
		map[string]any{"name": "Gamma", "qty": 3},
	}})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.RowsGenerated != 3 {
		t.Errorf("rows generated = %d", rep.RowsGenerated)
	}
	s := mustSlide(t, p)
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	var tbl *TableShape
	for _, sh := range shapes {
		if v, ok := sh.(*TableShape); ok {
			tbl = v
		}
	}
	if tbl == nil {
		t.Fatal("no table on slide")
	}
	rows, err := tbl.RowCount()
	if err != nil {
		t.Fatalf("RowCount: %v", err)
	}
	if rows != 4 {
		t.Fatalf("rows = %d, want 4 (header + 3)", rows)
	}
	want := []string{"Alpha", "Beta", "Gamma"}
	for i, name := range want {
		cell, err := tbl.Cell(i+1, 0)
		if err != nil {
			t.Fatalf("Cell: %v", err)
		}
		tf, err := cell.TextFrame()
		if err != nil {
			t.Fatalf("TextFrame: %v", err)
		}
		paras, err := tf.Paragraphs()
		if err != nil {
			t.Fatalf("Paragraphs: %v", err)
		}
		if len(paras) != 1 {
			t.Fatalf("row %d cell0 paragraphs = %d, want 1 (markers removed)", i+1, len(paras))
		}
		txt, err := paras[0].Text()
		if err != nil {
			t.Fatalf("Text: %v", err)
		}
		if txt != name {
			t.Errorf("row %d name = %q, want %q", i+1, txt, name)
		}
		qcell, err := tbl.Cell(i+1, 1)
		if err != nil {
			t.Fatalf("Cell: %v", err)
		}
		qtf, err := qcell.TextFrame()
		if err != nil {
			t.Fatalf("TextFrame: %v", err)
		}
		qparas, err := qtf.Paragraphs()
		if err != nil {
			t.Fatalf("Paragraphs: %v", err)
		}
		qtxt, err := qparas[0].Text()
		if err != nil {
			t.Fatalf("Text: %v", err)
		}
		if want := string(rune('1' + i)); qtxt != want {
			t.Errorf("row %d qty = %q, want %q", i+1, qtxt, want)
		}
	}
}

func TestBindRowLoopEmptyRemovesTemplateRow(t *testing.T) {
	p := bindTableDeck(t,
		tableRow("", tableCell("", "Name")+tableCell("", "Qty"))+
			tableRow("370840",
				multiParaCell("{{#each rows}}", "{{name}}", "{{/each}}")+
					tableCell("", "{{qty}}")))
	rep, err := p.Bind(map[string]any{"rows": []any{}})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.RowsRemoved != 1 {
		t.Errorf("rows removed = %d", rep.RowsRemoved)
	}
	s := mustSlide(t, p)
	shapes, _ := s.Shapes()
	for _, sh := range shapes {
		if tbl, ok := sh.(*TableShape); ok {
			rows, err := tbl.RowCount()
			if err != nil {
				t.Fatalf("RowCount: %v", err)
			}
			if rows != 1 {
				t.Fatalf("rows = %d, want 1 (header only)", rows)
			}
		}
	}
}

func TestBindRowLoopEmptySingleRowRefused(t *testing.T) {
	p := bindTableDeck(t,
		tableRow("370840",
			multiParaCell("{{#each rows}}", "{{name}}", "{{/each}}")+
				tableCell("", "{{qty}}")))
	_, err := p.Bind(map[string]any{"rows": []any{}})
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("err = %v, want ErrUnsupportedEdit", err)
	}
}

func TestBindRowLoopUnpairedMarkerRejected(t *testing.T) {
	p := bindTableDeck(t,
		tableRow("", tableCell("", "Name"))+
			tableRow("370840", multiParaCell("{{#each rows}}", "{{name}}")))
	_, err := p.Bind(map[string]any{"rows": []any{map[string]any{"name": "A"}}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestBindRowLoopNonSliceRejected(t *testing.T) {
	p := bindTableDeck(t,
		tableRow("", tableCell("", "Name"))+
			tableRow("370840", multiParaCell("{{#each rows}}", "{{name}}", "{{/each}}")))
	_, err := p.Bind(map[string]any{"rows": "not a slice"})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestBindEachOutsideTableRejected(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>{{#each rows}}</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{name}}</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{/each}}</a:t></a:r></a:p>`)
	_, err := p.Bind(map[string]any{"rows": []any{map[string]any{"name": "A"}}})
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("err = %v, want ErrUnsupportedEdit", err)
	}
}

func TestBindChartData(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>Title</a:t></a:r></a:p>`)
	s := mustSlide(t, p)
	chart, err := s.AddChart(context.Background(), ChartSpec{
		Type:       ChartBar,
		Title:      "Revenue",
		Categories: []string{"A"},
		Series:     []ChartSeries{{Name: "S1", Values: []float64{1}}},
		Width:      5486400, // 6in
		Height:     3657600, // 4in
	})
	if err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	rep, err := p.Bind(map[string]any{chart.Name(): ChartData{
		Type:       ChartBar,
		Title:      "Revenue",
		Categories: []string{"Q1", "Q2"},
		Series:     []ChartSeries{{Name: "S1", Values: []float64{3, 4}}},
	}})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.ChartsBound != 1 {
		t.Errorf("charts bound = %d", rep.ChartsBound)
	}
	got, err := chart.Data()
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	if strings.Join(got.Categories, ",") != "Q1,Q2" {
		t.Errorf("categories = %v", got.Categories)
	}
	if len(got.Series) != 1 || len(got.Series[0].Values) != 2 || got.Series[0].Values[0] != 3 {
		t.Errorf("series = %+v", got.Series)
	}
}

func TestBindAtomicNoPartialWrite(t *testing.T) {
	// 页内两处占位符，其一缺键：plan 阶段拒绝，文档字节不变。
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>{{ ok }}</a:t></a:r></a:p>`+
		`<a:p><a:r><a:t>{{ missing }}</a:t></a:r></a:p>`)
	s := mustSlide(t, p)
	before := slideXML(t, s)
	rev := p.Revision()
	if _, err := p.Bind(map[string]any{"ok": "fine"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
	if got := slideXML(t, s); got != before {
		t.Errorf("slide bytes changed after failed bind:\nbefore=%s\nafter =%s", before, got)
	}
	if p.Revision() != rev {
		t.Errorf("revision = %d, want %d (no commit)", p.Revision(), rev)
	}
}

func TestBindReportPartsSortedAndRevision(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>{{ a }}</a:t></a:r></a:p>`)
	rev := p.Revision()
	rep, err := p.Bind(map[string]any{"a": "1"})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if rep.Revision != rev+1 {
		t.Errorf("revision = %d, want %d", rep.Revision, rev+1)
	}
	if len(rep.Parts) != 1 {
		t.Errorf("parts = %v", rep.Parts)
	}
}

func TestBindClosedPresentation(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>{{ a }}</a:t></a:r></a:p>`)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := p.Bind(map[string]any{"a": "1"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}
