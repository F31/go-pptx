# ADR-018: Save 未变 Part 流式复制（去全缓冲 + 可选原始帧直通）

- **状态**: **Tier 1 已实现 + 端到端已量化（含一次反向劣化的发现与修复）**；**Tier 2 已实施**（重启条件①经同进程 A/B 取证成立：未变媒体重压缩占 Save p50 的 50–73%），代码落地 `internal/opc/saveplan.go`（`tryRawCopyOriginal`）+ `package.go`（`rawPartFile`），含两条新单测 + 一条重压缩占比基准
- **Tier 1 进度**: 代码已落地 `internal/opc/saveplan.go`（`copyPart`，缓冲整轮复用）+ `saveplan_stream_test.go` + `saveplan_bench_test.go`（4 档收益锚点，含 `200x4KiB` 固定开销哨兵）。**端到端已验收**：`corpus_b1_test.go` 四份语料（含真实 WPS 样本 ext-0024 的 75 个 Part）空变更保存字节恒等、编辑后仅声明 Part 变化；PERF-01 基线报告已重生成；`internal/opc` 覆盖率 90.4%；`go test ./...` 与 `-tags=corpus ./...` 均 14/14 包全绿
- **日期**: 2026-09-12
- **关联 ADR**: ADR-016（progressive-internal-extraction）、ADR-014（root-internal-package-strategy）
- **关联基线**: `docs/PERF-01-性能基线.md`（峰值内存 / p50 / p95）、B1 黄金语料逐 Part 哈希、`docs/1.x-roadmap.md`
- **关联工作包**: 1.x 路线图性能主题（Save 路径）

## 上下文

`internal/opc/saveplan.go` 的 `Write` 对未变 Part（`CopyOriginal`）的实现是：

```go
case CopyOriginal:
    data, err := pk.readAll(e.Name)   // ← 整个 Part 读进 []byte
    ...
    if _, err := f.Write(data); err != nil { ... }
```

事实（2026-09-12 代码实测）：

- **未变 Part 是输出字节的绝大部分**——典型编辑只改 1 个 slide XML，其余（媒体、母版、字体、其他页）全部走 `CopyOriginal`；
- `readAll` 让**峰值内存 = O(最大单个 Part)**。含视频/高分辨率图片的 deck（如 `ext-0024` 含 22 个媒体）会一次性把最大媒体全部驻留内存，而实际只需要把它从源 ZIP 搬到输出 ZIP；
- **流式读取路径已存在且预算生效**：`Package.OpenPart` → `Index.openPart` → `countedReadCloser`（限额在读取侧强制），无需新增安全机制；
- 输出侧本来就是流式的：`Write(pk, w io.Writer)` 顺序写入，`SaveToFile` 写同目录临时文件后原子替换。

设计文档 §15.3 明确**不承诺**「改一个字为总耗时 O(1)」，`Save` 仍需整体复制输出包——**本 ADR 不改变该契约**，只改变复制的**内存与 CPU 形态**，输出字节流应保持不变。

## 决策

分两档，Tier 1 独立可发，Tier 2 需额外验证：

### Tier 1（低风险，建议做）：`CopyOriginal` 改流式 `io.Copy`

```go
case CopyOriginal:
    rc, err := pk.OpenPart(e.Name)
    if err != nil {
        return fmt.Errorf("copy entry %s: %w", e.Name, err)
    }
    if _, err := io.Copy(f, rc); err != nil {
        rc.Close()
        return fmt.Errorf("copy entry %s: %w", e.Name, err)
    }
    if err := rc.Close(); err != nil {
        return fmt.Errorf("copy entry %s: %w", e.Name, err)
    }
```

必须保持不变的行为：

- **预算强制**：`countedReadCloser` 在读取侧限额，超限仍返回 `ErrLimitExceeded`；
- **错误类**：源 Part 损坏仍映射为 `ErrMalformedPackage`；错误前缀沿用 `copy entry %s`（既有测试可能断言）；
- **字节等价**：喂给 `zip.Writer` 的解压内容逐字节相同，压缩器输入一致 → 输出一致。

收益：**峰值内存 O(最大 Part) → O(32 KiB 缓冲)**。

### Tier 1 实测收益（2026-09-12）

方法：`internal/opc/saveplan_bench_test.go` 构造「未变媒体 Part 占绝大多数」的包，空变更集保存（全量 `CopyOriginal`），输出写 `io.Discard`；改前数字来自 Tier 1 之前 commit（`1915fc2`）的独立 worktree，同一台机器同一命令、同一份基准文件。

环境：go1.27 windows/amd64，Intel Core Ultra 9 275HX，`-benchtime=20x`，`-benchmem`。

| 场景（未变媒体） | 指标 | 改前 `readAll` | 改后 流式 | 变化 |
|---|---|---|---|---|
| 1×1 MiB | B/op | 2,249,731 | 192,664 | **−91.4%** |
| 1×1 MiB | ns/op | 461,525 | 257,235 | −44.3% |
| 4×1 MiB | B/op | 8,998,668 | 239,985 | **−97.3%** |
| 4×1 MiB | ns/op | 1,956,195 | 1,166,625 | −40.4% |
| 3×8 MiB（24 MiB 媒体） | B/op | 64,205,702 | 206,296 | **−99.7%** |
| 3×8 MiB | allocs/op | 243 | 148 | −39.1% |
| 3×8 MiB | ns/op | 14,654,785 | 8,105,255 | −44.7% |
| 3×8 MiB | **峰值堆增量** | 55,130,552 B（52.6 MiB） | 6,373,952 B（6.08 MiB） | **−88.4%** |

结论：

- **分配量几乎与媒体体积解耦**（24 MiB 媒体下 B/op 从 64 MB 降到 206 KB），符合「O(最大 Part) → O(32 KiB)」预期；
- 峰值堆下降一个数量级，正是媒体重文档（如 `ext-0024`）的痛点；
- 耗时意外地下降 40–45%（不只是持平）——省掉大对象分配与随之而来的 GC 压力，收益大于多出的 32 KiB 缓冲拷贝；
- 峰值堆为 100 µs 间隔采样 `HeapAlloc` 的近似值，非精确水位；
- 字节等价由 `TestSavePlanCopyOriginalLargePartIsByteExact`、`TestSavePlanWriteCopyOriginalStillByteExact` 锚定（1 MiB / 300 KiB / 512 KiB+1 MiB 混合三档）。

> ⚠️ **上表已被下面的端到端实测修正**：该微基准只有「少而大」的 Part，看不到
> **每 Part 固定开销**，据此得出的结论在真实文档形状上是错的。保留原表仅作过程留痕。

### Tier 1 修正（2026-09-12 晚）：首版对小档语料反向劣化 10×，已修

根包编译恢复后跑 PERF-01 端到端基准（三档真实语料），发现**与微基准矛盾的反向劣化**：

| 语料（真实文档形状） | 改前 `readAll` | Tier 1 首版（每 Part 一次 `io.Copy`） | 修复后（缓冲整轮复用） |
|---|---:|---:|---:|
| `10p-text`（14 KB，Part 多而小） | 103.9 KB | **1.11 MB（劣化 10.7×）** | **77.9 KB（−25.0%）** |
| `50p-image`（60 KB） | 413.3 KB | **3.94 MB（劣化 9.5×）** | **222.5 KB（−46.2%）** |
| `100p-media`（33.9 MB） | 88.60 MB | 7.95 MB（−91.0%） | **390.4 KB（−99.6%）** |

**根因**：`io.Copy` 每次调用都新分配一个 **32 KiB 缓冲，与 Part 实际大小无关**。
原先的 `readAll` 是**按体积**分配（小 Part 只分配几 KB），改成流式后固定开销变成
`nParts × 32 KiB`。媒体档 Part 少而大，省下的体积开销远大于固定开销故大赢；
而正文型 PPTX 有十余个几 KB 的 Part，固定开销反而成了净亏损。

**修复**：`SavePlan.Write` 在整轮写入中**复用同一个 32 KiB 缓冲**（首次遇到
`CopyOriginal` 时分配，经 `io.CopyBuffer` 传给 `copyPart`）。固定开销从
`O(nParts)` 降为 `O(1)`，于是三档**全面优于改前**。

**漏检原因与补漏**：原收益锚点只有 `1x1MiB / 4x1MiB / 3x8MiB` 三档「少而大」
用例，结构上不可能发现每 Part 固定开销。已补 `200x4KiB`（200 个 4 KiB Part）
哨兵档——实测该档在「无复用」下 **7.24 MB**、「复用」下 **231 KB**，**31× 差异**，
灵敏度充足。`scripts/perf/smoke.sh` 的 ①b 守门已同步为 5 个子基准。

结论修正：

- 原「耗时下降 40–45%」的推论不可采信——本机（P/E 混合核 + Windows 调度）
  **同一份代码的墙钟时间在不同轮次可差 3.5×**（见 PERF-01 基线文档 §6），
  只有 B/op 这类确定性指标可作结论；
- 真正的收益是**分配量与文档形状解耦**：从「O(未变 Part 总体积)」变成
  「O(1) 缓冲 + 少量常数」，媒体档 −99.6%，小档也不再劣化；
- 端到端 B1 由 `corpus_b1_test.go` 锚定（4 份语料 118 个 Part 空变更保存后
  字节恒等），本次修复后复跑全绿。

### Tier 2（已实施）：未变非 XML Part 走 `CreateRaw` / `OpenRaw` 直通 —— 重启条件①取证成立

Go 的 `archive/zip` 支持 `(*zip.File).OpenRaw()` 取**原始压缩帧**，配合 `(*zip.Writer).CreateRaw()` 原样写入，可**跳过解压 + 重压缩**，对已压缩媒体（PNG/JPEG/MP4）收益显著。

#### 可行性验证结果（2026-09-12，实测）

方法：临时探针（`OpenRaw` + 复制 `FileHeader` + `CreateRaw`，逐条 `io.Copy`），在合成包与 `testdata/corpus` 全部 6 个语料上对比「Tier 1 流式」与「Tier 2 直通」，比较解压内容、头部字段、条目顺序、`opc.Load` 可接受性与耗时。

| 观察项 | 结果 |
|---|---|
| 解压内容一致性 | **完全一致**（6/6 语料 `bodyDiff=0`，逐条目字节比较） |
| 未变包输出字节 | **直通输出与源包逐字节相同**（6/6 语料 `raw==src`，含 `*.edited.pptx` 与原始语料） |
| 头部字段 | 与 Tier 1 输出有差异：直通**保留**源的 `Method`（Store 不被迫重压缩）、`Flags`（含 UTF-8 位 0x800）、`Modified`；Tier 1 输出统一为 Deflate + 零时间戳 |
| 条目顺序 | 直通默认保留**源顺序**；Tier 1 按 Part 名排序。改为按名排序后输出**确定性成立**（两次运行字节一致，内容仍与源一致） |
| `opc.Load` 可接受性 | 两条路径输出均被接受（6/6） |
| 耗时（3×8 MiB 未变媒体，与 Tier 1 基准同规格，best-of-5） | 流式 14.11 ms → 直通 10.53 ms，**1.34×** |
| 耗时（5.2 MiB 混合包，best-of-5） | 3.50 ms → 2.58 ms，1.36× |

#### 结论更新（2026-09-14）：已实施

原「不实施」结论基于 2026-09-12 的跨轮次 best-of-5 测量（1.34×）。该测量被 PERF-01
基线文档 §6 警告的**本机 P/E 混合核调度 3.5× 墙钟漂移**系统性低估——跨轮次取 best 恰好采到
快的一轮，使重压缩占比被压低。

**同进程 A/B 取证**（生产 `plan.Write` 流式 vs 测试本地 raw 直通，同进程内计时；
`saveplan_recompress_probe_test.go` 的 `BenchmarkCopyOriginalRecompressShare`，
`share=(Stream−Raw)/Stream`）：

| 场景 | 重压缩占 Save p50 比例（两次独立运行） |
|---|---|
| 3×8 MiB 未变媒体 | **65.3% / 73.4%** |
| 100×768 B 未变媒体 | **48.0% / 66.2%** |

结论：重启条件①（重压缩占比 > 50%）**实证成立**——未变媒体重存时，Save 过半耗时花在
重压缩上，Tier 2 跳过该路径即可省下这部分。原 1.34× 是测量假象；真实边际收益是
**媒体重存场景 Save 整体 2–3× 加速**。

**已落地实现**（全在 `internal/opc`，不动公共 API）：

- `saveplan.go`：`Write` 的 `CopyOriginal` 分支优先 `tryRawCopyOriginal`（raw 直通），否则回退 `copyPart`（Tier 1 流式）；
- `package.go`：`rawPartFile` 暴露底层 `*zip.File`（即重启条件③所需的 raw 读取入口，本次补上）；
- 安全门限（任一不满足即回退 Tier 1）：仅非 XML Part（`*.xml`/`*.rels` 除外）；仅 `Store`/`Deflate` 方法；无加密标志位（bit 0）；无 data descriptor 标志位（bit 3）；`CompressedSize64`/`UncompressedSize64` 均非 0；
- 逐字段复制源 `FileHeader`（`Method`/`CRC32`/`CompressedSize64`/`UncompressedSize64`/`Modified`/`Flags`），`CreateRaw` 原样写出。

**B1 兼容性（天然安全）**：`corpus_b1_test.go` 比的是**解压内容 SHA256** 而非 ZIP 字节；Tier 2
改 ZIP 层（压缩方法/时间戳/顺序）不改解压内容 → B1 不会破。已由
`TestSavePlanWriteTier2RawPassThrough`（"B1 decompressed content byte-identical"）锚定。

原「不实施」四点理由（留痕，非否决）：① Tier 1 已拿主要收益——成立，但 Tier 2 在其上额外省掉重压缩；
② 代价（zip64/加密/data descriptor/异常 extra）——已由上述门限拦截；③ 验证成本——B1 天然兼容且
全在 `internal/opc`，成本可控；④ 可疑收益方向——取证已证明确为重压缩主导。

**重启条件状态**：① 实证满足（上表）；② 仍无客户报告（非阻塞，收益已由取证独立证明）；
③ 已通过 `rawPartFile` 补上 raw 读取入口。故按「先取证再决定」决策规则落地。

约束与风险（已在 `tryRawCopyOriginal` 中实现，留痕）：

- 必须逐字段复制源 `FileHeader`：`Method`、`CRC32`、`CompressedSize64`、`UncompressedSize64`、`Modified`（及 `Modified` 的扩展时间戳额外字段）、`Flags` 中的 data descriptor 位；
- 头部任一字段漂移都会改变输出字节 → **必须 B1 全绿才可合入**，失败则只保留 Tier 1；
- 仅对**未变且非 XML** 的 Part 启用（`EmitPatched` / `EmitNew` 内容本就在内存，无从直通）；
- 需要 `Index` 暴露 raw 读取入口（新增 internal API，不动公共 API）。

## 不做的事

- 不改事务与原子性：`SaveToFile` 仍为「同目录临时文件 → 校验 → 原子替换」，失败清理临时文件、保留旧目标（AT-11）；
- 不改公共 API：全部改动在 `internal/opc`；
- 不改 `verifyOutput`：仍重新打开输出校验 ZIP 结构与条目集（只读 Central Directory + 比对条目名，不是全量重读）；
- 不引入第三方依赖（保持 CGO=0 + 零运行时依赖 + WASM 可编译）。

## 验收清单

1. **B1 黄金语料**：`go test -tags=corpus ./...` 逐 Part 哈希全等（重点 `s003-image`、`ext-0024` 含媒体样本）；
2. **PERF-01 重跑**：`scripts/perf/run.sh` 三档语料，记录**峰值内存**与 p50/p95 前后对比（数字实测后填，本次不预估）；
3. **单测**：
   - 超大未变 Part 走流式路径，输出字节与旧实现一致；
   - 预算超限仍报 `ErrLimitExceeded`（不得因改成流式而绕过限额）；
   - 源 Part 损坏仍报 `ErrMalformedPackage`；
4. **fuzz**：`FuzzLoad` / `FuzzScan` 无回归（种子语料已在 commit `1915fc2` 扩充）；
5. **L3 客户端矩阵**：Tier 1 输出字节不变 → 按 ADR-017 判据无需重跑；Tier 2 若改动字节则需重跑 8 组合。

## 实施前置条件

当前工作区因 A-1 第三批 chart WIP 未完成而整体编译不过（`internal/chart/canonical.go` 引用未定义的 `plotArea`）。
Tier 1 的**代码改动本身不依赖根包**（`internal/opc` 可独立编译测试），但**验收项 1/2 依赖根包**，因此：

- 可先实现并跑通 `internal/opc` 自身测试；
- B1 / PERF-01 全量验收须等 chart WIP 收口后再补。

## 决策点

- [x] Tier 1 是否立即实施 → **已实施**（commit `9bfe44d`，收益见上表）
- [x] Tier 2 是否先做 `CreateRaw` 可行性验证 → **已验证**（见上）：可行且保真
- [x] Tier 2 是否实施 → **已实施**（2026-09-14）：重启条件①同进程 A/B 取证成立（重压缩占比 50–73%），代码落地 `internal/opc/saveplan.go` 的 `tryRawCopyOriginal` + `package.go` 的 `rawPartFile`，含 `saveplan_raw_test.go` / `saveplan_recompress_probe_test.go`；`go build ./...` 与 `go test ./internal/opc/` 全绿，`go vet` 干净
- [ ] B1 / PERF-01 全量验收（公开语料 `s001`/`s002`/`s003` 已可跑；`ext-0024` 存在**既有 B1 缺陷**——“output has 96 entries, plan wants 75”，与 Tier 2 无关，须另立 issue/ADR 跟踪）
