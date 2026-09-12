package opc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustSaveToFile 保存到新路径并重新 Load 校验。
func mustSaveToFile(t *testing.T, pk *Package, cs *ChangeSet, path string, opts ...SaveOption) {
	t.Helper()
	plan, err := BuildSavePlan(pk, cs)
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	if err := plan.SaveToFile(pk, path, opts...); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if _, err := Load(bytes.NewReader(data), int64(len(data)), Budget{}); err != nil {
		t.Fatalf("output not reloadable: %v", err)
	}
}

// noTempFiles 断言目录无残留临时文件（.go-pptx-save-*.tmp）。
func noTempFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".go-pptx-save-*.tmp"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("temp files left behind: %v", matches)
	}
}

func TestSaveToFileBasic(t *testing.T) {
	pk := loadMiniPackage(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pptx")

	mustSaveToFile(t, pk, &ChangeSet{}, path)

	// 输出主 Part 与源一致（B1 链路端到端）。
	data, _ := os.ReadFile(path)
	out := outputPartMap(t, data)
	if got := string(out["/ppt/deck.xml"]); got != `<p:presentation xmlns:p="urn:p"/>` {
		t.Errorf("main part content = %q", got)
	}
	noTempFiles(t, dir)
}

func TestSaveToFileDurabilityFull(t *testing.T) {
	pk := loadMiniPackage(t)
	dir := t.TempDir()
	mustSaveToFile(t, pk, &ChangeSet{},
		filepath.Join(dir, "out.pptx"), WithDurability(DurabilityFull))
	noTempFiles(t, dir)
}

// TestSaveToFileNoOverwrite 验证默认禁止覆盖：目标保持原样、无临时残留。
func TestSaveToFileNoOverwrite(t *testing.T) {
	pk := loadMiniPackage(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pptx")
	if err := os.WriteFile(path, []byte("OLD-TARGET"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	err = plan.SaveToFile(pk, path)
	if !errors.Is(err, ErrOutputExists) {
		t.Fatalf("err = %v, want ErrOutputExists", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "OLD-TARGET" {
		t.Errorf("target modified: %q", got)
	}
	noTempFiles(t, dir)
}

// TestSaveToFileOverwrite 显式覆盖成功。
func TestSaveToFileOverwrite(t *testing.T) {
	pk := loadMiniPackage(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pptx")
	if err := os.WriteFile(path, []byte("OLD-TARGET"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	if err := plan.SaveToFile(pk, path, WithOverwrite(true)); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.HasPrefix(string(data), "OLD-TARGET") {
		t.Error("target not replaced")
	}
	if _, err := Load(bytes.NewReader(data), int64(len(data)), Budget{}); err != nil {
		t.Fatalf("replaced target not reloadable: %v", err)
	}
	noTempFiles(t, dir)
}

// TestSaveToFileFailureKeepsTarget 是 AT-11 的 OPC 层等价物：写入阶段
// 失败（非法条目名使 Write 中途报错）后，旧目标保持不变、临时文件被
// 清理、错误链可识别。
func TestSaveToFileFailureKeepsTarget(t *testing.T) {
	pk := loadMiniPackage(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pptx")
	if err := os.WriteFile(path, []byte("OLD-TARGET"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	// 手工构造非法计划：条目名不是合法 PartName → Write 中途失败。
	badPlan := &SavePlan{Entries: []PlannedEntry{
		{Name: PartName("not-a-partname"), Action: EmitNew, Content: []byte("x")},
	}}
	err := badPlan.SaveToFile(pk, path, WithOverwrite(true))
	if err == nil {
		t.Fatal("expected failure from invalid entry name")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "OLD-TARGET" {
		t.Errorf("target modified after failure: %q", got)
	}
	noTempFiles(t, dir)
}

// TestSaveToFileUnwritableDir 临时文件创建失败快速返回且不动目标。
func TestSaveToFileUnwritableDir(t *testing.T) {
	pk := loadMiniPackage(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pptx")
	if err := os.WriteFile(path, []byte("OLD-TARGET"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	// 目标"目录"指向一个文件路径 → CreateTemp 失败。
	err = plan.SaveToFile(pk, filepath.Join(path, "child.pptx"))
	if err == nil {
		t.Fatal("expected failure from unwritable dir")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "OLD-TARGET" {
		t.Errorf("target modified: %q", got)
	}
	noTempFiles(t, dir)
}

// failingWriter 直接失败（writer 故障注入）。zip.Writer 全程缓冲、
// Close 时一次性落盘，因此任意一次底层写入失败即可模拟磁盘满/IO 错误。
type failingWriter struct {
	n int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.n++
	return 0, errors.New("injected write failure")
}

// TestWriteFailingWriter 注入 writer 中途失败：错误上抛，调用方可感知
// "输出可能不完整"（Write 路径无回滚能力，方案 §5）。
func TestWriteFailingWriter(t *testing.T) {
	pk := loadMiniPackage(t)
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	fw := &failingWriter{}
	if err := plan.Write(pk, fw); err == nil {
		t.Fatal("expected injected write failure")
	}
}

// TestSaveToFileVerifyCatchesCorruption 校验层能拦截与计划不符的输出
// （条目缺失）——通过对 verifyOutput 的直接单测模拟。
func TestSaveToFileVerifyCatchesCorruption(t *testing.T) {
	pk := loadMiniPackage(t)
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	// 正常输出应通过校验。
	dir := t.TempDir()
	good := filepath.Join(dir, "good.tmp")
	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := os.WriteFile(good, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := verifyOutput(good, plan); err != nil {
		t.Fatalf("verifyOutput(good): %v", err)
	}
	// 缺条目输出应被拒绝。
	bad := &SavePlan{Entries: plan.Entries[:len(plan.Entries)-1]}
	if err := verifyOutput(good, bad); err == nil {
		t.Error("verifyOutput accepted truncated output")
	}
}

// TestVerifyOutputNonZipFile covers the zip.NewReader error branch of
// verifyOutput: when the output file is parseable as a file but not a valid
// ZIP archive (bad magic bytes / truncated CD), verifyOutput must return a
// wrapped ErrMalformedPackage. This is the "save succeeded but file is
// corrupt on disk" defense path.
func TestVerifyOutputNonZipFile(t *testing.T) {
	pk := loadMiniPackage(t)
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	dir := t.TempDir()
	notAZip := filepath.Join(dir, "broken.tmp")
	if err := os.WriteFile(notAZip, []byte("not a zip archive — plain text"), 0o644); err != nil {
		t.Fatalf("write non-zip: %v", err)
	}
	err = verifyOutput(notAZip, plan)
	if err == nil {
		t.Fatal("verifyOutput accepted non-zip file")
	}
	if !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("err = %v, want ErrMalformedPackage", err)
	}
}

// TestVerifyOutputEntryNameMismatch covers the per-entry name comparison loop
// branch (count matches but a name differs): a ZIP whose entry names align in
// count but diverge in content from the plan must be rejected. The existing
// truncation test covers the count-mismatch branch; this covers the
// name-mismatch branch.
func TestVerifyOutputEntryNameMismatch(t *testing.T) {
	pk := loadMiniPackage(t)
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	dir := t.TempDir()
	// Serialize the plan to a temp file first, then patch one entry's bytes
	// to rename it so the count is preserved but a name disagrees.
	tmp := filepath.Join(dir, "patched.tmp")
	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Rebuild plan with one entry renamed to a non-existent path — entry
	// count is the same but a name comparison fails.
	if len(plan.Entries) == 0 {
		t.Skip("plan has no entries to mismatch")
	}
	renamed := make([]PlannedEntry, len(plan.Entries))
	copy(renamed, plan.Entries)
	renamed[0] = PlannedEntry{Name: PartName("/ppt/ghost.xml"), Action: plan.Entries[0].Action}
	mismatched := &SavePlan{Entries: renamed}
	if err := verifyOutput(tmp, mismatched); err == nil {
		t.Error("verifyOutput accepted output with mismatched entry name")
	}
}
