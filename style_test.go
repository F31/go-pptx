package pptx

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// ---------- STYLE-01 EffectiveFont 测试 ----------

// 测试用样式模板 XML：slide→slideLayout→slideMaster→theme 完整链。
//   - layout：body idx=1 占位符 lstStyle lvl1 defRPr b=1 sz=2400
//   - master：body idx=1 占位符 lstStyle lvl1 defRPr i=1 accent3 +mn-lt；
//     body idx=2 占位符 lstStyle lvl1 defRPr i=1 sz=3000 accent2 +mn-lt；
//     txStyles titleStyle(lvl1 dk1 +mj-lt sz4400 b1)、bodyStyle(lvl1 tx1
//     sz1800；lvl3 tx1 sz1200 +mn-lt)、otherStyle(lvl1 sz1800 +mn-lt)
//   - theme：accent3 = srgbClr A5A5A5 + lumMod 60%；accent2=ED7D31；
//     dk1 = sysClr windowText lastClr 000000
// clrMap：tx1→dk1（标准映射）。

const tplStyleLayout = `<p:sldLayout xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:cSld name="Style"><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Body Placeholder 1"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:lstStyle>` +
	`<a:lvl1pPr><a:defRPr b="1" sz="2400"/></a:lvl1pPr>` +
	`</a:lstStyle><a:p/></p:txBody></p:sp>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
	`</p:sldLayout>`

const tplStyleMaster = `<p:sldMaster xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:cSld><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Body Placeholder 1"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:lstStyle>` +
	`<a:lvl1pPr><a:defRPr i="1"><a:solidFill><a:schemeClr val="accent3"/></a:solidFill><a:latin typeface="+mn-lt"/></a:defRPr></a:lvl1pPr>` +
	`</a:lstStyle><a:p/></p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Body Placeholder 2"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="2"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:lstStyle>` +
	`<a:lvl1pPr><a:defRPr i="1" sz="3000"><a:solidFill><a:schemeClr val="accent2"/></a:solidFill><a:latin typeface="+mn-lt"/></a:defRPr></a:lvl1pPr>` +
	`</a:lstStyle><a:p/></p:txBody></p:sp>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
	`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>` +
	`<p:txStyles>` +
	`<p:titleStyle><a:lvl1pPr><a:defRPr sz="4400" b="1"><a:solidFill><a:schemeClr val="dk1"/></a:solidFill><a:latin typeface="+mj-lt"/></a:defRPr></a:lvl1pPr></p:titleStyle>` +
	`<p:bodyStyle>` +
	`<a:lvl1pPr><a:defRPr sz="1800"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/></a:defRPr></a:lvl1pPr>` +
	`<a:lvl3pPr><a:defRPr sz="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/></a:defRPr></a:lvl3pPr>` +
	`</p:bodyStyle>` +
	`<p:otherStyle><a:lvl1pPr><a:defRPr sz="1800"><a:latin typeface="+mn-lt"/></a:defRPr></a:lvl1pPr></p:otherStyle>` +
	`</p:txStyles>` +
	`</p:sldMaster>`

// slideStyleDeckXML 生成含四类形状的页面：title、body idx1（三段：普通、
// run rPr 覆盖、lvl=2）、body idx2、普通文本框（一段普通、一段 phClr）。
const slideStyleDeckXML = `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
	`<p:cSld><p:spTree>` +
	`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr/>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Title Text</a:t></a:r></a:p></p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Body 1"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/>` +
	`<a:p><a:r><a:t>Plain</a:t></a:r><a:r><a:rPr b="0" sz="2000"/><a:t>Override</a:t></a:r></a:p>` +
	`<a:p><a:pPr lvl="2"/><a:r><a:t>Level3</a:t></a:r></a:p>` +
	`</p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="4" name="Body 2"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="2"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Second</a:t></a:r></a:p></p:txBody></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="5" name="Text Box"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
	`<p:spPr/><p:txBody><a:bodyPr/>` +
	`<a:p><a:r><a:t>Box</a:t></a:r></a:p>` +
	`<a:p><a:r><a:rPr><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:rPr><a:t>Pink</a:t></a:r></a:p>` +
	`</p:txBody></p:sp>` +
	`</p:spTree></p:cSld></p:sld>`

// styleFixture 构造完整样式链测试包：theme accent3 附加 lumMod 60%。
func styleFixture(t *testing.T) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	theme := strings.Replace(string(parts["/ppt/theme/theme1.xml"]),
		`<a:accent3><a:srgbClr val="A5A5A5"/></a:accent3>`,
		`<a:accent3><a:srgbClr val="A5A5A5"><a:lumMod val="60000"/></a:srgbClr></a:accent3>`, 1)
	if !strings.Contains(theme, "lumMod") {
		t.Fatal("theme accent3 replacement failed")
	}
	parts["/ppt/theme/theme1.xml"] = []byte(theme)
	parts["/ppt/slideMasters/slideMaster1.xml"] = []byte(xmlDecl + tplStyleMaster)
	parts["/ppt/slideLayouts/slideLayout1.xml"] = []byte(xmlDecl + tplStyleLayout)
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl + slideStyleDeckXML)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
	parts["/[Content_Types].xml"] = bytes.Replace(parts["/[Content_Types].xml"],
		[]byte("</Types>"),
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`), 1)
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`<Relationship Id="rId2" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	return openFixture(t, buildPackageZipPanic(parts))
}

func buildPackageZipPanic(parts map[opc.PartName][]byte) []byte {
	data, err := buildPackageZip(parts)
	if err != nil {
		panic(err)
	}
	return data
}

// tfByPh 返回页面上 (type, idx) 占位符形状的 TextFrame。
func tfByPh(t *testing.T, s *Slide, typ string, idx uint32) *TextFrame {
	t.Helper()
	doc, err := s.p.docOf(s.part)
	if err != nil {
		t.Fatalf("docOf: %v", err)
	}
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for _, sid := range tree.Children {
			sp := doc.Node(sid)
			if sp.Namespace != nsPresentationML || sp.Local() != "sp" {
				continue
			}
			k, ok := phKeyOf(doc, sp)
			if !ok || k.typ != typ || k.idx != idx {
				continue
			}
			tx := childOfKind(doc, sp, nsPresentationML, "txBody", 0)
			if tx == nil {
				t.Fatalf("placeholder %s/%d has no txBody", typ, idx)
			}
			return &TextFrame{textNode: textNode{p: s.p, part: s.part, path: recordPath(doc, tx.ID)}}
		}
	}
	t.Fatalf("placeholder %s/%d not found", typ, idx)
	return nil
}

// tfNthPlain 返回页面上第 nth 个（0 基）非占位符形状的 TextFrame。
func tfNthPlain(t *testing.T, s *Slide, nth int) *TextFrame {
	t.Helper()
	doc, err := s.p.docOf(s.part)
	if err != nil {
		t.Fatalf("docOf: %v", err)
	}
	seen := 0
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for _, sid := range tree.Children {
			sp := doc.Node(sid)
			if sp.Namespace != nsPresentationML || sp.Local() != "sp" {
				continue
			}
			if _, ok := phKeyOf(doc, sp); ok {
				continue
			}
			if seen != nth {
				seen++
				continue
			}
			tx := childOfKind(doc, sp, nsPresentationML, "txBody", 0)
			if tx == nil {
				t.Fatal("plain shape has no txBody")
			}
			return &TextFrame{textNode: textNode{p: s.p, part: s.part, path: recordPath(doc, tx.ID)}}
		}
	}
	t.Fatalf("plain shape #%d not found", nth)
	return nil
}

func runAt(t *testing.T, tf *TextFrame, pi, ri int) *TextRun {
	t.Helper()
	paras, err := tf.Paragraphs()
	if err != nil || len(paras) <= pi {
		t.Fatalf("paragraphs(%d) = %d, %v", pi, len(paras), err)
	}
	runs, err := paras[pi].Runs()
	if err != nil || len(runs) <= ri {
		t.Fatalf("runs(%d,%d) = %d, %v", pi, ri, len(runs), err)
	}
	return runs[ri]
}

func hasDiagCode(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestEffectiveFontTitleStyleChain(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	run := runAt(t, tfByPh(t, s, "title", 0), 0, 0)

	rf, diags, err := run.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !rf.Bold.Resolved || !rf.Bold.Value {
		t.Errorf("bold = %+v", rf.Bold)
	}
	if !rf.Size.Resolved || rf.Size.Value != 44 {
		t.Errorf("size = %+v", rf.Size)
	}
	if !rf.Color.Resolved || rf.Color.RGB != "000000" {
		t.Errorf("color = %+v", rf.Color)
	}
	if rf.Color.Spec.Scheme != "dk1" {
		t.Errorf("color spec = %+v", rf.Color.Spec)
	}
	if !rf.Latin.Resolved || rf.Latin.Value != "Calibri Light" {
		t.Errorf("latin = %+v", rf.Latin)
	}
	// trace：标题样式来自母版 txStyles（layout/master 均无 title 占位符）。
	if len(rf.Size.Trace) == 0 || !strings.Contains(rf.Size.Trace[0].Detail, "titleStyle") {
		t.Errorf("size trace = %+v", rf.Size.Trace)
	}
	if rf.Size.Trace[0].Part != "/ppt/slideMasters/slideMaster1.xml" {
		t.Errorf("size trace part = %q", rf.Size.Trace[0].Part)
	}
	// 颜色 trace 含主题展开（sysClr lastClr 000000）。
	if len(rf.Color.Trace) < 2 || rf.Color.Trace[len(rf.Color.Trace)-1].Source != SourceTheme {
		t.Errorf("color trace = %+v", rf.Color.Trace)
	}
	_ = diags
}

func TestEffectiveFontLayoutThenMasterChain(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	// body idx=1 段 0 run0（无本地 rPr）：layout 提供 b/sz，master 提供
	// i/color/latin。
	run := runAt(t, tfByPh(t, s, "body", 1), 0, 0)
	rf, diags, err := run.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	// 模板主题 minorFont 只定义 latin（ea/cs typeface 为空）：ea/cs 如实
	// 未决并给出 STYLE_UNRESOLVED；核心五属性不得有诊断。
	for _, d := range diags {
		if !strings.Contains(d.Message, "ea") && !strings.Contains(d.Message, "cs") {
			t.Errorf("unexpected diag: %+v", d)
		}
	}
	if !rf.EastAsian.Resolved || !rf.ComplexScript.Resolved {
		t.Logf("ea/cs unresolved (theme ea/cs empty): %+v / %+v", rf.EastAsian, rf.ComplexScript)
	}
	if !rf.Bold.Resolved || !rf.Bold.Value {
		t.Errorf("bold = %+v", rf.Bold)
	}
	if !rf.Size.Resolved || rf.Size.Value != 24 {
		t.Errorf("size = %+v", rf.Size)
	}
	if !rf.Italic.Resolved || !rf.Italic.Value {
		t.Errorf("italic = %+v", rf.Italic)
	}
	// accent3 #A5A5A5 × lumMod 60% → #636363。
	if !rf.Color.Resolved || rf.Color.RGB != "636363" {
		t.Errorf("color = %+v", rf.Color)
	}
	if !rf.Latin.Resolved || rf.Latin.Value != "Calibri" {
		t.Errorf("latin = %+v", rf.Latin)
	}
	// size trace 首步来自 layout 占位符 lstStyle。
	if rf.Size.Trace[0].Part != "/ppt/slideLayouts/slideLayout1.xml" ||
		!strings.Contains(rf.Size.Trace[0].Detail, "layout lstStyle") {
		t.Errorf("size trace = %+v", rf.Size.Trace)
	}
	// color trace 首步来自 master 占位符 lstStyle，末步为主题展开。
	if rf.Color.Trace[0].Part != "/ppt/slideMasters/slideMaster1.xml" {
		t.Errorf("color trace = %+v", rf.Color.Trace)
	}
	if last := rf.Color.Trace[len(rf.Color.Trace)-1]; last.Source != SourceTheme ||
		!strings.Contains(last.Detail, "accent3") {
		t.Errorf("color theme trace = %+v", rf.Color.Trace)
	}
	// latin 的 "+mn-lt" 展开为主题 minorFont。
	if last := rf.Latin.Trace[len(rf.Latin.Trace)-1]; last.Source != SourceTheme ||
		!strings.Contains(last.Detail, "minorFont") {
		t.Errorf("latin trace = %+v", rf.Latin.Trace)
	}
}

func TestEffectiveFontRunOverrides(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	// body idx=1 段 0 run1：本地 rPr b=0 sz=2000 覆盖；i/color/latin 继承。
	run := runAt(t, tfByPh(t, s, "body", 1), 0, 1)
	rf, _, err := run.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !rf.Bold.Resolved || rf.Bold.Value {
		t.Errorf("bold = %+v (want explicit false)", rf.Bold)
	}
	if !rf.Size.Resolved || rf.Size.Value != 20 {
		t.Errorf("size = %+v", rf.Size)
	}
	if !rf.Italic.Resolved || !rf.Italic.Value {
		t.Errorf("italic = %+v", rf.Italic)
	}
	if !rf.Color.Resolved || rf.Color.RGB != "636363" {
		t.Errorf("color = %+v", rf.Color)
	}
	if rf.Bold.Trace[0].Source != SourceRun || rf.Bold.Trace[0].Detail != "run rPr" {
		t.Errorf("bold trace = %+v", rf.Bold.Trace)
	}
}

func TestEffectiveFontLevel3AndClrMap(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	// body idx=1 段 1（pPr lvl=2 → lvl3pPr）：layout/master 占位符均无
	// lvl3 → 母版 bodyStyle lvl3 defRPr sz=1200 color tx1（经 clrMap→dk1）。
	run := runAt(t, tfByPh(t, s, "body", 1), 1, 0)
	rf, diags, err := run.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !rf.Size.Resolved || rf.Size.Value != 12 {
		t.Errorf("size = %+v", rf.Size)
	}
	if !rf.Color.Resolved || rf.Color.RGB != "000000" {
		t.Errorf("color = %+v", rf.Color)
	}
	// color trace 含 clrMap tx1→dk1 与主题展开两 Theme 步。
	var sawClrMap, sawScheme bool
	for _, st := range rf.Color.Trace {
		if strings.Contains(st.Detail, "clrMap tx1") {
			sawClrMap = true
		}
		if strings.Contains(st.Detail, "clrScheme dk1") {
			sawScheme = true
		}
	}
	if !sawClrMap || !sawScheme {
		t.Errorf("color trace = %+v", rf.Color.Trace)
	}
	if !rf.Latin.Resolved || rf.Latin.Value != "Calibri" {
		t.Errorf("latin = %+v", rf.Latin)
	}
	// lvl3 defRPr 无 b/i：bold/italic 未决（诊断 STYLE_UNRESOLVED）。
	if rf.Bold.Resolved || rf.Italic.Resolved {
		t.Errorf("bold/italic should be unresolved: %+v / %+v", rf.Bold, rf.Italic)
	}
	if !hasDiagCode(diags, "STYLE_UNRESOLVED") {
		t.Errorf("expected STYLE_UNRESOLVED diag, got %+v", diags)
	}
}

func TestEffectiveFontMasterOnlyPlaceholder(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	// body idx=2：layout 无该占位符 → master idx2 lstStyle（sz30 accent2）。
	run := runAt(t, tfByPh(t, s, "body", 2), 0, 0)
	rf, _, err := run.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !rf.Size.Resolved || rf.Size.Value != 30 {
		t.Errorf("size = %+v", rf.Size)
	}
	if !rf.Italic.Resolved || !rf.Italic.Value {
		t.Errorf("italic = %+v", rf.Italic)
	}
	if !rf.Color.Resolved || rf.Color.RGB != "ED7D31" {
		t.Errorf("color = %+v", rf.Color)
	}
	// master idx2 defRPr 无 b：bold 未决。
	if rf.Bold.Resolved {
		t.Errorf("bold should be unresolved: %+v", rf.Bold)
	}
}

func TestEffectiveFontOtherStyleAndFallback(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	box := tfNthPlain(t, s, 0)
	run := runAt(t, box, 0, 0)

	// 无回退：color 未决；size 18（otherStyle）；latin minorFont。
	rf, diags, err := run.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	if rf.Color.Resolved {
		t.Errorf("color should be unresolved: %+v", rf.Color)
	}
	if !rf.Size.Resolved || rf.Size.Value != 18 {
		t.Errorf("size = %+v", rf.Size)
	}
	if !rf.Latin.Resolved || rf.Latin.Value != "Calibri" {
		t.Errorf("latin = %+v", rf.Latin)
	}
	if !hasDiagCode(diags, "STYLE_UNRESOLVED") {
		t.Errorf("expected STYLE_UNRESOLVED diag, got %+v", diags)
	}

	// 显式回退：color/bold 采用并标记 Fallback。
	ctx := ResolveContext{Fallback: FontStyle{
		Color: NewOptional(ColorSpec{RGB: "FF0000"}),
		Bold:  NewOptional(true),
	}}
	rf2, _, err := run.EffectiveFont(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rf2.Color.Resolved || !rf2.Color.Fallback || rf2.Color.RGB != "FF0000" {
		t.Errorf("color fallback = %+v", rf2.Color)
	}
	if !rf2.Bold.Resolved || !rf2.Bold.Fallback || !rf2.Bold.Value {
		t.Errorf("bold fallback = %+v", rf2.Bold)
	}
	if rf2.Size.Fallback {
		t.Errorf("size must not be fallback: %+v", rf2.Size)
	}
	if rf2.Color.Trace[0].Source != SourceFallback {
		t.Errorf("color trace = %+v", rf2.Color.Trace)
	}
}

func TestEffectiveFontStrictAndPhClr(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	box := tfNthPlain(t, s, 0)

	// phClr 引用：STYLE_PARTIAL、Spec 保留 phClr、Resolved=false。
	runP := runAt(t, box, 1, 0)
	rfp, diagsP, err := runP.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	if rfp.Color.Resolved || rfp.Color.Spec.Scheme != "phClr" {
		t.Errorf("phClr color = %+v", rfp.Color)
	}
	if !hasDiagCode(diagsP, "STYLE_PARTIAL") {
		t.Errorf("expected STYLE_PARTIAL diag, got %+v", diagsP)
	}

	// 普通文本框段落（无回退）+ Strict → ErrUnresolvedStyle。
	run0 := runAt(t, box, 0, 0)
	if _, _, err := run0.EffectiveFont(ResolveContext{Strict: true}); !errors.Is(err, ErrUnresolvedStyle) {
		t.Errorf("strict err = %v", err)
	}
	// 同 run 非 strict：无 error。
	if _, _, err := run0.EffectiveFont(ResolveContext{}); err != nil {
		t.Errorf("unexpected err = %v", err)
	}
}

func TestEffectiveFontClosedDoc(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	run := runAt(t, tfByPh(t, s, "body", 1), 0, 0)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run.EffectiveFont(ResolveContext{}); !errors.Is(err, ErrClosed) {
		t.Errorf("closed err = %v", err)
	}
}

func TestEffectiveFontNotesChain(t *testing.T) {
	p := styleFixture(t)
	s := mustSlide(t, p)
	if err := s.SetSpeakerNotes("Hello notes"); err != nil {
		t.Fatal(err)
	}
	// 修复回归：创建后同会话可再读到 notes Part。
	txt, err := s.SpeakerNotesText()
	if err != nil || txt != "Hello notes" {
		t.Fatalf("SpeakerNotesText = %q, %v", txt, err)
	}
	notes, err := s.SpeakerNotes()
	if err != nil {
		t.Fatal(err)
	}
	run := runAt(t, notes, 0, 0)
	rf, _, err := run.EffectiveFont(ResolveContext{})
	if err != nil {
		t.Fatal(err)
	}
	// notesMaster 无占位符/notesStyle：字体落主题 minorFont 缺省；
	// color/size/bold 未决。
	if !rf.Latin.Resolved || rf.Latin.Value != "Calibri" {
		t.Errorf("latin = %+v", rf.Latin)
	}
	if rf.Color.Resolved || rf.Size.Resolved {
		t.Errorf("color/size should be unresolved: %+v / %+v", rf.Color, rf.Size)
	}
}
