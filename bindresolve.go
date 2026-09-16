package pptx

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
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
		if v, ok := member(scope, segs[0]); ok {
			cur, start = v, 1
		}
	}
	if start == 0 {
		v, ok := member(s.data, segs[0])
		if !ok {
			return nil, false, nil
		}
		cur, start = v, 1
	}
	for _, seg := range segs[start:] {
		v, ok := member(cur, seg)
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
	items, ok := asSlice(v)
	if !ok {
		return nil, &OperationError{Op: op, Message: fmt.Sprintf(
			"row loop key %q must be a slice, got %T", path, v), Err: ErrInvalidArgument}
	}
	return items, nil
}

// member 取 map 键 / 切片下标 / 结构体字段（反射兜底支持
// []map[string]any 等具体类型）。
func member(v any, key string) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		x, ok := t[key]
		return x, ok
	case []any:
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i >= len(t) {
			return nil, false
		}
		return t[i], true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Map:
		kt := rv.Type().Key()
		if kt.Kind() != reflect.String {
			return nil, false
		}
		x := rv.MapIndex(reflect.ValueOf(key).Convert(kt))
		if !x.IsValid() {
			return nil, false
		}
		return x.Interface(), true
	case reflect.Slice, reflect.Array:
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i >= rv.Len() {
			return nil, false
		}
		return rv.Index(i).Interface(), true
	case reflect.Struct:
		f := rv.FieldByName(key)
		if !f.IsValid() || !f.CanInterface() {
			// 大小写不敏感回退（首字母小写字段名）。
			f = rv.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, key) })
			if !f.IsValid() || !f.CanInterface() {
				return nil, false
			}
		}
		return f.Interface(), true
	}
	return nil, false
}

// asSlice 把任意切片/数组值展开为 []any。
func asSlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	if t, ok := v.([]any); ok {
		return t, true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = rv.Index(i).Interface()
		}
		return out, true
	}
	return nil, false
}

// truthy 判定条件真值：nil / false / 空串 / 零数值 / 空容器 → 假。
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.Len() > 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() > 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.Bool:
		return rv.Bool()
	}
	return true
}

// formatBindValue 把数据值呈现为占位符文本（不可呈现类型返回 false）。
func formatBindValue(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case int:
		return strconv.Itoa(t), true
	case int8:
		return strconv.FormatInt(int64(t), 10), true
	case int16:
		return strconv.FormatInt(int64(t), 10), true
	case int32:
		return strconv.FormatInt(int64(t), 10), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case uint:
		return strconv.FormatUint(uint64(t), 10), true
	case uint8:
		return strconv.FormatUint(uint64(t), 10), true
	case uint16:
		return strconv.FormatUint(uint64(t), 10), true
	case uint32:
		return strconv.FormatUint(uint64(t), 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case time.Time:
		return t.Format("2006-01-02"), true
	case fmt.Stringer:
		return t.String(), true
	}
	return "", false
}
