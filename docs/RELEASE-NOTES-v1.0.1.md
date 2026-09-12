# go-pptx v1.0.1 Patch Release Notes

> 2026-09-12 · 将随 `git tag -s v1.0.1`（SSH 签名）发布
>
> **本版本是 v1.0.0 后的首个 patch release**——公共 API **零变化**（binary-compat with v1.0.0），主要工作是内部实现层重构、bug 修复、覆盖率收敛、客户端矩阵真机执行与文档体系完善。

## 关键不变量（v1.0.0 → v1.0.1 不变项）

| 项 | v1.0.0 | v1.0.1 | 结论 |
|---|---:|---:|---|
| `// Stable:` 段落数 | 34 | 34 | ✅ 不变 |
| Stable 符号数 | 50（33 type + 17 哨兵） | 50 | ✅ 不变 |
| `// Experimental:` 段落数 | 5 | 5 | ✅ 不变 |
| 公共 API type 总数 | 158 | 158 | ✅ 不变 |
| 错误哨兵语义 | 锁死 | 锁死 | ✅ 不变 |
| 黄金语料 B1 哈希 | PASS | PASS | ✅ 不变 |

## Changed（内部实现层 & 行为）

### ADR-016 渐进式 internal 抽取完成首轮收敛（commit 76a630f）

按 [ADR-016](adr/ADR-016-progressive-internal-extraction.md) 推动：
- 新增 `internal/document`（PartStore / ReadStore / PatchStore 契约）、`internal/textmap`（rune 映射 / 文本逻辑位置）、`internal/editplan`（`SinglePartPatch` / `MultiPartPlan`）。
- 根包新增未导出 `document_store.go` adapter，把生产业务写入路径全部收敛到 `applySinglePartPatch` / `applyMultiPartPlan`，**直接 `stageAdd / stagePatch / stageDelete / commit` 调用仅保留在 `presentation.go` 底层 primitive 与 `document_store.go` adapter 边界内**。
- 已迁移路径：文本、形状、transition、audio timing、table、AddSlide / RemoveSlide、notes、Bind、图片 / 音频 / 视频、chart、clone、docProps。
- 配套根包导入图收敛：根 → 7 个 internal 子包；internal → opc / xmlstore；反向 import 闭环。
- 公开 API 零变化。

### ir 包表格文本投影闭环（commit def9140）

- `ir.readTableText` / `ir.cellText` 由 0% 覆盖补齐——新增 `ir/table_ir_test.go` 自建最小 zip fixture deck（公开 `OpenReader`），覆盖 2×2 文本、空单元格、零行表格、非正维度直接调用四类场景。
- 副作用：`ir.Diff` 现在能更准确地报告表格单元格文本变化，公开 API 签名零变化。

### 覆盖率收敛（COV-01 / 02 / 04）

- **COV-01 达成**（full coverprofile total ≥ 80%）：full 78.3% → **80.0%**（2026-09-11）。
- **COV-02 达成**（root ≥ 82%）：root 81.5% → **82.0%**，full total 82.5% → **82.9%**（2026-09-11）。
- **COV-04 评估**：放弃统一 90% per-package 口径——**90% 仅适用于低层格式包**（opc / xmlstore / videoprobe / audioprobe / textmap / editplan），root SDK 目标 85%，command/helper 目标 85%，`ir` / `render` 行为优先。
- **5 包行为优先补测**（2026-09-12）：
  - `internal/editplan` 81.8% → **100.0%**（26 测试新增）
  - `internal/textmap` 82.2% → **100.0%**
  - `internal/opc` 86.6% → **88.9%**
  - `internal/audioprobe` 84.9% → **88.4%**
  - root SDK 82.7% → **82.8%**
  - **full-repo 加权 total 83.2% → 84.4%**，4/6 低层格式包 ≥ 90%（`videoprobe` 92.6% / `xmlstore` 90.8% / `editplan` 100% / `textmap` 100%）。

## Fixed（行为修复）

### MediaSource nil 流保护（commit d087aff）

- `MediaSource.Reader()` / `MediaSource.Length()` / `MediaSource.MediaType()` 三个公共访问器对 nil receiver 显式返回错误（之前 panic 在 nil deref）。详见 commit d087aff。
- 副作用：依赖 `nil` 接收方走默认行为的代码需注意——但此行为之前是 panic，按"由 panic 改为 error"算**只加防御不减能力**，严格 PATCH。

### 多处小 bug 修复与诊断改进（commit ae48794）

- Stable 计数 off-by-one 口径修正：33 独立 type + 17 哨兵 = 50 符号 / 34 段落 / 5 Experimental / 120 API / 158 总。勘误前 6 处文档（CHANGELOG / RELEASE-NOTES-v1.0.0 / v1.0-freeze-list / 实施状态跟踪 / 技术白皮书 / MEMORY.md）数字统一。
- ADR-014 三处缺陷修正：① "因…而…"悬空残句；② 依赖清单补充 `internal/document` / `internal/editplan` / `internal/textmap`；③ 目录树补 `internal/document`。
- 文档数字与代码实况同步。

## Documented（文档体系完善）

### L3 客户端矩阵真机执行完成（commit a103484 + 5f99d80 + dfc27c3）

- **8/8 通过**：PowerPoint 16.0.20326（`PowerPoint.Application`）+ WPS 演示 12.1.0.28599（`Kwpp.Application`）× 4 样本（`s001-text` / `s002-table` / `s003-image` / `ext-0024`）。
- Windows 11 宿主机（WSL 触发 COM 自动化）打开样本均**无修复提示**，另存 `.pptx` 成功，go-pptx 重开后 `Validate` errorCount=0，编辑内容保留。
- 新增可复现工具 `scripts/l3/run_client.sh` + `scripts/l3/ppt_open_resave.ps1`；证据 hash 与重存文件在 `.l3-output/`（gitignore）。
- 详见 [`client-compat-matrix.md`](client-compat-matrix.md)。

### go-pptx 技术白皮书（commit 0218b29）

- 新增 [`go-pptx-技术白皮书.md`](go-pptx-技术白皮书.md)（746 行 / 12 章），覆盖：
  - 产品定位、技术架构（原始字节 + 命名空间环境双模型 / 事务化编辑 / 字节级保真 / 能力自描述 / 语义 diff / 语料驱动）
  - 功能特性、应用场景（自动化报告 / CI 质量门 / 受限环境 / 隐私敏感 / 模板驱动）
  - 与 python-pptx / Apache POI / LibreOffice UNO / Aspose.Slides / Open XML SDK 的对比矩阵
  - 7 项技术创新点详解（STALE-GUARD 三层 / xmlstore span patch / DIFF-01 加权 LCS / Jaccard / Capability 六维 / 三级 API 承诺 / 公开金样 CI / 原子保存）

### 1.x 路线图方案（commit 24d4fa6）

- 新增 [`1.x-roadmap.md`](1.x-roadmap.md)（213 行 / 8 章）：
  - 5 个候选方向（A 架构 refactor / B 覆盖率收尾 / C v1.0.1 patch / D 用户场景按需 / E 客户端+分发可选）
  - 5 阶段路线（patch → COV-04 全闭合 → 架构 refactor 主线 → 分发扩展 → 用户场景）
  - 6 风险登记 + 5 推荐决策点

### 稳定性文档同步（commit ae48794）

- ADR-014 / 015 / 016 完成情况审计与代码实况对照：
  - **ADR-016 已全部落地**：internal/document / textmap / editplan 三包 + adapter + 业务路径收敛；公开 API 零变化。
  - **ADR-014 依赖方向干净**：根 → 7 个 internal；internal → opc / xmlstore；ir 只被 cmd/pptx 与 wasm/check 引用；render 零引用；fmt.Sprintf 拼 XML 仅 audiotiming.go 2 处。
  - **ADR-015 v1.0 Stable 计数 off-by-one 修正**：见上"Fixed"段。

## Verification（如何验证）

```bash
# 全测试（含语料 replay）
go test ./...

# 覆盖率（口径同步到 09-12 下午实测）
CGO_ENABLED=0 go test ./... -coverprofile=/tmp/cover.out
go tool cover -func=/tmp/cover.out | tail -1    # total: 84.4% (of statements)

# 公开语料 replay
scripts/gen_corpus/run.sh validate testdata/corpus
go test -tags=corpus ./...

# 四目标交叉构建
GOOS=js GOARCH=wasm go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=wasip1 GOARCH=wasm go build ./...
GOOS=linux GOARCH=arm64 go build ./...

# v1.0.0 → v1.0.1 binary-compat 断言
diff <(git show v1.0.0:errors.go) <(git show v1.0.1:errors.go)  # 仅 expect 空 diff
diff <(git show v1.0.0:capability.go) <(git show v1.0.1:capability.go)
```

## Compatibility

- **v1.0.0 → v1.0.1**：binary-compatible（公共 API 零变化；见上"关键不变量"）
- **v1.0.1 → v1.1.0**（未来）：待 ADR-017 决定

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- v1.0.1 的主要场景适用——L3 客户端兼容性初次确认、覆盖率收敛里程碑、内部事务边界稳定

## Acknowledgments

本 patch 基于 v1.0.0 的稳定 API 锁定，全部增量在 `[Unreleased]` 段累积后归并。详见 [`../CHANGELOG.md`](../CHANGELOG.md) `## [1.0.1] - 2026-09-12` 段。
