package pptx

import (
	"strconv"
	"strings"
	"time"

	"github.com/F31/go-pptx/internal/opc"
)

// Slide 是页面的受控句柄（方案 §5/§14）。
//
// 句柄不持有资源；有效性由所属 Presentation 的关闭状态与页面在
// sldIdLst 中的存在性决定：文档已关闭返回 ErrClosed，页面已被删除
// 返回 ErrStaleHandle。删除后不允许继续写入游离对象。
//
// Stable: 核心对象模型，v1.0 后承诺向后兼容。AddShape/AddTextBox/
// RemoveShape/MoveShape 等公共方法的签名与副作用语义视为已锁定。
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

// PartName 返回页面 Part 名（OPC 风格路径，如 "/ppt/slides/slide1.xml"）。
// 句柄失效（页面被删除或文档关闭）返回空串。
func (s *Slide) PartName() string {
	if s == nil {
		return ""
	}
	if err := s.alive(); err != nil {
		return ""
	}
	return string(s.part)
}

// Name 返回页面名称（presentation 端的可读标识）。当前实现返回
// PartName 的基础名（与 PowerPoint 行为对齐留口；M6 LAYOUT-01 接入
// 后改为 sld@name 与 rels 派名）。失败或 PartName 为空返回 ""。
func (s *Slide) Name() string {
	pn := s.PartName()
	if pn == "" {
		return ""
	}
	// 取最末路径段（去 .xml）。
	base := pn
	for i := len(pn) - 1; i >= 0; i-- {
		if pn[i] == '/' {
			base = pn[i+1:]
			break
		}
	}
	for i := 0; i < len(base); i++ {
		if base[i] == '.' {
			base = base[:i]
			break
		}
	}
	return base
}

// HasTiming 返回本页是否含任何 timing 子树（p:timing 或 c:timing 元素）。
// 用于 IR/M6 时间轴投影前的快速探测，不解析子树。
func (s *Slide) HasTiming() bool {
	if err := s.alive(); err != nil {
		return false
	}
	doc, _, err := s.slideTree()
	if err != nil {
		return false
	}
	// 查询 p:timing 与 c:timing（命名空间在 doc 已绑定）。
	for _, ns := range []string{nsPresentationML, "http://schemas.openxmlformats.org/drawingml/2006/main"} {
		if len(doc.Elements(ns, "timing")) > 0 {
			return true
		}
	}
	return false
}

// NotesPart 返回备注页 Part 名（如 "/ppt/notesSlides/notesSlide1.xml"）；
// 页面无备注返回 ""。
func (s *Slide) NotesPart() string {
	if err := s.alive(); err != nil {
		return ""
	}
	if name, ok := s.notesPartOf(); ok {
		return string(name)
	}
	return ""
}

// Hidden 返回页面是否标记为隐藏（p:sldId@show="0"）。
//
// OOXML 语义：sldId 的 show 属性仅在隐藏时出现，取值固定为 "0"；缺省或
// 取其他值（含空串、show="1"）一律视为可见。返回 true 表示该页被显式
// 标记隐藏，调用方（例如讲稿生成器）可据此跳过。
//
// 句柄失效（页面已删/文档已关）返回 ErrStaleHandle/ErrClosed。
//
// Stable: 公开只读方法，binary-compat（仅追加）。
func (s *Slide) Hidden() (bool, error) {
	if err := s.alive(); err != nil {
		return false, Annotate(err, "Slide.Hidden")
	}
	doc, err := s.p.presentationDoc()
	if err != nil {
		return false, Annotate(err, "Slide.Hidden")
	}
	for _, lstID := range doc.Elements(nsPresentationML, "sldIdLst") {
		lst := doc.Node(lstID)
		for _, cid := range lst.Children {
			n := doc.Node(cid)
			if n.Namespace != nsPresentationML || n.Local() != "sldId" {
				continue
			}
			idStr, ok := n.Attr("", "id")
			if !ok {
				continue
			}
			id, perr := parseUint32(idStr)
			if perr != nil || SlideID(id) != s.id {
				continue
			}
			if v, ok := n.Attr("", "show"); ok && v == "0" {
				return true, nil
			}
			return false, nil
		}
	}
	// 未在 sldIdLst 中找到对应 id（alive 已通过）：理论上不可能，但
	// 防御性返回 false。
	return false, nil
}

// AdvanceAfter 返回页面自动翻页时长（p:transition@advTm 毫秒）。
//
// 行为：
//   - 第二个返回值 ok=true 表示该页存在显式 advTm（自 SetAdvanceAfter
//     或外部 OOXML 编辑写入），返回 time.Duration 为其毫秒值；
//     ok=false 表示未设或被外部清空，返回零时长；
//   - 页面无 p:transition 节点返回 0, false, nil；
//   - advTm 解析失败（非整型）返回 0, false, ErrMalformedPackage；
//   - 句柄失效返回 ErrStaleHandle/ErrClosed。
//
// 与 SetAdvanceAfter 严格成对——该方法补齐读侧，使 ppts 等客户端无需
// 自行解析 XML 即可拿到自动切页时长。
//
// Stable: 公开只读方法，binary-compat（仅追加）。
func (s *Slide) AdvanceAfter() (time.Duration, bool, error) {
	if err := s.alive(); err != nil {
		return 0, false, Annotate(err, "Slide.AdvanceAfter")
	}
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return 0, false, Annotate(err, "Slide.AdvanceAfter")
	}
	root := doc.Root()
	if root == nil {
		return 0, false, Annotate(ErrStaleHandle, "Slide.AdvanceAfter")
	}
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != nsPresentationML || c.Local() != "transition" {
			continue
		}
		v, ok := c.Attr("", "advTm")
		if !ok || strings.TrimSpace(v) == "" {
			return 0, false, nil
		}
		ms, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil || ms < 0 {
			opErr := &OperationError{
				Op:      "Slide.AdvanceAfter",
				Part:    string(s.part),
				Message: "invalid advTm value: " + v,
				Err:     ErrMalformedPackage,
			}
			return 0, false, Annotate(opErr, "Slide.AdvanceAfter")
		}
		return time.Duration(ms) * time.Millisecond, true, nil
	}
	return 0, false, nil
}

// partName 返回页面 Part 名（OPC 风格）。当前仅库内部与测试使用；
// 随 TEXT-01（段落/Run）扩展为公共能力。
func (s *Slide) partName() (opc.PartName, error) {
	if err := s.alive(); err != nil {
		return "", err
	}
	return s.part, nil
}
