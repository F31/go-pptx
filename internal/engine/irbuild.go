package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/F31/go-pptx/internal/document/model"
	"github.com/F31/go-pptx/internal/ir"
	"github.com/F31/go-pptx/internal/ooxml"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/pptx"
)

// 本文件是门面 → IR 的**适配器**（v2.0 演进：`internal/ir` 只吃
// internal/document/model + xmlstore，不再 import 门面）。投影逻辑原先
// 位于 internal/ir.FromPresentation；现由 engine（编排层，允许依赖门面）
// 承载，输出 internal/ir 的稳定 DTO。形状/文本/表格已改用
// internal/ooxml 的 schema 只读投影（pptx.PartBytes 只读桥）。

// ProjectIR 把 Presentation 投影为 IR（原 ir.FromPresentation）。
func ProjectIR(p *pptx.Presentation, opts ir.Options) (*ir.Document, error) {
	if p == nil {
		return nil, fmt.Errorf("ir: nil presentation")
	}
	doc := &ir.Document{
		SchemaVersion: ir.SchemaVersion,
		SDKVersion:    ir.SDKVersion,
	}
	doc.DocumentID = documentFingerprint(p)

	core, diags := projectCore(p)
	doc.Core = core
	doc.Diagnostics = append(doc.Diagnostics, diags...)

	slides, err := p.Slides()
	if err != nil {
		return nil, fmt.Errorf("ir: list slides: %w", err)
	}
	doc.Pages = make([]ir.Page, 0, len(slides))
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
	ir.SortDiagnostics(doc.Diagnostics)
	return doc, nil
}

func projectCore(p *pptx.Presentation) (*ir.Core, ir.Diagnostics) {
	props, err := p.CoreProperties()
	if err != nil {
		return nil, ir.Diagnostics{{
			Code: "IR_CORE_READ", Severity: ir.SevWarning, Message: err.Error(),
		}}
	}
	c := &ir.Core{
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
func optionalString(o model.Optional[string]) string {
	if !o.Set {
		return ""
	}
	return o.Value
}

func projectPage(p *pptx.Presentation, s *pptx.Slide, idx int, opts ir.Options) (ir.Page, error) {
	page := ir.Page{
		Index:   idx,
		SlideID: s.ID(),
		Part:    s.PartName(),
		Name:    s.Name(),
	}
	part := opc.PartName(s.PartName())
	slideBytes, ok := pptx.PartBytes(p, part)
	if !ok {
		page.Diagnostics = append(page.Diagnostics, ir.Diagnostic{
			Code: "IR_SHAPES_READ", Severity: ir.SevWarning,
			Part: s.PartName(), Message: "slide part unreadable",
		})
	} else {
		shapes, diags := projectShapes(slideBytes, s.PartName(), p, part)
		page.Shapes = shapes
		page.Diagnostics = append(page.Diagnostics, diags...)
	}
	if opts.IncludeNotes {
		notes, err := projectNotes(p, part)
		if err != nil {
			page.Diagnostics = append(page.Diagnostics, ir.Diagnostic{
				Code: "IR_NOTES_READ", Severity: ir.SevWarning,
				Part: s.PartName(), Message: err.Error(),
			})
		} else {
			page.NotesText = notes
		}
	}
	if opts.IncludeTimingNode && ok {
		has, err := ooxml.SlideHasTiming(slideBytes)
		if err == nil {
			page.HasTiming = has
		}
	}
	if opts.IncludeHidden {
		if h, err := projectHidden(p, uint32(s.ID())); err == nil {
			page.Hidden = &h
		} else {
			page.Diagnostics = append(page.Diagnostics, ir.Diagnostic{
				Code: "IR_HIDDEN_READ", Severity: ir.SevWarning,
				Part: s.PartName(), Message: err.Error(),
			})
		}
	}
	if opts.IncludeTimingIR && page.HasTiming && ok {
		raw, _, err := ooxml.SlideTimingRaw(slideBytes)
		if err != nil {
			page.Diagnostics = append(page.Diagnostics, ir.Diagnostic{
				Code: "IR_TIMING_READ", Severity: ir.SevWarning,
				Part:    s.PartName(),
				Message: err.Error(),
			})
		} else if len(raw) > 0 {
			pt, perr := ir.ProjectTimingTree(s.PartName(), raw)
			if perr != nil {
				page.Diagnostics = append(page.Diagnostics, pt.Diagnostics...)
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

// projectNotes 以 ooxml 投影读取讲稿正文：slide 关系流 →
// notesSlide Part → 正文占位符文本。无 notes Part/关系返回 ("", nil)。
func projectNotes(p *pptx.Presentation, part opc.PartName) (string, error) {
	relsBytes, ok := pptx.PartBytes(p, ooxml.RelsPartName(part))
	if !ok {
		return "", nil
	}
	notesPart, ok := ooxml.NotesPartOf(relsBytes, part)
	if !ok {
		return "", nil
	}
	notesBytes, ok := pptx.PartBytes(p, notesPart)
	if !ok {
		return "", fmt.Errorf("notes part %s unreadable", notesPart)
	}
	text, err := ooxml.NotesText(notesBytes)
	if err != nil {
		return "", fmt.Errorf("notes body: %w", err)
	}
	return text, nil
}

// projectHidden 以 ooxml 投影读取页面隐藏标记（presentation sldIdLst）。
func projectHidden(p *pptx.Presentation, slideID uint32) (bool, error) {
	presBytes, ok := pptx.MainPartBytes(p)
	if !ok {
		return false, fmt.Errorf("presentation part unreadable")
	}
	return ooxml.SlideHidden(presBytes, slideID)
}

// projectShapes 以 internal/ooxml 的 schema 只读投影枚举形状（取代门面
// 句柄 Shapes），文本/表格/图表亦由 schema 投影抽取。
func projectShapes(b []byte, partStr string, p *pptx.Presentation, source opc.PartName) ([]ir.Shape, ir.Diagnostics) {
	infos, err := ooxml.SlideShapes(b)
	if err != nil {
		return nil, ir.Diagnostics{{
			Code: "IR_SHAPES_READ", Severity: ir.SevWarning,
			Part: partStr, Message: err.Error(),
		}}
	}
	// 预解析含图表的形状（rid → chart info 映射）。
	var chartMap map[model.ShapeID]*ooxml.ChartInfo
	for _, info := range infos {
		if info.Chart && info.ChartRID != "" {
			if chartMap == nil {
				chartMap = map[model.ShapeID]*ooxml.ChartInfo{}
			}
			if ci := projectChart(p, source, info.ChartRID); ci != nil {
				chartMap[model.ShapeID(info.ID)] = ci
			}
		}
	}
	out := make([]ir.Shape, 0, len(infos))
	for _, info := range infos {
		var ci *ooxml.ChartInfo
		if chartMap != nil {
			ci = chartMap[model.ShapeID(info.ID)]
		}
		out = append(out, toIRShape(info, ci))
	}
	return out, nil
}

// projectChart 通过 ooxml 投影解析图表数据（rid → chart part → decode）。
func projectChart(p *pptx.Presentation, source opc.PartName, rid string) *ooxml.ChartInfo {
	relsPart := ooxml.RelsPartName(source)
	relsData, ok := pptx.PartBytes(p, relsPart)
	if !ok || len(relsData) == 0 {
		return nil
	}
	chartPart, ok := ooxml.ChartPartOf(relsData, source, rid)
	if !ok {
		return nil
	}
	chartBytes, ok := pptx.PartBytes(p, chartPart)
	if !ok || len(chartBytes) == 0 {
		return nil
	}
	ci, err := ooxml.ChartData(chartBytes)
	if err != nil {
		return nil
	}
	return &ci
}

func toIRShape(info ooxml.ShapeInfo, chart *ooxml.ChartInfo) ir.Shape {
	s2 := ir.Shape{
		ID:         model.ShapeID(info.ID),
		Name:       info.Name,
		Kind:       info.Kind,
		NodePath:   info.NodePath,
		AltText:    info.AltText,
		Decorative: info.Decorative,
		Text:       info.Text,
		TableRows:  info.TableRows,
		TableCols:  info.TableCols,
	}
	if info.Bounds != nil {
		s2.Bounds = &ir.Box{X: info.Bounds.X, Y: info.Bounds.Y, Width: info.Bounds.Width, Height: info.Bounds.Height}
	}
	if chart != nil {
		s2.ChartType = chart.Type
		var sb strings.Builder
		if chart.Title != "" {
			sb.WriteString(chart.Title)
			sb.WriteString(": ")
		}
		for i, c := range chart.Categories {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(c)
		}
		s2.Text = sb.String()
	}
	return s2
}

func convertDiagnostics(in []pptx.Diagnostic) ir.Diagnostics {
	if len(in) == 0 {
		return nil
	}
	out := make(ir.Diagnostics, 0, len(in))
	for _, d := range in {
		out = append(out, ir.Diagnostic{
			Code:     d.Code,
			Severity: ir.SeverityString(d.Severity.String()),
			Part:     d.Part,
			Message:  d.Message,
		})
	}
	return out
}

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
