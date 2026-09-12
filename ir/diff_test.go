package ir

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/F31/go-pptx"
)

// ---------- DIFF-01：语义 diff 与审计报告（§18.3 / §24） ----------

func diffPage(index int, id int, part string, shapes ...Shape) Page {
	return Page{Index: index, SlideID: pptx.SlideID(id), Part: part, Shapes: shapes}
}

func diffShape(id int, kind, name, text string) Shape {
	return Shape{ID: pptx.ShapeID(id), Name: name, Kind: kind, Text: text,
		NodePath: "p:sld/p:cSld/p:spTree/p:sp[" + strconv.Itoa(id) + "]"}
}

func doc(pages ...Page) *Document {
	return &Document{SchemaVersion: SchemaVersion, Pages: pages}
}

func kindsOf(rep DiffReport) []string {
	out := make([]string, 0, len(rep.Entries))
	for _, e := range rep.Entries {
		out = append(out, e.Kind)
	}
	return out
}

func hasKind(rep DiffReport, kind string) bool {
	for _, e := range rep.Entries {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func TestDiffIdenticalDocuments(t *testing.T) {
	a := doc(diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "Body 1", "Hello")))
	b := doc(diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "Body 1", "Hello")))
	rep := Diff(a, b)
	if rep.Changed {
		t.Fatalf("changed = true, entries = %+v", rep.Entries)
	}
	if len(rep.Entries) != 0 {
		t.Errorf("entries = %+v", rep.Entries)
	}
	if rep.SchemaVersion != SchemaVersionDiff {
		t.Errorf("schemaVersion = %q", rep.SchemaVersion)
	}
	if rep.A.Pages != 1 || rep.B.Pages != 1 {
		t.Errorf("source refs = %+v / %+v", rep.A, rep.B)
	}
}

func TestDiffTextChangedWithNodePath(t *testing.T) {
	a := doc(diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "Body 1", "Hello")))
	b := doc(diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "Body 1", "Hello Go")))
	rep := Diff(a, b)
	if !rep.Changed || len(rep.Entries) != 1 {
		t.Fatalf("entries = %+v", rep.Entries)
	}
	e := rep.Entries[0]
	if e.Kind != DiffShapeText || e.Field != "text" || e.From != "Hello" || e.To != "Hello Go" {
		t.Errorf("entry = %+v", e)
	}
	// 验收：定位可回溯到 Part/NodePath。
	if e.Part != "/ppt/slides/slide1.xml" || e.NodePath != "p:sld/p:cSld/p:spTree/p:sp[2]" {
		t.Errorf("trace = %q / %q", e.Part, e.NodePath)
	}
	if rep.Stats.TextChanges != 1 || rep.Stats.ShapesChanged != 1 || rep.Stats.PagesChanged != 1 {
		t.Errorf("stats = %+v", rep.Stats)
	}
}

func TestDiffPageAddedAndRemoved(t *testing.T) {
	p0 := diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "A", "one"))
	p1 := diffPage(1, 257, "/ppt/slides/slide2.xml", diffShape(2, "autoshape", "B", "two"))
	p2 := diffPage(2, 258, "/ppt/slides/slide3.xml", diffShape(2, "autoshape", "C", "three"))
	rep := Diff(doc(p0, p1), doc(p0, p1, p2))
	if !hasKind(rep, DiffPageAdded) || rep.Stats.PagesAdded != 1 {
		t.Fatalf("added: entries = %v, stats = %+v", kindsOf(rep), rep.Stats)
	}
	rep = Diff(doc(p0, p1, p2), doc(p0, p1))
	if !hasKind(rep, DiffPageRemoved) || rep.Stats.PagesRemoved != 1 {
		t.Fatalf("removed: entries = %v, stats = %+v", kindsOf(rep), rep.Stats)
	}
	// 删除页条目带 A 侧定位。
	for _, e := range rep.Entries {
		if e.Kind == DiffPageRemoved && (e.Part != "/ppt/slides/slide3.xml" || e.PageIndexA != 2) {
			t.Errorf("removed entry = %+v", e)
		}
	}
}

func TestDiffPageMovedByLCS(t *testing.T) {
	p0 := diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "A", "one"))
	p1 := diffPage(1, 257, "/ppt/slides/slide2.xml", diffShape(2, "autoshape", "B", "two"))
	// 交换顺序：两页内容不变，仅位置变化 → 两次 page.moved。
	rep := Diff(doc(p0, p1), doc(
		diffPage(0, 257, "/ppt/slides/slide2.xml", diffShape(2, "autoshape", "B", "two")),
		diffPage(1, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "A", "one")),
	))
	if rep.Stats.PagesMoved != 2 {
		t.Fatalf("moved = %d, entries = %v", rep.Stats.PagesMoved, kindsOf(rep))
	}
	if !hasKind(rep, DiffPageMoved) {
		t.Errorf("entries = %+v", rep.Entries)
	}
}

func TestDiffShapeAddedAndRemoved(t *testing.T) {
	base := diffShape(2, "autoshape", "A", "one")
	extra := diffShape(3, "autoshape", "B", "two")
	rep := Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", base)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", base, extra)),
	)
	if !hasKind(rep, DiffShapeAdded) || rep.Stats.ShapesAdded != 1 {
		t.Fatalf("added: %v / %+v", kindsOf(rep), rep.Stats)
	}
	rep = Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", base, extra)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", base)),
	)
	if !hasKind(rep, DiffShapeRemoved) || rep.Stats.ShapesRemoved != 1 {
		t.Fatalf("removed: %v / %+v", kindsOf(rep), rep.Stats)
	}
}

func TestDiffGroupChildrenRecurse(t *testing.T) {
	child := diffShape(4, "autoshape", "Child", "old")
	child2 := diffShape(4, "autoshape", "Child", "new")
	group := diffShape(3, "group", "Group", "")
	group.Children = []Shape{child}
	group2 := diffShape(3, "group", "Group", "")
	group2.Children = []Shape{child2}
	rep := Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", group)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", group2)),
	)
	if !hasKind(rep, DiffShapeText) {
		t.Fatalf("child text change missing: %+v", rep.Entries)
	}
	// 组类别属 opaque 覆盖：应登记 opaque 区域。
	if len(rep.Opaque) != 1 || rep.Opaque[0].Reason == "" || !rep.Opaque[0].Changed {
		t.Errorf("opaque = %+v", rep.Opaque)
	}
}

func TestDiffOpaqueRegionReportedUnchanged(t *testing.T) {
	// 图片：内容未投影；几何变化 → opaque 区域 + opaque.diff 条目，
	// 不猜测像素差异。
	pic := diffShape(2, "picture", "Logo", "")
	pic.Bounds = &Box{X: 0, Y: 0, Width: 100, Height: 100}
	pic2 := pic
	pic2.Bounds = &Box{X: 10, Y: 0, Width: 100, Height: 100}
	rep := Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", pic)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", pic2)),
	)
	if !hasKind(rep, DiffOpaqueChanged) {
		t.Fatalf("opaque.diff missing: %+v", rep.Entries)
	}
	if len(rep.Opaque) != 1 {
		t.Fatalf("opaque regions = %+v", rep.Opaque)
	}
	o := rep.Opaque[0]
	if o.Part != "/ppt/slides/slide1.xml" || o.NodePath == "" || !o.Changed {
		t.Errorf("opaque region = %+v", o)
	}
	if !strings.Contains(o.Reason, "图片") {
		t.Errorf("reason = %q", o.Reason)
	}
	// 完全一致时仍登记 coverage，但 Changed=false 且无变更条目。
	rep = Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", pic)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", pic)),
	)
	if len(rep.Opaque) != 1 || rep.Opaque[0].Changed {
		t.Errorf("opaque coverage = %+v", rep.Opaque)
	}
	if rep.Changed {
		t.Errorf("changed = true, entries = %+v", rep.Entries)
	}
}

func TestDiffIgnoreGeometryAndWhitespace(t *testing.T) {
	s1 := diffShape(2, "autoshape", "A", "Hello   World")
	s1.Bounds = &Box{X: 0, Y: 0, Width: 10, Height: 10}
	s2 := diffShape(2, "autoshape", "A", "Hello World")
	s2.Bounds = &Box{X: 999, Y: 0, Width: 10, Height: 10}
	rep := Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", s1)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", s2)),
	)
	if !hasKind(rep, DiffShapeBounds) || !hasKind(rep, DiffShapeText) {
		t.Fatalf("default entries = %v", kindsOf(rep))
	}
	rep = Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", s1)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", s2)),
		WithIgnoreGeometry(true), WithIgnoreWhitespace(true),
	)
	if rep.Changed {
		t.Fatalf("ignored entries = %+v", rep.Entries)
	}
}

func TestDiffNotesAndTiming(t *testing.T) {
	p1 := diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "A", "x"))
	p1.NotesText = "note A"
	p2 := diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "A", "x"))
	p2.NotesText = "note B"
	rep := Diff(doc(p1), doc(p2))
	if !hasKind(rep, DiffPageNotes) || rep.Stats.NotesChanges != 1 {
		t.Fatalf("notes: %v / %+v", kindsOf(rep), rep.Stats)
	}
	rep = Diff(doc(p1), doc(p2), WithIgnoreNotes(true))
	if hasKind(rep, DiffPageNotes) {
		t.Errorf("notes not ignored: %+v", rep.Entries)
	}

	// 时序 IR 摘要变化。
	t1 := diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "A", "x"))
	t1.HasTiming = true
	t1.Timing = &PageTiming{Totals: TimingTotals{NodeCount: 2, AudioNodes: 1}}
	t2 := t1
	t2.Timing = &PageTiming{Totals: TimingTotals{NodeCount: 3, AudioNodes: 1}}
	rep = Diff(doc(t1), doc(t2))
	if !hasKind(rep, DiffPageTiming) || rep.Stats.TimingChanges != 1 {
		t.Fatalf("timing: %v / %+v", kindsOf(rep), rep.Stats)
	}
}

func TestDiffCoreChanged(t *testing.T) {
	a := doc()
	a.Core = &Core{Title: "Q2"}
	b := doc()
	b.Core = &Core{Title: "Q3"}
	rep := Diff(a, b)
	if !hasKind(rep, DiffCoreChanged) {
		t.Fatalf("core: %+v", rep.Entries)
	}
	if rep.Entries[0].Field != "title" || rep.Entries[0].From != "Q2" || rep.Entries[0].To != "Q3" {
		t.Errorf("entry = %+v", rep.Entries[0])
	}
}

func TestDiffTableSizeAndChartType(t *testing.T) {
	tbl1 := diffShape(2, "table", "T", "")
	tbl1.TableRows, tbl1.TableCols = 2, 3
	tbl2 := tbl1
	tbl2.TableRows, tbl2.TableCols = 3, 3
	rep := Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", tbl1)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", tbl2)),
	)
	if !hasKind(rep, DiffShapeTableSize) {
		t.Fatalf("table: %+v", rep.Entries)
	}

	c1 := diffShape(2, "chart", "C", "")
	c1.ChartType = "bar"
	c2 := c1
	c2.ChartType = "line"
	rep = Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", c1)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", c2)),
	)
	if !hasKind(rep, DiffShapeChartType) {
		t.Fatalf("chart: %+v", rep.Entries)
	}
}

func TestDiffMaxEntriesTruncates(t *testing.T) {
	var shapesA, shapesB []Shape
	for i := 0; i < 10; i++ {
		shapesA = append(shapesA, diffShape(i+2, "autoshape", "S", "old"))
		shapesB = append(shapesB, diffShape(i+2, "autoshape", "S", "new"))
	}
	rep := Diff(
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", shapesA...)),
		doc(diffPage(0, 256, "/ppt/slides/slide1.xml", shapesB...)),
		WithMaxEntries(3),
	)
	if len(rep.Entries) != 3 || !rep.Truncated {
		t.Fatalf("entries = %d, truncated = %v", len(rep.Entries), rep.Truncated)
	}
	if rep.Stats.TextChanges != 10 {
		t.Errorf("stats should keep counting: %+v", rep.Stats)
	}
}

func TestDiffNilSideTreatedAsEmpty(t *testing.T) {
	p := diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "autoshape", "A", "x"))
	rep := Diff(nil, doc(p))
	if !rep.Changed || rep.Stats.PagesAdded != 1 || !hasKind(rep, DiffPageAdded) {
		t.Fatalf("nil A: %v / %+v", kindsOf(rep), rep.Stats)
	}
	rep = Diff(doc(p), nil)
	if !rep.Changed || rep.Stats.PagesRemoved != 1 || !hasKind(rep, DiffPageRemoved) {
		t.Fatalf("nil B: %v / %+v", kindsOf(rep), rep.Stats)
	}
}

func TestPurePageScoreAndShapeIDSet(t *testing.T) {
	a := diffShape(2, "autoshape", "A", "")
	b := diffShape(3, "autoshape", "B", "")
	c := diffShape(9, "autoshape", "A", "")
	ids := shapeIDSet([]Shape{a, b})
	if len(ids) != 2 || !ids[2] || !ids[3] {
		t.Fatalf("shapeIDSet = %v", ids)
	}
	child := diffShape(9, "autoshape", "C", "")
	g := diffShape(5, "group", "G", "")
	g.Children = []Shape{child}
	if got := shapeIDSet([]Shape{g}); len(got) != 2 || !got[5] || !got[9] {
		t.Fatalf("shapeIDSet with group = %v", got)
	}
	p1 := diffPage(0, 256, "/ppt/slides/slide1.xml", a)
	p2 := diffPage(0, 257, "/ppt/slides/slide1.xml", b)
	p3 := diffPage(0, 256, "/ppt/slides/slide1.xml", c)
	if got := pageScore(p1, p3); got != 1 {
		t.Fatalf("identical SlideID score = %v, want 1", got)
	}
	// 两个非零不同 SlideID → 0。
	if got := pageScore(p1, p2); got != 0 {
		t.Fatalf("different SlideID score = %v, want 0", got)
	}
	// 双方都无形状 → 1。
	empty := diffPage(0, 0, "/ppt/slides/slide1.xml")
	if got := pageScore(empty, diffPage(1, 0, "/ppt/slides/slide2.xml")); got != 1 {
		t.Fatalf("empty-empty score = %v, want 1", got)
	}
	// 一方有形状、一方空 → Jaccard = 0。
	if got := pageScore(empty, p1); got != 0 {
		t.Fatalf("empty-vs-shapes score = %v, want 0", got)
	}
	// 部分交集。
	partial1 := diffPage(0, 0, "/ppt/slides/slide1.xml", a, b)
	partial2 := diffPage(0, 0, "/ppt/slides/slide2.xml", a, c)
	if got := pageScore(partial1, partial2); got != 1.0/3.0 {
		t.Fatalf("partial overlap score = %v, want 1/3", got)
	}
}

func TestPureBoxString(t *testing.T) {
	if got := boxString(nil); got != "" {
		t.Fatalf("boxString(nil) = %q", got)
	}
	if got := boxString(&Box{X: 1, Y: 2, Width: 300, Height: 400}); got != "(1,2 300x400)" {
		t.Fatalf("boxString = %q", got)
	}
	if got := boxEqual(nil, nil); !got {
		t.Fatal("boxEqual(nil, nil) = false")
	}
	if got := boxEqual(&Box{X: 1}, nil); got {
		t.Fatal("boxEqual(&Box, nil) = true")
	}
	if got := boxEqual(nil, &Box{X: 1}); got {
		t.Fatal("boxEqual(nil, &Box) = true")
	}
	if got := boxEqual(&Box{X: 1, Y: 2}, &Box{X: 1, Y: 2}); !got {
		t.Fatal("boxEqual equal boxes = false")
	}
	if got := boxEqual(&Box{X: 1}, &Box{X: 2}); got {
		t.Fatal("boxEqual different boxes = true")
	}
}

func TestDiffReportJSONRoundTrip(t *testing.T) {
	a := doc(diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "picture", "Logo", "")))
	b := doc(diffPage(0, 256, "/ppt/slides/slide1.xml", diffShape(2, "picture", "Logo", "")))
	b.Pages[0].Shapes[0].AltText = "new alt"
	rep := Diff(a, b)
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back DiffReport
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.SchemaVersion != SchemaVersionDiff || back.Stats.OpaqueRegions != 1 {
		t.Errorf("round-trip = %+v", back)
	}
	if len(back.Opaque) != 1 || back.Opaque[0].NodePath == "" {
		t.Errorf("opaque lost: %+v", back.Opaque)
	}
}

func TestDiffLCSFallbackLargeDocuments(t *testing.T) {
	// 超过 lcs 单元格上限时退化为下标对齐（不 panic、仍产出结果）。
	const n = 2500
	a, b := make([]Page, 0, n), make([]Page, 0, n)
	for i := 0; i < n; i++ {
		a = append(a, diffPage(i, 256+i, "/ppt/slides/slide1.xml",
			diffShape(2, "autoshape", "S", "same")))
		b = append(b, diffPage(i, 256+i, "/ppt/slides/slide1.xml",
			diffShape(2, "autoshape", "S", "same")))
	}
	rep := Diff(doc(a...), doc(b...))
	if rep.Changed {
		t.Fatalf("identical large docs changed: %d entries", len(rep.Entries))
	}
}
