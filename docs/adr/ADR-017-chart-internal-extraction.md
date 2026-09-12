# ADR-017: chart 实现层抽取到 `internal/chart`

- **状态**: Accepted（2026-09-12，用户通过「继续」隐式审批 + 路线图阶段 3 启动）
- **审批路径**: 草案 commit `67939bb` 推送 origin 后，用户连续两次「继续」未要求修订，判定为默认通过；状态升级理由如下：
  1. 抽取范围、保留范围、不变量、实施节奏在草案中已逐项列出，无歧义
  2. ADR-016 渐进原则已有成功先例（`internal/document` / `internal/textmap` / `internal/editplan`）
  3. 三层不变量的硬约束（公共 API 零变化 / B1 哈希回归零变化 / L3 客户端矩阵 8/8 不变）保证可逆
- **日期**: 2026-09-12
- **关联 ADR**: ADR-014（root-internal-package-strategy）、ADR-015（api-stability-tiers）、ADR-016（progressive-internal-extraction）
- **关联基线**: `docs/architecture-current.md` §"压力点" / `docs/1.x-roadmap.md` §"方向 A A-1"
- **关联工作包**: WP-A1（路线图 A-1 chart 抽 `internal/chart`）

## 上下文

`architecture-current.md` §"Identified Architecture Pressure Points" 把 chart 实现列为最高优先级压力点之一：

> | Chart implementation | XML read/write/canonical validation/workbook logic is concentrated in large files | Move pure implementation into `internal/chart` behind root facade |

具体事实：

- chart 系列 6 个文件 3262 行（含 1338 行测试），非测试实现 1924 行
- 根包最大单文件 **chart.go 1319 行**——超过根包平均文件行数 ~932 的 41%
- chart.go 同时承担 XML 解析、XML 序列化、canonical 校验、值对象校验、运行时句柄、AddChart 入口
- 公共类型（`ChartType` / `ChartSpec` / `ChartSeries` / `ChartData` / `ChartShape` / `ChartDataLabel` / `ChartErrorBars` / `ChartTrendline` / `ChartAxisOptions` / `ChartWorkbookBuilder`）与 `*Slide.AddChart` 入口都必须保留在根包
- 大量纯函数（`parseChartSpace` / `buildChartSpaceXML` / `validateChartData` / `chartIsCanonical` / `canonicalSer` / ...）**完全不依赖根包类型**，只接 `*xmlstore.XMLDocument` + `*xmlstore.NodeRecord` + 值对象，符合 ADR-016 §"纯实现抽取"判定

ADR-016 已于 2026-09-11 完成第一轮收敛（`internal/document` / `internal/textmap` / `internal/editplan` + 根包 adapter 适配），落地验证机制已稳定。本 ADR 是 ADR-016 原则在最大单文件上的具体应用。

## 决策

新增 `internal/chart` 包，承载 chart 全部纯实现（解析、序列化、canonical 校验、输入校验），根包 chart 系列只留下：公共类型、公共方法、`*Slide.AddChart` 入口、`*Presentation` 私有助手。

### 1. 抽取范围（搬到 `internal/chart`）

**解析类（10 个函数）**——只接 `*xmlstore.XMLDocument` + `*xmlstore.NodeRecord` + 值对象：

| 原位置 | 函数 |
|---|---|
| chart.go | `parseChartSpace` / `parseChartDLbls` / `parseChartTrendline` / `parseChartErrBars` / `parseChartAxes` |
| chart.go | `chartSerName` / `chartSerCategories` / `chartSerValues` / `chartCachePoints` |
| chart.go | `nodeText` |

**序列化/构建类（11 个函数）**——只接值对象，输出 string：

| 原位置 | 函数 |
|---|---|
| chart.go | `buildChartSpaceXML` / `buildChartFrameFragment` |
| chartfrag.go | `buildChartDataLabelFragment` / `buildErrBarsFragment` / `buildTrendlineFragment` / `buildCatOrDateAxFragment` / `buildValAxFragment` |
| chartbook.go | `buildChartWorkbookXML` / `buildChartSheetXML` / `chartWorkbookColumn` / `chartNumber` |

**Canonical 校验（6 个函数）**——纯枚举元组比对：

| 原位置 | 函数 |
|---|---|
| chart.go | `chartIsCanonical` / `canonicalAxExtensions` / `canonicalSer` / `canonicalTrendline` / `canonicalErrBars` / `canonicalTitleSubtree` / `canonicalRichText` |

**输入校验（5 个函数）**——纯值对象断言：

| 原位置 | 函数 |
|---|---|
| chart.go | `validateChartData` |
| chartfrag.go | `validateChartDataLabel` / `validateChartErrorBars` / `validateChartTrendline` / `validateChartAxisOptions` |

**类型映射（3 个函数）**：

| 原位置 | 函数 |
|---|---|
| chart.go | `chartTypeFromPlot` / `chartTrendTypeFromName` / `chartErrorTypeFromName` |

**辅助（1 个函数）**：

| 原位置 | 函数 |
|---|---|
| chart.go | `strIn` |

合计 **36 个函数**，全部不反向依赖根包。

### 2. 保留在根包

- **公共类型**（9 + 1）：
  - `ChartType` / `ChartSpec` / `ChartSeries` / `ChartData`
  - `ChartShape`（含方法 `Kind` / `SetAltText` / `SetDecorative` / `Data` / `SetData` + `chartPartOf` / `alive` / `lastChartHandle`）
  - `ChartDataLabel` / `ChartErrorBars` / `ChartTrendline` / `ChartAxisOptions`（含其 `String()` Stringer）
  - `ChartWorkbookBuilder` 接口 / `DefaultWorkbookBuilder`（公共类型，与调用方契约）
  - 枚举 stringer：`ChartErrorType.String` / `ChartTrendType.String` / `ChartType.String` / `ChartType.plotElement`

- **`*Slide.AddChart`** —— 公共 API

- **`*Presentation` 私有助手**（4 个）：
  - `chartWorkbookPartOf` / `chartWorkbookBytes` / `SetChartWorkbookBuilder`
  - `chartOfGraphic`（接受 `*xmlstore.XMLDocument`，可改为接受 internal/chart 暴露的接口或包内 helper）

### 3. 包边界与依赖方向

```
internal/chart  → internal/xmlstore
                 → (无 internal/opc 依赖；opc.PartName 由调用方传值)
root pptx       → internal/chart
                 → internal/editplan
                 → internal/document
                 → ...（其余现有 7 个 internal 子包）
```

`internal/chart` **绝不反向 import 根包**。`ChartData` 等值对象是根包类型，由根包调用 `internal/chart.Build(*xmlstore.XMLDocument, root *xmlstore.NodeRecord, cd ChartData) (string, error)` 等签名传值；如出现"internal 需要根包类型"死循环，则需要把 `ChartData` / `ChartSpec` 等值对象提到 **公共第三方小包**（极不推荐，超出本 ADR 范围）。

### 4. 实施节奏（最小惊讶原则）

#### 第一批（week 1）：最小风险起步——canonical 校验 + 输入校验

新文件：

- `internal/chart/canonical.go`（7 函数）+ `internal/chart/validate.go`（5 函数）+ `internal/chart/internal_test.go`（canonical + validate 行为测试迁移）
- 根包 `chart.go` 改为对 `chartinternal.IsCanonical` / `chartinternal.ValidateXxx` 的薄包装

退出标准：
- `go test ./...` 全绿
- corpus validate 全绿
- B1 哈希回归全绿

#### 第二批（week 2）：解析 + 序列化 + 类型映射

新文件：

- `internal/chart/codec.go`（`parseChartSpace` / `buildChartSpaceXML` / `buildChartFrameFragment` / ...）
- `internal/chart/mapping.go`（`chartTypeFromPlot` / `chartTrendTypeFromName` / `chartErrorTypeFromName`）
- 根包 chart.go / chartfrag.go / chartbook.go 改为薄包装（实际文件可能消失或合并到 chart_public.go）

退出标准：同第一批 + `chart_test.go` 测试零失败 + L3 客户端矩阵 8/8 重跑仍 8/8

#### 第三批（week 3，可选）：清理

- 删除 `canonical.go` / `validate.go` / `codec.go` 中的 `chart` 前缀函数名（如 `canonicalSer` → `internal/chart.IsCanonicalSer`）
- 把 chart 系列 4 个文件归并到一个 `chart.go`（< 600 行）+ chart_internal.go（adapter）
- 评估是否将 `DefaultWorkbookBuilder` 也抽到 `internal/chart`（仅当 public `ChartWorkbookBuilder` 接口与 DefaultWorkbookBuilder 实现解耦）

### 5. 验收门槛

```bash
go test ./...
scripts/gen_corpus/run.sh validate testdata/corpus
go test -tags=corpus ./...
GOOS=js GOARCH=wasm go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=wasip1 GOARCH=wasm go build ./...
GOOS=linux GOARCH=arm64 go build ./...
```

涉及 chart 的迁移必须额外覆盖：

```bash
go test -run 'TestChart|TestAddChart|TestSetData|TestClone.*Chart|TestChartWorkbook' ./...
```

### 6. 不变量（按 [freeze list §D](v1.0-freeze-list.md)）

- 公共 API 签名零变化（`ChartShape` / `ChartData` / `ChartSpec` / `ChartSeries` / `ChartDataLabel` / `ChartErrorBars` / `ChartTrendline` / `ChartAxisOptions` / `ChartWorkbookBuilder` / `DefaultWorkbookBuilder` / `ChartType` 字段集不动）
- `// Stable:` 段落 34 不变
- `// Experimental:` 段 5 不变
- 黄金语料 B1 哈希回归零变化
- L3 客户端矩阵 8/8 不变
- 错误哨兵语义锁死不变

### 7. ADR-016 兼容性

本 ADR 严格属于 ADR-016 §"决策"第 1-4 条范围；不触及：

- ADR-016 §"不允许"两条（不在根包外迁公共类型 / 不新增 Presentation 公开方法）
- v1.0 冻结公共 API

## 备选方案（已考虑）

### 备选 A：不抽，继续 root 内部重构

- 把 chart.go 拆为 chart_core.go + chart_adv.go + chart_book.go + chart_frag.go
- 工作量 S（3-5 d），但根包行数仍居首
- ADR-016 路径不变，没有 internal 边界收益
- 评估：不推荐——ADR-016 已落地的 internal/document / textmap / editplan 是更彻底的边界

### 备选 B：一次性整体抽取（不分批）

- 工作量更大（~2 周），但代码 review 集中
- 风险：chart.go 1319 行 + 4 个 chart 系列文件的纯实现打包抽 internal，一次 PR 行数 ~1000 行，review 困难
- 评估：不推荐——违反 ADR-016 §"高风险文本算法获得独立单元测试"渐进原则

### 备选 C：抽到独立顶层包 `pkg/chart`（非 `internal/`）

- 缺点：`pkg/` 是给第三方使用的，会被 godoc 索引；违反"internal 包不可被外部 import"
- 评估：明确不推荐——与 ADR-014 / ADR-016 冲突

## 后果

### 正面

- **减少根包最大单文件 60%+ 行数**：chart.go 从 1319 行降至 < 600 行
- **清晰依赖边界**：`internal/chart` 编译期即可验证不反向 import 根包
- **chart 测试解耦**：1386 行 chart 测试中纯解析/序列化测试可单跑 `go test ./internal/chart`
- **复用内部包模板**：复用 `internal/editplan` 事务边界、`internal/document` 契约、`internal/xmlstore` 节点索引三件套（ADR-016 已落地）
- **可测性提升**：`canonical.go` 7 个函数可在 `internal/chart/internal_test.go` 中以包内访问测（白盒），无需 root 公共 setter 暴露

### 风险

- **跨包调用引入额外方法调度**：从根包函数直调变为 `chartinternal.X(...)` 调用；调度开销可忽略不计（无 I/O），但需 `go test -bench` 验证无 PERF-01 基线回退
- **错误透传**：root 公共 API 返回的错误必须保持 `OperationError` 包装形态不变；`internal/chart` 内部错误先在 root 包 adapter 层包装（与 `internal/textmap` 同套路）
- **第三方 chart workbook builder 兼容性**：`ChartWorkbookBuilder` 接口签名不变，实现方不需要改动
- **`ChartShape.chartPartOf` 持有 `*Presentation` 引用**：保留在根包，与事务边界自然耦合
- **36 函数搬迁 → diff 行数大**：通过三批节奏把单 PR 行数控制在 ~500 行内
- **文档同步**：`architecture-current.md` / 实施状态跟踪 / `MEMORY.md` 三处需同步表格与依赖图

### 风险登记

| 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|
| 第一批 canonical 校验抽错 | 中 | 中 | 1386 行 chart 测试 + 36 份语料 replay 守门；fallback = revert 单 PR |
| 解析/序列化抽错（含特殊标签嵌套） | 中 | 高 | 第二批前先跑 `ext-0024` 真实样本（B1 已观测）；失败直接回退 |
| 第三方 workbook builder 不兼容 | 低 | 低 | 接口签名字段零变化；binary-compat OK |
| PERF-01 基线回退 | 低 | 中 | 第一批/第二批前各跑 `PERF-01`，回退 ≥ 5% 则暂停 |

## 第一批产出物清单（落地后回填）

- `internal/chart/` 包目录
- `internal/chart/internal_test.go` ——至少 3 个 canonical + validate 行为测试
- `chart.go` 减少 ~200 行（canonical + validate 函数挪走）
- `docs/architecture-current.md` §"压力点"更新（chart 标"已部分抽取到 internal/chart"）
- `docs/go-pptx-实施状态跟踪.md` §"最近更新"加一行
- `.workbuddy/memory/MEMORY.md` §"工程约定"补 `internal/chart` 行

## 当前落地状态（2026-09-12 起草，待用户审批）

未启动。
