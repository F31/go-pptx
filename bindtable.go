package pptx

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是模板绑定的**表格处理**：行循环模板行识别与逐格绑定（bindTable）、
// 行模板渲染（buildRowPatch/renderRow）。正文绑定见 bindbody.go。
// bindTable 处理一个表格：先识别行循环模板行，再逐个单元格绑定。
func (s *bindScanner) bindTable(t *TableShape) error {
	const op = "Presentation.Bind"
	rows, err := t.RowCount()
	if err != nil {
		return err
	}
	cols, err := t.ColumnCount()
	if err != nil {
		return err
	}
	type cellParas struct {
		paras []*Paragraph
	}
	byRow := make([][]cellParas, rows)
	for r := 0; r < rows; r++ {
		byRow[r] = make([]cellParas, cols)
		for c := 0; c < cols; c++ {
			cell, err := t.Cell(r, c)
			if err != nil {
				continue
			}
			tf, err := cell.TextFrame()
			if err != nil || tf == nil {
				continue
			}
			paras, err := tf.Paragraphs()
			if err != nil {
				continue
			}
			byRow[r][c] = cellParas{paras: paras}
		}
	}

	for r := 0; r < rows; r++ {
		path, hasEach := "", false
		hasEnd := false
		for c := 0; c < cols; c++ {
			for _, para := range byRow[r][c].paras {
				txt, err := para.Text()
				if err != nil {
					return err
				}
				kind, p, ok := parseDirective(strings.TrimSpace(txt))
				if !ok {
					continue
				}
				switch kind {
				case "each":
					if hasEach {
						return &OperationError{Op: op, Message: fmt.Sprintf(
							"multiple %s#each%s markers in one table row (%d)", tplMarkOpen, tplMarkClose, r),
							Err: ErrUnsupportedEdit}
					}
					hasEach, path = true, p
				case "endeach":
					hasEnd = true
				case "if", "endif":
					return &OperationError{Op: op, Message: fmt.Sprintf(
						"conditional blocks are not supported inside a row loop (row %d)", r),
						Err: ErrUnsupportedEdit}
				}
			}
		}
		if hasEach != hasEnd {
			return &OperationError{Op: op, Message: fmt.Sprintf(
				"%s#each%s/%s markers must be paired within table row %d", tplMarkOpen, tplMarkClose, tplMarkOpen, r),
				Err: ErrInvalidArgument}
		}
		if !hasEach {
			for c := 0; c < cols; c++ {
				if err := s.bindBody(fmt.Sprintf("table row %d col %d", r, c), byRow[r][c].paras); err != nil {
					return err
				}
			}
			continue
		}
		// 该行为模板行：整行被生成行替换，其单元格不再单独绑定。
		items, err := s.resolveItems(path)
		if err != nil {
			return err
		}
		if len(items) == 0 && rows == 1 {
			return &OperationError{Op: op, Message: fmt.Sprintf(
				"row loop %q produced no rows and the table would be empty", path), Err: ErrUnsupportedEdit}
		}
		if err := s.buildRowPatch(t, r, path, items); err != nil {
			return err
		}
	}
	return nil
}

// buildRowPatch 用数据条目渲染模板行，并用生成行整体替换模板行区间。
func (s *bindScanner) buildRowPatch(t *TableShape, row int, path string, items []any) error {
	doc, tr, err := t.rowNode(row)
	if err != nil {
		return err
	}
	rowBytes := doc.Slice(tr.Source)
	var out [][]byte
	for _, item := range items {
		rb, err := s.renderRow(rowBytes, item)
		if err != nil {
			return err
		}
		out = append(out, rb)
		s.rep.RowsGenerated++
	}
	if len(items) == 0 {
		s.rep.RowsRemoved++
	}
	s.addRow(t.part, tr.Source.Start, tr.Source.End, rowBytes, bytes.Join(out, nil))
	return nil
}

// renderRow 在临时文档中渲染一行：删除 each 标记段落、按条目作用域
// 替换占位符，返回渲染后的 a:tr 字节。
//
// 行内多个占位符与标记删除按"起始偏移降序"回放（同 applyPart），
// 同一段落的多个占位符逐个重新索引后应用，避免 rebuild 补丁重叠。
func (s *bindScanner) renderRow(rowBytes []byte, item any) ([]byte, error) {
	const op = "Presentation.Bind"
	wrapped := make([]byte, 0, len(tplWrapOpen)+len(rowBytes)+len(tplWrapClose))
	wrapped = append(wrapped, tplWrapOpen...)
	wrapped = append(wrapped, rowBytes...)
	wrapped = append(wrapped, tplWrapClose...)
	tmp, err := xmlstore.Index(wrapped)
	if err != nil {
		return nil, Annotate(mapXMLError(err), op)
	}
	root := tmp.Root()
	if root == nil {
		return nil, &OperationError{Op: op, Message: "row loop: cannot index template row", Err: ErrMalformedPackage}
	}
	tr := childOfKind(tmp, root, nsDrawingML, "tr", 0)
	if tr == nil || tr.Namespace != nsDrawingML {
		return nil, &OperationError{Op: op, Message: "row loop: template row is not a DrawingML a:tr", Err: ErrMalformedPackage}
	}

	// 1) 收集段落级操作（标记删除 / 占位符替换）。
	type paraUnit struct {
		start int
		del   bool
		path  []nodeStep
		pairs []bindPair
	}
	var units []paraUnit
	for _, tc := range childElems(tmp, tr, "tc") {
		for _, tx := range childElems(tmp, tc, "txBody") {
			for _, p := range childElems(tmp, tx, "p") {
				text := paragraphText(tmp, p)
				if kind, _, ok := parseDirective(strings.TrimSpace(text)); ok {
					switch kind {
					case "each", "endeach":
						units = append(units, paraUnit{start: p.Source.Start, del: true})
					default:
						return nil, &OperationError{Op: op, Message: fmt.Sprintf(
							"unsupported marker %q inside row loop", strings.TrimSpace(text)), Err: ErrUnsupportedEdit}
					}
					continue
				}
				toks := scanInline(text)
				if len(toks) == 0 {
					continue
				}
				s.rep.Placeholders += len(toks)
				u := paraUnit{start: p.Source.Start, path: recordPath(tmp, p.ID)}
				seen := map[string]bool{}
				for _, tok := range toks {
					if seen[tok.literal] {
						continue
					}
					seen[tok.literal] = true
					v, found, err := s.resolve(item, tok.path)
					if err != nil {
						return nil, err
					}
					if !found {
						if s.o.strict {
							return nil, &OperationError{Op: op, Message: fmt.Sprintf(
								"placeholder %q not found in row item", tok.path), Err: ErrInvalidArgument}
						}
						s.diag("bind.key_missing", "", fmt.Sprintf(
							"placeholder %q missing in row item; left as-is", tok.path))
						continue
					}
					rep, ok := formatBindValue(v)
					if !ok {
						return nil, &OperationError{Op: op, Message: fmt.Sprintf(
							"placeholder %q has unsupported value type %T", tok.path, v), Err: ErrInvalidArgument}
					}
					u.pairs = append(u.pairs, bindPair{needle: tok.literal, value: rep})
				}
				if len(u.pairs) > 0 {
					units = append(units, u)
				}
			}
		}
	}

	// 2) 按起始偏移降序回放。
	sort.SliceStable(units, func(a, b int) bool { return units[a].start > units[b].start })
	work := tmp.Original()
	for _, u := range units {
		if u.del {
			work, err = xmlstore.ApplyPatches(work, []xmlstore.SpanPatch{deletePatch(tmp, tmpNodeAt(tmp, u.start), "each-marker")})
			if err != nil {
				return nil, Annotate(mapXMLError(err), op)
			}
			continue
		}
		for _, pr := range u.pairs {
			cur, err := xmlstore.Index(work)
			if err != nil {
				return nil, Annotate(mapXMLError(err), op)
			}
			node := resolvePath(cur, u.path)
			if node == nil {
				return nil, &OperationError{Op: op, Message: "row loop: paragraph no longer resolvable", Err: ErrStaleHandle}
			}
			res, patches, err := replacePatches(cur, node, pr.needle, pr.value, replaceOptions{mode: s.o.mode})
			if err != nil {
				return nil, err
			}
			s.rep.Substituted += res.Replaced
			if len(patches) == 0 {
				continue
			}
			work, err = xmlstore.ApplyPatches(work, patches)
			if err != nil {
				return nil, Annotate(mapXMLError(err), op)
			}
		}
	}
	if len(work) < len(tplWrapOpen)+len(tplWrapClose) {
		return nil, &OperationError{Op: op, Message: "row loop: rendered row is empty", Err: ErrMalformedPackage}
	}
	return work[len(tplWrapOpen) : len(work)-len(tplWrapClose)], nil
}

// tmpNodeAt 在（未修改的）临时文档中按起始偏移定位节点。
func tmpNodeAt(doc *xmlstore.XMLDocument, start int) *xmlstore.NodeRecord {
	for i := 0; i < doc.Len(); i++ {
		if n := doc.Node(xmlstore.NodeID(i)); n != nil && n.Source.Start == start {
			return n
		}
	}
	return nil
}

// ---------- 图表绑定 ----------
