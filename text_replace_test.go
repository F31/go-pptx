package pptx

import (
	"strings"
	"testing"
)

// ---------- TEXT-02 ReplaceText 测试 ----------

// para0 / paraText 便捷：取唯一段落的唯一 TextFrame。
func tfPara(t *testing.T, bodyInner string) (*Presentation, *Slide, *TextFrame, *Paragraph) {
	t.Helper()
	p := slideWithBody(t, bodyInner)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	paras, err := tf.Paragraphs()
	if err != nil || len(paras) != 1 {
		t.Fatalf("paragraphs = %d, %v", len(paras), err)
	}
	return p, s, tf, paras[0]
}

func paraText(t *testing.T, para *Paragraph) string {
	t.Helper()
	s, err := para.Text()
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	return s
}

func TestReplaceTextSingleRun(t *testing.T) {
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p><a:r><a:t>Hello World</a:t></a:r></a:p>`)
	r, err := para.ReplaceText("World", "Go")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 1 || r.Replaced != 1 || r.Skipped != 0 {
		t.Errorf("result = %+v", r)
	}
	if len(r.Hits) != 1 || r.Hits[0].StartRune != 6 || r.Hits[0].EndRune != 11 || !r.Hits[0].Replaced {
		t.Errorf("hits = %+v", r.Hits)
	}
	if got := paraText(t, para); got != "Hello Go" {
		t.Errorf("text = %q", got)
	}
	xml := slideXML(t, s)
	// 无 rPr 的 Run 原地替换：结构保持不变（无新增 run）。
	if strings.Count(xml, "<a:r>") != 1 || !strings.Contains(xml, "<a:t>Hello Go</a:t>") {
		t.Errorf("xml = %s", xml)
	}
}

func TestReplaceTextCrossRunFirstCharacter(t *testing.T) {
	// 段 0：run0 "Hello "（sz=1800 b=1），run1 "Dolly & 朋友"（b=0）。
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:rPr lang="en-US" sz="1800" b="1"/><a:t>Hello </a:t></a:r>`+
		`<a:r><a:rPr b="0"/><a:t>Dolly &amp; 朋友</a:t></a:r>`+
		`</a:p>`)
	r, err := para.ReplaceText("Hello Dolly", "Hi 羊")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 1 || r.Replaced != 1 {
		t.Errorf("result = %+v", r)
	}
	if got := paraText(t, para); got != "Hi 羊 & 朋友" {
		t.Errorf("text = %q", got)
	}
	xml := slideXML(t, s)
	// replacement 继承首字符 Run（run0）格式：sz=1800 b=1。
	if !strings.Contains(xml, `sz="1800" b="1"`) {
		t.Errorf("replacement lost first-run format: %s", xml)
	}
	// 后缀保留 run1 的格式（b=0）。
	if !strings.Contains(xml, `<a:rPr b="0"/><a:t> &amp; 朋友</a:t>`) {
		t.Errorf("suffix format lost: %s", xml)
	}
	// 未命中内容与格式保留：lang 属性只应出现在 rep run。
	if !strings.Contains(xml, `lang="en-US"`) {
		t.Errorf("rep run rPr incomplete: %s", xml)
	}
}

func TestReplaceTextSuffixInLaterRunDoesNotCorruptPrefix(t *testing.T) {
	// 真实 WPS 样本 ext-0024 的标题形态：英文/中文被拆成多个 Run，
	// 命中只落在最后一个中文 Run 内。曾因 locateBlockSpan 只用
	// gei > lo 判定首 Run，把命中错误锚到 run0，输出 NUL 字节。
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:rPr lang="en-US"/><a:t>89144 SW</a:t></a:r>`+
		`<a:r><a:rPr lang="zh-CN"/><a:t>板</a:t></a:r>`+
		`<a:r><a:rPr lang="en-US"/><a:t>-HPC</a:t></a:r>`+
		`<a:r><a:rPr lang="zh-CN"/><a:t>双上行双</a:t></a:r>`+
		`<a:r><a:rPr lang="en-US"/><a:t>fabric</a:t></a:r>`+
		`<a:r><a:rPr lang="zh-CN"/><a:t>模式拓扑方案</a:t></a:r>`+
		`</a:p>`)
	r, err := para.ReplaceText("拓扑方案", "拓扑验证")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 1 || r.Replaced != 1 || r.Skipped != 0 {
		t.Errorf("result = %+v", r)
	}
	if got := paraText(t, para); got != "89144 SW板-HPC双上行双fabric模式拓扑验证" {
		t.Errorf("text = %q", got)
	}
	xml := slideXML(t, s)
	if strings.ContainsRune(xml, '\x00') {
		t.Fatalf("xml contains NUL bytes: %q", xml)
	}
	if !strings.Contains(xml, `<a:t>89144 SW</a:t>`) {
		t.Errorf("prefix run corrupted: %s", xml)
	}
	if !strings.Contains(xml, `<a:t>模式拓扑验证</a:t>`) {
		t.Errorf("target run not patched in-place: %s", xml)
	}
}

func TestReplaceTextEqualLengthPerRune(t *testing.T) {
	// run0 "ab"（b=1），run1 "cd"（i=1）：跨 Run 等长替换 "bc" → "XY"。
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:rPr b="1"/><a:t>ab</a:t></a:r>`+
		`<a:r><a:rPr i="1"/><a:t>cd</a:t></a:r>`+
		`</a:p>`)
	r, err := para.ReplaceText("bc", "XY", WithReplaceMode(ReplaceEqualLengthPerRune))
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 1 || r.Replaced != 1 {
		t.Errorf("result = %+v", r)
	}
	if got := paraText(t, para); got != "aXYd" {
		t.Errorf("text = %q", got)
	}
	xml := slideXML(t, s)
	// 逐 Run 继承格式：run0 文本 aX 仍带 b=1；run1 文本 Yd 仍带 i=1。
	if !strings.Contains(xml, `<a:rPr b="1"/><a:t>aX</a:t>`) {
		t.Errorf("run0 format broken: %s", xml)
	}
	if !strings.Contains(xml, `<a:rPr i="1"/><a:t>Yd</a:t>`) {
		t.Errorf("run1 format broken: %s", xml)
	}
	// 不拆 Run：仍恰好两个 run。
	if strings.Count(xml, "<a:r>") != 2 {
		t.Errorf("run count changed: %s", xml)
	}
}

func TestReplaceTextExplicitStyle(t *testing.T) {
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:rPr b="1"/><a:t>Hello World</a:t></a:r></a:p>`)
	style := FontStyle{Bold: NewOptional(false), Color: NewOptional(ColorSpec{RGB: "FF0000"})}
	r, err := para.ReplaceText("World", "Go",
		WithReplaceMode(ReplaceExplicitStyle), WithReplacementStyle(style))
	if err != nil {
		t.Fatal(err)
	}
	if r.Replaced != 1 {
		t.Errorf("result = %+v", r)
	}
	if got := paraText(t, para); got != "Hello Go" {
		t.Errorf("text = %q", got)
	}
	xml := slideXML(t, s)
	// 前缀 "Hello " 保留原格式 b=1；替换片段显式 b=0 + 红。
	if !strings.Contains(xml, `<a:rPr b="1"/><a:t>Hello </a:t>`) {
		t.Errorf("prefix format lost: %s", xml)
	}
	if !strings.Contains(xml, `<a:rPr b="0"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:rPr><a:t>Go</a:t>`) {
		t.Errorf("explicit style not applied: %s", xml)
	}
}

func TestReplaceTextExplicitStyleRequiresStyle(t *testing.T) {
	_, _, _, para := tfPara(t, `<a:bodyPr/><a:p><a:r><a:t>x</a:t></a:r></a:p>`)
	if _, err := para.ReplaceText("x", "y", WithReplaceMode(ReplaceExplicitStyle)); err == nil {
		t.Error("expected error when ExplicitStyle without WithReplacementStyle")
	} else if !strings.Contains(err.Error(), "WithReplacementStyle") {
		t.Errorf("err = %v", err)
	}
}

func TestReplaceTextRemoveAll(t *testing.T) {
	// replacement 为空且命中整 run → run 被删除（空 run 不留）。
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:rPr b="1"/><a:t>foo</a:t></a:r>`+
		`<a:r><a:t>keep</a:t></a:r></a:p>`)
	r, err := para.ReplaceText("foo", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Replaced != 1 {
		t.Errorf("result = %+v", r)
	}
	if got := paraText(t, para); got != "keep" {
		t.Errorf("text = %q", got)
	}
	xml := slideXML(t, s)
	if strings.Contains(xml, "foo") || strings.Count(xml, "<a:r>") != 1 {
		t.Errorf("empty run not removed: %s", xml)
	}
}

func TestReplaceTextBoundaries(t *testing.T) {
	// br 边界：文本视图 "abcd"，但 b/c 分属不同块 → 无跨块命中。
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:t>ab</a:t></a:r><a:br/><a:r><a:t>cd</a:t></a:r></a:p>`)
	r, err := para.ReplaceText("bc", "X")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 0 || r.Skipped != 0 {
		t.Errorf("br boundary: result = %+v", r)
	}
	if got := paraText(t, para); got != "abcd" {
		t.Errorf("text changed across br: %q", got)
	}
	// 同一块内的替换不受影响（"ab" 在 br 之前）。
	r2, err := para.ReplaceText("ab", "A")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Replaced != 1 || r2.Skipped != 0 {
		t.Errorf("in-block replace: %+v", r2)
	}
	if got := paraText(t, para); got != "Acd" {
		t.Errorf("text = %q", got)
	}
	_ = s
}

func TestReplaceTextLinkBoundary(t *testing.T) {
	// 链接 run（hlinkClick r:id）与无链接 run 之间不可跨；链接 run 内部
	// 单 run 原地替换允许（链接原样保留）。
	inner := `<a:bodyPr/><a:p>` +
		`<a:r><a:t>ab</a:t></a:r>` +
		`<a:r><a:rPr><a:hlinkClick r:id="rId9"/></a:rPr><a:t>cd</a:t></a:r>` +
		`</a:p>`
	_, s, _, para := tfPara(t, inner)
	// 跨链接边界：无命中（两块）。
	r, err := para.ReplaceText("bc", "X")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 0 {
		t.Errorf("link boundary crossed: %+v", r)
	}
	// 链接 run 内部替换：FirstCharacter 单 run 原地 → 允许且 hlink 保留。
	r2, err := para.ReplaceText("cd", "XY")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Replaced != 1 {
		t.Errorf("in-link replace failed: %+v", r2)
	}
	xml := slideXML(t, s)
	if !strings.Contains(xml, "hlinkClick") || !strings.Contains(xml, "<a:t>XY</a:t>") {
		t.Errorf("link lost on in-run replace: %s", xml)
	}
}

func TestReplaceTextUnsafeRunSkipped(t *testing.T) {
	// 中间 Run 含未知扩展（a:extLst）→ 跨 Run 替换整段重建会丢扩展 → 跳过。
	inner := `<a:bodyPr/><a:p>` +
		`<a:r><a:t>ab</a:t></a:r>` +
		`<a:r><a:extLst><a:ext uri="urn:x"/></a:extLst><a:t>cd</a:t></a:r>` +
		`<a:r><a:t>ef</a:t></a:r></a:p>`
	_, s, _, para := tfPara(t, inner)
	r, err := para.ReplaceText("bcde", "X")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 1 || r.Skipped != 1 || r.Replaced != 0 {
		t.Errorf("unsafe run not skipped: %+v", r)
	}
	if got := paraText(t, para); got != "abcdef" {
		t.Errorf("content changed despite skip: %q", got)
	}
	xml := slideXML(t, s)
	if !strings.Contains(xml, "extLst") {
		t.Errorf("extension lost: %s", xml)
	}
}

func TestReplaceTextGraphemeBoundary(t *testing.T) {
	// "a"+U+0301 组合 + "b"：old="a" 终点切开组合序列 → 跳过。
	// 用 rune 构造组合字符，避免源码转义层级歧义。
	comb := string(rune(0x0301))
	_, _, _, para := tfPara(t, "<a:bodyPr/><a:p><a:r><a:t>a"+comb+"b</a:t></a:r></a:p>")
	r, err := para.ReplaceText("a", "X")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 1 || r.Skipped != 1 || r.Replaced != 0 {
		t.Errorf("grapheme skip: %+v", r)
	}
	// "b" 可安全替换。
	r2, err := para.ReplaceText("b", "Y")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Replaced != 1 {
		t.Errorf("b replace: %+v", r2)
	}
	if got := paraText(t, para); got != "a"+comb+"Y" {
		t.Errorf("text = %q", got)
	}
}

func TestReplaceTextMultiOccurrenceBackToFront(t *testing.T) {
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p><a:r><a:t>x1x2x</a:t></a:r></a:p>`)
	r, err := para.ReplaceText("x", "X")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 3 || r.Replaced != 3 || r.Skipped != 0 {
		t.Errorf("result = %+v", r)
	}
	if got := paraText(t, para); got != "X1X2X" {
		t.Errorf("text = %q", got)
	}
	// Hits 前向有序。
	if len(r.Hits) != 3 ||
		r.Hits[0].StartRune != 0 || r.Hits[1].StartRune != 2 || r.Hits[2].StartRune != 4 {
		t.Errorf("hits order = %+v", r.Hits)
	}
	// 不递归处理替换结果：replacement 含 old 也不再次匹配。
	r2, err := para.ReplaceText("X1", "XX1")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Matches != 1 || r2.Replaced != 1 {
		t.Errorf("no-recursion rule: %+v", r2)
	}
	_ = s
}

func TestReplaceTextValidation(t *testing.T) {
	_, _, _, para := tfPara(t, `<a:bodyPr/><a:p><a:r><a:t>ab</a:t></a:r></a:p>`)
	if _, err := para.ReplaceText("", "x"); err == nil {
		t.Error("empty old accepted")
	}
	if _, err := para.ReplaceText("a", "x\ny"); err == nil {
		t.Error("replacement with newline accepted")
	}
	if _, err := para.ReplaceText("a", "xy", WithReplaceMode(ReplaceEqualLengthPerRune)); err == nil {
		t.Error("unequal length accepted in EqualLengthPerRune")
	}
	// 非法 XML 字符拒绝。
	if _, err := para.ReplaceText("a", "x\x00"); err == nil {
		t.Error("invalid XML char accepted")
	}
}

func TestReplaceTextFirstCharKeepsSuffixFormat(t *testing.T) {
	// 命中跨 run，末 run 后缀格式（i=1）必须保留。
	_, s, _, para := tfPara(t, `<a:bodyPr/><a:p>`+
		`<a:r><a:t>ab</a:t></a:r>`+
		`<a:r><a:rPr i="1"/><a:t>cd</a:t></a:r></a:p>`)
	r, err := para.ReplaceText("abc", "Z")
	if err != nil {
		t.Fatal(err)
	}
	if r.Replaced != 1 {
		t.Errorf("result = %+v", r)
	}
	if got := paraText(t, para); got != "Zd" {
		t.Errorf("text = %q", got)
	}
	xml := slideXML(t, s)
	if !strings.Contains(xml, `<a:rPr i="1"/><a:t>d</a:t>`) {
		t.Errorf("suffix format not preserved: %s", xml)
	}
}

func TestTextFrameReplaceText(t *testing.T) {
	inner := `<a:bodyPr/><a:p><a:r><a:t>one</a:t></a:r></a:p>` +
		`<a:p><a:r><a:t>one two</a:t></a:r></a:p>`
	p := slideWithBody(t, inner)
	s := mustSlide(t, p)
	tf := slideBodyTF(t, s)
	r, err := tf.ReplaceText("one", "1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches != 2 || r.Replaced != 2 || r.Skipped != 0 {
		t.Errorf("result = %+v", r)
	}
	paras, _ := tf.Paragraphs()
	if paraText(t, paras[0]) != "1" || paraText(t, paras[1]) != "1 two" {
		t.Errorf("para texts after replace")
	}
	_ = s
}
