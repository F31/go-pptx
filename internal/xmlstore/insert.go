package xmlstore

import (
	"errors"
	"fmt"
)

// 受控结构插入：把一个 well-formed 单根 XML 片段插入到既有文档的指定
// 位置，返回与补丁引擎同一形态的 SpanPatch（Start==End 的零长替换），
// 由 ApplyPatches 统一应用（方案 §19.3）。
//
// "受控"体现在三点：
//  1. 片段必须可独立解析（Index 成功，即 well-formed 且单根）；
//  2. 片段不得依赖插入点之外未声明的前缀——片段内显式前缀若在片段
//     自身作用域解析不到，必须在插入点作用域可解析，否则拒绝
//     （ErrFragmentNamespace）；片段自带 xmlns 声明则自足；
//  3. 插入点必须存在：AppendChild 不接受自闭合目标（需要重写标签，
//     首版显式拒绝，ErrInsertPoint）。
//
// 片段应用后插入点后续字节不受影响；重建索引由调用方负责。

var (
	// ErrInsertPoint 表示指定位置不支持插入（如自闭合元素内）。
	ErrInsertPoint = errors.New("xmlstore: invalid insert point")
	// ErrFragmentNamespace 表示片段依赖插入点作用域未声明的前缀，
	// 直接插入会产生不可解析的 XML（方案 §19.3：必须补齐声明或完整
	// 可证明的前缀重写；首版选择拒绝而非隐式重写）。
	ErrFragmentNamespace = errors.New("xmlstore: fragment depends on undeclared namespace prefix")
)

// InsertBefore 返回在元素 n 开标签之前插入 fragment 的补丁。
// 片段将成为 n 的前一个兄弟节点（继承 n 所在层的作用域）。
func InsertBefore(n *NodeRecord, fragment []byte) (SpanPatch, error) {
	if n == nil {
		return SpanPatch{}, &PatchError{Op: "range", Err: fmt.Errorf("%w: nil target", ErrInsertPoint)}
	}
	if err := checkFragmentScope(fragment, n.Scope()); err != nil {
		return SpanPatch{}, err
	}
	return SpanPatch{
		Start:       n.Source.Start,
		End:         n.Source.Start,
		Replacement: append([]byte(nil), fragment...),
		Desc:        fmt.Sprintf("insert before <%s>", n.Name()),
	}, nil
}

// InsertAfter 返回在元素 n 闭标签之后插入 fragment 的补丁。
// 片段将成为 n 的后一个兄弟节点。
func InsertAfter(n *NodeRecord, fragment []byte) (SpanPatch, error) {
	if n == nil {
		return SpanPatch{}, &PatchError{Op: "range", Err: fmt.Errorf("%w: nil target", ErrInsertPoint)}
	}
	if err := checkFragmentScope(fragment, n.Scope()); err != nil {
		return SpanPatch{}, err
	}
	return SpanPatch{
		Start:       n.Source.End,
		End:         n.Source.End,
		Replacement: append([]byte(nil), fragment...),
		Desc:        fmt.Sprintf("insert after <%s>", n.Name()),
	}, nil
}

// AppendChild 返回在 parent 元素内容末尾（闭标签之前）插入 fragment
// 的补丁。自闭合目标不支持（首版拒绝，不隐式改写标签形态）。
func AppendChild(parent *NodeRecord, fragment []byte) (SpanPatch, error) {
	if parent == nil {
		return SpanPatch{}, &PatchError{Op: "range", Err: fmt.Errorf("%w: nil target", ErrInsertPoint)}
	}
	if parent.SelfClosing() || parent.CloseStart < 0 {
		return SpanPatch{}, &PatchError{
			Op:  "range",
			Err: fmt.Errorf("%w: <%s> is self-closing", ErrInsertPoint, parent.Name()),
		}
	}
	if err := checkFragmentScope(fragment, parent.Scope()); err != nil {
		return SpanPatch{}, err
	}
	return SpanPatch{
		Start:       parent.CloseStart,
		End:         parent.CloseStart,
		Replacement: append([]byte(nil), fragment...),
		Desc:        fmt.Sprintf("append child to <%s>", parent.Name()),
	}, nil
}

// checkFragmentScope 校验片段可独立解析，且其使用的显式前缀要么在片段
// 自身已声明（Index 后 Namespace != ""），要么能在插入点作用域 scope
// 中解析到；否则返回 ErrFragmentNamespace。
//
// 无前缀元素/属性不校验：无默认命名空间绑定时它们属于"无命名空间"，
// 本身是合法 XML；对 OOXML 语义有要求的上层可另行约束。
func checkFragmentScope(fragment []byte, scope *NamespaceScope) error {
	fd, err := Index(fragment)
	if err != nil {
		return &PatchError{Op: "fragment", Err: err}
	}
	var walkErr error
	fd.walk(fd.root, func(n *NodeRecord) bool {
		if n.QName.Prefix != "" && n.Namespace == "" {
			if _, ok := scope.Resolve(n.QName.Prefix); !ok {
				walkErr = &PatchError{
					Op:  "fragment",
					Err: fmt.Errorf("%w: prefix %q on element <%s>", ErrFragmentNamespace, n.QName.Prefix, n.QName.String()),
				}
				return false
			}
		}
		for i := range n.Attrs {
			a := &n.Attrs[i]
			if q := splitQName(a.RawName); q.Prefix != "" && a.Namespace == "" {
				if _, ok := scope.Resolve(q.Prefix); !ok {
					walkErr = &PatchError{
						Op:  "fragment",
						Err: fmt.Errorf("%w: prefix %q on attribute %q", ErrFragmentNamespace, q.Prefix, a.RawName),
					}
					return false
				}
			}
		}
		return true
	})
	return walkErr
}
