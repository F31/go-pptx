package pptx

import (
	"fmt"
	"strings"

	"github.com/F31/go-pptx/internal/bind"
)

// 本文件是模板绑定的**数据源解析与取值**：点分路径解析（resolve）、
// 集合迭代（resolveItems）、map/切片/结构体成员访问（member/asSlice）、
// 真值判定（truthy）与字符串化（formatBindValue）。

// ---------- 数据解析 ----------

// resolve 解析点分路径：首段先在 scope（行循环条目）中查找，未命中则
// 回退到根数据源；支持 map 键、切片下标与结构体字段。
func (s *bindScanner) resolve(scope any, path string) (any, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false, &OperationError{
			Op: "Presentation.Bind", Message: "empty placeholder path", Err: ErrInvalidArgument}
	}
	if path == "." {
		if scope == nil {
			return nil, false, nil
		}
		return scope, true, nil
	}
	segs := strings.Split(path, ".")
	var cur any
	start := 0
	if scope != nil {
		if v, ok := bind.Member(scope, segs[0]); ok {
			cur, start = v, 1
		}
	}
	if start == 0 {
		v, ok := bind.Member(s.data, segs[0])
		if !ok {
			return nil, false, nil
		}
		cur, start = v, 1
	}
	for _, seg := range segs[start:] {
		v, ok := bind.Member(cur, seg)
		if !ok {
			return nil, false, nil
		}
		cur = v
	}
	return cur, true, nil
}

// resolveItems 解析行循环数据，要求值为切片/数组。
func (s *bindScanner) resolveItems(path string) ([]any, error) {
	const op = "Presentation.Bind"
	v, found, err := s.resolve(nil, path)
	if err != nil {
		return nil, err
	}
	if !found {
		if s.o.strict {
			return nil, &OperationError{Op: op, Message: fmt.Sprintf(
				"row loop key %q not found in data", path), Err: ErrInvalidArgument}
		}
		s.diag("bind.key_missing", "", fmt.Sprintf("row loop key %q missing; no rows generated", path))
		return nil, nil
	}
	items, ok := bind.AsSlice(v)
	if !ok {
		return nil, &OperationError{Op: op, Message: fmt.Sprintf(
			"row loop key %q must be a slice, got %T", path, v), Err: ErrInvalidArgument}
	}
	return items, nil
}
