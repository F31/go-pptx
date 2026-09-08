package pptx

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// ---------- GEOM-01：单位、组矩阵、四角边界（方案 §5.2/§8） ----------

// xfrmAttr 便捷构造形状片段：type 为元素名，prefix 是标签前缀。
// 非组形状使用 spPr/a:xfrm 形式。

// unitFrag 构造单个顶层形状的 XML 片段（含 xfrm）。kind ∈ {"sp","pic"}。
func unitFrag(kind, id string, xfrmAttrs, body string) string {
	return `<p:` + kind + `><p:nv` + kindName(kind) + `Pr><p:cNvPr id="` + id + `" name="s` + id + `"/><p:cNv` +
		kindName(kind) + `Pr/><p:nvPr/></p:nv` + kindName(kind) + `Pr>` +
		body + `</p:` + kind + `>`
}

func kindName(kind string) string {
	if kind == "pic" {
		return "Pic"
	}
	return "Sp"
}

// xfrmEl 构造 <a:xfrm ...><a:off/><a:ext/>[<a:chOff/><a:chExt/>]</a:xfrm>。
// （保留给后续直接注入场景；当前测试经 spXfrm/grpXfrm 组装。）

// geomSlide 构造含 body（spTree 内容）的单页文档并打开。
func geomSlide(t *testing.T, spTreeBody string) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		spTreeBody +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
	parts["/[Content_Types].xml"] = []byte(string(parts["/[Content_Types].xml"])[:len(parts["/[Content_Types].xml"])-len("</Types>")] +
		`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`)
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`<Relationship Id="rId2" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	return openFixture(t, buildPackageZipPanic(parts))
}

// spXfrm 构造 p:sp 完整元素（带 spPr/a:xfrm；可选 txBody 文本）。
func spXfrm(id, xfrmAttrs, offX, offY, extCX, extCY string, withText bool) string {
	body := `<p:spPr><a:xfrm` + xfrmAttrs + `>` +
		`<a:off x="` + offX + `" y="` + offY + `"/>` +
		`<a:ext cx="` + extCX + `" cy="` + extCY + `"/>` +
		`</a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`
	if withText {
		body += `<p:txBody><a:bodyPr/><a:p><a:r><a:t>g</a:t></a:r></a:p></p:txBody>`
	}
	return unitFrag("sp", id, "", body)
}

// grpXfrm 构造 p:grpSp 完整元素（含组级 xfrm + 子形状）。
func grpXfrm(id, xfrmAttrs, offX, offY, extCX, extCY, chOffX, chOffY, chExtCX, chExtCY, children string) string {
	body := `<p:grpSpPr><a:xfrm` + xfrmAttrs + `>` +
		`<a:off x="` + offX + `" y="` + offY + `"/>` +
		`<a:ext cx="` + extCX + `" cy="` + extCY + `"/>` +
		`<a:chOff x="` + chOffX + `" y="` + chOffY + `"/>` +
		`<a:chExt cx="` + chExtCX + `" cy="` + chExtCY + `"/>` +
		`</a:xfrm></p:grpSpPr>` +
		children
	return `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="` + id + `" name="g` + id + `"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		body + `</p:grpSp>`
}

func firstShape(t *testing.T, p *Presentation) Shape {
	t.Helper()
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide(0): %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	return shapes[0]
}

func eqEMU(t *testing.T, what string, got, want EMU) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", what, got, want)
	}
}

func eqPoint(t *testing.T, what string, got, want Point) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

func eqQuad(t *testing.T, got, want Quad) {
	t.Helper()
	eqPoint(t, "TL", got.TopLeft, want.TopLeft)
	eqPoint(t, "TR", got.TopRight, want.TopRight)
	eqPoint(t, "BR", got.BottomRight, want.BottomRight)
	eqPoint(t, "BL", got.BottomLeft, want.BottomLeft)
}

// ---------- 单位换算 ----------

func TestEMUConversions(t *testing.T) {
	in, err := EMUFromInches(1)
	if err != nil || in != 914400 {
		t.Errorf("1in = %d, %v; want 914400", in, err)
	}
	pt, err := EMUFromPoints(1)
	if err != nil || pt != 12700 {
		t.Errorf("1pt = %d, %v; want 12700", pt, err)
	}
	if got := (EMU(914400)).Inches(); math.Abs(got-1) > 1e-12 {
		t.Errorf("914400 in inches = %v", got)
	}
	if got := (EMU(12700)).Points(); math.Abs(got-1) > 1e-12 {
		t.Errorf("12700 in points = %v", got)
	}
	// 舍入：0.5pt → 6350。
	if v, err := EMUFromPoints(0.5); err != nil || v != 6350 {
		t.Errorf("0.5pt = %d, %v; want 6350", v, err)
	}
	// 负值合法。
	if v, err := EMUFromInches(-2); err != nil || v != -1828800 {
		t.Errorf("-2in = %d, %v", v, err)
	}
	// 溢出 / 非有限拒绝。
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e300} {
		if _, err := EMUFromInches(bad); err == nil {
			t.Errorf("EMUFromInches(%v) = nil error, want error", bad)
		}
	}
}

// ---------- 顶层形状 Bounds/WorldQuad/WorldAABB ----------

func TestShapeBoundsTopLevel(t *testing.T) {
	p := geomSlide(t, spXfrm("10", "", "1000", "2000", "3000", "1500", false))
	defer p.Close()
	sh := firstShape(t, p)
	shp := sh.(*AutoShape)
	b, err := shp.Bounds()
	if err != nil {
		t.Fatalf("Bounds: %v", err)
	}
	want := Rect{X: 1000, Y: 2000, W: 3000, H: 1500}
	if b != want {
		t.Errorf("Bounds = %+v, want %+v", b, want)
	}
	// 无翻转旋转：世界四角 = 本地四角（页面坐标即本地）。
	q, err := shp.WorldQuad()
	if err != nil {
		t.Fatalf("WorldQuad: %v", err)
	}
	wantQ := Quad{
		TopLeft:     Point{1000, 2000},
		TopRight:    Point{4000, 2000},
		BottomRight: Point{4000, 3500},
		BottomLeft:  Point{1000, 3500},
	}
	eqQuad(t, q, wantQ)
	ab, err := shp.WorldAABB()
	if err != nil {
		t.Fatalf("WorldAABB: %v", err)
	}
	if ab != want {
		t.Errorf("WorldAABB = %+v, want %+v (未旋转 AABB=Bounds)", ab, want)
	}
}

// 负坐标合法（§8）。
func TestShapeNegativeCoordinatesAllowed(t *testing.T) {
	p := geomSlide(t, spXfrm("10", "", "-1000", "-500", "3000", "1500", false))
	defer p.Close()
	sh := firstShape(t, p).(*AutoShape)
	b, err := sh.Bounds()
	if err != nil {
		t.Fatalf("Bounds: %v", err)
	}
	if b.X != -1000 || b.Y != -500 || b.W != 3000 || b.H != 1500 {
		t.Errorf("Bounds = %+v", b)
	}
}

// ---------- 旋转正方向与翻转 ----------

func TestShapeWorldQuadRotationPositiveClockwise(t *testing.T) {
	// rot=90°（5400000）：数学正方向（逆时针）在 y 向下坐标系中
	// 视觉为顺时针，与 ECMA ST_Angle 语义一致（金样待语料确认）。
	// 形状 off=(0,0) ext=(2000,1000)，中心 C=(1000,500)。
	p := geomSlide(t, spXfrm("10", ` rot="5400000"`, "0", "0", "2000", "1000", false))
	defer p.Close()
	sh := firstShape(t, p).(*AutoShape)
	q, err := sh.WorldQuad()
	if err != nil {
		t.Fatalf("WorldQuad: %v", err)
	}
	// 点相对中心 (1000,500) 旋转 +90°（x,y）→(−y,x) 再平移回中心。
	rot := func(x, y int64) Point {
		dx, dy := float64(x-1000), float64(y-500)
		return Point{X: 1000 + EMU(math.Round(-dy)), Y: 500 + EMU(math.Round(dx))}
	}
	want := Quad{
		TopLeft:     rot(0, 0),
		TopRight:    rot(2000, 0),
		BottomRight: rot(2000, 1000),
		BottomLeft:  rot(0, 1000),
	}
	eqQuad(t, q, want)
	// 检查旋转 90° 后 AABB：宽=1000（原高），高=2000（原宽）。
	ab, err := sh.WorldAABB()
	if err != nil {
		t.Fatalf("WorldAABB: %v", err)
	}
	if ab.W != 1000 || ab.H != 2000 {
		t.Errorf("AABB after 90° = %+v, want W=1000 H=2000", ab)
	}
}

func TestShapeWorldQuadFlipH(t *testing.T) {
	// flipH=1：绕形状中心 x 镜像。off=(1000,2000) ext=(2000,1000)，
	// 中心 (2000,2500)。镜像后 x' = 2*2000 - x。
	p := geomSlide(t, spXfrm("10", ` flipH="1"`, "1000", "2000", "2000", "1000", false))
	defer p.Close()
	sh := firstShape(t, p).(*AutoShape)
	q, err := sh.WorldQuad()
	if err != nil {
		t.Fatalf("WorldQuad: %v", err)
	}
	want := Quad{
		TopLeft:     Point{3000, 2000}, // 原 TR
		TopRight:    Point{1000, 2000}, // 原 TL
		BottomRight: Point{1000, 3000},
		BottomLeft:  Point{3000, 3000},
	}
	eqQuad(t, q, want)
}

// ---------- 组映射（Mgroup=T(C)·R·F·T(-C)·G） ----------

// 无翻转旋转的组：x' = off.x + (x - chOff.x)×ext.cx/chExt.cx（§8）。
func TestGroupScaleTranslate(t *testing.T) {
	// 组框：off=(10000,20000) ext=(1000,500)；
	// 子坐标 chOff=(0,0) chExt=(2000,1000) → 等比缩放 0.5。
	// 子形状在组坐标 (400,200)-(1400,700)。
	child := spXfrm("20", "", "400", "200", "1000", "500", false)
	grp := grpXfrm("15", "", "10000", "20000", "1000", "500", "0", "0", "2000", "1000", child)
	p := geomSlide(t, grp)
	defer p.Close()
	s, err := p.Slide(0)
	if err != nil {
		t.Fatalf("Slide: %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	g := shapes[0].(*GroupShape)
	// 组 Bounds = 组框 off/ext。
	b, err := g.Bounds()
	if err != nil {
		t.Fatalf("group Bounds: %v", err)
	}
	if b != (Rect{X: 10000, Y: 20000, W: 1000, H: 500}) {
		t.Errorf("group Bounds = %+v", b)
	}
	kids, err := g.Children()
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(kids) != 1 {
		t.Fatalf("children = %d", len(kids))
	}
	sub := kids[0].(*AutoShape)
	// 子形状本地框仍为组坐标值（不应用组矩阵）。
	sb, err := sub.Bounds()
	if err != nil {
		t.Fatalf("child Bounds: %v", err)
	}
	if sb != (Rect{X: 400, Y: 200, W: 1000, H: 500}) {
		t.Errorf("child Bounds = %+v (本地=组坐标)", sb)
	}
	// 世界映射：x' = 10000 + 0.5x, y' = 20000 + 0.5y。
	q, err := sub.WorldQuad()
	if err != nil {
		t.Fatalf("child WorldQuad: %v", err)
	}
	want := Quad{
		TopLeft:     Point{10000 + 400/2, 20000 + 200/2},
		TopRight:    Point{10000 + (400+1000)/2, 20000 + 200/2},
		BottomRight: Point{10000 + (400+1000)/2, 20000 + (200+500)/2},
		BottomLeft:  Point{10000 + 400/2, 20000 + (200+500)/2},
	}
	eqQuad(t, q, want)
}

// 组翻转：flipH 绕组中心镜像（在 G 之后应用）。
func TestGroupFlipHAfterMap(t *testing.T) {
	// 组：off=(0,0) ext=(2000,1000) flipH；chOff=0 chExt 同 ext → G 恒等。
	child := spXfrm("20", "", "400", "200", "1000", "500", false)
	grp := grpXfrm("15", ` flipH="1"`, "0", "0", "2000", "1000", "0", "0", "2000", "1000", child)
	p := geomSlide(t, grp)
	defer p.Close()
	s, _ := p.Slide(0)
	shapes, _ := s.Shapes()
	kids, _ := shapes[0].(*GroupShape).Children()
	sub := kids[0].(*AutoShape)
	q, err := sub.WorldQuad()
	if err != nil {
		t.Fatalf("WorldQuad: %v", err)
	}
	// 组中心 C=(1000,500)。子形状经 G(恒等) 后原四角，再 x 镜像绕 C。
	want := Quad{
		TopLeft:     Point{1600, 200}, // 原 (400,200) → 2000-400=1600
		TopRight:    Point{600, 200},  // 原 (1400,200) → 2000-1400=600
		BottomRight: Point{600, 700},
		BottomLeft:  Point{1600, 700},
	}
	eqQuad(t, q, want)
}

// 非等比组缩放 + 子自身旋转的组合顺序。
func TestGroupNonUniformScaleChildRotation(t *testing.T) {
	// 组非等比：ext=(2000,1000)，chExt=(4000,1000) → sx=0.5 sy=1。
	// 子形状自身 rot=90°（绕自身中心旋转）——先子翻转旋转、后组映射。
	child := spXfrm("20", ` rot="5400000"`, "0", "0", "2000", "1000", false)
	grp := grpXfrm("15", "", "0", "0", "2000", "1000", "0", "0", "4000", "1000", child)
	p := geomSlide(t, grp)
	defer p.Close()
	s, _ := p.Slide(0)
	shapes, _ := s.Shapes()
	kids, _ := shapes[0].(*GroupShape).Children()
	sub := kids[0].(*AutoShape)
	q, err := sub.WorldQuad()
	if err != nil {
		t.Fatalf("WorldQuad: %v", err)
	}
	// 期望：子坐标四角先在子中心 (1000,500) 旋转 90°，再组映射
	// G(x,y)=(0.5x, y)。旋转后四角（组坐标）：
	//   TL(0,0)→(1000,1500)? 手算：
	// 绕 C=(1000,500)：(x,y)→(C.x-(y-C.y), C.y+(x-C.x))
	rotQ := Quad{
		TopLeft:     Point{1000 - (0 - 500), 500 + (0 - 1000)},       // (1500,-500)
		TopRight:    Point{1000 - (0 - 500), 500 + (2000 - 1000)},    // (1500,1500)
		BottomRight: Point{1000 - (1000 - 500), 500 + (2000 - 1000)}, // (500,1500)
		BottomLeft:  Point{1000 - (1000 - 500), 500 + (0 - 1000)},    // (500,-500)
	}
	_ = rotQ
	// G 应用：(x,y)→(x/2, y)。上面角点 → (750,-500),(750,1500),(250,1500),(250,-500)。
	want := Quad{
		TopLeft:     Point{1500 / 2, -500},
		TopRight:    Point{1500 / 2, 1500},
		BottomRight: Point{500 / 2, 1500},
		BottomLeft:  Point{500 / 2, -500},
	}
	eqQuad(t, q, want)
}

// 嵌套组：父组矩阵左乘。
func TestNestedGroupLeftMultiply(t *testing.T) {
	// 外层组 G1：off=(1000,1000) ext=(2000,1000) chOff=(0,0) chExt=(2000,1000) → 恒等 G + 平移到 (1000,1000)。
	// 内层组 G2（G1 子坐标内）：off=(0,0) ext=(1000,500) chOff=(0,0) chExt=(1000,500) → 恒等。
	// 叶子（G2 子坐标）：off=(100,100) ext=(400,200)。
	leaf := spXfrm("30", "", "100", "100", "400", "200", false)
	g2 := grpXfrm("25", "", "0", "0", "1000", "500", "0", "0", "1000", "500", leaf)
	g1 := grpXfrm("20", "", "1000", "1000", "2000", "1000", "0", "0", "2000", "1000", g2)
	p := geomSlide(t, g1)
	defer p.Close()
	s, _ := p.Slide(0)
	shapes, _ := s.Shapes()
	outer := shapes[0].(*GroupShape)
	kids, _ := outer.Children()
	if len(kids) != 1 || kids[0].Kind() != ShapeGroup {
		t.Fatalf("outer children = %d kinds, want 1 group", len(kids))
	}
	inner := kids[0].(*GroupShape)
	leaves, _ := inner.Children()
	if len(leaves) != 1 {
		t.Fatalf("inner children = %d", len(leaves))
	}
	q, err := leaves[0].WorldQuad()
	if err != nil {
		t.Fatalf("leaf WorldQuad: %v", err)
	}
	want := Quad{
		TopLeft:     Point{1100, 1100},
		TopRight:    Point{1500, 1100},
		BottomRight: Point{1500, 1300},
		BottomLeft:  Point{1100, 1300},
	}
	eqQuad(t, q, want)
}

// 零 chExt 拒绝（§8：禁止除法，不默认 0）。
func TestGroupZeroChExtRejected(t *testing.T) {
	child := spXfrm("20", "", "400", "200", "1000", "500", false)
	grp := grpXfrm("15", "", "10000", "20000", "1000", "500", "0", "0", "0", "1000", child)
	p := geomSlide(t, grp)
	defer p.Close()
	s, _ := p.Slide(0)
	shapes, _ := s.Shapes()
	g := shapes[0].(*GroupShape)
	// 组自身 Bounds 不除 chExt，仍可用。
	if _, err := g.Bounds(); err != nil {
		t.Fatalf("group Bounds (chExt=0) = %v", err)
	}
	kids, err := g.Children()
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	sub := kids[0].(*AutoShape)
	// 子世界映射需除 chExt → 拒绝。
	if _, err := sub.WorldQuad(); !errors.Is(err, ErrMalformedPackage) {
		t.Errorf("zero chExt WorldQuad err = %v, want ErrMalformedPackage", err)
	}
	if _, err := sub.WorldAABB(); !errors.Is(err, ErrMalformedPackage) {
		t.Errorf("zero chExt WorldAABB err = %v, want ErrMalformedPackage", err)
	}
	// 子本地 Bounds 无需组矩阵 → 可用。
	if _, err := sub.Bounds(); err != nil {
		t.Errorf("child Bounds (chExt=0) = %v, want nil", err)
	}
}

// 无 xfrm 的形状：几何查询返回 ErrNotFound。
func TestShapeNoXfrmNotFound(t *testing.T) {
	body := `<p:sp><p:nvSpPr><p:cNvPr id="5" name="nofrm"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/></p:sp>`
	p := geomSlide(t, body)
	defer p.Close()
	sh := firstShape(t, p).(*AutoShape)
	if _, err := sh.Bounds(); !errors.Is(err, ErrNotFound) {
		t.Errorf("Bounds no-xfrm err = %v, want ErrNotFound", err)
	}
	if _, err := sh.WorldQuad(); !errors.Is(err, ErrNotFound) {
		t.Errorf("WorldQuad no-xfrm err = %v, want ErrNotFound", err)
	}
}

// ---------- GroupShape.Children 生命周期与 Save 往返 ----------

func TestGroupShapeChildrenEnumerateNested(t *testing.T) {
	child1 := spXfrm("30", "", "100", "100", "400", "200", false)
	child2 := spXfrm("31", "", "600", "300", "200", "100", false)
	g2 := grpXfrm("25", "", "0", "0", "1000", "500", "0", "0", "1000", "500", child1+child2)
	g1 := grpXfrm("20", "", "1000", "1000", "2000", "1000", "0", "0", "2000", "1000", g2)
	p := geomSlide(t, g1)
	defer p.Close()
	s, _ := p.Slide(0)
	shapes, _ := s.Shapes()
	outer := shapes[0].(*GroupShape)
	kids, err := outer.Children()
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(kids) != 1 || kids[0].Kind() != ShapeGroup {
		t.Fatalf("outer children kinds = %d, want nested group", len(kids))
	}
	inner := kids[0].(*GroupShape)
	leaves, err := inner.Children()
	if err != nil {
		t.Fatalf("inner Children: %v", err)
	}
	if len(leaves) != 2 {
		t.Fatalf("inner children = %d, want 2 (z-order)", len(leaves))
	}
	if leaves[0].ID() != 30 || leaves[1].ID() != 31 {
		t.Errorf("leaf ids = %d,%d", leaves[0].ID(), leaves[1].ID())
	}
}

func TestGeometryStaleAndClosed(t *testing.T) {
	p := geomSlide(t, spXfrm("10", "", "0", "0", "1000", "500", false))
	s, _ := p.Slide(0)
	shapes, _ := s.Shapes()
	sh := shapes[0].(*AutoShape)
	if err := p.RemoveSlide(s.ID()); err != nil {
		t.Fatalf("RemoveSlide: %v", err)
	}
	if _, err := sh.Bounds(); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("stale Bounds err = %v, want ErrStaleHandle", err)
	}
	if _, err := sh.WorldQuad(); !errors.Is(err, ErrStaleHandle) {
		t.Errorf("stale WorldQuad err = %v, want ErrStaleHandle", err)
	}

	p2 := geomSlide(t, spXfrm("10", "", "0", "0", "1000", "500", false))
	s2, _ := p2.Slide(0)
	shapes2, _ := s2.Shapes()
	sh2 := shapes2[0].(*AutoShape)
	if err := p2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := sh2.WorldQuad(); !errors.Is(err, ErrClosed) {
		t.Errorf("closed WorldQuad err = %v, want ErrClosed", err)
	}
}

// saveBytes 把文档序列化为包字节（测试便捷）。
func saveBytes(t *testing.T, p *Presentation) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

// Save 往返：几何数值与 XML 保真一致（重新打开后 xfrm 原样）。
func TestGeometrySaveRoundTrip(t *testing.T) {
	child := spXfrm("20", ` rot="2700000" flipV="1"`, "400", "200", "1000", "500", false)
	grp := grpXfrm("15", ` flipH="1"`, "1000", "2000", "800", "600", "0", "0", "1600", "600", child)
	p := geomSlide(t, grp)
	s, _ := p.Slide(0)
	shapes, _ := s.Shapes()
	g := shapes[0].(*GroupShape)
	kids, _ := g.Children()
	qBefore, err := kids[0].WorldQuad()
	if err != nil {
		t.Fatalf("WorldQuad before: %v", err)
	}
	data := saveBytes(t, p)
	p.Close()

	p2 := openFixture(t, data)
	defer p2.Close()
	s2, _ := p2.Slide(0)
	shapes2, _ := s2.Shapes()
	g2 := shapes2[0].(*GroupShape)
	kids2, _ := g2.Children()
	qAfter, err := kids2[0].WorldQuad()
	if err != nil {
		t.Fatalf("WorldQuad after: %v", err)
	}
	if qBefore != qAfter {
		t.Errorf("quad changed across save: before %+v after %+v", qBefore, qAfter)
	}
}
