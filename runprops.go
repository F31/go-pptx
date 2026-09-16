package pptx

// 本文件是 M3 的**Run 高级属性**：baseline/spc/highlight/caps/sym → RunProps。

// ---------- Run 高级属性 ----------

// RunProps 是 Run 字符属性（a:rPr）中 STYLE-01 之外的高级项解析结果。
type RunProps struct {
	// Baseline 是基线偏移（a:rPr@baseline，千分比；负为下标）。
	Baseline int32
	// Spacing 是字间距（a:rPr@spc，磅 = val/100）。
	Spacing float64
	// Highlight 是高亮颜色（a:highlight 内的颜色元素）。
	Highlight ParsedColor
	// Caps 是大小写形式（none/all/small）。
	Caps string
	// Strike 是删除线（noStrike/sglStrike/dblStrike）。
	Strike string
	// Underline 是下划线类型（a:rPr@u）。
	Underline string
	// Language / AltLanguage 是语言标签（a:rPr@lang/@altLang）。
	Language    string
	AltLanguage string
	// Kern 是字距调整（a:rPr@kern，磅 = val/100）。
	Kern float64
	// Symbol 是符号字体与字符（a:sym@font/@char）。
	Symbol *RunSymbol
	// Dirty 与 SpellError 表示待重新计算/拼写错误标记。
	Dirty      bool
	SpellError bool
	// Unknown 是未识别的 a:rPr 属性/子元素名。
	Unknown []string
}

// RunSymbol 是 a:sym 符号引用。
type RunSymbol struct {
	Font string
	Char string
}

// AdvancedProps 返回 Run 的高级字符属性（§2.3 矩阵"Run 高级属性"
// 起步解析）：baseline、spc、highlight、caps、strike、u、lang、sym 等。
// 未知项经 Unknown 输出，不臆造取值。
func (r *TextRun) AdvancedProps() (RunProps, []Diagnostic, error) {
	var out RunProps
	doc, run, err := r.locateRun()
	if err != nil {
		return out, nil, Annotate(err, "TextRun.AdvancedProps")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return out, nil, nil
	}
	var diags []Diagnostic
	env, err := r.p.styleEnv(r.part)
	if err != nil {
		return out, nil, Annotate(err, "TextRun.AdvancedProps")
	}
	known := map[string]bool{
		"lang": true, "altLang": true, "sz": true, "b": true, "i": true, "u": true,
		"strike": true, "cap": true, "spc": true, "baseline": true, "kern": true,
		"dirty": true, "err": true, "smtClean": true, "spellErr": true,
	}
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace != "" {
			out.Unknown = append(out.Unknown, a.RawName)
			continue
		}
		switch a.RawName {
		case "baseline":
			out.Baseline = intAttr(a.Value)
		case "spc":
			out.Spacing = float64(intAttr(a.Value)) / 100
		case "cap":
			out.Caps = a.Value
		case "strike":
			out.Strike = a.Value
		case "u":
			out.Underline = a.Value
		case "lang":
			out.Language = a.Value
		case "altLang":
			out.AltLanguage = a.Value
		case "kern":
			out.Kern = float64(intAttr(a.Value)) / 100
		case "dirty":
			out.Dirty = a.Value == "1" || a.Value == "true"
		case "spellErr":
			out.SpellError = a.Value == "1" || a.Value == "true"
		default:
			if !known[a.RawName] {
				out.Unknown = append(out.Unknown, a.RawName)
			}
		}
	}
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "highlight":
			out.Highlight = r.p.parseColorNode(doc, env, string(r.part), colorChildOf(doc, c), &diags)
		case "sym":
			s := &RunSymbol{}
			s.Font, _ = c.Attr("", "font")
			s.Char, _ = c.Attr("", "char")
			out.Symbol = s
		case "latin", "ea", "cs", "solidFill", "noFill", "gradFill", "ln",
			"effectLst", "effectDag", "uLnTx", "uLn", "uFillTx", "uFill",
			"blipFill", "pattFill", "grpFill", "hlinkClick", "hlinkMouseOver",
			"rtl", "extLst":
			// 已知但本包不展开的项（STYLE-01 或后续工作包负责）。
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out, diags, nil
}
