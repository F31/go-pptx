---
title: "FEAT-002 项 3 图表坐标轴单位/数字格式 需求确认单"
date: 2026-09-12
linked_ppts_doc: §7/FEAT-002 项 3（图表单位）
go_pptx_commit: 本批次（v1.0.4+，ADR-019 设计空间 + ADR-020 降置信子项已实施）
status: NEW → 待 ppts 按 §4 回填需求
---

# FEAT-002 (ppts §7) — 项 3 图表单位：需求确认单

> 本文件是 go-pptx 仓库发给 ppts 方的**需求确认单**（对应 ADR-019 §4 模板）。
> 目的：在盲定字段前，先与 ppts 对齐"到底要哪些轴单位/格式字段、怎么消费"，避免过早承诺
> Stable 表面或语义错配。

## 背景（go-pptx 端现状）

| 维度 | 现状 |
|---|---|
| 轴对象模型 | `ChartAxisOptions`（`internal/chart/types.go:207`，R 档非 Stable）仅含 `CategoryAsDate / ValueLogBase / Min / Max / Position` |
| 提取能力 | `ParseChartAxes`（`parse.go:223`）只读上述 5 字段，静默丢弃 `majorUnit / minorUnit / numFmt / tickMark / gridlines / dispUnits / tickLblPos` 等轴语义字段 |
| 公共 API | 根包未暴露 `ChartAxisOptions`（仅 `ChartShape` 为 Stable），加字段当前不触发 CI 门禁 |

## 已上线 interim：降置信诊断（ADR-020，v1.0.4+ 可用）

不扩张 API、零字节变化，ppts 立即可用：

- `ChartShape.DataWithDiagnostics() (ChartData, []Diagnostic, error)`：数据快照与 `Data()` 一致；
  当源图表轴含未提取字段时，返回 `Diagnostic{Code: "chart.axis.unread", Severity: SeverityInfo}`，
  并列出具体未提取字段名（如 `majorUnit, numFmt`）。
- `ir.Shape.Diagnostics`（`json:"diagnostics,omitempty"`）：导出 JSON 时携带该诊断，ppts 可在
  读取 IR 后据此整体降低对该图表轴单位/刻度的置信度。

> 这意味着：ppts 现在就能"感知轴信息不完整"并标注"单位可能不准确"，无需等到精确字段落地。

## 设计空间（详见 ADR-019）

- **方案 A**：扩展 `ChartAxisOptions` 补全字段（信息最完整，但过早承诺 Stable）。
- **方案 B**：诊断驱动（已实施，见上）。
- **方案 C**：新增 `ChartAxis` 富类型（语义清晰、可扩展，但 API 表面最大，需 ADR-015 升档评审）。

go-pptx 建议两阶段：**B 先行（已上线）+ A/C 待定**，待下方 §4 回填后钉死。

## §4 ppts 需求回填模板（请填写发回）

> ppts 方请填写以下信息，作为 ADR-019 阶段二（精确字段）的输入：

1. **需要的具体字段集**（勾选/补充）：
   - [ ] `MajorUnit`（主刻度单位）
   - [ ] `MinorUnit`（次刻度单位）
   - [ ] `NumFmt` —— formatCode 原文（如 `0.0%` / `#,##0`）
   - [ ] `NumFmt` —— 归一化描述（如"百分比，1 位小数"）
   - [ ] `TickMark`（主/次刻度标记）
   - [ ] `Gridlines`（主/次网格线）
   - [ ] `DispUnits`（显示单位）
   - [ ] `TickLblPos`（刻度标签位置）

2. **numFmt 消费方式**：
   - [ ] 原样 formatCode 字符串用于下游渲染
   - [ ] 归一化为"单位 + 小数位"描述用于文案
   - [ ] 两者都要

3. **单位语义**：
   - [ ] 轴单位即系列值单位
   - [ ] 轴单位需独立于系列值单位单独呈现

4. **是否接受先以诊断降级（方案 B）上线、后续再补精确字段（方案 A/C）**：
   - [ ] 接受
   - [ ] 不接受（需阶段二同期交付）

5. **期望集成版本**：____（v1.1.0 / v1.2.0）

## ppts 端要求

1. 回填上述 §4 并发回本仓库（或直接在 ppts 跟踪计划文档批注）。
2.  interim 阶段：集成 `ChartShape.DataWithDiagnostics` / `ir.Shape.Diagnostics`，对
   `chart.axis.unread` 诊断做降置信处理（文案标注"单位可能不准确"）。
3. 阶段二启动后，参与 ADR-019 阶段二的字段集与 Stable 承诺评审。

## 兼容性

- 阶段二任何落地都必须：同步 `api_surface_test.go` golden 清单；保持 `Chart.Data()` /
  `ChartShape.Data()` 签名不变（binary-compat）；不静默丢弃已提取字段。
