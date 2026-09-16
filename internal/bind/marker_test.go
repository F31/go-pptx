package bind

import (
	"testing"

	"github.com/F31/go-pptx/internal/xmlstore"
)

const testNSDrawingML = "http://schemas.openxmlformats.org/drawingml/2006/main"

func TestBindPatchHelpers(t *testing.T) {
	doc, err := xmlstore.Index([]byte(`<a:p xmlns:a="` + testNSDrawingML + `"><a:r><a:t>x</a:t></a:r></a:p>`))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	p := doc.Root()
	patch := EmptyParaPatch(doc, p)
	if patch == nil {
		t.Fatal("EmptyParaPatch returned nil for non-empty para")
	}
	if patch.Desc != "empty-paragraph" || patch.Start >= patch.End {
		t.Fatalf("patch = %+v", patch)
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{*patch})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if string(out) != `<a:p xmlns:a="`+testNSDrawingML+`"></a:p>` {
		t.Fatalf("emptied para = %s", out)
	}
	// 自闭合 / 无子元素 → nil。
	doc2, err := xmlstore.Index([]byte(`<a:p xmlns:a="` + testNSDrawingML + `"/>`))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if got := EmptyParaPatch(doc2, doc2.Root()); got != nil {
		t.Fatalf("self-closing patch = %+v", got)
	}
	// DeletePatch 构造带锚定补丁。
	doc3, err := xmlstore.Index([]byte(`<a:p xmlns:a="` + testNSDrawingML + `"><a:r/></a:p>`))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	dp := DeletePatch(doc3, doc3.Root(), "del")
	if dp.Desc != "del" || dp.Expect == nil || dp.Start >= dp.End {
		t.Fatalf("DeletePatch = %+v", dp)
	}
	out3, err := xmlstore.ApplyPatches(doc3.Original(), []xmlstore.SpanPatch{dp})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if len(out3) != 0 {
		t.Fatalf("delete output = %s", out3)
	}
	// ChildElems 按本地名过滤。
	doc4, err := xmlstore.Index([]byte(`<a:p xmlns:a="` + testNSDrawingML + `"><a:r/><a:pPr/><a:r/></a:p>`))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if got := ChildElems(doc4, doc4.Root(), "r"); len(got) != 2 {
		t.Fatalf("ChildElems(r) = %d", len(got))
	}
	if got := ChildElems(doc4, doc4.Root(), "nope"); len(got) != 0 {
		t.Fatalf("ChildElems(nope) = %d", len(got))
	}
}

func TestBindPureParseDirectiveAndScanInline(t *testing.T) {
	if kind, path, ok := ParseDirective("{{#if show}}"); !ok || kind != "if" || path != "show" {
		t.Fatalf("if = %q %q %v", kind, path, ok)
	}
	if kind, _, ok := ParseDirective("{{#each rows}}"); !ok || kind != "each" {
		t.Fatalf("each = %q %v", kind, ok)
	}
	if kind, _, ok := ParseDirective("{{/if}}"); !ok || kind != "endif" {
		t.Fatalf("endif = %q %v", kind, ok)
	}
	if kind, _, ok := ParseDirective("{{/each}}"); !ok || kind != "endeach" {
		t.Fatalf("endeach = %q %v", kind, ok)
	}
	if _, _, ok := ParseDirective("{{#if }}"); ok {
		t.Fatal("empty if path matched")
	}
	if _, _, ok := ParseDirective("plain text"); ok {
		t.Fatal("non-directive matched")
	}
	toks := ScanInline("a {{x}} b {{ y }} c {{#if z}} d {{/if}}")
	if len(toks) != 2 {
		t.Fatalf("inline tokens = %v", toks)
	}
	if toks[0].Path != "x" || toks[1].Path != "y" {
		t.Fatalf("paths = %v", toks)
	}
	if got := ScanInline("no markers"); len(got) != 0 {
		t.Fatalf("no markers = %v", got)
	}
	if got := ScanInline("{{}}"); len(got) != 0 {
		t.Fatalf("empty marker = %v", got)
	}
	if got := ScanInline("{{unclosed"); len(got) != 0 {
		t.Fatalf("unclosed marker = %v", got)
	}
}
