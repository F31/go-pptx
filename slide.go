package pptx

import "github.com/F31/go-pptx/internal/opc"

// Slide 是页面的受控句柄（方案 §5/§14）。
//
// 句柄不持有资源；有效性由所属 Presentation 的关闭状态与页面在
// sldIdLst 中的存在性决定：文档已关闭返回 ErrClosed，页面已被删除
// 返回 ErrStaleHandle。删除后不允许继续写入游离对象。
type Slide struct {
	p    *Presentation
	id   SlideID
	part opc.PartName
	rev  uint64 // 句柄创建时的 revision（用于失效判断的辅助快照）
}

// ID 返回页面标识（presentation.xml sldId@id，文档范围唯一）。
func (s *Slide) ID() SlideID { return s.id }

// alive 检查句柄有效性：文档未关闭且页面仍列在当前 sldIdLst 中。
func (s *Slide) alive() error {
	if s.p == nil || s.p.closed {
		return Annotate(ErrClosed, "Slide")
	}
	doc, err := s.p.presentationDoc()
	if err != nil {
		return Annotate(err, "Slide")
	}
	for _, lstID := range doc.Elements(nsPresentationML, "sldIdLst") {
		lst := doc.Node(lstID)
		for _, cid := range lst.Children {
			n := doc.Node(cid)
			if n.Namespace != nsPresentationML || n.Local() != "sldId" {
				continue
			}
			if idStr, ok := n.Attr("", "id"); ok {
				if id, err := parseUint32(idStr); err == nil && SlideID(id) == s.id {
					return nil
				}
			}
		}
	}
	return Annotate(ErrStaleHandle, "Slide")
}

// partName 返回页面 Part 名（OPC 风格）。当前仅库内部与测试使用；
// 随 TEXT-01（段落/Run）扩展为公共能力。
func (s *Slide) partName() (opc.PartName, error) {
	if err := s.alive(); err != nil {
		return "", err
	}
	return s.part, nil
}
