package pptx

import "github.com/F31/go-pptx/v2/internal/diag"

// Severity 是诊断严重级别（方案 §12.1）。
//
// Stable: iota 枚举值（SeverityInfo / SeverityWarning / SeverityError）
// 及其 String() 返回值（"info" / "warning" / "error"）在 v1.0 后锁死——
// 下游 switch/case 完备性依赖于此。仅允许追加新枚举值（追加到 iota 末
// 尾），不可重命名或移除。
//
// v2.0：定义在 internal/diag，此处以 alias 暴露。
type Severity = diag.Severity

// 严重级别常量（alias 到 internal/diag）。
const (
	// SeverityInfo 为信息提示。
	SeverityInfo = diag.SeverityInfo
	// SeverityWarning 为可保留的非关键问题。
	SeverityWarning = diag.SeverityWarning
	// SeverityError 为阻止保存或影响可靠性的问题。
	SeverityError = diag.SeverityError
)

// Diagnostic 是结构/语义诊断条目（方案 §12.1）。
//
// Code 使用稳定错误码字符串（与错误哨兵一一对应或为组件扩展码）；
// Part/NodePath/SlideID/ShapeID 用于错误定位，无关字段为零值。
//
// Stable: 字段集（Code / Severity / Part / NodePath / SlideID / ShapeID /
// Message）在 v1.0 后保持稳定——下游可基于反射或 errors.As 提取诊断条目。
// 仅允许追加新字段（向后兼容），不可重命名或移除。
//
// v2.0：定义在 internal/diag，此处以 alias 暴露。
type Diagnostic = diag.Diagnostic

// ValidationReport 是一次校验的结果（方案 §12.1，ValidationMode：
// Changed / Structural / ExternalStrict，默认 Structural）。
//
// Stable: 字段集（Mode / Diagnostics）在 v1.0 后保持稳定——下游按
// Diagnostics 切片遍历、按 Severity 分桶过滤。仅允许追加新字段。
//
// v2.0：定义在 internal/diag，此处以 alias 暴露。
type ValidationReport = diag.ValidationReport
