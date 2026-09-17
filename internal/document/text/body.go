package text

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/textutil"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// BodyProps 描述 a:bodyPr 中可安全写入的子集（TEXT-03 R 档）：
//   - Columns（numCol）：分栏数，>0；Columns=0 等价于"未设置"，删除
//     本地属性以恢复主题继承；
//   - Vertical（vert）：竖排方向；未设置取空串。合法取值见 VertAllowed；
//   - AnchorCenter（anchorCtr）：文本框内垂直居中（bool）。
//
// 三个字段都遵循"未提及属性 = 不动"语义。
type BodyProps struct {
	Columns      model.Optional[int]
	Vertical     model.Optional[string]
	AnchorCenter model.Optional[bool]
}

// vertAllowed 是 vert 属性的合法取值（OOXML ST_TextVerticalType）。
var vertAllowed = map[string]bool{
	"":              true, // 空串用作"删除本地属性"
	"horz":          true,
	"vert":          true,
	"vert270":       true,
	"wordArtVert":   true,
	"eaVert":        true,
	"mongolianVert": true,
}

// VertAllowed 报告 vert 值是否在合法白名单内。
func VertAllowed(v string) bool { return vertAllowed[v] }

// BodyPropsAnySet 检查 BodyProps 是否含显式设置。
func BodyPropsAnySet(p BodyProps) bool {
	return p.Columns.Set || p.Vertical.Set || p.AnchorCenter.Set
}

// ParseBodyProps 从 a:bodyPr 抽取 R 档字段（全部以 Set=true 输出）。
func ParseBodyProps(doc *xmlstore.XMLDocument, bp *xmlstore.NodeRecord) BodyProps {
	var out BodyProps
	for i := range bp.Attrs {
		a := &bp.Attrs[i]
		if a.Namespace != "" {
			continue
		}
		switch a.RawName {
		case "numCol":
			out.Columns = model.Optional[int]{Value: int(xmlstore.IntAttr(a.Value)), Set: true}
		case "vert":
			out.Vertical = model.Optional[string]{Value: a.Value, Set: true}
		case "anchorCtr":
			out.AnchorCenter = model.Optional[bool]{Value: a.Value == "1" || a.Value == "true", Set: true}
		}
	}
	return out
}

// ApplyBodyPropsPatch 计算对既有 bodyPr 的属性补丁集。
//
// Columns/Vertical/AnchorCenter 三个字段各自独立：
//   - Columns.Set：Columns.Value>0 写入/更新；Columns.Value<=0 删除
//   - Vertical.Set：Vertical.Value 非空写入/更新；空串删除
//   - AnchorCenter.Set：按 Value 写入/更新 true/false
func ApplyBodyPropsPatch(doc *xmlstore.XMLDocument, bp *xmlstore.NodeRecord, prefix string, p BodyProps) ([]xmlstore.SpanPatch, error) {
	var patches []xmlstore.SpanPatch
	// numCol
	if p.Columns.Set {
		if p.Columns.Value >= 1 {
			v := strconv.Itoa(p.Columns.Value)
			if pat, err := SetAttrPatch(bp, "numCol", v, prefix); err != nil {
				return nil, err
			} else if pat != nil {
				patches = append(patches, *pat)
			}
		} else {
			if pat := RemoveAttrIfExists(doc, bp, "numCol"); pat != nil {
				patches = append(patches, *pat)
			}
		}
	}
	// vert
	if p.Vertical.Set {
		if p.Vertical.Value != "" {
			if pat, err := SetAttrPatch(bp, "vert", p.Vertical.Value, prefix); err != nil {
				return nil, err
			} else if pat != nil {
				patches = append(patches, *pat)
			}
		} else {
			if pat := RemoveAttrIfExists(doc, bp, "vert"); pat != nil {
				patches = append(patches, *pat)
			}
		}
	}
	// anchorCtr
	if p.AnchorCenter.Set {
		v := "0"
		if p.AnchorCenter.Value {
			v = "1"
		}
		if pat, err := SetAttrPatch(bp, "anchorCtr", v, prefix); err != nil {
			return nil, err
		} else if pat != nil {
			patches = append(patches, *pat)
		}
	}
	return patches, nil
}

// SetAttrPatch 在 n 上写入/更新属性值；返回 nil 表示值未变化。
//
// 自闭合元素（<foo …/>）的 OpenEnd 指向 '>' 之后；插入位置需取 '/'
// 之前（即 OpenEnd-2）；非自闭合元素 OpenEnd 指向 '>' 之后，插入位
// 置取 '>' 之前（OpenEnd-1）。
func SetAttrPatch(n *xmlstore.NodeRecord, name, value, _ string) (*xmlstore.SpanPatch, error) {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			if a.Value == value {
				return nil, nil
			}
			p := xmlstore.SpanPatch{Start: a.ValueStart, End: a.ValueEnd, Replacement: []byte(value)}
			return &p, nil
		}
	}
	pos := n.OpenEnd - 1
	if n.SelfClosing() {
		pos = n.OpenEnd - 2
	}
	repl := []byte(" " + name + `="` + value + `"`)
	p := xmlstore.SpanPatch{Start: pos, End: pos, Replacement: repl}
	return &p, nil
}

// RemoveAttrIfExists 删除指定本地属性；不存在返回 nil。
func RemoveAttrIfExists(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, name string) *xmlstore.SpanPatch {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			sp := textutil.RemoveAttrPatch(doc, n, i)
			return &sp
		}
	}
	return nil
}

// BuildBodyPrFragment 构造完整 bodyPr 片段（自闭合形式）。
func BuildBodyPrFragment(prefix string, p BodyProps) (string, error) {
	var sb strings.Builder
	sb.WriteString("<" + prefix + ":bodyPr")
	if p.Columns.Set && p.Columns.Value >= 1 {
		sb.WriteString(` numCol="` + strconv.Itoa(p.Columns.Value) + `"`)
	}
	if p.Vertical.Set && p.Vertical.Value != "" {
		sb.WriteString(` vert="` + p.Vertical.Value + `"`)
	}
	if p.AnchorCenter.Set {
		v := "0"
		if p.AnchorCenter.Value {
			v = "1"
		}
		sb.WriteString(` anchorCtr="` + v + `"`)
	}
	sb.WriteString("/>")
	return sb.String(), nil
}

// NSPrefix 返回节点 QName 的前缀；缺省回落 "a"。
func NSPrefix(n *xmlstore.NodeRecord) string {
	if n.QName.Prefix != "" {
		return n.QName.Prefix
	}
	return "a"
}
