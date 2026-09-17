package pptx

import (
	"context"
	"io"

	"github.com/F31/go-pptx/v2/internal/document/media"
)

// 本文件是 IMAGE-01 媒体输入契约的根包接线：v2.0 已将实现迁入
// internal/document/media，此处以 alias 暴露类型/常量并对构造器与纯
// 原语做薄委托（消费方调用点零改动）。
//
// MediaSource 每次 Open 返回新流、组件负责关闭；提供文件、byte slice 与
// 调用方工厂适配器；从 io.Reader 添加的便捷方法在返回前完成有界复制。
// 媒体类型通过签名与结构检查确认（PNG/JPEG 魔数 + DecodeConfig）。

// MediaSource 是一次媒体输入的抽象（方案 §21.1）。
type MediaSource = media.MediaSource

// imageKind 描述探测确认的图片格式（alias 到 internal/document/media）。
type imageKind = media.ImageKind

// 媒体暂存上限与 IMAGE-01 支持类型常量（alias 到 internal/document/media）。
const (
	maxStagingBytes = media.MaxStagingBytes
	relImage        = media.RelImage
	ctImagePNG      = media.CTImagePNG
	ctImageJPEG     = media.CTImageJPEG
)

// FileMedia 以磁盘路径构造媒体源（不做 I/O，方案 §28）；输入不存在等
// 错误在消费源的 API 中返回。媒体类型以内容签名为准（无需声明）。
func FileMedia(path string) MediaSource { return media.FileMedia(path) }

// BytesMedia 以内存字节构造媒体源。字节在构造时拷贝（消费方后续改写
// 传入切片不影响媒体内容）；contentType 可空。
func BytesMedia(data []byte, contentType string) MediaSource {
	return media.BytesMedia(data, contentType)
}

// FuncMedia 以调用方提供的打开函数构造媒体源。open 每次调用须返回
// 新流；返回 nil 流视为错误。
func FuncMedia(open func(context.Context) (io.ReadCloser, error), contentType string) MediaSource {
	return media.FuncMedia(open, contentType)
}

// ReaderMedia 从 r 有界复制到内存并返回媒体源；读取在返回前完成，
// 后续 r 的变化不影响媒体内容。contentType 可空。
func ReaderMedia(r io.Reader, contentType string) (MediaSource, error) {
	return media.ReaderMedia(r, contentType)
}

// readMedia 打开媒体源并在返回前完成有界复制（ctx 取消可中断）。
func readMedia(ctx context.Context, src MediaSource) ([]byte, error) {
	return media.ReadMedia(ctx, src)
}

// readBounded 复制 r 直到上限；超限返回 ErrLimitExceeded（不继续读）。
func readBounded(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	return media.ReadBounded(ctx, r, limit)
}

// probeImage 通过签名与结构检查确认图片格式与像素尺寸。
func probeImage(data []byte, declared string) (imageKind, error) {
	return media.ProbeImage(data, declared)
}

// sniffImage 检测魔数并解析尺寸；只读取头部所需字节。
func sniffImage(data []byte) (imageKind, error) { return media.SniffImage(data) }

// imageTypeEqual 比较 MIME 的等价形式（大小写、image/jpg ↔ image/jpeg）。
func imageTypeEqual(a, b string) bool { return media.ImageTypeEqual(a, b) }
