package pptx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// minimalMP3 / minimalWAV / audioDeck / slideXML 由 audio_test.go 与
// text_test.go 提供。

func TestSetPlayback_AppendsTiming(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "sp", Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](time.Second)})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	if err := as.SetPlayback(PlaybackSpec{Trigger: PlaybackOnSlideEnter, StartDelay: 1500 * time.Millisecond}); err != nil {
		t.Fatalf("SetPlayback: %v", err)
	}
	xb := slideXML(t, slides[0])
	for _, want := range []string{"<p:timing>", "spTgt", `delay="1500"`, "cMediaNode"} {
		if !strings.Contains(xb, want) {
			t.Errorf("slide XML missing %q:\n%s", want, xb)
		}
	}
	// 生成的 XML 必须可再解析（结构合法）。
	if _, err := xmlstore.Index([]byte(xb)); err != nil {
		t.Fatalf("re-parse slide: %v", err)
	}
}

func TestSetPlayback_IdempotentRewrite(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "idem", Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](time.Second)})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := as.SetPlayback(PlaybackSpec{Trigger: PlaybackOnSlideEnter}); err != nil {
			t.Fatalf("SetPlayback #%d: %v", i, err)
		}
	}
	xb := slideXML(t, slides[0])
	// 只能有一个 p:timing。
	if n := strings.Count(xb, "<p:timing>"); n != 1 {
		t.Errorf("p:timing count = %d, want 1", n)
	}
	if _, err := xmlstore.Index([]byte(xb)); err != nil {
		t.Fatalf("re-parse slide: %v", err)
	}
}

func TestSetPlayback_ConflictOnComplexTiming(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	s := slides[0]
	// 在 p:sld 根级注入含 p:anim（非白名单）的复杂 timing。
	doc, err := p.docOf(s.part)
	if err != nil {
		t.Fatalf("docOf: %v", err)
	}
	root := doc.Root()
	complexFrag := `<p:timing><p:tnLst><p:par><p:cTn id="1"><p:childTnLst><p:anim/></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`
	ap, err := xmlstore.AppendChild(root, []byte(complexFrag))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{ap})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := p.stagePatch(s.part, out); err != nil {
		t.Fatalf("patch: %v", err)
	}
	p.commit()

	mp3 := minimalMP3()
	as, err := s.AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "sp2", Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](time.Second)})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	if err := as.SetPlayback(PlaybackSpec{Trigger: PlaybackOnSlideEnter}); !errors.Is(err, ErrTimingConflict) {
		t.Errorf("err = %v, want ErrTimingConflict", err)
	}
}

func TestSetAdvanceAfter_CreatesTransition(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	if err := slides[0].SetAdvanceAfter(2 * time.Second); err != nil {
		t.Fatalf("SetAdvanceAfter: %v", err)
	}
	xb := slideXML(t, slides[0])
	if !strings.Contains(xb, `advTm="2000"`) {
		t.Errorf("slide XML missing advTm=2000:\n%s", xb)
	}
	// 二次设置更新而非重复追加。
	if err := slides[0].SetAdvanceAfter(3 * time.Second); err != nil {
		t.Fatalf("SetAdvanceAfter #2: %v", err)
	}
	xb = slideXML(t, slides[0])
	if strings.Count(xb, "<p:transition") != 1 || !strings.Contains(xb, `advTm="3000"`) {
		t.Errorf("transition not updated in place:\n%s", xb)
	}
}

func TestUpsertNarration_ExistingTrackKey(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	src := BytesMedia(mp3, "audio/mpeg")
	spec := AudioSpec{TrackKey: "up", Source: src, Role: AudioRoleNarration,
		Duration: NewOptional[time.Duration](time.Second)}
	if _, err := slides[0].AddAudio(context.Background(), src, spec); err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	// 同字节 upsert → unchanged + Version=2。
	as, action, err := slides[0].UpsertNarration(context.Background(), src, spec,
		PlaybackSpec{Trigger: PlaybackOnSlideEnter})
	if err != nil {
		t.Fatalf("UpsertNarration: %v", err)
	}
	if action != "unchanged" {
		t.Errorf("action = %q, want unchanged", action)
	}
	if as.Profile().Version != 2 {
		t.Errorf("version = %d, want 2", as.Profile().Version)
	}
	// 不同字节 upsert → updated + Version=3。
	other := BytesMedia(minimalWAV(1600), "audio/wav")
	spec2 := AudioSpec{TrackKey: "up", Source: other, Role: AudioRoleNarration,
		Duration: NewOptional[time.Duration](100 * time.Millisecond)}
	as, action, err = slides[0].UpsertNarration(context.Background(), other, spec2,
		PlaybackSpec{Trigger: PlaybackOnSlideEnter})
	if err != nil {
		t.Fatalf("UpsertNarration #2: %v", err)
	}
	if action != "updated" {
		t.Errorf("action = %q, want updated", action)
	}
	if as.Profile().Version != 3 {
		t.Errorf("version = %d, want 3", as.Profile().Version)
	}
}

func TestPlanTimingSync_AT06Formula(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	a, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "a", Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](10 * time.Second)})
	if err != nil {
		t.Fatalf("AddAudio a: %v", err)
	}
	_ = a
	b, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "b", Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](8 * time.Second)})
	if err != nil {
		t.Fatalf("AddAudio b: %v", err)
	}
	// b 起点 6s：Ej = max(0+10, 6+8) = 14s；尾 padding 1s → 15s。
	if err := b.SetPlayback(PlaybackSpec{Trigger: PlaybackOnSlideEnter, StartDelay: 6 * time.Second}); err != nil {
		t.Fatalf("SetPlayback b: %v", err)
	}
	plan, err := p.PlanTimingSync(context.Background(), TimingSyncOptions{TailPadding: time.Second})
	if err != nil {
		t.Fatalf("PlanTimingSync: %v", err)
	}
	if len(plan.PageJumps) != 1 {
		t.Fatalf("page jumps = %d, want 1 (skipped=%v)", len(plan.PageJumps), plan.Skipped)
	}
	if got := plan.PageJumps[0].AdvanceAfter; got != 15*time.Second {
		t.Errorf("AdvanceAfter = %v, want 15s", got)
	}
	// 应用：slide XML 出现 advTm="15000"。
	rep, err := p.ApplyTimingPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("ApplyTimingPlan: %v", err)
	}
	if rep.Applied != 1 {
		t.Errorf("applied = %d, want 1", rep.Applied)
	}
	if xb := slideXML(t, slides[0]); !strings.Contains(xb, `advTm="15000"`) {
		t.Errorf("slide XML missing advTm=15000:\n%s", xb)
	}
}

func TestPlanTimingSync_UnknownDurationFail(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	// 显式 0 时长（调用方"提供"了 0）：Plan 阶段按未知拒绝（AT-07）。
	if _, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "z", Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](0)}); err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	_, err := p.PlanTimingSync(context.Background(), TimingSyncOptions{TailPadding: time.Second})
	if !errors.Is(err, ErrDurationUnknown) {
		t.Errorf("err = %v, want ErrDurationUnknown", err)
	}
	// Skip 策略：不报错，该页跳过。
	plan, err := p.PlanTimingSync(context.Background(), TimingSyncOptions{
		TailPadding:     time.Second,
		UnknownDuration: UnknownDurationSkip,
	})
	if err != nil {
		t.Fatalf("Plan(skip): %v", err)
	}
	if len(plan.PageJumps) != 0 {
		t.Errorf("page jumps = %d, want 0", len(plan.PageJumps))
	}
}

func TestPlanTimingSync_NoTracksReturnsPlan(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	plan, err := p.PlanTimingSync(context.Background(), TimingSyncOptions{TailPadding: time.Second})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.PageJumps) != 0 {
		t.Errorf("page jumps = %d, want 0", len(plan.PageJumps))
	}
}

func TestApplyTimingPlan_PlanRevisionMismatch(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	// 旧 revision 提交一个 plan，然后操作文档使 revision 变化。
	plan, err := p.PlanTimingSync(context.Background(), TimingSyncOptions{TailPadding: time.Second})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	// 修改文档（触发 revision+1）。
	cur, err := p.partBytes("/ppt/presentation.xml")
	if err != nil {
		t.Fatalf("partBytes: %v", err)
	}
	if err := p.stagePatch("/ppt/presentation.xml", append(append([]byte(nil), cur...), []byte("<!-- touched -->")...)); err != nil {
		t.Fatalf("stagePatch: %v", err)
	}
	p.commit()
	_, err = p.ApplyTimingPlan(context.Background(), plan)
	if !errors.Is(err, ErrConcurrentModification) {
		t.Errorf("err = %v, want ErrConcurrentModification", err)
	}
}

func TestSlideByID(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	if len(slides) == 0 {
		t.Fatal("no slides")
	}
	id := slides[0].ID()
	got, err := p.SlideByID(id)
	if err != nil || got.ID() != id {
		t.Errorf("got=%v err=%v", got, err)
	}
	_, err = p.SlideByID(999999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// 编译期确认 opc 常量在测试中可用（fixture 构造依赖）。
var _ = opc.RelSlide
