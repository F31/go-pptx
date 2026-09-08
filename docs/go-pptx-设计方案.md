# go-pptx 组件设计方案

> 目标：融合 office_oxide 的**性能与可靠性工程**、goppt 的**细粒度构建API与渲染能力**、python-pptx 的**人体工学与生态成熟度**，用纯Go（零cgo、零外部运行时依赖）打造一个专门服务于"PPT自动讲解工具"的组件库，核心能力是通用PPT读写 + 音频嵌入 + 自动播放计时。

---

## 一、三者优点提炼与借鉴策略

| 来源 | 值得借鉴的优点 | 在go-pptx里如何落地 |
|---|---|---|
| **office_oxide** | 大规模真实文件测试验证（6062个文件、100%通过率）；懒加载/流式解析带来的极致性能；格式无关的IR中间表示思想；对畸形/损坏文件的健壮容错 | 建立自己的"真实文件语料库"做回归测试；解析层默认懒加载，按需解析slide part；内部保留一份IR用于跨格式转换和调试，但**不**用IR作为唯一写入路径 |
| **goppt** | 细粒度、类型安全的构建API（`CreateChartShape`、`CreateRichTextShape`等）；渲染slide为PNG的能力；对图表/表格/动画分组的建模 | 直接借鉴其Shape/Chart/Table的API设计风格（链式调用+强类型枚举）；渲染模块作为可选子包，供MP4导出链路使用 |
| **python-pptx** | 成熟的对象模型（Presentation→Slide→Shape树）；`shapes`/`placeholders`集合式访问方式；社区多年踩坑积累的边界情况处理经验；文档和示例的完善度 | API命名与访问模式向python-pptx用户的心智模型靠拢，降低团队从Python迁移的学习成本；参考其issue列表提前规避已知的畸形文件问题 |

**核心差异化**（三者都没有做好的地方，也是go-pptx存在的意义）：
- **原生音频媒体嵌入 API**（`Slide.AddAudio()`）
- **幻灯片自动播放计时控制**（`Slide.SetAdvanceAfter(duration)`，对应OOXML `advTm`）
- **零cgo、纯Go静态编译**，同时追求接近office_oxide的解析性能

---

## 二、总体架构

```
go-pptx/
├── opc/                    # Open Packaging Convention 底层实现
│   ├── package.go          # zip读写、Part管理（借鉴goppt的opc/设计）
│   ├── relationship.go     # 关系图管理（.rels文件）
│   ├── contenttypes.go     # [Content_Types].xml 管理
│   └── streampkg.go        # 流式/懒加载包读取（借鉴office_oxide的性能思路）
│
├── ooxml/                  # PresentationML XML结构体定义
│   ├── presentation.go     # presentation.xml 映射
│   ├── slide.go            # slideN.xml 映射
│   ├── slidelayout.go
│   ├── slidemaster.go
│   ├── notesslide.go       # 备注页映射
│   └── media.go            # 媒体关系相关XML结构（新增，三者都缺）
│
├── pptx/                   # 面向用户的高层API（核心包）
│   ├── presentation.go     # Presentation对象，入口
│   ├── slide.go            # Slide对象，含AddAudio/SetAdvanceAfter等
│   ├── shape.go            # Shape树及各子类型
│   ├── text.go             # 富文本/段落/Run
│   ├── chart.go            # 图表（借鉴goppt）
│   ├── table.go            # 表格
│   ├── media.go            # 音频/视频嵌入的高层封装（核心差异化功能）
│   └── timing.go           # 幻灯片切换与计时控制（核心差异化功能）
│
├── render/                 # 可选子包：slide转图片（借鉴goppt renderer）
│   ├── renderer.go
│   ├── fontcache.go
│   └── layout.go           # 文本排版引擎，含CJK换行规则
│
├── ir/                     # 可选子包：格式无关中间表示（借鉴office_oxide思想）
│   ├── ir.go                # 用于跨格式转换、调试、序列化为JSON排查问题
│   └── convert.go
│
├── internal/
│   ├── xmlutil/             # XML命名空间处理工具
│   └── emu/                 # EMU单位换算（借鉴goppt的measurement.go）
│
├── testdata/                # 真实文件语料库（借鉴office_oxide的测试方法论）
│   ├── corpus/               # 收集自LibreOffice/POI测试集等公开来源的真实pptx
│   └── fuzz/                 # go-fuzz语料
│
└── cmd/
    └── go-pptx-cli/          # 命令行工具，便于调试和CI集成
```

### 分层设计原则
1. **opc层**：只关心"这是一个zip包，里面有若干Part，Part之间有关系图"，完全不理解PresentationML语义，可复用于未来扩展docx/xlsx。
2. **ooxml层**：纯粹的XML结构体+序列化/反序列化，是Presentation ML schema的直接映射，不包含业务逻辑。
3. **pptx层**：用户实际使用的高层API，隐藏XML细节，提供符合Go习惯的链式调用/Builder模式。
4. **render/ir层**：可选功能，按需import，不引入到核心依赖链，保持核心包体积最小。

---

## 三、核心数据模型设计

### 3.1 Presentation / Slide 对象模型（借鉴python-pptx的心智模型 + goppt的强类型）

```go
type Presentation struct {
    pkg          *opc.Package       // 底层包，持有zip读写句柄
    slides       []*Slide           // 懒加载，首次访问时才解析对应slide part
    slideMasters []*SlideMaster
    slideLayouts []*SlideLayout
    props        *DocumentProperties
}

func Open(path string) (*Presentation, error)      // 懒加载模式打开
func OpenEager(path string) (*Presentation, error) // 一次性全解析，调试/小文件场景用
func New() *Presentation                            // 从空白模板创建

func (p *Presentation) Slides() []*Slide             // 触发懒加载
func (p *Presentation) Slide(index int) (*Slide, error)
func (p *Presentation) AddSlide(layout *SlideLayout) *Slide
func (p *Presentation) Save(path string) error
func (p *Presentation) SaveTo(w io.Writer) error
```

**懒加载设计**（关键性能借鉴点）：`Open()`时只解析 `presentation.xml` 和关系图，得到slide数量和顺序；每个 `Slide` 对象内部持有一个"未解析"标记，首次调用 `.Shapes()`、`.Notes()` 等方法时才真正解析对应的 `slideN.xml`。这直接对标office_oxide强调的"200页大文件不应该在打开时就全量解析"的性能诉求，也回应了python-pptx社区里关于大文件性能的issue反馈。

### 3.2 Shape树（借鉴goppt的强类型Shape体系）

```go
type Shape interface {
    Type() ShapeType
    ID() uint32
    Name() string
    Bounds() Rect  // EMU单位
}

type TextShape struct { BaseShape; Paragraphs []*Paragraph }
type PictureShape struct { BaseShape; ImageData []byte; MimeType string }
type MediaShape struct {                       // 核心新增：音频/视频形状
    BaseShape
    MediaType   MediaType  // Audio | Video
    MediaData   []byte
    AutoPlay    bool
    HideIcon    bool
    LoopPlay    bool
}
type ChartShape struct { BaseShape; Chart Chart }
type TableShape struct { BaseShape; Rows [][]Cell }
type GroupShape struct { BaseShape; Children []Shape }
```

---

## 四、核心差异化功能设计（这是go-pptx存在的意义）

### 4.1 音频嵌入 API

```go
// Slide级别的高层封装，隐藏OOXML底层细节
func (s *Slide) AddAudio(audioPath string, opts AudioOptions) (*MediaShape, error)
func (s *Slide) AddAudioData(data []byte, mimeType string, opts AudioOptions) (*MediaShape, error)

type AudioOptions struct {
    AutoPlay   bool          // 放映时自动播放
    HideIcon   bool          // 隐藏播放图标
    StartDelay time.Duration // 延迟播放
}
```

**底层实现要做的事**（这是三个参考库都没做的核心工作）：
1. 计算音频文件的MIME类型，向 `[Content_Types].xml` 注册对应的`Default`/`Override`节点（如`audio/mpeg`）
2. 把音频二进制写入 `ppt/media/audioN.mp3`
3. 在对应 `slideN.xml.rels` 里添加 `relationship`，type指向 `.../relationships/audio`
4. 在slide XML里插入媒体形状节点：包含 `<p:pic>`（播放图标占位图）+ `<p:nvPr><a:audioFile r:link="rIdX"/></p:nvPr>` + `<p:timing>`节点里的媒体播放触发设置（`autoPlay`对应 `<p:cond evt="onBegin" delay="0">`）

### 4.2 幻灯片自动播放计时 API

```go
func (s *Slide) SetAdvanceAfter(d time.Duration)          // 设置该页停留时长后自动切下一页
func (s *Slide) SetAdvanceOnClick(enabled bool)           // 是否允许点击切换
func (p *Presentation) SyncTimingToAudio() error           // 便捷方法：遍历所有slide，
                                                             // 若该页有MediaShape，则自动将
                                                             // AdvanceAfter设置为音频时长
```

`SyncTimingToAudio()`是专门为你的"讲解PPT自动播放"场景设计的一键式API——这是三个参考库都不可能提供的、高度贴合你业务场景的便利方法。底层需要解码音频文件获取准确时长（可以嵌入一个轻量的mp3/wav时长探测器，避免引入完整的音频解码依赖）。

### 4.3 备注（Notes）读写增强

```go
func (s *Slide) Notes() string
func (s *Slide) SetNotes(text string)
func (s *Slide) NotesRuns() []*Run  // 支持富文本备注，而不只是纯文本
```

python-pptx对备注的支持是纯文本层面的，go-pptx应支持保留备注里的富文本格式（字体、加粗等），因为你的产品可能需要用户在备注里做"停顿标记"、"强调标记"这类富文本标注。

---

## 五、性能与可靠性工程（对标office_oxide的方法论）

### 5.1 真实文件语料库测试

在 `testdata/corpus/` 收集来自公开渠道的真实pptx文件（LibreOffice测试集、Apache POI测试集等，与office_oxide用的同源公开语料，都是宽松协议可复用），建立自动化回归测试：
- 每次CI跑全量corpus，断言零panic、零无限循环
- 记录解析耗时基准线，防止性能退化
- 记录pass rate指标，作为对外宣传的可信数据（模仿office_oxide"98.4% pass rate"这种量化可信度的方式）

### 5.2 Fuzz Testing

用Go原生的 `testing/fuzz`对zip解包层和XML解析层做fuzz测试，重点覆盖：
- 畸形zip（截断、损坏的中央目录）
- XML炸弹（entity expansion攻击）
- 超大slide数量/超大文本量的边界情况

### 5.3 懒加载 + 内存优化
- Part级别懒解析（见3.1节）
- 图片等媒体资源按需读取，避免像python-pptx那样"每张图片都被完整解码进内存"
- 提供 `Presentation.Close()` 显式释放zip句柄，避免python-pptx社区反馈的"未关闭文件"问题

---

## 六、渲染子系统设计（用于MP4导出链路）

直接参考goppt的思路，作为独立子包 `render/`：

```go
type RenderOptions struct {
    Width, Height int
    DPI           float64
    Format        ImageFormat // PNG
}

func (p *Presentation) RenderSlide(index int, opts RenderOptions) (image.Image, error)
func (p *Presentation) RenderAllSlides(opts RenderOptions) ([]image.Image, error)
```

渲染引擎需要处理：
- 文本排版（字体度量、自动换行，含CJK换行规则——这是goppt特别提到且做得比较细的部分，值得直接参考其思路）
- 形状填充、边框、阴影绘制
- 图表渲染（复用ChartShape的数据模型）

这一步做好后，你之前方案里"MP4导出依赖LibreOffice外部进程"这一环就可以被go-pptx自己的渲染器替代，真正实现整个后端服务是**单一Go二进制、零外部进程依赖**，完全达成你最初"部署简单"的目标。

---

## 七、API设计示例（体现python-pptx式的人体工学）

```go
package main

import (
    "time"
    "github.com/yourorg/go-pptx/pptx"
)

func main() {
    p, err := pptx.Open("input.pptx")
    if err != nil { panic(err) }
    defer p.Close()

    for _, slide := range p.Slides() {
        notes := slide.Notes()
        if notes == "" {
            continue
        }

        // 假设已经通过TTS生成好了对应的音频文件
        audioPath := generateTTSAudio(notes)

        _, err := slide.AddAudio(audioPath, pptx.AudioOptions{
            AutoPlay: true,
            HideIcon: true,
        })
        if err != nil { panic(err) }
    }

    // 一键把每页停留时间同步为对应音频时长
    if err := p.SyncTimingToAudio(); err != nil {
        panic(err)
    }

    if err := p.Save("output_with_narration.pptx"); err != nil {
        panic(err)
    }
}
```

这段代码风格上很接近python-pptx用户熟悉的写法（`Open`/`Slides()`/`Save`），但同时暴露了python-pptx原生做不到的`AddAudio`和`SyncTimingToAudio`——这正是go-pptx要解决的核心痛点。

---

## 八、分阶段开发路线图

| 阶段 | 目标 | 关键交付物 |
|---|---|---|
| **阶段一（2-3周）** | 打通opc层+基础读写 | 能打开/保存一个不改变内容的pptx（round-trip无损），验证opc层设计正确性 |
| **阶段二（2周）** | 文本/备注读写 | `Slide.Notes()`/`SetNotes()`，文本框读写，覆盖你项目"提取讲稿"的需求 |
| **阶段三（2-3周）** | **音频嵌入 + 计时控制**（核心） | `AddAudio`、`SetAdvanceAfter`、`SyncTimingToAudio`，跑通"自动配音PPT"最小闭环 |
| **阶段四（2周）** | 语料库测试体系搭建 | 参照office_oxide方法论，建corpus+CI回归测试，产出可信的pass rate指标 |
| **阶段五（3-4周）** | 渲染子系统 | Slide转PNG，为MP4导出链路去除对LibreOffice的依赖 |
| **阶段六（持续）** | 图表/表格/动画等通用能力补齐 | 逐步对齐goppt已有的图表类型和形状类型覆盖度 |

**建议优先级排序的理由**：阶段一到三是你的产品能跑起来的最小闭环，务必优先；阶段四虽然不直接产出用户可见功能，但强烈建议不要拖到最后——越早建立语料库回归测试，越能在开发阶段三、五这些复杂逻辑时及时发现round-trip损坏问题，避免后期返工。

---

## 九、关键技术决策记录

| 决策点 | 选择 | 理由 |
|---|---|---|
| 是否引入cgo | **否**，纯Go实现 | 保持你最初"部署简单、跨平台交叉编译无痛"的核心诉求，这是选择自研而非直接用office_oxide的根本原因 |
| 是否内置IR中间表示 | **是，但作为可选子包**，不作为唯一写入路径 | 借鉴office_oxide思想用于调试/跨格式场景，但避免因IR表达力不足而限制精细化OOXML操作（这是我们分析office_oxide时发现的潜在短板） |
| 音频时长探测 | 自研轻量mp3/wav头部解析器，不引入完整解码库 | 避免引入FFmpeg或CGO音频库，保持依赖树干净 |
| 渲染引擎字体处理 | 参考goppt思路，自建字体缓存+双精度hinting模式 | goppt已验证这条路径可行，没必要重新摸索 |
| 版本兼容性目标 | 兼容PowerPoint 2007+生成的文件 | 与goppt/python-pptx保持一致的现实主义目标，不追求兼容更古老的二进制.ppt格式（这部分可以未来考虑接入office_oxide的legacy格式转换能力作为补充） |

---

## 十、与你整体产品方案的衔接

回到你最初的技术方案文档，go-pptx将替代原方案里的这几个环节：

- **PPT解析模块**：go-pptx的`pptx.Open()` + `Slide.Notes()`直接替代python-pptx
- **PPT合成模块（导出带语音PPT）**：go-pptx的`AddAudio` + `SyncTimingToAudio`直接替代手写OOXML XML操作
- **MP4导出链路的"PPT转图片"步骤**：go-pptx的`render`子包替代LibreOffice headless依赖，真正做到后端服务零外部进程依赖

最终效果：你的整个后端服务可以退化成一个**纯Go静态二进制**，内部依赖只有go-pptx（自研）+ 一个TTS客户端SDK + FFmpeg（仅用于最后的音频/图片序列拼接成MP4，这一步FFmpeg调用相对简单且成熟，暂不建议自研替代）+ 任务队列客户端，完全符合你最初"部署简单、资源占用低、性能高效"的技术选型目标。
