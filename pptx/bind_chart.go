package pptx

import (
	"fmt"
)

// 本文件是模板绑定的**图表处理**：同名 ChartData 查找与预检
// （bindChart/chartBindPreflight）。

// ---------- 图表绑定 ----------

// bindChart 若数据源中存在与图表同名的 ChartData，则预检并登记绑定。
func (s *bindScanner) bindChart(c *ChartShape) error {
	const op = "Presentation.Bind"
	name := c.Name()
	if name == "" {
		return nil
	}
	v, found, err := s.resolve(nil, name)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	cd, ok := v.(ChartData)
	if !ok {
		return &OperationError{Op: op, Message: fmt.Sprintf(
			"chart %q expects pptx.ChartData in the data source, got %T", name, v), Err: ErrInvalidArgument}
	}
	if err := chartBindPreflight(c, cd); err != nil {
		return err
	}
	s.charts = append(s.charts, chartBind{shape: c, data: cd})
	return nil
}

// chartBindPreflight 复刻 ChartShape.SetData 的前置校验（只读），使
// plan 阶段即可判定该图表能否绑定——避免 apply 阶段中途失败。
func chartBindPreflight(c *ChartShape, cd ChartData) error {
	const op = "Presentation.Bind"
	if err := c.alive(); err != nil {
		return Annotate(err, op)
	}
	if err := validateChartData(cd, op); err != nil {
		return err
	}
	part, err := c.chartPartOf()
	if err != nil {
		return Annotate(err, op)
	}
	doc, err := c.p.docOf(part)
	if err != nil {
		return Annotate(err, op)
	}
	root := doc.Root()
	if root == nil || root.Namespace != nsChartML || root.Local() != "chartSpace" {
		return &OperationError{Op: op, Part: string(part),
			Message: "chart part root is not c:chartSpace", Err: ErrMalformedPackage}
	}
	if !chartIsCanonical(doc, root) {
		return &OperationError{Op: op, Part: string(part),
			Message: "chart " + c.Name() + " layout is not the library-canonical form; refused to avoid partial merge",
			Err:     ErrUnsupportedEdit}
	}
	cur, err := parseChartSpace(doc, root)
	if err != nil {
		return Annotate(err, op)
	}
	if cur.Type != cd.Type {
		return &OperationError{Op: op, Part: string(part), Message: fmt.Sprintf(
			"chart %q type change (%s → %s) is not a supported edit", c.Name(), cur.Type, cd.Type),
			Err: ErrUnsupportedEdit}
	}
	if _, ok := c.p.chartWorkbookPartOf(part); !ok {
		return &OperationError{Op: op, Part: string(part),
			Message: "chart " + c.Name() + " has no embedded workbook", Err: ErrUnsupportedEdit}
	}
	return nil
}
