package pptx

import (
	"errors"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 TEXT-01 的**字符格式解析与片段构建**：a:rPr 子元素排序/解析
// （parseLocalFont/parseSolidFill/parseCentipoints/rPrChildRank）与
// rPr/填充片段构造（buildRPrFragment/solidFillFragment/fillChildOf）。
// 写入路径见 textfont.go。
// anyChildSet 报告 style 是否含子元素类字段。
func (f FontStyle) anyChildSet() bool {
	return f.Color.Set || f.Latin.Set || f.EastAsian.Set || f.ComplexScript.Set
}

// isNSDeclAttr 报告属性名是否为 xmlns 声明（rPr 重建时跳过）。
func isNSDeclAttr(raw string) bool {
	return raw == "xmlns" || strings.HasPrefix(raw, "xmlns:")
}

// rPrChildRank 返回 rPr 子元素族别的 schema 序号；未知返回 ok=false。
func rPrChildRank(ns, local string) (int, bool) {
	if ns != nsDrawingML {
		return 0, false
	}
	switch local {
	case "ln":
		return 0, true
	case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
		return 1, true
	case "effectLst", "effectDag":
		return 2, true
	case "highlight":
		return 3, true
	case "uLnTx", "uLn":
		return 4, true
	case "uFillTx", "uFill":
		return 5, true
	case "latin":
		return 6, true
	case "ea":
		return 7, true
	case "cs":
		return 8, true
	case "sym":
		return 9, true
	case "hlinkClick", "hlinkMouseOver":
		return 10, true
	case "rtl":
		return 11, true
	case "extLst":
		return 12, true
	}
	return 0, false
}

const (
	fillRank  = 1
	latinRank = 6
	eaRank    = 7
	csRank    = 8
)

// fillChildOf 返回 rPr 中填充族子元素（含 solidFill/noFill/grad 等）。
func fillChildOf(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
			return c
		}
	}
	return nil
}

// solidFillFragment 生成 solidFill 片段。
func solidFillFragment(prefix string, c ColorSpec) string {
	if c.Scheme != "" {
		esc, _ := xmlstore.EscapeAttrValue(c.Scheme, '"')
		return "<" + prefix + ":solidFill><" + prefix + ":schemeClr val=\"" + esc + "\"/></" + prefix + ":solidFill>"
	}
	return "<" + prefix + ":solidFill><" + prefix + ":srgbClr val=\"" + c.RGB + "\"/></" + prefix + ":solidFill>"
}

// buildRPrFragment 从 style 构造全新 rPr 片段（属性 + 子元素按序）。
func buildRPrFragment(prefix string, style FontStyle) (string, error) {
	var attrs []string
	if style.Bold.Set {
		attrs = append(attrs, `b="`+boolVal(style.Bold.Value)+`"`)
	}
	if style.Italic.Set {
		attrs = append(attrs, `i="`+boolVal(style.Italic.Value)+`"`)
	}
	if style.Size.Set {
		attrs = append(attrs, `sz="`+sizeCentipoints(style.Size.Value)+`"`)
	}
	var children []string
	if style.Color.Set {
		if !style.Color.Value.Valid() {
			return "", &OperationError{Message: "empty ColorSpec", Err: ErrInvalidArgument}
		}
		children = append(children, solidFillFragment(prefix, style.Color.Value))
	}
	for _, fc := range []struct {
		set  Optional[string]
		kind string
	}{
		{style.Latin, "latin"},
		{style.EastAsian, "ea"},
		{style.ComplexScript, "cs"},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return "", Annotate(mapXMLError(err), "SetFont")
		}
		children = append(children, "<"+prefix+":"+fc.kind+` typeface="`+esc+`"/>`)
	}
	var sb strings.Builder
	sb.WriteString("<" + prefix + ":rPr")
	for _, a := range attrs {
		sb.WriteString(" ")
		sb.WriteString(a)
	}
	if len(children) == 0 {
		sb.WriteString("/>")
	} else {
		sb.WriteString(">")
		for _, c := range children {
			sb.WriteString(c)
		}
		sb.WriteString("</" + prefix + ":rPr>")
	}
	return sb.String(), nil
}

// anySet 报告 FontStyle 是否含显式字段。
func (f FontStyle) anySet() bool {
	return f.Bold.Set || f.Italic.Set || f.Size.Set || f.Color.Set ||
		f.Latin.Set || f.EastAsian.Set || f.ComplexScript.Set
}

func boolVal(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// sizeCentipoints 把 pt 字号转 XML 的百分之一 pt 整数（四舍五入）。
func sizeCentipoints(sz FontSize) string {
	cp := int(float64(sz)*100 + 0.5)
	return intString(cp)
}

func intString(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// parseLocalFont 读取 rPr 中可安全表示的本地属性。
func parseLocalFont(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) FontStyle {
	var f FontStyle
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace != "" {
			continue
		}
		switch a.RawName {
		case "b":
			f.Bold = Optional[bool]{Value: a.Value != "0", Set: true}
		case "i":
			f.Italic = Optional[bool]{Value: a.Value != "0", Set: true}
		case "sz":
			if cp, err := parseCentipoints(a.Value); err == nil {
				f.Size = Optional[FontSize]{Value: FontSize(cp) / 100.0, Set: true}
			}
		}
	}
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "solidFill":
			if spec, ok := parseSolidFill(doc, c); ok {
				f.Color = Optional[ColorSpec]{Value: spec, Set: true}
			}
		case "latin":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.Latin = Optional[string]{Value: tf, Set: true}
			}
		case "ea":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.EastAsian = Optional[string]{Value: tf, Set: true}
			}
		case "cs":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.ComplexScript = Optional[string]{Value: tf, Set: true}
			}
		}
	}
	return f
}

// parseSolidFill 只识别 a:schemeClr / a:srgbClr 两种可安全表示的形态。
func parseSolidFill(doc *xmlstore.XMLDocument, fill *xmlstore.NodeRecord) (ColorSpec, bool) {
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "schemeClr":
			if v, ok := c.Attr("", "val"); ok {
				return ColorSpec{Scheme: v}, true
			}
		case "srgbClr":
			if v, ok := c.Attr("", "val"); ok {
				return ColorSpec{RGB: v}, true
			}
		}
	}
	return ColorSpec{}, false
}

func parseCentipoints(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	v := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errors.New("non-digit")
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}
