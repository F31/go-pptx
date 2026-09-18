# Changelog

All notable changes to go-pptx will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to a [Semantic API Stability](docs/adr/ADR-015-api-stability-tiers.md) model
(`// Stable:` / `// Experimental:` godoc tags). The per-type assignment is maintained in
`docs/v1.0-freeze-list.md`.

## [Unreleased] - 2026-09-18

代码评审（安全性 / 稳定性 / 易用性 / 性能，见 `docs/code-review-2026-09-17.md`）后的一轮加固。
除标注 ⚠️ 的两条外均为**追加式或纯修正**，公共签名无移除、无改名。

### Added

- **`pptx.Budget` / `pptx.Durability` / `pptx.PartName` 类型别名**（新文件 `pptx/aliases.go`，Stable）；
  并配套导出 `pptx.DefaultBudget()` 与取值常量 `pptx.DurabilityDefault` / `pptx.DurabilityFull`。
  此前 `WithBudget` / `WithNewBudget` / `WithSaveDurability` / `PartBytes` 以及
  `AudioProfile.MediaPart`、`VideoProfile.MediaPart`、`LayoutReport.Part/Parts` 等导出字段
  **直接引用 `internal/opc` 的类型** —— Go 的 internal 规则使外部模块无法引用它们，
  这些"资源预算旋钮"承诺了却拧不动（外部模块实测编译报 `use of internal package`）。
  别名与原类型同一类型，零转换成本、零语义漂移。
- `pptx.PartName.String()` / `.Valid()` / `.EntryName()` 随之进入 Stable 方法面。
- 回归测试：`internal/chart/cache_bounds_test.go`、`internal/videoprobe/brand_bounds_test.go`、
  `pptx/save_contract_test.go`。

### Fixed

- **【安全】图表 `c:pt/@idx` 无界分配**（`internal/chart/parse.go`）：`idx` 直接取自文件内容，
  `make([]string, max+1)` 可被单文件驱动到 `makeslice` panic 或约 16 GB 分配，
  且经公共 API `ChartShape.Data()` 可达、库内无 recover。现加上界并保留"缺号补空串"语义。
- **【安全】MP4 `ftyp` 兼容品牌列表无上限**（`internal/videoprobe/mp4.go`）：每 4 字节 append 一个
  string，可放大 21×；现收集上限 64 个。
- **【性能】`Presentation.Slides()` 每页重复读取并解析主关系流**：该调用是循环不变量，
  提到环外并按 revision 缓存。实测 `BenchmarkPerfTraverse/100p-media`：
  16.4 ms → **3.96 ms**，26.77 MB → **1.17 MB**（−95.6%），223,284 → **10,713** allocs（−95.2%）。
- **【性能】`AddPicture` 去重 O(N²)**：`findExistingMedia` 对每个已存媒体都全量读字节再 SHA256；
  新增 part→hash 缓存（随 revision 失效）。
- **【工具】`scripts/perf/{run,smoke}.sh` 目标包写成 `.`**：ADR-029 后模块根已无 Go 文件，
  脚本长期 `no Go files … FAIL`（CI 两个性能任务因此长期红）。改为 `./pptx/`。
- **【易用】v2 迁移指南事实错误**（`docs/RELEASE-NOTES-v2.0.0.md`）：把 v1.0.x 的模块路径写成
  `github.com/F31/go-pptx/v2`（实为 `github.com/F31/go-pptx`），给出的 `grep|sed` 对任何 v1 用户
  都匹配不到——跑完以为迁完，实际一行没改。同时补报此前漏记的破坏性变更：
  v1 的公共包 `ir/` 在 v2 变为 `internal/ir` 且门面不再暴露。
- **【稳定性】`Save(nil, …)` 直接 panic**，而同包的 `Write` 早已做 nil 归一化、`Validate` 容忍 nil → 统一为 nil → `context.Background()`。
- **【易用】CLI**：子命令 `--help` 此前退出码 2 且只写 stderr（顶层 `--help` 却是 0 且写 stdout），
  `pptx inspect --help | less` 之类常规用法失效 → 统一为 usage 写 stdout、退出 0；真正的参数错误仍走 stderr + 退出 2。
- **【易用】CLI 覆盖保护存在 TOCTOU**：`writePresentation` 先 `os.Stat` 再无条件传
  `WithSaveOverwrite(true)`，既绕开库自身的 `ErrOutputExists` 守卫，也有 Stat 与 Save 之间
  目标被他人创建的竞态 → 改由库在一次原子检查内判定。
- **【文档】** README(中/英) 三处 `158 types / 131 methods / 40 Stable 段` 与金样不符 → 166 / 134 / 41；
  `PictureShape` 的 Stable godoc 列举了三个从未存在的方法（`SetPictureFit`/`PictureFit`/`PictureSource`）；
  `doc.go` 仍称"根包"且只列六子命令（实为九）；`SDKVersion` 的 ldflags 路径仍是 v1 的模块根。

### Changed

- ⚠️ **`Presentation.Close()` 改为幂等**：此前二次调用返回 `ErrClosed`，与 `defer p.Close()` 惯用法
  冲突（显式 Close 后再 defer，会拿到一个无法区分"真失败 / 已关闭"的错误）。现在重复调用一律返回
  `nil`；**Close 之后的业务方法仍然返回 `ErrClosed`**，句柄失效的可观测性未削弱。
  这是一次语义放宽，不影响任何合理调用方；唯一的适配点是此前依赖"二次 Close 返回错误"的断言。
- ⚠️ **`opc.Budget` 移除 `MaxXMLDepth` 字段**：该字段全仓只有定义 / normalize / 自身测试，**没有任何
  解析器消费**——调用方设置了以为深度受控，实际没有（典型"假能力"）。真实深度限制在
  `internal/xmlstore`（256）。此字段此前随 `internal/opc` 无法被外部引用，故移除不构成对外的破坏性变更。

### Removed

- 无。

## [2.0.0] - 2026-09-17

**BREAKING：公共导入路径迁移 `github.com/F31/go-pptx/v2` → `github.com/F31/go-pptx/v2/pptx`**
（ADR-030 演进第 3 步：门面收敛）。

- 模块根变为元仓库（无 `package pptx` 文件）；公共面收窄为 `pptx/` 子包
- 实现按功能垂直下沉 `internal/document/{model,style,geometry,text,media,table}`
  与 `internal/{bind,chart,opc,xmlstore,errs,diag,ooxmlns,…}`
- 迁移方法：所有 importer 把 `"github.com/F31/go-pptx/v2"` 改为
  `"github.com/F31/go-pptx/v2/pptx"`（一次性的 import 路径替换，无 API 签名变化）
- api_surface golden 不变（163 type / 40 Stable 段 / 60 符号 / 131 方法 /
  17 哨兵）；`render` 仍为公共契约（`render → pptx/pptx`）
- 新增英文 API 接口参考文档：`docs/api-reference.md`（AST 驱动生成器
  `scripts/gen/apidoc`，163 types / 36 顶层函数 / 143 导出方法 / 149 常量 / 18 哨兵）
- 架构（ADR-030 全闭环）：ooxml/schema 生成管线、逐域搬迁
  （geometry/style/text/media/table/bind）、ir 门面解耦、
  `internal/engine` 编排层（CLI+WASM 共享）、`internal/archlint` CI 依赖校验、
  gate 完整性收口；`projectShapes` 全路径零门面句柄读取（chart 投影改走
  `internal/ooxml` schema）

## [1.0.7] - 2026-09-16

**v1.0.6 后的第七个 patch release，定位为"音频形状可用性修复"。修复 6 处独立缺陷，使音频形状从"能生成但客户端三态各异"（能否打开 / 图标是否可见 / 是否有声彼此独立）变为 **PowerPoint 与 WPS 均可打开、喇叭图标可见、F5 放映自动出声**。唯一公共 API 变化是 `AudioSpec` 追加位置/尺寸字段（仅追加，binary-compat with v1.0.0–v1.0.6）。**

### Added

- **`AudioSpec.X/Y/Width/Height`（EMU，仅追加）**：音频形状在页面上的位置与尺寸。全零时默认 `1in×1in` 并定位到**页面右下角**（距右/下边各 0.25in）——此前音频形状恒为 `0×0`（不可见）。新增 `audioGeometry` 与 `Presentation.slideSize()`（读 `presentation.xml` 的 `p:sldSz`，缺失回退 16:9）。
- **内置喇叭图标** `assets/audio-speaker.png`（64×64 PNG，`//go:embed`，`audio_icon.go`）：作为音频 `p:pic` 的 poster 图标。
- **公开音频样本** `testdata/corpus/s004-audio/`（`.pptx`/`.edited.pptx`/`.actions.json`/`compat-smoke.json`/`manifest.json` + 生成脚本 `scripts/gen_audio/main.go`）：公开语料 36 → **37** 样本，闭合 V2.6 §15.3 第 3 条。

### Fixed

- **音频形状 `0×0` 导致客户端不可见（中，ADR-027）**：`buildAudioPicFragment` 硬编码 `a:ext cx="0" cy="0"`，而 `AudioSpec` 无几何字段、库亦无设置形状几何的写 API（`MoveShape` 仅调 z-order）→ 形状永久 0×0。
- **`a:blip` 指向音频文件导致无 poster 图标（中，ADR-027 续）**：客户端音频 `p:pic` 要求 `p:blipFill/a:blip` 指向**图片**（图标），音频仅经 `p:nvPr/a:audioFile@r:link` 关联；同时补 `<a:hlinkClick action="ppaction://media"/>`（可点击播放）与 `<a:picLocks noChangeAspect="1"/>`。
- **`p:cMediaNode vol="80"` 量纲错误导致静音（中，ADR-027 续二）**：`vol` 的 XSD 类型是 `ST_PositiveFixedPercentage`（0..100000 千分比），`80` = **0.08%**；改为 `vol="80000"`（80%）。
- **PowerPoint 打不开含 poster 的音频（高，ADR-027 续三）**：`a:blip` 指向图片时，`p:nvPr` 必须带 `p14:media`（Microsoft 2007 `.../relationships/media`）关联媒体；缺它 PowerPoint 判包不可用（WPS 宽容）。**该结论有适用边界**：`blip→媒体文件` 不需要 `p14:media`（video 路线，ADR-026），`blip→图片` 才需要。
- **WPS 不自动播放（中，ADR-027 续四）**：极简 `p:audio`+`delay="0"` 形态仅 PowerPoint 自动触发；WPS 需要显式播放命令。计时结构改为 PowerPoint 原生媒体播放形态：`p:seq`（`mainSeq`/`interactiveSeq`）→ `mediacall` 效果（`withEffect`/`clickEffect`）→ `p:cmd type="call" cmd="playFrom(0.0)"`，外加 `p:audio/p:cMediaNode`（`delay="indefinite"` + `onStopAudio`）；节点 ID 改 `900000+ShapeID*100`；`timingAudioSafeLocals` 白名单扩展。
- **`clone` 不认识新媒体关系类型（连带修复）**：`classifyCloneRel` 归类 `.../2007/relationships/media`（否则克隆含音频页面报 "unsupported internal relationship type"）。
- **（行为变更）音频默认定位**：由"不可见的 0×0"改为"可见的右下角 1in×1in"。仅影响此前实际不可用的音频形状，无破坏性。

### L3 真机客户端矩阵（第五轮）

| 验证项 | 结果 |
|---|---|
| `s004-audio`（公开样本） | PowerPoint 16.0.20326 + WPS 12.1.0.28599 均 `OPEN=ok`、图标可见、**F5 自动出声**（2026-09-16 用户确认） |

### Verification

```bash
go test ./...                                     # 全包 PASS
go test -tags=corpus ./...                        # 含 B1 金样比对（37 样本）
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v .   # AST 守门
go run ./scripts/gen_audio                        # 复现 s004-audio 样本
```

### Compatibility

- **v1.0.6 → v1.0.7：binary-compatible + API-compatible（仅 `AudioSpec` 追加字段）**——无 Stable 段/符号/方法/哨兵变化（163 type / 40 段 / 60 符号 / 131 方法 / 17 哨兵不变）。
- 下游升级动作：**建议**。任何生成含音频 PPTX 的下游都应升级——否则产物在客户端表现为图标不可见 / 无 poster / 静音 / PowerPoint 拒开 / WPS 不自动播放。

## [1.0.6] - 2026-09-16

**v1.0.5 后的第六个 patch release，纯 bug-fix（ADR-025 配音 / ADR-026 视频两条 OOXML 合规修复），公共 API 表面与 v1.0.5 逐项一致、binary-compat with v1.0.0–v1.0.5，输出字节布局不变。**

### Fixed

- **含配音的产物在 PowerPoint 下被判"文件或目录损坏"（高危，ADR-025）**：制作出第一个含
  配音的样本后实测暴露——`s001-text.narrated.pptx` 在 PowerPoint 16.0.20326 报
  `0x80070570 文件或目录损坏`，而同机同会话下原样本与 Tier 2 产物均能打开；
  **WPS 12.1.0.28599 却正常接受**（故代码级测试与 WPS 验证都未能发现）。
  根因是两处 OOXML 不合规，**均为必要条件**（最小实验矩阵证明缺一即失败）：
  - `buildAudioPicFragment` 生成的 audio `p:pic` 缺必需的 `p:nvPr`，且 `a:audioFile`
    缺必需的 `r:link`、位置错误（应在 `p:nvPr` 内而非 `p:blipFill` 内）；
  - `SetAdvanceAfter` 只扫描 `p:sld` 的直接子元素，**看不见**被 `mc:AlternateContent`
    包裹的既有 `p:transition`，于是追加了第二个 `p:transition`，违反 `CT_Slide`
    的 `maxOccurs=1`。
  修复：补全 pic 结构；新增 `alternateContentTransitions` 展开 mc 的 Choice/Fallback
  并把 `advTm` 写到**全部**既有 transition（保留源模板的 `spd`/`p14:dur` 与 mc 结构，
  不放宽任何校验）。
  - **真机复验**：修复后 PowerPoint 与 WPS 均 `OPEN=ok` + `SAVE=ok`，都把音频形状
    识别为 `type=16 (msoMedia)`，`advanceTime=2.5` 正确读出（修复前 WPS 只认作
    type=13）。
  - 新增 `audio_ooxml_compliance_test.go` 两条守门测试，并注入退化验证其必红。
  - **性质修正**：V2.6 §15.3 第 3 条由"环境型缺口（缺 audio 语料）"改为
    "**真实代码缺陷**（已修复）"；"播放记录"仍需人工录屏/音频会话枚举补证。
- **`AudioShape.Profile()` 字段丢失（中，与上条同批）**：`lastProfileByMedia` 是一份
  手写精简解析副本，只读 `trackKey`/`role`/`media`/`sha256`/`shapeID`/`version`，
  漏掉 `slide`/`durMs`/`stMs`/`trigger` —— `Profile().Duration` 恒为 0（用户读不到
  时长），而 `PlanTimingSync` 走完整解析器 `parseAudioProfile` 却有值，**两条路径
  语义不一致**。修复：`lastProfileByMedia` 改为复用 `parseAudioProfile`，消除副本。
  - 影响面：主链路 `PlanTimingSync`/`ApplyTimingPlan` 不受影响（走完整解析器），
    属**契约一致性缺陷**而非产物缺陷。
  - 守门 `TestAudioShapeProfileExposesDuration`（断言 `Duration`/`Role`/`MediaPart`/
    `SlidePart`/`ShapeID`/`ContentSHA256` 均有效），**注入退化验证必红**（`Duration = 0s, want 2s`）。
- **读取侧同源修复（中，ADR-025 同批；由端到端往返测试暴露）**：写入侧修好后 go-pptx
  **读不回自己写的音频形状**，三处读取侧同源问题一并修复：
  - `picMediaKind`（`shape.go`）原本只在 `p:blipFill` 子树里找 `a:audioFile` → 改为先探测
    正确位置 `p:nvPicPr > p:nvPr`，再回退 `p:blipFill`（**兼容 v1.0.5 及更早产物**）；
  - `classifyShape` 构造 `AudioShape` 时**不填 `profile`** → 读回后 `Profile()` 全零值、
    `AudioSource()` 因 `MediaPart` 为空报 `ErrNotFound` → 改为按 `cNvPr@id` 从
    `/docProps/audio.xml` 取回 Profile；
  - `Slide.AdvanceAfter()` 与写入侧犯同样的错（只扫 `p:sld` 直接子元素）→ 读不到
    mc 包裹的 `advTm` → 复用 `alternateContentTransitions` 展开 mc 两个分支。
  - 新增端到端往返守门 `TestNarratedDeckRoundTrip`（生成→保存→重开→断言形状/Profile/
    `AdvanceAfter` 完整往返）。**自包含：音频用代码合成，无需往语料库放二进制样本**，
    CI 恒可执行 —— 故本项不需要 opencode 侧入库配合。
  - **未在本次范围**：video 的探测（`p:videoFile`）。曾试图一并修正其命名空间，
    既有测试 `TestSlideAddVideo_PicClassifiedAsVideo` / `TestSlideClone_PreservesVideoProfile`
    立即变红，已回退；待先确认真实产物的 video 写法。

- **含视频的产物在 PowerPoint 下被判"文件或目录损坏"（高危，ADR-026，与 ADR-025 同源）**：
  修复 audio 后顺带排查发现视频形状存在**同构缺陷**——`buildVideoPicFragment` 同样缺必需的
  `p:nvPr`，且视频引用写成 `p:videoFile`（**命名空间错**）、缺必需的 `r:link`、位置错
  （应在 `p:nvPr` 内）。实测：`s001-text.video.pptx` 在 PowerPoint 报 `0x80070570`，
  WPS 则 `OPEN=ok` 但只认作 `type=13` —— 与 audio 修复前逐项一致。
  正确形式经 PowerPoint 原生 `AddMediaObject2` 的实包取证确认：
  `<p:nvPr><a:videoFile r:link="rId2"/></p:nvPr>`。
  - 修复：补 `p:nvPr`；视频引用改为 `<a:videoFile r:link="rIdX"/>` 并移入 `p:nvPr`；
    读侧 `picMediaKind` **追加**正确命名空间探测并**保留**旧 `p:videoFile` 分支（兼容
    v1.0.5 及更早产物）；`hasVideoFile` 改为复用 `picMediaKind`，消除重复实现。
  - **真机复验**：修复后 PowerPoint 与 WPS 均 `OPEN=ok`，`Video 11` 均为
    **`type=16 (msoMedia)`**（修复前 WPS 只认作 type=13）。
  - 新增 `video_ooxml_compliance_test.go`（片段合规 + 读侧双形式兼容，4 子用例），
    **注入退化验证必红**。
  - **教训**：这是一**类**缺陷而非两个独立事故 —— 同一套错误写法被复制到 audio / video
    两个函数，读侧也各维护一份重复探测；**真机验证必须覆盖每一个复制点**，代码级测试
    只断言"自己生成的形态"，发现不了"生成的形态本身不合规"。

### Verification

- **配音证据链推进到自动化极限**（§15.3 第 3 条；详见 `docs/client-compat-matrix.md` §第三轮）：
  用 PowerPoint COM `CreateVideo(UseTimingsAndNarrations)` 做对照实验确认——
  - 产物被 PowerPoint 与 WPS 识别为 `type=16 (msoMedia)`，与 PowerPoint **原生** `AddMediaObject2`
    插入的音频形状在 COM 视角下等价；
  - 写入的 `advanceTime="2500"` 被**实际采用**：导出 mp4 时长为 **2.508s**，而非常量参数
    `DefaultSlideDuration = 2`（证明计时结构被解析执行）；
  - **`CreateVideo` 不渲染页内音频对象**：原生对照组同样无音频轨（box 结构逐项一致），
    故该路线不能作为"配音可播放"的证据；
  - **人工录屏完成闭合**：录屏 `.mp4` 含 2 条 trak（`vide` + **`soun`**；838 AAC 帧 /
    423191 B / ≈126 kbps），**人耳确认可听到声音**（音频源为麦克风 —— 声音经空气传播，
    反而排除"仅软件渲染、实际无输出"的可能）。
  - **§15.3 第 3 条据此闭合 → V2.6 §15.3 五条发布硬门槛全部闭合**。

## [1.0.5] - 2026-09-16

**v1.0.4 后的第五个 patch release，也是 v1.0.1 以来首个含生产代码改动的 patch**（v1.0.1–v1.0.4 均为测试/文档增量）。v1.0.4 → v1.0.5 共 **12 个 commit / 39 文件（+2291 / −109）**。

公共 API 表面**只增不改**：`// Stable:` 段 34 → **40**、Stable 符号 50 → **60**、Stable 方法 129 → **131**、`// Experimental:` 5 → **0**、公共 type 158 → **163**——**binary-compat with v1.0.0 / v1.0.1 / v1.0.2 / v1.0.3 / v1.0.4**，零签名变更、零字段删除。

本版三条主线：① **Save 性能**（ADR-018 Tier 2 原始帧直通，未变媒体不再重压缩）；② **API 表面预备**（A-2 形状能力窄接口 + WASM API GA 化）；③ **Tier 2 的两处同源缺陷修复**（ADR-024 重复条目 + COV-04 覆盖率门槛回归）。

### Added (API)

- **A-2 形状能力窄接口（ADR-021）**：新增 5 个 `// Stable:` 能力接口
  —— `GeometryProvider` / `FillProvider` / `EffectsProvider` /
  `StyleMatrixRefsProvider` / `LineProvider`，并补 8 条形状编译期断言。
  既有 `Shape` 接口的 getter **全部保留**，新调用方可按需做窄接口断言组合；
  `goldenStableSymbols` 50 → 55。
- **FEAT-002 项 3 降置信诊断（ADR-020）**：新增
  `ChartShape.DataWithDiagnostics` + `internal/chart.ChartAxisUnreadFieldNames`
  + `ir.Shape.Diagnostics`（`// Experimental:` IR 字段）。图表轴单位读不到显式
  `c:numFmt` 时给出可解释的降置信标记，而非静默取默认值。零 API 扩张。

### Performance

- **ADR-018 Tier 2：未变非 XML Part 走原始帧直通**（`(*zip.File).OpenRaw` +
  `(*zip.Writer).CreateRaw`），跳过解压与重压缩。同进程 A/B 取证：未变媒体重压缩
  占 Save p50 的 **50–73%**（3×8 MiB 65.3% / 73.4%、100×768 48.0% / 66.2%）。
  安全门限：仅非 XML、仅 `Store` / `Deflate`、无加密位(bit0)、无 data-descriptor
  位(bit3)、声明尺寸非零；不满足者自动退回 Tier 1 流式复制。B1 语义保持
  （解压内容逐字节不变）。

### Fixed

- **`SavePlan.Write` 重复条目（高危，ADR-024）**：ADR-018 Tier 2（raw 直通）引入的回归
  —— `Write` 循环开头无条件 `zw.Create(entry)`，而 `tryRawCopyOriginal` 走通时又
  `zw.CreateRaw` 注册同名第二个条目（`archive/zip` 允许同名重复，静默通过）。后果是
  **任何含「未变 + 非 XML」Part 的文档**（即含图片/视频/音频/嵌入对象的绝大多数真实
  PPTX）经 `Save`/`SaveToFile` 产出重复条目：`verifyOutput` 报
  `output has 96 entries, plan wants 75`、`opc.Load` 报 `duplicate entry`。
  ext-0024 实测 21 个 media 各重复一次（75 + 21 = 96）。修复：`zw.Create` 下沉到
  `EmitPatched`/`EmitNew` 与「raw 回退」两个真正需要的分支。
  - **根因定位耗时三轮**（前两轮结论已作废，详见 ADR-024 §取证过程）：最初误判为
    "目录条目口径问题"（源 91 = 75 文件 + 16 目录，凑巧接近 96），实为
    `96 = 75 + 21`，21 恰为走 raw 直通的 non-XML Part 数。
  - 新增守门 `TestSavePlanWriteNoDuplicateEntries`（按 `zr.File` 逐条计数不经 map +
    断言条目数 == 计划条目数 + 断言 `opc.Load` 接受），并已注入退化验证其必红。
    `TestRealExt0024_SaveUnchangedB1` 新增 `assertEntryCountInvariant`。
  - **零公共 API 变化**；`go test ./...` 与 `-tags=corpus ./...` 均 14/14 全绿。
- 修补 4 层测试盲区：Tier 2 测试用 `map[name][]byte` 收集条目导致同名重复互相覆盖；
  合成包无目录条目且未断言条目数；ext-0024 私有语料 CI 恒 Skip 且末段仅 `t.Logf`；
  `Write` 单测自建 `zip.NewReader` 不校验重复。
- **`internal/opc` 覆盖率门槛回归（中，ADR-018 Tier 2 第二处同源缺陷）**：Tier 2 新增的
  `tryRawCopyOriginal` 含 4 条安全门限拒绝分支（非 XML / 仅 Store 或 Deflate / 排除加密位
  与 data-descriptor 位 / 尺寸必须已知），**落地时零测试覆盖** → `internal/opc` 覆盖率从
  90.4% 静默跌到 **89.5%**，跌破 COV-04 的 90% 门槛，三天无人察觉（只跑了 build/test/vet）。
  取证：`git worktree` 隔离实测 `f7c8dad`（90.4%）vs `27a539d`（89.6%），确认降幅由 Tier 2
  引入、ADR-024 的结构调整无额外影响。
  - 新增 `internal/opc/saveplan_raw_guard_test.go`：8 case 安全门限表（含"被拒帧不得在输出
    注册任何条目"，同时守住 ADR-024 的缺陷形态）+ data-descriptor 帧端到端回退验证 +
    3 项写入错误注入。手工 ZIP 构造 helper 用于产出 `zip.Writer` 无法生成的异常帧。
  - 覆盖率 **89.5% → 90.3%**（corpus 89.6% → **90.5%**），门槛恢复。**零生产代码改动。**

### Changed (API stability)

- **WASM API GA 化（D-5，ADR-023）**：5 个 `// Experimental:` 段全部升为 `// Stable:`
  —— `ChartWorkbookBuilder` / `ChartDataBook` / `DefaultWorkbookBuilder` /
  `CustomPropertyKind` / `CustomPropertyValue`。公共 API 表面分类由
  `34/50/5/130`（Stable 段/符号/Experimental/Stable 方法）调整为
  `40/60/0/131`，**无任何签名或字段变更，binary-compat with v1.0.x**。
  WASM 嵌入消费者现获得稳定契约，不再受 1.x 内静默变更的威胁。
- 不在本次范围：`ir.Page.Hidden` / `ir.Shape.Diagnostics` 的 `// Experimental:`
  字段标记（属 `ir` 包，需各自 ADR 评审，见 FEAT-003 / ADR-020 文档）。

### Verification

- `go test ./...` 与 `go test -tags=corpus ./...` 各 **14/14 全绿**；`api_surface_test.go`
  7 个 AST 断言 PASS（与 v1.0.4 的计数差异见上方「公共 API 表面」行，属**有意**的
  golden 名单更新，已过 ADR-021 / ADR-023 评审）；
- `CGO_ENABLED=0 go vet ./...` 与 `-tags=corpus` 双口径零警告；`gofmt -l .` 干净；
- 四目标交叉构建（`js/wasm` / `wasip1/wasm` / `darwin/arm64` / `linux/arm64`）全部通过；
- **L3 真机客户端矩阵（Tier 2 产物）8/8 通过**：PowerPoint 16.0.20326 + WPS
  12.1.0.28599 × 4 样本（`s001-text` / `s002-table` / `s003-image` / `ext-0024`）均
  无修复提示打开、重存成功；重存文件经 go-pptx `Validate` 全部 errorCount=0。Tier 2
  改变了输出字节，故按 ADR-018 验收清单第 5 项重跑——详见
  `docs/client-compat-matrix.md` 第二轮。
- 覆盖率：root 合并口径 **84.4%**；`internal/opc` **90.3%**（Tier 2 曾使其跌到
  89.5%，本版补齐门限守门测试后恢复）/ chart 91.2% / xmlstore 90.8% /
  videoprobe 92.6% / audioprobe 88.4% / editplan·textmap 100% / ir 86.1% /
  render 84.2%。

### Compatibility

- **v1.0.4 → v1.0.5：binary-compatible + API-compatible（只增不改）**——新增 5 个
  Stable 窄接口与 2 个方法/字段，既有签名与字段零变更；5 个 Experimental 段升
  Stable 属**承诺增强**，不构成破坏性变更。
- 下游升级动作：可选。使用 WASM 嵌入 API 者，本版把此前的 Experimental 段变为稳定
  契约（收益是消除 1.x 内静默变更的风险）；其余调用方行为完全一致。
- **注意**：Tier 2 之后 `SavePlan.Write` 的**输出字节布局**与 v1.0.4 不同（未变媒体
  Part 保留源压缩帧与 Extra，不再重新 Deflate），但**解压内容逐字节一致**——B1 语义
  与 OPC Part 视角（PartNames + 内容哈希）的等价性不变。若下游做过输出 ZIP 的逐字节
  比对，需改用 Part 视角比对。

## [1.0.4] - 2026-09-13

**v1.0.3 后的第四个 patch release——质量里程碑版**。v1.0.3 → v1.0.4 共 10 个 commit，**全部为测试与文档增量，零生产代码改动**：公共 API 表面与 v1.0.3 逐项一致（158 type / 34 Stable 段 / 50 Stable 符号 / 129 Stable 方法 / 5 Experimental / 17 哨兵全部不变），**binary-compat with v1.0.0 / v1.0.1 / v1.0.2 / v1.0.3**。

本版核心是 6 轮覆盖率补测（commits `120c1b3` / `a2efe4a` / `dbaf0d3` / `968110a` / `315e077` + 各轮 memory sync）：根包合并口径覆盖率 **82.3% → 84.4%**，**零覆盖函数清单全部清零**，测试资产 **+1251 行 / 15 个测试文件**。

### Added (tests-only)

- **6 轮覆盖率补测**（全部为表驱动/白盒单元测试，不走 fixture，回归快）：
  - `OpaqueShape.Kind()` 三路径 0% → 100%（cxnSp / graphicFrame / 未知类型）；
  - `Rect.Contains` 0% → 100%（22 个子测：边界语义 / 越界 / 负宽高 / 退化矩形 / 负坐标）；
  - `clone.go` 4 helper：`fallbackCloneCT` 28.6% → 100%、`retargetRel` 60% → 100%、`classifyCloneRel` 75% → 100%、`splitTrailingDigits` 78.6% → 92.9%（余 1 行为 Atoi 整数溢出退化分支，按"追逻辑分支不追退化安全网"原则不追）；
  - `leafTextPatch` 40% → 100% / `resolveColorSpec` 39.1% → 95.7% / `translateSentinel` 30% → 100% / `clamp01` 60% → 100%；
  - **10 个 0% 函数清零**：`chartTypeFromPlot` / `GeometryKind.String` / `EffectKind.String` / `appendPartUnique` / `inferCols` / `sizeCentipoints` / `parseHexRune` / `parseDecRune` / `nsPrefix` → 100%，`kindIndex` → 90.9%；
  - 低覆盖提升：`cloneChangeSet` 11.8% → 100%（深拷贝回滚语义）、`timingReferencesShape` 15.8% → 94.7%、`rPrChildRank` 23.5% → 100%、`xmlUnescape` 54.8% → 100%。

### Fixed (documentation)

- **[1.0.3] 段日期笔误修正**：`2026-09-22` → `2026-09-12`（tag 实际创建日；早前会话时钟漂移所致，v1.0.0–v1.0.3 四个 tag 均创建于 2026-09-11/12）。

### Verification

- `go test ./...` 全包 PASS 无回归；`api_surface_test.go` 7 个 AST 断言 PASS（数字与 v1.0.3 完全一致）；
- `CGO_ENABLED=0 go vet ./...` 零警告；`CGO_ENABLED=0 GOOS=js GOARCH=wasm go build ./...` 通过；
- 覆盖率：根包合并口径（`-coverpkg=./.`）82.3% → **84.4%**；opc 90.4% / chart 91.4% / audioprobe 88.4% 维持。

### Compatibility

- v1.0.4 = binary-compat + API-compat with v1.0.0 / v1.0.1 / v1.0.2 / v1.0.3（**零生产代码改动**；下游消费者无任何升级动作，本版收益为可审计的质量基线与回归资产）。

## [1.0.3] - 2026-09-12

**v1.0.2 后的第三个 patch release**——按 ppts 项目《go-pptx 特性与 bug 跟踪计划》FEAT-003 实施（ppts/内部登记号 G1-2；本仓库 doc 文件位置 `docs/ppts-sync/FEAT-003-hidden-advtm-read.md`）。公共 API 表面扩展 2 个 Stable 只读方法 + 1 个 Experimental IR 字段，**与 v1.0.0 / v1.0.1 / v1.0.2 binary-compat**——零破坏、零字段删除、零签名变更。

### Added

- **FEAT-003 读侧补全**（按 ppts《go-pptx 特性与bug跟踪计划》V1.0 §7/FEAT-003）：
  - `Slide.Hidden() (bool, error)`：**新增 Stable 公开方法**——读 `p:sldId@show="0"` 解析页面隐藏标志；OOXML 语义：仅 `show="0"` 视为真隐藏，缺省/其他值/空串均为可见。**binary-compat with v1.0.2**。配套测试 `TestSlide_Hidden_*` 3 条 + 文档注记。
  - `Slide.AdvanceAfter() (time.Duration, bool, error)`：**新增 Stable 公开方法**——读 `p:transition@advTm` 毫秒值并转 `time.Duration`；与 `SetAdvanceAfter` 写入对偶。第二个返回值 `ok` 区分"显式设定"与"未设"。**binary-compat with v1.0.2**。配套测试 `TestSlide_AdvanceAfter_*` 3 条。
  - `ir.Page.Hidden *bool`：**新增 Experimental IR 字段**——三态（`nil`=未读/`&false`=确认可见/`&true`=确认隐藏）；同时新增 `ir.Options.IncludeHidden bool`（默认 `true`）。ppts 默认过滤器可据此跳过隐藏页。配套测试 `TestFromPresentation_PageHiddenProjection` 2 子测试。

### Documented

- **FEAT-002 项 1 备注过滤契约注记**：`notes.go` 顶部补"隐式过滤契约"段，明确 `SpeakerNotesText` 仅取 `p:ph type="body"`，自动跳过 `hdr/ftr/sldNum/dt` 等模板占位符；ppts 项目库对此无需自行再判断。代码行为早在 v1.0 已生效，本版仅为契约文档化。

### Fixed

- **BUG-001（已在 HEAD 自愈，无须打 commit 改动代码）**：`validateChartData` 双声明问题——在 ppts《go-pptx 特性与bug跟踪计划》V1.0 §7/BUG-001 报告时（2026-09-12）描述的"chart.go + chartfrag.go 同时存在 validateChartData 函数体"已于 ADR-017 r3（commit `a3abfca` + 后续 33 个 commit）后自愈；当前 HEAD `c47b7a3` 仅 `chartfrag.go:22` 一处声明；`go build ./...` 全绿。ppts 侧可直接把该 BUG-001 条目标记为 CLOSED（自愈）。

### Verification

- `gofmt -l` 零输出（除 CRLF 假阳性外）；`go vet ./...` 零警告
- `go test ./...` 默认 14/14 包 ok
- `go test -tags=corpus ./...` 14/14 包 ok
- 4 目标交叉构建（`js/wasm` + `darwin/arm64` + `wasip1/wasm` + `linux/arm64`）`CGO_ENABLED=0` 零失败
- `api_surface_test.go` 7 个 AST 断言 PASS（`TestAPIFrozenStableMethods` 由 127 → 129，反映新增 2 个 Stable 方法）
- 36 样本语料 validate 0 错误（corpus tag 跑过的回归）
- 新增功能矩阵登记：`Inspect` 维度 + `Read` 维度（`Hidden`/`AdvanceAfter` 读侧）

### Compatibility

- v1.0.3 = ABI-compat with v1.0.0 / v1.0.1 / v1.0.2（仅追加只读公开方法 + IR 字段；旧调用方零修改）
- v1.0.3 = API-compat with v1.0.0 / v1.0.1 / v1.0.2（冻结清单 50 Stable 符号 + 127→129 个 Stable 方法；类型总数、哨兵数、Experimental 段数不变）

### Acknowledgments

- ppts 项目工程提供 §7《go-pptx 特性与bug跟踪计划》V1.0 跟踪文档与 FEAT-001/002/003 验收契约；本版针对 §7/BUG-001（自愈）+ §7/FEAT-002 项 1（仅文档注记）+ §7/FEAT-003 全量。

## [1.0.2] - 2026-09-12

**v1.0.1 后的第二个 patch release**——公共 API 零变化（binary-compat with v1.0.0 / v1.0.1），主要工作是 chart 实现搬迁到 `internal/chart`（[ADR-017](docs/adr/ADR-017-chart-internal-extraction.md) 三批）、Save 流式复制落地与量化（[ADR-018](docs/adr/ADR-018-save-streaming-copy.md) Tier 1）+ 反向劣化修复、B1 金样比对真正接入 CI、冻结清单不变量自动化守门、`internal/opc` 达到 COV-04 全闭合。

关键不变量：`// Stable:` 段落 34（不变）/ Stable 符号 50（不变）/ `// Experimental:` 段 5（不变）/ 公共 type 总数 158（不变）/ 错误哨兵语义锁死（不变）/ 黄金语料 B1 哈希 PASS（不变）。本版新增 `api_surface_test.go`（7 个 AST 断言）把以上不变量以自动化方式锁死，未来公共 API 任何意外漂移都会在 `go test` 失败。

### Changed

- **ADR-017 chart 抽 internal 三批完成**（commits 46c2f2d / ce3f66c / a3abfca / bc533d3）：① 第一批搬 3 个真零依赖函数 + 4 常量；② 第二批 11 个值对象通过 type alias 方式 move（含 `font.go` 的 `Optional[T] = chartinternal.Optional[T]`，类型身份与源码级均不变）；③ 第三批 parse/build/canonical/validate/fragment/workbook 实现全搬迁 + 删除 31 个已无生产调用方的根包 facade。根包 `chart.go` 1319 → 481 行，`internal/chart` 覆盖率 **91.4%**。公开 API 零变化。
- **ADR-018 Save 流式复制（Tier 1）落地 + 反向劣化修复**（commits 9bfe44d / abd9a9f / ee4bb17）：`SavePlan.Write` 的 `CopyOriginal` 从整 Part `readAll` 改为 `Package.OpenPart` + `io.Copy`，峰值内存 O(最大 Part) → O(32 KiB 缓冲)。量化（3×8 MiB 未变媒体，go1.27 windows/amd64，Ultra 9 275HX，`-benchtime=20x`，输出写 `io.Discard`）：B/op 64.2 MB → 206 KB（**−99.7%**）、峰值堆增量 52.6 MiB → 6.08 MiB（**−88.4%**）、耗时 14.65 ms → 8.11 ms（**−44.7%**）；Tier 2（OpenRaw 直通）经可行性验证后**不实施**（重启条件见 ADR-018）。反向劣化：发现并修复 Tier 1 引入的小档 10× 劣化（`io.Copy` 每调用分配 32 KiB → 整轮复用同一缓冲，10p-text 分配 103.9 KB → 76.5 KB）。
- **PERF-01 报告与 CI 冒烟守门**（commit ee4bb17）：`scripts/perf/summarize` 新增 `SavePlanWriteCopyOriginal` 分组；`scripts/perf/run.{sh,ps1}` 新增 `OPC_BENCH` / `OPC_BENCHTIME`（默认 `20x`）/ `OPC_COUNT`（默认 3）/ `SKIP_OPC`；`scripts/perf/smoke.sh` ①b 守门加 5 个 opc 子基准。
- **WASM 产物瘦身 + perf 产物收口**（commit 5f105a1）：`scripts/check_wasm.{sh,ps1}` 加 `-trimpath -ldflags="-s -w"`（可用 `SLIM=0` 关闭）——`pptx_check.wasm` 6.069 MB → 5.951 MB（−118 KB / −1.9%）；`scripts/perf/raw-bench.log` 退库转 gitignore，RAW 默认改 `perf-out/raw-bench.log`（`perf-out/` 早已 gitignore）。
- **执行策略卫生**（commits d1d2854 / 2b0eb10）：原入库的运行产物退库；文档数字与代码实况同步。
- **`internal/document` 接口编译期契约测试**（commit 1915fc2）：`var _ Foo = (*Bar)(nil)` + 反射方法数兜底。

### Fixed

- **B1 金样比对真正接入 CI**（commit b0144b3）：原 B1 断言只挂在合成 fixture 与私有样本 `ext-0024`（原文件未入库 → CI 恒 Skip）上，"未修改 Part 解压内容哈希一致"这条 V2.6 §15.3 发布硬门槛**此前从未在 CI 真正执行过**。新增 `corpus_b1_test.go`（tag=corpus）建在库内可再分发的公开样本上：`TestCorpusB1UnchangedSave`（空编辑保存各 Part SHA256 恒等）+ `TestCorpusB1AfterTextEdit`（仅 `SaveReport.ChangedParts` 声明的 Part 可变，且变更集合必须等于该集合）+ `TestCorpusB1PresentButSkipWhenNoSample`（源样本缺席优雅 Skip）。抽 `corpusApplyReplaceText` 供 replay 与 B1 共用。
- **opc 88.9% → 90.4% / COV-04 全闭合**（commit b676bab）：14 个行为优先测试（Write Omit / 未知 action / 无效 PartName / readAll 缺失 Part / relsPartOf invalid / lastIndexByte 无匹配 / ParseContentTypes 缺属性 / 忽略未知子元素 / extensionOf 边缘 / addOverride 无效 / 重复 / removeOverride / mustAttrEscape 回退）。`internal/opc` 升至 **90.4%**，锁住 COV-04 全部 6 包低层格式 5/6 ≥ 90%（`audioprobe` 88.4% 按 1.x-roadmap B-2 决策不追）。
- **冻结清单不变量自动化守门**（commits 20b12a4 / b2cac60）：`api_surface_test.go` 用 `go/ast` 解析根包非测试文件，断言 158 type / 34 Stable 段 / 50 Stable 符号 / 127 Stable 方法 / 5 Experimental / 17 哨兵 / 错误字符串字面量锁死 / 根包无 `//go:build` 约束。替代此前不可靠的人工 grep（`grep '^type [A-Z]'` 只数出 149，漏掉分组声明 9 个）；v1.0.0 时就出过一次口径错误（把"段落 grep 数 34"误作"独立 type 数"），事后连改 6 处文档。
- **opc fuzz 安全导向种子扩充**（commit 1915fc2）：3 条种子（恶意条目名 `../` / `\` / 控制字符 / 200 条目逼近预算 / 64 层深路径）+ `mustSeedZip` helper 集中构造恶意但合法的 ZIP 流。

### Added (documentation-only)

- [`docs/adr/ADR-017-chart-internal-extraction.md`](docs/adr/ADR-017-chart-internal-extraction.md) —— chart 抽 `internal/chart` 的分批路径与踩坑记录（含"严格区分真零依赖 vs 接收根包值对象"、"const 必为编译期常量不可直接引用 var 包常量"、"type alias 不能定义方法"三条经验）。
- [`docs/adr/ADR-018-save-streaming-copy.md`](docs/adr/ADR-018-save-streaming-copy.md) —— Save 流式复制（含 Tier 2 不实施决策与反向劣化修正章节）。
- `docs/PERF-01-benchmark-report.md`（更新）—— 接入 ADR-018 收益与守门说明。
- `docs/release-readiness-2026-09-12.md` —— 本次发布的就绪度评估报告。
- [`docs/RELEASE-NOTES-v1.0.2.md`](docs/RELEASE-NOTES-v1.0.2.md) —— 本版本完整 release notes。

## [1.0.1] - 2026-09-12

**v1.0.0 后的首个 patch release**——公共 API 零变化（binary-compat with v1.0.0），主要工作是内部实现层重构、bug 修复、覆盖率收敛、客户端矩阵真机执行与文档体系完善。

关键不变量：`// Stable:` 段落 34（不变）/ Stable 符号 50（不变）/ `// Experimental:` 段 5（不变）/ 公共 type 总数 158（不变）/ 错误哨兵语义锁死（不变）/ 黄金语料 B1 哈希 PASS（不变）。

### Changed

- **ADR-016 渐进式 internal 抽取完成首轮收敛**（commit 76a630f）：新增 `internal/document`（PartStore / ReadStore / PatchStore 契约）、`internal/textmap`（rune 映射）、`internal/editplan`（`SinglePartPatch` / `MultiPartPlan`）；生产业务写入路径全部收敛到 `applySinglePartPatch` / `applyMultiPartPlan`，直接 `stage*/commit` 调用仅保留在 `presentation.go` 与 `document_store.go` adapter 边界内；公开 API 零变化。
- **ir 包表格文本投影闭环**（commit def9140）：`ir.readTableText` / `ir.cellText` 由 0% 覆盖补齐，新增 `ir/table_ir_test.go` 自建最小 zip fixture deck 覆盖 2×2 文本、空单元格、零行表格、非正维度四场景；副作用 `ir.Diff` 现在能更准确地报告表格单元格文本变化，公开 API 签名零变化。
- **覆盖率收敛（COV-01 / 02 / 04）**：
  - COV-01 达成（full total ≥ 80%）：full 78.3% → 80.0%（2026-09-11）
  - COV-02 达成（root ≥ 82%）：root 81.5% → 82.0%，full total 82.5% → 82.9%（2026-09-11）
  - COV-04 评估：放弃统一 90% per-package 口径——90% 仅适用于低层格式包（opc / xmlstore / videoprobe / audioprobe / textmap / editplan），root SDK 目标 85%，command/helper 目标 85%
  - 5 包行为优先补测（2026-09-12）：`internal/editplan` 81.8% → **100.0%** / `internal/textmap` 82.2% → **100.0%** / `internal/opc` 86.6% → 88.9% / `internal/audioprobe` 84.9% → 88.4% / root 82.7% → 82.8%
  - **full-repo 加权 total 83.2% → 84.4%**，4/6 低层格式包 ≥ 90%（`videoprobe` 92.6% / `xmlstore` 90.8% / `editplan` 100% / `textmap` 100%）
- **L3 文档同步收口**（commit dfc27c3）：testdata/corpus/README.md / 实施状态跟踪 / MEMORY.md 共 3 处 L3 客户端矩阵完成事实登记，新增"已登记客户端版本与平台"段。

### Fixed

- **MediaSource nil 流保护**（commit d087aff）：`MediaSource.Reader()` / `MediaSource.Length()` / `MediaSource.MediaType()` 三个公共访问器对 nil receiver 显式返回错误（之前 panic 在 nil deref）。按"由 panic 改为 error"算只加防御不减能力，严格 PATCH。
- **Stable 计数 off-by-one 口径修正**（commit ae48794）：33 独立 type + 17 哨兵 = 50 符号 / 34 段落 / 5 Experimental / 120 API / 158 总；勘误前 6 处文档（CHANGELOG / RELEASE-NOTES-v1.0.0 / v1.0-freeze-list / 实施状态跟踪 / 技术白皮书 / MEMORY.md）数字统一。
- **ADR-014 三处缺陷修正**（commit ae48794）：① "因…而…"悬空残句补全；② 依赖清单补充 `internal/document` / `internal/editplan` / `internal/textmap`；③ 目录树补 `internal/document`。
- **CI corpus-replay 鲁棒性**（commit 6361bdd）：公开样本源缺席从 `Fatalf` 改为 `Skipf`，新克隆优雅跳过，真样本到位自动转真跑。
- **VideoShape 公开访问器与 probe 错误映射覆盖**（commit 6361bdd）。

### Added (documentation-only)

- [`docs/go-pptx-技术白皮书.md`](docs/go-pptx-技术白皮书.md)（commit 0218b29，746 行 / 12 章）—— 综合性技术披露，覆盖产品定位、技术架构、应用场景、对比矩阵、7 项技术创新点详解。
- `docs/1.x-roadmap.md`（commit 24d4fa6，213 行 / 8 章）—— 1.x 演化窗口路线图：5 候选方向 + 5 阶段路线 + 6 风险 + 5 推荐决策点。
- `docs/client-compat-matrix.md` L3 真机执行记录（commit a103484 + 5f99d80）—— PowerPoint 16.0.20326 + WPS 演示 12.1.0.28599 × 4 样本 8/8 通过。
- [`docs/RELEASE-NOTES-v1.0.1.md`](docs/RELEASE-NOTES-v1.0.1.md) —— 本版本完整 release notes。

## [1.0.0] - 2026-09-11

The first stable release of go-pptx.

### Highlights

- **Three-tier API stability** — Stable (50 symbols: 33 independent types + 17 error sentinels sharing one aggregated section, 34 `// Stable:` sections total) / API default (120 types, additive evolution allowed) / Experimental (5 types, may change in 1.x). See `docs/v1.0-freeze-list.md` and [ADR-015](docs/adr/ADR-015-api-stability-tiers.md).
- **M0–M8 milestones complete** — full PPTX read/edit stack: OPC engine, XML store with span patches, slide/shape/text/table/chart/audio/video model, capability manifest, semantic diff, template binding.
- **Public corpus** — three LibreOffice-generated public samples (`s001-text` / `s002-table` / `s003-image`) gated by `//go:build corpus` CI job.
- **Cross-platform** — pure Go (CGO=0), CI-verified builds for `js/wasm`, `darwin/arm64`, `wasip1/wasm`, `linux/arm64`.

### Stable API (33 types + 17 error sentinels = 50 symbols)

- **Core entry points (6)** — `Presentation`, `Slide`, `Shape`, `TextFrame`, `Paragraph`, `TextRun`
- **Error sentinel family (17)** — one shared `// Stable:` section in `errors.go`; individual `Err*` constants append-only:
  `ErrClosed`, `ErrStaleHandle`, `ErrInvalidArgument`, `ErrOutOfRange`, `ErrNotFound`, `ErrForeignReference`,
  `ErrUnsupportedFormat`, `ErrUnsupportedEdit`, `ErrLimitExceeded`, `ErrMalformedPackage`, `ErrUnresolvedStyle`,
  `ErrValidationFailed`, `ErrTimingConflict`, `ErrDurationUnknown`, `ErrConcurrentModification`,
  `ErrOutputExists`, `ErrAtomicReplaceUnavailable`
- **Error type** — `OperationError`
- **Diagnostic contract (4)** — `Diagnostic`, `Severity`, `ValidationReport`, `CapabilityStatus`
- **Capability Output family (4 + 2 constants)** — `CapabilityManifest`, `CapabilityManifestSource`,
  `CapabilityDimension`, `CapabilityFeature`; JSON tag set locked, schema-version axis enforced via
  `CapabilityManifestSchemaVersion` / `CapabilityManifestDimensionKey`
- **Geometry value objects (4)** — `EMU`, `Point`, `Rect`, `Quad`
- **Handle ID types (2)** — `SlideID`, `ShapeID` (u32 + documented semantics)
- **Enums (3)** — `ReplaceMode`, `MultiCellTextPolicy`, `ShapeKind`
- **Shape handles (8)** — `GroupShape`, `AutoShape`, `OpaqueShape`, `PictureShape`, `TableShape`,
  `ChartShape`, `AudioShape`, `VideoShape`
- **Type alias** — `TextShape` (alias of `AutoShape`, same Stable contract)

### Experimental API (5, may change in 1.x)

- `ChartWorkbookBuilder` — adapter interface; 1.0 may add methods like `Close()` / `Validate()`
- `ChartDataBook` — workbook snapshot; fields may grow with builder extensions
- `DefaultWorkbookBuilder` — minimum xlsx output; may add fields later (no breakage for users not depending on them)
- `CustomPropertyKind` — OOXML variant enum; iota may grow in 1.x
- `CustomPropertyValue` — multi-field union; may be split by `Kind` in 1.x

### M0–M8 milestones

- **M0** — repo bootstrap (CI three-OS + WASM) + OPC ZIP index + XML scanner/index/patch + vertical validation
- **M1** — relationships/content-types/save plan/atomic save/Presentation skeleton
- **M2** — rich-text model + cross-Run replace + style resolution + image media
- **M3** — units/group matrix + table merge & style + format depth subset
- **M4** — media probe + audio embedding + narration playback
- **M5** — limited three chart types + same-document restricted page clone
- **M6** — transition animation + video shape + text advanced + chart extensions + layout diagnostic + theme style matrix + geometry R-tier
- **M7** — capability manifest + browser-native check tool + animation timing IR + template binding
- **M8** — DIFF-01 semantic diff + cross-document restricted clone + STALE-GUARD handle identity fix

### Implementation highlights

- **OPC**: ZIP index with budget + part discovery + relationships + content types (saving plan with byte-level preservation)
- **XML store**: namespace-aware scanner + node index tree + span patches (additive inserts, controlled namespace) — unknown subtrees preserved
- **Atomic save**: snapshot/restore on failure, no torn writes; default refuses overwrite (`ErrOutputExists`, `WithOverwrite` opt-in)
- **Concurrent-edit guard**: revision-counter snapshot rejects commits from stale sessions (`ErrConcurrentModification`)
- **STALE-GUARD**: shape/text/cell handle identity = cNvPr@id; survives `MoveShape` / `AddShape` / text edits; invalidates only on `RemoveShape`

### Tools

- `pptx capability` — emits 6-dimension capability manifest (`Inspect` / `Create` / `Edit` / `Preserve` / `Render` / `Play`)
- `pptx inspect` — read-only report (geometry / fill / effects / style matrix / layout info)
- `pptx diff` — semantic diff over two documents (`go-pptx.diff/1.0`)
- `pptx bind` — template data binding (`{{path}}` / `{{#if}}` / `{{#each}}`)
- `pptx validate` — diagnostic report (`ValidateOption` for level)
- `pptx check` (WASM) — browser-native privacy-preserving capability / inspect / validate

### Documentation

- `docs/v1.0-freeze-list.md` — full freeze list with 5-phase review trail
- [`docs/adr/ADR-014-root-internal-package-strategy.md`](docs/adr/ADR-014-root-internal-package-strategy.md) — internal package split policy
- [`docs/adr/ADR-015-api-stability-tiers.md`](docs/adr/ADR-015-api-stability-tiers.md) — three-tier stability model
- `docs/corpus-入库指南.md` — public sample corpus onboarding guide
- `docs/M6-排序输入.md` / `docs/M7-排序输入.md` / `docs/M8-里程碑总结.md`
- `docs/PERF-01-性能基线.md` + `docs/PERF-01-benchmark-report.md` — performance baseline + CI gate

### Known limitations

- **L3 client matrix not verified** — no PowerPoint/WPS real-machine smoke. Tracked in 实施状态跟踪 §"当前阶段"; per V2.6 §26 P1 this is a hard release gap.
- **Coverage 86.6%** (`cmd/pptx`) / **77.1%** (root). Below V2.6 §15.3 90% target but not a hard release gate per ADR-015 §4.
- **Public corpus** — only 3 LibreOffice-generated samples. Private `ext-*` (33 files) indexed but not redistributed (WPS source / restricted license).

### CI / Build

- `lint` job — `gofmt -l .` + `go vet` (default + `corpus` build tag)
- `corpus-replay` job — public sample replay against latest code
- 4 cross-builds verified per push — `js/wasm`, `darwin/arm64`, `wasip1/wasm`, `linux/arm64`

[1.0.0]: https://github.com/F31/go-pptx/releases/tag/v1.0.0
