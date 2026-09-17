package model

import "testing"

func TestOptional(t *testing.T) {
	zero := Optional[int]{}
	if zero.Set {
		t.Fatal("zero should be unset")
	}
	v := NewOptional(42)
	if !v.Set || v.Value != 42 {
		t.Fatalf("NewOptional = %+v", v)
	}
	var s Optional[string]
	if s.Set {
		t.Fatal("string zero should be unset")
	}
}
