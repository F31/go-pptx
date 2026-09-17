// Package text 是 v2.0 域层「文本」垂直切片：a:rPr 字符格式的解析与片段
// 构造、写入补丁计算，以及文本框/字段级纯辅助。
//
// 句柄类型（TextFrame/Paragraph/TextRun/Field）仍留在根包 pptx 门面，本包
// 只承载无状态的纯函数；根包方法做薄委托。禁止本包反向 import 根包。
package text

import (
	"strings"

	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/errs"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// rPr 子元素族别在 schema 中的排列序号（用于新子元素插入定位）。
const (
	// FillRank 是填充族（solidFill/gradFill/...）。
	FillRank = 1
	// LatinRank 是 a:latin。
	LatinRank = 6
	// EaRank 是 a:ea。
	EaRank = 7
	// CsRank 是 a:cs。
	CsRank = 8
)

// IsNSDeclAttr 报告属性名是否为 xmlns 声明（rPr 重建时跳过）。
func IsNSDeclAttr(raw string) bool {
	return raw == "xmlns" || strings.HasPrefix(raw, "xmlns:")
}

// RPrChildRank 返回 rPr 子元素族别的 schema 序号；未知返回 ok=false。
func RPrChildRank(ns, local string) (int, bool) {
	if ns != ooxmlns.DrawingML {
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

// FillChildOf 返回 rPr 中填充族子元素（含 solidFill/noFill/grad 等）。
func FillChildOf(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
			return c
		}
	}
	return nil
}

// SolidFillFragment 生成 solidFill 片段。
func SolidFillFragment(prefix string, c style.ColorSpec) string {
	if c.Scheme != "" {
		esc, _ := xmlstore.EscapeAttrValue(c.Scheme, '"')
		return "<" + prefix + ":solidFill><" + prefix + ":schemeClr val=\"" + esc + "\"/></" + prefix + ":solidFill>"
	}
	return "<" + prefix + ":solidFill><" + prefix + ":srgbClr val=\"" + c.RGB + "\"/></" + prefix + ":solidFill>"
}

// BuildRPrFragment 从 st 构造全新 rPr 片段（属性 + 子元素按序）。
func BuildRPrFragment(prefix string, st style.FontStyle) (string, error) {
	var attrs []string
	if st.Bold.Set {
		attrs = append(attrs, `b="`+BoolVal(st.Bold.Value)+`"`)
	}
	if st.Italic.Set {
		attrs = append(attrs, `i="`+BoolVal(st.Italic.Value)+`"`)
	}
	if st.Size.Set {
		attrs = append(attrs, `sz="`+SizeCentipoints(st.Size.Value)+`"`)
	}
	var children []string
	if st.Color.Set {
		if !st.Color.Value.Valid() {
			return "", &errs.OperationError{Message: "empty ColorSpec", Err: errs.ErrInvalidArgument}
		}
		children = append(children, SolidFillFragment(prefix, st.Color.Value))
	}
	for _, fc := range []struct {
		set  model.Optional[string]
		kind string
	}{
		{st.Latin, "latin"},
		{st.EastAsian, "ea"},
		{st.ComplexScript, "cs"},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return "", errs.Annotate(err, "SetFont")
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

// BoolVal 把 bool 转 XML 的 0/1 字面量。
func BoolVal(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// SizeCentipoints 把 pt 字号转 XML 的百分之一 pt 整数（四舍五入）。
func SizeCentipoints(sz style.FontSize) string {
	cp := int(float64(sz)*100 + 0.5)
	return IntString(cp)
}

// IntString 把 int 转十进制字符串（避免 fmt 依赖的小工具）。
func IntString(v int) string {
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
