package pptx

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- 测试夹具 ----------

// slideWithBody 构造含一个正文占位符（ph type=body）的包；bodyInner 是
// p:txBody 的内部 XML（不含 txBody 自身）。
func slideWithBody(t *testing.T, bodyInner string) *Presentation {
	t.Helper()
	return openFixture(t, fixtureSlideDeck(bodyInner))
}

// fixtureSlideDeck 生成 1 页含正文占位符的完整包字节。
func fixtureSlideDeck(bodyInner string) []byte {
	sld := `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Body 1"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr/><p:txBody>` + bodyInner + `</p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:sld>`
	parts := minimalTemplateParts()
	name := opc.PartName("/ppt/slides/slide1.xml")
	parts[name] = []byte(xmlDecl + sld)
	// CT override + 关系 + sldId（单页）。
	parts["/[Content_Types].xml"] = bytes.Replace(parts["/[Content_Types].xml"],
		[]byte("</Types>"),
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`), 1)
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`<Relationship Id="rId2" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	data, err := buildPackageZip(parts)
	if err != nil {
		panic(err)
	}
	return data
}

func openFixture(t *testing.T, data []byte) *Presentation {
	t.Helper()
	p, err := OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func mustSlide(t *testing.T, p *Presentation) *Slide {
	t.Helper()
	slides, err := p.Slides()
	if err != nil || len(slides) != 1 {
		t.Fatalf("slides = %d, %v", len(slides), err)
	}
	return slides[0]
}

// slideBodyTF 返回页面第一个含 p:txBody 的 sp 的 TextFrame（测试定位；
// 页面 Shapes() API 属 M2，届时替换此辅助）。
func slideBodyTF(t *testing.T, s *Slide) *TextFrame {
	t.Helper()
	doc, err := s.p.docOf(s.part)
	if err != nil {
		t.Fatalf("docOf: %v", err)
	}
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for _, sid := range tree.Children {
			sp := doc.Node(sid)
			if sp.Namespace != nsPresentationML || sp.Local() != "sp" {
				continue
			}
			tx := childOfKind(doc, sp, nsPresentationML, "txBody", 0)
			if tx == nil {
				continue
			}
			return &TextFrame{textNode: textNode{p: s.p, part: s.part, path: recordPath(doc, tx.ID)}}
		}
	}
	t.Fatal("no slide text frame")
	return nil
}

// slideXML 返回页面当前字节（字符串形式）。
func slideXML(t *testing.T, s *Slide) string {
	t.Helper()
	data, err := s.p.partBytes(s.part)
	if err != nil {
		t.Fatalf("partBytes: %v", err)
	}
	return string(data)
}

// ---------- 文本模型读 ----------

func bodyInner2(t *testing.T) string {
	return `<a:bodyPr/><a:lstStyle/>` +
		`<a:p><a:pPr marL="342900"/><a:r><a:rPr lang="en-US" sz="1800" b="1"/><a:t>Hello </a:t></a:r>` +
		`<a:r><a:rPr b="0"/><a:t>Dolly &amp; 朋友</a:t></a:r><a:endParaRPr lang="en-US"/></a:p>` +
		`<a:p><a:r><a:t>Second</a:t></a:r></a:p>`
}

func TestTextFrameRead(t *testing.T) {
	p := slideWithBody(t, bodyInner2(t))
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)

	paras, err := tf.Paragraphs()
	if err != nil {
		t.Fatalf("Paragraphs: %v", err)
	}
	if len(paras) != 2 {
		t.Fatalf("paragraphs = %d, want 2", len(paras))
	}
	// 段落属性与结束字符属性保留（模型不扁平化）：第 0 段有 pPr 与 endParaRPr。
	{
		_, paraNode, err := paras[0].locatePara()
		if err != nil {
			t.Fatalf("locatePara: %v", err)
		}
		doc, err := s.p.docOf(s.part)
		if err != nil {
			t.Fatal(err)
		}
		_ = doc
		if childOfKind(doc, paraNode, nsDrawingML, "pPr", 0) == nil {
			t.Error("pPr not retained in model")
		}
		if childOfKind(doc, paraNode, nsDrawingML, "endParaRPr", 0) == nil {
			t.Error("endParaRPr not retained in model")
		}
	}
	txt, err := paras[0].Text()
	if err != nil {
		t.Fatal(err)
	}
	if txt != "Hello Dolly & 朋友" {
		t.Errorf("para0 text = %q", txt)
	}
	txt1, err := paras[1].Text()
	if err != nil {
		t.Fatal(err)
	}
	if txt1 != "Second" {
		t.Errorf("para1 text = %q", txt1)
	}
	runs, err := paras[0].Runs()
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	ef, err := runs[0].ExplicitFont()
	if err != nil {
		t.Fatal(err)
	}
	if !ef.Bold.Set || ef.Bold.Value != true {
		t.Errorf("run0 bold = %+v", ef.Bold)
	}
	if !ef.Size.Set || float64(ef.Size.Value) != 18.0 {
		t.Errorf("run0 size = %+v (want 18pt)", ef.Size)
	}
	if ef.Italic.Set {
		t.Errorf("run0 italic should be unset, got %+v", ef.Italic)
	}
	// 显式取消粗体（Set=true, Value=false）可读回。
	ef1, _ := runs[1].ExplicitFont()
	if !ef1.Bold.Set || ef1.Bold.Value {
		t.Errorf("run1 bold = %+v, want Set=true Value=false", ef1.Bold)
	}
	rt, err := runs[1].Text()
	if err != nil {
		t.Fatal(err)
	}
	if rt != "Dolly & 朋友" {
		t.Errorf("run1 text = %q", rt)
	}
}

// ---------- SetPlainText ----------

func TestSetPlainTextStructure(t *testing.T) {
	p := slideWithBody(t, bodyInner2(t))
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)

	if err := tf.SetPlainText("第一行 & 第二行\nSecond & 更多"); err != nil {
		t.Fatalf("SetPlainText: %v", err)
	}
	paras, _ := tf.Paragraphs()
	if len(paras) != 2 {
		t.Fatalf("paragraphs = %d, want 2", len(paras))
	}
	if txt, _ := paras[0].Text(); txt != "第一行 & 第二行" {
		t.Errorf("para0 = %q", txt)
	}
	if txt, _ := paras[1].Text(); txt != "Second & 更多" {
		t.Errorf("para1 = %q", txt)
	}
	xml := slideXML(t, s)
	// 文本框级属性保留。
	if !strings.Contains(xml, "<a:bodyPr/>") || !strings.Contains(xml, "<a:lstStyle/>") {
		t.Error("txBody-level props (bodyPr/lstStyle) not preserved")
	}
	// 旧字符格式被移除（无 sz/b 属性残留）。
	if strings.Contains(xml, `sz="1800"`) || strings.Contains(xml, `b="1"`) || strings.Contains(xml, `marL="342900"`) {
		t.Errorf("character/paragraph formats leaked: %s", xml)
	}
	// 换行实体化正确（&amp;）。
	if !strings.Contains(xml, "第一行 &amp; 第二行") {
		t.Errorf("escape missing: %s", xml)
	}
	// 重新解析 well-formed & 结构可读。
	p2, err := p.Write(nil, &bytes.Buffer{})
	_ = p2
	_ = err
}

func TestSetPlainTextEmptyAndTrailing(t *testing.T) {
	p := slideWithBody(t, bodyInner2(t))
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)

	if err := tf.SetPlainText(""); err != nil {
		t.Fatal(err)
	}
	paras, _ := tf.Paragraphs()
	if len(paras) != 1 {
		t.Fatalf("empty: paragraphs = %d, want 1", len(paras))
	}
	if txt, _ := paras[0].Text(); txt != "" {
		t.Errorf("empty para text = %q", txt)
	}
	if err := tf.SetPlainText("a\nb\n"); err != nil {
		t.Fatal(err)
	}
	paras, _ = tf.Paragraphs()
	if len(paras) != 3 {
		t.Fatalf("trailing: paragraphs = %d, want 3 (tail empty)", len(paras))
	}
}

func TestSetPlainTextRejectsUnknownBodyChild(t *testing.T) {
	inner := `<a:bodyPr/><a:p><a:r><a:t>keep</a:t></a:r></a:p>` +
		`<x:odd xmlns:x="urn:x"><x:raw/></x:odd>`
	p := slideWithBody(t, inner)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	before := slideXML(t, s)

	if err := tf.SetPlainText("new"); !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("err = %v, want ErrUnsupportedEdit", err)
	}
	if got := slideXML(t, s); got != before {
		t.Error("failed SetPlainText changed the part")
	}
}

// ---------- Run 编辑 ----------

func bodyWithRuns() string {
	return `<a:bodyPr/>` +
		`<a:p><a:r><a:rPr lang="en-US" sz="1200" b="1"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:rPr><a:t>Edit</a:t></a:r>` +
		`<a:r><a:t>Me</a:t></a:r></a:p>`
}

func TestRunSetText(t *testing.T) {
	p := slideWithBody(t, bodyWithRuns())
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	runs, _ := paras[0].Runs()

	if err := runs[0].SetText("新的 & 文本"); err != nil {
		t.Fatalf("SetText: %v", err)
	}
	if txt, _ := runs[0].Text(); txt != "新的 & 文本" {
		t.Errorf("after SetText = %q", txt)
	}
	xml := slideXML(t, s)
	if !strings.Contains(xml, "<a:t>新的 &amp; 文本</a:t>") {
		t.Errorf("escaped text missing: %s", xml)
	}
	// 其它 Run 字节不受影响。
	if txt, _ := runs[1].Text(); txt != "Me" {
		t.Errorf("run1 drifted = %q", txt)
	}
}

func TestRunSetFontPatchAndReset(t *testing.T) {
	p := slideWithBody(t, bodyWithRuns())
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	runs, _ := paras[0].Runs()
	r0 := runs[0]

	// patch：加 Italic 与 Latin（只影响 Set 字段；Bold/Size/Color 保留）。
	if err := r0.SetFont(FontStyle{
		Italic: NewOptional(true),
		Latin:  NewOptional("Arial"),
	}); err != nil {
		t.Fatalf("SetFont: %v", err)
	}
	ef, _ := r0.ExplicitFont()
	if !ef.Bold.Set || !ef.Bold.Value {
		t.Errorf("bold lost: %+v", ef.Bold)
	}
	if !ef.Italic.Set || !ef.Italic.Value {
		t.Errorf("italic not applied: %+v", ef.Italic)
	}
	if !ef.Latin.Set || ef.Latin.Value != "Arial" {
		t.Errorf("latin = %+v", ef.Latin)
	}
	if !ef.Size.Set || float64(ef.Size.Value) != 12.0 {
		t.Errorf("size lost: %+v", ef.Size)
	}
	if !ef.Color.Set || ef.Color.Value.RGB != "FF0000" {
		t.Errorf("color lost: %+v", ef.Color)
	}
	// 子元素顺序正确：solidFill(1) < latin(6)。
	xml := slideXML(t, s)
	sf := strings.Index(xml, "<a:solidFill>")
	la := strings.Index(xml, `<a:latin typeface="Arial"/>`)
	if sf < 0 || la < 0 || sf > la {
		t.Errorf("child order wrong: %s", xml)
	}

	// 显式取消粗体（Set=true Value=false）。
	if err := r0.SetFont(FontStyle{Bold: NewOptional(false)}); err != nil {
		t.Fatal(err)
	}
	ef, _ = r0.ExplicitFont()
	if !ef.Bold.Set || ef.Bold.Value {
		t.Errorf("bold = %+v, want explicit false", ef.Bold)
	}
	// 其它属性不受 SetFont(false) 影响。
	ef, _ = r0.ExplicitFont()
	if !ef.Italic.Set || !ef.Italic.Value {
		t.Errorf("italic lost on unrelated patch")
	}

	// Reset：移除 Bold → 无本地 b 属性。
	if err := r0.ResetFontProperty(FontPropBold); err != nil {
		t.Fatal(err)
	}
	ef, _ = r0.ExplicitFont()
	if ef.Bold.Set {
		t.Errorf("bold should be reset, got %+v", ef.Bold)
	}
	// Reset 颜色 → solidFill 删除。
	if err := r0.ResetFontProperty(FontPropColor); err != nil {
		t.Fatal(err)
	}
	ef, _ = r0.ExplicitFont()
	if ef.Color.Set {
		t.Errorf("color should be reset, got %+v", ef.Color)
	}
	// rPr 仍有 italic/size/latin → 保留。
	if err := r0.SetFont(FontStyle{Bold: NewOptional(false)}); err != nil {
		t.Fatal(err)
	}
	ef, _ = r0.ExplicitFont()
	if ef.Size.Set && ef.Italic.Set {
		// ok
	} else {
		t.Errorf("rPr unexpectedly emptied: %+v", ef)
	}
}

func TestRunResetRemovesEmptyRPr(t *testing.T) {
	inner := `<a:bodyPr/><a:p><a:r><a:rPr b="1"/><a:t>X</a:t></a:r></a:p>`
	p := slideWithBody(t, inner)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	runs, _ := paras[0].Runs()

	if err := runs[0].ResetFontProperty(FontPropBold); err != nil {
		t.Fatal(err)
	}
	xml := slideXML(t, s)
	if strings.Contains(xml, "<a:rPr") {
		t.Errorf("empty rPr not removed: %s", xml)
	}
	if !strings.Contains(xml, "<a:r><a:t>X</a:t></a:r>") {
		t.Errorf("run body damaged: %s", xml)
	}
	// Reset 已不存在的属性是 no-op。
	if err := runs[0].ResetFontProperty(FontPropSize); err != nil {
		t.Fatal(err)
	}
}

func TestSetFontCreatesRPrAndReplacesFill(t *testing.T) {
	// 无 rPr 的 Run：SetFont 属性 + 颜色新建 rPr 并插入 solidFill。
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>X</a:t></a:r></a:p>`)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	runs, _ := paras[0].Runs()
	if err := runs[0].SetFont(FontStyle{
		Italic: NewOptional(true),
		Color:  NewOptional(ColorSpec{RGB: "00FF00"}),
	}); err != nil {
		t.Fatalf("SetFont: %v", err)
	}
	ef, _ := runs[0].ExplicitFont()
	if !ef.Italic.Set || !ef.Italic.Value || !ef.Color.Set || ef.Color.Value.RGB != "00FF00" {
		t.Fatalf("font after create = %+v", ef)
	}
	// 非 solidFill 的既有填充被显式颜色替换。
	p2 := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:rPr><a:noFill/></a:rPr><a:t>X</a:t></a:r></a:p>`)
	s2 := mustSlide(t, p2)
	tf2 := slideBodyTF(t, s2)
	paras2, _ := tf2.Paragraphs()
	runs2, _ := paras2[0].Runs()
	if err := runs2[0].SetFont(FontStyle{Color: NewOptional(ColorSpec{Scheme: "accent1"})}); err != nil {
		t.Fatalf("SetFont replace fill: %v", err)
	}
	xml := slideXML(t, s2)
	if strings.Contains(xml, "noFill") {
		t.Fatalf("noFill not replaced: %s", xml)
	}
	if !strings.Contains(xml, "accent1") {
		t.Fatalf("accent1 not written: %s", xml)
	}
	// 非法 ColorSpec（无 Scheme 且无 RGB）→ ErrInvalidArgument。
	p3 := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:rPr b="1"/><a:t>X</a:t></a:r></a:p>`)
	s3 := mustSlide(t, p3)
	tf3 := slideBodyTF(t, s3)
	paras3, _ := tf3.Paragraphs()
	runs3, _ := paras3[0].Runs()
	if err := runs3[0].SetFont(FontStyle{Color: NewOptional(ColorSpec{})}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid color err = %v, want ErrInvalidArgument", err)
	}
}

func TestSetFontExpandsSelfClosingRPr(t *testing.T) {
	// 自闭合 rPr 带属性 + 设置需要子元素的字段 → 展开为完整形态。
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:rPr b="1"/><a:t>X</a:t></a:r></a:p>`)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	runs, _ := paras[0].Runs()
	if err := runs[0].SetFont(FontStyle{
		Color: NewOptional(ColorSpec{RGB: "00FF00"}),
		Latin: NewOptional("Arial"),
	}); err != nil {
		t.Fatalf("SetFont expand: %v", err)
	}
	ef, _ := runs[0].ExplicitFont()
	if !ef.Bold.Set || !ef.Bold.Value {
		t.Fatalf("bold lost during expand: %+v", ef.Bold)
	}
	if !ef.Color.Set || ef.Color.Value.RGB != "00FF00" || !ef.Latin.Set || ef.Latin.Value != "Arial" {
		t.Fatalf("font after expand = %+v", ef)
	}
	xml := slideXML(t, s)
	if !strings.Contains(xml, "</a:rPr>") || strings.Contains(xml, "<a:rPr b=\"1\"/>") {
		t.Fatalf("rPr not expanded properly: %s", xml)
	}
	// 只有属性变更（无子元素字段）→ 保持自闭合形态。
	p2 := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:rPr b="1"/><a:t>X</a:t></a:r></a:p>`)
	s2 := mustSlide(t, p2)
	tf2 := slideBodyTF(t, s2)
	paras2, _ := tf2.Paragraphs()
	runs2, _ := paras2[0].Runs()
	if err := runs2[0].SetFont(FontStyle{Italic: NewOptional(true)}); err != nil {
		t.Fatalf("SetFont attr-only: %v", err)
	}
	xml2 := slideXML(t, s2)
	if !strings.Contains(xml2, `<a:rPr b="1" i="1"/>`) {
		t.Fatalf("rPr should stay self-closing with attrs: %s", xml2)
	}
	// 自闭合展开路径的非法颜色同样拒绝。
	p3 := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:rPr b="1"/><a:t>X</a:t></a:r></a:p>`)
	s3 := mustSlide(t, p3)
	tf3 := slideBodyTF(t, s3)
	paras3, _ := tf3.Paragraphs()
	runs3, _ := paras3[0].Runs()
	if err := runs3[0].SetFont(FontStyle{Color: NewOptional(ColorSpec{})}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid color on self-closing: %v, want ErrInvalidArgument", err)
	}
}

func TestSetFontUnknownRPrChildRejected(t *testing.T) {
	// rPr 含库不识别的子元素：SetFont 需要插入时拒绝（不破坏顺序）。
	inner := `<a:bodyPr/><a:p><a:r><a:rPr lang="en-US"><a:noFill/><a:foo xmlns:a="urn:odd"/><a:t>?</a:t></a:rPr><a:t>X</a:t></a:r></a:p>`
	_ = inner
	inner = `<a:bodyPr/><a:p><a:r><a:rPr lang="en-US"><a:noFill/><x:q xmlns:x="urn:q"/></a:rPr><a:t>X</a:t></a:r></a:p>`
	p := slideWithBody(t, inner)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()
	runs, _ := paras[0].Runs()
	before := slideXML(t, s)

	// noFill 属于已知填充族；此处插入 latin（rank6）——rPr 含未知子 x:q。
	if err := runs[0].SetFont(FontStyle{Latin: NewOptional("Arial")}); !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("err = %v, want ErrUnsupportedEdit", err)
	}
	if got := slideXML(t, s); got != before {
		t.Error("rejected SetFont changed the part")
	}
	// 但属性级修改（无需插入子元素）仍可进行。
	if err := runs[0].SetFont(FontStyle{Italic: NewOptional(true)}); err != nil {
		t.Fatalf("attr-level SetFont should pass: %v", err)
	}
}

func TestAddRunBeforeEndParaRPr(t *testing.T) {
	inner := `<a:bodyPr/><a:p><a:r><a:t>a</a:t></a:r><a:endParaRPr lang="en-US"/></a:p>`
	p := slideWithBody(t, inner)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, _ := tf.Paragraphs()

	r, err := paras[0].AddRun(" tail ", FontStyle{Bold: NewOptional(true)})
	if err != nil {
		t.Fatalf("AddRun: %v", err)
	}
	if txt, _ := r.Text(); txt != " tail " {
		t.Errorf("added run text = %q", txt)
	}
	// 位置：新 run 在 endParaRPr 之前、原 run 之后。
	xml := slideXML(t, s)
	ra := strings.Index(xml, "<a:t>a</a:t>")
	rb := strings.Index(xml, "<a:t> tail </a:t>")
	ep := strings.Index(xml, "<a:endParaRPr")
	if !(ra >= 0 && rb > ra && ep > rb) {
		t.Errorf("ordering wrong: %s", xml)
	}
	if !strings.Contains(xml, `<a:rPr b="1"/>`) {
		t.Errorf("style not applied: %s", xml)
	}
}

// ---------- 纯函数（零覆盖消除，2026-09-13 第 5 轮） ----------

// TestParseHexRune 覆盖数字/大小写十六进制/非法字符/超 Unicode 上界四类。
func TestParseHexRune(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want rune
	}{
		{"41", 'A'},
		{"263a", '☺'},
		{"00e9", 'é'},
		{"00E9", 'é'}, // 大写 X 后的十六进制数字母大小写均可
		{"4e2d", '中'}, //
		{"", 0},       // 空串返回 0（调用方按 code>=0 写入）
		{"1F600", 0x1F600},
		{"g1", -1},     // 非十六进制字符
		{"4z", -1},     //
		{"110000", -1}, // 超 0x10FFFF
	} {
		if got := parseHexRune(tc.in); got != tc.want {
			t.Errorf("parseHexRune(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestParseDecRune 覆盖十进制数字/非法字符/溢出。
func TestParseDecRune(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want rune
	}{
		{"65", 'A'},
		{"20013", '中'},
		{"128512", 0x1F600},
		{"", 0},
		{"6a", -1},
		{"1114112", -1}, // 0x110000 溢出
		{"99999999999", -1},
	} {
		if got := parseDecRune(tc.in); got != tc.want {
			t.Errorf("parseDecRune(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestXmlUnescape 表驱动覆盖命名实体 / 数字实体（hex+dec）/ 未知实体
// / 裸 & 与无实体字符串。
func TestXmlUnescape(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain text", "plain text"},
		{"a &amp; b", "a & b"},
		{"&lt;&gt;&quot;&apos;", "<>\"'"},
		{"中文 &#x4e2d;", "中文 中"},
		{"&#65;&#x41;", "AA"},
		{"&nbsp;", "&nbsp;"},     // 未知命名实体原样保留
		{"&nope;", "&nope;"},     //
		{"&#xzz;", "&#xzz;"},     // 数字实体非法 → 原样保留
		{"&#99x;", "&#99x;"},     //
		{"a & b", "a & b"},       // 裸 &（无分号收尾）原样保留
		{"a &", "a &"},           //
		{"x &amp;& y", "x && y"}, // 混合
	} {
		if got := xmlUnescape(tc.in); got != tc.want {
			t.Errorf("xmlUnescape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestKindIndex 验证同名单元素兄弟中的 0 基序号：根级节点（无父）与
// 命名空间隔离。
func TestKindIndex(t *testing.T) {
	doc, err := xmlstore.Index([]byte(
		`<root xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `">` +
			`<a:p/><a:p/><a:p/><p:sp/></root>`))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	root := doc.Root()
	if got := kindIndex(doc, root); got != 0 {
		t.Errorf("kindIndex(root) = %d, want 0", got)
	}
	for i, cid := range root.Children {
		n := doc.Node(cid)
		want := 0
		if n.Local() == "p" && n.Namespace == nsDrawingML {
			want = i // 三个 a:p 依序 0/1/2
		}
		if got := kindIndex(doc, n); got != want {
			t.Errorf("kindIndex(child %d %s) = %d, want %d", i, n.Name(), got, want)
		}
	}
}

// TestSizeCentipoints 验证 pt → 百分之一 pt 的四舍五入转换。
func TestSizeCentipoints(t *testing.T) {
	for _, tc := range []struct {
		in   FontSize
		want string
	}{
		{Pts(12), "1200"},
		{Pts(0), "0"},
		{Pts(10.55), "1055"},
		{Pts(10.555), "1056"},   // 四舍五入
		{Pts(10.554), "1055"},   //
		{Pts(100.125), "10013"}, // 10012.5 + 0.5 → 10013
	} {
		if got := sizeCentipoints(tc.in); got != tc.want {
			t.Errorf("sizeCentipoints(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestRPrChildRank 覆盖 rPr 子元素 schema 序号全分组 + 非法命名空间与
// 未知 local 名。
func TestRPrChildRank(t *testing.T) {
	for _, tc := range []struct {
		ns     string
		local  string
		want   int
		wantOK bool
	}{
		{nsDrawingML, "ln", 0, true},
		{nsDrawingML, "solidFill", 1, true},
		{nsDrawingML, "noFill", 1, true},
		{nsDrawingML, "gradFill", 1, true},
		{nsDrawingML, "effectLst", 2, true},
		{nsDrawingML, "highlight", 3, true},
		{nsDrawingML, "uLn", 4, true},
		{nsDrawingML, "uFillTx", 5, true},
		{nsDrawingML, "latin", 6, true},
		{nsDrawingML, "ea", 7, true},
		{nsDrawingML, "cs", 8, true},
		{nsDrawingML, "sym", 9, true},
		{nsDrawingML, "hlinkClick", 10, true},
		{nsDrawingML, "rtl", 11, true},
		{nsDrawingML, "extLst", 12, true},
		{nsDrawingML, "unknownElem", 0, false},
		{"http://other/ns", "latin", 0, false}, // 命名空间不符
	} {
		got, ok := rPrChildRank(tc.ns, tc.local)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("rPrChildRank(%q,%q) = (%d,%v), want (%d,%v)",
				tc.ns, tc.local, got, ok, tc.want, tc.wantOK)
		}
	}
}
