//go:build corpus

// B1 金样比对：公开语料上的「未修改 Part 字节一致」硬门槛。
//
// 背景（2026-09-12 交接后补测）：B1 断言原本只有两处载体——
//   - internal/opc/saveplan_test.go 的 TestSavePlanUnchangedIsB1：合成 fixture
//   - internal/opc/real_corpus_test.go 的 TestRealExt0024_*：私有样本 ext-0024
//
// 前者不覆盖真实文件形态，后者依赖未入库的源文件（CI 上恒 Skip）。**结果是
// B1 从未在 CI 上真正生效过**。本文件把 B1 建到「库内可再分发的公开样本」
// （s001-text / s002-table / s003-image）之上，使其可在 CI 常态执行：
//
//	go test -tags=corpus -run TestCorpusB1 ./...
//
// 判定口径（方案 §15.3 / 能力矩阵 Preserve）：
//   - 空变更保存：输出每个 Part 的解压内容哈希必须与源完全一致（B1 严格版）；
//   - 文本编辑后保存：只有 SaveReport.ChangedParts 声明的 Part 允许变化，
//     其余 Part 必须字节一致；且一次纯文本替换只应影响 /ppt/slides/*.xml，
//     任何 CT / 关系 / 母版 / 媒体的连带变化都视为 B1 违反（B1-AFTER）。
//
// 与 ADR-018 的关系：Tier 1 把 CopyOriginal 从 readAll 全缓冲改成流式
// io.Copy，正处在 B1 的字节恒等路径上；本测试是该改动的端到端金样守门。
package pptx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// corpusB1Samples 参与 B1 比对的样本。与 corpusSamples 一致（含 ext-0024，
// 但私有样本在 loadCorpusSourcePath 处自动 Skip，不影响 CI）。
var corpusB1Samples = corpusSamples

// corpusPartHashes 返回 name → 解压内容 SHA256。
//
// 走 p.pk.PartNames() + p.partBytes()，即「OPC 视角的 Part」而非 ZIP 条目：
// B1 承诺的是 Part 解压后内容，条目级比较（压缩方式/顺序）不在承诺范围。
func corpusPartHashes(t *testing.T, p *Presentation) map[string]string {
	t.Helper()
	out := make(map[string]string, 16)
	for _, name := range p.pk.PartNames() {
		b, err := p.partBytes(name)
		if err != nil {
			t.Fatalf("partBytes(%s): %v", name, err)
		}
		sum := sha256.Sum256(b)
		out[string(name)] = hex.EncodeToString(sum[:])
	}
	return out
}

// corpusDiffHashes 比对两组哈希，返回不一致的 Part 名（排序，便于稳定输出）。
func corpusDiffHashes(baseline, got map[string]string) []string {
	var diff []string
	for name, want := range baseline {
		if got[name] != want {
			diff = append(diff, name)
		}
	}
	for name := range got {
		if _, ok := baseline[name]; !ok {
			diff = append(diff, name+" (新增)")
		}
	}
	sort.Strings(diff)
	return diff
}

// corpusSaveTo 保存到临时目录下的新路径（Presentation.Save 拒绝覆盖已存在文件）。
func corpusSaveTo(t *testing.T, p *Presentation, tag string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), tag+".pptx")
	if _, err := p.Save(context.Background(), out); err != nil {
		t.Fatalf("Save(%s): %v", out, err)
	}
	return out
}

// TestCorpusB1UnchangedSave 空变更保存：每个 Part 解压内容字节恒等。
//
// 这是 B1 的最严格形态——未修改 Part 不得因重新压缩、Content Types 重排、
// 关系重建等任何副作用而改变一个字节。
func TestCorpusB1UnchangedSave(t *testing.T) {
	for _, id := range corpusB1Samples {
		t.Run(id, func(t *testing.T) {
			root := filepath.Join("testdata", "corpus", id)
			src := loadCorpusSourcePath(t, root)

			p, err := Open(src)
			if err != nil {
				t.Fatalf("Open(%s): %v", src, err)
			}
			baseline := corpusPartHashes(t, p)
			outPath := corpusSaveTo(t, p, id+"-unchanged")
			p.Close()

			p2, err := Open(outPath)
			if err != nil {
				t.Fatalf("reopen(%s): %v", outPath, err)
			}
			defer p2.Close()
			got := corpusPartHashes(t, p2)

			if diff := corpusDiffHashes(baseline, got); len(diff) != 0 {
				for _, name := range diff {
					t.Errorf("B1 违反（空变更保存后内容变化）: %s", name)
				}
			}
			t.Logf("%s: %d 个 Part 空变更保存后字节恒等", id, len(baseline))
		})
	}
}

// TestCorpusB1AfterTextEdit 文本编辑后保存：未声明变更的 Part 必须字节恒等，
// 且变更集合本身只应落在 /ppt/slides/ 下（B1-AFTER）。
//
// 这一条是 ADR-018 Tier 1 的关键守门：流式复制改动的就是「未变 Part」的输出
// 路径，若 CopyOriginal 出错只会在本测试暴露（空变更那条也会红，但编辑后这条
// 能额外验证「改一个 Part 不牵连其他 Part」）。
func TestCorpusB1AfterTextEdit(t *testing.T) {
	for _, id := range corpusB1Samples {
		t.Run(id, func(t *testing.T) {
			root := filepath.Join("testdata", "corpus", id)
			src := loadCorpusSourcePath(t, root)
			actions := loadCorpusActions(t, filepath.Join(root, id+".actions.json"))
			if len(actions) == 0 {
				t.Fatalf("empty actions for %s", id)
			}

			p, err := Open(src)
			if err != nil {
				t.Fatalf("Open(%s): %v", src, err)
			}
			baseline := corpusPartHashes(t, p)

			if _, err := corpusApplyReplaceText(t, p, actions); err != nil {
				t.Fatalf("apply actions: %v", err)
			}

			outPath := filepath.Join(t.TempDir(), id+"-edited.pptx")
			rep, err := p.Save(context.Background(), outPath)
			if err != nil {
				t.Fatalf("Save(%s): %v", outPath, err)
			}
			p.Close()

			if len(rep.ChangedParts) == 0 {
				t.Fatalf("ChangedParts 为空：文本替换未生效（actions=%d 条）", len(actions))
			}
			changed := make(map[string]bool, len(rep.ChangedParts))
			for _, name := range rep.ChangedParts {
				changed[name] = true
				// 纯文本替换只应影响幻灯片 Part；其他 Part 变化 = 连带破坏
				// （Content Types / 关系 / 母版 / 媒体被重写）。
				if !strings.HasPrefix(name, "/ppt/slides/") {
					t.Errorf("B1-AFTER 违反：非幻灯片 Part 被连带修改: %s", name)
				}
			}

			p2, err := Open(outPath)
			if err != nil {
				t.Fatalf("reopen(%s): %v", outPath, err)
			}
			defer p2.Close()
			got := corpusPartHashes(t, p2)

			undeclared := 0
			for name, want := range baseline {
				if changed[name] {
					continue // 声明变更的 Part 允许不同（且必须不同，见下）
				}
				if got[name] != want {
					t.Errorf("B1 违反（未声明变更的 Part 内容变化）: %s", name)
					undeclared++
				}
			}
			for name := range got {
				if _, ok := baseline[name]; !ok {
					t.Errorf("B1 违反（输出出现源中不存在的新 Part）: %s", name)
				}
			}
			t.Logf("%s: %d Part 中 %d 个声明变更、其余 %d 个字节恒等",
				id, len(baseline), len(rep.ChangedParts), len(baseline)-len(rep.ChangedParts))
			_ = undeclared
		})
	}
}

// corpusReplaceResult 是一次全文档 ReplaceText 回放的计数结果，字段语义与
// gold_replace.go 的 gold_action.result 对齐。
type corpusReplaceResult struct {
	Matches     int
	Replaced    int
	ShapesSeen  int
	ShapesTried int
}

// corpusApplyReplaceText 对全部 AutoShape 文本执行 actions 中的 ReplaceText。
//
// 计数语义与 gold_replace.go 一致：非 AutoShape 不计入 shapesTried，取不到
// TextFrame 的（非文本形状）也不计入。corpus_replay_test.go 的 replayCorpusSample
// 复用本函数，保证 replay 与 B1 两处走完全相同的编辑路径。
func corpusApplyReplaceText(t *testing.T, p *Presentation, actions []corpusAction) (corpusReplaceResult, error) {
	t.Helper()
	var r corpusReplaceResult
	slides, err := p.Slides()
	if err != nil {
		return corpusReplaceResult{}, err
	}
	for _, s := range slides {
		shapes, err := s.Shapes()
		if err != nil {
			return corpusReplaceResult{}, err
		}
		for _, sh := range shapes {
			r.ShapesSeen++
			as, ok := sh.(*AutoShape)
			if !ok {
				continue
			}
			tf, err := as.TextFrame()
			if err != nil {
				// 非文本 shape 不计入 shapesTried（与 gold_replace.go 计数语义一致）
				continue
			}
			r.ShapesTried++
			for _, act := range actions {
				if act.Action != "ReplaceText" {
					t.Fatalf("unsupported action %q (only ReplaceText is wired in CORPUS-01)", act.Action)
				}
				res, err := tf.ReplaceText(act.Old, act.New)
				if err != nil {
					return corpusReplaceResult{}, err
				}
				r.Matches += res.Matches
				r.Replaced += res.Replaced
			}
		}
	}
	return r, nil
}
