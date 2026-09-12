package chart

// Optional[T] 是"未设置/显式设置"的双态包装。
//
// ADR-017 第二批：与 ChartAxisOptions 等值对象一起搬到 internal/chart，
// 根包用 type alias 引用。
//
// Set=false 表示无本地覆盖（继承）；Set=true 时必须按 Value 理解。
type Optional[T any] struct {
	Value T
	Set   bool
}

// NewOptional 构造显式设置值。
func NewOptional[T any](v T) Optional[T] { return Optional[T]{Value: v, Set: true} }
