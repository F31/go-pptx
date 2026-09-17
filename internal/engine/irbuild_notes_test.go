package engine

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/F31/go-pptx/internal/ir"
	"github.com/F31/go-pptx/pptx"
)

// irRichCfg 是灵活的投影测试配置（notes/hidden/timing）。
type irRichCfg struct {
	slideBody string // p:sld 内 spTree 内容
	timing    string // p:timing 内容（"" 表示无）
	hidden    bool   // sldId@show="0"
	notesBody string // notesSlide 正文占位符内容（"" 表示无 notes part）
	// 仅添加 notesSlide 关系/Content-Type 而不写 Part（覆盖读取失败）。
	notesDangling bool
	badTiming     bool // slide timing 元素畸形（覆盖 SlideTimingRaw 错误）
	badNotes      bool // notes part 内容畸形
}

func irRichZip(t *testing.T, cfg irRichCfg) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	put := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	ct := `<Types xmlns="` + irNSCT + `">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>` +
		`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>` +
		`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>` +
		`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>` +
		`<Override PartName="/ppt/notesSlides/notesSlide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"/>` +
		`</Types>`
	put("[Content_Types].xml", ct)
	put("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="`+irNSRels+`">`+
		`<Relationship Id="rId1" Type="`+irNSR+`/officeDocument" Target="ppt/presentation.xml"/>`+
		`</Relationships>`)
	show := ""
	if cfg.hidden {
		show = ` show="0"`
	}
	put("ppt/presentation.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<p:presentation xmlns:r="`+irNSR+`" xmlns:p="`+irNSP+`">`+
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`+
		`<p:sldIdLst><p:sldId id="256" r:id="rId2"`+show+`/></p:sldIdLst>`+
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>`+
		`</p:presentation>`)
	put("ppt/_rels/presentation.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="`+irNSRels+`">`+
		`<Relationship Id="rId1" Type="`+irNSR+`/slideMaster" Target="slideMasters/slideMaster1.xml"/>`+
		`<Relationship Id="rId2" Type="`+irNSR+`/slide" Target="slides/slide1.xml"/>`+
		`</Relationships>`)
	put("ppt/slideMasters/slideMaster1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<p:sldMaster xmlns:r="`+irNSR+`" xmlns:p="`+irNSP+`"><p:cSld><p:spTree>`+
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>`+
		`</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/></p:sldMaster>`)
	put("ppt/slideLayouts/slideLayout1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<p:sldLayout xmlns:r="`+irNSR+`" xmlns:p="`+irNSP+`" type="blank" preserve="1">`+
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>`+
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`)
	put("ppt/slideLayouts/_rels/slideLayout1.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="`+irNSRels+`">`+
		`<Relationship Id="rId1" Type="`+irNSR+`/slideMaster" Target="../slideMasters/slideMaster1.xml"/>`+
		`</Relationships>`)
	timing := ""
	if cfg.timing != "" {
		timing = cfg.timing
	}
	if cfg.badTiming {
		timing = `<p:timing`
	}
	put("ppt/slides/slide1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<p:sld xmlns:a="`+irNSA+`" xmlns:r="`+irNSR+`" xmlns:p="`+irNSP+`">`+
		`<p:cSld><p:spTree>`+
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`+
		`<p:grpSpPr/>`+cfg.slideBody+
		`</p:spTree></p:cSld>`+
		timing+
		`</p:sld>`)
	slideRels := `<Relationships xmlns="` + irNSRels + `">` +
		`<Relationship Id="rId1" Type="` + irNSR + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`
	if cfg.notesBody != "" || cfg.notesDangling {
		slideRels += `<Relationship Id="rId2" Type="` + irNSR + `/notesSlide" Target="../notesSlides/notesSlide1.xml"/>`
	}
	slideRels += `</Relationships>`
	put("ppt/slides/_rels/slide1.xml.rels", slideRels)
	if cfg.notesBody != "" && !cfg.notesDangling {
		body := cfg.notesBody
		if cfg.badNotes {
			body = `<p:notes`
		}
		put("ppt/notesSlides/notesSlide1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
			`<p:notes xmlns:a="`+irNSA+`" xmlns:p="`+irNSP+`">`+
			body+
			`</p:notes>`)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func irRichDeck(t *testing.T, cfg irRichCfg) *pptx.Presentation {
	t.Helper()
	data := irRichZip(t, cfg)
	p, err := pptx.OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func irNotesBody(text string) string {
	p := ""
	if text != "" {
		p = `<a:p><a:r><a:t>` + text + `</a:t></a:r></a:p>`
	}
	return `<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name=""/><p:cNvSpPr txBox="1"/>` +
		`<p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/>` + p + `</p:txBody></p:sp>` +
		`</p:spTree></p:cSld>`
}

func TestFromPresentation_NotesProjected(t *testing.T) {
	p := irRichDeck(t, irRichCfg{
		notesBody: `<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
			`<p:sp><p:nvSpPr><p:cNvPr id="2" name=""/><p:cNvSpPr txBox="1"/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
			`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>talk one</a:t></a:r></a:p>` +
			`<a:p><a:r><a:t>talk two</a:t></a:r></a:p></p:txBody></p:sp>` +
			`</p:spTree></p:cSld>`,
	})
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if len(doc.Pages) != 1 || doc.Pages[0].NotesText != "talk one\ntalk two" {
		t.Fatalf("notes = %+v", doc.Pages)
	}
}

func TestFromPresentation_NotesMissing(t *testing.T) {
	// 有 notesSlide 关系但没有正文占位符 → 空串。
	p := irRichDeck(t, irRichCfg{notesBody: `<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name=""/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>hdr</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld>`})
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if len(doc.Pages) != 1 || doc.Pages[0].NotesText != "" {
		t.Fatalf("notes = %+v", doc.Pages[0].NotesText)
	}
}

func TestFromPresentation_NotesBad(t *testing.T) {
	// 畸形 notes part → IR_NOTES_READ 诊断。
	p := irRichDeck(t, irRichCfg{notesBody: `<p:cSld>`, badNotes: true})
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "IR_NOTES_READ" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want IR_NOTES_READ, got %+v", doc.Diagnostics)
	}
}

func TestFromPresentation_NoNotesRel(t *testing.T) {
	// 无 notesSlide 关系 → 空串且无诊断。
	p := irRichDeck(t, irRichCfg{})
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if len(doc.Pages) != 1 || doc.Pages[0].NotesText != "" {
		t.Fatalf("notes = %+v", doc.Pages[0].NotesText)
	}
	for _, d := range doc.Diagnostics {
		if d.Code == "IR_NOTES_READ" {
			t.Fatalf("unexpected IR_NOTES_READ: %+v", d)
		}
	}
}

func TestFromPresentation_NotesRelDangling(t *testing.T) {
	// 关系存在但 notes Part 缺失 → IR_NOTES_READ。
	p := irRichDeck(t, irRichCfg{notesDangling: true})
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "IR_NOTES_READ" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want IR_NOTES_READ, got %+v", doc.Diagnostics)
	}
}

func TestFromPresentation_HiddenProjected(t *testing.T) {
	p := irRichDeck(t, irRichCfg{hidden: true})
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if len(doc.Pages) != 1 || doc.Pages[0].Hidden == nil || !*doc.Pages[0].Hidden {
		t.Fatalf("hidden = %+v", doc.Pages[0].Hidden)
	}
	// op-out: IncludeHidden=false → nil。
	opts := ir.DefaultOptions()
	opts.IncludeHidden = false
	doc, err = ProjectIR(p, opts)
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if doc.Pages[0].Hidden != nil {
		t.Fatalf("hidden should be nil when opted out, got %v", *doc.Pages[0].Hidden)
	}
}

func TestFromPresentation_TimingProjected(t *testing.T) {
	p := irRichDeck(t, irRichCfg{
		timing: `<p:timing><p:tnLst><p:par><p:cTn id="900000" dur="indefinite" nodeType="tmRoot">` +
			`<p:stCondLst><p:cond evt="begin" delay="0"/></p:stCondLst>` +
			`<p:childTnLst><p:par><p:cTn id="1" dur="500"><p:tgtEl><p:spTgt spid="7"/></p:tgtEl></p:cTn></p:par></p:childTnLst>` +
			`</p:cTn></p:par></p:tnLst></p:timing>`,
	})
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("pages = %d", len(doc.Pages))
	}
	if !doc.Pages[0].HasTiming {
		t.Fatal("HasTiming should be true")
	}
	if doc.Pages[0].Timing == nil || doc.Pages[0].Timing.Root == nil {
		t.Fatalf("Timing not projected: %+v", doc.Pages[0].Timing)
	}
	// IncludeTimingIR 关闭时只填 HasTiming。
	opts := ir.DefaultOptions()
	opts.IncludeTimingIR = false
	doc, err = ProjectIR(p, opts)
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if !doc.Pages[0].HasTiming || doc.Pages[0].Timing != nil {
		t.Fatalf("opts: HasTiming=%v Timing=%+v", doc.Pages[0].HasTiming, doc.Pages[0].Timing)
	}
}
