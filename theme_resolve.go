package pptx

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 STYLE-01 的**主题/母版解析**：theme/master 文档、clrMap、
// 文本样式节点、字体 face（major/minor + typeface 展开）与 scheme 颜色解析。

// ---------- 主题与母版读取 ----------

// themeDoc 解析主题 Part 文档（nil 表示不可达）。
func (p *Presentation) themeDoc(env *style.Env) *xmlstore.XMLDocument {
	if env.Theme == "" {
		return nil
	}
	doc, err := p.docOf(env.Theme)
	if err != nil {
		return nil
	}
	return doc
}

// masterDoc 解析母版 Part 文档（slideMaster 或 notesMaster；nil 不可达）。
func (p *Presentation) masterDoc(env *style.Env) *xmlstore.XMLDocument {
	if env.Master == "" {
		return nil
	}
	doc, err := p.docOf(env.Master)
	if err != nil {
		return nil
	}
	return doc
}

// 母版/主题读取辅助（masterClrMap/textStyleNode/defRPrAtLevel/lstStyleOf/
// themeFontFace/expandTypeface/fontSchemeName）已迁至 internal/document/style。

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
