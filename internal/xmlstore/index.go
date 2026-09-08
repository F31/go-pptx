package xmlstore

import (
	"errors"
	"fmt"
)

// 节点索引树：把一份 XML Part 的原始字节解析为元素节点树，为每个元素
// 保留字节跨度（Source/OpenEnd/CloseStart）、命名空间作用域、属性和
// 子元素顺序（方案 §18.1）。树对全部命名空间的元素一视同仁——未知
// 子树照常建节点、保留原始字节与嵌套关系，不解释、不丢弃，从而支持
// "只改一个 Run、同页扩展与关联 Part 字节不变"的保真目标（方案 §4.3）。
//
// 非元素内容（文本、注释、CDATA、PI、DOCTYPE）不单独建节点：它们被
// 开/闭标签跨度夹在元素之间，原始字节始终完整保留在 XMLDocument.original
// 中；需要文本定位的层（textmap）基于跨度工作。元素 Source 覆盖其全部
// 后代字节，用于约束编辑不越出元素边界。

var (
	// ErrDepthLimit 表示 XML 嵌套深度超过预算（IndexWith.MaxDepth）。
	ErrDepthLimit = errors.New("xmlstore: XML nesting exceeds depth budget")
)

// DefaultMaxDepth 是未配置深度预算时的回退值（与 opc 包默认对齐，256）。
const DefaultMaxDepth = 256

// DepthError 携带超限深度的预算错误。
type DepthError struct {
	Offset int
	Depth  int
	Limit  int
}

func (e *DepthError) Error() string {
	return fmt.Sprintf("xmlstore: element depth %d exceeds limit %d (byte offset %d)",
		e.Depth, e.Limit, e.Offset)
}

// Unwrap 使 errors.Is(err, ErrDepthLimit) 成立。
func (e *DepthError) Unwrap() error { return ErrDepthLimit }

// NodeID 是文档内稳定元素句柄（方案 §18.1）。ID 分配后不复用；
// 删除/重建保留存活节点到新 ID 的映射由上层负责。
type NodeID int

// NoNode 表示无父节点（根元素）或无效引用。
const NoNode NodeID = -1

// ByteRange 是原始 UTF-8 字节的半开区间 [Start, End)。
type ByteRange struct{ Start, End int }

// AttributeRecord 是命名空间解析后的属性记录：保留原始名、解码值、
// 值区间与解析出的命名空间 URI（属性无默认命名空间，未绑定前缀留空）。
type AttributeRecord struct {
	RawName string
	Value   string
	// Namespace 是属性名前缀解析后的 URI；xmlns 声明自身为空。
	Namespace string
	// ValueStart/ValueEnd 是引号内原始值区间（半开）。
	ValueStart int
	ValueEnd   int
}

// Name 返回属性的扩展名（前缀:本地 或 本地）。
func (a AttributeRecord) Name() QName { return splitQName(a.RawName) }

// Local 返回属性本地名。
func (a AttributeRecord) Local() string { return splitQName(a.RawName).Local }

// NodeRecord 是元素节点（方案 §18.1 的 NodeRecord 落地形态）。
//
// Source 覆盖整个元素（开标签起始 ~ 闭标签 '>' 或自闭合 '/>'），
// OpenEnd 是开标签结束偏移（内容起始），CloseStart 是闭标签 '<' 偏移
// （自闭合元素为 -1）。三者均为原始字节偏移，半开区间语义与索引一致。
type NodeRecord struct {
	ID     NodeID
	Parent NodeID
	QName  QName
	// Namespace 是本元素前缀在自身作用域解析后的 URI（未绑定前缀留空）。
	Namespace  string
	Source     ByteRange
	OpenEnd    int
	CloseStart int
	Attrs      []AttributeRecord
	Children   []NodeID
	scope      *NamespaceScope
}

// Local 返回本地名（无前缀形式）。
func (n *NodeRecord) Local() string { return n.QName.Local }

// Name 返回原始名前缀形式（q:p 或 p）。
func (n *NodeRecord) Name() string { return n.QName.String() }

// Attr 返回首个 Namespace==ns 且 Local==local 的属性值；未找到返回
// ok=false。ns 传 "" 匹配无前缀属性（含未绑定前缀属性）。
func (n *NodeRecord) Attr(ns, local string) (string, bool) {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace != ns {
			continue
		}
		if q := splitQName(a.RawName); q.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// AttrLocal 返回首个本地名为 local 的属性值（不区分命名空间）；
// 用于 mc:Ignorable 等以原始名出现、前缀固定场景的便捷读取。
func (n *NodeRecord) AttrLocal(local string) (string, bool) {
	for i := range n.Attrs {
		if splitQName(n.Attrs[i].RawName).Local == local {
			return n.Attrs[i].Value, true
		}
	}
	return "", false
}

// Scope 返回本元素的命名空间作用域（含继承绑定，可 Resolve）。
// 根元素以上为 nil。
func (n *NodeRecord) Scope() *NamespaceScope { return n.scope }

// SelfClosing 报告元素是否为自闭合（无独立闭标签）。
func (n *NodeRecord) SelfClosing() bool { return n.CloseStart < 0 }

// ChildCount 返回直接元素子节点数。
func (n *NodeRecord) ChildCount() int { return len(n.Children) }

// XMLDocument 是一份 XML 的解析索引：保留原始字节 + 扁平元素表。
// NodeID 即 nodes 切片下标（0 起连续分配），因此
// XMLDocument 在索引重建时通过 id 映射表保持句柄语义（上层负责）。
type XMLDocument struct {
	original []byte
	nodes    []NodeRecord
	root     NodeID
}

// IndexOptions 控制索引构建的资源边界。
type IndexOptions struct {
	// MaxDepth 限制元素嵌套深度（<=0 回退 DefaultMaxDepth）。
	MaxDepth int
}

func (o IndexOptions) withDefaults() IndexOptions {
	if o.MaxDepth <= 0 {
		o.MaxDepth = DefaultMaxDepth
	}
	return o
}

// Index 以默认选项构建节点索引树。
func Index(data []byte) (*XMLDocument, error) { return IndexWith(data, IndexOptions{}) }

// IndexWith 以给定选项构建节点索引树。
//
// 错误：
//   - ErrEncoding：非 UTF-8（无法建立可靠字节映射，方案 §19.3）
//   - ErrMalformed：XML 结构不合法（由 Scanner 报告）
//   - ErrDepthLimit：元素嵌套深度超限（IndexOptions.MaxDepth）
//
// 成功返回后，doc.Original() 与入参 data 共享底层数组（只读约定）。
func IndexWith(data []byte, opts IndexOptions) (*XMLDocument, error) {
	opts = opts.withDefaults()
	s, err := NewScanner(data)
	if err != nil {
		return nil, err
	}

	doc := &XMLDocument{original: data, root: NoNode}
	type frame struct {
		id NodeID
	}
	var stack []frame
	// 根前内容（XML 声明/DOCTYPE/注释/PI）不出现在树中。
	var scope *NamespaceScope

	for s.Next() {
		tok := s.Token()
		switch tok.Kind {
		case TokenStart:
			depth := len(stack) + 1
			if depth > opts.MaxDepth {
				return nil, &DepthError{Offset: tok.Start, Depth: depth, Limit: opts.MaxDepth}
			}

			// 1) 收集本元素的 xmlns 声明（声明优先于属性/元素名解析，
			//    元素可绑定自身声明的前缀）。
			var ownDecls []Attr // 仅 xmlns 声明
			for _, a := range tok.Attrs {
				if isXMLNSAttr(a.RawName) {
					ownDecls = append(ownDecls, a)
				}
			}
			var cur *NamespaceScope
			if len(ownDecls) > 0 {
				cur = newScope(scope)
				for _, a := range ownDecls {
					pfx, _ := xmlnsAttrPrefix(a.RawName)
					cur.bind(pfx, a.Value)
				}
			} else {
				cur = scope
			}

			// 2) 元素自身 URI。
			ns := ""
			if tok.QName.Prefix != "" {
				if uri, ok := cur.Resolve(tok.QName.Prefix); ok {
					ns = uri
				}
				// 未绑定前缀保持空（容错；语义层可据 mc:Ignorable 等拒绝编辑）
			} else if uri, ok := cur.Resolve(""); ok {
				ns = uri
			}

			// 3) 属性解析（xmlns 声明之外）；属性无默认命名空间。
			var attrs []AttributeRecord
			if len(tok.Attrs) > 0 {
				attrs = make([]AttributeRecord, 0, len(tok.Attrs)-len(ownDecls))
				for _, a := range tok.Attrs {
					if isXMLNSAttr(a.RawName) {
						continue
					}
					rec := AttributeRecord{
						RawName:    a.RawName,
						Value:      a.Value,
						ValueStart: a.ValueStart,
						ValueEnd:   a.ValueEnd,
					}
					if a.Name.Prefix != "" {
						if uri, ok := cur.Resolve(a.Name.Prefix); ok {
							rec.Namespace = uri
						}
					}
					attrs = append(attrs, rec)
				}
			}

			id := NodeID(len(doc.nodes))
			node := NodeRecord{
				ID:         id,
				Parent:     NoNode,
				QName:      tok.QName,
				Namespace:  ns,
				Source:     ByteRange{Start: tok.Start, End: tok.End},
				OpenEnd:    tok.End,
				CloseStart: -1,
				Attrs:      attrs,
				scope:      cur,
			}
			if len(stack) > 0 {
				top := &doc.nodes[stack[len(stack)-1].id]
				node.Parent = top.ID
				top.Children = append(top.Children, id)
			} else if doc.root == NoNode {
				doc.root = id
			} else {
				return nil, &SyntaxError{Offset: tok.Start, Msg: "multiple root elements"}
			}
			doc.nodes = append(doc.nodes, node)
			if !tok.SelfClosing {
				stack = append(stack, frame{id: id})
				scope = cur
			}
		case TokenEnd:
			if len(stack) == 0 {
				// Scanner 已保证闭合匹配，此分支不可达；防御保留。
				return nil, &SyntaxError{Offset: tok.Start, Msg: "unbalanced end tag"}
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n := &doc.nodes[top.id]
			n.CloseStart = tok.Start
			n.Source.End = tok.End
			// 离开该元素后，作用域回退到其父作用域。
			if len(stack) == 0 {
				scope = nil
			} else {
				// 父元素作用域由父元素构建时确定；但若父元素无声明，
				// scope 指针在子元素入栈时被覆盖为子作用域——需要回退
				// 到父记录持有的 scope。
				scope = doc.nodes[stack[len(stack)-1].id].scope
			}
		default:
			// Text/Comment/PI/CDATA/Doctype：不建节点，字节保留在原数据中。
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if doc.root == NoNode {
		return nil, &SyntaxError{Offset: 0, Msg: "document has no root element"}
	}
	return doc, nil
}

// Original 返回原始字节（只读约定，勿修改）。
func (d *XMLDocument) Original() []byte { return d.original }

// Root 返回根元素记录；无根元素时返回 nil。
func (d *XMLDocument) Root() *NodeRecord { return d.Node(d.root) }

// Node 返回 id 对应的元素记录；越界或 NoNode 返回 nil。
func (d *XMLDocument) Node(id NodeID) *NodeRecord {
	if id < 0 || int(id) >= len(d.nodes) {
		return nil
	}
	return &d.nodes[id]
}

// Len 返回元素节点总数（含根）。
func (d *XMLDocument) Len() int { return len(d.nodes) }

// Elements 按文档序返回全部 Namespace==ns 且 Local==local 的元素。
// ns 传 "" 可匹配所有未绑定命名空间的元素；local 传 "" 匹配该 ns 下全部元素。
// 复杂度 O(N)；仅作导航辅助，高频路径由调用方缓存。
func (d *XMLDocument) Elements(ns, local string) []NodeID {
	var out []NodeID
	d.walk(d.root, func(n *NodeRecord) bool {
		if n.Namespace == ns && (local == "" || n.QName.Local == local) {
			out = append(out, n.ID)
		}
		return true
	})
	return out
}

// walk 先序（文档序）遍历以 start 为根的子树；fn 返回 false 停止。
func (d *XMLDocument) walk(start NodeID, fn func(*NodeRecord) bool) bool {
	if start == NoNode {
		return true
	}
	n := d.Node(start)
	if n == nil {
		return true
	}
	if !fn(n) {
		return false
	}
	for _, c := range n.Children {
		if !d.walk(c, fn) {
			return false
		}
	}
	return true
}

// Slice 返回原始字节的 [r.Start, r.End) 视图。
func (d *XMLDocument) Slice(r ByteRange) []byte {
	if r.Start < 0 || r.End > len(d.original) || r.Start > r.End {
		return nil
	}
	return d.original[r.Start:r.End]
}

// ContentSlice 返回元素内部内容字节（开标签之后、闭标签之前）。
func (d *XMLDocument) ContentSlice(n *NodeRecord) []byte {
	if n == nil || n.SelfClosing() {
		return nil
	}
	return d.original[n.OpenEnd:n.CloseStart]
}
