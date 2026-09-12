package chart

import (
	"strconv"
	"strings"
)

// ChartNumber 输出数值的规范十进制文本（供图表缓存与工作簿共用，
// 保证两侧字节一致；非有限值由上游校验拒绝）。
//
// 与 chartbook.go 历史行为一致：`strconv.FormatFloat(v, 'g', -1, 64)`
// ——'g' 精度 + -1 = 自动选最短能往返表示 + 去除尾随零。
// 例如 12.34 → "12.34"（不是 "12.3"），0 → "0"，-3.14 → "-3.14"。
func ChartNumber(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// WorkbookColumn 把 1-based 列号映射为 OOExcel 列字母（A, B, ..., Z, AA, ...）。
//
// 与 chartbook.go 历史行为一致：纯字符串映射，无 0 列支持（调用方负责 +2 偏移）。
func WorkbookColumn(n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	for n > 0 {
		n--
		b.WriteByte(byte('A' + n%26))
		n /= 26
	}
	out := b.String()
	// reverse
	r := []byte(out)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// BuildChartFrameFragment 构造 p:graphicFrame XML 片段，引用 chart Part。
//
// 参数 id 是 p:cNvPr@id（按规范布局生成）；x/y/cx/cy 是 a:off/a:ext 坐标；
// rid 是 p:graphicFrame 到 chart Part 的关系 ID。
func BuildChartFrameFragment(id, x, y, cx, cy int64, rid string) string {
	return `<p:graphicFrame>` +
		`<p:nvGraphicFramePr><p:cNvPr id="` + strconv.FormatInt(id, 10) + `" name="Chart ` +
		strconv.FormatInt(id, 10) + `"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="` + strconv.FormatInt(x, 10) + `" y="` + strconv.FormatInt(y, 10) +
		`"/><a:ext cx="` + strconv.FormatInt(cx, 10) + `" cy="` + strconv.FormatInt(cy, 10) + `"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="` + GraphicURI + `">` +
		`<c:chart xmlns:c="` + GraphicURI + `" r:id="` + rid + `"/>` +
		`</a:graphicData></a:graphic>` +
		`</p:graphicFrame>`
}
