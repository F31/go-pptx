// Package engine 是 v2.0 编排层起步（ADR-030 演进第 5 步）：承载 CLI
// （cmd/pptx）与 WASM（wasm/check）共用的只读编排——Inspect / Validate /
// Capability，避免两处各自贴门面导致事实来源漂移。
//
// 本包只依赖门面（pptx）与只读 IR（internal/ir），不做任何 I/O、不感知
// flag/exit code/stdout；调用方负责打开输入与输出形态。
//
// 依赖方向注记：按 ADR-030，engine 编排 Open/Save/Bind/Clone 并承载
// 门面→IR 适配器，需要门面公共句柄，故允许 internal/engine → pptx
// （唯一的显式临时例外，见 internal/archlint）。
package engine

import (
	"context"

	"github.com/F31/go-pptx/v2/internal/ir"
	"github.com/F31/go-pptx/v2/pptx"
)

// InspectResult 是 inspect 的域结果（不含 JSON 外壳/输入名）。
type InspectResult struct {
	Pages      int
	SDKVersion string
	Document   *ir.Document
	ReadOnly   bool
}

// Inspect 对已打开的演示文稿做 IR 全集投影（页面 + Document）。
func Inspect(p *pptx.Presentation) (InspectResult, error) {
	slides, err := p.Slides()
	if err != nil {
		return InspectResult{}, err
	}
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		return InspectResult{}, err
	}
	return InspectResult{
		Pages:      len(slides),
		SDKVersion: pptx.SDKVersion,
		Document:   doc,
		ReadOnly:   true,
	}, nil
}

// ValidateResult 是 L0 结构校验的域结果（诊断 + 分级计数）。
type ValidateResult struct {
	Level       string
	Mode        string
	Diagnostics []pptx.Diagnostic
	ErrorCount  int
	WarnCount   int
}

// Validate 对已打开的演示文稿执行 L0 结构校验并汇总诊断分级计数。
func Validate(ctx context.Context, p *pptx.Presentation) ValidateResult {
	report := p.Validate(ctx)
	e, w := countSeverities(report.Diagnostics)
	return ValidateResult{
		Level:       "structural",
		Mode:        report.Mode,
		Diagnostics: report.Diagnostics,
		ErrorCount:  e,
		WarnCount:   w,
	}
}

// countSeverities 统计诊断中的 error / warning 数（其余不计）。
func countSeverities(diags []pptx.Diagnostic) (errs, warns int) {
	for _, d := range diags {
		switch d.Severity {
		case pptx.SeverityError:
			errs++
		case pptx.SeverityWarning:
			warns++
		}
	}
	return
}

// CapabilityResult 是 capability 的域结果：manifest + 缩进 JSON 视图。
type CapabilityResult struct {
	Manifest   pptx.CapabilityManifest
	IndentJSON []byte
}

// Capability 构造编译期能力 manifest（CAP-01）。不打开输入；sourceInput
// 仅用于 Source 展示（空串表示无源），sourceSize 为源字节数（0 表示未知）。
// 与 wasm/check 及 CLI 的 capability 子命令共享同一事实来源。
func Capability(sourceInput string, sourceSize int64) (CapabilityResult, error) {
	m := pptx.NewCapabilityManifest()
	if sourceInput != "" {
		m.Source.Input = sourceInput
		m.Source.Size = sourceSize
	}
	pptx.PopulateCapabilityDimensions(&m)
	pptx.PopulateCapabilityFeatures(&m)
	pptx.SortCapabilityFeatures(&m)
	b, err := pptx.MarshalManifestIndent(m, "  ")
	if err != nil {
		return CapabilityResult{}, err
	}
	return CapabilityResult{Manifest: m, IndentJSON: b}, nil
}
