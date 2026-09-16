package xmlstore

import "testing"

// TestChildOfKind 覆盖 nth 序号定位、跨命名空间隔离与未命中。
func TestChildOfKind(t *testing.T) {
	d := mustIndex(t, `<root xmlns:a="urn:a" xmlns:b="urn:b"><a:x/><b:x/><a:x/></root>`)
	root := d.Root()

	first := ChildOfKind(d, root, "urn:a", "x", 0)
	if first == nil || first.Namespace != "urn:a" {
		t.Fatalf("first a:x = %+v", first)
	}
	second := ChildOfKind(d, root, "urn:a", "x", 1)
	if second == nil || second == first {
		t.Fatalf("second a:x = %+v", second)
	}
	if got := ChildOfKind(d, root, "urn:a", "x", 2); got != nil {
		t.Fatalf("out-of-range nth = %+v, want nil", got)
	}
	// 命名空间隔离：urn:b 下同名元素独立计数。
	if got := ChildOfKind(d, root, "urn:b", "x", 0); got == nil || got.Namespace != "urn:b" {
		t.Fatalf("b:x = %+v", got)
	}
	// 未命中 local。
	if got := ChildOfKind(d, root, "urn:a", "y", 0); got != nil {
		t.Fatalf("missing local = %+v, want nil", got)
	}
}

// TestCountKind 覆盖计数与零命中。
func TestCountKind(t *testing.T) {
	d := mustIndex(t, `<root xmlns:a="urn:a" xmlns:b="urn:b"><a:x/><b:x/><a:x/><a:y/></root>`)
	root := d.Root()

	if got := CountKind(d, root, "urn:a", "x"); got != 2 {
		t.Fatalf("CountKind(a:x) = %d, want 2", got)
	}
	if got := CountKind(d, root, "urn:b", "x"); got != 1 {
		t.Fatalf("CountKind(b:x) = %d, want 1", got)
	}
	if got := CountKind(d, root, "urn:a", "z"); got != 0 {
		t.Fatalf("CountKind(a:z) = %d, want 0", got)
	}
}

// TestParseUint32 覆盖合法值、上界、空串、非数字与溢出。
func TestParseUint32(t *testing.T) {
	ok := []struct {
		in   string
		want uint32
	}{
		{"0", 0},
		{"1", 1},
		{"12345", 12345},
		{"4294967295", 1<<32 - 1},
	}
	for _, tc := range ok {
		got, err := ParseUint32(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("ParseUint32(%q) = %d, %v; want %d", tc.in, got, err, tc.want)
		}
	}
	bad := []string{"", "12a", "a12", "1 2", "-1", "4294967296", "99999999999999"}
	for _, in := range bad {
		if _, err := ParseUint32(in); err == nil {
			t.Fatalf("ParseUint32(%q) = nil err, want error", in)
		}
	}
}

func TestIntAttr(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int32
	}{
		{"0", 0},
		{"42", 42},
		{"-7", -7},
		{"2147483647", 2147483647},
		{"", 0},
		{"abc", 0},
		{"12x", 0},
		{"99999999999", 0},
	} {
		if got := IntAttr(tc.in); got != tc.want {
			t.Errorf("IntAttr(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
