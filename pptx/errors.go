package pptx

import "github.com/F31/go-pptx/v2/internal/errs"

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
//
// v2.0：定义在 internal/errs，此处以 alias 暴露。
var (
	// ErrClosed 表示文档已关闭，对象访问失败。
	ErrClosed = errs.ErrClosed
	// ErrStaleHandle 表示节点已被删除，句柄失效。
	ErrStaleHandle = errs.ErrStaleHandle
	// ErrInvalidArgument 表示参数无效。
	ErrInvalidArgument = errs.ErrInvalidArgument
	// ErrOutOfRange 表示索引越界。
	ErrOutOfRange = errs.ErrOutOfRange
	// ErrNotFound 表示对象不存在。
	ErrNotFound = errs.ErrNotFound
	// ErrForeignReference 表示句柄来自其他文档。
	ErrForeignReference = errs.ErrForeignReference
	// ErrUnsupportedFormat 表示文件类型未支持（如加密文件、二进制 .ppt）。
	ErrUnsupportedFormat = errs.ErrUnsupportedFormat
	// ErrUnsupportedEdit 表示编辑行为未支持（无法证明可安全保留时显式失败，不静默删除）。
	ErrUnsupportedEdit = errs.ErrUnsupportedEdit
	// ErrLimitExceeded 表示资源预算超限。
	ErrLimitExceeded = errs.ErrLimitExceeded
	// ErrMalformedPackage 表示包结构非法。
	ErrMalformedPackage = errs.ErrMalformedPackage
	// ErrUnresolvedStyle 表示调用方要求完整格式，但解析不足（无回退值不臆测）。
	ErrUnresolvedStyle = errs.ErrUnresolvedStyle
	// ErrValidationFailed 表示保存计划未通过所选校验。
	ErrValidationFailed = errs.ErrValidationFailed
	// ErrTimingConflict 表示播放树冲突，无法在不覆盖原树的前提下合并。
	ErrTimingConflict = errs.ErrTimingConflict
	// ErrDurationUnknown 表示音频时长缺失或无法探测。
	ErrDurationUnknown = errs.ErrDurationUnknown
	// ErrConcurrentModification 表示保存计划基于的 revision 已不匹配。
	ErrConcurrentModification = errs.ErrConcurrentModification
	// ErrOutputExists 表示目标已存在且未显式允许覆盖。
	ErrOutputExists = errs.ErrOutputExists
	// ErrAtomicReplaceUnavailable 表示平台无法提供可验证的原子替换。
	ErrAtomicReplaceUnavailable = errs.ErrAtomicReplaceUnavailable
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
//
// v2.0：定义在 internal/errs，此处以 alias 暴露。
type OperationError = errs.OperationError

// Annotate 包装 err 并附带操作上下文；err 为 nil 时返回 nil。
func Annotate(err error, op string) error { return errs.Annotate(err, op) }
