---
title: "go-pptx 仓库对 ppts 项目跟踪计划 V1.0 §7 的实施记录"
version: v1.0.0
date: 2026-09-22
status: Active
---

# ppts 跟踪计划 V1.0 §7 实施应答

> 本目录是 go-pptx 仓库对 ppts 项目侧《go-pptx 特性与 bug 跟踪计划》V1.0（文件路径：`E:\projects\ppts\docs\go-pptx特性与bug跟踪计划.md`，原文版本 2026-09-12）的**实施应答 + 同步文本**。
>
> 用法：ppts 开发读到本目录时按 ppts §6.验证闭环 走契约测试；Go-pptx 仓库维护方读本目录时按"提交前"映射避免单向漂移。

---

## 索引

| ppts §7 条目 | go-pptx 应答文件 | go-pptx commit | ppts 验收状态 |
|---|---|---|---|
| §7/BUG-001 `validateChartData` 双声明 | [`BUG-001-self-healed.md`](BUG-001-self-healed.md) | c47b7a3（已存于 HEAD） | **CLOSED（自愈），无须代码改动** |
| §7/FEAT-001 带音频 PPTX 配音完整化 | （未实施；ppts 端 G0-8 真机门禁）| — | NEW；代码侧已闭环，ppts 端做真机门禁 |
| §7/FEAT-002 讲稿提取增强 | [`FEAT-002-notes-filtering-contract.md`](FEAT-002-notes-filtering-contract.md)（项 1：仅文档注记）| （本批次，wait commit） | NEW；代码行为已生效，本版仅契约文档化 |
| §7/FEAT-002 图表单位 | 未实施 | — | NEW；需新 ADR-019 评审 |
| §7/FEAT-002 阅读顺序 | 未实施 | — | NEW；需新 ADR 评审 |
| §7/FEAT-003 隐藏页报告 + 页序读取 | [`FEAT-003-hidden-advtm-read.md`](FEAT-003-hidden-advtm-read.md) | （本批次，wait commit） | **NEW → 待 ppts 端契约验证** |

---

## 同步文本（直接复制给 ppts 方）

> ## go-pptx 仓库应答同步（2026-09-22）
>
> 收到贵方的《go-pptx 特性与bug跟踪计划》V1.0 §7，按 status=NEW/P0 顺序处理完毕。详细应答见 [`docs/ppts-sync/`](https://github.com/F31/go-pptx/tree/main/docs/ppts-sync/) 目录：
>
> ### BUG-001 已自愈（无须改动）
> **条目 §7/BUG-001 `validateChartData` 双声明、阻塞本地全量构建**——在 go-pptx HEAD `c47b7a3` 已自愈。该问题源于 2026-09-12 之前的 chart 重构中间态，**ADR-017 r3（commit `a3abfca` + 后续 33 个 commit）** 删除旧实现 + 后续 patch (commit `c08249d`) 加 `TestNoBuildConstraintsInRootPackage` 都已根除相关残留。当前：
>
> - `rg "func validateChartData" --glob '*.go'` 仅 1 处（`chartfrag.go:22`）
> - `go build ./...` 全绿
> - `go test ./...` 14/14 包 ok
> - `go test -tags=corpus ./...` 14/14 包 ok
>
> 你们在 ppts 侧可直接把 BUG-001 标 **CLOSED（自愈）**，无须再做规避（`GOWORK=off` 等）。
>
> ### FEAT-003 完整实施（待 ppts 端 CONTRACT 验证）
> **§7/FEAT-003 隐藏页报告 + `p:transition@advTm` 读侧**——已在 go-pptx 仓库内实施完毕，3 个新增公共 API 全部 binary-compat with v1.0.0 / v1.0.1 / v1.0.2：
>
> | API | 读侧语义 | 配套约定 |
> |---|---|---|
> | `Slide.Hidden() (bool, error)` | `p:sldId@show="0"` → true；其他（含缺省）→ false | 与 `Slides()` 严格成对，活页才能读 |
> | `Slide.AdvanceAfter() (time.Duration, bool, error)` | 第二个返回值 `ok=true` 表示显式 `advTm` 设定 | 与 `SetAdvanceAfter` 写入对偶 |
> | `ir.Page.Hidden *bool` + `ir.Options.IncludeHidden`（默认 true）| 三态字段（`nil`/`&false`/`&true`）| ppts 默认过滤器可据此跳过隐藏页 |
>
> 详细测试与验证见 `docs/ppts-sync/FEAT-003-hidden-advtm-read.md`。请 ppts 端按 `internal/project` 读路径适配器契约测试 + `internal/integrations` 写路径做适配。
>
> ### FEAT-002 项 1 仅文档注记（无代码改动）
> **§7/FEAT-002 备注过滤**——`SpeakerNotesText` 实现已正确过滤非 body 占位符（`notesPhType` 严格只匹配 `type=body` 或缺省），本次仅把契约注记落到 `notes.go` 顶部，无任何代码改动。详细见 `docs/ppts-sync/FEAT-002-notes-filtering-contract.md`。
>
> ### 暂未实施的 FEAT-002 子项（需新 ADR 评审）
> §7/FEAT-002 项 2（阅读顺序建议）与项 3（图表嵌入数据/单位）涉及公共 API 表面扩张：
> - **项 3** 触及 `ChartAxisOptions` 字段扩张 → 需要新 **ADR-019** 评审；
> - **项 2** 触及新 `ReadingOrder` API + IR 扩展 → 需要新 **ADR-020** 评审；
> - **FEAT-001** 真机门禁非 go-pptx 代码工作，归 ppts 端 PowerPoint/WPS 安装后跑 G0-8 真实播放验证。
>
> 贵方如需在这些未实施条目上启动 ADR，请把 §4 模板填写完发回本仓库做 IN_PROGRESS。
>
> ## 提交列表（本批次）
>
> 1 个 commit（汇总）：
> - 测试与守门更新：`api_surface_test.go` goldenStableMethods 由 127 → 129，`wantStableMethods` 同步更新
> - 新公开方法 + 测试 + IR 字段 + 注记 + 文档
>
> HEAD 仍为 `c47b7a3` 之后的新 commit；`git tag v1.0.3` 待 ppts 端 VERIFIED 后补打。
