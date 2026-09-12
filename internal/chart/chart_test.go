// Package chart 单元测试（ADR-017 第一批）。
//
// 4 常量正确性测试 + 3 函数行为测试。
package chart

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// TestGraphicURIConstant 验证 GraphicURI 与 DrawingML 图表命名空间一致。
func TestGraphicURIConstant(t *testing.T) {
	const want = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	if GraphicURI != want {
		t.Errorf("GraphicURI = %q, want %q", GraphicURI, want)
	}
}

// TestSheetNameConstant 验证 SheetName 默认值与 chartbook.go 历史实现一致。
func TestSheetNameConstant(t *testing.T) {
	if SheetName != "Sheet1" {
		t.Errorf("SheetName = %q, want %q", SheetName, "Sheet1")
	}
}

// TestCatAxIDConstant 验证 CatAxID 固定为 100000001。
func TestCatAxIDConstant(t *testing.T) {
	if CatAxID != 100000001 {
		t.Errorf("CatAxID = %d, want %d", CatAxID, 100000001)
	}
}

// TestValAxIDConstant 验证 ValAxID 固定为 100000002。
func TestValAxIDConstant(t *testing.T) {
	if ValAxID != 100000002 {
		t.Errorf("ValAxID = %d, want %d", ValAxID, 100000002)
	}
}

// TestChartNumberPositive 验证 chartNumber 正数保持原值最短表示（与 chartbook.go 历史一致）。
func TestChartNumberPositive(t *testing.T) {
	got := ChartNumber(12.34)
	want := "12.34"
	if got != want {
		t.Errorf("ChartNumber(12.34) = %q, want %q", got, want)
	}
}

// TestChartNumberZero 验证 chartNumber 零值返回 "0"（与 chartbook.go 历史一致）。
func TestChartNumberZero(t *testing.T) {
	if got := ChartNumber(0); got != "0" {
		t.Errorf("ChartNumber(0) = %q, want %q", got, "0")
	}
}

// TestChartNumberNegative 验证 chartNumber 负数加 "-" 前缀（与 chartbook.go 历史一致）。
func TestChartNumberNegative(t *testing.T) {
	got := ChartNumber(-3.14)
	want := "-3.14"
	if got != want {
		t.Errorf("ChartNumber(-3.14) = %q, want %q", got, want)
	}
}

// TestChartNumberIntegerLike 验证整数自动去 .0 后缀（'g' 格式特征）。
func TestChartNumberIntegerLike(t *testing.T) {
	if got := ChartNumber(100); got != "100" {
		t.Errorf("ChartNumber(100) = %q, want %q", got, "100")
	}
	if got := ChartNumber(1e10); got != "1e+10" {
		t.Errorf("ChartNumber(1e10) = %q, want %q", got, "1e+10")
	}
}

// TestChartWorkbookColumnBasic 验证 chartWorkbookColumn 前 26 列映射正确。
func TestChartWorkbookColumnBasic(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "A"},
		{2, "B"},
		{26, "Z"},
		{27, "AA"},
		{28, "AB"},
		{52, "AZ"},
		{53, "BA"},
		{702, "ZZ"},
		{703, "AAA"},
	}
	for _, c := range cases {
		if got := WorkbookColumn(c.n); got != c.want {
			t.Errorf("WorkbookColumn(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestBuildChartFrameFragmentStructure 验证 BuildChartFrameFragment 输出包含
// p:graphicFrame 骨架 + a:graphicData + c:chart + r:id 引用。
func TestBuildChartFrameFragmentStructure(t *testing.T) {
	frag := BuildChartFrameFragment(7, 100, 200, 3000000, 4000000, "rId42")
	must := []string{
		`<p:graphicFrame>`,
		`<p:cNvPr id="7" name="Chart 7"/>`,
		`<a:off x="100" y="200"/>`,
		`<a:ext cx="3000000" cy="4000000"/>`,
		`<a:graphicData uri="` + GraphicURI + `">`,
		`<c:chart xmlns:c="` + GraphicURI + `" r:id="rId42"/>`,
		`</p:graphicFrame>`,
	}
	for _, sub := range must {
		if !strings.Contains(frag, sub) {
			t.Errorf("BuildChartFrameFragment output missing %q\nfull output: %s", sub, frag)
		}
	}
}

// TestBuildChartFrameFragmentLargeCoords 验证大数坐标精确写出。
func TestBuildChartFrameFragmentLargeCoords(t *testing.T) {
	frag := BuildChartFrameFragment(7, 100, 200, 3000000, 4000000, "rId42")
	big := strconv.FormatInt(3000000, 10)
	if !strings.Contains(frag, `cx="`+big+`"`) {
		t.Errorf("large cx not formatted verbatim: %s not in %s", big, frag)
	}
	big2 := strconv.FormatInt(4000000, 10)
	if !strings.Contains(frag, `cy="`+big2+`"`) {
		t.Errorf("large cy not formatted verbatim: %s not in %s", big2, frag)
	}
}

// TestBuildChartFrameFragmentZeroValues 验证零值 ID/坐标也能生成合法 XML。
func TestBuildChartFrameFragmentZeroValues(t *testing.T) {
	frag := BuildChartFrameFragment(0, 0, 0, 0, 0, "")
	if !bytes.HasPrefix([]byte(frag), []byte(`<p:graphicFrame>`)) {
		t.Errorf("BuildChartFrameFragment(0,0,0,0,0,\"\") does not start with <p:graphicFrame>:\n%s", frag)
	}
	// 必须包含空 r:id（不省略）。
	if !strings.Contains(frag, `r:id=""`) {
		t.Errorf("BuildChartFrameFragment empty rid must remain literal empty attribute:\n%s", frag)
	}
	// 验证 cNvPr id="0" name="Chart 0"——零值 ID 也保留。
	if !strings.Contains(frag, `id="0" name="Chart 0"`) {
		t.Errorf("zero-id literal cNvPr missing:\n%s", frag)
	}
}
