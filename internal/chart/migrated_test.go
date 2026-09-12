package chart

import (
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件承载 ADR-017 第三批从根包 chart_test.go / chartadv_test.go 迁入的
// 白盒测试。
//
// 迁移原因：这些用例直接断言 internal/chart 的实现语义（canonical 白名单、
// 系列读取分支）。留在根包时，被测语句记在 internal/chart 名下却由根包测试
// 二进制执行——per-package 覆盖率不归属，会让全仓 total 跌破 COV-01 的 80%
// 里程碑。迁入本包后归属恢复，且不必为覆盖而保留已被删除的根包 facade。

const testChartNS = "http://schemas.openxmlformats.org/drawingml/2006/chart"

func TestChartTypeFromPlotMapping(t *testing.T) {
	for typ, want := range map[ChartType]string{
		ChartBar:  "barChart",
		ChartLine: "lineChart",
		ChartPie:  "pieChart",
	} {
		if got := typ.PlotElement(); got != want {
			t.Fatalf("plotElement(%d) = %q, want %q", typ, got, want)
		}
		if back, ok := ChartTypeFromPlot(want); !ok || back != typ {
			t.Fatalf("ChartTypeFromPlot(%q) = %d %v, want %d", want, back, ok, typ)
		}
	}
	if ChartType(99).PlotElement() != "" {
		t.Fatal("unknown plotElement not empty")
	}
	if _, ok := ChartTypeFromPlot("areaChart"); ok {
		t.Fatal("unknown plot matched")
	}
}

func TestSerCategoriesAllPaths(t *testing.T) {
	cases := []struct {
		name string
		cat  string
		want []string
	}{
		{"strRef", `<c:cat><c:strRef><c:strCache><c:pt idx="0"><c:v>A</c:v></c:pt><c:pt idx="1"><c:v>B</c:v></c:pt></c:strCache></c:strRef></c:cat>`, []string{"A", "B"}},
		{"numRef", `<c:cat><c:numRef><c:numCache><c:pt idx="0"><c:v>1</c:v></c:pt><c:pt idx="1"><c:v>2</c:v></c:pt></c:numCache></c:numRef></c:cat>`, []string{"1", "2"}},
		{"strLit", `<c:cat><c:strLit><c:pt idx="0"><c:v>X</c:v></c:pt></c:strLit></c:cat>`, []string{"X"}},
		{"numLit", `<c:cat><c:numLit><c:pt idx="0"><c:v>7</c:v></c:pt></c:numLit></c:cat>`, []string{"7"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xml := `<c:chartSpace xmlns:c="` + testChartNS + `"><c:chart><c:plotArea><c:barChart><c:ser>` +
				tc.cat + `<c:val><c:numRef><c:numCache><c:pt idx="0"><c:v>1</c:v></c:pt></c:numCache></c:numRef></c:val>` +
				`</c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`
			doc, err := xmlstore.Index([]byte(xml))
			if err != nil {
				t.Fatalf("Index: %v", err)
			}
			sers := doc.Elements(testChartNS, "ser")
			if len(sers) != 1 {
				t.Fatalf("ser elements = %d", len(sers))
			}
			got, ok := SerCategories(doc, doc.Node(sers[0]))
			if !ok {
				t.Fatal("categories not resolved")
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("categories = %v, want %v", got, tc.want)
			}
		})
	}
	// 无 cat → false。
	xml := `<c:chartSpace xmlns:c="` + testChartNS + `"><c:chart><c:plotArea><c:barChart><c:ser><c:val><c:numRef><c:numCache/></c:numRef></c:val></c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`
	doc, err := xmlstore.Index([]byte(xml))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	sers := doc.Elements(testChartNS, "ser")
	if got, ok := SerCategories(doc, doc.Node(sers[0])); ok || got != nil {
		t.Fatalf("no-cat = %v %v", got, ok)
	}
}

func TestSerValuesAndName(t *testing.T) {
	mkDoc := func(serBody string) (*xmlstore.XMLDocument, *xmlstore.NodeRecord) {
		xml := `<c:chartSpace xmlns:c="` + testChartNS + `"><c:chart><c:plotArea><c:barChart><c:ser>` +
			serBody + `</c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`
		doc, err := xmlstore.Index([]byte(xml))
		if err != nil {
			t.Fatalf("Index: %v", err)
		}
		sers := doc.Elements(testChartNS, "ser")
		return doc, doc.Node(sers[0])
	}
	// 系列名：tx/v 与 tx/strRef/strCache。
	doc, ser := mkDoc(`<c:tx><c:v>Direct</c:v></c:tx>`)
	if got := SerName(doc, ser); got != "Direct" {
		t.Fatalf("tx/v name = %q", got)
	}
	doc, ser = mkDoc(`<c:tx><c:strRef><c:strCache><c:pt idx="0"><c:v>RefName</c:v></c:pt></c:strCache></c:strRef></c:tx>`)
	if got := SerName(doc, ser); got != "RefName" {
		t.Fatalf("strRef name = %q", got)
	}
	doc, ser = mkDoc(`<c:tx><c:strRef><c:strCache/></c:strRef></c:tx>`)
	if got := SerName(doc, ser); got != "" {
		t.Fatalf("empty cache name = %q", got)
	}
	// 无 tx → 空名。
	doc, ser = mkDoc(``)
	if got := SerName(doc, ser); got != "" {
		t.Fatalf("no-tx name = %q", got)
	}
	// 数值：numRef/numCache 与 numLit，含非数值 pt → 0。
	doc, ser = mkDoc(`<c:val><c:numRef><c:numCache><c:pt idx="0"><c:v>1</c:v></c:pt><c:pt idx="1"><c:v>2.5</c:v></c:pt></c:numCache></c:numRef></c:val>`)
	got := SerValues(doc, ser)
	if len(got) != 2 || got[0] != 1 || got[1] != 2.5 {
		t.Fatalf("numRef values = %v", got)
	}
	doc, ser = mkDoc(`<c:val><c:numLit><c:pt idx="0"><c:v>3</c:v></c:pt><c:pt idx="1"><c:v>bad</c:v></c:pt></c:numLit></c:val>`)
	got = SerValues(doc, ser)
	if len(got) != 2 || got[0] != 3 || got[1] != 0 {
		t.Fatalf("numLit values = %v", got)
	}
	doc, ser = mkDoc(`<c:val><c:numRef><c:numCache/></c:numRef></c:val>`)
	if got := SerValues(doc, ser); len(got) != 0 {
		t.Fatalf("empty values = %v", got)
	}
	doc, ser = mkDoc(`<c:val/>`)
	if got := SerValues(doc, ser); len(got) != 0 {
		t.Fatalf("no-val = %v", got)
	}
}

func TestIsCanonicalBranches(t *testing.T) {
	ser := `<c:ser><c:idx val="0"/><c:order val="0"/><c:tx><c:v>s</c:v></c:tx><c:cat><c:strRef><c:strCache><c:pt idx="0"><c:v>a</c:v></c:pt></c:strCache></c:strRef></c:cat><c:val><c:numRef><c:numCache><c:pt idx="0"><c:v>1</c:v></c:pt></c:numCache></c:numRef></c:val></c:ser>`
	bar := `<c:barChart><c:barDir val="col"/><c:grouping val="clustered"/>` + ser + `<c:axId val="1"/><c:axId val="2"/></c:barChart>`
	line := `<c:lineChart><c:grouping val="standard"/>` + ser + `<c:axId val="1"/><c:axId val="2"/></c:lineChart>`
	pie := `<c:pieChart><c:varyColors val="0"/>` + ser + `</c:pieChart>`
	axes := `<c:catAx><c:axId val="1"/></c:catAx><c:valAx><c:axId val="2"/></c:valAx>`
	chart := `<c:chart><c:plotArea>` + bar + axes + `</c:plotArea></c:chart>`

	idx := func(x string) (*xmlstore.XMLDocument, *xmlstore.NodeRecord) {
		t.Helper()
		doc, err := xmlstore.Index([]byte(`<c:chartSpace xmlns:c="` + testChartNS + `">` + x + `</c:chartSpace>`))
		if err != nil {
			t.Fatalf("Index: %v", err)
		}
		return doc, doc.Root()
	}
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"canonical-bar", chart, true},
		{"canonical-line", `<c:chart><c:plotArea>` + line + axes + `</c:plotArea></c:chart>`, true},
		{"canonical-pie", `<c:chart><c:plotArea>` + pie + `</c:plotArea></c:chart>`, true},
		{"extra-chartSpace-child", chart + `<c:foo xmlns:c="` + testChartNS + `"/>`, false},
		{"non-chartML-child", `<c:chart><c:plotArea><x:x xmlns:x="urn:x"/></c:plotArea></c:chart>`, false},
		{"missing-plotArea", `<c:chart/>`, false},
		{"layout-nonempty", `<c:chart><c:plotArea><c:layout><c:manualLayout/></c:layout>` + bar + axes + `</c:plotArea></c:chart>`, false},
		{"multi-plot", `<c:chart><c:plotArea>` + bar + pie + axes + `</c:plotArea></c:chart>`, false},
		{"unknown-bar-child", `<c:chart><c:plotArea><c:barChart><c:bad/></c:barChart>` + axes + `</c:plotArea></c:chart>`, false},
		{"zero-ser", `<c:chart><c:plotArea><c:barChart><c:axId val="1"/><c:axId val="2"/></c:barChart>` + axes + `</c:plotArea></c:chart>`, false},
		{"bar-missing-valAx", `<c:chart><c:plotArea><c:barChart>` + ser + `<c:axId val="1"/></c:barChart><c:catAx><c:axId val="1"/></c:catAx></c:plotArea></c:chart>`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, root := idx(tc.body)
			if got := IsCanonical(doc, root); got != tc.want {
				t.Fatalf("IsCanonical = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanonicalPredicatesMigrated(t *testing.T) {
	const ad = nsDrawingML
	mk := func(inner string) (*xmlstore.XMLDocument, *xmlstore.NodeRecord) {
		t.Helper()
		doc, err := xmlstore.Index([]byte(`<r xmlns:c="` + testChartNS + `" xmlns:a="` + ad + `">` + inner + `</r>`))
		if err != nil {
			t.Fatalf("Index: %v", err)
		}
		root := doc.Root()
		if len(root.Children) == 0 {
			t.Fatal("empty test element")
		}
		return doc, doc.Node(root.Children[0])
	}

	t.Run("canonicalSer", func(t *testing.T) {
		good := `<c:ser><c:idx/><c:order/><c:tx/><c:cat/><c:val/></c:ser>`
		doc, root := mk(good)
		if !CanonicalSer(doc, root, ChartBar) {
			t.Fatal("bar ser rejected")
		}
		doc, root = mk(`<c:ser><c:smooth/></c:ser>`)
		if !CanonicalSer(doc, root, ChartLine) {
			t.Fatal("line smooth rejected")
		}
		if CanonicalSer(doc, root, ChartBar) {
			t.Fatal("smooth allowed on bar")
		}
		doc, root = mk(`<c:ser><c:bad/></c:ser>`)
		if CanonicalSer(doc, root, ChartBar) {
			t.Fatal("unknown ser child allowed")
		}
		doc, root = mk(`<c:ser><x:x xmlns:x="urn:x"/></c:ser>`)
		if CanonicalSer(doc, root, ChartBar) {
			t.Fatal("non-chartML ser child allowed")
		}
		doc, root = mk(`<c:ser><c:trendline><c:bad/></c:trendline></c:ser>`)
		if CanonicalSer(doc, root, ChartBar) {
			t.Fatal("bad trendline accepted")
		}
		doc, root = mk(`<c:ser><c:errBars><c:bad/></c:errBars></c:ser>`)
		if CanonicalSer(doc, root, ChartBar) {
			t.Fatal("bad errBars accepted")
		}
	})

	t.Run("canonicalTrendlineAndErrBars", func(t *testing.T) {
		doc, root := mk(`<c:trendline><c:name/><c:trendlineType/><c:dispEq/></c:trendline>`)
		if !CanonicalTrendline(doc, root) {
			t.Fatal("good trendline rejected")
		}
		doc, root = mk(`<c:trendline><x:x xmlns:x="urn:x"/></c:trendline>`)
		if CanonicalTrendline(doc, root) {
			t.Fatal("non-chartML trendline accepted")
		}
		doc, root = mk(`<c:errBars><c:errDir/><c:errBarType/><c:noEndCap/></c:errBars>`)
		if !CanonicalErrBars(doc, root) {
			t.Fatal("good errBars rejected")
		}
		doc, root = mk(`<c:errBars><c:weird/></c:errBars>`)
		if CanonicalErrBars(doc, root) {
			t.Fatal("unknown errBars accepted")
		}
	})

	t.Run("canonicalTitleAndRichText", func(t *testing.T) {
		good := `<c:title><c:overlay/><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>x</a:t></a:r></a:p></c:rich></c:tx></c:title>`
		doc, root := mk(good)
		if !CanonicalTitleSubtree(doc, root) {
			t.Fatal("good title rejected")
		}
		doc, root = mk(`<c:title><c:bad/></c:title>`)
		if CanonicalTitleSubtree(doc, root) {
			t.Fatal("unknown title child accepted")
		}
		// rich 缺失 / 多子元素 / 非法富文本元素。
		doc, root = mk(`<c:tx/>`)
		if CanonicalRichText(doc, root) {
			t.Fatal("missing rich accepted")
		}
		doc, root = mk(`<c:tx><c:rich/><c:extra/></c:tx>`)
		if CanonicalRichText(doc, root) {
			t.Fatal("extra tx child accepted")
		}
		doc, root = mk(`<c:tx><c:rich><a:bad/></c:rich></c:tx>`)
		if CanonicalRichText(doc, root) {
			t.Fatal("bad rich element accepted")
		}
	})

	t.Run("canonicalAxExtensions", func(t *testing.T) {
		doc, root := mk(`<c:catAx><c:axId/><c:scaling><c:orientation/></c:scaling></c:catAx>`)
		if !CanonicalAxExtensions(doc, root, true) {
			t.Fatal("good axes rejected")
		}
		doc, root = mk(`<c:catAx><c:scaling><c:logBase/></c:scaling></c:catAx>`)
		if CanonicalAxExtensions(doc, root, false) {
			t.Fatal("logBase accepted when disallowed")
		}
		if !CanonicalAxExtensions(doc, root, true) {
			t.Fatal("logBase rejected when allowed")
		}
		doc, root = mk(`<c:catAx><c:scaling><x:x xmlns:x="urn:x"/></c:scaling></c:catAx>`)
		if CanonicalAxExtensions(doc, root, true) {
			t.Fatal("non-chartML scaling child accepted")
		}
		doc, root = mk(`<c:catAx><c:weird/></c:catAx>`)
		if CanonicalAxExtensions(doc, root, true) {
			t.Fatal("unknown axes child accepted")
		}
	})
}
