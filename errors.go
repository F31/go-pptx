package pptx

import (
	"errors"
)

// 稳定错误码（方案 §20.4）。
//
// 所有可检查错误都支持 errors.Is；具体上下文通过 errors.As 获取
// OperationError。取消与超时保留 context.Canceled / context.DeadlineExceeded
// 的可检测性，不在此重复包装。
//
// Stable: 本组哨兵错误是 API 兼容契约的硬约束。错误码字符串一旦随 v1.0
// 发布即锁死（下游客户通过 errors.Is(err, pptx.ErrXxx) 编写分支）——
// 1.x 内只允许追加新错误码，已存在的字符串值与语义不变；移除或重命名
// 任何一条均视为破坏性变更，必须走 ADR-015 §"降档"流程。
var (
	// ErrClosed 表示文档已关闭，对象访问失败。
	ErrClosed = errors.New("pptx: document closed")
	// ErrStaleHandle 表示节点已被删除，句柄失效。
	ErrStaleHandle = errors.New("pptx: stale handle")
	// ErrInvalidArgument 表示参数无效。
	ErrInvalidArgument = errors.New("pptx: invalid argument")
	// ErrOutOfRange 表示索引越界。
	ErrOutOfRange = errors.New("pptx: index out of range")
	// ErrNotFound 表示对象不存在。
	ErrNotFound = errors.New("pptx: not found")
	// ErrForeignReference 表示句柄来自其他文档。
	ErrForeignReference = errors.New("pptx: foreign reference")
	// ErrUnsupportedFormat 表示文件类型未支持（如加密文件、二进制 .ppt）。
	ErrUnsupportedFormat = errors.New("pptx: unsupported format")
	// ErrUnsupportedEdit 表示编辑行为未支持（无法证明可安全保留时显式失败，不静默删除）。
	ErrUnsupportedEdit = errors.New("pptx: unsupported edit")
	// ErrLimitExceeded 表示资源预算超限。
	ErrLimitExceeded = errors.New("pptx: resource limit exceeded")
	// ErrMalformedPackage 表示包结构非法。
	ErrMalformedPackage = errors.New("pptx: malformed package")
	// ErrUnresolvedStyle 表示调用方要求完整格式，但解析不足（无回退值不臆测）。
	ErrUnresolvedStyle = errors.New("pptx: unresolved style")
	// ErrValidationFailed 表示保存计划未通过所选校验。
	ErrValidationFailed = errors.New("pptx: validation failed")
	// ErrTimingConflict 表示播放树冲突，无法在不覆盖原树的前提下合并。
	ErrTimingConflict = errors.New("pptx: timing conflict")
	// ErrDurationUnknown 表示音频时长缺失或无法探测。
	ErrDurationUnknown = errors.New("pptx: duration unknown")
	// ErrConcurrentModification 表示保存计划基于的 revision 已不匹配。
	ErrConcurrentModification = errors.New("pptx: concurrent modification")
	// ErrOutputExists 表示目标已存在且未显式允许覆盖。
	ErrOutputExists = errors.New("pptx: output file exists")
	// ErrAtomicReplaceUnavailable 表示平台无法提供可验证的原子替换。
	ErrAtomicReplaceUnavailable = errors.New("pptx: atomic replace unavailable")
)

// OperationError 为错误附带操作上下文（方案 §20.4）。
//
// Stable: 结构字段 Op / Part / NodePath / SlideID / ShapeID / Message / Err
// 在 v1.0 发布后保持稳定——下游客户可基于 errors.As(err, *OperationError)
// 提取定位信息。仅允许在 1.x 内追加新字段（向后兼容：旧客户端零值字段
// 不会破坏现有逻辑）；移除或重命名字段视为破坏性变更。Unwrap/Error()
// 行为契约亦锁定。
//
// 分类（可检查错误）通过 Err 字段携带的哨兵错误完成：OperationError 实现
// Unwrap，因此 errors.Is(err, pptx.ErrNotFound) 等判断可用；
// 定位信息（Part/NodePath/SlideID/ShapeID/Message）通过 errors.As 读取。
type OperationError struct {
	// Op 是失败的操作名，如 "Presentation.Open"、"Slide.AddAudio"。
	Op string
	// Part 是相关 Part 名（OPC 风格，如 "/ppt/slides/slide1.xml"）。
	Part string
	// NodePath 是错误定位的节点路径（xmlstore 语义）。
	NodePath string
	// SlideID 与 ShapeID 在相关时给出；无关时为零值。
	SlideID SlideID
	ShapeID ShapeID
	// Message 是面向调用方的补充说明。
	Message string
	// Err 是底层错误（哨兵或链上错误）。
	Err error
}

func (e *OperationError) Error() string {
	detail := ""
	if e.Err != nil {
		detail = e.Err.Error()
	}
	if e.Message != "" {
		if detail != "" {
			detail = e.Message + ": " + detail
		} else {
			detail = e.Message
		}
	}
	if detail == "" {
		return "pptx: operation failed"
	}
	if e.Op == "" {
		return detail
	}
	return "pptx: " + e.Op + ": " + detail
}

// Unwrap 支持 errors.Is / errors.As 沿链检查。
func (e *OperationError) Unwrap() error { return e.Err }

// Annotate 包装 err 并附带操作上下文；err 为 nil 时返回 nil。
func Annotate(err error, op string) error {
	if err == nil {
		return nil
	}
	return &OperationError{Op: op, Err: err}
}
