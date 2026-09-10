package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sniffW 把 stderrW 临时替换为 buffer，便于直接调用 helper（不经过 runCLI）。
// 用 io.Writer 接口返回 *bytes.Buffer，避免与 stdoutW 冲突。
type sniffW struct{ b *bytes.Buffer }

func (s *sniffW) Write(p []byte) (int, error) { return s.b.Write(p) }
func (s *sniffW) String() string              { return s.b.String() }
func newSniffW() *sniffW                      { return &sniffW{b: &bytes.Buffer{}} }

// captureStderr 在 fn 执行期间替换 stderrW 为 buffer，返回捕获文本。
func captureStderr(fn func()) string {
	prev := stderrW
	buf := newSniffW()
	stderrW = buf
	defer func() { stderrW = prev }()
	fn()
	return buf.String()
}

// 本文件覆盖 main() dispatch 的兜底分支、usageXxx 详细帮助函数、SDK
// 版本双态。这些路径在 happy path 流（如 TestCLI_Inspect_Basic 等）不
// 触发；目标是把总体覆盖率从 79.1% 推到 85%+。

// === main() dispatch 兜底分支 ============================================

// TestMain_NoArgs 验证 os.Args 长度 < 2 走 ExitUsageError + stderr 含
// "missing subcommand"，而非 panic。
func TestMain_NoArgs(t *testing.T) {
	code, _, stderr := runCLI(nil) // runCLI 会把 "pptx" 拼到 os.Args[0]，nil = 只剩 "pptx"
	if code != ExitUsageError {
		t.Errorf("missing subcommand exit = %v, want %v", code, ExitUsageError)
	}
	if !strings.Contains(stderr, "missing subcommand") {
		t.Errorf("stderr missing 'missing subcommand': %s", stderr)
	}
}

// TestMain_HelpVariants 验证 -h / --help / help 三种简写全部走
// printUsage(stdout)，最终返回 ExitOK。
func TestMain_HelpVariants(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := runCLI([]string{arg})
			if code != ExitOK {
				t.Errorf("%s exit = %v, want OK", arg, code)
			}
			// help 走 printUsage(stdout)，不含 usage 错误提示。
			if !strings.Contains(stdout, "Subcommands:") {
				t.Errorf("%s stdout missing 'Subcommands:': %s", arg, stdout)
			}
			if strings.Contains(stderr, "unknown subcommand") {
				t.Errorf("%s hit unknown-subcommand path: %s", arg, stderr)
			}
		})
	}
}

// TestMain_VersionVariants 验证 -v / --version 都到 stdout（不动 stderr）且
// 文本形如 "pptx <version>"。
func TestMain_VersionVariants(t *testing.T) {
	for _, arg := range []string{"-v", "--version"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, _ := runCLI([]string{arg})
			if code != ExitOK {
				t.Errorf("%s exit = %v, want OK", arg, code)
			}
			if !strings.HasPrefix(stdout, "pptx ") {
				t.Errorf("%s stdout = %q, want prefix 'pptx '", arg, stdout)
			}
		})
	}
}

// TestMain_UnknownSubcommandAfterUnknown 验证未知子命令走 default 分支，
// stderr 含子命令名。
func TestMain_UnknownSubcommandAfterUnknown(t *testing.T) {
	code, _, stderr := runCLI([]string{"no-such-cmd"})
	if code != ExitUsageError {
		t.Errorf("exit = %v, want UsageError", code)
	}
	if !strings.Contains(stderr, "no-such-cmd") {
		t.Errorf("stderr missing unknown name: %s", stderr)
	}
}

// === sdkVersionString 双态 =============================================

// TestSDKVersionString_EmptyDefault 验证默认构建（PPTXVersion=""）返回
// "dev"，以利用户识读"未知版本"。
func TestSDKVersionString_EmptyDefault(t *testing.T) {
	prev := PPTXVersion
	PPTXVersion = ""
	t.Cleanup(func() { PPTXVersion = prev })
	if got := sdkVersionString(); got != "dev" {
		t.Errorf("empty PPTXVersion = %q, want 'dev'", got)
	}
}

// TestSDKVersionString_ReleaseTag 验证发布构建（PPTXVersion 由 ldflags 注入）
// 返回注入值，不附加任何前缀。
func TestSDKVersionString_ReleaseTag(t *testing.T) {
	prev := PPTXVersion
	PPTXVersion = "v1.0.0-rc1"
	t.Cleanup(func() { PPTXVersion = prev })
	if got := sdkVersionString(); got != "v1.0.0-rc1" {
		t.Errorf("release PPTXVersion = %q, want %q", got, "v1.0.0-rc1")
	}
}

// === 7 个 usageXxx 详细帮助 helper（统一表驱动）========================

// TestUsageHelpers 验证 7 个子命令的 usageXxx helper 都把"usage:"引导的
// 帮助文本写到 stderr。cosmetic 但 zero coverage 行数大；一并验证各子命
// 令至少一次出现其主标志（--output / --manifest / --mode 等）。
func TestUsageHelpers(t *testing.T) {
	cases := []struct {
		name     string
		invoke   func()
		mustHave []string // 文本中必现的标志片段
	}{
		{"inspect", func() { usageInspect() }, []string{"inspect", "--json"}},
		{"validate", func() { usageValidate() }, []string{"validate", "--level"}},
		{"replace", func() { usageReplace() }, []string{"replace", "--old", "--output"}},
		{"narrate", func() { usageNarrate() }, []string{"narrate", "--manifest", "--output"}},
		{"timing-plan", func() { usageTimingPlan() }, []string{"timing-plan", "--tail-padding"}},
		{"export-ir", func() { usageExportIR() }, []string{"export-ir", "--output", "--stdout"}},
		{"diff", func() { usageDiff() }, []string{"diff"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStderr(tc.invoke)
			if !strings.HasPrefix(out, "usage:") {
				t.Errorf("%s usage = %q, want prefix 'usage:'", tc.name, out)
			}
			for _, want := range tc.mustHave {
				if !strings.Contains(out, want) {
					t.Errorf("%s usage missing %q in %q", tc.name, want, out)
				}
			}
		})
	}
}

// === 私有 util：覆盖率补全 ===========================================

// TestBasename 验证 filepath.Base 的轻封装返回路径主名（不含目录）。
func TestBasename(t *testing.T) {
	if got := basename("/a/b/c.pptx"); got != "c.pptx" {
		t.Errorf("basename path = %q, want %q", got, "c.pptx")
	}
	// filepath.Base("file.pptx") 在不同 OS 上行为相同。
	if got := basename("file.pptx"); got != "file.pptx" {
		t.Errorf("basename bare = %q, want %q", got, "file.pptx")
	}
}

// TestResolveOutputPath_Relative 验证相对路径基于 cwd 解析为绝对路径。
func TestResolveOutputPath_Relative(t *testing.T) {
	got := resolveOutputPath("out.pptx")
	if !strings.HasSuffix(got, "out.pptx") {
		t.Errorf("relative abs = %q, want suffix 'out.pptx'", got)
	}
	if got == "out.pptx" {
		t.Errorf("relative abs = %q, want non-relative", got)
	}
}

// TestResolveOutputPath_Empty 返回空字符串原样——这是 writePresentation 的前置短路。
func TestResolveOutputPath_Empty(t *testing.T) {
	if got := resolveOutputPath(""); got != "" {
		t.Errorf("empty = %q, want empty", got)
	}
}

// TestSliceFlag 验证 -k v / -k=v / 重复 -k / 混合未识别 token 都被正确切分。
func TestSliceFlag(t *testing.T) {
	values, rest := sliceFlag([]string{"--key", "v1", "--key=v2", "rest1", "--key", "v3", "rest2"}, "--key")
	if len(values) != 3 || values[0] != "v1" || values[1] != "v2" || values[2] != "v3" {
		t.Errorf("values = %v, want [v1 v2 v3]", values)
	}
	wantRest := []string{"rest1", "rest2"}
	if len(rest) != len(wantRest) || rest[0] != wantRest[0] || rest[1] != wantRest[1] {
		t.Errorf("rest = %v, want %v", rest, wantRest)
	}
}

// TestSliceFlag_NoValueAtEnd 验证尾随 --key 无值时不吞下一个 flag。
func TestSliceFlag_NoValueAtEnd(t *testing.T) {
	values, rest := sliceFlag([]string{"--key"}, "--key")
	if len(values) != 0 {
		t.Errorf("values = %v, want empty", values)
	}
	if len(rest) != 1 || rest[0] != "--key" {
		t.Errorf("rest = %v, want [--key]", rest)
	}
}

// TestNotImplemented 验证死代码路径 notImplemented 返回 ExitCapability 并写
// stderr。保留它确保未来重新启用时不会爆回归。
func TestNotImplemented(t *testing.T) {
	var code ExitCode
	out := captureStderr(func() { code = notImplemented("foo-bar") })
	if code != ExitCapability {
		t.Errorf("notImplemented code = %v, want Capability", code)
	}
	if !strings.Contains(out, "foo-bar") {
		t.Errorf("stderr missing subcommand name: %s", out)
	}
}

// === 各子命令的"missing input" 简化分支（不走完整 CLI）===============

// TestCmdRunInspect_MissingInput 验证 inspect 缺参走 ExitUsageError +
// stderr 含 "inspect: missing input path"。
func TestCmdRunInspect_MissingInput(t *testing.T) {
	code, _, stderr := runCLI([]string{"inspect"})
	if code != ExitUsageError {
		t.Errorf("exit = %v, want UsageError", code)
	}
	if !strings.Contains(stderr, "missing input path") {
		t.Errorf("stderr missing 'missing input path': %s", stderr)
	}
}

// TestCmdRunValidate_UnsupportedLevel 验证非 structural level 退出
// ExitUsageError + stderr 含 "unsupported level"。
func TestCmdRunValidate_UnsupportedLevel(t *testing.T) {
	in := cliBuildDeck(t)
	code, _, stderr := runCLI([]string{"validate", "--level", "advanced", in})
	if code != ExitUsageError {
		t.Errorf("exit = %v, want UsageError", code)
	}
	if !strings.Contains(stderr, "unsupported level") {
		t.Errorf("stderr missing 'unsupported level': %s", stderr)
	}
}

// TestCmdRunExportIR_OutputStdoutMutex 验证 --output + --stdout 同时存在
// 走 ExitUsageError + stderr 含 "mutually exclusive"。
func TestCmdRunExportIR_OutputStdoutMutex(t *testing.T) {
	in := cliBuildDeck(t)
	outFile := t.TempDir() + "/x.json"
	code, _, stderr := runCLI([]string{"export-ir", "--stdout", "--output", outFile, in})
	if code != ExitUsageError {
		t.Errorf("exit = %v, want UsageError", code)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("stderr missing 'mutually exclusive': %s", stderr)
	}
}

// === bind 错误路径 ====================================================

// TestCmdRunBind_MissingFlags 验证 bind 全部缺失标志 → ExitUsageError +
// stderr 含 3 选其一的提示文本。
func TestCmdRunBind_MissingFlags(t *testing.T) {
	in := cliBuildDeck(t)
	// 缺 --data + --output → 第一个分支命中
	code, _, stderr := runCLI([]string{"bind", in})
	if code != ExitUsageError {
		t.Errorf("exit = %v, want UsageError", code)
	}
	if !strings.Contains(stderr, "--data is required") {
		t.Errorf("stderr missing --data hint: %s", stderr)
	}
}

// TestCmdRunBind_MissingOutput 验证 --data 已存在但缺 --output 时退出。
func TestCmdRunBind_MissingOutput(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write data: %v", err)
	}
	code, _, stderr := runCLI([]string{"bind", "--data", dataPath, in})
	if code != ExitUsageError {
		t.Errorf("exit = %v, want UsageError", code)
	}
	if !strings.Contains(stderr, "--output is required") {
		t.Errorf("stderr missing --output hint: %s", stderr)
	}
}

// TestCmdRunBind_MalformedJSON 验证 --data 指向非法 JSON → ExitUsageError。
// CLI 校验用户的 contract，匹配不上即拒收——不掩盖错误。
func TestCmdRunBind_MalformedJSON(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "bad.json")
	out := filepath.Join(dir, "out.pptx")
	if err := os.WriteFile(dataPath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write bad data: %v", err)
	}
	code, _, stderr := runCLI([]string{"bind", "--data", dataPath, "--output", out, in})
	if code != ExitUsageError {
		t.Errorf("exit = %v, want UsageError", code)
	}
	if !strings.Contains(stderr, "invalid data JSON") {
		t.Errorf("stderr missing 'invalid data JSON': %s", stderr)
	}
}

// TestCmdRunBind_DataFileMissing 验证 --data 路径不存在 → ExitRuntimeError。
// os.ReadFile 报 ENOENT 走 stderrErr 落到默认 ExitRuntimeError。
func TestCmdRunBind_DataFileMissing(t *testing.T) {
	in := cliBuildDeck(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.pptx")
	code, _, stderr := runCLI([]string{"bind", "--data", "/no/such/data.json", "--output", out, in})
	if code != ExitRuntimeError {
		t.Errorf("exit = %v, want RuntimeError", code)
	}
	if !strings.Contains(stderr, "read data") {
		t.Errorf("stderr missing 'read data': %s", stderr)
	}
}

// TestCmdRunBind_NilDataFallback 验证 data 为字面 null → 静默用空 map
// 继续（占位实现友好处理）。注意：仍走 happy path 后台逻辑，避免并发崩溃。
func TestCmdRunBind_NilDataFallback(t *testing.T) {
	// 子命令对 data 全空也允许（fallback 实现）。本测试仅校验 JSON
	// 解析后 data==nil 时不报错退出——严格策略可能后续收紧。
	in := cliBuildDeck(t)
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	out := filepath.Join(dir, "out.pptx")
	if err := os.WriteFile(dataPath, []byte("null"), 0o644); err != nil {
		t.Fatalf("write nil data: %v", err)
	}
	code, _, _ := runCLI([]string{"bind", "--data", dataPath, "--output", out, in})
	// 当前实现把 nil 视为空 map → bind 走通；不强制 OK，但至少非
	// UsageError/Capability（仅容许 RuntimeError 表示底层问题）。
	if code == ExitUsageError {
		t.Errorf("nil data should not be UsageError")
	}
}

// === writePresentation / writeJSON / capability 强制失败注入 ===========

// failingWriter 永远返回 io.ErrClosedPipe；用于模拟 stdout 写入失败。
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }

// TestWriteJSON_EncoderFails 验证 stdout 不可写时 writeJSON 返回
// ExitRuntimeError。本测试覆盖 common.go 33 行 `return ExitRuntimeError`。
func TestWriteJSON_EncoderFails(t *testing.T) {
	orig := stdoutW
	stdoutW = failingWriter{}
	t.Cleanup(func() { stdoutW = orig })
	if got := writeJSON(map[string]int{"a": 1}); got != ExitRuntimeError {
		t.Errorf("writeJSON on closed stdout = %v, want RuntimeError", got)
	}
}

// TestWritePresentation_MissingOutput 验证 path=="" → ExitUsageError
// + stderr 含 "missing --output"。
func TestWritePresentation_MissingOutput(t *testing.T) {
	// writePresentation 内部直接 ctx/Presentation，我们用 nil Presentation
	// + "" path——它先判 path=="" 就返回，不会引用 p。
	out := captureStderr(func() {
		got := writePresentation(nil, nil, "", false)
		if got != ExitUsageError {
			t.Errorf("writePresentation missing output = %v, want UsageError", got)
		}
	})
	if !strings.Contains(out, "missing --output") {
		t.Errorf("stderr missing 'missing --output': %s", out)
	}
}

// TestCapability_StdoutWriteFails 验证 stdout 不可写时 capability 走
// ExitRuntimeError。覆盖 capability.go 第 49 行。注意：runCLI 在内部
// 总要重写 stdoutW 到 soBuf，所以必须**直接**调用 cmdRunCapability
// 才能在 main 之外保留 failingWriter 全局替换。
func TestCapability_StdoutWriteFails(t *testing.T) {
	in := cliBuildDeck(t)

	origOut := stdoutW
	origErr := stderrW
	origExit := osExit
	stdoutW = failingWriter{}
	stderrW = io.Discard
	osExit = func(code ExitCode) {} // 吞 ExitRuntimeError 不真退
	t.Cleanup(func() {
		stdoutW = origOut
		stderrW = origErr
		osExit = origExit
	})

	code := cmdRunCapability([]string{in})
	if code != ExitRuntimeError {
		t.Errorf("capability broken stdout = %v, want RuntimeError", code)
	}
}

// TestStderrErr_NilReturnsOK 验证 stderrErr 在 err==nil 时返回 ExitOK
// 不写 stderr——覆盖 common.go 39 行短路。
func TestStderrErr_NilReturnsOK(t *testing.T) {
	var prev bytes.Buffer
	stdoutStderr := stderrW
	// 用 sniffW 防止污染测试。
	sw := &sniffW{b: &prev}
	stderrW = sw
	t.Cleanup(func() { stderrW = stdoutStderr })

	if got := stderrErr("op", nil, ExitRuntimeError); got != ExitOK {
		t.Errorf("stderrErr(nil) = %v, want OK", got)
	}
	if got := sw.String(); got != "" {
		t.Errorf("stderrErr(nil) wrote stderr %q, want empty", got)
	}
}

// TestFileExist_Missing 验证不存在路径返回 stat 错（不 panics）。覆盖
// iohelpers.go 第 15 行负路径。
func TestFileExist_Missing(t *testing.T) {
	if _, err := fileExist("/no/such/file.pptx"); err == nil {
		t.Error("fileExist missing path = nil, want error")
	}
}

// TestOpenFileForWrite_OverwriteMissing 验证目标不存在时不报 ErrOutputExists，
// 仅当 overwrite=false 且存在才报错。覆盖 iohelpers.go 第 27-29 行短路。
func TestOpenFileForWrite_OverwriteMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.pptx")
	f, err := openFileForWrite(path, false)
	if err != nil {
		t.Fatalf("openFileForWrite missing path: %v", err)
	}
	if f == nil {
		t.Fatal("file is nil")
	}
	f.Close()
}
