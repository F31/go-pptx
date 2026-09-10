# go-pptx 技术白皮书

> **版本**：v1.0.0 · 2026-09-11
> **代码基线**：[`github.com/F31/go-pptx`](https://github.com/F31/go-pptx) `v1.0.0`（SSH 签名 tag，commit `44b9ba7`）
> **设计基线**：《go-pptx 完整设计方案 V2.6 开发实施版》（`docs/go-pptx_完整设计方案_V2_6_开发实施版.md`）
> **文档定位**：面向技术决策者、架构师与潜在集成方的综合性技术披露；与 [`RELEASE-NOTES-v1.0.0.md`](RELEASE-NOTES-v1.0.0.md)（发布通告）和 [`CHANGELOG.md`](../CHANGELOG.md)（变更日志）互补。
> **关联 ADR**：ADR-014（内部包拆分策略）、ADR-015（API 稳定性分级）、ADR-016（渐进式 internal 实现层抽取）。

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
11. [已知边界与 1.x 路线图](#11-已知边界与-1x-路线图)
12. [版本、许可与社区](#12-版本许可与社区)

---

## 1. 产品概述

**go-pptx** 是一个面向服务端与工具开发者的纯 Go 演示文稿组件，提供类似 python-pptx 的对象模型体验（`Presentation → Slide → Shape → TextFrame → Paragraph → Run`），同时在字节级保真、命名空间安全、事务化编辑与跨平台分发四个维度上做了系统性投入，可作为静态依赖嵌入任何 Go 服务端应用，无需 Python、Microsoft Office、FFmpeg 或 CGO。

v1.0.0 是组件的第一个稳定版本，完成了《项目实施计划》M0–M8 全部里程碑：

| 维度 | 现状 |
|---|---|
| 导出类型总数 | 158（含 2 个 `type alias`） |
| Stable 段落 | 51（34 个独立 type + 1 个错误码集合段聚合 17 哨兵） |
| Experimental | 5（chart workbook / 自定义属性） |
| API 默认级 | 102（允许追加新字段，不破坏已有签名） |
| Deprecated | 0 |
| 默认测试 | 9 包 0 失败 |
| 公开金样回归 | 3 个 LibreOffice 生成样本（`s001-text` / `s002-table` / `s003-image`）+ 33 私有 `ext-*` 索引 |
| 交叉构建验证 | `js/wasm` · `darwin/arm64` · `wasip1/wasm` · `linux/arm64` 四平台零失败 |
| 覆盖率 | 根包 77.1% / `cmd/pptx` 86.6%（V2.6 §15.3 目标 90%，非硬性门槛） |

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
| 模块路径 | `github.com/F31/go-pptx` |
| 公共门面 | 唯一公共入口为根包 `pptx`；其余包以 `internal/` 隔离（`opc` / `xmlstore` / `document` / `textmap` 等） |
| 错误约定 | 普通错误以 `error` 返回；不使用 panic；不通过返回 nil 掩盖未支持能力 |
| 接口便捷性 | Options、Spec、高层组合方法；不引入吞掉错误的链式 Setter |
| 能力披露 | 保真 / 兼容 / 渲染 / 播放分别声明能力范围；发布声明必须由测试证据支持 |

### 2.3 MVP 双闭环

项目以两个最小闭环作为产品成型的最低门槛：

- **闭环 1（模板报告）**：模板打开 → 读取讲稿 → 更新文本/备注 → 保存
- **闭环 2（自动讲解）**：音频嵌入 → 计时 → 放映

两个闭环均已在 v1.0.0 中以真实语料回归用例覆盖。

---

## 3. 技术架构

### 3.1 包模型与依赖方向

go-pptx 采用"**根包做门面、内部包承实现**"的依赖结构，确保用户代码只面对一个稳定命名空间，同时保留实现层的演进空间：

```
                        ┌───────────────────────┐
                        │      cmd/pptx         │  ← CLI 入口（pptx capability / inspect / diff / bind / validate）
                        └──────┬─────────────┬──┘
                               │             │
                ┌──────────────▼─┐         ┌─▼─────────────┐
                │  wasm/check    │         │  ir           │  ← 只读 IR + 语义 diff + 计时
                └──────┬─────────┘         └─┬─────────────┘
                       │                     │
                       └────────┬────────────┘
                                ▼
                ┌───────────────────────────┐
                │   package pptx (根)       │  ← 公共 SDK 门面 + 主要领域逻辑
                │   74 .go · ~35.6k 行      │     158 导出类型 / 732 导出符号
                └──────┬──────┬──────┬──────┘
                       │      │      │
              ┌────────▼─┐  ┌─▼────┐ ┌▼────────────┐ ┌▼──────────┐
              │internal/ │  │internal/ │ internal/    │ internal/  │
              │  opc     │  │xmlstore  │ {audio,video}│ document   │
              │ (ZIP/    │  │(XML 扫描/│  probe       │ (PartStore │
              │  关系)   │  │ 索引/patch)│             │  契约)     │
              └──────────┘  └─────┬───┘ └─────────────┘ └───────────┘
                                  │
                            ┌─────▼──────┐
                            │internal/   │
                            │textmap     │  ← TEXT-02 纯文本映射原语
                            └────────────┘
```

**依赖方向纪律**（ADR-014 / ADR-016）：

- ✅ **允许**：根包 → 全部 `internal/*`；`internal/xmlstore` → `internal/textmap`；`internal/opc` → `internal/xmlstore`；`ir` → `internal/xmlstore`
- ❌ **禁止**：`internal/*` → 根包（避免循环依赖、避免内部实现反向污染公共 API）
- ❌ **禁止**：根包公开新公开方法只为适配 internal 包（强制门禁写于 ADR-016）

当前已落地的 internal 包与职责：

| 包 | 职责 |
|---|---|
| `internal/opc` | OPC ZIP 索引、内容类型、关系图、保存规划、原子保存 |
| `internal/xmlstore` | 命名空间感知 XML 扫描器、节点索引树、span patch 引擎 |
| `internal/textmap` | TEXT-02 跨 Run 替换的纯文本映射原语（rune 匹配 / 字素边界 / run span 定位） |
| `internal/document` | `PartStore` / `ReadStore` / `PatchStore` 最小文档存储契约（ADR-016 第一批落地） |
| `internal/editplan` | 编辑计划与冲突检测骨架 |
| `internal/audioprobe` | 音频容器探测（时长、采样率、声道） |
| `internal/videoprobe` | 视频容器探测（分辨率、时长、编码） |

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

每个对象的关键约定：

| 对象 | 关键能力 | 句柄身份语义 |
|---|---|---|
| `Presentation` | 文档生命周期 / `Capability` 六维能力报告 | — |
| `Slide` | 页面管理、备注、版面诊断 | `SlideID` (`uint32`，唯一持久身份) |
| `Shape` | 几何 / 样式 / 效果报告、子节点枚举 | `ShapeID` = cNvPr@id（在 spTree 内全局唯一） |
| `TextFrame` | 跨 Run 替换、plain text 替换、字段、Autofit 诊断 | shapeHint = 所属形状 cNvPr@id |
| `Paragraph` | 段落属性 (alignment, spacing, bullets) | shapeHint |
| `TextRun` | Run 属性 (font, color, bold, …) + 文本内容 | shapeHint |

### 3.3 OPC 层：包模型与原子保存

OPC（Open Packaging Conventions）是 PPTX 的物理容器。`internal/opc` 包对 OPC 做了三个关键抽象：

1. **ZIP 索引 + 预算**：打开时仅做中央目录解析与轻量索引；不立即解压全部 Part；媒体字节流惰性按需读取。
2. **关系图 + 内容类型**：完整建模 `_rels/.rels` 与各 Part 的 `Content_Types.xml`，提供 `RelationshipType` / `ContentType` 常量体系（避免硬编码散落）。
3. **保存规划**：区分三类写入路径（V2.6 §4.2）——
   - **完全透传 Part**（未修改 → 字节级保留）
   - **修改 Part**（走 xmlstore span patch）
   - **新增 Part**（如媒体、克隆页）走命名空间受控的新建路径

**原子保存**：`Save()` 默认采用"先写临时文件 + fsync + rename"策略，禁止撕裂写入；默认拒绝覆盖（返回 `ErrOutputExists`），调用方需显式 `WithOverwrite(true)` 才会覆盖。

### 3.4 XML store：命名空间感知的最小侵入编辑引擎

PPTX 的实际内容是大量带命名空间前缀的 XML。`internal/xmlstore` 是组件最具技术含量的内部子系统，提供：

- **命名空间感知扫描器**：识别 `p:` / `a:` / `r:` / `c:` / `mc:` / `p14:` 等命名空间，不强制 schema，对未知元素保留原文。
- **节点索引树**：把每个 XML Part 建成节点位置索引，外部代码可通过 `NodePath`（路径式定位符）寻址。
- **span patch 引擎**：所有写操作以"字节级 patch"形式落到原 XML 上，最小化变更区段；未触及的子树字节级保留——这是组件能"可控保真"的核心机制。
- **未知内容保护**（V2.6 §4.3）：当 patch 与未知节点冲突时返回诊断（`Diagnostic` / `Severity`），而不是静默丢弃。

### 3.5 事务与变更集（ADR-013）

修改操作遵循统一事务模型，杜绝"两个可独立修改的文档真相"（V2.6 §3）：

```go
// 高层公共 API 内部走以下流程
staged := pres.stagePatch(patch)        // 收集变更到 ChangeSet
err    := pres.commit(staged, revision) // 应用变更并更新 Revision
```

- **ChangeSet**：聚合所有"尚未提交"的修改
- **Revision**：每次成功提交单调递增；任何旧 Revision 持有的句柄后续再写会触发 `ErrConcurrentModification`
- **stagePatch / commit 分层**：所有 `replace.go` / `clone.go` / `bind.go` / `presentation.go` 都遵循同一契约

### 3.6 句柄身份体系（STALE-GUARD）

任何"句柄式"对象（Shape / TextFrame / Paragraph / TextRun / Cell）都需要回答"如何安全失效"。go-pptx 的答案是不维护独立的句柄 ID，而是**复用 OOXML 已经存在的稳定身份**：

| 句柄层 | 稳定身份 |
|---|---|
| `Shape` | `cNvPr@id`（DrawingML 规范要求 spTree 内全局唯一） |
| `TextFrame` / `Paragraph` / `TextRun` | shapeHint = 所属形状 cNvPr@id |
| `Cell`（表格） | shapeHint = 所属表格 cNvPr@id |

**语义**（M8 落地，三层闭环）：

- `MoveShape` / `AddShape`（兄弟增）/ 文本编辑 → 句柄仍有效（cNvPr@id 未变）
- `RemoveShape` → 目标 cNvPr@id 消失 → 句柄返回 `ErrStaleHandle`
- 重新定位（locate）流程：先按 NodePath 解析出目标元素，再向上遍历找最近 `p:sp` / `p:cxnSp` / `p:graphicFrame` / `p:grpSp` 的 cNvPr@id 与 hint 比对——不等/找不到即返回 `ErrStaleHandle`
- `hint=0` 走纯路径判定（向后兼容 notes 等老句柄）

### 3.7 跨平台与 WASM

CI 在每次 push 与 PR 上交叉编译以下四个目标：

| 目标 | 用途 |
|---|---|
| `js/wasm` | 浏览器原生 PPT 解析（`wasm/check` 子包），无需服务端中转，可用于隐私敏感场景 |
| `darwin/arm64` | Apple Silicon 原生 |
| `wasip1/wasm` | WASI 1.0（服务端沙箱可执行容器） |
| `linux/arm64` | ARM 服务器（AWS Graviton 等） |

`CGO_ENABLED=0` 约束保证任何目标都不需要 C 工具链；同时也意味着组件不依赖 glibc/musl 之外的任何系统库，可静态链接到容器镜像中。

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
err = pres.Save("./out.pptx")                                  // 默认拒绝覆盖
err = pres.Save("./out.pptx", pptx.WithOverwrite(true))         // 显式覆盖
err = pres.Write(io.Discard)                                   // 仅内存序列化（基准/预览）
```

完整生命周期覆盖：创建 / 打开 / 流式打开 / 修改 / 保存（原子）/ 落盘序列化 / 仅内存序列化 / 关闭。

### 4.2 形状 / 几何 / 图片

- **形状分类**：`ShapeKind` 枚举 + 8 个类型化句柄（`GroupShape` / `AutoShape` / `OpaqueShape` / `PictureShape` / `TableShape` / `ChartShape` / `AudioShape` / `VideoShape`）+ `TextShape` 别名
- **几何**：EMU（English Metric Units）单位、Point / Rect / Quad 值对象、190+ `prstGeom` 预设 + 自定义几何只读
- **效果模型**：阴影 / 发光 / 反射 / 柔边 / 3D 场景的只读报告
- **样式矩阵**：theme `fmtScheme` 引用链解析，本地 / 继承 / 布局三层格式区分
- **图片插入**：自动建 relationship、自动 content type、自动媒体去重（按内容哈希）

### 4.3 表格与图表

- **表格**：逻辑网格、单元格合并 / 拆分、单元格边框与对齐（`tcPr`）、样式分区 12 标志位优先级矩阵
- **图表（受限三类）**：柱状 / 折线 / 饼图（CHART-01，M5 落地）
- **图表扩展**：标签 / 误差线 / 趋势线 / 日期轴 / 对数轴（CHART-02，M6 落地）
- **图表 workbook**：`ChartWorkbookBuilder` / `ChartDataBook` / `DefaultWorkbookBuilder` 三个 Experimental 类型（v1.x 期间可继续扩展）

### 4.4 媒体（音频 / 视频 / 图片）

- **音频嵌入**：AUDIO-02（AUDIO+M4）—— 含配音挂接、受限 ID 分配、时长/采样率/声道探测
- **视频媒体形状**：VIDEO-01（M6）—— 复用 MEDIA/AUDIO 机制，poster frame 处理
- **媒体探测**：内置 `internal/audioprobe` / `internal/videoprobe`，不调用 FFmpeg
- **媒体去重**：按内容 SHA-256 哈希去重，避免重复嵌入

### 4.5 文本编辑（TEXT-02 跨 Run 替换）

最危险的领域——文本内容是 PPTX 中最常被修改、也最易破坏未修改内容的地方。go-pptx 在该领域做了大量投入：

- **富文本模型**：`TextFrame → Paragraph → TextRun` 三层，与 OOXML 一一对应
- **跨 Run 替换**：当目标串跨越多个 Run 边界时，自动规划最小 patch；保证前后 Run 内容字节级保留
- **替换模式**：`ReplaceMode` 枚举（`Exact` / `Regex` / `Glob`/ …）
- **双向重叠校验**：QA-01 实测修复了 `ReplaceText` 跨 Run 命中错锚产生 NUL 字节的 bug（`locateBlockSpan` 双向重叠校验）
- **plain text 替换**：`SetPlainText` 用于模板批量替换场景，绕过富文本结构

### 4.6 页面复制（CLONE-01 / CLONE-02）

- **同文档受限复制**（CLONE-01）：保留版面与母版引用，自动分配新 cNvPr@id 避免冲突
- **跨文档受限复制**（CLONE-02，M8）：`CopySlideFrom` + `clonePlan` 显式区分 srcDoc / crossDoc 模式；版面 / 母版内容按字节匹配复用，差异部分走 patch

### 4.7 模板数据绑定（TPL-01）

`bind.go` 子系统提供 Mustache 风格的占位符替换：

- `{{path}}` 字段访问（含字典/列表路径）
- `{{#if expr}} … {{/if}}` 条件块
- `{{#each list}} … {{/each}}` 迭代块

绑定器构建在保真 patch 之上（ADR-013）：未命中的占位符视为未知内容保留；命中失败返回诊断，不静默丢弃。

### 4.8 动画时序只读 IR（TIMIR-01）

- **只读 timing IR**：M7 落地的只读中间表示，完整解析 `p:timing` 子树、`seq` / `par` / `excl`、动作路径
- **最小时间求值器**：检测并返回 `ErrTimingConflict` / `ErrDurationUnknown`，不承诺完整播放行为计算
- **过渡动画**：R 全集识别 + E 基础过渡的受控编辑（ANIM-02，M6）

### 4.9 语义 diff（DIFF-01，M8 官方承诺）

`ir.Diff` + `pptx diff` CLI：

- **页面对齐**：SlideID 强匹配 + 形状 ID Jaccard 相似度（加权 LCS）
- **页内配对**：基于 ShapeID
- **opaque diff**：返回 `Part` + `NodePath` 回溯，定位到具体 XML 节点
- **格式**：`go-pptx.diff/1.0`，独立格式可被第三方消费

### 4.10 自动讲解与播放（AUDIO-02）

与配套的"PPT 自动讲解工具"产品集成（M8 + AUTO-PPT）：

- 富文本备注 → 配音文本源
- 音频嵌入 + 受限 ID 分配 + 时长探测 → 页面时长自动填入
- 同步操作的诊断：缺音频、配音冲突、时长冲突均返回结构化错误

### 4.11 渲染契约（RENDER-01）

- **接口落地**：M8 落地的 `Renderer` / `RenderOptions` / `RenderCapabilities` / `RenderedSlide` / `RenderAll` 契约（`render` 子包）
- **实现后续立项**：渲染实现按 ADR-014 不进入核心里程碑，V1 不承诺；接口已可在 wasm 编译通过
- **现状披露**：`rendering.native` 能力状态在 Capability Manifest 中如实保留为 `Untested`

---

## 5. API 稳定性分级体系

go-pptx 公开承诺**三级稳定性**（[ADR-015](adr/ADR-015-api-stability-tiers.md)），godoc 段落标签为唯一标记：

| 标签 | godoc 标记 | 兼容性承诺 |
|---|---|---|
| **Stable** | `// Stable: <reason>` 紧贴 type/func 上方 | v1.0 后承诺向后兼容——只增不破；行为/语义可加严但不可放宽；允许弃用但必须保留替代入口至少两个小版本 |
| **API（默认）** | 无标签 | v1.0 锁定前可调整字段；锁定承诺由 ADR-015 §"v1.0 锁定流程"规定 |
| **Experimental** | `// Experimental: <reason>` 紧贴 type/func 上方 | 1.0 内可能改——可新增方法/字段、调整不兼容语义；事件必须发公告，不静默改 |
| **Deprecated** | Go 1.19+ 标准 `// Deprecated: <替代>` 标签 | v1.0 锁定前通常不引入；保留至少两个小版本 |

**错误码 / 错误类型默认 Stable**——错误码是 API 兼容契约的硬约束（17 个 `Err*` 哨兵 + `OperationError`）。

### 5.1 v1.0 冻结清单（2026-09-10 → 09-11 收口）

完整的逐项评审记录见 [`docs/v1.0-freeze-list.md`](v1.0-freeze-list.md)；最终结果：

| 阶段 | 时间 | 增量 | 累计 Stable |
|---|---|---|---|
| 评审开始日 | 2026-09-10 | 核心入口 3（Presentation/Slide/Shape） | 3 |
| T-2 周首批 | 2026-09-10 | 错误码 17 集合段 + OperationError 1 + 诊断契约 4 + 几何值对象 4 + 枚举 2 = +28 | 31 |
| T-1 周收敛 | 2026-09-10 | Capability Output 4 + 句柄 ID 类型 2 = +6 | 37 |
| T-3 日补齐 | 2026-09-10 | `CapabilityFeature` / `CapabilityManifestSource` 段落补齐（f188c69 漏段修复） | 39 |
| T+0 文本入口 | 2026-09-10 | `TextFrame` / `Paragraph` / `TextRun` = +3 | 42 |
| T+0 末窗口形状 | 2026-09-10 | `ShapeKind` + 8 Shape 句柄 + `TextShape` 别名 = +9 | 51 |

**v1.0 锁定的 51 个 Stable 段落**包含：
- 核心入口 6：`Presentation` / `Slide` / `Shape` / `TextFrame` / `Paragraph` / `TextRun`
- 错误码集合段 1（聚合 17 哨兵 + `OperationError`）
- 诊断契约 4：`Diagnostic` / `Severity` / `ValidationReport` / `CapabilityStatus`
- 几何值对象 4：`EMU` / `Point` / `Rect` / `Quad`
- 枚举 3：`ReplaceMode` / `MultiCellTextPolicy` / `ShapeKind`
- Capability Output 4：`CapabilityManifest` / `CapabilityManifestSource` / `CapabilityDimension` / `CapabilityFeature`（JSON tag 集锁死）
- 句柄 ID 类型 2：`SlideID` / `ShapeID`（`uint32` + 文档语义）
- Shape 句柄 8：`GroupShape` / `AutoShape` / `OpaqueShape` / `PictureShape` / `TableShape` / `ChartShape` / `AudioShape` / `VideoShape`
- 类型别名 1：`TextShape` ≡ `AutoShape`

### 5.2 升降档规则（ADR-015 §"决策"）

- **升档**：必须证明公共面契约收敛、有失败回退路径，并在 `ADR-015.md#changelog` 登记
- **降档**：视为破坏性变更，必须走 ADR 评审，在跟踪文档顶部"破坏性变更公告"段登记，至少两个小版本过渡期并存替代入口
- **新增 Stable**：仅限"通用对象模型入口"或"错误/诊断契约"；加性工作一律默认 API 级，不在 PR 中讨论升 Stable

---

## 6. 质量保障与工程纪律

### 6.1 测试矩阵

CI（`.github/workflows/ci.yml`）在每次 push 与 PR 上执行：

| Job | 内容 |
|---|---|
| `lint` | `gofmt -l .` + `go vet`（默认 + `corpus` build tag） |
| 默认测试 | `go test ./...`（9 包） |
| `corpus-replay` | `go test -tags=corpus ./... -v`，公开金样 replay 自动回归 |
| 交叉构建 | `js/wasm` / `darwin/arm64` / `wasip1/wasm` / `linux/arm64` 四目标 |

新增的 `perf-smoke` job 跑确定性守门（无 ns/op 阈值断言）：

1. 基准套件可编译，6 组 × 3 档语料全部实际执行（防基准被静默跳过）
2. `scripts/perf/summarize` 能解析当前 Go 版本输出（防版本变更导致报告生成器静默失效）
3. 三档语料 `pkg-bytes` 高于预期下界（守住"大媒体被媒体哈希去重合并"这类静默回归——PERF-01 首版真实发生过）

每日定时 `baseline` job（`.github/workflows/perf.yml`）在固定 runner 上跑 `COUNT=10` 全量基线，原始日志与报告作为 artifact 归档 90 天。

### 6.2 真实语料回归（CORPUS-01）

- **公开样本（CI 可复现）**：3 个 LibreOffice 生成金样（`s001-text` / `s002-table` / `s003-image`），由 `//go:build corpus` build tag 隔离，公开分发许可完整
- **私有样本（索引但不分发）**：33 个 `ext-*`（含 WPS 源、限制许可样本）作为索引存在，不纳入开源分发
- **金样验证契约**：以 `s001/s002/s003` replay 后字节级与原始一致作为门禁；任何改动引入的新差异区间必须由回归测试锁定
- **垂直验证**：ext-0024 单 Run 替换差异区间收敛到 1 字节（实施计划 §123 退出标准落成自动化）

### 6.3 性能基线（PERF-01）

**三档语料**（`docs/PERF-01-性能基线.md`）：

| 档位 | 页数 | 内容 | 用途 |
|---|---|---|---|
| `10p-text` | 10 | 文本框（5 段，首段含唯一标记 `ALPHA`） | 纯文本基线 |
| `50p-image` | 50 | 文本框 + 一张 64×64 小图 | 图文基线 |
| `100p-media` | 100 | 文本框 + 20 张 768×768 互不相同噪声大图 | 含大媒体基线 |

**六类基准操作**：`Open` / `Traverse` / `Replace` / `SaveMem`（内存序列化）/ `SaveDisk`（落盘）/ `PeakHeap`（峰值内存）。

**代表性数据**（Intel Core Ultra 9 275HX / Go 1.27.0 / windows-amd64 / COUNT=10，详见 [`PERF-01-benchmark-report.md`](PERF-01-benchmark-report.md)）：

| 操作 | 10p-text (14 KiB) | 50p-image (60 KiB) | 100p-media (33.9 MiB) |
|---|---|---|---|
| `Open` p50 | 1.10 ms | 3.93 ms | 3.12 ms |
| `Traverse` p50 | 875 µs | 9.41 ms | 31.33 ms |
| `Replace`（单处）p50 | 40.8 µs | 47.7 µs | 59.2 µs |
| `SaveMem` p50 | 2.97 ms | 11.11 ms | 100.19 ms |
| `SaveDisk` p50 | 5.75 ms | 14.93 ms | 155.18 ms |
| `PeakHeap` | 1.93 MiB | 10.41 MiB | 59.67 MiB |

> **披露纪律**：性能数据与硬件/工具链强相关；跨机器比较须先固定平台与 Go 版本；不写"比 Python 快多少倍"（V2.6 §15.3：在没有基线前不写）。

### 6.4 Fuzz 与安全

- **fuzz 八目标**：打开链路五目标（`opc.Load` / `opc.Scan` / `xmlstore.Scanner` / `xmlstore.Index` / `OpenReader`）+ 文本编辑两目标（`ReplaceText` / `SetPlainText`）+ 模板绑定一目标（`FuzzBind`）
- **AT-14 修复**：恶意包 panic 修复，三入口（`Open` / `OpenReader` / `New`）显式错误返回，删除 `mustMainPart`
- **资源预算**：OPC ZIP 解析时设置中央目录大小上限、Part 数量上限；超出返回 `ErrLimitExceeded` / `ErrMalformedPackage` 而非 panic

### 6.5 工程纪律（根包可维护性）

- Go 反模式扫描：`fmt.Sprintf` 拼 XML 仅 2 处（已知 schema + 整型）；裸 XML 常量 0 处；唯一大函数 `populateCapabilityFeatures`（220 行）纯声明式数据表
- 错误用 error 不 panic；文档/状态文件用中文；commit message 关联 WP 编号；相关 md 置于 `docs/`
- 仓库级 git 身份：go-pptx-dev <go-pptx-dev@local>（仅仓库内，不外发）

---

## 7. 工具链与命令行

`cmd/pptx` 子模块提供 6 个 CLI 子命令：

| 子命令 | 功能 | 适用场景 |
|---|---|---|
| `pptx capability` | 输出六维能力 manifest（`Inspect` / `Create` / `Edit` / `Preserve` / `Render` / `Play`） | 环境审计 / 部署前能力确认 |
| `pptx inspect` | 只读报告（几何 / 填充 / 效果 / 样式矩阵 / 版面信息） | 内容审计 / 调试 |
| `pptx diff <a> <b>` | 两个文档的语义 diff（`go-pptx.diff/1.0` 格式） | 回归审计 / 文档比对 |
| `pptx bind <tpl> <data> <out>` | 模板数据绑定（`{{path}}` / `{{#if}}` / `{{#each}}`） | 批量报告生成 |
| `pptx validate <file>` | 诊断报告（`ValidateOption` 控制级别） | 发布前自检 |
| `pptx check`（WASM） | 浏览器原生能力检查 / inspect / validate | 零信任隐私场景 |

### 7.1 Capability Manifest

v1.0 capability 六维状态：

| 维度 | 状态 | 说明 |
|---|---|---|
| Inspect | Supported | 完整只读检查能力 |
| Create | Partial | 受支持特性可创建 |
| Edit | Partial | TEXT-02 跨 Run + 表格 + 媒体 + clone + bind |
| Preserve | Supported | 字节级保真（垂直验证 ext-0024 单 Run 差异收敛到 1 字节） |
| Render | Untested | 接口已落地（`render` 子包），实现未立项 |
| Play | Partial | TIMIR-01 计时只读 + AUDIO-02 配音挂接 |

### 7.2 WASM 浏览器侧工具（`wasm/check`）

- `GOOS=js GOARCH=wasm` 编译为单文件 wasm
- 提供 `check` / `inspect` / `validate` 三类浏览器侧只读操作
- 适用场景：上传前的客户端预检、不愿将敏感 PPT 上传到服务器的审计场景

---

## 8. 产品特色与差异化优势

### 8.1 与主流同类方案的对比定位

| 维度 | go-pptx | python-pptx | 其他 Go 库 |
|---|---|---|---|
| 语言 | **Go** | Python | Go |
| CGO | **CGO=0** | — | 大多 cgo 依赖 |
| 跨平台 | **4 平台 CI** | 受 Python 解释器限制 | 多数仅 linux/darwin |
| 字节级保真 | **是**（xmlstore span patch） | 部分 | 否 |
| 受控编辑失败 | **显式错误**（Diagnostic/Severity） | 部分抛异常 | 多为静默损坏 |
| API 稳定性分级 | **三级 + godoc 标记**（ADR-015） | 无 | 无 |
| 真实语料回归 | **36 样本索引 + 3 公开金样 CI** | 无强制 | 无 |
| Fuzz 内建 | **8 目标** | 无 | 极少数 |
| 性能基线 | **三档 × 六类** 自动报告 | 无 | 无 |
| 语义 diff | **DIFF-01**（加权 LCS + Jaccard） | 无 | 无 |
| WASM 浏览器侧 | **原生支持** | 不适用 | 极少数 |
| 公开承诺文档 | **5 篇 ADR + 冻结清单 + 实施跟踪 + 白皮书** | 无 | 无 |

### 8.2 五大差异化优势

1. **服务端原生 + 跨平台**：唯一在 `js/wasm` / `darwin/arm64` / `wasip1/wasm` / `linux/arm64` 四平台有 CI 验证的 PPTX 库；可作为任何 Go 服务的静态依赖，不引入解释器或 C 工具链
2. **字节级保真的受控编辑**：xmlstore span patch 引擎保证未触及子树字节级保留；同时拒绝不安全操作而非静默损坏——这是"工程可靠性"的核心
3. **三级 API 稳定性承诺**：ADR-015 godoc 段落标记 + 升降档规则 + v1.0 冻结清单 + 实施跟踪公开通告——下游可基于白纸黑字的承诺做升级决策
4. **真实语料回归 + 性能护栏 + fuzz 三位一体**：36 样本索引、3 公开金样 CI 自动 replay、三档性能基线每日定时归档、8 fuzz 目标——把"能工作"和"能持续工作"分开度量
5. **从编辑器到浏览器的完整链路**：服务端编辑 → 语义 diff → 模板批量绑定 → 浏览器原生只读审计，所有环节在同一代码库、同一真实语料回归体系下闭环

---

## 9. 技术创新点

### 9.1 STALE-GUARD：句柄身份的三层闭环（业界首创）

**问题**：任何"句柄式 API"都要回答"句柄如何安全失效"。业界常用方案是单独维护一份 ID 表，导致两套真相、易出 bug。

**go-pptx 的方案**：

- 所有句柄身份 = 所属形状的 cNvPr@id（OOXML 已要求 spTree 内全局唯一）
- path 仅作"在哪个 spTree / grpSp 下查找"的父容器提示
- locate 流程：先按 path 解析出目标元素，再向上遍历找最近 `p:sp` / `p:cxnSp` / `p:graphicFrame` / `p:grpSp` 的 cNvPr@id 与 hint 比对——不等/找不到返回 `ErrStaleHandle`
- `hint=0` 走纯路径判定（向后兼容 notes/老句柄）

**创新点**：复用 OOXML 已经存在的稳定身份，把"句柄有效性"从组件自行维护的元数据降级为"对底层 XML 的可推导属性"——任何修改只要不破坏 OOXML 规范，句柄自动正确失效。

**落地范围**：三层覆盖——shapeNode（形状）/ textNode（Paragraph / TextRun / TextFrame）/ Cell（表格单元格）。

### 9.2 xmlstore：命名空间感知的最小侵入 patch 引擎

**问题**：PPTX 的 XML 充满命名空间（`p:` / `a:` / `r:` / `c:` / `mc:` / `p14:` 等），传统 XML 编辑器要么强行 schema 化（破坏未知内容）、要么全文本替换（破坏无关内容）。

**go-pptx 的方案**：

- 节点索引树：每个 Part 建成节点位置索引，外部代码通过 `NodePath` 寻址
- span patch 引擎：所有写操作以"字节级 patch"形式落到原 XML，最小化变更区段
- 命名空间受控：新增节点走受控路径，避免命名空间漂移
- 未知内容保护：patch 与未知节点冲突时返回诊断而非静默丢弃

**创新点**：用"字节级 patch + 节点索引"替代"DOM 树序列化"，在保留 XML 编辑的语义友好同时获得文本编辑的保真能力。垂直验证 ext-0024 单 Run 替换差异区间收敛到 **1 字节**。

### 9.3 DIFF-01：加权 LCS + Jaccard 双相似度页面对齐

**问题**：纯 SlideID 匹配无法应对页面增删 / 重排；纯 LCS 对小幅编辑过于敏感。

**go-pptx 的方案**：

- **页面对齐**：SlideID 强匹配 + 形状 ID Jaccard 相似度（加权 LCS）
- **页内配对**：基于 ShapeID
- **opaque diff**：返回 `Part` + `NodePath` 回溯

**创新点**：双相似度组合——先用 SlideID 锚定明确匹配，再用形状 ID Jaccard 配对"被移动 / 被复制"的页面。`go-pptx.diff/1.0` 格式独立可被第三方消费。

### 9.4 Capability Manifest：六维能力披露

**问题**：PPT 库往往只能"能 / 不能"二元表达，让集成方无法做精细能力规划。

**go-pptx 的方案**：六维度（`Inspect` / `Create` / `Edit` / `Preserve` / `Render` / `Play`）+ 每维四档（`Supported` / `Partial` / `Untested` / `Unsupported`）+ `CapabilityManifest` 输出为可消费 JSON。

**创新点**：把能力分维度 + 分档位的二维矩阵作为一等公民公开（`pptx capability` CLI），与 §2.3 OOXML 特性能力矩阵（P / R / E / F）一一映射。`Preserve=Supported` 是关键卖点。

### 9.5 三级 API 稳定性承诺与升降档规则（ADR-015）

**问题**：开源库经常"随便改"破坏下游；纯靠 SemVer 也无法表达"哪些字段承诺兼容"。

**go-pptx 的方案**：

- godoc 段落标记 `// Stable:` / `// Experimental:` 直接在源码里声明
- pkg.go.dev 自动渲染
- 升降档规则写成 ADR（升档须证明公共面契约收敛 + 有失败回退路径；降档视为破坏性变更须走 ADR 评审 + 至少两个小版本过渡）
- v1.0 冻结清单按 T-2 周启动 → T-0 tag 的窗口期分阶段评审

**创新点**：把"API 稳定性"从发布说明里的文字承诺升级为源码注释 + ADR 评审流程 + 实施跟踪公开通告的三重护栏。下游升级决策可基于 godoc 直接判定。

### 9.6 公开金样 CI 闭环（CORP-01）

**问题**：很多库"自测通过"但拿到客户文件就坏——因为测试语料太小、太干净。

**go-pptx 的方案**：

- 公开 3 个 LibreOffice 生成金样（`s001-text` / `s002-table` / `s003-image`），分发许可完整
- `//go:build corpus` build tag 隔离，CI 自动 replay
- 私有 33 个 `ext-*`（WPS 源、限制许可）作为索引存在（不入开源分发）
- `corpus.py validate` 36/0 errors 作为前置门禁
- 任何差异必须由回归测试锁定（垂直验证用例）

**创新点**：把"金样 CI 闭环"作为发布硬门槛（V2.6 §26 P1），且把语料可复现性与真实覆盖度作为两套独立维度分别披露。

### 9.7 原子保存 + 并发修订快照

**问题**：保存中途崩溃导致 PPTX 文件损坏；多协程并发修改造成内容丢失。

**go-pptx 的方案**：

- 原子保存：先写临时文件 + fsync + rename；默认拒绝覆盖（`ErrOutputExists`），调用方需显式 `WithOverwrite(true)`
- 修订快照：每次 commit 单调递增 Revision；旧 Revision 持有的句柄后续写操作触发 `ErrConcurrentModification`
- 失败时回滚到上一稳定 Revision

**创新点**：把"事务"概念显式落地到 Revision 级别，让并发场景下的数据丢失风险可观测、可恢复。

---

## 10. 典型应用场景

### 10.1 批量报告 / 培训材料生成

`pptx bind` + Go 模板：

```go
pres, _ := pptx.Open("./quarterly-report-template.pptx")
data := loadQ4Financials()
out   := bytes.Buffer{}
pres.Bind(data)               // 应用 {{path}} / {{#each regions}} / {{#if netPositive}}
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

- 富文本备注 → TTS 文本源
- 音频嵌入 → 页面时长自动填入
- `ErrTimingConflict` / `ErrDurationUnknown` → 结构化修复提示

适用：在线教育 / 培训自动化 / 无障碍辅助。

### 10.6 模板克隆 + 个性化（CLONE-01 / CLONE-02）

```go
tpl, _ := pptx.Open("./company-template.pptx")
defer tpl.Close()

for _, client := range clients {
    pres, _ := pptx.New()
    pres.CopySlideFrom(tpl, slideIdx)         // 跨文档受限复制
    pres.Slides()[0].Bind(client.Data)        // 个性化数据
    pres.Save(fmt.Sprintf("./%s.pptx", client.Name), pptx.WithOverwrite(true))
}
```

适用：客户提案 / 营销材料个性化 / 报告分发。

---

## 11. 已知边界与 1.x 路线图

### 11.1 v1.0.0 已披露的边界

| 项 | 状态 | 详情 |
|---|---|---|
| **L3 客户端矩阵**（PowerPoint / WPS 真机打开无修复提示 + 编辑后重存） | ❌ 未验证 | 本机无真机环境；外部依赖；V2.6 §26 P1 标注为发布级缺口，但 ADR-015 §4 未作为硬门槛 |
| **覆盖率** | ⚠️ 86.6% / 77.1% | 低于 V2.6 §15.3 目标 90%；非硬门槛；cmd/pptx 与根包错误路径 / 边界用例可继续补 |
| **`rendering.native`** 能力状态 | `Untested` | `render` 子包接口已落地，实现后续立项（ADR-014） |
| **私有 `ext-*` 样本** | 不分发 | 许可 / 体积限制；CI 用 3 个公开金样兜底 |
| **加密 / 签名 / OLE** | 识别 + 拒绝编辑 | 按 V2.6 §2.2 边界表策略：识别格式、显式返回不支持，不静默转换 |
| **动画播放求值** | `ErrTimingConflict` 检测 | 最小时间求值器；不承诺完整播放行为计算 |

### 11.2 1.x 路线图（按 v1.0 freeze list §D + ADR-016）

- **1.0.x**：L3 客户端矩阵补齐（一旦有真机环境）、覆盖率向 90% 收敛、bug 修复
- **1.1.x**：ADR-016 渐进式 internal 实现层抽取—— `internal/document` / `internal/textmap` / `internal/editplan` 等纯实现层逐步从根包抽出，公共 API 零变动
- **1.2.x**：实验类型收敛（`ChartWorkbookBuilder` / `ChartDataBook` / `DefaultWorkbookBuilder` / `CustomPropertyKind` / `CustomPropertyValue` 视使用情况决定升档或弃用）
- **2.0**：原生渲染实现（如启动）；遵循 ADR-014 不进入核心里程碑原则，需独立立项

### 11.3 演进原则

- **不破坏 Stable**：v1.x 期间 51 个 Stable 段落承诺向后兼容
- **API 默认可加**：102 个 API 默认类型允许追加新字段与方法；不允许删除或重命名
- **Experimental 可改**：5 个 Experimental 类型可继续演化，必须发公告不静默
- **架构压力按触发条件拆**（ADR-014）：合并冲突热点 / 第三方扩展需求 / 编译时间瓶颈任一触发才启动 `internal/edit` 等深层抽取

---

## 12. 版本、许可与社区

### 12.1 版本与签名

- **当前版本**：v1.0.0（2026-09-11 发布）
- **Git tag**：`v1.0.0`（SSH 签名，绑定 GitHub 账号 `jinfeng105` 的 ed25519 公钥）
- **commit 基线**：`44b9ba7` → 收口 commit `5d61f97`
- **CI**：每次 push / PR 跑默认 9 包测试 + corpus-replay + 4 交叉构建 + lint

### 12.2 许可证

- **代码**：Apache-2.0（与远程仓库 LICENSE 字节一致）
- **公开金样**（`s001-text` / `s002-table` / `s003-image`）：CC0 / Public Domain（LibreOffice 生成）
- **私有 `ext-*` 样本**：仅作索引，不纳入开源分发（许可 / 体积原因）

### 12.3 仓库与社区资源

| 资源 | 路径 |
|---|---|
| 仓库 | [`github.com/F31/go-pptx`](https://github.com/F31/go-pptx) |
| 模块路径 | `github.com/F31/go-pptx` |
| 设计基线 | `docs/go-pptx_完整设计方案_V2_6_开发实施版.md` |
| 实施计划 | `docs/go-pptx-项目实施计划.md` |
| 实施跟踪 | `docs/go-pptx-实施状态跟踪.md` |
| v1.0 冻结清单 | `docs/v1.0-freeze-list.md` |
| 发布通告 | `docs/RELEASE-NOTES-v1.0.0.md` |
| 变更日志 | `CHANGELOG.md` |
| API 稳定性分级 | `docs/adr/ADR-015-api-stability-tiers.md` |
| 内部包拆分策略 | `docs/adr/ADR-014-root-internal-package-strategy.md` |
| 渐进式 internal 抽取 | `docs/adr/ADR-016-progressive-internal-extraction.md` |
| 架构基线 | `docs/architecture-current.md` |
| 性能基线方法论 | `docs/PERF-01-性能基线.md` |
| 性能基线报告 | `docs/PERF-01-benchmark-report.md`（自动生成） |
| M8 里程碑总结 | `docs/M8-里程碑总结.md` |
| 语料入库指南 | `docs/corpus-入库指南.md` |

### 12.4 反馈与升级指引

- **升级到 1.0**：51 个 Stable 段落承诺向后兼容——升级不会破坏现有调用
- **Experimental 用户**：留意公告；如有破坏性变更，至少两个小版本过渡
- **API 默认用户**：字段追加无需关注；删除 / 重命名不会发生
- **问题反馈**：通过 GitHub Issues（首选）；附最小复现 + 原始 PPTX 哈希（不含文件本体）

---

> **白皮书维护约定**：与 [`CHANGELOG.md`](../CHANGELOG.md) 同步；每个小版本（1.x.0）发布时由负责人更新"版本 / 路线图 / 创新点落地"段；纯文档工作纳入 `.commit-msg-*.txt` 工作流（详见仓库 `.gitignore`）。