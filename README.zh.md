# go-pptx

[English](./README.md) | [中文](./README.zh.md)

纯 Go 编写的 PowerPoint（OOXML / PPTX）创建、编辑、审计 SDK。零外部运行时依赖。支持 Linux / Windows / macOS 与浏览器 WASM。

```text
产品名    go-pptx
许可证    Apache-2.0
Go        >= 1.24
模块路径  github.com/F31/go-pptx/v2
导入路径  github.com/F31/go-pptx/v2/pptx
```

[![CI](https://github.com/F31/go-pptx/actions/workflows/ci.yml/badge.svg)](https://github.com/F31/go-pptx/actions/workflows/ci.yml)

---

## 什么是 go-pptx？

go-pptx 面向"**程序化加工 PPTX**"这一在 Go 生态中长期空缺的场景：开发者希望像操作结构化文档一样——创建页面、替换文本、嵌入图片/图表/音频/视频、批量渲染模板、审计文档差异——而**不依赖** Office 客户端、LibreOffice 安装或任何解释器运行时。

与把文档"读出再整体重写"的实现不同，go-pptx 以 **OOXML 原始字节为不变量**：只对目标区域做精确补丁，未触碰的 Part 与未知扩展保持字节恒等（B1），从而把"程序加工"对文档的副作用降到最低——这是它区别于通用序列化式库的核心。

**v2.0 架构**（ADR-030）：代码库采用分层结构，公共门面（`pptx/`）+ 域逻辑（`internal/`）+ ECMA-376 标准生成的 schema 类型模型。详见[架构](#架构v20)。

---

## 安装

```bash
go get github.com/F31/go-pptx/v2/pptx
```

---

## 快速开始

### 创建与编辑

```go
package main

import (
	"context"
	"log"

	"github.com/F31/go-pptx/v2/pptx"
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
		X: 914400, Y: 914400, Width: 6096000, Height: 914400, // EMU 单位，1 英寸 = 914400
		Text: "Hello, go-pptx",
	})
	tf, _ := box.TextFrame()
	tf.ReplaceText("go-pptx", "World") // 跨 Run 保真替换，自动继承格式

	if _, err := p.Save(context.Background(), "out.pptx"); err != nil {
		log.Fatal(err)
	}
}
```

### 模板数据绑定

```go
// 内联占位符：{{name}} / {{#if}} 条件段落 / {{#each rows}} 表格行循环 / 图表数据
rep, _ := p.Bind(map[string]any{
	"name":  "Q3 营收",
	"rows":  []any{map[string]any{"k": "华东", "v": 120}},
})
_ = rep // BindReport{...}
```

> 完整 API 清单见包文档（`go doc github.com/F31/go-pptx/v2/pptx`）与 [API 参考](./docs/api-reference.md)。

---

## 核心能力

| 类别 | 能力 |
|---|---|
| **创建** | `New()` 生成最小合法模板；`AddSlide` / `AddTextBox` / `AddAutoShape` / `AddPicture` / `AddChart` / `AddAudio` / `AddVideo`；`MoveSlide` / `RemoveSlide` / `MoveShape` / `RemoveShape` |
| **文本编辑** | 段落 / Run 对象模型；跨 Run 字面替换（3 种格式策略，字素簇保护，br/字段/链接边界安全）；字体/颜色/字号 patch 与 Reset；文本框属性与 slidenum/datetime 字段 |
| **图片/媒体** | PNG/JPEG 嵌入与 4 种 Fit 模式（拉伸/Contain/Cover/原尺寸）；媒体内容哈希去重；MP3/WAV/MP4/WebM 纯 Go 探测（时长、签名、容器品牌）；音频播放树与计时计划 |
| **图表/表格** | 柱/折/饼三类图表 + 内嵌工作簿生成；趋势线/误差线/数据标签/轴扩展；富文本表格、单元格合并、表格样式解析与优先级矩阵 |
| **模板绑定** | 内联 `{{path}}`、条件段落、表格行循环、图表数据绑定；plan 阶段纯读校验 + 单事务原子提交，**失败零残留** |
| **只读审计** | 只读 IR（schemaVersion 化 JSON）、语义 `Diff`、`Validate`、`Capability` 六维能力 manifest |
| **保真机制** | SpanPatch 精确字节补丁、未知子树/扩展原样保留、revision 并发防护、原子落盘与失败恢复 |
| **浏览器端** | WASM 编译目标 + 离线静态检查页（文件不离开设备，无外网/无 CDN） |

---

## 为什么选择 go-pptx？

### 与主流开源产品对比

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

### 差异化要点

1. **唯一"字节级保真"的纯 Go 实现**：真实样本上只改一个 Run 时，编辑前/后其余 Part 字节恒等（B1），被改 Part 内未触碰区域差异收敛到 1 字节——适合对"文档不被工具重写破坏"敏感的存量文档加工。
2. **Go 生态中目前缺少同量级 PPTX SDK**：单二进制分发、与现有 Go 服务/CI 无缝集成、WASM 直接进浏览器。
3. **内置审计与能力协商**：`Diff`、`Validate`、`Capability` 让调用方在部署前就知道哪些能力可用、哪些受限，避免运行时才发现不支持。
4. **开源且可自持**：Apache-2.0，无运行时/许可成本，对比商业方案（Aspose.Slides）在批处理与自动化场景成本更低。

---

## 技术创新

### 1. 原始字节 + 命名空间环境双模型

自研 `internal/xmlstore` 词法扫描器同时保留每个元素的字节跨度与解析后的命名空间作用域；未知命名空间子树照常建节点、保留原始字节，从而支撑"只改一个 Run、其余字节不变"的保真目标。同时规避了 `encoding/xml` 在特定自闭合标签组合上的深度同步偏差。

### 2. 事务化编辑

所有业务写路径收敛为 `SinglePartPatch` / `MultiPartPlan`——先纯读校验（plan），再单事务应用（apply），任一失败零残留；revision 校验拒绝并发修改。行为与"数据库事务"对齐，而非"就地改写字符串"。

### 3. 字节级保真补丁引擎（B1）

SpanPatch 区间替换 + 锚定校验 + 冲突检测，同一变更集整体校验后一次性按偏移降序应用；未变 Part 原样复制，Content Types 与变更集同源再生成。

### 4. 能力自描述与安全边界

`CapabilityManifest` 六维状态（Inspect/Create/Edit/Preserve/Render/Play）逐工作包登记支持度与限制；WASM 检查页在浏览器本地完成，文件不出设备。

### 5. 语义 diff 的页面对齐

加权 LCS + 形状 ID 集合相似度，把"同一页的两次修订"与"删页+加页"区分开；顺序乱序残留用高阈值二次配对并标记移动。

### 6. 语料驱动的工程验证

36 份真实 PPTX 样本 + 3 份可再分发 LibreOffice 金样（含 actions 重放与字节级 B1 断言）；编辑后文件经 PowerPoint / WPS 真机打开无修复提示（L3 8/8）。

---

## 架构（v2.0）

```text
┌─────────────────────────────────────────────────────────┐
│                    公共门面层                             │
│               github.com/F31/go-pptx/v2/pptx              │
│  Presentation · Slide · Shape · TextFrame · TableShape  │
│  ChartShape · PictureShape · AudioShape · VideoShape    │
│  （166 类型 · 134 Stable 方法 · 17 哨兵）               │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                   域逻辑层（internal/）                   │
│                                                         │
│  document/geometry   几何/填充/效果解析                    │
│  document/style      样式解析链                           │
│  document/text       文本字段/正文/片段辅助                │
│  document/media      媒体输入契约 + 探测                   │
│  document/table      逻辑网格 + 单元格辅助                │
│  document/model      共享值类型（EMU, ID）                │
│                                                         │
│  chart               图表 XML 解析/构建/校验              │
│  bind                模板绑定内部实现                      │
│  ir                  只读中间表示 + 语义 diff              │
│  engine              编排层（CLI + WASM 共享）            │
│  archlint            CI 依赖方向校验                      │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                    格式层                                │
│                                                         │
│  ooxml             OOXML 只读投影                         │
│  ooxml/schema      基于 ECMA-376 XSD 生成的类型          │
│                    （8 文件 · 7,300+ 行）                 │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                    传输层                                 │
│                                                         │
│  opc               OPC 包加载、关系流、内容类型、原子保存   │
│  xmlstore          XML 扫描器、节点树、SpanPatch 引擎     │
└─────────────────────────────────────────────────────────┘
```

**依赖方向规则**：`internal/*` 不得反向 import 公共门面（临时例外：`internal/engine` 编排层）。由 `internal/archlint` 在 CI 中强制执行。

---

## 包结构

```text
pptx/                   公共 SDK 门面（166 类型，134 Stable 方法）
internal/
  opc/                  OPC 包加载、ZIP 索引、关系流、内容类型、原子保存
  xmlstore/             XML 扫描器、节点索引树、命名空间环境、SpanPatch 引擎
  ooxml/                OOXML 只读投影（形状、备注、时序、图表）
  ooxml/schema/         基于 ECMA-376 Transitional XSD 生成的类型（8 命名空间，7,300+ 行）
  document/
    model/              共享值类型：SlideID、ShapeID、EMU、Point、Rect
    geometry/           几何/填充/效果解析（prstGeom、custGrad、填充种类）
    style/              样式解析链（段落/字符/列表/主题、颜色变换）
    text/               文本字段解析、正文/片段辅助、字体解析
    media/              媒体输入契约（MediaSource、有界复制、图片探测）
    table/              逻辑网格、单元格辅助、合并跨度
  chart/                图表 XML 模型：解析/构建/规范化/校验/片段/工作簿
  bind/                 模板数据绑定内部实现（Member、AsSlice、Truthy）
  ir/                   只读中间表示 + 语义 diff
  engine/               编排层（CLI + WASM 共享，ProjectIR 适配器）
  archlint/             CI 依赖方向校验器（ADR-030 机制 4）
  editplan/             单 Part / 多 Part 编辑计划（StageAdd/Delete/Patch）
  textmap/              文本 rune 映射与 span 定位原语
  textutil/             共享 XML 文本工具
  audioprobe/           音频容器探测（MP3 帧解析、WAV 格式变体）
  videoprobe/           视频容器探测（MP4 moov/mvex、WebM doc type）
  diag/                 跨层诊断类型（Severity、Diagnostic）
  errs/                 稳定错误码与 OperationError
  ooxmlns/              命名空间 URI 常量
cmd/
  pptx/                 CLI 工具（9 个子命令）
  pptx_check/           WASM 浏览器检查入口
wasm/
  check/                浏览器端纯函数检查
  site/                 离线静态检查页
render/                 渲染适配器契约（暂无实现）
testdata/corpus/        样本索引与公开金样
docs/                   设计文档、ADR、覆盖率路线图、兼容矩阵
```

---

## CLI 命令

| 命令 | 说明 | 模式 |
|---|---|---|
| `inspect` | 读取并汇总文档结构（页面、媒体、备注、能力）输出 JSON | 只读 |
| `validate` | L0 结构校验，输出诊断；错误数 > 0 时退出码 3 | 只读 |
| `replace` | 跨形状体的字面文本替换（`--old`、`--new`、`--mode`、`--output`） | 写入 |
| `bind` | 用 JSON 数据源渲染模板（`--data`、`--output`、`--loose`） | 写入 |
| `diff` | 两个演示文稿的语义 diff（JSON，支持 `--ignore-geometry/whitespace/notes`） | 只读 |
| `capability` | 输出能力 manifest JSON（六维状态报告） | 只读 |
| `narrate` | 从 `tracks.json` 清单嵌入音频（`--manifest`、`--output`） | 写入 |
| `timing-plan` | 预览时序同步计划（页跳转、尾部填充、strict/skip） | 只读 |
| `export-ir` | 导出中间表示为 JSON（`--output` 或 stdout） | 写入 |

退出码：`0` 成功 · `1` 运行时错误 · `2` 用法错误 · `3` 能力/校验错误 · `4` 资源限制。

---

## WASM / 浏览器

go-pptx 可编译为 WebAssembly，用于浏览器端只读检查。文件始终留在用户设备上。

```go
// 通过 syscall/js 暴露三个纯函数
wasm/check.Inspect(ctx, pptxBytes, fileName)    // → JSON，含 IR 投影
wasm/check.Validate(ctx, pptxBytes, fileName)   // → JSON，含校验诊断
wasm/check.Capability(fileName)                  // → JSON，含能力 manifest
```

**构建**：
```bash
GOOS=js GOARCH=wasm go build -o wasm/site/check.wasm ./cmd/pptx_check
```

**静态站点**：`wasm/site/` 包含 `check.html`、`check.js`、`wasm_exec.js`。在现代浏览器中打开 `check.html` 即可使用，无需服务器。

---

## 质量与验证

| 指标 | 数值 |
|---|---|
| **覆盖率（全仓）** | 84.4% 加权总计 |
| **覆盖率（pptx 门面）** | 83.2%（门槛 82%） |
| **覆盖率（internal 包）** | 15 个包 ≥ 85%，9 个包 ≥ 90% |
| **CI 门槛** | `scripts/coverage/gate.sh` 强制执行各包门槛 |
| **语料** | 36 份真实 PPTX 样本 + 3 份公开 LibreOffice 金样 |
| **L3 兼容** | PowerPoint 16.0 + WPS 12.1：8/8 通过，无修复提示 |
| **API 表面** | 166 类型 · 42 Stable 段 · 134 方法 · 17 哨兵（golden 锁定） |
| **交叉构建** | `CGO_ENABLED=0`，Linux、`js/wasm`、`wasip1/wasm` |
| **静态分析** | `go vet` + `gofmt`（默认 + corpus 构建标签） |
| **依赖方向** | `internal/archlint` 在 CI 中强制 R1-R5 规则 |

---

## 文档索引

| 文档 | 说明 |
|---|---|
| [API 参考](./docs/api-reference.md) | 生成的英文 API 参考（类型、方法、常量） |
| [架构现状](docs/architecture-current.md) | 当前包布局与依赖方向 |
| [技术白皮书](docs/go-pptx-技术白皮书.md) | 技术白皮书 |
| [ADR 目录](docs/adr/) | 架构决策记录（ADR-014 到 ADR-030） |
| [发布说明](docs/) | 各版本发布说明（`RELEASE-NOTES-*.md`） |

---

## 开发命令

```bash
# 构建
go build ./...                                     # CGO_ENABLED=0（CI 强制）
go build ./pptx/                                   # 仅公共门面

# 测试
go test ./...                                      # 全部单元 + 金样测试
go test -tags=corpus ./...                         # 带 corpus 构建标签
go test -cover ./...                               # 带覆盖率报告

# 静态检查
gofmt -l .                                         # 检查格式
go vet ./...                                       # 静态分析

# 覆盖率门槛
bash scripts/coverage/gate.sh                      # 强制各包门槛
TOLERANCE=0.5 bash scripts/coverage/gate.sh        # 带容差

# WASM
GOOS=js GOARCH=wasm go build ./cmd/pptx_check     # 构建 WASM 二进制
bash scripts/check_wasm.sh                         # 构建浏览器检查工具

# 语料与 L3
bash scripts/gen_corpus/run.sh validate testdata/corpus   # 校验语料索引
bash scripts/l3/run_client.sh ppt <src> <dst>             # PowerPoint/WPS 真机测试
```

---

## 许可证

[Apache-2.0](LICENSE) —— 自由使用、修改与分发。无运行时/许可成本。
