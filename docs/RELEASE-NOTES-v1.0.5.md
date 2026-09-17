# go-pptx v1.0.5 Patch Release Notes

> 2026-09-16 · 随 `git tag -s v1.0.5`（SSH 签名）发布
>
> **v1.0.4 后的第五个 patch release，也是 v1.0.1 以来首个含生产代码改动的 patch**（v1.0.1–v1.0.4 均为测试/文档增量）。v1.0.4 → v1.0.5 共 **12 个 commit / 39 文件（+2291 / −109）**。公共 API 表面**只增不改**，**binary-compat with v1.0.0 / v1.0.1 / v1.0.2 / v1.0.3 / v1.0.4**。

## 版本定位

本版把在此之前已落在 `main` 上、但尚未进入任何 tag 的三条工作线一并发布：

1. **Save 性能 —— ADR-018 Tier 2 原始帧直通**：未变非 XML Part 走 `OpenRaw` + `CreateRaw`，跳过解压与重压缩。这是 v1.0 以来 Save 路径最大的一次性能改动。
2. **API 表面预备 —— A-2 形状能力窄接口（ADR-021）+ WASM API GA 化（ADR-023）**：新增 5 个 Stable 窄接口，并把 5 个 `// Experimental:` 段升为 `// Stable:`。
3. **Tier 2 的两处同源缺陷修复 —— ADR-024 + COV-04 覆盖率门槛回归**：Tier 2 落地时验证不完整，留下一个高危产物损坏缺陷与一个静默的覆盖率门槛回归，两者在本版修复合一。

> **关于版本号**：本版含 API 表面**新增**（5 个 Stable 接口 + 2 个方法/字段）与稳定性**升档**（Experimental → Stable）。两者都不破坏兼容性，故沿用本项目"binary-compat 即 patch"的既有口径（v1.0.3 亦曾以 patch 形式追加 2 个 Stable 方法）。
>
> **关于范围**：A-2（`15137ca`）与 ADR-020 的 commit 落在 **v1.0.4 tag 之后**，因此它们**不在 v1.0.4 里**——`v1.0.4 = 零生产代码改动` 的声明仍然成立；`docs/v1.0-bug-registry.md` §1.4 标题中的"v1.0.3 / v1.0.4 期间"是**时间口径**（09-12/13 两日），不是版本口径。

## 关键不变量（v1.0.0 → v1.0.5 全程）

| 项 | v1.0.0 | v1.0.1 | v1.0.2 | v1.0.3 | v1.0.4 | **v1.0.5** | 结论 |
|---|---:|---:|---:|---:|---:|---:|---|
| `// Stable:` 段落数 | 34 | 34 | 34 | 34 | 34 | **40** | 只增（A-2 +1 段、D-5 +5 段） |
| Stable 符号数 | 50 | 50 | 50 | 50 | 50 | **60** | 只增（A-2 50→55、D-5 55→60） |
| `// Experimental:` 段落数 | 5 | 5 | 5 | 5 | 5 | **0** | 全部升 Stable（ADR-023） |
| 公共 API type 总数 | 158 | 158 | 158 | 158 | 158 | **163** | 只增（A-2 5 接口 + D-5 0 新 type） |
| Stable 方法数 | 127 | 127 | 127 | 129 | 129 | **131** | 只增（v1.0.3 +2、本版 +2） |
| 错误哨兵语义 | 锁死 | 锁死 | 锁死 | 锁死 | 锁死 | 锁死 | ✅ 不变 |
| 黄金语料 B1 哈希 | PASS | PASS | PASS | PASS | PASS | PASS | ✅ 不变 |

> `api_surface_test.go` 的 7 个 AST 断言全程把守；本版的计数变化是**有意**的 golden 名单更新（已过 ADR-021 / ADR-023 评审），任何**意外**漂移仍会在 `go test` 阶段失败。

## Added (API)

- **A-2 形状能力窄接口（ADR-021）** —— `GeometryProvider` / `FillProvider` / `EffectsProvider` / `StyleMatrixRefsProvider` / `LineProvider`（5 个 `// Stable:` 接口）+ 8 条形状编译期断言。既有 `Shape` 接口 getter 全部保留，新调用方可按需组合窄接口断言，避免为读一个几何属性而依赖整个 `Shape` 表面。
- **FEAT-002 项 3 降置信诊断（ADR-020）** —— `ChartShape.DataWithDiagnostics` + `internal/chart.ChartAxisUnreadFieldNames` + `ir.Shape.Diagnostics`。图表轴单位读不到显式 `c:numFmt` 时给出可解释的降置信标记，而非静默取默认值。

## Performance

- **ADR-018 Tier 2**：未变非 XML Part 原样搬运源压缩帧。同进程 A/B 取证显示未变媒体重压缩占 Save p50 的 **50–73%**（3×8 MiB 65.3% / 73.4%、100×768 48.0% / 66.2%）。
  - 安全门限（不满足即自动退回 Tier 1 流式）：仅非 XML、仅 `Store` / `Deflate`、无加密位(bit0)、无 data-descriptor 位(bit3)、声明尺寸非零。
  - B1 语义保持：解压内容逐字节不变；**OPC Part 视角等价性不变**，但输出 ZIP 的字节布局与 v1.0.4 不同（压缩帧保留 + 条目顺序可能不同）。

## Fixed

- **`SavePlan.Write` 重复条目（高危，ADR-024）**：Tier 2 的 raw 直通路径与循环头的 `zw.Create` 双重注册同名条目，导致**任何含未变非 XML Part 的文档**（绝大多数真实 PPTX）`Save` / `SaveToFile` 产出重复条目并失败（`output has 96 entries, plan wants 75` / `duplicate entry`）。修复方式是让 `zw.Create` 只出现在真正需要的分支，**不放宽** `verifyOutput` 校验、**不放宽** raw 安全门限。
- **`internal/opc` 覆盖率门槛回归**：Tier 2 新增的 4 条安全门限拒绝分支落地时零测试覆盖，把包覆盖率从 90.4% 静默拉到 89.5%（跌破 COV-04 门槛，三日无人发现）。本版补齐 `saveplan_raw_guard_test.go`，覆盖率恢复至 **90.3% / 90.5%（corpus）**。
- **测试盲区修补（4 层）**：`map[name][]byte` 收集条目致同名覆盖、合成包未断言条目数、私有语料在 CI 恒 Skip、`Write` 单测自建 `zip.NewReader` 不校验重复——全部以"逐条计数不经 map"的新守门替代。

## L3 真机客户端矩阵（第二轮，Tier 2 产物）

Tier 2 改变了输出字节，故按 ADR-018 验收清单第 5 项**重跑 8 组合**（用本版代码重新生成的编辑后产物）：

| Client | 版本 | 结果 |
|---|---|---|
| PowerPoint | 16.0.20326 | 4 样本 × 无修复提示打开 + 重存成功；重存文件 `Validate` errorCount=0 |
| WPS 演示 | 12.1.0.28599 | 同上 |

**8/8 通过**（总耗时 13.3s）。这是 Tier 2 安全门限在真机上的端到端背书——raw 直通保留的原始压缩帧被两家客户端正常接受。详见 `docs/client-compat-matrix.md`。

## Verification（如何验证）

```bash
go test ./...                                     # 14/14 包 PASS
go test -tags=corpus ./...                        # 含 B1 金样比对
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v .   # 7/7 AST 守门

CGO_ENABLED=0 go vet ./...                        # 零警告
CGO_ENABLED=0 go vet -tags=corpus ./...           # 零警告
gofmt -l .                                        # 零输出
CGO_ENABLED=0 GOOS=js     GOARCH=wasm go build ./...
CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm go build ./...
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build ./...

# 覆盖率复核（应得 internal/opc 90.3%，root 合并口径 84.4%）
go test ./... -cover
```

## Compatibility

- **v1.0.4 → v1.0.5：binary-compatible + API-compatible（只增不改）**——新增 5 个 Stable 窄接口与 2 个方法/字段，既有签名与字段零变更；5 个 Experimental 段升 Stable 属**承诺增强**。
- 下游升级动作：**可选**。使用 WASM 嵌入 API 的项目可获得稳定契约（消除 1.x 内静默变更风险）；其余调用方行为完全一致。
- ⚠️ **输出字节布局变化**：Tier 2 后 `SavePlan.Write` 对未变媒体 Part 保留源压缩帧与 Extra 字段，不再重新 Deflate——**解压内容逐字节一致**，但做过输出 ZIP 逐字节比对的下游需改用 OPC Part 视角（PartNames + 内容哈希）比对。

## 已知限制（本版未闭合）

- **音频真机播放验证**：V2.6 §15.3 第 3 条后半"配音须有播放记录"仍未闭合——语料中缺少带 audio 标签的样本，L3 从未在真机上验证音频播放（AUDIO-01/02/03 有代码级测试）。这是**唯一**的发布级硬缺口，属环境型限制。
- 覆盖率的其余缺口按既有决策保留：`audioprobe` 88.4%（B-2 决策不追，剩余为不可达防御代码）；root per-package 82.8%→85% 目标延后到 1.x（边际收益递减）。

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 本版适用场景：所有使用 Save 路径的下游（性能收益 + 高危缺陷修复），以及依赖 WASM 嵌入 API 的项目（获得稳定契约）。
