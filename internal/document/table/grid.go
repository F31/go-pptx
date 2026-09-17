// Package table 是 v2.0 域层「表格」垂直切片的起步：a:tbl 逻辑网格
// （gridSpan/rowSpan/hMerge/vMerge → 行×列映射）与单元格纯辅助。
//
// 句柄（TableShape/Cell）仍留根包门面；本包只承载无状态纯函数。禁止
// 本包反向 import 根包。
package table

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/textutil"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// GraphicURI 是表格图形数据的 URI（a:graphicData@uri）。
const GraphicURI = "http://schemas.openxmlformats.org/drawingml/2006/table"

// TableOfGraphic 返回图形框内的 a:tbl（非表格图形框返回 nil）。
func TableOfGraphic(doc *xmlstore.XMLDocument, frame *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	g := xmlstore.ChildOfKind(doc, frame, ooxmlns.DrawingML, "graphic", 0)
	if g == nil {
		return nil
	}
	gd := xmlstore.ChildOfKind(doc, g, ooxmlns.DrawingML, "graphicData", 0)
	if gd == nil {
		return nil
	}
	if uri, ok := gd.Attr("", "uri"); !ok || uri != GraphicURI {
		return nil
	}
	return xmlstore.ChildOfKind(doc, gd, ooxmlns.DrawingML, "tbl", 0)
}

// Slot 是逻辑网格中的一个格位（对应一个 a:tc）。
type Slot struct {
	// TC 是该格位 DOM 中的 a:tc（continuation 也有自己的节点）。
	TC *xmlstore.NodeRecord
	// Anchor 是该格位所属合并区的锚点 tc（自身为锚点时等于 TC）。
	Anchor *xmlstore.NodeRecord
	// Row/Col 是格位坐标。
	Row, Col int
	// IsContinuation 表示该格位被其它锚点覆盖（hMerge/vMerge）。
	IsContinuation bool
}

// Grid 是表格的逻辑网格映射（行 × 列）。
type Grid struct {
	Cols  int
	Slots [][]*Slot
	TRs   []*xmlstore.NodeRecord // 行节点（文档序）
	GCols []*xmlstore.NodeRecord // 列节点（文档序）
}

// Rows 返回行数。
func (g *Grid) Rows() int { return len(g.TRs) }

// At 返回格位；越界或网格缺失返回 nil。
func (g *Grid) At(r, c int) *Slot {
	if r < 0 || r >= len(g.Slots) || c < 0 || c >= g.Cols {
		return nil
	}
	return g.Slots[r][c]
}

// BuildGrid 构建逻辑网格：按 DOM 顺序为每个 a:tc 分配格位，
// 由 gridSpan/rowSpan 占据后续格位，continuation（hMerge/vMerge）认领
// 锚点覆盖区内尚未分配节点的格位。
func BuildGrid(doc *xmlstore.XMLDocument, tbl *xmlstore.NodeRecord) (*Grid, error) {
	grid := xmlstore.ChildOfKind(doc, tbl, ooxmlns.DrawingML, "tblGrid", 0)
	g := &Grid{}
	if grid != nil {
		for _, cid := range grid.Children {
			c := doc.Node(cid)
			if c.Namespace == ooxmlns.DrawingML && c.Local() == "gridCol" {
				g.GCols = append(g.GCols, c)
			}
		}
	}
	for _, cid := range tbl.Children {
		c := doc.Node(cid)
		if c.Namespace == ooxmlns.DrawingML && c.Local() == "tr" {
			g.TRs = append(g.TRs, c)
		}
	}
	g.Cols = len(g.GCols)
	if g.Cols == 0 {
		// 列数缺失：由首行 tc 数与跨度推断（不臆造 gridCol，仅用于读取）。
		g.Cols = InferCols(doc, g.TRs)
	}
	if g.Cols == 0 || len(g.TRs) == 0 {
		return g, nil
	}
	g.Slots = make([][]*Slot, len(g.TRs))
	for i := range g.Slots {
		g.Slots[i] = make([]*Slot, g.Cols)
	}
	for r, tr := range g.TRs {
		c := 0
		for _, cid := range tr.Children {
			tc := doc.Node(cid)
			if tc.Namespace != ooxmlns.DrawingML || tc.Local() != "tc" {
				continue
			}
			cont := CellIsContinuation(tc)
			// 非 continuation 跳过已被（上方 rowSpan 或本行 gridSpan）占据的格。
			if !cont {
				for c < g.Cols && g.Slots[r][c] != nil {
					c++
				}
			}
			if c >= g.Cols {
				break
			}
			if cont {
				// 归属：纵向取上方同列锚点，横向取本行左侧锚点。
				anchor := tc
				if r > 0 && g.Slots[r-1][c] != nil {
					anchor = g.Slots[r-1][c].Anchor
				} else if c > 0 && g.Slots[r][c-1] != nil {
					anchor = g.Slots[r][c-1].Anchor
				}
				g.Slots[r][c] = &Slot{TC: tc, Anchor: anchor, Row: r, Col: c, IsContinuation: true}
				c++
				continue
			}
			gs, rs := CellSpans(tc)
			for i := 0; i < rs && r+i < len(g.Slots); i++ {
				for j := 0; j < gs && c+j < g.Cols; j++ {
					if g.Slots[r+i][c+j] == nil {
						g.Slots[r+i][c+j] = &Slot{TC: tc, Anchor: tc, Row: r + i, Col: c + j}
					}
				}
			}
			// 只推进一格：覆盖区内的后续格位由 continuation 节点认领。
			c++
		}
	}
	return g, nil
}

// InferCols 在无 a:tblGrid 时由行内 tc 数与 gridSpan 推断列数。
func InferCols(doc *xmlstore.XMLDocument, trs []*xmlstore.NodeRecord) int {
	max := 0
	for _, tr := range trs {
		n := 0
		for _, cid := range tr.Children {
			tc := doc.Node(cid)
			if tc.Namespace != ooxmlns.DrawingML || tc.Local() != "tc" {
				continue
			}
			if CellIsContinuation(tc) {
				n++
				continue
			}
			gs, _ := CellSpans(tc)
			n += gs
		}
		if n > max {
			max = n
		}
	}
	return max
}

// CellSpans 返回单元格的 gridSpan/rowSpan（缺省 1，非法值按 1）。
func CellSpans(tc *xmlstore.NodeRecord) (gridSpan, rowSpan int) {
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

// CellIsContinuation 返回单元格是否为合并覆盖区的 continuation
// （hMerge 或 vMerge 为真）。
func CellIsContinuation(tc *xmlstore.NodeRecord) bool {
	if v, ok := tc.Attr("", "hMerge"); ok && (v == "1" || v == "true") {
		return true
	}
	if v, ok := tc.Attr("", "vMerge"); ok && (v == "1" || v == "true") {
		return true
	}
	return false
}

// CellHasText 返回单元格正文是否含非空白文本。
func CellHasText(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) bool {
	tx := xmlstore.ChildOfKind(doc, tc, ooxmlns.DrawingML, "txBody", 0)
	if tx == nil {
		return false
	}
	for _, pid := range tx.Children {
		p := doc.Node(pid)
		if p.Namespace != ooxmlns.DrawingML || p.Local() != "p" {
			continue
		}
		for _, rid := range p.Children {
			r := doc.Node(rid)
			if r.Namespace != ooxmlns.DrawingML || (r.Local() != "r" && r.Local() != "fld") {
				continue
			}
			if r.Local() == "fld" {
				return true // 字段视为有内容（不臆测是否可见）
			}
			for _, tid := range r.Children {
				t := doc.Node(tid)
				if t.Namespace == ooxmlns.DrawingML && t.Local() == "t" {
					raw := ""
					if !t.SelfClosing() {
						raw = string(doc.Original()[t.OpenEnd:t.CloseStart])
					}
					if strings.TrimSpace(textutil.XmlUnescape(raw)) != "" {
						return true
					}
				}
			}
		}
	}
	return false
}

// ClearCellTextPatches 返回清空单元格正文（删除 a:txBody 内全部 a:p）
// 的补丁；保留 a:bodyPr/a:lstStyle 与其它结构。
func ClearCellTextPatches(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) []xmlstore.SpanPatch {
	tx := xmlstore.ChildOfKind(doc, tc, ooxmlns.DrawingML, "txBody", 0)
	if tx == nil {
		return nil
	}
	var out []xmlstore.SpanPatch
	for _, cid := range tx.Children {
		c := doc.Node(cid)
		if c.Namespace == ooxmlns.DrawingML && c.Local() == "p" {
			out = append(out, xmlstore.SpanPatch{Start: c.Source.Start, End: c.Source.End})
		}
	}
	return out
}
