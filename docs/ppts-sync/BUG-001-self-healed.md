---
title: "BUG-001 自愈通知"
date: 2026-09-22
linked_ppts_doc: §7/BUG-001
go_pptx_commit: c47b7a3（HEAD 当前基底）+ 后续 v1.0.3 commit（wait）
---

# BUG-001 (ppts §7) — 自愈通知

## ppts 报告原文节录

> #### 描述
> go-pptx 本地产库（`/mnt/e/projects/go-pptx`，ADR-017 重构进行中，改动未提交）中，
> `validateChartData` 被**同时定义两次**：
> - `chartfrag.go:37`：新薄包装，委托 `chartinternal.ValidateChartData`（预期保留）
> - `chart.go:~331`：旧完整实现（预期应删除但残留）
>
> 现象：`go build ./...` 报 `validateChartData redeclared in this block`。
>
> #### 复现
> - 最小样例：`cd /mnt/e/projects/go-pptx && go build .`
> - 实际行为：`chartfrag.go:33:6: validateChartData redecladed in this block`（`../go-pptx/chart.go:331:6` 处另一声明）
> - 环境：go-pptx 本地产库当前 working tree（3537adf 之后未提交改动）

## 自愈证据（go-pptx HEAD c47b7a3）

```
$ rg "func validateChartData" --glob '*.go'
chartfrag.go:22:func validateChartData(cd ChartData, op string) error {

（仅 1 处声明）
```

`chart.go` 内已无 `func validateChartData` 定义，调用点（`chart.go:218` / `chart.go:413` / `bind.go:826`）全部委托到 `chartfrag.go:22` 的薄包装。

## 自愈路径（提交链）

1. **ADR-017 r1**（commit `46c2f2d`）：第一批真零依赖函数搬迁 + 4 个常量搬迁，**未触动 validateChartData**（保留为根包私有 facade）。
2. **ADR-017 r2**（commits `ce3f66c` / `bc533d3`）：第二批 11 个值对象通过 type alias 方式 move（含 `font.go` 的 `Optional[T]`），**仍保留 validateChartData facade**。
3. **ADR-017 r3**（commit `a3abfca`）：第三批 parse/build/canonical/validate/fragment/workbook 实现**全搬迁到 internal/chart**——其中 **validate 函数**（含 `validateChartData`）搬到 `internal/chart.ValidateChartData`。`chart.go` 内的旧实现**已删除**（解决 r3 前的双声明），根包 facade 仍有调用方（`chart.go:218/413` + `bind.go:826`）所以保留。
4. **commit `5171dbc`**：ADR-017 后续收尾（归并、覆盖率恢复）。
5. **commit `c08249d`**：补 `TestNoBuildConstraintsInRootPackage` 断言根包禁止构建约束，确保守门测的是单一跨平台 API 表面。
6. **commit `20b12a4`** / `b2cac60`：v1.0 冻结清单 AST 守门 + 根包禁止构建约束——两者都在 CI 上拒收 `validateChartData` 双声明**任何回归**。

## 质量门禁（自愈后实测，2026-09-22 22:0x）

| 门禁 | 命令 | 结果 |
|---|---|---|
| 编译 | `go build ./...` | PASS（0 警告 / 0 错误）|
| 静态检查 | `go vet ./...` | PASS（0 警告） |
| 单元测试 | `go test ./...` | PASS（14/14 包 ok）|
| 语料测试 | `go test -tags=corpus ./...` | PASS（14/14 包 ok）|
| WASM 构建 | `CGO_ENABLED=0 GOOS=js GOARCH=wasm go build ./...` | PASS |
| darwin arm64 构建 | `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...` | PASS |
| wasip1 wasm 构建 | `CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm go build ./...` | PASS |
| linux arm64 构建 | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...` | PASS |
| API 表面守门 | `go test -run 'TestAPIFrozen' .` | PASS（5/5） |

## ppts 端要求

按 ppts §6 "验证闭环"：

1. ✅ code-buddy 工作完成（自愈，已就绪）；
2. ppts 端把 `GOWORK=on`（本地开发模式）切回可工作模式；
3. ppts 端跑全量构建 + 跑 `internal/project` 与 `internal/integrations` 契约测试，所有断言通过；
4. ppts 端把该 BUG-001 条目标记为 **CLOSED**，备注 "go-pptx v1.0.2 已自愈（ADR-017 r3）"，不再要求 ppts 侧规避 `GOWORK=off`。

## go-pptx 仓库侧

无 commit 改动（自愈已发生于历史 commit），但本应答文件登记在 `docs/ppts-sync/BUG-001-self-healed.md`，随 v1.0.3 patch release 一同发布。

## 历史追溯

- 报告时点：2026-09-12（ppts V1.0 文档登记）
- 自愈时点：2026-09-12 ADR-017 r3 落地（commit `a3abfca`）+ 后续 patch 链路
- 应答登记：2026-09-22（本文件）
- 关联 commit：`a3abfca`（r3 实现全搬迁）、`c08249d`（根包禁止构建约束）、`20b12a4`（冻结守门）
