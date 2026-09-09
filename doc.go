// Package pptx 是 go-pptx 组件的公共入口（module 根包，方案 §3）。
//
// 本包提供 Presentation → Slide → Shape → TextFrame → Paragraph → Run
// 的对象模型，目标是提供类似 python-pptx 的使用体验，并满足模板报告生成、
// 已有文档精细编辑与带配音 PPT 合成三类场景（方案 §1）。
//
// 已交付（M0+M1）：稳定错误码/ID/诊断类型（§12.1、§20.4）；
// Presentation 骨架（MODEL-01）——New/Open/OpenReader/Save/Write/
// Close/Validate、Slide 受控句柄、ErrClosed/ErrStaleHandle 语义、
// revision 事务骨架（隐式事务 stage/commit、保存计划快照与
// ErrConcurrentModification 守卫）、库内最小合法模板。
// 已交付（M2 文本模板 MVP）：富文本与备注（TEXT-01）、跨 Run 替换
// （TEXT-02）、有效样式解析（STYLE-01）、图片与媒体（IMAGE-01）、
// 文档级元数据（docProps 5.1）、页面 API（Slide(index)/Slides/Layouts/
// AddSlide/MoveSlide/RemoveSlide/Shapes/Placeholders，§20.1）与
// 无障碍替代文本（AltText 8.1，PictureShape/AutoShape）。
// 已交付（M3）：单位与几何（GEOM-01，§5.2/§8）——EMU 单位换算、
// Point/Rect/Quad 值类型、3×3 仿射矩阵（列向量）、组映射
// Mgroup=T(C)·R·F·T(-C)·G 与嵌套组左乘、Shape 的 Bounds/WorldQuad/
// WorldAABB、GroupShape.Children；表格（TABLE-01，§9.1）——TableShape
// 与逻辑网格、富文本 Cell、Merge/Unmerge（AT-09 多非空默认拒绝）、
// 行高列宽、样式子集与 EffectiveCellStyle；格式深度子集——颜色变换
// 全集（19 种）、线条系统、段落属性全集、Run 高级属性、主题样式
// 矩阵引用链与 themeMatrixEntry；配音（M4）——媒体探测、音频嵌入、
// 受限播放树与计时计划；图表（CHART-01，M5 首项）——受限三类
// （柱/折/饼）创建、ChartWorkbookBuilder 工作簿适配、缓存读取与
// 受限 SetData（缓存与工作簿一致重建）；页面复制（CLONE-01）——
// 同文档受限克隆（依赖闭包 + Part 映射 + 关系重写，图表数据隔离，
// 未知关系整体拒绝）；CLI 与 IR（TOOL-01）——只读中间表示
// ir.FromPresentation（schemaVersion="go-pptx.ir/1.0"，不嵌入媒体）
// 与 cmd/pptx 六子命令（inspect / validate / replace / narrate /
// timing-plan / export-ir；统一退出码 0/1/2/3/4，§23.2）；
// 过渡动画（ANIM-02，M6 首项）——p:transition 受限白名单
// （none/fade/push/wipe/split/cover/cut/dissolve）、容器属性 spd 与
// advClick、p14:morph 整体拒绝、与 p:timing 共存不破坏 timing 树。
// 视频形状（VIDEO-01，M6 第二项）——Slide.AddVideo 嵌入 MP4/WebM
// 视频 Part + video 关系 + p:pic/p:blipFill/p:videoFile 片段；
// VideoProfile 落 /docProps/video.xml；可选 PosterFrame 叠加占位图片。
// 文本框高级项与字段全集（TEXT-03，M6 第三项）——a:bodyPr 的
// numCol/vert/anchorCtr（BodyProps Optional 语义）；a:fld 白名单
// 字段 slidenum/datetime（Paragraph.AppendField/InsertField/Fields/
// Remove/SetText/Kind/Guide），未知字段类型与 datetime 未识别格式
// 整体拒绝；Paragraph.Text() 拼接 a:fld 缓存文本。
// 图表扩展（CHART-02，M6 第四项）——图表级数据标签 c:dLbls
// （ChartDataLabel{Show, Position 白名单}）；系列级误差线 c:errBars
// （ChartErrorBars，4 类型 + Direction/NoEndCap，Pie 拒绝）；系列级
// 趋势线 c:trendline（ChartTrendline，6 类型 + DisplayEq/DisplayRSq/
// SetIntercept，Polynomial Order∈[2,6] / MovingAverage Period≥2）；
// 轴扩展（ChartAxisOptions，ValueLogBase ∈ {0,2..32}，Min/Max，
// CategoryAsDate 切换 c:dateAx 替代 c:catAx）。R 档白名单严格——
// 越界即 ErrInvalidArgument。
// 版式/章节/嵌入字体/讲义母版/避头尾规则（LAYOUT-01，M6 第五项）——
// Presentation.LayoutInfo() R 档只读报告：解析 p14:sectionLst 章节
// （含 unknown sldId 引用诊断）、p:embeddedFontLst 嵌入字体（master ×
// typeface × 4 变体 regular/bold/italic/boldItalic，关系不可达记
// layout.font.broken_ref 诊断）、p:handoutMasterIdLst + 主关系
// RelHandoutMaster 讲义母版（缺关系目标记 layout.handout.broken_ref）、
// 母版 p:txStyles 文本样式的 a:lang/altLang/kumimoji/kinsoku 聚合
// （KinsokuRule 按 lang 文档序聚合）。四项均不提供写入 API。
// 主题样式矩阵与颜色变换全集（STYLE-02，M6 第六项）——颜色变换补齐到
// ECMA EG_ColorTransform 全集 28 种（新增绝对量 hue/sat/lum/red/green/
// blue 与曲线 gamma/invGamma）；修复 hslToRGB 两处缺陷（conv 闭包写回
// 共享 p 导致通道串行污染；hf 误用 /60 应为 /360，量纲 0..6 而非 0..1），
// 使全部 HSL 变换（satMod/hueMod/hueOff/comp/hue/sat/lum）结果正确；
// 样式矩阵引用链新增 a:fontRef（RefFont + ThemeFontSlot major/minor，
// 解析到主题 a:fontScheme 的 latin/ea/cs 字体名），并把主题条目解析为
// 可呈现颜色（StyleMatrixRef.ThemeColor，phClr 以引用方颜色代入后套用
// 主题条目自身变换）。R 档——解析并输出诊断，不提供写入 API。
// 几何/填充/效果 R 档报告（GEOM-02，M6 第七项收尾）——Shape 扩展三方法
// Geometry() / Fill() / Effects()，均走 R 档只读报告；GeometryInfo 覆盖
// a:prstGeom 预设全集与 a:custGeom 自定义路径（gdLst 公式 / pathLst 命令
// moveTo/lnTo/arcTo/cubicBezTo/quadBezTo/close 与坐标）；FillInfo 覆盖
// spPr/a:fill 六类（noFill/solidFill/gradFill 含 gsLst 渐变停止与 lin/path
// 子容器/pattFill/blipFill 含 srcRect/blip@embed 与 dpi/grpFill），复用
// tablestyle.go 的 FillKind（FillUnspecified/FillNone/FillSolid/FillGradient/
// FillPattern/FillPicture/FillGroup）；EffectInfo 覆盖 spPr/a:effectLst 与
// a:effectDag 容器下 outerShdw/innerShdw/glow/softEdge/reflection/
// fillOverlay + a:scene3d（camera/lightRig/backdrop）+ a:sp3d（extrusionH/
// contourW/bevelT/bevelB/presetMaterial）。R 档——解析失败降级为
// Diagnostic；不提供写入 API。
// Capability manifest（CAP-01，M7 首项）——Presentation.Capability(
// sourcePath) 返回 CapabilityManifest（六维状态 Inspect/Create/Edit/
// Preserve/Render/Play × Supported/Partial/Unsupported/Untested，与
// SaveReport 同事实来源）；CapabilityFeature 对应 §24 一行工作包 + §2.3
// 一行特性（矩阵↔工作包追溯不断）；schemaVersion="go-pptx.capability/1.0"
// （独立于 SDK 版本）；已落地但有已知限制的工作包
// （TOOL-02/TIMIR-01/TPL-01/DIFF-01）标 Partial 并在 Limits 登记；
// `pptx capability` CLI 入口（§23.2，CAP-01）。
// WASM 浏览器端检查工具（TOOL-02，M7 第二项）——`wasm/check` 包导出
// 纯函数 Inspect/Capability/Validate（统一返回带 ok/error envelope 的
// JSON 字符串，便于 wasm/js 边界消费）；`cmd/pptx_check` WASM 主入口
// （仅 `js && wasm` 构建约束，syscall/js 注册 GoPptxCheck.{schemeVersion,
// inspect, capability, validate}）；`wasm/site/{check.html,check.js,
// wasm_exec.js,pptx_check.wasm,smoke.cjs}` 静态网页 UI 与 node.js
// 烟雾测试；`scripts/check_wasm.{sh,ps1}` 构建脚本。文件不离开用户设备
// ——无外网、无 CDN、无后端；离线加载即可使用（V2.6 §23.2 + §26）。
// 模板数据绑定引擎（TPL-01，M7 第四项，方案 §2.4 / ADR 013）——
// Presentation.Bind(data) 按数据源渲染模板标记：内联占位符
// `{{path}}`（复用 TEXT-02 跨 Run 保真替换）、条件段落
// `{{#if}}…{{/if}}`、表格行循环 `{{#each}}…{{/each}}`（模板行按数据
// 条数复制，条目为作用域）、图表数据绑定（数据源中值为 ChartData 且
// 键等于图表形状名）。两阶段：plan 纯读取校验（失败不落任何补丁）+
// apply 按 Part 聚合、单事务提交（无部分写入）。
// 动画时序只读 IR（TIMIR-01，M7 第三项，方案 §21.5）——
// Slide.TimingTreeRaw() 返回 p:timing 原始字节；ir 包把 p:timing →
// tnLst → par/cTn 树投影为只读时序 IR（PageTiming/TimingNode：Kind
// 并行/序列/cTn/audio/video/cmd/anim/set/opaque，触发条件 Begin/End，
// 动画目标 Target）；未识别子元素输出 OpaqueNode 并计入诊断（不猜
// 测、不省略）。R 档——不提供动画编辑 API；估计值 Duration 不承诺与
// Office 客户端播放帧一致，且不得作为 AdvanceAfter 计算输入（§21.5）。
// 语义 diff 与审计（DIFF-01，M8 首项，方案 §18.3 / §24）——ir.Diff
// 以 IR 为输入视图比较两份文档：页面对齐用加权 LCS（SlideID 强匹配 /
// 形状 ID Jaccard 相似度，乱序残留二次配对），页内形状按 ShapeID 配对
// 逐字段比较（文本/几何/类型/名称/表格尺寸/图表类型，组递归），备注与
// 动画时序摘要单独成项；未识别区域聚合为 opaque diff（Part + NodePath
// 定位可回溯，不猜测内部变化）。报告 schemaVersion
// "go-pptx.diff/1.0"；`pptx diff` CLI 入口（只读）。
// 设计基线：《go-pptx 完整设计方案 V2.6 开发实施版》。
package pptx
