package chart

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// TestChartAxisUnreadFieldNames 验证 FEAT-002 项 3 降置信子项的核心检测逻辑：
// 源图表轴含本库未提取的字段（majorUnit/minorUnit/numFmt 等）时，正确返回
// 这些字段名（去重、字典序），且已提取字段（logBase/scaling/axPos 等）不计入。
func TestChartAxisUnreadFieldNames(t *testing.T) {
	doc := `<?xml version="1.0"?>
<c:chartSpace xmlns:c="` + nsChartML + `">
  <c:chart>
    <c:plotArea>
      <c:barChart><c:ser/></c:barChart>
      <c:catAx>
        <c:numFmt formatCode="0%"/>
        <c:axPos val="b"/>
      </c:catAx>
      <c:valAx>
        <c:majorUnit val="5"/>
        <c:minorUnit val="1"/>
        <c:logBase val="10"/>
        <c:scaling/>
        <c:axPos val="b"/>
        <c:tickLblPos val="nextTo"/>
        <c:majorGridlines/>
      </c:valAx>
    </c:plotArea>
  </c:chart>
</c:chartSpace>`
	d, err := xmlstore.Index([]byte(doc))
	if err != nil {
		t.Fatalf("index chart xml: %v", err)
	}
	root := d.Root()
	if root == nil {
		t.Fatal("nil root")
	}
	got := ChartAxisUnreadFieldNames(d, root)
	want := []string{"majorGridlines", "majorUnit", "minorUnit", "numFmt", "tickLblPos"}
	if len(got) != len(want) {
		t.Fatalf("ChartAxisUnreadFieldNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ChartAxisUnreadFieldNames = %v, want %v", got, want)
		}
	}
}

// TestChartAxisUnreadFieldNamesNone 验证规范布局（本库生成的图表）不含未提取
// 轴字段时返回 nil——降置信信号不应误报。
func TestChartAxisUnreadFieldNamesNone(t *testing.T) {
	cd := baseChartData(ChartBar)
	doc, err := BuildChartSpaceXML(cd, testSheetName, testCatAxID, testValAxID)
	if err != nil {
		t.Fatalf("build chart: %v", err)
	}
	d, err := xmlstore.Index([]byte(doc))
	if err != nil {
		t.Fatalf("index chart: %v", err)
	}
	root := d.Root()
	if root == nil {
		t.Fatal("nil root")
	}
	if got := ChartAxisUnreadFieldNames(d, root); got != nil {
		t.Fatalf("canonical chart should report no unread axis fields, got %v", got)
	}
}
