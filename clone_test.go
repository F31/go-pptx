package pptx

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/opc"
)

// 本文件覆盖 CLONE-01 验收：同文档受限复制的结构正确性、图表数据隔离
// 金样、媒体共享/独立策略、未知关系拒绝（AT-13 同文档口径）、TrackKey
// 派生唯一化与保存往返。

// cloneDeckWithRels 构造带一条 slide 的最小模板，slide 关系流可注入
// 额外 Relationship 条目（测未知关系拒绝）。
func cloneDeckWithRels(t *testing.T, extraRels string) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	const nsP = nsPresentationML
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`</p:sld>`)
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rIdS1" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		extraRels +
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rIdS1"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	parts["/[Content_Types].xml"] = append(parts["/[Content_Types].xml"][:len(parts["/[Content_Types].xml"])-len("</Types>")],
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="`+ctSlide+`"/></Types>`)...)
	zipBytes, err := buildPackageZip(parts)
	if err != nil {
		t.Fatalf("buildPackageZip: %v", err)
	}
	p, err := OpenReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	return p
}

// cloneDeckWithAudio 构造含一条 WAV 音轨的页面（默认共享媒体用例基座）。
func cloneDeckWithAudio(t *testing.T) (*Presentation, *Slide) {
	t.Helper()
	p := cloneDeckWithRels(t, "")
	s := SlidesOf(t, p)[0]
	wav := minimalWAV(8000) // 0.5s
	if _, err := s.AddAudio(context.Background(), BytesMedia(wav, "audio/wav"), AudioSpec{
		TrackKey: "narr",
		Role:     AudioRoleNarration,
		Duration: Optional[time.Duration]{Value: 500 * time.Millisecond, Set: true},
	}); err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	return p, s
}

func TestSlideClone_BasicStructure(t *testing.T) {
	p, s := cloneDeckWithAudio(t)
	defer p.Close()
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	if err := s.SetSpeakerNotes("讲稿内容"); err != nil {
		t.Fatalf("SetSpeakerNotes: %v", err)
	}
	srcSlideXML, _ := p.partBytes(s.part)

	c, err := s.Clone(nil)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if c.ID() == s.ID() {
		t.Fatalf("cloned slide id %d equals source", c.ID())
	}
	slides := SlidesOf(t, p)
	if len(slides) != 2 || slides[1].ID() != c.ID() {
		t.Fatalf("slides after clone = %d (want 2, clone last)", len(slides))
	}
	// 目标页 Part 字节级等于源页（形状/timing/引用原样保留）。
	dstXML, err := p.partBytes("/ppt/slides/slide2.xml")
	if err != nil {
		t.Fatalf("cloned slide part: %v", err)
	}
	if !bytes.Equal(srcSlideXML, dstXML) {
		t.Error("cloned slide XML differs from source")
	}
	// 目标页关系流：ID/Type 保留，chart Target 指向克隆 chart。
	rels, ok, err := p.relsOf("/ppt/slides/slide2.xml")
	if err != nil || !ok {
		t.Fatalf("cloned slide rels: %v ok=%v", err, ok)
	}
	sawChart, sawLayout := false, false
	for _, r := range rels {
		switch r.Type {
		case relChart:
			sawChart = true
			if r.TargetPart != "/ppt/charts/chart2.xml" {
				t.Errorf("chart rel target = %s, want /ppt/charts/chart2.xml", r.TargetPart)
			}
		case opc.RelSlideLayout:
			sawLayout = true
			if r.TargetPart != "/ppt/slideLayouts/slideLayout1.xml" {
				t.Errorf("layout rel target = %s, want reuse slideLayout1", r.TargetPart)
			}
		}
	}
	if !sawChart || !sawLayout {
		t.Errorf("cloned rels missing chart(%v)/layout(%v)", sawChart, sawLayout)
	}
	// 版式不复制：包内仍只有一个 layout Part。
	if !p.hasPartCurrent("/ppt/slideLayouts/slideLayout1.xml") || p.hasPartCurrent("/ppt/slideLayouts/slideLayout2.xml") {
		t.Error("layout should be reused, not cloned")
	}
}

func TestSlideClone_ChartDataIsolation(t *testing.T) {
	p, s := cloneDeckWithAudio(t)
	defer p.Close()
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	c, err := s.Clone(nil)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	_ = c
	srcChart, _ := p.partBytes("/ppt/charts/chart1.xml")
	srcWB, _ := p.partBytes("/ppt/embeddings/Microsoft_Excel_Worksheet1.xlsx")
	dstChart, err := p.partBytes("/ppt/charts/chart2.xml")
	if err != nil {
		t.Fatalf("cloned chart: %v", err)
	}
	dstWB, err := p.partBytes("/ppt/embeddings/Microsoft_Excel_Worksheet2.xlsx")
	if err != nil {
		t.Fatalf("cloned workbook: %v", err)
	}
	if !bytes.Equal(srcChart, dstChart) {
		t.Error("cloned chart bytes differ from source")
	}
	if !bytes.Equal(srcWB, dstWB) {
		t.Error("cloned workbook bytes differ from source")
	}
	// 改目标页图表数据：源图表与源工作簿字节不变（隔离金样）。
	cloned := SlidesOf(t, p)[1]
	shapes, err := cloned.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	var cs *ChartShape
	for _, sh := range shapes {
		if ch, ok := sh.(*ChartShape); ok {
			cs = ch
		}
	}
	if cs == nil {
		t.Fatal("no ChartShape on cloned slide")
	}
	cd, err := cs.Data()
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	cd.Title = "改后标题"
	cd.Series[0].Values[0] = 999
	if err := cs.SetData(cd); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	afterSrcChart, _ := p.partBytes("/ppt/charts/chart1.xml")
	afterSrcWB, _ := p.partBytes("/ppt/embeddings/Microsoft_Excel_Worksheet1.xlsx")
	if !bytes.Equal(srcChart, afterSrcChart) {
		t.Error("source chart changed after editing the clone")
	}
	if !bytes.Equal(srcWB, afterSrcWB) {
		t.Error("source workbook changed after editing the clone")
	}
	// 源页图表数据读取不受影响。
	srcShapes, _ := s.Shapes()
	for _, sh := range srcShapes {
		if ch, ok := sh.(*ChartShape); ok {
			d, err := ch.Data()
			if err != nil {
				t.Fatalf("source Data: %v", err)
			}
			if d.Title != "季度营收" || d.Series[0].Values[0] == 999 {
				t.Errorf("source chart data leaked clone edit: %+v", d)
			}
		}
	}
}

func TestSlideClone_MediaSharedByDefault(t *testing.T) {
	p, s := cloneDeckWithAudio(t)
	defer p.Close()
	c, err := s.Clone(nil)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	_ = c
	// 共享：媒体 Part 只有一份，两页关系都指向它。
	if p.hasPartCurrent("/ppt/media/audio2.wav") {
		t.Error("media cloned under default policy (want shared)")
	}
	rels1, _, _ := p.relsOf("/ppt/slides/slide1.xml")
	rels2, _, err2 := p.relsOf("/ppt/slides/slide2.xml")
	if err2 != nil {
		t.Fatalf("cloned rels: %v", err2)
	}
	var m1, m2 string
	for _, r := range rels1 {
		if r.Type == relAudio {
			m1 = string(r.TargetPart)
		}
	}
	for _, r := range rels2 {
		if r.Type == relAudio {
			m2 = string(r.TargetPart)
		}
	}
	if m1 == "" || m1 != m2 {
		t.Errorf("audio targets: src=%q clone=%q (want equal)", m1, m2)
	}
}

func TestSlideClone_MediaIndependent(t *testing.T) {
	p, s := cloneDeckWithAudio(t)
	defer p.Close()
	if _, err := s.Clone(&ClonePolicy{IndependentMedia: true}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	src, err := p.partBytes("/ppt/media/audio1.wav")
	if err != nil {
		t.Fatalf("source media: %v", err)
	}
	dst, err := p.partBytes("/ppt/media/audio2.wav")
	if err != nil {
		t.Fatalf("cloned media: %v", err)
	}
	if !bytes.Equal(src, dst) {
		t.Error("independent media bytes differ")
	}
	rels2, _, err2 := p.relsOf("/ppt/slides/slide2.xml")
	if err2 != nil {
		t.Fatalf("cloned rels: %v", err2)
	}
	sawClone := false
	for _, r := range rels2 {
		if r.Type == relAudio && r.TargetPart == "/ppt/media/audio2.wav" {
			sawClone = true
		}
	}
	if !sawClone {
		t.Error("cloned slide does not reference the copied media part")
	}
	// Profile 的 MediaPart 同步指向独立副本。
	profs := p.audioProfilesOfSlide("/ppt/slides/slide2.xml")
	if len(profs) != 1 {
		t.Fatalf("clone profiles = %d (want 1)", len(profs))
	}
	if profs[0].MediaPart != "/ppt/media/audio2.wav" {
		t.Errorf("profile media = %s, want copied part", profs[0].MediaPart)
	}
}

func TestSlideClone_NotesClonedAndBackrefRewritten(t *testing.T) {
	p := cloneDeckWithRels(t, "")
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.EnsureSpeakerNotes(); err != nil {
		t.Fatalf("EnsureSpeakerNotes: %v", err)
	}
	if err := s.SetSpeakerNotes("克隆我"); err != nil {
		t.Fatalf("SetSpeakerNotes: %v", err)
	}
	c, err := s.Clone(nil)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	txt, err := c.SpeakerNotesText()
	if err != nil || txt != "克隆我" {
		t.Errorf("cloned notes text = %q err=%v (want 克隆我)", txt, err)
	}
	// 克隆 notes 回引目标页；源 notes 不动。
	rels, _, err := p.relsOf("/ppt/notesSlides/notesSlide2.xml")
	if err != nil {
		t.Fatalf("cloned notes rels: %v", err)
	}
	backref := ""
	for _, r := range rels {
		if r.Type == opc.RelSlide {
			backref = string(r.TargetPart)
		}
	}
	if backref != "/ppt/slides/slide2.xml" {
		t.Errorf("notes backref = %s, want /ppt/slides/slide2.xml", backref)
	}
	// 源 notes 回引保持源页。
	rels1, _, _ := p.relsOf("/ppt/notesSlides/notesSlide1.xml")
	for _, r := range rels1 {
		if r.Type == opc.RelSlide && r.TargetPart != "/ppt/slides/slide1.xml" {
			t.Errorf("source notes backref changed: %s", r.TargetPart)
		}
	}
	// notesMaster 复用：包内仍只有一个。
	if p.hasPartCurrent("/ppt/notesMasters/notesMaster2.xml") {
		t.Error("notesMaster cloned (want reuse)")
	}
}

func TestSlideClone_UnknownRelRejected(t *testing.T) {
	// AT-13 同文档口径：未知内部关系 → 整体拒绝，不生成残缺目标页。
	p := cloneDeckWithRels(t, `<Relationship Id="rId9" Type="`+opc.RelTypePrefix+
		`oleObject" Target="../embeddings/oleObject1.bin"/>`)
	defer p.Close()
	// 伪造依赖 Part 存在（关系目标存在性检查通过后才分类拒绝）。
	if err := p.stageAdd("/ppt/embeddings/oleObject1.bin", []byte("bin"), "application/vnd.openxmlformats-officedocument.oleObject"); err != nil {
		t.Fatalf("stageAdd: %v", err)
	}
	p.commit()
	s := SlidesOf(t, p)[0]
	_, err := s.Clone(nil)
	if err == nil {
		t.Fatal("Clone with unknown relationship succeeded (want rejection)")
	}
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Errorf("err = %v, want ErrUnsupportedEdit", err)
	}
	// 零残留：不产生目标页/主关系/sldId。
	slides := SlidesOf(t, p)
	if len(slides) != 1 {
		t.Errorf("slides = %d after rejected clone (want 1)", len(slides))
	}
	if p.hasPartCurrent("/ppt/slides/slide2.xml") {
		t.Error("residual cloned slide part")
	}
}

func TestSlideClone_AudioProfileDerivedKey(t *testing.T) {
	p, s := cloneDeckWithAudio(t)
	defer p.Close()
	if _, err := s.Clone(nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if _, err := s.Clone(nil); err != nil {
		t.Fatalf("Clone twice: %v", err)
	}
	profs1 := p.audioProfilesOfSlide("/ppt/slides/slide2.xml")
	profs2 := p.audioProfilesOfSlide("/ppt/slides/slide3.xml")
	if len(profs1) != 1 || len(profs2) != 1 {
		t.Fatalf("profiles: slide2=%d slide3=%d (want 1/1)", len(profs1), len(profs2))
	}
	if profs1[0].TrackKey == "narr" || profs2[0].TrackKey == "narr" {
		t.Errorf("derived keys collide with source: %q %q", profs1[0].TrackKey, profs2[0].TrackKey)
	}
	if profs1[0].TrackKey == profs2[0].TrackKey {
		t.Errorf("derived keys collide: both %q", profs1[0].TrackKey)
	}
	// 源 Profile 保持原样。
	srcProfs := p.audioProfilesOfSlide("/ppt/slides/slide1.xml")
	if len(srcProfs) != 1 || srcProfs[0].TrackKey != "narr" {
		t.Errorf("source profile mutated: %+v", srcProfs)
	}
}

func TestSlideClone_SaveRoundTrip(t *testing.T) {
	p, s := cloneDeckWithAudio(t)
	defer p.Close()
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	if err := s.SetSpeakerNotes("round trip"); err != nil {
		t.Fatalf("SetSpeakerNotes: %v", err)
	}
	if _, err := s.Clone(&ClonePolicy{IndependentMedia: true}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p2, err := OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer p2.Close()
	slides := SlidesOf(t, p2)
	if len(slides) != 2 {
		t.Fatalf("slides after round trip = %d (want 2)", len(slides))
	}
	// 克隆页图表可读且数据等于源。
	shapes, err := slides[1].Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	var cs *ChartShape
	for _, sh := range shapes {
		if ch, ok := sh.(*ChartShape); ok {
			cs = ch
		}
	}
	if cs == nil {
		t.Fatal("no chart on cloned slide after round trip")
	}
	d, err := cs.Data()
	if err != nil {
		t.Fatalf("cloned chart Data: %v", err)
	}
	if d.Title != "季度营收" || len(d.Series) != 2 {
		t.Errorf("cloned chart data = %+v", d)
	}
	// 备注与音轨 Profile 保持。
	txt, err := slides[1].SpeakerNotesText()
	if err != nil || txt != "round trip" {
		t.Errorf("cloned notes = %q err=%v", txt, err)
	}
	if profs := p2.audioProfilesOfSlide("/ppt/slides/slide2.xml"); len(profs) != 1 {
		t.Errorf("clone profiles after round trip = %d (want 1)", len(profs))
	}
}

func TestSlideClone_StaleAndClosedGuards(t *testing.T) {
	p, s := cloneDeckWithAudio(t)
	if _, err := s.Clone(nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := p.RemoveSlide(s.ID()); err != nil {
		t.Fatalf("RemoveSlide: %v", err)
	}
	if _, err := s.Clone(nil); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("clone of removed slide: %v (want ErrStaleHandle)", err)
	}
	p.Close()
	if _, err := s.Clone(nil); !errors.Is(err, ErrClosed) {
		t.Errorf("clone after close: %v (want ErrClosed)", err)
	}
}

func TestSlideClone_ExternalRelPreserved(t *testing.T) {
	// 外部关系（超链接）逐字节保留：Target/TargetMode 原样。
	p := cloneDeckWithRels(t, `<Relationship Id="rId8" Type="`+opc.RelTypePrefix+
		`hyperlink" Target="https://example.org/a?x=1&amp;y=2" TargetMode="External"/>`)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.Clone(nil); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	raw, err := p.partBytes("/ppt/slides/_rels/slide2.xml.rels")
	if err != nil {
		t.Fatalf("cloned rels: %v", err)
	}
	if !strings.Contains(string(raw), `Target="https://example.org/a?x=1&amp;y=2"`) ||
		!strings.Contains(string(raw), `TargetMode="External"`) {
		t.Errorf("external rel not preserved verbatim:\n%s", raw)
	}
}

// ---------- CLONE-02 跨文档受限复制（V2.6 §11 + AT-13 跨包口径） ----------

// cloneCrossDeck 构造用于跨文档复制的源/目标 deck；两者 slideLayout1 字节
// 由 minimalTemplateParts() 确定性一致——天然满足 matchReusePart 的复用条件。
func cloneCrossDeck(t *testing.T) (srcP, dstP *Presentation, srcSlide *Slide) {
	t.Helper()
	srcP = cloneDeckWithRels(t, "")
	dstP = cloneDeckWithRels(t, "")
	srcSlide = SlidesOf(t, srcP)[0]
	return srcP, dstP, srcSlide
}

func TestCopySlideFrom_BasicLayoutReuse(t *testing.T) {
	// 两 deck 的 slideLayout1 字节一致 → 复用命中 → 跨文档复制成功。
	srcP, dstP, srcSlide := cloneCrossDeck(t)
	defer srcP.Close()
	defer dstP.Close()

	origDstCount := len(SlidesOf(t, dstP))
	newSlide, err := dstP.CopySlideFrom(srcSlide, nil)
	if err != nil {
		t.Fatalf("CopySlideFrom: %v", err)
	}
	if newSlide == nil {
		t.Fatal("nil cloned slide")
	}
	if newSlide.ID() == srcSlide.ID() {
		t.Errorf("cloned slide ID collides with source: %d", newSlide.ID())
	}
	if len(SlidesOf(t, dstP)) != origDstCount+1 {
		t.Errorf("page count: %d (want %d)", len(SlidesOf(t, dstP)), origDstCount+1)
	}
	// 跨文档复制后源文档不应受影响。
	if len(SlidesOf(t, srcP)) != 1 {
		t.Errorf("source page count changed: %d", len(SlidesOf(t, srcP)))
	}
}

func TestCopySlideFrom_ReuseLayoutMismatch(t *testing.T) {
	// 改写目标 deck 的 slideLayout1 字节，使其与源不一致 → matchReusePart
	// 失败 → 跨文档复制整体拒绝 ErrUnsupportedEdit，且目标暂存区零残留。
	srcP, dstP, srcSlide := cloneCrossDeck(t)
	defer srcP.Close()
	defer dstP.Close()

	// 篡改目标 slideLayout1：补一个额外空注释改变字节。
	parts := readAllParts(t, dstP)
	parts["/ppt/slideLayouts/slideLayout1.xml"] = append(
		parts["/ppt/slideLayouts/slideLayout1.xml"],
		[]byte("<!--tampered-->")...)
	writeAllParts(t, dstP, parts)
	if _, err := dstP.CopySlideFrom(srcSlide, nil); !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("CopySlideFrom: %v (want ErrUnsupportedEdit)", err)
	}
	// 拒绝路径零残留：目标暂存区不应留下任何 patch。
	if saved := dumpPending(dstP); len(saved) > 0 {
		t.Errorf("pending left after rejected copy: %s", saved)
	}
}

func TestCopySlideFrom_SameDocDispatchedToClone(t *testing.T) {
	// src.p == dst → 走同文档 Clone 路径（更完整：媒体可共享）。
	p, s := cloneDeckWithAudio(t)
	defer p.Close()
	ns, err := p.CopySlideFrom(s, nil)
	if err != nil {
		t.Fatalf("CopySlideFrom same doc: %v", err)
	}
	if ns.ID() == s.ID() {
		t.Errorf("cloned slide collides with source ID: %d", ns.ID())
	}
	if len(SlidesOf(t, p)) != 2 {
		t.Errorf("page count: %d (want 2)", len(SlidesOf(t, p)))
	}
}

func TestCopySlideFrom_SaveRoundTrip(t *testing.T) {
	// 跨文档复制 + Save → OpenReader 重开 → 结构完好。
	srcP, dstP, srcSlide := cloneCrossDeck(t)
	defer srcP.Close()
	defer dstP.Close()

	ns, err := dstP.CopySlideFrom(srcSlide, nil)
	if err != nil {
		t.Fatalf("CopySlideFrom: %v", err)
	}
	nsID := ns.ID()
	var buf bytes.Buffer
	if _, err := dstP.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.Bytes()
	reopened, err := OpenReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	slides := SlidesOf(t, reopened)
	if len(slides) != 2 {
		t.Fatalf("reopened page count: %d (want 2)", len(slides))
	}
	// 跨文档复制产生的新页面应在重开后仍可达。
	var found *Slide
	for _, sl := range slides {
		if sl.ID() == nsID {
			found = sl
			break
		}
	}
	if found == nil {
		t.Fatalf("cloned slide ID %d not present after reopen", nsID)
	}
	relsPartName := relsPart(found.part)
	hasRels := reopened.hasPartCurrent(relsPartName)
	if !hasRels {
		t.Fatalf("slide rels %s missing after reopen (slide=%s)", relsPartName, found.part)
	}
	// relsOf 接收被引用方 PartName（slide），内部自动算 relsPart 并解析。
	rels, ok, err := reopened.relsOf(found.part)
	if err != nil || !ok || len(rels) == 0 {
		rawRels, _ := reopened.partBytes(relsPartName)
		t.Fatalf("slide rels parse after reopen: err=%v ok=%v rels=%d raw=%q",
			err, ok, len(rels), string(rawRels))
	}
	hit := false
	for _, rel := range rels {
		if rel.Type == opc.RelSlideLayout && rel.Mode == opc.TargetInternal {
			hit = true
			if !reopened.hasPartCurrent(rel.TargetPart) {
				t.Errorf("layout rel target %s missing after reopen", rel.TargetPart)
			}
		}
	}
	if !hit {
		t.Errorf("no slideLayout rel after reopen:\n%+v", rels)
	}
}

// readAllParts 把当前 p 视图的全部 Part 字节读出为 map（根目录读路径 +
// 会话 added 优先），便于跨文档 mismatch 测试篡改某个 Part 后再写回。
func readAllParts(t *testing.T, p *Presentation) map[opc.PartName][]byte {
	t.Helper()
	out := map[opc.PartName][]byte{}
	for _, name := range p.pk.PartNames() {
		b, err := p.partBytes(name)
		if err != nil {
			t.Fatalf("partBytes(%s): %v", name, err)
		}
		out[name] = b
	}
	return out
}

// writeAllParts 把 map 中全部 Part 字节覆盖到当前已提交视图（partBytes
// 只看 overrides，因此走 stagePatch + commit 让改动立刻可见，便于测试
// 篡改某个 Part 后再让 CopySlideFrom 看到不同字节）。
func writeAllParts(t *testing.T, p *Presentation, parts map[opc.PartName][]byte) {
	t.Helper()
	for name, b := range parts {
		if !p.hasPartCurrent(name) {
			t.Fatalf("part %s missing in destination view", name)
		}
		if err := p.stagePatch(name, b); err != nil {
			t.Fatalf("stagePatch(%s): %v", name, err)
		}
	}
	p.commit()
}

// dumpPending 把暂存区展开为可读字符串（仅在断言非空时调用）。
func dumpPending(p *Presentation) string {
	if p.pending == nil {
		return ""
	}
	var buf bytes.Buffer
	for n := range p.pending.Patched {
		buf.WriteString("patched:")
		buf.WriteString(string(n))
		buf.WriteByte('\n')
	}
	for n := range p.pending.Added {
		buf.WriteString("added:")
		buf.WriteString(string(n))
		buf.WriteByte('\n')
	}
	for n := range p.pending.Deleted {
		buf.WriteString("deleted:")
		buf.WriteString(string(n))
		buf.WriteByte('\n')
	}
	return buf.String()
}

// ---------- 纯函数 helper 单测（无 fixture，clone 主体外的内部逻辑） ----------

// TestSplitTrailingDigits 覆盖 clone.go:545 splitTrailingDigits 的全部分支。
// 该函数把"chart1.xml"拆为 ("chart", ".xml", 1)；返回 ok=false 时调用方
// 走 allocCloneName 的非数字结尾路径（base-N 后缀模式）。
func TestSplitTrailingDigits(t *testing.T) {
	cases := []struct {
		name       string
		base       string
		wantStem   string
		wantExt    string
		wantDigits int
		wantOK     bool
	}{
		// 正常路径（含边界：单数字 / 多数字 / 0 / stem 内含数字）
		{"trailing_single_digit", "chart1.xml", "chart", ".xml", 1, true},
		{"trailing_multi_digit", "notesSlide100.xml", "notesSlide", ".xml", 100, true},
		{"trailing_zero", "sheet0.xlsx", "sheet", ".xlsx", 0, true},
		{"stem_with_internal_digit", "x9chart5.xml", "x9chart", ".xml", 5, true},

		// 提前返回 false 的三类：全数字结尾 / 无数字 / 无扩展名 / 扩展名在第 0 位
		{"all_digits_head", "123.xml", "", "", 0, false},   // head="123" → k=0
		{"no_digit_at_all", "chart.xml", "", "", 0, false}, // head="chart" → k==len
		{"no_extension", "chart1", "", "", 0, false},       // j<0
		{"ext_at_pos_zero", ".txt", "", "", 0, false},      // j=0 → j<=0 false
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stem, ext, digits, ok := splitTrailingDigits(tc.base)
			if ok != tc.wantOK || stem != tc.wantStem || ext != tc.wantExt || digits != tc.wantDigits {
				t.Errorf("splitTrailingDigits(%q) = (%q,%q,%d,%v), want (%q,%q,%d,%v)",
					tc.base, stem, ext, digits, ok, tc.wantStem, tc.wantExt, tc.wantDigits, tc.wantOK)
			}
		})
	}
}

// TestRetargetRel 覆盖 clone.go:490 retargetRel 的全部分支。
// 绝对 Target → 整体替换；相对带目录 → 保留目录前缀；无目录 → 仅 basename。
// 这是 cloned Part 关系流重写的最后一环，决定补丁后的 Target 字符串。
func TestRetargetRel(t *testing.T) {
	cases := []struct {
		name      string
		oldTarget string
		dst       opc.PartName
		want      string
	}{
		{"absolute_target_same_dir", "/ppt/charts/chart1.xml", "/ppt/charts/chart2.xml", "/ppt/charts/chart2.xml"},
		{"absolute_target_different_dir", "/ppt/media/image1.png", "/ppt/media/image2.png", "/ppt/media/image2.png"},
		{"relative_with_dir", "../charts/chart1.xml", "/ppt/charts/chart2.xml", "../charts/chart2.xml"},
		{"relative_deeply_nested", "../slideLayouts/slideLayout1.xml", "/ppt/slideLayouts/slideLayout2.xml", "../slideLayouts/slideLayout2.xml"},
		{"no_dir_basename_only", "chart1.xml", "/ppt/charts/chart2.xml", "chart2.xml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retargetRel(tc.oldTarget, tc.dst); got != tc.want {
				t.Errorf("retargetRel(%q, %q) = %q, want %q", tc.oldTarget, tc.dst, got, tc.want)
			}
		})
	}
}

// TestClassifyCloneRel 覆盖 clone.go:388 classifyCloneRel 的全部 case 与容器组合。
// 7 个 relType 类别（reuse / media / notes / chart / workbook / backref / unknown）
// 与不同 container 的组合，决定 cloneWalkContainer 走 reuse/media/notes/chart/
// workbook/backref/unknown 哪条分支（unknown 走 ErrUnsupportedEdit 拒绝）。
func TestClassifyCloneRel(t *testing.T) {
	cases := []struct {
		name      string
		relType   string
		container string
		want      string
	}{
		// reuse：版式/母版在 slide 与 notes 容器中都属复用（跨文档按字节匹配）
		{"slide_layout_in_slide", opc.RelSlideLayout, "slide", "reuse"},
		{"slide_layout_in_notes", opc.RelSlideLayout, "notes", "reuse"},
		{"notes_master_in_slide", relNotesMaster, "slide", "reuse"},
		{"notes_master_in_notes", relNotesMaster, "notes", "reuse"},

		// media：图片/音频/视频/媒体在任何容器中都属 media
		{"image_in_slide", relImage, "slide", "media"},
		{"audio_in_slide", relAudio, "slide", "media"},
		{"video_in_slide", relVideo, "slide", "media"},
		{"media_in_slide", relMedia, "slide", "media"},
		{"image_in_chart", relImage, "chart", "media"},

		// notes：notesSlide 关系仅在 slide 容器中合法
		{"notes_slide_in_slide", opc.RelNotesSlide, "slide", "notes"},
		{"notes_slide_in_chart", opc.RelNotesSlide, "chart", "unknown"},

		// chart：chart 关系仅在 slide 容器中合法
		{"chart_in_slide", relChart, "slide", "chart"},
		{"chart_in_notes", relChart, "notes", "unknown"},

		// workbook：包关系仅在 chart 容器中合法（图表嵌入工作簿）
		{"workbook_in_chart", relPackage, "chart", "workbook"},
		{"workbook_in_slide", relPackage, "slide", "unknown"},

		// backref：slide 关系仅在 notes 容器中合法（notes→slide 回引）
		{"slide_in_notes", opc.RelSlide, "notes", "backref"},
		{"slide_in_slide", opc.RelSlide, "slide", "unknown"},

		// 完全未知 relType（OLE/SmartArt/外部生成图表的样式 Part 等）
		{"unknown_rel_type_in_slide", "http://example.com/foo", "slide", "unknown"},
		{"unknown_rel_type_in_chart", "http://example.com/foo", "chart", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyCloneRel(tc.relType, tc.container); got != tc.want {
				t.Errorf("classifyCloneRel(%q, %q) = %q, want %q", tc.relType, tc.container, got, tc.want)
			}
		})
	}
}

// TestFallbackCloneCT 覆盖 clone.go:640 fallbackCloneCT 的全部分支。
// 源读得到内容类型优先用源（cp.ct != "" 早返回）；读不到时按 kind 兜底
// 到根包常量；其它 kind（media / 未知）→ ""。该兜底仅在源 OPC ContentType
// 查询失败时启用，正常路径不会被触发——因此 100% 覆盖 = 8 个 case。
func TestFallbackCloneCT(t *testing.T) {
	cases := []struct {
		name string
		cp   clonePartPlan
		want string
	}{
		{"ct_already_set_notes_kind", clonePartPlan{ct: "custom/ct", kind: "notes"}, "custom/ct"},
		{"ct_already_set_chart_kind", clonePartPlan{ct: "custom/ct", kind: "chart"}, "custom/ct"},
		{"kind_notes_no_ct", clonePartPlan{kind: "notes"}, ctNotesSlide},
		{"kind_chart_no_ct", clonePartPlan{kind: "chart"}, ctChartPart},
		{"kind_workbook_no_ct", clonePartPlan{kind: "workbook"}, ctWorkbook},
		{"kind_media_no_ct", clonePartPlan{kind: "media"}, ""},   // media 不在兜底白名单
		{"unknown_kind", clonePartPlan{kind: "weird"}, ""},
		{"empty", clonePartPlan{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fallbackCloneCT(tc.cp); got != tc.want {
				t.Errorf("fallbackCloneCT(%+v) = %q, want %q", tc.cp, got, tc.want)
			}
		})
	}
}
