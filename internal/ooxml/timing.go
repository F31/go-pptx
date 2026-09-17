package ooxml

import (
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是时间轴投影（IR HasTiming / TIMIR-01 原始字节）：以 xmlstore
// 只读索引（传输层，位于本层之下）定位 p:sld 的 p:timing 子树原始字节，
// 供 internal/ir.ProjectTimingTree 独立投影。不进入写路径。

// SlideHasTiming 报告 p:sld 是否含任何 timing 子树（p:timing 或
// dml 命名空间的 timing 元素）。与门面 Slide.HasTiming 同义。
func SlideHasTiming(data []byte) (bool, error) {
	doc, err := xmlstore.Index(data)
	if err != nil {
		return false, err
	}
	for _, ns := range []string{pmlMainNS, dmlMainNS} {
		if len(doc.Elements(ns, "timing")) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// SlideTimingRaw 返回 p:sld 根 p:timing 元素的原始字节（XML 文本）。
//
// 行为（与门面 Slide.TimingTreeRaw 一致）：
//   - 无 p:timing → (nil, false, nil)；
//   - 自闭合 → ([]byte{}, true, nil)；
//   - 解析失败 → 错误。
func SlideTimingRaw(data []byte) ([]byte, bool, error) {
	doc, err := xmlstore.Index(data)
	if err != nil {
		return nil, false, err
	}
	root := doc.Root()
	if root == nil {
		return nil, false, nil
	}
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c != nil && c.Namespace == pmlMainNS && c.Local() == "timing" {
			if c.SelfClosing() {
				return []byte{}, true, nil
			}
			return doc.Slice(c.Source), true, nil
		}
	}
	return nil, false, nil
}
