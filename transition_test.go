package pptx

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// transitionDeck 建一个单页演示文稿。
func transitionDeck(t *testing.T) *Presentation {
	t.Helper()
	return audioDeck(t)
}

// transitionNode 取页面的 p:transition 节点（与首个 p:* 子元素）。
func transitionNode(t *testing.T, p *Presentation) (*xmlstore.XMLDocument, *xmlstore.NodeRecord) {
	t.Helper()
	doc, err := p.docOf("/ppt/slides/slide1.xml")
	if err != nil {
		t.Fatalf("docOf: %v", err)
	}
	ids := doc.Elements(nsPresentationML, "transition")
	if len(ids) == 0 {
		return doc, nil
	}
	return doc, doc.Node(ids[0])
}

// transitionChildLocal 返回 transition 首个 p:* 子元素的 local 名。
func transitionChildLocal(t *testing.T, p *Presentation) string {
	t.Helper()
	doc, tr := transitionNode(t, p)
	if tr == nil {
		return ""
	}
	for _, cid := range tr.Children {
		c := doc.Node(cid)
		if c.Namespace == nsPresentationML {
			return c.Local()
		}
	}
	return ""
}

// slideBytes 取 slide1.xml 当前字节（已应用未提交修改）。
func slideBytes(t *testing.T, p *Presentation) []byte {
	t.Helper()
	doc, err := p.docOf("/ppt/slides/slide1.xml")
	if err != nil {
		t.Fatalf("docOf: %v", err)
	}
	out := make([]byte, len(doc.Original()))
	copy(out, doc.Original())
	return out
}

// TestTransition_NotFound 验证无 transition 容器时返回 ErrNotFound。
func TestTransition_NotFound(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.Transition(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Transition 无容器应返回 ErrNotFound，实际 %v", err)
	}
}

// TestTransition_None_NoChild 测试 SetTransition(none) 写入无子元素容器。
func TestTransition_None_NoChild(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetTransition(TransitionSpec{Type: TransitionNone}); err != nil {
		t.Fatalf("SetTransition(none): %v", err)
	}
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.Type != TransitionNone {
		t.Errorf("Type=%q want none", got.Type)
	}
	if got.Speed != SpeedFast {
		t.Errorf("Speed=%q want fast（默认）", got.Speed)
	}
	if !*got.AdvanceClick {
		t.Errorf("AdvanceClick=false want true（默认）")
	}
	if local := transitionChildLocal(t, p); local != "" {
		t.Errorf("none 容器不应有 p: 子元素，实际 %q", local)
	}
}

// TestTransition_Fade 测试 fade（含 throughBlack）写入读取。
func TestTransition_Fade(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	spec := TransitionSpec{
		Type:  TransitionFade,
		Speed: SpeedSlow,
		Fade:  FadeOptions{ThroughBlack: true},
	}
	if err := s.SetTransition(spec); err != nil {
		t.Fatalf("SetTransition(fade): %v", err)
	}
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.Type != TransitionFade {
		t.Errorf("Type=%q want fade", got.Type)
	}
	if got.Speed != SpeedSlow {
		t.Errorf("Speed=%q want slow", got.Speed)
	}
	if !got.Fade.ThroughBlack {
		t.Errorf("Fade.ThroughBlack=false want true")
	}
}

// TestTransition_PushDir 测试 push + Dir。
func TestTransition_PushDir(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetTransition(TransitionSpec{Type: TransitionPush, Dir: DirRight}); err != nil {
		t.Fatalf("SetTransition(push/r): %v", err)
	}
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.Type != TransitionPush || got.Dir != DirRight {
		t.Errorf("got Type=%q Dir=%q want push/r", got.Type, got.Dir)
	}
	if local := transitionChildLocal(t, p); local != "push" {
		t.Errorf("子元素=%q want push", local)
	}
}

// TestTransition_Split 测试 split 双重属性。
func TestTransition_Split(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	spec := TransitionSpec{
		Type:      TransitionSplit,
		SplitDir:  SplitOut,
		SplitAxis: AxisVert,
	}
	if err := s.SetTransition(spec); err != nil {
		t.Fatalf("SetTransition(split): %v", err)
	}
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.Type != TransitionSplit || got.SplitDir != SplitOut || got.SplitAxis != AxisVert {
		t.Errorf("split 读取不匹配：%+v", got)
	}
}

// TestTransition_Dissolve 测试 dissolve（无属性）。
func TestTransition_Dissolve(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetTransition(TransitionSpec{Type: TransitionDissolve}); err != nil {
		t.Fatalf("SetTransition(dissolve): %v", err)
	}
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.Type != TransitionDissolve {
		t.Errorf("Type=%q want dissolve", got.Type)
	}
	if local := transitionChildLocal(t, p); local != "dissolve" {
		t.Errorf("子元素=%q want dissolve", local)
	}
}

// TestTransition_AdvanceClickFalse 测试 advClick=0 写入。
func TestTransition_AdvanceClickFalse(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetTransition(TransitionSpec{Type: TransitionFade, AdvanceClick: BoolPtr(false)}); err != nil {
		t.Fatalf("SetTransition(fade click=false): %v", err)
	}
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.AdvanceClick == nil || *got.AdvanceClick {
		t.Errorf("AdvanceClick 应显式为 false")
	}
}

// TestTransition_PreservesAdvTm 测试 SetTransition 不覆盖 SetAdvanceAfter 写入的 advTm。
func TestTransition_PreservesAdvTm(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetAdvanceAfter(7500 * time.Millisecond); err != nil {
		t.Fatalf("SetAdvanceAfter: %v", err)
	}
	if err := s.SetTransition(TransitionSpec{Type: TransitionFade}); err != nil {
		t.Fatalf("SetTransition: %v", err)
	}
	data := slideBytes(t, p)
	if !bytes.Contains(data, []byte(`advTm="7500"`)) {
		t.Errorf("advTm 应保留 7500，字节片段：%s", string(data))
	}
}

// TestTransition_AllDirections 覆盖四方向写入读取 + parseDir 非法值。
func TestTransition_AllDirections(t *testing.T) {
	for _, dir := range []TransitionDir{DirLeft, DirRight, DirUp, DirDown} {
		p := transitionDeck(t)
		s := SlidesOf(t, p)[0]
		if err := s.SetTransition(TransitionSpec{Type: TransitionWipe, Dir: dir}); err != nil {
			p.Close()
			t.Fatalf("SetTransition(%s): %v", dir, err)
		}
		got, err := s.Transition()
		p.Close()
		if err != nil {
			t.Fatalf("Transition(%s): %v", dir, err)
		}
		if got.Dir != dir {
			t.Fatalf("dir = %q, want %q", got.Dir, dir)
		}
	}
	// 手动构造非法 dir 值读回 → parseDir 返回空。
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	hookup := []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`<p:transition spd="med"><p:wipe dir="diagonal"/></p:transition>` +
		`</p:sld>`)
	if err := p.stagePatch("/ppt/slides/slide1.xml", hookup); err != nil {
		t.Fatalf("stagePatch: %v", err)
	}
	p.commit()
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.Dir != "" {
		t.Fatalf("invalid dir parsed as %q, want empty", got.Dir)
	}
}

// TestTransition_ReplaceInPlace 测试二次 SetTransition 替换不重复创建。
func TestTransition_ReplaceInPlace(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetTransition(TransitionSpec{Type: TransitionFade}); err != nil {
		t.Fatalf("首次 SetTransition: %v", err)
	}
	if err := s.SetTransition(TransitionSpec{Type: TransitionPush, Dir: DirLeft}); err != nil {
		t.Fatalf("二次 SetTransition: %v", err)
	}
	data := slideBytes(t, p)
	occurrences := bytes.Count(data, []byte("<p:transition"))
	if occurrences != 1 {
		t.Errorf("应仅一个 p:transition，实际 %d 处", occurrences)
	}
	got, err := s.Transition()
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if got.Type != TransitionPush || got.Dir != DirLeft {
		t.Errorf("替换后类型/dir 不匹配：%+v", got)
	}
}

// TestTransition_RejectsUnknownType 测试未知类型返回 ErrInvalidArgument。
func TestTransition_RejectsUnknownType(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	err := s.SetTransition(TransitionSpec{Type: TransitionType("morph")})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("morph 应返回 ErrInvalidArgument，实际 %v", err)
	}
}

// TestTransition_RejectsUnknownDir 测试未知 dir 返回 ErrInvalidArgument。
func TestTransition_RejectsUnknownDir(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	err := s.SetTransition(TransitionSpec{Type: TransitionPush, Dir: TransitionDir("x")})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("未知 dir 应返回 ErrInvalidArgument，实际 %v", err)
	}
}

// TestTransition_RejectsUnknownSpeed 测试未知 spd 返回 ErrInvalidArgument。
func TestTransition_RejectsUnknownSpeed(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	err := s.SetTransition(TransitionSpec{Type: TransitionFade, Speed: TransitionSpeed("extreme")})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("未知 speed 应返回 ErrInvalidArgument，实际 %v", err)
	}
}

// TestTransition_RejectsP14Child 测试含 p14 子元素的容器整体拒绝。
func TestTransition_RejectsP14Child(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	// 手工注入含 p14 元素的容器（绕过 SetTransition 的白名单写入）。
	hookup := []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `" xmlns:p14="` + nsP14ML + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`<p:transition><p:morph/></p:transition>` +
		`</p:sld>`)
	if err := p.stagePatch("/ppt/slides/slide1.xml", hookup); err != nil {
		t.Fatalf("stagePatch: %v", err)
	}
	p.commit()
	_, err := s.Transition()
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("p14 子元素应返回 ErrUnsupportedEdit，实际 %v", err)
	}
}

// TestTransition_Remove 测试 RemoveTransition 删除容器。
func TestTransition_Remove(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetTransition(TransitionSpec{Type: TransitionFade}); err != nil {
		t.Fatalf("SetTransition: %v", err)
	}
	if err := s.RemoveTransition(); err != nil {
		t.Fatalf("RemoveTransition: %v", err)
	}
	if _, err := s.Transition(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("移除后应返回 ErrNotFound，实际 %v", err)
	}
	data := slideBytes(t, p)
	if bytes.Contains(data, []byte("<p:transition")) {
		t.Errorf("移除后不应含 p:transition：%s", string(data))
	}
}

// TestTransition_RemoveNoOp 测试无 transition 时 RemoveTransition 为 no-op。
func TestTransition_RemoveNoOp(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.RemoveTransition(); err != nil {
		t.Fatalf("RemoveTransition no-op 应成功，实际 %v", err)
	}
}

// TestTransition_CoexistsWithTiming 测试 transition 与 p:timing 共存不破坏 timing 树。
func TestTransition_CoexistsWithTiming(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	timingFrag := `<p:timing><p:tnLst><p:par><p:cTn id="1"/></p:par></p:tnLst></p:timing>`
	hookup := []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		timingFrag +
		`</p:sld>`)
	if err := p.stagePatch("/ppt/slides/slide1.xml", hookup); err != nil {
		t.Fatalf("stagePatch timing: %v", err)
	}
	p.commit()
	if err := s.SetTransition(TransitionSpec{Type: TransitionFade}); err != nil {
		t.Fatalf("SetTransition: %v", err)
	}
	if !s.HasTiming() {
		t.Errorf("SetTransition 不应破坏 p:timing")
	}
	data := slideBytes(t, p)
	if !strings.Contains(string(data), timingFrag) {
		t.Errorf("p:timing 字节应不变")
	}
}

// TestTransition_SaveRoundTrip 测试保存往返。
func TestTransition_SaveRoundTrip(t *testing.T) {
	p := transitionDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if err := s.SetTransition(TransitionSpec{
		Type:  TransitionPush,
		Speed: SpeedMed,
		Dir:   DirUp,
	}); err != nil {
		t.Fatalf("SetTransition: %v", err)
	}
	buf := saveBytes(t, p)
	p2, err := OpenReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer p2.Close()
	s2 := SlidesOf(t, p2)[0]
	got, err := s2.Transition()
	if err != nil {
		t.Fatalf("往返后 Transition: %v", err)
	}
	if got.Type != TransitionPush || got.Speed != SpeedMed || got.Dir != DirUp {
		t.Errorf("往返后 spec 不匹配：%+v", got)
	}
}

// TestTransition_ClosedDocument 测试 close 后返回 ErrClosed。
func TestTransition_ClosedDocument(t *testing.T) {
	p := transitionDeck(t)
	s := SlidesOf(t, p)[0]
	p.Close()
	if _, err := s.Transition(); !errors.Is(err, ErrClosed) {
		t.Errorf("Close 后 Transition 应返回 ErrClosed，实际 %v", err)
	}
	if err := s.SetTransition(TransitionSpec{Type: TransitionFade}); !errors.Is(err, ErrClosed) {
		t.Errorf("Close 后 SetTransition 应返回 ErrClosed，实际 %v", err)
	}
	if err := s.RemoveTransition(); !errors.Is(err, ErrClosed) {
		t.Errorf("Close 后 RemoveTransition 应返回 ErrClosed，实际 %v", err)
	}
}
