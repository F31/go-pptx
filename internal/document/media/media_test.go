package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/go-pptx/v2/internal/errs"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 5)), nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

func TestFileMedia(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.bin")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := FileMedia(path)
	if m.DeclaredType() != "" {
		t.Errorf("DeclaredType = %q", m.DeclaredType())
	}
	rc, err := m.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "hello" {
		t.Errorf("data = %q", got)
	}
	// 不存在的路径：构造不算错，Open 才报错。
	if _, err := FileMedia(filepath.Join(dir, "missing")).Open(context.Background()); err == nil {
		t.Error("missing file should error on Open")
	}
	// ctx 取消。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Open(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled Open err = %v", err)
	}
}

func TestBytesMediaAndFuncMedia(t *testing.T) {
	data := []byte("abc")
	m := BytesMedia(data, "  image/png ")
	data[0] = 'z' // 构造后改写不影响
	rc, err := m.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "abc" {
		t.Errorf("copy semantics broken: %q", got)
	}
	if m.DeclaredType() != "image/png" {
		t.Errorf("DeclaredType = %q", m.DeclaredType())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Open(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled bytes Open err = %v", err)
	}

	fm := FuncMedia(func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("fn")), nil
	}, "text/plain")
	if fm.DeclaredType() != "text/plain" {
		t.Errorf("func DeclaredType = %q", fm.DeclaredType())
	}
	rc2, err := fm.Open(context.Background())
	if err != nil {
		t.Fatalf("func Open: %v", err)
	}
	b, _ := io.ReadAll(rc2)
	rc2.Close()
	if string(b) != "fn" {
		t.Errorf("func data = %q", b)
	}
}

func TestReaderMedia(t *testing.T) {
	if _, err := ReaderMedia(nil, ""); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("nil reader err = %v", err)
	}
	m, err := ReaderMedia(strings.NewReader("data"), "")
	if err != nil {
		t.Fatalf("ReaderMedia: %v", err)
	}
	got, err := ReadMedia(context.Background(), m)
	if err != nil || string(got) != "data" {
		t.Fatalf("ReadMedia = %q, %v", got, err)
	}
}

func TestReadMediaErrors(t *testing.T) {
	if _, err := ReadMedia(context.Background(), nil); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("nil src err = %v", err)
	}
	// Open 返回错误。
	if _, err := ReadMedia(context.Background(), &funcMedia{open: func(context.Context) (io.ReadCloser, error) {
		return nil, errors.New("boom")
	}}); err == nil {
		t.Error("Open error should propagate")
	}
	// Open 返回 nil 流。
	if _, err := ReadMedia(context.Background(), &funcMedia{open: func(context.Context) (io.ReadCloser, error) {
		return nil, nil
	}}); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("nil stream err = %v", err)
	}
	// ctx 为 nil 时回退 Background。
	m, _ := ReaderMedia(strings.NewReader("x"), "")
	if _, err := ReadMedia(nil, m); err != nil {
		t.Errorf("nil ctx: %v", err)
	}
}

func TestReadBounded(t *testing.T) {
	got, err := ReadBounded(context.Background(), strings.NewReader("hello"), 100)
	if err != nil || string(got) != "hello" {
		t.Fatalf("ReadBounded = %q, %v", got, err)
	}
	if _, err := ReadBounded(context.Background(), strings.NewReader(strings.Repeat("x", 20)), 10); !errors.Is(err, errs.ErrLimitExceeded) {
		t.Errorf("limit err = %v", err)
	}
	// 读取错误传播。
	if _, err := ReadBounded(context.Background(), errReader{}, 100); err == nil {
		t.Error("read error should propagate")
	}
	// ctx 取消。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadBounded(ctx, strings.NewReader("x"), 100); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled err = %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func TestSniffAndProbeImage(t *testing.T) {
	// PNG。
	k, err := SniffImage(pngBytes(t))
	if err != nil || k.CT != CTImagePNG || k.Ext != "png" || k.Width != 2 || k.Hgt != 3 || k.ByteSize == 0 {
		t.Fatalf("png kind = %+v err=%v", k, err)
	}
	// JPEG。
	jk, err := SniffImage(jpegBytes(t))
	if err != nil || jk.CT != CTImageJPEG || jk.Ext != "jpg" || jk.Width != 4 || jk.Hgt != 5 {
		t.Fatalf("jpeg kind = %+v err=%v", jk, err)
	}
	// 损坏 PNG 魔数。
	if _, err := SniffImage(append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0x00, 0x01)); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("corrupt png err = %v", err)
	}
	// 损坏 JPEG 魔数。
	if _, err := SniffImage([]byte{0xff, 0xd8, 0xff, 0x00}); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("corrupt jpeg err = %v", err)
	}
	// 不支持的格式。
	if _, err := SniffImage([]byte("GIF89a...")); !errors.Is(err, errs.ErrUnsupportedFormat) {
		t.Errorf("unsupported err = %v", err)
	}
	// ProbeImage：声明不匹配。
	if _, err := ProbeImage(pngBytes(t), "image/jpeg"); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("mismatch err = %v", err)
	}
	// 声明匹配（含 jpg↔jpeg 等价）。
	if _, err := ProbeImage(jpegBytes(t), "image/jpg"); err != nil {
		t.Errorf("jpg alias should match: %v", err)
	}
	// 空声明。
	if _, err := ProbeImage(pngBytes(t), ""); err != nil {
		t.Errorf("empty declared: %v", err)
	}
}

func TestImageTypeEqual(t *testing.T) {
	if !ImageTypeEqual("IMAGE/JPG", "image/jpeg") {
		t.Error("jpg/jpeg alias should be equal")
	}
	if ImageTypeEqual("image/png", "image/jpeg") {
		t.Error("png/jpeg should differ")
	}
	if !ImageTypeEqual(" image/png ", "image/png") {
		t.Error("trim should apply")
	}
}
