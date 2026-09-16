package pptx

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/style"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 STYLE-01 的**有效样式解析状态机**：effState 与逐属性族解析
// （bold/italic/size/color/typeface），产出 ResolvedFont/ResolvedColor。

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

	class   style.TextClass
	lvl     int
	phKey   style.PhKey
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
	if sp := style.AncestorShape(doc, run); sp != nil {
		if k, ok := style.PhKeyOf(doc, sp); ok {
			st.phKey, st.isPh = k, true
		}
	}
	st.class = style.ClassOf(st.phKey, st.isPh, env.kind)
	// 级别（L3 需要）。
	var para *xmlstore.NodeRecord
	if para = style.RunPara(doc, run); para != nil {
		st.lvl = style.ParaLevel(doc, para)
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
				if sp := style.FindPlaceholderShape(ldoc, st.phKey); sp != nil {
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
				if sp := style.FindPlaceholderShape(mdoc, st.phKey); sp != nil {
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
	return "ph type=" + st.phKey.Typ + " idx=" + strconv.FormatUint(uint64(st.phKey.Idx), 10)
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
		major := st.class == style.ClassTitle
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
