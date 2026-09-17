package pptx

// 本文件是 TABLE-01 的表格样式访问器薄委托（v2.0 域搬迁）：解析已迁至
// internal/document/style 的 ResolveEffectiveCellStyle / ParseToggle /
// TablePrNode。保留根包接线：tableStyles.xml 定位（tblStyleLstDoc）与
// 表格几何（tableGeom，依赖 buildTableGrid）。

import (
	"github.com/F31/go-pptx/v2/internal/document/style"
	tablepkg "github.com/F31/go-pptx/v2/internal/document/table"
	"github.com/F31/go-pptx/v2/internal/opc"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// relTableStyles 是 presentation → tableStyles.xml 的关系类型。
const relTableStyles = opc.RelTypePrefix + "tableStyles"

// StyleID 返回表格样式 ID（a:tblPr@tableStyleId）；未设置返回空串。
func (t *TableShape) StyleID() (string, error) {
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return "", Annotate(err, "TableShape.StyleID")
	}
	pr := style.TablePrNode(doc, tbl)
	if pr == nil {
		return "", nil
	}
	v, _ := pr.Attr("", "tableStyleId")
	return v, nil
}

// StyleFlags 返回表格级区域开关的三态值。
func (t *TableShape) StyleFlags() (TableStyleFlags, error) {
	var f TableStyleFlags
	doc, tbl, err := t.locateTbl()
	if err != nil {
		return f, Annotate(err, "TableShape.StyleFlags")
	}
	pr := style.TablePrNode(doc, tbl)
	if pr == nil {
		return f, nil
	}
	f.FirstRow = style.ParseToggle(pr.Attr("", "firstRow"))
	f.BandRow = style.ParseToggle(pr.Attr("", "bandRow"))
	f.LastRow = style.ParseToggle(pr.Attr("", "lastRow"))
	f.FirstCol = style.ParseToggle(pr.Attr("", "firstCol"))
	f.BandCol = style.ParseToggle(pr.Attr("", "bandCol"))
	f.LastCol = style.ParseToggle(pr.Attr("", "lastCol"))
	return f, nil
}

// tblStyleLstDoc 返回 tableStyles.xml 的文档（可选 Part；缺失返回
// nil，不报错）。
func (p *Presentation) tblStyleLstDoc() (*xmlstore.XMLDocument, opc.PartName, error) {
	rels, ok, err := p.relsOf(p.main)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", nil
	}
	for _, rel := range rels {
		if rel.Mode == opc.TargetInternal && rel.Type == relTableStyles {
			doc, err := p.docOf(rel.TargetPart)
			if err != nil {
				if errIsNotFound(err) {
					return nil, "", nil
				}
				return nil, "", err
			}
			return doc, rel.TargetPart, nil
		}
	}
	return nil, "", nil
}

// tableGeom 返回单元格所在表格的行数、列数与样式开关（读取失败时
// 返回保守值，不影响样式解析主流程）。
func (p *Presentation) tableGeom(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) (rows, cols int, flags TableStyleFlags) {
	tbl := style.AncestorOf(doc, tc, nsDrawingML, "tbl")
	if tbl == nil {
		return 0, 0, flags
	}
	g, err := tablepkg.BuildGrid(doc, tbl)
	if err != nil {
		return 0, 0, flags
	}
	if pr := style.TablePrNode(doc, tbl); pr != nil {
		flags.FirstRow = style.ParseToggle(pr.Attr("", "firstRow"))
		flags.BandRow = style.ParseToggle(pr.Attr("", "bandRow"))
		flags.LastRow = style.ParseToggle(pr.Attr("", "lastRow"))
		flags.FirstCol = style.ParseToggle(pr.Attr("", "firstCol"))
		flags.BandCol = style.ParseToggle(pr.Attr("", "bandCol"))
		flags.LastCol = style.ParseToggle(pr.Attr("", "lastCol"))
	}
	return g.Rows(), g.Cols, flags
}

// EffectiveCellStyle 解析单元格的有效样式（§9.1）。
//
// 来源优先级：单元格显式覆盖（a:tcPr）→ 表格样式库命中区域部分
// （tableStyles.xml）→ 未定义（Resolved=false + 诊断，不臆造内置样式
// 映射）。错误仅在文档关闭/句柄失效/结构异常时返回；样式级解析不足
// 通过字段与诊断表达。
func (c *Cell) EffectiveCellStyle() (EffectiveCellStyle, []Diagnostic, error) {
	var out EffectiveCellStyle
	doc, tc, err := c.locate()
	if err != nil {
		return out, nil, Annotate(err, "Cell.EffectiveCellStyle")
	}
	env, err := c.p.styleEnv(c.part)
	if err != nil {
		return out, nil, Annotate(err, "Cell.EffectiveCellStyle")
	}
	var styleDoc *xmlstore.XMLDocument
	if sdoc, _, err := c.p.tblStyleLstDoc(); err == nil {
		styleDoc = sdoc
	}
	ctx := style.CellStyleCtx{
		Env:      env,
		Docs:     c.p.styleDocs(),
		StyleDoc: styleDoc,
		Part:     string(c.part),
		Geom:     c.p.tableGeom,
	}
	out, diags := style.ResolveEffectiveCellStyle(doc, tc, c.row, c.col, ctx)
	return out, diags, nil
}
