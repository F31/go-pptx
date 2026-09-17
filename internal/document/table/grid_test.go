package table

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

const dd = "http://schemas.openxmlformats.org/drawingml/2006/main"

func idx(t *testing.T, s string) *xmlstore.XMLDocument {
	t.Helper()
	d, err := xmlstore.Index([]byte(s))
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	return d
}

// TestInferCols 验证无 a:tblGrid 时的列数推断：普通 tc 计 1、gridSpan
// 计跨度、hMerge continuation 计 1、取各行最大值、空表返回 0。
func TestInferCols(t *testing.T) {
	doc := idx(t, `<a:tbl xmlns:a="`+dd+`">`+
		`<a:tr><a:tc/><a:tc gridSpan="2"/></a:tr>`+ // 1 + 2 = 3
		`<a:tr><a:tc/><a:tc/><a:tc/><a:tc hMerge="1"/></a:tr>`+ // 3 + 1 = 4
		`<a:tr><a:notTc/></a:tr>`+ // 非 tc 不计
		`</a:tbl>`)
	root := doc.Root()
	var trs []*xmlstore.NodeRecord
	for _, cid := range root.Children {
		if n := doc.Node(cid); n.Namespace == ooxmlns.DrawingML && n.Local() == "tr" {
			trs = append(trs, n)
		}
	}
	if len(trs) != 3 {
		t.Fatalf("trs = %d, want 3", len(trs))
	}
	if got := InferCols(doc, trs); got != 4 {
		t.Errorf("InferCols = %d, want 4 (row2: 3 normal + 1 hMerge continuation)", got)
	}
	if got := InferCols(doc, trs[:1]); got != 3 {
		t.Errorf("InferCols(row1) = %d, want 3 (1 + gridSpan 2)", got)
	}
	if got := InferCols(doc, nil); got != 0 {
		t.Errorf("InferCols(nil) = %d, want 0", got)
	}
}

func TestBuildGrid(t *testing.T) {
	doc := idx(t, `<a:tbl xmlns:a="`+dd+`">`+
		`<a:tblGrid><a:gridCol w="1"/><a:gridCol w="1"/><a:gridCol w="1"/></a:tblGrid>`+
		`<a:tr><a:tc><a:txBody><a:p><a:r><a:t>Hi</a:t></a:r></a:p></a:txBody></a:tc><a:tc hMerge="1"/><a:tc/></a:tr>`+
		`<a:tr><a:tc/><a:tc rowSpan="2"/><a:tc/></a:tr>`+
		`</a:tbl>`)
	g, err := BuildGrid(doc, doc.Root())
	if err != nil {
		t.Fatalf("BuildGrid: %v", err)
	}
	if g.Rows() != 2 || g.Cols != 3 {
		t.Fatalf("grid = %dx%d, want 2x3", g.Rows(), g.Cols)
	}
	if len(g.TRs) != 2 || len(g.GCols) != 3 {
		t.Fatalf("TRs=%d GCols=%d", len(g.TRs), len(g.GCols))
	}
	s00 := g.At(0, 0)
	if s00 == nil || s00.IsContinuation || s00.Row != 0 || s00.Col != 0 || s00.TC == nil {
		t.Fatalf("At(0,0) = %+v", s00)
	}
	s01 := g.At(0, 1)
	if s01 == nil || !s01.IsContinuation || s01.Anchor != s00.Anchor {
		t.Fatalf("At(0,1) = %+v (anchor=%p want %p)", s01, s01.Anchor, s00.Anchor)
	}
	if s := g.At(1, 1); s == nil || s.Row != 1 || s.Col != 1 || s.IsContinuation {
		t.Fatalf("At(1,1) = %+v", s)
	}
	if g.At(9, 0) != nil || g.At(0, 9) != nil || g.At(-1, 0) != nil {
		t.Error("out-of-range At should be nil")
	}
	// 无 tblGrid 时按推断列数构建。
	noGrid := idx(t, `<a:tbl xmlns:a="`+dd+`"><a:tr><a:tc/><a:tc/></a:tr></a:tbl>`)
	g2, err := BuildGrid(noGrid, noGrid.Root())
	if err != nil || g2.Cols != 2 || g2.At(0, 1) == nil {
		t.Fatalf("inferred grid = %+v err=%v", g2, err)
	}
	// 空表：不构建 slots。
	empty := idx(t, `<a:tbl xmlns:a="`+dd+`"/>`)
	g3, err := BuildGrid(empty, empty.Root())
	if err != nil || g3.Rows() != 0 || g3.Cols != 0 {
		t.Fatalf("empty grid = %+v err=%v", g3, err)
	}
}

func TestCellSpansAndContinuation(t *testing.T) {
	plain := idx(t, `<a:tc xmlns:a="`+dd+`"/>`).Root()
	if gs, rs := CellSpans(plain); gs != 1 || rs != 1 {
		t.Errorf("default spans = %d,%d", gs, rs)
	}
	span := idx(t, `<a:tc xmlns:a="`+dd+`" gridSpan="2" rowSpan="3"/>`).Root()
	if gs, rs := CellSpans(span); gs != 2 || rs != 3 {
		t.Errorf("spans = %d,%d", gs, rs)
	}
	bad := idx(t, `<a:tc xmlns:a="`+dd+`" gridSpan="0" rowSpan="x"/>`).Root()
	if gs, rs := CellSpans(bad); gs != 1 || rs != 1 {
		t.Errorf("invalid spans = %d,%d", gs, rs)
	}
	if !CellIsContinuation(idx(t, `<a:tc xmlns:a="`+dd+`" hMerge="1"/>`).Root()) {
		t.Error("hMerge=1 should be continuation")
	}
	if !CellIsContinuation(idx(t, `<a:tc xmlns:a="`+dd+`" vMerge="true"/>`).Root()) {
		t.Error("vMerge=true should be continuation")
	}
	if CellIsContinuation(idx(t, `<a:tc xmlns:a="`+dd+`" hMerge="0"/>`).Root()) {
		t.Error("hMerge=0 should not be continuation")
	}
	if CellIsContinuation(plain) {
		t.Error("plain tc should not be continuation")
	}
}

func TestCellHasTextAndClear(t *testing.T) {
	textDoc := idx(t, `<a:tc xmlns:a="`+dd+`"><a:txBody><a:p><a:r><a:t>Hi</a:t></a:r></a:p></a:txBody></a:tc>`)
	if !CellHasText(textDoc, textDoc.Root()) {
		t.Error("text cell should be non-empty")
	}
	wsDoc := idx(t, `<a:tc xmlns:a="`+dd+`"><a:txBody><a:p><a:r><a:t>   </a:t></a:r></a:p></a:txBody></a:tc>`)
	if CellHasText(wsDoc, wsDoc.Root()) {
		t.Error("whitespace cell should be empty")
	}
	fldDoc := idx(t, `<a:tc xmlns:a="`+dd+`"><a:txBody><a:p><a:fld/></a:p></a:txBody></a:tc>`)
	if !CellHasText(fldDoc, fldDoc.Root()) {
		t.Error("field cell should be non-empty")
	}
	noBody := idx(t, `<a:tc xmlns:a="`+dd+`"/>`)
	if CellHasText(noBody, noBody.Root()) {
		t.Error("no txBody should be empty")
	}
	// 自闭合 a:t → 空。
	selfT := idx(t, `<a:tc xmlns:a="`+dd+`"><a:txBody><a:p><a:r><a:t/></a:r></a:p></a:txBody></a:tc>`)
	if CellHasText(selfT, selfT.Root()) {
		t.Error("self-closing t should be empty")
	}
	// ClearCellTextPatches：2 个 a:p → 2 补丁；无 txBody → nil。
	doc := idx(t, `<a:tc xmlns:a="`+dd+`"><a:txBody><a:bodyPr/><a:p/><a:p/></a:txBody></a:tc>`)
	if got := ClearCellTextPatches(doc, doc.Root()); len(got) != 2 {
		t.Errorf("clear patches = %d, want 2", len(got))
	}
	if ClearCellTextPatches(noBody, noBody.Root()) != nil {
		t.Error("no txBody clear should be nil")
	}
}

func TestTableOfGraphic(t *testing.T) {
	good := idx(t, `<p:graphicFrame xmlns:p="urn:p" xmlns:a="`+dd+`">`+
		`<a:graphic><a:graphicData uri="`+GraphicURI+`"><a:tbl/></a:graphicData></a:graphic></p:graphicFrame>`)
	if TableOfGraphic(good, good.Root()) == nil {
		t.Error("table frame should resolve a:tbl")
	}
	mismatch := idx(t, `<p:graphicFrame xmlns:p="urn:p" xmlns:a="`+dd+`">`+
		`<a:graphic><a:graphicData uri="urn:other"><a:tbl/></a:graphicData></a:graphic></p:graphicFrame>`)
	if TableOfGraphic(mismatch, mismatch.Root()) != nil {
		t.Error("uri mismatch should be nil")
	}
	noGraphic := idx(t, `<p:graphicFrame xmlns:p="urn:p"/>`)
	if TableOfGraphic(noGraphic, noGraphic.Root()) != nil {
		t.Error("no graphic should be nil")
	}
}
