package style

// 本文件是 M3 的 **Run 高级属性**解析（根包 format_runprops.go 下沉）：
// baseline/spc/highlight/caps/sym 等 → RunProps。依赖仅来自
// Env + DocFunc（主题链文档只供 highlight 颜色解析）。

import (
	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

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

// rPrKnownAttrs 是 STYLE-01 / 本解析已识别的 a:rPr 属性（不出现在
// Unknown）。StyleStep 解析所需的 sz/b/i 等由 ParseLocalFont 处理，
// 此处一并视为已知避免误报。
func rPrKnownAttrs() map[string]bool {
	return map[string]bool{
		"lang": true, "altLang": true, "sz": true, "b": true, "i": true, "u": true,
		"strike": true, "cap": true, "spc": true, "baseline": true, "kern": true,
		"dirty": true, "err": true, "smtClean": true, "spellErr": true,
	}
}

// rPrKnownChildren 是本解析识别的 a:rPr 子元素（不展开、不出现在
// Unknown）：STYLE-01 或后续工作包负责。
func rPrKnownChildren() map[string]bool {
	return map[string]bool{
		"highlight": true, "latin": true, "ea": true, "cs": true, "solidFill": true,
		"noFill": true, "gradFill": true, "ln": true, "effectLst": true,
		"effectDag": true, "uLnTx": true, "uLn": true, "uFillTx": true,
		"uFill": true, "blipFill": true, "pattFill": true, "grpFill": true,
		"hlinkClick": true, "hlinkMouseOver": true, "rtl": true, "extLst": true,
		"sym": true,
	}
}

// ParseRunProps 解析 a:rPr 的 Run 高级字符属性（§2.3 矩阵"Run 高级属性"
// 起步解析）：baseline、spc、highlight、caps、strike、u、lang、sym 等。
// 未知项经 Unknown 输出，不臆造取值。env/docs 提供主题链（仅 highlight）。
func ParseRunProps(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord,
	env *Env, docs DocFunc, part string) (RunProps, []diag.Diagnostic) {
	var out RunProps
	if rPr == nil {
		return out, nil
	}
	var diags []diag.Diagnostic
	known := rPrKnownAttrs()
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace != "" {
			out.Unknown = append(out.Unknown, a.RawName)
			continue
		}
		switch a.RawName {
		case "baseline":
			out.Baseline = xmlstore.IntAttr(a.Value)
		case "spc":
			out.Spacing = float64(xmlstore.IntAttr(a.Value)) / 100
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
			out.Kern = float64(xmlstore.IntAttr(a.Value)) / 100
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
	knownKids := rPrKnownChildren()
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "highlight":
			out.Highlight = ParseColorNode(doc, ColorChild(doc, c),
				themeDocOf(docs, env), masterDocOf(docs, env), part, &diags)
		case "sym":
			s := &RunSymbol{}
			s.Font, _ = c.Attr("", "font")
			s.Char, _ = c.Attr("", "char")
			out.Symbol = s
		default:
			if !knownKids[c.Local()] {
				out.Unknown = append(out.Unknown, c.Local())
			}
		}
	}
	return out, diags
}
