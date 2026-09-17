# ADR-030: v2.0 目标架构——分层元仓库（layered meta-repo）

- **状态**: **In Progress**（v2.0 实施中；2026-09-16 经用户决策正式启动，**不承诺 v1.x 兼容**——允许一次性破坏性变更与 import 路径迁移）
- **日期**: 2026-09-16
- **修订**: 2026-09-16 第 3 轮——状态 Proposed→In Progress；新增 §别名策略（DTO 用 alias、句柄类型用真实类型 + 薄委托）；落地地基包 `internal/document/model` + `internal/diag`
- **修订**: 2026-09-16 第 1 轮（依据外部评审）——基线校正（行数/方法数）、机制 2 重写为只读投影、补新包门槛档位与验收口径、试点选型改为 bind 私有实现、ir/render 定性修正
- **修订**: 2026-09-16 第 2 轮——三批试点完成（textutil/bind/style）；`render` 定性由「死代码待三选一」更正为「有意发布的公共契约，保留」（见验收口径 §4）；`ir` 与 `render` 依赖方向在图中分列
- **关联设计**: 《go-pptx 完整设计方案 V2.6 开发实施版》§3（总体架构）、§4（写入路径）
- **替代/废止**: 无（本 ADR 为 v2.0 演进目标；落地时取代 ADR-014 §1、ADR-016、ADR-029 的 v1.x 拆分策略条款）
- **关联 ADR**: ADR-014（根包内部包策略）、ADR-015（API 稳定性分级）、ADR-016（渐进式 internal 抽取）、ADR-029（同包拆文件）

## 上下文

### 现状盘点（2026-09-16）

模块根**同时是公共门面与实现本体**：

| 位置 | 内容 |
|---|---|
| 根包 `pptx` | 63 非测试文件 / 21,560 行 / **129 个 Presentation+Slide 方法**（Presentation 86 + Slide 43），公共类型（Presentation/Slide/Shape/TextRun/…）与业务实现 100% 纠缠 |
| 顶层 `ir/` | **名义公共包**，实际仅被 `cmd/pptx` 与 `wasm/check` 引用（ADR-014 已记录）——公开暴露但无人从外部用 |
| 顶层 `render/` | **有意的公共契约**（M8 RENDER-01，`render.go` 自述「仅定义契约」，6 用例覆盖），零包外引用——v1.x 已发布故**保留**，见需求边界表 |
| `internal/` | 已有雏形：`opc` / `xmlstore` / `chart` / `document` / `editplan` / `audioprobe` / `videoprobe` / `textmap` / `textutil` / `bind` / `style` / `ooxmlns`（后四者为 v2.0 试点新增） |
| `cmd/` | `pptx`（CLI 六子命令）、`pptx_check`（WASM） |
| `wasm/` | `check`（纯函数包）、`site`（离线静态 UI） |

### 问题的本质

- 早期把公共对象模型放进根包并冻结（ADR-015）后，**实现被永久焊死在门面上**：`internal/style`、`geom`、`validate` 被 ADR-014 §1 判为"永久不拆"，理由正是"公共类型必须在根包"——v1.x 内该结论正确，代价是没有层。（注：ADR-014 §1 把已由 ADR-016 抽出的 `textmap` 误列入"永久不拆"清单，与其自身状态表矛盾——此处不采信其对 `textmap` 的表述。）
- 文件层面的归一化是冻结面内的**权宜**：ADR-029 §决策先做同包拆文件（连写命名，如 `textfontparse.go`），"续二"（commit `13fc964`）才引入 `text_*`/`style_*` 等域前缀——两者都改善了导航，但 63 个文件仍同属一个无层包，129 个门面方法仍与实现同文件共生。
- 症状（可量化）：门面方法数 = 实现函数数（无委托层）；改动门面签名必然触碰实现；新增能力只能继续堆进根包。

### 为何 v2.0 是正确时点

v2.0 允许破坏性变更——ADR-014"type alias re-export 收益≈0"的否决理由在 v2.0 消失：公共类型**整体搬进独立门面包**，不是 alias，是真实移动 + 薄委托。一次性的 import 路径迁移换永久的分层自由。

## 决策

**v2.0 目标：模块根变为元仓库（不含任何 `package pptx` 文件），唯一正式公共导入路径为 `github.com/F31/go-pptx/pptx`；全部实现移入 `internal/`，按功能垂直切片。**

### 目标布局

```
go-pptx/
├── go.mod / LICENSE / README.md          # 元仓库根（无 .go 文件）
├── internal/                             # 全部实现（Go 语言级私有）
│   ├── ooxml/                            # 格式层：OOXML 类型模型（生成）
│   │   ├── schema/                       #   ECMA-376 XSD → go:generate 生成 CT_* 类型
│   │   ├── part/                         #   Part 层级（PresentationPart/SlidePart/…）
│   │   ├── rels/                         #   关系图 + content-types
│   │   └── docprops/                     #   app/core/custom 元数据
│   ├── document/                         # 域层：业务逻辑（无裸 XML）
│   │   ├── model/                        #   纯内存对象（Slide/Shape/Text/…）
│   │   ├── text/  style/  geometry/      #   按功能垂直切片子包
│   │   ├── shapes/  table/  chart/  media/  layout/
│   │   └── bind/                         #   数据绑定引擎（plan/apply）
│   ├── opc/  xmlstore/                   # 传输/存储层（现状保留，位置下沉）
│   ├── engine/                           # 编排层：Open/Save/Bind/Clone 跨域事务
│   ├── ir/  diff/                        # 只读 IR / 语义 diff（仅吃 ooxml）
│   └── diag/  capa/                      # 跨层只向上（被门面与工具消费）
├── pptx/                                 # 公共门面（唯一被外部 import 的包）
│   ├── pptx.go  text.go  media.go  table.go  chart.go
│   ├── options.go  report.go             # *Spec/*Options/*Info/*Report
│   ├── doc.go                            # 包文档 + API 分级说明
│   └── *_test.go                         # 门面契约（黄金计数/行为/示例）
├── render/                               # 公共渲染适配器契约（保留；只依赖 pptx 门面）
├── cmd/pptx  cmd/pptx_check              # 薄入口（flag 解析 + 编排 engine）
├── wasm/site/                            # 离线静态 UI（现状保留）
├── scripts/gen|l3|coverage|release/      # 工程脚本按用途分目录
└── docs/                                 # 现状保留（ADR/设计/路线图/矩阵）
```

### 六条关键机制

1. **门面薄、域厚**。`pptx/` 只放稳定类型与薄委托（方法体 ≈ 1 行，转调 `internal/document`）；业务逻辑全部下沉。公共签名冻结后实现可无限迭代。参照 python-pptx 内部结构（`pptx/` + `pptx/oxml/` + `pptx/opc/`）。
2. **OOXML 类型生成 = 最大杠杆（限只读投影）**。淘汰"stringly-typed XML"反模式——几十个手写 `*_parse.go` / `text_node.go` nodeStep 路径，替换为从 ECMA-376 XSD 生成的 typed `encoding/xml` 结构（同 excelize 路线），配少量微软扩展补丁（p14/morph）。`schema/` 一个目录替代全部手写解析。**硬约束：生成类型仅用于只读投影（替换手写 parse），不得进入写路径**——`encoding/xml` 往返不保证属性序、命名空间前缀选择、自闭合形式与空白不变；一旦生成类型参与写入，`corpus_b1_test.go`（空变更集→字节恒等）、`internal/opc/saveplan_test.go:TestSavePlanUnchangedIsB1`、ext-0024 单 Run 替换 1 字节差异这三条字节级保真证据立即失效。**写路径维持 `xmlstore` span 补丁 + 未修改 Part 字节拷贝。**
3. **测试随包走**。每个 internal 子包自带白盒测试 → **ADR-016 覆盖率归属陷阱从根上消失**（代码与测试一起搬，覆盖率按包归属天然正确）。`internal/chart` 5.5% 的历史悲剧不再复现。
4. **依赖单向、CI 强制**。门面→域→格式→传输；`ir/diff` 只吃 `ooxml`、`render` 只依赖门面；`diag/capa` 只向上。用 **std-lib 自建依赖规则 job**（`go/parser` + `go list`，与 `api_surface_test.go` 同手法）在 CI 执行，防回潮——**不引入 `go-arch-lint` 等外部工具**，遵守 `ci.yml` 零依赖政策。
5. **cmd/wasm 共享 engine**。CLI 六子命令与 WASM check 不再各自贴门面，统一编排 `internal/engine`。
6. **模块根变元仓库**。`internal/` 受语言级保护；公共面收窄为一个 `pptx/` 子包——"63 文件根包"的结构压力自动归零，未来文件增长被分摊到各子域包。

### 现有投资处置

| 现状 | 目标 | 说明 |
|---|---|---|
| 根包 63 文件 | 拆分：`pptx/` 门面（≈8-10 文件）+ `internal/document/*` | 公共类型搬门面，业务/私有实现下沉 |
| `internal/opc` / `xmlstore` | 保留 | 传输/存储层，位置不动 |
| `internal/document` | 扩容为域层根 | 吸纳根包全部业务逻辑 |
| `internal/chart` | 并回 `document/chart` | 聚合为域层一部分（当前 90.8% 已达标，非为救覆盖率） |
| `internal/audioprobe` / `videoprobe` / `textmap` / `editplan` | 归入 `document/media` / `document/text` / `engine` | 按功能收编 |
| 顶层 `ir/` | 移入 `internal/ir` | **实为改造**：现 `ir/ir.go`、`ir/diff.go` import 根包，须重写为只吃 `ooxml`，且依赖演进第 3 步门面收敛 |
| 顶层 `render/` | **保留**（公共契约） | 非意外死代码：M8 RENDER-01 有意发布的**渲染适配器接口契约**（`Renderer`/`RenderOptions`/`RenderCapabilities`/`RenderedSlide`/`RenderAll`），自述「仅定义契约，不含实现」，`render_test.go` 6 用例覆盖。v1.x 已发布 → 删除即 breaking；指定消费者=后续立项的原生渲染实现（V2.6 §23.1 / ADR-014）与第三方适配器。定案见验收口径 §4 |
| `api_surface_test.go` 黄金计数 | 随迁 `pptx/` 门面包 | 计数语义不变 |
| ADR-029（同包拆文件） | v1.x 权宜，v2.0 由分层拆分取代 | 策略边界见本 ADR |
| `docs/` / `scripts/` / `wasm/site/` / `testdata/` | 保留 | 工程层不受影响 |

### 依赖方向（强制单向，禁止反向）

```
pptx (门面) ─→ internal/document (域) ─→ internal/ooxml (格式) ─→ internal/opc + internal/xmlstore (传输)
                    ↑
     internal/engine 编排跨域事务（Open/Save/Bind/Clone）
cmd/pptx ─→ internal/engine
wasm/check ─→ internal/engine（纯函数子集）
internal/ir / diff ─→ internal/ooxml（只读投影，不依赖门面）
render ─→ pptx（公共契约只依赖门面，门面不反向依赖 render）
internal/diag / internal/capa ─→ 仅被门面与工具消费
```

### 演进顺序（v2.0 启动后按此推进）

1. **立 `ooxml/schema` 生成管线**（只读投影，见机制 2；最慢、最独立，可并行推进）。
   _2026-09-16 实测：ECMA-376 官方 XSD 下载链接失效，输入获取待解——本步**不阻塞**
   逐域搬迁，先并行推进第 2 步。_
   _**2026-09-17 更新：输入已解**——需的是 **Transitional** 而非 Strict；Part 4
   （`ECMA-376-4_5th_edition_december_2016.zip` → `OfficeOpenXML-XMLSchema-Transitional.zip`）
   使用 `schemas.openxmlformats.org` 命名空间（与真实 PPTX 一致），Part 1 附的 Strict
   用 `purl.oclc.org`，不可用。管线已落地，见下文「ooxml 生成管线」。_
2. **逐域搬迁**：`geometry` → `style` → `text` → `media` → `table/chart` → `bind`，每域一个 PR，白盒测试随行（ADR-016 陷阱免疫）。
3. **收敛门面**：根包 → `pptx/` 子包（唯一的 breaking 点），同步迁移 `api_surface_test`；中间态按验收口径 §2 维护 golden。
4. **收编/处置顶层包**：`ir/` 改造入 `internal/ir`（**依赖第 3 步门面收敛**——现 import 根包，须先有 `pptx/` 门面可依赖）；`render/` **保留**（有意发布的公共契约，见需求边界表；非意外死代码）。
5. **engine 化 cmd/wasm**：CLI 与 WASM 改编排 `internal/engine`。
6. **CI 加依赖方向校验**（std-lib 自建规则 job，零新依赖，见机制 4）。
7. **gate 完整性收口**：`go list ./...` 集合 ⊄ FLOORS → FAIL（堵住新包静默不受检），新包按门槛档位表入表。**已于 v1.x 提前落地**（2026-09-16，含显式 `SKIP` 豁免清单）。

### 新包覆盖率门槛档位（2026-09-16 补齐，阻塞项）

现状 `scripts/coverage/gate.sh` 的 `FLOORS` 是**白名单**：未列包**静默不受检**，且档位只有 root/cmd/低层格式三档。v2.0 逐域搬迁会不断产生新包，必须先定义档位：

| 档位 | 门槛 | 适用包 |
|---|---|---|
| T1 公共门面 | ≥82 | `pptx/`（承接现 root 档） |
| T2 域层包 | ≥85 | `internal/document/*`、`internal/bind`、`internal/engine` |
| T3 低层格式包 | ≥90 | `internal/ooxml/*`、`opc`、`xmlstore`、`textmap`、`editplan` |
| T4 工具/只读包 | ≥85 | `internal/ir`、`internal/diff`、`cmd/*`、`wasm/check` |
| 防回归下界 | 取现测值 | `chart`（90.8）、`render`（84，**保留**）等行为优先包 |

搬迁时点依据（2026-09-16 按域归集：语句数 / 域覆盖率 / 搬走后 root 覆盖率）：

```
  media  1519/85.3% → root 83.6%     style   410/85.9% → 83.8%
  text   1436/82.7% → root 84.2%     clone   378/81.5% → 84.0%
  table   690/85.4% → root 83.8%     format  370/76.5% → 84.2%
  geom    613/87.3% → root 83.6%     chart   213/77.5% → 84.1%
  bind    575/86.4% → root 83.7%     theme   204/78.9% → 84.0%
```

推论：任何单域搬走，root 覆盖率**上升或持平**（83.6–84.2%，均 ≥82% 门槛）——
ADR-016 归属陷阱在「单域 + 测试随迁」下不发作；但若新包套用 T3（≥90%），上表
无一域一次达标（最高 geom 87.3%），故域层包定 **T2（≥85%）**。最差四域
format 76.5 / chart 77.5 / theme 78.9 / clone 81.5 **应先补测试再搬**。

### 零外部依赖不变量

仓库现状零外部依赖（`go.mod` 无 `require`、无 `go.sum`；`go list -m all` 仅主模块）。
本 ADR 将「**运行时 + 构建期零外部依赖**（工具链仅 Go 标准库）」立为不变量：

- 依赖方向校验用 std-lib 自建 job（机制 4），不引入 `go-arch-lint`；
- `ooxml/schema` 生成管线若实现，优先自研最小 XSD→Go 生成器（XSD 本身是 XML，
  `encoding/xml` 可解析）；确需现成工具时放独立 tool module（不进主 `go.mod`/`go.sum`）；
- 新代码引入任何第三方包前须先走 ADR 评审。

## 后果

### 正面

- 层存在：门面签名与实现解耦，v2.1+ 的能力扩展不再堆进单一无层包；
- 覆盖率归属正确：测试随包走，各 internal 包独立达标，ADR-016 陷阱根除；
- 公共面收窄且可审计：外部可见符号 = `pptx/` 一个包，capability manifest 与文档一致性维护成本下降；
- 类型安全提升：XSD 生成的 CT_* 类型消灭手写 nodeStep/解析（**只读投影**）；写入侧字节级保真仍由 `xmlstore` 补丁与 Part 字节拷贝保障（见机制 2 硬约束）。

### 负面 / 风险

- **破坏性变更**：`github.com/F31/go-pptx` → `github.com/F31/go-pptx/pptx`，所有 importer 需迁移（v2.0 一次性）；
- 搬迁量大（根包 63 文件 + 顶层 2 包 + 测试），review 与回归成本集中；
- 历史文档/脚本中旧路径引用需随迁（ADR-029 已记录同名问题的处理口径）；
- `ooxml/schema` 生成器是长期资产，短期投入大、见效慢——属 v2.0 周期的前期投入。

### 验收口径（v2.0 启动前必须补齐）

1. **新包门槛档位表**：见 §决策——按 T1..T4 入表。**gate 完整性校验已在 v1.x
   提前落地**（2026-09-16，`gate.sh`：`go list ./...` ⊄ FLOORS∪SKIP → FAIL）；
   v2.0 只需把新包按档位值登记。
2. **中间态 golden 维护**：逐域搬迁期间长期存在「部分类型在 `pptx/`、部分在根包」的
   双门面，`api_surface_test` 黄金计数（158 类型 / 506 导出函数 / 131 Stable 方法）与
   ADR-015 分级如何跨中间态维护——每个搬迁 PR 的验收必须含「golden 不变或按 ADR 流程
   显式变更」声明。
3. **ir 重写依赖**：`ir` 从 import 根包改为只吃 `ooxml`，须在门面收敛（演进第 3 步）后
   进行；提前迁移将无 `pptx/` 门面可依赖。
4. **render 决策（2026-09-16 已定案：保留）**：非意外死代码，而是 M8 RENDER-01 有意
   发布的**公共接口契约**（`render.go` 自述「仅定义契约，不含渲染实现」，6 用例覆盖，
   白皮书 / `v1.0-bug-registry.md` / `architecture-current.md` 均有记载）。v1.x 已发布
   → 删除即 breaking；指定消费者=后续立项的原生渲染实现（V2.6 §23.1 / ADR-014）与第三方
   适配器。「降 `internal/render` 藏死代码」不采纳——它本就非死代码，且降 internal 会切断
   已发布的公共导入路径。

### 触发条件（何时正式启动 v2.0）

任一命中即启动（**2026-09-16 实测：三条均未命中**）：

1. **v1.x 出现需要动门面签名的新能力**——最近提交均为文档/重构收尾，无门面签名变更，未触发；
2. **根包编译/导航成本明确受限**——实测热重建 0.29s ≪ 5s、根包非测试文件 63 < 80，未触发；
3. **外部明确需要 `ir` 作为公共 API**（`render` 已定案保留，不再是决策项）——实测
   `ir` 仅被 `cmd/pptx` 与 `wasm/check` 引用、`render` 零包外引用，未触发。

### 后续动作

- 本文为 v2.0 **提案**；v1.x 期间不实施，继续按 ADR-014/029 维护；
- **试点（2026-09-16 修订后选定；kimi 评审后更正）**：`text_util.go` → `internal/textutil/` 迁移。
  原选 `bind` 私有实现（5 文件零导出）因 Go 方法必须在类型所在包定义的硬约束
  导致循环依赖（`bindScanner` 定义于根包 `bind.go`，迁入 `internal/bind` 后根包
  `bind.go` 须 import `internal/bind` 以调用方法 → 包循环），改选 `text_util.go`
  （136 行 / 零导出 / 零方法 / 6 个自由函数 / 域覆盖 82.5%）。
- 试点结果（2026-09-16 实测）：
  - 12 个调用方文件更新 import（9 根 + 2 internal/chart + 1 cmd 脚本无关），
    测试随迁至 `internal/textutil/util_test.go`（package textutil，白盒）；
    `internal/chart/codec_test.go` 保留本地副本测试（chart 自有 xmlUnescape 拷贝
    与根包同步维护的契约不变）。
  - `go test ./...` 与 `-tags=corpus` 15/15 全绿；`api_surface_test` golden 不变
    （零公共 API 变更）。
  - `internal/textutil` 覆盖率 82.5%（FLOORS 设 T1=82 暂放，待补 `RemoveAttrPatch`
    / `RemoveElementPatch` / `FirstChildOf` 测试后有望达 T2=85）。
  - 验证两件事：① 测试随包走 → 覆盖率归属正确（textutil 独立计数，root 从 83.9%
    微调到 83.8%，符合预期）；② 依赖方向可强制（根包单向 import `internal/textutil`，
    无反向）。
- 边界：验不了 v2.0 核心前提「公共类型真实移动 + 薄委托门面」——那只在真 breaking
  版本验，不混入本次试点。

### 试点续：`bind_marker.go` → `internal/bind/`（2026-09-16 第二批）

同类迁移第二例——`bind_marker.go`（112 行，零导出/零方法）迁入 `internal/bind`：

- 迁出内容：`TplToken`（字段导出 `Literal`/`Path`）、`ParseDirective`、`ScanInline`、
  `DeletePatch`、`EmptyParaPatch`、`ChildElems`；原定义于根包 `bind.go` 的定界符常量
  `TplMarkOpen`/`TplMarkClose` 随实现一并下沉（导出后根包引用 `bind.TplMarkOpen`）。
- 调用方更新：`bind.go`（移除常量定义）、`bind_body.go`、`bind_table.go`、`create.go`
  （`deletePatch` 跨域调用点）。
- 测试随迁：`internal/bind/marker_test.go`（`TestBindPatchHelpers`、
  `TestBindPureParseDirectiveAndScanInline`，见 `internal/bind` 覆盖率 **95.5%**）。
- 守恒：`go test ./...` 与 `-tags=corpus` **16/16** 全绿；`api_surface_test` golden 不变
  （零公共 API 变更）；gate 新增 `internal/bind=90`（实测 95.5%）。
- 关键约束复证：**Go 方法必须在类型所在包定义**——`bindScanner` 系列方法（bind_body/
  table/chart/resolve）仍无法迁出（其类型定义在根包 `bind.go`），故 bind 域只能逐文件
  渐进而非整域搬迁。本批证明「零方法 + 零根类型依赖」的文件可独立迁出。

### 试点续二：共享 helper 下沉 + `theme_placeholder.go` → `internal/style/`（2026-09-16 第三批）

**前置：共享 helper 下沉**（commit `c20e9f5`）。前两批迁出后，剩余「零方法」文件都依赖
根包 helper（`childOfKind` 172 处、`ns*` 41 文件、`parseUint32`、`styleKind*`），直接迁会
因 root↔internal 双向 import 成环。故先把跨包 helper 下沉：

- `internal/xmlstore/dom.go`：`ChildOfKind` / `CountKind` / `ParseUint32`（+包内测试）
- `internal/ooxmlns/ns.go`：OOXML/OPC 命名空间 URI 常量
- 根包改薄委托/别名（零调用方改动）；`internal/chart` 的重复实现同步收敛

**迁移：`theme_placeholder.go` → `internal/style/placeholder.go`**（135 行）。与前两批不同，
本文件的类型（`phKey`/`textClass`）虽非公开，但被根包 `shape.go`/`style_resolve.go`/
`theme_resolve.go` 使用，需**导出类型与字段**（`PhKey{Typ,Idx}`、`TextClass`、`StyleKind*`、
`ClassTitle/Body/Other/Notes`）并更新调用方：

- 导出：`PhKey` / `PhKeyOf` / `FindPlaceholderShape` / `AncestorShape` / `RunPara` /
  `ParaLevel` / `TextClass` / `ClassOf` / `StyleKindSlide` / `StyleKindNotes`
- 调用方更新：`style_resolve.go`（含 effState 字段类型）、`shape.go`（`shapePhKey`）、
  `theme_resolve.go`、`style_env.go`、`style.go`（移除 styleKind 常量）、`style_test.go`、
  `table_style_test.go`
- 测试随迁：`internal/style/placeholder_test.go`，覆盖率 **94.6%**；gate 新增
  `internal/style=90`
- 守恒：`go test ./...` 与 `-tags=corpus` **17/17** 全绿；`api_surface_test` golden 不变

**小结**：三批试点共迁出 3 个文件 + 建立 3 个共享/内部包。剩余「零方法」文件
（`geom_parse/fill/effect.go`、`chart_frag.go`、`media_audioicon.go`）**全部引用根包
公开类型**（`GeometryInfo`/`FillInfo`/`EffectInfo`/`ChartData`/`imageKind` 等），在 v1.x
冻结面内**无法迁出**——它们要等 v2.0 破坏性版本连同公共类型一起搬。**clean 迁移集已尽。**
- **render 决策**（2026-09-16 已定案）：**保留**为公共契约（理由见验收口径 §4）；
- 启动时同步维护 1.x→2.0 迁移文档（import 路径改写 + golden 计数随迁）。

### 别名策略（2026-09-16 第 3 轮明确）

早先表述「不是 alias，是真实移动 + 薄委托」需按类型性质二分（Go 语言约束所致）：

| 类型性质 | 例 | 策略 | 理由 |
|---|---|---|---|
| **句柄/行为类型** | `Presentation` / `Slide` / `Shape` / `TextFrame` / `TableShape` | **门面真实定义**（struct 持 internal 状态），方法体 ≈ 1 行转调 `internal/document` | 方法必须定义在类型所在包；alias 无法附加方法，做不了薄委托 |
| **DTO/值类型** | `Diagnostic` / `Severity` / `SlideID` / `GeometryInfo` / `Point` / `EMU` | internal 定义 + 门面 `type X = pkg.X` **alias 暴露** | 纯数据无行为；alias 零成本、零转换、零重复定义。若强行"真实移动 + 转换"，将为 ~160 个 DTO 制造双份定义与逐字段转换样板，违背可维护性 |

配套：`api_surface_test` 升级为**别名感知**（`addAliasMethods` 解析根包 import 的
模块内包，按接收者类型名补入 alias 目标的导出方法），保证 DTO alias 不丢公共方法面。

### 地基包：`internal/document/model` + `internal/diag`（2026-09-16 第 3 轮）

逐域搬迁前必须先立被多域共享的底层类型包——否则域包无法命名 `Diagnostic` /
`SlideID` 等公共面类型（internal 不得 import 根包，ADR-016 陷阱的变体）：

- `internal/document/model`：`SlideID` / `ShapeID`（纯标识值类型，零依赖）
- `internal/diag`：`Severity` / `Diagnostic` / `ValidationReport`（跨层诊断，只被向上
  消费；依赖 model）
- 根包 `ids.go` / `diagnostics.go` 改 alias 暴露（Stable 文档段保留）；**零调用方改动**
- `api_surface_test` 升级为别名感知（见上）；golden（163 type / 40 Stable 段 / 60 符号
  / 131 方法 / 17 哨兵）**零变更**
- gate：`internal/diag=90`（实测 100%）、`internal/document/model` 无可测语句入 SKIP
- 守恒：`go test ./...` 与 `-tags=corpus` **18/18** 全绿；`api_surface_test` golden 不变

### 域搬迁 1：geometry → `internal/document/geometry`（2026-09-16）

第一个域。边界：**纯几何**（度量单位 + prstGeom/custGeom 只读解析）；填充/效果解析
（依赖 `styleEnv`）随 style 域后迁。

- 迁出：`EMU`（+`Inches`/`Points`）、`Point`、`GeometryKind`（+`String`）、`GeometryInfo`、
  `GeomAdjust`、`GeomGuide`、`GeomPath`、`PathCommand`；`geom_parse.go` 全部解析
  （`ParseShapeGeometry` + adjust/guide/path/command/point 辅助）
- 根包：`geometry_alias.go` 以 alias 暴露上述 DTO（`EMU`/`Point` 的 Stable 文档段保留）；
  `geom_parse.go` 瘦身为仅 `spPrOf`/`fillContainer`（填充/效果解析仍在本包）；
  `shapeNode.Geometry()` 转调 `geometry.ParseShapeGeometry`
- `EMUFromInches`/`EMUFromPoints`/`scaleEMU` 暂留根包（依赖公共错误类型
  `OperationError`/`ErrInvalidArgument`/`ErrLimitExceeded`——错误类型下沉是独立地基项）
- 测试随迁：`internal/document/geometry/parse_test.go`，覆盖率 **91.2%**；gate 增
  `internal/document/geometry=90`
- 附带清理：别名使 `Point` 成跨包类型，`go vet` 的 composites 规则暴露 `geom_test.go`
  未命名结构体字面量 → 改 keyed
- 守恒：`go test ./...` 与 `-tags=corpus` **19/19** 全绿；`api_surface_test` golden 不变

### 域搬迁 2：style（切片 1：段落属性）（2026-09-16）

style 域庞大且与 `Presentation`/`styleEnv` 深度耦合，按切片推进。切片 1 取**零根类型
依赖**的段落属性解析：

- 前置：`EMU` 从 geometry 迁至 **`internal/document/model`**（共享度量单位；geometry
  内部以 `type EMU = model.EMU` 别名继续使用，根包 alias 指向 model）
- 前置：通用 `intAttr` 下沉为 `xmlstore.IntAttr`（4 处调用点更新）
- 迁出：`Spacing` / `BulletKind`(+`String`) / `Bullet` / `TabStop` / `ParagraphProps`
  + `ParseParagraphProps`（原 `Paragraph.Props()` 的解析体）→ `internal/document/style`
- 根包：`format_paraprops.go` 改 alias + `Paragraph.Props()` 薄委托
- 测试随迁 `internal/document/style/paraprops_test.go`（**94.9%**）；gate 增
  `internal/document/style=90`
- 附带：新增 `xmlstore.IntAttr` 使 xmlstore 覆盖率短暂跌破 90（ADR-016 陷阱），补
  `TestIntAttr` 后回到 **90.7%**
- 守恒：`go test ./...` 与 `-tags=corpus` **21/21** 全绿；`api_surface_test` golden 不变

### 域搬迁 2 续：styleEnv 解耦 + `internal/style` 并入（2026-09-16）

切片 2 的前置是把 `styleEnv` 从 `Presentation` 解耦：

- **包名冲突**：试点包 `internal/style`（`theme_placeholder.go`）与域包
  `internal/document/style` 同名 → **并入**域包（本应同域）。gate 移除
  `internal/style`，`internal/document/style` 保留（实测 95.2%）。
- **解耦手法（函数式关系源）**：`styleEnv` → `style.Env`（导出字段
  Kind/Layout/Master/Theme）；解析逻辑 `ResolveEnv(part, RelsFunc)` 注入
  `RelsFunc = func(opc.PartName) ([]*opc.Relationship, bool, error)`。
  根包 `Presentation.styleEnv(part)` 薄委托 `style.ResolveEnv(part, p.relsOf)`，
  **零公共 API 变更**（`relsOf` 仍为私有方法，以方法值注入）。
- **常量归位**：`relNotesMaster`（根包私有）上移为 `opc.RelNotesMaster`
  （与 RelTheme/RelSlideMaster/RelSlideLayout 一致）；`clone.go`/`text_notes.go`
  调用点更新。
- 8 个根文件随 `*styleEnv`→`*style.Env` 与字段导出名更新；`go vet`/gofmt 干净
- 测试：`internal/document/style/env_test.go`（ResolveEnv slide/notes 链、无 rels、
  错误、layout rels 失败忽略、ThemeOf）
- 守恒：`go test ./...` 与 `-tags=corpus` **20/20** 全绿；`api_surface_test` golden 不变

### 域搬迁 2 续二：style 核心类型 + 主题纯函数（2026-09-16）

切片 2b（先做不依赖颜色机器的部分）：

- 迁出 `internal/document/style/step.go`：`StyleSource`(+`String`) + 7 个来源常量 + `StyleStep`
- 迁出 `internal/document/style/theme.go`：`MasterClrMap` / `TextStyleNode` /
  `DefRPrAtLevel` / `LstStyleOf` / `ThemeFontFace` / `ExpandTypeface` / `FontSchemeName`
  （改用 `xmlstore.ChildOfKind` + `ooxmlns.*`；`Replace` 保持 `StyleStep` 同包）
- 根包：`style.go` 以 alias 暴露 `StyleSource`/`StyleStep` 与来源常量（零调用方改动）；
  `theme_resolve.go` 仅保留 `themeDoc`/`masterDoc`（Presentation 绑定）+ `schemeRGB`
  颜色解析；调用点加 `style.` 前缀（style_resolve/table_style/format_color）
- 暂留根包（下一子切片）：`schemeRGB` 及其颜色变换机器（`applyColorTransforms` 等）与
  `parseColorNode`（依赖公共 `ColorSpec`）
- 测试随迁 `internal/document/style/theme_test.go`（**94.3%** 总覆盖）
- 守恒：`go test ./...` 与 `-tags=corpus` **20/20** 全绿；`api_surface_test` golden 不变

### 域搬迁 2 续三：颜色解析全集 + ColorSpec（2026-09-16）

切片 2b-ii：把颜色解析与变换全集下沉，前提是 `ColorSpec` 一并下沉（`ParsedColor`/
`ResolvedColor` 引用它）。

- `ColorSpec` 从 text 域（`text_fonttypes.go`）下沉至 `internal/document/style`
  （根包 alias；`String`/`Valid` 经别名感知测试保留）
- 迁出 `internal/document/style/color.go`：`ColorTransform` / `ParsedColor` /
  `ApplyColorTransforms`（28 种变换全集）/ `SchemeRGB` / `IsHexRGB` / `Clamp01` /
  `ClrMapIndirect` + 私有辅助（scrgb/hsl 通道、predefined 色、rgb↔hsl）
- `parseColorNode` → `style.ParseColorNode(doc, clr, themeDoc, masterDoc, part, diags)`
  （文档由门面注入）；根包 `Presentation.parseColorNode` 薄委托，**零调用方改动**
  （geom_fill/geom_effect/style_adv/style_matrix/format/format_runprops）
- `theme_resolve.go` 仅保留 `themeDoc`/`masterDoc` 接线
- 测试随代码迁：`TestTransformSetComplete`（ECMA 28 全集）移入包内；
  新增 `color_test.go` 覆盖变换/`SchemeRGB`/`ParseColorNode` 全分支；覆盖率 **92.7%**
- 守恒：`go test ./...` 与 `-tags=corpus` **20/20** 全绿；`api_surface_test` golden 不变

### 域搬迁 2 续四：font 类型 + effState 有效样式解析（2026-09-16）

把 STYLE-01 有效样式解析完整下沉，前提是 font 类型一并下沉：

- **`Optional[T]` 从 internal/chart 迁至 `internal/document/model`**（generic 值包装，
  与 chart 解耦；chart/根包以 alias 引用）——消除 style→chart 的不正依赖
- 迁出 `internal/document/style`：
  - `fonttype.go`：`FontSize` / `FontProperty` / `FontStyle`(+`String`/`AnySet`/`AnyChildSet`)
  - `resolved.go`：`ResolvedValue[T]` / `ResolvedColor` / `ResolvedFont` / `ResolveContext`
  - `parsefont.go`：`ParseLocalFont` + `parseSolidFill`/`parseCentipoints`（原 text_fontparse）
  - `effstate.go`：`effState` 状态机 + `ResolveEffectiveFont(ctx, env, docs, doc, run, part)`
- **解耦手法（函数式文档源）**：`DocFunc = func(opc.PartName) *xmlstore.XMLDocument` 注入，
  替代 `p.docOf`/`p.masterDoc`/`p.themeDoc` 三处根绑定；`style_resolve.go` 整文件删除
- 根包：`text_fonttypes.go`/`style.go` 以 alias 暴露；`TextRun.EffectiveFont` 薄委托；
  `TextRun.ExplicitFont` 转调 `style.ParseLocalFont`（注意 `style` 在该文件是参数名，
  仅包路径处引用不受影响）
- 命名冲突处置：`CoreProperties.anySet`（docProps，另一类型）未被误伤；`FontStyle.
  anySet/anyChildSet` 上移为导出方法并更新调用点
- 测试：`internal/document/style/effstate_test.go`（L1/L2/L3/L4 全链、schemeClr 间接色、
  缺 clrMap、主题引用展开、strict、fallback）；`model` 补 EMU/Optional 测试 → **100%**，
  由 SKIP 移入 FLOORS=90
- 守恒：`go test ./...` 与 `-tags=corpus` **21/21** 全绿；`api_surface_test` golden 不变；
  `internal/document/style` **92.3%**

### 域搬迁 3：样式访问器 1+2 与填充/效果下沉（2026-09-17）

- **访问器 1**（commit be6ef6f）：runprops/line/styleMatrix 迁入
  `internal/document/style`：
  - `RunProps`/`RunSymbol` + `ParseRunProps`（format_runprops.go）
  - `LineStyle`/`LineEnd` + `ParseShapeLine`（format.go 线条系统）
  - `MatrixRefKind`/`StyleMatrixRef`/`ThemeFontSlot` + `ParseStyleMatrixRefs`
    （style_matrix.go + style_adv.go 合并，含 phClr 替换/字体槽位）
  - 解耦：`themeDocOf`/`masterDocOf`/`ColorChild` + `styleDocs() DocFunc`
    注入，替代 p.docOf/themeDoc/masterDoc；根 `colorChildOf` 并入
    `style.ColorChild`；style_adv.go 删除
- **访问器 2**（commit 32332b2）：表格样式迁入 `internal/document/style`
  （table.go）：StyleToggle/TableStyleFlags/StylePart/FillKind/CellFill/
  CellBorder/CellText/CellBorders/EffectiveCellStyle + `ResolveEffectiveCellStyle`
  + `CellStyleCtx`（Env/Docs/StyleDoc/Part/Geom 注入），根保留
  tblStyleLstDoc/tableGeom 接线；`ParseToggle`/`TablePrNode`/`AncestorOf` 导出
- **填充/效果**（本次）：GEOM-02 fill/effect 迁入 `internal/document/geometry`
  （fill.go/effect.go）：FillInfo/GradientFill/PatternFill/BlipFillInfo +
  `ParseShapeFill`、EffectKind/EffectInfo/Effect/Scene3D/Shape3D/Bevel + 
  `ParseShapeEffects`；颜色经 `style.ResolveColor`（新增 Env+Docs 封装）；
  geom_fill.go/geom_effect.go/geom_parse.go 删除；`Optional[T]`/`NewOptional`
  与 geometry 内部别名
- 命名守恒：全部 DTO 类型经根别名暴露（style_dto.go / geom_adv.go /
  FillKind→style）；golden 零变更
- 测试随迁：style accessors_test.go（矩阵链/phClr/runprops/line）→ 90.5%、
  table_test.go（resolveColorSpec 12 分支/显式/样式库/unresolved）→ 90.5%、
  geometry fill_effect_test.go（fill 全形态/渐变路径/图案/blip/效果全集/
  scene3d+sp3d）→ 92.2%
- 守恒：`go test ./...` 21/21 + corpus；api_surface golden 不变；gate PASS

### 域搬迁 4：文本域（2026-09-17）

新建 `internal/document/text`（文本垂直切片；句柄 TextFrame/Paragraph/
TextRun/Field 仍留根门面，纯函数下沉、根方法薄委托）：

- **切片 A（字符格式写入）**：
  - 片段构造/解析从 root `text_fontparse.go` 迁入 `font.go`：
    `RPrChildRank`/`FillChildOf`/`SolidFillFragment`/`BuildRPrFragment`/
    `BoolVal`/`SizeCentipoints`/`IntString`/`IsNSDeclAttr` + rank 常量
  - 写入补丁计算从 root `text_font.go` 迁入 `fontwrite.go`：
    `BuildFontPatches`/`PatchExistingRPr`/`SetRPrAttr`/`InsertRPrChild`/
    `ExpandSelfClosingRPr`/`RPrChildrenFragment`；`text_fontparse.go` 删除；
    根 `TextRun.SetFont` 只留 locate+ApplyPatches+提交，helper 全委托
  - 依赖解耦：`nsDrawingML`→`ooxmlns.DrawingML`、`Annotate`/
    `mapXMLError`/`OperationError`/哨兵→`internal/errs`；根调用点经
    alias `textpkg` 收敛（`intString` 同时供 shape/text_replace）
- **切片 B（文本框/字段/段落纯辅助）**：
  - `body.go`：`BodyProps` DTO + `VertAllowed`/`BodyPropsAnySet`/
    `ParseBodyProps`/`ApplyBodyPropsPatch`/`SetAttrPatch`/
    `RemoveAttrIfExists`/`BuildBodyPrFragment`/`NSPrefix`
  - `field.go`：`FieldKind`/字段常量/`FieldSpec` + `ValidateFieldSpec`/
    `BuildFieldFragment`（datetime guide 白名单随迁）
  - `fragment.go`：`BodyRawShapeOK`/`ParaPrefix`/`BuildPlainParagraph`/
    `RunPrefix`/`EndParaAnchor`/`ParagraphText`
  - 命名守恒：`BodyProps`/`FieldKind`/`FieldSpec` 根别名暴露（text_adv.go）；
    `Paragraph.Text`/`SetPlainText`/`AddRun`、`TextFrame.BodyProps`/
    `SetBodyProps`、`Paragraph.AppendField`/`InsertField` 改为薄委托
- 测试随迁（ADR-016 陷阱免疫）：`TestRPrChildRank`/`TestSizeCentipoints`
  （原 text_test.go）、`TestNsPrefix`（原 text_adv_test.go）迁入；新增
  `font_test.go`/`body_test.go` 白盒用例
- 门槛：`internal/document/text` 入 gate `FLOORS=90`（实测 **94.0%**）
- 守恒：`go test ./...` 22/22 + corpus 22；api_surface golden 不变；gate PASS

### 域搬迁 5：媒体域 切片 1——媒体输入契约与图片探测（2026-09-17）

新建 `internal/document/media`（媒体垂直切片起步）。媒体域的句柄/编排
（AudioShape/VideoShape/PictureShape 与 `*Presentation`/`*Slide` 的
Add*/plan* 方法）与 `Presentation` 深度耦合，须待第 3 步门面收敛才能整体
下沉；本切片先搬**无状态、自包含**的输入契约与探测原语：

- `media.go`：`MediaSource` 接口 + `FileMedia`/`BytesMedia`/`FuncMedia`/
  `ReaderMedia` 适配器；`ReadMedia`/`ReadBounded` 有界复制；
  `ImageKind` + `ProbeImage`/`SniffImage`/`ImageTypeEqual`；
  `MaxStagingBytes`/`RelImage`/`CTImagePNG`/`CTImageJPEG`
- 解耦：`Annotate`/`OperationError`/哨兵 → `internal/errs`；`RelTypePrefix`
  经 `internal/opc`
- 命名守恒：`MediaSource` 根别名；`imageKind` 根别名（字段改导出
  `CT/Ext/Width/Hgt/ByteSize`，root 调用点同步）；常量根别名
- 根 `media.go`（234 → 92 行）保留同名薄委托（`FileMedia`/`BytesMedia`/
  `FuncMedia`/`ReaderMedia` + `readMedia`/`readBounded`/`probeImage`/
  `sniffImage`/`imageTypeEqual`），消费方零改动
- 门槛：`internal/document/media` 入 gate `FLOORS=90`（实测 **95.8%**）
- 守恒：`go test ./...` 23/23 + corpus 23；api_surface golden 不变；gate PASS
- 后续媒体切片（profile DTO/片段构造/编排）待门面收敛后推进

### 域搬迁 6：表格域 切片 1——逻辑网格与单元格纯辅助（2026-09-17）

新建 `internal/document/table`，搬入表格域最实质的纯逻辑：a:tbl 逻辑网格
（gridSpan/rowSpan/hMerge/vMerge → 行×列映射）与单元格辅助。`TableShape`/
`Cell` 句柄与 Merge/Unmerge 事务仍留根门面（依赖 shapeNode + 补丁提交）。

- `grid.go`：`Slot`/`Grid`（导出字段 TC/Anchor/Row/Col/IsContinuation/
  Cols/Slots/TRs/GCols + `Rows`/`At`）；`BuildGrid`/`InferCols`/`CellSpans`/
  `CellIsContinuation`/`CellHasText`/`ClearCellTextPatches`/`TableOfGraphic`
  + `GraphicURI`
- 解耦：`nsDrawingML`→`ooxmlns.DrawingML`、`childOfKind`→
  `xmlstore.ChildOfKind`；根 `tblGraphicURI` 删除（改用 `table.GraphicURI`）
- 根 `table.go` 858 → ~600 行：网格类型/handler 移除，调用点直呼
  `tablepkg.*`；`table_style.go`/`shape.go` 同步；字段改导出后 root
  调用点随改（`g.Rows()`/`g.Cols`/`s.TC`/`s.IsContinuation` 等）
- 测试随迁（ADR-016 陷阱免疫）：`TestInferCols`（原 table_test.go）迁入；
  新增 `grid_test.go`（网格锚点/continuation/行列推断/span 校验/cellHasText
  /clear/TableOfGraphic）
- 门槛：`internal/document/table` 入 gate `FLOORS=90`（实测 **94.1%**）
- 守恒：`go test ./...` 24/24 + corpus 24；api_surface golden 不变；gate PASS

### 域搬迁 7：绑定域 切片 1——无状态取值原语（2026-09-17）

绑定域的扫描/编排（`bindScanner`、`Presentation.Bind`、`bindBody`/`bindTable`/
`bindChart`）持 Presentation/Shape/Paragraph/TableShape 句柄，与媒体 profile
同理须待门面收敛；本切片先搬入**无状态取值原语**，与既有
`internal/bind`（marker/plan）合并：

- `resolve.go`：`Member`（map/切片/结构体反射访问）、`AsSlice`（任意
  切片/数组展开）、`Truthy`（条件真值）、`FormatBindValue`（占位符
  字符串化）
- 根 `bind_resolve.go` 230 → ~75 行：只留 `resolve`/`resolveItems`
  （bindScanner 方法），调用点直呼 `bind.Member`/`bind.AsSlice`；
  `bind_body.go`/`bind_table.go` 改用 `bind.Truthy`/`bind.FormatBindValue`
- 测试随迁（ADR-016 陷阱免疫）：`TestBindPure*` 6 例 + `bindFields`/
  `bindStringer` 迁入 `internal/bind/resolve_test.go`
- 门槛：`internal/bind` 保持 `FLOORS=90`（实测 100% → **92.1%**，新增
  纯函数分支后仍达标）
- 守恒：`go test ./...` 24/24 + corpus 24；api_surface golden 不变；gate PASS

### 门面收敛：根包 → `pptx/`（2026-09-17，演进第 3 步，唯一 breaking 点）

模块根变为元仓库（无 `.go`）：根包 `package pptx` 整体移入 `pptx/`，
唯一正式公共导入路径改为 **`github.com/F31/go-pptx/pptx`**。

- 迁移：105 个根 `.go`（104 `pptx` + 1 `pptx_test`）`git mv` → `pptx/`；
  `assets/audio-speaker.png` → `pptx/assets/`（`//go:embed` 禁 `..`）
- 门面契约：`api_surface_test.go` 随迁 `pptx/`；`addAliasMethods` 对模块内
  包的解析基准改为 `..`（模块根）。door face 仍为**真实定义 + 薄委托**的
  混合体——按机制 1，后续能力扩展按域继续下沉 `internal/document/*`，
  本次只完成"公共面收窄为一个子包"的结构点
- 引用更新：24 importer → `.../go-pptx/pptx`；6 个 corpus 测试
  `testdata/corpus` → `../testdata/corpus`（`testdata/` 留在元仓库根，符合
  目标布局）
- CI/gate：`gate.sh` root 门槛 → `github.com/F31/go-pptx/pptx=82`；
  `fuzz.yml` 4 个 root fuzz 目标 `'.'` → `'./pptx'`；README import 示例同步
- 守恒：api_surface golden 零变更（163 type / 40 Stable 段 / 60 符号 /
  131 方法 / 17 哨兵）；`go test ./...` 24/24 + corpus 24；vet/gofmt clean；
  coverage gate PASS（`pptx` 83.3% ≥ 82）。

### 顶层包收编：`ir/` → `internal/ir`（2026-09-17，演进第 4 步·机械下沉）

- `ir/`（7 文件：ir.go/diff.go/timingir.go + 4 测试）`git mv` → `internal/ir/`；
  importer（`cmd/pptx/{inspect,exportir,diff}.go`、`wasm/check/check.go`）改
  `.../go-pptx/ir` → `.../go-pptx/internal/ir`；gate 门槛 `/ir=85` →
  `/internal/ir=85`（实测 85.2%）
- **门面解耦（同日续）**：`internal/ir` **不再 import `pptx`**——投影入口
  `FromPresentation`（门面→IR 适配器）移入 `internal/engine`（`ProjectIR`；
  engine 允许依赖门面），`internal/ir` 只吃 `internal/document/model`（
  SlideID/ShapeID/Optional）与 `xmlstore`/stdlib。`Page.SlideID`/`Shape.ID`
  改 `model.*`（原先即门面 alias，JSON 字段不变）；`diff.go`/`diff_test.go`
  同步换 `model`。
- **真改造（同日续）**：engine `ProjectIR` 的形状/文本/表格投影**改由
  `internal/ooxml` 的 schema 只读投影驱动**——新增 `internal/ooxml/ooxml.go`
  （`Open`=opc.Load、`Bytes`=Part 只读字节桥）与 `internal/ooxml/shapes.go`
  （`SlideShapes`：p:sld 解码 + 形状 kind 推断/组内联扁平化/表格行列与文本/
  graphicFrame table+chart 捕获，含 `xsd:any` RawElem + `RawElem.Attrs`），
  门面侧 `pptx/bridge.go` 提供 `PartBytes(p, opc.PartName)` 只读桥（包级函数，
  不进 api-surface golden）。`internal/ir` 真改造仍在长尾：notes/timing/hidden
  仍经门面句柄，由 `internal/engine.ProjectIR` 承载，下一步将其逐个换掉。
  `archlint` 临时例外面收敛为仅 **`internal/engine`**（R1 由 archlint_test
  守护 ir→pptx=违规）。
- 测试随迁（ADR-016 陷阱免疫）：`ir_test.go`/`table_ir_test.go` 迁入
  engine（`irbuild_test.go`/`irbuild_table_test.go`）；`timingir_test`/
  `diff_test` 留守 `internal/ir`
- `render/` 按决策**保留**（有意发布的公共契约），未动
- 守恒：`go test ./...` 27/27 + corpus 27；vet/gofmt clean；golden 不变；
  gate PASS（engine 86.6% ≥85；ir 85.2% ≥85；ooxml 93.4% ≥90；archlint 全合规）

### 编排层：`internal/engine` 起步（2026-09-17，演进第 5 步）

CLI（`cmd/pptx`）与 WASM（`wasm/check`）此前各自重复实现 Inspect/
Validate/Capability 的核心逻辑（wasm 文档自陈"CLI 保持独立实现"）。新建
`internal/engine`（T2，依赖门面 + `internal/ir`，不做 I/O、不感知
flag/exit/stdout）：

- `Inspect(p) InspectResult`——Slides + `ir.FromPresentation` 投影
- `Validate(ctx, p) ValidateResult`——`p.Validate` + 分级计数
  （`countSeverities` 纯函数，便于白盒测试）
- `Capability(sourceInput, sourceSize) CapabilityResult`——manifest 构建 +
  缩进 JSON
- 接线：`wasm/check` 三个入口改委托 engine（保留自身 JSON 外壳/schema）；
  `cmd/pptx/{inspect,validate,capability}.go` 改委托 engine（保留 flag/
  exit code/输出形态）；两处事实来源合一
- 门槛：`internal/engine` 入 gate `FLOORS=85`（实测 **92.3%**）
- 依赖方向：`internal/engine → pptx` 是经 `internal/archlint` 登记的
  临时例外（编排层需门面公共句柄）

### CI 依赖方向校验：`internal/archlint`（2026-09-17，演进第 6 步）

std-lib 自建规则包（零新依赖），`archlint_test.go` 调 `go list` 取真实
导入边后断言：

- R1 `internal/*` 不得 import 门面（临时例外：仅 `internal/engine`——
  `internal/ir` 已于同日完成门面解耦）
- R2 `internal/*` 不得 import `render`/`cmd`/`wasm`
- R3 门面不得 import `render`/`cmd`/`wasm`/`internal/ir`/`internal/engine`
- R4 `render` 只依赖门面（不下沉 internal）
- R5 叶子包（`xmlstore`/`ooxmlns`/`textmap`/`audioprobe`/`videoprobe`）零模块内依赖
- 门槛：`internal/archlint` 入 gate `FLOORS=85`（实测 **100%**）；当前模块
  R1–R5 全合规

- 守恒：`go test ./...` 26/26 + corpus 26；vet/gofmt clean；golden 不变；
  gate PASS

### ooxml 生成管线（2026-09-17，演进第 1 步落地）

- **输入获取（阻塞点已解）**：ECMA-376 第 5 版 **Part 4** 附
  `OfficeOpenXML-XMLSchema-Transitional.zip`（26 XSD，`schemas.openxmlformats.org`
  命名空间）。此前误取 Part 1 的 **Strict**（`purl.oclc.org`），故"链接失效/
  命名空间不符"。`scripts/gen/schema/fetch.sh` 负责下载并解包到
  `internal/ooxml/schema/.xsd/`（.gitignore 忽略）。
- **生成器**：`scripts/gen/schema`（**仅 std-lib**：encoding/xml + go/format）：
  - 解析 `complexType`（含 `complexContent/extension` 展平继承）、
    `simpleType`（restriction/enumeration → `type ST_ string` + 常量）、
    `group` 引用内联、`sequence/choice/all` 粒子、`minOccurs/maxOccurs`
    （含组/choice 的 unbounded → 切片）、`attribute`。
  - 类型名按命名空间短前缀消歧（`P_`/`A_`/`S_`/`R_`/`C_`/`CP_`/`DV_`/`EP_`）；
    跨命名空间未加载类型与匿名内联 complexType、`xsd:any` 退化为 `*RawElem`。
  - 默认只生成 PPTX 只读投影相关的 8 个命名空间（`-only` 可扩）。
  - struct 字段去重、复杂类型统一指针（避免非法递归值类型）。
- **生成物**：`internal/ooxml/schema/zz_generated_*.go`（8 文件 ≈7.1k 行，入库）；
  手写 `doc.go`（RawElem + go:generate）与 `decode.go`
  （`Unmarshal`/`DecodeSlide|SlideLayout|SlideMaster|Presentation`）。
- **硬约束遵守**：生成类型仅用于 `encoding/xml` 只读投影（`decode_test` 用真实
  幻灯片 XML 验证 `p:sld → P_CT_Slide → spTree.sp[].txBody.p[].r[].t` 投影链），
  未进入任何写路径。
- **门槛**：`internal/ooxml/schema` 入 gate `FLOORS=90`（实测 **100%**）；
  生成器 `scripts/gen/schema` 入 `SKIP`（package main）。
- **后续（未完成）**：`internal/ir` 由「import 门面」重写为「只吃
  `internal/ooxml/schema`」（演进第 4 步的真改造）；逐步以生成类型替换手写
  `*_parse.go`/nodeStep 路径；补齐 `xsd:any`/跨命名空间复杂类型与 p14/morph 扩展。
- 守恒：`go test ./...` 27/27 + corpus 27；vet/gofmt clean；golden 不变；gate PASS

## 参考

- 设计文档 §3 总体架构与模块职责、§4.2 三类写入路径
- ADR-014（根包策略，本 ADR 在 v2.0 语境下取代其 §1）
- ADR-015（API 稳定性分级，`pptx/` 门面仍按三级标注）
- ADR-016（渐进式 internal 抽取，覆盖率归属陷阱由"测试随包走"消解）
- ADR-029（同包拆文件，v1.x 权宜，v2.0 由本 ADR 分层取代）
- 参考实现：python-docx/pptx（`pptx/oxml` 内部层）、excelize（XSD 生成路线）
