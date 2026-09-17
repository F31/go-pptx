package pptx

// 本文件是形状几何/填充/效果访问器簿委托：解析已迁至
// internal/document/geometry（v2.0 域搬迁）；此处以 alias 暴露 DTO 类型
// 并提供 shapeNode 上的薄委托。

import (
	"github.com/F31/go-pptx/v2/internal/document/geometry"
)

// 几何类型（GeometryKind/GeometryInfo/GeomAdjust/GeomGuide/GeomPath/
// PathCommand）定义在 internal/document/geometry，见 geometry_alias.go。

// ---------- 填充 FillInfo ----------

// FillKind 在 style_dto.go 已声明（alias 到 internal/document/style）。

// FillInfo 是形状填充（spPr/a:fill 内首元素）的解析结果。
type FillInfo = geometry.FillInfo

// GradientFill 是 a:gradFill 解析结果。
type GradientFill = geometry.GradientFill

// GradientStop 是 a:gs/a:stop 一个停止点。
type GradientStop = geometry.GradientStop

// PatternFill 是 a:pattFill 解析结果。
type PatternFill = geometry.PatternFill

// BlipFillInfo 是 a:blipFill 的占位结构（图片关系未解析）。
type BlipFillInfo = geometry.BlipFillInfo

// ---------- 效果 EffectInfo ----------

// EffectKind 是效果类别（a:effectLst 或 a:effectDag 顶层元素）。
type EffectKind = geometry.EffectKind

// 效果类别常量（alias 到 internal/document/geometry）。
const (
	// EffectNone 表示无效果（effectLst 元素存在但无内容）。
	EffectNone = geometry.EffectNone
	// EffectOuterShadow 是 a:outerShdw。
	EffectOuterShadow = geometry.EffectOuterShadow
	// EffectInnerShadow 是 a:innerShdw。
	EffectInnerShadow = geometry.EffectInnerShadow
	// EffectGlow 是 a:glow。
	EffectGlow = geometry.EffectGlow
	// EffectSoftEdge 是 a:softEdge。
	EffectSoftEdge = geometry.EffectSoftEdge
	// EffectReflection 是 a:reflection。
	EffectReflection = geometry.EffectReflection
	// EffectFillOverlay 是 a:fillOverlay。
	EffectFillOverlay = geometry.EffectFillOverlay
	// EffectBlur 是 a:blur（a14 扩展）。
	EffectBlur = geometry.EffectBlur
	// EffectUnknown 是未识别/缺失类别。
	EffectUnknown = geometry.EffectUnknown
)

// EffectInfo 是形状效果（spPr/a:effectLst 或 a:effectDag）的解析结果。
type EffectInfo = geometry.EffectInfo

// Effect 是单个效果条目。
type Effect = geometry.Effect

// Scene3DInfo 是 a:scene3d 顶层解析（仅"声明存在"+关键子元素）。
type Scene3DInfo = geometry.Scene3DInfo

// Camera3D 是 a:scene3d/a:camera 解析。
type Camera3D = geometry.Camera3D

// LightRig3D 是 a:scene3d/a:lightRig 解析。
type LightRig3D = geometry.LightRig3D

// Shape3DInfo 是 a:sp3d 解析。
type Shape3DInfo = geometry.Shape3DInfo

// Bevel3D 是 a:bevelT/a:bevelB 解析。
type Bevel3D = geometry.Bevel3D

// ---------- Shape 接口扩展 ----------

// Geometry 返回形状几何（spPr/a:prstGeom 或 a:custGeom）的 R 档解析结果。
// 组合（grpSp）/未知 OpaqueShape 等无 spPr 的形状返回零值 + Warning 诊断。
func (s *shapeNode) Geometry() (GeometryInfo, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return GeometryInfo{}, nil, Annotate(err, "shape.Geometry")
	}
	return geometry.ParseShapeGeometry(doc, el)
}

// Fill 返回形状填充（spPr/a:fill）的 R 档解析结果。
// 组合（grpSp）等无 spPr 的形状返回零值 + Warning 诊断。
func (s *shapeNode) Fill() (FillInfo, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return FillInfo{}, nil, Annotate(err, "shape.Fill")
	}
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return FillInfo{}, nil, Annotate(err, "shape.Fill")
	}
	out, diags := geometry.ParseShapeFill(doc, el, env, s.p.styleDocs(), string(s.part))
	return out, diags, nil
}

// Effects 返回形状效果（spPr/a:effectLst|a:effectDag + a:scene3d + a:sp3d）
// 的 R 档解析结果。
func (s *shapeNode) Effects() (EffectInfo, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return EffectInfo{}, nil, Annotate(err, "shape.Effects")
	}
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return EffectInfo{}, nil, Annotate(err, "shape.Effects")
	}
	out, diags := geometry.ParseShapeEffects(doc, el, env, s.p.styleDocs(), string(s.part))
	return out, diags, nil
}
