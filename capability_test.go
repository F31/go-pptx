// CAP-01 验收测试：六维状态、JSON schema 形态、SaveReport 一致性、SDK
// 链接器变量、Diagnostic 字段路径、Markdown 报告同源。
package pptx

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

func TestCapability_SchemaVersionStable(t *testing.T) {
	if CapabilityManifestSchemaVersion == "" {
		t.Fatal("schema version must be non-empty")
	}
	if CapabilityManifestSchemaVersion != "go-pptx.capability/1.0" {
		t.Fatalf("unexpected schema version: %q", CapabilityManifestSchemaVersion)
	}
}

func TestCapability_AllSixDimensionsPresent(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	want := []string{
		CapabilityInspect,
		CapabilityCreate,
		CapabilityEdit,
		CapabilityPreserve,
		CapabilityRender,
		CapabilityPlay,
	}
	for _, k := range want {
		if _, ok := m.Dimensions[k]; !ok {
			t.Errorf("dimension %q missing from manifest", k)
		}
	}
	if got := len(m.Dimensions); got != 6 {
		t.Errorf("manifest must contain exactly 6 dimensions, got %d", got)
	}
}

func TestCapability_RenderUntestedByDesign(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	d := m.Dimensions[CapabilityRender]
	if d.Status != StatusUntested {
		t.Errorf("render dimension should be Untested (per §14), got %s", d.Status)
	}
	if d.Notes == "" {
		t.Error("render dimension Notes should explain deferred to later milestone")
	}
	if len(d.Limits) == 0 {
		t.Error("render dimension should expose explicit limits")
	}
}

func TestCapability_PlayStatusMatchesAudioScope(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	d := m.Dimensions[CapabilityPlay]
	if d.Status != StatusPartial {
		t.Errorf("expected Play = Partial, got %s", d.Status)
	}
	joined := strings.Join(d.AppliesTo, " ")
	if !strings.Contains(joined, "AddAudio") {
		t.Errorf("Play AppliesTo must reference AddAudio; got %v", d.AppliesTo)
	}
}

func TestCapability_PreserveSupported_SaveReportAligned(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	d := m.Dimensions[CapabilityPreserve]
	if d.Status != StatusSupported {
		t.Errorf("Preserve must be Supported, got %s", d.Status)
	}
	if !strings.Contains(d.Notes, "SaveReport") {
		t.Errorf("Preserve Notes must reference SaveReport: %q", d.Notes)
	}
	joinedLimits := strings.Join(d.Limits, " ")
	if !strings.Contains(joinedLimits, "字节") {
		t.Errorf("Preserve Limits must mention byte-level preservation; got %v", d.Limits)
	}
}

func TestCapability_FeaturesCoverBacklog(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	keys := map[string]bool{}
	for _, f := range m.Features {
		keys[f.Key] = true
	}
	for _, must := range []string{
		"tool.cli_ir",
		"tool.capability_manifest",
		"core.opc_relationships",
		"save.atomic",
		"text.cross_run_replace",
		"style.matrix_and_color",
		"geom.full_readonly",
		"layout.section_fonts_handout_kinsoku",
		"chart.labels_trendline_error_axis",
		"text.bodyprops_advanced",
	} {
		if !keys[must] {
			t.Errorf("feature %q missing from manifest", must)
		}
	}
}

func TestCapability_FutureWorkMarkedUntested(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	for _, f := range m.Features {
		switch f.WorkPackage {
		case "TPL-01", "DIFF-01":
			if f.Status != StatusUntested {
				t.Errorf("%s must be Untested until that WP ships, got %s (%s)",
					f.WorkPackage, f.Status, f.Key)
			}
		case "TIMIR-01":
			// TIMIR-01 已落地（M7 第三项）：Partial（空 childTnLst
			// 边缘 case 以诊断报告，见 ir/timingir.go 包注释）。
			if f.Status != StatusPartial {
				t.Errorf("TIMIR-01 must be Partial after shipping, got %s (%s)",
					f.Status, f.Key)
			}
		}
	}
}

func TestCapability_JSONRoundTrip(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("test.pptx")
	b, err := MarshalManifest(m)
	if err != nil {
		t.Fatalf("MarshalManifest: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("JSON unmarshal sanity check: %v", err)
	}
	if raw["schemaVersion"] != CapabilityManifestSchemaVersion {
		t.Errorf("schemaVersion mismatch: got %v want %v", raw["schemaVersion"], CapabilityManifestSchemaVersion)
	}
	m2, err := UnmarshalManifest(b)
	if err != nil {
		t.Fatalf("UnmarshalManifest: %v", err)
	}
	if m2.SchemaVersion != m.SchemaVersion {
		t.Errorf("round trip schemaVersion: %q vs %q", m2.SchemaVersion, m.SchemaVersion)
	}
	if _, ok := m2.Dimensions[CapabilityRender]; !ok {
		t.Error("dimensions lost in round trip")
	}
}

func TestCapability_StatusEnumMarshalJSON(t *testing.T) {
	cases := []struct {
		s    CapabilityStatus
		want string
	}{
		{StatusUntested, "Untested"},
		{StatusUnsupported, "Unsupported"},
		{StatusPartial, "Partial"},
		{StatusSupported, "Supported"},
	}
	for _, c := range cases {
		b, err := json.Marshal(c.s)
		if err != nil {
			t.Fatalf("Marshal %s: %v", c.s, err)
		}
		if string(b) != `"`+c.want+`"` {
			t.Errorf("Marshal(%s) = %s, want %q", c.s, b, c.want)
		}
		var back CapabilityStatus
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("Unmarshal %s: %v", c.s, err)
		}
		if back != c.s {
			t.Errorf("round trip %s → %v", c.s, back)
		}
	}
}

func TestCapability_StatusEnumUnmarshalRejectsUnknownSafely(t *testing.T) {
	var s CapabilityStatus
	if err := json.Unmarshal([]byte(`"SomeFutureValue"`), &s); err != nil {
		t.Fatalf("unknown status should map to Untested, got err: %v", err)
	}
	if s != StatusUntested {
		t.Errorf("unknown status must fall back to Untested, got %s", s)
	}
}

func TestCapability_SourcePassthrough(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	tmp := filepath.Join(t.TempDir(), "capability.pptx")
	if err := os.WriteFile(tmp, make([]byte, 1234), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m := p.Capability(tmp)
	if m.Source.Input != tmp {
		t.Errorf("source input passthrough failed: got %q want %q", m.Source.Input, tmp)
	}
	if m.Source.Size != 1234 {
		t.Errorf("source size passthrough failed: got %d want 1234", m.Source.Size)
	}
}

func TestCapability_SourceNonExistentStillRecordsPath(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	missing := filepath.Join(t.TempDir(), "missing.pptx")
	m := p.Capability(missing)
	if m.Source.Input != missing {
		t.Errorf("missing source should still be recorded, got %q", m.Source.Input)
	}
	if m.Source.Size != 0 {
		t.Errorf("missing source size should be 0, got %d", m.Source.Size)
	}
}

func TestCapability_FeaturesSortedStable(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	for i := 1; i < len(m.Features); i++ {
		if m.Features[i].Key < m.Features[i-1].Key {
			t.Fatalf("features must be sorted by Key: %q after %q",
				m.Features[i].Key, m.Features[i-1].Key)
		}
	}
}

func TestCapability_FeatureTargetTierAnnotated(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	for _, f := range m.Features {
		switch f.TargetTier {
		case "P", "R", "E", "F", "", "R+E", "E+F":
			// R+E / E+F 是合规的复合档位（如 M6 收口项的 R+E 子集）。
		default:
			t.Errorf("feature %s has invalid TargetTier %q", f.Key, f.TargetTier)
		}
	}
}

func TestCapability_GeneratedAtRFC3339(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	m := p.Capability("")
	if !strings.Contains(m.GeneratedAt, "T") {
		t.Errorf("GeneratedAt should be RFC3339-ish: %q", m.GeneratedAt)
	}
}

func TestCapability_NilPresentationCompat(t *testing.T) {
	m := newCapabilityManifest()
	populateCapabilityDimensions(&m)
	populateCapabilityFeatures(&m)
	if len(m.Dimensions) != 6 {
		t.Errorf("default manifest must declare 6 dimensions, got %d", len(m.Dimensions))
	}
	if m.SchemaVersion != CapabilityManifestSchemaVersion {
		t.Errorf("default schemaVersion mismatch")
	}
}

func TestCapability_SDKVersionOverride(t *testing.T) {
	orig := SDKVersion
	defer func() { SDKVersion = orig }()
	SDKVersion = "v0.42.0-test"
	m := newCapabilityManifest()
	if m.SDKVersion != "v0.42.0-test" {
		t.Errorf("SDKVersion override not picked up: %q", m.SDKVersion)
	}
}

// newTestPresentation 构造最小模板 Presentation + 自动关闭钩子。
func newTestPresentation(t *testing.T) (*Presentation, func()) {
	t.Helper()
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, func() { _ = p.Close() }
}

// 验证 opc 包可以反序列化由本 SDK 写出的最小文档（不依赖 testdata）。
func TestCapability_NoExternalFixtureRequired(t *testing.T) {
	p, closer := newTestPresentation(t)
	defer closer()
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	pk, err := opc.Load(bytes.NewReader(buf.Bytes()), int64(buf.Len()), opc.Budget{})
	if err != nil {
		t.Fatalf("opc.Load round-trip: %v", err)
	}
	main, err := pk.MainPart()
	if err != nil {
		t.Fatalf("MainPart: %v", err)
	}
	if main == "" {
		t.Fatal("main part discovery failed")
	}
}
