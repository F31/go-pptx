// Package ir 提供 PPTX 的可选只读中间表示（方案 §18.3）。
//
// 关键约束：
//   - IR 是只读快照，不可作为 PPTX → PPTX 默认覆盖保存的输入；未来导入
//     只通过明确的有损新建接口（§18.3 顶部）；
//   - IR 不嵌入媒体二进制；仅通过 Part 名 + Content Type 引用（v1）；
//   - IR 的 schemaVersion 与 SDK 版本独立管理（SchemaVersion 常量），后续
//     兼容性按 schemaVersion 而非 go-pptx 版本号判定；
//   - 核心包（github.com/F31/go-pptx）不得导入本包；本包仅依赖公共 pptx
//     与标准库。
//
// 典型用途：内容提取、调试、预览输入、跨格式适配（§18.3）、两份文档
// 语义 diff（DIFF-01）的输入视图；动画时序投影属 TIMIR-01（M7）。
//
// v1 范围（TOOL-01）：
//   - 文档元数据摘要（core.xml）；
//   - 页面顺序、SlideID、名称、备注文本；
//   - 顶层形状：ID/Name/Kind/AltText/Decorative/Bounds/WorldBox；
//   - ShapeChart 类型与首组 Categories（ChartShape.Data）；
//   - ShapeTable 行/列/纯文本拼接（Cell.TextFrame → Paragraph.Text）；
//   - ShapeTextBox/ShapeAutoShape 文本提取（AutoShape.TextFrame →
//     Paragraph.Text）；
//   - 媒体 Part 名 + ContentType 列表（不嵌入二进制）。
//
// 暂未覆盖（本版本）：形状矩阵变换详情、动画时序投影（TIMIR-01）、
// 主题/版式/母版内容、嵌入对象的解析、占位符索引。
package ir

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/F31/go-pptx"
)

// SchemaVersion 是 IR 的当前 schema 版本号。本字段独立于 SDK 版本演进。
// 新增可选字段必须保持向后兼容；语义变化（删除/重命名/类型变更）必须
// 递增 schemaVersion，并保留旧 SchemaVersion 的版本入口。
const SchemaVersion = "go-pptx.ir/1.0"

// SDKVersion 是构建本库时的语义版本号（独立于 SchemaVersion）。
// 默认空串；发布构建可由 CI 注入。
var SDKVersion = ""

// SeverityString 是 Diagnostic.Severity 的字符串值，与 pptx.Severity
// 在序列化层对齐（"info"/"warning"/"error"）。
type SeverityString string

const (
	SevInfo    SeverityString = "info"
	SevWarning SeverityString = "warning"
	SevError   SeverityString = "error"
)

// Diagnostics 是导出过程中的非阻断诊断。
type Diagnostics []Diagnostic

// Diagnostic 单条诊断项。
type Diagnostic struct {
	Code     string         `json:"code"`
	Severity SeverityString `json:"severity"`
	Part     string         `json:"part,omitempty"`
	Message  string         `json:"message"`
}

// Document 是 IR 顶层对象。
type Document struct {
	SchemaVersion string      `json:"schemaVersion"`
	SDKVersion    string      `json:"sdkVersion,omitempty"`
	DocumentID    string      `json:"documentID,omitempty"`
	Core          *Core       `json:"core,omitempty"`
	Pages         []Page      `json:"pages"`
	Diagnostics   Diagnostics `json:"diagnostics,omitempty"`
}

// Core 是文档核心属性（§20.3 docProps）摘要。
type Core struct {
	Title       string     `json:"title,omitempty"`
	Subject     string     `json:"subject,omitempty"`
	Creator     string     `json:"creator,omitempty"`
	Keywords    string     `json:"keywords,omitempty"`
	Description string     `json:"description,omitempty"`
	Created     *time.Time `json:"created,omitempty"`
	Modified    *time.Time `json:"modified,omitempty"`
}

// Page 是单页 IR 视图。
type Page struct {
	Index     int          `json:"index"`
	SlideID   pptx.SlideID `json:"slideID"`
	Part      string       `json:"part"`
	Name      string       `json:"name,omitempty"`
	Shapes    []Shape      `json:"shapes"`
	NotesText string       `json:"notesText,omitempty"`
	HasTiming bool         `json:"hasTiming,omitempty"`
	// Timing 是 PageTiming 投影（TIMIR-01）。当 IncludeTimingIR=false 或
	// 页无 timing 时为空。
	Timing      *PageTiming `json:"timing,omitempty"`
	Diagnostics Diagnostics `json:"diagnostics,omitempty"`
}

// Shape 是单个形状的只读摘要。
type Shape struct {
	ID   pptx.ShapeID `json:"id"`
	Name string       `json:"name,omitempty"`
	Kind string       `json:"kind"`
	// NodePath 是形状在所属 Part 内的元素路径（形如
	// p:sld/p:cSld/p:spTree/p:sp[2]）——DIFF-01 报告据此回溯到
	// Part/NodePath（设计 §18.3）。句柄失效时为空。
	NodePath   string  `json:"nodePath,omitempty"`
	AltText    string  `json:"altText,omitempty"`
	Decorative bool    `json:"decorative,omitempty"`
	Bounds     *Box    `json:"bounds,omitempty"`
	WorldBox   *Box    `json:"worldBox,omitempty"`
	Text       string  `json:"text,omitempty"`
	Children   []Shape `json:"children,omitempty"`
	// Chart 专属字段（ShapeChart）。
	ChartType string `json:"chartType,omitempty"`
	// Table 专属字段（ShapeTable）。
	TableRows int `json:"tableRows,omitempty"`
	TableCols int `json:"tableCols,omitempty"`
}

// Box 是轴对齐矩形（EMU）。
type Box struct {
	X      int64 `json:"x"`
	Y      int64 `json:"y"`
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

// Options 控制 FromPresentation 的行为。
type Options struct {
	// IncludeNotes 收集 SpeakerNotesText（默认 true）。
	IncludeNotes bool
	// IncludeTimingNode 仅在 HasTiming=true 时置位；本页是否含任何 timing
	// 子树（不做投影，TIMIR-01）。
	IncludeTimingNode bool
	// IncludeTimingIR 控制是否把 p:timing 投影为 PageTiming（TIMIR-01）。
	IncludeTimingIR bool
}

// DefaultOptions 默认 IR 投影参数。
func DefaultOptions() Options {
	return Options{
		IncludeNotes:      true,
		IncludeTimingNode: true,
		IncludeTimingIR:   true,
	}
}

// FromPresentation 把当前 Presentation 投影为 IR（§18.3）。
//
// 行为约定：
//   - 失败发生时，已收集的部分内容会被丢弃并返回错误；调用方可多次
//     重试（IR 计算是读取快照，下一次从最新 revision 重建）；
//   - v1 仅覆盖方案 §18.3 的最小集。
func FromPresentation(p *pptx.Presentation, opts Options) (*Document, error) {
	if p == nil {
		return nil, fmt.Errorf("ir: nil presentation")
	}
	doc := &Document{
		SchemaVersion: SchemaVersion,
		SDKVersion:    SDKVersion,
	}
	doc.DocumentID = documentFingerprint(p)

	core, diags := projectCore(p)
	doc.Core = core
	doc.Diagnostics = append(doc.Diagnostics, diags...)

	slides, err := p.Slides()
	if err != nil {
		return nil, fmt.Errorf("ir: list slides: %w", err)
	}
	doc.Pages = make([]Page, 0, len(slides))
	for i, s := range slides {
		page, err := projectPage(p, s, i, opts)
		if err != nil {
			return nil, fmt.Errorf("ir: project page %d: %w", i, err)
		}
		doc.Pages = append(doc.Pages, page)
		if len(page.Diagnostics) > 0 {
			doc.Diagnostics = append(doc.Diagnostics, page.Diagnostics...)
		}
	}

	// 稳定顺序：诊断按 (Severity, Code, Part) 排序，便于 diff 视图。
	sortDiags(doc.Diagnostics)
	return doc, nil
}

// Marshal 把 IR 写为 JSON。序列化失败直接返回错误。
func (d *Document) Marshal() ([]byte, error) {
	return json.Marshal(d)
}

// Unmarshal 把 JSON 解析回 IR。附带 schemaVersion 校验。
func Unmarshal(data []byte) (*Document, error) {
	var d Document
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("ir: unmarshal: %w", err)
	}
	if d.SchemaVersion == "" {
		return nil, fmt.Errorf("ir: schemaVersion missing")
	}
	if d.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("ir: schemaVersion %q not supported (current %q)",
			d.SchemaVersion, SchemaVersion)
	}
	return &d, nil
}

// ---------- 投影实现（包内未导出） ----------

func projectCore(p *pptx.Presentation) (*Core, Diagnostics) {
	props, err := p.CoreProperties()
	if err != nil {
		return nil, Diagnostics{{
			Code: "IR_CORE_READ", Severity: SevWarning, Message: err.Error(),
		}}
	}
	c := &Core{
		Title:       optionalString(props.Title),
		Subject:     optionalString(props.Subject),
		Creator:     optionalString(props.Author),
		Keywords:    optionalString(props.Keywords),
		Description: optionalString(props.Comments),
	}
	if props.Created.Set && !props.Created.Value.IsZero() {
		t := props.Created.Value.UTC()
		c.Created = &t
	}
	if props.Modified.Set && !props.Modified.Value.IsZero() {
		t := props.Modified.Value.UTC()
		c.Modified = &t
	}
	return c, nil
}

// optionalString 把 Optional[string] 转为字符串：未设置返回 ""。
func optionalString(o pptx.Optional[string]) string {
	if !o.Set {
		return ""
	}
	return o.Value
}

func projectPage(p *pptx.Presentation, s *pptx.Slide, idx int, opts Options) (Page, error) {
	page := Page{
		Index:   idx,
		SlideID: s.ID(),
		Part:    s.PartName(),
		Name:    s.Name(),
	}
	shapes, diags := projectShapes(p, s)
	page.Shapes = shapes
	page.Diagnostics = append(page.Diagnostics, diags...)
	if opts.IncludeNotes {
		if notes, err := s.SpeakerNotesText(); err == nil {
			page.NotesText = notes
		} else {
			page.Diagnostics = append(page.Diagnostics, Diagnostic{
				Code: "IR_NOTES_READ", Severity: SevWarning,
				Part: s.PartName(), Message: err.Error(),
			})
		}
	}
	if opts.IncludeTimingNode {
		page.HasTiming = s.HasTiming()
	}
	if opts.IncludeTimingIR && page.HasTiming {
		raw, _, err := s.TimingTreeRaw()
		if err != nil {
			page.Diagnostics = append(page.Diagnostics, Diagnostic{
				Code: "IR_TIMING_READ", Severity: SevWarning,
				Part:    s.PartName(),
				Message: err.Error(),
			})
		} else if len(raw) > 0 {
			pt, perr := projectTimingTree(s.PartName(), raw)
			if perr != nil {
				page.Diagnostics = append(page.Diagnostics, pt.Diagnostics...)
				// Parse 失败但仍能透出诊断；不让 IR 整体失败。
			} else {
				page.Timing = &pt
				if len(pt.Diagnostics) > 0 {
					page.Diagnostics = append(page.Diagnostics, pt.Diagnostics...)
				}
			}
		}
	}
	return page, nil
}

func projectShapes(p *pptx.Presentation, s *pptx.Slide) ([]Shape, Diagnostics) {
	shapes, err := s.Shapes()
	if err != nil {
		return nil, Diagnostics{{
			Code: "IR_SHAPES_READ", Severity: SevWarning,
			Part: s.PartName(), Message: err.Error(),
		}}
	}
	out := make([]Shape, 0, len(shapes))
	for _, sh := range shapes {
		// 递归处理组合（GroupShape）：GroupShape 直接走 Children()。
		if gs, ok := sh.(*pptx.GroupShape); ok && gs != nil {
			children, err := gs.Children()
			if err == nil {
				for _, c := range children {
					out = append(out, projectShape(p, s, c))
				}
			} else {
				out = append(out, projectShape(p, s, gs))
			}
			continue
		}
		out = append(out, projectShape(p, s, sh))
	}
	return out, nil
}

// projectShape 把单个形状按类型分发投影。
//
// 文本框/自选图形走 Paragraph；表格走 Cell.TextFrame；图表走
// ChartShape.Data；图片仅记录元信息。
func projectShape(p *pptx.Presentation, s *pptx.Slide, sh pptx.Shape) Shape {
	s2 := shapeSummary(sh)
	switch sh.Kind() {
	case pptx.ShapeTable:
		if ts, ok := sh.(*pptx.TableShape); ok && ts != nil {
			rows, _ := ts.RowCount()
			cols, _ := ts.ColumnCount()
			s2.TableRows = rows
			s2.TableCols = cols
			s2.Text = readTableText(ts, rows, cols)
		}
	case pptx.ShapeChart:
		if cs, ok := sh.(*pptx.ChartShape); ok && cs != nil {
			if data, err := cs.Data(); err == nil {
				s2.ChartType = data.Type.String()
				var sb strings.Builder
				if data.Title != "" {
					sb.WriteString(data.Title)
					sb.WriteString(": ")
				}
				for i, c := range data.Categories {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(c)
				}
				s2.Text = sb.String()
			}
		}
	case pptx.ShapeTextBox, pptx.ShapeAutoShape:
		if tf := shapeTextFrame(sh); tf != nil {
			if paragraphs, err := tf.Paragraphs(); err == nil {
				var sb strings.Builder
				for i, par := range paragraphs {
					if i > 0 {
						sb.WriteByte('\n')
					}
					if txt, err := par.Text(); err == nil {
						sb.WriteString(txt)
					}
				}
				s2.Text = sb.String()
			}
		}
	}
	return s2
}

func shapeSummary(sh pptx.Shape) Shape {
	s := Shape{
		ID:         sh.ID(),
		Name:       sh.Name(),
		Kind:       sh.Kind().String(),
		NodePath:   sh.NodePath(),
		AltText:    sh.AltText(),
		Decorative: sh.IsDecorative(),
	}
	if b, err := sh.Bounds(); err == nil {
		s.Bounds = &Box{X: int64(b.X), Y: int64(b.Y), Width: int64(b.W), Height: int64(b.H)}
	}
	if wb, err := sh.WorldAABB(); err == nil {
		s.WorldBox = &Box{X: int64(wb.X), Y: int64(wb.Y), Width: int64(wb.W), Height: int64(wb.H)}
	}
	return s
}

// shapeTextFrame 取得 p:sp 的 TextFrame（类型断言）；非 AutoShape 返回 nil。
func shapeTextFrame(sh pptx.Shape) *pptx.TextFrame {
	if as, ok := sh.(*pptx.AutoShape); ok && as != nil {
		if tf, err := as.TextFrame(); err == nil {
			return tf
		}
	}
	return nil
}

// readTableText 把表格内容序列化为多行 tab 分隔串。
func readTableText(ts *pptx.TableShape, rows, cols int) string {
	if rows <= 0 || cols <= 0 {
		return ""
	}
	var sb strings.Builder
	for r := 0; r < rows; r++ {
		if r > 0 {
			sb.WriteByte('\n')
		}
		row, err := ts.Cell(r, 0)
		if err != nil {
			continue
		}
		_ = row
		for c := 0; c < cols; c++ {
			cell, err := ts.Cell(r, c)
			if err != nil {
				continue
			}
			if c > 0 {
				sb.WriteByte('\t')
			}
			text := cellText(cell)
			sb.WriteString(strings.TrimRight(text, "\n"))
		}
	}
	return sb.String()
}

// cellText 提取单元格拼接文本。
func cellText(c *pptx.Cell) string {
	tf, err := c.TextFrame()
	if err != nil {
		return ""
	}
	paragraphs, err := tf.Paragraphs()
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for i, par := range paragraphs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		txt, err := par.Text()
		if err != nil {
			continue
		}
		sb.WriteString(txt)
	}
	return sb.String()
}

// documentFingerprint 生成轻量指纹：core.xml 修改时间。
func documentFingerprint(p *pptx.Presentation) string {
	core, err := p.CoreProperties()
	if err != nil {
		return ""
	}
	if !core.Modified.Set || core.Modified.Value.IsZero() {
		return ""
	}
	return core.Modified.Value.UTC().Format(time.RFC3339)
}

func sortDiags(diags Diagnostics) {
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Severity != diags[j].Severity {
			return severityRank(diags[i].Severity) > severityRank(diags[j].Severity)
		}
		if diags[i].Code != diags[j].Code {
			return diags[i].Code < diags[j].Code
		}
		return diags[i].Part < diags[j].Part
	})
}

func severityRank(s SeverityString) int {
	switch s {
	case SevError:
		return 3
	case SevWarning:
		return 2
	case SevInfo:
		return 1
	}
	return 0
}
