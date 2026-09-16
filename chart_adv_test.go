package pptx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// chartSpecWithExtras 在 chartSpecFixture 上加 DataLabel + Series[0].Trendline/
// ErrorBars + Axes。
func chartSpecWithExtras() ChartSpec {
	s := chartSpecFixture()
	s.DataLabel = &ChartDataLabel{Show: true, Position: "outB"}
	s.Series[0].Trendline = &ChartTrendline{Type: ChartTrendLinear, DisplayEq: true, Name: "T1"}
	s.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrFixed, Value: 1.5, Direction: "both"}
	s.Axes = &ChartAxisOptions{ValueLogBase: 10, Min: NewOptional(0.5), Max: NewOptional(99.5)}
	return s
}

func TestChartAdv_DataLabel_RoundTrip(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	spec := chartSpecFixture()
	spec.DataLabel = &ChartDataLabel{Show: true, Position: "ctr"}
	if _, err := s.AddChart(context.Background(), spec); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	got, err := cs.Data()
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	if got.DataLabel == nil || got.DataLabel.Position != "ctr" || !got.DataLabel.Show {
		t.Errorf("DataLabel = %+v", got.DataLabel)
	}
}

func TestChartAdv_Trendline_AllTypes(t *testing.T) {
	cases := []ChartTrendline{
		{Type: ChartTrendLinear, Name: "L"},
		{Type: ChartTrendLogarithmic, Name: "Log"},
		{Type: ChartTrendExponential, Name: "Exp", DisplayRSq: true},
		{Type: ChartTrendPolynomial, Order: 3, Name: "Poly"},
		{Type: ChartTrendPower, Name: "Pow"},
		{Type: ChartTrendMovingAverage, Period: 4, Name: "MA"},
	}
	for i, tr := range cases {
		p := audioDeck(t)
		s := SlidesOf(t, p)[0]
		spec := chartSpecFixture()
		spec.Series[0].Trendline = &ChartTrendline{
			Type: tr.Type, Period: tr.Period, Order: tr.Order,
			Name: tr.Name, DisplayRSq: tr.DisplayRSq,
		}
		if _, err := s.AddChart(context.Background(), spec); err != nil {
			t.Errorf("case %d (%s): AddChart: %v", i, tr.Type, err)
			p.Close()
			continue
		}
		cs := chartShapesOf(t, s)[0]
		got, err := cs.Data()
		if err != nil {
			t.Errorf("case %d: Data: %v", i, err)
			p.Close()
			continue
		}
		if len(got.Series) == 0 || got.Series[0].Trendline == nil {
			t.Errorf("case %d: trendline missing in readback", i)
		} else if got.Series[0].Trendline.Type != tr.Type {
			t.Errorf("case %d: type = %v want %v", i, got.Series[0].Trendline.Type, tr.Type)
		}
		p.Close()
	}
}

func TestChartAdv_ErrorBars_AllTypes(t *testing.T) {
	cases := []ChartErrorBars{
		{Type: ChartErrStandardDeviation},
		{Type: ChartErrStandardError, NoEndCap: true},
		{Type: ChartErrFixed, Value: 1.0, Direction: "plus"},
		{Type: ChartErrPercentage, Value: 5, Direction: "minus"},
	}
	for i, eb := range cases {
		p := audioDeck(t)
		s := SlidesOf(t, p)[0]
		spec := chartSpecFixture()
		spec.Series[0].ErrorBars = &ChartErrorBars{
			Type: eb.Type, Value: eb.Value, Direction: eb.Direction, NoEndCap: eb.NoEndCap,
		}
		if _, err := s.AddChart(context.Background(), spec); err != nil {
			t.Errorf("case %d: AddChart: %v", i, err)
			p.Close()
			continue
		}
		cs := chartShapesOf(t, s)[0]
		got, err := cs.Data()
		if err != nil {
			t.Errorf("case %d: Data: %v", i, err)
			p.Close()
			continue
		}
		if len(got.Series) == 0 || got.Series[0].ErrorBars == nil {
			t.Errorf("case %d: errorBars missing", i)
		} else if got.Series[0].ErrorBars.Type != eb.Type {
			t.Errorf("case %d: type = %v want %v", i, got.Series[0].ErrorBars.Type, eb.Type)
		}
		p.Close()
	}
}

func TestChartAdv_Axes_LogAndBounds(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	spec := chartSpecFixture()
	spec.Axes = &ChartAxisOptions{
		ValueLogBase: 10,
		Min:          NewOptional(0.0),
		Max:          NewOptional(1000.0),
	}
	if _, err := s.AddChart(context.Background(), spec); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	got, err := cs.Data()
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	if got.Axes == nil {
		t.Fatalf("Axes missing")
	}
	if got.Axes.ValueLogBase != 10 {
		t.Errorf("ValueLogBase = %d", got.Axes.ValueLogBase)
	}
	if !got.Axes.Min.Set || got.Axes.Min.Value != 0 {
		t.Errorf("Min = %+v", got.Axes.Min)
	}
	if !got.Axes.Max.Set || got.Axes.Max.Value != 1000 {
		t.Errorf("Max = %+v", got.Axes.Max)
	}
	chartBytes, _ := p.partBytes("/ppt/charts/chart1.xml")
	for _, want := range []string{
		`<c:logBase val="10"/>`,
		`<c:min val="0"`,
		`<c:max val="1000"`,
	} {
		if !strings.Contains(string(chartBytes), want) {
			t.Errorf("chart XML missing %q", want)
		}
	}
}

func TestChartAdv_Axes_DateAxis(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	spec := chartSpecFixture()
	spec.Axes = &ChartAxisOptions{CategoryAsDate: true}
	if _, err := s.AddChart(context.Background(), spec); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	got, err := cs.Data()
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	if got.Axes == nil || !got.Axes.CategoryAsDate {
		t.Errorf("Axes.CategoryAsDate = %+v", got.Axes)
	}
	chartBytes, _ := p.partBytes("/ppt/charts/chart1.xml")
	if !strings.Contains(string(chartBytes), "<c:dateAx>") {
		t.Errorf("chart XML has no c:dateAx")
	}
	if strings.Contains(string(chartBytes), "<c:catAx>") {
		t.Errorf("date-axis chart must not contain c:catAx")
	}
}

func TestChartAdv_SetData_AcceptsExtras(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	spec := chartSpecWithExtras()
	if _, err := s.AddChart(context.Background(), spec); err != nil {
		t.Fatalf("AddChart: %v", err)
	}
	cs := chartShapesOf(t, s)[0]
	cur, _ := cs.Data()
	cur.DataLabel = spec.DataLabel
	cur.Series[0].Trendline = spec.Series[0].Trendline
	cur.Series[0].ErrorBars = spec.Series[0].ErrorBars
	cur.Axes = spec.Axes
	if err := cs.SetData(cur); err != nil {
		t.Fatalf("SetData with extras: %v", err)
	}
	got, _ := cs.Data()
	if got.Axes == nil || got.Axes.ValueLogBase != 10 {
		t.Errorf("after SetData axes = %+v", got.Axes)
	}
	if got.Series[0].Trendline == nil || got.Series[0].Trendline.Type != ChartTrendLinear {
		t.Errorf("trendline lost: %+v", got.Series[0].Trendline)
	}
	if got.Series[0].ErrorBars == nil || got.Series[0].ErrorBars.Type != ChartErrFixed {
		t.Errorf("errBars lost: %+v", got.Series[0].ErrorBars)
	}
}

func TestChartAdv_Validation_Whitelist(t *testing.T) {
	// 每个用例各自一份 Presentation 避免 s 被多个失败污染。
	cases := []struct {
		name string
		spec func() ChartSpec
		want error
	}{
		{
			name: "label position invalid",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.DataLabel = &ChartDataLabel{Show: true, Position: "nowhere"}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "errBars direction invalid",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrFixed, Value: 1, Direction: "up"}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "errBars fixed value negative",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrFixed, Value: -1}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "errBars percentage out of range",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrPercentage, Value: 1500}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "trendline poly order out of range",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Series[0].Trendline = &ChartTrendline{Type: ChartTrendPolynomial, Order: 1}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "trendline movingAvg period too small",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Series[0].Trendline = &ChartTrendline{Type: ChartTrendMovingAverage, Period: 1}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "axes logBase out of range",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Axes = &ChartAxisOptions{ValueLogBase: 64}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "axes logBase zero ok",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Axes = &ChartAxisOptions{ValueLogBase: 0}
				return sp
			},
			want: nil,
		},
		{
			name: "axes logBase 2 ok",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Axes = &ChartAxisOptions{ValueLogBase: 2}
				return sp
			},
			want: nil,
		},
		{
			name: "axes position invalid",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Axes = &ChartAxisOptions{Position: "middle"}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "axes min > max",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Axes = &ChartAxisOptions{Min: NewOptional(2.0), Max: NewOptional(1.0)}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "pie errBars forbidden",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Type = ChartPie
				sp.Series[0].ErrorBars = &ChartErrorBars{Type: ChartErrFixed, Value: 0.5}
				return sp
			},
			want: ErrInvalidArgument,
		},
		{
			name: "pie trendline forbidden",
			spec: func() ChartSpec {
				sp := chartSpecFixture()
				sp.Type = ChartPie
				sp.Series[0].Trendline = &ChartTrendline{Type: ChartTrendLinear}
				return sp
			},
			want: ErrInvalidArgument,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := audioDeck(t)
			defer p.Close()
			s := SlidesOf(t, p)[0]
			_, err := s.AddChart(context.Background(), tc.spec())
			if tc.want == nil {
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Errorf("AddChart succeeded, want error")
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want chain contains %v", err, tc.want)
			}
		})
	}
}
