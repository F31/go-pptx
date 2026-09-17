// STALE-GUARD-textNode 补充：Cell 句柄失效语义（V2.6 §M8 遗留项收尾）。
//
// Cell 是表格单元格句柄，其 path 指向 a:tc，通过独立 row/col 逻辑坐标
// 暴露行列。此前 Cell.locate 只按 path 解析——表格形状增删后，原 Cell
// 句柄可能错配到其他表格的同形 a:tc。本文件验证 shapeHint 校验：
//
//   - RemoveShape（删除所属表格）→ Cell 句柄 ErrStaleHandle
//   - AddShape（增兄弟 shape）→ Cell 句柄仍有效（graphicFrame 的 cNvPr@id 不变）
//   - Close → ErrClosed / ErrStaleHandle
package pptx

import "testing"

// TestCellStale_OnTableRemoved 验证：删除所属表格形状后，原 Cell 句柄
// 的读操作返回 ErrStaleHandle（而非错配到其他表格的单元格）。
func TestCellStale_OnTableRemoved(t *testing.T) {
	rows := tableRow("370840", tableCell("", "A1")+tableCell("", "B1")) +
		tableRow("370840", tableCell("", "A2")+tableCell("", "B2"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()

	tb := firstTable(t, p)
	c, err := tb.Cell(0, 0)
	if err != nil {
		t.Fatalf("Cell(0,0): %v", err)
	}
	// 句柄初始有效。
	if _, err := c.IsMerged(); err != nil {
		t.Fatalf("IsMerged before RemoveShape: %v", err)
	}

	// 删除所属表格形状（graphicFrame cNvPr@id=4）。
	s, _ := p.Slide(0)
	if err := s.RemoveShape(tb.idHint); err != nil {
		t.Fatalf("RemoveShape: %v", err)
	}

	if _, err := c.IsMerged(); !isStaleHandle(err) {
		t.Errorf("IsMerged after RemoveShape: err=%v, want ErrStaleHandle", err)
	}
}

// TestCellStale_OnSiblingShapeAdded 验证：新增兄弟 shape 不影响 Cell
// 句柄有效性——Cell 的 shapeHint 指向表格 graphicFrame 的 cNvPr@id，
// 增 shape 不改变它。
func TestCellStale_OnSiblingShapeAdded(t *testing.T) {
	rows := tableRow("370840", tableCell("", "A1")+tableCell("", "B1"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800", "304800"}, rows), "")
	defer p.Close()

	tb := firstTable(t, p)
	c, err := tb.Cell(0, 0)
	if err != nil {
		t.Fatalf("Cell(0,0): %v", err)
	}

	s, _ := p.Slide(0)
	if _, err := s.AddTextBox(TextBoxSpec{X: 100, Y: 100, Width: 100, Height: 100}); err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}

	if _, err := c.IsMerged(); err != nil {
		t.Errorf("IsMerged after AddTextBox: err=%v, want nil (handle should still be valid)", err)
	}
}

// TestCellStale_ClosedDocument 验证：文档关闭后 Cell 句柄读操作返回
// ErrClosed / ErrStaleHandle。
func TestCellStale_ClosedDocument(t *testing.T) {
	rows := tableRow("370840", tableCell("", "A1"))
	p := tableDeck(t, tableFrame("4", "", []string{"304800"}, rows), "")
	tb := firstTable(t, p)
	c, err := tb.Cell(0, 0)
	if err != nil {
		t.Fatalf("Cell(0,0): %v", err)
	}
	p.Close()

	if _, err := c.IsMerged(); !isClosedOrStale(err) {
		t.Errorf("IsMerged after Close: err=%v, want ErrClosed or ErrStaleHandle", err)
	}
}
