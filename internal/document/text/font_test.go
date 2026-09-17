package text

import (
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/document/style"
	"github.com/F31/go-pptx/internal/errs"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

const dd = "http://schemas.openxmlformats.org/drawingml/2006/main"

func idx(t *testing.T, s string) *xmlstore.XMLDocument {
	t.Helper()
	d, err := xmlstore.Index([]byte(s))
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	return d
}

func rPrOf(t *testing.T, doc *xmlstore.XMLDocument) *xmlstore.NodeRecord {
	t.Helper()
	rPr := xmlstore.ChildOfKind(doc, doc.Root(), ooxmlns.DrawingML, "rPr", 0)
	if rPr == nil {
		t.Fatal("rPr not found")
	}
	return rPr
}

func TestIsNSDeclAttrIntStringBoolVal(t *testing.T) {
	if !IsNSDeclAttr("xmlns") || !IsNSDeclAttr("xmlns:a") || IsNSDeclAttr("b") {
		t.Fatal("IsNSDeclAttr mismatch")
	}
	for _, tc := range []struct {
		in   int
		want string
	}{{0, "0"}, {42, "42"}, {-7, "-7"}, {10013, "10013"}} {
		if got := IntString(tc.in); got != tc.want {
			t.Errorf("IntString(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if BoolVal(true) != "1" || BoolVal(false) != "0" {
		t.Fatal("BoolVal mismatch")
	}
}

// TestRPrChildRank 覆盖 rPr 子元素 schema 序号全分组 + 非法命名空间与
// 未知 local 名。
func TestRPrChildRank(t *testing.T) {
	for _, tc := range []struct {
		ns     string
		local  string
		want   int
		wantOK bool
	}{
		{ooxmlns.DrawingML, "ln", 0, true},
		{ooxmlns.DrawingML, "solidFill", 1, true},
		{ooxmlns.DrawingML, "noFill", 1, true},
		{ooxmlns.DrawingML, "gradFill", 1, true},
		{ooxmlns.DrawingML, "effectLst", 2, true},
		{ooxmlns.DrawingML, "highlight", 3, true},
		{ooxmlns.DrawingML, "uLn", 4, true},
		{ooxmlns.DrawingML, "uFillTx", 5, true},
		{ooxmlns.DrawingML, "latin", 6, true},
		{ooxmlns.DrawingML, "ea", 7, true},
		{ooxmlns.DrawingML, "cs", 8, true},
		{ooxmlns.DrawingML, "sym", 9, true},
		{ooxmlns.DrawingML, "hlinkClick", 10, true},
		{ooxmlns.DrawingML, "rtl", 11, true},
		{ooxmlns.DrawingML, "extLst", 12, true},
		{ooxmlns.DrawingML, "unknownElem", 0, false},
		{"http://other/ns", "latin", 0, false}, // 命名空间不符
	} {
		got, ok := RPrChildRank(tc.ns, tc.local)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("RPrChildRank(%q,%q) = (%d,%v), want (%d,%v)",
				tc.ns, tc.local, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestSizeCentipoints 验证 pt → 百分之一 pt 的四舍五入转换。
func TestSizeCentipoints(t *testing.T) {
	for _, tc := range []struct {
		in   style.FontSize
		want string
	}{
		{style.Pts(12), "1200"},
		{style.Pts(0), "0"},
		{style.Pts(10.55), "1055"},
		{style.Pts(10.555), "1056"},   // 四舍五入
		{style.Pts(10.554), "1055"},   //
		{style.Pts(100.125), "10013"}, // 10012.5 + 0.5 → 10013
	} {
		if got := SizeCentipoints(tc.in); got != tc.want {
			t.Errorf("SizeCentipoints(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFillChildOfSolidFillFragment(t *testing.T) {
	doc := idx(t, `<a:rPr xmlns:a="`+dd+`"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:rPr>`)
	if FillChildOf(doc, doc.Root()) == nil {
		t.Fatal("FillChildOf want non-nil")
	}
	empty := idx(t, `<a:rPr xmlns:a="`+dd+`"/>`)
	if FillChildOf(empty, empty.Root()) != nil {
		t.Fatal("FillChildOf want nil")
	}
	// 非 DrawingML 子元素 + DrawingML 非填充子元素 → nil。
	mixed := idx(t, `<a:rPr xmlns:a="`+dd+`" xmlns:x="urn:x"><x:foo/><a:latin typeface="L"/></a:rPr>`)
	if FillChildOf(mixed, mixed.Root()) != nil {
		t.Fatal("FillChildOf want nil for non-fill children")
	}
	if got := SolidFillFragment("a", style.ColorSpec{RGB: "FF0000"}); !strings.Contains(got, `srgbClr val="FF0000"`) {
		t.Fatalf("rgb frag = %s", got)
	}
	if got := SolidFillFragment("a", style.ColorSpec{Scheme: "accent1"}); !strings.Contains(got, `schemeClr val="accent1"`) {
		t.Fatalf("scheme frag = %s", got)
	}
}

func TestBuildRPrFragment(t *testing.T) {
	st := style.FontStyle{
		Bold:   model.NewOptional(true),
		Italic: model.NewOptional(false),
		Size:   model.NewOptional(style.Pts(12)),
		Color:  model.NewOptional(style.ColorSpec{RGB: "00FF00"}),
		Latin:  model.NewOptional("Calibri"),
	}
	frag, err := BuildRPrFragment("a", st)
	if err != nil {
		t.Fatalf("BuildRPrFragment: %v", err)
	}
	for _, want := range []string{`b="1"`, `i="0"`, `sz="1200"`, `srgbClr val="00FF00"`, `latin typeface="Calibri"`} {
		if !strings.Contains(frag, want) {
			t.Errorf("frag missing %q: %s", want, frag)
		}
	}
	// 无子元素：自闭合。
	selfClosed, err := BuildRPrFragment("a", style.FontStyle{Bold: model.NewOptional(true)})
	if err != nil {
		t.Fatalf("BuildRPrFragment self-closing: %v", err)
	}
	if !strings.HasSuffix(selfClosed, "/>") {
		t.Errorf("want self-closing, got %s", selfClosed)
	}
	// 空 ColorSpec → ErrInvalidArgument。
	if _, err := BuildRPrFragment("a", style.FontStyle{Color: model.NewOptional(style.ColorSpec{})}); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("empty color err = %v, want ErrInvalidArgument", err)
	}
}

func TestBuildFontPatchesCreatesRPr(t *testing.T) {
	bold := style.FontStyle{Bold: model.NewOptional(true)}
	// 有相邻子元素 → InsertBefore。
	doc := idx(t, `<a:r xmlns:a="`+dd+`"><a:t>x</a:t></a:r>`)
	patches, err := BuildFontPatches(doc, doc.Root(), "a", bold)
	if err != nil || len(patches) != 1 {
		t.Fatalf("with anchor: patches=%d err=%v", len(patches), err)
	}
	// 空 run（无子元素）→ AppendChild。
	doc2 := idx(t, `<a:r xmlns:a="`+dd+`"></a:r>`)
	patches, err = BuildFontPatches(doc2, doc2.Root(), "a", bold)
	if err != nil || len(patches) != 1 {
		t.Fatalf("empty run: patches=%d err=%v", len(patches), err)
	}
}

func TestBuildFontPatchesExistingRPr(t *testing.T) {
	// 非自闭合 rPr：属性就地改。
	doc := idx(t, `<a:r xmlns:a="`+dd+`"><a:rPr b="0"><a:latin typeface="X"/></a:rPr></a:r>`)
	patches, err := BuildFontPatches(doc, doc.Root(), "a", style.FontStyle{Bold: model.NewOptional(true)})
	if err != nil || len(patches) != 1 {
		t.Fatalf("existing rPr bold: patches=%d err=%v", len(patches), err)
	}
	// 既有 latin → 原地替换。
	patches, err = BuildFontPatches(doc, doc.Root(), "a", style.FontStyle{Latin: model.NewOptional("Y")})
	if err != nil || len(patches) != 1 {
		t.Fatalf("existing latin: patches=%d err=%v", len(patches), err)
	}
	// 自闭合 rPr → ExpandSelfClosingRPr。
	doc2 := idx(t, `<a:r xmlns:a="`+dd+`"><a:rPr xmlns:a="`+dd+`" b="0"/></a:r>`)
	patches, err = BuildFontPatches(doc2, doc2.Root(), "a", style.FontStyle{Italic: model.NewOptional(true), Latin: model.NewOptional("Y")})
	if err != nil || len(patches) != 1 {
		t.Fatalf("self-closing rPr: patches=%d err=%v", len(patches), err)
	}
	// 既有 b="0" 保留；新增 i="1" 与 latin 子元素。
	if got := string(patches[0].Replacement); !strings.Contains(got, `b="0"`) || !strings.Contains(got, `i="1"`) {
		t.Errorf("expanded = %s", got)
	}
}

func TestPatchExistingRPrColor(t *testing.T) {
	// 既有填充 → 替换。
	doc := idx(t, `<a:r xmlns:a="`+dd+`"><a:rPr><a:solidFill><a:srgbClr val="000000"/></a:solidFill></a:rPr></a:r>`)
	patches, err := PatchExistingRPr(doc, doc.Root(), rPrOf(t, doc), "a", style.FontStyle{Color: model.NewOptional(style.ColorSpec{RGB: "FFFFFF"})})
	if err != nil || len(patches) != 1 {
		t.Fatalf("replace fill: patches=%d err=%v", len(patches), err)
	}
	// 无填充 → 插入。
	doc2 := idx(t, `<a:r xmlns:a="`+dd+`"><a:rPr><a:latin typeface="X"/></a:rPr></a:r>`)
	patches, err = PatchExistingRPr(doc2, doc2.Root(), rPrOf(t, doc2), "a", style.FontStyle{Color: model.NewOptional(style.ColorSpec{Scheme: "accent1"})})
	if err != nil || len(patches) != 1 {
		t.Fatalf("insert fill: patches=%d err=%v", len(patches), err)
	}
	// 空颜色 → 错误。
	if _, err := PatchExistingRPr(doc2, doc2.Root(), rPrOf(t, doc2), "a", style.FontStyle{Color: model.NewOptional(style.ColorSpec{})}); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("empty color err = %v", err)
	}
}

func TestPatchExistingRPrAttrs(t *testing.T) {
	doc := idx(t, `<a:r xmlns:a="`+dd+`"><a:rPr b="1" i="1" sz="1200"><a:latin typeface="X"/></a:rPr></a:r>`)
	rPr := rPrOf(t, doc)
	// 全部同值 → 0 patch（覆盖 p==nil 分支）。
	patches, err := PatchExistingRPr(doc, doc.Root(), rPr, "a", style.FontStyle{
		Bold:   model.NewOptional(true),
		Italic: model.NewOptional(true),
		Size:   model.NewOptional(style.Pts(12)),
	})
	if err != nil || len(patches) != 0 {
		t.Fatalf("same attrs: patches=%d err=%v", len(patches), err)
	}
	// italic/size 变化 → 2 patch。
	patches, err = PatchExistingRPr(doc, doc.Root(), rPr, "a", style.FontStyle{
		Italic: model.NewOptional(false),
		Size:   model.NewOptional(style.Pts(18)),
	})
	if err != nil || len(patches) != 2 {
		t.Fatalf("changed attrs: patches=%d err=%v", len(patches), err)
	}
	// EA 已有 → 替换；CS 缺失 → 插入。
	doc2 := idx(t, `<a:r xmlns:a="`+dd+`"><a:rPr><a:ea typeface="E"/></a:rPr></a:r>`)
	patches, err = PatchExistingRPr(doc2, doc2.Root(), rPrOf(t, doc2), "a", style.FontStyle{
		EastAsian:     model.NewOptional("E2"),
		ComplexScript: model.NewOptional("C"),
	})
	if err != nil || len(patches) != 2 {
		t.Fatalf("ea/cs: patches=%d err=%v", len(patches), err)
	}
}

func TestSetRPrAttr(t *testing.T) {
	doc := idx(t, `<a:rPr xmlns:a="`+dd+`" b="1"/>`)
	rPr := doc.Root()
	if p, err := SetRPrAttr(doc, rPr, "b", "1", "a"); err != nil || p != nil {
		t.Errorf("same value: p=%v err=%v", p, err)
	}
	if p, err := SetRPrAttr(doc, rPr, "b", "0", "a"); err != nil || p == nil {
		t.Errorf("changed value: p=%v err=%v", p, err)
	}
	if p, err := SetRPrAttr(doc, rPr, "i", "1", "a"); err != nil || p == nil {
		t.Errorf("missing attr: p=%v err=%v", p, err)
	}
}

func TestInsertRPrChild(t *testing.T) {
	// 在 latin 前插入。
	doc := idx(t, `<a:rPr xmlns:a="`+dd+`"><a:latin typeface="X"/></a:rPr>`)
	if _, err := InsertRPrChild(doc, doc.Root(), "<a:solidFill/>", FillRank); err != nil {
		t.Errorf("insert before latin: %v", err)
	}
	// 排在其后 → 追加末尾。
	doc2 := idx(t, `<a:rPr xmlns:a="`+dd+`"><a:ln w="1"/></a:rPr>`)
	if _, err := InsertRPrChild(doc2, doc2.Root(), "<a:latin/>", LatinRank); err != nil {
		t.Errorf("append: %v", err)
	}
	// 未知子元素 → ErrUnsupportedEdit。
	doc3 := idx(t, `<a:rPr xmlns:a="`+dd+`"><a:foo/></a:rPr>`)
	if _, err := InsertRPrChild(doc3, doc3.Root(), "<a:solidFill/>", FillRank); !errors.Is(err, errs.ErrUnsupportedEdit) {
		t.Errorf("unknown child err = %v", err)
	}
}

func TestExpandSelfClosingRPr(t *testing.T) {
	doc := idx(t, `<a:rPr xmlns:a="`+dd+`" b="0"/>`)
	// 仅属性字段（且与既有属性不同名）→ 保持自闭合（跳过 xmlns 声明属性）。
	s, err := ExpandSelfClosingRPr(doc, doc.Root(), "a", style.FontStyle{Italic: model.NewOptional(true)})
	if err != nil {
		t.Fatalf("expand attrs only: %v", err)
	}
	if !strings.Contains(s, `i="1"`) || !strings.HasSuffix(s, "/>") {
		t.Errorf("attrs only = %s", s)
	}
	if strings.Contains(s, "xmlns") {
		t.Errorf("xmlns should be skipped: %s", s)
	}
	// b/sz 属性字段分支。
	s3, err := ExpandSelfClosingRPr(doc, doc.Root(), "a", style.FontStyle{Bold: model.NewOptional(true), Size: model.NewOptional(style.Pts(12))})
	if err != nil {
		t.Fatalf("expand b/sz: %v", err)
	}
	if !strings.Contains(s3, `b="0"`) || !strings.Contains(s3, `sz="1200"`) {
		t.Errorf("b/sz = %s", s3)
	}
	// 含子元素 → 展开。
	s2, err := ExpandSelfClosingRPr(doc, doc.Root(), "a", style.FontStyle{Latin: model.NewOptional("X")})
	if err != nil {
		t.Fatalf("expand children: %v", err)
	}
	if !strings.Contains(s2, "</a:rPr>") {
		t.Errorf("children = %s", s2)
	}
}

func TestRPrChildrenFragment(t *testing.T) {
	frag, err := RPrChildrenFragment("a", style.FontStyle{
		Color: model.NewOptional(style.ColorSpec{RGB: "112233"}),
		Latin: model.NewOptional("C"),
	})
	if err != nil {
		t.Fatalf("RPrChildrenFragment: %v", err)
	}
	if !strings.Contains(frag, "solidFill") || !strings.Contains(frag, `latin typeface="C"`) {
		t.Errorf("frag = %s", frag)
	}
	ea, err := RPrChildrenFragment("a", style.FontStyle{
		EastAsian:     model.NewOptional("E"),
		ComplexScript: model.NewOptional("C"),
	})
	if err != nil || !strings.Contains(ea, `ea typeface="E"`) || !strings.Contains(ea, `cs typeface="C"`) {
		t.Errorf("ea/cs frag = %s err=%v", ea, err)
	}
	if _, err := RPrChildrenFragment("a", style.FontStyle{Color: model.NewOptional(style.ColorSpec{})}); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("empty color err = %v", err)
	}
}
