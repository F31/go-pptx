package pptx

import (
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是模板绑定的**占位符词法与 XML 补丁辅助**：内联模板 token 扫描
// （tplToken/scanInline）、指令解析（parseDirective）与删除/空段/子元素
// 补丁构造（deletePatch/emptyParaPatch/childElems）。

// ---------- 模板语法 ----------

// tplToken 是一个内联占位符：literal 为文档中的原始字面量。
type tplToken struct {
	literal string
	path    string
}

// parseDirective 解析独占段落的块标记；ok=false 表示不是块标记。
// kind 取值：if / endif / each / endeach。
func parseDirective(s string) (kind, path string, ok bool) {
	if !strings.HasPrefix(s, tplMarkOpen) || !strings.HasSuffix(s, tplMarkClose) {
		return "", "", false
	}
	inner := strings.TrimSpace(s[len(tplMarkOpen) : len(s)-len(tplMarkClose)])
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

// scanInline 扫描文本中的内联占位符（跳过块标记形态）。
func scanInline(s string) []tplToken {
	var out []tplToken
	for i := 0; i+len(tplMarkClose) <= len(s); {
		at := strings.Index(s[i:], tplMarkOpen)
		if at < 0 {
			break
		}
		start := i + at
		end := strings.Index(s[start+len(tplMarkOpen):], tplMarkClose)
		if end < 0 {
			break
		}
		end = start + len(tplMarkOpen) + end + len(tplMarkClose)
		lit := s[start:end]
		inner := strings.TrimSpace(lit[len(tplMarkOpen) : len(lit)-len(tplMarkClose)])
		i = end
		if inner == "" || strings.HasPrefix(inner, "#") || strings.HasPrefix(inner, "/") {
			continue // 块标记由 parseDirective 处理
		}
		out = append(out, tplToken{literal: lit, path: inner})
	}
	return out
}

// ---------- 补丁辅助 ----------

// deletePatch 构造删除整个元素的补丁（带原文锚定）。
func deletePatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, desc string) xmlstore.SpanPatch {
	return xmlstore.SpanPatch{
		Start:  n.Source.Start,
		End:    n.Source.End,
		Expect: doc.Slice(n.Source),
		Desc:   desc,
	}
}

// emptyParaPatch 构造"清空段落内容"的补丁（保留 a:p 元素本身）；
// 无子元素时返回 nil。
func emptyParaPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *xmlstore.SpanPatch {
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

// childElems 返回按文档序的直接子元素中本地名为 local 的节点。
func childElems(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, local string) []*xmlstore.NodeRecord {
	var out []*xmlstore.NodeRecord
	for _, id := range n.Children {
		c := doc.Node(id)
		if c.Local() == local {
			out = append(out, c)
		}
	}
	return out
}
