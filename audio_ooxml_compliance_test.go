package pptx

import (
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/opc"
)

// 本文件守门 ADR-025 修复的两处 OOXML 合规性问题。
//
// 背景：含配音的产物在 PowerPoint 下被判"文件或目录损坏"（0x80070570），
// 而 WPS 宽容接受 —— 只有真机 PowerPoint 验证才暴露得出。两处缺陷都是
// **必要条件**（最小实验矩阵证明缺任何一个都不通过）：
//
//  1. audio p:pic 结构不合规：p:nvPicPr 缺必需的 p:nvPr，且 a:audioFile
//     被放在 p:blipFill 内、缺必需的 r:link（正确位置是 p:nvPr 内）。
//  2. SetAdvanceAfter 识别不出被 mc:AlternateContent 包裹的既有 p:transition，
//     于是追加了第二个 p:transition，违反 CT_Slide 的 maxOccurs=1。

// TestBuildAudioPicFragmentIsSchemaCompliant 断言 audio 形状片段的结构合规性。
func TestBuildAudioPicFragmentIsSchemaCompliant(t *testing.T) {
	frag := buildAudioPicFragment(11, "Audio 11", "rId2", "wav")

	// 1) p:nvPr 存在（CT_PictureNonVisual 三项均 minOccurs=1）。
	if !strings.Contains(frag, "<p:nvPr>") {
		t.Errorf("fragment lacks required <p:nvPr>: %s", frag)
	}
	// 2) a:audioFile 带必需的 r:link，且位于 p:nvPr 内（不在 blipFill 内）。
	nvPr := strings.Index(frag, "<p:nvPr>")
	audio := strings.Index(frag, `<a:audioFile r:link="rId2"/>`)
	blipStart := strings.Index(frag, "<p:blipFill>")
	blipEnd := strings.Index(frag, "</p:blipFill>")
	if nvPr < 0 || audio < 0 {
		t.Fatalf("fragment lacks nvPr or audioFile r:link: %s", frag)
	}
	if audio < nvPr {
		t.Errorf("a:audioFile must live inside p:nvPr (audio@%d < nvPr@%d): %s", audio, nvPr, frag)
	}
	if blipStart >= 0 && blipEnd >= 0 && audio > blipStart && audio < blipEnd {
		t.Errorf("a:audioFile must not live inside p:blipFill: %s", frag)
	}
	// 3) blipFill 保留 a:blip 引用。
	if !strings.Contains(frag, `<a:blip r:embed="rId2"/>`) {
		t.Errorf("fragment lacks a:blip reference: %s", frag)
	}
}

// TestSetAdvanceAfterKeepsSingleTransition 断言 SetAdvanceAfter 不会在
// mc:AlternateContent 之外再追加一个 p:transition（后者会使 CT_Slide 的
// maxOccurs=1 被违反，PowerPoint 判包损坏）。
//
// 样本用公开语料 s001-text：其 slide1.xml 自带
// `<mc:AlternateContent><mc:Choice Requires="p14"><p:transition .../>` +
// `<mc:Fallback><p:transition .../></mc:AlternateContent>`，正是触发该缺陷的形态。
func TestSetAdvanceAfterKeepsSingleTransition(t *testing.T) {
	p, err := Open("testdata/corpus/s001-text/s001-text.pptx")
	if err != nil {
		t.Skipf("corpus sample unavailable: %v", err)
	}
	defer p.Close()

	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	if len(slides) == 0 {
		t.Fatal("no slides")
	}
	before, err := p.partBytes(opc.PartName("/ppt/slides/slide1.xml"))
	if err != nil {
		t.Fatalf("read slide before: %v", err)
	}
	// 前提校验：源模板确实自带 mc:AlternateContent 包裹的 transition。
	if !strings.Contains(string(before), "<mc:AlternateContent>") {
		t.Skip("sample no longer carries mc:AlternateContent; premise invalid")
	}
	if err := slides[0].SetAdvanceAfter(2500 * time.Millisecond); err != nil {
		t.Fatalf("SetAdvanceAfter: %v", err)
	}
	after, err := p.partBytes(opc.PartName("/ppt/slides/slide1.xml"))
	if err != nil {
		t.Fatalf("read slide after: %v", err)
	}
	s := string(after)

	// mc 的 Choice 与 Fallback 各一个 transition —— 且**只有**这两个。
	if n := strings.Count(s, "<p:transition"); n != 2 {
		t.Errorf("p:transition count = %d, want 2 (Choice + Fallback only, no extra direct child)", n)
	}
	if n := strings.Count(s, `advTm="2500"`); n != 2 {
		t.Errorf("advTm written on %d transitions, want 2 (both mc branches)", n)
	}
	// 不得出现"直接子元素形态"的新 transition。
	if strings.Contains(s, `<p:transition advTm="2500"/>`) {
		t.Errorf("a direct-child <p:transition advTm=.../> was appended (duplicate transition)")
	}
}
