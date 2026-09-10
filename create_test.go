package pptx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// createSlide 返回带一页的测试 Presentation 与 Slide。
func createSlide(t *testing.T) (*Presentation, *Slide) {
	t.Helper()
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	layouts, err := p.Layouts()
	if err != nil || len(layouts) == 0 {
		p.Close()
		t.Fatalf("Layouts: %v (n=%d)", err, len(layouts))
	}
	s, err := p.AddSlide(layouts[0])
	if err != nil {
		p.Close()
		t.Fatalf("AddSlide: %v", err)
	}
	return p, s
}

func TestAddTextBox_Basic(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()

	tb, err := s.AddTextBox(TextBoxSpec{
		X: 100, Y: 200, Width: 914400, Height: 457200,
		Name: "Custom Box", Text: "Hello\nWorld",
	})
	if err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	if len(shapes) != 1 {
		t.Fatalf("shapes = %d, want 1", len(shapes))
	}
	if shapes[0].Kind() != ShapeTextBox {
		t.Errorf("kind = %v, want ShapeTextBox", shapes[0].Kind())
	}
	if shapes[0].Name() != "Custom Box" {
		t.Errorf("name = %q", shapes[0].Name())
	}
	if shapes[0].ID() == 0 {
		t.Errorf("id = 0, want allocated")
	}

	// 句柄可用：TextFrame 读取两段文本。
	tf, err := tb.TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	paras, err := tf.Paragraphs()
	if err != nil || len(paras) != 2 {
		t.Fatalf("Paragraphs: %v (n=%d)", err, len(paras))
	}
	got0, _ := paras[0].Text()
	got1, _ := paras[1].Text()
	if got0 != "Hello" || got1 != "World" {
		t.Errorf("text = %q / %q, want Hello/World", got0, got1)
	}

	// 保存回读后结构仍有效。
	out := t.TempDir() + "/tb.pptx"
	if _, err := p.Save(context.Background(), out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p2, err := Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p2.Close()
	slides, _ := p2.Slides()
	shapes2, err := slides[0].Shapes()
	if err != nil || len(shapes2) != 1 {
		t.Fatalf("reload shapes: %v (n=%d)", err, len(shapes2))
	}
	if shapes2[0].Kind() != ShapeTextBox {
		t.Errorf("reload kind = %v", shapes2[0].Kind())
	}
}

func TestAddTextBox_DefaultNameAndEmptyText(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	tb, err := s.AddTextBox(TextBoxSpec{X: 0, Y: 0, Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}
	if !strings.HasPrefix(tb.Name(), "TextBox ") {
		t.Errorf("default name = %q", tb.Name())
	}
	tf, err := tb.TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	paras, _ := tf.Paragraphs()
	if len(paras) != 1 {
		t.Errorf("empty text should yield one empty paragraph, got %d", len(paras))
	}
}

func TestAddTextBox_InvalidSize(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	if _, err := s.AddTextBox(TextBoxSpec{Width: 0, Height: 100}); err == nil {
		t.Error("zero width must fail")
	}
	if _, err := s.AddTextBox(TextBoxSpec{Width: 100, Height: -5}); err == nil {
		t.Error("negative height must fail")
	}
}

func TestAddAutoShape_Basic(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	sh, err := s.AddAutoShape(AutoShapeSpec{
		X: 1000, Y: 2000, Width: 500000, Height: 300000,
		Geometry: "roundRect", Name: "Bullet",
		AltText: "a rounded rectangle", Text: "label",
	})
	if err != nil {
		t.Fatalf("AddAutoShape: %v", err)
	}
	shapes, _ := s.Shapes()
	if len(shapes) != 1 {
		t.Fatalf("shapes = %d", len(shapes))
	}
	if shapes[0].Kind() != ShapeAutoShape {
		t.Errorf("kind = %v", shapes[0].Kind())
	}
	if shapes[0].AltText() != "a rounded rectangle" {
		t.Errorf("alt text = %q", shapes[0].AltText())
	}
	// 几何与文本经 XML 校验：prstGeom prst=roundRect 存在。
	doc, err := p.docOf(s.part)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc.Original()), `prst="roundRect"`) {
		t.Error("prstGeom prst=roundRect missing in slide XML")
	}
	// 句柄 TextFrame 读回文本。
	tf, err := sh.TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	paras, _ := tf.Paragraphs()
	if len(paras) != 1 {
		t.Fatalf("paragraphs = %d", len(paras))
	}
	txt, _ := paras[0].Text()
	if txt != "label" {
		t.Errorf("text = %q", txt)
	}
}

func TestAddAutoShape_InvalidGeometry(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	for _, g := range []string{"", "has space", `bad"quote`, "<xml>"} {
		if _, err := s.AddAutoShape(AutoShapeSpec{Width: 10, Height: 10, Geometry: g}); err == nil {
			t.Errorf("geometry %q must be rejected", g)
		}
	}
}

func TestAddShape_DecorativeExcludesAltText(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	if _, err := s.AddTextBox(TextBoxSpec{
		Width: 10, Height: 10, AltText: "ignored", IsDecorative: true,
	}); err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}
	shapes, _ := s.Shapes()
	if !shapes[0].IsDecorative() {
		t.Error("decorative flag not set")
	}
	if shapes[0].AltText() != "" {
		t.Errorf("alt text = %q, want empty (mutual exclusion)", shapes[0].AltText())
	}
}

func TestAddShape_MultipleIDsIncrease(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	a, err := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AddAutoShape(AutoShapeSpec{Width: 10, Height: 10, Geometry: "ellipse"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() == b.ID() {
		t.Errorf("ids must differ: %d", a.ID())
	}
	shapes, _ := s.Shapes()
	if len(shapes) != 2 {
		t.Fatalf("shapes = %d", len(shapes))
	}
}

// shapeName 把当前 spTree 顶层形状的 Name 列出来（用 s.Shapes() 取得
// 新鲜句柄——既有的 Shape 句柄在文档被编辑后可能路径错位，需重新获取）。
func shapeName(s *Slide) []string {
	shapes, _ := s.Shapes()
	out := make([]string, len(shapes))
	for i, sh := range shapes {
		out[i] = sh.Name()
	}
	return out
}

func TestRemoveShape_Basic(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	_, _ = s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	_, _ = s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "B"})
	pre := shapeName(s)
	if strings.Join(pre, ",") != "A,B" {
		t.Fatalf("pre = %v, want A,B", pre)
	}
	// 用新鲜句柄定位 A 的 id（既有句柄在文档被反复编辑后路径可能
	// 错位——本测试不依赖早期持有的 handle）。
	preShapes, _ := s.Shapes()
	aID := preShapes[0].ID()
	if err := s.RemoveShape(aID); err != nil {
		t.Fatalf("RemoveShape: %v", err)
	}
	post := shapeName(s)
	if strings.Join(post, ",") != "B" {
		t.Fatalf("remaining = %v, want B", post)
	}
	// 注：本库 Shape 句柄的路径解析在文档被反复编辑后可能落到相邻
	// 兄弟元素上（resolvePath 在路径目标缺失时回退到 nth 同名元素），
	// 属于 shapes.go resolvePath 的已知语义。RemoveShape 的契约是
	// "把目标 sp 从 spTree 中删除"，由 Slide.Shapes() 取得的新鲜句柄
	// 保证正确；旧句柄的有效性不在本测试覆盖范围。
	// 重复删除 → ErrNotFound。
	if err := s.RemoveShape(aID); err == nil {
		t.Error("second remove must fail")
	}
}

func TestRemoveShape_NotFound(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	if err := s.RemoveShape(9999); err == nil {
		t.Error("unknown id must fail")
	}
}

func TestMoveShape_ZOrder(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	_, _ = s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	_, _ = s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "B"})
	_, _ = s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "C"})

	pre, _ := s.Shapes()
	aID := pre[0].ID()

	// A 前移到末尾（zIndex 2）。
	if err := s.MoveShape(aID, 2); err != nil {
		t.Fatalf("MoveShape: %v", err)
	}
	if got := strings.Join(shapeName(s), ","); got != "B,C,A" {
		t.Errorf("after move-to-end = %v", got)
	}
	// 用新鲜句柄拿 C 的当前 id（早期 handle 在 MoveShape 后可能路径错位）。
	cur, _ := s.Shapes()
	cCur := cur[1].ID() // C 现在 index 1
	if err := s.MoveShape(cCur, 0); err != nil {
		t.Fatalf("MoveShape: %v", err)
	}
	if got := strings.Join(shapeName(s), ","); got != "C,B,A" {
		t.Errorf("after move-to-front = %v", got)
	}
	// 原地移动是空操作。
	if err := s.MoveShape(cCur, 0); err != nil {
		t.Fatalf("no-op MoveShape: %v", err)
	}
	// 越界与未知 id。
	if err := s.MoveShape(aID, 3); err == nil {
		t.Error("out-of-range zIndex must fail")
	}
	if err := s.MoveShape(9999, 0); err == nil {
		t.Error("unknown id must fail")
	}
}

func TestMoveShape_SaveReload(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	_, _ = s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	_, _ = s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "B"})
	pre, _ := s.Shapes()
	if err := s.MoveShape(pre[0].ID(), 1); err != nil {
		t.Fatalf("MoveShape: %v", err)
	}
	out := t.TempDir() + "/moved.pptx"
	if _, err := p.Save(context.Background(), out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p2, err := Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p2.Close()
	slides, _ := p2.Slides()
	shapes, _ := slides[0].Shapes()
	if len(shapes) != 2 || shapes[0].Name() != "B" || shapes[1].Name() != "A" {
		t.Fatalf("reload order = %+v", shapes)
	}
}

func TestTextFrame_AddParagraph(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	tb, err := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Text: "first"})
	if err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}
	tf, err := tb.TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	para, err := tf.AddParagraph(ParagraphSpec{Text: "second"})
	if err != nil {
		t.Fatalf("AddParagraph: %v", err)
	}
	paras, _ := tf.Paragraphs()
	if len(paras) != 2 {
		t.Fatalf("paragraphs = %d", len(paras))
	}
	txt, err := para.Text()
	if err != nil || txt != "second" {
		t.Errorf("new paragraph text = %q (err=%v)", txt, err)
	}
	// 空文本段落（仅占位，无 Run）。
	if _, err := tf.AddParagraph(ParagraphSpec{}); err != nil {
		t.Fatalf("AddParagraph empty: %v", err)
	}
	paras, _ = tf.Paragraphs()
	if len(paras) != 3 {
		t.Fatalf("paragraphs = %d", len(paras))
	}
}

func TestCreate_ClosedGuard(t *testing.T) {
	p, s := createSlide(t)
	p.Close()
	if _, err := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10}); err == nil {
		t.Error("AddTextBox after Close must fail")
	}
	if _, err := s.AddAutoShape(AutoShapeSpec{Width: 10, Height: 10, Geometry: "rect"}); err == nil {
		t.Error("AddAutoShape after Close must fail")
	}
	if err := s.RemoveShape(1); err == nil {
		t.Error("RemoveShape after Close must fail")
	}
	if err := s.MoveShape(1, 0); err == nil {
		t.Error("MoveShape after Close must fail")
	}
}

// ---------- STALE-GUARD 测试（M8 STALE-GUARD 修复）----------
//
// 句柄构造时记下 cNvPr@id（idHint），locate 解析路径后校验 cNvPr@id
// 一致——不一致返回 ErrStaleHandle，杜绝"原元素消失后 path 解析到相邻
// 兄弟"导致 ID/Name 错位的问题。

// staleGuard_NameAfterRemove：删除同一页前部形状后，**旧**句柄的
// 任何属性读取必须返回 ErrStaleHandle（之前的行为是静默返回相邻兄弟
// 的 ID/Name——SHAPE-CREATE 暴露的 bug）。
func TestStaleGuard_NameAfterRemove(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	// 显式添加三个文本框，确保至少三个顶层 sp 元素。
	a, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	b, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "B"})
	c, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "C"})
	aID, bID, cID := a.ID(), b.ID(), c.ID()
	if err := s.RemoveShape(aID); err != nil {
		t.Fatalf("RemoveShape: %v", err)
	}
	// a 现在仍持有原 path（sp[0]），但该位置已被 b 占据——
	// 修复前 a.Name() == "B"、a.ID() == bID；修复后必须返回
	// ErrStaleHandle，Name/ID 走错误分支返回空值/0。
	if got := a.Name(); got != "" {
		t.Errorf("stale a.Name() = %q, want \"\" (must be ErrStaleHandle)", got)
	}
	if got := a.ID(); got != 0 {
		t.Errorf("stale a.ID() = %d, want 0 (must be ErrStaleHandle)", got)
	}
	if _, err := a.Bounds(); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("stale a.Bounds err = %v, want ErrStaleHandle", err)
	}
	// b/c 的 path 仍指向原 sp[1]/sp[2] 位置（无前移发生）→ 仍有效。
	if b.ID() != bID || b.Name() != "B" {
		t.Errorf("b handle broken: id=%d name=%q", b.ID(), b.Name())
	}
	if c.ID() != cID || c.Name() != "C" {
		t.Errorf("c handle broken: id=%d name=%q", c.ID(), c.Name())
	}
}

// staleGuard_NameAfterMove：把一个形状 MoveShape 到末尾后，形状本
// 身（cNvPr@id）仍在文档中，句柄**仍应有效**——只是位置变化。这是
// STALE-GUARD 的关键不变量：句柄身份 = cNvPr@id，不是 path。
// 行为变化（位置 vs 句柄是否有效）的解读：编辑后想拿"新位置"应重
// 新取 s.Shapes()。
func TestStaleGuard_NameAfterMove(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	a, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	b, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "B"})
	preOrder := func() string {
		shapes, _ := s.Shapes()
		names := make([]string, len(shapes))
		for i, sh := range shapes {
			names[i] = sh.Name()
		}
		return strings.Join(names, ",")
	}
	if got := preOrder(); got != "A,B" {
		t.Fatalf("pre order = %q, want A,B", got)
	}
	if err := s.MoveShape(a.ID(), 1); err != nil {
		t.Fatalf("MoveShape: %v", err)
	}
	// z-order 已变：B,A。
	if got := preOrder(); got != "B,A" {
		t.Errorf("post-move order = %q, want B,A", got)
	}
	// 关键不变量：a 句柄仍有效（cNvPr@id 未变）。
	if a.Name() != "A" {
		t.Errorf("a.Name() = %q after MoveShape, want A (handle should remain valid)", a.Name())
	}
	if _, err := a.Bounds(); err != nil {
		t.Errorf("a.Bounds err = %v after MoveShape, want nil (handle should remain valid)", err)
	}
	if b.Name() != "B" {
		t.Errorf("b.Name() = %q after sibling move, want B", b.Name())
	}
}

// staleGuard_RefreshedHandleOK：编辑后用 Shapes() 重新取句柄，操作
// 一切正常。这是推荐用法，验证 fix 不破坏正常路径。
func TestStaleGuard_RefreshedHandleOK(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	a, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	if err := s.RemoveShape(a.ID()); err != nil {
		t.Fatalf("RemoveShape: %v", err)
	}
	// 重新枚举——句柄新鲜，Name 正确返回。
	shapes, _ := s.Shapes()
	if len(shapes) != 0 {
		t.Errorf("shapes after remove = %d, want 0", len(shapes))
	}
}

// staleGuard_NewlyCreatedHandleStillValid：构造后立即操作（无中间
// 编辑）的句柄必须仍能正常解析。
func TestStaleGuard_NewlyCreatedHandleStillValid(t *testing.T) {
	p, s := createSlide(t)
	defer p.Close()
	a, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "Alpha"})
	if a.Name() != "Alpha" {
		t.Errorf("a.Name() = %q, want Alpha", a.Name())
	}
	if _, err := a.Bounds(); err != nil {
		t.Errorf("fresh a.Bounds err = %v, want nil", err)
	}
}

// staleGuard_SaveReloadPreservesGuards：Save+Open 后，旧句柄（来自
// 序列化前的内存）自然失效（c.p 指针已变/文档对象不同）——验证守护
// 不会让"无意义"句柄绕过判定。
func TestStaleGuard_SaveReloadPreservesGuards(t *testing.T) {
	p, s := createSlide(t)
	a, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	// Save+Open 后旧 a 不在新 doc 里——p 已关闭，句柄应已失效。
	out := t.TempDir() + "/sg.pptx"
	if _, err := p.Save(context.Background(), out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := a.Bounds(); !errors.Is(err, ErrClosed) {
		t.Errorf("a.Bounds after Close err = %v, want ErrClosed", err)
	}
	_ = s
}
