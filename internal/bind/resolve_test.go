package bind

import (
	"testing"
	"time"
)

// ---------- TPL-01 纯 helper 边界（v2.0 随 resolve.go 迁入） ----------

type bindStringer struct{ v string }

func (s bindStringer) String() string { return "s:" + s.v }

type bindFields struct {
	Name  string
	Count int
	ok    bool // 未导出字段
}

func TestBindPureMember(t *testing.T) {
	m := map[string]any{"name": "A", "inner": map[string]any{"qty": 3}, "items": []any{"x", "y"}}
	if v, ok := Member(m, "name"); !ok || v != "A" {
		t.Fatalf("map member = %v %v", v, ok)
	}
	if v, ok := Member(m, "inner"); !ok {
		t.Fatalf("nested map missing: %v %v", v, ok)
	}
	if v, ok := Member(m, "missing"); ok {
		t.Fatalf("missing key matched: %v", v)
	}
	if v, ok := Member([]any{1, 2, 3}, "1"); !ok || v != 2 {
		t.Fatalf("slice member = %v %v", v, ok)
	}
	if v, ok := Member([]any{1}, "5"); ok {
		t.Fatalf("slice oob matched: %v", v)
	}
	if _, ok := Member([]any{1}, "x"); ok {
		t.Fatal("non-numeric slice index matched")
	}
	// 具体类型反射路径。
	if v, ok := Member(bindFields{Name: "N", Count: 7}, "Count"); !ok || v != 7 {
		t.Fatalf("struct field = %v %v", v, ok)
	}
	if _, ok := Member(bindFields{ok: true}, "ok"); ok {
		t.Fatal("unexported struct field matched")
	}
	if v, ok := Member(&bindFields{Name: "P"}, "name"); !ok || v != "P" {
		t.Fatalf("case-insensitive ptr struct = %v %v", v, ok)
	}
	var nilMap map[string]any
	if _, ok := Member(nilMap, "k"); ok {
		t.Fatal("nil map matched")
	}
	var nilPtr *bindFields
	if _, ok := Member(nilPtr, "Name"); ok {
		t.Fatal("nil ptr matched")
	}
}

func TestBindPureAsSlice(t *testing.T) {
	if _, ok := AsSlice(nil); ok {
		t.Fatal("nil matched")
	}
	if s, ok := AsSlice([]any{1, 2}); !ok || len(s) != 2 {
		t.Fatalf("[]any = %v %v", s, ok)
	}
	if s, ok := AsSlice([]string{"a", "b", "c"}); !ok || len(s) != 3 || s[2] != "c" {
		t.Fatalf("[]string = %v %v", s, ok)
	}
	if s, ok := AsSlice([2]int{5, 6}); !ok || len(s) != 2 || s[1] != 6 {
		t.Fatalf("array = %v %v", s, ok)
	}
	var nilSlice []string
	if s, ok := AsSlice(nilSlice); !ok || len(s) != 0 {
		t.Fatalf("nil slice = %v %v, want empty ok", s, ok)
	}
	if _, ok := AsSlice(42); ok {
		t.Fatal("scalar matched")
	}
}

func TestBindPureTruthy(t *testing.T) {
	falsy := []any{nil, false, "", 0, 0.0, int64(0), uint32(0), []any{}, map[string]any{}, []int{}}
	for _, v := range falsy {
		if Truthy(v) {
			t.Fatalf("Truthy(%#v) = true, want false", v)
		}
	}
	truth := []any{true, "x", 1, 2.5, int8(-1), uint64(9), []any{1}, map[string]any{"a": 1}}
	for _, v := range truth {
		if !Truthy(v) {
			t.Fatalf("Truthy(%#v) = false, want true", v)
		}
	}
	var nilMap map[string]int
	if Truthy(nilMap) {
		t.Fatal("nil map truthy")
	}
}

func TestBindPureMemberTypedReflection(t *testing.T) {
	// 具体类型 map（type switch 不命中，走 reflect.Map）。
	if v, ok := Member(map[string]int{"k": 5}, "k"); !ok || v != 5 {
		t.Fatalf("typed map = %v %v", v, ok)
	}
	// 非字符串键 map → false。
	if _, ok := Member(map[int]string{1: "a"}, "1"); ok {
		t.Fatal("non-string-key map matched")
	}
	// 具体类型 slice / array（reflect.Slice/Array）。
	if v, ok := Member([]int{10, 20, 30}, "1"); !ok || v != 20 {
		t.Fatalf("typed slice = %v %v", v, ok)
	}
	if v, ok := Member([2]string{"a", "b"}, "1"); !ok || v != "b" {
		t.Fatalf("typed array = %v %v", v, ok)
	}
	if _, ok := Member([]int{1}, "9"); ok {
		t.Fatal("typed slice oob matched")
	}
	if _, ok := Member([]int{1}, "x"); ok {
		t.Fatal("typed slice non-numeric matched")
	}
}

func TestBindPureTruthyTypedReflection(t *testing.T) {
	if Truthy(map[string]int{}) {
		t.Fatal("empty typed map truthy")
	}
	if !Truthy(map[string]int{"k": 1}) {
		t.Fatal("non-empty typed map falsy")
	}
	if Truthy([]string{}) {
		t.Fatal("empty typed slice truthy")
	}
	if !Truthy([]string{"x"}) {
		t.Fatal("non-empty typed slice falsy")
	}
	type MyString string
	if Truthy(MyString("")) {
		t.Fatal("empty typed string truthy")
	}
	if !Truthy(MyString("x")) {
		t.Fatal("non-empty typed string falsy")
	}
	type MyBool bool
	if Truthy(MyBool(false)) {
		t.Fatal("false typed bool truthy")
	}
	if !Truthy(MyBool(true)) {
		t.Fatal("true typed bool falsy")
	}
	// 未知 kind（struct）→ 默认真。
	if !Truthy(struct{ A int }{1}) {
		t.Fatal("struct default should be truthy")
	}
}

func TestBindPureFormatBindValue(t *testing.T) {
	for in, want := range map[any]string{
		nil:               "",
		"str":             "str",
		true:              "true",
		false:             "false",
		42:                "42",
		int8(8):           "8",
		int16(16):         "16",
		int32(32):         "32",
		int64(64):         "64",
		uint(1):           "1",
		uint8(8):          "8",
		uint16(16):        "16",
		uint32(32):        "32",
		uint64(64):        "64",
		float32(1.5):      "1.5",
		float64(2.75):     "2.75",
		bindStringer{"x"}: "s:x",
	} {
		got, ok := FormatBindValue(in)
		if !ok || got != want {
			t.Fatalf("FormatBindValue(%#v) = %q %v, want %q", in, got, ok, want)
		}
	}
	if got, ok := FormatBindValue(time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)); !ok || got != "2026-09-11" {
		t.Fatalf("FormatBindValue(time) = %q %v", got, ok)
	}
	if _, ok := FormatBindValue(struct{ A int }{1}); ok {
		t.Fatal("struct matched")
	}
}
