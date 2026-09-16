package pptx

import (
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 STYLE-01 的**占位符匹配与层级导航**：phKey/phKeyOf、占位符形状
// 查找、祖先形状/段落定位、层级与文本类（classOf）。

// ---------- 占位符与段落上下文 ----------

// phKey 是占位符的规范化匹配键：type 缺省 "obj"、idx 缺省 0
// （ECMA-376 CT_Placeholder @type/@idx 默认值）。
type phKey struct {
	typ string
	idx uint32
}

// phKeyOf 读取形状（p:sp）的占位符键。非占位符返回 ok=false。
func phKeyOf(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord) (phKey, bool) {
	nvSpPr := childOfKind(doc, sp, nsPresentationML, "nvSpPr", 0)
	if nvSpPr == nil {
		return phKey{}, false
	}
	nvPr := childOfKind(doc, nvSpPr, nsPresentationML, "nvPr", 0)
	if nvPr == nil {
		return phKey{}, false
	}
	ph := childOfKind(doc, nvPr, nsPresentationML, "ph", 0)
	if ph == nil {
		return phKey{}, false
	}
	k := phKey{typ: "obj", idx: 0}
	if t, ok := ph.Attr("", "type"); ok {
		k.typ = t
	}
	if s, ok := ph.Attr("", "idx"); ok {
		if v, err := parseUint32(s); err == nil {
			k.idx = v
		}
	}
	return k, true
}

// findPlaceholderShape 在 spTree 中查找 (type, idx) 规范化匹配的占位符
// 形状；未命中返回 nil。
func findPlaceholderShape(doc *xmlstore.XMLDocument, want phKey) *xmlstore.NodeRecord {
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for _, sid := range tree.Children {
			sp := doc.Node(sid)
			if sp.Namespace != nsPresentationML || sp.Local() != "sp" {
				continue
			}
			got, ok := phKeyOf(doc, sp)
			if !ok || got != want {
				continue
			}
			return sp
		}
	}
	return nil
}

// ancestorShape 返回 run 最近的 p:sp 祖先；找不到返回 nil（形状位于
// 图形框架/表格等其它容器时，按无占位符普通文本处理）。
func ancestorShape(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for cur := doc.Node(run.Parent); cur != nil; cur = doc.Node(cur.Parent) {
		if cur.Namespace == nsPresentationML && cur.Local() == "sp" {
			return cur
		}
	}
	return nil
}

// runPara 返回 run 的父段落（a:p）；异常时返回 nil。
func runPara(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if run.Parent == xmlstore.NoNode {
		return nil
	}
	n := doc.Node(run.Parent)
	if n.Namespace == nsDrawingML && n.Local() == "p" {
		return n
	}
	return nil
}

// paraLevel 返回段落列表级别 0..8（a:pPr@lvl，缺省 0；越界收敛到 8）。
func paraLevel(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) int {
	pPr := childOfKind(doc, para, nsDrawingML, "pPr", 0)
	if pPr == nil {
		return 0
	}
	s, ok := pPr.Attr("", "lvl")
	if !ok {
		return 0
	}
	v, err := parseUint32(s)
	if err != nil {
		return 0
	}
	if v > 8 {
		return 8
	}
	return int(v)
}

// textClass 是母版文本样式表的归类键（对应 p:titleStyle/bodyStyle/
// otherStyle/notesStyle）。
type textClass string

const (
	classTitle textClass = "title"
	classBody  textClass = "body"
	classOther textClass = "other"
	classNotes textClass = "notes"
)

// classOf 由占位符类型归类（ECMA 语义：title/ctrTitle → 标题样式，
// 其余占位符 → 正文样式）。phOk=false（非占位符形状）→ otherStyle；
// 备注页统一为 notesStyle。
func classOf(k phKey, phOk bool, kind string) textClass {
	if kind == styleKindNotes {
		return classNotes
	}
	if !phOk {
		return classOther
	}
	switch k.typ {
	case "title", "ctrTitle":
		return classTitle
	default:
		return classBody
	}
}
