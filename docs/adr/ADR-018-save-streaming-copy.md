# ADR-018: Save 未变 Part 流式复制（去全缓冲 + 可选原始帧直通）

- **状态**: Proposed（**Tier 1 已实现，待全量验收**；Tier 2 建议先做可行性验证）
- **Tier 1 进度**: 代码已落地 `internal/opc/saveplan.go`（`copyPart`）+ `saveplan_stream_test.go`（大 Part 字节一致 / 幂等 / 两类错误分支）；`internal/opc` 自身测试与 `-tags=corpus` 全绿、覆盖率 90.3%（守 90% 门槛）。**B1 全量语料与 PERF-01 峰值内存对比须等根包编译恢复（A-1 第三批 WIP 阻塞）后补做**
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

收益：**峰值内存 O(最大 Part) → O(32 KiB 缓冲)**。耗时预期基本持平（省一次大对象分配与 GC，多一次 32 KiB 缓冲拷贝）。

### Tier 2（中风险，先验证）：未变非 XML Part 走 `CreateRaw` / `OpenRaw` 直通

Go 的 `archive/zip` 支持 `(*zip.File).OpenRaw()` 取**原始压缩帧**，配合 `(*zip.Writer).CreateRaw()` 原样写入，可**跳过解压 + 重压缩**，对已压缩媒体（PNG/JPEG/MP4）收益显著。

约束与风险：

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

- [ ] 是否批准 Tier 1 立即实施（建议：是）
- [ ] 是否批准 Tier 2 先做 `CreateRaw` 可行性验证（建议：先验证，输出字节不变才继续）
