package pptx

import (
	"github.com/F31/go-pptx/v2/internal/document/style"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是 STYLE-01 的**主题/母版文档读取**（Presentation 绑定）。
// v2.0：解析辅助（clrMap/文本样式/字体 face/schemeRGB）已迁至
// internal/document/style；此处仅保留按环境取文档的接线。

// themeDoc 解析主题 Part 文档（nil 表示不可达）。
func (p *Presentation) themeDoc(env *style.Env) *xmlstore.XMLDocument {
	if env.Theme == "" {
		return nil
	}
	doc, err := p.docOf(env.Theme)
	if err != nil {
		return nil
	}
	return doc
}

// masterDoc 解析母版 Part 文档（slideMaster 或 notesMaster；nil 不可达）。
func (p *Presentation) masterDoc(env *style.Env) *xmlstore.XMLDocument {
	if env.Master == "" {
		return nil
	}
	doc, err := p.docOf(env.Master)
	if err != nil {
		return nil
	}
	return doc
}
