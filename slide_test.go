package pptx

// 本文件覆盖 FEAT-003（go-pptx 特性与 bug 跟踪计划，ppts FEAT-003 读侧补全）
// 的两个新稳定 API：
//   - Slide.Hidden()：p:sldId@show 读侧；
//   - Slide.AdvanceAfter()：p:transition@advTm 读侧。
// 两个方法均只读，与原 SetAdvanceAfter + Presentation.Slides 写入/枚举路径对偶。

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/opc"
)

// hiddenAudioDeck 构造一个含 sldId@show="0" 的 fixture：模拟 PowerPoint
// "Hide Slide" 输出。其余结构与 audioDeck 完全一致，便于 HIDDEN-01 端到端
// 走 Hidden() 公开路径。
func hiddenAudioDeck(t *testing.T) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	const nsP = nsPresentationML
	const nsA = nsDrawingML
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsA + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
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
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		// 注意 show="0" 才是隐藏语义；缺省/其他值均视为可见。
		`<p:sldIdLst><p:sldId id="256" r:id="rIdS1" show="0"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	parts["/[Content_Types].xml"] = append(parts["/[Content_Types].xml"][:len(parts["/[Content_Types].xml"])-len("</Types>")],
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`)...)
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

func TestSlide_Hidden_VisibleByDefault(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	got, err := slides[0].Hidden()
	if err != nil {
		t.Fatalf("Hidden: %v", err)
	}
	if got {
		t.Error("fresh slide (show attr missing) should be visible")
	}
}

func TestSlide_Hidden_AfterMark(t *testing.T) {
	p := hiddenAudioDeck(t)
	defer p.Close()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	got, err := slides[0].Hidden()
	if err != nil {
		t.Fatalf("Hidden: %v", err)
	}
	if !got {
		t.Error("sldId@show=\"0\" slide should report Hidden()=true")
	}
}

func TestSlide_Hidden_OnClosedPresentation(t *testing.T) {
	p := audioDeck(t)
	slides, _ := p.Slides()
	p.Close()
	if _, err := slides[0].Hidden(); !errors.Is(err, ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

func TestSlide_AdvanceAfter_AbsentByDefault(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	d, ok, err := slides[0].AdvanceAfter()
	if err != nil {
		t.Fatalf("AdvanceAfter: %v", err)
	}
	if ok {
		t.Errorf("ok = true on fresh slide, want false (got d=%v)", d)
	}
	if d != 0 {
		t.Errorf("d = %v, want 0", d)
	}
}

func TestSlide_AdvanceAfter_AfterSet(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	if err := slides[0].SetAdvanceAfter(4500 * time.Millisecond); err != nil {
		t.Fatalf("SetAdvanceAfter: %v", err)
	}
	d, ok, err := slides[0].AdvanceAfter()
	if err != nil {
		t.Fatalf("AdvanceAfter: %v", err)
	}
	if !ok {
		t.Fatal("ok = false after SetAdvanceAfter, want true")
	}
	if want := 4500 * time.Millisecond; d != want {
		t.Errorf("d = %v, want %v", d, want)
	}
}

func TestSlide_AdvanceAfter_OnClosedPresentation(t *testing.T) {
	p := audioDeck(t)
	slides, _ := p.Slides()
	p.Close()
	if _, _, err := slides[0].AdvanceAfter(); !errors.Is(err, ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}
