package pptx

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// docPropsParts returns minimal template parts, optionally stripped of
// core.xml / app.xml / custom.xml, their root-rels entries and their
// [Content_Types].xml Override entries, so that on-demand creation paths
// can be exercised (a part absent from the package must also have no
// lingering CT Override, else the save plan's re-generated CT would clash).
func docPropsParts(t *testing.T, dropCore, dropApp, dropCustom bool) map[opc.PartName][]byte {
	parts := minimalTemplateParts()
	dropEntry := func(part, relType, target string) {
		// 1) root rels entry.
		if _, exists := parts["/_rels/.rels"]; exists {
			s := string(parts["/_rels/.rels"])
			needle := `Type="` + relType + `" Target="` + target + `"`
			if i := strings.Index(s, needle); i >= 0 {
				start := strings.LastIndex(s[:i], "<Relationship")
				end := strings.Index(s[i:], "/>") + i + 2
				parts["/_rels/.rels"] = []byte(s[:start] + s[end:])
			}
		}
		// 2) Content-Types Override for the part.
		if ct, exists := parts["/[Content_Types].xml"]; exists {
			s := string(ct)
			needle := `<Override PartName="` + part + `"`
			if i := strings.Index(s, needle); i >= 0 {
				end := strings.Index(s[i:], "/>") + i + 2
				parts["/[Content_Types].xml"] = []byte(s[:i] + s[end:])
			}
		}
	}
	if dropCore {
		delete(parts, "/docProps/core.xml")
		dropEntry("/docProps/core.xml", relCoreProps, "docProps/core.xml")
	}
	if dropApp {
		delete(parts, "/docProps/app.xml")
		dropEntry("/docProps/app.xml", relExtProps, "docProps/app.xml")
	}
	if dropCustom {
		delete(parts, "/docProps/custom.xml")
		dropEntry("/docProps/custom.xml", relCustomProps, "docProps/custom.xml")
	}
	return parts
}

// openDocProps builds a package from parts and opens it.
func openDocProps(t *testing.T, parts map[opc.PartName][]byte) *Presentation {
	t.Helper()
	data, err := buildPackageZip(parts)
	if err != nil {
		t.Fatalf("buildPackageZip: %v", err)
	}
	p, err := OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	return p
}

func TestCorePropertiesReadDefaults(t *testing.T) {
	// New() 模板 core.xml 仅含 dc:creator=go-pptx。
	p := openDocProps(t, minimalTemplateParts())
	defer p.Close()
	cp, err := p.CoreProperties()
	if err != nil {
		t.Fatal(err)
	}
	if !cp.Author.Set || cp.Author.Value != "go-pptx" {
		t.Errorf("author = %+v, want go-pptx", cp.Author)
	}
	if cp.Title.Set || cp.Created.Set || cp.Modified.Set || cp.Company.Set {
		t.Errorf("unexpected defaults set: %+v", cp)
	}
}

func TestCorePropertiesMissingReturnsZero(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, true, false, false))
	defer p.Close()
	cp, err := p.CoreProperties()
	if err != nil {
		t.Fatal(err)
	}
	if cp.anySet() {
		t.Errorf("missing core.xml should read zero value, got %+v", cp)
	}
}

func TestCorePropertiesSetPatchAndReadBack(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, true, false, false)) // 无 core.xml：按需创建
	defer p.Close()

	when := time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC)
	if err := p.SetCoreProperties(CorePropertiesPatch{
		Title:    NewOptional("季度报告"),
		Subject:  NewOptional("Q3 2026"),
		Author:   NewOptional("jinfeng105"),
		Company:  NewOptional("F31 Inc"),
		Keywords: NewOptional("a&b <c>"),
		Category: NewOptional("内部"),
		Comments: NewOptional("评审前定稿"),
		Created:  NewOptional(when),
		Modified: NewOptional(when), // 显式传入 → 保存不自动刷新
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	p2 := openDocPropsReader(t, buf.Bytes())
	defer p2.Close()
	cp, err := p2.CoreProperties()
	if err != nil {
		t.Fatal(err)
	}
	if !cp.Title.Set || cp.Title.Value != "季度报告" {
		t.Errorf("title = %+v", cp.Title)
	}
	if !cp.Subject.Set || cp.Subject.Value != "Q3 2026" {
		t.Errorf("subject = %+v", cp.Subject)
	}
	if !cp.Author.Set || cp.Author.Value != "jinfeng105" {
		t.Errorf("author = %+v", cp.Author)
	}
	if !cp.Company.Set || cp.Company.Value != "F31 Inc" {
		t.Errorf("company = %+v", cp.Company)
	}
	if !cp.Keywords.Set || cp.Keywords.Value != "a&b <c>" {
		t.Errorf("keywords = %+v", cp.Keywords)
	}
	if !cp.Category.Set || cp.Category.Value != "内部" {
		t.Errorf("category = %+v", cp.Category)
	}
	if !cp.Comments.Set || cp.Comments.Value != "评审前定稿" {
		t.Errorf("comments = %+v", cp.Comments)
	}
	if !cp.Created.Set || !cp.Created.Value.Equal(when) {
		t.Errorf("created = %+v, want %v", cp.Created, when)
	}
	if !cp.Modified.Set || !cp.Modified.Value.Equal(when) {
		t.Errorf("modified = %+v, want %v (explicit must survive save)", cp.Modified, when)
	}
	// 新 Part + 关系 + CT 齐全。
	if !p2.pk.HasPart("/docProps/core.xml") {
		t.Error("core.xml part missing after save")
	}
	if _, ok := p2.pk.ContentType("/docProps/core.xml"); !ok {
		t.Error("core.xml content type missing")
	}
	if rel, ok := p2.rootRelTargetPublic(relCoreProps); !ok || rel != "/docProps/core.xml" {
		t.Errorf("core rel missing after save: %q %v", rel, ok)
	}
}

func TestCorePropertiesCreatesCoreAndAppRootRelsTogether(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, true, true, false))
	defer p.Close()

	if err := p.SetCoreProperties(CorePropertiesPatch{
		Title:   NewOptional("Report"),
		Company: NewOptional("F31"),
	}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	p2 := openDocPropsReader(t, buf.Bytes())
	defer p2.Close()
	if rel, ok := p2.rootRelTargetPublic(relCoreProps); !ok || rel != "/docProps/core.xml" {
		t.Fatalf("core rel = %q %v, want /docProps/core.xml true", rel, ok)
	}
	if rel, ok := p2.rootRelTargetPublic(relExtProps); !ok || rel != "/docProps/app.xml" {
		t.Fatalf("app rel = %q %v, want /docProps/app.xml true", rel, ok)
	}
	cp, err := p2.CoreProperties()
	if err != nil {
		t.Fatal(err)
	}
	if cp.Title.Value != "Report" || cp.Company.Value != "F31" {
		t.Fatalf("core props = %+v, want title and company", cp)
	}
}

// rootRelTargetPublic 暴露内部辅助供测试断言。
func (p *Presentation) rootRelTargetPublic(relType string) (opc.PartName, bool) {
	name, ok, _ := p.rootRelTarget(relType)
	return name, ok
}

// openDocPropsReader opens an in-memory pptx.
func openDocPropsReader(t *testing.T, data []byte) *Presentation {
	t.Helper()
	p, err := OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	return p
}

func TestCorePropertiesPatchDoesNotClearUnmentioned(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, true, false, false))
	defer p.Close()
	if err := p.SetCoreProperties(CorePropertiesPatch{
		Title: NewOptional("A"), Author: NewOptional("me"),
	}); err != nil {
		t.Fatal(err)
	}
	// 第二次只改 Title：Author 必须保留。
	if err := p.SetCoreProperties(CorePropertiesPatch{Title: NewOptional("B")}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	cp, err := openDocPropsReader(t, buf.Bytes()).CoreProperties()
	if err != nil {
		t.Fatal(err)
	}
	if cp.Title.Value != "B" || !cp.Author.Set || cp.Author.Value != "me" {
		t.Errorf("patch cleared unmentioned: %+v", cp)
	}
}

func TestCorePropertiesModifiedAutoUpdatedOnSave(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, true, false, false))
	defer p.Close()
	// 未显式 Modified → 保存时自动刷新为当前时间。
	if err := p.SetCoreProperties(CorePropertiesPatch{Title: NewOptional("X")}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	cp, err := openDocPropsReader(t, buf.Bytes()).CoreProperties()
	if err != nil {
		t.Fatal(err)
	}
	if !cp.Modified.Set {
		t.Fatal("modified not auto-set on save")
	}
	if d := time.Since(cp.Modified.Value); d > 5*time.Minute || d < -5*time.Minute {
		t.Errorf("modified = %v, not near now", cp.Modified.Value)
	}
}

func TestCustomPropertiesRoundTrip(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, false, false, true)) // 无 custom.xml
	defer p.Close()

	when := time.Date(2026, 9, 8, 3, 30, 0, 0, time.UTC)
	sets := []struct {
		name string
		val  CustomPropertyValue
	}{
		{"editor", StringCustomProperty("张三")},
		{"pages", IntegerCustomProperty(42)},
		{"done", BooleanCustomProperty(true)},
		{"due", DateTimeCustomProperty(when)},
	}
	for _, s := range sets {
		if err := p.SetCustomProperty(s.name, s.val); err != nil {
			t.Fatalf("SetCustomProperty(%s): %v", s.name, err)
		}
	}
	// 同名覆盖（保留 pid）。
	if err := p.SetCustomProperty("pages", IntegerCustomProperty(99)); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	p2 := openDocPropsReader(t, buf.Bytes())
	defer p2.Close()
	m, err := p2.CustomProperties()
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 4 {
		t.Fatalf("got %d props, want 4: %v", len(m), m)
	}
	if v := m["editor"]; v.Kind != CustomPropertyString || v.Str != "张三" {
		t.Errorf("editor = %+v", v)
	}
	if v := m["pages"]; v.Kind != CustomPropertyInteger || v.Int != 99 {
		t.Errorf("pages = %+v", v)
	}
	if v := m["done"]; v.Kind != CustomPropertyBoolean || !v.Bool {
		t.Errorf("done = %+v", v)
	}
	if v := m["due"]; v.Kind != CustomPropertyDateTime || !v.Time.Equal(when) {
		t.Errorf("due = %+v", v)
	}
	if !p2.pk.HasPart("/docProps/custom.xml") {
		t.Error("custom.xml missing after save")
	}
	if _, ok := p2.pk.ContentType("/docProps/custom.xml"); !ok {
		t.Error("custom.xml content type missing")
	}
	if rel, ok := p2.rootRelTargetPublic(relCustomProps); !ok || rel != "/docProps/custom.xml" {
		t.Errorf("custom rel missing after save: %q %v", rel, ok)
	}
}

func TestCustomPropertiesMissingReturnsEmpty(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, false, false, true))
	defer p.Close()
	m, err := p.CustomProperties()
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty, got %v", m)
	}
}

func TestCustomPropertiesInvalidArgs(t *testing.T) {
	p := openDocProps(t, docPropsParts(t, false, false, true))
	defer p.Close()
	if err := p.SetCustomProperty("  ", StringCustomProperty("x")); err == nil {
		t.Error("empty name must fail")
	}
	if err := p.SetCustomProperty("big", IntegerCustomProperty(1<<40)); err == nil {
		t.Error("i4 overflow must fail")
	}
}

// TestCustomPropertiesUnsupportedVariant 预置含 vt:variant 的 custom.xml，
// 读取必须报 ErrUnsupportedFormat。
func TestCustomPropertiesUnsupportedVariant(t *testing.T) {
	parts := minimalTemplateParts()
	parts["/docProps/custom.xml"] = []byte(xmlDecl +
		`<Properties xmlns="` + nsCustomProps + `" xmlns:vt="` + nsVTypes + `">` +
		`<property fmtid="{D5CDD505-2E9C-101B-9397-08002B2CF9AE}" pid="2" name="bad">` +
		`<vt:variant><vt:lpwstr>x</vt:lpwstr></vt:variant></property></Properties>`)
	// 根 rels 与 CT 由模板缺省补充：模板已有 custom rel/CT 吗？模板没有
	// custom.xml 关系，手动补。
	rootRels := string(parts["/_rels/.rels"])
	rid := nextRID([]byte(rootRels))
	rootRels = string(insertRel([]byte(rootRels),
		`<Relationship Id="`+rid+`" Type="`+relCustomProps+`" Target="docProps/custom.xml"/>`))
	parts["/_rels/.rels"] = []byte(rootRels)

	p := openDocProps(t, parts)
	defer p.Close()
	if _, err := p.CustomProperties(); err == nil || !strings.Contains(err.Error(), "unsupported variant") {
		t.Errorf("want ErrUnsupportedFormat-ish error, got %v", err)
	}
}

func TestDocPropsClosedErrors(t *testing.T) {
	p := openDocProps(t, minimalTemplateParts())
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CoreProperties(); err == nil {
		t.Error("CoreProperties on closed doc must fail")
	}
	if err := p.SetCoreProperties(CorePropertiesPatch{Title: NewOptional("x")}); err == nil {
		t.Error("SetCoreProperties on closed doc must fail")
	}
	if _, err := p.CustomProperties(); err == nil {
		t.Error("CustomProperties on closed doc must fail")
	}
	if err := p.SetCustomProperty("a", StringCustomProperty("b")); err == nil {
		t.Error("SetCustomProperty on closed doc must fail")
	}
}

func TestCorePropertiesModifiedSemanticsPerCall(t *testing.T) {
	// 语义：每次 SetCoreProperties 独立判断——显式携带 Modified 的调用
	// 写入固定值；后续未携带 Modified 的调用恢复"库代管"，保存时刷新。
	p := openDocProps(t, docPropsParts(t, true, false, false))
	defer p.Close()
	fixed := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := p.SetCoreProperties(CorePropertiesPatch{Modified: NewOptional(fixed)}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetCoreProperties(CorePropertiesPatch{Title: NewOptional("t")}); err != nil {
		t.Fatal(err) // 未带 Modified → 恢复库代管
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	cp, err := openDocPropsReader(t, buf.Bytes()).CoreProperties()
	if err != nil {
		t.Fatal(err)
	}
	if !cp.Modified.Set || cp.Modified.Value.Equal(fixed) {
		t.Errorf("modified should be refreshed by library (not fixed %v): %+v", fixed, cp.Modified)
	}
	if d := time.Since(cp.Modified.Value); d > 5*time.Minute || d < -5*time.Minute {
		t.Errorf("modified = %v, not near now", cp.Modified.Value)
	}
}

// TestLeafTextPatch 直接驱动 leafTextPatch 的三条路径：普通元素的文本
// 区间替换（含特殊字符转义）、自闭合元素的整元素重建（保留原始属性）、
// 非法 XML 1.0 字符拒绝。补丁语义经 ApplyPatches 回放验证。
func TestLeafTextPatch(t *testing.T) {
	base := []byte(`<cp:coreProperties xmlns:cp="` + nsCoreProps + `" xmlns:dc="` + nsDC + `">` +
		`<dc:title>old</dc:title><dc:subject/><dc:keywords xml:lang="zh"/></cp:coreProperties>`)
	doc, err := xmlstore.Index(base)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	root := doc.Root()
	title := childByNSLocal(doc, root, nsDC, "title")
	subject := childByNSLocal(doc, root, nsDC, "subject")
	keywords := childByNSLocal(doc, root, nsDC, "keywords")
	if title == nil || subject == nil || keywords == nil {
		t.Fatalf("nodes not found: title=%v subject=%v keywords=%v", title, subject, keywords)
	}

	// 1) 普通元素：替换 [OpenEnd, CloseStart) 文本区间 + 实体转义。
	p1, err := leafTextPatch(doc, title, "new <&> 值")
	if err != nil {
		t.Fatalf("leafTextPatch(title): %v", err)
	}
	if p1.Start != title.OpenEnd || p1.End != title.CloseStart {
		t.Errorf("span = [%d,%d), want [%d,%d)", p1.Start, p1.End, title.OpenEnd, title.CloseStart)
	}
	if string(p1.Replacement) != "new &lt;&amp;&gt; 值" {
		t.Errorf("replacement = %q, want escaped text", p1.Replacement)
	}

	// 2) 自闭合无属性：重建开标签（"/>" → ">"）+ 文本 + 闭标签。
	p2, err := leafTextPatch(doc, subject, "s")
	if err != nil {
		t.Fatalf("leafTextPatch(subject): %v", err)
	}
	if p2.Start != subject.Source.Start || p2.End != subject.Source.End {
		t.Errorf("self-closing span = [%d,%d), want whole element [%d,%d)",
			p2.Start, p2.End, subject.Source.Start, subject.Source.End)
	}

	// 3) 自闭合带属性：保留原始属性文本（xml:lang）。
	p3, err := leafTextPatch(doc, keywords, "k")
	if err != nil {
		t.Fatalf("leafTextPatch(keywords): %v", err)
	}

	out, err := xmlstore.ApplyPatches(base, []xmlstore.SpanPatch{p1, p2, p3})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		`<dc:title>new &lt;&amp;&gt; 值</dc:title>`,
		`<dc:subject>s</dc:subject>`,
		`<dc:keywords xml:lang="zh">k</dc:keywords>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}

	// 4) XML 1.0 禁止字符：拒绝而非静默转码（方案 §19.3）。
	_, err = leafTextPatch(doc, title, "bad\x00char")
	if err == nil {
		t.Fatal("invalid control char must be rejected")
	}
	var oe *OperationError
	if !errors.As(err, &oe) || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("err = %v, want OperationError wrapping ErrInvalidArgument", err)
	}
}
