package pptx

// 本文件定义 Shape 能力窄接口（A-2，ADR-021）。它们从 Shape 接口按能力维度
// 切出，使调用方在不依赖完整 Shape 接口的前提下，只断言自己关心的能力
// （例如"只要能取几何"就接受任意形状）。
//
// 设计约束（ADR-021）：
//   - 仅新增接口，不修改 Shape 接口、不移除既有 getter；
//   - 所有具体形状类型已通过 shapeNode 或自身方法实现这些接口，声明即满足，
//     不改动既有类型；
//   - 接口方法签名与 Shape 完全一致，保证语义一致；
//   - 新 caller 走窄接口断言，既有 caller 继续用 Shape，双轨并存。
//
// Stable: A-2 形状能力窄接口（v1.1.0 引入）。作为公共 API 承诺：接口集与方法
// 签名在 v1.x 内稳定；仅允许追加新接口/方法，不移除/重命名。
type (
	// GeometryProvider 仅暴露几何解析能力（GEOM-02）。
	GeometryProvider interface {
		Geometry() (GeometryInfo, []Diagnostic, error)
	}
	// FillProvider 仅暴露填充解析能力（GEOM-02）。
	FillProvider interface {
		Fill() (FillInfo, []Diagnostic, error)
	}
	// EffectsProvider 仅暴露效果解析能力（GEOM-02）。
	EffectsProvider interface {
		Effects() (EffectInfo, []Diagnostic, error)
	}
	// StyleMatrixRefsProvider 仅暴露样式矩阵引用能力（STYLE-02）。
	StyleMatrixRefsProvider interface {
		StyleMatrixRefs() ([]StyleMatrixRef, []Diagnostic, error)
	}
	// LineProvider 仅暴露线条解析能力（GEOM-02）。
	LineProvider interface {
		Line() (LineStyle, []Diagnostic, error)
	}
)
