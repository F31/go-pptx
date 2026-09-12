package chart

import "errors"

// 错误码契约（与根包 pptx.errors.go 共享语义）：
//
//   - ErrInvalidArgument：白名单外输入、值不在合法区间、约束违反等。
//   - ErrStaleHandle：句柄失效（cNvPr@id 已变 / 节点已删除）。
//   - ErrClosed：Presentation / Slide 已关闭。
//   - ErrNotFound：节点查不到。
//
// 根包 wrapper 在适配 *pptx.OperationError 时按需使用 errors.Is 判定；
// chart 侧的 sentinel 与根包同名实例不同（errors.New 各持一份），translateSentinel
// 按错误字符串翻译回根包 sentinel（保持 errors.Is 链）。
// 不要在本包内增加 ErrXxx 哨兵——保持与根包 errors.go 等长。

// ErrInvalidArgument 表示客户端输入违反 schema / 白名单 / 区间约束。
var ErrInvalidArgument = errors.New("chart: invalid argument")

// ErrStaleHandle 表示句柄失效（cNvPr@id 已变 / 节点被删除）。
var ErrStaleHandle = errors.New("chart: stale handle")

// ErrClosed 表示 Presentation / Slide 已关闭。
var ErrClosed = errors.New("chart: presentation closed")

// ErrNotFound 表示 xmlstore 节点查不到。
var ErrNotFound = errors.New("chart: not found")

// ErrUnsupportedFormat 表示 OOXML schema / 元素类型不在受限范围（与
// 根包 pptx.ErrUnsupportedFormat 同语义；translateSentinel 自动翻译）。
var ErrUnsupportedFormat = errors.New("chart: unsupported format")

// ErrMalformedPackage 表示包结构非法（与根包 pptx.ErrMalformedPackage 同语义）。
var ErrMalformedPackage = errors.New("chart: malformed package")

// ValidationError 是 chart 内部统一的输入校验错误包装。
//
// 字段：
//   - Op：触发校验的对外方法名（例如 "AddChart" / "SetData"）。
//   - Message：具体说明（已含 sentinel 类别前缀）。
//   - Sentinel：归属的根包公共错误哨兵（errors.Is 可穿透）。
//
// 错误链：errors.Is(err, ErrXxx) 命中 Sentinel → 与根包 *OperationError
// 行为一致。Unwrap 返回 Sentinel（不是 Op / Message）。
type ValidationError struct {
	Op       string
	Message  string
	Sentinel error
}

// Error 满足 error interface。
func (e *ValidationError) Error() string {
	if e.Op == "" {
		return e.Message
	}
	return e.Op + ": " + e.Message
}

// Unwrap 暴露 Sentinel 给 errors.Is 链。
func (e *ValidationError) Unwrap() error { return e.Sentinel }

// BuildError 是 chart 内部统一的 XML 生成错误包装。
//
// 与 ValidationError 区别：用于"编码阶段"（escape 失败、缓存溢出等）；
// 根包 wrapper 默认 Op 由调用方注入（例如 "Slide.AddChart"）。
type BuildError struct {
	Op       string
	Message  string
	Sentinel error
}

// Error 满足 error interface。
func (e *BuildError) Error() string {
	if e.Op == "" {
		return e.Message
	}
	return e.Op + ": " + e.Message
}

// Unwrap 暴露 Sentinel 给 errors.Is 链。
func (e *BuildError) Unwrap() error { return e.Sentinel }

// errBuild 与 ValidationError 同形；不同名以让根包 wrapper 按类型分派。
func errBuild(op, msg string, sentinel error) error {
	return &BuildError{Op: op, Message: msg, Sentinel: sentinel}
}
