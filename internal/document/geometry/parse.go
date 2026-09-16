package geometry

import (
	"strconv"

	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/ooxmlns"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件是 GEOM-02 的**几何解析**：a:prstGeom（preset + adjust 公式）与
// a:custGeom（路径 pathLst、命令、guide）→ GeometryInfo（只读投影）。

// spPrOf 查找形状元素下的 spPr。
func spPrOf(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	return xmlstore.ChildOfKind(doc, el, ooxmlns.PresentationML, "spPr", 0)
}

// ParseShapeGeometry 解析 spPr 下 a:prstGeom / a:custGeom。
func ParseShapeGeometry(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) (GeometryInfo, []diag.Diagnostic, error) {
	var diags []diag.Diagnostic
	sp := spPrOf(doc, el)
	if sp == nil {
		// 组合（grpSp）等无 spPr 的情况：返回 unknown + Warning。
		diags = append(diags, diag.Diagnostic{
			Code: "geom.spPr.missing", Severity: diag.SeverityWarning,
			Message: "shape has no spPr; geometry not applicable",
		})
		return GeometryInfo{Kind: GeometryUnknown, Diagnostics: diags}, diags, nil
	}
	// 在 spPr 子节点中按文档序查找首个 a:prstGeom 或 a:custGeom。
	var node *xmlstore.NodeRecord
	var kind GeometryKind
	for _, cid := range sp.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "prstGeom":
			node = c
			kind = GeometryPreset
		case "custGeom":
			node = c
			kind = GeometryCustom
		}
		if node != nil {
			break
		}
	}
	if node == nil {
		// spPr 存在但无几何元素：合法（容器继承），按 unknown 处理。
		return GeometryInfo{Kind: GeometryUnknown}, diags, nil
	}
	info := GeometryInfo{Kind: kind}
	switch kind {
	case GeometryPreset:
		info.Preset, _ = node.Attr("", "prst")
		parsePresetGeomAdjusts(doc, node, &info, &diags)
	case GeometryCustom:
		parseCustomGeomPathList(doc, node, &info, &diags)
	}
	// 未识别的兄弟元素（紧邻 prstGeom/custGeom）记 Unknown。
	for _, cid := range sp.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		if c == node {
			continue
		}
		switch c.Local() {
		case "xfrm", "fill", "ln", "effectLst", "effectDag", "scene3d", "sp3d", "extLst", "style":
			// 这些是 spPr 的合法非几何元素，不视为 Unknown。
			continue
		}
		// 仅对"看起来像几何"的兄弟记录未知（如 a:prstGeom/custGeom 之外的）。
		if c.Local() == "prstGeom" || c.Local() == "custGeom" {
			info.Unknown = append(info.Unknown, c.Local())
		}
	}
	info.Diagnostics = diags
	return info, diags, nil
}

// parsePresetGeomAdjusts 解析 a:prstGeom 内 a:avLst/a:gd。
func parsePresetGeomAdjusts(doc *xmlstore.XMLDocument, node *xmlstore.NodeRecord,
	info *GeometryInfo, diags *[]diag.Diagnostic) {
	avLst := xmlstore.ChildOfKind(doc, node, ooxmlns.DrawingML, "avLst", 0)
	if avLst == nil {
		return
	}
	for _, cid := range avLst.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML || c.Local() != "gd" {
			if c.Namespace == ooxmlns.DrawingML {
				info.Unknown = append(info.Unknown, c.Local())
			}
			continue
		}
		name, _ := c.Attr("", "name")
		fmla, _ := c.Attr("", "fmla")
		info.Adjusts = append(info.Adjusts, GeomAdjust{Name: name, Fmla: fmla})
	}
}

// parseCustomGeomPathList 解析 a:custGeom/a:pathLst/a:path 与 a:gdLst。
func parseCustomGeomPathList(doc *xmlstore.XMLDocument, node *xmlstore.NodeRecord,
	info *GeometryInfo, diags *[]diag.Diagnostic) {
	gdLst := xmlstore.ChildOfKind(doc, node, ooxmlns.DrawingML, "gdLst", 0)
	if gdLst != nil {
		for _, cid := range gdLst.Children {
			c := doc.Node(cid)
			if c.Namespace != ooxmlns.DrawingML || c.Local() != "gd" {
				continue
			}
			name, _ := c.Attr("", "name")
			fmla, _ := c.Attr("", "fmla")
			info.Guides = append(info.Guides, GeomGuide{Name: name, Fmla: fmla})
		}
	}
	pathLst := xmlstore.ChildOfKind(doc, node, ooxmlns.DrawingML, "pathLst", 0)
	if pathLst == nil {
		return
	}
	for _, cid := range pathLst.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML || c.Local() != "path" {
			continue
		}
		p := GeomPath{}
		if v, ok := c.Attr("", "w"); ok {
			if n, e := strconv.ParseInt(v, 10, 64); e == nil {
				p.Width = EMU(n)
			} else {
				*diags = append(*diags, diag.Diagnostic{
					Code: "geom.custGeom.invalid_w", Severity: diag.SeverityWarning,
					Message: "invalid w attribute on path: " + v,
				})
			}
		}
		if v, ok := c.Attr("", "h"); ok {
			if n, e := strconv.ParseInt(v, 10, 64); e == nil {
				p.Height = EMU(n)
			} else {
				*diags = append(*diags, diag.Diagnostic{
					Code: "geom.custGeom.invalid_h", Severity: diag.SeverityWarning,
					Message: "invalid h attribute on path: " + v,
				})
			}
		}
		p.Fill, _ = c.Attr("", "fill")
		p.Stroke, _ = c.Attr("", "stroke")
		parsePathCommands(doc, c, &p, diags)
		info.Paths = append(info.Paths, p)
	}
}

// parsePathCommands 解析 a:path 内全部 a:moveTo/a:lnTo/a:arcTo/a:cubicBezTo/
// a:quadBezTo/a:close。
func parsePathCommands(doc *xmlstore.XMLDocument, path *xmlstore.NodeRecord,
	out *GeomPath, diags *[]diag.Diagnostic) {
	for _, cid := range path.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		switch c.Local() {
		case "close":
			out.Commands = append(out.Commands, PathCommand{Kind: "close"})
		case "moveTo", "lnTo", "arcTo":
			pt, ok := parseSinglePoint(doc, c)
			if !ok {
				*diags = append(*diags, diag.Diagnostic{
					Code: "geom.custGeom.missing_pt", Severity: diag.SeverityWarning,
					Message: c.Local() + " missing/invalid pt",
				})
				continue
			}
			out.Commands = append(out.Commands, PathCommand{Kind: c.Local(), Points: []Point{pt}})
		case "cubicBezTo":
			pts := parseTriplePoints(doc, c)
			if len(pts) < 3 {
				*diags = append(*diags, diag.Diagnostic{
					Code: "geom.custGeom.missing_pt", Severity: diag.SeverityWarning,
					Message: "cubicBezTo requires 3 points",
				})
				continue
			}
			out.Commands = append(out.Commands, PathCommand{Kind: c.Local(), Points: pts[:3]})
		case "quadBezTo":
			pts := parseTriplePoints(doc, c)
			if len(pts) < 2 {
				*diags = append(*diags, diag.Diagnostic{
					Code: "geom.custGeom.missing_pt", Severity: diag.SeverityWarning,
					Message: "quadBezTo requires 2 points",
				})
				continue
			}
			out.Commands = append(out.Commands, PathCommand{Kind: c.Local(), Points: pts[:2]})
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
}

// parseSinglePoint 从容器元素中取首个 a:pt，解析其 x/y。
func parseSinglePoint(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord) (Point, bool) {
	pt := xmlstore.ChildOfKind(doc, parent, ooxmlns.DrawingML, "pt", 0)
	if pt == nil {
		return Point{}, false
	}
	x, okX := pt.Attr("", "x")
	y, okY := pt.Attr("", "y")
	if !okX || !okY {
		return Point{}, false
	}
	nx, e1 := strconv.ParseInt(x, 10, 64)
	ny, e2 := strconv.ParseInt(y, 10, 64)
	if e1 != nil || e2 != nil {
		return Point{}, false
	}
	return Point{X: EMU(nx), Y: EMU(ny)}, true
}

// parseTriplePoints 从容器元素中取前 3 个 a:pt；用于 cubicBezTo / quadBezTo。
func parseTriplePoints(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord) []Point {
	var out []Point
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML || c.Local() != "pt" {
			continue
		}
		x, okX := c.Attr("", "x")
		y, okY := c.Attr("", "y")
		if !okX || !okY {
			continue
		}
		nx, e1 := strconv.ParseInt(x, 10, 64)
		ny, e2 := strconv.ParseInt(y, 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		out = append(out, Point{X: EMU(nx), Y: EMU(ny)})
		if len(out) >= 3 {
			break
		}
	}
	return out
}
