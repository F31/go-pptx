// Package check 提供 pptx 只读检视的纯函数版本，TOOL-02（WASM 浏览器
// 检查工具）复用此包导出 syscall/js 入口；CLI 的子命令（cmd/pptx）
// 当前保持独立实现，但输出 JSON 结构需与本包保持一致——即 §23.2 的
// "与 CLI 一致"要求。文件不离本机：所有输入经调用方传入 []byte，不触
// 任何文件系统写入路径；输入只在 wasm 线性内存中存在。
//
// 支持的子操作：
//   - Inspect：IR 全集投影（页面/媒体/备注/能力）
//   - Capability：六维能力报告（不需要输入字节，返回默认 manifest）
//   - Validate：L0 结构校验 + 诊断聚合
//
// 所有函数返回 JSON 字符串而非 bytes — 调用方（JS）直接接收文本，避免
// 在 wasm 边界搬运 byte slice 的额外步骤。错误也以 JSON 对象返回，便于
// UI 端统一处理。
package check

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/F31/go-pptx"
	"github.com/F31/go-pptx/ir"
)

// FailJSON 构造一个错误的 JSON 字符串（用于 syscall/js 顶层 fallback）。
//
// 该函数不是 check 子操作的常规返回路径——大多数失败都在各 Inspect/
// Capability/Validate 内部就以 envelope 形式返回。仅当调用方忘记传参
// 这类前置错误时使用。
func FailJSON(msg string) string {
	body, _ := json.Marshal(struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}{OK: false, Error: msg})
	return string(body)
}

// SchemeVersion 返回当前 SDK manifest 的 schemaVersion（CAP-01）。
//
// 该常量在 wasm 与 cmd/pptx/CLI 之间必须保持一致；JS 端可在 UI 展示
// 时校验，避免误显示不匹配版本的 manifest。
func SchemaVersion() string { return pptx.CapabilityManifestSchemaVersion }

// InspectResult 是 Inspect 在 JS 端的 JSON 输出形态。
//
// 与 cmd/pptx/inspect.go 的 inspectSummary 结构等价；为了避免依赖
// cmd/pptx 包（main 包）且便于序列化，独立定义。
type InspectResult struct {
	OK         bool         `json:"ok"`
	Input      string       `json:"input"`
	Pages      int          `json:"pages"`
	SDKVersion string       `json:"sdkVersion"`
	Document   *ir.Document `json:"document,omitempty"`
	ReadOnly   bool         `json:"readOnly"`
	Error      string       `json:"error,omitempty"`
}

// CapabilityResult 是 Capability 在 JS 端的 JSON 输出形态（透传
// pptx.CapabilityManifest）。
type CapabilityResult struct {
	OK           bool                     `json:"ok"`
	Input        string                   `json:"input"`
	Manifest     *pptx.CapabilityManifest `json:"manifest,omitempty"`
	ManifestJSON map[string]any           `json:"manifestJson,omitempty"` // 已 MarshalIndent 的字符串视图，便于 UI 直接展示
	Error        string                   `json:"error,omitempty"`
}

// ValidateResult 是 Validate 在 JS 端的 JSON 输出形态（与 cmd/pptx
// validateSummary 对齐）。
type ValidateResult struct {
	OK          bool              `json:"ok"`
	Input       string            `json:"input"`
	Level       string            `json:"level"`
	Mode        string            `json:"mode"`
	ErrorCount  int               `json:"errorCount"`
	WarnCount   int               `json:"warnCount"`
	Diagnostics []pptx.Diagnostic `json:"diagnostics"`
	Error       string            `json:"error,omitempty"`
}

// Inspect 接收 PPTX 字节流，返回 IR 投影的 JSON 字符串。仅在 wasm 内存
// 中处理输入字节，不写入任何文件系统位置。
func Inspect(ctx context.Context, input []byte, fileName string) (string, error) {
	p, err := pptx.OpenReader(byteReaderAt(input), int64(len(input)))
	if err != nil {
		return marshalError(&InspectResult{Input: fileName, ReadOnly: true}, fmt.Errorf("open: %w", err))
	}
	defer p.Close()

	slides, err := p.Slides()
	if err != nil {
		return marshalError(&InspectResult{Input: fileName, ReadOnly: true}, fmt.Errorf("slides: %w", err))
	}
	doc, err := ir.FromPresentation(p, ir.DefaultOptions())
	if err != nil {
		return marshalError(&InspectResult{Input: fileName, ReadOnly: true}, fmt.Errorf("ir: %w", err))
	}
	res := &InspectResult{
		OK:         true,
		Input:      fileName,
		Pages:      len(slides),
		SDKVersion: pptx.SDKVersion,
		Document:   doc,
		ReadOnly:   true,
	}
	return marshalOK(res)
}

// Capability 不需要输入字节，返回 SDK 编译期的能力 manifest（CAP-01）。
// 内存代价为 O(1)（manifest 是固定结构）。
func Capability(fileName string) (string, error) {
	m := pptx.NewCapabilityManifest()
	pptx.PopulateCapabilityDimensions(&m)
	pptx.PopulateCapabilityFeatures(&m)
	pptx.SortCapabilityFeatures(&m)
	// 同时填充 ManifestJSON 字符串视图，UI 可直接展示而不必再做 MarshalIndent
	if fileName != "" {
		m.Source.Input = fileName
	}
	b, err := pptx.MarshalManifestIndent(m, "  ")
	if err != nil {
		return marshalError(&CapabilityResult{Input: fileName}, err)
	}
	var pretty map[string]any
	_ = json.Unmarshal(b, &pretty)
	res := &CapabilityResult{
		OK:           true,
		Input:        fileName,
		Manifest:     &m,
		ManifestJSON: pretty,
	}
	return marshalOK(res)
}

// Validate 接收 PPTX 字节流，执行 L0 结构校验并汇总诊断。
func Validate(ctx context.Context, input []byte, fileName string) (string, error) {
	p, err := pptx.OpenReader(byteReaderAt(input), int64(len(input)))
	if err != nil {
		return marshalError(&ValidateResult{Input: fileName, Level: "structural"}, err)
	}
	defer p.Close()
	report := p.Validate(ctx)
	res := &ValidateResult{
		OK:          true,
		Input:       fileName,
		Level:       "structural",
		Mode:        report.Mode,
		Diagnostics: report.Diagnostics,
	}
	for _, d := range report.Diagnostics {
		switch d.Severity {
		case pptx.SeverityError:
			res.ErrorCount++
		case pptx.SeverityWarning:
			res.WarnCount++
		}
	}
	return marshalOK(res)
}

// ---------- 辅助 ----------

// byteReaderAt 把 []byte 适配到 io.ReaderAt（opc.Load/pptx.OpenReader 入参）。
type memBytesReader struct {
	b []byte
	o int64
}

func (r *memBytesReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r.b)) {
		return 0, fmt.Errorf("read past end: offset=%d len=%d", off, len(r.b))
	}
	n := copy(p, r.b[off:])
	if n < len(p) {
		return n, nil // 允许短读
	}
	return n, nil
}

// byteReaderAt 构造一个 io.ReaderAt 视图用于 PPTX 字节流。
func byteReaderAt(b []byte) *memBytesReader { return &memBytesReader{b: b} }

// marshalOK 序列化为 JSON 字符串。
func marshalOK(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalError 把 err 注入 res.Error 后返回 JSON。err 永远非空。
func marshalError(res any, err error) (string, error) {
	type withErr interface {
		// 私有接口，仅约束"接受 Error 字段" 的结构体使用。
	}
	_ = withErr(nil)
	b, jerr := json.Marshal(res)
	if jerr != nil {
		return "", jerr
	}
	// 二次把 Error 字段插入
	var generic map[string]any
	if uerr := json.Unmarshal(b, &generic); uerr == nil {
		generic["error"] = err.Error()
		generic["ok"] = false
		if rb, jerr2 := json.Marshal(generic); jerr2 == nil {
			return string(rb), nil
		}
	}
	return "", fmt.Errorf("marshal error envelope: %w", jerr)
}

// AsError 把返回的 JSON 字符串解析为 {ok, error, ...} 形式以检查失败。
// 这个 helper 旨在让调用方在 wasm 边界处统一行为。
func AsError(jsonStr string) error {
	var probe struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &probe); err != nil {
		return fmt.Errorf("parse result: %w", err)
	}
	if probe.OK {
		return nil
	}
	if probe.Error == "" {
		return fmt.Errorf("ok=false but no error message")
	}
	return &opcError{msg: probe.Error}
}

// opcError 是 wasm 边界处的轻量错误占位（避免依赖 opc 包内部 error
// 类型链路）。
type opcError struct{ msg string }

func (e *opcError) Error() string { return e.msg }
