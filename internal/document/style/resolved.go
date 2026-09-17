package style

// ResolvedValue 是单个解析属性（非颜色）的三要素载体。
type ResolvedValue[T any] struct {
	// Value 是解析后的值；Resolved=false 时为零值。
	Value T
	// Resolved 表示沿链得到可呈现值（含回退）。
	Resolved bool
	// Fallback 表示值来自调用方回退（此时 Resolved=true）。
	Fallback bool
	// Trace 是实际贡献链（最近来源在前；含主题展开步）。
	Trace []StyleStep
}

// ResolvedColor 是颜色属性的三要素载体。
type ResolvedColor struct {
	// Spec 保留原始颜色引用（scheme:xxx 或 #RRGGBB），无论是否完全解析。
	Spec ColorSpec
	// RGB 是可呈现的 sRGB RRGGBB（仅 Resolved=true 时非空）。
	RGB string
	// Resolved 表示已得到最终可呈现 RGB。
	Resolved bool
	// Fallback 表示值来自调用方回退（此时 Resolved=true）。
	Fallback bool
	// Trace 是实际贡献链（最近来源在前；scheme 引用含主题展开步）。
	Trace []StyleStep
}

// ResolvedFont 是 EffectiveFont 的逐属性解析结果（方案 §6.1）。
type ResolvedFont struct {
	Bold          ResolvedValue[bool]
	Italic        ResolvedValue[bool]
	Size          ResolvedValue[FontSize]
	Color         ResolvedColor
	Latin         ResolvedValue[string]
	EastAsian     ResolvedValue[string]
	ComplexScript ResolvedValue[string]
}

// ResolveContext 携带 EffectiveFont 的解析上下文（方案 §6.1）。
//
// Fallback 提供逐字段回退（Optional：仅 Set=true 的字段会被采用）；
// 采用回退的属性 Resolved=true 且 Fallback=true。Strict 为 true 时，
// 任一属性未解析（含部分解析的颜色）都会使 EffectiveFont 返回
// ErrUnresolvedStyle（诊断仍随结果返回）。
type ResolveContext struct {
	Fallback FontStyle
	Strict   bool
}
