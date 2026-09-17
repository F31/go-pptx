package style

import (
	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是 STYLE-01 的**占位符匹配与层级导航** + 文本样式归类：
// PhKey/PhKeyOf、占位符形状查找、祖先形状/段落定位、层级与文本类（ClassOf）。
//
// 原实现位于根包 theme_placeholder.go，为 v2.0 分层做准备下沉至此；
// 依赖的 DOM/命名空间 helper 由 internal/xmlstore、internal/ooxmlns 提供。

// StyleKind 区分正文页面与备注页（样式源宿主不同）。
const (
	StyleKindSlide = "slide"
	StyleKindNotes = "notes"
)

// ---------- 占位符与段落上下文 ----------

// PhKey 是占位符的规范化匹配键：类型缺省 "obj"、索引缺省 0
// （ECMA-376 CT_Placeholder @type/@idx 默认值）。
type PhKey struct {
	Typ string
	Idx uint32
}

// PhKeyOf 读取形状（p:sp）的占位符键。非占位符返回 ok=false。
func PhKeyOf(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord) (PhKey, bool) {
	nvSpPr := xmlstore.ChildOfKind(doc, sp, ooxmlns.PresentationML, "nvSpPr", 0)
	if nvSpPr == nil {
		return PhKey{}, false
	}
	nvPr := xmlstore.ChildOfKind(doc, nvSpPr, ooxmlns.PresentationML, "nvPr", 0)
	if nvPr == nil {
		return PhKey{}, false
	}
	ph := xmlstore.ChildOfKind(doc, nvPr, ooxmlns.PresentationML, "ph", 0)
	if ph == nil {
		return PhKey{}, false
	}
	k := PhKey{Typ: "obj", Idx: 0}
	if t, ok := ph.Attr("", "type"); ok {
		k.Typ = t
	}
	if s, ok := ph.Attr("", "idx"); ok {
		if v, err := xmlstore.ParseUint32(s); err == nil {
			k.Idx = v
		}
	}
	return k, true
}

// FindPlaceholderShape 在 spTree 中查找 (type, idx) 规范化匹配的占位符
// 形状；未命中返回 nil。
func FindPlaceholderShape(doc *xmlstore.XMLDocument, want PhKey) *xmlstore.NodeRecord {
	for _, tid := range doc.Elements(ooxmlns.PresentationML, "spTree") {
		tree := doc.Node(tid)
		for _, sid := range tree.Children {
			sp := doc.Node(sid)
			if sp.Namespace != ooxmlns.PresentationML || sp.Local() != "sp" {
				continue
			}
			got, ok := PhKeyOf(doc, sp)
			if !ok || got != want {
				continue
			}
			return sp
		}
	}
	return nil
}

// AncestorShape 返回 run 最近的 p:sp 祖先；找不到返回 nil（形状位于
// 图形框架/表格等其它容器时，按无占位符普通文本处理）。
func AncestorShape(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for cur := doc.Node(run.Parent); cur != nil; cur = doc.Node(cur.Parent) {
		if cur.Namespace == ooxmlns.PresentationML && cur.Local() == "sp" {
			return cur
		}
	}
	return nil
}

// RunPara 返回 run 的父段落（a:p）；异常时返回 nil。
func RunPara(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if run.Parent == xmlstore.NoNode {
		return nil
	}
	n := doc.Node(run.Parent)
	if n.Namespace == ooxmlns.DrawingML && n.Local() == "p" {
		return n
	}
	return nil
}

// ParaLevel 返回段落列表级别 0..8（a:pPr@lvl，缺省 0；越界收敛到 8）。
func ParaLevel(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) int {
	pPr := xmlstore.ChildOfKind(doc, para, ooxmlns.DrawingML, "pPr", 0)
	if pPr == nil {
		return 0
	}
	s, ok := pPr.Attr("", "lvl")
	if !ok {
		return 0
	}
	v, err := xmlstore.ParseUint32(s)
	if err != nil {
		return 0
	}
	if v > 8 {
		return 8
	}
	return int(v)
}

// TextClass 是母版文本样式表的归类键（对应 p:titleStyle/bodyStyle/
// otherStyle/notesStyle）。
type TextClass string

const (
	ClassTitle TextClass = "title"
	ClassBody  TextClass = "body"
	ClassOther TextClass = "other"
	ClassNotes TextClass = "notes"
)

// ClassOf 由占位符类型归类（ECMA 语义：title/ctrTitle → 标题样式，
// 其余占位符 → 正文样式）。phOk=false（非占位符形状）→ otherStyle；
// 备注页统一为 notesStyle。
func ClassOf(k PhKey, phOk bool, kind string) TextClass {
	if kind == StyleKindNotes {
		return ClassNotes
	}
	if !phOk {
		return ClassOther
	}
	switch k.Typ {
	case "title", "ctrTitle":
		return ClassTitle
	default:
		return ClassBody
	}
}
