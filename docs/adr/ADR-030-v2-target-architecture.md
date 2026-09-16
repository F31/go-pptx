# ADR-030: v2.0 目标架构——分层元仓库（layered meta-repo）

- **状态**: Proposed（提案，v2.0 目标态，未实施）
- **日期**: 2026-09-16
- **关联设计**: 《go-pptx 完整设计方案 V2.6 开发实施版》§3（总体架构）、§4（写入路径）
- **替代/废止**: 无（本 ADR 为 v2.0 演进目标；落地时取代 ADR-014 §1、ADR-016、ADR-029 的 v1.x 拆分策略条款）
- **关联 ADR**: ADR-014（根包内部包策略）、ADR-015（API 稳定性分级）、ADR-016（渐进式 internal 抽取）、ADR-029（同包拆文件）

## 上下文

### 现状盘点（2026-09-16）

模块根**同时是公共门面与实现本体**：

| 位置 | 内容 |
|---|---|
| 根包 `pptx` | 63 非测试文件 / 21,534 行 / **128 个 Presentation+Slide 方法**，公共类型（Presentation/Slide/Shape/TextRun/…）与业务实现 100% 纠缠 |
| 顶层 `ir/`、`render/` | **名义公共包**，实际仅被 `cmd/pptx` 与 `wasm/check` 引用（ADR-014 已记录）——公开暴露但无人从外部用 |
| `internal/` | 已有雏形：`opc` / `xmlstore` / `document` / `chart` / `editplan` / `audioprobe` / `videoprobe` / `textmap` |
| `cmd/` | `pptx`（CLI 六子命令）、`pptx_check`（WASM） |
| `wasm/` | `check`（纯函数包）、`site`（离线静态 UI） |

### 问题的本质

- 早期把公共对象模型放进根包并冻结（ADR-015）后，**实现被永久焊死在门面上**：`internal/style`、`geom`、`textmap` 等被 ADR-014 否决抽取，理由正是"公共类型必须在根包"——v1.x 内该结论正确，代价是没有层。
- ADR-029 的域前缀拆分（`text_*`/`style_*`/…）是冻结面内的**权宜**：改善了导航，但 63 个文件仍同属一个无层包，128 个门面方法仍与实现同文件共生。
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
2. **OOXML 类型生成 = 最大杠杆**。淘汰"stringly-typed XML"反模式——几十个手写 `*_parse.go` / `text_node.go` nodeStep 路径，替换为从 ECMA-376 XSD 生成的 typed `encoding/xml` 结构（同 excelize 路线），配少量微软扩展补丁（p14/morph）。`schema/` 一个目录替代全部手写解析。
3. **测试随包走**。每个 internal 子包自带白盒测试 → **ADR-016 覆盖率归属陷阱从根上消失**（代码与测试一起搬，覆盖率按包归属天然正确）。`internal/chart` 5.5% 的历史悲剧不再复现。
4. **依赖单向、CI 强制**。门面→域→格式→传输；`ir/diff/render` 只吃 `ooxml`；`diag/capa` 只向上。用 `go-arch-lint`（或 import 规则 job）在 CI 执行，防回潮。
5. **cmd/wasm 共享 engine**。CLI 六子命令与 WASM check 不再各自贴门面，统一编排 `internal/engine`。
6. **模块根变元仓库**。`internal/` 受语言级保护；公共面收窄为一个 `pptx/` 子包——"63 文件根包"的结构压力自动归零，未来文件增长被分摊到各子域包。

### 现有投资处置

| 现状 | 目标 | 说明 |
|---|---|---|
| 根包 63 文件 | 拆分：`pptx/` 门面（≈8-10 文件）+ `internal/document/*` | 公共类型搬门面，业务/私有实现下沉 |
| `internal/opc` / `xmlstore` | 保留 | 传输/存储层，位置不动 |
| `internal/document` | 扩容为域层根 | 吸纳根包全部业务逻辑 |
| `internal/chart` | 并回 `document/chart` | 自带测试后覆盖率问题消解 |
| `internal/audioprobe` / `videoprobe` / `textmap` / `editplan` | 归入 `document/media` / `document/text` / `engine` | 按功能收编 |
| 顶层 `ir/`、`render/` | 移入 `internal/ir`、`internal/render` | 只被 cmd/wasm 引用，撤销名义公共面 |
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

1. **立 `ooxml/schema` 生成管线**（最慢、最独立，可并行推进）。
2. **逐域搬迁**：`geometry` → `style` → `text` → `media` → `table/chart` → `bind`，每域一个 PR，白盒测试随行（ADR-016 陷阱免疫）。
3. **收敛门面**：根包 → `pptx/` 子包（唯一的 breaking 点），同步迁移 `api_surface_test`。
4. **收编名义公共包**：`ir/`、`render/` → `internal/`。
5. **engine 化 cmd/wasm**：CLI 与 WASM 改编排 `internal/engine`。
6. **CI 加依赖方向校验**（go-arch-lint / import 规则）。

## 后果

### 正面

- 层存在：门面签名与实现解耦，v2.1+ 的能力扩展不再堆进单一无层包；
- 覆盖率归属正确：测试随包走，各 internal 包独立达标，ADR-016 陷阱根除；
- 公共面收窄且可审计：外部可见符号 = `pptx/` 一个包，capability manifest 与文档一致性维护成本下降；
- 类型安全提升：XSD 生成的 CT_* 类型消灭手写 nodeStep/解析，字节级保真由类型层兜底。

### 负面 / 风险

- **破坏性变更**：`github.com/F31/go-pptx` → `github.com/F31/go-pptx/pptx`，所有 importer 需迁移（v2.0 一次性）；
- 搬迁量大（根包 63 文件 + 顶层 2 包 + 测试），review 与回归成本集中；
- 历史文档/脚本中旧路径引用需随迁（ADR-029 已记录同名问题的处理口径）；
- `ooxml/schema` 生成器是长期资产，短期投入大、见效慢——属 v2.0 周期的前期投入。

### 触发条件（何时正式启动 v2.0）

任一命中即启动：

1. **v1.x 出现需要动门面签名的新能力**（当前所有工作包均为追加式，未触发）；
2. **根包编译/导航成本明确受限**（如增量编译 > 5s，或根包非测试文件 > 80 个）；
3. **外部明确需要 `ir` / `render` 作为公共 API**（则直接放 `internal/` 之外并按 ADR-015 定级，而非维持现状的名义公共包）。

### 后续动作

- 本文为 v2.0 **提案**；v1.x 期间不实施，继续按 ADR-014/029 维护；
- 候选先行试点：以 `geometry` 或 `style` 单域做一次"搬迁可行性试点"，验证测试随包走的覆盖率表现与依赖方向，为正式启动积累数据；
- 启动时同步维护 1.x→2.0 迁移文档（import 路径改写 + golden 计数随迁）。

## 参考

- 设计文档 §3 总体架构与模块职责、§4.2 三类写入路径
- ADR-014（根包策略，本 ADR 在 v2.0 语境下取代其 §1）
- ADR-015（API 稳定性分级，`pptx/` 门面仍按三级标注）
- ADR-016（渐进式 internal 抽取，覆盖率归属陷阱由"测试随包走"消解）
- ADR-029（同包拆文件，v1.x 权宜，v2.0 由本 ADR 分层取代）
- 参考实现：python-docx/pptx（`pptx/oxml` 内部层）、excelize（XSD 生成路线）
