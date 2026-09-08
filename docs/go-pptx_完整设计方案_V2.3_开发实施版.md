# go-pptx 组件完整设计方案 V2.3

> 文档类型：开发实施基线｜日期：2026-09-07
> 核心语言：Go｜核心约束：纯 Go、CGO_ENABLED=0、无外部运行时强依赖
> 适用范围：通用 PPTX 创建与编辑组件，以及自动讲解产品的 PPT 解析、备注、配音和播放计时模块。
> 本文定义目标架构、开发契约与验收要求，不表示相关功能已经实现或完成兼容测试。

**阅读导航：** 1–5 章定义范围、架构与核心接口；6–14 章定义文档能力；15–17 章定义验收与阶段；18–23 章提供实现细节和扩展接口；24–28 章可直接用于拆分任务、编写测试与集成开发；29 章列参考依据。

## 1. 项目定位与总体目标

go-pptx 是面向服务端和工具开发者的 Go 演示文稿组件，采用 Presentation → Slide → Shape → TextFrame → Paragraph → Run 对象模型，提供类似 python-pptx 的使用体验。组件同时满足模板报告生成、已有文档精细编辑，以及带配音 PPT 的合成需求。

项目以六项能力构成产品主线：

1. 通用 PPTX 读写：创建、解析、保存、页面管理、形状、图片、文本、备注、表格与受支持图表。
2. 可控保真编辑：保留未修改 Part 和可保护的未知 XML 内容，拒绝不能安全执行的操作。
3. 精细格式处理：区分本地格式、继承格式与布局结果，支持跨 Run 编辑、组合变换及表格样式。
4. 自动讲解支持：富文本备注、音频嵌入、播放配置、翻页计时及带诊断的同步操作。
5. 工程可靠性：惰性解析、资源预算、事务提交、校验报告、真实文档回归和原生 fuzz。
6. 可选扩展：只读 IR、CLI、媒体探测及渲染适配，保持核心依赖简单。

### 1.1 部署与职责

核心 SDK 可构建为 Go 应用中的静态依赖，不调用 Python、Office 或 FFmpeg。TTS、视频编码、外部排版与渲染由业务服务或可选适配器负责；使用这些能力的完整产品可能依赖外部进程，不能据此宣称整条视频链路零依赖。

### 1.2 开发约束

- 公共入口为 module 根包 `pptx`，正式 module path 在建仓时确定；文档中的示例占位域名不用于生产发布。
- 所有修改通过受控 API 完成；低层存储以 internal 包封装。
- 普通错误以 `error` 返回，不使用 panic；不通过返回 nil 掩盖未支持能力。
- 接口便捷性通过 Options、Spec 和高层组合方法实现，不使用吞掉错误的链式 Setter。
- 保真、兼容、渲染和播放分别声明能力范围，发布声明必须由测试证据支持。
- MVP 优先形成“模板打开→读取讲稿→更新文本/备注→保存”和“音频嵌入→计时→放映”两个闭环。

## 2. 产品范围与边界

### 2.1 核心使用场景

1. 根据模板批量生成报告、方案和培训材料。
2. 提取页面、文本、备注、媒体及结构信息，供搜索或 AI 应用使用。
3. 修改文字、图片、备注和常用格式，同时保留未操作内容。
4. 创建可编辑文本框、图片、基础形状、表格和受支持图表。
5. 按受支持的客户端配置嵌入配音、设置播放行为与翻页时间。

### 2.2 文件和运行时边界

| 对象 | 首版策略 |
|---|---|
| `.pptx` Transitional | 首要读写目标；兼容范围由测试矩阵限定 |
| `.pptx` Strict | 识别格式，首版显式拒绝语义编辑；后续独立适配，不静默转换命名空间 |
| `.potx` | 后续模板入口；转换输出需同步主 Part Content Type 等信息 |
| `.pptm` / `.potm` | 首版识别并拒绝写入；未来单独加入宏保留路径，不伪装成 pptx |
| 二进制 `.ppt` | 不直接支持；格式转换交给外部工具 |
| 加密 Office 文件 | 明确返回不支持加密，不当作损坏 ZIP |
| 带包签名的文件 | 首版禁止语义修改后保存；原文件只读导出不受影响 |
| 外链、OLE、嵌入字体 | 检测并按保留规则处理，不自动访问或执行 |
| 核心运行时 | 纯 Go、可跨平台；不依赖 Python、Office、CGO 或在线服务 |
| 预览、PDF、视频 | 可选适配器，不作为核心读写的强依赖 |

首版目标是可交付的 python-pptx 类组件，不承诺全部 OOXML 特性、像素级复刻 PowerPoint 或完整动画编辑。各项特性相对完整 OOXML 能力的目标档位与阶段见 2.3 能力矩阵。

### 2.3 OOXML 特性能力矩阵

本矩阵声明各特性相对完整 OOXML 能力的**目标档位**，不表达已验证状态；发布声明仍以第 15 章测试证据为准。档位调整属于能力范围变更，须按 ADR 流程记录。

| 档位 | 含义 |
|---|---|
| P 透传保留 | 字节级保留，不识别语义，不提供读取视图（对应 B1） |
| R 只读识别 | 解析并输出语义与诊断，不提供写入 API |
| E 受控编辑 | 在声明的补丁保证范围内读写；未覆盖部分显式失败（对应 S1） |
| F 完整支持 | 全量读写并纳入金样验收；视觉/播放类另需 V1/P1 证据 |

| 能力组 | 特性 | 关键 XML 结构 | 目标档位 | 目标阶段 |
|---|---|---|---|---|
| 演示文稿级 | 章节、自定义放映 | `p14:sectionLst`、`p:customShowLst` | R | M6 |
| 演示文稿级 | 嵌入字体 | `p:embeddedFontLst`、`a:embeddedFont` | R；写入后续立项 | M6 |
| 演示文稿级 | 讲义母版、视图属性 | `p:handoutMaster`、viewProps | R | M6 |
| 演示文稿级 | 避头尾规则 | `p:kinsoku` | R | M6 |
| 演示文稿级 | 视频媒体形状 | `p:videoFile`、`p14:media`、poster frame | E（复用 MEDIA/AUDIO 机制） | M6 |
| 演示文稿级 | OLE / ActiveX 对象 | `p:oleObj` | P | 维持边界表策略 |
| 形状级 | 预设几何全集 | `a:prstGeom`（190+ preset 及 adjust 公式） | R 全集；E 常用子集 | R 全集 M6；E 子集 M2–M3 |
| 形状级 | 自定义几何 | `a:custGeom`（路径、guide 求值） | R | M6 |
| 形状级 | 效果模型 | `a:effectLst`（阴影/发光/反射/柔边）、`a:scene3d`/`a:sp3d` | R | M6；效果渲染随原生渲染立项 |
| 形状级 | 渐变与图案填充 | `a:gsLst`（tileFlip、path 渐变）、`a:pattFill` | R；E 常用渐变 | M6；E 后续 |
| 形状级 | 线条系统 | `a:ln`（箭头、dash、join） | E | M3 |
| 形状级 | 文字效果 | `a:prstTxWarp`、文本描边/阴影 | P | 后续立项 |
| 文本级 | 段落属性全集 | `a:lnSpc`(spcPts)、`buChar`/`buAutoNum`/`buBlip`、`a:tabLst` | E | M3 |
| 文本级 | Run 高级属性 | `baseline`、`spc`、`highlight`、caps、`sym` | E | M3 |
| 文本级 | 文本框高级项 | `bodyPr` 的 `numCol`、`vert`、`anchorCtr` | R；E 子集 | M6 |
| 文本级 | 字段全集 | `a:fld`（slidenum/datetime 各格式） | R（M2 已含基础）；E 逐类型 | M6 |
| 表格 | 单元格边框与对齐 | `tcPr` 内边距、anchor、对角线边框 | E | M3 |
| 表格 | 样式分区标志位 | band/first/last 共 12 个标志的优先级矩阵 | E | M3 |
| 图表 | 受限三类（柱/折/饼） | `c:` 命名空间 | E | M5（CHART-01） |
| 图表 | 其他经典图表 | 面积/散点/雷达/气泡/组合/双轴 | E 逐类立项，每类独立验收 | M6 之后 |
| 图表 | 标签、误差线、趋势线、轴扩展 | `c:dLbls`、`c:errBars`、`c:trendline`、日期轴/对数轴 | R | M6 |
| 图表 | 现代扩展图表与 3D 视图 | chartEx（c15/c16）、`c:view3D` | P | 长期透传 |
| 主题 | 样式矩阵 | `a:fmtScheme` 及 `styleMatrixReference` 引用链 | R | M3 |
| 主题 | 颜色变换全集 | `lumMod`/`lumOff`/`shade`/`tint`/`satMod` 等约 20 种变换 | R 全集（像素级一致随渲染立项） | M3 起 |
| 渲染 | 字体栈、shaping、断行、行距实算、Autofit 收敛、SmartArt 布局 | — | 原生渲染整体后续立项（见 23.1），不进入核心里程碑，不得用于 V1 声明 | 后续立项 |
| 动画 | timing 树保留 | `p:timing` 全子树 | P（M0 起纳入保留验证） | M0 |
| 动画 | 配音挂接与受限 ID 分配 | 受限 AudioProfile 计时结构 | E（仅声明 Profile） | M4（AUDIO-02） |
| 动画 | 过渡动画 | `p:transition`（含 `p14:morph`） | R；E 基础过渡 | M6；E 后续 |
| 动画 | 动画对象模型与行为库 | `seq`/`par`/`excl`、`cTn`、`p:anim*`、`p:bldP`、动作路径 | R 后续；E 完整编辑 | 后续立项 |
| 动画 | 扩展动画分支 | `mc:AlternateContent` Choice 分支内动画 | P；Choice 读取规则随 4.3 定义 | M0 起 |
| 动画 | 最小时间求值器 | — | `ErrTimingConflict` 检测规则随本项定义 | 后续立项 |

优先级说明：**过渡动画（p:transition）、视频形状、嵌入字体、颜色变换全集**四项在高频真实模板中出现率最高，且可复用 xmlstore 补丁机制、不动现有架构，是 M6 扩展阶段的首选补充项。其余 R 档特性的作用是缩小"未知内容"面积、提升诊断质量，按语料统计驱动排期；任何 E/F 档承诺必须先有对应金样与客户端验证（第 15 章）。

## 3. 总体架构与模块职责

采用单 Go module 的分层库。扩展优先使用显式接口与构造参数，不采用动态插件扫描、全局注册器或 Go plugin ABI。只有出现稳定的独立依赖边界后才拆 module。

| 层 | 内容 | 依赖规则 |
|---|---|---|
| 公共对象层 | Presentation、Slide、Shape、TextFrame、Paragraph、Run、Table、Notes | 使用内部服务，不暴露可绕过校验的原始存储 |
| 语义服务层 | 样式解析、文本编辑、媒体、图表、复制、几何 | 通过变更集修改 XML 与包关系 |
| 文档存储层 | 原始 XML、节点索引、补丁、脏状态、未知扩展 | 作为唯一权威状态；对象层只保留句柄和缓存 |
| OPC 层 | ZIP 条目、Part URI、Content Types、关系图、流式媒体 | 不依赖 PresentationML 业务对象 |
| 保存与诊断层 | 保存计划、校验、保真报告、错误定位 | 检查跨层不变量，管理提交 |
| 可选适配层 | 渲染、字体查询、CLI、外部验证 | 依赖公共接口；核心不反向依赖 |

建议源码目录：

```text
go-pptx/
  presentation.go, slide.go, shape.go, text.go
  table.go, chart.go, media.go, notes.go
  options.go, diagnostics.go, save.go
  internal/opc/       # Part、关系图、ZIP、Content Types
  internal/xmlstore/  # 原始字节、token/节点跨度、命名空间环境
  internal/edit/      # 变更集、冲突检测、事务提交
  internal/style/     # 属性继承、颜色变换、表格样式
  internal/textmap/   # 文本逻辑位置与 XML 节点映射
  internal/geom/      # 单位、矩阵、边界计算
  internal/validate/  # 结构与语义规则
  render/             # 渲染接口，适配实现按需拆分
  cmd/pptx/           # inspect、validate、replace 等便捷入口
  testdata/corpus/    # 生成器与特性双标签索引
  docs/               # API 契约、兼容矩阵、ADR
```

**关键约束：不维护两个可独立修改的文档真相。** 强类型模型从 xmlstore 投影，Setter 形成补丁或已知节点变更。缓存通过文档 revision 和依赖 revision 失效，禁止保存时用陈旧对象全量重建 XML。

## 4. OPC 与保真保存机制

### 4.1 包模型

PPTX 是由 Part 与关系组织的包，不应靠固定文件名猜测逻辑结构。[S1][S3]

- 从包级 officeDocument 关系查找主 Part；沿关系获取页面、版式、母版及主题，不能硬编码 `theme1.xml`。
- PartName 与磁盘路径分离；内部关系目标按源 Part URI 解析，允许合法的 `../media/...`，拒绝解析后越出包根的路径。
- `rId` 在各源 Part 的关系集合内唯一；页面 ID、形状 ID、动画节点 ID 各有自己的作用域和引用规则。
- Content Type 先查对应 Part 的 Override，再查扩展名 Default；不能只检查扩展名是否出现。
- 外部关系保留 Target 与 TargetMode，但不自动下载。标准关系允许循环，如版式与母版间关系；遍历使用 visited，不能假设包图为 DAG。
- 删除后默认保留无法证明可安全删除的孤立 Part，清理作为显式操作。共享媒体不能因一处删除被误删。

### 4.2 三类写入路径

| 对象状态 | 写入行为 | 保证范围 |
|---|---|---|
| 未修改 Part | 复制原始压缩条目或复用原始内容 | Part 解压后内容字节一致 |
| 已修改、支持精确补丁的 XML | 替换被修改的属性/文本跨度，保留其余原始字节 | 补丁以外的原始区域保留 |
| 新建 Part / 受控结构增删 | 用规范化序列化器生成已知结构并保留挂接的未知片段 | 语义及已声明保留能力；不承诺整个 Part 字节一致 |

Go `zip.Writer.Copy` 可以直接复制压缩条目，但会绕过解压和验证，因此不能把它当作输入完整性检查；ZIP 目录和封装信息也不因此保证完全相同。[S5]

### 4.3 已修改 XML 的未知内容保护

建立可保留词法信息的 XML 存储层：

- 保留原始字节、元素与属性跨度、命名空间作用域、未知子树及节点顺序。
- 使用 URI/local name 识别元素，同时保留前缀声明及 `mc:Ignorable`、`mc:AlternateContent` 等上下文。
- 读取视图可选择支持的 AlternateContent 分支；不能为了读取而删除其他分支。编辑需要同步备用分支时，必须实现同步或拒绝该操作。
- 不使用正则表达式定位 XML，不把整个文档 `Unmarshal → 修改结构体 → Marshal` 作为模板编辑默认路径。`encoding/xml` 可用于语义解析，但本身不是词法保真编辑器。[S6]
- 文本替换必须正确转义 XML，处理空白保留属性；补丁按原始区间降序应用，拒绝重叠冲突。
- 结构改动后重建受影响节点索引；节点移动或跨页复制必须重建有效命名空间环境，不能直接剪贴裸 XML。
- 首版若不能证明某操作能保留同节点中的未知扩展，返回 `ErrUnsupportedEdit`；允许调用方选择其他对象继续处理，不能静默删除。

词法保留是高风险基础工程，必须先做垂直验证：包含未知扩展和动画的页面，仅修改一个普通文本 Run，验证其余节点和关联 Part 不变。

### 4.4 保真等级

| 等级 | 定义 | 验收方法 |
|---|---|---|
| B0 原文件字节一致 | 原文件不做修改直接复制 | 整个文件 SHA256 一致 |
| B1 未修改 Part 字节一致 | 保存后每个未修改 Part 内容不变 | 解压后按 Part 哈希比较 |
| S1 支持对象语义一致 | 受支持对象结构和引用保持预期 | 结构断言、再次解析、语义差异报告 |
| V1 视觉相近 | 指定客户端、字体环境下渲染满足阈值 | 截图差异与人工复核 |
| P1 播放一致 | 指定客户端播放和计时达到用例要求 | 实际播放验证 |

“无修改重新打包”默认只承诺 B1；要求 B0 时走明确的原文件复制路径。V1、P1 不能由哈希或 Schema 校验推导。

## 5. 公共 API 与状态管理

以下为接口规格，代码块用于约束签名与语义；实现仓库须按本设计补齐类型、注释、测试和可编译示例。

```go
func New(opts ...NewOption) (*Presentation, error)
func Open(path string, opts ...OpenOption) (*Presentation, error)
func OpenReader(r io.ReaderAt, size int64, opts ...OpenOption) (*Presentation, error)
func (p *Presentation) Close() error
func (p *Presentation) Slides() ([]*Slide, error)
func (p *Presentation) AddSlide(layout LayoutRef) (*Slide, error)
func (p *Presentation) RemoveSlide(id SlideID) error
func (p *Presentation) MoveSlide(id SlideID, index int) error
func (p *Presentation) Save(ctx context.Context, path string, opts ...SaveOption) (SaveReport, error)
func (p *Presentation) Write(ctx context.Context, w io.Writer, opts ...SaveOption) (SaveReport, error)
func (p *Presentation) Validate(ctx context.Context, opts ...ValidateOption) ValidationReport
```

- `New()` 使用库内合法最小模板或调用方模板，模板分发需附许可信息。
- `Slides()` 返回页面句柄的新切片，不触发各页面形状解析；资源已关闭或索引不可用时返回错误。元素为受控句柄。对象内部字段不公开可变切片和 map。
- Shape、Run 等句柄在删除后失效，访问返回可识别错误；不能继续写入游离对象。
- `OpenReader` 不关闭调用方传入的 ReaderAt；`Open` 持有文件资源，由 `Close` 释放。媒体支持惰性加载及大对象落盘。
- 单个 Presentation 首版不保证并发安全，包括会填充缓存的读取；多个独立实例可并行使用。
- setter 错误不改变对象；跨 Part 操作先暂存，再一次提交。成功提交递增 revision。
- `Save(path)` 默认不允许与仍被惰性读取的源文件同一路径或指向同一文件实体。另行实现安全原位替换前，要求新输出路径。
- 文件保存先写同目录临时文件，完成 ZIP Close 和校验后再替换目标；实现并验证各平台的替换与失败恢复语义，不能先删除旧文件。提交前失败保留旧目标。
- `Write(w)` 无法回滚已写字节，应先完成保存计划和校验；I/O 失败时明确报告输出可能不完整。

### 5.1 属性与单位

```go
type Optional[T any] struct {
    Value T
    Set   bool
}
type EMU int64
type FontStyle struct {
    Bold   Optional[bool]
    Italic Optional[bool]
    Size   Optional[FontSize]
    Color  Optional[ColorSpec]
    Latin  Optional[string]
    EastAsian Optional[string]
    ComplexScript Optional[string]
}
```

`Set=false` 表示无本地覆盖；`Set=true, Value=false` 表示显式取消粗体。填充使用 `Inherit / None / Solid / Gradient / ...` 的独立类型，不能用 nil 同时代表无填充与继承。提供 `ResetFontProperty` 恢复继承。

坐标尺寸内部使用 EMU；`1 inch = 914400 EMU`，`1 pt = 12700 EMU`。字号、角度与百分比使用各自类型，集中实现与 XML 单位的转换、舍入及溢出检查，避免全部混用 float64。

## 6. 有效格式、主题与 Autofit

### 6.1 解析输入与结果

按属性族编写独立规则表，不能统一做几层结构体覆盖。输入至少包含：Run 显式属性、段落默认字符属性、列表级别、文本框样式、占位符映射、版式与母版样式、presentation 默认文本样式、主题字体与颜色映射。占位符匹配需要遵循 idx/type 等规则，不能按页面坐标或名称猜测。[S7]

```go
func (r *TextRun) ExplicitFont() FontStyle
func (r *TextRun) EffectiveFont(ctx ResolveContext) (ResolvedFont, []Diagnostic, error)
```

每个解析属性携带 `Value / Resolved / SourceTrace`。无法解析时返回明确状态；只有调用方指定回退值才能采用，并标记为 fallback。

- 主题从真实关系获取；处理 `clrMap`、`clrMapOvr`、样式引用中的占位颜色及主题覆盖上下文。
- 颜色保留原始 ColorSpec，同时输出可解析的 RGBA；有序应用已支持变换，保留未知变换并返回未完全解析的诊断。
- 字体按 Latin、East Asian、Complex Script 与语言脚本解析。主题中的字体名称不保证本机安装，字形替代属于布局器职责。
- 缓存 key 包含文档 revision、节点、段落级别、语言及上下文；主题或母版更新必须使相关缓存失效。

### 6.2 Autofit 分层

保留 `noAutofit / normAutofit / spAutoFit` 三种状态。`EffectiveFont` 表示样式层的有效字号；额外提供 `StoredAutofit()` 读取文件保存的缩放提示，`LayoutResult` 才描述某个布局引擎计算的文字尺寸。

可以提供标记为“基于保存值的估计”的字号计算，但不把它称为实际渲染字号。`lnSpcReduction` 不应简单作为全部行距统一乘数。文本编辑后保存的缩放参数可能陈旧：支持保留并诊断、交由指定客户端重算或显式移除缩放三种策略；库不声称自己已完成排版。

## 7. 跨 Run 文本编辑

### 7.1 数据模型

TextFrame 含段落；段落包含普通 Run、换行、字段及其他受支持内联节点。保留段落属性与结束字符属性，不把它们扁平化成 `[]string`。

逻辑文本使用 Unicode rune 索引，底层保留 UTF-8 字节跨度和 XML 位置映射。`Text()` 的换行和字段展示规则必须固定；提取结果还应提供 node kind，供调用方区分可编辑文本与显示字段。

### 7.2 确定性的替换规则

```go
func (p *Paragraph) ReplaceText(old, replacement string, opts ...ReplaceOption) (ReplaceResult, error)
```

默认规则：大小写敏感、字面匹配、左到右非重叠、不递归处理替换结果；拒绝空 old。匹配从初始文本快照计算，补丁从后向前应用。

| 策略 | 语义 |
|---|---|
| FirstCharacterStyle，默认 | 替换片段继承匹配首字符的显式字符格式；未命中的前后内容完全保留 |
| EqualLengthPerRune | old 与 replacement 的 rune 数必须一致；逐位置继承格式，否则返回错误 |
| ExplicitStyle | 调用方提供替换片段的格式，格式来源可审计 |

默认不跨段落、换行、字段或不同超链接动作边界；命中这些边界返回跳过原因或错误，由选项确定。replacement 含换行时首版拒绝，调用方使用显式段落/换行 API。复杂字素簇被部分匹配时默认拒绝，底层检查匹配边界；后续可加入字素簇索引策略。

首字符格式不自动包含超链接和动作。只有匹配范围具有相同链接目标且选项允许时才继承链接。删除空 Run 时保留仍有语义的未知扩展、字段与动作；不得顺手清理整个段落。

`ReplaceResult` 返回匹配数、修改数、跳过数、定位信息和诊断。全文替换由 Presentation 遍历可编辑段落，页面正文、备注、图表文本是否纳入必须由范围选项决定。

## 8. 形状、几何与图片

公共对象覆盖 TextShape、AutoShape、Picture、Group、Connector、Table、Chart 和 OpaqueShape。访问通用元信息不要求理解对象全部内容。保留形状顺序作为 z-order；提取文本顺序不自动等同于视觉阅读顺序。

对于不含旋转翻转的组，子坐标映射为：

```text
x' = off.x + (x - chOff.x) × ext.cx / chExt.cx
y' = off.y + (y - chOff.y) × ext.cy / chExt.cy
```

实现使用 3×3 仿射矩阵，并明确采用列向量。组映射 G 后，在组中心 C 周围执行局部翻转 F 和旋转 R：`Mgroup = T(C) × R × F × T(-C) × G`；嵌套组按父矩阵左乘。叶子形状的本地旋转、翻转在其自身坐标阶段组合，不重复应用组变换。屏幕坐标轴方向与旋转正方向须用规范及金样测试确认。

- `chExt` 为零时禁止除法，返回诊断；不把结果默认为 0。
- `Bounds()` 返回本地框；`WorldQuad()` 返回变换后的四角；`WorldAABB()` 返回世界轴对齐包围框。
- 框边界不含阴影、发光、描边膨胀，也不代表可见路径的精确包围盒；效果边界另行定义。
- 负位置合法；尺寸按具体 schema 类型及对象约束校验，不施加统一“所有数值必须大于零”规则。

图片首版支持 PNG/JPEG，明确原尺寸、裁剪、填充和保持比例行为。默认保留源媒体字节，不隐式重采样。SVG/EMF/WMF 等可保留，但编辑与预览能力分别声明。资源去重限定为已确认安全的媒体类型，按内容哈希及类型判断；不能把嵌入工作簿等任意二进制统一共享。

## 9. 表格与图表

### 9.1 表格

使用固定逻辑网格、行高列宽、富文本 Cell、合并锚点和覆盖单元格映射。合并必须验证矩形区域、不重叠，并按 OOXML 结构同步跨度和 continuation 标记。

提供 `Cell(row,col)`、`Merge(range)`、`Unmerge(range)`、`SetRowHeight`、`SetColumnWidth`。合并区域有多个非空单元格时默认拒绝；调用方明确选择只保留锚点或按指定规则合并文本。取消合并不承诺恢复被明确丢弃的内容。

样式分离为 StyleID、区域开关、单元格显式覆盖和解析结果。解析 whole table、条带、首末行列、角单元格等规则，由测试固定重叠优先级。若内置样式 ID 未在文件中给出完整定义，且库没有对应内置样式映射，返回 unresolved；不假定 `tableStyles.xml` 一定含有全部可计算定义。

`EffectiveCellStyle()` 应返回填充、边框、文字等各自解析状态，而非只返回一个 Color。首版可仅实现有测试覆盖的样式子集，但合并模型和保留策略必须从基础阶段建立。

### 9.2 图表

图表是涉及 chart Part、关系、数据缓存、可能存在的嵌入工作簿及样式资源的复合对象。数据更新必须同时维护相关缓存、系列引用及可编辑数据源，不能只改图表显示文本。

先支持创建明确限定的柱状、折线、饼图配置；每种声明轴、系列、标签和组合方式范围。复杂图表、外链数据、现代扩展图表先保留，不伪称可编辑。嵌入工作簿更新通过可替换适配接口实现，核心不因此承担完整 Excel 库开发。

在没有工作簿一致性实现前，不发布已有图表 `SetData` 为稳定能力。

## 10. 备注、音频与计时

备注是独立 Part 与母版关系体系。`SetSpeakerNotes()` 定位备注正文占位符，保留页码、页眉页脚、其他备注形状及未知节点，不通过重建整张 notesSlide 实现普通讲稿替换。

音频接口拆成两个步骤：

```go
func (s *Slide) AddAudio(ctx context.Context, src MediaSource, opts ...AudioOption) (*AudioShape, error)
func (a *AudioShape) SetPlayback(spec PlaybackSpec) error
func (s *Slide) SetAdvanceAfter(d time.Duration) error
```

嵌入音频与自动播放不是同一能力。AddAudio 管理媒体、图标/后备表示及关系；SetPlayback 按已验证的播放配置写入计时结构，可能涉及标准节点与 Office 扩展；SetAdvanceAfter 管理页面切换时间，不推断音频时长。

- 首版优先 MP3/WAV 的受测编码配置；文件扩展名不等于实际编解码格式。
- 现有 timing 树默认完整保留。只有能安全分配时间节点 ID、挂接动作并维持现有引用时才增量修改。
- 不支持合并的复杂动画返回错误；禁止为加入配音覆盖整个 timing 树。
- 对页面切换的单击行为、延迟起播、循环、跨页播放分别建模，后两者可先不支持。
- 音频时长由调用方传入或经 probe 获取；媒体解析和外部 FFmpeg 不是核心隐式依赖。
- 区分“成功嵌入”“客户端可播放”“可自动播放”“可按期翻页”，逐项测试发布。

## 11. 页面复制与关系闭包

复制页面不是复制一个 slide XML 文件。提供 ClonePolicy，分别决定媒体共享、图表及嵌入数据独立复制、版式与母版复用。对未知关系默认拒绝跨文档复制。

实现步骤为：遍历依赖闭包并用 visited 防循环；建立 Part 与 ID 映射；分配目标关系；重写已知引用；处理页内形状和 timing 引用；验证备注及图表依赖；一次提交。对无法识别的扩展内引用，不能宣称完成安全重映射。

首版先实现同文档内受限复制，跨文档合并放在基础读写稳定之后。删除页面也需检查节、自定义放映及已知跳转等引用；无法处理时拒绝操作，不能只删 presentation 中的一项。

## 12. 校验、诊断与保存策略

### 12.1 四层校验

| 层级 | 检查内容 | 执行时机 |
|---|---|---|
| L0 包与输入安全 | ZIP 条目、路径、大小预算、XML 深度、重复条目、受读条目完整性 | 打开及读取时；不可跳过资源边界 |
| L1 库内不变量 | 修改对象、ID、Content Type、受影响关系目标、引用一致性 | 每次保存必须执行 |
| L2 结构及规范规则 | 已实现的内容模型、字段范围、元素顺序、全包语义扫描 | 默认结构扫描；完整覆盖程度明确公布 |
| L3 客户端兼容 | 打开是否修复、视觉、播放与编辑后重存 | 发布流水线和金样测试 |

完整 Schema 验证可通过可选 Open XML SDK 验证器侧车执行，指定目标文件版本；其存在不意味着纯 Go 核心已实现完整规范验证，也不替代客户端测试。[S4]

`ValidationMode` 提供 `Changed / Structural / ExternalStrict`，默认 Structural。已有且可保留的非关键扩展问题可记 warning；缺失关键关系、无法保证输出一致性等 error 阻止保存。不是“既有错误一律放过”：安全边界和结构可靠性不可降级。

```go
type Diagnostic struct {
    Code     string
    Severity Severity
    Part     string
    NodePath string
    SlideID  SlideID
    ShapeID  ShapeID
    Message  string
}
```

### 12.2 操作级能力报告

用 `Inspect / Create / Edit / Preserve / Render / Play` 六维状态替代单个 FullySupported 标签。状态为 Supported、Partial、Unsupported、Untested，并附限制和适用客户端。

`SaveReport` 列出修改 Part、保留 Part、诊断、明确丢弃内容以及本次能主张的保真等级。默认不允许有意丢弃未知内容；需要 destructive 操作时使用针对具体目标的显式选项，不能一个全局 `AllowLossy` 静默放行全部内容。

## 13. 安全与资源控制

处理不可信 PPTX 时，资源限制是解析器基本能力：

- 限制条目数量、单 Part 与总解压大小、XML 深度、属性数量和媒体尺寸；阈值可配置，超限返回定位错误。
- 拒绝重复或规范化冲突的条目名称；不将包内路径直接拼接为本地提取路径。
- 禁止 DTD/外部实体处理与外链自动抓取；保留超链接不代表访问链接。
- 不执行宏、OLE、脚本或嵌入程序；可选媒体转换与渲染在受限进程中执行。
- 大媒体避免整包读入内存；从压缩输入读到的数据按实际字节计数，不能只信 ZIP 元数据。
- `context` 取消覆盖扫描、媒体复制与保存；错误日志不记录原始正文或音频内容。

建议起始预算：10,000 条目、单 XML 32 MiB、总解压 1 GiB、XML 深度 256；这些只是可调整工程初值，须以实际语料确定服务默认值。超大文件不自动放宽限制。

## 14. 渲染与 AI 集成边界

先定义外部 Renderer 接口，返回页面图像、字体环境、诊断和能力信息。LibreOffice 或 PowerPoint 自动化等实现应单独部署，具体平台约束与使用条件由适配器文档说明。

不在 MVP 同时开发完整字体 shaping、换行、复杂脚本、艺术字、SmartArt 和动画渲染。简单 Go 预览器若后续实现，标注为有限预览，不能用于证明 PowerPoint 视觉一致。

面向 AI 的提取结果包括：页面 ID、形状 ID、结构类型、文本及备注、几何、层级线索和诊断。隐藏页、画布外对象、被覆盖文本、备用分支及 OLE 中的文字分别标记；结构提取无法保证等于“人眼当前可见内容”。阅读顺序推断可选并标记推断结果。AI 生成内容经公共 API 回写，模型不直接操作未经检查的 OOXML。

## 15. 测试矩阵与发布门槛

### 15.1 语料组织

每个样本记录：来源许可、生成器及具体版本/平台、创建步骤、字体环境、OOXML 类型、特性标签、预期断言和已知问题。每个原生样本保留修改前后的金样，允许同一文件命中多个标签。

| 生成器组 | 读取 | 无编辑 B1 | 文本/备注编辑 | 图片/表格 | 音频播放 |
|---|---|---|---|---|---|
| PowerPoint：逐版本/平台 | 待测试 | 待测试 | 待测试 | 待测试 | 待测试 |
| WPS：逐版本/平台 | 待测试 | 待测试 | 待测试 | 待测试 | 待测试 |
| LibreOffice：逐版本 | 待测试 | 待测试 | 待测试 | 待测试 | 待测试 |
| Google Slides：导出日期 | 待测试 | 待测试 | 待测试 | 待测试 | 待测试 |
| Keynote：版本/平台 | 待测试 | 待测试 | 待测试 | 待测试 | 待测试 |

以上是测试计划，不是已验证兼容声明。发布时必须带通过数/样本数和特性覆盖，不能只给平均成功率。

### 15.2 必须覆盖的测试

| 类型 | 关键用例 |
|---|---|
| OPC | 相对关系、循环、共享媒体、重复名称、Override、外链、缺失目标 |
| XML 保留 | 未知属性/节点、前缀作用域、AlternateContent、实体转义、空白、补丁冲突 |
| 文本 | 中文、emoji、组合字符、跨 3 个 Run、超链接、字段、零匹配、重复匹配、格式取消 |
| 样式 | 逐属性缺省/false、占位符、列表级别、颜色映射、未解析字体、主题更新缓存失效 |
| 几何 | 非等比组缩放、双翻转、旋转、嵌套组、负坐标、零 chExt、四角边界 |
| 表格/图表 | 合并冲突、富文本、未知内置样式、缓存/工作簿一致、复制后数据隔离 |
| 音频 | 原动画保留、时间 ID 冲突、播放/切页独立、缺失媒体、时长越界 |
| 保存 | 磁盘满、writer 中途失败、取消、临时文件清理、目标恢复、重复保存 |
| 鲁棒性 | fuzz ZIP/XML/文本编辑输入；panic、无限循环、越界、资源突破均为阻断缺陷 |

### 15.3 发布硬门槛

- 所有已声明支持的金样操作通过结构断言及再次解析；输出不新增校验错误。
- 所有未修改 Part 的解压内容哈希一致；修改 Part 的未知区域符合其声明的补丁保证。
- PowerPoint/WPS 的受支持关键用例实际打开无修复提示；配音功能必须有播放记录。
- 已知高严重度数据丢失、悬空引用、保存损坏和安全缺陷为零。
- 示例通过编译，API 文档说明限制；benchmark 报告硬件、Go 版本、输入尺寸与 p50/p95/峰值内存。

性能先建立三档基线：10 页纯文本、50 页图文、100 页含大媒体；分别测打开、遍历、单处替换、保存。保存整体仍需复制输出包，不能承诺修改一个字为总耗时 O(1)。在没有基线前，不写“比 Python 快多少倍”。

## 16. 分阶段交付

以下工作量以熟悉 Go、能持续获取 Office/WPS 测试环境的开发者人周估算。阶段按依赖推进，功能交付以验收门槛为准；语料整理、格式适配和客户端验证计入任务。

| 阶段 | 交付物 | 依赖和验收 | 估算 |
|---|---|---|---|
| M0 技术验证 | OPC 打开、原始 XML 索引、单文本补丁、B1 报告 | 含动画及扩展样本修改后保留成功 | 2–3 人周 |
| M1 基础内核 | 关系图、最小新建、诊断、安全预算、受控保存 | 包级和失败保存门槛通过 | 3–5 人周 |
| M2 文本模板 MVP | 形状/图片、文本/备注、跨 Run 替换、占位符和基础样式 | 完成实际报告模板生成与编辑闭环 | 5–8 人周 |
| M3 格式和表格 | 颜色/字体扩展解析、几何、表格合并及样式 | 中英文与复杂组合金样通过 | 4–7 人周 |
| M4 配音功能包 | 嵌入、受限计时、翻页、客户端验证 | 保留原动画并实际播放通过 | 3–6 人周 |
| M5 图表与复制 | 受限图表、数据源同步、同文档复制 | 关系闭包和数据隔离验证通过 | 5–9 人周 |
| M6 扩展阶段 | 跨文档合并、Strict、更多客户端、渲染适配 | 各功能独立验收发布 | 单独估算 |

**M0–M2 约 10–16 人周，形成首个可用 MVP；M0–M5 合计约 22–38 人周，未含 M6。** 关键技术验证失败时先调整保存方案，避免上层功能建立在不可保真的底座上。M4 可在 M2 完成后按业务优先级前移，但不能绕过计时保留验证。

保留与诊断从首阶段实施；表格模型在表格功能开始时即考虑合并与样式；有效格式按有测试覆盖的子集逐步交付。

## 17. 关键决策记录

| ADR | 决策 | 原因 |
|---|---|---|
| 001 | 单 module、小核心、显式接口 | 控制独立开发规模和依赖复杂度 |
| 002 | xmlstore 唯一权威状态 | 防止结构体与原始 XML 双写分歧 |
| 003 | 未修改 Part 透传，已修改 Part 精确补丁优先 | 保留真实模板中的未知内容 |
| 004 | 保真以对象和操作范围声明 | 防止把包字节、语义和视觉混为一谈 |
| 005 | 继承属性与布局结果分离 | 核心可以解析样式但不等于排版引擎 |
| 006 | 写入最低不变量不可跳过 | 输出可靠性是核心产品能力 |
| 007 | 受控句柄、事务及 revision | 统一错误语义和缓存失效 |
| 008 | 不支持的高风险编辑显式失败 | 用户能依据诊断选择处理路径 |
| 009 | 渲染、TTS、AI、Excel 数据源适配独立 | 保持 Go PPTX 组件职责清晰 |
| 010 | 兼容矩阵由金样证据驱动 | 避免来源不明的兼容与性能承诺 |

## 18. 详细内部数据结构

### 18.1 文档与 Part 状态

内部结构示意如下；字段不能直接通过公共对象暴露。

```go
type DocumentStore struct {
    Package  *PackageStore
    Revision uint64
    Closed   bool
    Parts    map[PartName]*PartState
}

type PartState struct {
    Name        PartName
    ContentType string
    Original    EntrySource
    XML         *XMLDocument
    State       PartStatus // Unloaded / Loaded / Modified / Created / Deleted
    Revision    uint64
}

type XMLDocument struct {
    Original []byte
    Nodes    map[NodeID]*NodeRecord
    Root     NodeID
}

type NodeRecord struct {
    ID          NodeID
    Parent      NodeID
    QName       QName
    SourceRange ByteRange
    Attributes  []AttributeRecord
    Children    []NodeID
    Scope       NamespaceScope
}
```

`EntrySource` 持有可重复打开的原始条目访问能力；不强制将媒体放在 byte slice。Original 字节只针对已读取 XML 分配，按预算计费。节点 ID 是库内稳定句柄，不等同于 OOXML ShapeID；索引重建保留仍存活节点的句柄映射，删除后的 ID 不复用。

新建节点没有原始跨度，使用显式“无来源”状态，不能用 `[0,0)` 暗示它位于文件开头。ByteRange 采用半开区间且按 UTF-8 原始字节计数。

### 18.2 保存计划

```go
type SavePlan struct {
    BaseRevision uint64
    Entries      []PlannedEntry
    Diagnostics  []Diagnostic
    ChangedParts []PartName
}

type PlannedEntry struct {
    Name   PartName
    Action EntryAction // CopyOriginal / EmitPatched / EmitNew / Omit
    Source EntrySource
}
```

保存计划是当前文档 revision 的只读快照。在执行期间禁止对同一实例修改；若内部检测到 revision 不一致，返回 `ErrConcurrentModification`。它不是线程安全锁的替代，调用方仍须遵守单实例串行约定。

同一输出 Part 只能存在一个 PlannedEntry。Content Types、关系集合和主文档顺序必须基于同一变更集生成，不能分次保存。

### 18.3 IR 与诊断快照

`ir` 是可选只读包，依赖公共 `pptx`，核心不得导入 `ir`。IR 包含 schemaVersion、documentID、页面顺序、对象 ID、类型、文字、备注、变换、已解析格式、媒体引用和能力诊断。

IR 默认不嵌入媒体二进制，也不复制不透明 Part；通过资源句柄/哈希引用。输出 JSON 的 schemaVersion 与 SDK 版本独立管理。IR 适合内容提取、调试、预览输入和跨格式适配，但不允许 IR → PPTX 的默认覆盖式保存；未来导入只提供明确的有损新建接口。

## 19. 变更事务与底层算法

### 19.1 编辑事务

每个公共修改方法隐式开启内部事务。事务记录受影响 Part 的 revision、节点补丁、关系增删、ID 分配与资源暂存。

1. 定位对象与依赖，确认句柄有效、文件类型受支持。
2. 预检查不透明依赖、命名空间与关系重映射风险。
3. 在暂存区分配资源名、对象 ID、关系 ID 和时间节点 ID。
4. 验证补丁不重叠，验证跨 Part 的修改后状态。
5. 一次提交到 DocumentStore，并更新相关 revision。
6. 任一步骤失败丢弃暂存；公共对象可观察状态不改变。

普通 Setter 内存级回滚；包含大媒体的事务允许使用临时文件。事务提交前媒体复制/校验必须成功。资源暂存失败不留下可见 MediaShape。

首版不开放用户任意嵌套事务。全文替换和整套配音使用专门的批量计划 API，满足整批原子提交需求，避免暴露尚未稳定的底层事务对象。

### 19.2 Part 命名与标识符

- 新 Part 使用类型前缀和可用递增序号，但通过已存在名称集合查重，不以文件数推断编号。
- `rId` 在对应源 Part 内查重；保持原有 ID，不为美观全量重排。
- slide ID 按规范合法区间分配，并检查耗尽；排序用 presentation 中的列表顺序，不按数值排序。
- ShapeID 在对应形状树的适用作用域中检查；计时与连接器等引用同步更新。
- 与未知扩展共享的引用无法判断时，拒绝需要重映射的操作。

### 19.3 命名空间与序列化

新生成节点采用固定前缀表，输出时保证该前缀在目标作用域绑定到正确 URI。被移动片段如果依赖外层声明，必须补齐声明或进行完整可证明的前缀重写。涉及属性值中的 QName/前缀列表时，单纯重写元素前缀不够。

序列化器按受支持内容模型输出子元素顺序；通过局部插入锚点保持原节点次序。非 UTF-8 或无法建立可靠字节映射的 XML 首版拒绝语义编辑并给出编码诊断；只读或原样保留另行声明。不能在未知情况下转码整个 Part 后仍声称词法保真。

### 19.4 保存执行

保存分为预检、计划、临时输出、输出完成检查、提交与清理。XML 和变更后的关系检查在输出前完成；ZIP Close 错误必须处理。输出检查至少确认条目集合与计划一致、必要条目可再次解析；严格模式检查所有输出条目的解压完整性。

`Save` 默认禁止覆盖已存在目标，调用方通过 `WithOverwrite(true)` 显式启用。即使启用，也不能绕过源文件同实体限制。覆盖提交采用平台可验证的原子替换机制；平台无法满足时返回 `ErrAtomicReplaceUnavailable` 并保留旧目标，不退化成“删旧再写”。

提供 `WithDurability(Durable)`，在支持的平台执行文件同步及必要目录同步。默认保证错误处理和原子可见性，不承诺突然断电后的持久性。清理临时资源失败应作为诊断上报，不能伪装成文档输出失败或忽略目标已提交的事实。

## 20. 公共接口目录与统一语义

### 20.1 文档和页面

| 接口 | 结果与行为 | 阶段 |
|---|---|---|
| `New(opts...)` | 加载合法默认模板，返回文档和 error | M1 |
| `Open(path, opts...)` | 建立包索引与页面句柄；默认惰性解析 | M1 |
| `OpenReader(r, size, opts...)` | 使用调用方 ReaderAt，调用方维持其生命周期 | M1 |
| `Slide(index)` | 零基下标，越界返回错误 | M2 |
| `Slides()` | 新切片及 error，不解析全部页面形状 | M2 |
| `Layouts()` | 返回 LayoutRef 集合及 error | M2 |
| `AddSlide(layout)` | 增加页面及引用，返回句柄与 error | M2 |
| `MoveSlide(id,index)` | index 为最终列表目标位置；事务修改顺序 | M2 |
| `RemoveSlide(id)` | 处理已知引用，未知依赖阻止删除 | M2 |
| `Slide.Shapes()` | 返回顶层形状及 error，组合子树单独访问 | M2 |
| `Slide.Placeholders()` | 返回占位符集合及 error | M2 |
| `Save(ctx,path,opts...)` | 返回 SaveReport、error | M1 |
| `Write(ctx,w,opts...)` | 返回 SaveReport、error，不关闭调用方 writer | M1 |
| `Close()` | 幂等释放资源；关闭后对象访问返回 ErrClosed | M1 |

`LayoutRef` 绑定所属文档，不能把另一个文档的版式句柄直接传给 AddSlide。未显式导入前返回 `ErrForeignReference`。不存在“自动用第一个版式替代”的回退行为。

### 20.2 形状和富文本

```go
func (s *Slide) AddTextBox(spec TextBoxSpec) (*TextShape, error)
func (s *Slide) AddPicture(ctx context.Context, src MediaSource, spec PictureSpec) (*PictureShape, error)
func (s *Slide) AddAutoShape(spec AutoShapeSpec) (*AutoShape, error)
func (s *Slide) RemoveShape(id ShapeID) error
func (s *Slide) MoveShape(id ShapeID, zIndex int) error
func (s *GroupShape) Children() ([]Shape, error)
func (s *TextShape) TextFrame() (*TextFrame, error)
func (t *TextFrame) Paragraphs() ([]*Paragraph, error)
func (t *TextFrame) SetPlainText(text string) error
func (t *TextFrame) AddParagraph(spec ParagraphSpec) (*Paragraph, error)
func (p *Paragraph) Runs() ([]*TextRun, error)
func (p *Paragraph) AddRun(text string, style FontStyle) (*TextRun, error)
func (r *TextRun) SetText(text string) error
func (r *TextRun) SetFont(style FontStyle) error
func (r *TextRun) ResetFontProperty(prop FontProperty) error
```

`SetPlainText` 是明确的结构替换操作：换行生成段落，移除被替换正文内的字符格式、字段和链接，但保留文本框级属性。若正文存在不能安全删除的扩展，返回错误；不自动去除限制。保留格式的业务应调用 ReplaceText。

`SetFont` 采用 patch 语义：仅更新 `Set=true` 字段，未设置字段不变；恢复继承必须调用 Reset。读取 FontStyle 中的 `Set=false` 则表示原 XML 没有该本地属性。读取状态和写入 patch 的语义要在 GoDoc 中分别说明。

### 20.3 备注

```go
func (s *Slide) SpeakerNotesText() (string, error)
func (s *Slide) SpeakerNotes() (*TextFrame, error)
func (s *Slide) EnsureSpeakerNotes() (*TextFrame, error)
func (s *Slide) SetSpeakerNotes(text string) error
```

没有 notes Part 时，SpeakerNotesText 返回空串和 nil；SpeakerNotes 返回 `ErrNotFound`，读取不能创建 Part。EnsureSpeakerNotes 在需要时创建 notesSlide、notesMaster 及关系；已有合法对象直接复用。SetSpeakerNotes 明确替换讲稿正文的字符格式；精细编辑通过 SpeakerNotes 的富文本 API 完成。

### 20.4 配置和错误

OpenOption 负责加载模式、资源预算、解析策略；SaveOption 负责覆盖、验证范围、耐久性与输出策略；ReplaceOption 只负责文本替换。选项不得跨作用域生效。

所有可检查错误支持 `errors.Is`；具体上下文通过 `errors.As` 获取 `OperationError`。稳定错误码至少包括：

| 错误 | 含义 |
|---|---|
| `ErrClosed` / `ErrStaleHandle` | 文档关闭或节点已删除 |
| `ErrInvalidArgument` / `ErrOutOfRange` | 参数无效或索引越界 |
| `ErrNotFound` / `ErrForeignReference` | 对象不存在或来自其他文档 |
| `ErrUnsupportedFormat` / `ErrUnsupportedEdit` | 文件类型或编辑行为未支持 |
| `ErrLimitExceeded` / `ErrMalformedPackage` | 资源预算超限或包结构非法 |
| `ErrUnresolvedStyle` | 调用方要求完整格式，但解析不足 |
| `ErrValidationFailed` | 保存计划未通过所选校验 |
| `ErrTimingConflict` / `ErrDurationUnknown` | 播放树冲突或时长缺失 |
| `ErrConcurrentModification` | 计划 revision 不再匹配 |
| `ErrOutputExists` / `ErrAtomicReplaceUnavailable` | 目标已存在或无法安全替换 |

取消与超时保留 `context.Canceled` / `context.DeadlineExceeded` 的可检测性。解析模式 `Compatible` 可以容忍可保留的扩展，但不能容忍不确定的包路径冲突；`Strict` 对已知规范违例更严格，两者均受安全预算约束。

## 21. 自动配音与播放计时实施规格

### 21.1 媒体输入和所有权

```go
type MediaSource interface {
    Open(ctx context.Context) (io.ReadCloser, error)
    DeclaredType() string
}

type AudioSpec struct {
    TrackKey string
    Role     AudioRole // Narration / Background / Effect
    Source   MediaSource
    Duration Optional[time.Duration]
}

type PlaybackSpec struct {
    Trigger    PlaybackTrigger // OnSlideEnter / OnClick
    StartDelay time.Duration
    IconMode   IconMode        // Visible / HiddenDuringShow
}
```

MediaSource.Open 每次返回新流，组件负责关闭它。提供文件、byte slice 和调用方工厂适配器；从 `io.Reader` 添加的便捷方法必须在返回前完成有界复制。文件源也在修改事务完成前暂存校验，不让后续源文件变化影响 Save。

TrackKey 是当前文档中调用方控制的稳定业务标识，建议与页面和讲解片段关联。库将其写入自有命名空间的扩展元数据，并同时记录 role、shapeID、版本和内容哈希。只在受支持宿主节点存放经验证可保留的扩展；不能依赖 Shape.Name 唯一。

`Slide.UpsertNarration(ctx, AudioSpec, PlaybackSpec)` 按 TrackKey 查找并新增或更新自己的音轨。更新保留句柄或按报告给出映射，删除旧资源遵循共享引用规则。对不是库创建的音轨只读识别，未经显式绑定不覆盖。重复使用相同 key 和内容时不生成重复音频形状。

### 21.2 音频探测

媒体类型通过签名与结构检查确认，扩展名和调用方 MIME 作为辅助；不一致时返回错误。核心探测接口：

```go
type MediaProbe interface {
    Probe(ctx context.Context, src MediaSource) (MediaInfo, error)
}
```

MediaInfo 包含容器、编码、采样率、声道、时长、时长精度和来源。内置探测器分阶段支持明确的 WAV PCM 和 MP3 配置：WAV 需遍历 chunk 并考虑填充，不能固定读取某个偏移；MP3 需处理 ID3、VBR 信息及必要帧扫描，不能只按文件大小除以某一帧码率。未知/损坏/受限时返回 DurationUnknown。

声明时长不等于实测时长；报告记录 CallerProvided / HeaderDerived / FrameScanned 及允许误差。核心不为探测自动调用 FFmpeg。用户提供的探测器可支持其他编码，但播放能力仍受客户端矩阵约束。

### 21.3 页内计时计算

音轨 j 的结束时刻 `Ej = StartDelayj + Durationj`；若业务指定串行音轨，先显式计算各 StartDelay，核心不根据插入顺序猜测串行关系。

自动翻页时长为：`AdvanceAfter = max(Ej) + TailPadding`。参与集合默认只包含 Trigger=OnSlideEnter 且 Role=Narration 的有限音轨。背景音、点击触发和循环音轨不参与。已有复杂动画若未能解析，无法保证配音结束就适合翻页；默认冲突返回错误，业务可选择保留原计时并取得诊断。

- 无参与音轨：保持该页原计时并记录 skipped。
- 时长未知：默认整批计划失败；可选择跳过该页，但报告必须明确列出。
- 多音轨：取最大结束时刻，不把可并行音轨时长直接求和。
- 负时长或负 padding：拒绝；零时长无效音频不能参与同步。
- Duration 转毫秒：对非整毫秒向上取整，检查目标 schema 数值范围，避免提前翻页。
- 用户单击翻页开关独立指定，同步时不隐式改变。
- 同步只更新选中的页面，不重设未涉及页面的 transition 或 timing。

```go
func (p *Presentation) PlanTimingSync(ctx context.Context, opts TimingSyncOptions) (TimingPlan, error)
func (p *Presentation) ApplyTimingPlan(plan TimingPlan) (TimingSyncReport, error)
func (p *Presentation) SyncTimingToAudio(ctx context.Context, opts TimingSyncOptions) (TimingSyncReport, error)
```

Plan 不修改文档，包含 BaseRevision、每页音轨、计算依据、原/新时间、跳过原因与冲突。Apply 先检查 revision，再原子提交。Sync 是 Plan+Apply 的便捷封装。初版无通用复杂动画求值器，因此已有相关动画依赖需按受支持配置检测并限制。

### 21.4 OOXML 写入实现要求

为每个受支持 AudioProfile 保存“客户端创建的原始金样→归一化结构说明→模板生成器→语义断言”。Profile 说明媒体关系、图标、标准/扩展媒体标记、时间节点及引用位置；不可将单个 `onBegin` 条件当作完整自动播放实现。

写入器仅对已声明的 Profile 生成结构，按版本能力筛选。隐藏播放图标必须由 Profile 和客户端用例验证，不能通过随意设零宽高实现。AddAudio 默认只嵌入；自动播放和翻页需显式调用或通过组合 API 完成。

## 22. 自动讲解产品接入流程

业务服务分为任务编排、讲稿准备、TTS、文档合成、可选渲染和可选视频编码。SDK 不持有任务数据库、账号、模型密钥或对象存储凭据。

推荐流程：

1. 对原始 PPTX 计算源版本标识，在独立文档实例打开并提取页面/备注。
2. 有备注则使用讲稿；无备注则业务调用 AI 生成，存储来源和人工编辑后的最终稿。
3. TTS 在业务侧并行生成音频，缓存 key 包含讲稿、音色、语速及模型版本。
4. 获取每段音频的可靠时长；全部准备好后，在单个合成实例中串行执行 UpsertNarration。
5. PlanTimingSync 生成可检查计划，业务根据预设策略自动处理或呈现冲突，不把普通成功流程强制变成人工审批。
6. ApplyTimingPlan，保存为新文件，持久化 SaveReport 和 TimingSyncReport。
7. 如需 MP4，调用 Renderer 输出页面图，再交由视频编码器组合。静态页面图片不能复现 PPT 内动画；需要动画视频时必须选择具备放映捕获能力的后端。

业务请求使用 sourceHash + narrationConfigHash + SDKVersion 形成任务幂等标识。数据库事务与文件提交由业务编排补偿，不把文件保存成功等同于外部任务状态已更新。用户原始文档只读保留，成果文档单独管理。

## 23. 渲染、字体与 CLI 扩展接口

### 23.1 可选渲染接口

接口定义位于 `render` 子包，不在 Presentation 上提供会导致核心反向依赖的 RenderSlide 方法。

```go
type Renderer interface {
    Capabilities() RenderCapabilities
    RenderSlide(ctx context.Context, doc *pptx.Presentation,
        slideID pptx.SlideID, opts RenderOptions) (RenderedSlide, error)
}
```

RenderOptions 包含目标像素尺寸、DPI、背景、字体策略；如尺寸与 DPI 冲突，明确以像素尺寸为输出约束，DPI 仅作为元数据/换算输入。RenderedSlide 返回可流式读取的结果与诊断；RenderAll 采用逐页回调/迭代，避免一次返回所有页面图占用大内存。

外部适配器对内存文档先保存临时副本，再渲染；临时副本无权覆盖输入。它必须声明是否支持动画、字体替代、隐藏页和 Office 特性。进程超时、内存限制及临时文件清理由适配器负责。

纯 Go 原生预览若启动，单独立项并定义文本 shaping、CJK 换行、字体缓存、嵌入字体处理及支持覆盖表。该项目不阻断核心发布，也不以完整 Office 视觉兼容作为短期承诺。

### 23.2 CLI

| 命令 | 用途 | 默认行为 |
|---|---|---|
| `pptx inspect input.pptx --json` | 输出页面、媒体、备注及能力信息 | 只读，不导出媒体正文 |
| `pptx validate input.pptx --level structural` | 校验并输出诊断 | 不修复原文件 |
| `pptx replace input.pptx --old A --new B --output out.pptx` | 字面文本替换 | 默认不含备注，不覆盖已有目标 |
| `pptx narrate input.pptx --manifest tracks.json --output out.pptx` | 按外部音轨清单合成 | 不自动调用 TTS |
| `pptx timing-plan input.pptx --json` | 预览计时同步计划 | 不修改文件 |
| `pptx export-ir input.pptx --output out.json` | 导出结构快照 | 不内嵌媒体 |

统一退出码：0 成功，1 执行/I/O 错误，2 参数错误，3 校验或能力限制，4 资源超限。JSON 写 stdout，进度和错误摘要写 stderr；不得把日志混入 JSON。`--overwrite` 为显式覆盖开关。

tracks.json 使用带 schemaVersion 的结构，逐项指定 slideID、trackKey、source、role、durationMs、startDelayMs 和播放方式。相对 source 按 manifest 所在目录解析，禁止无意引用任意工作目录；业务服务可进一步限制允许根目录。

## 24. 开发工作包与完成标准

以下任务可作为开发 backlog，编号用于 PR、测试和发布记录关联。前置任务未通过时不得标记依赖功能完成。

| 编号 | 工作包 | 前置 | 完成标准 |
|---|---|---|---|
| CORE-01 | 初始化 module、CI、许可证和模板资源 | 无 | CGO_ENABLED=0 构建；最小示例编译；资源许可齐全 |
| OPC-01 | ZIP 索引、PartName 和资源预算 | CORE-01 | 路径/重复项/超限测试通过 |
| OPC-02 | 关系图、Content Types、主 Part 发现 | OPC-01 | 非固定名称、循环关系、Override 金样通过 |
| XML-01 | 命名空间感知解析与原始跨度 | OPC-01 | 前缀、空白、未知节点及编码限制用例通过 |
| XML-02 | 文本/属性补丁和结构插入 | XML-01 | 未修改区域保留，补丁冲突可检测 |
| SAVE-01 | 保存计划、未变 Part 复制 | OPC-02、XML-02 | B1 哈希回归全通过 |
| SAVE-02 | 原子保存、失败恢复和输出检查 | SAVE-01 | writer/磁盘/取消故障注入通过 |
| MODEL-01 | 页面、形状、句柄、惰性读取 | OPC-02、XML-01 | 关闭/删除后错误语义稳定 |
| TEXT-01 | 段落/Run、属性 patch 和备注 | MODEL-01、XML-02 | 富文本与 notesMaster 保留通过 |
| TEXT-02 | 跨 Run 替换和整批变更 | TEXT-01、SAVE-02 | Unicode/链接/字段边界通过 |
| STYLE-01 | 基础占位符和样式解析 | MODEL-01、TEXT-01 | 属性来源与未解析状态可追溯 |
| IMAGE-01 | PNG/JPEG 添加替换及媒体暂存 | MODEL-01、SAVE-02 | 裁剪、比例、共享引用不破坏 |
| GEOM-01 | 单位、组矩阵、四角边界 | MODEL-01 | 嵌套旋转翻转金样通过 |
| TABLE-01 | 富文本表格、合并、样式子集 | TEXT-01、STYLE-01 | 合并/取消与未知样式诊断通过 |
| MEDIA-01 | 媒体输入、类型检查、MP3/WAV probe | OPC-02、SAVE-02 | VBR、ID3、WAV chunk、超限通过 |
| AUDIO-01 | AudioProfile、音频形状和元数据 | MEDIA-01、MODEL-01 | 嵌入后客户端可播放 |
| AUDIO-02 | 播放树增量编辑和翻页 | AUDIO-01、XML-02 | 原动画保留及自动播放金样通过 |
| AUDIO-03 | 幂等配音和计时计划 | AUDIO-02 | 多轨/未知时长/重复执行无重复项 |
| CHART-01 | 受限图表和数据源适配 | MODEL-01、SAVE-02 | 缓存及工作簿一致，客户端可编辑 |
| CLONE-01 | 同文档受限复制 | CHART-01、AUDIO-02 | 资源/ID 重映射正确；未知关系拒绝 |
| TOOL-01 | CLI 和 IR | MODEL-01、TEXT-01 | JSON schema、退出码和只读约束通过 |
| QA-01 | 语料、fuzz、兼容报告 | CORE-01 起持续 | 每个功能均有金样与回归项 |

每个工作包提交：实现代码、公开接口 GoDoc、最小使用示例、正反向测试、相关语料许可记录、能力矩阵更新。不能以“接口已定义”或“XML 已生成”作为功能完成标准。

## 25. 验收用例与可复现证据

| 用例 | 输入与动作 | 必须观察到的结果 |
|---|---|---|
| AT-01 | 打开含 SmartArt/动画/嵌入对象文档，不修改保存 | 未修改 Part 哈希一致，受支持客户端不修复 |
| AT-02 | 把跨三个 Run 的“2025年营收”改为“2026年营收” | 未命中内容/格式保持，默认首字符格式规则成立 |
| AT-03 | 用 SetFont 显式关闭继承来的粗体，再 Reset | 第一次输出显式 false，第二次恢复继承 |
| AT-04 | 修改备注正文，原备注含页码和其他形状 | 只改变正文；notesMaster 和其他形状保留 |
| AT-05 | 给有原动画的页面新增讲解音轨 | 原 timing 子树保留，受测播放配置正确；不能合并则返回冲突 |
| AT-06 | 两段音频时长 10s/8s，起点 0s/6s，尾停顿 1s | 翻页为 15s，而非 19s；单击设置独立 |
| AT-07 | 存在未知时长的讲解音轨 | 默认整批同步不提交，报告准确定位页面和音轨 |
| AT-08 | 相同 TrackKey 和音频重复合成 | 不新增重复形状/媒体关系；报告为未变或更新 |
| AT-09 | 表格合并范围含多个非空单元格 | 默认拒绝，原单元格和样式不变 |
| AT-10 | 某图表的源数据更新 | 图表缓存与嵌入工作簿一致，客户端可继续编辑 |
| AT-11 | 写入临时目标中途失败，旧目标已存在 | 旧目标仍可读取且未变，输出失败状态清楚 |
| AT-12 | 计划后修改同一文档，再应用旧计划 | revision 检测拒绝旧计划，无部分提交 |
| AT-13 | 带未知依赖的跨文档复制请求 | 返回能力限制，不生成残缺目标页 |
| AT-14 | 恶意重复 ZIP 名称、超大 XML、越界路径 | 在预算内失败，无 panic、无文件逃逸 |

原始 PPT、操作参数、预期报告、SDK commit、工具链、客户端版本及截图/播放记录组成一份证据包。视觉比较固定字体和运行环境；不同客户端的差异分别记录。自动读取成功只说明结构可解析，不能替代实际打开和播放证据。

## 26. 工具链、依赖与发布管理

- 建仓时选择受支持的 Go 稳定工具链并写入 go.mod/toolchain；CI 验证项目最低版本与选定当前版本。文档不将某个动态版本写成永久“最新”。
- 标准库优先使用 archive/zip、encoding/xml、io、context、crypto/sha256、testing；第三方依赖必须纯 Go、许可清楚、用途局限，并记录替代方案。
- 核心构建验证 `CGO_ENABLED=0`；Windows、Linux、macOS 执行适用测试。原子替换及路径行为必须平台测试，不能仅交叉编译。
- 使用原生 fuzz、race 检查和静态分析；race 主要验证独立实例与内部工具，不代表单实例 API 已变为并发安全。
- CI PR 阶段跑单元和核心金样，定时任务跑完整 corpus、长时间 fuzz 和外部规范校验，发布候选跑客户端矩阵。
- 公共 API 在 v0.x 允许受控变更，发布说明列迁移方式；v1 前冻结句柄、选项、错误码、单位、报告结构和保存语义。
- 对外披露每个版本的支持矩阵、已知限制、依赖清单、性能基线和模板/语料许可。公开来源不自动意味着测试文件可自由再分发，逐项核实。
- 私有企业样本经脱敏和授权后进入受限回归库，不随开源项目分发。

## 27. 启动配置与执行顺序

无需等待完整功能选型即可采用以下基线启动：

| 配置项 | 默认决策 |
|---|---|
| 核心包路径 | module 根包 pptx；opc/xmlstore/ooxml 编解码细节置于 internal |
| 文件类型 | Transitional PPTX 优先，其余按文件范围表识别处理 |
| 解析 | 惰性、Compatible、资源限额开启 |
| 输出 | 新路径、Structural 校验、不默认覆盖、不默认清理孤立 Part |
| 编辑 | 精确补丁优先；不能安全保留则返回错误 |
| 并发 | 文档实例串行，跨文档并行 |
| 字体 | 解析样式，不暗中安装或下载字体 |
| 音轨 | 讲解音轨、有限时长、显式起点；背景和点击播放不自动计入翻页 |
| TTS/视频 | 业务层实现，SDK 不持有运行时依赖 |
| 预览 | 可选 Renderer，原生完整渲染单独规划 |
| 首批客户端 | PowerPoint 与 WPS 的实际可用版本，测试记录精确版本/平台 |
| 首个可交付范围 | M0–M2 模板编辑；配音相关 M4 可在依赖通过后前移 |

执行顺序为 CORE-01 → OPC/XML 并行模块开发 → SAVE → MODEL/TEXT/NOTES → 模板 MVP 验收 → MEDIA/AUDIO/TIMING → 表格、图表与复制扩展。这里的模块并行指团队排期，可由单人按依赖串行执行。

开工时先取得 20–30 份最小结构与真实文档样本，完成“只改一个 Run 并保留同页扩展”的验证程序，再扩展对象 API。模块名、正式许可、工具链和客户端版本应在仓库初始化提交中记录；这些项目配置不改变本文的实现边界。

## 28. 开发用接口组合示例

以下示例展示目标 SDK 的调用契约；在对应工作包完成后，须放入 examples 并通过编译及金样测试。示例使用已准备的讲稿和音轨，TTS 在调用方完成。module path 为建仓占位符。

配套类型和便利入口定义如下：

```go
func FileMedia(path string) MediaSource
func (s *Slide) UpsertNarration(ctx context.Context, audio AudioSpec,
    playback PlaybackSpec) (*AudioShape, error)

type TimingSyncOptions struct {
    TailPadding     time.Duration
    UnknownDuration UnknownDurationPolicy // Fail，零值；SkipSlide
    AnimationPolicy AnimationTimingPolicy // RejectUnresolved，零值；KeepExisting
}
```

FileMedia 构造时不执行 I/O，输入不存在等错误在消费源的 API 中返回。结构类型中所有零值的行为写入 GoDoc；含时间、单位或资源限制的配置不能把零值不加区分地解释为“无限”。

```go
package main

import (
    "context"
    "fmt"
    "time"

    pptx "example.com/yourorg/go-pptx"
)

type NarrationInput struct {
    SlideIndex int
    TrackKey   string
    Notes      string
    AudioPath  string
    Duration   time.Duration
}

func BuildNarratedPPT(ctx context.Context, input, output string,
    tracks []NarrationInput) error {
    doc, err := pptx.Open(input)
    if err != nil {
        return err
    }
    defer doc.Close()

    slides, err := doc.Slides()
    if err != nil {
        return err
    }
    for _, track := range tracks {
        if track.SlideIndex < 0 || track.SlideIndex >= len(slides) {
            return fmt.Errorf("slide index out of range: %d", track.SlideIndex)
        }
        slide := slides[track.SlideIndex]
        if err := slide.SetSpeakerNotes(track.Notes); err != nil {
            return err
        }
        _, err := slide.UpsertNarration(ctx, pptx.AudioSpec{
            TrackKey: track.TrackKey,
            Role:     pptx.Narration,
            Source:   pptx.FileMedia(track.AudioPath),
            Duration: pptx.Optional[time.Duration]{Value: track.Duration, Set: true},
        }, pptx.PlaybackSpec{
            Trigger:  pptx.OnSlideEnter,
            IconMode: pptx.HiddenDuringShow,
        })
        if err != nil {
            return err
        }
    }
    if _, err := doc.SyncTimingToAudio(ctx, pptx.TimingSyncOptions{
        TailPadding: time.Second,
    }); err != nil {
        return err
    }
    _, err = doc.Save(ctx, output)
    return err
}
```

该函数处理的是新打开的独立实例，任一步失败不保存成果文件，原始输入保持不变。单个 Upsert 操作原子，但整个 for 循环不是事务；如果调用方需要复用已修改实例并要求多音轨全有或全无，应实现专门的 NarrationPlan 批量接口后使用。生产服务还需收集 SyncTimingToAudio 和 Save 的报告，并将清单里的页面索引转换为带源文档版本的稳定页面 ID；不能在文档重新排序后复用旧索引。

## 29. 官方参考资料

以下资料作为实现依据入口。每个涉及 XML 结构的功能须在代码或测试清单记录具体规范条款、支持版本和金样来源；工程规则、工作量及功能排期以本文定义为准。

- [S1] [ECMA-376：Office Open XML 文件格式标准](https://ecma-international.org/publications-and-standards/standards/ecma-376/)
- [S2] [python-pptx：Text-related objects](https://python-pptx.readthedocs.io/en/latest/api/text.html)
- [S3] [Microsoft：Working with presentations](https://learn.microsoft.com/en-us/office/open-xml/presentation/working-with-presentations)
- [S4] [Microsoft：OpenXmlValidator](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.validation.openxmlvalidator?view=openxml-3.0.1)
- [S5] [Go：archive/zip](https://pkg.go.dev/archive/zip)
- [S6] [Go：encoding/xml 官方源码](https://github.com/golang/go/blob/master/src/encoding/xml/xml.go)
- [S7] [python-pptx：占位符与继承说明](https://github.com/scanny/python-pptx/blob/master/docs/user/placeholders-understanding.rst)
