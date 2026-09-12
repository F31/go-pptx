package pptx

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
)

// 本文件实现 IMAGE-01 的媒体输入与所有权契约（方案 §21.1 的图片子集）：
// MediaSource 每次 Open 返回新流、组件负责关闭；提供文件、byte slice 与
// 调用方工厂适配器；从 io.Reader 添加的便捷方法在返回前完成有界复制。
// 文件源在修改事务完成前被完整读取（暂存校验），不让后续源文件变化
// 影响 Save。
//
// 媒体类型通过签名与结构检查确认（PNG/JPEG 魔数 + DecodeConfig），扩展名
// 与调用方声明的 MIME 只作辅助；不一致返回错误。

// maxStagingBytes 限制单次媒体暂存复制的上限（镜像 opc 预算的媒体初值；
// 调用方配置预算后的精确额度随版本接入）。超过返回 ErrLimitExceeded。
const maxStagingBytes = int64(512 << 20) // 512 MiB

// MediaSource 是一次媒体输入的抽象（方案 §21.1）。
//
// Open 每次调用返回一个新流（可重复打开）；消费方负责关闭。实现必须
// 是幂等的数据源，不得消耗一次性状态。DeclaredType 返回调用方声明的
// MIME 类型（可为空，探测以内容签名为准）。
type MediaSource interface {
	Open(ctx context.Context) (io.ReadCloser, error)
	DeclaredType() string
}

// fileMedia 是磁盘文件源：构造时不执行 I/O；输入不存在等错误在消费源
// 的 API 中返回。每次 Open 打开新文件句柄。
type fileMedia struct {
	path string
}

func (m *fileMedia) Open(ctx context.Context) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(m.path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (m *fileMedia) DeclaredType() string { return "" }

// FileMedia 以磁盘路径构造媒体源（不做 I/O，方案 §28）；输入不存在等
// 错误在消费源的 API 中返回。媒体类型以内容签名为准（无需声明）。
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

// BytesMedia 以内存字节构造媒体源。字节在构造时拷贝（消费方后续改写
// 传入切片不影响媒体内容）；contentType 可空。
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

// FuncMedia 以调用方提供的打开函数构造媒体源。open 每次调用须返回
// 新流；返回 nil 流视为错误。
func FuncMedia(open func(context.Context) (io.ReadCloser, error), contentType string) MediaSource {
	return &funcMedia{open: open, ct: strings.TrimSpace(contentType)}
}

// ReaderMedia 从 r 有界复制到内存并返回媒体源；读取在返回前完成，
// 后续 r 的变化不影响媒体内容。contentType 可空。
func ReaderMedia(r io.Reader, contentType string) (MediaSource, error) {
	if r == nil {
		return nil, Annotate(ErrInvalidArgument, "ReaderMedia")
	}
	data, err := readBounded(context.Background(), r, maxStagingBytes)
	if err != nil {
		return nil, err
	}
	return BytesMedia(data, contentType), nil
}

// imageKind 描述探测确认的图片格式。
type imageKind struct {
	ct         string // 规范 MIME：image/png / image/jpeg
	ext        string // 媒体 Part 扩展名：png / jpg
	width, hgt int    // 像素尺寸（DecodeConfig）
	byteSize   int64
}

// readMedia 打开媒体源并在返回前完成有界复制（ctx 取消可中断）。
func readMedia(ctx context.Context, src MediaSource) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if src == nil {
		return nil, Annotate(ErrInvalidArgument, "media source")
	}
	rc, err := src.Open(ctx)
	if err != nil {
		return nil, err
	}
	if rc == nil {
		return nil, Annotate(ErrInvalidArgument, "media source open")
	}
	defer rc.Close()
	return readBounded(ctx, rc, maxStagingBytes)
}

// readBounded 复制 r 直到上限；超限返回 ErrLimitExceeded（不继续读）。
func readBounded(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
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
				return nil, Annotate(ErrLimitExceeded, "media staging")
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

// probeImage 通过签名与结构检查确认图片格式与像素尺寸。
//
//   - 非 PNG/JPEG 魔数 → ErrUnsupportedFormat（含 SVG/GIF/EMF 等未支持类型）；
//   - 魔数匹配但结构损坏（DecodeConfig 失败）→ ErrInvalidArgument；
//   - declared 非空且与探测出的规范 MIME 不一致 → ErrInvalidArgument。
func probeImage(data []byte, declared string) (imageKind, error) {
	kind, err := sniffImage(data)
	if err != nil {
		return imageKind{}, err
	}
	if declared != "" {
		if !imageTypeEqual(declared, kind.ct) {
			return imageKind{}, &OperationError{
				Op:      "media probe",
				Message: fmt.Sprintf("declared type %q does not match detected %q", declared, kind.ct),
				Err:     ErrInvalidArgument,
			}
		}
	}
	return kind, nil
}

// sniffImage 检测魔数并解析尺寸；只读取头部所需字节。
func sniffImage(data []byte) (imageKind, error) {
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return imageKind{}, &OperationError{
				Op: "media probe", Message: "invalid PNG data: " + err.Error(), Err: ErrInvalidArgument,
			}
		}
		return imageKind{ct: "image/png", ext: "png", width: cfg.Width, hgt: cfg.Height, byteSize: int64(len(data))}, nil
	}
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}) {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return imageKind{}, &OperationError{
				Op: "media probe", Message: "invalid JPEG data: " + err.Error(), Err: ErrInvalidArgument,
			}
		}
		return imageKind{ct: "image/jpeg", ext: "jpg", width: cfg.Width, hgt: cfg.Height, byteSize: int64(len(data))}, nil
	}
	return imageKind{}, &OperationError{
		Op:      "media probe",
		Message: "unsupported media format (IMAGE-01 supports PNG/JPEG only)",
		Err:     ErrUnsupportedFormat,
	}
}

// imageTypeEqual 比较 MIME 的等价形式（大小写、image/jpg ↔ image/jpeg）。
func imageTypeEqual(a, b string) bool {
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

// imageCTAndExt 是 IMAGE-01 支持类型的常量表辅助。
const (
	relImage    = opc.RelTypePrefix + "image"
	ctImagePNG  = "image/png"
	ctImageJPEG = "image/jpeg"
)
