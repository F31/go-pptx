---
title: "FEAT-002 项 1 备注过滤契约注记"
date: 2026-09-22
linked_ppts_doc: §7/FEAT-002
go_pptx_commit: 待 commit（wait）— v1.0.3 patch
---

# FEAT-002 (ppts §7) — 项 1 备注过滤仅文档注记

## ppts 报告原文节录

> #### 描述
> 1. 备注正文提取时过滤备注母版页脚、页码等非讲稿内容
> 2. 阅读顺序辅助：用坐标、组结构和文本语义生成建议顺序（对象顺序不必然等于阅读顺序）
> 3. 图表优先读取嵌入数据与坐标轴/单位；无法可靠提取时降低置信度
>
> #### 验收标准
> - [ ] 备注页脚/页码/母版模板内容不进讲稿正文

## go-pptx 端评估

按 2026-09-22 HEAD `c47b7a3` 实测：

| ppts §验收点 | go-pptx 现状 | 是否仍需改动 |
|---|---|---|
| **备注页脚/页码不进讲稿正文** | ✅ **`SpeakerNotesText` 已隐式过滤**：`notesPhType`（notes.go:84-102）严格只匹配 `p:ph type="body"` 或缺省 type，自动跳过 `hdr` / `ftr` / `sldNum` / `dt` 等模板占位符 | ⚠️ 行为已生效，但**契约文档未显式说明**——本版仅补契约注记 |
| 阅读顺序辅助 | ❌ 完全无 helper | 需新 ADR（FEAT-002 项 2，未实施） |
| 图表嵌入数据 | ✅ `Chart.Data()` 走 `c:numCache/c:strCache` 完整反序列化到 `ChartData.Categories/Series.Values` | 已实施 |
| 图表单位 | ❌ `ChartAxisOptions` 仅含 `CategoryAsDate / ValueLogBase / Min / Max / Position` 五项，无 `MajorUnit / MinorUnit / numFmt` | 需新 ADR-019（FEAT-002 项 3，未实施） |
| 不可靠时降置信 | ⚠️ IR 已有 `Diagnostics` 机制但**未为 ChartAxis 未读字段单独发降置信** | 需新 ADR |

**本版仅处理项 1**：把"自动过滤非 body 占位符"的契约**写明到 notes.go 顶部注释**。代码行为早在 v1.0.0 即生效，本版无运行时改动。

## 实施变更

### `notes.go` 顶部补"隐式过滤契约"段

```go
// notes.go
//
// 隐式过滤契约（FEAT-002 项 1，2026-09-22 登记）：
//   - SpeakerNotesText / SpeakerNotes / SetSpeakerNotes 仅操作 p:ph type
//     为缺省或 "body" 的占位符（notesPhType 实现）；
//   - p:ph type="hdr" / "ftr" / "sldNum" / "dt" 等页眉/页码/日期占位符
//     不会进入讲稿正文——它们继承自 notesMaster，不在本库的合同面上
//     单独保留/修改。
//   - 调用方（ppts 自动讲解 / GenerateNarration）无需自行判断哪些占位符
//     是讲稿 vs 页脚，所有"用 SpeakerNotesText 抽取讲稿正文" 的用法
//     都自动得到正确结果。
```

### 测试（已存在）

- `notes_error_test.go` 已覆盖：
  - `TestNotesPartOf_NoNotesPart`（无 notes Part：返回空串）
  - `TestNotesPartOf_BodyPlaceholder`（只 body 占位符返回其文本）
  - `TestSpeakerNotesText_ClosedPresentation`（CLOSED → ErrClosed）
- 注：**未**存在单独测试 `TestSpeakerNotesText_IgnoresHeaderFooterPlaceholder`——若需补，可由 ppts 端契约测试一并验证（构造一个 notes Part 同时含 `p:ph type="body"` + `p:ph type="ftr"`，断言 SpeakerNotesText 只返回 body 文本）。

## ppts 端要求

1. **契约测试**（ppts 内部）：构造 fixture（notes Part 同时含 body 占位符 + ftr/sldNum 模板占位符），断言：
   ```go
   if got, _ := slide.SpeakerNotesText(); got != "讲稿文本" {
       t.Errorf("got %q, want %q", got, "讲稿文本")
   }
   ```
2. **验收通过**：ppts 端把 FEAT-002 项 1 标 **CLOSED（契约文档化）**。
3. **未实施子项**仍保持 NEW：
   - §7/FEAT-002 项 2（阅读顺序）：需新 ADR 评审
   - §7/FEAT-002 项 3（图表单位）：需新 ADR-019 评审

## 兼容性

- v1.0.3 = binary-compat with v1.0.0 / v1.0.1 / v1.0.2（**纯文档改动**，零代码语义变化）。
- 文档同步：CHANGELOG `[1.0.3] / Documented` 段。

## 已知边界

- 本版**未做**实现层"显式过滤"——v1.0 隐式行为已生效 1.0 余月，未发现任何"非 body 占位符被错误吸入"的实测报告（5 个 v1.0.x corpus 跑过全 validate）。
- 如果未来发现需要"显式白名单导出非 body 占位符文本"（如 ppts 想生成"扩展讲稿含 ftr 上下文"），应新增 API（如 `SpeakerNotesExtended()`）而非改动现有 `SpeakerNotesText` 语义（binary-compat 锁死）。
