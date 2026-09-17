package chart

import "github.com/F31/go-pptx/v2/internal/document/model"

// Optional[T] 是"未设置/显式设置"的双态包装。
// v2.0：定义在 internal/document/model，此处 alias 供本包沿用。
type Optional[T any] = model.Optional[T]

// NewOptional 构造显式设置值。
func NewOptional[T any](v T) Optional[T] { return model.NewOptional(v) }
