package pptx

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 STYLE-01 的**主题/母版解析**：theme/master 文档、clrMap、
// 文本样式节点、字体 face（major/minor + typeface 展开）与 scheme 颜色解析。

// ---------- 主题与母版读取 ----------

// themeDoc 解析主题 Part 文档（nil 表示不可达）。
func (p *Presentation) themeDoc(env *styleEnv) *xmlstore.XMLDocument {
	if env.theme == "" {
		return nil
	}
	doc, err := p.docOf(env.theme)
	if err != nil {
		return nil
	}
	return doc
}

// masterDoc 解析母版 Part 文档（slideMaster 或 notesMaster；nil 不可达）。
func (p *Presentation) masterDoc(env *styleEnv) *xmlstore.XMLDocument {
	if env.master == "" {
		return nil
	}
	doc, err := p.docOf(env.master)
	if err != nil {
		return nil
	}
	return doc
}

// masterClrMap 读取母版 p:clrMap 的属性（bg1/bg2/tx1/tx2/hlink 等 →
// 主题色名）。无 clrMap 返回空表。
func masterClrMap(doc *xmlstore.XMLDocument) map[string]string {
	out := make(map[string]string)
	if doc == nil {
		return out
	}
	for _, cid := range doc.Root().Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML || c.Local() != "clrMap" {
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

// textStyleNode 返回母版 txStyles 中 class 对应的样式节点
// （CT_TextListStyle：直接含 a:lvlNpPr）。无 txStyles 或无该 class
// 返回 nil。
func textStyleNode(doc *xmlstore.XMLDocument, class textClass) *xmlstore.NodeRecord {
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
		if c.Namespace != nsPresentationML {
			continue
		}
		if c.Local() == "txStyles" {
			return childOfKind(doc, c, nsPresentationML, local, 0)
		}
	}
	return nil
}

// defRPrAtLevel 从文本样式节点（CT_TextListStyle）取 lvl（0..8）的
// a:lvlNpPr/a:defRPr；该级缺失或该级无 defRPr 返回 nil。
func defRPrAtLevel(doc *xmlstore.XMLDocument, style *xmlstore.NodeRecord, lvl int) *xmlstore.NodeRecord {
	if style == nil {
		return nil
	}
	lvlName := "lvl" + strconv.Itoa(lvl+1) + "pPr"
	pPr := childOfKind(doc, style, nsDrawingML, lvlName, 0)
	if pPr == nil {
		return nil
	}
	return childOfKind(doc, pPr, nsDrawingML, "defRPr", 0)
}

// lstStyleOf 返回占位符形状 txBody 的 a:lstStyle（无则 nil）。
func lstStyleOf(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	tx := childOfKind(doc, sp, nsPresentationML, "txBody", 0)
	if tx == nil {
		return nil
	}
	return childOfKind(doc, tx, nsDrawingML, "lstStyle", 0)
}

// themeFontFace 返回主题 fontScheme 中 major/minor 字体族 kind
// （latin/ea/cs）的 typeface；元素缺失返回 ok=false。
func themeFontFace(tdoc *xmlstore.XMLDocument, major bool, kind string) (string, bool) {
	if tdoc == nil {
		return "", false
	}
	root := tdoc.Root()
	if root == nil {
		return "", false
	}
	elems := childOfKind(tdoc, root, nsDrawingML, "themeElements", 0)
	if elems == nil {
		return "", false
	}
	scheme := childOfKind(tdoc, elems, nsDrawingML, "fontScheme", 0)
	if scheme == nil {
		return "", false
	}
	name := "minorFont"
	if major {
		name = "majorFont"
	}
	f := childOfKind(tdoc, scheme, nsDrawingML, name, 0)
	if f == nil {
		return "", false
	}
	kindNode := childOfKind(tdoc, f, nsDrawingML, kind, 0)
	if kindNode == nil {
		return "", false
	}
	return kindNode.Attr("", "typeface")
}

// expandTypeface 展开主题字体引用：typeface 以 "+mj-"/"+mn-" 开头时查
// 主题 fontScheme（+mj-lt → majorFont latin 等）。无前缀返回原样且
// 无需主题（ok=true，tstep 零值）。kind ∈ {latin,ea,cs}。
func expandTypeface(tdoc *xmlstore.XMLDocument, typeface, kind string) (final string, tstep StyleStep, ok bool) {
	major, isRef := themeFontScheme(typeface)
	if !isRef {
		return typeface, StyleStep{}, true
	}
	if tdoc == nil {
		return "", StyleStep{}, false
	}
	final, found := themeFontFace(tdoc, major, kind)
	if !found {
		return "", StyleStep{}, false
	}
	return final, StyleStep{Source: SourceTheme,
		Detail: "fontScheme " + fontSchemeName(major) + " " + kind}, true
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

func fontSchemeName(major bool) string {
	if major {
		return "majorFont"
	}
	return "minorFont"
}

// ---------- 颜色解析 ----------

// 已知系统色（sysClr）映射表；有 lastClr 属性时优先于本表。
var sysColorFallback = map[string]string{
	"windowtext": "000000",
	"window":     "FFFFFF",
}

// schemeRGB 解析主题 clrScheme 条目的可呈现 RGB。partial=true 表示
// 条目存在但无法完全解析（未知变换/未知系统色/未知 scheme/非
// srgbClr|sysClr 形态）；此时 rgb 可能为基础色（可部分参考）。
func schemeRGB(tdoc *xmlstore.XMLDocument, scheme string) (rgb string, partial bool) {
	if tdoc == nil {
		return "", true
	}
	root := tdoc.Root()
	if root == nil {
		return "", true
	}
	elems := childOfKind(tdoc, root, nsDrawingML, "themeElements", 0)
	if elems == nil {
		return "", true
	}
	cs := childOfKind(tdoc, elems, nsDrawingML, "clrScheme", 0)
	if cs == nil {
		return "", true
	}
	entry := childOfKind(tdoc, cs, nsDrawingML, scheme, 0)
	if entry == nil {
		return "", true // 未知 scheme 名
	}
	var colorNode *xmlstore.NodeRecord
	for _, cid := range entry.Children {
		c := tdoc.Node(cid)
		if c.Namespace == nsDrawingML {
			colorNode = c
			break
		}
	}
	if colorNode == nil {
		return "", true
	}
	base := ""
	switch colorNode.Local() {
	case "srgbClr":
		v, _ := colorNode.Attr("", "val")
		base = strings.ToUpper(v)
	case "sysClr":
		v, _ := colorNode.Attr("", "val")
		if lc, ok := colorNode.Attr("", "lastClr"); ok && lc != "" {
			base = strings.ToUpper(lc)
		} else if m, known := sysColorFallback[strings.ToLower(v)]; known {
			base = m
		} else {
			return "", true
		}
	default:
		return "", true
	}
	if !isHexRGB(base) {
		return "", true
	}
	// 变换序列（GEOM/M3 颜色变换全集：由 applyColorTransforms 统一处理）。
	rgb = base
	var ts []ColorTransform
	for _, cid := range colorNode.Children {
		c := tdoc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		v, _ := c.Attr("", "val")
		val, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			val = 0
		}
		ts = append(ts, ColorTransform{Kind: c.Local(), Value: int32(val)})
	}
	out, _, unknown := applyColorTransforms(rgb, ts)
	if len(unknown) > 0 {
		// 未知变换：保留基础色并标记部分解析（不臆造取值）。
		if isHexRGB(out) {
			return out, true
		}
		return base, true
	}
	// alpha 变换无法在 RGB 输出中表达 → 标记部分解析。
	for _, t := range ts {
		if t.Kind == "alpha" || t.Kind == "alphaMod" || t.Kind == "alphaOff" {
			return out, true
		}
	}
	return out, false
}

func isHexRGB(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func hexByte(s string) uint8 {
	v, _ := strconv.ParseUint(s, 16, 8)
	return uint8(v)
}
