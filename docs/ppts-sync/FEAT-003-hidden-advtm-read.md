---
title: "FEAT-003 读侧补全"
date: 2026-09-22
linked_ppts_doc: §7/FEAT-003
go_pptx_commit: 待 commit（wait）— v1.0.3 patch
---

# FEAT-003 (ppts §7) — 读侧补全实施记录

## ppts 报告原文节录

> #### 描述
> 1. 按 presentation 关系读取真实页序（已由 Inspect Supported 覆盖则仅做契约确认测试）
> 2. 隐藏页状态报告（供 ppts 默认跳过、可选纳入）
> 3. 保留原页 ID 与原页码映射
>
> #### 验收标准
> - [ ] 页序与 presentation.xml 关系顺序一致，非文件名排序
> - [ ] 隐藏页标记可读，ppts 可按策略跳过/纳入
> - [ ] 页 ID 与页码映射稳定可追踪

## go-pptx 端评估（G0 已覆盖 vs 真缺口）

按 2026-09-22 HEAD `c47b7a3` 实测：

| ppts §验收点 | go-pptx 现状 | 是否仍需改动 |
|---|---|---|
| **1 页序与 presentation.xml 关系顺序一致** | ✅ `Presentation.Slides() ([]*Slide, error)` 在 `presentation.go:179` 严格按 `p:sldIdLst` 顺序返回（注释明确"不按 id 排序，方案 §19.2"） | ❌ 零改动 |
| **2 隐藏页标记可读** | ❌ **真缺口**：`Slide.Hidden()` 公开方法不存在；`ir.Page.Hidden` 字段不存在 | ✅ 已新增 |
| **3 页 ID 与页码映射稳定可追踪** | ✅ `Slide.ID() SlideID` 已在 `slide.go:21`；`ir.Page.Index/SlideID/Part/Name` 四元组完整；`p:sldIdLst` 内的 r:id ↔ Part 由 `p:SldID@r:id ↔ presentation.xml.rels` 解析 | ❌ 零改动 |

**结论**：本版实现 §7/FEAT-003 项 2 读侧缺口，项 1、3 已隐式可用。

## 实施变更（v1.0.3 patch）

### 1. 新增 Stable 公开方法：`Slide.Hidden() (bool, error)`

```go
// go-pptx/slide.go（新增，标 Stable）
func (s *Slide) Hidden() (bool, error)
```

读取 `presentation.xml` 的 `p:sldIdLst/p:sldId[@id=slideID]/@show` 属性。OOXML 语义：

| `@show` 值 | Hidden 返回 |
|---|---|
| 缺省（属性不存在）| `false`（可见）|
| `""`（空字符串）| `false`（可见）|
| `"0"` | **`true`（被隐藏）** |
| 其他（`"1"`，虽不合法但存在）| `false` |

错误路径：

| 触发条件 | 返回 |
|---|---|
| 文档已关闭 | `ErrClosed` |
| 页面已删（句柄失效）| `ErrStaleHandle` |
| `presentationDoc()` 失败 | 包内 Annotation |

**binary-compat with v1.0.0/v1.0.1/v1.0.2**——仅追加只读公开方法。

### 2. 新增 Stable 公开方法：`Slide.AdvanceAfter() (time.Duration, bool, error)`

```go
// go-pptx/slide.go（新增，标 Stable）
func (s *Slide) AdvanceAfter() (time.Duration, bool, error)
```

读取 `p:slide/p:transition/@advTm` 毫秒值并转 `time.Duration`。第二个返回值 `ok` 区分"显式设定"与"未设"：

| 触发条件 | `d` | `ok` | 错误 |
|---|---|---|---|
| 无 `p:transition` 节点 | `0` | `false` | `nil` |
| 节点存在但 `advTm` 缺省/空字符串 | `0` | `false` | `nil` |
| `advTm` 是合法非负整数 `N` | `N*time.Millisecond` | **`true`** | `nil` |
| `advTm` 是负数或非整型 | `0` | `false` | `ErrMalformedPackage` |
| 句柄失效 | `0` | `false` | `ErrStaleHandle`/`ErrClosed` |

与 `SetAdvanceAfter(d time.Duration)` 写入对偶——读侧补齐后，ppts 等客户端无需自行 `xmlstore` 解析即可拿到自动切页时长。

**binary-compat with v1.0.0/v1.0.1/v1.0.2**——仅追加只读公开方法。

### 3. 新增 IR 字段 + Options：`Page.Hidden` 与 `Options.IncludeHidden`

```go
// go-pptx/ir/ir.go（新增，标 Experimental）
type Page struct {
    ...
    Hidden *bool `json:"hidden,omitempty"`  // 三态字段
    ...
}

type Options struct {
    ...
    IncludeHidden bool  // 默认 true
}
```

**Page.Hidden 三态语义**：

| `IncludeHidden` | OOXML 状态 | `Page.Hidden` |
|---|---|---|
| `true`（默认） | 页面 `show="0"` | `&true` |
| `true`（默认） | 页面 `show` 缺省/非 "0" | `&false` |
| `false`（opt-out） | 任意 | `nil`（不读取） |

ppts 端调用 `ir.FromPresentation(p, ir.DefaultOptions())` 后，过滤可见页可用：

```go
visible := slices.DeleteFunc(doc.Pages, func(p ir.Page) bool {
    return p.Hidden != nil && *p.Hidden
})
```

或者可换 "跳过隐藏页 + 保留可见页" 的策略。

**字段标 Experimental**——1.x 阶段若需要升 Stable，需新 ADR-019 评审 JSON 协议稳定性。

## 测试守门

### 新增测试

| 文件 | 测试函数 | 覆盖点 |
|---|---|---|
| `slide_test.go`（新建）| `TestSlide_Hidden_VisibleByDefault` | 缺省 → false |
| 同上 | `TestSlide_Hidden_AfterMark` | sldId@show="0" → true |
| 同上 | `TestSlide_Hidden_OnClosedPresentation` | 闭后 → ErrClosed |
| 同上 | `TestSlide_AdvanceAfter_AbsentByDefault` | 无 transition → (0, false, nil) |
| 同上 | `TestSlide_AdvanceAfter_AfterSet` | SetAdvanceAfter + AdvanceAfter 往返正确 |
| 同上 | `TestSlide_AdvanceAfter_OnClosedPresentation` | 闭后 → ErrClosed |
| `ir/ir_test.go` | `TestOptions_Defaults`（追加断言）| IncludeHidden 默认 true |
| 同上 | `TestFromPresentation_PageHiddenProjection/default_yields_pointer_false` | 默认 IncludeHidden=true 时 `Page.Hidden != nil && !*Hidden` |
| 同上 | `…/opt_out_yields_nil` | IncludeHidden=false 时 `Page.Hidden == nil` |

### 守门更新

- `api_surface_test.go::wantStableMethods`：`127 → 129`（+2 个 Stable 方法）
- `api_surface_test.go::goldenStableMethods`：新增 `"Slide.AdvanceAfter"` + `"Slide.Hidden"`

## ppts 端要求（按 §6 验证闭环）

1. **适配器契约测试**（ppts 内部）：在 `internal/project/reader_test.go` 与 `internal/integrations/narrationwriter_test.go` 增加断言：
   - `internal/project`：用一段含隐藏页的 PPTX fixture（PowerPoint "Hide Slide" 导出），断言 `reader.Hidden(idx)` 等价于 `Slide.Hidden()`。
   - `internal/integrations`：`NarrationWriter` 在隐藏页上调用 `SetAdvanceAfter` 后回读 `AdvanceAfter` 应等于设定值。
2. **真机门禁**（如适用）：PowerPoint 打开 ppts 写出的 PPTX，确认隐藏页确实在放映时被跳过；自动切页时长播放后准时翻页。
3. **验收通过**：ppts 端打 VERIFIED + 入库回归，go-pptx 仓库打 `git tag -s v1.0.3` 终验。
4. **未通过**：标 REOPENED，附差异与复现，退回本仓库做 IN_PROGRESS。

## 已知边界

- 本版实现**未触及 §7/FEAT-002**：阅读顺序建议 + 图表单位字段扩张需新 ADR 评审。
- 本版**未触及 §7/FEAT-001** 真机门禁（属 ppts 端 PowerPoint 安装 + 真实播放记录，非 go-pptx 代码工作）。
- IR 字段标 Experimental——ppts 端集成时若发现需要 `--schema-version` 升级路径稳定档，可走 ADR-019 走评审。
