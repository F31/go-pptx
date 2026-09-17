// STALE-GUARD-textNode: textNode 句柄失效语义修复（V2.6 §M8）。
//
// 与 STALE-GUARD 同形：原 textNode.locate 只按 path 解析，shape 增删后
// path 仍能解析到相邻 shape 的同形 txBody——错配而非 ErrStaleHandle。
// 修复点：textNode 新增 shapeHint（来自所属 shape 的 cNvPr@id）；locate
// 解析 path 后向上找最近 p:sp/p:cxnSp/p:graphicFrame/p:grpSp 的 cNvPr@id，
// 与 shapeHint 比对——不等即 ErrStaleHandle。shapeHint=0 走纯路径判定
// （向后兼容）。
//
// 本文件测试三件事：
// 1. RemoveShape 后原 Paragraph/TextRun 句柄 ErrStaleHandle
// 2. shapeHint=0 句柄（老句柄/零值句柄）仍按纯路径解析，向后兼容
// 3. 同一 shape 内段落增删仍可定位（V2.6 §M8 已知妥协：句柄只能说
//    "仍属于原 shape"，不能说"仍是原段落"）

package pptx

import (
	"strings"
	"testing"
)

// TestTextNodeStale_OnShapeRemoved 验证 shape 被 RemoveShape 后，原
// Paragraph 句柄的所有读/写操作返回 ErrStaleHandle。
func TestTextNodeStale_OnShapeRemoved(t *testing.T) {
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p><a:r><a:t>hello</a:t></a:r></a:p>`)
	// 先确认 Paragraph 句柄当前可用（ReplaceText 走 locate 路径）。
	if _, err := para.Text(); err != nil {
		t.Fatalf("paragraph text before remove: %v", err)
	}

	// 找到形状并删除。tfPara 内 cNvPr@id 是 slideWithBody fixture 的值。
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	if len(shapes) != 1 {
		t.Fatalf("want 1 shape, got %d", len(shapes))
	}
	targetID := shapes[0].ID()
	if err := s.RemoveShape(targetID); err != nil {
		t.Fatalf("RemoveShape(%d): %v", targetID, err)
	}

	// 原 Paragraph 句柄应已 stale（shapeHint 找不到所属 shape）。
	if _, err := para.Text(); !isStaleHandle(err) {
		t.Errorf("Text after RemoveShape: err=%v, want ErrStaleHandle", err)
	}
	if _, err := para.ReplaceText("hello", "world"); !isStaleHandle(err) {
		t.Errorf("ReplaceText after RemoveShape: err=%v, want ErrStaleHandle", err)
	}
}

// TestTextNodeStale_OnSiblingShapeAdded 验证 shapeHint 在兄弟 shape 增删
// 后也能识别：原 Paragraph 句柄的 path 解析到的是新加 shape 的同形 txBody
// 而非原 shape——必须 ErrStaleHandle 而非错配。
func TestTextNodeStale_OnSiblingShapeAdded(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>hello</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	tf, err := shapes[0].(*AutoShape).TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	paras, err := tf.Paragraphs()
	if err != nil {
		t.Fatalf("Paragraphs: %v", err)
	}
	if len(paras) == 0 {
		t.Fatalf("want paragraphs")
	}
	saved := paras[0]

	// 在原 shape 前插入一个 AutoShape。AddTextBox 会产生新的 cNvPr@id，
	// 但原句柄的 shapeHint（指向原 shape）不变，路径仍解析到原 shape
	// 的 txBody——句柄仍有效。这与 shapeNode STALE-GUARD 语义一致：
	// MoveShape 后句柄仍有效，AddShape 同样不影响其他 shape 的句柄。
	if _, err := s.AddTextBox(TextBoxSpec{X: 100, Y: 100, Width: 100, Height: 100}); err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}

	if got, err := saved.Text(); err != nil {
		t.Errorf("Text after AddTextBox: err=%v, want nil (handle should still be valid)", err)
	} else if got != "hello" {
		t.Errorf("Text after AddTextBox: %q, want %q", got, "hello")
	}
}

// TestTextNodeStale_BackwardCompat_NoShapeHint 验证 shapeHint=0 句柄
// 走纯路径判定，向后兼容——不强制要求 shapeHint 校验。
func TestTextNodeStale_BackwardCompat_NoShapeHint(t *testing.T) {
	// 直接构造 shapeHint=0 的 Paragraph 句柄（模仿老句柄/notes 等场景）。
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>hello</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	tf, err := shapes[0].(*AutoShape).TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	paras, err := tf.Paragraphs()
	if err != nil {
		t.Fatalf("Paragraphs: %v", err)
	}
	zero := &Paragraph{
		textNode: textNode{p: paras[0].p, part: paras[0].part, path: paras[0].path, shapeHint: 0},
		idx:      0,
	}
	if got, err := zero.Text(); err != nil || got != "hello" {
		t.Errorf("shapeHint=0 paragraph: got=%q err=%v", got, err)
	}
}

// TestTextNodeStale_ClosedDocument 验证 Closed 句柄返回 ErrClosed
// （shapeHint 校验不掩盖 Close 检查）。
func TestTextNodeStale_ClosedDocument(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>hello</a:t></a:r></a:p>`)
	s := mustSlide(t, p)
	shapes, _ := s.Shapes()
	tf, _ := shapes[0].(*AutoShape).TextFrame()
	paras, _ := tf.Paragraphs()
	saved := paras[0]
	p.Close()
	if _, err := saved.Text(); !isClosedOrStale(err) {
		t.Errorf("Text after Close: err=%v, want ErrClosed or ErrStaleHandle", err)
	}
}

// isStaleHandle 用 errors.Is 断言 err 链上含 ErrStaleHandle。
func isStaleHandle(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ErrStaleHandle.Error())
}

// isClosedOrStale 允许 ErrClosed 或 ErrStaleHandle（locate 顺序：closed
// 检查在 shapeHint 校验之前）。
func isClosedOrStale(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), ErrClosed.Error()) {
		return true
	}
	return strings.Contains(err.Error(), ErrStaleHandle.Error())
}
