package pptx

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// ---------- 页面 API（§20.1 / M2 页面 API 收口） ----------

// slideIDsOf 返回文档页面 ID 顺序。
func slideIDsOf(t *testing.T, p *Presentation) []SlideID {
	t.Helper()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	out := make([]SlideID, 0, len(slides))
	for _, s := range slides {
		out = append(out, s.ID())
	}
	return out
}

// deckSlidesWithRels 生成含 n 页的包字节；每页均带自身 .rels（指向
// slideLayout1），保证会话内备注补丁/删除路径可落盘（withSlides 的
// 简化夹具省略了页面关系流，不适合备注与删除场景）。
func deckSlidesWithRels(t *testing.T, n int) []byte {
	t.Helper()
	parts := minimalTemplateParts()
	layoutRels := opc.PartName("/ppt/slideLayouts/_rels/slideLayout1.xml.rels")
	if _, ok := parts[layoutRels]; !ok {
		parts[layoutRels] = []byte(xmlDecl + `<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="../slideMasters/slideMaster1.xml"/>` +
			`</Relationships>`)
	}
	var sldIDs strings.Builder
	relsParts := make(map[string]string)
	for i := 1; i <= n; i++ {
		slide := opc.PartName("/ppt/slides/slide" + itoa(i) + ".xml")
		parts[slide] = []byte(xmlDecl +
			`<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
			`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
			`</p:sld>`)
		ct := `<Override PartName="` + string(slide) + `" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`
		parts["/[Content_Types].xml"] = append(
			bytes.TrimSuffix(parts["/[Content_Types].xml"], []byte("</Types>")),
			[]byte(ct+"</Types>")...)
		relsParts[string(relsPart(slide))] = xmlDecl +
			`<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
			`</Relationships>`
		sldIDs.WriteString(`<p:sldId id="` + itoa(255+i) + `" r:id="rIdS` + itoa(i) + `"/>`)
	}
	for name, content := range relsParts {
		parts[opc.PartName(name)] = []byte(content)
	}
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		slideRelEntries(n) + `</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst>` + sldIDs.String() + `</p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	return buildPackageZipPanic(parts)
}

// slideRelEntries 生成主 Part 指向各 slide 的关系条目。
func slideRelEntries(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(`<Relationship Id="rIdS` + itoa(i) + `" Type="` + opc.RelSlide + `" Target="slides/slide` + itoa(i) + `.xml"/>`)
	}
	return sb.String()
}

func itoa(v int) string { return strconv.Itoa(v) }

func TestSlideByIndex(t *testing.T) {
	p := openFixture(t, withSlides(t, 3))
	defer p.Close()
	if _, err := p.Slide(-1); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("Slide(-1) err = %v, want ErrOutOfRange", err)
	}
	if _, err := p.Slide(3); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("Slide(3) err = %v, want ErrOutOfRange", err)
	}
	s, err := p.Slide(1)
	if err != nil {
		t.Fatalf("Slide(1): %v", err)
	}
	if s.ID() != 257 {
		t.Errorf("Slide(1).ID = %d, want 257", s.ID())
	}
}

func TestLayoutsOnTemplate(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layouts, err := p.Layouts()
	if err != nil {
		t.Fatalf("Layouts: %v", err)
	}
	if len(layouts) != 1 {
		t.Fatalf("layouts = %d, want 1", len(layouts))
	}
	if got := layouts[0].Name(); got != "Blank" {
		t.Errorf("layout name = %q, want %q", got, "Blank")
	}
}

func TestAddSlideGrowsDocumentAndPersists(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	layouts, err := p.Layouts()
	if err != nil || len(layouts) != 1 {
		t.Fatalf("Layouts: %d, %v", len(layouts), err)
	}
	lay := layouts[0]

	s1, err := p.AddSlide(lay)
	if err != nil {
		t.Fatalf("AddSlide #1: %v", err)
	}
	if s1.ID() != 256 {
		t.Errorf("first slide id = %d, want 256", s1.ID())
	}
	// 新页为空 spTree：无形状、无占位符。
	if shapes, err := s1.Shapes(); err != nil || len(shapes) != 0 {
		t.Errorf("new slide Shapes = %d, %v; want 0", len(shapes), err)
	}
	if phs, err := s1.Placeholders(); err != nil || len(phs) != 0 {
		t.Errorf("new slide Placeholders = %d, %v; want 0", len(phs), err)
	}
	s2, err := p.AddSlide(lay)
	if err != nil {
		t.Fatalf("AddSlide #2: %v", err)
	}
	if s2.ID() != 257 {
		t.Errorf("second slide id = %d, want 257", s2.ID())
	}
	if got := slideIDsOf(t, p); len(got) != 2 || got[0] != 256 || got[1] != 257 {
		t.Errorf("slide order = %v, want [256 257]", got)
	}

	// 保存往返：页面数/顺序/Part 均保持；新增 slide 与 rels Part 存在。
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if rep := p.Validate(context.Background()); rep.HasErrors() {
		t.Errorf("Validate before close: %+v", rep.Diagnostics)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	p2 := openFixture(t, buf.Bytes())
	got := slideIDsOf(t, p2)
	if len(got) != 2 || got[0] != 256 || got[1] != 257 {
		t.Fatalf("reopened order = %v, want [256 257]", got)
	}
	for _, part := range []string{
		"/ppt/slides/slide1.xml", "/ppt/slides/slide2.xml",
		"/ppt/slides/_rels/slide1.xml.rels", "/ppt/slides/_rels/slide2.xml.rels",
	} {
		if !p2.pk.HasPart(opc.PartName(part)) {
			t.Errorf("reopened missing %s", part)
		}
	}
	if rep := p2.Validate(context.Background()); rep.HasErrors() {
		t.Errorf("Validate after reopen: %+v", rep.Diagnostics)
	}
}

func TestAddSlideErrors(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatalf("New a: %v", err)
	}
	defer a.Close()
	b, err := New()
	if err != nil {
		t.Fatalf("New b: %v", err)
	}
	defer b.Close()

	layouts, _ := a.Layouts()
	layA := layouts[0]

	// nil layout。
	if _, err := b.AddSlide(nil); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("AddSlide(nil) err = %v, want ErrInvalidArgument", err)
	}
	// 跨文档版式。
	if _, err := b.AddSlide(layA); !errors.Is(err, ErrForeignReference) {
		t.Errorf("AddSlide(foreign) err = %v, want ErrForeignReference", err)
	}
	// 已关闭文档。
	layoutsB, _ := b.Layouts()
	if err := b.Close(); err != nil {
		t.Fatalf("close b: %v", err)
	}
	if _, err := b.AddSlide(layoutsB[0]); !errors.Is(err, ErrClosed) {
		t.Errorf("AddSlide on closed err = %v, want ErrClosed", err)
	}
}

func TestMoveSlideFinalPositionSemantics(t *testing.T) {
	p := openFixture(t, withSlides(t, 3))
	defer p.Close()
	// [256,257,258] → 移动 256 到最终位置 2：后移 → [257,258,256]。
	if err := p.MoveSlide(256, 2); err != nil {
		t.Fatalf("MoveSlide(256,2): %v", err)
	}
	if got := slideIDsOf(t, p); len(got) != 3 || got[0] != 257 || got[1] != 258 || got[2] != 256 {
		t.Fatalf("order after back-move = %v, want [257 258 256]", got)
	}
	// [257,258,256] → 移动 258 到最终位置 0：前移 → [258,257,256]。
	if err := p.MoveSlide(258, 0); err != nil {
		t.Fatalf("MoveSlide(258,0): %v", err)
	}
	if got := slideIDsOf(t, p); got[0] != 258 || got[1] != 257 || got[2] != 256 {
		t.Fatalf("order after front-move = %v, want [258 257 256]", got)
	}
	// 移到当前位 no-op（revision 不变）。
	rev := p.Revision()
	if err := p.MoveSlide(258, 0); err != nil {
		t.Fatalf("MoveSlide no-op: %v", err)
	}
	if p.Revision() != rev {
		t.Errorf("no-op move changed revision %d → %d", rev, p.Revision())
	}
	// 错误路径。
	if err := p.MoveSlide(999, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("MoveSlide(999) err = %v, want ErrNotFound", err)
	}
	if err := p.MoveSlide(258, 3); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("MoveSlide index 3 err = %v, want ErrOutOfRange", err)
	}
	if err := p.MoveSlide(258, -1); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("MoveSlide index -1 err = %v, want ErrOutOfRange", err)
	}
}

func TestRemoveSlideMiddleAndPersist(t *testing.T) {
	p := openFixture(t, withSlides(t, 3))
	defer p.Close()
	s2 := mustSlideAt(t, p, 1)
	if err := p.RemoveSlide(257); err != nil {
		t.Fatalf("RemoveSlide(257): %v", err)
	}
	if got := slideIDsOf(t, p); len(got) != 2 || got[0] != 256 || got[1] != 258 {
		t.Fatalf("order after remove = %v, want [256 258]", got)
	}
	// 句柄失效。
	if _, err := s2.SpeakerNotesText(); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("removed slide handle err = %v, want ErrStaleHandle", err)
	}
	// 删除再次执行 → ErrNotFound。
	if err := p.RemoveSlide(257); !errors.Is(err, ErrNotFound) {
		t.Errorf("second RemoveSlide err = %v, want ErrNotFound", err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p2 := openFixture(t, buf.Bytes())
	if got := slideIDsOf(t, p2); len(got) != 2 {
		t.Fatalf("reopened slides = %v", got)
	}
	if p2.pk.HasPart(opc.PartName("/ppt/slides/slide2.xml")) {
		t.Error("removed slide part still present after save")
	}
	if !p2.pk.HasPart(opc.PartName("/ppt/slides/slide1.xml")) ||
		!p2.pk.HasPart(opc.PartName("/ppt/slides/slide3.xml")) {
		t.Error("retained slide parts missing after save")
	}
	if rep := p2.Validate(context.Background()); rep.HasErrors() {
		t.Errorf("Validate after reopen: %+v", rep.Diagnostics)
	}
}

// TestRemoveSlideCascadesNotes 删除带备注的页面：notesSlide 连带删除，
// notesMaster 保留（共享母版资源不删）。
func TestRemoveSlideCascadesNotes(t *testing.T) {
	p := openFixture(t, deckSlidesWithRels(t, 1))
	defer p.Close()
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	notes, ok := s.notesPartOf()
	if ok {
		t.Fatal("unexpected initial notes part")
	}
	tf, err := s.EnsureSpeakerNotes()
	if err != nil {
		t.Fatalf("EnsureSpeakerNotes: %v", err)
	}
	if err := tf.SetPlainText("讲稿"); err != nil {
		t.Fatalf("SetPlainText: %v", err)
	}
	notes, ok = s.notesPartOf()
	if !ok || notes == "" {
		t.Fatalf("notes part missing after ensure")
	}
	if _, err := p.partBytes(notes); err != nil {
		t.Fatalf("notes part readable: %v", err)
	}

	if err := p.RemoveSlide(s.ID()); err != nil {
		t.Fatalf("RemoveSlide: %v", err)
	}
	if _, err := p.partBytes(notes); !errors.Is(err, ErrNotFound) {
		t.Errorf("notes part after remove err = %v, want ErrNotFound", err)
	}
	// notesMaster 保留。
	if _, err := p.partBytes(opc.PartName("/ppt/notesMasters/notesMaster1.xml")); err != nil {
		t.Errorf("notesMaster should be retained: %v", err)
	}
	if got := slideIDsOf(t, p); len(got) != 0 {
		t.Fatalf("slides after remove = %v, want empty", got)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p2 := openFixture(t, buf.Bytes())
	if p2.pk.HasPart(notes) {
		t.Error("notesSlide part still present after save")
	}
	if !p2.pk.HasPart(opc.PartName("/ppt/notesMasters/notesMaster1.xml")) {
		t.Error("notesMaster missing after save")
	}
	if rep := p2.Validate(context.Background()); rep.HasErrors() {
		t.Errorf("Validate after reopen: %+v", rep.Diagnostics)
	}
}

// TestRemoveSlideUnknownReferenceBlocked 未知 Part 引用 slide → 拒绝删除。
func TestRemoveSlideUnknownReferenceBlocked(t *testing.T) {
	p := openFixture(t, withSlides(t, 1))
	defer p.Close()
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	// 会话内新增一个"未知"XML Part，其关系流指向 slide（模拟未知消费方）。
	foreign := opc.PartName("/ppt/foreign1.xml")
	if err := p.stageAdd(foreign, []byte(xmlDecl+`<a:doc xmlns:a="`+nsDrawingML+`"/>`), ""); err != nil {
		t.Fatalf("stageAdd foreign: %v", err)
	}
	foreignRels := relsPart(foreign)
	sn, err := s.partName()
	if err != nil {
		t.Fatalf("slide partName: %v", err)
	}
	rel := `<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlide + `" Target="slides/` + slideName(sn) + `"/>` +
		`</Relationships>`
	if err := p.stageAdd(foreignRels, []byte(xmlDecl+rel), ""); err != nil {
		t.Fatalf("stageAdd foreign rels: %v", err)
	}
	p.commit()

	rev := p.Revision()
	if err := p.RemoveSlide(s.ID()); !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("RemoveSlide err = %v, want ErrUnsupportedEdit", err)
	}
	if p.Revision() != rev {
		t.Error("blocked RemoveSlide still changed revision")
	}
	if got := slideIDsOf(t, p); len(got) != 1 {
		t.Errorf("slide removed despite unknown dependency: %v", got)
	}
}

func mustSlideAt(t *testing.T, p *Presentation, idx int) *Slide {
	t.Helper()
	s, err := p.Slide(idx)
	if err != nil {
		t.Fatalf("Slide(%d): %v", idx, err)
	}
	return s
}
