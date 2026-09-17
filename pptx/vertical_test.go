// M0 垂直验证（实施计划 §123「M0 垂直验证程序」）。
//
// 垂直验证程序定义：对**含动画与未知扩展**的页面仅修改一个普通文本
// Run → 保存 → 逐 Part 哈希比对（B1）+ 同节点未知区字节不变断言。
//
// 与既有 B1 测试的分工：
//   - internal/opc/saveplan_test.go 的 TestSavePlanUnchangedIsB1 验证
//     **空变更集**（无任何编辑）下保存后全部 Part 字节一致——纯 OPC 层，
//     不经过公共 API 的编辑与事务路径。
//   - 本文件验证**非空变更集**：确实改了一个 Run 的文本，此时必须做到
//     ① 其它 Part 仍字节一致（B1）；
//     ② 被改的那个 Part 内部，未被触碰的区域（p:timing 动画子树、
//     p:extLst 未知扩展）字节不变——这是"保真补丁"相对"重新序列化
//     整棵树"的核心优势，也是 ANIM-02 / TIMIR-01 / OPC-02 三方交叠
//     处最容易回归的地方。
//
// 语料说明：testdata/corpus/ 目前无真实样本（QA-01 长期阻塞项），
// 本文件用**合成语料**覆盖垂直验证的断言结构。真实语料回填后应保留
// 本测试的断言骨架，只替换 fixture 来源（见 testdata/corpus/README.md）。
package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"sort"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// ---------- 合成语料：含动画 + 未知扩展的页面 ----------

// verticalUnknownExt 是页面 p:extLst 内的未知扩展子树——使用本库完全
// 不认识的命名空间与元素名，任何"重新序列化整棵树"的实现都会破坏它。
const verticalUnknownExt = `<p:extLst>` +
	`<p:ext uri="{A1B2C3D4-E5F6-4A7B-8C9D-0E1F2A3B4C5D}">` +
	`<vendor:payload xmlns:vendor="http://example.invalid/vendor/1.0"` +
	` version="3" checksum="DEADBEEF">` +
	`<vendor:item id="1"><vendor:raw>原始字节 &amp; 实体</vendor:raw></vendor:item>` +
	`<vendor:item id="2"><vendor:raw>嵌套 <vendor:deep x="1"/></vendor:raw></vendor:item>` +
	`</vendor:payload>` +
	`</p:ext>` +
	`</p:extLst>`

// verticalTiming 是一段真实形态的 PowerPoint"点击淡入"动画树
// （tmRoot → par → cTn → clickPar → clickEffect → set + animEffect），
// 同时含有自闭合 cond——即 TIMIR-01 之前会解析错位的形态。
const verticalTiming = `<p:timing>` +
	`<p:tnLst><p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot">` +
	`<p:childTnLst><p:par><p:cTn id="2" fill="hold">` +
	`<p:stCondLst><p:cond delay="indefinite"/></p:stCondLst>` +
	`<p:childTnLst><p:par><p:cTn id="3" fill="hold">` +
	`<p:stCondLst><p:cond delay="0"/></p:stCondLst>` +
	`<p:childTnLst><p:par><p:cTn id="4" presetID="10" presetClass="entr"` +
	` presetSubtype="0" fill="hold" grpId="0" nodeType="clickEffect">` +
	`<p:stCondLst><p:cond delay="0"/></p:stCondLst>` +
	`<p:childTnLst>` +
	`<p:set><p:cBhvr>` +
	`<p:cTn id="5" dur="1" fill="hold"/>` +
	`<p:tgtEl><p:spTgt spid="2"/></p:tgtEl>` +
	`<p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst>` +
	`</p:cBhvr><p:to><p:strVal val="visible"/></p:to></p:set>` +
	`<p:animEffect transition="in" filter="fade">` +
	`<p:cBhvr><p:cTn id="6" dur="500"/><p:tgtEl><p:spTgt spid="2"/></p:tgtEl></p:cBhvr>` +
	`</p:animEffect>` +
	`</p:childTnLst></p:cTn></p:par></p:childTnLst>` +
	`</p:cTn></p:par></p:childTnLst>` +
	`</p:cTn></p:par></p:childTnLst>` +
	`</p:cTn></p:par></p:tnLst></p:timing>`

// verticalSlideXML 组装垂直验证页面：正文占位符（含替换目标）+ 未知
// 扩展 + 完整动画树。CT_Slide 子元素次序遵循 ECMA：cSld → clrMapOvr →
// transition → timing（本 fixture 无 transition，故 timing 直接跟在
// clrMapOvr 之后）。
func verticalSlideXML() string {
	return `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument +
		`" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Body 1"/><p:cNvSpPr/><p:nvPr>` +
		`<p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr/><p:txBody>` +
		`<a:bodyPr/><a:lstStyle/>` +
		`<a:p><a:r><a:rPr lang="en-US"/><a:t>Quarterly REPORT_BODY Summary</a:t></a:r></a:p>` +
		`<a:p><a:r><a:rPr lang="zh-CN"/><a:t>季度汇总保持不变</a:t></a:r></a:p>` +
		`</p:txBody></p:sp>` +
		verticalUnknownExt +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		verticalTiming +
		`</p:sld>`
}

// verticalDeck 构造垂直验证用的完整包字节。
func verticalDeck(t *testing.T) []byte {
	t.Helper()
	parts := minimalTemplateParts()
	parts[opc.PartName("/ppt/slides/slide1.xml")] = []byte(xmlDecl + verticalSlideXML())
	// CT override + 关系 + sldId（单页）。
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
	data, err := buildPackageZip(parts)
	if err != nil {
		t.Fatalf("buildPackageZip: %v", err)
	}
	return data
}

// ---------- 断言辅助 ----------

// zipEntryHashes 返回包内每个条目的解压内容 SHA-256（B1 基线）。
//
// 直接读容器条目而非经 opc 层，以校验"实际落盘字节"——B1 的判定对象
// 就是输出包里的字节，绕开任何解析层视图。
func zipEntryHashes(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	out := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			rc.Close()
			t.Fatalf("read %s: %v", f.Name, err)
		}
		rc.Close()
		sum := sha256.Sum256(buf.Bytes())
		out[f.Name] = string(sum[:])
	}
	return out
}

// diffHashes 返回两份哈希表的差异（内容变化 / 新增 / 删除）。
func diffHashes(before, after map[string]string) (changed, added, removed []string) {
	for name, h := range before {
		got, ok := after[name]
		switch {
		case !ok:
			removed = append(removed, name)
		case got != h:
			changed = append(changed, name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			added = append(added, name)
		}
	}
	sort.Strings(changed)
	sort.Strings(added)
	sort.Strings(removed)
	return changed, added, removed
}

// ---------- 垂直验证 ----------

// TestVertical_B1SingleRunEdit 是 M0 垂直验证主用例：仅修改一个普通
// 文本 Run，验证 B1——除被编辑的 slide Part 外所有条目字节级一致。
func TestVertical_B1SingleRunEdit(t *testing.T) {
	p := openFixture(t, verticalDeck(t))
	s := mustSlide(t, p)

	// 1) 基线：保存一次（无编辑），取全条目哈希。
	var baseline bytes.Buffer
	if _, err := p.Write(context.Background(), &baseline); err != nil {
		t.Fatalf("baseline Write: %v", err)
	}
	before := zipEntryHashes(t, baseline.Bytes())

	// 2) 唯一一次编辑：改一个 Run 的文本。
	tf := slideBodyTF(t, s)
	paras, err := tf.Paragraphs()
	if err != nil || len(paras) != 2 {
		t.Fatalf("paragraphs = %d, %v", len(paras), err)
	}
	res, err := paras[0].ReplaceText("REPORT_BODY", "Q3-2026")
	if err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	if res.Replaced != 1 {
		t.Fatalf("expected exactly one replacement, got %+v", res)
	}

	// 3) 保存并比对。
	var out bytes.Buffer
	if _, err := p.Write(context.Background(), &out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	after := zipEntryHashes(t, out.Bytes())

	changed, added, removed := diffHashes(before, after)
	if len(changed) != 1 || changed[0] != "ppt/slides/slide1.xml" {
		t.Errorf("B1 violation: changed entries = %v (want only ppt/slides/slide1.xml)", changed)
	}
	if len(added) != 0 {
		t.Errorf("unexpected added entries: %v", added)
	}
	if len(removed) != 0 {
		t.Errorf("unexpected removed entries: %v", removed)
	}
}

// TestVertical_UnknownExtAndTimingBytePreserved 验证**同一 Part 内**
// 未被触碰的区域字节不变——保真补丁相对"重新序列化整棵树"的核心优势。
func TestVertical_UnknownExtAndTimingBytePreserved(t *testing.T) {
	p := openFixture(t, verticalDeck(t))
	s := mustSlide(t, p)

	// 编辑前确认 fixture 含两个关键区域。
	origXML := slideXML(t, s)
	if !strings.Contains(origXML, verticalUnknownExt) {
		t.Fatalf("fixture missing unknown ext; slide XML = %s", origXML)
	}
	if !strings.Contains(origXML, verticalTiming) {
		t.Fatalf("fixture missing timing subtree; slide XML = %s", origXML)
	}

	tf := slideBodyTF(t, s)
	paras, err := tf.Paragraphs()
	if err != nil || len(paras) != 2 {
		t.Fatalf("paragraphs = %d, %v", len(paras), err)
	}
	if _, err := paras[0].ReplaceText("REPORT_BODY", "Q3-2026"); err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	newXML := slideXML(t, s)

	// 断言 1：未知扩展子树字节完全保留（含实体与嵌套）。
	if !strings.Contains(newXML, verticalUnknownExt) {
		t.Errorf("unknown extension subtree not byte-preserved.\nwant substring: %s\ngot slide XML: %s",
			verticalUnknownExt, newXML)
	}
	// 断言 2：p:timing 动画子树字节完全保留。
	if !strings.Contains(newXML, verticalTiming) {
		t.Errorf("p:timing subtree not byte-preserved.\nwant substring: %s\ngot slide XML: %s",
			verticalTiming, newXML)
	}
	// 断言 3：文本确实改了（否则前面两条断言形同虚设）。
	if !strings.Contains(newXML, "Q3-2026") || strings.Contains(newXML, "REPORT_BODY") {
		t.Errorf("edit did not take effect; slide XML = %s", newXML)
	}
	// 断言 4：第二个段落未受影响。
	if !strings.Contains(newXML, "季度汇总保持不变") {
		t.Errorf("untouched paragraph lost; slide XML = %s", newXML)
	}
}

// TestVertical_TimingIRSurvivesRoundTrip 验证垂直验证的时序侧：
// 编辑 → 保存 → 重新打开后，p:timing 原始字节不退化。TIMIR-01 的 IR
// 投影由 ir 包消费与测试（ir/timingir_test.go），本包只负责保证透出的
// 原始字节在往返后一致。
func TestVertical_TimingIRSurvivesRoundTrip(t *testing.T) {
	p := openFixture(t, verticalDeck(t))
	s := mustSlide(t, p)

	rawBefore, diags, err := s.TimingTreeRaw()
	if err != nil {
		t.Fatalf("TimingTreeRaw: %v", err)
	}
	if len(rawBefore) == 0 {
		t.Fatalf("expected non-empty p:timing raw bytes from vertical fixture (diags=%+v)", diags)
	}

	// 编辑一个 Run 后保存。
	tf := slideBodyTF(t, s)
	paras, err := tf.Paragraphs()
	if err != nil || len(paras) != 2 {
		t.Fatalf("paragraphs = %d, %v", len(paras), err)
	}
	if _, err := paras[0].ReplaceText("REPORT_BODY", "Q3-2026"); err != nil {
		t.Fatalf("ReplaceText: %v", err)
	}
	var out bytes.Buffer
	if _, err := p.Write(context.Background(), &out); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// 重新打开，比较时序原始字节。
	p2, err := OpenReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer p2.Close()
	s2 := mustSlide(t, p2)
	rawAfter, _, err := s2.TimingTreeRaw()
	if err != nil {
		t.Fatalf("TimingTreeRaw after round-trip: %v", err)
	}
	if !bytes.Equal(rawBefore, rawAfter) {
		t.Errorf("p:timing raw bytes changed across edit+save+reopen.\nbefore: %s\nafter:  %s",
			rawBefore, rawAfter)
	}
}
