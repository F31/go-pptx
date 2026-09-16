package pptx

import (
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 纯几何解析已迁至 internal/document/geometry（v2.0 域层）。
// 本文件仅保留被填充/效果解析（仍属本包）共用的 spPr 定位辅助。

// fillContainer 在形状元素下查找 spPr/a:fill 容器。spPr 不存在返回 nil。
func fillContainer(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	sp := xmlstore.ChildOfKind(doc, el, ooxmlns.PresentationML, "spPr", 0)
	if sp == nil {
		return nil
	}
	return xmlstore.ChildOfKind(doc, sp, ooxmlns.DrawingML, "fill", 0)
}

// spPrOf 查找形状元素下的 spPr。
func spPrOf(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	return xmlstore.ChildOfKind(doc, el, ooxmlns.PresentationML, "spPr", 0)
}
