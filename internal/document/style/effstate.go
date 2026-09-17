package style

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/v2/internal/diag"
	"github.com/F31/go-pptx/v2/internal/document/model"
	"github.com/F31/go-pptx/v2/internal/errs"
	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/opc"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是 STYLE-01 的**有效样式解析状态机**：逐属性族解析
// （bold/italic/size/color/typeface），产出 ResolvedFont/ResolvedColor。
// 演示文稿存储访问通过注入的 DocFunc 解耦。

// DocFunc 返回 Part 的 XML 文档（不可达/出错返回 nil）。由门面注入。
type DocFunc func(part opc.PartName) *xmlstore.XMLDocument

// propNames 是全部属性族名（strict 检查与诊断用，固定顺序）。
var propNames = []string{"bold", "italic", "size", "color", "latin", "ea", "cs"}

// famSource 是解析链上的一层 rPr（run rPr / pPr defRPr / 列表级 defRPr）。
type famSource struct {
	style FontStyle
	step  StyleStep
}

// effState 承载一次有效样式解析的全部中间状态。
type effState struct {
	ctx  ResolveContext
	env  *Env
	docs DocFunc
	doc  *xmlstore.XMLDocument
	run  *xmlstore.NodeRecord

	class   TextClass
	lvl     int
	phKey   PhKey
	isPh    bool
	sources []famSource // L1..L3
	res     map[string]bool
	diags   []diag.Diagnostic
}

// ResolveEffectiveFont 解析 run 的有效字符样式（方案 §6.1 契约）。
//
// 逐属性独立沿解析链取第一个显式值。错误仅在文档不可达 / 结构错误
// （及 ctx.Strict 触发）时返回；属性级解析不足通过字段与诊断表达。
func ResolveEffectiveFont(ctx ResolveContext, env *Env, docs DocFunc,
	doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord, part string) (ResolvedFont, []diag.Diagnostic, error) {
	st := newEffState(ctx, env, docs, doc, run)
	rf := st.resolve()
	if ctx.Strict {
		for _, name := range propNames {
			if !st.res[name] {
				st.diags = append(st.diags, diag.Diagnostic{
					Code: "STYLE_STRICT", Severity: diag.SeverityWarning,
					Part: part, Message: "property unresolved under strict context: " + name,
				})
			}
		}
		if len(st.diags) > 0 {
			return rf, st.diags, &errs.OperationError{
				Op: "TextRun.EffectiveFont", Part: part,
				Message: "effective font not fully resolved (strict)",
				Err:     errs.ErrUnresolvedStyle,
			}
		}
	}
	return rf, st.diags, nil
}

// newEffState 收集解析链上下文与 L1..L3 源。
func newEffState(ctx ResolveContext, env *Env, docs DocFunc, doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *effState {
	st := &effState{
		ctx: ctx, env: env, docs: docs, doc: doc, run: run,
		res: make(map[string]bool),
	}
	// 占位符与 class。
	if sp := AncestorShape(doc, run); sp != nil {
		if k, ok := PhKeyOf(doc, sp); ok {
			st.phKey, st.isPh = k, true
		}
	}
	st.class = ClassOf(st.phKey, st.isPh, env.Kind)
	// 级别（L3 需要）。
	var para *xmlstore.NodeRecord
	if para = RunPara(doc, run); para != nil {
		st.lvl = ParaLevel(doc, para)
	}
	// L1 run rPr。
	if rPr := xmlstore.ChildOfKind(doc, run, ooxmlns.DrawingML, "rPr", 0); rPr != nil {
		st.sources = append(st.sources, famSource{
			style: ParseLocalFont(doc, rPr),
			step:  StyleStep{Source: SourceRun, Detail: "run rPr"},
		})
	}
	// L2 段落默认字符。
	if para != nil {
		if pPr := xmlstore.ChildOfKind(doc, para, ooxmlns.DrawingML, "pPr", 0); pPr != nil {
			if d := xmlstore.ChildOfKind(doc, pPr, ooxmlns.DrawingML, "defRPr", 0); d != nil {
				st.sources = append(st.sources, famSource{
					style: ParseLocalFont(doc, d),
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
	if st.isPh {
		st.appendPlaceholderLstStyle(st.env.Layout, "layout lstStyle")
		st.appendPlaceholderLstStyle(st.env.Master, "master lstStyle")
	}
	st.appendTextStyleLst(st.env.Master)
}

// appendPlaceholderLstStyle 在 part（layout/master）上定位占位符形状
// lstStyle 对应级别的 defRPr 并加入 L3 源；part 不可达、非占位符或
// 该级别无定义时为空操作。
func (st *effState) appendPlaceholderLstStyle(part opc.PartName, which string) {
	doc := st.docOf(part)
	if doc == nil {
		return
	}
	sp := FindPlaceholderShape(doc, st.phKey)
	if sp == nil {
		return
	}
	if d := DefRPrAtLevel(doc, LstStyleOf(doc, sp), st.lvl); d != nil {
		st.sources = append(st.sources, famSource{
			style: ParseLocalFont(doc, d),
			step: StyleStep{Source: SourceListStyle, Part: string(part),
				Detail: st.phDetail() + " lvl=" + strconv.Itoa(st.lvl) + " " + which},
		})
	}
}

// appendTextStyleLst 把 master txStyles[class] 对应级别的 defRPr 加入
// L3 源；master 不可达或该类无文本样式时的空操作。
func (st *effState) appendTextStyleLst(part opc.PartName) {
	doc := st.docOf(part)
	if doc == nil {
		return
	}
	ts := TextStyleNode(doc, st.class)
	if ts == nil {
		return
	}
	if d := DefRPrAtLevel(doc, ts, st.lvl); d != nil {
		st.sources = append(st.sources, famSource{
			style: ParseLocalFont(doc, d),
			step: StyleStep{Source: SourceListStyle, Part: string(part),
				Detail: "txStyles " + string(st.class) + "Style lvl=" + strconv.Itoa(st.lvl)},
		})
	}
}

// docOf 返回 Env 链上给定 Part 的文档；Part 为空或不可达返回 nil。
func (st *effState) docOf(part opc.PartName) *xmlstore.XMLDocument {
	if part == "" {
		return nil
	}
	return st.docs(part)
}

// phDetail 生成占位符描述（Detail 用）。
func (st *effState) phDetail() string {
	return "ph type=" + st.phKey.Typ + " idx=" + strconv.FormatUint(uint64(st.phKey.Idx), 10)
}

// note 记录属性族是否解析成功。
func (st *effState) note(name string, resolved bool) { st.res[name] = resolved }

func (st *effState) unresolvedDiag(name string) {
	st.note(name, false)
	st.diags = append(st.diags, diag.Diagnostic{
		Code: "STYLE_UNRESOLVED", Severity: diag.SeverityWarning,
		Message: "property unresolved: " + name,
	})
}

// pickFirst 返回链上第一个显式值及其来源。
func pickFirst[T any](sources []famSource, pick func(FontStyle) model.Optional[T]) (T, StyleStep, bool) {
	for _, s := range sources {
		if v := pick(s.style); v.Set {
			return v.Value, s.step, true
		}
	}
	var zero T
	return zero, StyleStep{}, false
}

// fallbackOrUnresolved 处理链上无值的属性：调用方回退或标记未决。
func fallbackOrUnresolved[T any](st *effState, name string, fb func(ResolveContext) model.Optional[T]) ResolvedValue[T] {
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
	rf.Bold = st.resolveBool("bold", func(f FontStyle) model.Optional[bool] { return f.Bold },
		func(c ResolveContext) model.Optional[bool] { return c.Fallback.Bold })
	rf.Italic = st.resolveBool("italic", func(f FontStyle) model.Optional[bool] { return f.Italic },
		func(c ResolveContext) model.Optional[bool] { return c.Fallback.Italic })
	rf.Size = st.resolveSize()
	rf.Color = st.resolveColor()
	rf.Latin = st.resolveTypeface("latin", "latin", func(f FontStyle) model.Optional[string] { return f.Latin },
		func(c ResolveContext) model.Optional[string] { return c.Fallback.Latin })
	rf.EastAsian = st.resolveTypeface("ea", "ea", func(f FontStyle) model.Optional[string] { return f.EastAsian },
		func(c ResolveContext) model.Optional[string] { return c.Fallback.EastAsian })
	rf.ComplexScript = st.resolveTypeface("cs", "cs", func(f FontStyle) model.Optional[string] { return f.ComplexScript },
		func(c ResolveContext) model.Optional[string] { return c.Fallback.ComplexScript })
	return rf
}

// resolveBool 解析布尔属性（粗体/斜体）。
func (st *effState) resolveBool(name string, pick func(FontStyle) model.Optional[bool], fb func(ResolveContext) model.Optional[bool]) ResolvedValue[bool] {
	if v, step, ok := pickFirst(st.sources, pick); ok {
		st.note(name, true)
		return ResolvedValue[bool]{Value: v, Resolved: true, Trace: []StyleStep{step}}
	}
	return fallbackOrUnresolved(st, name, fb)
}

// resolveSize 解析字号。
func (st *effState) resolveSize() ResolvedValue[FontSize] {
	pick := func(f FontStyle) model.Optional[FontSize] { return f.Size }
	fb := func(c ResolveContext) model.Optional[FontSize] { return c.Fallback.Size }
	if v, step, ok := pickFirst(st.sources, pick); ok {
		st.note("size", true)
		return ResolvedValue[FontSize]{Value: v, Resolved: true, Trace: []StyleStep{step}}
	}
	return fallbackOrUnresolved(st, "size", fb)
}

// resolveTypeface 解析字体族：命中值若是主题引用（+mj-*/+mn-*）即展开；
// 全部未命中时按 class 用主题缺省字体（标题 major，其余 minor）；
// kind 为主题 fontScheme 子元素名（latin/ea/cs）。
func (st *effState) resolveTypeface(name, kind string, pick func(FontStyle) model.Optional[string], fb func(ResolveContext) model.Optional[string]) ResolvedValue[string] {
	tdoc := st.docOf(st.env.Theme)
	for _, s := range st.sources {
		v := pick(s.style)
		if !v.Set {
			continue
		}
		trace := []StyleStep{s.step}
		if final, tstep, ok := ExpandTypeface(tdoc, v.Value, kind); ok {
			if tstep.Source != 0 || tstep.Detail != "" {
				trace = append(trace, tstep)
			}
			st.note(name, true)
			return ResolvedValue[string]{Value: final, Resolved: true, Trace: trace}
		}
		// 主题引用但无法展开（无主题/主题缺该字体）：部分解析。
		st.note(name, false)
		st.diags = append(st.diags, diag.Diagnostic{
			Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning,
			Message: "theme typeface reference cannot be expanded: " + v.Value,
		})
		return ResolvedValue[string]{Value: v.Value, Resolved: false, Trace: trace}
	}
	// 主题缺省字体（L4）。
	if tdoc != nil {
		major := st.class == ClassTitle
		if face, found := ThemeFontFace(tdoc, major, kind); found && face != "" {
			st.note(name, true)
			return ResolvedValue[string]{Value: face, Resolved: true, Trace: []StyleStep{{
				Source: SourceTheme,
				Detail: "fontScheme default " + kind + " (" + FontSchemeName(major) + ")",
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
			st.diags = append(st.diags, diag.Diagnostic{
				Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning,
				Message: "scheme color phClr (placeholder color mapping) is not resolved by STYLE-01",
			})
			return ResolvedColor{Spec: spec, Resolved: false, Trace: trace}
		}
		scheme := spec.Scheme
		if ClrMapIndirect(scheme) {
			m := MasterClrMap(st.docOf(st.env.Master))
			mapped, ok := m[scheme]
			if !ok {
				st.note("color", false)
				st.diags = append(st.diags, diag.Diagnostic{
					Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning,
					Message: "clrMap has no entry for scheme color " + scheme,
				})
				return ResolvedColor{Spec: spec, Resolved: false, Trace: trace}
			}
			trace = append(trace, StyleStep{Source: SourceTheme,
				Detail: "clrMap " + scheme + " → " + mapped})
			scheme = mapped
		}
		tdoc := st.docOf(st.env.Theme)
		rgb, partial := SchemeRGB(tdoc, scheme)
		if partial {
			st.note("color", false)
			st.diags = append(st.diags, diag.Diagnostic{
				Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning,
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
