package ooxml

import (
	"github.com/F31/go-pptx/internal/ooxml/schema"
	"github.com/F31/go-pptx/internal/opc"
)

// 本文件是图表投影（CHART-01/CHART-02 R 档）：以 schema 只读投影
// 取代门面句柄读取（演进第 2 步长尾）。只读，不创建任何 Part/关系。

// ChartInfo 是图表投影的精简 DTO——仅含 IR build 所需的三个字段。
type ChartInfo struct {
	Type       string   // "bar" / "line" / "pie" / ""（未识别）
	Title      string   // 图表标题（c:title/c:tx/c:rich 下 a:t 拼接）
	Categories []string // 类别标签（首个含 cat cache 的系列）
}

// ChartPartOf 从 slide 关系流解析 r:id 指向的图表 Part（首个内部
// 目标关系）；无匹配返回 ("", false)。data 是关系流字节。
func ChartPartOf(data []byte, source opc.PartName, rid string) (opc.PartName, bool) {
	if len(data) == 0 || rid == "" {
		return "", false
	}
	set, err := opc.ParseRelationships(source, data)
	if err != nil {
		return "", false
	}
	for _, rel := range set.All() {
		if rel.ID == rid && rel.Mode == opc.TargetInternal {
			return rel.TargetPart, true
		}
	}
	return "", false
}

// ChartData 解码 chart Part 字节，返回图表类型/标题/类别。
// 无法解码或缺少 c:chart 时返回零值和错误（best-effort）。
func ChartData(data []byte) (ChartInfo, error) {
	if len(data) == 0 {
		return ChartInfo{}, nil
	}
	var cs schema.C_CT_ChartSpace
	if err := schema.Unmarshal(data, &cs); err != nil {
		return ChartInfo{}, err
	}
	if cs.Chart == nil {
		return ChartInfo{}, nil
	}
	info := ChartInfo{}
	info.Title = chartTitle(cs.Chart)
	if cs.Chart.PlotArea != nil {
		info.Type, info.Categories = chartPlotInfo(cs.Chart.PlotArea)
	}
	return info, nil
}

// chartTitle 从 c:chart/c:title/c:tx/c:rich 下递归收集 a:t 文本。
func chartTitle(ch *schema.C_CT_Chart) string {
	if ch == nil || ch.Title == nil || ch.Title.Tx == nil || ch.Title.Tx.Rich == nil {
		return ""
	}
	return aTText(ch.Title.Tx.Rich)
}

// aTText 递归收集 A_CT_TextBody 下所有 a:t 文本拼接。
func aTText(tb *schema.A_CT_TextBody) string {
	if tb == nil {
		return ""
	}
	var out string
	for _, p := range tb.P {
		for _, r := range p.R {
			out += string(r.T)
		}
	}
	return out
}

// chartPlotInfo 从 plotArea 中找首个受限图表组，返回类型和类别。
func chartPlotInfo(pa *schema.C_CT_PlotArea) (string, []string) {
	if pa == nil {
		return "", nil
	}
	// barChart
	for i := range pa.BarChart {
		if cats := plotFirstSerCat(&pa.BarChart[i]); len(cats) > 0 || i == 0 {
			return "bar", cats
		}
	}
	// lineChart
	for i := range pa.LineChart {
		if cats := lineSerCat(&pa.LineChart[i]); len(cats) > 0 || i == 0 {
			return "line", cats
		}
	}
	// pieChart
	for i := range pa.PieChart {
		if cats := pieSerCat(&pa.PieChart[i]); len(cats) > 0 || i == 0 {
			return "pie", cats
		}
	}
	return "", nil
}

// plotFirstSerCat 从 BarChart 首个系列提取类别。
func plotFirstSerCat(bc *schema.C_CT_BarChart) []string {
	if bc == nil || len(bc.Ser) == 0 {
		return nil
	}
	return axDataCatPoints(bc.Ser[0].Cat)
}

// lineSerCat 从 LineChart 首个系列提取类别。
func lineSerCat(lc *schema.C_CT_LineChart) []string {
	if lc == nil || len(lc.Ser) == 0 {
		return nil
	}
	return axDataCatPoints(lc.Ser[0].Cat)
}

// pieSerCat 从 PieChart 首个系列提取类别。
func pieSerCat(pc *schema.C_CT_PieChart) []string {
	if pc == nil || len(pc.Ser) == 0 {
		return nil
	}
	return axDataCatPoints(pc.Ser[0].Cat)
}

// axDataCatPoints 从 AxDataSource 提取类别缓存点值。
// 优先级与 internal/chart SerCategories 锁死：strRef/strCache → numRef/numCache → strLit → numLit。
func axDataCatPoints(cat *schema.C_CT_AxDataSource) []string {
	if cat == nil {
		return nil
	}
	// strRef/strCache
	if cat.StrRef != nil && cat.StrRef.StrCache != nil {
		return strDataPoints(cat.StrRef.StrCache)
	}
	// numRef/numCache
	if cat.NumRef != nil && cat.NumRef.NumCache != nil {
		return numDataPoints(cat.NumRef.NumCache)
	}
	// strLit
	if cat.StrLit != nil {
		return strDataPoints(cat.StrLit)
	}
	// numLit
	if cat.NumLit != nil {
		return numDataPoints(cat.NumLit)
	}
	return nil
}

// strDataPoints 从 StrData（strCache/strLit）提取 pt/v 文本。
func strDataPoints(d *schema.C_CT_StrData) []string {
	if d == nil {
		return nil
	}
	out := make([]string, 0, len(d.Pt))
	for _, pt := range d.Pt {
		out = append(out, string(pt.V))
	}
	return out
}

// numDataPoints 从 NumData（numCache/numLit）提取 pt/v 文本。
func numDataPoints(d *schema.C_CT_NumData) []string {
	if d == nil {
		return nil
	}
	out := make([]string, 0, len(d.Pt))
	for _, pt := range d.Pt {
		out = append(out, string(pt.V))
	}
	return out
}
