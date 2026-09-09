# go-pptx

纯 Go、`CGO_ENABLED=0`、无外部运行时强依赖的 PPTX 创建与编辑组件（Presentation → Slide → Shape → TextFrame → Paragraph → Run 对象模型）。

- 远程仓库：`https://github.com/F31/go-pptx`
- 设计基线：《go-pptx 完整设计方案 V2.6 开发实施版》（`docs/go-pptx_完整设计方案_V2_6_开发实施版.md`）
- 实施计划：《go-pptx 项目实施计划》（`docs/go-pptx-项目实施计划.md`）
- 状态跟踪：`docs/go-pptx-实施状态跟踪.md`

## 仓库基线记录（CORE-01，建仓提交）

| 配置项 | 当前值 | 状态 |
|---|---|---|
| module path | `github.com/F31/go-pptx` | 已随 CORE-02 由占位 `go-pptx` 正式化 |
| 许可证 | Apache-2.0 | 与远程仓库 LICENSE（Initial commit）一致 |
| Go 工具链 | go.mod 声明 `go 1.24.0`（最低），本地开发 1.27.0 | CI 双版本验证 |
| 平台 | Windows 主开发；CI 覆盖 Windows/Linux/macOS | 原子替换等平台行为后续真机验证 |
| 首批客户端 | PowerPoint / WPS（具体版本待登记） | 待补充到 `testdata/corpus/README.md` |

> 模块名、正式许可、工具链与客户端版本应在仓库初始化提交中记录（方案 §27）。

## 当前状态（M2 已收口，M3 推进中）

- [x] CORE-01：module/目录/许可/CI 骨架（含 `GOOS=js GOARCH=wasm` 编译验证 job）
- [x] OPC-01 首批：ZIP 条目索引、PartName 校验、资源预算、实际字节计数读取（`internal/opc`）
- [x] XML-01：命名空间感知扫描器 + 节点索引树（NodeRecord/ns 环境/未知子树保留/深度预算，`internal/xmlstore`）
- [x] XML-02：文本/属性补丁与结构插入（SpanPatch 冲突检测/转义/受控插入，含垂直验证单元级雏形）
- [x] OPC-02：关系图、Content Types（Override 优先）、主 Part 发现（非固定名称）、循环安全遍历（`internal/opc`）
- [x] SAVE-01：保存计划（PlannedEntry 四动作）、未变 Part 复制、CT 同源再生成、B1 哈希回归全绿（`internal/opc`）
- [x] SAVE-02：原子落盘（临时文件→Close+校验→原子替换）、失败保留旧目标、WithOverwrite/WithDurability（`internal/opc`）
- [ ] M0 垂直验证：真实语料 B1 哈希比对（**阻塞于语料收集**）
- [x] MODEL-01：Presentation 骨架（New/Open/OpenReader/Save/Write/Close/Validate）、Slide 受控句柄、revision 事务骨架、库内最小合法模板
- [x] TEXT-01：DocumentStore 增删/文档缓存扩展、Optional/FontStyle/ColorSpec、TextFrame/Paragraph/TextRun（SetPlainText/SetText/AddRun/SetFont/ResetFontProperty）、备注四 API（SpeakerNotes*）
- [x] TEXT-02：Paragraph/TextFrame.ReplaceText（跨 Run 字面替换、三格式策略、br/fld/链接边界、字素簇保护、ReplaceResult 报告、整批单事务提交）
- [x] STYLE-01：占位符 (type,idx) 规范化匹配、EffectiveFont 样式链（Run→段落→占位符/版式/母版→主题）、每属性 Value/Resolved/SourceTrace、主题色/字体解析（clrMap、sysClr、lumMod/lumOff/shade/tint、+mj-*/+mn-*）、未决/部分解析诊断与回退/Strict 语义
- [x] IMAGE-01：MediaSource 适配器与 PNG/JPEG 探测、Slide.AddPicture 四 Fit 模式（原尺寸/拉伸/Contain/Cover-crop）、媒体内容哈希去重、ReplaceImage 共享引用保护、AltText/IsDecorative
- [x] docProps(5.1)：CoreProperties/CustomProperties 读写（Optional patch 语义、Modified 保存时自动更新、core/custom.xml 缺失按需建 Part/CT/根关系、lpwstr/i4/bool/filetime 四变体）
- [x] 页面 API 收口（M2 收口）：Slide(index)/Slides()（读视图）、Layouts/LayoutRef（绑定文档，跨文档 AddSlide 返回 ErrForeignReference）、AddSlide（新建 slide Part+rId+sldId 注册、最小空闲 id、含自闭合/缺失 sldIdLst 展开）、MoveSlide（index=最终位置语义、整元素字节搬移保真）、RemoveSlide（连带 notesSlide、未知依赖 ErrUnsupportedEdit 阻止、notesMaster 保留）
- [x] 形状枚举与 AltText(8.1)（M2 收口）：Slide.Shapes()/Placeholders()（z-order 枚举，nvGrpSpPr/grpSpPr 跳过；Shape 公共面 ID/Name/Kind/AltText/IsDecorative）、AutoShape 句柄（TextBox/AutoShape 判别、TextFrame 读写、占位符 Type/Index 规范化 obj/0）、OpaqueShape 只读回退、AutoShape/PictureShape §8.1 读写（装饰标记与空串语义互斥区分，共用 shapeNode 基元）、保存往返保真
- [x] M2 代码项全部收口（QA-01 语料冒烟与真实客户端验证待语料/环境到位）

## 当前状态（M3 格式与表格，GEOM-01/TABLE-01/格式深度子集已完成）

- [x] GEOM-01：EMU 单位与换算（舍入+溢出检查）、Point/Rect/Quad 几何值类型、3×3 仿射矩阵（列向量）、xfrm 解析（负坐标合法、rot=1/60000 度顺时针、flipH/flipV）、组映射 Mgroup=T(C)·R·F·T(-C)·G（非等比缩放 G=T(off)·S·T(-chOff)、chExt 零拒绝除法）、嵌套组父矩阵左乘、Shape 接口 Bounds/WorldQuad/WorldAABB（本地框=直接父坐标 off/ext；WorldQuad=页面坐标四角）、GroupShape 正式句柄（grpSp 从 OpaqueShape 升级，Children() 组内 z-order 枚举+嵌套递归）
- [x] TABLE-01：TableShape 句柄（含 a:tbl 的图形框）与逻辑网格（gridSpan/rowSpan + hMerge/vMerge continuation 映射）、富文本 Cell（复用 TextFrame）、Merge/Unmerge（矩形与跨边界校验、**AT-09 多非空单元格默认拒绝**、可明确保留锚点文本）、RowHeight/ColumnWidth 读写、样式子集（三态区域开关 + 12 个 band/first/last 优先级矩阵 + tableStyles.xml 解析 + EffectiveCellStyle 逐属性状态，未知样式 ID → unresolved）
- [x] 格式深度子集：颜色变换全集（19 种：lumMod/lumOff/shade/tint/{red,green,blue}{Mod,Off}/satMod/satOff/hueMod/hueOff/alpha/alphaMod/alphaOff/inv/gray/comp，整数除法 val/100000）、线条系统（a:ln 全属性：w/cap/cmpd/algn/prstDash/custDash/round/bevel/miter/headEnd/tailEnd+颜色+Unknown 子元素）、段落属性全集（a:pPr 全属性+lnSpc/spcBef/spcAft+tabLst/buChar/buAutoNum/buBlip/buFont/buSzPct/buSzPts）、Run 高级属性（a:rPr baseline/spc/cap/strike/u/lang/altLang/kern/highlight/sym 等）、主题样式矩阵引用链（fillRef/lnRef/effectRef→themeMatrixEntry 沿 fmtScheme 定位）、Shape 接口扩展 Line/StyleMatrixRefs；起步解析 R 档，未知项→Unknown 字段或诊断
- [x] M4 配音功能包：MEDIA-01 媒体探测（WAV chunk 遍历/MP3 ID3+Xing/VBRI VBR/三级时长来源）；AUDIO-01 嵌入与 AudioProfile（/docProps/audio.xml 自有扩展、SHA-256 去重、未知时长拒绝）；AUDIO-02 受限播放树（纯音频 p:timing 幂等重建、复杂树 ErrTimingConflict、SetAdvanceAfter）；AUDIO-03 UpsertNarration 幂等三态 + PlanTimingSync/ApplyTimingPlan（Ej 公式、revision 校验、未知时长策略）
- [x] CHART-01（M5 首项）：受限三类图表（柱/折/饼）——`Slide.AddChart` 创建 chart Part + 嵌入工作簿 + 双侧关系 + graphicFrame；`ChartWorkbookBuilder` 可替换工作簿适配接口（默认纯 Go 最小 xlsx，inlineStr，无第三方依赖）；`ChartShape.Data()` 读取缓存（标题/类别/系列，支持 strRef/numRef/字面量与组合图取首组）；`SetData` 受限数据更新——缓存与工作簿同数据快照重建保证一致（AT-10 代码级等价），仅规范布局可编辑（外部/被改写图表 ErrUnsupportedEdit 拒绝部分合并，类型不可变更）
- [x] CLONE-01（M5）：同文档受限页面复制 `Slide.Clone(ClonePolicy)`——依赖闭包遍历（remap visited 防循环）、版式/母版复用、图表+嵌入工作簿**总是独立复制**（数据隔离金样）、媒体默认共享/可策略独立、外部关系逐字节保留、未知内部关系整体拒绝（ErrUnsupportedEdit，AT-13 同文档口径，零残留）；rId 保留 + Target 仅重写被克隆 Part、ShapeID/timing 节点 ID 页内作用域恒等成立；notesSlide 克隆并回引重写、AudioProfile 派生 TrackKey 落新页；验证与暂存两阶段 + 单事务提交（失败恢复暂存区）
- [x] TOOL-01（M5 收口）：**ir 包** 只读中间表示（schemaVersion="go-pptx.ir/1.0"，不嵌入媒体二进制，FromPresentation 投影核心属性、页面顺序、形状摘要与 Chart/Table/AutoShape 文本，**Unmarshal 严格 schemaVersion 校验**）+ **CLI 二进制**（cmd/pptx）：inspect / validate / replace / narrate / timing-plan / export-ir 六子命令；统一退出码 0/1/2/3/4（§23.2）；JSON 写 stdout 日志写 stderr；写命令要求 `--output`、明确 `--overwrite` 才覆盖；tracks.json schemaVersion 校验（"go-pptx.tracks/1.0"）；只读命令拒绝 `--output`
- [x] **ANIM-02（M6 首项）**：受限过渡动画基础子集——`Slide.Transition()/SetTransition(TransitionSpec)/RemoveTransition()`；白名单 8 个过渡类型（none/fade/push/wipe/split/cover/cut/dissolve）+ 容器属性 spd（slow/med/fast）+ advClick（指针 *bool 区分默认/显式关闭）+ advTm（由 SetAdvanceAfter 维护，SetTransition 保留不覆盖）；`p:transition` 子元素仅识别白名单，p14:morph 等未支持子元素整体拒绝 ErrUnsupportedEdit（不做部分合并）；新建容器按 CT_Slide 子元素序插在 cSld/clrMapOvr 之后、timing 之前；与 p:timing 树共存不破坏 timing
- [x] **VIDEO-01（M6 第二项）**：视频形状——`Slide.AddVideo(VideoSpec)` 嵌入 MP4/WebM 视频 Part + video 关系 + `p:pic/p:blipFill/p:videoFile` 片段；`internal/videoprobe` 签名探测（MP4 ISO BMFF ftyp/WebM EBML 0x1A45DFA3，DurationUnknown 时返回未知）；`VideoProfile` 落 `/docProps/video.xml`（自有扩展 `go-pptx/video/2026`，SHA-256 去重）；可选 PosterFrame 叠加占位图片；Clone 用 `profileAddsV`/`stageAppendVideoProfiles` 派生 TrackKey 重映射（AT-13 关系链整体拒绝、零残留）
- [x] **TEXT-03（M6 第三项）**：文本框高级项与字段全集——`TextFrame.BodyProps()/SetBodyProps(BodyProps)`（`Optional[T]` 语义仅 Set=true 的字段被写入/清除；`numCol/vert/anchorCtr` 三个 R 档字段，vert 6 值白名单、numCol <0 拒绝）；`Paragraph.AppendField/InsertField(at)/Fields/Remove`（`a:fld` 白名单 `slidenum/datetime`，未知类型整体 ErrUnsupportedEdit；datetime guide 17 项白名单，未识别 ErrInvalidArgument；始终生成展开 `a:fld …><a:t>` 形态）；`Field.Kind()/Guide()/Text()/SetText()/Remove()`；Paragraph.Text() 拼接 a:fld 缓存文本（a:br 仍不贡献）；runs/fld 互不渗透
- [x] **CHART-02（M6 第四项）**：图表标签/误差线/趋势线/轴扩展——`ChartDataLabel{Show,Position 白名单 11 值}`、`ChartErrorBars{Type 四值 / Direction 三值 / Value / NoEndCap}`、`ChartTrendline{Type 六值 / Period / Order / DisplayEq / DisplayRSq / Name / SetIntercept+Intercept}`、`ChartAxisOptions{CategoryAsDate / ValueLogBase ∈{0,2..32} / Min/Max Optional / Position}`；R 档白名单——Polynomial Order∈[2,6] / MovingAverage Period≥2；Pie 拒绝 ErrorBars/Trendline；`chartIsCanonical` 接受 dLbls/trendline/errBars/dateAx/logBase/min/max；`parseChartSpace` 读回全部扩展字段；SetData 保持规范不变
- [x] **LAYOUT-01（M6 第五项）**：版式/章节/嵌入字体/讲义母版/避头尾规则——R 档只读报告 `Presentation.LayoutInfo()` 返回 `LayoutReport`：① 章节 `p14:sectionLst`（Section{ID,Name,Type,SlideIDs}，unknown sldId 引用记 `layout.section.broken_ref` 诊断）；② 嵌入字体 `p:embeddedFontLst`（按 master × typeface × 4 变体 regular/bold/italic/boldItalic，关系不可达记 `layout.font.broken_ref`）；③ 讲义母版 `p:handoutMasterIdLst` + 主关系 `RelHandoutMaster`（缺目标记 `layout.handout.broken_ref`，新增 `opc.RelHandoutMaster` 常量）；④ 母版 `p:txStyles` 文本样式的 `a:lang/altLang/kumimoji/kinsoku` 按 lang 文档序聚合（KinsokuRule{Lang,AltLang,Kumimoji,KinsokuFlag,Parts}）。四项均不提供写入 API（仅解析-输出诊断）
- [x] **STYLE-02（M6 第六项）**：主题样式矩阵 + 颜色变换全集——① 颜色变换补齐到 ECMA `EG_ColorTransform` **全集 28 种**（新增绝对量 `hue/sat/lum/red/green/blue` 与曲线 `gamma/invGamma`；越界 val 约束而非回绕，`gamma g<=0` 视为无操作）；② 修复 `hslToRGB` 两处缺陷——`conv` 闭包写回共享 `p` 导致三通道串行污染、`hf` 误用 `/60`（量纲 0..6）应为 `/360`（0..1），使 `satMod/hueMod/hueOff/comp/hue/sat/lum` 全部 HSL 变换结果正确；③ 样式矩阵引用链新增 `a:fontRef`（`RefFont` + `ThemeFontSlot{major,minor}`，`idx` 支持 `major/minor/1/2`，解析到主题 `a:fontScheme` 的 latin/ea/cs 字体名，越界 idx 不臆造）；④ 主题条目解析为可呈现颜色 `StyleMatrixRef.ThemeColor`——`phClr` 以引用方颜色代入为基色后套用主题条目自身变换（ECMA §20.1.2.3.23），引用方颜色未解析时不臆造取值；`effectRef` 无颜色元素不产生诊断。R 档——解析并输出诊断，不提供写入 API
- [x] **GEOM-02（M6 第七项收尾）**：几何全集/效果/渐变/线条——Shape 扩展 R 档三方法 `Geometry() / Fill() / Effects()`：`GeometryInfo`（`a:prstGeom` Preset + 全部 `a:avLst/a:gd` Adjusts，`a:custGeom` gdLst 公式 + pathLst 全命令 `moveTo/lnTo/arcTo/cubicBezTo/quadBezTo/close` 与坐标）；`FillInfo`（`spPr/a:fill` 六类 noFill/solidFill/gradFill 含 gsLst 渐变停止与 lin/path 子容器、pattFill、blipFill 含 srcRect/blip@embed/dpi、grpFill，复用 `tablestyle.FillKind`）；`EffectInfo`（`spPr/a:effectLst` 或 `a:effectDag` 下 outerShdw/innerShdw/glow/softEdge/reflection/fillOverlay + `a:scene3d` camera/lightRig/backdrop + `a:sp3d` extrusionH/contourW/bevelT/B/presetMaterial）。R 档——解析失败降级为 Diagnostic，不提供写入 API
- [x] **CAP-01（M7 首项）**：capability manifest 与 JSON schema——`Presentation.Capability(sourcePath)` 返回 `CapabilityManifest`；六维状态 `Inspect/Create/Edit/Preserve/Render/Play × Supported/Partial/Unsupported/Untested`；`CapabilityFeature{Key,Name,Stage,TargetTier,Status,WorkPackage,Notes,Limits,AppliesTo}` 对应 §24 一行工作包 + §2.3 一行特性（**矩阵↔工作包追溯不断**）；schemaVersion="go-pptx.capability/1.0"（独立于 SDK 版本）；未实现 M7/M8 工作包（TOOL-02/TIMIR-01/TPL-01/DIFF-01）显式 Untested；`pptx capability` CLI（§23.2，CAP-01）
- [x] **TOOL-02（M7 第二项）**：WASM 编译目标与浏览器端只读检查工具——`wasm/check/` 包导出纯函数 `Inspect(bytes) / Capability() / Validate(bytes)`（统一返回带 `ok/error/envelope` 的 JSON 字符串，便于在 wasm/js 边界统一消费）；`cmd/pptx_check/` WASM 主入口（仅 `js && wasm` 构建约束，`syscall/js` 注册 `GoPptxCheck.{schemeVersion,inspect,capability,validate}`）；`wasm/site/{check.html,check.js}` 静态网页 UI（`FileReader` 读本地文件、`WebAssembly.instantiateStreaming` 加载 wasm、维度状态以卡片+JSON 形式展示）；`scripts/check_wasm.{sh,ps1}` 构建脚本（编译 `pptx_check.wasm` 至 `wasm/site/`，从 `$GOROOT/lib/wasm/wasm_exec.js` 拷贝 runtime）。**离线**：wasm + wasm_exec.js + check.html + check.js + smoke.cjs（同源 fetch），浏览器断网仍可用；文件不离开用户设备——无外网、无 CDN、无后端。CI 必检 `CGO_ENABLED=0 GOOS=js GOARCH=wasm go build ./...`（§26 V2.6 P1）

详情见 `docs/go-pptx-实施状态跟踪.md`。

## 目录结构（目标形态，方案 §3）

```text
go-pptx/
  *.go                 # 公共对象层（根包 pptx）
  internal/opc/        # ZIP 条目、Part URI、Content Types、关系图、流式媒体
  internal/xmlstore/   # 原始字节、token/节点跨度、命名空间环境
  internal/edit/       # 变更集、冲突检测、事务提交（后续）
  internal/style/      # 属性继承、颜色变换、表格样式（后续）
  internal/textmap/    # 文本逻辑位置与 XML 节点映射（后续）
  internal/geom/       # 单位、矩阵、边界计算（后续）
  internal/validate/   # 结构与语义规则（后续）
  render/              # 渲染接口，适配实现按需拆分（后续）
  cmd/pptx/            # inspect、validate、replace 等便捷入口（后续）
  testdata/corpus/     # 生成器与特性双标签索引
  docs/                # API 契约、兼容矩阵、ADR
```

## 开发命令

```bash
go build ./...                  # CGO_ENABLED=0 构建（CI 强制）
go vet ./...                    # 静态检查
go test ./...                   # 单元与金样测试
GOOS=js GOARCH=wasm go build ./...   # WASM 可编译性验证（方案 V2.6 §26）

# TOOL-02：构建浏览器端检查工具
./scripts/check_wasm.sh         # 或 pwsh -File scripts/check_wasm.ps1
# 产物在 wasm/site/ —— 用 python -m http.server 8080 即可访问 check.html
```
