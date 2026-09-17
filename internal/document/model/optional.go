package model

// Optional[T] 是"未设置/显式设置"的双态包装。
//
// ADR-017 第二批：Optional 由根包 alias 引用；v2.0 随共享值类型下沉到
// internal/document/model（与 chart 解耦，供 style/text 等域共同引用）。
//
// Set=false 表示无本地覆盖（继承）；Set=true 时必须按 Value 理解。
type Optional[T any] struct {
	Value T
	Set   bool
}

// NewOptional 构造显式设置值。
func NewOptional[T any](v T) Optional[T] { return Optional[T]{Value: v, Set: true} }
