//go:build corpus

// CORPUS-01: 公开样本金样自动 replay 回归。
//
// 把 testdata/corpus/s001-text / s002-table / s003-image 三个公开样本
//（LibreOffice 生成、可再分发、含 compat-smoke.json 五层证据）的
// `.actions.json` 接入 go test 链路，作为 QA-01 的 CI 守门。
//
// 守门策略：build tag `corpus`。
//   - 默认 `go test ./...` 不跑本文件，保持 dev 内循环速度
//   - CI 独立 job `corpus-replay` 跑 `-tags=corpus ./...`
//
// 验收口径：
//   - matches / replaced 累加值 == compat-smoke.gold_action.result
//   - shapes_seen / shapes_tried 计数器 == 同上
//   - 编辑前 Validate() 零错误（before_error_count == 0）
//   - 编辑后 Save→Open→Validate() 零错误（after_error_count == 0）
//   - 至少有一个 AutoShape 文本包含 compat-smoke.diff.changed_entry.to
//
// 不做字节比对：edited.pptx 是 vendor（LibreOffice/PowerPoint/WPS）写出来的，
// 字节特征必不一致；改用 compat-smoke 提供的 from→to 文本断言。

package pptx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpusSamples 列举参与 replay 的样本 ID（与 testdata/corpus/ 子目录对齐）。
//
// 公开样本（s00*）CI 默认跑；私有样本（ext-*）仅当 source pptx 可访问时
// 才跑——通过绝对路径 + os.IsNotExist 在 loadCorpusSourcePath 处自动 Skip，
// 无需新增 manifest 字段。任何样本缺 actions.json / compat-smoke.json 也
// 自动 Skip（无契约性输入则无可断言内容）。后续 dev 补齐文件即可自动启用。
var corpusSamples = []string{"s001-text", "s002-table", "s003-image", "ext-0024"}

// corpusManifest 仅解码 replay 所需的 pptx 路径字段，其他字段忽略。
type corpusManifest struct {
	Files struct {
		Pptx struct {
			Path string `json:"path"`
		} `json:"pptx"`
	} `json:"files"`
}

// corpusCompat 仅解码 replay 校验所需的子集；其他字段忽略。
type corpusCompat struct {
	GoldAction struct {
		Result struct {
			Matches     int `json:"matches"`
			Replaced    int `json:"replaced"`
			ShapesSeen  int `json:"shapes_seen"`
			ShapesTried int `json:"shapes_tried"`
		} `json:"result"`
	} `json:"gold_action"`
	Validate struct {
		BeforeErrorCount int `json:"before_error_count"`
		AfterErrorCount  int `json:"after_error_count"`
	} `json:"validate"`
	Diff struct {
		ChangedEntry struct {
			To string `json:"to"`
		} `json:"changed_entry"`
	} `json:"diff"`
}

// corpusAction 仅解码 replay 调度所需的子集；scope / expected 暂用于人工对照。
type corpusAction struct {
	Action string `json:"action"`
	Old    string `json:"old"`
	New    string `json:"new"`
}

// TestCorpusReplay 遍历 corpusSamples，逐个 replay 公开样本金样。
func TestCorpusReplay(t *testing.T) {
	for _, id := range corpusSamples {
		t.Run(id, func(t *testing.T) {
			replayCorpusSample(t, id)
		})
	}
}

// replayCorpusSample 实现单样本 replay：load fixtures → 跑 actions → 断言计数
// 与 Validate → 写临时文件 → 二次 Validate → 端到端文本断言。
func replayCorpusSample(t *testing.T, id string) {
	t.Helper()

	root := filepath.Join("testdata", "corpus", id)
	pptxPath := loadCorpusSourcePath(t, root)
	actions := loadCorpusActions(t, filepath.Join(root, id+".actions.json"))
	compat := loadCorpusCompat(t, filepath.Join(root, "compat-smoke.json"))
	if len(actions) == 0 {
		t.Fatalf("empty actions for %s", id)
	}

	p, err := Open(pptxPath)
	if err != nil {
		t.Fatalf("Open(%s): %v", pptxPath, err)
	}
	defer p.Close()

	if compat.Validate.BeforeErrorCount == 0 {
		if rep := p.Validate(context.Background()); rep.HasErrors() {
			t.Fatalf("validate(before) returned %d error(s) but compat expects 0", len(rep.Diagnostics))
		}
	}

	var (
		matches     int
		replaced    int
		shapesSeen  int
		shapesTried int
	)
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	for _, s := range slides {
		shapes, err := s.Shapes()
		if err != nil {
			t.Fatalf("Shapes: %v", err)
		}
		for _, sh := range shapes {
			shapesSeen++
			as, ok := sh.(*AutoShape)
			if !ok {
				continue
			}
			tf, err := as.TextFrame()
			if err != nil {
				// 非文本 shape 不计入 shapesTried（与 gold_replace.go 计数语义一致）
				continue
			}
			shapesTried++
			for _, act := range actions {
				if act.Action != "ReplaceText" {
					t.Fatalf("unsupported action %q (only ReplaceText is wired in CORPUS-01)", act.Action)
				}
				res, err := tf.ReplaceText(act.Old, act.New)
				if err != nil {
					t.Fatalf("ReplaceText shape id=%d name=%q: %v", sh.ID(), sh.Name(), err)
				}
				matches += res.Matches
				replaced += res.Replaced
			}
		}
	}

	if got, want := matches, compat.GoldAction.Result.Matches; got != want {
		t.Errorf("matches=%d want %d", got, want)
	}
	if got, want := replaced, compat.GoldAction.Result.Replaced; got != want {
		t.Errorf("replaced=%d want %d", got, want)
	}
	if got, want := shapesSeen, compat.GoldAction.Result.ShapesSeen; got != want {
		t.Errorf("shapes_seen=%d want %d", got, want)
	}
	if got, want := shapesTried, compat.GoldAction.Result.ShapesTried; got != want {
		t.Errorf("shapes_tried=%d want %d", got, want)
	}

	// 写入临时 PPTX 并重新打开，确保 Save→Open 闭环可工作（QA-01 round-trip）。
	// Presentation.Save 拒绝覆盖已存在文件（ErrOutputExists），故 CreateTemp
	// 创建占位文件后立刻 Remove，让 Save 自己原子创建。
	tmp, err := os.CreateTemp(t.TempDir(), "corpus-*.pptx")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	if err := os.Remove(tmpPath); err != nil {
		t.Fatalf("Remove temp file %s: %v", tmpPath, err)
	}
	if _, err := p.Save(context.Background(), tmpPath); err != nil {
		t.Fatalf("Save(%s): %v", tmpPath, err)
	}
	p2, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("reopen(%s): %v", tmpPath, err)
	}
	defer p2.Close()
	if compat.Validate.AfterErrorCount == 0 {
		if rep := p2.Validate(context.Background()); rep.HasErrors() {
			t.Fatalf("validate(after) returned %d error(s) but compat expects 0", len(rep.Diagnostics))
		}
	}

	// 端到端文本断言：至少一个 AutoShape 文本包含 compat-smoke.diff.changed_entry.to。
	// 注意 to 可能包含换行；按子串包含判定。
	if target := compat.Diff.ChangedEntry.To; target != "" {
		if !corpusShapeContains(t, p2, target) {
			t.Errorf("no shape contains target text %q after replay", target)
		}
	}
}

// loadCorpusSourcePath 从 manifest.json 解码 pptx 路径并 stat。
//
// 自动识别 local-only 私有样本：当 manifest.files.pptx.path 是绝对路径
// （Windows 的 C:\… 或 Unix 的 /…）且本地不存在（如 ext-0024 的
// /mnt/e/work/2026/服务器/拓扑方案.pptx 在 Windows 沙箱缺失；filepath.Join
// 在 Windows 上把 Unix 绝对路径当相对拼接，所以判定必须在 Join **之前**
// 对原始 path 做），t.Skipf 而非 t.Fatalf——CI 不因私有样本缺席而红，
// 本地 dev 挂载源文件后自动启用。相对路径缺失（公开样本源 pptx 误删）
// 仍 Fatalf，不掩盖 dev 错误。
//
// 跨平台绝对路径候选：当 manifest 指向绝对路径（典型 WSL /mnt/<drive>/…）
// 而本机走 Windows 原生 API 时，原路径不可达但 Win32 路径可能可达——
// corpusAbsPathCandidates 把 WSL/Git Bash/Windows 三种形态互相转换后依次
// stat，第一个命中即返回。相对路径（公开样本）直接 filepath.Join(root,…)，
// 失败时区分"本地误删" vs "私有样本本地无源"分别 Fatalf/Skipf。
func loadCorpusSourcePath(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m corpusManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if m.Files.Pptx.Path == "" {
		t.Fatalf("manifest.json missing files.pptx.path")
	}
	if !isCorpusAbs(m.Files.Pptx.Path) {
		// 公开样本：相对路径直接拼到 root；缺失即 dev 错误，不 Skip。
		pptxPath := filepath.Join(root, m.Files.Pptx.Path)
		if _, err := os.Stat(pptxPath); err != nil {
			if os.IsNotExist(err) {
				t.Fatalf("public sample source missing though manifest present: %s (delete manifest or restore file)", pptxPath)
			}
			t.Fatalf("stat source pptx %s: %v", pptxPath, err)
		}
		return pptxPath
	}
	// 私有样本：绝对路径可能在 WSL / Git Bash / Windows 之间需要互转。
	// 先试原路径，再试候选，第一个命中即可。
	if _, err := os.Stat(m.Files.Pptx.Path); err == nil {
		return m.Files.Pptx.Path
	}
	for _, cand := range corpusAbsPathCandidates(m.Files.Pptx.Path) {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	t.Skipf("source pptx not available: %s (data 尚未入库或为本地私有路径；候选：%s)",
		m.Files.Pptx.Path, strings.Join(corpusAbsPathCandidates(m.Files.Pptx.Path), ", "))
	return ""
}

// isCorpusAbs 同时识别 Win32 绝对（C:\... / \\…）与 POSIX 绝对（/...）。
// filepath.IsAbs 在 Windows 上只认前者；manifest.path 写的是 /mnt/e/… 或
// /work/… 形态时必须手动判断，否则会被当作相对路径拼到 root 后面。
func isCorpusAbs(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	return strings.HasPrefix(p, "/")
}

// corpusAbsPathCandidates 把一个绝对路径转成 WSL / Git Bash / Windows 三种
// 形态候选，便于跨平台脚本（Git Bash 调用 windows python.exe / Windows
// Go 进程访问 WSL 挂载的 ext4 / 反之）共享同一份 manifest。例如
// "/mnt/e/work/2026/服务器/拓扑方案.pptx" →
//   - 原路径
//   - "/e/work/2026/服务器/拓扑方案.pptx"（Git Bash 把 E: 盘映射为 /e/）
//   - "E:\work\2026\服务器\拓扑方案.pptx"（Win32）
//
// 反向也同理。
func corpusAbsPathCandidates(p string) []string {
	out := []string{p}
	switch {
	case strings.HasPrefix(p, "/mnt/") && len(p) > 6:
		// /mnt/<drive>/... → /<drive>/...（Git Bash）+ <Drive>:\...（Win32）
		drive := string(p[5])
		rest := p[6:]
		out = append(out, "/"+drive+rest)
		if isAlphaByte(drive) {
			out = append(out, strings.ToUpper(drive)+":"+filepath.FromSlash(rest))
		}
	case len(p) >= 3 && p[0] == '/' && p[2] == '/' && isAlphaByte(string(p[1])):
		// Git Bash /<drive>/... → /mnt/<drive>/...（WSL）+ <Drive>:\...（Win32）
		drive := string(p[1])
		rest := p[2:]
		out = append(out, "/mnt/"+strings.ToLower(drive)+rest)
		out = append(out, strings.ToUpper(drive)+":"+filepath.FromSlash(rest))
	}
	return out
}

func isAlphaByte(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// loadCorpusActions 解析 <id>.actions.json。文件不存在时 t.Skipf
// （无契约性输入则无可 replay 内容，例如 ext-0024 暂未归档 actions）。
func loadCorpusActions(t *testing.T, path string) []corpusAction {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("actions.json not available: %s", path)
		}
		t.Fatalf("read actions %s: %v", path, err)
	}
	var a []corpusAction
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatalf("parse actions %s: %v", path, err)
	}
	return a
}

// loadCorpusCompat 解析 compat-smoke.json。文件不存在时 t.Skipf
// （与 actions.json 一致——无金样则无断言目标）。
func loadCorpusCompat(t *testing.T, path string) corpusCompat {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("compat-smoke.json not available: %s", path)
		}
		t.Fatalf("read compat-smoke %s: %v", path, err)
	}
	var c corpusCompat
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("parse compat-smoke %s: %v", path, err)
	}
	return c
}

// corpusShapeContains 检查 Presentation 中是否有 AutoShape 文本包含 target 子串。
//
// 段落间以 \n 拼接，与 compat-smoke.diff.changed_entry.to 的 JSON 字符串
// 转义风格（字面 \n 表示段落分隔）保持一致——确保多段落样本的端到端断言
// 不会因段落拼接丢换行而误判（参见 s001-text 标题的多段三行结构）。
func corpusShapeContains(t *testing.T, p *Presentation, target string) bool {
	t.Helper()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	for _, s := range slides {
		shapes, err := s.Shapes()
		if err != nil {
			t.Fatalf("Shapes: %v", err)
		}
		for _, sh := range shapes {
			as, ok := sh.(*AutoShape)
			if !ok {
				continue
			}
			tf, err := as.TextFrame()
			if err != nil {
				continue
			}
			paras, err := tf.Paragraphs()
			if err != nil {
				continue
			}
			texts := make([]string, 0, len(paras))
			for _, par := range paras {
				if r, err := par.Text(); err == nil {
					texts = append(texts, r)
				}
			}
			if strings.Contains(strings.Join(texts, "\n"), target) {
				return true
			}
		}
	}
	return false
}
