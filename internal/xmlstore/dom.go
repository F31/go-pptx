package xmlstore

import (
	"errors"
	"fmt"
)

// 本文件提供与具体 OOXML 命名空间无关的 DOM 导航与属性解析辅助，
// 供根包与各 internal 子包共用（消除跨包重复实现）。

// ChildOfKind 返回 parent 下第 nth 个（0 基）namespace/local 匹配的直接
// 子元素；未命中返回 nil。
func ChildOfKind(doc *XMLDocument, parent *NodeRecord, ns, local string, nth int) *NodeRecord {
	seen := 0
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == ns && c.Local() == local {
			if seen == nth {
				return c
			}
			seen++
		}
	}
	return nil
}

// CountKind 统计 parent 下 namespace/local 匹配的直接子元素数。
func CountKind(doc *XMLDocument, parent *NodeRecord, ns, local string) int {
	n := 0
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == ns && c.Local() == local {
			n++
		}
	}
	return n
}

// ParseUint32 解析十进制无符号 32 位整数（用于 XML 属性值，如 idx/lvl）；
// 空串、非数字或溢出返回错误。
func ParseUint32(s string) (uint32, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	var v uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit %q", c)
		}
		v = v*10 + uint64(c-'0')
		if v > 1<<32-1 {
			return 0, fmt.Errorf("overflow")
		}
	}
	return uint32(v), nil
}
