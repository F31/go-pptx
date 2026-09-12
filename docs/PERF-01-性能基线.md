# PERF-01 性能基线（方法论文档）

> 依据：《go-pptx 完整设计方案 V2.6》§15.3「发布硬门槛 / 性能」、《go-pptx 项目实施计划》§9.3（自动化与发布节奏）、§9.4（发布硬门槛清单）。
>
> 本文件定义**指标口径与运行方式**（稳定、人工维护）；实际测量数据由 `scripts/perf/run.sh`（或 `run.ps1`）生成，写入 `docs/PERF-01-benchmark-report.md`（自动生成，请勿手工编辑）。

## 1. 范围与目标

§15.3 要求发布候选中给出「benchmark 报告硬件、Go 版本、输入尺寸与 p50/p95/峰值内存」。
本工作包把该要求落为**可复现的基准套件 + 自动报告**，用于：

1. 建立三档规模的**性能基线**，使后续优化有参照、有回归护栏；
2. 为发布候选提供**性能证据包**（原始日志 + 聚合报告）；
3. 落实 §15.3 的披露纪律——**在没有基线前，不写「比 Python 快多少倍」**。

明确不做（§15.3）：不承诺「修改一个字为总耗时 O(1)」。`Save` 仍需整体复制输出包。

## 2. 三档语料

按 §15.3「性能先建立三档基线」，由 `perf_bench_test.go` 用**公共 API**（`New` → `AddSlide` →
`AddTextBox` / `AddPicture` → `Save`）合成，参数化如下：

| 档位 | 页数 | 每页内容 | 说明 |
|---|---|---|---|
| `10p-text` | 10 | 文本框（5 段，首段含唯一标记 `ALPHA`） | 纯文本基线 |
| `50p-image` | 50 | 文本框 + 一张 64×64 小图 | 图文基线（图结构、媒体去重路径） |
| `100p-media` | 100 | 文本框 + 小图；前 20 页各叠加一张**互不相同**的 768×768 噪声大图 | 含大媒体基线 |

关于 `100p-media` 的两个设计要点：

- **噪声图**（`perfNoisePNG`）不可压缩，用于制造真实媒体体积；**每页使用不同种子**，
  否则会被媒体内容哈希去重合并为一份，语料就不再「含大媒体」。
- 语料**在计时区外构建**（`b.ResetTimer` 之前），并按进程缓存（`sync.Once` 语义），
  避免 `-count=N` 时重复生成大语料。

各档的实际输入包字节数（`pkg-bytes`）随实现演进而变，以自动报告为准（量级：约 14 KiB /
60 KiB / 34 MiB）。

## 3. 四类操作（+ 峰值内存）

| 基准 | 操作 | 计时区内容 | 备注 |
|---|---|---|---|
| `BenchmarkPerfOpen` | 打开 | `Open` + `Close` | 全量解析与索引构建 |
| `BenchmarkPerfTraverse` | 遍历 | 打开一次后反复枚举 页→形状→文本框→段落→文本 | 只读热路径 |
| `BenchmarkPerfReplace` | 单处替换 | `TextFrame.ReplaceText` 单命中（`ALPHA`↔`BETA` 可逆） | 每轮恰好 1 处命中，不随迭代退化 |
| `BenchmarkPerfSaveMem` | 保存（内存序列化） | `Write` → `io.Discard` | 纯序列化，不含文件系统 |
| `BenchmarkPerfSaveDisk` | 保存（落盘） | `Save` 到临时路径（含原子替换） | 每轮先删目标，规避 `ErrOutputExists` |
| `BenchmarkPerfPeakHeap` | 全流程峰值内存 | `Open` → 遍历 → `Write` 的堆峰值 | 采样会 STW，其 `ns/op` 不具参考意义 |

保存拆两份的原因：内存 `Write` 反映**序列化成本**，落盘 `Save` 额外含**原子替换与文件系统**
开销；两者语义不同，不混为一谈。

## 4. 指标口径

| 指标 | 单位 | 口径 |
|---|---|---|
| `ns/op` | ns | 框架标准输出；报告取多次 `-count` 采样的 **p50 / p95**（最近秩法） |
| `B/op` | B | `-benchmem` 报告的单次操作堆分配总量 |
| `allocs/op` | 次 | `-benchmem` 报告的单次操作分配次数 |
| `pkg-bytes` | B | **输入尺寸**：语料包落盘字节数（自定义指标） |
| `peak-heap-B` | B | **峰值内存**：采样窗口内相对基线的 `HeapAlloc` 峰值（自定义指标） |

> 报告不呈现「吞吐」列：`Open` 只建索引不解码媒体，`SetBytes` 派生的 MB/s 在小操作 + 大媒体包
> 上会得到无意义的大数（如 10⁵ MB/s 量级）。需要吞吐时按「输入包字节 ÷ 耗时分位数」自行推算，
> 并明确其含义是**整包处理速率**而非解码速率。

**分位数取值**：同一基准名在 `-count=N` 下的 N 个 `ns/op` 样本排序后取分位（p50 / p95）；
其余度量取中位数，`peak-heap-B` 取最大值（峰值语义）。

**峰值内存采样方式**：`perfPeakHeap` 起独立 goroutine，每 200µs 读一次
`runtime.MemStats.HeapAlloc`，记录相对基线的最大值。`ReadMemStats` 会 STW，
故该采样**只在 `BenchmarkPerfPeakHeap` 中进行**，不影响其它基准的 `ns/op`。

## 5. 运行方式

在仓库根执行：

```sh
# POSIX（含 Git Bash / WSL）
scripts/perf/run.sh
```

```powershell
# Windows PowerShell
pwsh -File scripts/perf/run.ps1
```

环境变量（两脚本一致）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `COUNT` | `10` | 采样次数，决定 p50/p95 的样本量 |
| `BENCH` | `BenchmarkPerf` | 基准筛选正则 |
| `BENCHTIME` | 空 | `-benchtime` 覆盖；留空由框架自动定标 |
| `TIMEOUT` | `30m` | `go test -timeout`；`COUNT` 大或 runner 慢时上调（勿依赖 Go 默认 10m） |
| `RAW` | `perf-out/raw-bench.log` | 原始日志（环境头 + `go test` 输出；本地运行日志，已 gitignore 不入库） |
| `OPC_BENCH` | `BenchmarkSavePlanWriteCopyOriginal` | `internal/opc` 组的基准筛选（ADR-018 收益锚点） |
| `OPC_BENCHTIME` | `20x` | 该组 `benchtime`（固定迭代数，便于跨机比较） |
| `OPC_COUNT` | `3` | 该组采样次数 |
| `SKIP_OPC` | `0` | 设为 `1` 跳过 `internal/opc` 组（根包暂不可编译等场合） |
| `OUT` | `docs/PERF-01-benchmark-report.md` | 聚合报告输出 |

快速冒烟（不用于基线，仅验证交付物完好）：

```sh
scripts/perf/smoke.sh          # 确定性守门，见 §9
COUNT=1 BENCHTIME=1x scripts/perf/run.sh   # 端到端跑一遍并出报告
```

### 5.1 `internal/opc` 组：Save 未变 Part 复制（ADR-018 收益锚点）

末段报告中的「保存·未变 Part 复制」来自 `internal/opc/saveplan_bench_test.go`，与三档真实语料
并列但**性质不同**——它是合成媒体包（1×1 MiB / 4×1 MiB / 3×8 MiB 未变媒体 + PeakHeap 行），
测的是保存链路中「把未变 Part 从源包搬到输出包」这一段的分配与峰值堆。

判读要点：

- **分配总量应与媒体体积解耦**：Tier 1（流式 `io.Copy`）下 1×1 MiB 与 3×8 MiB 同为百 KiB 量级
  （`OPC_BENCHTIME=20x` 实测：135.6 KiB / 234.4 KiB / 201.5 KiB）；若 3×8 MiB 行的
  `分配总量/op` 涨到**与媒体体积同量级（数十 MiB）**，说明回退到了「整个 Part 读进内存」的
  全缓冲路径。注意 `-benchtime=1x` 时一次性分配（构造包、zip writer 缓冲）被摊到单次迭代，
  读数会显著偏高（实测 1.27 MiB）——**比较必须在同一 `OPC_BENCHTIME` 下进行**。
- `PeakHeap` 行的 `ns/op` 含采样 STW，只解读**堆峰值增量**列。
- 该组不依赖根包，根包暂不可编译时可用 `SKIP_OPC=1` 之外的组合单独观察。

## 6. 已知限制

- **采样可能漏峰**：200µs 采样间隔会漏掉极短的分配尖峰，`peak-heap-B` 是**下界估计**。
- **未提供 p99**：`COUNT` 默认为 10，样本量不足以支撑 p99；发布候选可提高 `COUNT`。
- **机器强相关**：数值取决于 CPU / 内存 / 存储 / 工具链；跨机器比较前须先固定平台与 Go 版本。
- **GC 噪声**：单次运行存在 GC 时序抖动；用 `-count` 的分位数而非单值解读。
- **打开不解码媒体**：`Open` 只建索引不解码图片，故 `100p-media` 的「包吞吐」偏高，不代表解码性能。
- **落盘基准含文件系统抖动**：`BenchmarkPerfSaveDisk` 受磁盘与杀毒软件影响，解读时看趋势而非单点。

## 7. 披露纪律

- 只报告**已验证**的数据；能力矩阵目标档位不得作为性能结论。
- 未固定平台与 Go 版本前，**不做跨工具链/跨语言比较**（§15.3：在没有基线前，不写「比 Python 快多少倍」）。
- 对外披露性能时，须同时给出**硬件、Go 版本、输入尺寸与分位数**，缺一不可。

## 8. 产物与维护

| 产物 | 性质 |
|---|---|
| `perf_bench_test.go` | 基准套件（三档语料 + 四类操作 + 峰值内存） |
| `internal/opc/saveplan_bench_test.go` | Save「未变 Part 复制」基准（ADR-018 收益锚点：分配 / 峰值堆） |
| `scripts/perf/summarize/` | 报告生成器（解析 `go test -bench` 原始日志 → markdown） |
| `scripts/perf/run.sh` / `run.ps1` | 全量基线驱动脚本 |
| `scripts/perf/smoke.sh` | 确定性冒烟守门（CI 每次 push/PR 调用，见 §9） |
| `perf-out/raw-bench.log` | 本地运行原始日志（`RAW` 默认路径，gitignore，运行后再生；聚合证据以报告为准） |
| `docs/PERF-01-benchmark-report.md` | 最近一次聚合报告（自动生成） |
| `.github/workflows/ci.yml` → `perf-smoke` | 变更时确定性守门 |
| `.github/workflows/perf.yml` → `baseline` | 每日定时全量基线（归档 + Job Summary） |

维护约定：改动保存链路 / 解析链路 / 文本编辑链路后，重跑 `scripts/perf/run.sh` 并以
同一平台的历史报告为参照判断是否回归。

## 9. CI 守门与夜间基线

性能门禁分两层，避免「用不可靠的读数卡住合并」：

### 9.1 `perf-smoke`（`.github/workflows/ci.yml`，每次 push / PR，**阻断**）

调用 `scripts/perf/smoke.sh`，只验**确定性**事实，**不含任何 ns/op 或内存阈值断言**：

1. 基准套件可编译，且 6 组基准 × 3 档语料全部实际执行（防基准被误删/改名/静默跳过）；
2. `scripts/perf/summarize` 能解析当前工具链的 `go test -bench` 输出——**防未来 Go 版本
   变更 bench 行格式导致报告生成器静默失效**（解析失败即非零退出）；
3. 三档语料的 `pkg-bytes` 高于预期下界（8 KiB / 32 KiB / 20 MiB）。其中 `100p-media`
   的下界专门守住「大媒体被媒体内容哈希去重合并」这类使语料名不副实的静默回归
   （见 §2；该问题在 PERF-01 首版真实发生过，语料从 34 MiB 塌到 885 KiB）。

阈值取**下界 + 宽裕余量**而非等值：PNG 编码输出可能随 Go 版本微变，等值断言会误报；
下界足以捕获量级坍塌。

调用代价约数秒（`-benchtime=1x`），可接受放在每次 push/PR。

### 9.2 `baseline`（`.github/workflows/perf.yml`，每日定时 / 手动，**不阻断**）

在固定 runner（`ubuntu-latest` + 固定 Go 版本）跑 `COUNT=10` 全量基线，把报告与原始日志
作为 artifact 归档（保留 90 天），并把报告写入 Job Summary 便于直接阅读。

**为何不做自动回归阈值门禁**：GitHub 托管 runner 的硬件型号与邻居负载不可控，单次读数波动
可达数十个百分点；阈值门禁只会制造噪音红灯，反过来训练团队忽略红灯。回归判断以**同一
runner 家族的历史报告趋势**为准。

**两套读数不可互相比较**：开发机上的 `docs/PERF-01-benchmark-report.md`（如 Intel Core
Ultra 9 275HX）与 CI runner 是不同硬件，任何跨机器比较都须先固定平台与 Go 版本（§6、§7）。
