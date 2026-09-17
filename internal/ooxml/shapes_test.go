package ooxml

import (
	"strings"
	"testing"
)

const (
	tpNS = "http://schemas.openxmlformats.org/presentationml/2006/main"
	taNS = "http://schemas.openxmlformats.org/drawingml/2006/main"
)

func wrapSlide(body string) string {
	return `<p:sld xmlns:p="` + tpNS + `" xmlns:a="` + taNS + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		body +
		`</p:spTree></p:cSld></p:sld>`
}

func sp(id string, name, text, txBox string) string {
	tb := ""
	if txBox != "" {
		tb = ` txBox="1"`
	}
	x := `<p:sp><p:nvSpPr><p:cNvPr id="` + id + `" name="` + name + `"/>` +
		`<p:cNvSpPr` + tb + `/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="10" y="20"/><a:ext cx="300" cy="120"/></a:xfrm><a:prstGeom prst="rect"/></p:spPr>`
	if text != "" {
		x += `<p:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody>`
	}
	return x + `</p:sp>`
}

func TestSlideShapesTextAndAuto(t *testing.T) {
	x := wrapSlide(sp("11", "Box", "hi there", "1") + sp("12", "Rect", "", ""))
	out, err := SlideShapes([]byte(x))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("shapes = %d", len(out))
	}
	if out[0].ID != 11 || out[0].Name != "Box" || out[0].Kind != "textbox" || out[0].Text != "hi there" {
		t.Errorf("box = %+v", out[0])
	}
	if out[1].ID != 12 || out[1].Kind != "autoshape" || out[1].Text != "" {
		t.Errorf("rect = %+v", out[1])
	}
	if out[0].Bounds == nil || out[0].Bounds.X != 10 || out[0].Bounds.Y != 20 || out[0].Bounds.Width != 300 || out[0].Bounds.Height != 120 {
		t.Errorf("bounds = %+v", out[0].Bounds)
	}
	if out[0].NodePath == "" || !strings.Contains(out[0].NodePath, "p:sp[1]") {
		t.Errorf("nodePath = %q", out[0].NodePath)
	}
}

func TestSlideShapesGroupFlatten(t *testing.T) {
	grp := `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="21" name="G"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		sp("22", "Inner", "text", "1") +
		`</p:grpSp>`
	out, err := SlideShapes([]byte(wrapSlide(sp("20", "Top", "", "1") + grp)))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("shapes = %d, want group flattened to child only", len(out))
	}
	if out[0].Name != "Top" || out[1].Name != "Inner" {
		t.Errorf("names = %q/%q", out[0].Name, out[1].Name)
	}
	if out[1].NodePath != "" && !strings.Contains(out[1].NodePath, "p:grpSp[1]") {
		t.Errorf("inner nodePath = %q", out[1].NodePath)
	}
}

func TestSlideShapesPicture(t *testing.T) {
	pic := `<p:pic><p:nvPicPr><p:cNvPr id="31" name="Img" descr="alt text"/>` +
		`<p:cNvPicPr/><p:nvPr/></p:nvPicPr>` +
		`<p:blipFill><a:blip r:embed="rId3" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></p:blipFill>` +
		`<p:spPr><a:xfrm><a:off x="1" y="2"/><a:ext cx="3" cy="4"/></a:xfrm></p:spPr></p:pic>`
	out, err := SlideShapes([]byte(wrapSlide(pic)))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if len(out) != 1 || out[0].Kind != "picture" || out[0].AltText != "alt text" || out[0].Name != "Img" || out[0].ID != 31 {
		t.Fatalf("pic = %+v", out)
	}
}

func TestSlideShapesTable(t *testing.T) {
	tbl := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="41" name="Table"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="0" y="0"/><a:ext cx="10" cy="10"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="` + dmlTableURI + `"><a:tbl>` +
		`<a:tblPr/><a:tblGrid><a:gridCol w="1"/><a:gridCol w="1"/></a:tblGrid>` +
		`<a:tr><a:tc><a:txBody><a:p><a:r><a:t>A</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc>` +
		`<a:tc><a:txBody><a:p><a:r><a:t>B</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc></a:tr>` +
		`<a:tr><a:tc><a:txBody><a:p><a:r><a:t>C</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc>` +
		`<a:tc hMerge="1"><a:txBody/><a:tcPr/></a:tc></a:tr>` +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	out, err := SlideShapes([]byte(wrapSlide(tbl)))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if len(out) != 1 || out[0].Kind != "table" {
		t.Fatalf("table shape = %+v", out)
	}
	if out[0].TableRows != 2 || out[0].TableCols != 2 {
		t.Errorf("dims = %dx%d", out[0].TableRows, out[0].TableCols)
	}
	if out[0].Text != "A\tB\nC" {
		t.Errorf("table text = %q", out[0].Text)
	}
}

func TestSlideShapesChart(t *testing.T) {
	chart := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="51" name="Chart"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="0" y="0"/><a:ext cx="1" cy="1"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="` + dmlChartURI + `"><c:chart xmlns:c="` + dmlChartURI + `" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="rId5"/></a:graphicData></a:graphic></p:graphicFrame>`
	out, err := SlideShapes([]byte(wrapSlide(chart)))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if len(out) != 1 || !out[0].Chart || out[0].Kind != "chart" || out[0].ChartRID != "rId5" {
		t.Fatalf("chart = %+v", out)
	}
}

func TestSlideShapesMalformed(t *testing.T) {
	if _, err := SlideShapes([]byte("<p:sld")); err == nil {
		t.Fatal("want error on malformed slide")
	}
}

func TestSlideShapesEmptyTree(t *testing.T) {
	out, err := SlideShapes([]byte(`<p:sld xmlns:p="` + tpNS + `"><p:cSld/></p:sld>`))
	if err != nil || len(out) != 0 {
		t.Fatalf("empty: %v %d", err, len(out))
	}
}

func TestSlideShapesConnector(t *testing.T) {
	cx := `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="61" name="Line"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="5" cy="5"/></a:xfrm></p:spPr></p:cxnSp>`
	out, err := SlideShapes([]byte(wrapSlide(connectorSp())))
	_ = cx
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if len(out) != 1 || out[0].Kind != "connector" || out[0].ID != 71 {
		t.Fatalf("cxn = %+v", out)
	}
}

func connectorSp() string {
	return `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="71" name="Line"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="5" cy="5"/></a:xfrm></p:spPr></p:cxnSp>`
}

func TestTextBodyFieldsAndBreaks(t *testing.T) {
	x := wrapSlide(`<p:sp><p:nvSpPr><p:cNvPr id="81" name="T"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/>` +
		`<a:p><a:pPr/><a:fld id="1" type="slidenum"><a:t>1</a:t></a:fld><a:r><a:t>AB</a:t></a:r></a:p>` +
		`<a:p><a:r><a:t>CD</a:t></a:r></a:p>` +
		`</p:txBody></p:sp>`)
	out, err := SlideShapes([]byte(x))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	// 段落间换行；fld 缓存文本并入（textBody 先 r 后 fld，段内次序归组）。
	if out[0].Text != "AB1\nCD" {
		t.Errorf("text = %q, want %q", out[0].Text, "AB1\nCD")
	}
}

func TestNilGuards(t *testing.T) {
	if nv(nil) != nil || nvGroup(nil) != nil || nvCxn(nil) != nil || nvGF(nil) != nil || nvPic(nil) != nil {
		t.Fatal("nil guards must return nil")
	}
}

func TestDecodeTableMalformed(t *testing.T) {
	gf := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="91" name="T"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="0" y="0"/><a:ext cx="1" cy="1"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="` + dmlTableURI + `"><a:tbl><a:broken/></a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	out, err := SlideShapes([]byte(wrapSlide(gf)))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if out[0].Kind != "table" || out[0].TableRows != 0 || out[0].TableCols != 0 {
		t.Errorf("malformed table = %+v", out[0])
	}
}

func TestChartWithoutRID(t *testing.T) {
	gf := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="101" name="C"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="0" y="0"/><a:ext cx="1" cy="1"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="` + dmlChartURI + `"><c:chart xmlns:c="` + dmlChartURI + `"/></a:graphicData></a:graphic></p:graphicFrame>`
	out, err := SlideShapes([]byte(wrapSlide(gf)))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if out[0].Kind != "chart" || !out[0].Chart || out[0].ChartRID != "" {
		t.Errorf("chart = %+v", out[0])
	}
}

func TestGraphicFrameGenericURI(t *testing.T) {
	gf := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="111" name="G"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="0" y="0"/><a:ext cx="1" cy="1"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="urn:custom"><foo/></a:graphicData></a:graphic></p:graphicFrame>`
	out, err := SlideShapes([]byte(wrapSlide(gf)))
	if err != nil {
		t.Fatalf("SlideShapes: %v", err)
	}
	if out[0].Kind != "graphic-frame" {
		t.Errorf("kind = %q", out[0].Kind)
	}
}
