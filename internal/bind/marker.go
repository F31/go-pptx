package bind

import (
	"strings"

	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是模板绑定的**占位符词法与 XML 补丁辅助**：内联模板 token 扫描
// （TplToken/ScanInline）、指令解析（ParseDirective）与删除/空段/子元素
// 补丁构造（DeletePatch/EmptyParaPatch/ChildElems）。

// 模板标记定界符。原定义于根包 bind.go，随词法实现一并下沉。
const (
	TplMarkOpen  = "{{"
	TplMarkClose = "}}"
)

// ---------- 模板语法 ----------

// TplToken 是一个内联占位符：literal 为文档中的原始字面量。
type TplToken struct {
	Literal string
	Path    string
}

// ParseDirective 解析独占段落的块标记；ok=false 表示不是块标记。
// kind 取值：if / endif / each / endeach。
func ParseDirective(s string) (kind, path string, ok bool) {
	if !strings.HasPrefix(s, TplMarkOpen) || !strings.HasSuffix(s, TplMarkClose) {
		return "", "", false
	}
	inner := strings.TrimSpace(s[len(TplMarkOpen) : len(s)-len(TplMarkClose)])
	switch {
	case inner == "/if":
		return "endif", "", true
	case inner == "/each":
		return "endeach", "", true
	case strings.HasPrefix(inner, "#if "):
		p := strings.TrimSpace(strings.TrimPrefix(inner, "#if "))
		if p == "" {
			return "", "", false
		}
		return "if", p, true
	case strings.HasPrefix(inner, "#each "):
		p := strings.TrimSpace(strings.TrimPrefix(inner, "#each "))
		if p == "" {
			return "", "", false
		}
		return "each", p, true
	}
	return "", "", false
}

// ScanInline 扫描文本中的内联占位符（跳过块标记形态）。
func ScanInline(s string) []TplToken {
	var out []TplToken
	for i := 0; i+len(TplMarkClose) <= len(s); {
		at := strings.Index(s[i:], TplMarkOpen)
		if at < 0 {
			break
		}
		start := i + at
		end := strings.Index(s[start+len(TplMarkOpen):], TplMarkClose)
		if end < 0 {
			break
		}
		end = start + len(TplMarkOpen) + end + len(TplMarkClose)
		lit := s[start:end]
		inner := strings.TrimSpace(lit[len(TplMarkOpen) : len(lit)-len(TplMarkClose)])
		i = end
		if inner == "" || strings.HasPrefix(inner, "#") || strings.HasPrefix(inner, "/") {
			continue // 块标记由 ParseDirective 处理
		}
		out = append(out, TplToken{Literal: lit, Path: inner})
	}
	return out
}

// ---------- 补丁辅助 ----------

// DeletePatch 构造删除整个元素的补丁（带原文锚定）。
func DeletePatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, desc string) xmlstore.SpanPatch {
	return xmlstore.SpanPatch{
		Start:  n.Source.Start,
		End:    n.Source.End,
		Expect: doc.Slice(n.Source),
		Desc:   desc,
	}
}

// EmptyParaPatch 构造"清空段落内容"的补丁（保留 a:p 元素本身）；
// 无子元素时返回 nil。
func EmptyParaPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *xmlstore.SpanPatch {
	if n.SelfClosing() || len(n.Children) == 0 {
		return nil
	}
	first := doc.Node(n.Children[0])
	last := doc.Node(n.Children[len(n.Children)-1])
	return &xmlstore.SpanPatch{
		Start:  first.Source.Start,
		End:    last.Source.End,
		Expect: doc.Slice(xmlstore.ByteRange{Start: first.Source.Start, End: last.Source.End}),
		Desc:   "empty-paragraph",
	}
}

// ChildElems 返回按文档序的直接子元素中本地名为 local 的节点。
func ChildElems(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, local string) []*xmlstore.NodeRecord {
	var out []*xmlstore.NodeRecord
	for _, id := range n.Children {
		c := doc.Node(id)
		if c.Local() == local {
			out = append(out, c)
		}
	}
	return out
}
