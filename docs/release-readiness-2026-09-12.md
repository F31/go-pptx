# go-pptx 发布就绪度评估（2026-09-12）

> 评估基准：`HEAD = c08249d`（领先 `origin/main` 6 个提交），上一个 tag `v1.0.1`（commit 9f24866）
> 对照文档：《go-pptx 完整设计方案 V2.6 开发实施版》§15.3 / §16、《go-pptx 项目实施计划》、《1.x 路线图》、《coverage-roadmap》
> 本文所有"实测"数字均在本次评估中重新跑出，未沿用文档登记值。

---

## 0. 结论摘要

| 问题 | 结论 |
|---|---|
| 是否可以发布相对稳定版本？ | **可以**。建议 `v1.0.2`（patch），公共 API 表面零变化、B1 保真不变、14 包测试双口径全绿 |
| 技术方案（V2.6）是否闭环？ | **基本闭环**。§15.3 五条发布硬门槛中 4 条已闭合、1 条部分闭合（配音真机播放记录缺失，属环境型缺口） |
| 开发计划（M0–M8）是否闭环？ | **是**。工作包总表 34 项全部"已完成"，QA-01 为持续性工作包（进行中） |
| 发布前必须补的动作 | 4 项（见 §5）：推送 6 个未推提交、填 CHANGELOG、写 RELEASE-NOTES、跑远端 CI（含 race） |

---

## 1. v1.0.1 → HEAD 都做了什么（32 个提交，+6040 / −1953，51 文件）

按性质分为五条主线：

### 1.1 工程卫生与 CI（4）
- `d1d2854` / `2b0eb10`：清理误入库的运行产物（`scripts/perf/raw-bench.log` 退库转 gitignore + 口径修正）。
- `5f105a1`：WASM 产物瘦身（`-trimpath -ldflags="-s -w"`，6.069 → 5.951 MB，主收益是去掉构建机绝对路径泄漏）、perf 产物收口、新增 `.github/workflows/fuzz.yml`（周一 04:17 UTC 定时 + 手动触发，8 个 fuzz 目标）。

### 1.2 架构重构 ADR-017（chart 抽 `internal/chart`，5）
- `46c2f2d` 第一批：3 个真零依赖函数 + 4 个常量。
- `ce3f66c` 第二批：11 个值对象 + `Optional[T]` 以 **type alias** 方式 move（`font.go` 的 `Optional[T]` 改为 `= chartinternal.Optional[T]`，源码级与类型身份均不变）。
- `a3abfca` 第三批：parse/build/canonical/validate/fragment/workbook 全实现搬迁 + 删除 31 个已无生产调用方的根包 facade（含 `bc533d3` 死常量清理）。
- 结果：根包 `chart.go` 1319 → 481 行，`internal/chart` 覆盖率 **91.4%**，公共 API 零变化。

### 1.3 性能 ADR-018（Save 流式复制，4）
- `9bfe44d` Tier 1：`SavePlan.Write` 的 `CopyOriginal` 从"整 Part 读进内存"改为 `OpenPart` + `io.Copy`，峰值内存从 O(最大 Part) 降为 O(32 KiB 缓冲)。
- `d7c910e` 收益量化（3×8 MiB 未变媒体：B/op −99.7%、峰值堆 −88.4%、耗时 −44.7%）+ Tier 2 可行性验证（**结论：不实施**，仅再加 1.34× 却需改输出字节）。
- `ee4bb17` 收益接入 PERF-01 基线与 `scripts/perf/smoke.sh` 守门。
- `abd9a9f` **修复 Tier 1 引入的反向劣化**：`io.Copy` 每次调用分配 32 KiB，小档语料保存分配 103.9 KB → 1.11 MB（10×），改为整轮复用同一缓冲后回落至 76.5 KB。

### 1.4 质量闭环（4）
- `b676bab`：`internal/opc` 88.9% → **90.0%**（COV-04 全闭合）。
- `1915fc2`：`internal/document` 接口编译期契约测试 + opc fuzz 安全导向种子语料（恶意条目名 / 200 条目逼近预算 / 64 层深路径）。
- `b0144b3`：**B1 金样比对补测**——修复"B1 在 CI 上从未真正生效"的缺口（原断言只挂在合成 fixture 与未入库的私有样本 ext-0024 上，CI 恒 Skip）。新增 `corpus_b1_test.go`（tag=corpus），建在库内可再分发的公开语料上。
- `20b12a4` + `b2cac60`：**v1.0 冻结清单不变量自动化守门**——`api_surface_test.go` 用 `go/ast` 断言 158 type / 34 Stable 段 / 50 Stable 符号 / 127 Stable 方法 / 5 Experimental / 17 哨兵，替代此前不可靠的人工 grep（`grep '^type [A-Z]'` 只数出 149，漏掉分组声明的 9 个）。

### 1.5 文档同步（15）：ADR-017 / ADR-018、PERF-01 报告、coverage-roadmap、实施状态跟踪、MEMORY。

---

## 2. 对照技术方案与开发计划的闭环核对

### 2.1 开发计划 M0–M8：闭环 ✅

`docs/go-pptx-实施状态跟踪.md` §"工作包总表（34 项）"：CORE-01 → DIFF-01 全部标记**已完成**，唯一"进行中"是 QA-01（语料/fuzz/兼容报告，性质上属持续工作包）。

M8 遗留四项中：L3 客户端矩阵已于 2026-09-11 真机 8/8 闭合；原生渲染实现（ADR-014 后续立项）与私有语料入库属**设计态取舍**，非缺陷。

### 2.2 技术方案 V2.6 §15.3 发布硬门槛：4 闭合 / 1 部分闭合

| # | 硬门槛 | 状态 | 证据 |
|---|---|---|---|
| 1 | 已声明支持的金样操作通过结构断言及再次解析，输出不新增校验错误 | ✅ | corpus replay：36 样本索引 + 3 公开金样；`go test ./...` 与 `go test -tags=corpus ./...` 均 14/14 包 ok |
| 2 | 未修改 Part 的解压内容哈希一致 | ✅ | B1：`corpus_b1_test.go`（空变更保存各 Part SHA256 恒等 + 文本编辑后变更集合必须等于 `SaveReport.ChangedParts`），已接入 CI 且真实执行 |
| 3 | PowerPoint/WPS 关键用例实际打开无修复提示；**配音功能必须有播放记录** | 🟡 **部分闭合** | 打开/重存部分 ✅（PowerPoint 16.0.20326 + WPS 12.1.0.28599 × 4 样本 = 8/8，无修复提示、重存后 go-pptx `Validate` errorCount=0）；**配音播放记录 ❌**——语料 36 个 manifest 中无任何 audio 标签样本，L3 从未执行音频/配音播放验证 |
| 4 | 高严重度数据丢失 / 悬空引用 / 保存损坏 / 安全缺陷为零 | ✅ | AT-14 恶意包 panic 已修（三入口显式错误）；fuzz 八目标 + CI 定时冒烟 |
| 5 | 示例可编译、API 文档说明限制、benchmark 报告含硬件/Go 版本/输入尺寸/p50/p95/峰值内存 | ✅ | `docs/PERF-01-benchmark-report.md` 含 p50 / p95 / 分配量 / 峰值堆增量；`scripts/perf/` 可复现 |

> 关于第 3 条：这是**唯一实质未闭合的硬门槛**，性质是环境型缺口（缺音频语料 + COM 自动化无法验证播放效果），不是代码缺陷——AUDIO-01/02/03 有代码级测试（AT-05/06/07/08/12）。若要求"完全闭合"，需补一个含配音的公开样本并人工在 PowerPoint/WPS 上播放确认。

### 2.3 覆盖率目标 COV-01 ~ COV-04

| 目标 | 要求 | 实测（本次） | 状态 |
|---|---:|---:|---|
| COV-01 | full total ≥ 80% | **84.4%**（10984/13010） | ✅ |
| COV-02 | root ≥ 82% | **82.0%**（7288/8888） | ✅（**贴线**） |
| COV-03 | command/helper ≥ 85% | cmd/pptx 87.2% / wasm/check 89.5% / summarize 91.0% | ✅ |
| COV-04 | 低层格式包 ≥ 90% | xmlstore 90.8% / opc 90.4% / videoprobe 92.6% / textmap 100% / editplan 100% / audioprobe 88.4% | ✅（5/6，audioprobe 按 B-2 决策不追） |

> 注：文档登记的 root 为 82.8%，本次实测两种口径（默认 / `-tags=corpus`）均为 **82.0%**。COV-01 的 full total 84.4% 与文档一致。root 距 COV-02 门槛仅 0.0x 个百分点，后续任何把高覆盖代码搬出根包的重构都可能使其跌破——这是 1.x 需要持续盯守的数字。

### 2.4 1.x 路线图阶段核对

| 阶段 | 工作包 | 状态 |
|---|---|---|
| 阶段 1 | C-1 v1.0.1 patch | ✅ 已发布（tag `13d1376`） |
| | C-2 脚本目录收口 | ✅ |
| | C-3 已知小 bug 修复登记（bug registry） | ❌ **未落地**（无对应文档；0.5d，非发布阻塞） |
| | A-3 internal/document 编译期断言 | ✅（`1915fc2`） |
| 阶段 2 | B-1 opc → 90% | ✅ 90.4% |
| | B-3 root → 85% | ❌ 建议按路线图决策 3 **放弃**（边际收益低，停在 82%） |
| 阶段 3 | A-1 chart 抽 internal | 🟡 三批完成（根包 1319 → 481 行），第四批（值对象全面下沉）未启动 |
| | A-2 / A-4 / A-5 | ❌ 未启动（1.x 中期，看反馈） |

---

## 3. 当前实测质量快照（2026-09-12 21:4x 复测）

| 检查 | 结果 |
|---|---|
| `go build ./...` / `go vet ./...` | 通过（零告警） |
| `go test ./... -count=1` | **14/14 包 ok** |
| `go test -tags=corpus ./... -count=1` | **14/14 包 ok** |
| 冻结守门 `api_surface_test.go` | **7/7 PASS**（Counts / ExportedTypes / StableSymbols / ExperimentalSymbols / StableMethods / ErrorSentinels / NoBuildConstraints） |
| 交叉编译（CGO_ENABLED=0） | js/wasm ✅、darwin/arm64 ✅、wasip1/wasm ✅、linux/arm64 ✅ |
| gofmt | 4 个文件被标记，均为 **CRLF 行尾假阳性**（`git diff` 为空、工作区内容与 HEAD 一致），git 内为 LF |
| full total 覆盖率 | 84.4% |

---

## 4. 未闭环项清单（按性质分类）

**A. 真缺口（建议 1.x 早期补）**
1. **配音真机播放记录**（V2.6 §15.3 第 3 条）——缺音频语料 + 无人工播放证据。
2. **C-3 bug registry**——路线图阶段 1 的 0.5d 待办，未落地。

**B. 设计态（不算缺陷）**
3. **Render 维度 Untested**——M8 只交付 `render` 子包接口契约（RENDER-01），原生渲染实现按方案 §23.1 / ADR-014 属"V1 不承诺、后续立项"。
4. **私有 ext-* 原始 PPTX 入库**——许可/体积取舍，已用 3 个公开 LibreOffice 金样兜底。

**C. 已决策放弃**
5. **root 85%**（B-3，边际收益低）、**audioprobe 90%**（B-2，剩余为不可达防御代码）。

**D. 风险提示**
6. **root 覆盖率 82.0% 贴 COV-02 门槛线**——ADR-017 第三批已经让该数字从 82.8% 降到 82.0%；后续抽取类重构必须同步迁移白盒测试，否则会跌破 80% 关门值。

---

## 5. 发布建议

### 版本号：**v1.0.2（patch）**

理由：公共 API 表面零变化（AST 守门 7/7 锁死 158 type / 34 Stable 段 / 50 符号 / 127 方法 / 5 Experimental / 17 哨兵），B1 保真不变，无新增功能，ADR-017/018 均为内部实现与性能变更。若希望把两个新 ADR 与 `internal/chart` 的存在对外显式告知，也可升 `v1.1.0`，但按 semver 与 ADR-015 的"仅追加"原则，**patch 更准确**。

### 发布前必做 4 步

1. **推送 6 个未推提交**：`b0144b3` `abd9a9f` `8e89810` `20b12a4` `b2cac60` `c08249d`（当前 `main` ahead 6）。
2. **填 `CHANGELOG.md` 的 `[Unreleased]` 段**：登记 32 个提交（ADR-017 三批 / ADR-018 Tier 1 + 劣化修复 / B1 补测 / 冻结守门 / opc 90% / fuzz CI / WASM 瘦身）。
3. **新建 `docs/RELEASE-NOTES-v1.0.2.md`**（沿用 v1.0.1 模板：不变量清单 + Changed / Fixed / Added）。
4. **远端 CI 全绿确认**：含 ubuntu `go test -race`（本机无 gcc，跑不了）、corpus job、4 目标交叉构建；随后 `git tag -s v1.0.2`（仓库级 SSH 签名）+ push tag。

### 可选增强（不阻塞发布）
- 补一个含配音的公开语料样本 + 人工播放确认，闭合 §15.3 第 3 条。
- fuzz 八目标长跑一次（CI 已配周一定时）。
- root 覆盖率补 3–5 个高价值系统测试，把 COV-02 从"贴线"拉到 83%+ 留缓冲。
