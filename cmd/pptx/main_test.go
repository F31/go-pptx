package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx"
)

// runCLI 在捕获的 stdout/stderr 上执行 CLI；返回 exit code + 输出快照。
// osExit/stdoutW/stderrW 都被替换，结束后恢复。
func runCLI(args []string) (ExitCode, string, string) {
	origExit := osExit
	origStdout := stdoutW
	origStderr := stderrW
	soBuf := &bytes.Buffer{}
	seBuf := &bytes.Buffer{}
	stdoutW = io.Writer(soBuf)
	stderrW = io.Writer(seBuf)
	defer func() {
		osExit = origExit
		stdoutW = origStdout
		stderrW = origStderr
	}()
	osExit = func(code ExitCode) {
		// noop: 测试中不真实退出；通过 channel 让 runCLI 接住。
		capturedCode = code
	}
	capturedCode = ExitOK
	prevArgs := os.Args
	os.Args = append([]string{"pptx"}, args...)
	defer func() { os.Args = prevArgs }()
	main()
	return capturedCode, soBuf.String(), seBuf.String()
}

var capturedCode ExitCode = ExitOK

// cliBuildDeck 创建一个含两页 + 一张柱状图的最小 PPTX 文件供 CLI 调用。
func cliBuildDeck(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layouts, _ := p.Layouts()
	if _, err := p.AddSlide(layouts[0]); err != nil {
		t.Fatalf("AddSlide 1: %v", err)
	}
	if _, err := p.AddSlide(layouts[0]); err != nil {
		t.Fatalf("AddSlide 2: %v", err)
	}
	out := filepath.Join(dir, "in.pptx")
	if _, err := p.Save(context.Background(), out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return out
}

func TestCLI_HelpAndVersion(t *testing.T) {
	code, _, _ := runCLI([]string{"help"})
	if code != ExitOK {
		t.Errorf("help exit = %v, want OK", code)
	}
	// version subcommand path
	code, out, _ := runCLI([]string{"--version"})
	if code != ExitOK {
		t.Errorf("--version exit = %v, want OK", code)
	}
	if !strings.Contains(out, "pptx") {
		t.Errorf("version output missing 'pptx': %s", out)
	}
}

func TestCLI_UnknownSubcommand(t *testing.T) {
	code, _, err := runCLI([]string{"bogus"})
	if code != ExitUsageError {
		t.Errorf("unknown subcommand exit = %v, want %v", code, ExitUsageError)
	}
	if !strings.Contains(err, "unknown subcommand") {
		t.Errorf("stderr missing 'unknown subcommand': %s", err)
	}
}

func TestCLI_Inspect_Basic(t *testing.T) {
	in := cliBuildDeck(t)
	code, out, err := runCLI([]string{"inspect", in})
	if code != ExitOK {
		t.Fatalf("inspect exit = %v, err=%s", code, err)
	}
	var summary inspectSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("inspect JSON: %v\nout=%s", err, out)
	}
	if summary.Input != in {
		t.Errorf("inspect input = %q, want %q", summary.Input, in)
	}
	if summary.Pages != 2 {
		t.Errorf("inspect pages = %d, want 2", summary.Pages)
	}
	if summary.Document == nil {
		t.Fatal("document is nil")
	}
	if summary.Document.SchemaVersion != "go-pptx.ir/1.0" {
		t.Errorf("IR schemaVersion = %q", summary.Document.SchemaVersion)
	}
}

func TestCLI_Validate_Basic(t *testing.T) {
	in := cliBuildDeck(t)
	code, out, err := runCLI([]string{"validate", in})
	if code != ExitOK {
		t.Fatalf("validate exit = %v, err=%s", code, err)
	}
	var v validateSummary
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("validate JSON: %v", err)
	}
	if v.Level != "structural" {
		t.Errorf("level = %q, want structural", v.Level)
	}
}

func TestCLI_ExportIR_StdoutAndOutput(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	outFile := filepath.Join(dir, "doc.json")

	// stdout 路径
	code, stdout, _ := runCLI([]string{"export-ir", "--stdout", in})
	if code != ExitOK {
		t.Fatalf("export-ir --stdout exit = %v", code)
	}
	if !strings.Contains(stdout, "go-pptx.ir/1.0") {
		t.Errorf("stdout IR missing schemaVersion: %s", stdout)
	}

	// --output 路径 + --overwrite
	code, _, err := runCLI([]string{"export-ir", "--output", outFile, "--overwrite", in})
	if code != ExitOK {
		t.Fatalf("export-ir --output exit = %v, err=%s", code, err)
	}
	data, _ := os.ReadFile(outFile)
	var ir struct {
		Pages []map[string]any `json:"pages"`
	}
	if err := json.Unmarshal(data, &ir); err != nil {
		t.Fatalf("parse written IR: %v", err)
	}
	if len(ir.Pages) != 2 {
		t.Errorf("written IR pages = %d, want 2", len(ir.Pages))
	}

	// 重输出 + 无 --overwrite 应失败
	if err := os.WriteFile(outFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("touch: %v", err)
	}
	code, _, stderrOut := runCLI([]string{"export-ir", "--output", outFile, in})
	if code != ExitResource {
		t.Errorf("missing --overwrite exit = %v, want %v", code, ExitResource)
	}
	if !strings.Contains(stderrOut, "--overwrite") {
		t.Errorf("stderr missing --overwrite hint: %s", stderrOut)
	}
}

func TestCLI_TimingPlan_Basic(t *testing.T) {
	in := cliBuildDeck(t)
	code, out, _ := runCLI([]string{"timing-plan", "--tail-padding", "500ms", in})
	if code != ExitOK {
		t.Fatalf("timing-plan exit = %v", code)
	}
	var plan timingPlanReport
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("timing-plan JSON: %v\nout=%s", err, out)
	}
	if plan.Policy != "skip" {
		t.Errorf("policy = %q, want skip", plan.Policy)
	}
	if plan.TailPadMs != 500 {
		t.Errorf("tailPadMs = %d, want 500", plan.TailPadMs)
	}
}

func TestCLI_Replace_HappyPathAndExitCodes(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.pptx")

	// 缺 --output → ExitUsageError
	code, _, err := runCLI([]string{"replace", "--old", "foo", "--new", "bar", in})
	if code != ExitUsageError {
		t.Errorf("missing --output exit = %v, want %v", code, ExitUsageError)
	}
	if !strings.Contains(err, "--output") {
		t.Errorf("stderr missing --output hint: %s", err)
	}

	code, out, err := runCLI([]string{"replace", "--old", "x", "--new", "y", "--output", outFile, in})
	if code != ExitOK {
		t.Fatalf("replace exit = %v, err=%s", code, err)
	}
	var rep replaceReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("replace JSON: %v", err)
	}
	if rep.Status != "ok" {
		t.Errorf("status = %q, want ok", rep.Status)
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Errorf("output not written: %v", err)
	}

	// 覆盖存在的目标 → 需 --overwrite
	code, _, stderrOut := runCLI([]string{"replace", "--old", "x", "--new", "y", "--output", outFile, in})
	if code != ExitResource {
		t.Errorf("overwrite missing exit = %v, want %v", code, ExitResource)
	}
	if !strings.Contains(stderrOut, "--overwrite") {
		t.Errorf("stderr missing --overwrite hint: %s", stderrOut)
	}
}

func TestCLI_Narrate_ManifestSchemaMismatch(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.pptx")
	manifestPath := filepath.Join(dir, "tracks.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schemaVersion":"go-pptx.tracks/9999.0","tracks":[]}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	code, _, stderrOut := runCLI([]string{"narrate", "--manifest", manifestPath, "--output", outFile, in})
	if code != ExitRuntimeError {
		t.Errorf("mismatched schema exit = %v, want %v", code, ExitRuntimeError)
	}
	if !strings.Contains(stderrOut, "schemaVersion") {
		t.Errorf("stderr missing schemaVersion: %s", stderrOut)
	}
}

func TestCLI_Narrate_MalformedManifest(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.pptx")
	manifestPath := filepath.Join(dir, "tracks.json")
	if err := os.WriteFile(manifestPath, []byte(`not json`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	code, _, _ := runCLI([]string{"narrate", "--manifest", manifestPath, "--output", outFile, in})
	if code == ExitOK {
		t.Errorf("malformed manifest exit OK; expected error")
	}
}

func TestCLI_Narrate_HappyPath(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.pptx")
	manifestPath := filepath.Join(dir, "tracks.json")
	mf := `{
  "schemaVersion":"go-pptx.tracks/1.0",
  "tracks":[
    {"slideID":256,"trackKey":"track-A","source":"trackA.wav","role":"narration","durationMs":5000,"startDelayMs":0,"trigger":"onEnter"},
    {"slideID":257,"trackKey":"track-B","source":"trackB.wav","role":"narration","durationMs":3000,"startDelayMs":0,"trigger":"onEnter"}
  ]
}`
	if err := os.WriteFile(manifestPath, []byte(mf), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	code, out, _ := runCLI([]string{"narrate", "--manifest", manifestPath, "--output", outFile, in})
	if code != ExitOK {
		t.Fatalf("narrate exit = %v", code)
	}
	var rep narrateReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("narrate JSON: %v", err)
	}
	if rep.AppliedTo != 2 {
		t.Errorf("appliedTo = %d, want 2", rep.AppliedTo)
	}
	if rep.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", rep.Skipped)
	}
}

func TestCLI_ExitCodeConsistency(t *testing.T) {
	// ExitCode 常量值与方案 §23.2 对齐（0/1/2/3/4）。
	wantCodes := map[ExitCode]int{
		ExitOK:           0,
		ExitRuntimeError: 1,
		ExitUsageError:   2,
		ExitCapability:   3,
		ExitResource:     4,
	}
	for c, want := range wantCodes {
		if int(c) != want {
			t.Errorf("ExitCode %d = %d, want %d", c, int(c), want)
		}
	}
	_ = time.Second
}
