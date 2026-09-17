package pptx

import (
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 STYLE-01（方案 §6.1/§7.1）：Run 有效字体样式解析与
// 占位符 idx/type 匹配。
//
// 解析链（每个属性族独立，取第一个显式值；最近来源优先）：
//
//	L1 run 本地字符格式 a:rPr
//	L2 段落默认字符格式 a:pPr/a:defRPr
//	L3 列表级别样式（按段落 a:pPr@lvl，缺省 0，源依次为）：
//	   - 占位符形状：版式（layout）上 type/idx 匹配占位符的
//	     txBody/a:lstStyle 中对应级别；缺失继续
//	   - 母版（slideMaster/notesMaster）上 type/idx 匹配占位符的
//	     txBody/a:lstStyle 中对应级别；缺失继续
//	   - 母版文本样式表 txStyles（titleStyle/bodyStyle/otherStyle/
//	     notesStyle，由形状占位符类型归类）中对应级别
//	   非占位符形状直接从母版 txStyles 的 otherStyle 起。
//	L4 主题缺省字体（仅字体族）：标题类占位符取 fontScheme majorFont，
//	   其余取 minorFont。
//
// 占位符匹配使用规范化键：ph@type 缺省 "obj"、ph@idx 缺省 0
// （ECMA-376 CT_Placeholder 默认值），按 (type, idx) 全等匹配；
// 不按坐标、名称或文档顺序猜测。
//
// 结果三要素：每属性携带 Value / Resolved / Trace（StyleStep 链）。
// 无法解析的属性保持 Resolved=false 并产生 STYLE_UNRESOLVED 诊断；
// 只有调用方在 ResolveContext.Fallback 显式给出回退值才采用，并
// 标记 Fallback=true（来源 SourceFallback）。ResolveContext.Strict
// 要求任何属性都解析完毕，否则返回 ErrUnresolvedStyle。
//
// 颜色解析（保留原始 ColorSpec 的同时输出可呈现 RGB）：
// schemeClr val ∈ {bg1,bg2,tx1,tx2} 先经母版 clrMap 映射到主题色名，
// 再在主题 clrScheme 同名条目中取 srgbClr / sysClr（lastClr 优先，
// 其次 windowText/window 已知表）并有序应用支持的颜色变换子元素
// （lumMod/lumOff/shade/tint，按文档序）。未知变换、未知 scheme 名、
// phClr 与缺 lastClr 的未知系统色标记为部分解析（STYLE_PARTIAL，
// Resolved=false），不臆测最终值。完整颜色变换集合（sat/hue/alpha
// 等）与样式矩阵属 STYLE-02。
//
// 主题解析沿真实关系图（slide→slideLayout→slideMaster→theme 或
// notesSlide→notesMaster→theme），读取视图包含已提交的 rels 补丁，
// 不硬编码 Part 名。

// StyleSource / StyleStep 定义在 internal/document/style，此处以 alias 暴露
// （v2.0 域搬迁）。

// StyleSource 标识 StyleStep 的来源层。
type StyleSource = style.StyleSource

// 来源层常量（alias 到 internal/document/style）。
const (
	// SourceRun 是 run 本地 rPr（L1）。
	SourceRun = style.SourceRun
	// SourceParagraphDefault 是段落默认字符格式 pPr/defRPr（L2）。
	SourceParagraphDefault = style.SourceParagraphDefault
	// SourceListStyle 是列表级别样式（占位符 lstStyle 或母版 txStyles；L3）。
	SourceListStyle = style.SourceListStyle
	// SourceTheme 是主题（fontScheme/clrScheme 展开或缺省；L4）。
	SourceTheme = style.SourceTheme
	// SourceFallback 是调用方 ResolveContext.Fallback 回退值。
	SourceFallback = style.SourceFallback
	// SourceCellExplicit 是单元格显式样式覆盖（a:tcPr；TABLE-01）。
	SourceCellExplicit = style.SourceCellExplicit
	// SourceTableStyle 是表格样式库的区域部分（tableStyles.xml；TABLE-01）。
	SourceTableStyle = style.SourceTableStyle
)

// StyleStep 是单个属性的一个解析来源步。
type StyleStep = style.StyleStep

// ResolvedValue / ResolvedColor / ResolvedFont / ResolveContext 定义在
// internal/document/style，此处以 alias 暴露（v2.0 域搬迁）。

// ResolvedValue 是单个解析属性（非颜色）的三要素载体。
type ResolvedValue[T any] = style.ResolvedValue[T]

// ResolvedColor 是颜色属性的三要素载体。
type ResolvedColor = style.ResolvedColor

// ResolvedFont 是 EffectiveFont 的逐属性解析结果（方案 §6.1）。
type ResolvedFont = style.ResolvedFont

// ResolveContext 携带 EffectiveFont 的解析上下文（方案 §6.1）。
type ResolveContext = style.ResolveContext

// EffectiveFont 解析 run 的有效字符样式（方案 §6.1 契约）。
//
// 逐属性独立沿解析链取第一个显式值；读取基于当前已提交 revision。
// 错误仅在文档关闭/句柄失效/结构错误（及 ctx.Strict 触发）时返回；
// 属性级解析不足通过字段与诊断表达，不是 error。
func (r *TextRun) EffectiveFont(ctx ResolveContext) (ResolvedFont, []Diagnostic, error) {
	doc, run, err := r.locateRun()
	if err != nil {
		return ResolvedFont{}, nil, Annotate(err, "TextRun.EffectiveFont")
	}
	env, err := r.p.styleEnv(r.part)
	if err != nil {
		return ResolvedFont{}, nil, Annotate(err, "TextRun.EffectiveFont")
	}
	docs := func(part opc.PartName) *xmlstore.XMLDocument {
		d, _ := r.p.docOf(part)
		return d
	}
	rf, diags, err := style.ResolveEffectiveFont(ctx, env, docs, doc, run, string(r.part))
	if err != nil {
		return rf, diags, err
	}
	return rf, diags, nil
}

// ---------- 解析环境与关系读取视图 ----------
