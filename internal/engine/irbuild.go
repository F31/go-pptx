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
	shapes, diags := projectShapes(p, s)
	page.Shapes = shapes
	page.Diagnostics = append(page.Diagnostics, diags...)
	if opts.IncludeNotes {
		if notes, err := s.SpeakerNotesText(); err == nil {
			page.NotesText = notes
		} else {
			page.Diagnostics = append(page.Diagnostics, ir.Diagnostic{
				Code: "IR_NOTES_READ", Severity: ir.SevWarning,
				Part: s.PartName(), Message: err.Error(),
			})
		}
	}
	if opts.IncludeTimingNode {
		page.HasTiming = s.HasTiming()
	}
	if opts.IncludeHidden {
		if h, err := s.Hidden(); err == nil {
			page.Hidden = &h
		} else {
			page.Diagnostics = append(page.Diagnostics, ir.Diagnostic{
				Code: "IR_HIDDEN_READ", Severity: ir.SevWarning,
				Part: s.PartName(), Message: err.Error(),
			})
		}
	}
	if opts.IncludeTimingIR && page.HasTiming {
		raw, _, err := s.TimingTreeRaw()
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

// projectShapes 以 internal/ooxml 的 schema 只读投影枚举形状（取代门面
// 句柄 Shapes），文本/表格亦由 schema 抽取；图表经门面 ChartShape（按
// ID 匹配）补 ChartType/Text（category 投影未解前）。
func projectShapes(p *pptx.Presentation, s *pptx.Slide) ([]ir.Shape, ir.Diagnostics) {
	b, ok := pptx.PartBytes(p, opc.PartName(s.PartName()))
	if !ok {
		return nil, ir.Diagnostics{{
			Code: "IR_SHAPES_READ", Severity: ir.SevWarning,
			Part: s.PartName(), Message: "slide part unreadable",
		}}
	}
	infos, err := ooxml.SlideShapes(b)
	if err != nil {
		return nil, ir.Diagnostics{{
			Code: "IR_SHAPES_READ", Severity: ir.SevWarning,
			Part: s.PartName(), Message: err.Error(),
		}}
	}
	var charts map[model.ShapeID]*pptx.ChartShape
	if hasChart(infos) {
		charts = chartShapesByID(s)
	}
	out := make([]ir.Shape, 0, len(infos))
	for _, info := range infos {
		out = append(out, toIRShape(info, charts[model.ShapeID(info.ID)]))
	}
	return out, nil
}

func hasChart(infos []ooxml.ShapeInfo) bool {
	for _, i := range infos {
		if i.Chart {
			return true
		}
	}
	return false
}

// chartShapesByID 把门面 Shapes 中的 ChartShape 按 ID 建索引（仅供图表
// 退化链路使用）。
func chartShapesByID(s *pptx.Slide) map[model.ShapeID]*pptx.ChartShape {
	out := map[model.ShapeID]*pptx.ChartShape{}
	shapes, err := s.Shapes()
	if err != nil {
		return out
	}
	for _, sh := range shapes {
		if cs, ok := sh.(*pptx.ChartShape); ok && cs != nil {
			out[cs.ID()] = cs
		}
	}
	return out
}

func toIRShape(info ooxml.ShapeInfo, cs *pptx.ChartShape) ir.Shape {
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
	if cs != nil {
		data, diags, err := cs.DataWithDiagnostics()
		if err == nil {
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
			if len(diags) > 0 {
				s2.Diagnostics = append(s2.Diagnostics, convertDiagnostics(diags)...)
			}
		}
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
