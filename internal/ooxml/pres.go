package ooxml

import (
	"strconv"

	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// parseUint32 解析无符号十进制；失败返回 err（调用方按"未匹配"处理）。
func parseUint32(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	return uint32(n), err
}

// 本文件是页面隐藏投影（IR Page.Hidden），读取 presentation.xml 的
// sldIdLst：p:sldId@show="0" 表示该页被显式标记隐藏。与门面
// Slide.Hidden 同规（缺省/其他取值一律可见）。

// SlideHidden 报告 presentation 的 sldIdLst 中 id==slideID 的条目是否
// 显式标记隐藏（show="0"）。未找到对应条目返回 (false, nil)。
func SlideHidden(data []byte, slideID uint32) (bool, error) {
	doc, err := xmlstore.Index(data)
	if err != nil {
		return false, err
	}
	for _, lid := range doc.Elements(pmlMainNS, "sldIdLst") {
		lst := doc.Node(lid)
		if lst == nil {
			continue
		}
		for _, cid := range lst.Children {
			n := doc.Node(cid)
			if n == nil || n.Namespace != pmlMainNS || n.Local() != "sldId" {
				continue
			}
			idStr, ok := n.Attr("", "id")
			if !ok {
				continue
			}
			id, perr := parseUint32(idStr)
			if perr != nil || id != slideID {
				continue
			}
			if v, ok := n.Attr("", "show"); ok && v == "0" {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
}
