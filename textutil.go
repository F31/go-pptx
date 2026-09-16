package pptx

import (
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是文本模型的**通用 XML 补丁与转义辅助**：属性/元素删除补丁、
// 首子元素定位、XML 实体反转义（供 Run 文本读取）。

// removeAttrPatch 构造删除属性补丁：从属性名前导空白（若有）到值结束
// 引号之后（含引号）。NameStart/ValueEnd 来自 xmlstore 索引的精确
// 字节锚点；ValueEnd 指向闭合引号位置（值区间为引号内内容），因此
// 结束边界取 ValueEnd+1 以连同引号一并删除。
func removeAttrPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, attrIdx int) xmlstore.SpanPatch {
	a := &n.Attrs[attrIdx]
	orig := doc.Original()
	start := a.NameStart
	if start > n.Source.Start {
		switch orig[start-1] {
		case ' ', '\t', '\n', '\r':
			start--
		}
	}
	return xmlstore.SpanPatch{
		Start:       start,
		End:         a.ValueEnd + 1,
		Replacement: []byte(""),
	}
}

// removeElementPatch 构造删除元素补丁。
func removeElementPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) xmlstore.SpanPatch {
	return xmlstore.SpanPatch{
		Start:       n.Source.Start,
		End:         n.Source.End,
		Replacement: []byte(""),
	}
}

func firstChildOf(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if len(n.Children) == 0 {
		return nil
	}
	return doc.Node(n.Children[0])
}

// xmlUnescape 解码文本内容中出现的 XML 实体（含数字引用）。
func xmlUnescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		j := strings.IndexByte(s[i:], ';')
		if j < 0 {
			sb.WriteByte(s[i])
			i++
			continue
		}
		ent := s[i+1 : i+j]
		switch ent {
		case "amp":
			sb.WriteByte('&')
		case "lt":
			sb.WriteByte('<')
		case "gt":
			sb.WriteByte('>')
		case "quot":
			sb.WriteByte('"')
		case "apos":
			sb.WriteByte('\'')
		default:
			if len(ent) > 1 && ent[0] == '#' {
				var code rune = -1
				if ent[1] == 'x' || ent[1] == 'X' {
					code = parseHexRune(ent[2:])
				} else {
					code = parseDecRune(ent[1:])
				}
				if code >= 0 {
					sb.WriteRune(code)
				} else {
					sb.WriteString(s[i : i+j+1])
				}
			} else {
				sb.WriteString(s[i : i+j+1]) // 未知命名实体原样保留
			}
		}
		i += j + 1
	}
	return sb.String()
}

func parseHexRune(s string) rune {
	var v rune
	for i := 0; i < len(s); i++ {
		c := s[i]
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			v |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= rune(c-'A') + 10
		default:
			return -1
		}
		if v > 0x10FFFF {
			return -1
		}
	}
	return v
}

func parseDecRune(s string) rune {
	var v rune
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return -1
		}
		v = v*10 + rune(c-'0')
		if v > 0x10FFFF {
			return -1
		}
	}
	return v
}
