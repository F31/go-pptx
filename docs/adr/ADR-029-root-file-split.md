# ADR-029: 根包大文件按职责拆分（in-package file split）

- 状态：**已实施**（2026-09-16）
- 关联：[ADR-016](ADR-016-progressive-internal-extraction.md)（渐进式 internal 抽取）、[ADR-017](ADR-017-chart-internal-extraction.md)（chart 抽 `internal/chart`）、1.x 路线图 §3 方向 A（A-4/A-5）

## 上下文

根包 `pptx` 的非测试文件里，`text.go` 1356 行、`bind.go` 1179 行长期居首。1.x 路线图把它们列为方向 A 的收尾项（A-4 text.go / A-5 bind.go），目标是把根包文件平均行数压到 ~600 量级、提升可维护性。

关键判断：**这两处的正确做法是"同包内按职责拆文件"，而不是"抽到 `internal/`"**。原因：

- `text.go` 的句柄基元（`nodeStep`/`textNode`/路径解析）与 `Paragraph`/`TextRun` 紧密互锁，抽取会引入大量跨包接口；
- `bind.go` 的 `bindScanner` 状态机横跨 plan/apply，抽包需要把整台扫描器搬走；
- 二者都**不是** ADR-016/017 意义上的"可独立复用的内聚子系统"（chart 是），而是根包核心模型的一部分。

同时 ADR-016 的**归属陷阱**给出硬约束：**把高覆盖代码搬出根包会因白盒测试留在原处而使目标包覆盖率骤降**。〔ADR-017 第三批期间 `internal/chart` 一度记 5.5%、全仓 total 跌到 79.4%。〕同包拆文件不触发该陷阱（覆盖率按包归属，文件边界不影响）。

## 决策

1. **同包拆文件**（不改 package、不改任何声明体、公共 API 零变化），按职责分组：

   `text.go`（1356）→
   - `textnode.go`：句柄节点/路径基元
   - `text.go`：TextFrame/Paragraph/TextRun 句柄类型 + 读 + 原始文本写
   - `textfont.go`：字符格式写入路径（a:rPr 增量 patch）
   - `textfontparse.go`：字符格式解析与片段构建
   - `textutil.go`：通用 XML 补丁/转义辅助

   `bind.go`（1179）→
   - `bind.go`：选项/报告/Bind 入口 + `bindScanner` 状态与 plan 记录
   - `bindbody.go`：形状扫描与正文绑定
   - `bindtable.go`：表格绑定与行模板渲染
   - `bindchart.go`：图表绑定与预检
   - `bindresolve.go`：数据源路径解析与取值
   - `bindmarker.go`：模板占位符词法与补丁辅助

2. **零行为改动**：只移动声明与文档注释；每个新文件加一行职责说明；import 集合按文件实际使用推导（不引入外部工具，保持零依赖）。
3. **验证守恒**：声明计数逐文件核对（`text` 58→58、`bind` 39→39），全套测试 + corpus + 覆盖率门禁通过，覆盖率数值不变。
4. **不抽 `internal/`**：除非某子集被证明是"可独立复用的内聚子系统"（按 ADR-016 §触发条件评估），否则维持同包拆分。

## 验证

| 项 | 结果 |
|---|---|
| 声明数守恒 | text 58→58、bind 39→39（无丢失/重复） |
| `go build ./...` | 通过 |
| `go test ./...` / `-tags=corpus ./...` | 14/14 全绿 |
| root 覆盖率 | 83.9%（拆分前后不变） |
| 覆盖率门禁 `scripts/coverage/gate.sh` | PASS |
| 根包非测试文件 | 52 文件 / 平均 **413 行**（原 top2 退出前列） |

## 后果

- 根包最大文件从 1356（text.go）降为 1122（geomadv.go）；`text.go` 372、`bind.go` 342。
- 拆分为纯机械搬运，**review 成本低、回归风险≈0**（无逻辑改动、声明计数守恒、覆盖率不变）。
- 后续同类大文件（`geomadv.go` 1122 / `format.go` 1102 / `style.go` 1004）可按同一策略处理；若某文件内部出现"可独立复用子系统"，再走 ADR-016 的 internal 抽取路径。
- 拆分脚本未入库（一次性工具）；策略与边界已固化于本 ADR，后续按此执行。

## 续：第二批（geomadv.go / format.go / style.go）

同策略处理剩余三个千行文件：

| 原文件 | 行 | 拆分 |
|---|---:|---|
| `geomadv.go` | 1122 | `geomadv.go`(类型+访问器) + `geomparse.go` + `fillparse.go` + `effectparse.go` |
| `format.go` | 1102 | `colorparse.go` + `format.go`(线条) + `paraprops.go` + `runprops.go` + `stylematrix.go` |
| `style.go` | 1004 | `style.go`(类型+入口) + `styleenv.go` + `placeholder.go` + `themeresolve.go` + `effresolve.go` |

守恒验证：声明数 geomadv 49→49、format 38→38、style 53→53；`go test ./...` 与
`-tags=corpus` 14/14 全绿；覆盖率门禁 PASS。

**根包非测试文件累计**：63 文件 / 平均 **342 行**（起点 ~932）；最大的 `docProps.go`
951 行（原 top3 均已退出前列）。
