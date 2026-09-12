package pptx

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- TABLE-01：富文本表格、合并、样式子集（§9.1） ----------

// tableCell 构造 a:tc（可选文本与属性）。
func tableCell(attrs, text string) string {
	tx := `<a:txBody><a:bodyPr/><a:p/>`
	if text != "" {
		tx = `<a:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p>`
	}
	tx += `</a:txBody>`
	return `<a:tc` + attrs + `>` + tx + `<a:tcPr/></a:tc>`
}

// tableRow 构造 a:tr。
func tableRow(h string, cells string) string {
	attr := ""
	if h != "" {
		attr = ` h="` + h + `"`
	}
	return `<a:tr` + attr + `>` + cells + `</a:tr>`
}

// tableFrame 构造含表格的 p:graphicFrame（tblPr 可选属性）。
func tableFrame(id, tblPrAttrs string, widths []string, rows string) string {
	grid := `<a:tblGrid>`
	for _, w := range widths {
		grid += `<a:gridCol w="` + w + `"/>`
	}
	grid += `</a:tblGrid>`
	return `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="` + id + `" name="Table ` + id + `"/>` +
		`<p:cNvGraphicFramePr><a:graphicFrameLocks noGrp="1"/></p:cNvGraphicFramePr><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="1000" y="2000"/><a:ext cx="5000" cy="3000"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="` + tblGraphicURI + `"><a:tbl>` +
		`<a:tblPr` + tblPrAttrs + `/>` + grid + rows +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
}

// tableDeck 构造含给定 spTree 内容的单页文档（可附加 tableStyles.xml）。
func tableDeck(t *testing.T, spTreeBody string, tableStyles string) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` + spTreeBody +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
	ct := `<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`
	parts["/[Content_Types].xml"] = append(
		bytes.TrimSuffix(parts["/[Content_Types].xml"], []byte("</Types>")),
		[]byte(ct+"</Types>")...)
	mainRels := `<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`<Relationship Id="rId2" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>`
	if tableStyles != "" {
		parts["/ppt/tableStyles.xml"] = []byte(xmlDecl + tableStyles)
		parts["/[Content_Types].xml"] = append(
			bytes.TrimSuffix(parts["/[Content_Types].xml"], []byte("</Types>")),
			[]byte(`<Override PartName="/ppt/tableStyles.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml"/></Types>`)...)
		mainRels += `<Relationship Id="rId3" Type="` + relTableStyles + `" Target="tableStyles.xml"/>`
	}
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` + mainRels + `</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	return openFixture(t, buildPackageZipPanic(parts))
}

// firstTable 返回页面首个表格句柄。
func firstTable(t *testing.T, p *Presentation) *TableShape {
	t.Helper()
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	for _, sh := range shapes {
		if tb, ok := sh.(*TableShape); ok {
			return tb
		}
	}
	t.Fatal("no table shape on slide")
	return nil
}

func TestTableShapeClassificationAndSize(t *testing.T) {
	rows := tableRow("370840", tableCell("", "A1")+tableCell("", "B1")) +
		tableRow("370840", tableCell("", "A2")+tableCell("", "B2"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()

	s, _ := p.Slide(0)
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	if len(shapes) != 1 || shapes[0].Kind() != ShapeTable {
		t.Fatalf("shapes kinds = %v, want single table", shapes)
	}
	tb := shapes[0].(*TableShape)
	if n, err := tb.RowCount(); err != nil || n != 2 {
		t.Errorf("RowCount = %d, %v; want 2", n, err)
	}
	if n, err := tb.ColumnCount(); err != nil || n != 2 {
		t.Errorf("ColumnCount = %d, %v; want 2", n, err)
	}
	// 几何经 GEOM-01 的 graphicFrame → p:xfrm 路径。
	b, err := tb.Bounds()
	if err != nil {
		t.Fatalf("Bounds: %v", err)
	}
	if b != (Rect{X: 1000, Y: 2000, W: 5000, H: 3000}) {
		t.Errorf("table Bounds = %+v", b)
	}
}

// Cell.Row/Column 是公开 trivial getter（table.go:341-342），返回创建句柄
// 时的 c.row/c.col；此前覆盖率 0% 仅因无任何测试调用——本测试用 2x2 表
// 补齐 (0,0)/(0,1)/(1,0)/(1,1) 四个坐标的检索回路。
func TestCellRowAndColumnGetters(t *testing.T) {
	rows := tableRow("370840", tableCell("", "A1")+tableCell("", "B1")) +
		tableRow("370840", tableCell("", "A2")+tableCell("", "B2"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()
	s, _ := p.Slide(0)
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	tb := shapes[0].(*TableShape)
	for _, tc := range []struct{ r, c int }{{0, 0}, {0, 1}, {1, 0}, {1, 1}} {
		cell, err := tb.Cell(tc.r, tc.c)
		if err != nil {
			t.Fatalf("Cell(%d,%d): %v", tc.r, tc.c, err)
		}
		if got := cell.Row(); got != tc.r {
			t.Errorf("Cell(%d,%d).Row() = %d, want %d", tc.r, tc.c, got, tc.r)
		}
		if got := cell.Column(); got != tc.c {
			t.Errorf("Cell(%d,%d).Column() = %d, want %d", tc.r, tc.c, got, tc.c)
		}
	}
}

func TestTableCellTextReadWrite(t *testing.T) {
	rows := tableRow("", tableCell("", "标题")+tableCell("", ""))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()
	tb := firstTable(t, p)
	c, err := tb.Cell(0, 0)
	if err != nil {
		t.Fatalf("Cell: %v", err)
	}
	tf, err := c.TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	paras, err := tf.Paragraphs()
	if err != nil {
		t.Fatalf("Paragraphs: %v", err)
	}
	if len(paras) != 1 {
		t.Fatalf("paragraphs = %d", len(paras))
	}
	if txt, err := paras[0].Text(); err != nil || txt != "标题" {
		t.Errorf("cell text = %q, %v", txt, err)
	}
	// 写入正文（TEXT-01 语义复用）。
	if err := tf.SetPlainText("新内容\n第二行"); err != nil {
		t.Fatalf("SetPlainText: %v", err)
	}
	tf2, _ := tb.Cell(0, 0)
	tf2h, err := tf2.TextFrame()
	if err != nil {
		t.Fatalf("TextFrame after: %v", err)
	}
	ps, _ := tf2h.Paragraphs()
	if len(ps) != 2 {
		t.Fatalf("paragraphs after = %d, want 2", len(ps))
	}
	if txt, _ := ps[0].Text(); txt != "新内容" {
		t.Errorf("para0 = %q", txt)
	}
	// 越界。
	if _, err := tb.Cell(2, 0); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("Cell(2,0) err = %v, want ErrOutOfRange", err)
	}
}

// 合并：横向两格（AT-09 单非空通过）。
func TestTableMergeHorizontal(t *testing.T) {
	rows := tableRow("", tableCell("", "A")+tableCell("", ""))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800", "304800"},
		tableRow("", tableCell("", "A")+tableCell("", "")+tableCell("", "C"))), "")
	defer p.Close()
	tb := firstTable(t, p)
	if err := tb.Merge(CellRange{Row: 0, Col: 0, Rows: 1, Cols: 2}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	anchor, err := tb.Cell(0, 0)
	if err != nil {
		t.Fatalf("Cell: %v", err)
	}
	gs, rs, err := anchor.Spans()
	if err != nil || gs != 2 || rs != 1 {
		t.Errorf("anchor spans = %d,%d (%v), want 2,1", gs, rs, err)
	}
	cont, err := tb.Cell(0, 1)
	if err != nil {
		t.Fatalf("Cell(0,1): %v", err)
	}
	if ok, err := cont.IsContinuation(); err != nil || !ok {
		t.Errorf("continuation flag = %v, %v; want true", ok, err)
	}
	if merged, err := cont.IsMerged(); err != nil || !merged {
		t.Errorf("IsMerged = %v, %v; want true", merged, err)
	}
	// 合并区外的单元格不受影响。
	other, _ := tb.Cell(0, 2)
	tf, _ := other.TextFrame()
	ps, _ := tf.Paragraphs()
	if txt, _ := ps[0].Text(); txt != "C" {
		t.Errorf("untouched cell text = %q, want C", txt)
	}
	_ = rows
}

// AT-09：合并区域含多个非空单元格 → 默认拒绝，原单元格与样式不变。
func TestTableMergeRejectsMultipleNonEmpty(t *testing.T) {
	rows := tableRow("", tableCell("", "A")+tableCell("", "B"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()
	tb := firstTable(t, p)
	err := tb.Merge(CellRange{Row: 0, Col: 0, Rows: 1, Cols: 2})
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("Merge err = %v, want ErrUnsupportedEdit (AT-09)", err)
	}
	// 拒绝后原单元格与样式不变（无 span、无 continuation）。
	for _, col := range []int{0, 1} {
		c, err := tb.Cell(0, col)
		if err != nil {
			t.Fatalf("Cell: %v", err)
		}
		gs, rs, err := c.Spans()
		if err != nil || gs != 1 || rs != 1 {
			t.Errorf("cell(%d) spans = %d,%d after rejected merge, want 1,1", col, gs, rs)
		}
		if ok, _ := c.IsContinuation(); ok {
			t.Errorf("cell(%d) marked as continuation after rejected merge", col)
		}
	}
	// 明确保留锚点文本的策略可执行。
	if err := tb.Merge(CellRange{Row: 0, Col: 0, Rows: 1, Cols: 2}, WithMergeTextPolicy(MergeKeepAnchorText)); err != nil {
		t.Fatalf("Merge(keep anchor): %v", err)
	}
	anchor, _ := tb.Cell(0, 0)
	tf, _ := anchor.TextFrame()
	ps, _ := tf.Paragraphs()
	if txt, _ := ps[0].Text(); txt != "A" {
		t.Errorf("anchor text after merge = %q, want A", txt)
	}
	cont, _ := tb.Cell(0, 1)
	ctf, err := cont.TextFrame()
	if err != nil {
		t.Fatalf("continuation TextFrame: %v", err)
	}
	cps, _ := ctf.Paragraphs()
	for _, pp := range cps {
		if txt, _ := pp.Text(); txt != "" {
			t.Errorf("continuation text = %q, want empty", txt)
		}
	}
}

// 纵向合并：rowSpan 与第二行 vMerge continuation。
func TestTableMergeVertical(t *testing.T) {
	rows := tableRow("", tableCell("", "A")+tableCell("", "B")) +
		tableRow("", tableCell("", "")+tableCell("", "D"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()
	tb := firstTable(t, p)
	if err := tb.Merge(CellRange{Row: 0, Col: 0, Rows: 2, Cols: 1}); err != nil {
		t.Fatalf("Merge vertical: %v", err)
	}
	anchor, _ := tb.Cell(0, 0)
	gs, rs, _ := anchor.Spans()
	if gs != 1 || rs != 2 {
		t.Errorf("anchor spans = %d,%d, want 1,2", gs, rs)
	}
	// 第二行同列是 continuation，且仍映射到同一 DOM 单元格区域。
	cont, _ := tb.Cell(1, 0)
	if ok, _ := cont.IsContinuation(); !ok {
		t.Error("row 1 col 0 is not continuation after vertical merge")
	}
	// 第二行另一列不受影响。
	other, _ := tb.Cell(1, 1)
	tf, _ := other.TextFrame()
	ps, _ := tf.Paragraphs()
	if txt, _ := ps[0].Text(); txt != "D" {
		t.Errorf("row1 col1 text = %q, want D", txt)
	}
}

// 既有合并跨出区域边界 → 拒绝（不重叠要求）。
func TestTableMergeRejectsOverlap(t *testing.T) {
	// 锚点已跨 2 列，再合并 (0,1)-(0,2) 会切断既有合并。
	rows := tableRow("", tableCell(` gridSpan="2"`, "A")+tableCell(` hMerge="1"`, "")+tableCell("", "C"))
	p := tableDeck(t, tableFrame("4", "", []string{"200000", "200000", "200000"}, rows), "")
	defer p.Close()
	tb := firstTable(t, p)
	if err := tb.Merge(CellRange{Row: 0, Col: 1, Rows: 1, Cols: 2}); !errors.Is(err, ErrUnsupportedEdit) {
		t.Errorf("overlapping Merge err = %v, want ErrUnsupportedEdit", err)
	}
}

func TestTableUnmergeClearsSpans(t *testing.T) {
	rows := tableRow("", tableCell(` gridSpan="2"`, "A")+tableCell(` hMerge="1"`, ""))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()
	tb := firstTable(t, p)
	c0, _ := tb.Cell(0, 0)
	if gs, _, _ := c0.Spans(); gs != 2 {
		t.Fatalf("initial gridSpan = %d, want 2", gs)
	}
	if err := tb.Unmerge(CellRange{Row: 0, Col: 0, Rows: 1, Cols: 2}); err != nil {
		t.Fatalf("Unmerge: %v", err)
	}
	c0b, _ := tb.Cell(0, 0)
	if gs, rs, _ := c0b.Spans(); gs != 1 || rs != 1 {
		t.Errorf("after unmerge spans = %d,%d, want 1,1", gs, rs)
	}
	c1, _ := tb.Cell(0, 1)
	if ok, _ := c1.IsContinuation(); ok {
		t.Error("continuation flag remains after unmerge")
	}
	// 文本内容不被恢复但也不被额外破坏。
	tf, _ := c0b.TextFrame()
	ps, _ := tf.Paragraphs()
	if txt, _ := ps[0].Text(); txt != "A" {
		t.Errorf("anchor text after unmerge = %q, want A", txt)
	}
}

func TestTableRowHeightAndColumnWidth(t *testing.T) {
	rows := tableRow("370840", tableCell("", "A")+tableCell("", "B")) +
		tableRow("400000", tableCell("", "C")+tableCell("", "D"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "457200"}, rows), "")
	defer p.Close()
	tb := firstTable(t, p)
	if h, err := tb.RowHeight(0); err != nil || h != 370840 {
		t.Errorf("RowHeight(0) = %d, %v; want 370840", h, err)
	}
	if w, err := tb.ColumnWidth(1); err != nil || w != 457200 {
		t.Errorf("ColumnWidth(1) = %d, %v; want 457200", w, err)
	}
	if err := tb.SetRowHeight(1, 500000); err != nil {
		t.Fatalf("SetRowHeight: %v", err)
	}
	if h, _ := tb.RowHeight(1); h != 500000 {
		t.Errorf("RowHeight(1) after set = %d, want 500000", h)
	}
	if err := tb.SetColumnWidth(0, 100000); err != nil {
		t.Fatalf("SetColumnWidth: %v", err)
	}
	if w, _ := tb.ColumnWidth(0); w != 100000 {
		t.Errorf("ColumnWidth(0) after set = %d, want 100000", w)
	}
	// 负值与越界。
	if err := tb.SetRowHeight(0, -1); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("SetRowHeight(-1) err = %v, want ErrInvalidArgument", err)
	}
	if _, err := tb.RowHeight(9); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("RowHeight(9) err = %v, want ErrOutOfRange", err)
	}
}

// 样式：单元格显式填充/边框/文本属性优先于样式库。
func TestTableCellEffectiveStyleExplicit(t *testing.T) {
	cellA := `<a:tc><a:txBody><a:bodyPr/><a:p/></a:txBody>` +
		`<a:tcPr anchor="ctr" marL="91440"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill>` +
		`<a:lnL w="12700"><a:solidFill><a:srgbClr val="00FF00"/></a:solidFill></a:lnL></a:tcPr></a:tc>`
	rows := tableRow("", cellA+tableCell("", "B"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()
	tb := firstTable(t, p)
	c, _ := tb.Cell(0, 0)
	st, diags, err := c.EffectiveCellStyle()
	if err != nil {
		t.Fatalf("EffectiveCellStyle: %v", err)
	}
	if !st.Fill.Resolved || st.Fill.Value.Kind != FillSolid || st.Fill.Value.Color.RGB != "FF0000" {
		t.Errorf("fill = %+v, want solid #FF0000 resolved", st.Fill)
	}
	if st.Fill.Trace[0].Source != SourceCellExplicit {
		t.Errorf("fill source = %v, want cell-explicit", st.Fill.Trace[0].Source)
	}
	if !st.Borders.Left.Resolved || st.Borders.Left.Value.Width != 12700 ||
		st.Borders.Left.Value.Color.RGB != "00FF00" {
		t.Errorf("left border = %+v", st.Borders.Left)
	}
	if st.Borders.Top.Resolved {
		t.Error("top border should stay unresolved (not specified)")
	}
	if !st.Text.Resolved || st.Text.Value.Anchor != "ctr" || st.Text.Value.MarginLeft != 91440 {
		t.Errorf("text style = %+v", st.Text)
	}
	_ = diags
}

// 样式库：未知 styleId → unresolved；已定义 → 按区域优先级命中。
func TestTableCellStyleFromTableStyles(t *testing.T) {
	styles := `<a:tblStyleLst xmlns:a="` + nsDrawingML + `" def="{ID}">` +
		`<a:tblStyle styleId="{ID}" styleName="Test">` +
		`<a:wholeTbl><a:tcStyle><a:solidFill><a:srgbClr val="111111"/></a:solidFill></a:tcStyle></a:wholeTbl>` +
		`<a:firstRow><a:tcStyle><a:solidFill><a:srgbClr val="222222"/></a:solidFill></a:tcStyle></a:firstRow>` +
		`<a:nwCell><a:tcStyle><a:solidFill><a:srgbClr val="333333"/></a:solidFill></a:tcStyle></a:nwCell>` +
		`</a:tblStyle></a:tblStyleLst>`
	rows := tableRow("", tableCell("", "A")+tableCell("", "B")) +
		tableRow("", tableCell("", "C")+tableCell("", "D"))
	p := tableDeck(t,
		tableFrame("4", ` firstRow="1" bandRow="1" tableStyleId="{ID}"`, []string{"304800", "304800"}, rows), styles)
	defer p.Close()
	tb := firstTable(t, p)
	if id, err := tb.StyleID(); err != nil || id != "{ID}" {
		t.Errorf("StyleID = %q, %v", id, err)
	}
	flags, err := tb.StyleFlags()
	if err != nil {
		t.Fatalf("StyleFlags: %v", err)
	}
	if flags.FirstRow != ToggleOn || flags.BandRow != ToggleOn || flags.LastCol != ToggleDefault {
		t.Errorf("flags = %+v", flags)
	}
	// 角单元格 (0,0)：nwCell 优先于 firstRow。
	c00, _ := tb.Cell(0, 0)
	st00, _, err := c00.EffectiveCellStyle()
	if err != nil {
		t.Fatalf("EffectiveCellStyle: %v", err)
	}
	if !st00.StyleResolved || st00.Part != PartNWCell || st00.Fill.Value.Color.RGB != "333333" {
		t.Errorf("(0,0) part=%v rgb=%q resolved=%v, want nwCell/333333",
			st00.Part, st00.Fill.Value.Color.RGB, st00.StyleResolved)
	}
	// (0,1) 首行非角 → firstRow。
	c01, _ := tb.Cell(0, 1)
	st01, _, _ := c01.EffectiveCellStyle()
	if st01.Part != PartFirstRow || st01.Fill.Value.Color.RGB != "222222" {
		t.Errorf("(0,1) part=%v rgb=%q, want firstRow/222222", st01.Part, st01.Fill.Value.Color.RGB)
	}
	// 未知 styleId → unresolved（不臆造内置映射）。
	p2 := tableDeck(t,
		tableFrame("4", ` firstRow="1" tableStyleId="{UNKNOWN}"`, []string{"304800", "304800"},
			tableRow("", tableCell("", "A")+tableCell("", "B"))), styles)
	defer p2.Close()
	tb2 := firstTable(t, p2)
	c2, _ := tb2.Cell(0, 0)
	st2, diags, err := c2.EffectiveCellStyle()
	if err != nil {
		t.Fatalf("EffectiveCellStyle: %v", err)
	}
	if st2.StyleResolved || st2.Fill.Resolved {
		t.Errorf("unknown styleId should be unresolved: %+v", st2)
	}
	if len(diags) == 0 {
		t.Error("expected unresolved diagnostic for unknown style id")
	}
}

func TestTableHandleLifecycle(t *testing.T) {
	rows := tableRow("", tableCell("", "A")+tableCell("", "B"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	s, _ := p.Slide(0)
	tb := firstTable(t, p)
	c, _ := tb.Cell(0, 0)
	if err := p.RemoveSlide(s.ID()); err != nil {
		t.Fatalf("RemoveSlide: %v", err)
	}
	if _, err := tb.Cell(0, 0); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("stale Cell err = %v, want ErrStaleHandle", err)
	}
	if _, err := c.TextFrame(); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("stale TextFrame err = %v, want ErrStaleHandle", err)
	}

	p2 := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	tb2 := firstTable(t, p2)
	if err := p2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := tb2.RowCount(); !errors.Is(err, ErrClosed) {
		t.Errorf("closed RowCount err = %v, want ErrClosed", err)
	}
}

// 保存往返：合并状态与行高列宽持久化。
func TestTableSaveRoundTrip(t *testing.T) {
	rows := tableRow("370840", tableCell("", "A")+tableCell("", ""))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	tb := firstTable(t, p)
	if err := tb.Merge(CellRange{Row: 0, Col: 0, Rows: 1, Cols: 2}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if err := tb.SetRowHeight(0, 555555); err != nil {
		t.Fatalf("SetRowHeight: %v", err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p.Close()

	p2 := openFixture(t, buf.Bytes())
	defer p2.Close()
	tb2 := firstTable(t, p2)
	c, err := tb2.Cell(0, 0)
	if err != nil {
		t.Fatalf("Cell: %v", err)
	}
	if gs, _, _ := c.Spans(); gs != 2 {
		t.Errorf("gridSpan after save = %d, want 2", gs)
	}
	if h, _ := tb2.RowHeight(0); h != 555555 {
		t.Errorf("row height after save = %d, want 555555", h)
	}
	cont, _ := tb2.Cell(0, 1)
	if ok, _ := cont.IsContinuation(); !ok {
		t.Error("continuation flag lost across save")
	}
}

func TestTableStyleHelperEdges(t *testing.T) {
	// nil 守卫。
	if got := tblStyleNode(nil, "x"); got != nil {
		t.Fatalf("tblStyleNode(nil) = %+v", got)
	}
	if got := tblStyleNode(mustIndexTable(t, `<a:tblStyleLst xmlns:a="`+nsDrawingML+`"/>`), ""); got != nil {
		t.Fatalf("tblStyleNode empty id = %+v", got)
	}
	if got := tblStylePartNode(nil, nil, PartFirstRow); got != nil {
		t.Fatalf("tblStylePartNode(nil,nil) = %+v", got)
	}
	if got := tcStyleNode(nil, nil); got != nil {
		t.Fatalf("tcStyleNode(nil,nil) = %+v", got)
	}
	if got, _ := fillIn(nil, nil); got != nil {
		t.Fatalf("fillIn(nil,nil) = %+v", got)
	}
	// 找不到 styleId → nil。
	doc := mustIndexTable(t, `<a:tblStyleLst xmlns:a="`+nsDrawingML+`"><a:tblStyle styleId="S1"/></a:tblStyleLst>`)
	if got := tblStyleNode(doc, "nope"); got != nil {
		t.Fatalf("missing style = %+v", got)
	}
	// fillPropIn 未知填充 → 默认。
	doc2 := mustIndexTable(t, `<a:tcPr xmlns:a="`+nsDrawingML+`"><a:weirdFill/></a:tcPr>`)
	root2 := doc2.Root()
	if node, kind := fillPropIn(doc2, root2); node != nil || kind != FillUnspecified {
		t.Fatalf("unknown fill = %+v %v", node, kind)
	}
}

// mustIndexTable 构造仅含给定 spTree 内容的最小单页表格文档（白盒辅助）。
func mustIndexTable(t *testing.T, xml string) *xmlstore.XMLDocument {
	t.Helper()
	doc, err := xmlstore.Index([]byte(xml))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	return doc
}

func TestParseToggleVariants(t *testing.T) {
	if got := parseToggle("on", true); got != ToggleOn {
		t.Fatalf("on = %v", got)
	}
	if got := parseToggle("1", true); got != ToggleOn {
		t.Fatalf("1 = %v", got)
	}
	if got := parseToggle("true", true); got != ToggleOn {
		t.Fatalf("true = %v", got)
	}
	if got := parseToggle("off", true); got != ToggleOff {
		t.Fatalf("off = %v", got)
	}
	if got := parseToggle("0", true); got != ToggleOff {
		t.Fatalf("0 = %v", got)
	}
	if got := parseToggle("false", true); got != ToggleOff {
		t.Fatalf("false = %v", got)
	}
	if got := parseToggle("weird", true); got != ToggleDefault {
		t.Fatalf("weird = %v", got)
	}
	if got := parseToggle("on", false); got != ToggleDefault {
		t.Fatalf("not-ok = %v", got)
	}
}

// ---------- 纯函数（零覆盖消除，2026-09-13 第 5 轮） ----------

// TestInferCols 验证无 a:tblGrid 时的列数推断：普通 tc 计 1、gridSpan
// 计跨度、hMerge continuation 计 1、取各行最大值、空表返回 0。
func TestInferCols(t *testing.T) {
	doc, err := xmlstore.Index([]byte(
		`<a:tbl xmlns:a="` + nsDrawingML + `">` +
			`<a:tr><a:tc/><a:tc gridSpan="2"/></a:tr>` + // 1 + 2 = 3
			`<a:tr><a:tc/><a:tc/><a:tc/><a:tc hMerge="1"/></a:tr>` + // 3 + 1 = 4
			`<a:tr><a:notTc/></a:tr>` + // 非 tc 不计
			`</a:tbl>`))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	root := doc.Root()
	var trs []*xmlstore.NodeRecord
	for _, cid := range root.Children {
		if n := doc.Node(cid); n.Namespace == nsDrawingML && n.Local() == "tr" {
			trs = append(trs, n)
		}
	}
	if len(trs) != 3 {
		t.Fatalf("trs = %d, want 3", len(trs))
	}
	if got := inferCols(doc, trs); got != 4 {
		t.Errorf("inferCols = %d, want 4 (row2: 3 normal + 1 hMerge continuation)", got)
	}
	if got := inferCols(doc, trs[:1]); got != 3 {
		t.Errorf("inferCols(row1) = %d, want 3 (1 + gridSpan 2)", got)
	}
	if got := inferCols(doc, nil); got != 0 {
		t.Errorf("inferCols(nil) = %d, want 0", got)
	}
}
