# go-pptx 代码实现评审（安全性 / 稳定性 / 易用性 / 性能）

> 评审对象：`github.com/F31/go-pptx/v2`，HEAD `e8229a2`（tag `v2.0.0` 之后 3 个 commit），工作区干净
> 评审日期：2026-09-17｜评审方式：全仓静态审查 + 本机实跑（构建/vet/gofmt/测试/基准/外部消费者实验）
> 结论性质：**不改变任何代码**，仅给出评审意见与优先级

## 0. 评审口径与证据分级

| 标记 | 含义 |
|---|---|
| 【实测】 | 我在本机（go1.27.0 windows/amd64）亲自复现或亲自跑出的数字 |
| 【代码核实】 | 逐行读源码确认的机制（附 `文件:行`） |
| 【代理实测】 | 并行派出的专项审查给出的实验数据；关键数字我已抽样复核（见各条注） |
| 【推断】 | 未实测，仅有代码依据 |

本轮先跑的基线（全绿，无噪声）：

```
go build ./...            → 通过
go vet ./...              → 干净
gofmt -l .                → 空
go test ./...             → 28 包 ok（含 -tags=corpus 再跑一遍同样 28 包 ok）
覆盖率：pptx 84.1% ｜ internal/opc 90.3% ｜ internal/xmlstore 91.2%     【实测】
规模：292 个 .go / 77,907 行 / 测试占比 43.9% ｜ fuzz 目标 8 个          【实测】
```

**总体印象**：这是一个**工程纪律明显高于平均水平**的库——分层清晰（ADR-030 门面 + internal）、保真策略（字节级 patch 而非重建 XML）自洽、金样/API 冻结/fuzz 齐全、原子保存与失败清理正确。问题**不集中在"会不会写坏文件"**，而集中在三类：**(1) 不可信输入的内存放大**（可被单文件打挂进程）、**(2) v2.0.0 破坏性变更的迁移说明与实际不符**（用户会被静默误导）、**(3) 公共 API 里混入了库外无法引用的 internal 类型**（承诺的旋钮实际拧不动）。性能上存在一个**低成本高收益**的缺陷（遍历路径的环路不变重复解析）。

---

## 1. 安全性

### 1.1 高危：`c:pt/@idx` 无界分配 → panic / OOM（不可信文件可打挂进程）

【代码核实 + 代理实测】`internal/chart/parse.go:400-421`

```go
if s, ok := cn.Attr("", "idx"); ok {
    if v, err := strconv.Atoi(s); err == nil { idx = v }   // 无上界校验
}
...
max := 0
for _, p := range pts { if p.idx > max { max = p.idx } }
out := make([]string, max+1)     // ← max 由文件内容决定
```

`@idx` 来自图表 Part 的 XML（**攻击者完全可控**），`Atoi` 在 64 位平台上接受至 `2^63-1`：

- `idx = 2^63-1` → `max+1` 溢出为负 → 运行时 `panic: makeslice: len out of range`；
- `idx = 1e9` → 单个 `[]string` 元素 16 字节 → **约 16 GB 分配**。

可达性确认：这是**公共 API** 路径——`pptx.OpenReader` → `Slides()[0].Shapes()` → `ChartShape.Data()`（`pptx/chart.go:347` → `parse.go:324`）。库内无 `recover()`，因此调用方进程/worker 直接崩。输入体积无关（图表 XML 受 `MaxXMLBytes` 32 MiB 限制，但放大倍数是 500×+）。【代理实测：153 B 输入 → 15.2 GB 分配；`idx=2^63-1` panic 已复现】

**修法（S）**：`idx` 视作稀疏键而非数组下标——`if idx < 0 || idx > len(pts)-1 { 记诊断并跳过 }`，或改为按出现顺序 `append` 后由 `idx` 排序回填（保持语义不变，零新增分配上界）。

### 1.2 高危：深层组合形状的路径串导致 O(深度²) 分配放大

【代码核实 + 代理实测】`internal/ooxml/shapes.go:57-78`（`walkGroup` 及其 4 个兄弟分支）

```go
func walkGroup(g *schema.P_CT_GroupShape, path string, out *[]ShapeInfo) {
    ...
    walkGroup(&grp, fmt.Sprintf("%s/p:grpSp[%d]", path, iGrp), out)   // 每层复制整条路径
    ...
}
```

路径随层数线性增长，**每下一层都把整条路径重新格式化一遍**，总代价 Θ(depth²) 字节。`encoding/xml` 的深度上限是 10000，因此 ~190 KB 的 slide XML 可达 1e4 层 → **数百 MiB 瞬时分配**。【代理实测：188 KB 输入 → 896 MiB / 400 ms，经 `internal/engine/irbuild.go:191` 在 Inspect 路径可达】

**修法（S/M）**：路径不要以字符串形态贯穿递归——改为传 `[]int` 下标栈（或仅传父指针），**只在真正需要报错/输出时**才惰性拼接一次；或对深度设硬上限（见 1.6，`MaxXMLDepth` 现为死字段）。

### 1.3 中危：预算只校验"声明值"，无压缩比/无实际读取总量上限

【代码核实】`internal/opc/zipindex.go:83` 依据 ZIP 头部**声明**的尺寸做总量校验；`countedReadCloser`（`zipindex.go:189`）只按**单个 Part** 计数。攻击者声明小尺寸、实际给出大膨胀 → 真实总量 = 条目数 × 单 Part 上限（10000 × 512 MiB 理论边界），**声明总量 1 GiB 形同虚设**。

**修法（M）**：加**全局实际读取字节计数**（跨 Part 累计，超限即中断），或加**单条目压缩比上限**（如 > 200:1 拒绝并记诊断）。

### 1.4 中危：`MaxEntries` 校验晚于索引构建

【代码核实】`zipindex.go:41` 先 `zip.NewReader`（构建全部条目表）→ `:45` 才校验条目数。2 万条目包在**被拒绝之前**已产生数倍于文件的分配。【代理实测：2.2 MB / 2 万条 → 5 MB 分配】

**修法（S）**：先读 EOCD 拿条目数做快速拒绝，再 `zip.NewReader`。

### 1.5 中危：MP4 `ftyp` brand 列表放大 21×

【代码核实 + 代理实测】`internal/videoprobe/mp4.go:59-63`——对 `ftyp` box 内每 4 字节 `append` 一个 `string`（每个 string 至少 16 字节头）。8 MiB box → 172.7 MiB；在 512 MiB 媒体暂存上限下最坏 ~11 GiB。【代理实测 21.6×】

**修法（S）**：brand 数量上限（如 64 个，超限截断并记诊断）——语义上 brand 列表本就只需判"是否含已知 brand"。

### 1.6 中危：`Budget.MaxXMLDepth` 是死字段（安全旋钮拧不动）

【实测/代码核实】全仓 `MaxXMLDepth` 仅出现在 `internal/opc/budget.go:17,26,36,55` 与 `package_test.go`（定义、归一化、断言），**无任何解析器消费**：

```
$ grep -rn "MaxXMLDepth" --include=*.go .
internal/opc/budget.go:17,18,26,36,55
internal/opc/package_test.go:417,427,429,430        ← 只有定义与测试
```

后果有两层：① 调用方设 `MaxXMLDepth: 8` 会**以为深度受控**（实际不限）；② 深度限制真实存在两条不一致路径——自研扫描器 256、`encoding/xml` 10000（`internal/ooxml/schema/decode.go:14`），文档未说明。

**修法（S）**：要么接线（把值注入两类解析器），要么**删字段并在 README 写明实际限制**。这类"声明了但不起作用"的旋钮，与该库自己在 A22 类问题上总结的教训（不做假能力）冲突。

### 1.7 低危与提醒

| # | 项 | 证据 | 备注 |
|---|---|---|---|
| a | 输出文件权限 0600（`CreateTemp` 保留） | `internal/opc/save.go:86` | 与常见 0644 预期不符；如需 0644 显式 `Chmod` |
| b | WASM 错误文本原样回传含攻击者可控名 | `wasm/check/check.go:190` | 仓库自带 UI 已 `escapeHtml`/`textContent`；第三方消费方勿用 `innerHTML` |
| c | WAV `ds64` 的 `int64(totalDataBytes)` 可溢出为负 | `internal/audioprobe/wav.go:140` | 仅影响诊断字段数值，不影响切片边界 |
| d | README 无安全/预算章节；默认 1 GiB 声明总量 / 512 MiB 单 Part 偏宽 | `internal/opc/budget.go:21-27` | 建议给出"面向服务端的推荐收紧值" |

### 1.8 已到位的防护（勿误判，逐项核实过）

- **路径穿越**：`partname.go:48` 拒绝 `..`/`.`/`\`/绝对路径/空段；`relationships.go:176` 拒绝越根；**全仓不存在"按 ZIP 条目名落盘"的代码**（唯一写盘点是 `save.go`，路径由调用方给出）。
- **原子保存**：`save.go:77` `Lstat` → 同目录 `CreateTemp` → 校验 → `rename`，各失败分支均清理，不跟随 symlink。【代理实测：目标已存在 / 目标是目录两种失败后**无临时文件残留**】
- **XXE / billion laughs 不可达**：自研扫描器（`internal/xmlstore/scanner.go:463`）仅识别预定义与数字实体，未知实体原样保留；`encoding/xml` 仅在 `internal/ooxml/schema/decode.go:14`，无 `Entity`/`CharsetReader` 注入。
- **库路径无显式 `panic(`**：48 处全部在 `scripts/`（工具脚本）；生产包 0 处。（但**运行时 panic 仍可达**，见 1.1、2.1。）
- **媒体探测有边界纪律**：WAV 有 `bodyEnd`/下溢检查（`wav.go:60-131`）、MP3 有前进保证与 Xing/VBRI 边界（`mp3.go:104-149`）、MP4 有长度检查（`mp4.go:27-58`）；图片只 `DecodeConfig` 不全量解码（`media.go:181,190`）。
- **并发假设自洽**：生产代码无 `Mutex`，包级 `var` 皆为只读表/哨兵错误，唯一 `init()` 只写静态字段（`media_audioicon.go:25`）——"单实例串行"的声明没有隐藏共享可变状态来打脸它。
- **fuzz 覆盖**：8 个目标（`internal/opc` ×2、`internal/xmlstore` ×2、`pptx` ×4），与 `fuzz.yml` 接线。

---

## 2. 稳定性

### 2.1 `Save(nil, ...)` 直接 panic，而 `Write(nil, ...)` 容忍——同族 API 行为不一致

【实测】我用外部模块跑出的原始输出：

```
[A] Save(nil, ...) PANIC: runtime error: invalid memory address or nil pointer dereference
[B] Write(nil, ...) returned err = <nil>
[C] Save(ctx, ...) err = <nil>
[D] first Close err = <nil> ; second Close err = pptx: document closed
```

【代码核实】`pptx/save.go:62` 直接 `ctx.Err()`（未判 nil），`pptx/save.go:101-103` 则显式归一化 `ctx == nil → context.Background()`；`Validate` 亦容忍 nil。同一门面内三种态度。

**修法（S）**：三处统一 nil 归一化（或统一拒绝），并在 godoc 写明。

### 2.2 `Close()` 非幂等，破坏 `defer p.Close()` 的安全网

【实测】第二次 `Close()` 返回 `pptx: document closed`（`pptx/save.go:34-37`）。惯用法 `defer func(){ _ = p.Close() }()` 没问题，但"显式 Close + defer Close"或公共清理函数里连调两次就会拿到伪错误。`io.Closer` 对重复调用未定义，但从库的可用性看，**幂等返回 nil** 更友好。

### 2.3 Tier 2 raw 直通在生产输入上命中率 ≈ 0（能力声明与实现不符）

【代码核实 + 代理实测】`internal/opc/saveplan.go:372` 拒绝 `flagDataDescriptor`（bit 3）；而 Go `zip.Writer` 在流式写入时**恒置 bit 3**——库自身的 `Save` 与语料生成器都如此。于是"raw 直通"对库自产文件也不生效。【代理实测：8 份真实语料 0/118 Part 命中；库自产文件二次 Load 亦 `raw=false`（flags=0x8）】

这不是数据错误（会安全回退到流式复制），但 **ADR-018 的"媒体重存 2–3× 加速"承诺在真实输入上不成立**，属"已交付但默认不生效"的能力。修法（M）：要么支持 data-descriptor 场景（可从中央目录取尺寸），要么在 ADR/README 明说适用条件。

### 2.4 稳定性亮点（值得写进 release notes 的部分）

- **字节确定性**：Open→Write 连续两次（含跨秒）**sha 相同**；仅当"库代管 Modified"被置位时跨秒不同（设计行为，但会破坏可复现构建，建议文档提示用 `SetCoreProperties` 显式固定）。【代理实测，代码依据 `pptx/docProps.go:883`】
- **大文件保存不驻留内存**：134 MB（2 个 Store Part）保存峰值堆 **1.17 MB**、总分配 1.17 MB、51 ms——流式复制成立。【代理实测；我复核了 `save.go` 的流式实现】
- **无 goroutine 泄漏面**：生产代码 0 个 `go func`；500 次 Open/Close 无异常（93.9 ms）。【代理实测】
- 失败分支清理正确（见 1.8）、`verifyOutput` 只读中央目录而非二次全量解压（`save.go:158`）。
- 代码里 149 处 `_ =` 忽略赋值中，**我未发现真正吞掉"写盘/Close/解压"错误的**关键点（抽查 `save.go`、`media*.go`）；`save.go:100` 忽略 `tmp.Close()` 错误属低危。

---

## 3. 易用性 / API 设计

这一节问题最密集，且**两条属"用户会因此踩坑"的阻断级**。

### 3.1 阻断：公共签名里混入 `internal/` 类型 → 库外**无法引用**

【实测】外部模块编译结果：

```
internalprobe\main.go:11:2: use of internal package github.com/F31/go-pptx/v2/internal/opc not allowed
```

【代码核实】门面把这些 **internal 类型写进了导出签名**：

| 位置 | 导出面 |
|---|---|
| `pptx/options.go:26` | `func WithNewBudget(b opc.Budget) NewOption` |
| `pptx/options.go:53` | `func WithBudget(b opc.Budget) OpenOption` |
| `pptx/options.go:73` | `func WithSaveDurability(d opc.Durability) SaveOption` |
| `pptx/bridge.go:13` | `func PartBytes(p *Presentation, name opc.PartName) ([]byte, bool)` |
| `pptx/layout_report.go:68,77-80,86,106` | `MasterPart`/`RegularTargetPart`/`Part`/`Parts` 等字段 |
| `pptx/media_audio.go:632,634` | `AudioProfile.MediaPart`/`SlidePart` |
| `pptx/media_video.go:705-707` | `VideoProfile.MediaPart`/`PosterPart`/`SlidePart` |

后果：**资源预算根本无法从库外配置**（调用方连参数类型都写不出来，也无法调用 `opc.DefaultBudget()` 取默认值）；`AudioProfile.MediaPart` 这类公开字段对消费者不可读。而 godoc 的措辞（"覆盖打开时的资源预算（默认 DefaultBudget）"）让人以为可以配置。

**这不是 v2 回归**——`git show v1.0.7:options.go` 有同样三行，说明**从 v1 起就存在**；但 v2.0.0 是"唯一一次破坏性发布"，正是修它的最佳窗口，错过了就要等 v3。

**修法（S，高价值）**：在 `pptx` 包内 `type Budget = opc.Budget`（type alias，零成本、零重复）/ `type Durability = opc.Durability` / `type PartName = opc.PartName`，导出签名改用别名；同时给 `api_surface_test.go` 或 `internal/archlint` 加一条规则：**导出签名不得引用 `internal/` 包类型**（现有 archlint 只查导入方向，天然看不见这个问题）。

### 3.2 阻断：迁移指南的"起点"是错的 → sed 静默不生效

【实测/代码核实】`docs/RELEASE-NOTES-v2.0.0.md:28-37` 声称 v1.0.x 的模块是 `github.com/F31/go-pptx/v2`。**实际**：

```
$ git show v1.0.7:go.mod
module github.com/F31/go-pptx          ← 没有 /v2
```

于是表格三行与那段 `sed 's|"github.com/F31/go-pptx/v2"|...|g'` **对任何 v1 用户都匹配不到**——用户跑完以为迁移完成，实际一行没改；下次编译报的错会指向"缺包"，而文档已让人相信"变更仅一处"。正确替换目标应是 `"github.com/F31/go-pptx"` → `"github.com/F31/go-pptx/v2/pptx"`。

### 3.3 高：迁移指南漏报第二个破坏性变更——公共包 `ir/` 在 v2 消失

【实测/代码核实】

```
$ git ls-tree --name-only v1.0.7 | grep '^ir$'   → ir            ← v1 顶层有公共 ir/ 包
$ git show v1.0.7:ir/ir.go | grep -cE '^func |^type '  → 26
$ grep -rn "internal/ir" pptx/*.go                → （无匹配）    ← 门面不再暴露它
```

v2 把 `ir` 收编为 `internal/ir`（`RELEASE-NOTES-v2.0.0.md:73` 列为已闭环步骤），但 Migration Guide 仍写"**变更仅一处：import 路径**"、"**无 API 签名变化**"。实际后果：v1 里可用的 `ir.Diff` / `ir.Page` / `ir.Options` 等**整包对库消费者不可达**（只剩 CLI/WASM 路径），而 `README.md:98` 仍把 IR 列为库能力。

**为什么会漏**：金样只统计**门面包**的导出符号（`pptx/api_surface_test.go:34` 的 163），"整个包被移入 internal" 这类变更不在计数维度里 → **不变量表（40 段/163 类型/131 方法）给出"未变"的假安全感**。建议：不变量表增加一行"公共包清单"（`go list` 出所有非 internal 包并锁定）。

### 3.4 高：Stable godoc 承诺了不存在的方法

【代码核实】`pptx/media_picture.go:167` 列举 `SetPictureFit / ReplacePicture / PictureFit / PictureSource`——全仓无定义（真实方法名是 `ReplaceImage`）。这类"文档承诺 → 用户编译失败"的组合最消耗信任，且它是 **Stable 段**（受 ADR-015 契约保护）。

### 3.5 中：CLI 帮助与退出码不符合 POSIX/Go 惯例

【实测】

```
$ pptx-cli inspect --help >/dev/null ; echo $?     → 2      （且 stdout 输出 0 行 → 帮助只写到 stderr）
$ pptx-cli --help         >/dev/null ; echo $?     → 0      （顶层与子命令不一致）
```

`--help` 走 stderr + 退出码 2，会让 `pptx inspect --help | less`、`man` 风格管道、以及 CI 里的 `--help` 探测失败；还额外打印 `flag: help requested`。修法（S）：在 `cmd/pptx` 统一识别 `flag.ErrHelp` → 输出到 stdout、退出 0（`cmd/pptx/main.go:64` 的顶层已经这么做了，子命令补齐即可）。

### 3.6 中：退出码语义冲突 + 覆盖保护的 TOCTOU

【代码核实】`README.md:264` 定义退出码 4 = 资源超限，而 `cmd/pptx/common.go` 把"输出已存在"也映射成 4；同时 CLI 先 `Stat` 判存在、再**无条件** `WithSaveOverwrite(true)`（`common.go:112-118`），**绕开了库自身的 `ErrOutputExists` 守卫**（先判后写之间存在竞态）。

**修法（S）**：仅在用户显式给 `--overwrite` 时传 true，让库做存在性权威判定；退出码为"已存在"另分配一个（或与 `ErrInvalidArgument` 同码并在文档写明）。

### 3.7 中：读侧能力缺口与文档质量

| 项 | 证据 | 影响 |
|---|---|---|
| `PictureShape` 无读侧接口（无 `PictureSource()`），而 Audio/Video 都有 `AudioSource`/`VideoSource` | `pptx/media_picture.go`、`media_audio.go`、`media_video.go` | "批量导出图片"没有一步 API，只能退到 `PartBytes`（且它本身受 3.1 影响） |
| `docs/api-reference.md` 不列结构体字段（163 个 Spec/Report 只给 `Kind: struct`）；生成器把泛型渲染成 `*ast.IndexExpr`、`FuncMedia` 截断为 `func(...)` | `docs/api-reference.md:1156,1163`（生成器 `scripts/gen/apidoc`） | 字段名/类型是调用方最需要的信息，缺失等于参考手册只到一半 |
| README 数字陈旧：三处写 **158 types**，金样是 **163** | `README.md:166,211,298` vs `pptx/api_surface_test.go:34` | 文档与自证数字打架 |
| `pptx/doc.go` 仍称"module 根包"、"六子命令"（现 9 个）、把已升 Stable 的能力列为 Experimental | `pptx/doc.go:1,32,170-173` | 新用户第一印象即过时 |
| `SDKVersion` 注入路径随 v2 失效 → `pptx -v` 永远显示 `dev`（静默不报错） | `pptx/capability.go:210` | 版本溯源不可用；建议加 CI 断言"release 构建必须有版本" |
| 文档断链/缺号：`docs/v1.0-freeze-list.md` 已删但被引用、ADR-022 缺号却称 ADR-014–030、`wasm/site/wasm_exec.js` 缺失（README 声称双击即用）、`architecture-current.md:132-153` 仍列 `.../v2/ir` | 见各文件 | 需一轮链接巡检 |
| `rejectOutputFlag` 为死代码 | `cmd/pptx/common.go:126-132` | 清理项 |

### 3.8 已到位（正面确认）

- **错误契约扎实**：17 个哨兵、公共边界统一 `OperationError` + `%w`，`errors.Is` 可判定；且有 `TestErrorSentinelsFrozen` 锁字符串（我复跑该测试通过）。
- **`docs/api-reference.md` 与生成器输出逐字节一致**（不陈旧），只是渲染维度不够。
- **README quickstart 可编译可运行**（代理在外部临时模块实测产出 out.pptx）。
- **门面收敛得当**：`archlint` R1–R5 强制方向（internal 不得依赖门面/工具层、leaf 包只依赖 std-lib），`go list` 门禁防止新包漏登记。

---

## 4. 性能

### 4.1 严重：`Slides()` 每页重复"读 + 解析主关系流"（环路不变、O(N) 冗余）

【代码核实】`pptx/presentation.go:221` 在 `sldId` 循环**内部**调用 `p.relsOf(p.main)`，而 `pptx/style_env.go:34-48` 的 `relsOf` **无任何缓存**：每次都 `partBytes`（`io.ReadAll` 整个关系流）+ `opc.ParseRelationships`（解析全部关系）。N 页 = N 次同样工作。

【实测（我本机）】

```
BenchmarkPerfTraverse/100p-media   16.40 ms/op   26,774,056 B/op   223,284 allocs/op   （输入 35.5 MB）
BenchmarkPerfTraverse/50p-image     5.91 ms/op    7,766,432 B/op    65,476 allocs/op
```

`26.8 MB / 223k 次分配` 用于一次 100 页遍历——**分配量已达输入的 0.75×**。【代理实测：把该调用提到循环外（副本实验）→ `8.98 ms → 0.179 ms`、`22.4 MB → 231 KB`、`189k → 2k allocs`；连带 `PerfTraverse/100p-media` 19.6→6.7 ms、`PerfPeakHeap` 29.4→7.4 MB】（我确认了"当前值"与我自测同量级，故该量级可信；50p-image 的 7.77 MB/65,476 allocs 与代理数字**完全一致**）

**修法（S）**：循环外取一次，或按 revision 缓存（`p.rev` 失效，与 `partDocs` 同策略）。这是**一行级改动换来 50× 遍历提速**——本轮性价比最高的一项。

### 4.2 严重：性能看守链路已断（脚本 + 基线文档双失）

【实测】

```
$ grep -n "go test" scripts/perf/run.sh        → :52,:54  … 目标包是 "."
$ grep -n "go test" scripts/perf/smoke.sh      → :37      … 目标包是 "./"
$ go test . -run '^$' -bench BenchmarkPerf
# .
no Go files in E:\projects\go-pptx
FAIL	. [setup failed]                      ← 复现成功
```

ADR-029 把根包搬进 `pptx/` 后，**性能脚本仍指向模块根**（已无 Go 文件）→ `No Go files` 直接失败；而 `.github/workflows/ci.yml:131`（perf-smoke）与 `perf.yml:38`（夜间）都在调它们 → **CI 上两个性能任务长期红**，且 `docs/PERF-01-性能基线.md` 已随 5e34c1c 删除 → **基线数据无留档**（ADR-018 引用的 p50/p95 表在仓内已无处可查）。

**修法（S）**：脚本目标 `.` → `./pptx/`；重建一份基线文档（或在 ADR 中固化当前数字）。

### 4.3 高：`AddPicture` 去重是 O(N²) 且全量读媒体

【代码核实 + 代理实测】`pptx/media_picture.go:600` `findExistingMedia`：对**每个已存在媒体**执行 `partBytes` + SHA-256 → 第 N 次 `AddPicture` 要重读并哈希 N 个媒体。构建 100p-media 语料（120 次 AddPicture）仅在 `partBytes` 上就分配 **1.85 GB**。【推断：真实批量插图场景（比如 1000 张图）代价平方增长】

**修法（M）**：缓存 `part → SHA-256`（按 revision 失效，与 `partDocs` 同策略）；或先按 ContentType + 尺寸预筛再比对字节。

### 4.4 中：遍历的分配系数偏高（126× 输入）

【实测/代理实测】`50p-image`（61 KB 输入）单次遍历 **7.77 MB / 65,476 allocs = 输入的 126×**，主要来自 `internal/xmlstore` 的 `IndexWith` → `readStartTag`（每节点 `string(rawName)` + Attrs 切片分配）。同属"遍历热路径"，可与 4.1 一并优化。

### 4.5 中：媒体输入整块驻留（与保存侧形成反差）

【代码核实 + 代理实测】`internal/document/media/media.go:130` 用 `bytes.Buffer` 倍增累积（上限 512 MiB）——`AddPicture`/`AddAudio`/`AddVideo` 的输入必须整体在内存。对比：**保存侧已完全流式**（134 MB 文件峰值堆 1.17 MB）。大媒体入库仍是内存瓶颈。

### 4.6 低

- Tier 2 raw 分支的 `io.Copy` 未复用缓冲（`saveplan.go:393`）——当前该分支基本不可达（见 2.3），修复 2.3 时一并处理。
- 稀疏分配 `make([]string, max+1)`（1.1）本身也是分配问题，一处修法同时消 2 个维度。

### 4.7 与文档声称不符的性能结论（需要更正）

1. **ADR-018 "媒体重存 Save 整体 2–3× 加速"**：代理复跑得 Stream→Raw `1.88×/1.67×`（share 47%/40%，文档记 65.3–73.4%/48–66.2%），且 Raw 侧 B/op 3.47 MB vs Stream 140 KB；更关键的是探测基准的 Raw 侧**绕过了全部安全门限**、源包又由 Go zip.Writer 生成，故**生产代码在该输入上 100% 回退**（2.3）。结论：ADR-018 的收益数字不可作为生产预期。
2. **Tier 1（流式复制）反而优于文档**：代理实测 200×4 KiB **239,593 B/op**（文档"复用后 231 KB"）、PeakHeap **2.02 MB**（文档 6.08 MB）。

---

## 5. 修复优先级建议

| 优先级 | 项 | 维度 | 规模 | 理由 |
|---|---|---|---|---|
| **P0** | 1.1 图表 `@idx` 无界分配 | 安全 | S | 唯一"单文件打挂进程"的确定路径，且是公共 API |
| **P0** | 3.2 迁移指南 module 路径错误 | 易用 | S | 现役 v1 用户按文档迁移会**静默失败**；改 2 行文字 |
| **P0** | 3.3 迁移指南漏报 `ir` 包消失 | 易用 | S | 同上，且是"文档说没变、其实变了"的信任问题 |
| **P0** | 4.2 perf 脚本/基线失效 | 性能 | S | 性能看守链条断了，后续所有性能改动无人兜底 |
| **P1** | 3.1 internal 类型外露（type alias 修复） | 易用 | S | 承诺的预算旋钮当前不可用；v2 是最后的低成本窗口 |
| **P1** | 4.1 `Slides()` 重复解析 | 性能 | S | 一行级改动 → 遍历约 50× 提速、内存减半 |
| **P1** | 1.2 路径串 O(深度²) | 安全 | S/M | 第二个内存放大面，Inspect 路径可达 |
| **P1** | 2.1 `Save(nil)` panic + 2.2 `Close` 非幂等 | 稳定 | S | 同族 API 一致性，改动小、可见度高 |
| **P1** | 1.6 `MaxXMLDepth` 死字段 | 安全 | S | "假旋钮"与其自身质量公约冲突，删或接线都行 |
| **P2** | 1.3 无压缩比/实际总量上限；1.4 EOCD 预检；1.5 brand 上限 | 安全 | M/S/S | 夯实"不可信输入"防线 |
| **P2** | 2.3 Tier 2 raw 命中率≈0（能力声明与实现不符） | 稳定/性能 | M | 要么实现要么明确适用范围 |
| **P2** | 4.3 `AddPicture` O(N²)；4.5 媒体驻留 | 性能 | M | 批量插图场景的真实瓶颈 |
| **P2** | 3.4/3.5/3.6/3.7 文档与 CLI 一致性（含重生成 api-reference 字段） | 易用 | S/M | 一轮集中清扫，避免"文档说错"累积 |
| **P3** | 1.7 权限/错误文本/ds64；4.4 分配系数；4.6；3.7 断链 | 各 | S/M | 常规清理 |

**建议的落地顺序**：P0（四条，均 ≤ 半天）→ P1（一轮 patch）→ 之后按 P2 排期。**P0/P1 全部改动都不触碰文件格式行为**，因此不涉及金样重打（除 1.1 的图表读侧语义需补一条"畸形 idx"测试）。

---

## 6. 值得保留与延续的机制（评审的正向结论）

1. **保真优先的编辑模型**（字节 patch + 未识别内容原样保留）是这个库最核心的竞争力，测试（43.9% 占比）与金样在支撑它。
2. **金样 + 冻结测试 + fuzz + archlint 四件套**已经能在"符号/方向/崩溃"三层自动兜底；本轮暴露的缺口恰好是它们**结构上看不见的三类**：内存放大（无上限断言）、整包外移（计数维度不含包清单）、公共签名引用 internal 类型（ast 金样只数名字、不看类型来源）。建议有针对性地各加一条断言。
3. **原子保存 + 失败清理 + 确定性输出**已达标，且实测过边界（目标已存在/目标是目录/连续 500 次 Open/Close）。
4. **保存侧流式复制优于文档预期**，说明 ADR-018 的工程实现质量是够的——问题在"能否命中"，不在"写的对不对"。
5. 文档体系（ADR 分阶段 + release notes + 白皮书）密度很高；**本轮问题集中在"文档与代码的同步滞后"**，而非"缺少文档"。建议把"文档断言"也纳入 CI（例如：README 里的数字由生成器注入、迁移指南的 sed 断言在临时模块里真的跑一遍）。

---

## 7. 本轮未验证 / 局限

- 代理报告的 `-race` 结果**未能跑通**（本机无 gcc，CGO 构建失败）；并发结论仅基于静态（无 Mutex、无共享可变全局态）。
- **无 50–200 MB 级真实 PPTX** 可测：仓内最大真实语料仅 ~14 KB，大输入用合成包替代；媒体驻留/大目录的实际曲线未实测。
- **未做真机客户端矩阵复验**（PowerPoint/WPS 打开 v2.0.0 产物），本轮只审实现，不重复 L3 验证。
- 1.2、4.3 的量化来自代理；我已复核代码机制与 1.2 的代价阶（Θ(深度²)），但未独立复现 896 MiB 的具体数值。
- 覆盖率与语料测试为本地口径（windows/amd64），未跨平台复跑；`v2.0.0` tag 是否已推远端未核。

---

## 附：复现命令

```bash
# 基线
GOPROXY=https://goproxy.cn,direct GOSUMDB=off go build ./... && go vet ./... && gofmt -l .
GOPROXY=https://goproxy.cn,direct GOSUMDB=off go test ./... && go test -tags=corpus ./...
GOPROXY=https://goproxy.cn,direct GOSUMDB=off go test ./pptx ./internal/opc ./internal/xmlstore -cover

# 性能（当前值）
GOPROXY=https://goproxy.cn,direct GOSUMDB=off \
  go test ./pptx -run '^$' -bench 'BenchmarkPerfOpen|BenchmarkPerfTraverse' -benchtime=1x -benchmem

# 性能脚本失效复现
GOPROXY=https://goproxy.cn,direct GOSUMDB=off go test . -run '^$' -bench 'BenchmarkPerf'

# nil ctx / Close 幂等（外部模块，验证库外可见行为）
go run .        # 见 2.1 输出；探针源码见评审会话

# internal 类型不可用（修复前）
go build ./internalprobe    # use of internal package ... not allowed
```

---

## 附二：修复轮次记录（2026-09-18）

本节记录本轮把上述发现落地为代码时的实际情况，**供后续读者判断哪些结论已经过时**。
变更明细见 `CHANGELOG.md` 的 `[Unreleased] - 2026-09-18`。

**已修（共 8 个文件涉及的核心改动 + 5 个新增测试文件）**

| 评审项 | 落地位置 | 验证方式 |
|---|---|---|
| 图表 `c:pt/@idx` 无界分配 | `internal/chart/parse.go` | 新增 `cache_bounds_test.go`（溢出/负数/超大 idx/缺号语义） |
| MP4 brand 21× 放大 | `internal/videoprobe/mp4.go` | 新增 `brand_bounds_test.go` |
| `MaxXMLDepth` 假旋钮 | `internal/opc/budget.go`（删除字段） + 注释写明真实深度策略 | `opc` 包测试同步更新 |
| `Slides()` 每页重复解析 | `pptx/presentation.go`（提环外 + revision 缓存） | bench 实测见下 |
| `AddPicture` 去重 O(N²) | `pptx/media_picture.go` + part→hash 缓存 | 全仓回归 |
| perf 脚本目标包失效 | `scripts/perf/{run,smoke}.sh` → `./pptx/` | 实跑 `smoke.sh` 通过 |
| v2 迁移指南事实错误 | `docs/RELEASE-NOTES-v2.0.0.md` | 与 `git show v1.0.7:go.mod` 逐项核对 |
| 导出签名引用 internal 类型 | 新增 `pptx/aliases.go`（3 别名 + `DefaultBudget` + 2 取值常量） | 外部模块 `go run` 实测通过 |
| `Save(nil)` panic / `Close` 非幂等 | `pptx/save.go` | 新增 `save_contract_test.go` |
| CLI 帮助/退出码/覆盖保护 TOCTOU | `cmd/pptx/common.go` + 9 个子命令 | 构建后实测 rc/stdout/stderr |
| 文档数字与 godoc 不符 | README(中/英)、`pptx/doc.go`、`pptx/capability.go`、`pptx/media_picture.go` | grep 核对 |

**性能实测（同一台机器，修复前 → 修复后）**

| 基准（`-benchtime=6x`） | 修复前 | 修复后 |
|---|---|---|
| `PerfTraverse/100p-media` | 16.40 ms / 26.77 MB / 223,284 allocs | **3.96 ms / 1.17 MB / 10,713 allocs** |

**对本评审报告的三处更正（修复过程中发现）**

1. **「`rejectOutputFlag` 是死代码」不成立。** 该函数在 `capability.go:30` 等 **4 个只读子命令**中正常调用，且有专项测试
   （`cli_subcommands_test.go:507`）。评审时只看到调用点少，误判为未被使用。**该条已撤回，未做改动。**
2. **金样冻结门禁不覆盖顶层导出函数。** `apiSurface`（`pptx/api_surface_test.go:165`）只有 type / Stable 段 /
   Stable 方法 / 哨兵四个维度，`loadAPISurface` 遍历时只处理 `*ast.GenDecl`——`func DefaultBudget()` 这类
   **顶层导出函数**新增或删除都不会让任何门禁失败。这是既有守卫的盲区，本轮未改动，建议单独建档。
3. **错误串泄露绝对路径。** 外部实测得到
   `pptx: output file exists: opc: output file exists: C:\Users\<user>\AppData\Local\Temp\ext...\out.pptx`。
   对 WASM / HTTP 服务场景属低危信息泄露；但 `goldenSentinelMessages` 锁的是哨兵自身字符串，
   这里泄露的是**包装层附加的**路径，改起来不需要动金样，属可独立处理的项。

**本轮未做（有意保留）**

- 大文件/超长函数拆分（需先接 golangci-lint 并配存量豁免，否则 diff 噪音过大）。
- `Open`/`OpenReader`/`New`/`Bind` 补 `context.Context` 入口——这是签名级变更，建议随下一个 minor 一并评审。
- Tier 2 raw 直通命中率 ≈0 的问题（该函数 reproduce 依赖 zip.Writer 的 data-descriptor，属功能缺失而非回归）。
