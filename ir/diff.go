package ir

import (
	"fmt"
	"sort"
	"strings"

	"github.com/F31/go-pptx"
)

// 本文件实现 DIFF-01 语义 diff 与审计报告（方案 §18.3 / §24）。
//
// 以 IR 为输入视图比较两份文档：先按签名做页面级 LCS 对齐（识别
// 新增/删除/移动），再在对齐页内按 ShapeID 配对比较形状字段（文本、
// 几何、类型、名称、替代文本、表格尺寸、图表类型、子形状递归）。
//
// 未识别/未投影区域一律标记为 **opaque diff**：报告给出 Part + NodePath
// 使其可回溯（§24 DIFF-01 验收"diff 报告含 opaque 区域且定位可回溯到
// Part/NodePath"），但**不猜测其内部变化**——只说明"已变更"与可确定
// 的投影字段差异。
//
// 语义 diff 不修改输入；报告是只读快照（schemaVersion 见
// SchemaVersionDiff，与 IR/SDK 版本独立管理）。

// SchemaVersionDiff 是语义 diff 报告的契约版本。
const SchemaVersionDiff = "go-pptx.diff/1.0"

// 变更类型（稳定字符串枚举，调用方可据此分支）。
const (
	DiffCoreChanged    = "document.core_changed"
	DiffPageAdded      = "page.added"
	DiffPageRemoved    = "page.removed"
	DiffPageMoved      = "page.moved"
	DiffPageNotes      = "page.notes_changed"
	DiffPageTiming     = "page.timing_changed"
	DiffShapeAdded     = "shape.added"
	DiffShapeRemoved   = "shape.removed"
	DiffShapeText      = "shape.text_changed"
	DiffShapeBounds    = "shape.bounds_changed"
	DiffShapeKind      = "shape.kind_changed"
	DiffShapeName      = "shape.name_changed"
	DiffShapeAltText   = "shape.alttext_changed"
	DiffShapeTableSize = "shape.table_size_changed"
	DiffShapeChartType = "shape.chart_type_changed"
	DiffOpaqueChanged  = "opaque.diff"
)

// opaqueKinds 是语义内容未（完整）投影到 IR 的形状类别：其差异只能
// 报告为 opaque diff（理由见 opaqueReason）。
var opaqueKinds = map[string]string{
	"opaque":  "形状类型未被本库识别（OpaqueShape），内部 XML 未投影",
	"picture": "图片像素内容不在 IR 中（仅投影几何/名称/替代文本）",
	"audio":   "音频媒体内容不在 IR 中",
	"video":   "视频媒体内容不在 IR 中",
	"chart":   "图表缓存数值不在 IR 中（仅投影图表类型）",
	"group":   "组内子形状已递归投影，组自身属性（如组变换）未逐项投影",
}

// DiffOptions 是 Diff 的选项（零值可直接用）。
type DiffOptions struct {
	// IgnoreGeometry 忽略位移/尺寸变化（只报内容差异）。
	IgnoreGeometry bool
	// IgnoreWhitespace 比较文本前做空白归一化（TrimSpace + 连续空白折叠）。
	IgnoreWhitespace bool
	// IgnoreNotes 忽略备注文本差异。
	IgnoreNotes bool
	// MaxEntries 限制报告条目数（0 表示不限）；超出后停止追加但继续
	// 统计——报告以 Truncated=true 标明。
	MaxEntries int
}

// DiffOption 是 Diff 的函数式选项。
type DiffOption func(*DiffOptions)

// WithIgnoreGeometry 忽略几何变化（内容 diff 场景）。
func WithIgnoreGeometry(v bool) DiffOption { return func(o *DiffOptions) { o.IgnoreGeometry = v } }

// WithIgnoreWhitespace 比较文本前归一化空白。
func WithIgnoreWhitespace(v bool) DiffOption { return func(o *DiffOptions) { o.IgnoreWhitespace = v } }

// WithIgnoreNotes 忽略备注文本差异。
func WithIgnoreNotes(v bool) DiffOption { return func(o *DiffOptions) { o.IgnoreNotes = v } }

// WithMaxEntries 限制报告条目数（0 表示不限）。
func WithMaxEntries(n int) DiffOption { return func(o *DiffOptions) { o.MaxEntries = n } }

// SourceRef 描述 diff 的一侧来源。
type SourceRef struct {
	Path          string `json:"path,omitempty"`
	DocumentID    string `json:"documentID,omitempty"`
	Pages         int    `json:"pages"`
	SchemaVersion string `json:"schemaVersion,omitempty"`
	SDKVersion    string `json:"sdkVersion,omitempty"`
}

// DiffStats 是 diff 的计数汇总（与 Entries 一致，便于调用方只看数字）。
type DiffStats struct {
	PagesAdded    int `json:"pagesAdded"`
	PagesRemoved  int `json:"pagesRemoved"`
	PagesMoved    int `json:"pagesMoved"`
	PagesChanged  int `json:"pagesChanged"`
	ShapesAdded   int `json:"shapesAdded"`
	ShapesRemoved int `json:"shapesRemoved"`
	ShapesChanged int `json:"shapesChanged"`
	TextChanges   int `json:"textChanges"`
	BoundsChanges int `json:"boundsChanges"`
	NotesChanges  int `json:"notesChanges"`
	TimingChanges int `json:"timingChanges"`
	OpaqueRegions int `json:"opaqueRegions"`
}

// DiffEntry 是一条语义变更，定位可回溯到 Part/NodePath。
type DiffEntry struct {
	Kind       string       `json:"kind"`
	PageIndexA int          `json:"pageIndexA,omitempty"`
	PageIndexB int          `json:"pageIndexB,omitempty"`
	SlideID    pptx.SlideID `json:"slideID,omitempty"`
	ShapeID    pptx.ShapeID `json:"shapeID,omitempty"`
	ShapeName  string       `json:"shapeName,omitempty"`
	Part       string       `json:"part,omitempty"`
	NodePath   string       `json:"nodePath,omitempty"`
	Field      string       `json:"field,omitempty"`
	From       string       `json:"from,omitempty"`
	To         string       `json:"to,omitempty"`
	Detail     string       `json:"detail,omitempty"`
}

// OpaqueRegion 是一处未识别/未投影区域（§24 DIFF-01 验收项）。
type OpaqueRegion struct {
	Part     string `json:"part"`
	NodePath string `json:"nodePath,omitempty"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail,omitempty"`
	Changed  bool   `json:"changed"`
}

// DiffReport 是两份文档的语义差异报告（只读快照）。
type DiffReport struct {
	SchemaVersion string         `json:"schemaVersion"`
	A             SourceRef      `json:"a"`
	B             SourceRef      `json:"b"`
	Changed       bool           `json:"changed"`
	Truncated     bool           `json:"truncated,omitempty"`
	Stats         DiffStats      `json:"stats"`
	Entries       []DiffEntry    `json:"entries,omitempty"`
	Opaque        []OpaqueRegion `json:"opaque,omitempty"`
	Diagnostics   Diagnostics    `json:"diagnostics,omitempty"`
}

// Diff 比较两份 IR，返回语义差异报告（DIFF-01）。
//
// a 或 b 为 nil 时视为空文档（仍产出 added/removed 条目），不返回错误
// ——审计场景允许与空基线比较。
func Diff(a, b *Document, opts ...DiffOption) DiffReport {
	o := DiffOptions{}
	for _, fn := range opts {
		fn(&o)
	}
	d := &differ{o: o, rep: DiffReport{SchemaVersion: SchemaVersionDiff}}
	if a != nil {
		d.rep.A = sourceRef(a)
	}
	if b != nil {
		d.rep.B = sourceRef(b)
	}
	pagesA, pagesB := []Page{}, []Page{}
	if a != nil {
		pagesA = a.Pages
	}
	if b != nil {
		pagesB = b.Pages
	}
	if a != nil && b != nil {
		d.diffCore(a.Core, b.Core)
	}
	d.diffPages(pagesA, pagesB)
	d.rep.Changed = len(d.rep.Entries) > 0
	d.rep.Stats.OpaqueRegions = len(d.rep.Opaque)
	return d.rep
}

// ---------- 内部：differ ----------

type differ struct {
	o   DiffOptions
	rep DiffReport
}

func sourceRef(d *Document) SourceRef {
	return SourceRef{
		DocumentID:    d.DocumentID,
		Pages:         len(d.Pages),
		SchemaVersion: d.SchemaVersion,
		SDKVersion:    d.SDKVersion,
	}
}

// add 追加一条变更（受 MaxEntries 限制）。
func (d *differ) add(e DiffEntry) {
	if d.o.MaxEntries > 0 && len(d.rep.Entries) >= d.o.MaxEntries {
		d.rep.Truncated = true
		return
	}
	d.rep.Entries = append(d.rep.Entries, e)
}

func (d *differ) opaque(r OpaqueRegion) {
	d.rep.Opaque = append(d.rep.Opaque, r)
}

// diffCore 比较核心属性（标题/主题/作者/关键词/描述）。
func (d *differ) diffCore(a, b *Core) {
	if a == nil && b == nil {
		return
	}
	ca, cb := Core{}, Core{}
	if a != nil {
		ca = *a
	}
	if b != nil {
		cb = *b
	}
	fields := []struct{ name, x, y string }{
		{"title", ca.Title, cb.Title},
		{"subject", ca.Subject, cb.Subject},
		{"creator", ca.Creator, cb.Creator},
		{"keywords", ca.Keywords, cb.Keywords},
		{"description", ca.Description, cb.Description},
	}
	for _, f := range fields {
		if f.x != f.y {
			d.add(DiffEntry{Kind: DiffCoreChanged, Field: f.name, From: f.x, To: f.y})
		}
	}
}

// diffPages 页面级对齐（加权 LCS）与逐页比较。
//
// 对齐不以内容相同为前提——否则"改了一个字"的页面会被判成删页+加页。
// 评分规则：SlideID 相同为强匹配（1.0）；SlideID 不同但都非零则判定为
// 不同页（0，不参与对齐）；其余按形状 ID 集合的 Jaccard 相似度
// （≥ matchThreshold 视为同一页的两次修订）。
func (d *differ) diffPages(a, b []Page) {
	pairs := alignPages(a, b)
	matchedA := map[int]bool{}
	matchedB := map[int]bool{}
	type idxPair struct{ i, j int }
	var aligned []idxPair
	for _, p := range pairs {
		matchedA[p.i], matchedB[p.j] = true, true
		aligned = append(aligned, idxPair{p.i, p.j})
	}
	// 二次匹配：顺序 LCS 无法表达的乱序残留（如两页互换位置）。
	// 以 SlideID 相等（1.0）或高相似度（≥ orphanThreshold）配对，
	// 不受顺序约束——配对结果仍逐页比较并标为移动。
	for i := range a {
		if matchedA[i] {
			continue
		}
		bestJ, bestScore := -1, 0.0
		for j := range b {
			if matchedB[j] {
				continue
			}
			if sc := pageScore(a[i], b[j]); sc >= orphanThreshold && sc > bestScore {
				bestJ, bestScore = j, sc
			}
		}
		if bestJ >= 0 {
			matchedA[i], matchedB[bestJ] = true, true
			aligned = append(aligned, idxPair{i, bestJ})
		}
	}
	sort.SliceStable(aligned, func(x, y int) bool { return aligned[x].i < aligned[y].i })
	for i := range a {
		if matchedA[i] {
			continue
		}
		d.rep.Stats.PagesRemoved++
		d.add(DiffEntry{
			Kind: DiffPageRemoved, PageIndexA: i, SlideID: a[i].SlideID,
			Part: a[i].Part, Detail: fmt.Sprintf("页内含 %d 个形状", len(a[i].Shapes)),
		})
	}
	for j := range b {
		if matchedB[j] {
			continue
		}
		d.rep.Stats.PagesAdded++
		d.add(DiffEntry{
			Kind: DiffPageAdded, PageIndexB: j, SlideID: b[j].SlideID,
			Part: b[j].Part, Detail: fmt.Sprintf("页内含 %d 个形状", len(b[j].Shapes)),
		})
	}
	for _, p := range aligned {
		d.diffPage(a[p.i], b[p.j])
	}
}

// diffPage 比较一对对齐页。
func (d *differ) diffPage(a, b Page) {
	before := len(d.rep.Entries)
	if a.Index != b.Index {
		d.rep.Stats.PagesMoved++
		d.add(DiffEntry{
			Kind: DiffPageMoved, PageIndexA: a.Index, PageIndexB: b.Index,
			SlideID: b.SlideID, Part: b.Part,
			Detail: fmt.Sprintf("页面位置 %d → %d", a.Index, b.Index),
		})
	}
	if !d.o.IgnoreNotes && a.NotesText != b.NotesText {
		d.rep.Stats.NotesChanges++
		d.add(DiffEntry{
			Kind: DiffPageNotes, PageIndexA: a.Index, PageIndexB: b.Index,
			SlideID: b.SlideID, Part: b.Part,
			From: a.NotesText, To: b.NotesText,
		})
	}
	if timingSignature(a) != timingSignature(b) {
		d.rep.Stats.TimingChanges++
		d.add(DiffEntry{
			Kind: DiffPageTiming, PageIndexA: a.Index, PageIndexB: b.Index,
			SlideID: b.SlideID, Part: b.Part,
			From: timingSignature(a), To: timingSignature(b),
			Detail: "动画时序 IR 摘要（节点数/估计数/媒体节点/opaque 数）",
		})
	}
	d.diffShapes(a, b, a.Shapes, b.Shapes)
	if len(d.rep.Entries) > before {
		d.rep.Stats.PagesChanged++
	}
}

// diffShapes 按 ShapeID 配对比较形状（含组内子形状递归）。
func (d *differ) diffShapes(pa, pb Page, a, b []Shape) {
	usedA := make([]bool, len(a))
	usedB := make([]bool, len(b))
	for j := range b {
		if i, ok := indexOfShape(a, b[j].ID); ok && !usedA[i] {
			usedA[i], usedB[j] = true, true
			d.diffShape(pa, pb, a[i], b[j])
		}
	}
	for i := range a {
		if usedA[i] {
			continue
		}
		d.rep.Stats.ShapesRemoved++
		d.add(DiffEntry{
			Kind: DiffShapeRemoved, PageIndexA: pa.Index, PageIndexB: pb.Index,
			SlideID: pa.SlideID, ShapeID: a[i].ID, ShapeName: a[i].Name,
			Part: pa.Part, NodePath: a[i].NodePath, Detail: a[i].Kind,
		})
	}
	for j := range b {
		if usedB[j] {
			continue
		}
		d.rep.Stats.ShapesAdded++
		d.add(DiffEntry{
			Kind: DiffShapeAdded, PageIndexA: pa.Index, PageIndexB: pb.Index,
			SlideID: pb.SlideID, ShapeID: b[j].ID, ShapeName: b[j].Name,
			Part: pb.Part, NodePath: b[j].NodePath, Detail: b[j].Kind,
		})
	}
}

// indexOfShape 返回首个匹配 ID 的形状下标。
func indexOfShape(s []Shape, id pptx.ShapeID) (int, bool) {
	for i := range s {
		if s[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

// diffShape 比较一对已配对形状的各个投影字段。
func (d *differ) diffShape(pa, pb Page, a, b Shape) {
	loc := DiffEntry{
		PageIndexA: pa.Index, PageIndexB: pb.Index, SlideID: pb.SlideID,
		ShapeID: b.ID, ShapeName: b.Name, Part: pb.Part, NodePath: b.NodePath,
	}
	before := len(d.rep.Entries)
	emit := func(kind, field, from, to string) {
		e := loc
		e.Kind, e.Field, e.From, e.To = kind, field, from, to
		d.add(e)
	}
	if a.Kind != b.Kind {
		emit(DiffShapeKind, "kind", a.Kind, b.Kind)
	}
	if a.Name != b.Name {
		emit(DiffShapeName, "name", a.Name, b.Name)
	}
	if a.AltText != b.AltText {
		emit(DiffShapeAltText, "altText", a.AltText, b.AltText)
	}
	ta, tb := d.norm(a.Text), d.norm(b.Text)
	if ta != tb {
		d.rep.Stats.TextChanges++
		emit(DiffShapeText, "text", a.Text, b.Text)
	}
	if !d.o.IgnoreGeometry && !boxEqual(a.Bounds, b.Bounds) {
		d.rep.Stats.BoundsChanges++
		emit(DiffShapeBounds, "bounds", boxString(a.Bounds), boxString(b.Bounds))
	}
	if a.TableRows != b.TableRows || a.TableCols != b.TableCols {
		emit(DiffShapeTableSize, "tableSize",
			fmt.Sprintf("%dx%d", a.TableRows, a.TableCols),
			fmt.Sprintf("%dx%d", b.TableRows, b.TableCols))
	}
	if a.ChartType != b.ChartType {
		emit(DiffShapeChartType, "chartType", a.ChartType, b.ChartType)
	}
	if len(a.Children) > 0 || len(b.Children) > 0 {
		d.diffShapes(pa, pb, a.Children, b.Children)
	}
	// opaque 覆盖：内容未（完整）投影的形状，任何差异只报"已变更"。
	if reason, ok := opaqueKinds[a.Kind]; ok {
		d.opaque(OpaqueRegion{
			Part: pb.Part, NodePath: b.NodePath, Reason: reason,
			Changed: len(d.rep.Entries) > before,
			Detail:  fmt.Sprintf("kind=%s", a.Kind),
		})
	}
	if len(d.rep.Entries) > before {
		d.rep.Stats.ShapesChanged++
		if reason, ok := opaqueKinds[a.Kind]; ok {
			e := loc
			e.Kind = DiffOpaqueChanged
			e.Detail = reason + "：已变更，但内部差异无法从 IR 判定"
			d.add(e)
		}
	}
}

// norm 按选项归一化文本。
func (d *differ) norm(s string) string {
	if !d.o.IgnoreWhitespace {
		return s
	}
	return strings.Join(strings.Fields(s), " ")
}

// ---------- 内部：签名与对齐 ----------

// lcsPair 是一对对齐下标（A 侧 i ↔ B 侧 j）。
type lcsPair struct{ i, j int }

// matchThreshold 是判定"同一页的两次修订"的最低相似度。
const matchThreshold = 0.5

// orphanThreshold 是顺序对齐后残留页（疑似移动/重排）的配对阈值：
// 明显高于 matchThreshold，避免把新增页与删除页误配成对。
const orphanThreshold = 0.8

// timingSignature 计算动画时序摘要（无时序为空串）。
func timingSignature(p Page) string {
	if !p.HasTiming || p.Timing == nil {
		return ""
	}
	t := p.Timing
	return fmt.Sprintf("nodes=%d,est=%d,opaque=%d,audio=%d,video=%d,effect=%d",
		t.Totals.NodeCount, t.Totals.EstimatedCount, t.Totals.OpaqueCount,
		t.Totals.AudioNodes, t.Totals.VideoNodes, t.Totals.EffectNodes)
}

// shapeIDSet 收集页内形状 ID（含组内子形状）。
func shapeIDSet(shapes []Shape) map[pptx.ShapeID]bool {
	out := map[pptx.ShapeID]bool{}
	var walk func([]Shape)
	walk = func(list []Shape) {
		for _, s := range list {
			out[s.ID] = true
			if len(s.Children) > 0 {
				walk(s.Children)
			}
		}
	}
	walk(shapes)
	return out
}

// pageScore 评估两页是否为"同一页的两次修订"（0 表示不是）。
func pageScore(a, b Page) float64 {
	if a.SlideID != 0 && b.SlideID != 0 {
		if a.SlideID == b.SlideID {
			return 1
		}
		return 0 // 明确的页面标识不同：视为不同页
	}
	sa, sb := shapeIDSet(a.Shapes), shapeIDSet(b.Shapes)
	if len(sa) == 0 && len(sb) == 0 {
		return 1
	}
	inter := 0
	for id := range sa {
		if sb[id] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// alignPages 以加权 LCS 对齐两串页面（顺序敏感），返回配对下标。
//
// 复杂度为页面数乘积：超过 alignMaxCells 时退化为"同 SlideID 优先、
// 其余按下标"的线性对齐，避免超大文档上的内存与耗时失控。
func alignPages(a, b []Page) []lcsPair {
	const alignMaxCells = 1_000_000
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	if len(a)*len(b) > alignMaxCells {
		return alignPagesLinear(a, b)
	}
	dp := make([][]float64, len(a)+1)
	for i := range dp {
		dp[i] = make([]float64, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			best := dp[i+1][j]
			if dp[i][j+1] > best {
				best = dp[i][j+1]
			}
			if sc := pageScore(a[i], b[j]); sc >= matchThreshold {
				if v := sc + dp[i+1][j+1]; v > best {
					best = v
				}
			}
			dp[i][j] = best
		}
	}
	out := []lcsPair{}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		sc := pageScore(a[i], b[j])
		if sc >= matchThreshold && sc+dp[i+1][j+1] >= dp[i][j]-1e-9 {
			out = append(out, lcsPair{i, j})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return out
}

// alignPagesLinear 是超大文档的退化对齐：相同 SlideID 优先配对，其余
// 按下标顺序配对（文档齐全时等价于常规对齐）。
func alignPagesLinear(a, b []Page) []lcsPair {
	out := []lcsPair{}
	usedB := make([]bool, len(b))
	for i := range a {
		for j := range b {
			if usedB[j] {
				continue
			}
			if a[i].SlideID != 0 && a[i].SlideID == b[j].SlideID {
				out = append(out, lcsPair{i, j})
				usedB[j] = true
				break
			}
		}
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for k := 0; k < n; k++ {
		if usedB[k] {
			continue
		}
		paired := false
		for _, p := range out {
			if p.i == k {
				paired = true
				break
			}
		}
		if paired {
			continue
		}
		if pageScore(a[k], b[k]) >= matchThreshold {
			out = append(out, lcsPair{k, k})
			usedB[k] = true
		}
	}
	sort.SliceStable(out, func(x, y int) bool { return out[x].i < out[y].i })
	return out
}

// ---------- 内部：值比较 ----------

func boxEqual(a, b *Box) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func boxString(b *Box) string {
	if b == nil {
		return ""
	}
	return fmt.Sprintf("(%d,%d %dx%d)", b.X, b.Y, b.Width, b.Height)
}
