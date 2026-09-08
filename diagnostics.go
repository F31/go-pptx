package pptx

// Severity 是诊断严重级别（方案 §12.1）。
type Severity int

const (
	// SeverityInfo 为信息提示。
	SeverityInfo Severity = iota
	// SeverityWarning 为可保留的非关键问题。
	SeverityWarning
	// SeverityError 为阻止保存或影响可靠性的问题。
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// Diagnostic 是结构/语义诊断条目（方案 §12.1）。
//
// Code 使用稳定错误码字符串（与错误哨兵一一对应或为组件扩展码）；
// Part/NodePath/SlideID/ShapeID 用于错误定位，无关字段为零值。
type Diagnostic struct {
	Code     string
	Severity Severity
	Part     string
	NodePath string
	SlideID  SlideID
	ShapeID  ShapeID
	Message  string
}

// ValidationReport 是一次校验的结果（方案 §12.1，ValidationMode：
// Changed / Structural / ExternalStrict，默认 Structural）。
type ValidationReport struct {
	// Mode 是本次校验使用的模式。
	Mode string
	// Diagnostics 为按严重级别汇总后的诊断条目。
	Diagnostics []Diagnostic
}

// HasErrors 报告是否存在 error 级诊断。
func (r ValidationReport) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}
