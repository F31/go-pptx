package model

import "testing"

func TestEMU(t *testing.T) {
	if EMU(914400).Inches() != 1.0 {
		t.Fatal("1 inch")
	}
	if EMU(12700).Points() != 1.0 {
		t.Fatal("1 pt")
	}
	if EMU(0).Inches() != 0 || EMU(0).Points() != 0 {
		t.Fatal("zero")
	}
}
