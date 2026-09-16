# ADR-030: v2.0 目标架构——分层元仓库（layered meta-repo）

- **状态**: Proposed（提案，v2.0 目标态，未实施）
- **日期**: 2026-09-16
- **修订**: 2026-09-16 第 1 轮（依据外部评审）——基线校正（行数/方法数）、机制 2 重写为只读投影、补新包门槛档位与验收口径、试点选型改为 bind 私有实现、ir/render 定性修正
- **关联设计**: 《go-pptx 完整设计方案 V2.6 开发实施版》§3（总体架构）、§4（写入路径）
- **替代/废止**: 无（本 ADR 为 v2.0 演进目标；落地时取代 ADR-014 §1、ADR-016、ADR-029 的 v1.x 拆分策略条款）
- **关联 ADR**: ADR-014（根包内部包策略）、ADR-015（API 稳定性分级）、ADR-016（渐进式 internal 抽取）、ADR-029（同包拆文件）

## 上下文

### 现状盘点（2026-09-16）

模块根**同时是公共门面与实现本体**：

| 位置 | 内容 |
|---|---|
| 根包 `pptx` | 63 非测试文件 / 21,560 行 / **129 个 Presentation+Slide 方法**（Presentation 86 + Slide 43），公共类型（Presentation/Slide/Shape/TextRun/…）与业务实现 100% 纠缠 |
| 顶层 `ir/`、`render/` | **名义公共包**，实际仅被 `cmd/pptx` 与 `wasm/check` 引用（ADR-014 已记录）——公开暴露但无人从外部用 |
| `internal/` | 已有雏形：`opc` / `xmlstore` / `document` / `chart` / `editplan` / `audioprobe` / `videoprobe` / `textmap` |
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
│   ├── ir/  render/  diff/               # 只读 IR / 渲染 / 语义 diff（仅吃 ooxml）
│   └── diag/  capa/                      # 跨层只向上（被门面与工具消费）
├── pptx/                                 # 公共门面（唯一被外部 import 的包）
│   ├── pptx.go  text.go  media.go  table.go  chart.go
│   ├── options.go  report.go             # *Spec/*Options/*Info/*Report
│   ├── doc.go                            # 包文档 + API 分级说明
│   └── *_test.go                         # 门面契约（黄金计数/行为/示例）
├── cmd/pptx  cmd/pptx_check              # 薄入口（flag 解析 + 编排 engine）
├── wasm/site/                            # 离线静态 UI（现状保留）
├── scripts/gen|l3|coverage|release/      # 工程脚本按用途分目录
└── docs/                                 # 现状保留（ADR/设计/路线图/矩阵）
```

### 六条关键机制

1. **门面薄、域厚**。`pptx/` 只放稳定类型与薄委托（方法体 ≈ 1 行，转调 `internal/document`）；业务逻辑全部下沉。公共签名冻结后实现可无限迭代。参照 python-pptx 内部结构（`pptx/` + `pptx/oxml/` + `pptx/opc/`）。
2. **OOXML 类型生成 = 最大杠杆（限只读投影）**。淘汰"stringly-typed XML"反模式——几十个手写 `*_parse.go` / `text_node.go` nodeStep 路径，替换为从 ECMA-376 XSD 生成的 typed `encoding/xml` 结构（同 excelize 路线），配少量微软扩展补丁（p14/morph）。`schema/` 一个目录替代全部手写解析。**硬约束：生成类型仅用于只读投影（替换手写 parse），不得进入写路径**——`encoding/xml` 往返不保证属性序、命名空间前缀选择、自闭合形式与空白不变；一旦生成类型参与写入，`corpus_b1_test.go`（空变更集→字节恒等）、`internal/opc/saveplan_test.go:TestSavePlanUnchangedIsB1`、ext-0024 单 Run 替换 1 字节差异这三条字节级保真证据立即失效。**写路径维持 `xmlstore` span 补丁 + 未修改 Part 字节拷贝。**
3. **测试随包走**。每个 internal 子包自带白盒测试 → **ADR-016 覆盖率归属陷阱从根上消失**（代码与测试一起搬，覆盖率按包归属天然正确）。`internal/chart` 5.5% 的历史悲剧不再复现。
4. **依赖单向、CI 强制**。门面→域→格式→传输；`ir/diff/render` 只吃 `ooxml`；`diag/capa` 只向上。用 **std-lib 自建依赖规则 job**（`go/parser` + `go list`，与 `api_surface_test.go` 同手法）在 CI 执行，防回潮——**不引入 `go-arch-lint` 等外部工具**，遵守 `ci.yml` 零依赖政策。
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
| 顶层 `render/` | 待决策 | **死代码**（零包外引用）——删 / 留公共+指定消费者 / 降 internal 三选一，见验收口径 §4 |
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
internal/ir / render / diff ─→ internal/ooxml（只读投影，不依赖门面）
internal/diag / internal/capa ─→ 仅被门面与工具消费
```

### 演进顺序（v2.0 启动后按此推进）

1. **立 `ooxml/schema` 生成管线**（只读投影，见机制 2；最慢、最独立，可并行推进）。
2. **逐域搬迁**：`geometry` → `style` → `text` → `media` → `table/chart` → `bind`，每域一个 PR，白盒测试随行（ADR-016 陷阱免疫）。
3. **收敛门面**：根包 → `pptx/` 子包（唯一的 breaking 点），同步迁移 `api_surface_test`；中间态按验收口径 §2 维护 golden。
4. **收编/处置顶层包**：`ir/` 改造入 `internal/ir`（**依赖第 3 步门面收敛**——现 import 根包，须先有 `pptx/` 门面可依赖）；`render/` 按验收口径 §4 三选一定案。
5. **engine 化 cmd/wasm**：CLI 与 WASM 改编排 `internal/engine`。
6. **CI 加依赖方向校验**（std-lib 自建规则 job，零新依赖，见机制 4）。
7. **gate 完整性收口**：`go list ./...` 集合 ⊄ FLOORS → FAIL（堵住新包静默不受检），新包按门槛档位表入表。

### 新包覆盖率门槛档位（2026-09-16 补齐，阻塞项）

现状 `scripts/coverage/gate.sh` 的 `FLOORS` 是**白名单**：未列包**静默不受检**，且档位只有 root/cmd/低层格式三档。v2.0 逐域搬迁会不断产生新包，必须先定义档位：

| 档位 | 门槛 | 适用包 |
|---|---|---|
| T1 公共门面 | ≥82 | `pptx/`（承接现 root 档） |
| T2 域层包 | ≥85 | `internal/document/*`、`internal/bind`、`internal/engine` |
| T3 低层格式包 | ≥90 | `internal/ooxml/*`、`opc`、`xmlstore`、`textmap`、`editplan` |
| T4 工具/只读包 | ≥85 | `internal/ir`、`internal/diff`、`cmd/*`、`wasm/check` |
| 防回归下界 | 取现测值 | `chart`（90.8）、`render`（84，视决策）等行为优先包 |

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

1. **新包门槛档位表**：见 §决策——按 T1..T4 入表，gate 完整性校验同步落地（阻塞项）。
2. **中间态 golden 维护**：逐域搬迁期间长期存在「部分类型在 `pptx/`、部分在根包」的
   双门面，`api_surface_test` 黄金计数（158 类型 / 506 导出函数 / 131 Stable 方法）与
   ADR-015 分级如何跨中间态维护——每个搬迁 PR 的验收必须含「golden 不变或按 ADR 流程
   显式变更」声明。
3. **ir 重写依赖**：`ir` 从 import 根包改为只吃 `ooxml`，须在门面收敛（演进第 3 步）后
   进行；提前迁移将无 `pptx/` 门面可依赖。
4. **render 决策**：死代码三选一（删 / 留公共+指定消费者 / 降 internal），启动前定案；
   「降 internal 藏死代码」不推荐。

### 触发条件（何时正式启动 v2.0）

任一命中即启动（**2026-09-16 实测：三条均未命中**）：

1. **v1.x 出现需要动门面签名的新能力**——最近提交均为文档/重构收尾，无门面签名变更，未触发；
2. **根包编译/导航成本明确受限**——实测热重建 0.29s ≪ 5s、根包非测试文件 63 < 80，未触发；
3. **外部明确需要 `ir` / `render` 作为公共 API**——实测 `render` 零包外引用、`ir` 仅被 `cmd/pptx` 与 `wasm/check` 引用，未触发。

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
- **render 决策**（启动前定案）：删 / 留公共+指定消费者 / 降 `internal/render` 三选一；
- 启动时同步维护 1.x→2.0 迁移文档（import 路径改写 + golden 计数随迁）。

## 参考

- 设计文档 §3 总体架构与模块职责、§4.2 三类写入路径
- ADR-014（根包策略，本 ADR 在 v2.0 语境下取代其 §1）
- ADR-015（API 稳定性分级，`pptx/` 门面仍按三级标注）
- ADR-016（渐进式 internal 抽取，覆盖率归属陷阱由"测试随包走"消解）
- ADR-029（同包拆文件，v1.x 权宜，v2.0 由本 ADR 分层取代）
- 参考实现：python-docx/pptx（`pptx/oxml` 内部层）、excelize（XSD 生成路线）
