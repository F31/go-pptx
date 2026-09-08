package xmlstore

import (
	"errors"
	"strings"
	"testing"
)

// mustApply 构造、应用并断言结果。
func mustApply(t *testing.T, data []byte, patches []SpanPatch) []byte {
	t.Helper()
	out, err := ApplyPatches(data, patches)
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	return out
}

func TestApplySingleTextPatch(t *testing.T) {
	doc := `<p:r xmlns:p="urn:pm"><p:t>Hello</p:t></p:r>`
	d := mustIndex(t, doc)
	txt := d.Elements("urn:pm", "t")
	if len(txt) != 1 {
		t.Fatalf("t elements = %v", txt)
	}
	n := d.Node(txt[0])
	start, end := n.OpenEnd, n.CloseStart

	p, err := NewTextPatch(start, end, "你好 <世界>")
	if err != nil {
		t.Fatalf("NewTextPatch: %v", err)
	}
	out := mustApply(t, []byte(doc), []SpanPatch{p})
	want := `<p:r xmlns:p="urn:pm"><p:t>你好 &lt;世界&gt;</p:t></p:r>`
	if string(out) != want {
		t.Errorf("result = %q, want %q", out, want)
	}
	// 应用结果可再次解析（well-formed round-trip）。
	if _, err := Index(out); err != nil {
		t.Fatalf("re-index: %v", err)
	}
}

func TestApplyDescendingMultiplePatches(t *testing.T) {
	doc := `<r><a>one</a><b>two</b><c>three</c></r>`
	d := mustIndex(t, doc)
	root := d.Root()

	var patches []SpanPatch
	for i, child := range root.Children {
		n := d.Node(child)
		p, err := NewTextPatch(n.OpenEnd, n.CloseStart, string(rune('X'+i)))
		if err != nil {
			t.Fatalf("patch %d: %v", i, err)
		}
		patches = append(patches, p)
	}
	out := mustApply(t, []byte(doc), patches)
	want := `<r><a>X</a><b>Y</b><c>Z</c></r>`
	if string(out) != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

func TestPatchOverlapDetection(t *testing.T) {
	data := []byte(`0123456789`)
	cases := []struct {
		name string
		ps   []SpanPatch
	}{
		{"partial overlap", []SpanPatch{
			{Start: 0, End: 5, Replacement: []byte("a")},
			{Start: 3, End: 8, Replacement: []byte("b")},
		}},
		{"containment", []SpanPatch{
			{Start: 2, End: 9, Replacement: []byte("a")},
			{Start: 4, End: 6, Replacement: []byte("b")},
		}},
		{"identical range", []SpanPatch{
			{Start: 1, End: 4, Replacement: []byte("a")},
			{Start: 1, End: 4, Replacement: []byte("b")},
		}},
	}
	for _, c := range cases {
		_, err := ApplyPatches(data, c.ps)
		if !errors.Is(err, ErrPatchOverlap) {
			t.Errorf("%s: err = %v, want ErrPatchOverlap", c.name, err)
		}
		var pe *PatchError
		if errors.As(err, &pe) && pe.Index != 1 {
			t.Errorf("%s: reported index = %d, want 1", c.name, pe.Index)
		}
	}
	// 相邻不冲突。
	out, err := ApplyPatches(data, []SpanPatch{
		{Start: 0, End: 5, Replacement: []byte("a")},
		{Start: 5, End: 8, Replacement: []byte("b")},
	})
	if err != nil {
		t.Fatalf("adjacent: %v", err)
	}
	if string(out) != "ab89" {
		t.Errorf("adjacent result = %q", out)
	}
}

func TestPatchRangeAndAnchor(t *testing.T) {
	data := []byte(`hello`)
	if _, err := ApplyPatches(data, []SpanPatch{{Start: 3, End: 2}}); !errors.Is(err, ErrPatchRange) {
		t.Errorf("start>end: %v", err)
	}
	if _, err := ApplyPatches(data, []SpanPatch{{Start: 0, End: 99}}); !errors.Is(err, ErrPatchRange) {
		t.Errorf("out of bounds: %v", err)
	}
	// 锚定不匹配：声明期望 "ell" 但实际区间是 "ello"。
	_, err := ApplyPatches(data, []SpanPatch{{Start: 1, End: 5, Replacement: []byte("x"), Expect: []byte("ell")}})
	if !errors.Is(err, ErrPatchAnchor) {
		t.Fatalf("anchor: %v", err)
	}
	// 锚定匹配则通过。
	out, err := ApplyPatches(data, []SpanPatch{{Start: 1, End: 5, Replacement: []byte("ey"), Expect: []byte("ello")}})
	if err != nil || string(out) != "hey" {
		t.Errorf("anchor ok: out=%q err=%v", out, err)
	}
	// 空集合原样返回（同一底层数组）。
	if out, _ := ApplyPatches(data, nil); &out[0] != &data[0] {
		t.Error("empty patch set should return input")
	}
	// 失败不产生部分应用：原始 data 未被修改。
	broken := []SpanPatch{
		{Start: 0, End: 1, Replacement: []byte("H")},
		{Start: 2, End: 99, Replacement: []byte("x")},
	}
	if _, err := ApplyPatches(data, broken); err == nil {
		t.Fatal("expected error")
	}
	if string(data) != "hello" {
		t.Errorf("input mutated: %q", data)
	}
}

func TestEscapeText(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"plain", "plain", false},
		{"a<b&c>d", "a&lt;b&amp;c&gt;d", false},
		{"]]>", "]]&gt;", false},
		{"tab\tnewline\n", "tab\tnewline\n", false},
		{"中文🚀", "中文🚀", false},
		{"bad\x01ctl", "", true},
		{"bad\x00nul", "", true},
	}
	for _, c := range cases {
		got, err := EscapeText(c.in)
		if c.wantErr {
			if !errors.Is(err, ErrEscapeInvalidRune) {
				t.Errorf("EscapeText(%q) err = %v, want ErrEscapeInvalidRune", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("EscapeText(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestEscapeAttrValue(t *testing.T) {
	// 双引号包裹：双引号实体化、单引号保留。
	got, err := EscapeAttrValue(`say "hi" & <go>`, '"')
	if err != nil || got != `say &quot;hi&quot; &amp; &lt;go&gt;` {
		t.Errorf("dq = %q, %v", got, err)
	}
	// 单引号包裹：单引号实体化、双引号保留。
	got, err = EscapeAttrValue(`it's "ok"`, '\'')
	if err != nil || got != `it&apos;s "ok"` {
		t.Errorf("sq = %q, %v", got, err)
	}
	// 空白属性：空白逐字保留、不折叠。
	got, err = EscapeAttrValue("  a\tb\nc  ", '"')
	if err != nil || got != "  a\tb\nc  " {
		t.Errorf("whitespace = %q, %v", got, err)
	}
	// 无引号字符时原样返回。
	got, err = EscapeAttrValue("simple", '"')
	if err != nil || got != "simple" {
		t.Errorf("simple = %q, %v", got, err)
	}
	// 非法引号参数。
	if _, err := EscapeAttrValue("x", '`'); err == nil {
		t.Error("invalid quote should fail")
	}
}

func TestSetAttrValuePatch(t *testing.T) {
	doc := `<p:sld xmlns:p="urn:pm" p:id="256" note='has "dq" &amp; more'/>`
	d := mustIndex(t, doc)
	root := d.Root()

	p, err := SetAttrValuePatch(d, root, "urn:pm", "id", "999")
	if err != nil {
		t.Fatalf("SetAttrValuePatch: %v", err)
	}
	out := mustApply(t, []byte(doc), []SpanPatch{p})
	want := `<p:sld xmlns:p="urn:pm" p:id="999" note='has "dq" &amp; more'/>`
	if string(out) != want {
		t.Errorf("result = %q, want %q", out, want)
	}

	// 单引号属性：引号风格保持，& 解码后重新转义。
	p2, err := SetAttrValuePatch(d, root, "", "note", `a "dq" & <lt>`)
	if err != nil {
		t.Fatalf("note patch: %v", err)
	}
	out2 := mustApply(t, []byte(doc), []SpanPatch{p2})
	want2 := `<p:sld xmlns:p="urn:pm" p:id="256" note='a "dq" &amp; &lt;lt&gt;'/>`
	if string(out2) != want2 {
		t.Errorf("note result = %q, want %q", out2, want2)
	}

	// 属性不存在。
	if _, err := SetAttrValuePatch(d, root, "urn:pm", "missing", "x"); err == nil {
		t.Error("missing attr should fail")
	}

	// 锚定生效：篡改锚定期望后应失败。
	p3, _ := SetAttrValuePatch(d, root, "urn:pm", "id", "1")
	p3.Expect = []byte("wrong")
	if _, err := ApplyPatches([]byte(doc), []SpanPatch{p3}); !errors.Is(err, ErrPatchAnchor) {
		t.Errorf("anchor mismatch: %v", err)
	}
}

func TestInsertSiblingsAndChild(t *testing.T) {
	doc := `<p:spTree xmlns:p="urn:pm"><p:sp></p:sp><p:cxnSp></p:cxnSp></p:spTree>`
	d := mustIndex(t, doc)
	tree := d.Elements("urn:pm", "spTree")[0]
	sp := d.Node(d.Root().Children[0])

	frag := []byte(`<p:pic/>`)
	pAppend, err := AppendChild(d.Node(tree), frag)
	if err != nil {
		t.Fatalf("AppendChild: %v", err)
	}
	pAfter, err := InsertAfter(sp, []byte(`<p:grpSp/>`))
	if err != nil {
		t.Fatalf("InsertAfter: %v", err)
	}
	pBefore, err := InsertBefore(sp, []byte(`<p:note/>`))
	if err != nil {
		t.Fatalf("InsertBefore: %v", err)
	}
	out := mustApply(t, []byte(doc), []SpanPatch{pAppend, pAfter, pBefore})
	want := `<p:spTree xmlns:p="urn:pm"><p:note/><p:sp></p:sp><p:grpSp/><p:cxnSp></p:cxnSp><p:pic/></p:spTree>`
	if string(out) != want {
		t.Errorf("result = %q\nwant   = %q", out, want)
	}
	// 插入结果可再次解析且结构正确。
	d2, err := Index(out)
	if err != nil {
		t.Fatalf("re-index: %v", err)
	}
	tree2 := d2.Elements("urn:pm", "spTree")[0]
	kids := d2.Node(tree2).Children
	if len(kids) != 5 {
		t.Fatalf("children = %d, want 5", len(kids))
	}
	names := []string{"p:note", "p:sp", "p:grpSp", "p:cxnSp", "p:pic"}
	for i, want := range names {
		if got := d2.Node(kids[i]).Name(); got != want {
			t.Errorf("child[%d] = %s, want %s", i, got, want)
		}
	}
}

func TestInsertNamespaceDependency(t *testing.T) {
	doc := `<r:root xmlns:r="urn:r" xmlns:a="urn:a"><r:body></r:body></r:root>`
	d := mustIndex(t, doc)
	root := d.Root()
	body := d.Node(d.Root().Children[0])

	// 1) 片段依赖未声明前缀 → 拒绝。
	if _, err := AppendChild(root, []byte(`<x:thing/>`)); !errors.Is(err, ErrFragmentNamespace) {
		t.Errorf("undeclared prefix: %v", err)
	}
	// 2) 插入点作用域提供前缀 → 通过（a 已在 root 声明）。
	p, err := AppendChild(root, []byte(`<a:note/>`))
	if err != nil {
		t.Fatalf("scoped prefix: %v", err)
	}
	out := mustApply(t, []byte(doc), []SpanPatch{p})
	want := `<r:root xmlns:r="urn:r" xmlns:a="urn:a"><r:body></r:body><a:note/></r:root>`
	if string(out) != want {
		t.Errorf("result = %q, want %q", out, want)
	}
	// 3) 片段自带 xmlns 声明 → 自足通过。
	p2, err := InsertAfter(body, []byte(`<x:thing xmlns:x="urn:x"/>`))
	if err != nil {
		t.Fatalf("self-declared: %v", err)
	}
	out2 := mustApply(t, []byte(doc), []SpanPatch{p2})
	if !strings.Contains(string(out2), `<x:thing xmlns:x="urn:x"/>`) {
		t.Errorf("self-declared result = %q", out2)
	}
	if _, err := Index(out2); err != nil {
		t.Errorf("re-index: %v", err)
	}
	// 4) 属性前缀依赖同样受检。
	if _, err := AppendChild(root, []byte(`<r:x q:flag="1"/>`)); !errors.Is(err, ErrFragmentNamespace) {
		t.Errorf("attr prefix: %v", err)
	}
}

func TestAppendChildSelfClosingRejected(t *testing.T) {
	d := mustIndex(t, `<r:root xmlns:r="urn:r"><r:leaf/></r:root>`)
	leaf := d.Elements("urn:r", "leaf")[0]
	if _, err := AppendChild(d.Node(leaf), []byte(`<r:kid/>`)); !errors.Is(err, ErrInsertPoint) {
		t.Errorf("self-closing append: %v", err)
	}
}

// TestPatchPreservesUntouchedRegions 是 M0 垂直验证的单元级雏形
// （实施计划 §12 第 2 周）：含 mc:AlternateContent 动画分支与未知扩展
// 子树的页面，仅替换一个普通文本 Run 的内容，断言除该文本区间外
// 全部字节逐字不变（真实语料的 B1 哈希比对在此基础上进行）。
func TestPatchPreservesUntouchedRegions(t *testing.T) {
	doc := `<p:sld xmlns:p="urn:pm" xmlns:mc="` + NSMarkupCompat + `" xmlns:ext="urn:ext">` +
		`<p:spTree>` +
		`<mc:AlternateContent><mc:Choice Requires="p14"><p:anim/></mc:Choice><mc:Fallback><p:sp/></mc:Fallback></mc:AlternateContent>` +
		`<p:sp><p:txBody><p:r><p:t>替换我</p:t></p:r></p:txBody></p:sp>` +
		`<ext:extLst><ext:custom keep="1"><ext:raw><![CDATA[<raw>&]]></ext:raw></ext:custom></ext:extLst>` +
		`</p:spTree></p:sld>`
	d := mustIndex(t, doc)

	ts := d.Elements("urn:pm", "t")
	if len(ts) != 1 {
		t.Fatalf("t elements = %v", ts)
	}
	n := d.Node(ts[0])
	start, end := n.OpenEnd, n.CloseStart

	p, err := NewTextPatch(start, end, "新文本 & 更多")
	if err != nil {
		t.Fatalf("NewTextPatch: %v", err)
	}
	out, err := ApplyPatches([]byte(doc), []SpanPatch{p})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}

	// 未触碰区域字节不变：输出 = 原文前缀 + 转义新文本 + 原文后缀。
	esc, _ := EscapeText("新文本 & 更多")
	want := doc[:start] + esc + doc[end:]
	if string(out) != want {
		t.Fatalf("output drift:/n got %q\nwant %q", out, want)
	}
	// 关键保真点：未知子树与动画分支原文（含 CDATA 与实体）逐字保留。
	for _, keep := range []string{
		`<mc:AlternateContent><mc:Choice Requires="p14"><p:anim/></mc:Choice><mc:Fallback><p:sp/></mc:Fallback></mc:AlternateContent>`,
		`<ext:extLst><ext:custom keep="1"><ext:raw><![CDATA[<raw>&]]></ext:raw></ext:custom></ext:extLst>`,
	} {
		if !strings.Contains(string(out), keep) {
			t.Errorf("preserved region missing: %q", keep)
		}
	}
	// 输出仍是 well-formed 且语义元素齐全。
	d2, err := Index(out)
	if err != nil {
		t.Fatalf("re-index: %v", err)
	}
	if got := d2.Elements(NSMarkupCompat, "AlternateContent"); len(got) != 1 {
		t.Errorf("AlternateContent after patch = %v", got)
	}
	if got := d2.Elements("urn:ext", "extLst"); len(got) != 1 {
		t.Errorf("unknown subtree after patch = %v", got)
	}
}
