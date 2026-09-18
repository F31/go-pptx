package pptx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// 本文件锁定保存路径的两条调用方契约。二者此前都不成立且无测试覆盖：
//
//   - Save(nil, …) 直接 panic：Save 未做 nil 归一化就调用 ctx.Err()（nil 接口
//     解引用），而同族的 Write/Validate 都做了归一化 —— 同一门面三种口径。
//   - 第二次 Close 返回 ErrClosed：破坏 `defer p.Close()` 惯用法（调用方常在
//     显式 Close 之外再挂 defer，或把 Close 放进公共清理函数），且让"清理失败"
//     与"已经关过"无法区分。

func TestSaveNilContextDoesNotPanic(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	out := filepath.Join(t.TempDir(), "nil-ctx.pptx")
	var rep SaveReport
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Save(nil, ...) panicked: %v", r)
			}
		}()
		rep, err = p.Save(nil, out)
	}()
	if err != nil {
		t.Fatalf("Save(nil, ...): %v", err)
	}
	// 产物确实落盘（New() 出来的文档 revision 为 0，故不断言 Revision，
	// 只确认保存动作真的完成）。
	st, serr := os.Stat(out)
	if serr != nil {
		t.Fatalf("stat saved file: %v", serr)
	}
	if st.Size() == 0 {
		t.Fatal("saved file is empty")
	}
	_ = rep
}

func TestWriteNilContextMatchesSave(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()
	var buf writeSink
	if _, err := p.Write(nil, &buf); err != nil {
		t.Fatalf("Write(nil, ...): %v", err)
	}
	if buf.n == 0 {
		t.Fatal("Write produced no bytes")
	}
}

// writeSink 是丢弃写入内容的 io.Writer（只计数）。
type writeSink struct{ n int }

func (w *writeSink) Write(p []byte) (int, error) {
	w.n += len(p)
	return len(p), nil
}

func TestCloseIsIdempotent(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("first Close = %v, want nil", err)
	}
	// 第二次（以及 defer 里的重复调用）一律 nil：清理路径不应产生伪错误。
	if err := p.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil (idempotent)", err)
	}
	// 已关闭语义不变：其它方法仍返回 ErrClosed。
	if _, err := p.Save(context.Background(), filepath.Join(t.TempDir(), "closed.pptx")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Save after Close = %v, want ErrClosed", err)
	}
}
