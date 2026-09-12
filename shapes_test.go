package pptx

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// ---------- Shapes/Placeholders 与 §8.1（M2 页面 API + AltText AutoShape） ----------

// shapeSlideBody 是形状枚举测试页面：8 个顶层形状（z-order 即文档序）。
//   - sp title 占位符（descr="标题 alt"）
//   - sp body idx=1 占位符
//   - sp 文本框 txBox=1（descr="框 alt"）
//   - sp 自选图形（无 txBody）
//   - p:pic（descr="图 alt"）
//   - p:grpSp 组合（descr="组 alt"，内含子 sp——不计入顶层）
//   - sp 装饰性图形 decorative="1"
//   - sp 占位符（仅 idx=2，无 type → 规范化 "obj"）
const shapeSlideBody = `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:cSld><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1" descr="标题 alt"/><p:cNvSpPr/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Slide Title</a:t></a:r></a:p></p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Body 1"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Body text</a:t></a:r></a:p></p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="4" name="TextBox 1" descr="框 alt"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Hello Box</a:t></a:r></a:p></p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="5" name="Oval 1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr><a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom></p:spPr></p:sp>` +
	`<p:pic><p:nvPicPr><p:cNvPr id="6" name="Picture 1" descr="图 alt"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr>` +
	`<p:blipFill/><p:spPr/></p:pic>` +
	`<p:grpSp><p:nvGrpSpPr><p:cNvPr id="7" name="Group 1" descr="组 alt"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="70" name="Inner 1"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Inner text</a:t></a:r></a:p></p:txBody></p:sp>` +
	`</p:grpSp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="8" name="Deco 1" decorative="1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Deco text</a:t></a:r></a:p></p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="9" name="Obj Ph 1"/><p:cNvSpPr/><p:nvPr><p:ph idx="2"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Obj text</a:t></a:r></a:p></p:txBody></p:sp>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
	`</p:sld>`

// shapeDeckFixture 构造含 shapeSlideBody 的单页文档。
func shapeDeckFixture(t *testing.T) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl + shapeSlideBody)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
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
	return openFixture(t, buildPackageZipPanic(parts))
}

// shapeByID 在 Shapes 中按 cNvPr@id 找句柄。
func shapeByID(t *testing.T, shapes []Shape, id uint32) Shape {
	t.Helper()
	for _, sh := range shapes {
		if uint32(sh.ID()) == id {
			return sh
		}
	}
	t.Fatalf("shape id %d not found in %d shapes", id, len(shapes))
	return nil
}

func TestShapesEnumerationOrderAndKinds(t *testing.T) {
	p := shapeDeckFixture(t)
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	if len(shapes) != 8 {
		t.Fatalf("shapes = %d, want 8 (nvGrpSpPr/grpSpPr 应被跳过，组内子形状不计入)", len(shapes))
	}
	wantKinds := []ShapeKind{
		ShapeAutoShape, ShapeAutoShape, ShapeTextBox, ShapeAutoShape,
		ShapePicture, ShapeGroup, ShapeAutoShape, ShapeAutoShape,
	}
	wantIDs := []uint32{2, 3, 4, 5, 6, 7, 8, 9}
	for i, sh := range shapes {
		if sh.Kind() != wantKinds[i] {
			t.Errorf("shapes[%d] kind = %v, want %v", i, sh.Kind(), wantKinds[i])
		}
		if uint32(sh.ID()) != wantIDs[i] {
			t.Errorf("shapes[%d] id = %d, want %d", i, sh.ID(), wantIDs[i])
		}
	}
	// 名称与只读元信息。
	if shapes[0].Name() != "Title 1" || shapes[6].Name() != "Deco 1" {
		t.Errorf("shape names wrong: %q, %q", shapes[0].Name(), shapes[6].Name())
	}
	// 组合的子形状不进入顶层（id 70 不可见）。
	for _, sh := range shapes {
		if uint32(sh.ID()) == 70 {
			t.Error("nested group child leaked into top-level Shapes")
		}
	}
}

func TestShapeAltTextSemantics(t *testing.T) {
	p := shapeDeckFixture(t)
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	title := shapeByID(t, shapes, 2)
	if title.AltText() != "标题 alt" || title.IsDecorative() {
		t.Errorf("title alt = %q dec=%v", title.AltText(), title.IsDecorative())
	}
	body := shapeByID(t, shapes, 3)
	if body.AltText() != "" || body.IsDecorative() {
		t.Errorf("undeclared body alt = %q dec=%v (读取不臆测)", body.AltText(), body.IsDecorative())
	}
	// 组合/图形框等只读句柄同样可读 AltText。
	grp := shapeByID(t, shapes, 7)
	if grp.AltText() != "组 alt" {
		t.Errorf("group alt = %q, want 组 alt", grp.AltText())
	}

	// §8.1 写入：SetAltText 清除装饰标记并写 @descr。
	deco := shapeByID(t, shapes, 8).(*AutoShape)
	if !deco.IsDecorative() || deco.AltText() != "" {
		t.Fatalf("deco initial dec=%v alt=%q", deco.IsDecorative(), deco.AltText())
	}
	if err := deco.SetAltText("说明文字"); err != nil {
		t.Fatalf("SetAltText: %v", err)
	}
	if deco.IsDecorative() || deco.AltText() != "说明文字" {
		t.Errorf("after SetAltText dec=%v alt=%q", deco.IsDecorative(), deco.AltText())
	}
	// SetDecorative(true) 清除替代文本、写装饰标记（语义不可互相替代）。
	if err := deco.SetDecorative(true); err != nil {
		t.Fatalf("SetDecorative(true): %v", err)
	}
	if !deco.IsDecorative() || deco.AltText() != "" {
		t.Errorf("after decorative dec=%v alt=%q", deco.IsDecorative(), deco.AltText())
	}
	// SetDecorative(false) 只清装饰标记。
	if err := deco.SetDecorative(false); err != nil {
		t.Fatalf("SetDecorative(false): %v", err)
	}
	if deco.IsDecorative() {
		t.Error("deco still decorative after SetDecorative(false)")
	}
	// 空串清除 @descr（不写 decorative）。
	if err := deco.SetAltText("有"); err != nil {
		t.Fatalf("SetAltText(有): %v", err)
	}
	if err := deco.SetAltText(""); err != nil {
		t.Fatalf("SetAltText(空): %v", err)
	}
	if deco.AltText() != "" || deco.IsDecorative() {
		t.Errorf("after empty clear alt=%q dec=%v", deco.AltText(), deco.IsDecorative())
	}

	// PictureShape §8.1 写路径（IMAGE-01 语义经共用基元保持）。
	pic := shapeByID(t, shapes, 6).(*PictureShape)
	if pic.AltText() != "图 alt" || pic.Kind() != ShapePicture {
		t.Fatalf("pic initial alt=%q kind=%v", pic.AltText(), pic.Kind())
	}
	if err := pic.SetAltText("新图说明"); err != nil {
		t.Fatalf("pic.SetAltText: %v", err)
	}
	if pic.AltText() != "新图说明" {
		t.Errorf("pic alt after set = %q", pic.AltText())
	}
}

func TestShapeAltTextPersistsRoundTrip(t *testing.T) {
	p := shapeDeckFixture(t)
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	tb := shapeByID(t, shapes, 4).(*AutoShape)
	if err := tb.SetAltText("持久化 alt"); err != nil {
		t.Fatalf("SetAltText: %v", err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p2 := openFixture(t, buf.Bytes())
	s2, err := p2.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0) reopened: %v", err)
	}
	shapes2, err := s2.Shapes()
	if err != nil {
		t.Fatalf("Shapes reopened: %v", err)
	}
	if got := shapeByID(t, shapes2, 4).AltText(); got != "持久化 alt" {
		t.Errorf("reopened alt = %q", got)
	}
}

func TestAutoShapeTextFrames(t *testing.T) {
	p := shapeDeckFixture(t)
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	// 文本框/自选图形经 TextFrame 读写正文。
	tb := shapeByID(t, shapes, 4).(*AutoShape)
	if tb.Kind() != ShapeTextBox {
		t.Fatalf("textbox kind = %v", tb.Kind())
	}
	tf, err := tb.TextFrame()
	if err != nil {
		t.Fatalf("TextFrame: %v", err)
	}
	if txt := tfText(t, tf); txt != "Hello Box" {
		t.Fatalf("initial text = %q", txt)
	}
	if err := tf.SetPlainText("新内容 Box"); err != nil {
		t.Fatalf("SetPlainText: %v", err)
	}
	if txt := tfText(t, tf); txt != "新内容 Box" {
		t.Errorf("text after set = %q", txt)
	}
	// 无正文的自选图形 → ErrNotFound。
	oval := shapeByID(t, shapes, 5).(*AutoShape)
	if _, err := oval.TextFrame(); !errors.Is(err, ErrNotFound) {
		t.Errorf("oval TextFrame err = %v, want ErrNotFound", err)
	}
}

func TestSlidePlaceholdersAndReadingNotes(t *testing.T) {
	p := shapeDeckFixture(t)
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	phs, err := s.Placeholders()
	if err != nil {
		t.Fatalf("Placeholders: %v", err)
	}
	if len(phs) != 3 {
		t.Fatalf("placeholders = %d, want 3", len(phs))
	}
	type wantPh struct {
		typ  string
		idx  uint32
		id   uint32
		text string
	}
	wants := []wantPh{
		{"title", 0, 2, "Slide Title"},
		{"body", 1, 3, "Body text"},
		{"obj", 2, 9, "Obj text"},
	}
	for i, w := range wants {
		ph := phs[i]
		if ph.Type != w.typ || ph.Index != w.idx {
			t.Errorf("placeholder[%d] = %s/%d, want %s/%d", i, ph.Type, ph.Index, w.typ, w.idx)
		}
		sh := ph.Shape
		if uint32(sh.ID()) != w.id {
			t.Errorf("placeholder[%d] shape id = %d, want %d", i, sh.ID(), w.id)
		}
		// 讲稿读取闭环：占位符形状（*AutoShape）→ TextFrame → Text。
		as, ok := sh.(*AutoShape)
		if !ok {
			t.Fatalf("placeholder[%d] shape type = %T", i, sh)
		}
		tf, err := as.TextFrame()
		if err != nil {
			t.Fatalf("placeholder[%d] TextFrame: %v", i, err)
		}
		if txt := tfText(t, tf); txt != w.text {
			t.Errorf("placeholder[%d] text = %q, want %q", i, txt, w.text)
		}
	}
}

func TestShapeHandleLifecycle(t *testing.T) {
	p := shapeDeckFixture(t)
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	as := shapeByID(t, shapes, 4).(*AutoShape)

	// 删除页面 → 句柄失效（写操作报 ErrStaleHandle）。
	if err := p.RemoveSlide(s.ID()); err != nil {
		t.Fatalf("RemoveSlide: %v", err)
	}
	if err := as.SetAltText("x"); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("stale SetAltText err = %v, want ErrStaleHandle", err)
	}
	if _, err := as.TextFrame(); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("stale TextFrame err = %v, want ErrStaleHandle", err)
	}
	p2 := shapeDeckFixture(t)
	s2, err := p2.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes2, _ := s2.Shapes()
	as2 := shapeByID(t, shapes2, 4).(*AutoShape)
	if err := p2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := as2.SetAltText("x"); !errors.Is(err, ErrClosed) {
		t.Errorf("closed SetAltText err = %v, want ErrClosed", err)
	}
	if _, err := s2.Shapes(); !errors.Is(err, ErrClosed) {
		t.Errorf("closed Shapes err = %v, want ErrClosed", err)
	}
}

func TestShapeNodePathAndPlaceholder(t *testing.T) {
	p := shapeDeckFixture(t)
	defer p.Close()
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	// 顶层形状 NodePath 以 spTree 为祖先。
	for _, sh := range shapes {
		np := sh.NodePath()
		if !strings.HasPrefix(np, "p:cSld[0]/p:spTree[0]/") {
			t.Errorf("id=%d NodePath = %q, want spTree ancestor", sh.ID(), np)
		}
	}
	// 占位符规范化：type=title → title/0；仅 idx=2 无 type → obj/2。
	title := shapeByID(t, shapes, 2).(*AutoShape)
	if typ, idx, ok := title.Placeholder(); !ok || typ != "title" || idx != 0 {
		t.Errorf("title placeholder = %q/%d/%v, want title/0/true", typ, idx, ok)
	}
	obj := shapeByID(t, shapes, 9).(*AutoShape)
	if typ, idx, ok := obj.Placeholder(); !ok || typ != "obj" || idx != 2 {
		t.Errorf("obj placeholder = %q/%d/%v, want obj/2/true", typ, idx, ok)
	}
	// 非占位符 TextBox → ok=false。
	box := shapeByID(t, shapes, 4).(*AutoShape)
	if typ, idx, ok := box.Placeholder(); ok {
		t.Errorf("textbox placeholder = %q/%d/%v, want not ok", typ, idx, ok)
	}
}

// tfText 返回 TextFrame 全文本（段落以 '\n' 连接；测试便捷读取）。
func tfText(t *testing.T, tf *TextFrame) string {
	t.Helper()
	paras, err := tf.Paragraphs()
	if err != nil {
		t.Fatalf("Paragraphs: %v", err)
	}
	lines := make([]string, 0, len(paras))
	for _, p := range paras {
		txt, err := p.Text()
		if err != nil {
			t.Fatalf("Paragraph.Text: %v", err)
		}
		lines = append(lines, txt)
	}
	return strings.Join(lines, "\n")
}

// ---------- OpaqueShape 触发三类 p:cxnSp / p:graphicFrame / 未知元素（shape.go:828）----------

// opaqueDeck 构造单页文档，slide1.xml = body（XML 字面量）。
func opaqueDeck(t *testing.T, body string) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl + body)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
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
	return openFixture(t, buildPackageZipPanic(parts))
}

func TestOpaqueShapeKinds(t *testing.T) {
	// 三个独立子测试触发 OpaqueShape 的三条构造路径（shape.go:651/661/663），
	// 全部走到 *OpaqueShape.Kind()（shape.go:828）。原 0% 覆盖率补齐。
	t.Run("cxnSp_yields_ShapeConnector", func(t *testing.T) {
		body := `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `">` +
			`<p:cSld><p:spTree>` +
			`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
			`<p:grpSpPr/>` +
			`<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="2" name="Connector 1"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr>` +
			`<p:spPr/>` +
			`</p:cxnSp>` +
			`</p:spTree></p:cSld>` +
			`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
			`</p:sld>`
		p := opaqueDeck(t, body)
		defer p.Close()
		s, err := p.Slide(0)
		if err != nil {
			t.Fatalf("Slide(0): %v", err)
		}
		shapes, err := s.Shapes()
		if err != nil {
			t.Fatalf("Shapes: %v", err)
		}
		if len(shapes) != 1 {
			t.Fatalf("shapes = %d, want 1", len(shapes))
		}
		op, ok := shapes[0].(*OpaqueShape)
		if !ok {
			t.Fatalf("shapes[0] type = %T, want *OpaqueShape", shapes[0])
		}
		if op.Kind() != ShapeConnector {
			t.Errorf("Kind = %v, want ShapeConnector", op.Kind())
		}
		if op.Name() != "Connector 1" {
			t.Errorf("Name = %q", op.Name())
		}
	})
	t.Run("graphicFrame_no_table_chart_yields_ShapeGraphicFrame", func(t *testing.T) {
		// 内含 a:graphic/a:graphicData 但既非 a:tbl 也非 c:chart —— 归 ShapeGraphicFrame。
		body := `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `">` +
			`<p:cSld><p:spTree>` +
			`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
			`<p:grpSpPr/>` +
			`<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="2" name="Frame 1"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
			`<p:xfrm><a:off x="0" y="0"/><a:ext cx="1000" cy="1000"/></p:xfrm>` +
			`<a:graphic><a:graphicData uri="urn:unknown"/></a:graphic>` +
			`</p:graphicFrame>` +
			`</p:spTree></p:cSld>` +
			`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
			`</p:sld>`
		p := opaqueDeck(t, body)
		defer p.Close()
		s, err := p.Slide(0)
		if err != nil {
			t.Fatalf("Slide(0): %v", err)
		}
		shapes, err := s.Shapes()
		if err != nil {
			t.Fatalf("Shapes: %v", err)
		}
		op, ok := shapes[0].(*OpaqueShape)
		if !ok {
			t.Fatalf("shapes[0] type = %T, want *OpaqueShape", shapes[0])
		}
		if op.Kind() != ShapeGraphicFrame {
			t.Errorf("Kind = %v, want ShapeGraphicFrame", op.Kind())
		}
	})
	t.Run("unknown_local_yields_ShapeOpaque", func(t *testing.T) {
		// <p:note/> 不在 spTree 元素识别白名单（sp/pic/grpSp/cxnSp/graphicFrame）
		// 之内 → 兜底 ShapeOpaque。
		body := `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `">` +
			`<p:cSld><p:spTree>` +
			`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
			`<p:grpSpPr/>` +
			`<p:note><p:cNvPr id="2" name="Note 1"/><p:cNvNotePr/></p:note>` +
			`</p:spTree></p:cSld>` +
			`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
			`</p:sld>`
		p := opaqueDeck(t, body)
		defer p.Close()
		s, err := p.Slide(0)
		if err != nil {
			t.Fatalf("Slide(0): %v", err)
		}
		shapes, err := s.Shapes()
		if err != nil {
			t.Fatalf("Shapes: %v", err)
		}
		op, ok := shapes[0].(*OpaqueShape)
		if !ok {
			t.Fatalf("shapes[0] type = %T, want *OpaqueShape", shapes[0])
		}
		if op.Kind() != ShapeOpaque {
			t.Errorf("Kind = %v, want ShapeOpaque", op.Kind())
		}
	})
}
