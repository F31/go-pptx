package ooxml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/ooxml/schema"
)

// OOXML 命名空间常量（与门面/internal/ooxmlns 对齐）。
const (
	pmlMainNS   = "http://schemas.openxmlformats.org/presentationml/2006/main"
	dmlMainNS   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	dmlTableURI = "http://schemas.openxmlformats.org/drawingml/2006/table"
	dmlChartURI = "http://schemas.openxmlformats.org/drawingml/2006/chart"
)

// Box 是轴对齐矩形（EMU）。
type Box struct {
	X, Y, Width, Height int64
}

// ShapeInfo 是单个形状的 schema 投影摘要（无门面依赖）。
type ShapeInfo struct {
	ID          int64
	Name        string
	Kind        string // textbox/autoshape/picture/group/connector/table/chart/graphic-frame
	NodePath    string
	AltText     string
	Decorative  bool
	Bounds      *Box
	Text        string
	TableRows   int
	TableCols   int
	Chart       bool
	ChartRID    string
	HasChildren bool
}

// SlideShapes 解码 p:sld 并返回形状投影。组内子形状**内联到顶层**
// （与门面 Shapes() 的 IR 扁平化语义一致）。
func SlideShapes(data []byte) ([]ShapeInfo, error) {
	slide, err := schema.DecodeSlide(data)
	if err != nil {
		return nil, err
	}
	if slide.CSld == nil || slide.CSld.SpTree == nil {
		return nil, nil
	}
	var out []ShapeInfo
	walkGroup(slide.CSld.SpTree, "p:sld/p:cSld/p:spTree", &out)
	return out, nil
}

// walkGroup 收集组合内的全部形状（含递归组合），组内子形状追加到 out。
func walkGroup(g *schema.P_CT_GroupShape, path string, out *[]ShapeInfo) {
	var iSp, iGrp, iPic, iGf, iCxn int
	for _, sp := range g.Sp {
		iSp++
		*out = append(*out, shapeFrom(sp, fmt.Sprintf("%s/p:sp[%d]", path, iSp)))
	}
	for _, grp := range g.GrpSp {
		iGrp++
		// 组是透明的：不产出组自身，仅内联其子形状（与门面 IR 扁平化一致）。
		walkGroup(&grp, fmt.Sprintf("%s/p:grpSp[%d]", path, iGrp), out)
	}
	for _, pic := range g.Pic {
		iPic++
		*out = append(*out, shapeFromPic(pic, fmt.Sprintf("%s/p:pic[%d]", path, iPic)))
	}
	for _, gf := range g.GraphicFrame {
		iGf++
		*out = append(*out, shapeFromGFrame(gf, fmt.Sprintf("%s/p:graphicFrame[%d]", path, iGf)))
	}
	for _, cx := range g.CxnSp {
		iCxn++
		info := baseFrom(nvCxn(cx.NvCxnSpPr), "connector", fmt.Sprintf("%s/p:cxnSp[%d]", path, iCxn), cx.SpPr)
		*out = append(*out, info)
	}
}

func nv(n *schema.P_CT_ShapeNonVisual) *schema.A_CT_NonVisualDrawingProps {
	if n == nil {
		return nil
	}
	return n.CNvPr
}

func nvGroup(n *schema.P_CT_GroupShapeNonVisual) *schema.A_CT_NonVisualDrawingProps {
	if n == nil {
		return nil
	}
	return n.CNvPr
}

func nvCxn(n *schema.P_CT_ConnectorNonVisual) *schema.A_CT_NonVisualDrawingProps {
	if n == nil {
		return nil
	}
	return n.CNvPr
}

func nvGF(n *schema.P_CT_GraphicalObjectFrameNonVisual) *schema.A_CT_NonVisualDrawingProps {
	if n == nil {
		return nil
	}
	return n.CNvPr
}

func nvPic(n *schema.P_CT_PictureNonVisual) *schema.A_CT_NonVisualDrawingProps {
	if n == nil {
		return nil
	}
	return n.CNvPr
}

func shapeFrom(sp schema.P_CT_Shape, path string) ShapeInfo {
	kind := "autoshape"
	if sp.NvSpPr != nil && sp.NvSpPr.CNvSpPr != nil && sp.NvSpPr.CNvSpPr.TxBox {
		kind = "textbox"
	}
	info := baseFrom(nv(sp.NvSpPr), kind, path, sp.SpPr)
	info.Text = textBody(sp.TxBody)
	return info
}

func shapeFromPic(pic schema.P_CT_Picture, path string) ShapeInfo {
	return baseFrom(nvPic(pic.NvPicPr), "picture", path, pic.SpPr)
}

func shapeFromGFrame(gf schema.P_CT_GraphicalObjectFrame, path string) ShapeInfo {
	info := baseFrom(nvGF(gf.NvGraphicFramePr), "graphic-frame", path, gf.Xfrm)
	if gf.Graphic == nil || gf.Graphic.GraphicData == nil {
		return info
	}
	gd := gf.Graphic.GraphicData
	switch gd.Uri {
	case dmlTableURI:
		info.Kind = "table"
		if tbl := decodeTable(gd.Any); tbl != nil {
			info.TableRows = len(tbl.Tr)
			info.TableCols = maxCols(tbl.Tr)
			info.Text = tableText(tbl)
		}
	case dmlChartURI:
		info.Kind = "chart"
		info.Chart = true
		info.ChartRID = chartRID(gd.Any)
	}
	return info
}

func decodeTable(any []schema.RawElem) *schema.A_CT_Table {
	for i := range any {
		re := &any[i]
		if re.XMLName.Local == "tbl" && (re.XMLName.Space == dmlMainNS || re.XMLName.Space == "") {
			wrap := `<a:tbl xmlns:a="` + dmlMainNS + `">` + re.Inner + `</a:tbl>`
			var tbl schema.A_CT_Table
			if err := schema.Unmarshal([]byte(wrap), &tbl); err != nil {
				return nil
			}
			return &tbl
		}
	}
	return nil
}

func chartRID(any []schema.RawElem) string {
	// c:chart 的 r:id（关系 ID 指向 chart Part）；为发给调用方回退门面。
	for i := range any {
		re := &any[i]
		if re.XMLName.Local == "chart" {
			for _, a := range re.Attrs {
				if a.Name.Local == "id" {
					return a.Value
				}
			}
			return ""
		}
	}
	return ""
}

func maxCols(rows []schema.A_CT_TableRow) int {
	m := 0
	for i := range rows {
		if len(rows[i].Tc) > m {
			m = len(rows[i].Tc)
		}
	}
	return m
}

func tableText(tbl *schema.A_CT_Table) string {
	var sb strings.Builder
	for r, row := range tbl.Tr {
		if r > 0 {
			sb.WriteByte('\n')
		}
		cells := make([]string, 0, len(row.Tc))
		for _, tc := range row.Tc {
			if tc.HMerge || tc.VMerge {
				continue
			}
			cells = append(cells, textBody(tc.TxBody))
		}
		sb.WriteString(strings.Join(cells, "\t"))
	}
	return sb.String()
}

// textBody 拼接 a:txBody 全部段落的文本（段落间换行；含 fld 缓存文本与
// br 换行近似）。
func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func textBody(tb *schema.A_CT_TextBody) string {
	if tb == nil {
		return ""
	}
	var sb strings.Builder
	for i, p := range tb.P {
		if i > 0 {
			sb.WriteByte('\n')
		}
		for _, r := range p.R {
			sb.WriteString(r.T)
		}
		for _, f := range p.Fld {
			sb.WriteString(f.T)
		}
		for _, br := range p.Br {
			_ = br
			sb.WriteByte('\n')
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// baseFrom 提取 id/name/altText/bounds 等通用摘要。
func baseFrom(cnv *schema.A_CT_NonVisualDrawingProps, kind, path string, xfrmOrSPPr any) ShapeInfo {
	var info ShapeInfo
	info.Kind = kind
	info.NodePath = path
	if cnv != nil {
		info.ID = atoi64(string(cnv.Id))
		info.Name = cnv.Name
		info.AltText = cnv.Descr
	}
	var xfrm *schema.A_CT_Transform2D
	switch t := xfrmOrSPPr.(type) {
	case *schema.A_CT_ShapeProperties:
		if t != nil {
			xfrm = t.Xfrm
		}
	case *schema.A_CT_Transform2D:
		xfrm = t
	}

	if xfrm != nil && xfrm.Off != nil && xfrm.Ext != nil {
		info.Bounds = &Box{
			X:      atoi64(string(xfrm.Off.X)),
			Y:      atoi64(string(xfrm.Off.Y)),
			Width:  atoi64(string(xfrm.Ext.Cx)),
			Height: atoi64(string(xfrm.Ext.Cy)),
		}
	}
	return info
}
