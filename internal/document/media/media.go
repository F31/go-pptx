// Package media 是 v2.0 域层「媒体」垂直切片的起步：媒体输入契约
// （MediaSource + 文件/字节/工厂/reader 适配器）与图片探测原语。
//
// 句柄/编排（AudioShape/VideoShape/PictureShape 与 *Presentation/
// *Slide 的 Add*/plan* 方法）仍留根包门面；本包只承载无状态纯函数与
// 输入抽象。禁止本包反向 import 根包。
package media

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"

	"github.com/F31/go-pptx/internal/errs"
	"github.com/F31/go-pptx/internal/opc"
)

// MaxStagingBytes 限制单次媒体暂存复制的上限（512 MiB）。超过返回
// ErrLimitExceeded。
const MaxStagingBytes = int64(512 << 20)

// MediaSource 是一次媒体输入的抽象（方案 §21.1）。
//
// Open 每次调用返回一个新流（可重复打开）；消费方负责关闭。实现必须
// 是幂等的数据源，不得消耗一次性状态。DeclaredType 返回调用方声明的
// MIME 类型（可为空，探测以内容签名为准）。
type MediaSource interface {
	Open(ctx context.Context) (io.ReadCloser, error)
	DeclaredType() string
}

// fileMedia 是磁盘文件源：构造时不执行 I/O。
type fileMedia struct {
	path string
}

func (m *fileMedia) Open(ctx context.Context) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.Open(m.path)
}

func (m *fileMedia) DeclaredType() string { return "" }

// FileMedia 以磁盘路径构造媒体源（不做 I/O，方案 §28）。
func FileMedia(path string) MediaSource {
	return &fileMedia{path: path}
}

// bytesMedia 是内存字节源（byte slice 适配器）。
type bytesMedia struct {
	data []byte
	ct   string
}

func (m *bytesMedia) Open(ctx context.Context) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

func (m *bytesMedia) DeclaredType() string { return m.ct }

// BytesMedia 以内存字节构造媒体源。字节在构造时拷贝。
func BytesMedia(data []byte, contentType string) MediaSource {
	return &bytesMedia{data: append([]byte(nil), data...), ct: strings.TrimSpace(contentType)}
}

// funcMedia 是调用方工厂适配器。
type funcMedia struct {
	open func(context.Context) (io.ReadCloser, error)
	ct   string
}

func (m *funcMedia) Open(ctx context.Context) (io.ReadCloser, error) { return m.open(ctx) }
func (m *funcMedia) DeclaredType() string                            { return m.ct }

// FuncMedia 以调用方提供的打开函数构造媒体源。
func FuncMedia(open func(context.Context) (io.ReadCloser, error), contentType string) MediaSource {
	return &funcMedia{open: open, ct: strings.TrimSpace(contentType)}
}

// ReaderMedia 从 r 有界复制到内存并返回媒体源；读取在返回前完成。
func ReaderMedia(r io.Reader, contentType string) (MediaSource, error) {
	if r == nil {
		return nil, errs.Annotate(errs.ErrInvalidArgument, "ReaderMedia")
	}
	data, err := ReadBounded(context.Background(), r, MaxStagingBytes)
	if err != nil {
		return nil, err
	}
	return BytesMedia(data, contentType), nil
}

// ImageKind 描述探测确认的图片格式。
type ImageKind struct {
	CT         string // 规范 MIME：image/png / image/jpeg
	Ext        string // 媒体 Part 扩展名：png / jpg
	Width, Hgt int    // 像素尺寸（DecodeConfig）
	ByteSize   int64
}

// ReadMedia 打开媒体源并在返回前完成有界复制（ctx 取消可中断）。
func ReadMedia(ctx context.Context, src MediaSource) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if src == nil {
		return nil, errs.Annotate(errs.ErrInvalidArgument, "media source")
	}
	rc, err := src.Open(ctx)
	if err != nil {
		return nil, err
	}
	if rc == nil {
		return nil, errs.Annotate(errs.ErrInvalidArgument, "media source open")
	}
	defer rc.Close()
	return ReadBounded(ctx, rc, MaxStagingBytes)
}

// ReadBounded 复制 r 直到上限；超限返回 ErrLimitExceeded（不继续读）。
func ReadBounded(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	chunk := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(chunk)
		if n > 0 {
			total += int64(n)
			if total > limit {
				return nil, errs.Annotate(errs.ErrLimitExceeded, "media staging")
			}
			buf.Write(chunk[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// ProbeImage 通过签名与结构检查确认图片格式与像素尺寸。
//
//   - 非 PNG/JPEG 魔数 → ErrUnsupportedFormat（含 SVG/GIF/EMF 等未支持类型）；
//   - 魔数匹配但结构损坏（DecodeConfig 失败）→ ErrInvalidArgument；
//   - declared 非空且与探测出的规范 MIME 不一致 → ErrInvalidArgument。
func ProbeImage(data []byte, declared string) (ImageKind, error) {
	kind, err := SniffImage(data)
	if err != nil {
		return ImageKind{}, err
	}
	if declared != "" {
		if !ImageTypeEqual(declared, kind.CT) {
			return ImageKind{}, &errs.OperationError{
				Op:      "media probe",
				Message: fmt.Sprintf("declared type %q does not match detected %q", declared, kind.CT),
				Err:     errs.ErrInvalidArgument,
			}
		}
	}
	return kind, nil
}

// SniffImage 检测魔数并解析尺寸；只读取头部所需字节。
func SniffImage(data []byte) (ImageKind, error) {
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return ImageKind{}, &errs.OperationError{
				Op: "media probe", Message: "invalid PNG data: " + err.Error(), Err: errs.ErrInvalidArgument,
			}
		}
		return ImageKind{CT: "image/png", Ext: "png", Width: cfg.Width, Hgt: cfg.Height, ByteSize: int64(len(data))}, nil
	}
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}) {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return ImageKind{}, &errs.OperationError{
				Op: "media probe", Message: "invalid JPEG data: " + err.Error(), Err: errs.ErrInvalidArgument,
			}
		}
		return ImageKind{CT: "image/jpeg", Ext: "jpg", Width: cfg.Width, Hgt: cfg.Height, ByteSize: int64(len(data))}, nil
	}
	return ImageKind{}, &errs.OperationError{
		Op:      "media probe",
		Message: "unsupported media format (IMAGE-01 supports PNG/JPEG only)",
		Err:     errs.ErrUnsupportedFormat,
	}
}

// ImageTypeEqual 比较 MIME 的等价形式（大小写、image/jpg ↔ image/jpeg）。
func ImageTypeEqual(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "image/jpg" {
		a = "image/jpeg"
	}
	if b == "image/jpg" {
		b = "image/jpeg"
	}
	return a == b
}

// IMAGE-01 支持类型的常量表辅助。
const (
	// RelImage 是图片关系类型后缀。
	RelImage = opc.RelTypePrefix + "image"
	// CTImagePNG 是 PNG 的规范 MIME。
	CTImagePNG = "image/png"
	// CTImageJPEG 是 JPEG 的规范 MIME。
	CTImageJPEG = "image/jpeg"
)
