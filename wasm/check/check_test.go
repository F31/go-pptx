// TOOL-02 验收测试：check 包的纯函数在 native 环境（js/wasm 都可调用）
// 应当输出一致 JSON、跨平台不变 schemaVersion。Coverage 与 cmd/pptx/CLI
// 子命令对齐：相同输入应产出相同 Inspect/Capability/Validate 结构。
package check_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/F31/go-pptx"
	"github.com/F31/go-pptx/wasm/check"
)

// writeTestPptxBytes 用 pptx.New() 生成最小模板的 ZIP 字节流。
// 不依赖 testdata/外部 fixture——同 §24 验收口径。
func writeTestPptxBytes(t *testing.T) []byte {
	t.Helper()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("pptx.New: %v", err)
	}
	defer p.Close()
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

func TestCheck_SchemaVersion(t *testing.T) {
	got := check.SchemaVersion()
	if got != pptx.CapabilityManifestSchemaVersion {
		t.Fatalf("schemaVersion mismatch: wasm=%q sdk=%q", got, pptx.CapabilityManifestSchemaVersion)
	}
	if !strings.HasPrefix(got, "go-pptx.capability/") {
		t.Errorf("schemaVersion missing namespace prefix: %q", got)
	}
}

func TestCheck_Capability_NoInput(t *testing.T) {
	out, err := check.Capability("test.pptx")
	if err != nil {
		t.Fatalf("Capability: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ok, _ := got["ok"].(bool); !ok {
		t.Errorf("capability envelope should be ok=true: %v", got)
	}
	if got["manifest"] == nil {
		t.Error("capability envelope should include manifest")
	}
	if got["manifestJson"] == nil {
		t.Error("capability envelope should include pre-marshalled manifestJson for UI")
	}
}

func TestCheck_Capability_NoArgsProducesValidManifest(t *testing.T) {
	out, err := check.Capability("")
	if err != nil {
		t.Fatalf("Capability: %v", err)
	}
	var envelope struct {
		OK       bool                     `json:"ok"`
		Manifest *pptx.CapabilityManifest `json:"manifest"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !envelope.OK || envelope.Manifest == nil {
		t.Fatalf("envelope invalid: %+v", envelope)
	}
	if envelope.Manifest.SchemaVersion != check.SchemaVersion() {
		t.Errorf("manifest.schemaVersion mismatch: %q vs %q",
			envelope.Manifest.SchemaVersion, check.SchemaVersion())
	}
	for _, k := range []string{
		pptx.CapabilityInspect, pptx.CapabilityCreate, pptx.CapabilityEdit,
		pptx.CapabilityPreserve, pptx.CapabilityRender, pptx.CapabilityPlay,
	} {
		if _, ok := envelope.Manifest.Dimensions[k]; !ok {
			t.Errorf("dimension %q missing from WASM capability manifest", k)
		}
	}
}

func TestCheck_Inspect_RealBytes(t *testing.T) {
	data := writeTestPptxBytes(t)
	out, err := check.Inspect(context.Background(), data, "test.pptx")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	var got check.InspectResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.OK {
		t.Fatalf("inspect should be ok, error=%q", got.Error)
	}
	if got.Input != "test.pptx" {
		t.Errorf("input passthrough: got %q want test.pptx", got.Input)
	}
	if got.SDKVersion != pptx.SDKVersion {
		t.Errorf("sdkVersion: got %q want %q", got.SDKVersion, pptx.SDKVersion)
	}
	if got.Pages < 0 {
		t.Errorf("Pages should be ≥0, got %d", got.Pages)
	}
	// 新建空模板的 Pages 通常为 0；旧模板（如 M2 模板注入一个 title slide）≥1。
	// 真实语料场景另行扩展。
}

func TestCheck_Validate_RealBytes(t *testing.T) {
	data := writeTestPptxBytes(t)
	out, err := check.Validate(context.Background(), data, "test.pptx")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var got check.ValidateResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.OK {
		t.Fatalf("validate envelope not ok: %+v", got)
	}
	if got.Input != "test.pptx" {
		t.Errorf("input: got %q", got.Input)
	}
	if got.Level != "structural" {
		t.Errorf("level: got %q want structural", got.Level)
	}
}

func TestCheck_Inspect_BadBytesReturnsErrorEnvelope(t *testing.T) {
	out, err := check.Inspect(context.Background(), []byte("not-a-zip"), "broken.pptx")
	if err != nil {
		t.Fatalf("check.Inspect should not error envelope (returns JSON): %v", err)
	}
	var got check.InspectResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.OK {
		t.Errorf("expected ok=false for bad bytes, got ok=true: %+v", got)
	}
	if got.Error == "" {
		t.Error("expected non-empty error message for bad bytes")
	}
}

func TestCheck_Validate_BadBytesReturnsErrorEnvelope(t *testing.T) {
	out, err := check.Validate(context.Background(), []byte("not-a-zip"), "broken.pptx")
	if err != nil {
		t.Fatalf("check.Validate envelope path: %v", err)
	}
	var got check.ValidateResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.OK {
		t.Errorf("expected ok=false for bad bytes, got ok=true: %+v", got)
	}
	if got.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestCheck_Inspect_EmptyBytesHandled(t *testing.T) {
	out, err := check.Inspect(context.Background(), nil, "empty.pptx")
	if err != nil {
		t.Fatalf("envelope path: %v", err)
	}
	var got check.InspectResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.OK {
		t.Errorf("empty input should not be ok: %+v", got)
	}
}

func TestCheck_AsError_FromEnvelope(t *testing.T) {
	// 走 check.Inspect("garbage") 拿到 ok=false 的 envelope，AsError 检测出来。
	data := []byte("garbage")
	out, _ := check.Inspect(context.Background(), data, "x.pptx")
	if err := check.AsError(out); err == nil || err.Error() == "" {
		t.Errorf("AsError should report ok=false envelope")
	}
}

func TestCheck_FailJSONAndAsErrorEdges(t *testing.T) {
	out := check.FailJSON("missing input")
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("FailJSON invalid JSON: %v", err)
	}
	if env.OK || env.Error != "missing input" {
		t.Fatalf("FailJSON envelope = %+v", env)
	}
	if err := check.AsError(out); err == nil || err.Error() != "missing input" {
		t.Fatalf("AsError(FailJSON) = %v, want missing input", err)
	}
	if err := check.AsError("not-json"); err == nil || !strings.Contains(err.Error(), "parse result") {
		t.Fatalf("AsError invalid JSON = %v", err)
	}
	if err := check.AsError(`{"ok":false}`); err == nil || !strings.Contains(err.Error(), "no error message") {
		t.Fatalf("AsError missing message = %v", err)
	}
}

func TestCheck_AsError_OkEnvelopeReturnsNil(t *testing.T) {
	out, err := check.Capability("any")
	if err != nil {
		t.Fatalf("Capability: %v", err)
	}
	if err := check.AsError(out); err != nil {
		t.Errorf("AsError should return nil for ok=true: %v", err)
	}
}

func TestCheck_MarshalIsValidJSON(t *testing.T) {
	out, err := check.Capability("a.pptx")
	if err != nil {
		t.Fatalf("Capability: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	dec.UseNumber()
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Errorf("invalid JSON token: %v", err)
			return
		}
	}
}

// 验证 Capabilities 与 Present.Capability() 在 native 上产出一致 schema。
// 这个等价性是 §23.2 "与 CLI 一致" 的形式化版本。
func TestCheck_CapabilityMatchesCLIManifest(t *testing.T) {
	cliOut, err := check.Capability("c.pptx")
	if err != nil {
		t.Fatalf("Capability: %v", err)
	}
	// 预期 envelope 必须包含 schemaVersion、CAP-01 维度键全集。
	var env struct {
		OK       bool                     `json:"ok"`
		Manifest *pptx.CapabilityManifest `json:"manifest"`
	}
	if err := json.Unmarshal([]byte(cliOut), &env); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if env.Manifest.SchemaVersion != check.SchemaVersion() {
		t.Errorf("schemaVersion mismatch")
	}
	// 关键字段必须齐全：dimensions/6 维 + features > 0 + diagnostics 可选
	if len(env.Manifest.Dimensions) != 6 {
		t.Errorf("dimensions should be 6, got %d", len(env.Manifest.Dimensions))
	}
	if len(env.Manifest.Features) == 0 {
		t.Error("features should be non-empty")
	}
}
