package style

// 主题链文档获取与公共节点助手——承接根包 theme_resolve.go 的
// themeDoc/masterDoc 语义，经 DocFunc 解耦文档存储。

import (
	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// themeDocOf 解析 Env 链上主题 Part 的文档（nil 表示不可达）。
func themeDocOf(docs DocFunc, env *Env) *xmlstore.XMLDocument {
	if env == nil || env.Theme == "" || docs == nil {
		return nil
	}
	return docs(env.Theme)
}

// masterDocOf 解析 Env 链上母版 Part 的文档（slideMaster/notesMaster；
// nil 表示不可达）。
func masterDocOf(docs DocFunc, env *Env) *xmlstore.XMLDocument {
	if env == nil || env.Master == "" || docs == nil {
		return nil
	}
	return docs(env.Master)
}

// ColorChild 返回填充容器内首个 DrawingML 颜色/子元素
// （solidFill→srgbClr 等）；容器本身为颜色元素时原样返回。无则 nil。
func ColorChild(doc *xmlstore.XMLDocument, fill *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if fill == nil {
		return nil
	}
	// 容器本身就是颜色元素（如某个调用了 ColorChild 的调用方传入 clr）。
	if fill.Namespace == ooxmlns.DrawingML && isColorElement(fill.Local()) {
		return fill
	}
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c != nil && c.Namespace == ooxmlns.DrawingML {
			return c
		}
	}
	return nil
}

// ResolveColor 解析颜色节点（ParseColorNode 的注入封装）：主题链文档经
// Env + Docs 自动取得。env/docs 缺失或不可达时按无主题处理（RGB 留空 +
// 诊断），不臆造。
func ResolveColor(doc *xmlstore.XMLDocument, clr *xmlstore.NodeRecord,
	env *Env, docs DocFunc, part string, diags *[]diag.Diagnostic) ParsedColor {
	return ParseColorNode(doc, clr, themeDocOf(docs, env), masterDocOf(docs, env), part, diags)
}
