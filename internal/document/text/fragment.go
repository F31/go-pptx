package text

import (
	"strings"

	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/textutil"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// BodyRawShapeOK 检查 txBody 的子元素只属于 {a:bodyPr, a:lstStyle, a:p}
// （未知命名空间/未知本地名的元素视为不可安全删除的扩展）。
func BodyRawShapeOK(doc *xmlstore.XMLDocument, body *xmlstore.NodeRecord) bool {
	for _, cid := range body.Children {
		c := doc.Node(cid)
		if c.Namespace == ooxmlns.DrawingML &&
			(c.Local() == "bodyPr" || c.Local() == "lstStyle" || c.Local() == "p") {
			continue
		}
		return false
	}
	return true
}

// ParaPrefix 返回正文段落应使用的前缀（取首个 a:p 的前缀；txBody 至少
// 含一个 a:p 才合法，缺失时回退 "a" 依赖作用域）。
func ParaPrefix(doc *xmlstore.XMLDocument, body *xmlstore.NodeRecord) string {
	for _, cid := range body.Children {
		c := doc.Node(cid)
		if c.Namespace == ooxmlns.DrawingML && c.Local() == "p" && c.QName.Prefix != "" {
			return c.QName.Prefix
		}
	}
	return "a"
}

// BuildPlainParagraph 生成无字符格式的段落：<p:…><a:r><a:t>…</a:t></a:r></p:…>。
func BuildPlainParagraph(prefix, s string) string {
	esc, err := xmlstore.EscapeText(s)
	if err != nil {
		return "" // 非法 XML 字符由上层 EscapeText 在 SetPlainText 内统一拒绝
	}
	return "<" + prefix + ":p><" + prefix + ":r><" + prefix + ":t>" + esc + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":p>"
}

// RunPrefix 返回段落内 Run 应使用的前缀（取首个 a:r 前缀，缺省 "a"）。
func RunPrefix(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) string {
	for _, cid := range para.Children {
		c := doc.Node(cid)
		if c.Namespace == ooxmlns.DrawingML && c.Local() == "r" && c.QName.Prefix != "" {
			return c.QName.Prefix
		}
	}
	return "a"
}

// EndParaAnchor 返回段落末的 a:endParaRPr 元素（存在时）及其子序号，供
// 插入定位。
func EndParaAnchor(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) (*xmlstore.NodeRecord, int) {
	for i := len(para.Children) - 1; i >= 0; i-- {
		c := doc.Node(para.Children[i])
		if c.Namespace == ooxmlns.DrawingML && c.Local() == "endParaRPr" {
			return c, i
		}
	}
	return nil, -1
}

// paragraphRunOrField 是段落内 a:r 与 a:fld 的有序序列（XML 文档序）。
type paragraphRunOrField struct {
	isField bool
	run     *xmlstore.NodeRecord
	fld     *xmlstore.NodeRecord
}

// paragraphChildren 解析段落子元素为 a:r / a:fld 有序列表（忽略 pPr /
// endParaRPr / extLst 等非内容元素）。
func paragraphChildren(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) []paragraphRunOrField {
	var out []paragraphRunOrField
	for _, cid := range para.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "r":
			out = append(out, paragraphRunOrField{run: c})
		case "fld":
			out = append(out, paragraphRunOrField{isField: true, fld: c})
		}
	}
	return out
}

// ParagraphText 拼接段落 a:r/a:fld 缓存文本；a:fld 节点以 a:t 缓存文本
// 嵌入（字段位置即显示位置）。a:br 不贡献文本。
func ParagraphText(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) string {
	var sb strings.Builder
	for _, item := range paragraphChildren(doc, para) {
		if item.isField {
			tNode := xmlstore.ChildOfKind(doc, item.fld, ooxmlns.DrawingML, "t", 0)
			if tNode == nil || tNode.SelfClosing() {
				continue
			}
			sb.WriteString(textutil.XmlUnescape(string(doc.Original()[tNode.OpenEnd:tNode.CloseStart])))
			continue
		}
		for _, tid := range item.run.Children {
			tt := doc.Node(tid)
			if tt.Namespace == ooxmlns.DrawingML && tt.Local() == "t" {
				if tt.SelfClosing() {
					continue
				}
				sb.WriteString(textutil.XmlUnescape(string(doc.Original()[tt.OpenEnd:tt.CloseStart])))
			}
		}
	}
	return sb.String()
}
