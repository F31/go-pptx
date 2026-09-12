package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- CHART-01 测试 ----------

// chartSpecFixture 返回一份标准三系列柱状图数据。
func chartSpecFixture() ChartSpec {
	return ChartSpec{
		Type:       ChartBar,
		Title:      "季度营收",
		Categories: []string{"Q1", "Q2", "Q3"},
		Series: []ChartSeries{
			{Name: "2025", Values: []float64{10, 20, 15}},
			{Name: "2026", Values: []float64{12, 25, 18}},
		},
		X:      914400,
		Y:      914400,
		Width:  6096000,
		Height: 4064000,
	}
}

// chartShapesOf 从页面形状中挑出图表句柄。
func chartShapesOf(t *testing.T, s *Slide) []*ChartShape {
	t.Helper()
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	var out []*ChartShape
	for _, sh := range shapes {
		if cs, ok := sh.(*ChartShape); ok {
			out = append(out, cs)
		}
	}
	return out
}

// workbookCells 解析嵌入工作簿 sheet1 的单元格（ref → 文本）。
func workbookCells(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader(workbook): %v", err)
	}
	var sheet []byte
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open sheet: %v", err)
			}
			sheet = bytesReadAll(t, rc)
			rc.Close()
		}
	}
	if sheet == nil {
		t.Fatal("workbook has no xl/worksheets/sheet1.xml")
	}
	doc, err := xmlstore.Index(sheet)
	if err != nil {
		t.Fatalf("index sheet: %v", err)
	}
	cells := map[string]string{}
	var walk func(n *xmlstore.NodeRecord)
	walk = func(n *xmlstore.NodeRecord) {
		if n.Namespace == nsSpreadsheetML && n.Local() == "c" {
			ref, _ := n.Attr("", "r")
			if ref != "" {
				if v := childOfKind(doc, n, nsSpreadsheetML, "v", 0); v != nil {
					cells[ref] = nodeText(doc, v)
				} else if is := childOfKind(doc, n, nsSpreadsheetML, "is", 0); is != nil {
					var sb strings.Builder
					var twalk func(m *xmlstore.NodeRecord)
					twalk = func(m *xmlstore.NodeRecord) {
						if m.Namespace == nsSpreadsheetML && m.Local() == "t" {
							sb.WriteString(nodeText(doc, m))
						}
						for _, cid := range m.Children {
							twalk(doc.Node(cid))
						}
					}
					twalk(is)
					cells[ref] = sb.String()
				}
			}
		}
		for _, cid := range n.Children {
			walk(doc.Node(cid))
		}
	}
	walk(doc.Root())
	return cells
}

func bytesReadAll(t *testing.T, r interface{ Read([]byte) (int, error) }) []byte {
	t.Helper()
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		buf.Write(tmp[:n])
		if err != nil {
			break
		}
	}
	return buf.Bytes()
}

// SlidesOf 是取页列表的测试便捷方法。
func SlidesOf(t *testing.T, p *Presentation) []*Slide {
	t.Helper()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	if len(slides) == 0 {
		t.Fatal("no slides")
	}
	return slides
}

func TestChartAddChart_BarStructure(t *testing.T) {
	p := audioDeck(t) // 单页空模板（与图表无关，仅复用建库辅助）
	defer p.Close()
	s := SlidesOf(t, p)[0]
	cs, err := s.AddChart(context.Background(), chartSpecFixture())
	if err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	if cs.Kind() != ShapeChart {
		t.Fatalf("Kind = %v, want ShapeChart", cs.Kind())
	}
	if cs.ID() == 0 {
		t.Fatal("chart shape id = 0")
	}
	// chart Part 与工作簿 Part 已创建。
	chartBytes, err := p.partBytes("/ppt/charts/chart1.xml")
	if err != nil {
		t.Fatalf("chart part: %v", err)
	}
	wbBytes, err := p.partBytes("/ppt/embeddings/Microsoft_Excel_Worksheet1.xlsx")
	if err != nil {
		t.Fatalf("workbook part: %v", err)
	}
	// chart Part：类型/缓存/引用。
	chartXML := string(chartBytes)
	for _, want := range []string{
		`<c:barChart>`, `<c:barDir val="col"/>`, `<c:grouping val="clustered"/>`,
		`Sheet1!$A$2:$A$4`, `Sheet1!$B$2:$B$4`, `Sheet1!$C$2:$C$4`,
		`<c:v>Q1</c:v>`, `<c:v>25</c:v>`, `<c:formatCode>General</c:formatCode>`,
		`<c:axId val="100000001"/>`, `<c:axId val="100000002"/>`,
		`<c:catAx>`, `<c:valAx>`, "季度营收",
	} {
		if !strings.Contains(chartXML, want) {
			t.Errorf("chart XML missing %q", want)
		}
	}
	// slide → chart 关系与 graphicFrame。
	slideBytes, _ := p.partBytes("/ppt/slides/slide1.xml")
	if !strings.Contains(string(slideBytes), "<p:graphicFrame>") {
		t.Fatal("slide has no graphicFrame")
	}
	if !strings.Contains(string(slideBytes), `uri="`+chartGraphicURI+`"`) {
		t.Fatal("graphicFrame has no chart graphicData uri")
	}
	rels, ok, _ := p.relsOf("/ppt/slides/slide1.xml")
	if !ok {
		t.Fatal("slide rels missing")
	}
	foundChart := false
	for _, rel := range rels {
		if rel.Type == relChart && rel.TargetPart == "/ppt/charts/chart1.xml" {
			foundChart = true
		}
	}
	if !foundChart {
		t.Errorf("slide rels have no chart relationship: %+v", rels)
	}
	// chart → 工作簿关系。
	crels, ok, _ := p.relsOf("/ppt/charts/chart1.xml")
	if !ok {
		t.Fatal("chart rels missing")
	}
	foundWb := false
	for _, rel := range crels {
		if rel.Type == relPackage && rel.TargetPart == "/ppt/embeddings/Microsoft_Excel_Worksheet1.xlsx" {
			foundWb = true
		}
	}
	if !foundWb {
		t.Errorf("chart rels have no package relationship: %+v", crels)
	}
	// 工作簿可解且数据一致。
	cells := workbookCells(t, wbBytes)
	if cells["A2"] != "Q1" || cells["A4"] != "Q3" {
		t.Errorf("workbook categories: %+v", cells)
	}
	if cells["B1"] != "2025" || cells["C1"] != "2026" {
		t.Errorf("workbook series names: %+v", cells)
	}
	if cells["B2"] != "10" || cells["C4"] != "18" {
		t.Errorf("workbook values: %+v", cells)
	}
	// 保存输出包含两个 Override。
	var out bytes.Buffer
	report, err := p.Write(context.Background(), &out)
	if err != nil {
		t.Fatalf("Write: %v (report %+v)", err, report)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatalf("saved zip: %v", err)
	}
	var ct []byte
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			rc, _ := f.Open()
			ct = bytesReadAll(t, rc)
			rc.Close()
		}
	}
	for _, want := range []string{
		`PartName="/ppt/charts/chart1.xml" ContentType="` + ctChartPart + `"`,
		`PartName="/ppt/embeddings/Microsoft_Excel_Worksheet1.xlsx" ContentType="` + ctWorkbook + `"`,
	} {
		if !strings.Contains(string(ct), want) {
			t.Errorf("content types missing %q", want)
		}
	}
	// 形状几何（GEOM-01 路径）。
	b, err := cs.Bounds()
	if err != nil {
		t.Fatalf("Bounds: %v", err)
	}
	if b.X != 914400 || b.W != 6096000 {
		t.Errorf("Bounds = %+v", b)
	}
}

func TestChartAddChart_LinePiePlots(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]

	spec := chartSpecFixture()
	spec.Type = ChartLine
	if _, err := s.AddChart(context.Background(), spec); err != nil {
		t.Fatalf("AddChart line: %v", err)
	}
	lineXML := string(mustPartBytes(t, p, "/ppt/charts/chart1.xml"))
	for _, want := range []string{"<c:lineChart>", `<c:marker val="1"/>`, "<c:smooth val=\"0\"/>"} {
		if !strings.Contains(lineXML, want) {
			t.Errorf("line chart missing %q", want)
		}
	}

	spec.Type = ChartPie
	spec.Title = ""
	if _, err := s.AddChart(context.Background(), spec); err != nil {
		t.Fatalf("AddChart pie: %v", err)
	}
	pieXML := string(mustPartBytes(t, p, "/ppt/charts/chart2.xml"))
	for _, want := range []string{"<c:pieChart>", `<c:varyColors val="1"/>`, `<c:firstSliceAng val="0"/>`} {
		if !strings.Contains(pieXML, want) {
			t.Errorf("pie chart missing %q", want)
		}
	}
	if strings.Contains(pieXML, "c:axId") {
		t.Error("pie chart must not have axes")
	}
	// 两个图表的嵌入工作簿独立。
	if _, err := p.partBytes("/ppt/embeddings/Microsoft_Excel_Worksheet2.xlsx"); err != nil {
		t.Errorf("second workbook part: %v", err)
	}
	// 同页两个图表形状。
	if got := len(chartShapesOf(t, s)); got != 2 {
		t.Errorf("chart shapes = %d, want 2", got)
	}
}

func TestChartAddChart_Validation(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	ctx := context.Background()

	cases := []struct {
		name string
		spec ChartSpec
	}{
		{"no series", ChartSpec{Type: ChartBar, Categories: []string{"A"}, Series: nil,
			X: 1, Y: 1, Width: 10, Height: 10}},
		{"no categories", ChartSpec{Type: ChartBar, Categories: nil,
			Series: []ChartSeries{{Name: "s", Values: []float64{1}}},
			X:      1, Y: 1, Width: 10, Height: 10}},
		{"length mismatch", ChartSpec{Type: ChartBar, Categories: []string{"A", "B"},
			Series: []ChartSeries{{Name: "s", Values: []float64{1}}},
			X:      1, Y: 1, Width: 10, Height: 10}},
		{"NaN value", ChartSpec{Type: ChartBar, Categories: []string{"A"},
			Series: []ChartSeries{{Name: "s", Values: []float64{nan()}}},
			X:      1, Y: 1, Width: 10, Height: 10}},
		{"zero size", ChartSpec{Type: ChartBar, Categories: []string{"A"},
			Series: []ChartSeries{{Name: "s", Values: []float64{1}}},
			X:      1, Y: 1, Width: 0, Height: 10}},
	}
	for _, tc := range cases {
		if _, err := s.AddChart(ctx, tc.spec); err == nil {
			t.Errorf("%s: AddChart succeeded, want error", tc.name)
		} else if !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", tc.name, err)
		}
	}
	// 全部失败后无 Part 残留。
	if _, err := p.partBytes("/ppt/charts/chart1.xml"); err == nil {
		t.Error("failed AddChart must not create chart part")
	}
}

func nan() float64 {
	var z float64
	return z / z
}

func TestChartData_RoundTrip(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	spec := chartSpecFixture()
	spec.Title = "A&B <营收> «图表»"
	spec.Series[0].Name = "系列<1>"
	if _, err := s.AddChart(context.Background(), spec); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	got, err := cs.Data()
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	if got.Type != ChartBar || got.Title != spec.Title {
		t.Errorf("type/title = %v/%q", got.Type, got.Title)
	}
	if len(got.Categories) != 3 || got.Categories[0] != "Q1" || got.Categories[2] != "Q3" {
		t.Errorf("categories = %+v", got.Categories)
	}
	if len(got.Series) != 2 {
		t.Fatalf("series count = %d", len(got.Series))
	}
	if got.Series[0].Name != "系列<1>" || got.Series[1].Name != "2026" {
		t.Errorf("series names = %q / %q", got.Series[0].Name, got.Series[1].Name)
	}
	if len(got.Series[0].Values) != 3 || got.Series[0].Values[2] != 15 || got.Series[1].Values[1] != 25 {
		t.Errorf("series values = %+v / %+v", got.Series[0].Values, got.Series[1].Values)
	}
}

// TestChartSetData_CacheAndWorkbookConsistent 是 AT-10 的代码级等价用例：
// 数据更新后图表缓存与嵌入工作簿一致（客户端可编辑性属 L3 冒烟）。
func TestChartSetData_CacheAndWorkbookConsistent(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	shapeID := cs.ID()
	revBefore := p.Revision()

	newData := ChartData{
		Type:       ChartBar,
		Title:      "季度营收（更新）",
		Categories: []string{"一月", "二月", "三月", "四月"},
		Series: []ChartSeries{
			{Name: "2026", Values: []float64{5.5, 7.25, 6, 9}},
			{Name: "2027", Values: []float64{8, 6.5, 11, 12.5}},
		},
	}
	if err := cs.SetData(newData); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	if p.Revision() != revBefore+1 {
		t.Errorf("revision = %d, want %d", p.Revision(), revBefore+1)
	}
	if cs.ID() != shapeID {
		t.Errorf("shape id changed: %d → %d", shapeID, cs.ID())
	}
	// 缓存侧。
	got, err := cs.Data()
	if err != nil {
		t.Fatalf("Data after SetData: %v", err)
	}
	if got.Title != newData.Title || len(got.Categories) != 4 || len(got.Series) != 2 {
		t.Fatalf("Data = %+v", got)
	}
	if got.Series[0].Values[3] != 9 || got.Series[1].Values[2] != 11 {
		t.Errorf("values = %+v / %+v", got.Series[0].Values, got.Series[1].Values)
	}
	// 工作簿侧：与缓存逐格一致。
	wbBytes := mustPartBytes(t, p, "/ppt/embeddings/Microsoft_Excel_Worksheet1.xlsx")
	cells := workbookCells(t, wbBytes)
	want := map[string]string{
		"B1": "2026", "C1": "2027",
		"A2": "一月", "A5": "四月",
		"B2": "5.5", "C5": "12.5",
		"B5": "9", "C4": "11",
	}
	for ref, v := range want {
		if cells[ref] != v {
			t.Errorf("workbook cell %s = %q, want %q", ref, cells[ref], v)
		}
	}
	// 引用范围同步扩展。
	chartXML := string(mustPartBytes(t, p, "/ppt/charts/chart1.xml"))
	if !strings.Contains(chartXML, "Sheet1!$A$2:$A$5") || !strings.Contains(chartXML, "Sheet1!$B$2:$B$5") {
		t.Errorf("chart references not extended: %s", chartXML)
	}
}

func TestChartSetData_RejectsForeignLayout(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	// 模拟外部/客户端改写：在系列内附加 c:dLbls（白名单之外）。
	before := mustPartBytes(t, p, "/ppt/charts/chart1.xml")
	tampered := bytes.Replace(before, []byte(`<c:idx val="0"/><c:order val="0"/>`),
		[]byte(`<c:idx val="0"/><c:order val="0"/><c:dLbls><c:showLegendKey val="0"/></c:dLbls>`), 1)
	if bytes.Equal(tampered, before) {
		t.Fatal("fixture injection failed (anchor not found)")
	}
	if err := p.stagePatch("/ppt/charts/chart1.xml", tampered); err != nil {
		t.Fatalf("stagePatch: %v", err)
	}
	p.commit()

	newData := ChartData{Type: ChartBar, Categories: []string{"X"},
		Series: []ChartSeries{{Name: "s", Values: []float64{1}}}}
	err := cs.SetData(newData)
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("SetData err = %v, want ErrUnsupportedEdit", err)
	}
	// 拒绝后无部分写入：chart Part 保持注入后的原样。
	if after := mustPartBytes(t, p, "/ppt/charts/chart1.xml"); !bytes.Equal(after, tampered) {
		t.Error("rejected SetData must not modify chart part")
	}
}

func TestChartSetData_TypeMismatch(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	before := mustPartBytes(t, p, "/ppt/charts/chart1.xml")
	err := cs.SetData(ChartData{Type: ChartPie, Categories: []string{"X"},
		Series: []ChartSeries{{Name: "s", Values: []float64{1}}}})
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Fatalf("err = %v, want ErrUnsupportedEdit", err)
	}
	if after := mustPartBytes(t, p, "/ppt/charts/chart1.xml"); !bytes.Equal(after, before) {
		t.Error("type-mismatch SetData must not modify chart part")
	}
}

// markerBuilder 是自定义工作簿适配器测试替身。
type markerBuilder struct {
	calls int
}

func (m *markerBuilder) Build(book ChartDataBook) ([]byte, error) {
	m.calls++
	return []byte("MARKER-WORKBOOK"), nil
}

func TestChartCustomWorkbookBuilder(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	mb := &markerBuilder{}
	if err := p.SetChartWorkbookBuilder(mb); err != nil {
		t.Fatalf("SetChartWorkbookBuilder: %v", err)
	}
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	if got := mustPartBytes(t, p, "/ppt/embeddings/Microsoft_Excel_Worksheet1.xlsx"); string(got) != "MARKER-WORKBOOK" {
		t.Errorf("workbook bytes = %q, want marker", got)
	}
	if mb.calls != 1 {
		t.Errorf("builder calls = %d, want 1", mb.calls)
	}
	// SetData 经同一适配器重建。
	cs := chartShapesOf(t, s)[0]
	if err := cs.SetData(ChartData{Type: ChartBar, Categories: []string{"A"},
		Series: []ChartSeries{{Name: "s", Values: []float64{1}}}}); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	if mb.calls != 2 {
		t.Errorf("builder calls after SetData = %d, want 2", mb.calls)
	}
	// 空输出拒绝且整体失败。
	empty := &markerBuilder{}
	p.SetChartWorkbookBuilder(func() ChartWorkbookBuilder { return emptyBuilder{} }())
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("empty builder err = %v, want ErrInvalidArgument", err)
	}
	_ = empty
}

type emptyBuilder struct{}

func (emptyBuilder) Build(ChartDataBook) ([]byte, error) { return nil, nil }

func TestChartSaveReopen(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	newData := ChartData{Type: ChartBar, Title: "重开",
		Categories: []string{"甲", "乙"},
		Series:     []ChartSeries{{Name: "S", Values: []float64{3, 4}}}}
	if err := cs.SetData(newData); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p2, err := OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer p2.Close()
	slides, err := p2.Slides()
	if err != nil || len(slides) == 0 {
		t.Fatalf("reopen Slides: %v", err)
	}
	charts := chartShapesOf(t, slides[0])
	if len(charts) != 1 {
		t.Fatalf("reopen chart shapes = %d", len(charts))
	}
	got, err := charts[0].Data()
	if err != nil {
		t.Fatalf("reopen Data: %v", err)
	}
	if got.Title != "重开" || len(got.Categories) != 2 || got.Series[0].Values[1] != 4 {
		t.Errorf("reopen data = %+v", got)
	}
}

func TestChartClosedErrors(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := cs.Data(); !errors.Is(err, ErrClosed) {
		t.Errorf("Data err = %v, want ErrClosed", err)
	}
	if err := cs.SetData(ChartData{Type: ChartBar, Categories: []string{"X"},
		Series: []ChartSeries{{Values: []float64{1}}}}); !errors.Is(err, ErrClosed) {
		t.Errorf("SetData err = %v, want ErrClosed", err)
	}
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); !errors.Is(err, ErrClosed) {
		t.Errorf("AddChart err = %v, want ErrClosed", err)
	}
}

func TestChartShapeSetAltTextAndDecorative(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	if err := cs.SetAltText("图表替代文本"); err != nil {
		t.Fatalf("SetAltText: %v", err)
	}
	if got := cs.AltText(); got != "图表替代文本" {
		t.Fatalf("AltText = %q", got)
	}
	if cs.IsDecorative() {
		t.Fatal("IsDecorative = true after SetAltText")
	}
	if err := cs.SetDecorative(true); err != nil {
		t.Fatalf("SetDecorative: %v", err)
	}
	if got := cs.AltText(); got != "" {
		t.Fatalf("AltText after decorative = %q, want empty（互斥）", got)
	}
	if !cs.IsDecorative() {
		t.Fatal("IsDecorative = false after SetDecorative(true)")
	}
	// 关闭后失效。
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := cs.SetAltText("x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed SetAltText %v, want ErrClosed", err)
	}
	if err := cs.SetDecorative(false); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed SetDecorative %v, want ErrClosed", err)
	}
}

func TestChartOfGraphicBranches(t *testing.T) {
	idx := func(x string) (*xmlstore.XMLDocument, *xmlstore.NodeRecord) {
		t.Helper()
		doc, err := xmlstore.Index([]byte(x))
		if err != nil {
			t.Fatalf("Index: %v", err)
		}
		return doc, doc.Root()
	}
	// 正常：graphic → graphicData(图表 URI) → c:chart。
	ok := `<p:graphicFrame xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `" xmlns:c="` + nsChartML + `">` +
		`<a:graphic><a:graphicData uri="` + chartGraphicURI + `"><c:chart/></a:graphicData></a:graphic></p:graphicFrame>`
	doc, root := idx(ok)
	if got := chartOfGraphic(doc, root); got == nil || got.Namespace != nsChartML || got.Local() != "chart" {
		t.Fatalf("normal chartOfGraphic = %+v", got)
	}
	// 非图表 URI → nil。
	other := `<p:graphicFrame xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `" xmlns:c="` + nsChartML + `">` +
		`<a:graphic><a:graphicData uri="urn:other"><c:chart/></a:graphicData></a:graphic></p:graphicFrame>`
	doc, root = idx(other)
	if got := chartOfGraphic(doc, root); got != nil {
		t.Fatalf("non-chart URI matched: %+v", got)
	}
	// 无 graphic → nil。
	doc, root = idx(`<p:graphicFrame xmlns:p="` + nsPresentationML + `"/>`)
	if got := chartOfGraphic(doc, root); got != nil {
		t.Fatalf("no graphic matched: %+v", got)
	}
	// graphic 无 graphicData → nil。
	doc, root = idx(`<p:graphicFrame xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `"><a:graphic/></p:graphicFrame>`)
	if got := chartOfGraphic(doc, root); got != nil {
		t.Fatalf("no graphicData matched: %+v", got)
	}
	// graphicData 有 URI 但无 c:chart 子元素 → nil。
	doc, root = idx(`<p:graphicFrame xmlns:a="` + nsDrawingML + `" xmlns:p="` + nsPresentationML + `"><a:graphic><a:graphicData uri="` + chartGraphicURI + `"/></a:graphic></p:graphicFrame>`)
	if got := chartOfGraphic(doc, root); got != nil {
		t.Fatalf("no chart child matched: %+v", got)
	}
}

func TestChartWorkbookPartOfBranches(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddChart(context.Background(), chartSpecFixture()); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	part, err := cs.chartPartOf()
	if err != nil {
		t.Fatalf("chartPartOf: %v", err)
	}
	if name, ok := p.chartWorkbookPartOf(part); !ok || !strings.Contains(string(name), "/ppt/embeddings/") {
		t.Fatalf("chartWorkbookPartOf = %q %v", name, ok)
	}
	// 不存在的 Part → false。
	if _, ok := p.chartWorkbookPartOf("/ppt/charts/nope.xml"); ok {
		t.Fatal("missing part reported workbook")
	}
	// 无 package 关系的 Part（slide 自身）→ false。
	slides, _ := p.Slides()
	if _, ok := p.chartWorkbookPartOf(slides[0].part); ok {
		t.Fatal("slide part reported workbook")
	}
}

func mustPartBytes(t *testing.T, p *Presentation, name opc.PartName) []byte {
	t.Helper()
	b, err := p.partBytes(name)
	if err != nil {
		t.Fatalf("partBytes(%s): %v", name, err)
	}
	return b
}

// ---------- 纯函数（零覆盖消除，2026-09-13 第 5 轮） ----------

// TestChartTypeFromPlot 由图表组元素名解析类型：三类已知 + 未知回落。
func TestChartTypeFromPlot(t *testing.T) {
	for _, tc := range []struct {
		local string
		want  ChartType
		ok    bool
	}{
		{"barChart", ChartBar, true},
		{"lineChart", ChartLine, true},
		{"pieChart", ChartPie, true},
		{"scatterChart", 0, false}, // 不在 CHART-01 受限范围
		{"", 0, false},
	} {
		got, ok := chartTypeFromPlot(tc.local)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("chartTypeFromPlot(%q) = (%v,%v), want (%v,%v)",
				tc.local, got, ok, tc.want, tc.ok)
		}
	}
}
