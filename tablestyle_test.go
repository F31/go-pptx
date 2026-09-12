package pptx

import (
	"testing"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// resolveColorSpecFillDoc 构造一个以 solidFill 为根的解析索引，
// 返回 (doc, fill 节点) 供 resolveColorSpec 直接驱动。
func resolveColorSpecFillDoc(t *testing.T, inner string) (*xmlstore.XMLDocument, *xmlstore.NodeRecord) {
	t.Helper()
	doc, err := xmlstore.Index([]byte(`<a:solidFill xmlns:a="` + nsDrawingML + `">` + inner + `</a:solidFill>`))
	if err != nil {
		t.Fatalf("Index(%s): %v", inner, err)
	}
	return doc, doc.Root()
}

// TestResolveColorSpec 表驱动覆盖颜色解析全部分支：srgbClr（缺 val /
// 直出 / 带变换降级）、sysClr（lastClr / 缺 lastClr）、schemeClr（主题
// 直解 / clrMap 间接映射 / 未知名 / 缺 val / 带变换）、无颜色与未知
// 形态的诊断路径。环境取自最小模板（theme1.xml + slideMaster1.xml）。
func TestResolveColorSpec(t *testing.T) {
	p := openDocProps(t, minimalTemplateParts())
	defer p.Close()
	env := &styleEnv{
		kind:   styleKindSlide,
		master: opc.PartName("/ppt/slideMasters/slideMaster1.xml"),
		theme:  opc.PartName("/ppt/theme/theme1.xml"),
	}

	for _, tc := range []struct {
		name       string
		inner      string
		wantRGB    string
		wantSettld bool // 期望 Resolved
		wantDiag   string
	}{
		// srgbClr 路径。
		{"srgb-direct", `<a:srgbClr val="abcdef"/>`, "ABCDEF", true, ""},
		{"srgb-missing-val", `<a:srgbClr/>`, "", false, ""},
		{"srgb-transform", `<a:srgbClr val="abcdef"><a:lumMod val="50000"/></a:srgbClr>`, "ABCDEF", false, "STYLE_PARTIAL"},
		// sysClr 路径。
		{"sysclr-lastclr", `<a:sysClr val="windowText" lastClr="12345a"/>`, "12345A", true, ""},
		{"sysclr-no-lastclr", `<a:sysClr val="windowText"/>`, "", false, "STYLE_UNRESOLVED"},
		// schemeClr 路径（theme 来自最小模板：accent1=#4472C4，clrMap
		// bg1→lt1（window/FFFFFF））。
		{"scheme-direct", `<a:schemeClr val="accent1"/>`, "4472C4", true, ""},
		{"scheme-clrmap-indirect", `<a:schemeClr val="bg1"/>`, "FFFFFF", true, ""},
		{"scheme-unknown", `<a:schemeClr val="nope"/>`, "", false, "STYLE_PARTIAL"},
		{"scheme-missing-val", `<a:schemeClr/>`, "", false, ""},
		{"scheme-transform", `<a:schemeClr val="accent1"><a:lumMod val="50000"/></a:schemeClr>`, "4472C4", false, "STYLE_PARTIAL"},
		// 无颜色 / 未知形态。
		{"empty-fill", ``, "", false, "STYLE_UNRESOLVED"},
		{"unknown-color-kind", `<a:hslClr hue="1"/>`, "", false, "STYLE_UNRESOLVED"},
	} {
		doc, fill := resolveColorSpecFillDoc(t, tc.inner)
		var diags []Diagnostic
		rc := p.resolveColorSpec(doc, env, fill, "/ppt/slides/slide1.xml", &diags)
		if rc.RGB != tc.wantRGB {
			t.Errorf("%s: RGB = %q, want %q", tc.name, rc.RGB, tc.wantRGB)
		}
		if rc.Resolved != tc.wantSettld {
			t.Errorf("%s: Resolved = %v, want %v (diags=%v)", tc.name, rc.Resolved, tc.wantSettld, diags)
		}
		if tc.wantDiag != "" && !hasDiagCode(diags, tc.wantDiag) {
			t.Errorf("%s: missing diag %s, got %+v", tc.name, tc.wantDiag, diags)
		}
		if tc.wantDiag == "" && len(diags) > 0 {
			t.Errorf("%s: unexpected diags %+v", tc.name, diags)
		}
	}

	// 无主题环境（env.theme/env.master 为空）：schemeClr 部分解析降级。
	doc, fill := resolveColorSpecFillDoc(t, `<a:schemeClr val="accent1"/>`)
	var diags []Diagnostic
	rc := p.resolveColorSpec(doc, &styleEnv{kind: styleKindSlide}, fill, "part", &diags)
	if rc.Resolved || rc.RGB != "" {
		t.Errorf("no-theme schemeClr: %+v, want unresolved empty", rc)
	}
	if !hasDiagCode(diags, "STYLE_PARTIAL") {
		t.Errorf("no-theme schemeClr: want STYLE_PARTIAL diag, got %+v", diags)
	}
}
