# ADR-019: 图表坐标轴单位 / 刻度 / 数字格式（numFmt）设计空间

- **状态**: Draft（待 ppts 按 §4 模板回填需求后转 Proposed → Accepted）
- **日期**: 2026-09-12
- **关联 ADR**: ADR-015（API 稳定性分级）、ADR-020（FEAT-002 项 3 降置信子项，已实施）
- **关联条目**: ppts §7 / FEAT-002 项 3（图表单位）

## 上下文

ppts（自动讲解工具）需要从图表提取"坐标轴单位 / 刻度单位 / 数字格式"，用于生成讲解文案
（如"纵轴单位：万元，保留 1 位小数"）。

当前 go-pptx 的 `ChartAxisOptions`（`internal/chart/types.go:207`，R 档、非 Stable）仅含
`CategoryAsDate / ValueLogBase / Min / Max / Position` 五项，缺失：

- `MajorUnit` / `MinorUnit`（主/次刻度单位）——轴数值粒度；
- `NumFmt`（数字格式代码 formatCode，如 `0.0%`、`#,##0`）——显示格式；
- `MajorTickMark` / `MinorTickMark` / `MajorGridlines` / `MinorGridlines` / `DispUnits` /
  `TickLblPos` 等轴外观/语义字段。

`ParseChartAxes`（`parse.go:223`）只读上述 5 字段，静默丢弃其余轴语义字段——这正是
"降置信"信号源（已由 ADR-020 的 `ChartAxisUnreadFieldNames` 检测器标识）。

ppts 侧需求**尚未确认**具体字段集与语义（例如：numFmt 是否需要 formatCode 原文、是否需要
locale、单位是否需与系列值单位解耦）。在需求未确认前，盲定字段会导致：

- 过度承诺 Stable 表面（字段一旦进入对象模型即受 binary-compat 约束）；
- 与 ppts 实际消费方式错配（如 ppts 只需要"是否有 numFmt"布尔，而非完整 formatCode）。

故本 ADR 采用**设计空间型**：只框定问题域、候选方案与选项权衡，不锁定具体字段；待 ppts
按 §4 模板回填需求后，由后续 ADR（或本 ADR 的 Implemented 更新）钉死字段集与 Stable 承诺。

## 设计空间（候选方案）

### 方案 A：扩展 `ChartAxisOptions`（轴对象模型补全）

在 `internal/chart/types.go` 的 `ChartAxisOptions` 增加 `MajorUnit/MinorUnit/NumFmt` 等字段，
并由 `ParseChartAxes` 一并提取。根包当前未暴露 `ChartAxisOptions`（仅 `ChartShape` 为 Stable），
故加字段不触发 CI 门禁；但一旦 ppts 经 `ir` 或新公共 API 读取，即进入公共契约。

- 优点：信息最完整，单一轴结构体聚合。
- 风险：过早承诺字段集合；numFmt 的 formatCode 解析语义（百分比/千分位/小数位）需与
  ppts 文案生成对齐。

### 方案 B：诊断驱动（ADR-020 已落地，作为 interim）

不在对象模型加字段，而是通过 `ChartShape.DataWithDiagnostics()` 返回 `chart.axis.unread`
诊断，提示"轴含未提取字段"。ppts 据此整体降低对该图表轴单位/刻度的置信度，文案标注
"单位可能不准确"。

- 优点：零 API 扩张、零字节变化、立即可用（已实施于 v1.0.4+）。
- 风险：ppts 无法拿到具体单位/格式，只能降级而非精确呈现。

### 方案 C：新增 `ChartAxis` 富类型（轴级对象模型）

引入 `ChartAxis` 类型（catAx/dateAx/valAx 各一），聚合单位/刻度/格式/网格线，由新公共
getter（如 `ChartShape.Axes()`）返回。

- 优点：语义清晰、可扩展、符合对象模型入口升 Stable 的路径。
- 风险：API 表面最大，需 ADR-015 升档评审 + freeze-list §D checklist；v1.1.0 前不宜仓促。

## 决策（当前）

采用"**B 先行 + A/C 待定**"的两阶段策略：

1. **阶段一（已实施，ADR-020）**：以诊断驱动（方案 B）立即为 ppts 提供降置信信号，不扩张 API。
2. **阶段二（待 ppts 需求确认）**：按 §4 模板回填后，在 A / C 间选择并钉死字段集与 Stable
   承诺；若选 C，需新 ADR 走 ADR-015 升档流程。

本 ADR 不锁定阶段二字段；任何阶段二落地都必须：

- 同步 `api_surface_test.go` golden 清单（ADR-015 守门）；
- 保持 `Chart.Data()` / `ChartShape.Data()` 签名不变（binary-compat）；
- 不静默丢弃已提取字段。

## ppts 需求回填模板（§4）

> ppts 方请填写以下信息发回本仓库，作为阶段二 ADR 的输入：
>
> 1. 需要的具体字段集（勾选/补充）：
>    □ MajorUnit □ MinorUnit □ NumFmt(formatCode 原文)
>    □ NumFmt(归一化描述) □ TickMark □ Gridlines □ DispUnits □ TickLblPos
> 2. numFmt 消费方式：
>    □ 原样 formatCode 字符串用于下游渲染
>    □ 归一化为"单位 + 小数位"描述用于文案 □ 两者都要
> 3. 单位语义：
>    □ 轴单位即系列值单位 □ 轴单位需独立于系列值单位单独呈现
> 4. 是否接受先以诊断降级（方案 B）上线、后续再补精确字段（方案 A/C）：
>    □ 接受 □ 不接受（需阶段二同期交付）
> 5. 期望集成版本（v1.1.0 / v1.2.0）：____

## 验收门槛（阶段二落地时）

```bash
go test ./...
go test -tags=corpus ./...
go test ./internal/chart/ -run 'TestParseChartAxes'
```

## 后果

### 正面

- 不为未知需求过早承诺 Stable 字段；
- ppts 立即获得降置信信号（方案 B 已上线），不会因为"等单位"而空转。

### 风险

- 方案 B 只能降级不能精确呈现，ppts 文案仍需用户对单位存疑；
- 阶段二若选 C，API 表面扩张需严格守门（ADR-015 / api_surface_test.go）。

## 当前落地状态

- 阶段一（方案 B）：已实施于 v1.0.4+，`ChartShape.DataWithDiagnostics()` +
  `internal/chart.ChartAxisUnreadFieldNames` + `ir.Shape.Diagnostics`，全量测试通过。
- 阶段二：待 ppts 按 §4 模板回填需求后启动。
- 关联需求单：`docs/ppts-sync/FEAT-002-chart-axis-units.md`。
