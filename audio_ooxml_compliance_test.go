package pptx

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
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
	frag := buildAudioPicFragment(11, "Audio 11", "rId2", "rId3", "wav", 914400, 914400, 914400, 914400)

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
	// 3) blipFill 的 a:blip 必须指向**图标图片**（iconRid=rId3），
	//    而非音频关系（历史上指向音频 → PowerPoint 无法渲染图标）。
	if !strings.Contains(frag, `<a:blip r:embed="rId3"/>`) {
		t.Errorf("fragment's a:blip must reference the poster icon (rId3): %s", frag)
	}
	if strings.Contains(frag, `<a:blip r:embed="rId2"/>`) {
		t.Errorf("a:blip must not reference the audio relationship: %s", frag)
	}
	// 4) 可点击媒体：cNvPr 带 ppaction://media 超链接；cNvPicPr 带 picLocks。
	if !strings.Contains(frag, `<a:hlinkClick r:id="" action="ppaction://media"/>`) {
		t.Errorf("fragment lacks ppaction://media hlinkClick: %s", frag)
	}
	if !strings.Contains(frag, `<a:picLocks noChangeAspect="1"/>`) {
		t.Errorf("fragment lacks picLocks: %s", frag)
	}
	// 5) 几何框必须写为调用方提供的非零值（历史上硬编码 0×0 导致
	//    客户端打开后看不到音频图标）。
	for _, want := range []string{
		`<a:off x="914400" y="914400"/>`,
		`<a:ext cx="914400" cy="914400"/>`,
	} {
		if !strings.Contains(frag, want) {
			t.Errorf("fragment lacks geometry %q: %s", want, frag)
		}
	}
	if strings.Contains(frag, `<a:ext cx="0" cy="0"/>`) {
		t.Errorf("fragment must not carry zero-size geometry: %s", frag)
	}
}

// TestAudioGeometryDefaults 断言 AudioSpec 全零几何时补默认可见尺寸，
// 并定位到页面右下角（距右/下边各 0.25in），避免 0×0 不可见或遮挡正文。
func TestAudioGeometryDefaults(t *testing.T) {
	const sw, sh = 12192000, 6858000 // 16:9
	const size, margin = 914400, 228600
	cases := []struct {
		name           string
		spec           AudioSpec
		ox, oy, cx, cy int64
	}{
		{"all zero -> bottom-right", AudioSpec{}, sw - margin - size, sh - margin - size, size, size},
		{"explicit bounds preserved", AudioSpec{X: 100, Y: 200, Width: 300, Height: 400}, 100, 200, 300, 400},
		{"position set, size zero", AudioSpec{X: 100, Y: 200}, 100, 200, size, size},
		{"size set, position zero", AudioSpec{Width: 300, Height: 400}, sw - margin - 300, sh - margin - 400, 300, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ox, oy, cx, cy := audioGeometry(tc.spec, sw, sh)
			if ox != tc.ox || oy != tc.oy || cx != tc.cx || cy != tc.cy {
				t.Errorf("audioGeometry = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					ox, oy, cx, cy, tc.ox, tc.oy, tc.cx, tc.cy)
			}
			if cx <= 0 || cy <= 0 {
				t.Errorf("geometry must be visible (cx=%d cy=%d)", cx, cy)
			}
		})
	}
}

// TestAudioShapeHasVisibleBounds 端到端断言 AddAudio 产出的形状具有
// 非零几何（经幻灯片 XML 文本核对）。
func TestAudioShapeHasVisibleBounds(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layouts, err := p.Layouts()
	if err != nil {
		t.Fatalf("Layouts: %v", err)
	}
	slide, err := p.AddSlide(layouts[0])
	if err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	if _, err := slide.AddAudio(context.Background(), BytesMedia(minimalWAV(64), "audio/wav"), AudioSpec{
		TrackKey: "k1",
		Role:     AudioRoleNarration,
		Source:   BytesMedia(minimalWAV(64), "audio/wav"),
	}); err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	doc, _, err := slide.slideTree()
	if err != nil {
		t.Fatalf("slideTree: %v", err)
	}
	xml := string(doc.Original())
	// 定位音频 p:pic 片段（以 a:audioFile 为锚），只在该片段内断言几何——
	// 幻灯片根的 p:grpSpPr 合法地是 0×0，不能全局断言。
	anchor := strings.Index(xml, `<a:audioFile r:link=`)
	if anchor < 0 {
		t.Fatalf("audio shape missing a:audioFile r:link: %s", xml)
	}
	segStart := strings.LastIndex(xml[:anchor], `<p:pic>`)
	segEnd := strings.Index(xml[anchor:], `</p:pic>`)
	if segStart < 0 || segEnd < 0 {
		t.Fatalf("cannot delimit audio p:pic fragment: %s", xml)
	}
	frag := xml[segStart : anchor+segEnd+len(`</p:pic>`)]
	if strings.Contains(frag, `<a:ext cx="0" cy="0"/>`) {
		t.Errorf("audio shape still has zero-size geometry: %s", frag)
	}
	if !strings.Contains(frag, `<a:ext cx="914400" cy="914400"/>`) {
		t.Errorf("audio shape lacks default 1in×1in geometry: %s", frag)
	}
	// 默认定位：右下角（12192000/6858000 - 228600 - 914400）。
	if !strings.Contains(frag, `<a:off x="11049000" y="5715000"/>`) {
		t.Errorf("audio shape not at default bottom-right position: %s", frag)
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

// synthTestWAV 合成 16-bit 单声道 PCM WAV（纯函数，无外部依赖）。
func synthTestWAV(seconds float64, sampleRate int) []byte {
	n := int(seconds * float64(sampleRate))
	pcm := make([]byte, n*2)
	for i := 0; i < n; i++ {
		v := int16(math.Sin(2*math.Pi*440*float64(i)/float64(sampleRate)) * 0.3 * math.MaxInt16)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}
	var buf bytes.Buffer
	write := func(v any) { _ = binary.Write(&buf, binary.LittleEndian, v) }
	buf.WriteString("RIFF")
	write(uint32(36 + len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	write(uint32(16))
	write(uint16(1))
	write(uint16(1))
	write(uint32(sampleRate))
	write(uint32(sampleRate * 2))
	write(uint16(2))
	write(uint16(16))
	buf.WriteString("data")
	write(uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

// TestAudioShapeProfileExposesDuration 守门 AudioShape.Profile() 的字段完整性。
//
// 历史上 lastProfileByMedia 是一份手写精简解析副本，漏读 durMs/stMs/trigger/
// slide，导致 Profile().Duration 恒为 0（而 PlanTimingSync 走完整解析器却有值，
// 两条路径语义不一致）。修复为复用 parseAudioProfile 后，本测试应通过。
func TestAudioShapeProfileExposesDuration(t *testing.T) {
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
	const wantDur = 2 * time.Second
	wav := synthTestWAV(2.0, 44100)
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(wav, "audio/wav"), AudioSpec{
		TrackKey: "narration-1",
		Role:     AudioRoleNarration,
	})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}

	prof := as.Profile()
	if prof.TrackKey != "narration-1" {
		t.Errorf("Profile().TrackKey = %q, want narration-1", prof.TrackKey)
	}
	if prof.Duration != wantDur {
		t.Errorf("Profile().Duration = %v, want %v (lastProfileByMedia must reuse parseAudioProfile)", prof.Duration, wantDur)
	}
	if prof.Role != AudioRoleNarration {
		t.Errorf("Profile().Role = %v, want narration", prof.Role)
	}
	if prof.MediaPart == "" {
		t.Errorf("Profile().MediaPart is empty")
	}
	if prof.SlidePart == "" {
		t.Errorf("Profile().SlidePart is empty (dropped by the hand-rolled parser copy)")
	}
	if prof.ShapeID == 0 {
		t.Errorf("Profile().ShapeID is zero")
	}
	if prof.ContentSHA256 == "" {
		t.Errorf("Profile().ContentSHA256 is empty")
	}
}

// TestNarratedDeckRoundTrip 端到端往返：生成含配音的文档 → 保存 → 重新打开，
// 断言 audio 形状与计时设置可完整读回。
//
// 自包含：音频用代码合成（synthTestWAV），**不需要往语料库放二进制样本**，
// 因此该覆盖在 CI 上恒可执行。
func TestNarratedDeckRoundTrip(t *testing.T) {
	const wantDur = 2 * time.Second
	const wantAdv = 2500 * time.Millisecond

	p, err := Open("testdata/corpus/s001-text/s001-text.pptx")
	if err != nil {
		t.Skipf("corpus sample unavailable: %v", err)
	}
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	if len(slides) == 0 {
		t.Fatal("no slides")
	}
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(synthTestWAV(2.0, 44100), "audio/wav"), AudioSpec{
		TrackKey: "narration-1",
		Role:     AudioRoleNarration,
	})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	if err := as.SetPlayback(PlaybackSpec{Trigger: PlaybackOnSlideEnter}); err != nil {
		t.Fatalf("SetPlayback: %v", err)
	}
	if err := slides[0].SetAdvanceAfter(wantAdv); err != nil {
		t.Fatalf("SetAdvanceAfter: %v", err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	p2, err := OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("OpenReader(round-trip): %v", err)
	}
	defer p2.Close()
	slides2, err := p2.Slides()
	if err != nil {
		t.Fatalf("Slides(2): %v", err)
	}
	shapes2, err := slides2[0].Shapes()
	if err != nil {
		t.Fatalf("Shapes(2): %v", err)
	}
	var audio *AudioShape
	for _, sh := range shapes2 {
		if a, ok := sh.(*AudioShape); ok {
			audio = a
		}
	}
	if audio == nil {
		t.Fatalf("audio shape not found after round-trip (shapes=%d)", len(shapes2))
	}
	if audio.Kind() != ShapeAudio {
		t.Errorf("Kind = %v, want ShapeAudio", audio.Kind())
	}
	if got := audio.Profile().Duration; got != wantDur {
		t.Errorf("round-trip Profile().Duration = %v, want %v", got, wantDur)
	}
	if src, err := audio.AudioSource(); err != nil {
		t.Errorf("AudioSource: %v", err)
	} else if src == nil {
		t.Errorf("AudioSource returned nil")
	}
	d, ok, err := slides2[0].AdvanceAfter()
	if err != nil {
		t.Fatalf("AdvanceAfter: %v", err)
	}
	if !ok {
		t.Errorf("AdvanceAfter: ok = false, want true")
	} else if d != wantAdv {
		t.Errorf("AdvanceAfter = %v, want %v", d, wantAdv)
	}
}
