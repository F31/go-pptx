package engine

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/F31/go-pptx/internal/ir"
	"github.com/F31/go-pptx/pptx"
)

const (
	irNSA     = "http://schemas.openxmlformats.org/drawingml/2006/main"
	irNSP     = "http://schemas.openxmlformats.org/presentationml/2006/main"
	irNSR     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	irNSRels  = "http://schemas.openxmlformats.org/package/2006/relationships"
	irNSCT    = "http://schemas.openxmlformats.org/package/2006/content-types"
	irNSTable = "http://schemas.openxmlformats.org/drawingml/2006/table"
)

// irZipBytes 构造含一个 slide1 的单页最小 zip 字节（slideXML 为空的
// p:sld 内容；传 nil 表示省略该 Part，用于覆盖投影读取错误分支）。
func irZipBytes(t *testing.T, slideXML *string) []byte {
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
	put("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Types xmlns="`+irNSCT+`">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`+
		`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>`+
		`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>`+
		`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`+
		`</Types>`)
	put("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="`+irNSRels+`">`+
		`<Relationship Id="rId1" Type="`+irNSR+`/officeDocument" Target="ppt/presentation.xml"/>`+
		`</Relationships>`)
	put("ppt/presentation.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<p:presentation xmlns:r="`+irNSR+`" xmlns:p="`+irNSP+`">`+
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`+
		`<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>`+
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
	if slideXML != nil {
		put("ppt/slides/slide1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
			`<p:sld xmlns:a="`+irNSA+`" xmlns:r="`+irNSR+`" xmlns:p="`+irNSP+`">`+
			*slideXML+
			`</p:sld>`)
	}
	put("ppt/slides/_rels/slide1.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="`+irNSRels+`">`+
		`<Relationship Id="rId1" Type="`+irNSR+`/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`+
		`</Relationships>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// irTableDeck 构造含一个 2×2 表格的单页文档（自建最小 zip，走公开 OpenReader）。
func irTableDeck(t *testing.T, spTreeBody string) *pptx.Presentation {
	t.Helper()
	body := `<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` + spTreeBody +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>`
	data := irZipBytes(t, &body)
	p, err := pptx.OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func irTableCell(text string) string {
	body := `<a:txBody><a:bodyPr/><a:p/>`
	if text != "" {
		body = `<a:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p>`
	}
	body += `</a:txBody>`
	return `<a:tc>` + body + `<a:tcPr/></a:tc>`
}

func irTableFrame(rows string) string {
	return `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="2" name="Table 2"/>` +
		`<p:cNvGraphicFramePr><a:graphicFrameLocks noGrp="1"/></p:cNvGraphicFramePr><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="1000" y="2000"/><a:ext cx="5000" cy="3000"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="` + irNSTable + `"><a:tbl>` +
		`<a:tblPr/><a:tblGrid><a:gridCol w="2500"/><a:gridCol w="2500"/></a:tblGrid>` + rows +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
}

func TestFromPresentation_SlideShapesReadError(t *testing.T) {
	// 损坏的 p:sld → SlideShapes 解码失败 → 页面 IR_SHAPES_READ 诊断。
	bad := `<p:cSld><p:spTree><broken`
	data := irZipBytes(t, &bad)
	p, err := pptx.OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer p.Close()
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "IR_SHAPES_READ" && d.Severity == ir.SevWarning {
			found = true
		}
	}
	if !found {
		t.Fatalf("want IR_SHAPES_READ diag, got %+v", doc.Diagnostics)
	}
}

func TestFromPresentation_SlidePartMissing(t *testing.T) {
	// sldIdLst 引用 slide1 但 Part 缺失 → PartBytes=false → IR_SHAPES_READ。
	data := irZipBytes(t, nil)
	p, err := pptx.OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer p.Close()
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(doc.Pages))
	}
	if len(doc.Pages[0].Shapes) != 0 {
		t.Errorf("shapes = %d, want 0", len(doc.Pages[0].Shapes))
	}
	found := false
	for _, d := range doc.Pages[0].Diagnostics {
		if d.Code == "IR_SHAPES_READ" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want IR_SHAPES_READ on page, got %+v", doc.Pages[0].Diagnostics)
	}
}

func TestFromPresentation_TableTextProjection(t *testing.T) {
	p := irTableDeck(t, irTableFrame(
		`<a:tr>`+irTableCell("Name")+irTableCell("Qty")+`</a:tr>`+
			`<a:tr>`+irTableCell("Apple")+irTableCell("3")+`</a:tr>`))
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("FromPresentation: %v", err)
	}
	if len(doc.Pages) != 1 || len(doc.Pages[0].Shapes) != 1 {
		t.Fatalf("pages/shapes = %d/%d", len(doc.Pages), len(doc.Pages[0].Shapes))
	}
	sh := doc.Pages[0].Shapes[0]
	if sh.Kind != "table" {
		t.Fatalf("kind = %q, want table", sh.Kind)
	}
	if sh.TableRows != 2 || sh.TableCols != 2 {
		t.Fatalf("table dims = %dx%d, want 2x2", sh.TableRows, sh.TableCols)
	}
	want := "Name\tQty\nApple\t3"
	if sh.Text != want {
		t.Fatalf("table text = %q, want %q", sh.Text, want)
	}
}

func TestFromPresentation_TableEmptyCellAndZeroDim(t *testing.T) {
	// 空单元格 → 保留分隔符。
	p := irTableDeck(t, irTableFrame(
		`<a:tr>`+irTableCell("A")+irTableCell("")+`</a:tr>`))
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("FromPresentation: %v", err)
	}
	if got := doc.Pages[0].Shapes[0].Text; got != "A\t" {
		t.Fatalf("empty-cell text = %q, want %q", got, "A\t")
	}
	// 零行表格 → 空文本（rowCount<=0 分支）。
	p2 := irTableDeck(t, irTableFrame(``))
	doc2, err := ProjectIR(p2, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("FromPresentation zero-row: %v", err)
	}
	if got := doc2.Pages[0].Shapes[0].Text; got != "" {
		t.Fatalf("zero-row text = %q, want empty", got)
	}
	if got := doc2.Pages[0].Shapes[0].TableRows; got != 0 {
		t.Fatalf("zero-row TableRows = %d", got)
	}
}

func irSp(name, id, text string) string {
	return `<p:sp>` +
		`<p:nvSpPr><p:cNvPr id="` + id + `" name="` + name + `"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100" cy="100"/></a:xfrm><a:prstGeom prst="rect"/></p:spPr>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody>` +
		`</p:sp>`
}

func TestProjectIR_GroupAndAutoShape(t *testing.T) {
	p := irTableDeck(t, irSp("Top", "10", "TopText")+
		`<p:grpSp><p:nvGrpSpPr><p:cNvPr id="21" name="Grp 1"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>`+
		irSp("Inner", "22", "InnerText")+
		`</p:grpSp>`)
	doc, err := ProjectIR(p, ir.DefaultOptions())
	if err != nil {
		t.Fatalf("ProjectIR: %v", err)
	}
	shapes := doc.Pages[0].Shapes
	if len(shapes) != 2 {
		t.Fatalf("shapes = %d, want 2 (group flattened)", len(shapes))
	}
	if shapes[0].Text != "TopText" || shapes[1].Text != "InnerText" {
		t.Fatalf("texts = %q / %q", shapes[0].Text, shapes[1].Text)
	}
	if shapes[0].Kind != "autoshape" || shapes[1].Name != "Inner" {
		t.Fatalf("kind/name = %q/%q", shapes[0].Kind, shapes[1].Name)
	}
}
