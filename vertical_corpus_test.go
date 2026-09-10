//go:build corpus

// M0 垂直验证程序（实施计划 §123）——真实语料版。
//
// 垂直验证程序定义：对**含动画与未知扩展**的页面仅修改一个普通文本 Run →
// 保存 → 逐 Part 哈希比对（B1）+ 同节点未知区字节不变断言。
//
// 与 vertical_test.go（合成语料）的分工：
//   - vertical_test.go 用合成 fixture 固定断言骨架（含人造未知扩展与完整
//     动画树），保证不依赖外部语料即可回归；
//   - 本文件把同一套断言骨架接到**真实生成器产物**上：
//   - 公开样本 s001/s002/s003（LibreOffice headless 导出、可再分发、
//     CI 默认跑——数据随语料入库后自动启用）；
//   - 私有真实样本 ext-0024（WPS，含 animation.timing/transition +
//     xml.unknown_ext + 22 媒体），本地挂载源文件后自动启用，CI 跳过。
//
// 断言（与 vertical_test.go 等价，作用于真实样本）：
//  1. B1：仅被编辑的 slide Part 条目内容变化，其余条目字节级一致；
//     无条目增删。
//  2. 同 Part 内未知区字节不变：被编辑 Part 与源 Part 的差异区域
//     （LCP/LCS 夹出的最小区间）不得覆盖 p:timing / p:transition /
//     p:extLst 子树——这是"保真补丁"相对"重新序列化整棵树"的核心优势。
//  3. 编辑确实生效：新文本出现在输出中。
//  4. Save→Open 闭环：输出可重新打开且 Validate 零错误。
//
// 守门策略沿用 build tag `corpus`：默认 `go test ./...` 不跑本文件；CI 独立
// job `corpus-replay` 跑 `-tags=corpus ./...`。样本数据缺席时逐样本 Skipf，
// 不阻断 CI。
package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// verticalRealPublicSamples 是参与垂直验证的公开样本 ID（与 testdata/corpus/
// 子目录对齐，与 corpus_replay_test.go 的公开样本一致）。
var verticalRealPublicSamples = []string{"s001-text", "s002-table", "s003-image"}

// verticalRealExt0024Paths 是私有真实样本 ext-0024（WPS）的本机候选路径。
// Git Bash / WSL 把 E:\ 挂载为 /e/；Go 进程走 Windows API 需要 Windows 风格
// 路径。任一路径存在即命中。
var verticalRealExt0024Paths = []string{
	`E:\work\2026\服务器\拓扑方案.pptx`,
	`/e/work/2026/服务器/拓扑方案.pptx`,
	`/mnt/e/work/2026/服务器/拓扑方案.pptx`,
}

// verticalRealPublicSource 返回公开样本源 pptx 路径（读 manifest 的
// files.pptx.path 后拼接到样本目录）。样本尚未入库时 Skipf，不阻断 CI；
// manifest 存在但源缺失则 Fatalf（真实 dev 错误，不掩盖）。
func verticalRealPublicSource(t *testing.T, id string) string {
	t.Helper()
	dir := filepath.Join("testdata", "corpus", id)
	manifestPath := filepath.Join(dir, "manifest.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("public sample %s not vendored (%s missing); generate via scripts/gen_corpus/run.sh generate", id, manifestPath)
		}
		t.Fatalf("read %s: %v", manifestPath, err)
	}
	var m corpusManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse %s: %v", manifestPath, err)
	}
	if m.Files.Pptx.Path == "" {
		t.Fatalf("%s missing files.pptx.path", manifestPath)
	}
	src := filepath.Join(dir, m.Files.Pptx.Path)
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("sample source missing though manifest present: %s: %v", src, err)
	}
	return src
}

// verticalRealExt0024Source 返回 ext-0024 私有样本源路径；源不可达时 Skipf。
func verticalRealExt0024Source(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("GOPPTX_REPLAY_SRC"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}
	for _, p := range verticalRealExt0024Paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skipf("ext-0024 source not available (set GOPPTX_REPLAY_SRC or mount one of %v)", verticalRealExt0024Paths)
	return ""
}

// verticalOpenReplaceSave 是垂直验证的公共前段：读源 bytes → 打开 →（可选）
// beforeEdit 钩子 → 按 actions 对所有 AutoShape 施加 ReplaceText → 内存保存。
//
// 返回源字节、输出字节与 B1 差异（changed/added/removed）。beforeEdit 供私有
// 样本在编辑前抓取 p:timing 等原始字节。
func verticalOpenReplaceSave(t *testing.T, srcPath string, actions []corpusAction,
	beforeEdit func(t *testing.T, p *Presentation)) (srcBytes, outBytes []byte, changed, added, removed []string) {
	t.Helper()
	var err error
	srcBytes, err = os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source %s: %v", srcPath, err)
	}
	baseline := zipPartHashes(t, srcBytes)

	p, err := Open(srcPath)
	if err != nil {
		t.Fatalf("Open(%s): %v", srcPath, err)
	}
	defer p.Close()

	if beforeEdit != nil {
		beforeEdit(t, p)
	}

	var matches, replaced int
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
			for _, act := range actions {
				res, err := tf.ReplaceText(act.Old, act.New)
				if err != nil {
					t.Fatalf("ReplaceText(%q→%q) shape id=%d name=%q: %v",
						act.Old, act.New, sh.ID(), sh.Name(), err)
				}
				matches += res.Matches
				replaced += res.Replaced
			}
		}
	}
	if replaced == 0 {
		t.Fatalf("no replacement performed for actions %+v", actions)
	}

	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	outBytes = buf.Bytes()
	changed, added, removed = diffHashes(baseline, zipPartHashes(t, outBytes))
	return srcBytes, outBytes, changed, added, removed
}

// commonAffix 返回 a、b 的最长公共前缀长度与最长公共后缀长度（后缀与前缀
// 不重叠）。用于把"编辑到底改动了哪一段字节"夹出来。
func commonAffix(a, b []byte) (prefix, suffix int) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for prefix < n && a[prefix] == b[prefix] {
		prefix++
	}
	for suffix < n-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	return prefix, suffix
}

// assertEditDiffRegionSafe 断言：源 Part 与编辑后 Part 的差异区域（LCP/LCS
// 夹出的最小区间）不覆盖动画 / 转场 / 未知扩展子树，且编辑整体生效。这是
// "同节点未知区字节不变"的通用化断言——不硬编码子树内容，改为证明差异被
// 局部在文本 Run 附近。
//
// 注意：不对差异区间本身做"包含完整新文本"的断言——当新旧文本共享前缀/后缀
// 时（如 "Synthetic PNG" → "Generated PNG" 共享 " PNG"），LCP/LCS 会把公共
// 部分排除在区间外，区间只含真正变化的片段。生效性用整段 Part 包含新文本判定。
func assertEditDiffRegionSafe(t *testing.T, origPart, editedPart []byte, newText string) {
	t.Helper()
	prefix, suffix := commonAffix(origPart, editedPart)
	if prefix+suffix > len(origPart) || prefix+suffix > len(editedPart) {
		t.Fatalf("affix overflow: prefix=%d suffix=%d len(orig)=%d len(edited)=%d",
			prefix, suffix, len(origPart), len(editedPart))
	}
	origRegion := origPart[prefix : len(origPart)-suffix]
	editRegion := editedPart[prefix : len(editedPart)-suffix]

	for _, marker := range []string{"<p:timing", "<p:transition", "<p:extLst"} {
		if bytes.Contains(origRegion, []byte(marker)) {
			t.Errorf("edit diff region overlaps %s subtree — unknown region NOT byte-preserved\nregion(%d bytes): %s",
				marker, len(origRegion), snippet(origRegion))
		}
	}
	if len(editRegion) == 0 {
		t.Errorf("edit diff region is empty — edit did not change the Part")
	}
	if newText != "" && !bytes.Contains(editedPart, []byte(newText)) {
		t.Errorf("edited Part does not contain new text %q", newText)
	}
	t.Logf("edit diff region bounded to %d source bytes (affix prefix=%d suffix=%d)", len(origRegion), prefix, suffix)
}

// snippet 返回字节切片的前 200 字节的字符串形式（用于失败信息，避免打印整段 XML）。
func snippet(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// zipPartHashes 返回 ZIP 内**内容条目**的解压哈希，跳过显式目录条目
// （名以 "/" 结尾）。OPC 语义上目录条目不是 Part；WPS 等生成器会写入它们，
// 而 go-pptx 保存时只写 Part 条目——B1 的判定对象是 Part 内容，故目录条目
// 不参与比对（丢弃目录条目不影响 OPC 消费者，属已知且良性的形态差异）。
func zipPartHashes(t *testing.T, data []byte) map[string]string {
	t.Helper()
	all := zipEntryHashes(t, data)
	out := make(map[string]string, len(all))
	var dirs []string
	for name, h := range all {
		if strings.HasSuffix(name, "/") {
			dirs = append(dirs, name)
			continue
		}
		out[name] = h
	}
	if len(dirs) > 0 {
		t.Logf("skipped %d explicit directory entries (not OPC parts): %v", len(dirs), dirs)
	}
	return out
}

// zipEntryBytes 返回 ZIP 容器内指定条目名的解压字节。
func zipEntryBytes(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return b
	}
	t.Fatalf("entry %s not found in zip", name)
	return nil
}

// TestVerticalReal_PublicSamples 对三份公开样本跑垂直验证：单命中替换 →
// 保存 → B1（仅 slide Part 变化）+ 差异区间不覆盖未知区 + Save→Open 闭环。
func TestVerticalReal_PublicSamples(t *testing.T) {
	for _, id := range verticalRealPublicSamples {
		t.Run(id, func(t *testing.T) {
			src := verticalRealPublicSource(t, id)
			actions := loadCorpusActions(t, filepath.Join("testdata", "corpus", id, id+".actions.json"))
			srcBytes, outBytes, changed, added, removed := verticalOpenReplaceSave(t, src, actions, nil)

			if len(added) != 0 {
				t.Errorf("unexpected added entries: %v", added)
			}
			if len(removed) != 0 {
				t.Errorf("unexpected removed entries: %v", removed)
			}
			if len(changed) == 0 {
				t.Fatalf("B1: no entry changed although a replacement happened")
			}
			for _, e := range changed {
				if !strings.HasPrefix(e, "ppt/slides/slide") {
					t.Errorf("B1 violation: non-slide entry changed: %s (changed=%v)", e, changed)
				}
			}

			// 逐被改 Part 断言差异区间安全。
			for _, e := range changed {
				assertEditDiffRegionSafe(t, zipEntryBytes(t, srcBytes, e), zipEntryBytes(t, outBytes, e), actions[0].New)
			}

			// Save→Open 闭环 + Validate 零错误。
			p2, err := OpenReader(bytes.NewReader(outBytes), int64(len(outBytes)))
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			defer p2.Close()
			if rep := p2.Validate(context.Background()); rep.HasErrors() {
				t.Errorf("validate(after) returned %d error(s)", len(rep.Diagnostics))
			}
			t.Logf("B1 OK: %d changed slide part(s): %v", len(changed), changed)
		})
	}
}

// TestVerticalReal_Ext0024 是真实语料的垂直验证主用例：对 WPS 样本 ext-0024
// （含动画 + 未知扩展 + 22 媒体）单命中替换一个 Run → 保存，断言
// B1（仅被编辑 slide Part 变化）+ p:timing 原始字节逐页保留（编辑→保存→重开后
// 逐字节一致）+ 差异区间不覆盖 p:timing/p:transition/p:extLst。
func TestVerticalReal_Ext0024(t *testing.T) {
	src := verticalRealExt0024Source(t)

	// 编辑前抓取每一页 p:timing 的原始字节（编辑后逐页比对）。
	var timingsBefore [][]byte
	capture := func(t *testing.T, p *Presentation) {
		slides, err := p.Slides()
		if err != nil {
			t.Fatalf("Slides: %v", err)
		}
		for i, s := range slides {
			raw, _, err := s.TimingTreeRaw()
			if err != nil {
				t.Fatalf("TimingTreeRaw slide %d: %v", i, err)
			}
			timingsBefore = append(timingsBefore, raw)
		}
		nonEmpty := 0
		for _, r := range timingsBefore {
			if len(r) > 0 {
				nonEmpty++
			}
		}
		if nonEmpty == 0 {
			t.Fatalf("ext-0024: no p:timing found on any slide, sample expectation broken (manifest timing_count>0)")
		}
	}

	// 与 compat-smoke.gold_action 一致的单命中替换（89144 → 89145）。
	actions := []corpusAction{{Action: "ReplaceText", Old: "89144", New: "89145"}}
	srcBytes, outBytes, changed, added, removed := verticalOpenReplaceSave(t, src, actions, capture)

	if len(added) != 0 {
		t.Errorf("unexpected added entries: %v", added)
	}
	if len(removed) != 0 {
		t.Errorf("unexpected removed entries: %v", removed)
	}
	if len(changed) != 1 || !strings.HasPrefix(changed[0], "ppt/slides/slide") {
		t.Fatalf("B1 violation: changed = %v, want exactly one slide Part", changed)
	}
	entry := changed[0]

	// 同 Part 内未知区字节不变：差异区间不覆盖 timing/transition/extLst。
	origSlide := zipEntryBytes(t, srcBytes, entry)
	editedSlide := zipEntryBytes(t, outBytes, entry)
	assertEditDiffRegionSafe(t, origSlide, editedSlide, actions[0].New)

	// 保存后逐页 p:timing 原始字节与编辑前一致（编辑→保存→重开闭环）。
	p2, err := OpenReader(bytes.NewReader(outBytes), int64(len(outBytes)))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer p2.Close()
	slides2, err := p2.Slides()
	if err != nil {
		t.Fatalf("reopened Slides: %v", err)
	}
	if len(slides2) != len(timingsBefore) {
		t.Fatalf("slide count changed: %d → %d", len(timingsBefore), len(slides2))
	}
	for i, s := range slides2 {
		raw, _, err := s.TimingTreeRaw()
		if err != nil {
			t.Fatalf("reopened TimingTreeRaw slide %d: %v", i, err)
		}
		if !bytes.Equal(raw, timingsBefore[i]) {
			t.Errorf("slide %d p:timing raw bytes changed across edit+save+reopen (before %d bytes, after %d bytes)",
				i, len(timingsBefore[i]), len(raw))
		}
	}

	if rep := p2.Validate(context.Background()); rep.HasErrors() {
		t.Errorf("validate(after) returned %d error(s)", len(rep.Diagnostics))
	}
	totalTiming := 0
	for _, r := range timingsBefore {
		totalTiming += len(r)
	}
	t.Logf("ext-0024 vertical OK: B1 (only %s), p:timing preserved across %d slide(s) (%d bytes)",
		entry, len(timingsBefore), totalTiming)
}
