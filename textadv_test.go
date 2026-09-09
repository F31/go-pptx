package pptx

import (
	"errors"
	"strings"
	"testing"
)

// ---------- TextFrame.BodyProps / SetBodyProps ----------

func bodyWith(text string) string {
	return `<a:bodyPr` + text + `/>` +
		`<a:p><a:r><a:t>body</a:t></a:r></a:p>`
}

func TestTextFrame_BodyProps_Empty(t *testing.T) {
	p := slideWithBody(t, bodyWith(""))
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	bp, err := tf.BodyProps()
	if err != nil {
		t.Fatalf("BodyProps: %v", err)
	}
	if bp.Columns.Set || bp.Vertical.Set || bp.AnchorCenter.Set {
		t.Errorf("expected zero body props, got %+v", bp)
	}
}

func TestTextFrame_BodyProps_WriteRead(t *testing.T) {
	p := slideWithBody(t, bodyWith(""))
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	want := BodyProps{
		Columns:      NewOptional(2),
		Vertical:     NewOptional("vert"),
		AnchorCenter: NewOptional(true),
	}
	if err := tf.SetBodyProps(want); err != nil {
		t.Fatalf("SetBodyProps: %v", err)
	}
	got, err := tf.BodyProps()
	if err != nil {
		t.Fatalf("BodyProps: %v", err)
	}
	if !got.Columns.Set || got.Columns.Value != 2 ||
		!got.Vertical.Set || got.Vertical.Value != "vert" ||
		!got.AnchorCenter.Set || got.AnchorCenter.Value != true {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestTextFrame_BodyProps_PatchSemantics(t *testing.T) {
	p := slideWithBody(t, bodyWith(` numCol="2" vert="horz"`))
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	// 仅设置 AnchorCenter → numCol/vert 不动。
	if err := tf.SetBodyProps(BodyProps{AnchorCenter: NewOptional(false)}); err != nil {
		t.Fatalf("SetBodyProps: %v", err)
	}
	got, err := tf.BodyProps()
	if err != nil {
		t.Fatalf("BodyProps: %v", err)
	}
	if !got.Columns.Set || got.Columns.Value != 2 {
		t.Errorf("numCol must be preserved: %+v", got.Columns)
	}
	if !got.Vertical.Set || got.Vertical.Value != "horz" {
		t.Errorf("vert must be preserved: %+v", got.Vertical)
	}
	if !got.AnchorCenter.Set || got.AnchorCenter.Value != false {
		t.Errorf("AnchorCenter not updated: %+v", got.AnchorCenter)
	}
}

func TestTextFrame_BodyProps_Clear(t *testing.T) {
	p := slideWithBody(t, bodyWith(` numCol="3" vert="vert270" anchorCtr="1"`))
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	// Set=true Value=零值 → 清除本地属性。
	if err := tf.SetBodyProps(BodyProps{
		Columns:  Optional[int]{Value: 0, Set: true},
		Vertical: Optional[string]{Value: "", Set: true},
	}); err != nil {
		t.Fatalf("SetBodyProps: %v", err)
	}
	got, err := tf.BodyProps()
	if err != nil {
		t.Fatalf("BodyProps: %v", err)
	}
	if got.Columns.Set || got.Vertical.Set {
		t.Errorf("expected cleared, got %+v", got)
	}
	if !got.AnchorCenter.Set {
		t.Errorf("AnchorCenter must be preserved (not in clear set)")
	}
}

func TestTextFrame_BodyProps_InvalidColumns(t *testing.T) {
	p := slideWithBody(t, bodyWith(""))
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	err := tf.SetBodyProps(BodyProps{Columns: NewOptional(-1)})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for negative Columns, got %v", err)
	}
}

func TestTextFrame_BodyProps_InvalidVert(t *testing.T) {
	p := slideWithBody(t, bodyWith(""))
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	err := tf.SetBodyProps(BodyProps{Vertical: NewOptional("weird")})
	if err == nil || !strings.Contains(err.Error(), "whitelist") {
		t.Errorf("expected whitelist error, got %v", err)
	}
}

func TestTextFrame_BodyProps_AllVertValues(t *testing.T) {
	for _, v := range []string{"horz", "vert", "vert270", "wordArtVert", "eaVert", "mongolianVert"} {
		p := slideWithBody(t, bodyWith(""))
		defer p.Close()
		s := mustSlide(t, p)
		tf := slideBodyTF(t, s)
		if err := tf.SetBodyProps(BodyProps{Vertical: NewOptional(v)}); err != nil {
			t.Errorf("SetBodyProps(%s): %v", v, err)
		}
		got, err := tf.BodyProps()
		if err != nil {
			t.Fatalf("BodyProps: %v", err)
		}
		if !got.Vertical.Set || got.Vertical.Value != v {
			t.Errorf("got %q want %q", got.Vertical.Value, v)
		}
	}
}

func TestTextFrame_BodyProps_AutoCreate(t *testing.T) {
	// txBody 不含 bodyPr（自闭合 txBody）→ SetBodyProps 必须新建 bodyPr。
	p := slideWithBody(t, `<a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	if err := tf.SetBodyProps(BodyProps{Columns: NewOptional(4)}); err != nil {
		t.Fatalf("SetBodyProps: %v", err)
	}
	got, err := tf.BodyProps()
	if err != nil {
		t.Fatalf("BodyProps: %v", err)
	}
	if !got.Columns.Set || got.Columns.Value != 4 {
		t.Errorf("got %+v want Columns=4", got)
	}
}

// ---------- Paragraph.Field / AppendField / Remove / Text 含字段 ----------

func TestParagraph_AppendField_SlideNum(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>Page </a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, err := tf.Paragraphs()
	if err != nil || len(paras) != 1 {
		t.Fatalf("Paragraphs: %v len=%d", err, len(paras))
	}
	fld, err := paras[0].AppendField(FieldSpec{Kind: FieldSlideNumber, Text: "1"})
	if err != nil {
		t.Fatalf("AppendField: %v", err)
	}
	if fld.fldIdx < 0 {
		t.Errorf("Field handle idx invalid: %d", fld.fldIdx)
	}
	kind, err := fld.Kind()
	if err != nil || kind != FieldSlideNumber {
		t.Errorf("Kind: %v %v", kind, err)
	}
	text, err := fld.Text()
	if err != nil || text != "1" {
		t.Errorf("Text: %q err=%v", text, err)
	}
	// Text() 拼接：原 Run + 字段缓存。
	got, err := paras[0].Text()
	if err != nil {
		t.Fatalf("Paragraph.Text: %v", err)
	}
	if got != "Page 1" {
		t.Errorf("Paragraph.Text=%q want %q", got, "Page 1")
	}
}

func TestParagraph_AppendField_DateTime(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>Today: </a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, err := tf.Paragraphs()
	if err != nil {
		t.Fatalf("Paragraphs: %v", err)
	}
	fld, err := paras[0].AppendField(FieldSpec{
		Kind:  FieldDateTime,
		Guide: "YYYY-MM-DD",
		Text:  "2026-09-09",
	})
	if err != nil {
		t.Fatalf("AppendField: %v", err)
	}
	guide, err := fld.Guide()
	if err != nil || guide != "YYYY-MM-DD" {
		t.Errorf("Guide=%q err=%v", guide, err)
	}
	kind, _ := fld.Kind()
	if kind != FieldDateTime {
		t.Errorf("Kind=%v", kind)
	}
}

func TestParagraph_AppendField_UnknownKind(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	_, err := paras[0].AppendField(FieldSpec{Kind: "user"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Errorf("expected ErrUnsupportedEdit, got %v (%T)", err, err)
		var oe *OperationError
		if errors.As(err, &oe) {
			t.Logf("Op=%s Msg=%s Err=%v (%T)", oe.Op, oe.Message, oe.Err, oe.Err)
		}
	}
}

func TestParagraph_AppendField_InvalidGuide(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	_, err := paras[0].AppendField(FieldSpec{
		Kind:  FieldDateTime,
		Guide: "garbage",
	})
	if err == nil || !strings.Contains(err.Error(), "whitelist") {
		t.Errorf("expected whitelist error, got %v", err)
	}
}

func TestParagraph_AppendField_SlideNumNoGuide(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	_, err := paras[0].AppendField(FieldSpec{Kind: FieldSlideNumber, Guide: "x"})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestParagraph_Fields_AndRemove(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>a</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	f1, err := paras[0].AppendField(FieldSpec{Kind: FieldSlideNumber, Text: "1"})
	if err != nil {
		t.Fatalf("AppendField1: %v", err)
	}
	if _, err := paras[0].AppendField(FieldSpec{Kind: FieldSlideNumber, Text: "2"}); err != nil {
		t.Fatalf("AppendField2: %v", err)
	}
	flds, err := paras[0].Fields()
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if len(flds) != 2 {
		t.Fatalf("Fields len=%d", len(flds))
	}
	// Remove 第一个。
	if err := f1.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	flds, _ = paras[0].Fields()
	if len(flds) != 1 {
		t.Fatalf("after remove, Fields len=%d", len(flds))
	}
	// Runs 不应包含字段。
	runs, err := paras[0].Runs()
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("Runs len=%d (fields must not appear)", len(runs))
	}
}

func TestParagraph_InsertField_AfterRun(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p><a:r><a:t>before</a:t></a:r><a:r><a:t>after</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	runs, _ := paras[0].Runs()
	if len(runs) != 2 {
		t.Fatalf("Runs len=%d", len(runs))
	}
	if _, err := paras[0].InsertField(runs[0], FieldSpec{Kind: FieldSlideNumber, Text: "9"}); err != nil {
		t.Fatalf("InsertField: %v", err)
	}
	got, _ := paras[0].Text()
	if got != "before9after" {
		t.Errorf("Text=%q want %q", got, "before9after")
	}
}

func TestParagraph_InsertField_NilAnchor(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	fld, err := paras[0].InsertField(nil, FieldSpec{Kind: FieldSlideNumber, Text: "1"})
	if err != nil {
		t.Fatalf("InsertField(nil): %v", err)
	}
	if fld.fldIdx < 0 {
		t.Errorf("fldIdx invalid")
	}
}

func TestParagraph_InsertField_StaleRun(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	// 引用不存在的段落索引 5 → 句柄无效。
	stale := &TextRun{textNode: textNode{p: s.p, part: s.part, path: nil}, paraIdx: 5, runIdx: 0}
	_, err := paras[0].InsertField(stale, FieldSpec{Kind: FieldSlideNumber, Text: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrStaleHandle) {
		t.Errorf("expected ErrStaleHandle, got %v", err)
	}
}

func TestField_SetText(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	fld, err := paras[0].AppendField(FieldSpec{Kind: FieldSlideNumber, Text: "1"})
	if err != nil {
		t.Fatalf("AppendField: %v", err)
	}
	if err := fld.SetText("42"); err != nil {
		t.Fatalf("SetText: %v", err)
	}
	got, _ := fld.Text()
	if got != "42" {
		t.Errorf("Text=%q want 42", got)
	}
}

func TestField_SetText_XMLEscape(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	fld, _ := paras[0].AppendField(FieldSpec{Kind: FieldSlideNumber, Text: "a"})
	// 含 "&" 必须转义为 &amp;。
	if err := fld.SetText("a & b"); err != nil {
		t.Fatalf("SetText: %v", err)
	}
	xml := slideXML(t, s)
	if !strings.Contains(xml, "&amp;") {
		t.Errorf("xml must contain escaped ampersand: %s", xml)
	}
	got, _ := fld.Text()
	if got != "a & b" {
		t.Errorf("Text=%q", got)
	}
}

func TestParagraph_Text_IncludesFieldCache(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p>`+
		`<a:r><a:t>P </a:t></a:r>`+
		`<a:fld type="slidenum"><a:t>7</a:t></a:fld>`+
		`<a:r><a:t> end</a:t></a:r>`+
		`</a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	got, err := paras[0].Text()
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if got != "P 7 end" {
		t.Errorf("Text=%q want %q", got, "P 7 end")
	}
}

func TestParagraph_Text_IncludesSelfClosedField(t *testing.T) {
	// 自闭合 fld（无 a:t）→ 字段位置贡献空字符串。
	p := slideWithBody(t, `<a:bodyPr/>`+
		`<a:p>`+
		`<a:r><a:t>A</a:t></a:r>`+
		`<a:fld type="slidenum"/>`+
		`<a:r><a:t>B</a:t></a:r>`+
		`</a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	got, _ := paras[0].Text()
	if got != "AB" {
		t.Errorf("Text=%q want %q", got, "AB")
	}
}

func TestParagraph_Fields_Empty(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	flds, err := paras[0].Fields()
	if err != nil {
		t.Fatalf("Fields: %v", err)
	}
	if len(flds) != 0 {
		t.Errorf("len=%d", len(flds))
	}
}

func TestParagraph_AppendField_WithStyle(t *testing.T) {
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	fld, err := paras[0].AppendField(FieldSpec{
		Kind:  FieldSlideNumber,
		Text:  "1",
		Style: FontStyle{Bold: NewOptional(true)},
	})
	if err != nil {
		t.Fatalf("AppendField: %v", err)
	}
	xml := slideXML(t, s)
	if !strings.Contains(xml, `<a:rPr b="1"`) {
		t.Errorf("expected rPr in field XML, got %s", xml)
	}
	if fld.fldIdx < 0 {
		t.Errorf("fldIdx invalid")
	}
}
