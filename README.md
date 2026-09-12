# go-pptx

纯 Go（`CGO_ENABLED=0`）编写的 PowerPoint（OOXML / PPTX）创建、编辑、审计 SDK，**零外部运行时依赖**，可在 Linux / Windows / macOS 与浏览器 WASM 环境运行。

```text
Product · go-pptx
License · Apache-2.0
Go · >= 1.24
Repo · github.com/F31/go-pptx
```

---

## 产品定位

go-pptx 面向"**程序化加工 PPTX**"这一在 Go 生态中长期空缺的场景：开发者希望像操作结构化文档一样——创建页面、替换文本、嵌入图片/图表/音频/视频、批量渲染模板、审计文档差异——而**不依赖** Office 客户端、LibreOffice 安装或任何解释器运行时。

与把文档"读出再整体重写"的实现不同，go-pptx 以 **OOXML 原始字节为不变量**：只对目标区域做精确补丁，未触碰的 Part 与未知扩展保持字节恒等（B1），从而把"程序加工"对文档的副作用降到最低——这是它区别于通用序列化式库的核心。

---

## 快速开始

```bash
go get github.com/F31/go-pptx
```

### 创建与编辑

```go
package main

import (
	"context"
	"log"

	"github.com/F31/go-pptx"
)

func main() {
	p, err := pptx.New()
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	layouts, _ := p.Layouts()
	slide, _ := p.AddSlide(layouts[0])

	box, _ := slide.AddTextBox(pptx.TextBoxSpec{
		X: 914400, Y: 914400, Width: 6096000, Height: 914400, // 单位 EMU，1 in = 914400
		Text: "Hello, go-pptx",
	})
	tf, _ := box.TextFrame()
	tf.ReplaceText("go-pptx", "World") // 跨 Run 保真替换，自动继承格式

	if _, err := p.Save(context.Background(), "out.pptx"); err != nil {
		log.Fatal(err)
	}
}
```

### 模板数据绑定与语义审计

```go
// 数据绑定：{{name}} / {{#if}} 条件段落 / {{#each rows}} 表格行循环 / 图表数据
rep, _ := p.Bind(map[string]any{
	"name":  "Q3 营收",
	"rows":  []any{map[string]any{"k": "华东", "v": 120}},
})

// 语义 diff：加权 LCS 页面对齐，"同一页两次修订"不会被误判为删页+加页
report := ir.Diff(docA, docB)
_ = rep // BindReport{...}
_ = report
```

> 完整 API 清单见包文档（`go doc github.com/F31/go-pptx`）与 `docs/go-pptx-实施状态跟踪.md`。

---

## 核心能力

| 类别 | 能力 |
|---|---|
| **创建** | `New()` 生成最小合法模板；`AddSlide` / `AddTextBox` / `AddAutoShape` / `AddPicture` / `AddChart` / `AddAudio` / `AddVideo`；`MoveSlide` / `RemoveSlide` / `MoveShape` / `RemoveShape` |
| **文本编辑** | 段落 / Run 对象模型；跨 Run 字面替换（3 种格式策略，字素簇保护，br/字段/链接边界安全）；字体/颜色/字号 patch 与 Reset；文本框属性与 slidenum/datetime 字段 |
| **图片/媒体** | PNG/JPEG 嵌入与 4 种 Fit 模式（拉伸/Contain/Cover/原尺寸）；媒体内容哈希去重；MP3/WAV/MP4/WebM 纯 Go 探测（时长、签名、容器品牌）；音频播放树与计时计划 |
| **图表/表格** | 柱/折/饼三类图表 + 内嵌工作簿生成（缓存与工作簿数据一致）；趋势线/误差线/数据标签/轴扩展；富文本表格、单元格合并、表格样式解析与优先级矩阵 |
| **模板绑定** | 内联 `{{path}}`、条件段落、表格行循环、图表数据绑定；plan 阶段纯读校验 + 单事务原子提交，**失败零残留** |
| **只读审计** | 只读 IR（schemaVersion 化 JSON）、语义 `Diff`、`Validate`、`Capability` 六维能力 manifest |
| **保真机制** | SpanPatch 精确字节补丁、未知子树/扩展原样保留、revision 并发防护、原子落盘与失败恢复 |
| **浏览器端** | WASM 编译目标 + 离线静态检查页（文件不离开设备，无外网/无 CDN） |

---

## 应用场景

- **自动化报告与文档流水线**：把数据库/接口数据渲染进模板，批量产出季度报告、方案书、销售材料；替换文本、图片、图表、表格数据。
- **CI 质量门**：在构建/发布流水线中对产物做 `validate` 与 `diff`（"本次构建相比基线改了什么"），阻止含损坏结构的 PPTX 流出。
- **受限/离线环境**：内网、边缘设备、浏览器端——单二进制或 WASM，无 Office 许可、无运行时安装。
- **隐私敏感处理**：浏览器端只读检查/审计在用户设备本地完成，文件不上传。
- **模板驱动的合同/标书**：`Bind` 按数据源一次渲染多页，条件段落自动显隐，表格按数据行数伸缩。

---

## 产品优势与主流开源产品对比

| 维度 | **go-pptx** | python-pptx | Apache POI (XSLF) | LibreOffice UNO | Aspose.Slides | Open XML SDK |
|---|---|---|---|---|---|---|
| 语言 / 运行时 | **Go，零运行时依赖** | Python 解释器 | JVM | C++/多语言 | .NET/Java/云 | .NET |
| 部署形态 | 单二进制 / WASM | 需 Python 环境 | 需 JVM | 需安装 LibreOffice | 需 runtime + 商业授权 | 需 .NET |
| 编辑保真 | **字节级精确补丁**，未触碰区零改动 | 读出后整体重写 package | 重建文档 | 重建文档 | 重建文档（保真较好） | 全量重写 |
| 未知扩展保留 | **原样字节保留**（Opaque 策略） | 依赖重写路径，易丢失 | 视解析覆盖 | 视解析覆盖 | 视版本 | 结构级保留 |
| 浏览器端离线只读 | **WASM，文件不离设备** | 无 | 无 | 无 | Web API（需上传） | 无 |
| 能力自描述 manifest | **有（六维 Supported/Partial/…）** | 无 | 无 | 无 | 无 | 无 |
| 语义 diff（页对齐） | **内置（加权 LCS）** | 无 | 无 | 无 | 有（差异比较） | 无 |
| 模板数据绑定 | **内置**（内联/条件/行循环/图表） | 需配合 jinja2 等 | 无 | 有（宏/脚本） | 有（模板 API） | 无 |
| 媒体时长/签名探测 | **纯 Go**（WAV/MP3/MP4/WebM） | 无内置 | 无内置 | 有 | 有 | 无 |
| 许可 | **Apache-2.0** | MIT | Apache-2.0 | MPL-2.0 | 商业 | MIT |

**差异化要点**

1. **唯一"字节级保真"的纯 Go 实现**：真实样本上只改一个 Run 时，编辑前/后其余 Part 字节恒等（B1），被改 Part 内未触碰区域差异收敛到 1 字节——适合对"文档不被工具重写破坏"敏感的存量文档加工。
2. **Go 生态中目前缺少同量级 PPTX SDK**：单二进制分发、与现有 Go 服务/CI 无缝集成、WASM 直接进浏览器。
3. **内置审计与能力协商**：`Diff`、`Validate`、`Capability` 让调用方在部署前就知道哪些能力可用、哪些受限，避免运行时才发现不支持。
4. **开源且可自持**：Apache-2.0，无运行时/许可成本，对比商业方案（Aspose.Slides）在批处理与自动化场景成本更低。

---

## 技术创新

- **原始字节 + 命名空间环境双模型**：自研 `internal/xmlstore` 词法扫描器同时保留每个元素的字节跨度与解析后的命名空间作用域；未知命名空间子树照常建节点、保留原始字节，从而支撑"只改一个 Run、其余字节不变"的保真目标。同时规避了 `encoding/xml` 在特定自闭合标签组合上的深度同步偏差。
- **事务化编辑**：所有业务写路径收敛为 `SinglePartPatch` / `MultiPartPlan`——先纯读校验（plan），再单事务应用（apply），任一失败零残留；revision 校验拒绝并发修改。行为与"数据库事务"对齐，而非"就地改写字符串"。
- **字节级保真补丁引擎（B1）**：SpanPatch 区间替换 + 锚定校验 + 冲突检测，同一变更集整体校验后一次性按偏移降序应用；未变 Part 原样复制，Content Types 与变更集同源再生成。
- **能力自描述与安全边界**：`CapabilityManifest` 六维状态（Inspect/Create/Edit/Preserve/Render/Play）逐工作包登记支持度与限制；WASM 检查页在浏览器本地完成，文件不出设备。
- **语义 diff 的页面对齐**：加权 LCS + 形状 ID 集合相似度，把"同一页的两次修订"与"删页+加页"区分开；顺序乱序残留用高阈值二次配对并标记移动。
- **语料驱动的工程验证**：36 份真实 PPTX 样本索引 + 3 份可再分发 LibreOffice 金样（含 actions 重放与字节级 B1 断言）；编辑后文件经 PowerPoint / WPS 真机打开无修复提示（L3 8/8）。

---

## 能力矩阵（Capability 六维）

| 维度 | 状态 | 说明 |
|---|---|---|
| Inspect | **Supported** | 只读 IR、形状/表格/图表/几何/效果/字体/版式/讲义/嵌入字体报告 |
| Create | **Partial** | 文本框/自选图形/图片/图表/音视频/页面创建；AutoShape 几何由调用方指定 preset 名 |
| Edit | **Partial** | 跨 Run 文本替换 + 属性 patch + 受限播放/过渡/绑定/复制 |
| Preserve | **Supported** | 字节级保留（B1）+ 保存报告 + 原子落盘 |
| Render | **Untested** | 设计态推迟（§14） |
| Play | **Partial** | 配音受限 + timing 只读透传 |

---

## 质量与验证

- **语料**：36 份真实 PPTX 样本（私有 33 份仅登记 manifest）+ 3 份公开 LibreOffice 金样（含 `.odp` / 原始 `.pptx` / 编辑后 `.edited.pptx` / 动作重放 / 兼容冒烟），`validate` 全过。
- **真机兼容（L3）**：编辑后样本经 **PowerPoint 16.0.20326** 与 **WPS 演示 12.1.0.28599** 打开无修复提示、重存后 go-pptx 重开 `Validate` 0 错误（8/8）。矩阵与复现工具见 `docs/client-compat-matrix.md`、`scripts/l3/`。
- **覆盖率**：full total 84.4%，root 82.8%（行/包加权口径）；路线图与口径见 `docs/coverage-roadmap.md`。
- **CI**：`go vet` / `go test` / `CGO_ENABLED=0` 三交叉构建（Linux、`js/wasm`、`wasip1/wasm`）。

---

## 架构与目录

```text
go-pptx/
  *.go                 # 公共对象层（根包 pptx）：Presentation / Slide / Shape / TextFrame …
  internal/opc/        # ZIP 索引、PartName、Content Types、关系图、原子保存
  internal/xmlstore/   # 原始字节扫描器、节点索引树、命名空间环境、SpanPatch 补丁引擎
  internal/document/   # PartStore / PatchStore 契约（编辑边界）
  internal/editplan/   # SinglePartPatch / MultiPartPlan（事务计划）
  internal/textmap/    # 文本逻辑位置与 XML 节点映射
  internal/audioprobe/ # WAV/MP3 探测
  internal/videoprobe/ # MP4/WebM 探测
  ir/                  # 只读中间表示 + 语义 Diff
  render/              # 渲染接口（适配实现后续拆分）
  cmd/pptx/            # inspect / validate / replace / bind / diff / capability 等 CLI
  cmd/pptx_check/      # WASM 主入口（js && wasm）
  wasm/site/           # 离线浏览器端检查页
  testdata/corpus/     # 样本索引与公开金样
  docs/                # 设计、ADR、兼容矩阵、覆盖率路线图
```

---

## 文档索引

- 设计基线：《go-pptx 完整设计方案 V2.6 开发实施版》（`docs/go-pptx_完整设计方案_V2_6_开发实施版.md`）
- 架构现状：`docs/architecture-current.md`
- 兼容矩阵（L3 真机证据）：`docs/client-compat-matrix.md`
- 覆盖率路线图：`docs/coverage-roadmap.md`
- 实施状态跟踪：`docs/go-pptx-实施状态跟踪.md`
- 架构决策（ADR）：`docs/adr/`
- 技术白皮书：`docs/go-pptx-技术白皮书.md`

---

## 开发命令

```bash
go build ./...                         # CGO_ENABLED=0 构建（CI 强制）
go test ./...                          # 单元与金样测试
go vet ./...                           # 静态检查
GOOS=js GOARCH=wasm go build ./...     # WASM 可编译性验证
scripts/gen_corpus/run.sh validate testdata/corpus   # 语料索引校验
scripts/l3/run_client.sh ppt <src> <dst>             # PowerPoint/WPS 真机打开+重存
scripts/check_wasm.sh                  # 构建浏览器端检查工具（产物在 wasm/site/）
```

## 许可证

[Apache-2.0](LICENSE) —— 自由使用、修改与分发；无运行时/许可成本。
