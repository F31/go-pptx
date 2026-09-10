//go:build corpus

// OPC-01 收口：真实样本 ext-0024（拓扑方案.pptx，WPS 私有语料）的 OPC 层冒烟。
//
// 该 build tag 守门的测试在默认 `go test ./...` 中不参与，避免无源文件时 CI
// 误报。CI runner 默认无私有语料（testdata/corpus/ext-*/ 仅 manifest，无原
// 始 .pptx），因此本文件默认 Skip。本地手动跑：
//
//	GOPPTX_REPLAY_SRC=/e/work/2026/服务器/拓扑方案.pptx \
//	  go test -tags=corpus -run TestRealExt -v ./internal/opc/
//
// 测试覆盖：
//
//  1. OPC 引擎 Load 真实样本（含动画/未知扩展/媒体 22 项）不报结构错误，
//     且 MainPart() 经 officeDocument 关系正确发现主 Part（不假设文件名）；
//  2. 空变更集 Save 后未变 Part 字节恒等（AT-01 OPC 层等价，用真实形态样本
//     验证——合成 fixture 的 TestSavePlanUnchangedIsB1 仅覆盖 OPC 引擎自身
//     正确性，本测试补充对生产环境真实形态的兼容性）；
//  3. SaveToFile 落盘 → 重 Load → 主 Part 与关键 Part 仍可访问且字节不变
//     （SAVE-02 原子落盘对真实样本的兼容验证）。
package opc

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// ext0024DefaultPaths 是本机 dev 默认源路径候选（Git Bash / WSL 把 E:\
// 挂载为 /e/，Go 进程走 Windows API 需要 Windows 风格路径；CI runner 通常
// 是 Linux，仅需 WSL/git-bash 风格）。任一路径存在即命中，都不存在 → Skip。
//
// WPS 生成的 PPTX 含 animation.timing / animation.transition / xml.unknown_ext
// 标签，是 OPC-01 收口的真实形态候选。
var ext0024DefaultPaths = []string{
	`E:\work\2026\服务器\拓扑方案.pptx`,     // Windows 原生
	`/e/work/2026/服务器/拓扑方案.pptx`,     // Git Bash / WSL
	`/mnt/e/work/2026/服务器/拓扑方案.pptx`, // 部分 WSL 配置
}

// replaySourcePath 解析本测试的源 PPTX 路径：优先 GOPPTX_REPLAY_SRC 环境
// 变量，再依次试探 ext0024DefaultPaths；都不存在 → t.Skip（不报错）。
func replaySourcePath(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("GOPPTX_REPLAY_SRC"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
		t.Logf("GOPPTX_REPLAY_SRC=%s not accessible, falling back to default candidates", v)
	}
	for _, p := range ext0024DefaultPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skipf("real corpus source not available (set GOPPTX_REPLAY_SRC or mount one of: %v)", ext0024DefaultPaths)
	return ""
}

// loadExt0024Package 读源并 opc.Load；返回 Package 与原始字节（便于后续
// 哈希比对与文件落盘复用）。
func loadExt0024Package(t *testing.T) (*Package, []byte) {
	t.Helper()
	path := replaySourcePath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source %s: %v", path, err)
	}
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("opc.Load source %s: %v", path, err)
	}
	return pk, data
}

// TestRealExt0024_OpenMainPart 验证 OPC 引擎对真实样本的 Load + MainPart 发现。
// WPS 生成的 PPTX 主 Part 不一定命名为 presentation.xml，但 officeDocument
// 关系目标必须是内部可达且可读取非空字节。
func TestRealExt0024_OpenMainPart(t *testing.T) {
	pk, _ := loadExt0024Package(t)
	main, err := pk.MainPart()
	if err != nil {
		t.Fatalf("MainPart: %v", err)
	}
	if !pk.HasPart(main) {
		t.Fatalf("main part %s not in package", main)
	}
	rc, err := pk.OpenPart(main)
	if err != nil {
		t.Fatalf("OpenPart main: %v", err)
	}
	defer rc.Close()
	buf := &bytes.Buffer{}
	n, _ := buf.ReadFrom(rc)
	if n == 0 {
		t.Fatalf("main part %s is empty", main)
	}
	t.Logf("main part: %s (%d bytes, %d total parts in package)", main, n, len(pk.PartNames()))
}

// TestRealExt0024_SaveUnchangedB1 验证 OPC 引擎面对真实样本时空变更集 Save
// 后未变 Part 字节恒等（AT-01 OPC 层等价用真实形态样本验证）。
func TestRealExt0024_SaveUnchangedB1(t *testing.T) {
	pk, _ := loadExt0024Package(t)
	baseline := partHashes(t, pk)

	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan empty: %v", err)
	}
	if len(plan.ChangedParts) != 0 {
		t.Fatalf("ChangedParts = %v, want empty", plan.ChangedParts)
	}
	for _, e := range plan.Entries {
		if e.Action != CopyOriginal {
			t.Errorf("entry %s action = %s, want CopyOriginal", e.Name, e.Action)
		}
	}

	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("plan.Write: %v", err)
	}
	out := outputPartMap(t, buf.Bytes())
	if len(out) != len(baseline) {
		t.Fatalf("output parts = %d, want %d", len(out), len(baseline))
	}
	for name, want := range baseline {
		got, ok := out[name]
		if !ok {
			t.Errorf("part %s missing in output", name)
			continue
		}
		if hashBytes(got) != want {
			t.Errorf("part %s content changed (B1 violation on real sample)", name)
		}
	}
	t.Logf("verified %d parts byte-identical through empty save round-trip on real WPS sample", len(baseline))
}

// TestRealExt0024_SaveToFileRoundTrip 验证 SaveToFile 落盘 → 重 Load →
// 主 Part 仍可访问且字节不变（SAVE-02 原子落盘对真实样本的兼容验证）。
func TestRealExt0024_SaveToFileRoundTrip(t *testing.T) {
	pk, _ := loadExt0024Package(t)
	main, err := pk.MainPart()
	if err != nil {
		t.Fatalf("MainPart: %v", err)
	}
	baselineMain := partHashes(t, pk)[main]

	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan empty: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "replay.pptx")
	if err := plan.SaveToFile(pk, tmp); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	stat, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatalf("output is empty")
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	out, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load output: %v", err)
	}
	outMain, err := out.MainPart()
	if err != nil {
		t.Fatalf("reopened MainPart: %v", err)
	}
	if outMain != main {
		t.Errorf("reopened main part = %s, want %s", outMain, main)
	}
	outMainHash := partHashes(t, out)[outMain]
	if outMainHash != baselineMain {
		t.Errorf("main part content changed through SaveToFile round-trip")
	}
	t.Logf("SaveToFile round-trip OK on real sample: %d bytes, main=%s", stat.Size(), outMain)
}
