package xmlstore

import "strings"

// 通用命名空间 URI 常量。xmlstore 本身不硬编码 OOXML 语义（doc.go），
// 这里仅提供 XML-01 需要的 mc（Markup Compatibility）上下文常量与若干
// OOXML 高频 URI，供上层以 URI/local name 识别元素（方案 §4.3），
// 不以前缀猜测语义（前缀可任意，URI 才具唯一性）。
const (
	// NSMarkupCompat 是 mc: 前缀通常绑定的标记兼容命名空间，
	// mc:AlternateContent / mc:Choice / mc:Fallback / mc:Ignorable 均属此 URI。
	NSMarkupCompat = "http://schemas.openxmlformats.org/markup-compatibility/2006"

	// NSDrawML 是 DrawingML 主命名空间（a: 前缀通常绑定）。
	NSDrawML = "http://schemas.openxmlformats.org/drawingml/2006/main"
	// NSPresentationML 是 PresentationML 主命名空间（p: 前缀通常绑定）。
	NSPresentationML = "http://schemas.openxmlformats.org/presentationml/2006/main"
	// NSRelationships 是关系 Part 的命名空间。
	NSRelationships = "http://schemas.openxmlformats.org/package/2006/relationships"
	// NSContentTypes 是 [Content_Types].xml 的命名空间。
	NSContentTypes = "http://schemas.openxmlformats.org/package/2006/content-types"
	// NSOfficeDocumentRelationships 是 r: 前缀通常绑定的关系引用命名空间。
	NSOfficeDocumentRelationships = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// NamespaceScope 是元素上的命名空间作用域：保存该元素自身声明的 xmlns
// 绑定，并链式指向父作用域以支持继承解析。不可变（构建期一次填充，
// 编辑期重建环境时整体新建，方案 §19.3）。
//
// 绑定键：显式前缀（"p"、"a"…）；默认命名空间使用键 ""。
// "xml" 与 "xmlns" 为规范保留前缀，不进入 decls。
type NamespaceScope struct {
	parent *NamespaceScope
	decls  map[string]string
}

// newScope 以 parent 为基创建作用域（parent 可为 nil，表示根文档作用域）。
func newScope(parent *NamespaceScope) *NamespaceScope {
	return &NamespaceScope{parent: parent, decls: make(map[string]string)}
}

// Resolve 返回前缀 prefix 在本作用域绑定到的 URI；未绑定返回 ok=false。
// 传入空串 "" 可查询默认命名空间。
func (s *NamespaceScope) Resolve(prefix string) (string, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if uri, ok := cur.decls[prefix]; ok {
			return uri, true
		}
	}
	return "", false
}

// bind 记录一条前缀绑定（调用方保证作用域为新建且仅构建期使用）。
func (s *NamespaceScope) bind(prefix, uri string) {
	s.decls[prefix] = uri
}

// Len 返回本作用域自身声明的绑定数（不含继承）。
func (s *NamespaceScope) Len() int { return len(s.decls) }

// declared 返回本作用域（含继承）中所有绑定中第一个把 uri 绑定到的前缀。
// 用于编辑期判断"该 URI 在目标作用域是否已有可用前缀"；未绑定返回 ok=false。
// 解析顺序为自身声明优先、外层次之，不保证稳定唯一（同一 URI 可被
// 多个前缀绑定），调用方应仅在判断存在性时使用。
func (s *NamespaceScope) declared(uri string) (string, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		for p, u := range cur.decls {
			if u == uri {
				return p, true
			}
		}
	}
	return "", false
}

// isXMLNSAttr 报告属性原始名是否为 xmlns 声明（默认或带前缀）。
func isXMLNSAttr(rawName string) bool {
	return rawName == "xmlns" || strings.HasPrefix(rawName, "xmlns:")
}

// xmlnsAttrPrefix 提取 xmlns 声明的绑定键：默认命名空间返回 ""，
// 显式前缀返回前缀部分；非声明属性返回 ("", false)。
func xmlnsAttrPrefix(rawName string) (string, bool) {
	switch {
	case rawName == "xmlns":
		return "", true
	case strings.HasPrefix(rawName, "xmlns:"):
		return rawName[len("xmlns:"):], true
	default:
		return "", false
	}
}
