# ADR-020: FEAT-002 项 3 降置信子项（诊断驱动）

- **状态**: Accepted（已实施）
- **日期**: 2026-09-12
- **关联 ADR**: ADR-019（同条目设计空间）、ADR-015（API 稳定性分级）
- **关联条目**: ppts §7 / FEAT-002 项 3

## 上下文

FEAT-002 项 3 要求"图表优先读取嵌入数据与坐标轴/单位；无法可靠提取时降低置信度"。坐标轴单位
（`majorUnit` / `minorUnit` / `numFmt` 等）当前未被 `ParseChartAxes` 提取（`parse.go:223` 只读
`CategoryAsDate / ValueLogBase / Min / Max / Position`），而 ppts 自动讲解据此生成单位文案。

在 ADR-019 阶段二（精确字段）落地前，需要一个**不扩张 API、零字节变化**的 interim 方案，让
ppts 至少能"感知"到轴信息不完整并主动降置信。

## 决策

采用 ADR-019 方案 B（诊断驱动），新增**追加式只读公开方法**与 internal 检测器：

1. `internal/chart.ChartAxisUnreadFieldNames(doc, root) []string`：扫描 `plotArea` 下
   `catAx/dateAx/valAx` 子元素，返回本库未提取的轴语义字段 local 名（去重 + 字典序）。
   零依赖根包类型。检测器字典 `unreadAxisFieldLocals` 当前含：
   `majorUnit / minorUnit / numFmt / majorTickMark / minorTickMark / majorGridlines /
   minorGridlines / dispUnits / tickLblPos`。纯布局/样式字段（`spPr` / `txPr` 等）不计入降置信。

2. `ChartShape.DataWithDiagnostics() (ChartData, []Diagnostic, error)`：复用 `chartDataContext()`
   解析，数据快照与 `Data()` 完全一致；当 `ChartAxisUnreadFieldNames` 非空时，追加一条
   `Diagnostic{Code: "chart.axis.unread", Severity: SeverityInfo, Part: <part>,
   ShapeID: c.ID(), Message: "图表轴含本库未提取字段（...），轴单位/刻度/数字格式信息可能不完整，
   置信度已降低"}`。

3. `ir.Shape.Diagnostics` 字段（`Diagnostics`，`json:"diagnostics,omitempty"`）：`projectShape`
   的 chart 分支改调 `cs.DataWithDiagnostics()`，将降置信诊断经 `convertDiagnostics`
   （根包 `Diagnostic` → IR `Diagnostic` 字段子集投影）写入 `s2.Diagnostics`。

## 兼容性

- 纯追加：不改 `Chart.Data()` / `ChartShape.Data()` 签名，不删不改既有 getter。
- binary-compat with v1.0.0..v1.0.4：`ChartShape.DataWithDiagnostics` 进入 goldenStableMethods
  （129 → 130）；`FillProvider / GeometryProvider / LineProvider / StyleMatrixRefsProvider`
  经 ADR-021 同步进入 goldenStableSymbols（50 → 55）。
- 零写入、零字节变化：未提取字段仍被静默保留在 XML 中，不改变任何 part。

## 验收门槛

```bash
go test ./...
go test -tags=corpus ./...
go test ./internal/chart/ -run 'TestChartAxisUnreadFieldNames'
go test . -run 'TestAPIFrozen'
```

## 后果

### 正面

- ppts 立即获得降置信信号，无需等待 ADR-019 阶段二；
- 检测器零依赖根包，可独立单测（`axis_diag_test.go`）；
- 不影响既有 `Data()` 消费者。

### 风险

- 仅降级不精确：ppts 仍无法拿到具体单位/格式，只能标注"可能不准确"；
- 检测器字典需随 `ParseChartAxes` 演进维护：一旦某字段被正式提取，必须从
  `unreadAxisFieldLocals` 移除，否则会持续误报降置信。

## 当前落地状态

- 代码已实施并通过全量测试（含 corpus）。
- 改动文件：`internal/chart/parse.go`（+`ChartAxisUnreadFieldNames` / `unreadAxisFieldLocals`）、
  `chart.go`（+`chartDataContext` / `DataWithDiagnostics`）、`ir/ir.go`
  （+`Shape.Diagnostics` / `convertDiagnostics` / chart 分支）、`api_surface_test.go`
  （golden 同步）、新增 `internal/chart/axis_diag_test.go`。
- commit：本批次（随 ADR-019/021 一并提交）。
