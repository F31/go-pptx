package pptx

import (
	"errors"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/v2/internal/opc"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件是 TEXT-01 句柄模型的**节点/路径基元**：nodeStep 稳定元素路径、
// textNode 公共载体（含 shapeHint 的 STALE-GUARD 判定）与路径解析辅助
// （resolvePath/recordPath/childOfKind/countKind/kindIndex）。
// 句柄整体语义（不缓存 NodeID、按稳定路径重定位）见 text.go 文件头。

// nodeStep 是从根元素到目标元素路径上的一步：[ns,local] 匹配父元素
// 的第 nth 个同名单元素子节点（0 基序号）。
type nodeStep struct {
	ns    string
	local string
	nth   int
}

// textNode 是文本模型句柄的公共载体。
type textNode struct {
	p    *Presentation
	part opc.PartName
	path []nodeStep
	// shapeHint 是所属形状的 cNvPr@id（V2.6 §M8 textNode 句柄失效语义
	// 修复点）。locate 解析 path 后向上找最近 p:sp 的 cNvPr@id 与之比对；
	// 不等即 ErrStaleHandle。零值表示"无 hint"，退回纯路径判定（向
	// 后兼容）。
	shapeHint ShapeID
}

// locate 解析句柄路径，返回当前 revision 索引与目标元素。NotFound
// 语义统一映射为 ErrStaleHandle（Part 或路径目标已不存在）。shapeHint
// 不为 0 时还需通过所属形状 cNvPr@id 校验。
func (h *textNode) locate() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	if h.p == nil || h.p.closed {
		return nil, nil, Annotate(ErrClosed, "textNode")
	}
	doc, err := h.p.docOf(h.part)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, Annotate(ErrStaleHandle, "textNode")
		}
		return nil, nil, err
	}
	n := resolvePath(doc, h.path)
	if n == nil {
		return nil, nil, Annotate(ErrStaleHandle, "textNode")
	}
	if h.shapeHint != 0 {
		if got := shapeIDFromAncestors(doc, n); got != h.shapeHint {
			return nil, nil, Annotate(ErrStaleHandle, "textNode")
		}
	}
	return doc, n, nil
}

// shapeIDFromAncestors 从节点向上遍历，找到最近 p:sp/p:cxnSp/p:graphicFrame/
// p:grpSp 等"承载 cNvPr@id"的祖先，返回其 cNvPr@id；找不到返回 0。
//
// 语义：textNode 句柄的目标节点是 txBody/a:p/a:r 等深度嵌套元素，路径
// 上必然经过所属形状的 sp 元素。shapeHint 校验即查这个 sp 的 cNvPr@id。
func shapeIDFromAncestors(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) ShapeID {
	if doc == nil || n == nil {
		return 0
	}
	// 形状元素的命名空间与 local 名。p:sp、p:cxnSp、p:graphicFrame、
	// p:grpSp 都通过 p:cNvPr（cNvPr/cNvSpPr/cNvGrpSpPr）携带 id。
	shapeNS := "http://schemas.openxmlformats.org/presentationml/2006/main"
	shapeLocals := map[string]bool{
		"sp": true, "cxnSp": true, "graphicFrame": true, "grpSp": true,
	}
	cur := n
	for {
		if cur == nil {
			return 0
		}
		if cur.Namespace == shapeNS && shapeLocals[cur.Local()] {
			// 在形状元素内找 p:cNvPr（cNvPr 在 p:nvSpPr/p:nvCxnSpPr/
			// p:nvGraphicFramePr/p:nvGrpSpPr 包裹下，按"前缀 nv + 后缀 Pr"
			// 识别）。
			for _, cid := range cur.Children {
				c := doc.Node(cid)
				if c == nil {
					continue
				}
				if c.Namespace != shapeNS {
					continue
				}
				loc := c.Local()
				if !strings.HasPrefix(loc, "nv") || !strings.HasSuffix(loc, "Pr") {
					continue
				}
				for _, gcid := range c.Children {
					gc := doc.Node(gcid)
					if gc != nil && gc.Namespace == shapeNS && gc.Local() == "cNvPr" {
						for _, aid := range gc.Attrs {
							if aid.Local() == "id" {
								if id, err := strconv.ParseInt(aid.Value, 10, 64); err == nil {
									return ShapeID(id)
								}
							}
						}
					}
				}
			}
			return 0
		}
		if cur.Parent == xmlstore.NoNode {
			return 0
		}
		cur = doc.Node(cur.Parent)
	}
}

// resolvePath 按路径步骤从根元素下降；任一步无匹配返回 nil。
func resolvePath(doc *xmlstore.XMLDocument, steps []nodeStep) *xmlstore.NodeRecord {
	cur := doc.Root()
	if cur == nil {
		return nil
	}
	for _, st := range steps {
		found := -1
		seen := 0
		for _, cid := range cur.Children {
			c := doc.Node(cid)
			if c.Namespace == st.ns && c.Local() == st.local {
				if seen == st.nth {
					found = int(cid)
					break
				}
				seen++
			}
		}
		if found < 0 {
			return nil
		}
		cur = doc.Node(xmlstore.NodeID(found))
	}
	return cur
}

// recordPath 把 doc 中元素 id 的祖先链录为路径（根元素本身返回空路径）。
func recordPath(doc *xmlstore.XMLDocument, id xmlstore.NodeID) []nodeStep {
	var rev []nodeStep
	cur := doc.Node(id)
	if cur == nil || cur.Parent == xmlstore.NoNode {
		return nil
	}
	for cur != nil && cur.Parent != xmlstore.NoNode {
		parent := doc.Node(cur.Parent)
		nth := 0
		for _, cid := range parent.Children {
			if cid == cur.ID {
				break
			}
			c := doc.Node(cid)
			if c.Namespace == cur.Namespace && c.Local() == cur.Local() {
				nth++
			}
		}
		rev = append(rev, nodeStep{ns: cur.Namespace, local: cur.Local(), nth: nth})
		cur = parent
	}
	// 反转成根→叶。
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// childOfKind 返回 parent 下第 nth 个（0 基）ns/local 匹配的子元素。
// 实现已下沉至 xmlstore.ChildOfKind（供根包与 internal 子包共用）。
func childOfKind(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, ns, local string, nth int) *xmlstore.NodeRecord {
	return xmlstore.ChildOfKind(doc, parent, ns, local, nth)
}

// countKind 统计 parent 下 ns/local 匹配的子元素数。
// 实现已下沉至 xmlstore.CountKind。
func countKind(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, ns, local string) int {
	return xmlstore.CountKind(doc, parent, ns, local)
}

// kindIndex 返回 child 在其 parent 同名单元素兄弟中的序号（0 基）。
func kindIndex(doc *xmlstore.XMLDocument, child *xmlstore.NodeRecord) int {
	if child.Parent == xmlstore.NoNode {
		return 0
	}
	parent := doc.Node(child.Parent)
	n := 0
	for _, cid := range parent.Children {
		if cid == child.ID {
			return n
		}
		c := doc.Node(cid)
		if c.Namespace == child.Namespace && c.Local() == child.Local() {
			n++
		}
	}
	return n
}
