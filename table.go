package pptx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 TABLE-01（方案 §9.1：富文本表格、合并、样式子集）。
//
// 表格是 p:graphicFrame 内 a:graphic/a:graphicData/a:tbl 的复合对象：
//
//	p:graphicFrame
//	  p:nvGraphicFramePr / p:xfrm              （形状层：几何与元信息）
//	  a:graphic/a:graphicData[@uri=table]/a:tbl
//	    a:tblPr           表格级样式开关与 tableStyleId
//	    a:tblGrid/a:gridCol@w     列宽（列数由 gridCol 决定）
//	    a:tr@h            行（a:tc 为单元格）
//	      a:tc[@rowSpan @gridSpan @hMerge @vMerge]
//	        a:txBody      富文本正文
//	        a:tcPr        单元格显式样式覆盖
//
// 合并语义（ECMA CT_TableCell）：锚点单元格以 gridSpan/rowSpan 声明跨
// 度，被覆盖格位上的单元格保留在 DOM 中并标记 hMerge="1"（横向）或
// vMerge="1"（纵向）作为 continuation。库按此维护逻辑网格：逻辑格位
// → (单元格节点, 锚点节点)，读取与合并均以逻辑网格为准，不按 DOM 序号
// 猜测（§9.1"合并必须验证矩形区域、不重叠"）。

// tblGraphicURI 是表格图形数据的 URI（a:graphicData@uri）。
const tblGraphicURI = "http://schemas.openxmlformats.org/drawingml/2006/table"

// ---------- 表格定位 ----------

// tableOfGraphic 返回图形框内的 a:tbl（非表格图形框返回 nil）。
func tableOfGraphic(doc *xmlstore.XMLDocument, frame *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	g := childOfKind(doc, frame, nsDrawingML, "graphic", 0)
	if g == nil {
		return nil
	}
	gd := childOfKind(doc, g, nsDrawingML, "graphicData", 0)
	if gd == nil {
		return nil
	}
	if uri, ok := gd.Attr("", "uri"); !ok || uri != tblGraphicURI {
		return nil
	}
	return childOfKind(doc, gd, nsDrawingML, "tbl", 0)
}

// ---------- TableShape ----------

// TableShape 是页面表格（p:graphicFrame + a:tbl）的受控句柄。
//
// 形状层元信息（ID/Name/AltText/几何）经 shapeNode 与 GEOM-01 一致；
// 表格层提供行列尺寸、富文本 Cell、合并与行高列宽。
type TableShape struct {
	shapeNode
}

// Kind 返回形状类别（恒为 ShapeTable）。
func (t *TableShape) Kind() ShapeKind { return ShapeTable }

// locateTbl 定位表格句柄对应的 a:tbl 元素。
func (t *TableShape) locateTbl() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, el, err := t.locate()
	if err != nil {
		return nil, nil, Annotate(err, "TableShape")
	}
	if el.Namespace != nsPresentationML || el.Local() != "graphicFrame" {
		return nil, nil, &OperationError{
			Op: "TableShape", Part: string(t.part),
			Message: fmt.Sprintf("handle target is p:%s, not p:graphicFrame", el.Local()),
			Err:     ErrUnsupportedFormat,
		}
	}
	tbl := tableOfGraphic(doc, el)
	if tbl == nil {
		return nil, nil, &OperationError{
			Op: "TableShape", Part: string(t.part),
			Message: "graphicFrame is not a table (no a:graphicData/a:tbl)", Err: ErrUnsupportedFormat,
		}
	}
	return doc, tbl, nil
}

// ---------- 逻辑网格 ----------

// cellSlot 是逻辑网格中的一个格位（对应一个 a:tc）。
type cellSlot struct {
	// tc 是该格位 DOM 中的 a:tc（continuation 也有自己的节点）。
	tc *xmlstore.NodeRecord
	// anchor 是该格位所属合并区的锚点 tc（自身为锚点时等于 tc）。
	anchor *xmlstore.NodeRecord
	// row/col 是格位坐标。
	row, col int
	// isContinuation 表示该格位被其它锚点覆盖（hMerge/vMerge）。
	isContinuation bool
}

// tableGrid 是表格的逻辑网格映射（行 × 列）。
type tableGrid struct {
	cols  int
	slots [][]*cellSlot
	trs   []*xmlstore.NodeRecord // 行节点（文档序）
	gcols []*xmlstore.NodeRecord // 列节点（文档序）
}

// rows 返回行数。
func (g *tableGrid) rows() int { return len(g.trs) }

// at 返回格位；越界或网格缺失返回 nil。
func (g *tableGrid) at(r, c int) *cellSlot {
	if r < 0 || r >= len(g.slots) || c < 0 || c >= g.cols {
		return nil
	}
	return g.slots[r][c]
}

// buildTableGrid 构建逻辑网格：按 DOM 顺序为每个 a:tc 分配格位，
// 由 gridSpan/rowSpan 占据后续格位，continuation（hMerge/vMerge）认领
// 锚点覆盖区内尚未分配节点的格位。
func buildTableGrid(doc *xmlstore.XMLDocument, tbl *xmlstore.NodeRecord) (*tableGrid, error) {
	grid := childOfKind(doc, tbl, nsDrawingML, "tblGrid", 0)
	g := &tableGrid{}
	if grid != nil {
		for _, cid := range grid.Children {
			c := doc.Node(cid)
			if c.Namespace == nsDrawingML && c.Local() == "gridCol" {
				g.gcols = append(g.gcols, c)
			}
		}
	}
	for _, cid := range tbl.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML && c.Local() == "tr" {
			g.trs = append(g.trs, c)
		}
	}
	g.cols = len(g.gcols)
	if g.cols == 0 {
		// 列数缺失：由首行 tc 数与跨度推断（不臆造 gridCol，仅用于读取）。
		g.cols = inferCols(doc, g.trs)
	}
	if g.cols == 0 || len(g.trs) == 0 {
		return g, nil
	}
	g.slots = make([][]*cellSlot, len(g.trs))
	for i := range g.slots {
		g.slots[i] = make([]*cellSlot, g.cols)
	}
	for r, tr := range g.trs {
		c := 0
		for _, cid := range tr.Children {
			tc := doc.Node(cid)
			if tc.Namespace != nsDrawingML || tc.Local() != "tc" {
				continue
			}
			cont := cellIsContinuation(tc)
			// 非 continuation 跳过已被（上方 rowSpan 或本行 gridSpan）占据的格。
			if !cont {
				for c < g.cols && g.slots[r][c] != nil {
					c++
				}
			}
			if c >= g.cols {
				break
			}
			if cont {
				// 归属：纵向取上方同列锚点，横向取本行左侧锚点。
				anchor := tc
				if r > 0 && g.slots[r-1][c] != nil {
					anchor = g.slots[r-1][c].anchor
				} else if c > 0 && g.slots[r][c-1] != nil {
					anchor = g.slots[r][c-1].anchor
				}
				g.slots[r][c] = &cellSlot{tc: tc, anchor: anchor, row: r, col: c, isContinuation: true}
				c++
				continue
			}
			gs, rs := cellSpans(tc)
			for i := 0; i < rs && r+i < len(g.slots); i++ {
				for j := 0; j < gs && c+j < g.cols; j++ {
					if g.slots[r+i][c+j] == nil {
						g.slots[r+i][c+j] = &cellSlot{tc: tc, anchor: tc, row: r + i, col: c + j}
					}
				}
			}
			// 只推进一格：覆盖区内的后续格位由 continuation 节点认领。
			c++
		}
	}
	return g, nil
}

// inferCols 在无 a:tblGrid 时由行内 tc 数与 gridSpan 推断列数。
func inferCols(doc *xmlstore.XMLDocument, trs []*xmlstore.NodeRecord) int {
	max := 0
	for _, tr := range trs {
		n := 0
		for _, cid := range tr.Children {
			tc := doc.Node(cid)
			if tc.Namespace != nsDrawingML || tc.Local() != "tc" {
				continue
			}
			if cellIsContinuation(tc) {
				n++
				continue
			}
			gs, _ := cellSpans(tc)
			n += gs
		}
		if n > max {
			max = n
		}
	}
	return max
}

// cellSpans 返回单元格的 gridSpan/rowSpan（缺省 1，非法值按 1）。
func cellSpans(tc *xmlstore.NodeRecord) (gridSpan, rowSpan int) {
	gs, rs := 1, 1
	if v, ok := tc.Attr("", "gridSpan"); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			gs = n
		}
	}
	if v, ok := tc.Attr("", "rowSpan"); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			rs = n
		}
	}
	return gs, rs
}

// cellIsContinuation 返回单元格是否为合并覆盖区的 continuation
// （hMerge 或 vMerge 为真）。
func cellIsContinuation(tc *xmlstore.NodeRecord) bool {
	if v, ok := tc.Attr("", "hMerge"); ok && (v == "1" || v == "true") {
		return true
	}
	if v, ok := tc.Attr("", "vMerge"); ok && (v == "1" || v == "true") {
		return true
	}
	return false
}

// cellHasText 返回单元格正文是否含非空白文本。
func cellHasText(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) bool {
	tx := childOfKind(doc, tc, nsDrawingML, "txBody", 0)
	if tx == nil {
		return false
	}
	for _, pid := range tx.Children {
		p := doc.Node(pid)
		if p.Namespace != nsDrawingML || p.Local() != "p" {
			continue
		}
		for _, rid := range p.Children {
			r := doc.Node(rid)
			if r.Namespace != nsDrawingML || (r.Local() != "r" && r.Local() != "fld") {
				continue
			}
			if r.Local() == "fld" {
				return true // 字段视为有内容（不臆测是否可见）
			}
			for _, tid := range r.Children {
				t := doc.Node(tid)
				if t.Namespace == nsDrawingML && t.Local() == "t" {
					raw := ""
					if !t.SelfClosing() {
						raw = string(doc.Original()[t.OpenEnd:t.CloseStart])
					}
					if strings.TrimSpace(xmlUnescape(raw)) != "" {
						return true
					}
				}
			}
		}
	}
	return false
}

// ---------- Cell ----------

// Cell 是表格逻辑格位上单元格（a:tc）的受控句柄。
//
// 句柄指向格位对应的 a:tc 节点：合并区锚点格位指向锚点单元格，
// continuation 格位指向带 hMerge/vMerge 标记的单元格。读取与写入均
// 基于当前 revision 索引重定位；节点删除返回 ErrStaleHandle。
type Cell struct {
	p    *Presentation
	part opc.PartName
	path []nodeStep
	// Row0/Col0 是创建句柄时的逻辑坐标（网格变化后不自动更新）。
	row int
	col int
	// shapeHint 是所属表格形状的 cNvPr@id（V2.6 §M8 textNode 句柄失效
	// 语义修复点）。Cell.TextFrame 返回的 TextFrame 继承此 hint。
	shapeHint ShapeID
}

// locate 定位单元格节点。
func (c *Cell) locate() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	if c.p == nil || c.p.closed {
		return nil, nil, Annotate(ErrClosed, "Cell")
	}
	doc, err := c.p.docOf(c.part)
	if err != nil {
		if errIsNotFound(err) {
			return nil, nil, Annotate(ErrStaleHandle, "Cell")
		}
		return nil, nil, err
	}
	n := resolvePath(doc, c.path)
	if n == nil {
		return nil, nil, Annotate(ErrStaleHandle, "Cell")
	}
	// shapeHint 校验：Cell 句柄身份 = 所属表格形状（p:graphicFrame）的
	// cNvPr@id。表格形状被删除后，原 Cell 句柄解析到的 a:tc 可能错配到
	// 其他表格的单元格——与 textNode.shapeHint 同语义（STALE-GUARD）。
	if c.shapeHint != 0 {
		if got := shapeIDFromAncestors(doc, n); got != c.shapeHint {
			return nil, nil, Annotate(ErrStaleHandle, "Cell")
		}
	}
	return doc, n, nil
}

// Row 与 Column 返回创建句柄时的逻辑坐标。
func (c *Cell) Row() int    { return c.row }
func (c *Cell) Column() int { return c.col }

// IsMerged 返回该格位是否属于某个合并区（锚点跨度大于 1 或为
// continuation 格位）。
func (c *Cell) IsMerged() (bool, error) {
	_, tc, err := c.locate()
	if err != nil {
		return false, Annotate(err, "Cell.IsMerged")
	}
	gs, rs := cellSpans(tc)
	return gs > 1 || rs > 1 || cellIsContinuation(tc), nil
}

// IsContinuation 返回该格位是否为合并区中被覆盖的 continuation
// （非锚点）。
func (c *Cell) IsContinuation() (bool, error) {
	_, tc, err := c.locate()
	if err != nil {
		return false, Annotate(err, "Cell.IsContinuation")
	}
	return cellIsContinuation(tc), nil
}

// Spans 返回锚点单元格的跨度（gridSpan/rowSpan，缺省 1）。
// continuation 格位返回其自身跨度（通常 1,1）。
func (c *Cell) Spans() (gridSpan, rowSpan int, err error) {
	_, tc, err2 := c.locate()
	if err2 != nil {
		return 0, 0, Annotate(err2, "Cell.Spans")
	}
	gs, rs := cellSpans(tc)
	return gs, rs, nil
}

// TextFrame 返回单元格正文（a:txBody）的 TextFrame；单元格无正文
// 返回 ErrNotFound。语义与形状正文一致（TEXT-01）。
func (c *Cell) TextFrame() (*TextFrame, error) {
	doc, tc, err := c.locate()
	if err != nil {
		return nil, Annotate(err, "Cell.TextFrame")
	}
	tx := childOfKind(doc, tc, nsDrawingML, "txBody", 0)
	if tx == nil {
		return nil, &OperationError{
			Op: "Cell.TextFrame", Part: string(c.part),
			Message: "cell has no a:txBody", Err: ErrNotFound,
		}
	}
	return &TextFrame{textNode: textNode{p: c.p, part: c.part, path: recordPath(doc, tx.ID), shapeHint: c.shapeHint}}, nil
}

// ---------- TableShape 行列与单元格 ----------

// RowCount 返回表格行数（a:tr 数）。
func (t *TableShape) RowCount() (int, error) {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return 0, Annotate(err, "TableShape.RowCount")
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return 0, err
	}
	return g.rows(), nil
}

// ColumnCount 返回表格列数（a:tblGrid/a:gridCol 数）。
func (t *TableShape) ColumnCount() (int, error) {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return 0, Annotate(err, "TableShape.ColumnCount")
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return 0, err
	}
	return g.cols, nil
}

// Cell 返回逻辑坐标 (row,col) 的单元格句柄；越界或网格缺失返回
// ErrOutOfRange。合并区内的非锚点格位返回该格位 continuation 单元格，
// 其 TextFrame 通常为空（合并后文本保留在锚点）。
func (t *TableShape) Cell(row, col int) (*Cell, error) {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return nil, Annotate(err, "TableShape.Cell")
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return nil, err
	}
	if row < 0 || col < 0 || row >= g.rows() || col >= g.cols {
		return nil, &OperationError{
			Op: "TableShape.Cell", Part: string(t.part),
			Message: fmt.Sprintf("cell (%d,%d) out of range [0,%d)x[0,%d)", row, col, g.rows(), g.cols),
			Err:     ErrOutOfRange,
		}
	}
	s := g.at(row, col)
	if s == nil {
		return nil, &OperationError{
			Op: "TableShape.Cell", Part: string(t.part),
			Message: fmt.Sprintf("cell (%d,%d) has no a:tc", row, col), Err: ErrMalformedPackage,
		}
	}
	return &Cell{p: t.p, part: t.part, path: recordPath(doc, s.tc.ID), row: row, col: col, shapeHint: t.idHint}, nil
}

// ---------- 行高与列宽 ----------

// RowHeight 返回行高（a:tr@h，EMU）；未设置返回 0。
func (t *TableShape) RowHeight(row int) (EMU, error) {
	_, tr, err := t.rowNode(row)
	if err != nil {
		return 0, err
	}
	v, ok := tr.Attr("", "h")
	if !ok {
		return 0, nil
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n < 0 {
		return 0, &OperationError{
			Op: "TableShape.RowHeight", Part: string(t.part),
			Message: fmt.Sprintf("invalid row height %q", v), Err: ErrMalformedPackage,
		}
	}
	return EMU(n), nil
}

// SetRowHeight 设置行高（a:tr@h，EMU）；负值返回 ErrInvalidArgument。
func (t *TableShape) SetRowHeight(row int, h EMU) error {
	if h < 0 {
		return &OperationError{
			Op: "TableShape.SetRowHeight", Message: fmt.Sprintf("negative height %d", h),
			Err: ErrInvalidArgument,
		}
	}
	doc, tr, err := t.rowNode(row)
	if err != nil {
		return err
	}
	_ = doc
	p, err := setOrAddAttr(doc, tr, "h", strconv.FormatInt(int64(h), 10))
	if err != nil {
		return Annotate(err, "TableShape.SetRowHeight")
	}
	return t.apply([]xmlstore.SpanPatch{p}, "TableShape.SetRowHeight")
}

// ColumnWidth 返回列宽（a:gridCol@w，EMU）；未设置返回 0。
func (t *TableShape) ColumnWidth(col int) (EMU, error) {
	_, gc, err := t.colNode(col)
	if err != nil {
		return 0, err
	}
	v, ok := gc.Attr("", "w")
	if !ok {
		return 0, nil
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n < 0 {
		return 0, &OperationError{
			Op: "TableShape.ColumnWidth", Part: string(t.part),
			Message: fmt.Sprintf("invalid column width %q", v), Err: ErrMalformedPackage,
		}
	}
	return EMU(n), nil
}

// SetColumnWidth 设置列宽（a:gridCol@w，EMU）；负值返回
// ErrInvalidArgument。
func (t *TableShape) SetColumnWidth(col int, w EMU) error {
	if w < 0 {
		return &OperationError{
			Op: "TableShape.SetColumnWidth", Message: fmt.Sprintf("negative width %d", w),
			Err: ErrInvalidArgument,
		}
	}
	doc, gc, err := t.colNode(col)
	if err != nil {
		return err
	}
	p, err := setOrAddAttr(doc, gc, "w", strconv.FormatInt(int64(w), 10))
	if err != nil {
		return Annotate(err, "TableShape.SetColumnWidth")
	}
	return t.apply([]xmlstore.SpanPatch{p}, "TableShape.SetColumnWidth")
}

func (t *TableShape) rowNode(row int) (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return nil, nil, Annotate(err, "TableShape")
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return nil, nil, err
	}
	if row < 0 || row >= g.rows() {
		return nil, nil, &OperationError{
			Op: "TableShape", Part: string(t.part),
			Message: fmt.Sprintf("row %d out of range [0,%d)", row, g.rows()), Err: ErrOutOfRange,
		}
	}
	return doc, g.trs[row], nil
}

func (t *TableShape) colNode(col int) (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return nil, nil, Annotate(err, "TableShape")
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return nil, nil, err
	}
	if col < 0 || col >= g.cols || col >= len(g.gcols) {
		return nil, nil, &OperationError{
			Op: "TableShape", Part: string(t.part),
			Message: fmt.Sprintf("column %d out of range [0,%d)", col, g.cols), Err: ErrOutOfRange,
		}
	}
	return doc, g.gcols[col], nil
}

// apply 以一次事务提交补丁（stagePatch + commit）。
func (t *TableShape) apply(patches []xmlstore.SpanPatch, op string) error {
	if len(patches) == 0 {
		return nil
	}
	doc, _, err := t.locate()
	if err != nil {
		return Annotate(err, op)
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), op)
	}
	if err := t.p.stagePatch(t.part, out); err != nil {
		return Annotate(err, op)
	}
	t.p.commit()
	return nil
}

// ---------- 合并 ----------

// CellRange 是表格上的矩形区域（左上角 Row/Col + 尺寸 Rows/Cols）。
type CellRange struct {
	Row  int
	Col  int
	Rows int
	Cols int
}

// MultiCellTextPolicy 决定合并区域内存在多个非空单元格时的行为
// （AT-09：默认拒绝）。
type MultiCellTextPolicy int

const (
	// MergeRejectMultipleText 默认：区域内多于一个非空单元格时拒绝
	// 合并，原单元格与样式不变（AT-09）。
	MergeRejectMultipleText MultiCellTextPolicy = iota
	// MergeKeepAnchorText 明确选择：保留锚点（左上）单元格文本，
	// 清空区域内其余单元格正文。
	MergeKeepAnchorText
)

// mergeOptions / MergeOption / WithMergeTextPolicy 已迁出至 options.go。

// Merge 合并矩形区域内的单元格（§9.1）：
//
//   - 校验区域在界内且至少覆盖两个格位；
//   - 区域内既有合并区的锚点必须完整落在区域内（不重叠、不跨边界）；
//   - 多于一个非空单元格时默认拒绝（AT-09），调用方可用
//     WithMergeTextPolicy(MergeKeepAnchorText) 明确保留锚点文本；
//   - 锚点单元格写 gridSpan/rowSpan，被覆盖格位写 hMerge/vMerge
//     continuation 标记，不清空文本以外的结构（tcPr 保留）。
//
// 整个操作为一次事务：任一校验失败不产生部分修改。
func (t *TableShape) Merge(r CellRange, opts ...MergeOption) error {
	o := &mergeOptions{}
	for _, f := range opts {
		f(o)
	}
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return Annotate(err, "TableShape.Merge")
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return err
	}
	if r.Rows <= 0 || r.Cols <= 0 {
		return &OperationError{
			Op: "TableShape.Merge", Part: string(t.part),
			Message: fmt.Sprintf("invalid range size %dx%d", r.Rows, r.Cols), Err: ErrInvalidArgument,
		}
	}
	if r.Row < 0 || r.Col < 0 || r.Row+r.Rows > g.rows() || r.Col+r.Cols > g.cols {
		return &OperationError{
			Op: "TableShape.Merge", Part: string(t.part),
			Message: fmt.Sprintf("range (%d,%d,%dx%d) outside grid %dx%d", r.Row, r.Col, r.Rows, r.Cols, g.rows(), g.cols),
			Err:     ErrOutOfRange,
		}
	}
	if r.Rows*r.Cols < 2 {
		return &OperationError{
			Op: "TableShape.Merge", Part: string(t.part),
			Message: "range covers a single cell", Err: ErrInvalidArgument,
		}
	}

	// 收集区域内格位；要求每个格位所属锚点完整落在区域内。
	seen := map[*xmlstore.NodeRecord]bool{}
	var nonEmpty []*cellSlot
	for i := 0; i < r.Rows; i++ {
		for j := 0; j < r.Cols; j++ {
			s := g.at(r.Row+i, r.Col+j)
			if s == nil {
				return &OperationError{
					Op: "TableShape.Merge", Part: string(t.part),
					Message: fmt.Sprintf("cell (%d,%d) has no a:tc", r.Row+i, r.Col+j),
					Err:     ErrMalformedPackage,
				}
			}
			if !s.isContinuation {
				gs, rs := cellSpans(s.tc)
				// 锚点跨度必须完整落在区域内。
				if s.row+rs > r.Row+r.Rows || s.col+gs > r.Col+r.Cols {
					return &OperationError{
						Op: "TableShape.Merge", Part: string(t.part),
						Message: fmt.Sprintf("existing merge at (%d,%d) spans outside range", s.row, s.col),
						Err:     ErrUnsupportedEdit,
					}
				}
			}
			if cellHasText(doc, s.tc) && !seen[s.tc] {
				seen[s.tc] = true
				nonEmpty = append(nonEmpty, s)
			}
		}
	}
	// AT-09：多非空单元格默认拒绝。
	if len(nonEmpty) > 1 && o.policy == MergeRejectMultipleText {
		return &OperationError{
			Op: "TableShape.Merge", Part: string(t.part),
			Message: fmt.Sprintf("range contains %d non-empty cells; use WithMergeTextPolicy(MergeKeepAnchorText) to keep the anchor text",
				len(nonEmpty)),
			Err: ErrUnsupportedEdit,
		}
	}

	anchorSlot := g.at(r.Row, r.Col)
	if anchorSlot == nil || anchorSlot.isContinuation {
		return &OperationError{
			Op: "TableShape.Merge", Part: string(t.part),
			Message: fmt.Sprintf("anchor cell (%d,%d) is not a merge anchor", r.Row, r.Col),
			Err:     ErrUnsupportedEdit,
		}
	}
	anchor := anchorSlot.tc

	var patches []xmlstore.SpanPatch
	// 锚点跨度。
	if r.Cols > 1 {
		p, err := setOrAddAttr(doc, anchor, "gridSpan", strconv.Itoa(r.Cols))
		if err != nil {
			return Annotate(err, "TableShape.Merge")
		}
		patches = append(patches, p)
	} else {
		patches = append(patches, dropAttrPatch(doc, anchor, "gridSpan")...)
	}
	if r.Rows > 1 {
		p, err := setOrAddAttr(doc, anchor, "rowSpan", strconv.Itoa(r.Rows))
		if err != nil {
			return Annotate(err, "TableShape.Merge")
		}
		patches = append(patches, p)
	} else {
		patches = append(patches, dropAttrPatch(doc, anchor, "rowSpan")...)
	}
	// 锚点自身不得残留 continuation 标记。
	patches = append(patches, dropAttrPatch(doc, anchor, "hMerge")...)
	patches = append(patches, dropAttrPatch(doc, anchor, "vMerge")...)

	// 被覆盖格位：写 continuation 标记；按需清空文本。
	cleared := map[*xmlstore.NodeRecord]bool{}
	for i := 0; i < r.Rows; i++ {
		for j := 0; j < r.Cols; j++ {
			if i == 0 && j == 0 {
				continue
			}
			s := g.at(r.Row+i, r.Col+j)
			if s == nil || s.tc == anchor {
				continue
			}
			if j > 0 {
				p, err := setOrAddAttr(doc, s.tc, "hMerge", "1")
				if err != nil {
					return Annotate(err, "TableShape.Merge")
				}
				patches = append(patches, p)
			}
			if i > 0 {
				p, err := setOrAddAttr(doc, s.tc, "vMerge", "1")
				if err != nil {
					return Annotate(err, "TableShape.Merge")
				}
				patches = append(patches, p)
			}
			if o.policy == MergeKeepAnchorText && !cleared[s.tc] && cellHasText(doc, s.tc) {
				cleared[s.tc] = true
				patches = append(patches, clearCellTextPatches(doc, s.tc)...)
			}
		}
	}
	return t.apply(patches, "TableShape.Merge")
}

// Unmerge 取消矩形区域内的合并：清除区域内单元格的 gridSpan/rowSpan
// 与 hMerge/vMerge continuation 标记（§9.1）。不承诺恢复先前被明确
// 丢弃的内容；文本内容保持不变。
func (t *TableShape) Unmerge(r CellRange) error {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return Annotate(err, "TableShape.Unmerge")
	}
	g, err := buildTableGrid(doc, tbl)
	if err != nil {
		return err
	}
	if r.Rows <= 0 || r.Cols <= 0 {
		return &OperationError{
			Op: "TableShape.Unmerge", Part: string(t.part),
			Message: fmt.Sprintf("invalid range size %dx%d", r.Rows, r.Cols), Err: ErrInvalidArgument,
		}
	}
	if r.Row < 0 || r.Col < 0 || r.Row+r.Rows > g.rows() || r.Col+r.Cols > g.cols {
		return &OperationError{
			Op: "TableShape.Unmerge", Part: string(t.part),
			Message: fmt.Sprintf("range (%d,%d,%dx%d) outside grid %dx%d", r.Row, r.Col, r.Rows, r.Cols, g.rows(), g.cols),
			Err:     ErrOutOfRange,
		}
	}
	var patches []xmlstore.SpanPatch
	done := map[*xmlstore.NodeRecord]bool{}
	for i := 0; i < r.Rows; i++ {
		for j := 0; j < r.Cols; j++ {
			s := g.at(r.Row+i, r.Col+j)
			if s == nil || done[s.tc] {
				continue
			}
			done[s.tc] = true
			for _, name := range []string{"gridSpan", "rowSpan", "hMerge", "vMerge"} {
				patches = append(patches, dropAttrPatch(doc, s.tc, name)...)
			}
		}
	}
	return t.apply(patches, "TableShape.Unmerge")
}

// setOrAddAttr 设置属性值：属性已存在走精确替换，否则在开标签内追加
// （xmlstore.SetAttrValuePatch 仅支持已存在属性）。
func setOrAddAttr(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, name, value string) (xmlstore.SpanPatch, error) {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			return xmlstore.SetAttrValuePatch(doc, n, "", name, value)
		}
	}
	esc, err := xmlstore.EscapeAttrValue(value, '"')
	if err != nil {
		return xmlstore.SpanPatch{}, Annotate(mapXMLError(err), "setOrAddAttr")
	}
	return addPlainAttrPatch(doc, n, name, esc)
}

// dropAttrPatch 返回移除指定属性的补丁（不存在则无补丁）。
func dropAttrPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, name string) []xmlstore.SpanPatch {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			p := removeAttrPatch(doc, n, i)
			return []xmlstore.SpanPatch{p}
		}
	}
	return nil
}

// clearCellTextPatches 返回清空单元格正文（删除 a:txBody 内全部 a:p）
// 的补丁；保留 a:bodyPr/a:lstStyle 与其它结构。
func clearCellTextPatches(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) []xmlstore.SpanPatch {
	tx := childOfKind(doc, tc, nsDrawingML, "txBody", 0)
	if tx == nil {
		return nil
	}
	var out []xmlstore.SpanPatch
	for _, cid := range tx.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML && c.Local() == "p" {
			out = append(out, xmlstore.SpanPatch{Start: c.Source.Start, End: c.Source.End})
		}
	}
	return out
}

// errIsNotFound 报告错误是否为 ErrNotFound（含包装）。
func errIsNotFound(err error) bool {
	var oe *OperationError
	return errors.As(err, &oe) && oe.Err == ErrNotFound
}
