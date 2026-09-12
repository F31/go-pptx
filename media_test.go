package pptx

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFuncMediaReadMedia(t *testing.T) {
	calls := 0
	src := FuncMedia(func(ctx context.Context) (io.ReadCloser, error) {
		calls++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return io.NopCloser(strings.NewReader("payload")), nil
	}, " image/png ")
	if got := src.DeclaredType(); got != "image/png" {
		t.Fatalf("DeclaredType = %q", got)
	}
	b, err := readMedia(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "payload" || calls != 1 {
		t.Fatalf("read = %q calls=%d", b, calls)
	}
}

func TestFuncMediaNilStreamIsInvalid(t *testing.T) {
	src := FuncMedia(func(context.Context) (io.ReadCloser, error) {
		return nil, nil
	}, "")
	_, err := readMedia(context.Background(), src)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestReaderMediaCopiesInput(t *testing.T) {
	r := strings.NewReader("abc")
	src, err := ReaderMedia(r, " image/jpeg ")
	if err != nil {
		t.Fatal(err)
	}
	if got := src.DeclaredType(); got != "image/jpeg" {
		t.Fatalf("DeclaredType = %q", got)
	}
	b, err := readMedia(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "abc" {
		t.Fatalf("read = %q", b)
	}
}

func TestReaderMediaRejectsNil(t *testing.T) {
	_, err := ReaderMedia(nil, "")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestReadBoundedLimitAndContext(t *testing.T) {
	_, err := readBounded(context.Background(), strings.NewReader("abcd"), 3)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("limit err = %v, want ErrLimitExceeded", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = readBounded(ctx, strings.NewReader("abcd"), 10)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx err = %v, want context.Canceled", err)
	}
}
