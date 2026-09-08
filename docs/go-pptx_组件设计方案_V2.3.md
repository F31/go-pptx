# go-pptx 组件设计方案 V2.3

> 版本：V2.3｜日期：2026-09-07｜状态：设计提案，尚未实现或实测。
> 定位：使用 Go 构建类似 python-pptx 的演示文稿创建、解析和编辑组件，优先保障模板编辑的可控保真、API 易用性与输出可靠性。
> 审视依据：本次提供的 V2.2 全文；未获得 V1/V2.1 原文，因此不把其中被引用但未展开的设计视为已经完成。本版本补齐必要基础，作为可独立阅读的实施基线。

## 1. 评审结论与版本目标

V2.2 提出的兼容矩阵、格式解析、跨 Run 替换、内容保留、写入校验、几何变换与表格样式七个方向均值得保留。主要问题是“能力愿景”与“实现契约”尚未分开，部分表述扩大了实际保证范围，基础保存模型和功能依赖也没有闭合。

V2.3 的核心决策是：**小型纯 Go 核心 + 原始 XML 保留层 + 受控语义编辑 + 可验证保存**。先交付稳定的 PPTX 创建和模板编辑能力，再逐步扩展音频、图表及外部渲染适配。AI 讲稿、TTS、视频合成属于上层业务。

### 1.1 V2.2 审视与修订表

| 优先级 | V2.2 问题 | V2.3 修订 | 实现收益 |
|---|---|---|---|
| P0 | 只声明未识别 Part 透传 | 同时保护已修改 Part 内的未知节点、属性、命名空间及关系引用 | 避免改一段文字导致动画或扩展丢失 |
| P0 | 用 round-trip SHA256 表示无损，未定义对象 | 区分完整 ZIP、Part 内容、结构语义、客户端视觉和播放行为 | 建立可执行的保真验收 |
| P0 | 校验清单被称为 Schema 校验 | 分为包结构、语义不变量、规范校验与客户端验证 | 避免虚假的正确性保证 |
| P0 | 可跳过全部保存校验 | 保留最低安全及内部一致性检查，可跳过昂贵扫描 | 防止悬空引用进入输出 |
| P0 | 负坐标被笼统视为非法 | 按字段类型校验；允许合法的画布外位置 | 避免拒绝正常 PPT |
| P0 | 没有编辑事务、脏标记和保存失败语义 | 引入受控 Setter、变更集、保存计划与失败恢复 | 多 Part 操作保持一致 |
| P1 | 以一个 bool 表示整个 Font 是否显式设置 | 每个属性独立表达缺省、显式 false、显式值 | 正确建模继承 |
| P1 | EffectiveFont 被等同于实际显示 | 区分有效格式、保存的 Autofit 提示、布局计算结果 | 避免给出无法保证的字号与字体 |
| P1 | 仅用主题→母版→版式→形状线性覆盖 | 按属性族实现占位符、段落级别、颜色映射与主题引用规则 | 提升解析准确性 |
| P1 | “尽量保留格式”缺乏确定规则 | 首字符格式、等长逐字符、调用方指定三种策略 | 可预测、可测试 |
| P1 | 表格使用 Text string 和公开 Rows | 使用富文本单元格、逻辑网格、合并锚点与样式解析结果 | 防止合并及多格式文本丢失 |
| P1 | 音频、基础图表直接列为完全支持 | 能力按操作和已测试配置声明 | 避免把计划写成事实 |
| P2 | 渲染器排期过早 | 核心不内置完整排版引擎，先提供外部渲染接口 | 降低首版工程规模 |
| P2 | 竞品结论缺少仓库和版本证据 | 删除未验证的数量、比较性断言 | 使方案有据可查 |

### 1.2 需要纠正的事实表述

- python-pptx 的字体属性返回 `None` 可以合法表示继承，不等于“读取错误”；在此之上提供独立有效格式解析，是本项目的增量能力。[S2]
- 命名空间前缀、属性顺序、自闭合写法不同本身并不构成不兼容；XML 语义识别应使用命名空间 URI 与 local name。涉及前缀值的兼容标记及原始片段保留，需要额外处理。
- 不预设某一生成器“最规范”，也不将某产品所有版本的行为归纳为已证实事实。V2.2 中 WPS、Google Slides、Keynote 的差异描述改为测试假设，逐样本确认。
- 未验证 office_oxide 的“6062 文件、11 个渠道”、goppt 的具体 API 与渲染实现，也未获得这些项目的精确仓库和提交版本。保留语料回归、强类型建模的设计思路，不保留数量背书与“三库都没做好”等结论。

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

首版目标是可交付的 python-pptx 类组件，不承诺全部 OOXML 特性、像素级复刻 PowerPoint 或完整动画编辑。

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

以下为拟定 API 契约片段，非已存在的可编译 SDK；辅助类型和枚举在实施时补齐。

```go
func New(opts ...NewOption) (*Presentation, error)
func Open(path string, opts ...OpenOption) (*Presentation, error)
func OpenReader(r io.ReaderAt, size int64, opts ...OpenOption) (*Presentation, error)
func (p *Presentation) Close() error
func (p *Presentation) Slides() []*Slide
func (p *Presentation) AddSlide(layout LayoutRef) (*Slide, error)
func (p *Presentation) RemoveSlide(id SlideID) error
func (p *Presentation) MoveSlide(id SlideID, index int) error
func (p *Presentation) Save(ctx context.Context, path string, opts ...SaveOption) (SaveReport, error)
func (p *Presentation) Write(ctx context.Context, w io.Writer, opts ...SaveOption) (SaveReport, error)
func (p *Presentation) Validate(ctx context.Context, opts ...ValidateOption) ValidationReport
```

- `New()` 使用库内合法最小模板或调用方模板，模板分发需附许可信息。
- `Slides()` 返回新切片；元素为受控句柄。对象内部字段不公开可变切片和 map。
- Shape、Run 等句柄在删除后失效，访问返回可识别错误；不能继续写入游离对象。
- `OpenReader` 不关闭调用方传入的 ReaderAt；`Open` 持有文件资源，由 `Close` 释放。媒体支持惰性加载及大对象落盘。
- 单个 Presentation 首版不保证并发安全，包括会填充缓存的读取；多个独立实例可并行使用。
- setter 错误不改变对象；跨 Part 操作先暂存，再一次提交。成功提交递增 revision。
- `Save(path)` 默认不允许与仍被惰性读取的源文件同路径。另行实现安全原位替换前，要求新输出路径。
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
func (s *Slide) AddAudio(src MediaSource, opts ...AudioOption) (*AudioShape, error)
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

以下工作量为设计估算，以熟悉 Go、能持续获取 Office/WPS 测试环境的开发者人周计；不是承诺工期。个人开发可按同一顺序推进。语料整理和接口学习会显著影响周期。

| 阶段 | 交付物 | 依赖和验收 | 估算 |
|---|---|---|---|
| M0 技术验证 | OPC 打开、原始 XML 索引、单文本补丁、B1 报告 | 含动画及扩展样本修改后保留成功 | 2–3 人周 |
| M1 基础内核 | 关系图、最小新建、诊断、安全预算、受控保存 | 包级和失败保存门槛通过 | 3–5 人周 |
| M2 文本模板 MVP | 形状/图片、文本/备注、跨 Run 替换、占位符和基础样式 | 完成实际报告模板生成与编辑闭环 | 5–8 人周 |
| M3 格式和表格 | 颜色/字体扩展解析、几何、表格合并及样式 | 中英文与复杂组合金样通过 | 4–7 人周 |
| M4 配音功能包 | 嵌入、受限计时、翻页、客户端验证 | 保留原动画并实际播放通过 | 3–6 人周 |
| M5 图表与复制 | 受限图表、数据源同步、同文档复制 | 关系闭包和数据隔离验证通过 | 5–9 人周 |
| M6 扩展阶段 | 跨文档合并、Strict、更多客户端、渲染适配 | 各功能单独评审发布 | 单独估算 |

**M0–M2 约 10–16 人周，形成首个可用 MVP；M0–M5 合计约 22–38 人周，未含 M6。** 关键技术验证失败时先调整保存方案，避免上层功能建立在不可保真的底座上。M4 可在 M2 完成后按业务优先级前移，但不能绕过计时保留验证。

相较 V2.2，保留与诊断从首阶段实施，表格模型在表格功能开始时即完整考虑；有效格式先交付可证实子集，不把全量继承解析压缩进固定 2–3 周。

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

## 18. 开工清单与未决项

可直接采用的默认决策：纯 Go 核心、单 module、Transitional PPTX 优先、默认保留未知内容、单实例不并发、输出到新路径、文本/备注/图片优先、渲染后置。

开工顺序：先建立 20–30 份覆盖关键结构的合法语料及边界样本；完成单段文字修改的 XML 保留原型；验证 PowerPoint 与 WPS 打开不修复；冻结 Open/Save、单位类型、Diagnostic 与属性状态语义；再扩展高层 API。

后续实施中需要确认但不阻断 M0 的事项：正式仓库和模块路径、开源许可、Go 最低版本、客户端测试版本、首批图表类型、音频编码配置、超大文件服务预算。竞品源码借鉴须记录精确仓库、commit、许可证与采纳范围。

## 19. 参考依据与证据范围

以下来源用于核验格式、API 语义与底层机制。模块划分、编辑策略、路线图和预算为本方案工程建议，不是来源直接给出的结论。访问日期：2026-09-07。

- [S1] ECMA-376，Office Open XML 文件格式标准入口：四部分分别涵盖标记语言、OPC、标记兼容与迁移特性。https://ecma-international.org/publications-and-standards/standards/ecma-376/
- [S2] python-pptx Text-related objects：字体属性缺省/继承、文本对象接口。https://python-pptx.readthedocs.io/en/latest/api/text.html
- [S3] Microsoft Learn，Working with presentations：PresentationML 文档结构及 Part 组织入口。https://learn.microsoft.com/en-us/office/open-xml/presentation/working-with-presentations
- [S4] Microsoft OpenXmlValidator：按目标文件版本验证元素、Part 与包的 API。https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.validation.openxmlvalidator?view=openxml-3.0.1
- [S5] Go archive/zip：Writer.Copy、Writer.Close 等保存机制及限制。https://pkg.go.dev/archive/zip
- [S6] Go encoding/xml 官方源码：语义解析与序列化实现参考，不作为原字节保留保证。https://github.com/golang/go/blob/master/src/encoding/xml/xml.go
- [S7] python-pptx 官方仓库，占位符继承说明。https://github.com/scanny/python-pptx/blob/master/docs/user/placeholders-understanding.rst

实施时应为具体算法补充对应 ECMA 条款编号与固定版本金样；本次未逐条展开标准全文，也未执行客户端或原型测试。
