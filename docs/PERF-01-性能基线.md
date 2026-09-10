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
| `RAW` | `scripts/perf/raw-bench.log` | 原始日志（环境头 + `go test` 输出，证据留档） |
| `OUT` | `docs/PERF-01-benchmark-report.md` | 聚合报告输出 |

快速冒烟（不用于基线，仅验证套件可跑）：

```sh
COUNT=1 BENCHTIME=1x scripts/perf/run.sh
```

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
| `scripts/perf/summarize/` | 报告生成器（解析 `go test -bench` 原始日志 → markdown） |
| `scripts/perf/run.sh` / `run.ps1` | 驱动脚本 |
| `scripts/perf/raw-bench.log` | 最近一次原始日志（证据） |
| `docs/PERF-01-benchmark-report.md` | 最近一次聚合报告（自动生成） |

维护约定：改动保存链路 / 解析链路 / 文本编辑链路后，重跑 `scripts/perf/run.sh` 并以
同一平台的历史报告为参照判断是否回归。
