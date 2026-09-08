# go-pptx 组件设计方案评审报告

> 评审对象：`docs/go-pptx-设计方案.md`
> 评审维度：架构清晰、组件化、可扩展、简单易用
> 结论：**方案整体方向正确、可立项**，三层分层、差异化定位、测试方法论、决策记录表都是亮点；但存在 **4 个 P0 级架构矛盾/缺失**，需在动工前修订，否则会在阶段三、五返工。

---

## 〇、总体评价（先说做得好的）

| 亮点 | 说明 |
|---|---|
| 分层边界意识好 | opc / ooxml / pptx / render+ir 四层职责划分明确，"opc 层不理解 PresentationML 语义"这条红线划得准，是未来扩展 docx/xlsx 的正确姿势 |
| 差异化定位清晰 | 音频嵌入 + 自动播放计时是真实痛点，且明确写成"三者都没做的核心工作"，产品驱动力强 |
| 测试方法论先行 | 真实语料库 + fuzz + 性能基线，且明确"阶段四不要拖到最后"，这是比代码本身更值钱的工程决策 |
| 决策记录（ADR 雏形）| 第九节的决策表是好习惯，建议每条补上"被否决的备选项"，防止未来反复横跳 |

---

## 一、P0 问题（动工前必须解决）

### P0-1【架构清晰】render 能力倒灌进核心包，与"可选子包"自相矛盾

第六节给出的 API 是：

```go
func (p *Presentation) RenderSlide(index int, opts RenderOptions) (image.Image, error)
```

`RenderSlide` 挂在核心 `Presentation` 上，意味着 `pptx` 包必须 import `render`（或 `RenderOptions` 必须定义在 `pptx` 包里），这与第二、四层"**render/ir 层可选、不进入核心依赖链**"直接矛盾。任何只需要读写能力的用户（你的主链路：读备注→嵌音频→存盘）都会被迫把渲染依赖（字体缓存、排版引擎）编译进二进制。

**修订方案**：渲染改为包函数式 API，依赖方向单向（render → pptx）：

```go
// render 包
func Render(s *pptx.Slide, opts Options) (image.Image, error)
func RenderAll(p *pptx.Presentation, opts Options) ([]image.Image, error)
```

`ir` 子包同理，必须是 `ir.FromPresentation(p)` 而不能是 `p.IR()`。**原则：核心包不 import 任何可选子包；可选能力一律"以核心类型为参数的包级函数"。**

### P0-2【组件化】媒体数据以 `[]byte` 驻留内存，推翻了自己的懒加载目标

3.2 节：

```go
type PictureShape struct { BaseShape; ImageData []byte; MimeType string }
type MediaShape   struct { BaseShape; MediaData []byte; ... }
```

第五节刚强调"图片等媒体资源按需读取，避免 python-pptx 那样每张图片都完整解码进内存"，但数据模型把全部媒体二进制做成了结构体字段——只要 `Slides()` 触发懒解析，音频文件就会全量进内存。对一个"200 页、每页 3 分钟 MP3"的讲解 PPT，这是几百 MB 的常驻内存。

**修订方案**：字段只存引用，数据按需拉取：

```go
type MediaShape struct {
    BaseShape
    MediaType MediaType
    AutoPlay, HideIcon, LoopPlay bool

    // 内部持有 part 引用 + 懒解析标记（未导出）
    // 首次调用才从 zip 流式读出
}
func (m *MediaShape) Data() ([]byte, error)     // 按需读取
func (m *MediaShape) WriteTo(w io.Writer) error // 流式导出，零整块缓冲
func (m *MediaShape) Duration() (time.Duration, error) // 按需探测时长并缓存
```

`PictureShape` 同理。这也让 `SyncTimingToAudio` 的时长探测天然变成按需 + 缓存，避免重复解码。

### P0-3【架构清晰】缺少"未知 Part 原样保留"的 round-trip 硬性策略

阶段一的验收标准是"round-trip 无损"，但全文没有定义**无损的判定标准和实现策略**。pptx 里存在大量 go-pptx 不理解的 Part：主题（theme）、视图属性（viewProps）、自定义 XML、第三方插件 Part、缩略图等。

**修订方案**（写进第二层原则，作为 P0 验收条款）：
1. opc 层对不认识的 Part **一律按原始字节保留**，不解析、不重排、不重新序列化 XML（未修改的 XML Part 也应原样透传字节，避免属性顺序/自闭合标签/命名空间前缀变化造成语义外的 diff）；
2. CI 增加 round-trip 断言：打开→保存后，对**未修改的 Part** 做 `sha256` 逐字节比对，100% 一致才算通过；
3. 由此明确一条实现纪律：ooxml 层结构体**只用于读和写己方修改的 Part**，"读出来再写回去"永远不是默认路径。

### P0-4【简单易用】懒加载的失败路径无处安放，API 签名吞错误

`Open()` 只解析 presentation.xml，`Slides() []*Slide`、`Notes() string` 都不返回 error。但懒加载意味着"任意一次访问都可能触发一次 zip 读 + XML 解析"，都可能失败（损坏 Part、IO 错误）。当前签名只能吞掉错误返回零值，用户拿到的是"静默的空内容"——这比崩溃更难排查。

**修订方案**：定义明确的错误传播策略，并落到签名上：

- **元数据在 Open 时就解析完**（slide 数量/顺序只是 presentation.xml 的几行数据，成本可忽略），所以 `Slides()` / `Slide(i)` 不会因懒解析失败——索引越界返回 error 即可；
- **会触发深解析的方法返回 error**：`func (s *Slide) Shapes() ([]Shape, error)`、`func (s *Slide) Notes() (string, error)`；
- 解析结果在 Slide 内缓存 + 记忆错误（首次失败后后续调用直接复用 error，不重复解析）；
- `OpenEager` 不再作为独立 API（见 P1-2），它的价值是"把所有懒加载错误提前到打开时暴露"，改为 `Open(path, WithEagerLoad())` 后语义不变。

同时在文档里补一节**"并发语义"**：Presentation/Slide 非并发安全，跨 goroutine 使用需外部加锁；渲染和读取可以并发的前提另行声明。

---

## 二、P1 问题（阶段一设计定型前解决）

### P1-1【简单易用】导入路径冗余：`go-pptx/pptx`

模块名 `go-pptx` + 高层包目录 `pptx/`，得到 `github.com/yourorg/go-pptx/pptx`。虽然能用，但违反 Go 命名惯例里"避免路径与包名重叠"的取向，且未来 `ir`、`render` 与核心的相对位置不对称。

**修订**：把高层 API 提升到**模块根包**：

```
github.com/yourorg/go-pptx        # 包名 pptx：Open/Save/Slide/Shape…（核心）
github.com/yourorg/go-pptx/render # 可选
github.com/yourorg/go-pptx/ir     # 可选
```

`opc`、`ooxml` 保持子目录但不对外承诺 API 稳定性（internal 化或文档声明"非稳定 API"），对外只承诺根包 + render + ir 三个稳定面。**组件化收敛的关键：对外暴露面越小，内部重构自由度越大。**

### P1-2【简单易用】构造 API 三套并存，约定不统一

`Open` / `OpenEager` / `New` 三个入口，本质是"配置不同"，应收敛为**函数式选项**一种约定，且全线统一：

```go
func Open(path string, opts ...Option) (*Presentation, error)
func OpenReader(r io.ReaderAt, size int64, opts ...Option) (*Presentation, error)
func New(opts ...Option) (*Presentation, error)

// 选项族
WithEagerLoad()          // 替代 OpenEager
WithReadOnly()           // 只读打开，Save 前拒绝写
WithSkipCorruptParts()   // 容错模式：坏 Part 跳过并收集到 p.Warnings()
```

`OpenReader` 是新增建议：你的产品后端会从对象存储读文件，`io.ReaderAt` 入口可以做到**下载即解析、不落盘**，这是简单易用 + 性能的双赢，成本极低（zip 标准库本身就支持 ReaderAt）。

同样地，`AddAudio(audioPath, opts)` / `AddAudioData(data, mimeType, opts)` 双 API 保留是合理的，但 `AudioOptions.StartDelay` 建议改为 `time.Duration` 之外再明确其实现依赖 `<p:timing>` 树——这是全文实现风险最高的 XML 区域，文档里应标注为"技术风险点"。

### P1-3【可扩展】音频时长探测应组件化为接口，而不是"内置一个探测器"

第九节决策"自研轻量 mp3/wav 头部解析器"方向正确，但把它设计成**可注册的探测接口**，才能在 TTS 输出格式变化（flac/opus/aac）时零侵入扩展：

```go
// 根包内定义接口，internal 内放 mp3/wav 实现
type DurationProbe interface {
    Match(mimeType string, head []byte) bool
    Duration(r io.Reader) (time.Duration, error)
}
func RegisterDurationProbe(p DurationProbe) // 用户可注入自定义格式
```

如果探测失败（不认识的格式），`SyncTimingToAudio` 的行为要定义清楚：整体失败？跳过该页并写入 Warnings？建议默认"跳过 + 警告"，并提供 `WithStrictTiming()` 严格模式。

### P1-4【组件化】Shape 模型缺口：占位符、集合访问、可变性

对照 python-pptx 心智模型，当前 Shape 体系缺三块：

1. **Placeholder 语义**：`AddSlide(layout)` 后往标题/正文占位符里填文本是最高频操作，需要 `slide.Placeholder(idx) (*PlaceholderShape, error)`，否则用户只能遍历 Shapes 按名字猜；
2. **集合与过滤**：提供 `Shapes()` 返回切片之外，补 `func ShapesOf[T Shape](s *Slide) []T` 或 `s.Shapes().Media()` / `.Text()` 这类过滤助手，讲解场景里"找这一页的音频"是高频查询；
3. **可变性**：`Shape` 接口只有 `Bounds() Rect` 读方法，补 `SetBounds(Rect)`、`SetName(string)`；没有写方法的接口意味着所有属性修改都要下沉到 XML，"简单易用"落空。

另外 `GroupShape` 需明确**子形状坐标系是组内坐标系**（chOff/chExt 变换），这是所有 PPT 库的经典踩坑点，建议写进 ooxml 层注释。

### P1-5【架构清晰】两个关键技术决策缺失，需补进第九节决策表

| 缺失决策 | 建议结论 |
|---|---|
| **XML 解析器选型** | `encoding/xml` 全量 DOM 化性能差且内存翻倍，达不到对标 office_oxide 的目标。建议：ooxml 层基于 `xml.Decoder` **流式 + 结构体直映射**，并在决策表记录"若性能不达标，fallback 到自研 tokenizer"的触发条件（如 corpus 基准 P95 超过 x ms/MB） |
| **Save 的写入口径** | 定义原子写：写临时文件 + `rename`，杜绝"Save 到 Open 的同一路径时中途失败留下半截文件"；同时定义 Save 之后对象是否可继续编辑（建议：可以，Save 是快照语义） |

### P1-6【简单易用】`AddSlide` 签名吞错误 + 空白模板来源未定义

`func (p *Presentation) AddSlide(layout *SlideLayout) *Slide`：
- layout 为 nil 时用默认 layout（合理），但新建 layout part 失败怎么办？改为返回 `(*Slide, error)`，与懒加载错误策略（P0-4）保持同一约定；
- `New()` 说"从空白模板创建"——模板文件是**嵌进二进制**还是外部资源？建议内嵌（`embed`），并在文档明确"内嵌模板支持的最大特性集"，因为从零生成 presentation.xml + master + layout + theme 是很大的隐藏工作量。**最省力且最稳的做法：内嵌一份 PowerPoint/WPS 生成的最小合法 pptx 作为种子模板。**

---

## 三、P2 问题（不阻塞立项，排期时留意）

1. **【可扩展】Chart/Table 模型只有名字没有模型**。`Chart` 类型未定义（数据系列/坐标轴/类型枚举），`Rows [][]Cell` 表达不了合并单元格（需要 `Cell{Span, RowSpan}`）。阶段六之前不要展开设计，但建议在文档里明确"阶段六前 API 不稳定"。
2. **【简单易用】文档声称"链式调用/Builder 模式"，但全文没有一处链式示例**。要么给一个 `slide.AddText("...").SetPosition(...).SetFontSize(18)` 的承诺并落实到实现，要么删掉这句话——写了不实现比不写更伤。
3. **【架构清晰】第六节工期严重低估**。一个能处理 CJK 换行、字体回退、形状填充/阴影、图表渲染的排版渲染引擎，3-4 周只够"文本框 + 纯色矩形"级别。建议阶段五改名"渲染器 MVP：仅覆盖你的讲解 PPT 实际用到的形状子集"，并给出"覆盖度清单"验收，而不是笼统的"Slide 转 PNG"。
4. **【可扩展】`MediaType`/`ShapeType` 建议文档化"枚举封闭性"**：枚举封闭（新增需改库），开放扩展点只有 `RegisterDurationProbe` 和 Options。明确这一点能避免用户拿类型断言硬猜。
5. **【简单易用】`Slide(index)` 与 `Slides()` 风格并存可以接受**（对标 python-pptx），但 `NotesRuns()` 建议并入 `Notes()` 的富文本版本 `NotesDocument()`，减少平行 API。
6. 语料库的**license 清单**要落库（LibreOffice/POI 测试文件各自协议不同），避免宣传 pass rate 时有合规瑕疵。

---

## 四、修订后的目录结构（合入建议）

```
go-pptx/
├── pptx.go / slide.go / shape.go / text.go / media.go / timing.go   # 核心：模块根包（对外稳定面 1）
├── options.go             # Open/OpenReader/New + 函数式选项
├── duration.go            # DurationProbe 接口 + 注册
├── opc/                   # 底层打包（文档声明：内部包，API 不稳定）
│   ├── package.go / relationship.go / contenttypes.go / streampkg.go
├── ooxml/                 # XML 结构体直映射（同上，不对外承诺）
├── render/                # 可选：render.Render(slide, opts)（对外稳定面 2）
├── ir/                    # 可选：ir.From(p)（对外稳定面 3）
├── internal/ {xmlutil, emu, probe(mp3/wav)}
├── testdata/ {corpus, fuzz}
├── examples/              # 01-roundtrip / 02-add-narration / 03-render（写文档前先写例子）
└── cmd/go-pptx-cli/
```

配套修订第四节分层原则，追加三条硬约束：
5. 核心包不 import render/ir；可选能力一律包级函数；
6. 未修改的 Part 原样透传字节，round-trip 以 sha256 逐 Part 一致为验收标准；
7. 所有会触发懒解析的公开方法必须返回 error；解析结果缓存并记忆错误。

---

## 五、结论与行动建议

| 优先级 | 行动 |
|---|---|
| 立即 | 按 P0-1 ~ P0-4 修订本文档（重点是 render 依赖倒置、媒体懒加载、round-trip 策略、错误传播策略） |
| 阶段一前 | 落实 P1-1/P1-2/P1-6（包结构、选项模式、AddSlide/模板种子），这些改动越晚做迁移成本越高 |
| 阶段一同步 | P0-3 的 sha256 round-trip 断言直接写进阶段一的验收标准 |
| 排期 | 阶段五改为"渲染器 MVP + 覆盖度清单"，避免按 3-4 周对内对外承诺 |

一句话总结：**这套方案输在"写出来的性能承诺"和"写出来的数据模型"互相打架（P0-2），以及"可选子包"和"方法挂在 Presentation 上"互相打架（P0-1）；把这两处矛盾解开、补上错误传播和 round-trip 两条硬策略，它就是一个架构清晰、边界干净、可以长期演进的好底座。**
