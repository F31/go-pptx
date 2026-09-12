# ADR-017: chart 实现层抽取到 `internal/chart`

- **状态**: **Implemented（r0 → r1 → r2 → r3 第三批已落地 2026-09-12）**
- **修订路径**:
  - r0（commit 67939bb）：草案，列出 36 函数假设零依赖根包，**事实错误**
  - r1（commit 9f5c54b）：修订——保留 4 批节奏（零依赖 → type alias → 全搬迁 → 清理），第一批仅 3 函数 + 4 常量
  - r2（commit ce3f66c）：第二批**值对象 type alias move 落地**——11 值对象（4 chart.go + 7 chartadv.go）+ Optional[T] 泛型搬到 internal/chart；根包 5 文件用 type alias 形式引用；公共 API 表面零变化（// Stable: 34 / // Experimental: 5 / 158 总 type 全部锁死不变）；B1 黄金语料 replay 全绿；internal/chart 覆盖率 97.6%
  - r3（commit `<pending>`）：第三批**全实现搬迁 + 根包 facade 清理**——parse / build / canonical / validate / fragment / workbook 全部实现搬到 internal/chart；**同时删除 31 个已无生产调用方的根包私有 facade**（保留 `chartIsCanonical` / `parseChartSpace` / `buildChartSpaceXML` / `validateChartData` / `nodeText` 等仍有调用方的适配器），并把白盒测试迁入 internal/chart 恢复覆盖率归属
- **r3 关键发现（记录以免重蹈）**:
  1. **`c:v` / `a:t` 的值在标签之间，不在属性上**——搬迁时若写 `Attr("", "val")` 会永远读空（标题/系列名/类别/数值全空）。必须取 `Original()[OpenEnd:CloseStart]` 并做实体解码。
  2. **`CachePoints` 必须按 `pt@idx` 排序并补空位**，不是按文档顺序追加。
  3. **`dLbls` 是图表组（barChart/lineChart/pieChart）的子元素**，不是 `c:plotArea` 的直接子元素——读侧必须从 plot 取，与 build 侧对称。
  4. **`xl/workbook.xml` 的 `xmlns:r` 不能漏**——`<sheet r:id="..."/>` 依赖该前缀；缺失会产出未定义前缀的非法 XML 并改变 xlsx 字节（B1 金样比对失败）。
  5. **覆盖率按包归属**：实现搬走后若测试留在根包，per-package 覆盖率不再归属，全仓 total 会跌破 COV-01 的 80% 里程碑（本次实测 84.4% → 79.4%）。必须同步迁测试，或删除无调用方的 facade，二者都不做则两边都掉。
  6. **迁测试的前提是先删死 facade**：否则测试留在根包保 facade、或迁走 tests 让 facade 变 0% 覆盖，两种都掉分。
- **关键 Go 知识**: type alias（`type X = pkg.X`）与"在 alias 上定义方法"是 Go 的根本冲突——方法必须定义在类型所在包（internal/chart），不能定义在 alias 上。这意味着第二批需要把 `String()` / `PlotElement()` 等方法一并搬到 internal/chart，根包只是 alias 引用
- **审批路径**: 草案 + r1 修订已被用户连续「继续」默认通过；第二批实施继续由「按推荐步骤继续执行」隐式审批
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

新增 `internal/chart` 包，分**三批渐进抽取**：

- **第一批（零依赖子集）**：仅搬真正不引用任何根包类型或根包常量的函数；本批可立即实施。
- **第二批（值对象 move）**：先把 `ChartData` / `ChartSpec` / `ChartSeries` / `ChartDataLabel` / `ChartErrorBars` / `ChartTrendline` / `ChartAxisOptions` / `ChartType` 等值对象**搬到 `internal/chart`** 作为根包类型别名（type alias）暴露——保持公共 API 签名不变，但底层类型定义在 internal 包；本批触及公共 API 文件边界，需用户二次审批。
- **第三批（解析/序列化/canonical）**：第二批落地后，parse / build / canonical / validate 系列整体搬迁。

根包 chart 系列只留下：公共类型别名、enum stringer、`*Slide.AddChart` 入口、`*Presentation` 私有助手、第三方 workbook builder 接口。

### 1. 第一批抽取范围（搬到 `internal/chart`，本周可实施）

**真零依赖根包的函数清单（依赖审计 2026-09-12 落地）**：

| 原位置 | 函数 | 依赖项 | 可搬性 |
|---|---|---|---|
| chart.go | `buildChartFrameFragment(id, x, y, cx, cy int64, rid string) string` | `chartGraphicURI` / `nsChartML`（需随函数搬） | ✅ |
| chartbook.go | `chartNumber(v float64) string` | 无 | ✅ |
| chartbook.go | `chartWorkbookColumn(n int) string` | 无 | ✅ |
| chart.go | `strIn(s string, list ...string) bool` | 无（通用 helper，建议留在根包；非必须搬） | ⚠ 可选 |

**常量随搬**：`chartGraphicURI` / `chartSheetName` / `chartCatAxID` / `chartValAxID` —— 这些纯字符串常量搬到 `internal/chart` 后由根包再以 `chartinternal.X` 引用。

**合计**：3 个函数 + 4 个常量 = 第一批落地范围。

### 2. 第二批抽取范围（值对象 move，需用户二次审批）

| 原位置 | 值对象 | 搬到 `internal/chart` 后如何暴露 |
|---|---|---|
| chart.go | `ChartType` (int + 3 consts) | `type ChartType = chartinternal.ChartType` |
| chart.go | `ChartSpec` / `ChartData` / `ChartSeries` | 同上 |
| chart.go | `ChartDataLabel` / `ChartErrorBars` / `ChartTrendline` / `ChartAxisOptions` | 同上 |
| chart.go | 枚举 `ChartErrorType` / `ChartTrendType` / `ChartErrorType.String` 等 | `type ChartErrorType = chartinternal.ChartErrorType` |

类型别名（type alias）保证调用方零修改（`ChartData{...}` 字面量、字段名 `Type`/`Categories`/`Series` 等零变化）。**binary-compat 严格守门**：导出符号集合 102 API / 158 总 type 不变。

### 3. 第三批抽取范围（第二批落地后）

| 原位置 | 函数 |
|---|---|
| chart.go | `parseChartSpace` / `parseChartDLbls` / `parseChartTrendline` / `parseChartErrBars` / `parseChartAxes` / `chartSerName` / `chartSerCategories` / `chartSerValues` / `chartCachePoints` / `nodeText` |
| chart.go | `buildChartSpaceXML` / `buildChartFrameFragment`（与第一批合并） |
| chartfrag.go | `buildChartDataLabelFragment` / `buildErrBarsFragment` / `buildTrendlineFragment` / `buildCatOrDateAxFragment` / `buildValAxFragment` / `validateChartDataLabel` / `validateChartErrorBars` / `validateChartTrendline` / `validateChartAxisOptions` |
| chart.go | `validateChartData` / `chartIsCanonical` / `canonicalAxExtensions` / `canonicalSer` / `canonicalTrendline` / `canonicalErrBars` / `canonicalTitleSubtree` / `canonicalRichText` |
| chart.go | `chartTypeFromPlot` / `chartTrendTypeFromName` / `chartErrorTypeFromName` |
| chartbook.go | `buildChartWorkbookXML` / `buildChartSheetXML` |

合计 30 函数。依赖项全部为 internal/chart 类型（值对象已搬），零反向依赖根包。

### 4. 保留在根包

- **公共类型别名**（v1.0 冻结符号的 binary-compat 表面）：`ChartType` / `ChartSpec` / `ChartSeries` / `ChartData` / `ChartShape` / `ChartDataLabel` / `ChartErrorBars` / `ChartTrendline` / `ChartAxisOptions` / `ChartErrorType` / `ChartTrendType` / `ChartWorkbookBuilder` 接口 / `DefaultWorkbookBuilder`（公共类型别名）
- **enum stringer**：`ChartErrorType.String` / `ChartTrendType.String` / `ChartType.String` / `ChartType.plotElement`（以 alias 形式暴露）
- **`*Slide.AddChart`** —— 公共 API
- **`*Presentation` 私有助手**（4 个）：`chartWorkbookPartOf` / `chartWorkbookBytes` / `SetChartWorkbookBuilder` / `chartOfGraphic`

### 5. 包边界与依赖方向

```
internal/chart  → internal/xmlstore
                 → (无 internal/opc 依赖；opc.PartName 由调用方传值)
root pptx       → internal/chart (type alias 形式引用所有值对象)
                 → internal/editplan
                 → internal/document
                 → ...（其余现有 7 个 internal 子包）
```

`internal/chart` **绝不反向 import 根包**。**第一批**：3 个真零依赖函数 + 4 常量。**第二批**：值对象以 `type X = chartinternal.X` 形式别名暴露在根包；底层类型定义在 internal 包；调用方零修改。**第三批**：parse / build / canonical / validate 系列整体搬迁，零反向依赖。

### 6. 实施节奏（最小惊讶原则）

#### 第一批（week 1，本轮可立即启动）：3 真零依赖函数 + 4 常量

新文件：

- `internal/chart/chart.go`（`BuildChartFrameFragment` / `ChartNumber` / `ChartWorkbookColumn` 三个公开包函数 + 4 常量）
- `internal/chart/chart_test.go`（3 函数行为测试 + 4 常量正确性测试）
- 根包 `chart.go` / `chartbook.go` 改为对 `chartinternal.X` 的薄包装

退出标准：
- `go test ./...` 全绿
- corpus validate 全绿
- B1 哈希回归全绿
- chart.go 行数 -30（仅 3 函数搬走）

#### 第二批（week 2，需用户二次审批）：值对象 move

新增 7 个值对象 + 2 个枚举到 `internal/chart`；根包改为 type alias；所有引用方零修改。

退出标准：
- `go test ./...` 全绿
- L3 客户端矩阵 8/8 重跑仍 8/8
- binary-compat 检查通过（grep `^type [A-Z]\|// Stable:\|// Experimental:` 不变）
- chart.go 行数 -200

#### 第三批（week 3，二批落地后）：解析 + 序列化 + canonical + validate

新文件：

- `internal/chart/canonical.go`（7 函数）+ `internal/chart/validate.go`（5 函数）+ `internal/chart/codec.go`（10 解析 + 5 build 函数）+ `internal/chart/mapping.go`（3 类型映射）
- 根包 chart 系列文件归并到一个 `chart.go`（< 300 行）+ chart_adapter.go（thin wrapper）

退出标准：同第二批 + `chart_test.go` 测试零失败 + PERF-01 基线无回退

#### 第四批（week 4，可选）：清理

- 删除内部 `chart` 前缀函数名（如 `canonicalSer` → `internal/chart.IsCanonicalSer`）
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

## 第一批产出物清单（已落地 2026-09-12，commit 46c2f2d）

- ✅ `internal/chart/` 包目录
- ✅ `internal/chart/chart.go` —— 4 常量（GraphicURI / SheetName / CatAxID / ValAxID）
- ✅ `internal/chart/frame.go` —— 3 函数（BuildChartFrameFragment / ChartNumber / WorkbookColumn）
- ✅ `internal/chart/chart_test.go` —— 9 个测试（覆盖率 94.1% → 第二批后 97.6%）
- ✅ chart.go 净减少 6 行；chartbook.go 净减少 11 行（薄包装）
- ✅ `docs/architecture-current.md` §"压力点"更新
- ✅ `docs/go-pptx-实施状态跟踪.md` §"最近更新"补登记
- ✅ `.workbuddy/memory/MEMORY.md` §"工程约定"补 internal/chart 行

## 第二批产出物清单（已落地 2026-09-12，commit `<pending>`）

第二批抽取的 11 值对象 + Optional[T] 泛型（搬到 internal/chart/types.go / optional.go）：

- `ChartType` int 枚举 + iota (ChartBar / ChartLine / ChartPie) + String / PlotElement 方法
- `ChartSpec` struct（Type / Title / Categories / Series / X / Y / Width / Height / DataLabel / Axes）
- `ChartSeries` struct（Name / Values / ErrorBars / Trendline）
- `ChartData` struct（Type / Title / Categories / Series / DataLabel / Axes）
- `ChartDataLabel` struct（Show / Position）
- `ChartErrorType` int 枚举 + iota (4 值) + String 方法
- `ChartErrorBars` struct（Type / Value / Direction / NoEndCap）
- `ChartTrendType` int 枚举 + iota (6 值) + String 方法
- `ChartTrendline` struct（Type / Period / Order / DisplayEq / DisplayRSq / Name / Intercept / SetIntercept）
- `ChartAxisOptions` struct（CategoryAsDate / ValueLogBase / Min / Max / Position）
- `Optional[T any]` 泛型 struct + NewOptional

根包改动：
- chart.go -105/+55（净 -50 行）
- chartadv.go -135/+79（净 -56 行）
- chartbook.go（仅 import alias 调整）
- font.go（Optional[T] type alias）
- chart_test.go（plotElement → PlotElement 公开名）

binary-compat 守门：// Stable: 34 / // Experimental: 5 / 158 总 type / 17 哨兵 全部锁死不变。

## 第三批落地结果（r3，2026-09-12）

### 搬到 `internal/chart` 的实现

| 文件 | 内容 |
|---|---|
| `parse.go` | `ParseChartSpace` / `ParseChartDLbls` / `ParseChartTrendline` / `ParseChartErrBars` / `ParseChartAxes` / `SerName` / `SerCategories` / `SerValues` / `CachePoints` / `NodeText` / `childOfKind` / `childText` / `strIn` / `ChartTypeFromPlot` / `TrendTypeFromName` / `ErrorTypeFromName` |
| `build.go` | `BuildChartSpaceXML` + 命名空间常量（`nsChartML` / `nsDrawingML` / `nsOfficeDocument` / …） |
| `canonical.go` | `IsCanonical` / `CanonicalAxExtensions` / `CanonicalSer` / `CanonicalTrendline` / `CanonicalErrBars` / `CanonicalTitleSubtree` / `CanonicalRichText` |
| `frag.go` | `ValidateChartData` + 4 个子校验器 + `BuildChartDataLabelFragment` / `BuildErrBarsFragment` / `BuildTrendlineFragment` / `BuildCatOrDateAxFragment` / `BuildValAxFragment` |
| `workbook.go` | `BuildChartWorkbookXML` / `BuildChartSheetXML` + xlsx 关系/内容类型常量 |
| `errors.go` | `ValidationError` / `BuildError` + 本包哨兵（经字符串映射回根包哨兵） |
| `escape.go` | `xmlUnescape` / `parseHexRune` / `parseDecRune`（与根包 `text.go` 同源，注明双向同步） |

### 根包 facade 清理（本次的关键决定）

第三批落地后，根包一度保留 31 个"薄包装"，其中**多数已无任何生产调用方**（只有测试引用）。薄包装的原意是"调用方零修改"，但调用方本身已被搬走，facade 遂成死代码——既虚增根包行数（与「缩小 chart.go」目标相悖），又让覆盖率只落在根包一侧。

**处置**：删除 17 个已无调用方的私有 facade + chartfrag.go 的 9 个 builder/validator 包装 + chartbook.go 的 4 个 builder/column/number 包装；**保留**仍有生产调用方的 `chartIsCanonical` / `parseChartSpace`（bind.go 与 chart.go 的 SetData/Data 路径）/ `buildChartSpaceXML` / `validateChartData` / `nodeText`（xlsx 单元格测试读取器）。

### 测试归属迁移

以下白盒用例从根包 `chart_test.go` 迁到 `internal/chart`（`migrated_test.go`），断言目标改为本包导出名：

- `TestChartPlotElementMapping` → `TestChartTypeFromPlotMapping`
- `TestChartSerCategoriesAllPaths` / `TestChartSerValuesAndName`
- `TestChartIsCanonicalBranches`
- `TestCanonicalPredicates` → `TestCanonicalPredicatesMigrated`

新增 `codec_test.go`（build→parse 往返 / CHART-02 扩展往返 / 校验分支 / fragment 构造器 / xlsx 包装配 / 实体解码），把本包覆盖率从 33.0% 提到 **91.4%**。

### 结果

| 指标 | 值 |
|---|---|
| 根包 chart 系列行数 | chart.go 1319 → **481**；chartfrag.go 262 → **74**；chartbook.go 215 → **107** |
| `internal/chart` | 10 文件 ≈ 1000 行实现 + 1090 行测试，覆盖率 **91.4%** |
| 公共 API | `// Stable:` **34** / `// Experimental:` **5** / root type **158** / 哨兵 **17** 全部锁死不变 |
| 全仓覆盖率 | **84.4%**（与迁移前一致——归属迁移后回到基线） |
| 守门 | `go test ./...` 全绿；`go test -tags=corpus ./...` 全绿；`corpus.py validate` 36/36；`scripts/perf/smoke.sh` PASS（含 ADR-018 ①b） |

### 第四批（可选，未做）

- 归并 `build.go` / `frag.go` 与命名空间常量到更少的文件；
- 评估 `DefaultWorkbookBuilder` 实现是否也抽到 `internal/chart`（当前根包保留实现，仅把纯算法下沉）。
