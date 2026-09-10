package render_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/F31/go-pptx"
	"github.com/F31/go-pptx/render"
)

// closeTracker 包装 io.ReadCloser，记录 Close 是否被调用。
type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error { c.closed = true; return nil }

// mockRenderer 实现 render.Renderer，记录调用顺序并返回可注入的错误。
type mockRenderer struct {
	caps render.RenderCapabilities
	seen []pptx.SlideID
	err  error
	// onRender 每页渲染前回调（用于注入页级错误）。
	onRender func(slideID pptx.SlideID) error
}

func (m *mockRenderer) Capabilities() render.RenderCapabilities { return m.caps }

func (m *mockRenderer) RenderSlide(_ context.Context, _ *pptx.Presentation, slideID pptx.SlideID, _ render.RenderOptions) (render.RenderedSlide, error) {
	if m.err != nil {
		return render.RenderedSlide{}, m.err
	}
	if m.onRender != nil {
		if err := m.onRender(slideID); err != nil {
			return render.RenderedSlide{}, err
		}
	}
	m.seen = append(m.seen, slideID)
	return render.RenderedSlide{
		SlideID: slideID, MIMEType: "image/png", Width: 100, Height: 56,
		Data: &closeTracker{Reader: strings.NewReader("png")},
	}, nil
}

// deckWithSlides 构造含 n 页的 Presentation。
func deckWithSlides(t *testing.T, n int) *pptx.Presentation {
	t.Helper()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	layouts, err := p.Layouts()
	if err != nil {
		p.Close()
		t.Fatalf("Layouts: %v", err)
	}
	if len(layouts) == 0 {
		p.Close()
		t.Fatal("no layouts")
	}
	for i := 0; i < n; i++ {
		if _, err := p.AddSlide(layouts[0]); err != nil {
			p.Close()
			t.Fatalf("AddSlide: %v", err)
		}
	}
	return p
}

func slideIDs(t *testing.T, p *pptx.Presentation) []pptx.SlideID {
	t.Helper()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	ids := make([]pptx.SlideID, 0, len(slides))
	for _, s := range slides {
		ids = append(ids, s.ID())
	}
	return ids
}

func TestRenderAll_IteratesAllSlides(t *testing.T) {
	p := deckWithSlides(t, 3)
	defer p.Close()
	ids := slideIDs(t, p)
	if len(ids) != 3 {
		t.Fatalf("want 3 slides, got %d", len(ids))
	}

	r := &mockRenderer{}
	var got []pptx.SlideID
	err := render.RenderAll(context.Background(), r, p, render.RenderOptions{}, func(rs render.RenderedSlide) error {
		got = append(got, rs.SlideID)
		return nil
	})
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("callbacks = %d, want 3", len(got))
	}
	for i := range ids {
		if got[i] != ids[i] {
			t.Fatalf("callback[%d] = %d, want %d (order must match Slides())", i, got[i], ids[i])
		}
	}
}

func TestRenderAll_AbortOnCallbackError(t *testing.T) {
	p := deckWithSlides(t, 3)
	defer p.Close()

	r := &mockRenderer{}
	want := errors.New("stop")
	calls := 0
	err := render.RenderAll(context.Background(), r, p, render.RenderOptions{}, func(render.RenderedSlide) error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("RenderAll err = %v, want %v", err, want)
	}
	if calls != 1 {
		t.Fatalf("callbacks = %d, want 1 (abort on first error)", calls)
	}
}

func TestRenderAll_AbortOnRenderError(t *testing.T) {
	p := deckWithSlides(t, 3)
	defer p.Close()

	want := errors.New("render failed")
	r := &mockRenderer{err: want}
	err := render.RenderAll(context.Background(), r, p, render.RenderOptions{}, func(render.RenderedSlide) error {
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("RenderAll err = %v, want %v", err, want)
	}
}

func TestRenderAll_ClosesData(t *testing.T) {
	p := deckWithSlides(t, 2)
	defer p.Close()

	r := &mockRenderer{}
	var closed []bool
	err := render.RenderAll(context.Background(), r, p, render.RenderOptions{}, func(rs render.RenderedSlide) error {
		ct := rs.Data.(*closeTracker)
		closed = append(closed, ct.closed) // 回调内 Data 尚未被 Close
		return nil
	})
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	if len(closed) != 2 {
		t.Fatalf("callbacks = %d, want 2", len(closed))
	}
	for i, c := range closed {
		if c {
			t.Fatalf("data[%d] closed inside callback (should close after callback returns)", i)
		}
	}
}

func TestRenderAll_AbortOnCancelledContext(t *testing.T) {
	p := deckWithSlides(t, 1)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &mockRenderer{}
	err := render.RenderAll(ctx, r, p, render.RenderOptions{}, func(render.RenderedSlide) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RenderAll err = %v, want context.Canceled", err)
	}
	if len(r.seen) != 0 {
		t.Fatalf("rendered %d slides, want 0 (ctx already cancelled)", len(r.seen))
	}
}

func TestRendererCapabilities(t *testing.T) {
	caps := render.RenderCapabilities{Animation: true, FontSubstitution: true, HiddenSlides: false, OfficeFeatures: false}
	r := &mockRenderer{caps: caps}
	if got := r.Capabilities(); got != caps {
		t.Fatalf("Capabilities = %+v, want %+v", got, caps)
	}
}
