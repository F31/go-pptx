# go-pptx 技术白皮书（V2.0）

> **版本**：V2.0 · 基于最新代码基线 **v2.0.1**（SSH 签名 tag，commit `0258392`，2026-09-18）
> **模块路径**：`github.com/F31/go-pptx/v2`；公共门面为子包 `github.com/F31/go-pptx/v2/pptx`
> **设计基线**：《go-pptx 完整设计方案 V2.6 开发实施版》
> **文档定位**：面向技术决策者、架构师与潜在集成方的综合性技术披露；与 [`RELEASE-NOTES-v2.0.0.md`](RELEASE-NOTES-v2.0.0.md) / [`RELEASE-NOTES-v2.0.1.md`](RELEASE-NOTES-v2.0.1.md)（发布通告）和 [`CHANGELOG.md`](../CHANGELOG.md)（变更日志）互补。
> **关联 ADR**：ADR-014（内部包拆分策略）、ADR-015（API 稳定性分级）、ADR-016（渐进式 internal 实现层抽取）、ADR-017（chart 实现层抽取）、ADR-018（OPC Tier 2 原始透传 / `SavePlan.Write`）、ADR-021（Shape 能力接口）、ADR-029（根包大文件拆分）、ADR-030（v2 目标架构：门面收敛 + 编排层 + ooxml 生成管线）。

---

## 目录

1. [产品概述](#1-产品概述)
2. [设计哲学与产品定位](#2-设计哲学与产品定位)
3. [技术架构](#3-技术架构)
4. [功能特性](#4-功能特性)
5. [API 稳定性分级体系](#5-api-稳定性分级体系)
6. [质量保障与工程纪律](#6-质量保障与工程纪律)
7. [工具链与命令行](#7-工具链与命令行)
8. [产品特色与差异化优势](#8-产品特色与差异化优势)
9. [技术创新点](#9-技术创新点)
10. [典型应用场景](#10-典型应用场景)
11. [已知边界与 V2.x 路线图](#11-已知边界与-v2x-路线图)
12. [版本、许可与社区](#12-版本许可与社区)

---

## 1. 产品概述

**go-pptx** 是一个面向服务端与工具开发者的纯 Go 演示文稿组件，提供类似 python-pptx 的对象模型体验（`Presentation → Slide → Shape → TextFrame → Paragraph → Run`），同时在**字节级保真、命名空间安全、事务化编辑、资源预算、跨平台分发**五个维度上做了系统性投入。组件可作为静态依赖嵌入任何 Go 服务端应用，无需 Python、Microsoft Office、FFmpeg 或 CGO。

V2.0 是组件的第二个**对外公开的大版本**，完成了《项目实施计划》全部里程碑，并落地了 ADR-030 架构演进：公共门面从根包收敛到 `pptx/` 子包、引入 `internal/engine` 编排层、以 `internal/ooxml` + `internal/ooxml/schema`（由 `scripts/gen/schema` 从 ECMA-376 Part 4 Transitional XSD 自动生成）承担只读投影。V2.0.1 进一步把 v2.0.0 发布后的一轮代码评审（安全 / 性能 / 易用性）全部落地（详见 §11 与 `docs/code-review-2026-09-17.md`）。

| 维度 | V2.0.1 现状 |
|---|---|
| 模块路径 | `github.com/F31/go-pptx/v2`（元仓库）；公共门面 `github.com/F31/go-pptx/v2/pptx` |
| 导出类型总数 | **166**（含 5 个 type alias：`TextShape` + v2.0.1 新增 `Budget` / `Durability` / `PartName` 等） |
| Stable 段落 | **42**（`// Stable:` 段） |
| Stable 符号 | **65** |
| Stable 方法 | **134** |
| Experimental 段落 | **0**（D-5 全部升 Stable，ADR-023） |
| 错误哨兵 | **17**（全部 Stable，含语义化 `Err*` 集合） |
| 顶层函数 / 方法 / 常量 / 包变量 | 37 / 143 / 151 / 18（见 `docs/api-reference.md`） |
| 默认测试 | `go test ./...` 与 `go test -tags=corpus ./...` 各 **28 包 ok**（56 包） |
| 公开金样回归 | 3 个 LibreOffice 生成金样（`s001-text` / `s002-table` / `s003-image`）+ 36 个语料索引；L3 客户端矩阵 PowerPoint/WPS 真机 8/8 通过 |
| 交叉构建验证 | `js/wasm` · `wasip1/wasm` · `darwin/arm64` · `linux/arm64` · `windows/amd64` 五平台零失败（CI `cross-build` job 固化） |
| 覆盖率门禁 | `scripts/coverage/gate.sh` **COVERAGE GATE PASSED**（root `pptx` 82%+；`opc` 90.3% / `chart` 91.3% / `xmlstore` 91.2% / `videoprobe` 92.9% 等） |

---

## 2. 设计哲学与产品定位

### 2.1 六项能力主线

go-pptx 的产品形态由六项能力构成（V2.6 §1）：

1. **通用 PPTX 读写**：创建、解析、保存、页面管理、形状、图片、文本、备注、表格与受支持图表。
2. **可控保真编辑**：保留未修改 Part 与可保护的未知 XML 内容，拒绝不能安全执行的操作。
3. **精细格式处理**：区分本地格式、继承格式与布局结果；支持跨 Run 编辑、组合变换及表格样式。
4. **自动讲解支持**：富文本备注、音频嵌入、播放配置、翻页计时及带诊断的同步操作。
5. **工程可靠性**：惰性解析、资源预算、事务提交、校验报告、真实文档回归与原生 fuzz。
6. **可选扩展**：只读 IR、CLI、媒体探测、渲染适配——保持核心依赖简单。

### 2.2 部署与职责边界

| 项 | 策略 |
|---|---|
| 部署形态 | Go 应用静态依赖；不调用 Python、Office、FFmpeg 或在线服务 |
| 核心约束 | **CGO_ENABLED=0**、纯 Go、无外部运行时强依赖 |
| 模块路径 | `github.com/F31/go-pptx/v2`（元仓库，无 Go 文件）；公共门面 `github.com/F31/go-pptx/v2/pptx` |
| 公共门面 | 门面收敛于子包 `pptx/`；`internal/*` 以 Go 内部包规则隔离（`opc` / `xmlstore` / `document` / `chart` / `ooxml` / `engine` 等） |
| 错误约定 | 普通错误以 `error` 返回；不使用 panic；不通过返回 nil 掩盖未支持能力 |
| 接口便捷性 | Options、Spec、高层组合方法；不引入吞掉错误的链式 Setter |
| 能力披露 | 保真 / 兼容 / 渲染 / 播放分别声明能力范围；发布声明必须由测试证据支持 |
| 资源预算 | 不可信输入受 `opc.Budget`（Part 数、中央目录尺寸、解压上限、`MaxUncompressedBytes`）约束；超出返回 `ErrLimitExceeded` / `ErrMalformedPackage` 而非 panic |

### 2.3 MVP 双闭环

- **闭环 1（模板报告）**：模板打开 → 读取讲稿 → 更新文本/备注 → 保存
- **闭环 2（自动讲解）**：音频嵌入 → 计时 → 放映

两个闭环均在 v2.0.1 中以真实语料回归用例覆盖。

---

## 3. 技术架构

### 3.1 包模型与依赖方向（ADR-030 门面收敛）

V2.0 的架构核心动作是**门面收敛**：模块根不再包含任何 Go 文件，成为元仓库；唯一的正式公共导入路径变为 `github.com/F31/go-pptx/v2/pptx`。内部实现按功能垂直下沉为一组 `internal/*` 包，并由 `internal/engine` 编排层统一承载 CLI 与 WASM 共享的核心逻辑（Inspect / Validate / Capability / ProjectIR）。

```
                        ┌───────────────────────┐
                        │      cmd/pptx         │  ← CLI 入口（9 个子命令）
                        └──────┬─────────────┬──┘
                               │             │
                ┌──────────────▼─┐         ┌─▼─────────────┐
                │  wasm/check    │         │  render        │  ← 渲染契约（接口，实现未立项）
                └──────┬─────────┘         └─┬─────────────┘
                       │                     │
                       └────────┬────────────┘
                                ▼
                ┌───────────────────────────┐
                │  internal/engine          │  ← 编排层（CLI+WASM 共享）：
                │  Inspect/Validate/        │     ProjectIR、EditBridge
                │  Capability/IR 投影        │
                └──────┬──────┬──────┬──────┘
                       │      │      │
                       │      │      └─────────► internal/ir（只读 IR + 计时 IR + 语义 diff）
                       │      │
          ┌────────────▼─┐  ┌─▼───────────────────────────────┐
          │  pptx/ (门面) │  │ internal/ooxml (+ schema)         │
          │ 公共 SDK 门面  │  │ OOXML 只读投影（由 XSD 生成）     │
          │ 166 类型      │  └─┬─────────────────────────────────┘
          └──┬──┬──┬──┬──┘    │
             │  │  │  │       │
   ┌─────────▼─┐│  │  │  ┌────▼─────────────────────────────┐
   │internal/  ││  │  │  │ internal/chart / document/        │
   │opc        ││  │  │  │ {model,style,geometry,text,       │
   │(ZIP/关系/ ││  │  │  │  media,table} / textmap /         │
   │预算/保存)  ││  │  │  │ textutil / bind / style /        │
   └────┬──────┘│  │  │  │ ooxmlns / audioprobe /            │
        │       │  │  │  │ videoprobe / editplan / errs /    │
        ▼       │  │  │  │ diag / archlint                   │
   ┌────────────▼─┐│  │  └───────────────────────────────────┘
   │internal/     ││  │
   │xmlstore      ││  │  依赖方向纪律：
   │(扫描/索引/   ││  │  ✅ pptx → 全部 internal/*；engine → ir/ooxml/xmlstore
   │ span patch)  ││  │  ✅ internal/chart → ooxmlns/textutil/xmlstore
   └──────────────┘│  │  ❌ internal/* → pptx（禁止反向污染门面）
                   │  │  ❌ internal/* → engine（除 engine 自身外）
                   ▼  ▼
              internal/xmlstore（最底层，被 opc/ir/ooxml/style/bind/textutil 共享）
```

**依赖方向纪律**（ADR-014 / ADR-016，由 `internal/archlint` 编译期强制）：

- ✅ **允许**：`pptx/` → 全部 `internal/*`；`internal/engine` → `internal/ir` + `internal/ooxml` + `internal/xmlstore`；`internal/chart` / `internal/style` / `internal/bind` / `internal/textutil` → `internal/xmlstore`；`internal/opc` → `internal/xmlstore`
- ❌ **禁止**：任何 `internal/*` → `pptx/` 门面（避免循环依赖、避免内部实现反向污染公共 API）；除 `engine` 外任何 `internal/*` → `internal/engine`
- ✅ **archlint 把关**：`go test` 即执行依赖方向校验，新包未登记 / 出现反向依赖立即 FAIL

V2.0 已落地的 internal 包与职责：

| 包 | 职责 |
|---|---|
| `internal/opc` | OPC ZIP 索引、内容类型、关系图、保存规划（`SavePlan`）、原子保存、资源预算 `Budget`、持久化档位 `Durability` |
| `internal/xmlstore` | 命名空间感知 XML 扫描器、节点索引树、span patch 引擎、DOM helper |
| `internal/chart` | 图表 XML 模型、workbook 同步、canonical 校验、fragments（ADR-017 整层抽取） |
| `internal/document` | `PartStore` / `ReadStore` / `PatchStore` 最小文档存储契约（ADR-016） |
| `internal/document/{model,style,geometry,text,media,table}` | 垂直下沉的领域模型 |
| `internal/editplan` | 单 Part / 多 Part 编辑计划与回滚骨架 |
| `internal/textmap` | TEXT-02 跨 Run 替换的纯文本映射原语（rune 匹配 / 字素边界 / run span 定位） |
| `internal/audioprobe` / `internal/videoprobe` | 音频 / 视频容器探测（时长、采样率、声道、分辨率、编码），不调用 FFmpeg |
| `internal/textutil` / `internal/bind` / `internal/style` | v2.0 试点抽出的 XML patch / 转义 helper、模板标记扫描、placeholder 解析（ADR-030） |
| `internal/ooxml` + `internal/ooxml/schema` | OOXML 只读投影；`schema` 由 `scripts/gen/schema`（std-lib XSD→Go）从 ECMA-376 Part 4 Transitional 生成，100% 覆盖、只读 |
| `internal/ir` | 只读 IR、timing IR、语义 diff 核心（v2.0 起不再对外公开，改由 `engine.ProjectIR` 投影） |
| `internal/engine` | 编排层：Inspect / Validate / Capability / `ProjectIR`；CLI 与 WASM 共享同一核心 |
| `internal/archlint` | std-lib 依赖方向校验（CI 经 `go test` 执行） |
| `internal/errs` / `internal/diag` | 错误注解与诊断类型 |
| `internal/ooxmlns` | 共享 OOXML/OPC 命名空间 URI 常量 |

> 注：V1 时期对外公开的 `ir/` 包在 V2.0 已收编为 `internal/ir`，库消费者不再能直接 import；IR 能力通过 `pptx` 门面的只读桥（`PartBytes` / `MainPartBytes`）与 CLI 的 `export-ir` / `diff` 子命令对外暴露。

### 3.2 公共对象模型

go-pptx 严格遵循 OOXML DrawingML 的层次：

```
Presentation (Root)
└── Slide 1..N
    └── Shape
        ├── GroupShape / AutoShape (含 TextShape 别名) / OpaqueShape
        ├── PictureShape / TableShape / ChartShape
        ├── AudioShape / VideoShape
        └── TextFrame
            └── Paragraph 1..N
                └── TextRun 1..N
```

每个对象的关键约定（与 V1 一致，句柄身份复用 OOXML 稳定身份，见 §3.6）：

| 对象 | 关键能力 | 句柄身份语义 |
|---|---|---|
| `Presentation` | 文档生命周期 / `Capability` 六维能力报告 | — |
| `Slide` | 页面管理、备注、版面诊断 | `SlideID`（`uint32`，唯一持久身份） |
| `Shape` | 几何 / 样式 / 效果报告、子节点枚举 | `ShapeID` = cNvPr@id（spTree 内全局唯一） |
| `TextFrame` | 跨 Run 替换、plain text 替换、字段、Autofit 诊断 | shapeHint = 所属形状 cNvPr@id |
| `Paragraph` | 段落属性（alignment, spacing, bullets） | shapeHint |
| `TextRun` | Run 属性（font, color, bold, …）+ 文本内容 | shapeHint |

### 3.3 OPC 层：包模型、预算与原子保存

OPC（Open Packaging Conventions）是 PPTX 的物理容器。`internal/opc` 对 OPC 做了三个关键抽象：

1. **ZIP 索引 + 预算**：打开时仅做中央目录解析与轻量索引；不立即解压全部 Part；媒体字节流惰性按需读取。预算 `Budget`（`MaxEntries` / `MaxCentrDirBytes` / `MaxUncompressedBytes` 等）对不可信输入强约束——超出返回 `ErrLimitExceeded` / `ErrMalformedPackage` 而非 panic。
2. **关系图 + 内容类型**：完整建模 `_rels/.rels` 与各 Part 的 `Content_Types.xml`，提供 `RelationshipType` / `ContentType` 常量体系（避免硬编码散落）。
3. **保存规划（`SavePlan`）**：区分三类写入路径（V2.6 §4.2）——
   - **完全透传 Part**（未修改 → 字节级保留，ADR-018 Tier 2 raw pass-through）
   - **修改 Part**（走 xmlstore span patch）
   - **新增 Part**（如媒体、克隆页）走命名空间受控的新建路径

**原子保存**：`Save()` 默认采用"先写临时文件 + fsync + rename"策略，禁止撕裂写入；默认拒绝覆盖（返回 `ErrOutputExists`），需显式 `WithSaveOverwrite(true)` 才会覆盖。`Durability` 档位（`DurabilityDefault` / `DurabilityFull`）控制落盘的 fsync 强度。`Save(nil, …)` 与同包 `Write` / `Validate` 一致：nil `context.Context` 归一化为 `context.Background()`（V2.0.1 修复）。

### 3.4 XML store：命名空间感知的最小侵入编辑引擎

`internal/xmlstore` 是组件最具技术含量的内部子系统，提供：

- **命名空间感知扫描器**：识别 `p:` / `a:` / `r:` / `c:` / `mc:` / `p14:` 等命名空间，不强制 schema，对未知元素保留原文。
- **节点索引树**：把每个 XML Part 建成节点位置索引，外部代码可通过 `NodePath`（路径式定位符）寻址。
- **span patch 引擎**：所有写操作以"字节级 patch"形式落到原 XML 上，最小化变更区段；未触及的子树字节级保留——这是组件能"可控保真"的核心机制。
- **未知内容保护**（V2.6 §4.3）：当 patch 与未知节点冲突时返回诊断（`Diagnostic` / `Severity`），而不是静默丢弃。

### 3.5 事务与变更集（ADR-013）

修改操作遵循统一事务模型，杜绝"两个可独立修改的文档真相"（V2.6 §3）：

```go
// 高层公共 API 内部构造编辑计划，再由门面 adapter 应用。
err := applySinglePartPatch(pres, part, bytes)
err := applyMultiPartPlan(pres, plan)   // 失败时自动恢复 pending，不残留半提交状态
```

- **ChangeSet**：聚合所有"尚未提交"的修改
- **Revision**：每次成功提交单调递增；任何旧 Revision 持有的句柄后续再写会触发 `ErrConcurrentModification`
- **EditPlan 分层**：业务路径通过 `SinglePartPatch` / `MultiPartPlan` 提交；低层 `stage*` / `commit` primitive 收敛在 `presentation.go` 与 `document_store.go` adapter 边界

### 3.6 句柄身份体系（STALE-GUARD）

任何"句柄式"对象（Shape / TextFrame / Paragraph / TextRun / Cell）都需要回答"如何安全失效"。go-pptx 的答案是**复用 OOXML 已经存在的稳定身份**：

| 句柄层 | 稳定身份 |
|---|---|
| `Shape` | `cNvPr@id`（DrawingML 规范要求 spTree 内全局唯一） |
| `TextFrame` / `Paragraph` / `TextRun` | shapeHint = 所属形状 cNvPr@id |
| `Cell`（表格） | shapeHint = 所属表格 cNvPr@id |

**语义**（三层闭环）：

- `MoveShape` / `AddShape`（兄弟增）/ 文本编辑 → 句柄仍有效（cNvPr@id 未变）
- `RemoveShape` → 目标 cNvPr@id 消失 → 句柄返回 `ErrStaleHandle`
- 重新定位（locate）流程：先按 NodePath 解析出目标元素，再向上遍历找最近 `p:sp` / `p:cxnSp` / `p:graphicFrame` / `p:grpSp` 的 cNvPr@id 与 hint 比对——不等/找不到即返回 `ErrStaleHandle`
- `hint=0` 走纯路径判定（向后兼容 notes 等老句柄）

> `Presentation.Close()` 在 V2.0.1 起改为**幂等**：重复调用一律返回 `nil`，与 `defer p.Close()` 惯用法兼容；Close 之后的业务方法（`Slides()` 等）仍返回 `ErrClosed`，句柄失效的可观测性未削弱。

### 3.7 跨平台与 WASM

CI 在每次 push 与 PR 上交叉编译以下五个目标：

| 目标 | 用途 |
|---|---|
| `js/wasm` | 浏览器原生 PPT 解析（`wasm/check` 子包），无需服务端中转，可用于隐私敏感场景 |
| `darwin/arm64` | Apple Silicon 原生 |
| `wasip1/wasm` | WASI 1.0（服务端沙箱可执行容器） |
| `linux/arm64` | ARM 服务器（AWS Graviton 等） |
| `windows/amd64` | Windows 服务端 / 桌面分发 |

`CGO_ENABLED=0` 约束保证任何目标都不需要 C 工具链；同时也意味着组件不依赖 glibc/musl 之外的任何系统库，可静态链接到容器镜像中。

### 3.8 编排层、依赖校验与 OOXML 生成管线（ADR-030 新增）

V2.0 相比 V1 的三项架构新增：

- **`internal/engine` 编排层**：统一 CLI 与 WASM 的 Inspect / Validate / Capability 核心，承接 `ProjectIR` 适配器，避免 CLI / WASM 各写一套。覆盖率 91.1%。
- **`internal/archlint` 依赖方向校验**：以 std-lib 实现，CI 经 `go test` 执行；任何 `internal/* → pptx/` 反向依赖、新包未登记、跨层 import 立即 FAIL——把"新包静默不受检"堵死。
- **`internal/ooxml` + `internal/ooxml/schema` 生成管线**：`scripts/gen/schema` 用 std-lib 把 ECMA-376 **Part 4 Transitional** XSD 生成 Go 只读投影（`schema.openxmlformats.org`，非 Part 1 Strict 的 `purl.oclc.org`）。`internal/ooxml`（Open / Bytes / `SlideShapes` 只读投影）+ `engine.ProjectIR` 驱动形状/文本/表格/notes/timing/hidden/图表的只读投影，`projectShapes` 全路径零门面句柄读取——IDE 友好且避免投影逻辑反向触碰可写句柄。

---

## 4. 功能特性

### 4.1 文档生命周期

```go
// 创建空演示文稿
pres, err := pptx.New()
defer pres.Close()

// 打开已有
pres, err = pptx.Open("./template.pptx")
pres, err = pptx.OpenReader(reader, size)  // 从流式输入

// 修改...
slide := pres.Slides()[0]
shape := slide.Shapes()[0]
tf    := shape.TextFrame()
tf.ReplaceText("Q3", "Q4", pptx.ReplaceModeExact)

// 保存
err = pres.Save("./out.pptx", pptx.WithSaveOverwrite(true))   // 显式覆盖
err = pres.Write(io.Discard)                                   // 仅内存序列化（基准/预览）

// 资源预算与持久化档位（V2.0.1 经 pptx 别名对外可用）
pres, err = pptx.Open("./in.pptx",
    pptx.WithBudget(pptx.Budget{MaxEntries: 5000, MaxUncompressedBytes: 256 << 20}),
    pptx.WithSaveDurability(pptx.DurabilityFull),
)
```

完整生命周期覆盖：创建 / 打开 / 流式打开 / 修改 / 保存（原子）/ 落盘序列化 / 仅内存序列化 / 关闭（幂等）。

### 4.2 形状 / 几何 / 图片

- **形状分类**：`ShapeKind` 枚举 + 8 个类型化句柄（`GroupShape` / `AutoShape` / `OpaqueShape` / `PictureShape` / `TableShape` / `ChartShape` / `AudioShape` / `VideoShape`）+ `TextShape` 别名
- **几何**：EMU 单位、Point / Rect / Quad 值对象、190+ `prstGeom` 预设 + 自定义几何只读
- **效果模型**：阴影 / 发光 / 反射 / 柔边 / 3D 场景的只读报告
- **样式矩阵**：theme `fmtScheme` 引用链解析，本地 / 继承 / 布局三层格式区分
- **图片插入**：自动建 relationship、自动 content type、自动媒体去重（按内容哈希；V2.0.1 增加 part→hash 缓存消除 O(N²)）

### 4.3 表格与图表

- **表格**：逻辑网格、单元格合并 / 拆分、单元格边框与对齐（`tcPr`）、样式分区 12 标志位优先级矩阵
- **图表（受限三类）**：柱状 / 折线 / 饼图；扩展标签 / 误差线 / 趋势线 / 日期轴 / 对数轴
- **图表 workbook**：v2.0 起实现层全在 `internal/chart`（ADR-017），门面公共 API 零变化；原 V1 的 `ChartWorkbookBuilder` 等 Experimental 类型已随 D-5 升 Stable

### 4.4 媒体（音频 / 视频 / 图片）

- **音频嵌入**：含配音挂接、受限 ID 分配、时长/采样率/声道探测
- **视频媒体形状**：复用 MEDIA/AUDIO 机制，poster frame 处理
- **媒体探测**：内置 `internal/audioprobe` / `internal/videoprobe`，不调用 FFmpeg
- **媒体去重**：按内容 SHA-256 哈希去重（V2.0.1 增加按 revision 失效的哈希缓存）

### 4.5 文本编辑（TEXT-02 跨 Run 替换）

- **富文本模型**：`TextFrame → Paragraph → TextRun` 三层，与 OOXML 一一对应
- **跨 Run 替换**：目标串跨越多个 Run 边界时自动规划最小 patch，保证前后 Run 字节级保留
- **替换模式**：`ReplaceMode` 枚举（`Exact` / `Regex` / `Glob` …）
- **plain text 替换**：`SetPlainText` 用于模板批量替换场景，绕过富文本结构

### 4.6 页面复制（CLONE-01 / CLONE-02）

- **同文档受限复制**（CLONE-01）：保留版面与母版引用，自动分配新 cNvPr@id
- **跨文档受限复制**（CLONE-02）：`CopySlideFrom` + `clonePlan` 显式区分 srcDoc / crossDoc

### 4.7 模板数据绑定（TPL-01）

`bind.go` 子系统提供 Mustache 风格占位符替换（`{{path}}` / `{{#if expr}} … {{/if}}` / `{{#each list}} … {{/each}}`）。绑定器构建在保真 patch 之上（ADR-013）：未命中占位符视为未知内容保留；命中失败返回诊断。

### 4.8 动画时序与只读 IR（TIMIR-01 / DIFF-01）

- **只读 timing IR**：解析 `p:timing` 子树、`seq` / `par` / `excl`、动作路径；最小时间求值器检测 `ErrTimingConflict` / `ErrDurationUnknown`
- **语义 diff（DIFF-01）**：`pptx diff` CLI 产出 `go-pptx.diff/1.0` 格式——页面对齐用 SlideID 强匹配 + 形状 ID Jaccard 相似度（加权 LCS）；页内配对基于 ShapeID；opaque diff 返回 `Part` + `NodePath` 回溯
- **IR 投影**：`internal/engine.ProjectIR` 经 `internal/ooxml` 只读投影，CLI `export-ir` 子命令输出 JSON

### 4.9 自动讲解与播放（AUDIO-02）

与配套的"PPT 自动讲解工具"集成：富文本备注 → 配音文本源；音频嵌入 + 受限 ID 分配 + 时长探测 → 页面时长自动填入；同步操作诊断（缺音频、配音冲突、时长冲突）均返回结构化错误。

### 4.10 渲染契约（RENDER-01）

- **接口落地**：`Renderer` / `RenderOptions` / `RenderCapabilities` / `RenderedSlide` / `RenderAll` 契约（`render` 子包）
- **实现未立项**：原生渲染按 ADR-014 不进入核心里程碑；接口已可在 wasm 编译通过
- **现状披露**：`rendering.native` 能力状态在 Capability Manifest 中如实保留为 `Untested`

---

## 5. API 稳定性分级体系

go-pptx 公开承诺**三级稳定性**（[ADR-015](adr/ADR-015-api-stability-tiers.md)），godoc 段落标签为唯一标记：

| 标签 | godoc 标记 | 兼容性承诺 |
|---|---|---|
| **Stable** | `// Stable: <reason>` 紧贴 type/func 上方 | v2.0 后承诺向后兼容——只增不破；行为/语义可加严但不可放宽；允许弃用但必须保留替代入口至少两个小版本 |
| **API（默认）** | 无标签 | v2.x 锁定前可调整字段；锁定承诺由 ADR-015 §"v2.0 锁定流程"规定 |
| **Deprecated** | Go 1.19+ 标准 `// Deprecated: <替代>` 标签 | v2.x 期间通常不引入；保留至少两个小版本 |

**错误码 / 错误类型默认 Stable**——错误码是 API 兼容契约的硬约束（17 个 `Err*` 哨兵 + `OperationError`）。

### 5.1 V2.0 冻结面（金样门禁，`pptx/api_surface_test.go`）

V2.0.1 的金样（权威值，写文档一律以此为准）：

| 维度 | 数量 |
|---|---|
| 公共 API type 总数 | **166** |
| `// Stable:` 段落数 | **42** |
| Stable 符号数 | **65** |
| Stable 方法数 | **134** |
| `// Experimental:` 段落数 | **0** |
| 错误哨兵 | **17** |

V2.0.1 相比 V2.0.0 的**纯追加**（零破坏性）：新增 `pptx.Budget` / `pptx.Durability` / `pptx.PartName` 三个 `type alias`（均在 `pptx/aliases.go`，Stable），并配套 `pptx.DefaultBudget()` 构造函数与 `pptx.DurabilityDefault` / `pptx.DurabilityFull` 取值常量；`pptx.PartName` 的 `String()` / `Valid()` / `EntryName()` 进入 Stable 方法面。别名与原 `internal/opc` 实体是**同一类型**，零转换、零语义漂移——修的是"此前 `WithBudget` 等直接引用 internal 类型、外部模块根本无法引用（编译报 `use of internal package`）"的契约缺口。

> **金样守卫盲区（V2.0.1 已知）**：`api_surface_test.go` 的 `loadAPISurface` 只遍历 `*ast.GenDecl`，**顶层导出函数**（如 `DefaultBudget` / `New` / `Open` / `PartBytes`）的新增或删除不会触发任何门禁失败。新增函数类 API 需人工核对 `docs/api-reference.md` 与 `RELEASE-NOTES` 计数。

### 5.2 升降档规则（ADR-015 §"决策"）

- **升档**：必须证明公共面契约收敛、有失败回退路径，并在 ADR-015 `#changelog` 登记
- **降档**：视为破坏性变更，必须走 ADR 评审，在跟踪文档顶部"破坏性变更公告"段登记，至少两个小版本过渡期并存替代入口
- **新增 Stable**：仅限"通用对象模型入口"或"错误/诊断契约"；加性工作一律默认 API 级

---

## 6. 质量保障与工程纪律

### 6.1 测试矩阵

CI 在每次 push 与 PR 上执行：

| Job | 内容 |
|---|---|
| `lint` | `gofmt -l .` + `go vet`（默认 + `corpus` build tag） |
| 默认测试 | `go test ./...`（**28 包** ok） |
| `corpus-replay` | `go test -tags=corpus ./... -v`（**28 包** ok，含 B1 金样字节级比对） |
| 交叉构建 | `js/wasm` / `darwin/arm64` / `wasip1/wasm` / `linux/arm64` / `windows/amd64` 五目标 |
| `coverage-gate` | `scripts/coverage/gate.sh`（COV-01..04 门槛，全包登记，未登记即 FAIL） |
| `perf-smoke` | 基准套件可编译、三档语料全执行、报告生成器可解析、pkg-bytes 下界守护 |

### 6.2 真实语料回归（CORPUS-01）

- **公开样本（CI 可复现）**：3 个 LibreOffice 生成金样（`s001-text` / `s002-table` / `s003-image`），分发许可完整
- **私有样本（索引但不分发）**：33 个 `ext-*`（含 WPS 源、限制许可样本）作为索引存在，不纳入开源分发
- **金样验证契约**：以 replay 后字节级与原始一致作为门禁；任何改动引入的新差异区间必须由回归测试锁定
- **垂直验证**：`ext-0024` 单 Run 替换差异区间收敛到 1 字节（自动化）

### 6.3 性能基线（PERF-01）与 V2.0.1 性能修复

**三档语料**（`docs/PERF-01-性能基线.md`）：`10p-text`（10 页纯文本）/ `50p-image`（50 页 + 小图）/ `100p-media`（100 页 + 20 张互不相同大图）。六类基准：`Open` / `Traverse` / `Replace` / `SaveMem` / `SaveDisk` / `PeakHeap`。

**V2.0.1 性能修复实测**（`BenchmarkPerfTraverse/100p-media`，`-benchtime=6x`）：

| 指标 | 修复前 | 修复后 | 降幅 |
|---|---:|---:|---:|
| 耗时 | 16.40 ms | **3.96 ms** | −75.8% |
| 内存 | 26.77 MB | **1.17 MB** | −95.6% |
| allocs | 223,284 | **10,713** | −95.2% |

根因：`Presentation.Slides()` 原本在循环内重复 `relsOf(p.main)`（循环不变量），且 `AddPicture` 去重对每个已存媒体全量读字节再 SHA256（O(N²)）；V2.0.1 将关系解析提到环外、新增随 revision 失效的媒体哈希缓存。

> **披露纪律**：性能数据与硬件/工具链强相关；跨机器比较须先固定平台与 Go 版本；不写"比 Python 快多少倍"（V2.6 §15.3：在没有基线前不写）。

### 6.4 Fuzz 与安全

- **原生 fuzz 目标**：打开链路（`opc.Load` / `opc.Scan` / `xmlstore.Scanner` / `xmlstore.Index` / `OpenReader`）+ 文本编辑（`ReplaceText` / `SetPlainText`）+ 模板绑定（`FuzzBind`）
- **不可信输入防护**：OPC 资源预算（Part 数 / 中央目录尺寸 / 解压上限）拒绝超大包；V2.0.1 新增图表 `c:pt/@idx` 上界（防 `make([]string, max+1)` 的 `makeslice` panic / ~16GB 分配，经公共 API `ChartShape.Data()` 可达）与 MP4 `ftyp` 兼容品牌收集上限 64（防 21× 放大）
- **恶意包处理**：三入口（`Open` / `OpenReader` / `New`）显式错误返回，不 panic

### 6.5 工程纪律

- Go 反模式扫描：`fmt.Sprintf` 拼 XML 仅 2 处（已知 schema + 整型）；裸 XML 常量 0 处
- 错误用 `error` 不 panic；文档/状态文件用中文；commit message 带 `AI-assisted: WorkBuddy` 标注（V1.4 规范符合项）
- `internal/archlint` 把依赖方向纳入 `go test`，新包 / 反向依赖立即可见
- 仓库级 git 身份：`go-pptx-dev <go-pptx-dev@local>`（仅仓库内，不外发）

---

## 7. 工具链与命令行

`cmd/pptx` 子模块提供 **9 个功能性 CLI 子命令**（另含 `-h/--help`、`--version`）：

| 子命令 | 功能 | 适用场景 |
|---|---|---|
| `pptx capability` | 输出六维能力 manifest | 环境审计 / 部署前能力确认 |
| `pptx inspect` | 只读报告（几何 / 填充 / 效果 / 样式矩阵 / 版面信息） | 内容审计 / 调试 |
| `pptx validate <file>` | 诊断报告（`ValidateOption` 控制级别） | 发布前自检 |
| `pptx replace` | 文本/字段批量替换 | 模板填充 |
| `pptx narrate` | 备注 → 配音/讲解文本源导出 | 自动讲解链路 |
| `pptx timing-plan` | 翻页计时方案导出 | 放映编排 |
| `pptx export-ir [--output\|--stdout]` | 导出只读 IR JSON | 第三方消费 / 调试 |
| `pptx bind <tpl> <data> <out>` | 模板数据绑定（`{{path}}` / `{{#if}}` / `{{#each}}`） | 批量报告生成 |
| `pptx diff <a> <b>` | 两个文档的语义 diff（`go-pptx.diff/1.0`） | 回归审计 / 文档比对 |

> CLI 健壮性（V2.0.1）：子命令 `--help` 走 stdout + 退出 0；真正的参数错误走 stderr + 退出 2；覆盖保护改为由库在一次原子检查内判定（`ErrOutputExists`），消除 Stat-then-Save 的 TOCTOU 竞态。

### 7.1 Capability Manifest（六维）

| 维度 | 状态 | 说明 |
|---|---|---|
| Inspect | Supported | 完整只读检查能力 |
| Create | Partial | 受支持特性可创建 |
| Edit | Partial | TEXT-02 跨 Run + 表格 + 媒体 + clone + bind |
| Preserve | Supported | 字节级保真（垂直验证 `ext-0024` 单 Run 差异收敛到 1 字节） |
| Render | Untested | 接口已落地（`render` 子包），实现未立项 |
| Play | Partial | TIMIR-01 计时只读 + AUDIO-02 配音挂接 |

### 7.2 WASM 浏览器侧工具（`wasm/check`）

- `GOOS=js GOARCH=wasm` 编译为单文件 wasm
- 提供只读的 check / inspect / validate 三类浏览器侧操作
- 适用场景：上传前的客户端预检、不愿将敏感 PPT 上传到服务器的审计场景

---

## 8. 产品特色与差异化优势

### 8.1 与主流同类方案的对比定位

| 维度 | go-pptx V2.0 | python-pptx | 其他 Go 库 |
|---|---|---|---|
| 语言 | **Go** | Python | Go |
| CGO | **CGO=0** | — | 大多 cgo 依赖 |
| 跨平台 | **5 平台 CI** | 受 Python 解释器限制 | 多数仅 linux/darwin |
| 字节级保真 | **是**（xmlstore span patch） | 部分 | 否 |
| 受控编辑失败 | **显式错误**（Diagnostic/Severity） | 部分抛异常 | 多为静默损坏 |
| API 稳定性分级 | **三级 + godoc 标记**（ADR-015） | 无 | 无 |
| 真实语料回归 | **36 样本索引 + 3 公开金样 CI** | 无强制 | 无 |
| Fuzz 内建 | **原生目标** | 无 | 极少数 |
| 性能基线 | **三档 × 六类** 自动报告 | 无 | 无 |
| 语义 diff | **DIFF-01**（加权 LCS + Jaccard） | 无 | 无 |
| WASM 浏览器侧 | **原生支持** | 不适用 | 极少数 |
| 架构可演进性 | **engine 编排层 + archlint 依赖门禁 + ooxml 生成管线** | 无对应 | 无 |

### 8.2 五大差异化优势

1. **服务端原生 + 跨平台**：唯一在 `js/wasm` / `darwin/arm64` / `wasip1/wasm` / `linux/arm64` / `windows/amd64` 五平台有 CI 验证的 PPTX 库；可作为任何 Go 服务的静态依赖，不引入解释器或 C 工具链。
2. **字节级保真的受控编辑**：xmlstore span patch 引擎保证未触及子树字节级保留；同时拒绝不安全操作而非静默损坏——这是"工程可靠性"的核心。
3. **三级 API 稳定性承诺**：ADR-015 godoc 段落标记 + 升降档规则 + 金样门禁 + 实施跟踪公开通告——下游可基于白纸黑字的承诺做升级决策。
4. **真实语料回归 + 性能护栏 + fuzz 三位一体**：36 样本索引、3 公开金样 CI 自动 replay、三档性能基线、`coverage-gate` 全包门槛、原生 fuzz 目标——把"能工作"和"能持续工作"分开度量。
5. **从编辑器到浏览器的完整链路 + 可演进架构**：服务端编辑 → 语义 diff → 模板批量绑定 → 浏览器原生只读审计；所有环节在同一代码库、同一真实语料回归体系下闭环；`engine` / `archlint` / `ooxml` 生成管线让内部实现可垂直演进而不污染公共门面。

---

## 9. 技术创新点

### 9.1 STALE-GUARD：句柄身份的三层闭环（业界首创）

**问题**：任何"句柄式 API"都要回答"句柄如何安全失效"。业界常用方案是单独维护一份 ID 表，导致两套真相、易出 bug。

**go-pptx 的方案**：所有句柄身份 = 所属形状的 cNvPr@id（OOXML 已要求 spTree 内全局唯一）；path 仅作父容器提示；locate 流程比对最近 `p:sp` / `p:cxnSp` / `p:graphicFrame` / `p:grpSp` 的 cNvPr@id 与 hint；`hint=0` 走纯路径判定。

**创新点**：复用 OOXML 已经存在的稳定身份，把"句柄有效性"从组件自行维护的元数据降级为"对底层 XML 的可推导属性"。V2.0.1 进一步使 `Close()` 幂等，句柄失效语义在并发/defer 场景下更健壮。

### 9.2 xmlstore：命名空间感知的最小侵入 patch 引擎

**问题**：PPTX 的 XML 充满命名空间，传统 XML 编辑器要么强行 schema 化（破坏未知内容）、要么全文本替换（破坏无关内容）。

**go-pptx 的方案**：节点索引树 + span patch 引擎 + 命名空间受控新建 + 未知内容保护。垂直验证 `ext-0024` 单 Run 替换差异区间收敛到 **1 字节**。

### 9.3 DIFF-01：加权 LCS + Jaccard 双相似度页面对齐

先用 SlideID 锚定明确匹配，再用形状 ID Jaccard 配对"被移动 / 被复制"的页面；`go-pptx.diff/1.0` 格式独立可被第三方消费。

### 9.4 Capability Manifest：六维能力披露

六维度（`Inspect` / `Create` / `Edit` / `Preserve` / `Render` / `Play`）+ 每维四档（`Supported` / `Partial` / `Untested` / `Unsupported`）+ 可消费 JSON。把"能力分维度 + 分档位"作为一等公民公开（`pptx capability` CLI），`Preserve=Supported` 是关键卖点。

### 9.5 三级 API 稳定性承诺与金样门禁（ADR-015）

godoc 段落标记 `// Stable:` / `// Experimental:` 直接在源码声明；升降档规则写成 ADR；`pptx/api_surface_test.go` 把 166 类型 / 42 Stable 段 / 65 符号 / 134 方法 / 17 哨兵固化为编译期门禁。

### 9.6 公开金样 CI 闭环（CORP-01）

公开 3 个 LibreOffice 生成金样（分发许可完整），`//go:build corpus` 隔离，CI 自动 replay；私有 33 个 `ext-*` 仅作索引（不入开源分发）。把"语料可复现性"与"真实覆盖度"作为两套独立维度分别披露。

### 9.7 原子保存 + 并发修订快照

原子保存（临时文件 + fsync + rename，默认拒绝覆盖）；修订快照（Revision 单调递增，旧 Revision 句柄写操作触发 `ErrConcurrentModification`）；V2.0.1 的 `Durability` 档位 + `Budget` 预算把"不可信输入"与"落盘强度"显式化为可配置契约。

### 9.8 编排层 + 依赖门禁 + OOXML 生成管线（ADR-030，V2.0 新增）

**问题**：门面同时承载公共 API 与大量领域实现，CLI / WASM 各写一套核心会造成实现漂移与反向依赖。

**go-pptx 的方案**：① `internal/engine` 统一 CLI 与 WASM 的 Inspect/Validate/Capability/ProjectIR；② `internal/archlint` 把依赖方向纳入 `go test`，新包/反向依赖立即可见；③ `scripts/gen/schema` 从 ECMA-376 Part 4 Transitional XSD 自动生成只读 OOXML 投影（`internal/ooxml/schema`），`projectShapes` 全路径零门面句柄读取。

**创新点**：把"可演进的内部架构"作为产品能力公开——公共门面在 v2.x 全程零破坏性变更的前提下，内部实现可垂直下沉、可被生成管线替代，且由 CI 强制保持依赖方向健康。

---

## 10. 典型应用场景

### 10.1 批量报告 / 培训材料生成

```go
pres, _ := pptx.Open("./quarterly-report-template.pptx")
data := loadQ4Financials()
out  := bytes.Buffer{}
pres.Bind(data)                // 应用 {{path}} / {{#each regions}} / {{#if netPositive}}
pres.Write(&out)              // 仅内存序列化（管道友好）
```

适用：财务季报 / 周会材料 / 培训讲义 / 投标响应。

### 10.2 内容审计与 AI 抽取

```go
pres, _ := pptx.Open("./external-deck.pptx")
report := pres.Inspect()       // 几何 / 填充 / 效果 / 样式矩阵 / 版面信息
for _, slide := range pres.Slides() {
    for _, shape := range slide.Shapes() {
        for _, para := range shape.TextFrame().Paragraphs() {
            // 喂给搜索 / RAG / 内容审核
        }
    }
}
```

适用：知识库 / 内部搜索 / 内容审计 / AI 训练数据抽取。

### 10.3 文档比对与回归确认

```bash
$ pptx diff ./old-version.pptx ./new-version.pptx > diff.json
```

适用：法务合同版本对比 / 翻译前后对比 / 编辑工作流审计。

### 10.4 浏览器侧隐私敏感审计（`wasm/check`）

把 `pptx check` 编译为 wasm，在浏览器内做只读能力确认 / inspect / validate——文件不出端。适用：律所 / 医疗 / 金融等不能把客户文件上传到服务器的场景。

### 10.5 自动讲解产品集成（AUDIO-02 + AUTO-PPT）

富文本备注 → TTS 文本源；音频嵌入 → 页面时长自动填入；`ErrTimingConflict` / `ErrDurationUnknown` → 结构化修复提示。适用：在线教育 / 培训自动化 / 无障碍辅助。

### 10.6 模板克隆 + 个性化（CLONE-01 / CLONE-02）

```go
tpl, _ := pptx.Open("./company-template.pptx")
defer tpl.Close()

for _, client := range clients {
    pres, _ := pptx.New()
    pres.CopySlideFrom(tpl, slideIdx)         // 跨文档受限复制
    pres.Slides()[0].Bind(client.Data)        // 个性化数据
    pres.Save(fmt.Sprintf("./%s.pptx", client.Name), pptx.WithSaveOverwrite(true))
}
```

适用：客户提案 / 营销材料个性化 / 报告分发。

---

## 11. 已知边界与 V2.x 路线图

### 11.1 V2.0.1 已披露的边界

| 项 | 状态 | 详情 |
|---|---|---|
| **L3 客户端矩阵**（PowerPoint / WPS 真机打开 + 编辑后重存） | ❌ 未验证 | 本机无真机环境；外部依赖；属发布级缺口但非硬门槛 |
| **加密 / 签名 / OLE** | 识别 + 拒绝编辑 | 按 V2.6 §2.2 边界表：识别格式、显式返回不支持，不静默转换 |
| **动画播放求值** | `ErrTimingConflict` 检测 | 最小时间求值器；不承诺完整播放行为计算 |
| **`rendering.native`** 能力状态 | `Untested` | `render` 子包接口已落地，实现后续立项（ADR-014） |
| **私有 `ext-*` 样本** | 不分发 | 许可 / 体积限制；CI 用 3 个公开金样兜底 |
| **金样守卫盲区** | 已知 | `loadAPISurface` 只扫 `*ast.GenDecl`，顶层导出函数新增/删除无门禁（见 §5.1） |
| **超长文件 / 函数拆分** | 暂缓 | 需先接入 golangci-lint 才有客观阈值 |
| **`Open` 系列补 `ctx`** | 暂缓 | 属签名级变更，留给 minor 版本 |
| **Tier 2 raw 直通** | 低命中率 | ADR-018 原始透传路径命中率仍偏低，未纳入本版重点 |

> 注：V2.0.1 已**修复** V1 时代"承诺了却拧不动"的资源预算旋钮——`WithBudget` / `WithNewBudget` / `WithSaveDurability` 此前直接引用 `internal/opc` 类型，外部模块无法引用；V2.0.1 以 `pptx` 包别名补齐（§5.1）。

### 11.2 V2.x 路线图

- **2.0.x（补丁线）**：持续收敛覆盖率向 90%、bug 修复、金样守卫盲区补强（纳入顶层函数类 API 的计数校验）、`golangci-lint` 接入后启动超长文件/函数拆分。
- **2.1.x**：`Open` 系列补 `ctx` 参数（签名级变更，需 ADR 评审 + 过渡期）；Tier 2 raw 直通命中率提升。
- **2.2.x**：实验能力按使用情况决定升档或弃用；`internal` 深层抽取按 ADR-014 触发条件推进。
- **3.0（如启动）**：原生渲染实现，遵循 ADR-014 不进入核心里程碑原则，需独立立项。

### 11.3 演进原则

- **不破坏 Stable**：V2.x 期间 42 个 Stable 段落（65 符号）承诺向后兼容
- **API 默认可加**：其余类型允许追加新字段与方法；不允许删除或重命名
- **架构压力按触发条件拆**（ADR-014）：合并冲突热点 / 第三方扩展需求 / 编译时间瓶颈任一触发才启动深层抽取

---

## 12. 版本、许可与社区

### 12.1 版本与签名

- **当前版本**：**V2.0.1**（2026-09-18 发布）
- **Git tag**：`v2.0.1`（SSH 签名附注标签，绑定 GitHub 账号 `jinfeng105` 的 ed25519 公钥）；`v2.0.0` 为同一系列的发布 tag
- **commit 基线**：`0258392`（V2.0.1 收口）→ 历史 `8158b53`（V2.0.0）、`a15ba75`（V2.0.0 发布）
- **模块/导入**：`github.com/F31/go-pptx/v2`；门面 `github.com/F31/go-pptx/v2/pptx`（V2.0 一次性 breaking 迁移，V1 的 `github.com/F31/go-pptx` 不再适用）
- **CI**：每次 push / PR 跑默认 28 包测试 + corpus 28 包 + 5 交叉构建 + coverage-gate + lint

### 12.2 许可证

- **代码**：Apache-2.0（与远程仓库 LICENSE 字节一致）
- **公开金样**（`s001-text` / `s002-table` / `s003-image`）：CC0 / Public Domain（LibreOffice 生成）
- **私有 `ext-*` 样本**：仅作索引，不纳入开源分发（许可 / 体积原因）

### 12.3 仓库与社区资源

| 资源 | 路径 |
|---|---|
| 仓库 | [`github.com/F31/go-pptx`](https://github.com/F31/go-pptx) |
| 模块路径 | `github.com/F31/go-pptx/v2` |
| 公共门面 | `github.com/F31/go-pptx/v2/pptx` |
| 发布通告 | `docs/RELEASE-NOTES-v2.0.0.md` / `docs/RELEASE-NOTES-v2.0.1.md` |
| 变更日志 | `CHANGELOG.md` |
| API 参考（自动生成） | `docs/api-reference.md`（166 / 37 / 143 / 151 / 18） |
| 架构基线 | `docs/architecture-current.md` |
| API 稳定性分级 | `docs/adr/ADR-015-api-stability-tiers.md` |
| 内部包拆分策略 | `docs/adr/ADR-014-root-internal-package-strategy.md` |
| 渐进式 internal 抽取 | `docs/adr/ADR-016-progressive-internal-extraction.md` |
| chart 实现层抽取 | `docs/adr/ADR-017-chart-implementation-extraction.md` |
| OPC Tier 2 原始透传 | `docs/adr/ADR-018-opc-tier2-raw-passthrough.md` |
| Shape 能力接口 | `docs/adr/ADR-021-shape-capability-interfaces.md` |
| 门面收敛 / 目标架构 | `docs/adr/ADR-030-v2-target-architecture.md` |
| 性能基线方法论 | `docs/PERF-01-性能基线.md` |
| 语料入库指南 | `docs/corpus-入库指南.md` |
| 代码评审（V2.0.1 来源） | `docs/code-review-2026-09-17.md` |
| 编码规范评估 | `docs/coding-standards-assessment-2026-09-17.md` |

### 12.4 联系方式

欢迎交流与提交需求建议：

- 邮箱：jinfeng105@126.com
- 邮箱：jinfeng105@gmail.com

### 12.5 反馈与升级指引

- **升级到 V2.0**：V2.0 是一次性 breaking 迁移（导入路径 `+ /pptx`），无 API 签名变化，仅需替换 import；42 个 Stable 段落（65 符号）承诺向后兼容——升级不会破坏现有调用。
- **V2.0 → V2.0.1**：纯追加 + 语义放宽（`Close()` 幂等），无需改动即可升级；唯一需适配的是依赖"二次 `Close` 返回错误"的测试断言。
- **Experimental 用户**：留意公告；如有破坏性变更，至少两个小版本过渡。
- **问题反馈**：通过 GitHub Issues（首选）；附最小复现 + 原始 PPTX 哈希（不含文件本体）。

---

> **白皮书维护约定**：与 [`CHANGELOG.md`](../CHANGELOG.md) 同步；每个小版本（2.0.x）发布时由负责人更新"版本 / 路线图 / 创新点落地"段；纯文档工作纳入 `.commit-msg-*.txt` 工作流。
