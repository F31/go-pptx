package chart

import (
	"strings"
	"testing"

	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件锁定 c:pt/@idx 的上界——不可信输入的内存放大防护。
//
// 缺陷原型：@idx 完全由文件内容决定，而回填需要 make([]string, max+1) 的稠密切片，
// 于是分配量与输入体积脱钩：idx=1e9 会让单个系列分配约 16 GB，idx=MaxInt64 会让
// max+1 溢出为负并触发运行时 makeslice panic。该路径经公共 API（ChartShape.Data()）
// 对不可信文件可达，因此这里把"越界点跳过 + 正常缺号仍补空串"固化为断言。

// cachePointsOf 用最小 chart XML 承载给定 c:pt 序列并调用 CachePoints。
func cachePointsOf(t *testing.T, body string) []string {
	t.Helper()
	xml := `<c:chartSpace xmlns:c="` + testChartNS + `"><c:chart><c:plotArea><c:barChart><c:ser><c:val><c:numLit>` +
		body + `</c:numLit></c:val></c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`
	doc, err := xmlstore.Index([]byte(xml))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	ids := doc.Elements(testChartNS, "numLit")
	if len(ids) != 1 {
		t.Fatalf("numLit nodes = %d, want 1", len(ids))
	}
	return CachePoints(doc, doc.Node(ids[0]))
}

func TestCachePointsIdxBounds(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			// 基线语义：缺号位补空串（不得因加上界而改变）。
			"gap-filled",
			`<c:pt idx="0"><c:v>A</c:v></c:pt><c:pt idx="2"><c:v>C</c:v></c:pt>`,
			[]string{"A", "", "C"},
		},
		{
			// idx=1e9：旧实现在此分配约 16 GB。
			"huge-idx-skipped",
			`<c:pt idx="0"><c:v>A</c:v></c:pt><c:pt idx="1000000000"><c:v>B</c:v></c:pt>`,
			[]string{"A"},
		},
		{
			// idx=MaxInt64：旧实现 max+1 溢出为负 → makeslice panic。
			"maxint64-idx-skipped",
			`<c:pt idx="0"><c:v>A</c:v></c:pt><c:pt idx="9223372036854775807"><c:v>B</c:v></c:pt>`,
			[]string{"A"},
		},
		{
			"negative-idx-skipped",
			`<c:pt idx="-1"><c:v>X</c:v></c:pt><c:pt idx="0"><c:v>A</c:v></c:pt>`,
			[]string{"A"},
		},
		{
			// 越界值非数字（Atoi 失败）时 idx 回退 0，不越界。
			"unparsable-idx-falls-back",
			`<c:pt idx="not-a-number"><c:v>A</c:v></c:pt>`,
			[]string{"A"},
		},
		{
			"empty-cache",
			``,
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cachePointsOf(t, tc.body)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("CachePoints = %v, want %v", got, tc.want)
			}
			// 上界硬约束：任何时候都不得超出 maxCachePoints（否则回到无界分配）。
			if len(got) > maxCachePoints {
				t.Fatalf("CachePoints len = %d exceeds bound %d", len(got), maxCachePoints)
			}
		})
	}
}

// TestSerValuesSkipsOutOfRangeIdx 确认上界在调用链上生效（SerValues 走 CachePoints）。
func TestSerValuesSkipsOutOfRangeIdx(t *testing.T) {
	xml := `<c:chartSpace xmlns:c="` + testChartNS + `"><c:chart><c:plotArea><c:barChart><c:ser>` +
		`<c:val><c:numLit>` +
		`<c:pt idx="0"><c:v>1.5</c:v></c:pt>` +
		`<c:pt idx="1000000000"><c:v>2.5</c:v></c:pt>` +
		`</c:numLit></c:val></c:ser></c:barChart></c:plotArea></c:chart></c:chartSpace>`
	doc, err := xmlstore.Index([]byte(xml))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	sers := doc.Elements(testChartNS, "ser")
	if len(sers) != 1 {
		t.Fatalf("ser elements = %d", len(sers))
	}
	got := SerValues(doc, doc.Node(sers[0]))
	if len(got) != 1 || got[0] != 1.5 {
		t.Fatalf("SerValues = %v, want [1.5]", got)
	}
}
