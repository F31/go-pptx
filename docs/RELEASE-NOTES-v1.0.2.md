# go-pptx v1.0.2 Patch Release Notes

> 2026-09-12 · 将随 `git tag -s v1.0.2`（SSH 签名）发布
>
> **本版本是 v1.0.1 后的第二个 patch release**——公共 API **零变化**（binary-compat with v1.0.0 / v1.0.1），主要工作是 chart 实现搬迁到 `internal/chart`（[ADR-017](adr/ADR-017-chart-internal-extraction.md) 三批）、Save 流式复制落地与量化（[ADR-018](adr/ADR-018-save-streaming-copy.md) Tier 1）+ 反向劣化修复、B1 金样比对真正接入 CI、冻结清单不变量自动化守门、`internal/opc` 达到 COV-04 全闭合。

## 关键不变量（v1.0.0 / v1.0.1 → v1.0.2 不变项）

| 项 | v1.0.0 | v1.0.1 | v1.0.2 | 结论 |
|---|---:|---:|---:|---|
| `// Stable:` 段落数 | 34 | 34 | 34 | ✅ 不变 |
| Stable 符号数 | 50（33 type + 17 哨兵） | 50 | 50 | ✅ 不变 |
| `// Experimental:` 段落数 | 5 | 5 | 5 | ✅ 不变 |
| 公共 API type 总数 | 158 | 158 | 158 | ✅ 不变 |
| 错误哨兵语义 | 锁死 | 锁死 | 锁死 | ✅ 不变 |
| 黄金语料 B1 哈希 | PASS | PASS | PASS | ✅ 不变 |

> v1.0.2 在 v1.0.1 之上新增 **`api_surface_test.go`（7 个 AST 断言）** 把"158 type / 50 Stable 符号 / 17 哨兵"以自动化方式锁死——未来公共 API 任何意外漂移都会在 `go test` 失败。

## Changed（内部实现层 & 性能）

### ADR-017 chart 抽 `internal/chart` 三批完成（commits 46c2f2d / ce3f66c / a3abfca / bc533d3）

按 [ADR-017](adr/ADR-017-chart-internal-extraction.md) 推动：

- **第一批**：搬 3 个真零依赖函数（`BuildChartFrameFragment` / `ChartNumber` / `WorkbookColumn`）+ 4 个常量（`GraphicURI` / `SheetName` / `CatAxID` / `ValAxID`）。依赖审计发现 ADR-017 r0 草案事实错误——canonical/validate/fragment/parse/build 系列大量接收根包值对象与常量，直接搬会反向 import 根包违反 ADR-014。
- **第二批**：11 个值对象通过 **type alias** 方式 move（`font.go` 的 `Optional[T] = chartinternal.Optional[T]`，类型身份与源码级均不变）；私有 → 公开微调（如 `plotElement()` → `PlotElement()`），调用方零修改。
- **第三批**：parse/build/canonical/validate/fragment/workbook 实现全搬迁 + 删除 31 个已无生产调用方的根包 facade。
- 结果：根包 `chart.go` 1319 → 481 行（−63%），`internal/chart` 覆盖率 **91.4%**，公开 API 零变化。
- 5 个搬迁期可复用缺陷：① `c:v`/`a:t` 的值在**标签之间**不在属性上，写成 `Attr("", "val")` 会永远读空；② `CachePoints` 必须按 `pt@idx` 排序补空位；③ `dLbls` 是图表组（barChart/lineChart/pieChart）的子元素，不是 `c:plotArea` 直接子元素；④ `xl/workbook.xml` 的 `xmlns:r` 不能漏（缺则改变 xlsx 字节 → B1 金样失败）；⑤ 抽取时不得"顺手放宽" canonical 白名单。

### ADR-018 Save 流式复制（Tier 1 落地 + 量化 + Tier 2 不实施）（commits 9bfe44d / d7c910e / ee4bb17 / abd9a9f）

按 [ADR-018](adr/ADR-018-save-streaming-copy.md) 推动：

- **问题发现**：`SavePlan.Write` 的 `CopyOriginal` 走 `pk.readAll` 把整个未变 Part 读进内存，峰值内存 = O(最大单个 Part)——媒体重文档的真正热点（"整体复制输出包"是设计契约，不打破）。
- **Tier 1**：改为 `Package.OpenPart`（读取侧 `countedReadCloser` 已强制预算）+ `io.Copy`，峰值内存降为 O(32 KiB 缓冲)。
- **Tier 1 量化**（3×8 MiB 未变媒体，go1.27 windows/amd64，Ultra 9 275HX，`-benchtime=20x`，未变媒体占绝对多数、输出写 `io.Discard`）：

  | 指标 | 改前（commit 1915fc2） | 改后（v1.0.2） | Δ |
  |---|---:|---:|---:|
  | B/op | 64.2 MB | 206 KB | **−99.7%** |
  | allocs | 243 | 148 | −39% |
  | 峰值堆增量 | 52.6 MiB | 6.08 MiB | **−88.4%** |
  | ns/op | 14.65 ms | 8.11 ms | **−44.7%** |

- **Tier 2**（`CreateRaw` / `OpenRaw` 原始帧直通跳过重压缩）经可行性验证：**结论不实施**——仅再加 1.34×，却需新增 `internal/opc` raw API + 输出字节变化须重跑 B1 / L3。收益锚点 `internal/opc/saveplan_bench_test.go` 已接入 PERF-01 与 `scripts/perf/smoke.sh` ①b 守门，重启条件见 ADR-018。
- **反向劣化发现与修复**（commit abd9a9f）：PERF-01 端到端跑出 10 页纯文本保存分配 103.9 KB → 1.11 MB（10×），归因为 `io.Copy` 每次分配 32 KiB 缓冲。改为整轮复用同一缓冲后 10p-text → 76.5 KB、50p-image → 181.9 KB、100p-media → 382.4 KB。

### 工程卫生 & 性能守门（commits 5f105a1 / d1d2854 / 2b0eb10 / 1915fc2）

- WASM 产物瘦身（`scripts/check_wasm.{sh,ps1}` 加 `-trimpath -ldflags="-s -w"`，可用 `SLIM=0` 关闭）——`pptx_check.wasm` 6.069 MB → 5.951 MB（−118 KB / −1.9%），主收益是去掉构建机绝对路径泄漏；脚本提示真正的大头在传输压缩。
- perf 产物收口：`RAW` 默认从 `scripts/perf/raw-bench.log` 改为 `perf-out/raw-bench.log`（`perf-out/` 早已 gitignore），原入库的 raw-bench.log 退库。
- CI fuzz 定时冒烟：`.github/workflows/fuzz.yml` 周一 04:17 UTC cron + 手动 dispatch，8 个 fuzz 目标（打开链路 5 + 文本编辑 2 + 模板绑定 1），种子回归 + 限时探索，崩溃 artifact 30 天。
- `internal/document` 接口编译期契约测试：`var _ Foo = (*Bar)(nil)` + 反射方法数兜底。
- 文档数字与代码实况同步（多处）。

## Fixed（行为修复）

### B1 金样比对真正接入 CI（commit b0144b3）

- **缺口**：原 B1 断言只有合成 fixture（`TestSavePlanUnchangedIsB1`）与私有样本 `ext-0024`（`TestRealExt0024_*`，源文件未入库 → CI 恒 Skip）两处载体——"未修改 Part 解压内容哈希一致"这条 V2.6 §15.3 发布硬门槛**此前从未在 CI 真正执行过**。
- **修复**：新增 `corpus_b1_test.go`（tag=corpus）建在库内可再分发的公开样本上：
  - `TestCorpusB1UnchangedSave`：空变更保存后每 Part 解压内容 SHA256 恒等；
  - `TestCorpusB1AfterTextEdit`：只有 `SaveReport.ChangedParts` 声明的 Part 可变，且变更集合必须等于该集合（其它 Part 字节恒等）；
  - `TestCorpusB1PresentButSkipWhenNoSample`：源样本缺席时优雅 Skip。
- 抽 `corpusApplyReplaceText` 供 replay 与 B1 共用。

### opc 88.9% → 90.4% / COV-04 全闭合（commit b676bab）

- 14 个行为优先测试（Write Omit / 未知 action / 无效 PartName / readAll 缺失 Part / relsPartOf invalid / lastIndexByte 无匹配 / ParseContentTypes 缺属性 / 忽略未知子元素 / extensionOf 边缘 / addOverride 无效 / 重复 / removeOverride / mustAttrEscape 回退）。
- `internal/opc` 升至 **90.4%**，锁住 COV-04 全部 6 包低层格式 5/6 ≥ 90%（`audioprobe` 88.4% 按 [1.x-roadmap B-2](1.x-roadmap.md#方向b覆盖率收尾cov-04-56--6) 决策不追）。

### 冻结清单不变量自动化守门（commits 20b12a4 / b2cac60）

- **缺口**：v1.0.0 时出过一次口径错误（把"段落 grep 数 34"误作"独立 type 数"，漏掉分组 `type ( ... )` 声明的 9 个 → 真实 158），事后连改 6 处文档；路线图 C-1 风险写的"CI 跑公共 API 断言"也从未落地；人工 grep `^type [A-Z]` 只数出 149。
- **修复**：新增 `api_surface_test.go`（根包，`go test` 随 CI 执行）用 `go/ast` 解析根包非测试文件：
  - `TestAPIFrozenCounts`：158 type / 34 Stable 段 / 50 Stable 符号 / 5 Experimental / 17 哨兵
  - `TestAPIFrozenExportedTypes`：导出 type 名集合与 golden 一致
  - `TestAPIFrozenStableSymbols`：Stable 符号集合与 golden 一致
  - `TestAPIFrozenExperimentalSymbols`：Experimental 符号集合与 golden 一致
  - `TestAPIFrozenStableMethods`：127 个 Stable 方法签名集合与 golden 一致
  - `TestErrorSentinelsFrozen`：错误字符串字面量锁死
  - `TestNoBuildConstraintsInRootPackage`：根包无 `//go:build` 约束
- 灵敏度已验证（新增 ZzProbeType → added(1)；Stable 方法改 → removed/renamed；ErrNotFound 字符串改 → want/got；//go:build linux → 报文件名）。
- golden 名单固化，未来公共 API 任何意外漂移都会在 `go test` 失败。

### opc fuzz 安全导向种子扩充（commit 1915fc2）

- 3 条种子：恶意条目名（`../` / `\` / 控制字符）/ 200 条目逼近预算 / 64 层深路径。
- `mustSeedZip` helper 集中构造恶意但合法的 ZIP 流。
- 配合 V2.6 §13 的"已知高严重度安全缺陷为零"硬门槛。

## Documented（文档体系完善）

- [`docs/adr/ADR-017-chart-internal-extraction.md`](docs/adr/ADR-017-chart-internal-extraction.md) —— chart 抽 `internal/chart` 的分批路径与踩坑记录（含"严格区分真零依赖 vs 接收根包值对象"、"const 必为编译期常量不可直接引用 var 包常量"、"type alias 不能定义方法"三条经验）。
- [`docs/adr/ADR-018-save-streaming-copy.md`](docs/adr/ADR-018-save-streaming-copy.md) —— Save 流式复制（含 Tier 2 不实施决策与反向劣化修正章节）。
- [`docs/PERF-01-benchmark-report.md`](docs/PERF-01-benchmark-report.md)（更新）—— 接入 ADR-018 收益与守门说明。
- [`docs/release-readiness-2026-09-12.md`](docs/release-readiness-2026-09-12.md) —— 本次发布的就绪度评估报告。

## Verification（如何验证）

```bash
# 全测试（含语料 replay + 冻结守门 + B1）
go test ./...                                        # 14/14 ok（默认）
go test -tags=corpus ./...                           # 14/14 ok（含 B1）

# 冻结守门（AST 断言 7 个测试 + B1 公开语料）
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v .     # 7/7 PASS
go test -run 'TestCorpusB1' -v -tags=corpus .                                   # 含 4 个公开语料 + 私有 Skip

# 覆盖率（口径同步到 2026-09-12 21:4x 实测）
CGO_ENABLED=0 go test ./... -coverprofile=/tmp/cover.out
# full total 84.4% / root 82.0% / opc 90.4% / chart 91.4%

# 交叉构建
GOOS=js GOARCH=wasm go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=wasip1 GOARCH=wasm go build ./...
GOOS=linux GOARCH=arm64 go build ./...

# v1.0.0 / v1.0.1 → v1.0.2 binary-compat 断言
#   1. TestAPIFrozenExportedTypes 测试通过 → 公共 type 集合零漂移
#   2. TestAPIFrozenStableMethods 测试通过 → 127 个 Stable 方法签名零漂移
#   3. TestCorpusB1UnchangedSave 通过 → Part 字节级保真

# fuzz 定时冒烟（CI 周一 04:17 UTC 自动）
gh workflow run fuzz.yml
```

## Compatibility

- **v1.0.0 → v1.0.1**：binary-compatible（公共 API 零变化）
- **v1.0.1 → v1.0.2**：binary-compatible（公共 API 零变化；`Optional[T]` 由定义类型变为 type alias 引用 `internal/chart.Optional[T]`，**类型身份不变**，因此既有的 `pptx.Optional[int]{Value:1, Set:true}` 复合字面量与 `NewOptional` 调用均无需修改）
- **v1.0.2 → v1.1.0**（未来）：待 ADR-017 第四批与 1.x 路线图决定

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- v1.0.2 的主要场景适用：chart 内部重构为后续 1.x 渲染/扩展能力提供基础；Save 流式复制把媒体重文档的内存峰值降一个数量级；B1 金样比对真正进入 CI 把守"字节级保真"硬门槛

## Acknowledgments

本 patch 基于 v1.0.1 的稳定 API 锁定 + 守门模板，全部增量在 `[Unreleased]` 段累积后归并。详见 [`../CHANGELOG.md`](../CHANGELOG.md) `## [1.0.2] - 2026-09-12` 段。