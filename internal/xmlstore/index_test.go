package xmlstore

import (
	"errors"
	"strings"
	"testing"
)

// mustIndex 解析成功否则 Fatal，返回文档。
func mustIndex(t *testing.T, doc string) *XMLDocument {
	t.Helper()
	d, err := Index([]byte(doc))
	if err != nil {
		t.Fatalf("Index %q: %v", doc, err)
	}
	return d
}

// collect 以 "ns|local" 形式收集以 start 为根的文档序元素。
func collect(d *XMLDocument, start NodeID) []string {
	var out []string
	d.walk(start, func(n *NodeRecord) bool {
		out = append(out, n.Namespace+"|"+n.QName.Local)
		return true
	})
	return out
}

func TestIndexBasicTree(t *testing.T) {
	doc := `<p:presentation xmlns:p="urn:pm">
  <p:sldIdLst>
    <p:sldId id="256"/>
    <p:sldId id="257"/>
  </p:sldIdLst>
</p:presentation>`
	d := mustIndex(t, doc)

	root := d.Root()
	if root == nil || root.Name() != "p:presentation" || root.Parent != NoNode {
		t.Fatalf("root = %+v", root)
	}
	if root.Namespace != "urn:pm" {
		t.Errorf("root ns = %q, want urn:pm", root.Namespace)
	}
	if d.Len() != 4 {
		t.Fatalf("Len = %d, want 4", d.Len())
	}

	lst := d.Node(root.Children[0])
	if lst.Name() != "p:sldIdLst" || lst.Parent != root.ID {
		t.Fatalf("lst = %+v", lst)
	}
	if got := lst.Children; len(got) != 2 {
		t.Fatalf("lst children = %v", got)
	}
	// 自闭合元素：无闭标签、Source 即整标签、是叶子。
	first := d.Node(lst.Children[0])
	if first.SelfClosing() != true {
		t.Error("sldId should be self-closing")
	}
	if first.CloseStart != -1 || first.ChildCount() != 0 {
		t.Errorf("sldId close = %d children = %d", first.CloseStart, first.ChildCount())
	}
	if got := string(d.Slice(first.Source)); got != `<p:sldId id="256"/>` {
		t.Errorf("first source slice = %q", got)
	}
}

func TestIndexSpansAndContent(t *testing.T) {
	doc := `<r:root xmlns:r="urn:r">text <r:a>in&amp;side</r:a><!--c--><r:b/></r:root>`
	d := mustIndex(t, doc)
	root := d.Root()

	a := d.Node(root.Children[0])
	wantOpen := `<r:a>`
	if got := string(d.original[a.Source.Start:a.OpenEnd]); got != wantOpen {
		t.Errorf("open = %q, want %q", got, wantOpen)
	}
	wantClose := `</r:a>`
	if got := string(d.original[a.CloseStart:a.Source.End]); got != wantClose {
		t.Errorf("close = %q, want %q", got, wantClose)
	}
	// 元素 Source 覆盖后代全部字节。
	if got := string(d.Slice(a.Source)); got != `<r:a>in&amp;side</r:a>` {
		t.Errorf("a source = %q", got)
	}
	// ContentSlice 保留原始转义（不重新解码）。
	if got := string(d.ContentSlice(a)); got != "in&amp;side" {
		t.Errorf("a content = %q, want in&amp;side", got)
	}

	b := d.Node(root.Children[1])
	if got := string(d.Slice(b.Source)); got != `<r:b/>` {
		t.Errorf("b source = %q", got)
	}
	if d.ContentSlice(b) != nil {
		t.Error("self-closing content should be nil")
	}
	// 相邻元素区间不重叠且保序（注释字节落在两者之间，树不建模但保留）。
	if a.Source.End > b.Source.Start {
		t.Errorf("span overlap: a ends %d > b starts %d", a.Source.End, b.Source.Start)
	}
}

func TestIndexNamespaceScoping(t *testing.T) {
	doc := `<a:x xmlns:p="urn:1">
  <p:y xmlns:p="urn:2" xmlns="" flag="1"/>
  <p:z/>
  <q:w xmlns:q="urn:3"/>
</a:x>`
	d := mustIndex(t, doc)
	root := d.Root()
	if root.Namespace != "" {
		t.Errorf("root ns = %q, want '' (no default ns)", root.Namespace)
	}

	y := d.Node(root.Children[0])
	if y.Namespace != "urn:2" {
		t.Errorf("y ns = %q, want urn:2 (shadowed)", y.Namespace)
	}
	// 无前缀属性无默认命名空间；显式前缀属性按元素作用域解析。
	if v, ok := y.Attr("", "flag"); !ok || v != "1" {
		t.Errorf("y flag attr = %q ok=%v", v, ok)
	}
	if len(y.Attrs) != 1 {
		t.Fatalf("y attrs = %+v (xmlns decls excluded)", y.Attrs)
	}

	// 离开 y 后作用域回退到父层：p 再次绑定 urn:1。
	z := d.Node(root.Children[1])
	if z.Namespace != "urn:1" {
		t.Errorf("z ns = %q, want urn:1 (scope restored)", z.Namespace)
	}
	// q 元素使用自身新声明。
	w := d.Node(root.Children[2])
	if w.Namespace != "urn:3" {
		t.Errorf("w ns = %q, want urn:3", w.Namespace)
	}
}

func TestIndexElementSelfDeclaredPrefix(t *testing.T) {
	doc := `<foo:bar xmlns:foo="urn:f"><foo:baz/></foo:bar>`
	d := mustIndex(t, doc)
	root := d.Root()
	if root.Namespace != "urn:f" {
		t.Errorf("root ns = %q, want urn:f (self-declared)", root.Namespace)
	}
	// 自身声明的绑定对属性同样生效。
	doc2 := `<r:root xmlns:r="urn:r" r:id="9"/>`
	d2 := mustIndex(t, doc2)
	r2 := d2.Root()
	if v, ok := r2.Attr("urn:r", "id"); !ok || v != "9" {
		t.Errorf("attr r:id = %q ok=%v, want 9", v, ok)
	}
}

func TestIndexMCAlternateContentPreserved(t *testing.T) {
	doc := `<p:sld xmlns:p="urn:pm" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" mc:Ignorable="p14">
  <p:spTree>
    <mc:AlternateContent>
      <mc:Choice Requires="p14"><p:sp/></mc:Choice>
      <mc:Fallback><p:sp/></mc:Fallback>
    </mc:AlternateContent>
  </p:spTree>
</p:sld>`
	d := mustIndex(t, doc)
	root := d.Root()
	// mc:Ignorable 属性以 {NSMarkupCompat, Ignorable} 形式保留。
	if v, ok := root.Attr(NSMarkupCompat, "Ignorable"); !ok || v != "p14" {
		t.Errorf("mc:Ignorable = %q ok=%v, want p14", v, ok)
	}

	alt := d.Elements(NSMarkupCompat, "AlternateContent")
	if len(alt) != 1 {
		t.Fatalf("AlternateContent nodes = %v, want 1", alt)
	}
	node := d.Node(alt[0])
	seq := collect(d, node.ID)
	want := []string{
		NSMarkupCompat + "|AlternateContent",
		NSMarkupCompat + "|Choice",
		"urn:pm|sp",
		NSMarkupCompat + "|Fallback",
		"urn:pm|sp",
	}
	if strings.Join(seq, ",") != strings.Join(want, ",") {
		t.Errorf("alt subtree order = %v, want %v", seq, want)
	}
}

func TestIndexUnknownSubtreePreserved(t *testing.T) {
	doc := `<p:root xmlns:p="urn:pm" xmlns:ext="urn:ext">
  <ext:ext ext:flag="1">
    <ext:inner>keep &amp; me</ext:inner>
    <ext:self/>
  </ext:ext>
  <p:known/>
</p:root>`
	d := mustIndex(t, doc)
	root := d.Root()
	ext := d.Node(root.Children[0])
	if ext.Namespace != "urn:ext" || ext.ChildCount() != 2 {
		t.Fatalf("ext node = %+v", ext)
	}
	inner := d.Node(ext.Children[0])
	if got := string(d.ContentSlice(inner)); got != "keep &amp; me" {
		t.Errorf("inner content = %q (raw entities must be kept)", got)
	}
	if v, ok := ext.Attr("urn:ext", "flag"); !ok || v != "1" {
		t.Errorf("ext attr = %q ok=%v", v, ok)
	}
	// 未知子树边界：ext.Source 覆盖其全部后代。
	if got := string(d.Slice(ext.Source)); !strings.Contains(got, "ext:inner") {
		t.Errorf("ext source slice = %q", got)
	}
}

func TestIndexElementsQuery(t *testing.T) {
	doc := `<r:root xmlns:r="urn:s"><s:leaf xmlns:s="urn:s"/><t:leaf xmlns:t="urn:s"/><r:leaf/></r:root>`
	d := mustIndex(t, doc)
	// 等价前缀异写：两个不同前缀绑到同一 URI，语义识别应一致。
	leaves := d.Elements("urn:s", "leaf")
	if len(leaves) != 3 {
		t.Fatalf("Elements = %v, want 3", leaves)
	}
	// 文档序断言。
	if d.Node(leaves[0]).Name() != "s:leaf" || d.Node(leaves[2]).Name() != "r:leaf" {
		t.Errorf("doc order broken: %v", leaves)
	}
	if got := d.Elements("urn:nope", ""); len(got) != 0 {
		t.Errorf("Elements miss = %v", got)
	}
}

func TestIndexDepthBudget(t *testing.T) {
	doc := `<a><b><c><d/></c></b></a>`
	// 4 层嵌套：预算 3 应拒绝（d 元素深度 4 > 3）。
	if _, err := IndexWith([]byte(doc), IndexOptions{MaxDepth: 3}); !errors.Is(err, ErrDepthLimit) {
		t.Fatalf("err = %v, want ErrDepthLimit", err)
	}
	// 预算 4 恰好通过。
	if _, err := IndexWith([]byte(doc), IndexOptions{MaxDepth: 4}); err != nil {
		t.Fatalf("MaxDepth=4: %v", err)
	}
	// 未配置回退默认（256 足够）。
	if _, err := Index([]byte(doc)); err != nil {
		t.Fatalf("default depth: %v", err)
	}
	var de *DepthError
	if _, err := IndexWith([]byte(doc), IndexOptions{MaxDepth: 2}); errors.As(err, &de) {
		if de.Depth != 3 || de.Limit != 2 || de.Offset <= 0 {
			t.Errorf("DepthError = %+v", de)
		}
	}
}

func TestIndexMalformedPropagates(t *testing.T) {
	for _, doc := range []string{
		`<a><b></a>`,     // 错配
		`<a></a><b></b>`, // 多根
		`<a>`,            // EOF 未闭合
	} {
		_, err := Index([]byte(doc))
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("Index(%q) err = %v, want ErrMalformed", doc, err)
		}
	}
}

func TestIndexPreambleIgnored(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<!-- top comment -->
<!DOCTYPE p:presentation>
<p:presentation xmlns:p="urn:pm"/>`
	d := mustIndex(t, doc)
	root := d.Root()
	if root.Name() != "p:presentation" || d.Len() != 1 {
		t.Fatalf("root = %s len=%d, want single element", root.Name(), d.Len())
	}
}

func TestIndexOriginalSharedAndRootless(t *testing.T) {
	// 无根元素：只有注释/文本 → 报错。
	if _, err := Index([]byte("just text")); err == nil {
		t.Error("rootless doc should fail")
	}
	// Original 与输入共享底层（读取视图，契约约定不改写）。
	data := []byte(`<a/>`)
	d, err := Index(data)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if &d.original[0] != &data[0] {
		t.Error("Original should share input backing array")
	}
}

func TestNodeAndAttributeAccessorsEdges(t *testing.T) {
	d := mustIndex(t, `<r xmlns:p="urn:p" p:id="7" plain="x"><a/></r>`)
	root := d.Root()
	if root.Local() != "r" {
		t.Fatalf("root local = %q", root.Local())
	}
	if v, ok := root.AttrLocal("id"); !ok || v != "7" {
		t.Fatalf("AttrLocal(id) = %q ok=%v", v, ok)
	}
	if _, ok := root.AttrLocal("missing"); ok {
		t.Fatal("missing AttrLocal matched")
	}
	if root.Scope() == nil {
		t.Fatal("missing root scope")
	}
	if uri, ok := root.Scope().Resolve("p"); !ok || uri != "urn:p" {
		t.Fatalf("scope p = %q ok=%v", uri, ok)
	}
	if got := root.Attrs[0].Name(); got.Prefix != "p" || got.Local != "id" {
		t.Fatalf("attr name = %+v", got)
	}
	if got := root.Attrs[1].Local(); got != "plain" {
		t.Fatalf("attr local = %q", got)
	}
	if n := d.Node(NodeID(99)); n != nil {
		t.Fatalf("out-of-range node = %+v", n)
	}
	if got := d.Slice(ByteRange{Start: -1, End: 1}); got != nil {
		t.Fatalf("invalid slice = %q", got)
	}
	if got := d.Slice(ByteRange{Start: 2, End: 1}); got != nil {
		t.Fatalf("reversed slice = %q", got)
	}
	if got := d.Slice(ByteRange{Start: 0, End: len(d.Original()) + 1}); got != nil {
		t.Fatalf("oversized slice = %q", got)
	}
}

func TestDepthErrorFormatting(t *testing.T) {
	_, err := IndexWith([]byte(`<a><b/></a>`), IndexOptions{MaxDepth: 1})
	var de *DepthError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want DepthError", err)
	}
	if !strings.Contains(de.Error(), "depth 2 exceeds limit 1") {
		t.Fatalf("DepthError string = %q", de.Error())
	}
}
