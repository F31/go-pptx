package style

// TABLE-01 表格样式解析的测试（根包 table_style.go 下沉后随迁）：
// resolveColorSpec / ParseToggle / tblStyleNode 等纯函数 + 单元格样式
// 解析（显式覆盖 → 样式库区域命中 → unresolved）。

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/diag"
	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/opc"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

const tableThemeXML = `<a:theme xmlns:a="` + dd + `"><a:themeElements>` +
	`<a:clrScheme><a:accent1><a:srgbClr val="4472C4"/></a:accent1>` +
	`<a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
	`</a:clrScheme></a:themeElements></a:theme>`

const tableMasterXML = `<p:sldMaster xmlns:p="` + pp + `" xmlns:a="` + dd + `"><p:clrMap bg1="lt1"/></p:sldMaster>`

func tableEnv(t *testing.T) (*Env, DocFunc) {
	t.Helper()
	env := &Env{Kind: StyleKindSlide, Master: "/m.xml", Theme: "/t.xml"}
	theme := idx(t, tableThemeXML)
	master := idx(t, tableMasterXML)
	parts := map[opc.PartName]*xmlstore.XMLDocument{env.Theme: theme, env.Master: master}
	return env, func(p opc.PartName) *xmlstore.XMLDocument { return parts[p] }
}

func TestParseToggleVariants(t *testing.T) {
	for v, want := range map[string]StyleToggle{
		"on": ToggleOn, "1": ToggleOn, "true": ToggleOn,
		"off": ToggleOff, "0": ToggleOff, "false": ToggleOff,
		"weird": ToggleDefault, "": ToggleDefault,
	} {
		if got := ParseToggle(v, true); got != want {
			t.Errorf("ParseToggle(%q) = %v, want %v", v, got, want)
		}
	}
	if got := ParseToggle("on", false); got != ToggleDefault {
		t.Errorf("not-ok = %v", got)
	}
}

func TestTableHelperEdges(t *testing.T) {
	if got := tblStyleNode(nil, "x"); got != nil {
		t.Fatalf("tblStyleNode(nil) = %+v", got)
	}
	if got := tblStyleNode(idx(t, `<a:tblStyleLst xmlns:a="`+dd+`"/>`), ""); got != nil {
		t.Fatalf("empty id = %+v", got)
	}
	if got := tblStylePartNode(nil, nil, PartFirstRow); got != nil {
		t.Fatalf("tblStylePartNode(nil,nil) = %+v", got)
	}
	if got := tcStyleNode(nil, nil); got != nil {
		t.Fatalf("tcStyleNode(nil,nil) = %+v", got)
	}
	if got, _ := fillIn(nil, nil); got != nil {
		t.Fatalf("fillIn(nil,nil) = %+v", got)
	}
	doc := idx(t, `<a:tblStyleLst xmlns:a="`+dd+`"><a:tblStyle styleId="S1"/></a:tblStyleLst>`)
	if got := tblStyleNode(doc, "nope"); got != nil {
		t.Fatalf("missing style = %+v", got)
	}
	doc2 := idx(t, `<a:tcPr xmlns:a="`+dd+`"><a:weirdFill/></a:tcPr>`)
	if node, kind := fillPropIn(doc2, doc2.Root()); node != nil || kind != FillUnspecified {
		t.Fatalf("unknown fill = %+v %v", node, kind)
	}
	if got := TablePrNode(doc2, doc2.Root()); got != nil {
		t.Fatalf("TablePrNode mismatch = %+v", got)
	}
}

func TestResolveColorSpec(t *testing.T) {
	env, docs := tableEnv(t)
	for _, tc := range []struct {
		name     string
		inner    string
		wantRGB  string
		wantRes  bool
		wantDiag string
	}{
		{"srgb-direct", `<a:srgbClr val="abcdef"/>`, "ABCDEF", true, ""},
		{"srgb-missing-val", `<a:srgbClr/>`, "", false, ""},
		{"srgb-transform", `<a:srgbClr val="abcdef"><a:lumMod val="50000"/></a:srgbClr>`, "ABCDEF", false, "STYLE_PARTIAL"},
		{"sysclr-lastclr", `<a:sysClr val="windowText" lastClr="12345a"/>`, "12345A", true, ""},
		{"sysclr-no-lastclr", `<a:sysClr val="windowText"/>`, "", false, "STYLE_UNRESOLVED"},
		{"scheme-direct", `<a:schemeClr val="accent1"/>`, "4472C4", true, ""},
		{"scheme-clrmap-indirect", `<a:schemeClr val="bg1"/>`, "FFFFFF", true, ""},
		{"scheme-unknown", `<a:schemeClr val="nope"/>`, "", false, "STYLE_PARTIAL"},
		{"scheme-missing-val", `<a:schemeClr/>`, "", false, ""},
		{"scheme-transform", `<a:schemeClr val="accent1"><a:lumMod val="50000"/></a:schemeClr>`, "4472C4", false, "STYLE_PARTIAL"},
		{"empty-fill", ``, "", false, "STYLE_UNRESOLVED"},
		{"unknown-color-kind", `<a:hslClr hue="1"/>`, "", false, "STYLE_UNRESOLVED"},
	} {
		fill := idx(t, `<a:solidFill xmlns:a="`+dd+`">`+tc.inner+`</a:solidFill>`)
		var diags []diag.Diagnostic
		rc := resolveColorSpec(fill, fill.Root(), env, docs, "/ppt/slides/slide1.xml", &diags)
		if rc.RGB != tc.wantRGB || rc.Resolved != tc.wantRes {
			t.Errorf("%s: RGB=%q Resolved=%v, want %q/%v (diags=%v)", tc.name, rc.RGB, rc.Resolved, tc.wantRGB, tc.wantRes, diags)
		}
		if (tc.wantDiag == "") != (len(diags) == 0) {
			t.Errorf("%s: diags=%+v, want code %q", tc.name, diags, tc.wantDiag)
		}
	}
	// 无主题环境：schemeClr 部分解析降级。
	fill := idx(t, `<a:solidFill xmlns:a="`+dd+`"><a:schemeClr val="accent1"/></a:solidFill>`)
	var diags []diag.Diagnostic
	rc := resolveColorSpec(fill, fill.Root(), &Env{Kind: StyleKindSlide}, nil, "part", &diags)
	if rc.Resolved || rc.RGB != "" {
		t.Errorf("no-theme schemeClr = %+v", rc)
	}
	if len(diags) == 0 || diags[0].Code != "STYLE_PARTIAL" {
		t.Errorf("no-theme want STYLE_PARTIAL, got %+v", diags)
	}
}

func TestStylePartFillKindToggleString(t *testing.T) {
	for _, c := range []struct {
		part StylePart
		want string
	}{{PartWholeTable, "wholeTbl"}, {PartBand1H, "band1H"}, {PartSECell, "seCell"}, {StylePart(99), "StylePart(99)"}} {
		if c.part.String() != c.want {
			t.Errorf("StylePart %v.String()=%q want %q", c.part, c.part.String(), c.want)
		}
	}
	if StyleToggle(7).String() != "def" || StyleToggle(0).String() != "def" ||
		ParseToggle("x", false) != ToggleDefault {
		t.Fatal("toggle string/default")
	}
	for _, c := range []struct {
		k    FillKind
		want string
	}{{FillNone, "none"}, {FillSolid, "solid"}, {FillGradient, "gradient"}, {FillPattern, "pattern"},
		{FillPicture, "picture"}, {FillGroup, "group"}, {FillUnspecified, "unspecified"}, {FillKind(99), "unspecified"}} {
		if c.k.String() != c.want {
			t.Errorf("FillKind %v.String()=%q want %q", c.k, c.k.String(), c.want)
		}
	}
}

func TestPartsForCell(t *testing.T) {
	// 2×2 表、默认开关（bandRow on）：NW 角 + 首行 + 首列 + 双条带 + whole。
	parts := partsForCell(0, 0, 2, 2, TableStyleFlags{})
	if len(parts) != 6 || parts[0] != PartNWCell || parts[len(parts)-1] != PartWholeTable {
		t.Fatalf("nw parts = %v", parts)
	}
	// 单行多列：无角、无 lastRow（rows 不>1），首行 + 双条带 + whole。
	p := partsForCell(0, 1, 1, 3, TableStyleFlags{})
	if len(p) != 4 {
		t.Fatalf("single-row parts = %v", p)
	}
	for _, x := range p {
		if x == PartLastRow || x == PartNWCell || x == PartSECell {
			t.Fatalf("single-row leaked corner/lastRow: %v", p)
		}
	}
	// bandRow 关闭时条带不出现。
	p2 := partsForCell(1, 0, 4, 4, TableStyleFlags{BandRow: ToggleOff})
	for _, x := range p2 {
		if x == PartBand1H || x == PartBand2H {
			t.Fatalf("bandRow off leaked: %v", p2)
		}
	}
	// bandCol on 时垂直条带出现。
	p3 := partsForCell(1, 1, 4, 4, TableStyleFlags{})
	found := false
	for _, x := range p3 {
		if x == PartBand1V || x == PartBand2V {
			found = true
		}
	}
	if !found {
		t.Fatalf("no bandV in %v", p3)
	}
	_ = partsForCell(3, 3, 4, 4, TableStyleFlags{})
}

func cellStyleFixture(t *testing.T) (*xmlstore.XMLDocument, *xmlstore.NodeRecord, *Env, DocFunc) {
	t.Helper()
	env, docs := tableEnv(t)
	tbl := idx(t, `<a:tbl xmlns:a="`+dd+`"><a:tblPr tableStyleId="S1"/>`+
		`<a:tblGrid/><a:tr><a:tc><a:tcPr anchor="ctr" marL="91440">`+
		`<a:solidFill><a:schemeClr val="bg1"/></a:solidFill>`+
		`<a:lnL w="12700"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:lnL>`+
		`</a:tcPr></a:tc><a:tc/></a:tr>`+
		`<a:tr><a:tc/><a:tc/></a:tr></a:tbl>`)
	tc := xmlstore.ChildOfKind(tbl, xmlstore.ChildOfKind(tbl, tbl.Root(), ooxmlns.DrawingML, "tr", 0), ooxmlns.DrawingML, "tc", 0)
	return tbl, tc, env, docs
}

// tnakedTable 返回无显式样式单元格的裸表（供 unresolved / 样式库测试）。
func cellAt(doc *xmlstore.XMLDocument, tr, tc int) *xmlstore.NodeRecord {
	tbl := doc.Root()
	trIdx := 0
	for _, cid := range tbl.Children {
		n := doc.Node(cid)
		if n == nil || n.Namespace != ooxmlns.DrawingML || n.Local() != "tr" {
			continue
		}
		if trIdx == tr {
			tcIdx := 0
			for _, cid2 := range n.Children {
				c2 := doc.Node(cid2)
				if c2 == nil || c2.Namespace != ooxmlns.DrawingML || c2.Local() != "tc" {
					continue
				}
				if tcIdx == tc {
					return c2
				}
				tcIdx++
			}
		}
		trIdx++
	}
	return nil
}

func TestResolveEffectiveCellStyleExplicit(t *testing.T) {
	doc, tc, env, docs := cellStyleFixture(t)
	geom := func(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) (int, int, TableStyleFlags) {
		return 2, 2, TableStyleFlags{FirstRow: ToggleOn, BandRow: ToggleOn}
	}
	out, diags := ResolveEffectiveCellStyle(doc, tc, 0, 0, CellStyleCtx{Env: env, Docs: docs, Geom: geom})
	if !out.Fill.Resolved || out.Fill.Trace[0].Source != SourceCellExplicit {
		t.Fatalf("fill = %+v", out.Fill)
	}
	// bg1 → clrMap→lt1→sysClr window lastClr FFFFFF。
	if out.Fill.Value.Color.RGB != "FFFFFF" {
		t.Fatalf("fill color = %+v", out.Fill.Value.Color)
	}
	if !out.Borders.Left.Resolved || out.Borders.Left.Value.Width != 12700 || out.Borders.Left.Value.Color.RGB != "FF0000" {
		t.Fatalf("lnL = %+v", out.Borders.Left)
	}
	if !out.Text.Resolved || out.Text.Value.Anchor != "ctr" || out.Text.Value.MarginLeft != 91440 {
		t.Fatalf("text = %+v", out.Text)
	}
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
}

func testStyleLib() string {
	return `<a:tblStyleLst xmlns:a="` + dd + `"><a:tblStyle styleId="S1">` +
		`<a:wholeTbl><a:tcStyle><a:fill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:fill>` +
		`<a:tcBdr><a:lnB w="6350"><a:solidFill><a:srgbClr val="999999"/></a:solidFill></a:lnB></a:tcBdr>` +
		`<a:anchr c="1" marL="9144"/></a:tcStyle></a:wholeTbl>` +
		`<a:firstRow><a:tcStyle><a:fill><a:solidFill><a:srgbClr val="00FF00"/></a:solidFill></a:fill></a:tcStyle></a:firstRow>` +
		`</a:tblStyle></a:tblStyleLst>`
}

func TestResolveEffectiveCellStyleLibrary(t *testing.T) {
	doc, _, env, docs := cellStyleFixture(t)
	tc := cellAt(doc, 1, 1)
	styleDoc := idx(t, testStyleLib())
	geom := func(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) (int, int, TableStyleFlags) {
		return 2, 2, TableStyleFlags{}
	}
	out, diags := ResolveEffectiveCellStyle(doc, tc, 1, 1, CellStyleCtx{
		Env: env, Docs: docs, StyleDoc: styleDoc, Part: "/p", Geom: geom})
	if !out.StyleResolved || out.StyleID != "S1" || out.Part != PartWholeTable {
		t.Fatalf("lib header = %+v", out)
	}
	if !out.Fill.Resolved || out.Fill.Value.Color.RGB != "4472C4" {
		t.Fatalf("wholeTbl fill = %+v", out.Fill)
	}
	if !out.Borders.Bottom.Resolved || out.Borders.Bottom.Value.Width != 6350 {
		t.Fatalf("lnB = %+v", out.Borders.Bottom)
	}
	if len(diags) != 0 {
		t.Fatalf("diags = %+v", diags)
	}
}

func TestResolveEffectiveCellStyleUnresolved(t *testing.T) {
	doc, _, env, docs := cellStyleFixture(t)
	tc := cellAt(doc, 0, 1) // 无 tcPr 的裸单元格。
	geom := func(doc *xmlstore.XMLDocument, tc *xmlstore.NodeRecord) (int, int, TableStyleFlags) {
		return 2, 2, TableStyleFlags{}
	}
	// 无样式库 + 无显式定义 → unresolved + 诊断。
	_, diags := ResolveEffectiveCellStyle(doc, tc, 0, 1, CellStyleCtx{Env: env, Docs: docs, Geom: geom})
	if len(diags) == 0 {
		t.Fatal("expected unresolved fill diag")
	}
	// 样式库定义了但无该 styleId。
	lib := idx(t, `<a:tblStyleLst xmlns:a="`+dd+`"><a:tblStyle styleId="X"/></a:tblStyleLst>`)
	out, diags := ResolveEffectiveCellStyle(doc, tc, 0, 1,
		CellStyleCtx{Env: env, Docs: docs, StyleDoc: lib, Part: "/p", Geom: geom})
	if out.StyleResolved || out.StyleID != "S1" {
		t.Fatalf("missing style = %+v", out)
	}
	if len(diags) == 0 {
		t.Fatal("expected STYLE_UNRESOLVED for unknown style id")
	}
}

func TestAncestorOf(t *testing.T) {
	tbl := idx(t, `<a:tbl xmlns:a="`+dd+`"><a:tr><a:tc><a:tcPr/></a:tc></a:tr></a:tbl>`)
	tc := xmlstore.ChildOfKind(tbl, xmlstore.ChildOfKind(tbl, tbl.Root(), ooxmlns.DrawingML, "tr", 0), ooxmlns.DrawingML, "tc", 0)
	if got := AncestorOf(tbl, tc, ooxmlns.DrawingML, "tbl"); got == nil {
		t.Fatal("ancestor found expected")
	}
	if got := AncestorOf(tbl, tbl.Root(), ooxmlns.DrawingML, "tbl"); got != nil {
		t.Fatal("no ancestor for root")
	}
}
