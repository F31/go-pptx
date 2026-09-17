package ooxml

import (
	"strings"
	"testing"
)

const chartNS = "http://schemas.openxmlformats.org/drawingml/2006/chart"
const aNS = "http://schemas.openxmlformats.org/drawingml/2006/main"
const rNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

func barChartXML(title string, cats []string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `" xmlns:r="` + rNS + `">`)
	sb.WriteString(`<c:chart>`)
	if title != "" {
		sb.WriteString(`<c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/>`)
		sb.WriteString(`<a:p><a:r><a:t>` + title + `</a:t></a:r></a:p>`)
		sb.WriteString(`</c:rich></c:tx></c:title>`)
	}
	sb.WriteString(`<c:plotArea>`)
	sb.WriteString(`<c:barChart>`)
	if len(cats) > 0 {
		sb.WriteString(`<c:ser>`)
		sb.WriteString(`<c:cat><c:strRef><c:strCache>`)
		sb.WriteString(`<c:ptCount val="` + itoa(len(cats)) + `"/>`)
		for i, c := range cats {
			sb.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>` + c + `</c:v></c:pt>`)
		}
		sb.WriteString(`</c:strCache></c:strRef></c:cat>`)
		sb.WriteString(`<c:val><c:numRef><c:numCache>`)
		sb.WriteString(`<c:formatCode>General</c:formatCode>`)
		sb.WriteString(`<c:ptCount val="` + itoa(len(cats)) + `"/>`)
		for i := range cats {
			sb.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>1</c:v></c:pt>`)
		}
		sb.WriteString(`</c:numCache></c:numRef></c:val>`)
		sb.WriteString(`</c:ser>`)
	}
	sb.WriteString(`<c:axId val="1"/><c:axId val="2"/>`)
	sb.WriteString(`</c:barChart>`)
	sb.WriteString(`</c:plotArea>`)
	sb.WriteString(`</c:chart>`)
	sb.WriteString(`</c:chartSpace>`)
	return sb.String()
}

func lineChartXML(cats []string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea><c:lineChart>`)
	if len(cats) > 0 {
		sb.WriteString(`<c:ser><c:cat><c:strRef><c:strCache>`)
		sb.WriteString(`<c:ptCount val="` + itoa(len(cats)) + `"/>`)
		for i, c := range cats {
			sb.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>` + c + `</c:v></c:pt>`)
		}
		sb.WriteString(`</c:strCache></c:strRef></c:cat>`)
		sb.WriteString(`<c:val><c:numRef><c:numCache>`)
		sb.WriteString(`<c:formatCode>General</c:formatCode>`)
		sb.WriteString(`<c:ptCount val="` + itoa(len(cats)) + `"/>`)
		for i := range cats {
			sb.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>1</c:v></c:pt>`)
		}
		sb.WriteString(`</c:numCache></c:numRef></c:val>`)
		sb.WriteString(`</c:ser>`)
	}
	sb.WriteString(`<c:axId val="1"/><c:axId val="2"/>`)
	sb.WriteString(`</c:lineChart></c:plotArea></c:chart></c:chartSpace>`)
	return sb.String()
}

func pieChartXML(cats []string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea><c:pieChart>`)
	if len(cats) > 0 {
		sb.WriteString(`<c:ser><c:cat><c:strRef><c:strCache>`)
		sb.WriteString(`<c:ptCount val="` + itoa(len(cats)) + `"/>`)
		for i, c := range cats {
			sb.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>` + c + `</c:v></c:pt>`)
		}
		sb.WriteString(`</c:strCache></c:strRef></c:cat>`)
		sb.WriteString(`<c:val><c:numRef><c:numCache>`)
		sb.WriteString(`<c:formatCode>General</c:formatCode>`)
		sb.WriteString(`<c:ptCount val="` + itoa(len(cats)) + `"/>`)
		for i := range cats {
			sb.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>1</c:v></c:pt>`)
		}
		sb.WriteString(`</c:numCache></c:numRef></c:val>`)
		sb.WriteString(`</c:ser>`)
	}
	sb.WriteString(`</c:pieChart></c:plotArea></c:chart></c:chartSpace>`)
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestChartData_BarChart(t *testing.T) {
	data := []byte(barChartXML("Sales", []string{"Q1", "Q2", "Q3", "Q4"}))
	info, err := ChartData(data)
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Type != "bar" {
		t.Errorf("Type = %q, want bar", info.Type)
	}
	if info.Title != "Sales" {
		t.Errorf("Title = %q, want Sales", info.Title)
	}
	if len(info.Categories) != 4 || info.Categories[0] != "Q1" || info.Categories[3] != "Q4" {
		t.Errorf("Categories = %v, want [Q1 Q2 Q3 Q4]", info.Categories)
	}
}

func TestChartData_LineChart(t *testing.T) {
	data := []byte(lineChartXML([]string{"Jan", "Feb", "Mar"}))
	info, err := ChartData(data)
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Type != "line" {
		t.Errorf("Type = %q, want line", info.Type)
	}
	if len(info.Categories) != 3 || info.Categories[0] != "Jan" {
		t.Errorf("Categories = %v", info.Categories)
	}
}

func TestChartData_PieChart(t *testing.T) {
	data := []byte(pieChartXML([]string{"A", "B", "C"}))
	info, err := ChartData(data)
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Type != "pie" {
		t.Errorf("Type = %q, want pie", info.Type)
	}
}

func TestChartData_NoTitle(t *testing.T) {
	data := []byte(barChartXML("", []string{"X", "Y"}))
	info, err := ChartData(data)
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Title != "" {
		t.Errorf("Title = %q, want empty", info.Title)
	}
	if info.Type != "bar" {
		t.Errorf("Type = %q, want bar", info.Type)
	}
}

func TestChartData_NoCategories(t *testing.T) {
	data := []byte(barChartXML("T", nil))
	info, err := ChartData(data)
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if len(info.Categories) != 0 {
		t.Errorf("Categories = %v, want empty", info.Categories)
	}
}

func TestChartData_Malformed(t *testing.T) {
	_, err := ChartData([]byte(`not xml`))
	if err == nil {
		t.Fatal("expected error for malformed XML")
	}
}

func TestChartData_Nil(t *testing.T) {
	info, err := ChartData(nil)
	if err != nil {
		t.Fatalf("ChartData(nil): %v", err)
	}
	if info.Type != "" || info.Title != "" || info.Categories != nil {
		t.Errorf("expected zero value for nil input: %+v", info)
	}
}

func TestChartData_NoChartElement(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<c:chartSpace xmlns:c="` + chartNS + `"/>`)
	info, err := ChartData(data)
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Type != "" {
		t.Errorf("Type = %q, want empty", info.Type)
	}
}

func TestChartPartOf(t *testing.T) {
	rels := `<Relationships xmlns="` + relNS + `">` +
		`<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart1.xml"/>` +
		`</Relationships>`
	part, ok := ChartPartOf([]byte(rels), "/ppt/slides/slide1.xml", "rId5")
	if !ok {
		t.Fatal("ChartPartOf returned false")
	}
	if string(part) != "/ppt/charts/chart1.xml" {
		t.Errorf("part = %q, want /ppt/charts/chart1.xml", part)
	}
}

func TestChartPartOf_NotFound(t *testing.T) {
	rels := `<Relationships xmlns="` + relNS + `">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="notes/notesSlide1.xml"/>` +
		`</Relationships>`
	_, ok := ChartPartOf([]byte(rels), "/ppt/slides/slide1.xml", "rId5")
	if ok {
		t.Fatal("expected false for non-existent rid")
	}
}

func TestChartPartOf_EmptyRid(t *testing.T) {
	_, ok := ChartPartOf([]byte(`<Relationships xmlns="`+relNS+`"/>`), "/ppt/slides/slide1.xml", "")
	if ok {
		t.Fatal("expected false for empty rid")
	}
}

func TestChartPartOf_EmptyData(t *testing.T) {
	_, ok := ChartPartOf(nil, "/ppt/slides/slide1.xml", "rId1")
	if ok {
		t.Fatal("expected false for nil data")
	}
}

func TestChartData_NumRefCategories(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea><c:barChart>`)
	sb.WriteString(`<c:ser><c:cat><c:numRef>`)
	sb.WriteString(`<c:f>Sheet1!$A$1:$A$3</c:f>`)
	sb.WriteString(`<c:numCache><c:formatCode>General</c:formatCode>`)
	sb.WriteString(`<c:ptCount val="3"/>`)
	sb.WriteString(`<c:pt idx="0"><c:v>10</c:v></c:pt>`)
	sb.WriteString(`<c:pt idx="1"><c:v>20</c:v></c:pt>`)
	sb.WriteString(`<c:pt idx="2"><c:v>30</c:v></c:pt>`)
	sb.WriteString(`</c:numCache></c:numRef></c:cat>`)
	sb.WriteString(`<c:val><c:numRef><c:numCache>`)
	sb.WriteString(`<c:formatCode>General</c:formatCode>`)
	sb.WriteString(`<c:ptCount val="3"/><c:pt idx="0"><c:v>1</c:v></c:pt>`)
	sb.WriteString(`<c:pt idx="1"><c:v>2</c:v></c:pt><c:pt idx="2"><c:v>3</c:v></c:pt>`)
	sb.WriteString(`</c:numCache></c:numRef></c:val>`)
	sb.WriteString(`</c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`)
	info, err := ChartData([]byte(sb.String()))
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if len(info.Categories) != 3 || info.Categories[0] != "10" {
		t.Errorf("Categories = %v, want [10 20 30]", info.Categories)
	}
}

func TestChartData_StrLitCategories(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea><c:barChart>`)
	sb.WriteString(`<c:ser><c:cat><c:strLit>`)
	sb.WriteString(`<c:ptCount val="2"/>`)
	sb.WriteString(`<c:pt idx="0"><c:v>X</c:v></c:pt>`)
	sb.WriteString(`<c:pt idx="1"><c:v>Y</c:v></c:pt>`)
	sb.WriteString(`</c:strLit></c:cat>`)
	sb.WriteString(`<c:val><c:numRef><c:numCache>`)
	sb.WriteString(`<c:formatCode>General</c:formatCode>`)
	sb.WriteString(`<c:ptCount val="2"/><c:pt idx="0"><c:v>1</c:v></c:pt>`)
	sb.WriteString(`<c:pt idx="1"><c:v>2</c:v></c:pt>`)
	sb.WriteString(`</c:numCache></c:numRef></c:val>`)
	sb.WriteString(`</c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`)
	info, err := ChartData([]byte(sb.String()))
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if len(info.Categories) != 2 || info.Categories[0] != "X" {
		t.Errorf("Categories = %v, want [X Y]", info.Categories)
	}
}

func TestChartData_NumLitCategories(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea><c:barChart>`)
	sb.WriteString(`<c:ser><c:cat><c:numLit>`)
	sb.WriteString(`<c:formatCode>General</c:formatCode>`)
	sb.WriteString(`<c:ptCount val="2"/>`)
	sb.WriteString(`<c:pt idx="0"><c:v>100</c:v></c:pt>`)
	sb.WriteString(`<c:pt idx="1"><c:v>200</c:v></c:pt>`)
	sb.WriteString(`</c:numLit></c:cat>`)
	sb.WriteString(`<c:val><c:numRef><c:numCache>`)
	sb.WriteString(`<c:formatCode>General</c:formatCode>`)
	sb.WriteString(`<c:ptCount val="2"/><c:pt idx="0"><c:v>1</c:v></c:pt>`)
	sb.WriteString(`<c:pt idx="1"><c:v>2</c:v></c:pt>`)
	sb.WriteString(`</c:numCache></c:numRef></c:val>`)
	sb.WriteString(`</c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`)
	info, err := ChartData([]byte(sb.String()))
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if len(info.Categories) != 2 || info.Categories[0] != "100" {
		t.Errorf("Categories = %v, want [100 200]", info.Categories)
	}
}

func TestChartData_UnknownChartType(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea>`)
	sb.WriteString(`<c:scatterChart><c:ser>`)
	sb.WriteString(`<c:cat><c:strLit><c:ptCount val="1"/><c:pt idx="0"><c:v>A</c:v></c:pt></c:strLit></c:cat>`)
	sb.WriteString(`<c:val><c:numRef><c:numCache><c:formatCode>General</c:formatCode>`)
	sb.WriteString(`<c:ptCount val="1"/><c:pt idx="0"><c:v>1</c:v></c:pt></c:numCache></c:numRef></c:val>`)
	sb.WriteString(`</c:ser></c:scatterChart>`)
	sb.WriteString(`</c:plotArea></c:chart></c:chartSpace>`)
	info, err := ChartData([]byte(sb.String()))
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Type != "" {
		t.Errorf("Type = %q, want empty for unsupported chart type", info.Type)
	}
}

func TestChartData_LineChartNilSer(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea><c:lineChart>`)
	sb.WriteString(`<c:axId val="1"/><c:axId val="2"/>`)
	sb.WriteString(`</c:lineChart></c:plotArea></c:chart></c:chartSpace>`)
	info, err := ChartData([]byte(sb.String()))
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Type != "line" {
		t.Errorf("Type = %q, want line", info.Type)
	}
	if len(info.Categories) != 0 {
		t.Errorf("Categories = %v, want empty", info.Categories)
	}
}

func TestChartData_PieChartNilSer(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">` +
		`<c:chart><c:plotArea>` +
		`<c:barChart/>` +
		`<c:pieChart><c:ser><c:cat><c:strLit>` +
		`<c:ptCount val="1"/><c:pt idx="0"><c:v>A</c:v></c:pt>` +
		`</c:strLit></c:cat>` +
		`<c:val><c:numRef><c:numCache><c:formatCode>General</c:formatCode>` +
		`<c:ptCount val="1"/><c:pt idx="0"><c:v>1</c:v></c:pt>` +
		`</c:numCache></c:numRef></c:val>` +
		`</c:ser></c:pieChart>` +
		`</c:plotArea></c:chart></c:chartSpace>`)
	info, err := ChartData(data)
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if info.Type != "bar" {
		t.Errorf("Type = %q, want bar (barChart is first)", info.Type)
	}
}

func TestChartData_AxDataNilCat(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<c:chartSpace xmlns:c="` + chartNS + `" xmlns:a="` + aNS + `">`)
	sb.WriteString(`<c:chart><c:plotArea><c:barChart>`)
	sb.WriteString(`<c:ser><c:val><c:numRef><c:numCache>`)
	sb.WriteString(`<c:formatCode>General</c:formatCode>`)
	sb.WriteString(`<c:ptCount val="1"/><c:pt idx="0"><c:v>1</c:v></c:pt>`)
	sb.WriteString(`</c:numCache></c:numRef></c:val>`)
	sb.WriteString(`</c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`)
	info, err := ChartData([]byte(sb.String()))
	if err != nil {
		t.Fatalf("ChartData: %v", err)
	}
	if len(info.Categories) != 0 {
		t.Errorf("Categories = %v, want empty when cat is nil", info.Categories)
	}
}
