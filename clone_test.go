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
