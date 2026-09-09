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
// 设计基线：《go-pptx 完整设计方案 V2.6 开发实施版》。
package pptx
