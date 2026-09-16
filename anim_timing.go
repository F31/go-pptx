// 时序 IR（TIMIR-01，方案 §21.5）根包侧：导出 TimingTree 的 p:timing
// 原始字节访问，让 ir 包独立完成 schemaVersion=go-pptx.ir/1.0 的 JSON
// 投影。R 档——只读，不提供任何动画编辑 API。
package pptx

import (
	"github.com/F31/go-pptx/internal/xmlstore"
)

// TimingTreeRaw 返回本页 p:timing 元素的原始字节（XML 文本）。
//
// 行为约定：
//   - 页无 HasTiming() 时返回 nil, nil, nil（这是正常情况，非错误）；
//   - 解析失败时返回错误（诊断由调用方按 §21.5 解读为 Partial/Untested）。
func (s *Slide) TimingTreeRaw() ([]byte, []Diagnostic, error) {
	if err := s.alive(); err != nil {
		return nil, nil, err
	}
	doc, timing, err := s.timingTreeXML()
	if err != nil {
		return nil, nil, err
	}
	if doc == nil || timing == nil {
		return nil, nil, nil
	}
	if timing.SelfClosing() {
		return []byte{}, nil, nil
	}
	// timing.Source 覆盖整个元素（开标签起始 ~ 闭标签 '>' 或自闭合 '/>'），
	// 即使自闭合在前面也安全（SelfClosing 已处理）。
	return doc.Slice(timing.Source), nil, nil
}

// timingTreeXML 内部：定位 p:timing 子树（p:sld 的直接子元素）——
// 返回值与 HasTiming 兼容：nil 表示没有 p:timing。
func (s *Slide) timingTreeXML() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return nil, nil, err
	}
	root := doc.Root()
	if root == nil {
		return nil, nil, &OperationError{
			Op: "Slide.timingTree", Part: string(s.part),
			Message: "slide root missing", Err: ErrMalformedPackage,
		}
	}
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c != nil && c.Namespace == nsPresentationML && c.Local() == "timing" {
			return doc, c, nil
		}
	}
	return doc, nil, nil
}
