package pptx

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/opc"
)

// 本文件把 ADR-025/026/027 系列暴露的"媒体四态"教训固化为**可执行契约**
// （媒体兼容性工程化）。
//
// 背景：audio 形状一个特性连续暴露 6 处**彼此独立**的缺陷，症状各不相同：
//
//	能否打开 / 图标是否可见 / 是否可点击 / 是否有声 —— 四套独立的客户端约束。
//
// 代码级测试若只断言"存在某元素"，会漏掉量纲错（vol=80 而非 80000）、
// 指向错（a:blip 指向音频而非图片）、缺关联（p14:media）、命名空间错
// （p:videoFile）等整类缺陷。
//
// 契约分三层：
//  1. **片段结构**（fragment）：生成器输出必须含哪些元素 / 值域（既有
//     audio/video compliance 测试）。
//  2. **关系图一致性**（rels，本文件）：slide XML 中每个 r:embed/r:link
//     必须解析到**类型匹配**的关系——blip→image、audioFile→audio、
//     videoFile→video、p14:media→media。这是"blip 指向音频文件"的直接守门。
//  3. **端到端往返**（本文件）：生成 → 保存 → 重开 → Validate → 分类。

var (
	reRelRef   = regexp.MustCompile(`r:(?:embed|link|id)="([^"]*)"`)
	reBlipRef  = regexp.MustCompile(`<a:blip r:embed="([^"]+)"`)
	reAudioRef = regexp.MustCompile(`<a:audioFile r:link="([^"]+)"`)
	reVideoRef = regexp.MustCompile(`<a:videoFile r:link="([^"]+)"`)
	reP14Media = regexp.MustCompile(`<p14:media [^>]*r:embed="([^"]+)"`)
)

// relByID 在关系集中按 Id 查找。
func relByID(rels []*opc.Relationship, id string) *opc.Relationship {
	for _, r := range rels {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// assertRelType 断言 rid 存在且 Type 匹配 wantType。
func assertRelType(t *testing.T, rels []*opc.Relationship, rid, wantType, ctx string) {
	t.Helper()
	r := relByID(rels, rid)
	if r == nil {
		t.Errorf("%s: relationship %q not found (dangling reference)", ctx, rid)
		return
	}
	if r.Type != wantType {
		t.Errorf("%s: relationship %q type = %q, want %q", ctx, rid, r.Type, wantType)
	}
}

// assertNoDanglingRefs 断言 XML 中每个非空 r:embed/r:link/r:id 都解析到关系
// （空串是 PowerPoint 原生 ppaction 链接的合法写法）。
func assertNoDanglingRefs(t *testing.T, xml string, rels []*opc.Relationship, ctx string) {
	t.Helper()
	for _, m := range reRelRef.FindAllStringSubmatch(xml, -1) {
		rid := m[1]
		if rid == "" {
			continue
		}
		if relByID(rels, rid) == nil {
			t.Errorf("%s: dangling relationship reference %q", ctx, rid)
		}
	}
}

// TestMediaRelationshipTargetsMatchAttribute 断言媒体四态之一：
// **关系指向类型匹配**。这是"a:blip 指向音频文件"（图标不可见）这类
// 缺陷的直接守门——修复前 audio 的 blip 与 audioFile 共用同一个音频关系。
//
// 同时覆盖 dangling 检查（所有 r:embed/r:link 必须解析）。
func TestMediaRelationshipTargetsMatchAttribute(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layouts, lerr := p.Layouts()
	if lerr != nil {
		t.Fatalf("Layouts: %v", lerr)
	}
	if err != nil {
		t.Fatalf("Layouts: %v", err)
	}
	sl, err := p.AddSlide(layouts[0])
	if err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	ctx := context.Background()

	auto, err := sl.AddAudio(ctx, BytesMedia(minimalMP3(), "audio/mpeg"), AudioSpec{
		TrackKey: "a-auto", Role: AudioRoleNarration,
		Source:   BytesMedia(minimalMP3(), "audio/mpeg"),
		Duration: NewOptional[time.Duration](time.Second),
	})
	if err != nil {
		t.Fatalf("AddAudio(auto): %v", err)
	}
	if err := auto.SetPlayback(PlaybackSpec{Trigger: PlaybackOnSlideEnter}); err != nil {
		t.Fatalf("SetPlayback(auto): %v", err)
	}
	click, err := sl.AddAudio(ctx, BytesMedia(minimalMP3(), "audio/mpeg"), AudioSpec{
		TrackKey: "a-click", Role: AudioRoleNarration,
		Source:   BytesMedia(minimalMP3(), "audio/mpeg"),
		Duration: NewOptional[time.Duration](time.Second),
	})
	if err != nil {
		t.Fatalf("AddAudio(click): %v", err)
	}
	if err := click.SetPlayback(PlaybackSpec{Trigger: PlaybackOnClick}); err != nil {
		t.Fatalf("SetPlayback(click): %v", err)
	}
	if _, err := sl.AddVideo(ctx, BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "v-1", Role: VideoRoleMain,
		X: 914400, Y: 914400, Width: 5486400, Height: 3086100,
	}); err != nil {
		t.Fatalf("AddVideo: %v", err)
	}

	rels, ok, err := p.relsOf(sl.part)
	if err != nil || !ok {
		t.Fatalf("relsOf: %v (ok=%v)", err, ok)
	}
	xml := slideXML(t, sl)
	assertNoDanglingRefs(t, xml, rels, "slide1")

	// audio：a:blip 必须指向图片；a:audioFile 指向音频；p14:media 指向 2007 media。
	audioBlip := reBlipRef.FindAllStringSubmatch(xml, -1)
	if len(audioBlip) < 2 {
		t.Fatalf("expected >=2 a:blip (2 audio + 1 video), got %d:\n%s", len(audioBlip), xml)
	}
	assertRelType(t, rels, audioBlip[0][1], relImage, "audio1 a:blip")
	assertRelType(t, rels, audioBlip[1][1], relImage, "audio2 a:blip")
	// 第三个 blip 属于视频（当前设计指向视频关系本身）。
	if len(audioBlip) >= 3 {
		r := relByID(rels, audioBlip[2][1])
		if r == nil {
			t.Errorf("video a:blip reference missing")
		} else if r.Type != opc.RelTypePrefix+"video" && r.Type != relImage {
			t.Errorf("video a:blip type = %q, want video or image", r.Type)
		}
	}
	for i, m := range reAudioRef.FindAllStringSubmatch(xml, -1) {
		assertRelType(t, rels, m[1], opc.RelTypePrefix+"audio", "a:audioFile")
		if i == 0 {
			audioRid := m[1]
			// p14:media 必须存在且指向 2007 media 关系（PowerPoint 打开要求）。
			p14 := reP14Media.FindStringSubmatch(xml)
			if p14 == nil {
				t.Fatalf("missing p14:media (PowerPoint cannot open poster audio):\n%s", xml)
			}
			assertRelType(t, rels, p14[1], relMedia2007, "p14:media")
			if p14[1] == audioRid {
				t.Errorf("p14:media must use a distinct media relationship, not the audio one")
			}
		}
	}
	for _, m := range reVideoRef.FindAllStringSubmatch(xml, -1) {
		assertRelType(t, rels, m[1], opc.RelTypePrefix+"video", "a:videoFile")
	}
	// 反例守门：绝不能出现错误命名空间的 p:videoFile 或 blip 指向音频。
	if strings.Contains(xml, "p:videoFile") {
		t.Errorf("slide uses wrong-namespace p:videoFile:\n%s", xml)
	}
	for _, m := range audioBlip {
		if r := relByID(rels, m[1]); r != nil && r.Type == opc.RelTypePrefix+"audio" {
			t.Errorf("a:blip must not reference the audio relationship (%s)", m[1])
		}
	}
}

// TestAudioTimingStructureByTrigger 断言两种触发方式的原生媒体播放结构。
// PowerPoint 需 p:cmd playFrom 才会播放，WPS 更需要它——两者都要有；
// 差异只在 seq/effect 的 nodeType 与是否带 onClick。
func TestAudioTimingStructureByTrigger(t *testing.T) {
	cases := []struct {
		name           string
		trigger        PlaybackTrigger
		seqNodeType    string
		effectNodeType string
		wantClick      bool
	}{
		{"auto", PlaybackOnSlideEnter, "mainSeq", "withEffect", false},
		{"click", PlaybackOnClick, "interactiveSeq", "clickEffect", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := audioDeck(t)
			defer p.Close()
			slides := SlidesOf(t, p)
			as, err := slides[0].AddAudio(context.Background(), BytesMedia(minimalMP3(), "audio/mpeg"), AudioSpec{
				TrackKey: "t-" + tc.name, Role: AudioRoleNarration,
				Source:   BytesMedia(minimalMP3(), "audio/mpeg"),
				Duration: NewOptional[time.Duration](2 * time.Second),
			})
			if err != nil {
				t.Fatalf("AddAudio: %v", err)
			}
			if err := as.SetPlayback(PlaybackSpec{Trigger: tc.trigger}); err != nil {
				t.Fatalf("SetPlayback: %v", err)
			}
			xb := slideXML(t, slides[0])
			for _, want := range []string{
				`nodeType="` + tc.seqNodeType + `"`,
				`nodeType="` + tc.effectNodeType + `"`,
				`presetClass="mediacall"`,
				`<p:cmd type="call" cmd="playFrom(0.0)">`,
				`<p:cMediaNode vol="80000">`,
				`<p:cond evt="onStopAudio"`,
			} {
				if !strings.Contains(xb, want) {
					t.Errorf("timing missing %q:\n%s", want, xb)
				}
			}
			if got := strings.Contains(xb, `evt="onClick"`); got != tc.wantClick {
				t.Errorf("onClick presence = %v, want %v:\n%s", got, tc.wantClick, xb)
			}
			if tc.wantClick && !strings.Contains(xb, `<p:sldTgt/>`) {
				t.Errorf("interactiveSeq should carry a sldTgt next-cond:\n%s", xb)
			}
		})
	}
}

// TestMediaDeckRoundTrip 端到端：生成含 audio(auto)+audio(click)+video 的
// 演示文稿 → 保存 → 重开 → Validate(0 error) → 形状分类正确。
func TestMediaDeckRoundTrip(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layouts, lerr := p.Layouts()
	if lerr != nil {
		t.Fatalf("Layouts: %v", lerr)
	}
	if err != nil {
		t.Fatalf("Layouts: %v", err)
	}
	sl, err := p.AddSlide(layouts[0])
	if err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	ctx := context.Background()

	as, err := sl.AddAudio(ctx, BytesMedia(minimalMP3(), "audio/mpeg"), AudioSpec{
		TrackKey: "rt-audio", Role: AudioRoleNarration,
		Source:   BytesMedia(minimalMP3(), "audio/mpeg"),
		Duration: NewOptional[time.Duration](2 * time.Second),
	})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	if err := as.SetPlayback(PlaybackSpec{Trigger: PlaybackOnSlideEnter}); err != nil {
		t.Fatalf("SetPlayback: %v", err)
	}
	if _, err := sl.AddVideo(ctx, BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "rt-video", Role: VideoRoleMain,
		X: 914400, Y: 3657600, Width: 5486400, Height: 3086100,
	}); err != nil {
		t.Fatalf("AddVideo: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "media-deck.pptx")
	if _, err := p.Save(ctx, tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("stat saved file: %v", err)
	}

	p2, err := Open(tmp)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer p2.Close()
	if rep := p2.Validate(ctx); rep.HasErrors() {
		for _, d := range rep.Diagnostics {
			t.Errorf("validate: %s: %s", d.Code, d.Message)
		}
	}
	slides2, err := p2.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	if len(slides2) == 0 {
		t.Fatal("no slides after reopen")
	}
	shapes, err := slides2[0].Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	var kinds []ShapeKind
	for _, sh := range shapes {
		kinds = append(kinds, sh.Kind())
	}
	if !containsKind(kinds, ShapeAudio) {
		t.Errorf("reopened slide lacks audio shape: %v", kinds)
	}
	if !containsKind(kinds, ShapeVideo) {
		t.Errorf("reopened slide lacks video shape: %v", kinds)
	}
}

func containsKind(ks []ShapeKind, want ShapeKind) bool {
	for _, k := range ks {
		if k == want {
			return true
		}
	}
	return false
}
