package bind

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// 本文件是模板绑定的**无状态取值原语**：成员访问（Member）、切片展开
// （AsSlice）、真值判定（Truthy）与字符串化（FormatBindValue）。持句柄
// 的扫描/编排（resolve/resolveItems/scan/apply）仍留根包 pptx。

// Member 取 map 键 / 切片下标 / 结构体字段（反射兜底支持
// []map[string]any 等具体类型）。
func Member(v any, key string) (any, bool) {
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

// AsSlice 把任意切片/数组值展开为 []any。
func AsSlice(v any) ([]any, bool) {
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

// Truthy 判定条件真值：nil / false / 空串 / 零数值 / 空容器 → 假。
func Truthy(v any) bool {
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

// FormatBindValue 把数据值呈现为占位符文本（不可呈现类型返回 false）。
func FormatBindValue(v any) (string, bool) {
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
