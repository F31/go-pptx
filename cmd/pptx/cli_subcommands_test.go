package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/go-pptx"
)

// 本文件覆盖 cmd/pptx 各子命令的错误路径与公共 helper：
//   - happy path 已在 main_test.go 验证
//   - 本文件补全 cmdRun{Inspect,Capability,Validate,TimingPlan,Replace,ExportIR,Narrate}
//     的参数校验失败 / 资源不存在 / 标志冲突分支
//   - 覆盖 classifyError / parseReplaceModeFlag / flagOrJSON / rejectOutputFlag
//     等公共 helper 单元测试

// cliWriteDeck 带一笔可见正文（"ORIG"）供测试 Contains 断言。
func cliWriteDeck(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	layouts, _ := p.Layouts()
	s, err := p.AddSlide(layouts[0])
	if err != nil {
		t.Fatalf("AddSlide: %v", err)
	}
	tb, err := s.AddTextBox(pptx.TextBoxSpec{X: 100, Y: 100, Width: 400, Height: 80, Text: "ORIG Body"})
	if err != nil {
		t.Fatalf("AddTextBox: %v", err)
	}
	if tb == nil {
		t.Fatal("AddTextBox returned nil")
	}
	out := dir + "/in.pptx"
	if _, err := p.Save(context.Background(), out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return out
}

// ---------- inspect ----------

func TestCLI_Inspect_ErrorPaths(t *testing.T) {
	in := cliWriteDeck(t)

	// 缺位置参数：ExitUsageError，stderr 含 "missing input path"。
	t.Run("missing_input", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"inspect"})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "missing input path") {
			t.Errorf("stderr missing hint: %s", stderr)
		}
	})

	// unknown flag：flag.ContinueOnError 报 flag error → ExitUsageError。
	t.Run("unknown_flag", func(t *testing.T) {
		code, _, _ := runCLI([]string{"inspect", "--bogus", in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
	})

	// 只读子命令 inspect 的 fs 未注册 --output，传 --output 走 fs.ContinueOnError
	// 报 "flag provided but not defined"。语义上等同于 unknown flag。
	// 单独的 rejectOutputFlag helper 测在 TestRejectOutputFlag。

	// 输入不存在：openPresentation → classifyError(ErrMalformedPackage/ErrEncoding)
	// → ExitCapability 或 ExitRuntimeError；至少非 ExitOK。
	t.Run("missing_file", func(t *testing.T) {
		code, _, _ := runCLI([]string{"inspect", "/nonexistent/missing.pptx"})
		if code == ExitOK {
			t.Errorf("missing file exit OK; want non-zero")
		}
	})
}

// ---------- capability（无 happy path 测试，全套补齐） ----------

func TestCLI_Capability_HappyPath(t *testing.T) {
	in := cliWriteDeck(t)
	code, out, _ := runCLI([]string{"capability", in})
	if code != ExitOK {
		t.Fatalf("capability exit = %v", code)
	}
	// 输出必须为合法 JSON；包含必要字段。
	var doc struct {
		SchemaVersion string                        `json:"schemaVersion"`
		Source        pptx.CapabilityManifestSource `json:"source"`
	}
	// output 含 features/dimensions/diagnostics 等其它字段；用非严格解码。
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("capability JSON: %v\nout=%s", err, out)
	}
	if doc.SchemaVersion != "go-pptx.capability/1.0" {
		t.Errorf("schemaVersion = %q", doc.SchemaVersion)
	}
	if doc.Source.Input != in {
		t.Errorf("source.input = %q, want %q", doc.Source.Input, in)
	}
}

func TestCLI_Capability_ErrorPaths(t *testing.T) {
	dir := t.TempDir()
	in := cliWriteDeck(t)

	t.Run("missing_input", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"capability"})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "missing input path") {
			t.Errorf("stderr missing hint: %s", stderr)
		}
	})

	t.Run("unknown_flag", func(t *testing.T) {
		code, _, _ := runCLI([]string{"capability", "--bogus", in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
	})

	// capability 只读子命令未注册 --output，传 --output 走 fs.ContinueOnError
	// 报 "flag provided but not defined"（与 unknown flag 同路径）。

	t.Run("missing_file", func(t *testing.T) {
		code, _, _ := runCLI([]string{"capability", "/nonexistent/missing.pptx"})
		if code == ExitOK {
			t.Errorf("missing file exit OK; want non-zero")
		}
	})

	// 空路径边界：openPresentation 内部判 path=="" → ExitUsageError。
	t.Run("empty_path_via_helper", func(t *testing.T) {
		_, _, code := openPresentation("")
		if code != ExitUsageError {
			t.Errorf("openPresentation(empty) code = %v, want %v", code, ExitUsageError)
		}
	})
	_ = dir
}

// ---------- validate ----------

func TestCLI_Validate_ErrorPaths(t *testing.T) {
	in := cliWriteDeck(t)

	t.Run("missing_input", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"validate"})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "missing input path") {
			t.Errorf("stderr missing hint: %s", stderr)
		}
	})

	t.Run("unsupported_level", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"validate", "--level", "semantic", in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "unsupported level") {
			t.Errorf("stderr missing unsupported level: %s", stderr)
		}
	})

	// validate 只读子命令未注册 --output，传 --output 走 fs.ContinueOnError
	// 报 "flag provided but not defined"（与 unknown flag 同路径）。

	t.Run("missing_file", func(t *testing.T) {
		code, _, _ := runCLI([]string{"validate", "/nonexistent/missing.pptx"})
		if code == ExitOK {
			t.Errorf("missing file exit OK; want non-zero")
		}
	})
}

// ---------- timing-plan ----------

func TestCLI_TimingPlan_StrictFlag(t *testing.T) {
	in := cliWriteDeck(t)
	code, out, _ := runCLI([]string{"timing-plan", "--strict", in})
	if code != ExitOK {
		t.Fatalf("timing-plan --strict exit = %v", code)
	}
	// planPolicy strict=true → "fail"。
	if !strings.Contains(out, `"policy": "fail"`) {
		t.Errorf("strict policy missing: %s", out)
	}
}

func TestCLI_TimingPlan_PlanPolicyHelper(t *testing.T) {
	if got := planPolicy(true); got != "fail" {
		t.Errorf("planPolicy(true) = %q, want fail", got)
	}
	if got := planPolicy(false); got != "skip" {
		t.Errorf("planPolicy(false) = %q, want skip", got)
	}
}

func TestCLI_TimingPlan_ErrorPaths(t *testing.T) {
	t.Run("missing_input", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"timing-plan"})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "missing input path") {
			t.Errorf("stderr missing hint: %s", stderr)
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		code, _, _ := runCLI([]string{"timing-plan", "/nonexistent/missing.pptx"})
		if code == ExitOK {
			t.Errorf("missing file exit OK; want non-zero")
		}
	})
}

// ---------- replace ----------

func TestCLI_Replace_ErrorPaths(t *testing.T) {
	in := cliWriteDeck(t)

	t.Run("missing_input", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"replace", "--old", "x", "--new", "y", "--output", "o.pptx"})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "missing input path") {
			t.Errorf("stderr missing hint: %s", stderr)
		}
	})

	t.Run("missing_old", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"replace", "--new", "y", "--output", "o.pptx", in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "--old is required") {
			t.Errorf("stderr missing --old hint: %s", stderr)
		}
	})

	t.Run("missing_output", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"replace", "--old", "x", "--new", "y", in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "--output is required") {
			t.Errorf("stderr missing --output hint: %s", stderr)
		}
	})

	t.Run("unknown_mode", func(t *testing.T) {
		dir := t.TempDir()
		outFile := dir + "/o.pptx"
		code, _, stderr := runCLI([]string{"replace", "--old", "x", "--new", "y", "--mode", "bogus", "--output", outFile, in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "unknown mode") {
			t.Errorf("stderr missing mode hint: %s", stderr)
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		dir := t.TempDir()
		outFile := dir + "/o.pptx"
		code, _, _ := runCLI([]string{"replace", "--old", "x", "--new", "y", "--output", outFile, "/nonexistent/missing.pptx"})
		if code == ExitOK {
			t.Errorf("missing input file exit OK; want non-zero")
		}
	})
}

// ---------- export-ir ----------

func TestCLI_ExportIR_ErrorPaths(t *testing.T) {
	in := cliWriteDeck(t)

	t.Run("missing_input", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"export-ir", "--stdout"})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "missing input path") {
			t.Errorf("stderr missing hint: %s", stderr)
		}
	})

	t.Run("output_and_stdout_mutually_exclusive", func(t *testing.T) {
		dir := t.TempDir()
		outFile := dir + "/x.json"
		code, _, stderr := runCLI([]string{"export-ir", "--stdout", "--output", outFile, in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "mutually exclusive") {
			t.Errorf("stderr missing mutex hint: %s", stderr)
		}
	})

	t.Run("output_exists_without_overwrite", func(t *testing.T) {
		dir := t.TempDir()
		outFile := dir + "/x.json"
		if err := os.WriteFile(outFile, []byte("{}"), 0o644); err != nil {
			t.Fatalf("touch: %v", err)
		}
		code, _, stderr := runCLI([]string{"export-ir", "--output", outFile, in})
		if code != ExitResource {
			t.Errorf("exit = %v, want %v", code, ExitResource)
		}
		if !strings.Contains(stderr, "--overwrite") {
			t.Errorf("stderr missing --overwrite hint: %s", stderr)
		}
	})
}

// ---------- narrate ----------

func TestCLI_Narrate_ErrorPaths(t *testing.T) {
	in := cliWriteDeck(t)

	t.Run("missing_input", func(t *testing.T) {
		dir := t.TempDir()
		code, _, stderr := runCLI([]string{"narrate", "--manifest", dir + "/m.json", "--output", dir + "/o.pptx"})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "missing input path") {
			t.Errorf("stderr missing hint: %s", stderr)
		}
	})

	t.Run("missing_manifest", func(t *testing.T) {
		code, _, stderr := runCLI([]string{"narrate", "--output", "o.pptx", in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "--manifest is required") {
			t.Errorf("stderr missing manifest hint: %s", stderr)
		}
	})

	t.Run("missing_output", func(t *testing.T) {
		dir := t.TempDir()
		manPath := dir + "/m.json"
		if err := os.WriteFile(manPath, []byte(`{"schemaVersion":"go-pptx.tracks/1.0","tracks":[]}`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		code, _, stderr := runCLI([]string{"narrate", "--manifest", manPath, in})
		if code != ExitUsageError {
			t.Errorf("exit = %v, want %v", code, ExitUsageError)
		}
		if !strings.Contains(stderr, "--output is required") {
			t.Errorf("stderr missing --output hint: %s", stderr)
		}
	})

	t.Run("schema_version_missing", func(t *testing.T) {
		dir := t.TempDir()
		outFile := dir + "/o.pptx"
		manPath := dir + "/m.json"
		if err := os.WriteFile(manPath, []byte(`{"tracks":[]}`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		code, _, stderr := runCLI([]string{"narrate", "--manifest", manPath, "--output", outFile, in})
		if code == ExitOK {
			t.Errorf("missing schemaVersion exit OK; want non-zero")
		}
		if !strings.Contains(stderr, "schemaVersion missing") {
			t.Errorf("stderr missing schemaVersion: %s", stderr)
		}
	})

	t.Run("duplicate_track_key", func(t *testing.T) {
		dir := t.TempDir()
		outFile := dir + "/o.pptx"
		manPath := dir + "/m.json"
		// 重复 trackKey A1。
		mf := `{"schemaVersion":"go-pptx.tracks/1.0","tracks":[
			{"slideID":256,"trackKey":"A1","source":"a.wav","role":"narration","durationMs":100,"startDelayMs":0,"trigger":"onEnter"},
			{"slideID":257,"trackKey":"A1","source":"b.wav","role":"narration","durationMs":100,"startDelayMs":0,"trigger":"onEnter"}
		]}`
		if err := os.WriteFile(manPath, []byte(mf), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		code, _, stderr := runCLI([]string{"narrate", "--manifest", manPath, "--output", outFile, in})
		if code == ExitOK {
			t.Errorf("duplicate key exit OK; want non-zero")
		}
		if !strings.Contains(stderr, "duplicate") {
			t.Errorf("stderr missing duplicate hint: %s", stderr)
		}
	})
}

// ---------- 公共 helper 单元测试 ----------

func TestParseReplaceModeFlag(t *testing.T) {
	cases := map[string]struct {
		wantMode pptx.ReplaceMode
		wantOK   bool
	}{
		"firstChar":     {pptx.ReplaceFirstCharacter, true},
		"first-char":    {pptx.ReplaceFirstCharacter, true},
		"FirstChar":     {pptx.ReplaceFirstCharacter, true}, // 大小写不敏感
		"equal":         {pptx.ReplaceEqualLengthPerRune, true},
		"equal-length":  {pptx.ReplaceEqualLengthPerRune, true},
		"explicit":      {pptx.ReplaceExplicitStyle, true},
		"explicitstyle": {pptx.ReplaceExplicitStyle, true},
		"bogus":         {pptx.ReplaceFirstCharacter, false}, // any zero + false
		"":              {pptx.ReplaceFirstCharacter, false},
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			mode, ok := parseReplaceModeFlag(in)
			if want.wantOK {
				if !ok || mode != want.wantMode {
					t.Errorf("parseReplaceModeFlag(%q) = (%v, %v), want (%v, true)", in, mode, ok, want.wantMode)
				}
			} else {
				if ok {
					t.Errorf("parseReplaceModeFlag(%q) ok=true, want false", in)
				}
			}
		})
	}
}

func TestFlagOrJSON(t *testing.T) {
	// --json 在中间：jsonMode=true，rest 保留其余项（顺序不变）。
	jsonMode, rest := flagOrJSON([]string{"--json", "in.pptx"})
	if !jsonMode {
		t.Errorf("jsonMode = false, want true")
	}
	if len(rest) != 1 || rest[0] != "in.pptx" {
		t.Errorf("rest = %v, want [in.pptx]", rest)
	}

	// --json 在末尾：jsonMode=true，rest 保留前项。
	jsonMode, rest = flagOrJSON([]string{"a", "b", "--json"})
	if !jsonMode {
		t.Errorf("expected jsonMode=true (flag位置无关)")
	}
	if len(rest) != 2 || rest[0] != "a" || rest[1] != "b" {
		t.Errorf("rest = %v, want [a b]", rest)
	}

	// 无 --json：jsonMode=false，rest = 原 args。
	jsonMode, rest = flagOrJSON([]string{"a", "b"})
	if jsonMode {
		t.Errorf("expected jsonMode=false")
	}
	if len(rest) != 2 || rest[0] != "a" || rest[1] != "b" {
		t.Errorf("rest = %v", rest)
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		fallback ExitCode
		want     ExitCode
	}{
		{"InvalidArgument", pptx.ErrInvalidArgument, ExitRuntimeError, ExitUsageError},
		{"OutOfRange", pptx.ErrOutOfRange, ExitRuntimeError, ExitUsageError},
		{"ValidationFailed", pptx.ErrValidationFailed, ExitRuntimeError, ExitCapability},
		{"UnsupportedEdit", pptx.ErrUnsupportedEdit, ExitRuntimeError, ExitCapability},
		{"UnsupportedFormat", pptx.ErrUnsupportedFormat, ExitRuntimeError, ExitCapability},
		{"Closed", pptx.ErrClosed, ExitRuntimeError, ExitCapability},
		{"LimitExceeded", pptx.ErrLimitExceeded, ExitRuntimeError, ExitResource},
		{"OutputExists", pptx.ErrOutputExists, ExitRuntimeError, ExitResource},
		{"UnknownError", errors.New("opaque"), ExitRuntimeError, ExitRuntimeError},
		{"Fallback_Soft", errors.New("opaque"), ExitOK, ExitOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyError(c.err, c.fallback)
			if got != c.want {
				t.Errorf("classifyError(%v, %v) = %v, want %v", c.err, c.fallback, got, c.want)
			}
		})
	}
}

func TestRejectOutputFlag(t *testing.T) {
	// 不含 --output 的 FlagSet → ExitOK。
	fsOK := flag.NewFlagSet("inspect", flag.ContinueOnError)
	if code := rejectOutputFlag(fsOK); code != ExitOK {
		t.Errorf("without --output = %v, want OK", code)
	}

	// 含 --output 的 FlagSet → ExitUsageError，stderr 含 "--output not supported"。
	fsReject := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fsReject.String("output", "", "output path")
	if code := rejectOutputFlag(fsReject); code != ExitUsageError {
		t.Errorf("with --output = %v, want UsageError", code)
	}
}

func TestContextFromArgsPreservesArgs(t *testing.T) {
	args := []string{"inspect", "input.pptx"}
	ctx, out := contextFromArgs(args)
	if ctx == nil {
		t.Fatal("context is nil")
	}
	if len(out) != len(args) || out[0] != args[0] || out[1] != args[1] {
		t.Fatalf("args = %v, want %v", out, args)
	}
}

func TestUsageBind(t *testing.T) {
	orig := stderrW
	var b strings.Builder
	stderrW = &b
	defer func() { stderrW = orig }()
	usageBind()
	if got := b.String(); !strings.Contains(got, "pptx bind") || !strings.Contains(got, "--data") || !strings.Contains(got, "--output") {
		t.Fatalf("usageBind output = %q", got)
	}
}

func TestOutputHelpers(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "out.pptx")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if f, err := openFileForWrite(existing, false); err == nil {
		_ = f.Close()
		t.Fatal("openFileForWrite existing without overwrite succeeded")
	}
	created := filepath.Join(dir, "created.pptx")
	f, err := openFileForWrite(created, false)
	if err != nil {
		t.Fatalf("openFileForWrite create: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := openFileForWrite(dir, true); err == nil {
		t.Fatal("openFileForWrite directory path succeeded")
	}
	if got := resolveOutputPath(""); got != "" {
		t.Fatalf("resolve empty = %q", got)
	}
	if got := resolveOutputPath("relative.pptx"); !filepath.IsAbs(got) || !strings.HasSuffix(got, "relative.pptx") {
		t.Fatalf("resolve relative = %q", got)
	}
}
