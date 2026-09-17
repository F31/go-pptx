package style

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是主题/母版的**纯读取辅助**：clrMap、文本样式节点、字体 face 与
// 主题字体引用展开。依赖注入的文档由调用方（门面）提供。

// MasterClrMap 读取母版 p:clrMap 的属性（bg1/bg2/tx1/tx2/hlink 等 →
// 主题色名）。无 clrMap 返回空表。
func MasterClrMap(doc *xmlstore.XMLDocument) map[string]string {
	out := make(map[string]string)
	if doc == nil {
		return out
	}
	for _, cid := range doc.Root().Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.PresentationML || c.Local() != "clrMap" {
			continue
		}
		for _, a := range c.Attrs {
			if a.Namespace == "" {
				out[a.RawName] = a.Value
			}
		}
	}
	return out
}

// TextStyleNode 返回母版 txStyles 中 class 对应的样式节点
// （CT_TextListStyle：直接含 a:lvlNpPr）。无 txStyles 或无该 class
// 返回 nil。
func TextStyleNode(doc *xmlstore.XMLDocument, class TextClass) *xmlstore.NodeRecord {
	if doc == nil {
		return nil
	}
	root := doc.Root()
	if root == nil {
		return nil
	}
	local := string(class) + "Style"
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.PresentationML {
			continue
		}
		if c.Local() == "txStyles" {
			return xmlstore.ChildOfKind(doc, c, ooxmlns.PresentationML, local, 0)
		}
	}
	return nil
}

// DefRPrAtLevel 从文本样式节点（CT_TextListStyle）取 lvl（0..8）的
// a:lvlNpPr/a:defRPr；该级缺失或该级无 defRPr 返回 nil。
func DefRPrAtLevel(doc *xmlstore.XMLDocument, style *xmlstore.NodeRecord, lvl int) *xmlstore.NodeRecord {
	if style == nil {
		return nil
	}
	lvlName := "lvl" + strconv.Itoa(lvl+1) + "pPr"
	pPr := xmlstore.ChildOfKind(doc, style, ooxmlns.DrawingML, lvlName, 0)
	if pPr == nil {
		return nil
	}
	return xmlstore.ChildOfKind(doc, pPr, ooxmlns.DrawingML, "defRPr", 0)
}

// LstStyleOf 返回占位符形状 txBody 的 a:lstStyle（无则 nil）。
func LstStyleOf(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	tx := xmlstore.ChildOfKind(doc, sp, ooxmlns.PresentationML, "txBody", 0)
	if tx == nil {
		return nil
	}
	return xmlstore.ChildOfKind(doc, tx, ooxmlns.DrawingML, "lstStyle", 0)
}

// ThemeFontFace 返回主题 fontScheme 中 major/minor 字体族 kind
// （latin/ea/cs）的 typeface；元素缺失返回 ok=false。
func ThemeFontFace(tdoc *xmlstore.XMLDocument, major bool, kind string) (string, bool) {
	if tdoc == nil {
		return "", false
	}
	root := tdoc.Root()
	if root == nil {
		return "", false
	}
	elems := xmlstore.ChildOfKind(tdoc, root, ooxmlns.DrawingML, "themeElements", 0)
	if elems == nil {
		return "", false
	}
	scheme := xmlstore.ChildOfKind(tdoc, elems, ooxmlns.DrawingML, "fontScheme", 0)
	if scheme == nil {
		return "", false
	}
	name := "minorFont"
	if major {
		name = "majorFont"
	}
	f := xmlstore.ChildOfKind(tdoc, scheme, ooxmlns.DrawingML, name, 0)
	if f == nil {
		return "", false
	}
	kindNode := xmlstore.ChildOfKind(tdoc, f, ooxmlns.DrawingML, kind, 0)
	if kindNode == nil {
		return "", false
	}
	return kindNode.Attr("", "typeface")
}

// ExpandTypeface 展开主题字体引用：typeface 以 "+mj-"/"+mn-" 开头时查
// 主题 fontScheme（+mj-lt → majorFont latin 等）。无前缀返回原样且
// 无需主题（ok=true，tstep 零值）。kind ∈ {latin,ea,cs}。
func ExpandTypeface(tdoc *xmlstore.XMLDocument, typeface, kind string) (final string, tstep StyleStep, ok bool) {
	major, isRef := themeFontScheme(typeface)
	if !isRef {
		return typeface, StyleStep{}, true
	}
	if tdoc == nil {
		return "", StyleStep{}, false
	}
	final, found := ThemeFontFace(tdoc, major, kind)
	if !found {
		return "", StyleStep{}, false
	}
	return final, StyleStep{Source: SourceTheme,
		Detail: "fontScheme " + FontSchemeName(major) + " " + kind}, true
}

// themeFontScheme 判断 "+mj-*"/"+mn-*" 引用指向 major 还是 minor。
func themeFontScheme(typeface string) (major, ok bool) {
	switch {
	case strings.HasPrefix(typeface, "+mj-"):
		return true, true
	case strings.HasPrefix(typeface, "+mn-"):
		return false, true
	}
	return false, false
}

// FontSchemeName 返回 major/minor 字体方案名。
func FontSchemeName(major bool) string {
	if major {
		return "majorFont"
	}
	return "minorFont"
}
