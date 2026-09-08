package pptx

import (
	"fmt"
	"strconv"
	"strings"

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

// StyleSource 标识 StyleStep 的来源层。
type StyleSource int

const (
	// SourceRun 是 run 本地 rPr（L1）。
	SourceRun StyleSource = iota
	// SourceParagraphDefault 是段落默认字符格式 pPr/defRPr（L2）。
	SourceParagraphDefault
	// SourceListStyle 是列表级别样式（占位符 lstStyle 或母版 txStyles；L3）。
	SourceListStyle
	// SourceTheme 是主题（fontScheme/clrScheme 展开或缺省；L4）。
	SourceTheme
	// SourceFallback 是调用方 ResolveContext.Fallback 回退值。
	SourceFallback
)

func (s StyleSource) String() string {
	switch s {
	case SourceRun:
		return "run"
	case SourceParagraphDefault:
		return "paragraph-default"
	case SourceListStyle:
		return "list-style"
	case SourceTheme:
		return "theme"
	case SourceFallback:
		return "fallback"
	default:
		return "unknown"
	}
}

// StyleStep 是单个属性的一个解析来源步。Trace 以最近来源在前排列；
// 颜色/字体经主题展开时在其后追加 SourceTheme 步。
type StyleStep struct {
	// Source 是该步的来源层。
	Source StyleSource
	// Part 是提供该值的 Part（空串表示 run 所在 Part）。
	Part string
	// Detail 是人类可读的补充（如 "ph type=body idx=1 lvl=0"、
	// "clrScheme accent3 lumMod 60%"）。
	Detail string
}

// ResolvedValue 是单个解析属性（非颜色）的三要素载体。
type ResolvedValue[T any] struct {
	// Value 是解析后的值；Resolved=false 时为零值。
	Value T
	// Resolved 表示沿链得到可呈现值（含回退）。
	Resolved bool
	// Fallback 表示值来自调用方回退（此时 Resolved=true）。
	Fallback bool
	// Trace 是实际贡献链（最近来源在前；含主题展开步）。
	Trace []StyleStep
}

// ResolvedColor 是颜色属性的三要素载体。
type ResolvedColor struct {
	// Spec 保留原始颜色引用（scheme:xxx 或 #RRGGBB），无论是否完全解析。
	Spec ColorSpec
	// RGB 是可呈现的 sRGB RRGGBB（仅 Resolved=true 时非空）。
	RGB string
	// Resolved 表示已得到最终可呈现 RGB。
	Resolved bool
	// Fallback 表示值来自调用方回退（此时 Resolved=true）。
	Fallback bool
	// Trace 是实际贡献链（最近来源在前；scheme 引用含主题展开步）。
	Trace []StyleStep
}

// ResolvedFont 是 EffectiveFont 的逐属性解析结果（方案 §6.1）。
type ResolvedFont struct {
	Bold          ResolvedValue[bool]
	Italic        ResolvedValue[bool]
	Size          ResolvedValue[FontSize]
	Color         ResolvedColor
	Latin         ResolvedValue[string]
	EastAsian     ResolvedValue[string]
	ComplexScript ResolvedValue[string]
}

// ResolveContext 携带 EffectiveFont 的解析上下文（方案 §6.1）。
//
// Fallback 提供逐字段回退（Optional：仅 Set=true 的字段会被采用）；
// 采用回退的属性 Resolved=true 且 Fallback=true。Strict 为 true 时，
// 任一属性未解析（含部分解析的颜色）都会使 EffectiveFont 返回
// ErrUnresolvedStyle（诊断仍随结果返回）。
type ResolveContext struct {
	Fallback FontStyle
	Strict   bool
}

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
	st := newEffState(r.p, ctx, env, doc, run)
	rf := st.resolve()
	if ctx.Strict {
		for _, name := range propNames {
			if !st.res[name] {
				st.diags = append(st.diags, Diagnostic{
					Code: "STYLE_STRICT", Severity: SeverityWarning,
					Part: string(r.part), Message: "property unresolved under strict context: " + name,
				})
			}
		}
		if len(st.diags) > 0 {
			return rf, st.diags, &OperationError{
				Op: "TextRun.EffectiveFont", Part: string(r.part),
				Message: "effective font not fully resolved (strict)",
				Err:     ErrUnresolvedStyle,
			}
		}
	}
	return rf, st.diags, nil
}

// propNames 是全部属性族名（strict 检查与诊断用，固定顺序）。
var propNames = []string{"bold", "italic", "size", "color", "latin", "ea", "cs"}

// ---------- 解析环境与关系读取视图 ----------

// styleKind 区分正文页面与备注页（样式源宿主不同）。
const (
	styleKindSlide = "slide"
	styleKindNotes = "notes"
)

// styleEnv 是从某 Part 出发可达的样式链环境（缺失环节留空）。
type styleEnv struct {
	kind   string // styleKindSlide / styleKindNotes
	layout opc.PartName
	master opc.PartName
	theme  opc.PartName
}

// styleEnv 沿真实关系图解析样式链（读取视图，含已提交 rels 补丁）：
// slide → slideLayout → slideMaster → theme；notesSlide → notesMaster → theme。
func (p *Presentation) styleEnv(part opc.PartName) (*styleEnv, error) {
	env := &styleEnv{kind: styleKindSlide}
	rels, ok, err := p.relsOf(part)
	if err != nil || !ok {
		return env, err
	}
	var layout, master opc.PartName
	for _, rel := range rels {
		if rel.Mode != opc.TargetInternal {
			continue
		}
		switch rel.Type {
		case relNotesMaster:
			env.kind = styleKindNotes
			master = rel.TargetPart
		case opc.RelSlideLayout:
			if layout == "" {
				layout = rel.TargetPart
			}
		}
	}
	env.layout = layout
	if env.kind == styleKindSlide && layout != "" {
		if lm, ok, err := p.relsOf(layout); err == nil && ok {
			for _, rel := range lm {
				if rel.Mode == opc.TargetInternal && rel.Type == opc.RelSlideMaster {
					master = rel.TargetPart
					break
				}
			}
		}
	}
	env.master = master
	if master != "" {
		env.theme = p.themeOf(master)
	}
	return env, nil
}

// themeOf 返回 Part 关系流中首个内部 theme 目标；无则空串。
func (p *Presentation) themeOf(part opc.PartName) opc.PartName {
	rels, ok, _ := p.relsOf(part)
	if !ok {
		return ""
	}
	for _, rel := range rels {
		if rel.Mode == opc.TargetInternal && rel.Type == opc.RelTheme {
			return rel.TargetPart
		}
	}
	return ""
}

// relsOf 返回 Part 的当前关系（读取视图：已提交 rels 补丁优先，其次
// 包内关系流）。返回 (nil, false, nil) 表示无关系流。补丁关系流畸形
// 时报错（不静默）。
func (p *Presentation) relsOf(part opc.PartName) ([]*opc.Relationship, bool, error) {
	rp := relsPart(part)
	var data []byte
	if b, ok := p.overrides[rp]; ok {
		data = b
	} else if p.pk.HasPart(rp) {
		var err error
		data, err = p.partBytes(rp)
		if err != nil {
			return nil, false, err
		}
	} else {
		return nil, false, nil
	}
	set, err := opc.ParseRelationships(part, data)
	if err != nil {
		return nil, false, &OperationError{
			Op: "style", Part: string(rp),
			Message: "relationships stream is malformed", Err: err,
		}
	}
	return set.All(), true, nil
}

// ---------- 占位符与段落上下文 ----------

// phKey 是占位符的规范化匹配键：type 缺省 "obj"、idx 缺省 0
// （ECMA-376 CT_Placeholder @type/@idx 默认值）。
type phKey struct {
	typ string
	idx uint32
}

// phKeyOf 读取形状（p:sp）的占位符键。非占位符返回 ok=false。
func phKeyOf(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord) (phKey, bool) {
	nvSpPr := childOfKind(doc, sp, nsPresentationML, "nvSpPr", 0)
	if nvSpPr == nil {
		return phKey{}, false
	}
	nvPr := childOfKind(doc, nvSpPr, nsPresentationML, "nvPr", 0)
	if nvPr == nil {
		return phKey{}, false
	}
	ph := childOfKind(doc, nvPr, nsPresentationML, "ph", 0)
	if ph == nil {
		return phKey{}, false
	}
	k := phKey{typ: "obj", idx: 0}
	if t, ok := ph.Attr("", "type"); ok {
		k.typ = t
	}
	if s, ok := ph.Attr("", "idx"); ok {
		if v, err := parseUint32(s); err == nil {
			k.idx = v
		}
	}
	return k, true
}

// findPlaceholderShape 在 spTree 中查找 (type, idx) 规范化匹配的占位符
// 形状；未命中返回 nil。
func findPlaceholderShape(doc *xmlstore.XMLDocument, want phKey) *xmlstore.NodeRecord {
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for _, sid := range tree.Children {
			sp := doc.Node(sid)
			if sp.Namespace != nsPresentationML || sp.Local() != "sp" {
				continue
			}
			got, ok := phKeyOf(doc, sp)
			if !ok || got != want {
				continue
			}
			return sp
		}
	}
	return nil
}

// ancestorShape 返回 run 最近的 p:sp 祖先；找不到返回 nil（形状位于
// 图形框架/表格等其它容器时，按无占位符普通文本处理）。
func ancestorShape(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for cur := doc.Node(run.Parent); cur != nil; cur = doc.Node(cur.Parent) {
		if cur.Namespace == nsPresentationML && cur.Local() == "sp" {
			return cur
		}
	}
	return nil
}

// runPara 返回 run 的父段落（a:p）；异常时返回 nil。
func runPara(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if run.Parent == xmlstore.NoNode {
		return nil
	}
	n := doc.Node(run.Parent)
	if n.Namespace == nsDrawingML && n.Local() == "p" {
		return n
	}
	return nil
}

// paraLevel 返回段落列表级别 0..8（a:pPr@lvl，缺省 0；越界收敛到 8）。
func paraLevel(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) int {
	pPr := childOfKind(doc, para, nsDrawingML, "pPr", 0)
	if pPr == nil {
		return 0
	}
	s, ok := pPr.Attr("", "lvl")
	if !ok {
		return 0
	}
	v, err := parseUint32(s)
	if err != nil {
		return 0
	}
	if v > 8 {
		return 8
	}
	return int(v)
}

// textClass 是母版文本样式表的归类键（对应 p:titleStyle/bodyStyle/
// otherStyle/notesStyle）。
type textClass string

const (
	classTitle textClass = "title"
	classBody  textClass = "body"
	classOther textClass = "other"
	classNotes textClass = "notes"
)

// classOf 由占位符类型归类（ECMA 语义：title/ctrTitle → 标题样式，
// 其余占位符 → 正文样式）。phOk=false（非占位符形状）→ otherStyle；
// 备注页统一为 notesStyle。
func classOf(k phKey, phOk bool, kind string) textClass {
	if kind == styleKindNotes {
		return classNotes
	}
	if !phOk {
		return classOther
	}
	switch k.typ {
	case "title", "ctrTitle":
		return classTitle
	default:
		return classBody
	}
}

// ---------- 主题与母版读取 ----------

// themeDoc 解析主题 Part 文档（nil 表示不可达）。
func (p *Presentation) themeDoc(env *styleEnv) *xmlstore.XMLDocument {
	if env.theme == "" {
		return nil
	}
	doc, err := p.docOf(env.theme)
	if err != nil {
		return nil
	}
	return doc
}

// masterDoc 解析母版 Part 文档（slideMaster 或 notesMaster；nil 不可达）。
func (p *Presentation) masterDoc(env *styleEnv) *xmlstore.XMLDocument {
	if env.master == "" {
		return nil
	}
	doc, err := p.docOf(env.master)
	if err != nil {
		return nil
	}
	return doc
}

// masterClrMap 读取母版 p:clrMap 的属性（bg1/bg2/tx1/tx2/hlink 等 →
// 主题色名）。无 clrMap 返回空表。
func masterClrMap(doc *xmlstore.XMLDocument) map[string]string {
	out := make(map[string]string)
	if doc == nil {
		return out
	}
	for _, cid := range doc.Root().Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML || c.Local() != "clrMap" {
			continue
		}
		for _, a := range c.Attrs {
			if a.Namespace == "" {
				out[a.RawName] = a.Value
			}
		}
	}
	return out
}

// textStyleNode 返回母版 txStyles 中 class 对应的样式节点
// （CT_TextListStyle：直接含 a:lvlNpPr）。无 txStyles 或无该 class
// 返回 nil。
func textStyleNode(doc *xmlstore.XMLDocument, class textClass) *xmlstore.NodeRecord {
	if doc == nil {
		return nil
	}
	root := doc.Root()
	if root == nil {
		return nil
	}
	local := string(class) + "Style"
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML {
			continue
		}
		if c.Local() == "txStyles" {
			return childOfKind(doc, c, nsPresentationML, local, 0)
		}
	}
	return nil
}

// defRPrAtLevel 从文本样式节点（CT_TextListStyle）取 lvl（0..8）的
// a:lvlNpPr/a:defRPr；该级缺失或该级无 defRPr 返回 nil。
func defRPrAtLevel(doc *xmlstore.XMLDocument, style *xmlstore.NodeRecord, lvl int) *xmlstore.NodeRecord {
	if style == nil {
		return nil
	}
	lvlName := "lvl" + strconv.Itoa(lvl+1) + "pPr"
	pPr := childOfKind(doc, style, nsDrawingML, lvlName, 0)
	if pPr == nil {
		return nil
	}
	return childOfKind(doc, pPr, nsDrawingML, "defRPr", 0)
}

// lstStyleOf 返回占位符形状 txBody 的 a:lstStyle（无则 nil）。
func lstStyleOf(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	tx := childOfKind(doc, sp, nsPresentationML, "txBody", 0)
	if tx == nil {
		return nil
	}
	return childOfKind(doc, tx, nsDrawingML, "lstStyle", 0)
}

// themeFontFace 返回主题 fontScheme 中 major/minor 字体族 kind
// （latin/ea/cs）的 typeface；元素缺失返回 ok=false。
func themeFontFace(tdoc *xmlstore.XMLDocument, major bool, kind string) (string, bool) {
	if tdoc == nil {
		return "", false
	}
	root := tdoc.Root()
	if root == nil {
		return "", false
	}
	elems := childOfKind(tdoc, root, nsDrawingML, "themeElements", 0)
	if elems == nil {
		return "", false
	}
	scheme := childOfKind(tdoc, elems, nsDrawingML, "fontScheme", 0)
	if scheme == nil {
		return "", false
	}
	name := "minorFont"
	if major {
		name = "majorFont"
	}
	f := childOfKind(tdoc, scheme, nsDrawingML, name, 0)
	if f == nil {
		return "", false
	}
	kindNode := childOfKind(tdoc, f, nsDrawingML, kind, 0)
	if kindNode == nil {
		return "", false
	}
	return kindNode.Attr("", "typeface")
}

// expandTypeface 展开主题字体引用：typeface 以 "+mj-"/"+mn-" 开头时查
// 主题 fontScheme（+mj-lt → majorFont latin 等）。无前缀返回原样且
// 无需主题（ok=true，tstep 零值）。kind ∈ {latin,ea,cs}。
func expandTypeface(tdoc *xmlstore.XMLDocument, typeface, kind string) (final string, tstep StyleStep, ok bool) {
	major, isRef := themeFontScheme(typeface)
	if !isRef {
		return typeface, StyleStep{}, true
	}
	if tdoc == nil {
		return "", StyleStep{}, false
	}
	final, found := themeFontFace(tdoc, major, kind)
	if !found {
		return "", StyleStep{}, false
	}
	return final, StyleStep{Source: SourceTheme,
		Detail: "fontScheme " + fontSchemeName(major) + " " + kind}, true
}

// themeFontScheme 判断 "+mj-*"/"+mn-*" 引用指向 major 还是 minor。
func themeFontScheme(typeface string) (major, ok bool) {
	switch {
	case strings.HasPrefix(typeface, "+mj-"):
		return true, true
	case strings.HasPrefix(typeface, "+mn-"):
		return false, true
	}
	return false, false
}

func fontSchemeName(major bool) string {
	if major {
		return "majorFont"
	}
	return "minorFont"
}

// ---------- 颜色解析 ----------

// applyColorTransform 应用单个颜色变换（百分比 val 值域 0..100000；
// ST_TransformEffect 千分比）。返回 ok=false 表示未知变换。
func applyColorTransform(channel uint8, local, val string) (uint8, bool) {
	pct, err := parseUint32(val)
	if err != nil || pct > 100000 {
		return channel, false
	}
	var n int64
	switch local {
	case "lumMod":
		n = int64(channel) * int64(pct) / 100000 // R' = R × pct
	case "shade":
		n = int64(channel) * (100000 - int64(pct)) / 100000 // 向黑混合 pct
	case "lumOff":
		n = int64(channel) + 255*int64(pct)/100000 // R' = R + 255×pct
	case "tint":
		n = int64(channel) + (255-int64(channel))*int64(pct)/100000 // 向白混合 pct
	default:
		return channel, false
	}
	if n < 0 {
		n = 0
	}
	if n > 255 {
		n = 255
	}
	return uint8(n), true
}

// 已知系统色（sysClr）映射表；有 lastClr 属性时优先于本表。
var sysColorFallback = map[string]string{
	"windowtext": "000000",
	"window":     "FFFFFF",
}

// schemeRGB 解析主题 clrScheme 条目的可呈现 RGB。partial=true 表示
// 条目存在但无法完全解析（未知变换/未知系统色/未知 scheme/非
// srgbClr|sysClr 形态）；此时 rgb 可能为基础色（可部分参考）。
func schemeRGB(tdoc *xmlstore.XMLDocument, scheme string) (rgb string, partial bool) {
	if tdoc == nil {
		return "", true
	}
	root := tdoc.Root()
	if root == nil {
		return "", true
	}
	elems := childOfKind(tdoc, root, nsDrawingML, "themeElements", 0)
	if elems == nil {
		return "", true
	}
	cs := childOfKind(tdoc, elems, nsDrawingML, "clrScheme", 0)
	if cs == nil {
		return "", true
	}
	entry := childOfKind(tdoc, cs, nsDrawingML, scheme, 0)
	if entry == nil {
		return "", true // 未知 scheme 名
	}
	var colorNode *xmlstore.NodeRecord
	for _, cid := range entry.Children {
		c := tdoc.Node(cid)
		if c.Namespace == nsDrawingML {
			colorNode = c
			break
		}
	}
	if colorNode == nil {
		return "", true
	}
	base := ""
	switch colorNode.Local() {
	case "srgbClr":
		v, _ := colorNode.Attr("", "val")
		base = strings.ToUpper(v)
	case "sysClr":
		v, _ := colorNode.Attr("", "val")
		if lc, ok := colorNode.Attr("", "lastClr"); ok && lc != "" {
			base = strings.ToUpper(lc)
		} else if m, known := sysColorFallback[strings.ToLower(v)]; known {
			base = m
		} else {
			return "", true
		}
	default:
		return "", true
	}
	if !isHexRGB(base) {
		return "", true
	}
	rgb = base
	for _, cid := range colorNode.Children {
		c := tdoc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		v, _ := c.Attr("", "val")
		rr := hexByte(rgb[0:2])
		gg := hexByte(rgb[2:4])
		bb := hexByte(rgb[4:6])
		var ok bool
		if rr, ok = applyColorTransform(rr, c.Local(), v); !ok {
			return base, true // 未知变换：保留基础色并标记部分解析
		}
		gg, _ = applyColorTransform(gg, c.Local(), v)
		bb, _ = applyColorTransform(bb, c.Local(), v)
		rgb = fmt.Sprintf("%02X%02X%02X", rr, gg, bb)
	}
	return rgb, false
}

func isHexRGB(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func hexByte(s string) uint8 {
	v, _ := strconv.ParseUint(s, 16, 8)
	return uint8(v)
}

// ---------- 逐属性解析状态 ----------

// famSource 是解析链上的一层 rPr（run rPr / pPr defRPr / 列表级 defRPr）。
type famSource struct {
	style FontStyle
	step  StyleStep
}

// effState 承载一次 EffectiveFont 的全部中间状态。
type effState struct {
	ctx ResolveContext
	env *styleEnv
	p   *Presentation
	doc *xmlstore.XMLDocument
	run *xmlstore.NodeRecord

	class   textClass
	lvl     int
	phKey   phKey
	isPh    bool
	sources []famSource // L1..L3
	res     map[string]bool
	diags   []Diagnostic
}

// newEffState 收集解析链上下文与 L1..L3 源。
func newEffState(p *Presentation, ctx ResolveContext, env *styleEnv, doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *effState {
	st := &effState{
		ctx: ctx, env: env, p: p, doc: doc, run: run,
		res: make(map[string]bool),
	}
	// 占位符与 class。
	if sp := ancestorShape(doc, run); sp != nil {
		if k, ok := phKeyOf(doc, sp); ok {
			st.phKey, st.isPh = k, true
		}
	}
	st.class = classOf(st.phKey, st.isPh, env.kind)
	// 级别（L3 需要）。
	var para *xmlstore.NodeRecord
	if para = runPara(doc, run); para != nil {
		st.lvl = paraLevel(doc, para)
	}
	// L1 run rPr。
	if rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0); rPr != nil {
		st.sources = append(st.sources, famSource{
			style: parseLocalFont(doc, rPr),
			step:  StyleStep{Source: SourceRun, Detail: "run rPr"},
		})
	}
	// L2 段落默认字符。
	if para != nil {
		if pPr := childOfKind(doc, para, nsDrawingML, "pPr", 0); pPr != nil {
			if d := childOfKind(doc, pPr, nsDrawingML, "defRPr", 0); d != nil {
				st.sources = append(st.sources, famSource{
					style: parseLocalFont(doc, d),
					step:  StyleStep{Source: SourceParagraphDefault, Detail: "pPr defRPr"},
				})
			}
		}
	}
	st.appendListSources()
	return st
}

// appendListSources 按形状占位符状态追加 L3 源：
//   - 占位符：layout 占位符 lstStyle → master 占位符 lstStyle →
//     master txStyles[class]，逐级取 lvl 的 defRPr；
//   - 非占位符：master txStyles[otherStyle]。
func (st *effState) appendListSources() {
	p := st.p
	lvl := strconv.Itoa(st.lvl)
	if st.isPh {
		if st.env.layout != "" {
			if ldoc, err := p.docOf(st.env.layout); err == nil {
				if sp := findPlaceholderShape(ldoc, st.phKey); sp != nil {
					if d := defRPrAtLevel(ldoc, lstStyleOf(ldoc, sp), st.lvl); d != nil {
						st.sources = append(st.sources, famSource{
							style: parseLocalFont(ldoc, d),
							step: StyleStep{Source: SourceListStyle, Part: string(st.env.layout),
								Detail: st.phDetail() + " lvl=" + lvl + " layout lstStyle"},
						})
					}
				}
			}
		}
		if st.env.master != "" {
			if mdoc := p.masterDoc(st.env); mdoc != nil {
				if sp := findPlaceholderShape(mdoc, st.phKey); sp != nil {
					if d := defRPrAtLevel(mdoc, lstStyleOf(mdoc, sp), st.lvl); d != nil {
						st.sources = append(st.sources, famSource{
							style: parseLocalFont(mdoc, d),
							step: StyleStep{Source: SourceListStyle, Part: string(st.env.master),
								Detail: st.phDetail() + " lvl=" + lvl + " master lstStyle"},
						})
					}
				}
			}
		}
	}
	if st.env.master != "" {
		if mdoc := p.masterDoc(st.env); mdoc != nil {
			if ts := textStyleNode(mdoc, st.class); ts != nil {
				if d := defRPrAtLevel(mdoc, ts, st.lvl); d != nil {
					st.sources = append(st.sources, famSource{
						style: parseLocalFont(mdoc, d),
						step: StyleStep{Source: SourceListStyle, Part: string(st.env.master),
							Detail: "txStyles " + string(st.class) + "Style lvl=" + lvl},
					})
				}
			}
		}
	}
}

// phDetail 生成占位符描述（Detail 用）。
func (st *effState) phDetail() string {
	return "ph type=" + st.phKey.typ + " idx=" + strconv.FormatUint(uint64(st.phKey.idx), 10)
}

// note 记录属性族是否解析成功。
func (st *effState) note(name string, resolved bool) { st.res[name] = resolved }

func (st *effState) unresolvedDiag(name string) {
	st.note(name, false)
	st.diags = append(st.diags, Diagnostic{
		Code: "STYLE_UNRESOLVED", Severity: SeverityWarning,
		Message: "property unresolved: " + name,
	})
}

// pickFirst 返回链上第一个显式值及其来源。
func pickFirst[T any](sources []famSource, pick func(FontStyle) Optional[T]) (T, StyleStep, bool) {
	for _, s := range sources {
		if v := pick(s.style); v.Set {
			return v.Value, s.step, true
		}
	}
	var zero T
	return zero, StyleStep{}, false
}

// fallbackOrUnresolved 处理链上无值的属性：调用方回退或标记未决。
func fallbackOrUnresolved[T any](st *effState, name string, fb func(ResolveContext) Optional[T]) ResolvedValue[T] {
	if f := fb(st.ctx); f.Set {
		st.note(name, true)
		return ResolvedValue[T]{Value: f.Value, Resolved: true, Fallback: true,
			Trace: []StyleStep{{Source: SourceFallback, Detail: "ctx.Fallback"}}}
	}
	st.unresolvedDiag(name)
	return ResolvedValue[T]{}
}

// resolve 执行逐属性解析（顺序即 propNames）。
func (st *effState) resolve() ResolvedFont {
	var rf ResolvedFont
	rf.Bold = st.resolveBool("bold", func(f FontStyle) Optional[bool] { return f.Bold },
		func(c ResolveContext) Optional[bool] { return c.Fallback.Bold })
	rf.Italic = st.resolveBool("italic", func(f FontStyle) Optional[bool] { return f.Italic },
		func(c ResolveContext) Optional[bool] { return c.Fallback.Italic })
	rf.Size = st.resolveSize()
	rf.Color = st.resolveColor()
	rf.Latin = st.resolveTypeface("latin", "latin", func(f FontStyle) Optional[string] { return f.Latin },
		func(c ResolveContext) Optional[string] { return c.Fallback.Latin })
	rf.EastAsian = st.resolveTypeface("ea", "ea", func(f FontStyle) Optional[string] { return f.EastAsian },
		func(c ResolveContext) Optional[string] { return c.Fallback.EastAsian })
	rf.ComplexScript = st.resolveTypeface("cs", "cs", func(f FontStyle) Optional[string] { return f.ComplexScript },
		func(c ResolveContext) Optional[string] { return c.Fallback.ComplexScript })
	return rf
}

// resolveBool 解析布尔属性（粗体/斜体）。
func (st *effState) resolveBool(name string, pick func(FontStyle) Optional[bool], fb func(ResolveContext) Optional[bool]) ResolvedValue[bool] {
	if v, step, ok := pickFirst(st.sources, pick); ok {
		st.note(name, true)
		return ResolvedValue[bool]{Value: v, Resolved: true, Trace: []StyleStep{step}}
	}
	return fallbackOrUnresolved(st, name, fb)
}

// resolveSize 解析字号。
func (st *effState) resolveSize() ResolvedValue[FontSize] {
	pick := func(f FontStyle) Optional[FontSize] { return f.Size }
	fb := func(c ResolveContext) Optional[FontSize] { return c.Fallback.Size }
	if v, step, ok := pickFirst(st.sources, pick); ok {
		st.note("size", true)
		return ResolvedValue[FontSize]{Value: v, Resolved: true, Trace: []StyleStep{step}}
	}
	return fallbackOrUnresolved(st, "size", fb)
}

// resolveTypeface 解析字体族：命中值若是主题引用（+mj-*/+mn-*）即展开；
// 全部未命中时按 class 用主题缺省字体（标题 major，其余 minor）；
// kind 为主题 fontScheme 子元素名（latin/ea/cs）。
func (st *effState) resolveTypeface(name, kind string, pick func(FontStyle) Optional[string], fb func(ResolveContext) Optional[string]) ResolvedValue[string] {
	tdoc := st.p.themeDoc(st.env)
	for _, s := range st.sources {
		v := pick(s.style)
		if !v.Set {
			continue
		}
		trace := []StyleStep{s.step}
		if final, tstep, ok := expandTypeface(tdoc, v.Value, kind); ok {
			if tstep.Source != 0 || tstep.Detail != "" {
				trace = append(trace, tstep)
			}
			st.note(name, true)
			return ResolvedValue[string]{Value: final, Resolved: true, Trace: trace}
		}
		// 主题引用但无法展开（无主题/主题缺该字体）：部分解析。
		st.note(name, false)
		st.diags = append(st.diags, Diagnostic{
			Code: "STYLE_PARTIAL", Severity: SeverityWarning,
			Message: "theme typeface reference cannot be expanded: " + v.Value,
		})
		return ResolvedValue[string]{Value: v.Value, Resolved: false, Trace: trace}
	}
	// 主题缺省字体（L4）。
	if tdoc != nil {
		major := st.class == classTitle
		if face, found := themeFontFace(tdoc, major, kind); found && face != "" {
			st.note(name, true)
			return ResolvedValue[string]{Value: face, Resolved: true, Trace: []StyleStep{{
				Source: SourceTheme,
				Detail: "fontScheme default " + kind + " (" + fontSchemeName(major) + ")",
			}}}
		}
	}
	return fallbackOrUnresolved(st, name, fb)
}

// resolveColor 解析颜色：链上首个 solidFill（srgbClr 直出；schemeClr
// 经 clrMap/主题解析）。
func (st *effState) resolveColor() ResolvedColor {
	for _, s := range st.sources {
		c := s.style.Color
		if !c.Set || !c.Value.Valid() {
			continue
		}
		spec := c.Value
		trace := []StyleStep{s.step}
		if spec.RGB != "" {
			st.note("color", true)
			return ResolvedColor{Spec: spec, RGB: strings.ToUpper(spec.RGB),
				Resolved: true, Trace: trace}
		}
		if spec.Scheme == "phClr" {
			st.note("color", false)
			st.diags = append(st.diags, Diagnostic{
				Code: "STYLE_PARTIAL", Severity: SeverityWarning,
				Message: "scheme color phClr (placeholder color mapping) is not resolved by STYLE-01",
			})
			return ResolvedColor{Spec: spec, Resolved: false, Trace: trace}
		}
		scheme := spec.Scheme
		if clrMapIndirect(scheme) {
			m := masterClrMap(st.p.masterDoc(st.env))
			mapped, ok := m[scheme]
			if !ok {
				st.note("color", false)
				st.diags = append(st.diags, Diagnostic{
					Code: "STYLE_PARTIAL", Severity: SeverityWarning,
					Message: "clrMap has no entry for scheme color " + scheme,
				})
				return ResolvedColor{Spec: spec, Resolved: false, Trace: trace}
			}
			trace = append(trace, StyleStep{Source: SourceTheme,
				Detail: "clrMap " + scheme + " → " + mapped})
			scheme = mapped
		}
		tdoc := st.p.themeDoc(st.env)
		rgb, partial := schemeRGB(tdoc, scheme)
		if partial {
			st.note("color", false)
			st.diags = append(st.diags, Diagnostic{
				Code: "STYLE_PARTIAL", Severity: SeverityWarning,
				Message: "theme color cannot be fully resolved: " + spec.Scheme,
			})
			if rgb != "" {
				return ResolvedColor{Spec: spec, RGB: rgb, Resolved: false, Trace: trace}
			}
			return ResolvedColor{Spec: spec, Resolved: false, Trace: trace}
		}
		trace = append(trace, StyleStep{Source: SourceTheme,
			Detail: "clrScheme " + scheme + " → #" + rgb})
		st.note("color", true)
		return ResolvedColor{Spec: spec, RGB: rgb, Resolved: true, Trace: trace}
	}
	// 未命中：回退。
	if f := st.ctx.Fallback.Color; f.Set && f.Value.Valid() {
		st.note("color", true)
		spec := f.Value
		rc := ResolvedColor{Spec: spec, Resolved: true, Fallback: true,
			Trace: []StyleStep{{Source: SourceFallback, Detail: "ctx.Fallback"}}}
		if spec.RGB != "" {
			rc.RGB = strings.ToUpper(spec.RGB)
		}
		return rc
	}
	st.unresolvedDiag("color")
	return ResolvedColor{}
}

// clrMapIndirect 报告 scheme 名是否为 clrMap 间接引用（映射到主题色）。
func clrMapIndirect(scheme string) bool {
	switch scheme {
	case "bg1", "bg2", "tx1", "tx2":
		return true
	}
	return false
}
