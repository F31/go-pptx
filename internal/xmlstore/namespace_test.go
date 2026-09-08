package xmlstore

import "testing"

func TestXMLNSAttrDetection(t *testing.T) {
	cases := []struct {
		raw     string
		isDecl  bool
		prefix  string
		declHit bool
	}{
		{raw: "xmlns", isDecl: true, prefix: "", declHit: true},
		{raw: "xmlns:p", isDecl: true, prefix: "p", declHit: true},
		{raw: "xmlns:mc", isDecl: true, prefix: "mc", declHit: true},
		{raw: "p:id", isDecl: false},
		{raw: "xmlnsfoo", isDecl: false},
		{raw: "id", isDecl: false},
		{raw: "xmlns:", isDecl: true, prefix: "", declHit: true}, // 空前缀异常形，防御语义
	}
	for _, c := range cases {
		if got := isXMLNSAttr(c.raw); got != c.isDecl {
			t.Errorf("isXMLNSAttr(%q) = %v, want %v", c.raw, got, c.isDecl)
		}
		pfx, ok := xmlnsAttrPrefix(c.raw)
		if ok != c.declHit || pfx != c.prefix {
			t.Errorf("xmlnsAttrPrefix(%q) = (%q,%v), want (%q,%v)",
				c.raw, pfx, ok, c.prefix, c.declHit)
		}
	}
}

func TestNamespaceScopeChain(t *testing.T) {
	root := newScope(nil)
	root.bind("p", "urn:pm")
	root.bind("a", "urn:dm")
	root.bind("", "urn:default")

	child := newScope(root)
	child.bind("a", "urn:override") // 遮蔽父层
	child.bind("q", "urn:q")

	if uri, ok := child.Resolve("p"); !ok || uri != "urn:pm" {
		t.Errorf("child p = %q,%v want urn:pm", uri, ok)
	}
	if uri, ok := child.Resolve("a"); !ok || uri != "urn:override" {
		t.Errorf("child a = %q,%v want urn:override (shadow)", uri, ok)
	}
	if uri, ok := child.Resolve("q"); !ok || uri != "urn:q" {
		t.Errorf("child q = %q,%v want urn:q", uri, ok)
	}
	if uri, ok := child.Resolve(""); !ok || uri != "urn:default" {
		t.Errorf("child default = %q,%v want urn:default", uri, ok)
	}
	if _, ok := child.Resolve("nope"); ok {
		t.Error("unbound prefix should resolve false")
	}

	// Len 只计自身声明。
	if child.Len() != 2 || root.Len() != 3 {
		t.Errorf("Len child=%d root=%d", child.Len(), root.Len())
	}

	// declared：本层与外层都可发现；未绑定 URI 失败。
	if _, ok := child.declared("urn:pm"); !ok {
		t.Error("declared should find inherited binding")
	}
	if _, ok := child.declared("urn:override"); !ok {
		t.Error("declared should find own binding")
	}
	if _, ok := child.declared("urn:missing"); ok {
		t.Error("declared should miss unbound uri")
	}
	// 独立子作用域不受兄弟影响。
	sib := newScope(root)
	if _, ok := sib.Resolve("q"); ok {
		t.Error("sibling should not see sibling bindings")
	}
}

func TestNamespaceScopeConstants(t *testing.T) {
	if NSMarkupCompat != "http://schemas.openxmlformats.org/markup-compatibility/2006" {
		t.Error("NSMarkupCompat drift")
	}
	// mc:Ignorable 等按 URI 识别：常量应可被 Elements 用于过滤。
	doc := `<r:r xmlns:r="urn:r" xmlns:mc="` + NSMarkupCompat + `" mc:Ignorable="w"/>`
	d := mustIndex(t, doc)
	if v, ok := d.Root().Attr(NSMarkupCompat, "Ignorable"); !ok || v != "w" {
		t.Errorf("mc:Ignorable lookup = %q,%v", v, ok)
	}
}
