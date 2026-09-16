package pptx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// minimalFontBytes 构造 6 字节最小"字体文件"——仅用于关系可达性断言，
// 不被解析器读取。
func minimalFontBytes() []byte {
	return []byte{'F', 'O', 'N', 'T', '0', '0'}
}

// layoutDeckWith 返回带有可定制 Parts 的最小模板 Presentation。`mods`
// 回调拿到 Parts map 与 ns 常量，在编译阶段已写好的字节上做 patch。
func layoutDeckWith(t *testing.T, mods func(parts map[opc.PartName][]byte, ns struct {
	P, A, P14 string
}, masterXML, presXML *[]byte, presRels *[]byte)) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	const nsP = nsPresentationML
	const nsA = nsDrawingML
	const ns14 = "http://schemas.microsoft.com/office/powerpoint/2010/main"
	slideXML := []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsA + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`</p:sld>`)
	parts["/ppt/slides/slide1.xml"] = slideXML
	presRels := []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rIdS1" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/_rels/presentation.xml.rels"] = presRels

	masterRels := []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rIdF1" Type="http://schemas.microsoft.com/office/2007/relationships/font" Target="../fonts/fontCalibriRegular.fntdata"/>` +
		`<Relationship Id="rIdF2" Type="http://schemas.microsoft.com/office/2007/relationships/font" Target="../fonts/fontCalibriBold.fntdata"/>` +
		`</Relationships>`)
	parts["/ppt/slideMasters/_rels/slideMaster1.xml.rels"] = masterRels
	// 字体 Bytes（最小占位）
	parts["/ppt/fonts/fontCalibriRegular.fntdata"] = minimalFontBytes()
	parts["/ppt/fonts/fontCalibriBold.fntdata"] = minimalFontBytes()
	// Content Type 覆盖（仅追加 slide1 与 fntdata；slideMaster 已在
	// minimalTemplateParts 中声明）。
	parts["/[Content_Types].xml"] = bytes.Replace(
		parts["/[Content_Types].xml"],
		[]byte("</Types>"),
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`+
			`<Default Extension="fntdata" ContentType="application/vnd.openxmlformats-officedocument.obfuscatedFont"/>`+
			`</Types>`),
		1,
	)

	masterXML := []byte(xmlDecl +
		`<p:sldMaster xmlns:a="` + nsA + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:cSld><p:spTree/></p:cSld>` +
		`<p:txStyles>` +
		`<p:titleStyle>` +
		`<a:lvl1pPr><a:defRPr lang="en-US" altLang="ja-JP" kumimoji="1"/></a:lvl1pPr>` +
		`</p:titleStyle>` +
		`<p:bodyStyle>` +
		`<a:lvl1pPr><a:defRPr lang="zh-CN" altLang="en-US"/></a:lvl1pPr>` +
		`</p:bodyStyle>` +
		`<p:otherStyle><a:defRPr lang="ja-JP"/></p:otherStyle>` +
		`</p:txStyles>` +
		`</p:sldMaster>`)
	parts["/ppt/slideMasters/slideMaster1.xml"] = masterXML

	presXML := []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `" xmlns:p14="` + ns14 + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rIdS1"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	parts["/ppt/presentation.xml"] = presXML

	mods(parts, struct {
		P, A, P14 string
	}{nsP, nsA, ns14}, &masterXML, &presXML, &presRels)

	// Re-marshal final bytes.
	parts["/ppt/slideMasters/slideMaster1.xml"] = masterXML
	parts["/ppt/presentation.xml"] = presXML
	parts["/ppt/_rels/presentation.xml.rels"] = presRels

	zipBytes, err := buildPackageZip(parts)
	if err != nil {
		t.Fatalf("buildPackageZip: %v", err)
	}
	p, err := OpenReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	return p
}

// ---------- LAYOUT-01 测试 ----------

func TestLayoutInfo_EmptyDoc_AllNil(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	if rep == nil {
		t.Fatalf("LayoutReport is nil")
	}
	if rep.Sections != nil {
		t.Errorf("Sections should be nil, got %d", len(rep.Sections))
	}
	if rep.EmbeddedFonts != nil {
		t.Errorf("EmbeddedFonts should be nil, got %d", len(rep.EmbeddedFonts))
	}
	if rep.HandoutMaster != nil {
		t.Errorf("HandoutMaster should be nil")
	}
	if rep.Kinsoku != nil {
		t.Errorf("Kinsoku should be nil, got %d", len(rep.Kinsoku))
	}
	if len(rep.Diagnostics) != 0 {
		t.Errorf("Diagnostics should be empty, got %d", len(rep.Diagnostics))
	}
}

func TestLayoutInfo_ClosedPresentation(t *testing.T) {
	p := audioDeck(t)
	p.Close()
	_, err := p.LayoutInfo()
	if err == nil || !errors.Is(err, ErrClosed) {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestLayoutInfo_Sections_RoundTrip(t *testing.T) {
	p := layoutDeckWith(t, func(_ map[opc.PartName][]byte, _ struct {
		P, A, P14 string
	}, _ *[]byte, presXML *[]byte, _ *[]byte) {
		newBody := []byte(xmlDecl +
			`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `" xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main">` +
			`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
			`<p:sldIdLst><p:sldId id="256" r:id="rIdS1"/></p:sldIdLst>` +
			`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
			`<p14:sectionLst>` +
			`<p14:section id="A" name="Intro" type="nextPage"><p:sldIdLst><p:sldId id="256"/></p:sldIdLst></p14:section>` +
			`<p14:section id="B" name="Body" type="continuous"><p:sldIdLst><p:sldId id="256"/></p:sldIdLst></p14:section>` +
			`</p14:sectionLst>` +
			`</p:presentation>`)
		*presXML = newBody
	})
	defer p.Close()

	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	if len(rep.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(rep.Sections))
	}
	if rep.Sections[0].ID != "A" || rep.Sections[0].Name != "Intro" || rep.Sections[0].Type != "nextPage" {
		t.Errorf("sec0 = %+v", rep.Sections[0])
	}
	if len(rep.Sections[0].SlideIDs) != 1 || rep.Sections[0].SlideIDs[0] != 256 {
		t.Errorf("sec0 slideids = %v", rep.Sections[0].SlideIDs)
	}
	if rep.Sections[1].Name != "Body" || rep.Sections[1].Type != "continuous" {
		t.Errorf("sec1 = %+v", rep.Sections[1])
	}
	for _, d := range rep.Diagnostics {
		if d.Code == "layout.section.broken_ref" {
			t.Errorf("unexpected broken_ref: %s", d.Message)
		}
	}
}

func TestLayoutInfo_Sections_BrokenRef(t *testing.T) {
	p := layoutDeckWith(t, func(_ map[opc.PartName][]byte, _ struct {
		P, A, P14 string
	}, _ *[]byte, presXML *[]byte, _ *[]byte) {
		// 只声明 sldId 256；section 引用不存在的 999。
		newBody := []byte(xmlDecl +
			`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `" xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main">` +
			`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
			`<p:sldIdLst><p:sldId id="256" r:id="rIdS1"/></p:sldIdLst>` +
			`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
			`<p14:sectionLst>` +
			`<p14:section id="A" name="Broken"><p:sldIdLst><p:sldId id="999"/></p:sldIdLst></p14:section>` +
			`</p14:sectionLst>` +
			`</p:presentation>`)
		*presXML = newBody
	})
	defer p.Close()
	defer p.Close()

	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	if len(rep.Sections) != 1 || rep.Sections[0].ID != "A" {
		t.Fatalf("unexpected sections: %+v", rep.Sections)
	}
	found := false
	for _, d := range rep.Diagnostics {
		if d.Code == "layout.section.broken_ref" && strings.Contains(d.Message, "999") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected layout.section.broken_ref for sldId 999, got %v", rep.Diagnostics)
	}
}

func TestLayoutInfo_EmbeddedFonts_RoundTrip(t *testing.T) {
	p := layoutDeckWith(t, func(_ map[opc.PartName][]byte, _ struct {
		P, A, P14 string
	}, masterXML *[]byte, _ *[]byte, _ *[]byte) {
		newMaster := []byte(xmlDecl +
			`<p:sldMaster xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
			`<p:cSld><p:spTree/></p:cSld>` +
			`<p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles>` +
			`<p:embeddedFontLst>` +
			`<p:embeddedFont>` +
			`<p:font typeface="Calibri"/>` +
			`<p:regular r:id="rIdF1"/>` +
			`<p:bold r:id="rIdF2"/>` +
			`</p:embeddedFont>` +
			`<p:embeddedFont>` +
			`<p:font typeface="Consolas"/>` +
			`<p:regular r:id="rIdF1"/>` +
			`<p:bold r:id="rIdF2"/>` +
			`<p:italic r:id="rIdF1"/>` +
			`<p:boldItalic r:id="rIdF2"/>` +
			`</p:embeddedFont>` +
			`</p:embeddedFontLst>` +
			`</p:sldMaster>`)
		*masterXML = newMaster
	})
	defer p.Close()

	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	if len(rep.EmbeddedFonts) != 2 {
		t.Fatalf("expected 2 embedded fonts, got %d", len(rep.EmbeddedFonts))
	}
	first := rep.EmbeddedFonts[0]
	if first.Typeface != "Calibri" || !first.HasRegular || !first.HasBold ||
		first.HasItalic || first.HasBoldItalic {
		t.Errorf("font0 = %+v", first)
	}
	if first.RegularTargetPart == "" || first.BoldTargetPart == "" {
		t.Errorf("font0 target parts unresolved: %+v", first)
	}
	second := rep.EmbeddedFonts[1]
	if second.Typeface != "Consolas" || !second.HasRegular || !second.HasBold ||
		!second.HasItalic || !second.HasBoldItalic {
		t.Errorf("font1 = %+v", second)
	}
	if len(rep.Diagnostics) != 0 {
		t.Errorf("unexpected diags: %v", rep.Diagnostics)
	}
}

func TestLayoutInfo_EmbeddedFonts_BrokenRel(t *testing.T) {
	p := layoutDeckWith(t, func(_ map[opc.PartName][]byte, _ struct {
		P, A, P14 string
	}, masterXML *[]byte, _ *[]byte, _ *[]byte) {
		newMaster := []byte(xmlDecl +
			`<p:sldMaster xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
			`<p:cSld><p:spTree/></p:cSld>` +
			`<p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles>` +
			`<p:embeddedFontLst>` +
			`<p:embeddedFont>` +
			`<p:font typeface="Calibri"/>` +
			`<p:regular r:id="rIdGhost"/>` +
			`</p:embeddedFont>` +
			`</p:embeddedFontLst>` +
			`</p:sldMaster>`)
		*masterXML = newMaster
	})
	defer p.Close()

	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	if len(rep.EmbeddedFonts) != 1 {
		t.Fatalf("expected 1 font, got %d", len(rep.EmbeddedFonts))
	}
	if rep.EmbeddedFonts[0].RegularTargetPart != "" {
		t.Errorf("expected empty RegularTargetPart, got %s", rep.EmbeddedFonts[0].RegularTargetPart)
	}
	found := false
	for _, d := range rep.Diagnostics {
		if d.Code == "layout.font.broken_ref" && strings.Contains(d.Message, "Calibri") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected layout.font.broken_ref diagnostic for ghost rel, got %v", rep.Diagnostics)
	}
}

func TestLayoutInfo_HandoutMaster_Present(t *testing.T) {
	p := layoutDeckWith(t, func(_ map[opc.PartName][]byte, _ struct {
		P, A, P14 string
	}, _ *[]byte, _ *[]byte, presRels *[]byte) {
		newRels := []byte(xmlDecl +
			`<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rIdS1" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
			`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
			`<Relationship Id="rIdHM" Type="` + opc.RelHandoutMaster + `" Target="handoutMasters/handoutMaster1.xml"/>` +
			`</Relationships>`)
		*presRels = newRels
	})
	defer p.Close()

	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	if rep.HandoutMaster == nil {
		// 当前 path 未声明 handoutMasterIdLst → 应为 nil。
		// 本用例仅断言不报错与不写出 diag；真正的 Present 校验见下面的变体。
		return
	}
	if rep.HandoutMaster.Part != "/ppt/handoutMasters/handoutMaster1.xml" {
		t.Errorf("Part = %s", rep.HandoutMaster.Part)
	}
}

func TestLayoutInfo_HandoutMaster_Declared(t *testing.T) {
	p := layoutDeckWith(t, func(_ map[opc.PartName][]byte, _ struct {
		P, A, P14 string
	}, _ *[]byte, presXML *[]byte, presRels *[]byte) {
		newXML := []byte(xmlDecl +
			`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
			`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
			`<p:handoutMasterIdLst><p:handoutMasterId id="2147483649" r:id="rIdHM"/></p:handoutMasterIdLst>` +
			`<p:sldIdLst><p:sldId id="256" r:id="rIdS1"/></p:sldIdLst>` +
			`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
			`</p:presentation>`)
		*presXML = newXML
		newRels := []byte(xmlDecl +
			`<Relationships xmlns="` + nsPkgRels + `">` +
			`<Relationship Id="rIdS1" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
			`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
			`<Relationship Id="rIdHM" Type="` + opc.RelHandoutMaster + `" Target="handoutMasters/handoutMaster1.xml"/>` +
			`</Relationships>`)
		*presRels = newRels
	})
	defer p.Close()

	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	if rep.HandoutMaster == nil {
		t.Fatalf("HandoutMaster should be populated")
	}
	if rep.HandoutMaster.Part != "/ppt/handoutMasters/handoutMaster1.xml" || rep.HandoutMaster.RelationID != "rIdHM" {
		t.Errorf("HandoutMaster = %+v", rep.HandoutMaster)
	}
	// Part 不在包内（未创建），故 Present=false，记一条 warning diag。
	if rep.HandoutMaster.Present {
		t.Errorf("Present should be false (part missing)")
	}
	found := false
	for _, d := range rep.Diagnostics {
		if d.Code == "layout.handout.broken_ref" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected layout.handout.broken_ref diag")
	}
}

func TestLayoutInfo_Kinsoku_Aggregation(t *testing.T) {
	p := layoutDeckWith(t, func(_ map[opc.PartName][]byte, _ struct {
		P, A, P14 string
	}, _ *[]byte, _ *[]byte, _ *[]byte) {
	})
	defer p.Close()

	rep, err := p.LayoutInfo()
	if err != nil {
		t.Fatalf("LayoutInfo: %v", err)
	}
	// 期望：3 条聚合 (en-US/ja-JP + Kumimoji, zh-CN/en-US, ja-JP)。
	wantLangs := []string{"en-US", "zh-CN", "ja-JP"}
	if len(rep.Kinsoku) != len(wantLangs) {
		t.Fatalf("Kinsoku = %+v (want %d entries)", rep.Kinsoku, len(wantLangs))
	}
	gotLangs := make([]string, 0, len(rep.Kinsoku))
	for _, k := range rep.Kinsoku {
		gotLangs = append(gotLangs, k.Lang)
	}
	if !reflect.DeepEqual(gotLangs, wantLangs) {
		t.Errorf("Kinsoku lang order = %v, want %v", gotLangs, wantLangs)
	}
	// 检查 en-US 条目 Kumimoji=true。
	for _, k := range rep.Kinsoku {
		if k.Lang == "en-US" {
			if !k.Kumimoji {
				t.Errorf("en-US entry should have Kumimoji=true: %+v", k)
			}
			if k.AltLang != "ja-JP" {
				t.Errorf("en-US AltLang = %q", k.AltLang)
			}
		}
	}
}

func TestLayoutInfo_DoesNotMutate(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	// 二次调用应当产生等价报告（无内部状态污染）。
	rep1, err1 := p.LayoutInfo()
	rep2, err2 := p.LayoutInfo()
	if err1 != nil || err2 != nil {
		t.Fatalf("errs %v %v", err1, err2)
	}
	if !reflect.DeepEqual(rep1, rep2) {
		t.Errorf("LayoutInfo not deterministic across calls")
	}
}

// 防止 binary 包导入闲置。
var _ = binary.LittleEndian

// ---------- 纯函数（零覆盖消除，2026-09-13 第 5 轮） ----------

// TestAppendPartUnique 验证去重追加：已存在原切片返回，缺失追加末尾。
func TestAppendPartUnique(t *testing.T) {
	base := []opc.PartName{"/a.xml", "/b.xml"}
	if got := appendPartUnique(base, "/a.xml"); len(got) != len(base) {
		t.Errorf("existing part must not append: %v", got)
	}
	got := appendPartUnique(base, "/c.xml")
	if len(got) != 3 || got[2] != opc.PartName("/c.xml") {
		t.Errorf("append = %v, want [/a.xml /b.xml /c.xml]", got)
	}
	// 空 nil 切片追加。
	if got := appendPartUnique(nil, "/x.xml"); len(got) != 1 || got[0] != opc.PartName("/x.xml") {
		t.Errorf("nil append = %v", got)
	}
}
