package pptx

import (
	"fmt"
	"math"
	"strconv"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 GEOM-01（方案 §5.2 单位、§8 组矩阵与四角边界）：
//
//   - EMU 单位与 XML 单位集中换算（舍入 + 溢出检查）；
//   - 3×3 仿射矩阵（列向量约定）与形状 xfrm 解析；
//   - 组映射 G 与中心旋转/翻转组合顺序 Mgroup=T(C)·R·F·T(-C)·G；
//   - 嵌套组按父矩阵左乘；
//   - 形状几何查询：Bounds()/WorldQuad()/WorldAABB()；
//   - 零 chExt 拒绝（不默认为 0）；负坐标合法。
//
// 坐标轴方向与旋转正方向（§8）：本库沿用 OOXML 约定——页面坐标
// y 轴向下增长；a:xfrm@rot 以 1/60000 度为单位，正值在 y 向下坐标系
// 中表现为顺时针（与 PowerPoint 视觉一致）。该正方向约定待真实语料
// 金样最终确认（实施计划 M3 GEOM-01 验收项），当前实现与 ECMA-376
// ST_Angle 语义一致。

// ---------- 单位（§5.2） ----------

// EMU 是 OOXML 长度单位（English Metric Unit，int64）。
// 1 in = 914400 EMU，1 pt = 12700 EMU；形状坐标、尺寸均以 EMU 表示。
type EMU int64

const (
	// EMUPerInch 是一英寸的 EMU 值。
	EMUPerInch EMU = 914400
	// EMUPerPoint 是一磅（1/72 in）的 EMU 值。
	EMUPerPoint EMU = 12700
	// anglePerDegree 是 a:xfrm@rot 一角度对应的属性值（1/60000 度单位）。
	anglePerDegree int64 = 60000
)

// EMUFromInches 把英寸值换算为 EMU（四舍五入）；NaN/Inf 或结果超出
// int64 范围返回 error。
func EMUFromInches(v float64) (EMU, error) {
	return scaleEMU(v, float64(EMUPerInch))
}

// EMUFromPoints 把磅值换算为 EMU（四舍五入）；NaN/Inf 或结果超出
// int64 范围返回 error。
func EMUFromPoints(v float64) (EMU, error) {
	return scaleEMU(v, float64(EMUPerPoint))
}

func scaleEMU(v, per float64) (EMU, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, &OperationError{Op: "EMU conversion", Message: fmt.Sprintf("non-finite value %v", v), Err: ErrInvalidArgument}
	}
	r := math.Round(v * per)
	if r > math.MaxInt64 || r < math.MinInt64 {
		return 0, &OperationError{Op: "EMU conversion", Message: fmt.Sprintf("value %v overflows EMU", v), Err: ErrLimitExceeded}
	}
	return EMU(int64(r)), nil
}

// Inches 返回英寸表示的浮点值。
func (e EMU) Inches() float64 { return float64(e) / float64(EMUPerInch) }

// Points 返回磅表示的浮点值。
func (e EMU) Points() float64 { return float64(e) / float64(EMUPerPoint) }

// ---------- 几何值类型 ----------

// Point 是 EMU 坐标点（y 向下为正）。
type Point struct {
	X EMU
	Y EMU
}

// Rect 是轴对齐矩形（X/Y 为左上角，W/H 为宽高，非负）。
type Rect struct {
	X EMU
	Y EMU
	W EMU
	H EMU
}

// Right 返回右边界（X+W）。
func (r Rect) Right() EMU { return r.X + r.W }

// Bottom 返回下边界（Y+H）。
func (r Rect) Bottom() EMU { return r.Y + r.H }

// Quad 是变换后的四角，顺序固定为左上、右上、右下、左下
// （对未旋转矩形即原始四角；旋转/翻转后仍按角点跟随顺序）。
type Quad struct {
	TopLeft     Point
	TopRight    Point
	BottomRight Point
	BottomLeft  Point
}

// Contains 报告点是否位于矩形内（含边界；W/H 为负视为空）。
func (r Rect) Contains(p Point) bool {
	if r.W < 0 || r.H < 0 {
		return false
	}
	return p.X >= r.X && p.X <= r.Right() && p.Y >= r.Y && p.Y <= r.Bottom()
}

// aabb 返回四角的轴对齐包围框（min/max 收敛）。
func (q Quad) aabb() Rect {
	minX, minY := q.TopLeft.X, q.TopLeft.Y
	maxX, maxY := q.TopLeft.X, q.TopLeft.Y
	for _, p := range []Point{q.TopRight, q.BottomRight, q.BottomLeft} {
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}

// ---------- 3×3 仿射矩阵（列向量） ----------

// affine 是 3×3 仿射矩阵（列向量约定，行存储为显式分量）：
//
//	[ x' ]   [ a  c  e ] [ x ]
//	[ y' ] = [ b  d  f ] [ y ]
//	[ 1  ]   [ 0  0  1 ] [ 1 ]
//
// 组合使用 compose：compose(A, B) 表示先应用 B 再应用 A（A∘B）。
type affine struct {
	a, b, c, d, e, f float64
}

// affineIdent 返回单位矩阵。
func affineIdent() affine { return affine{a: 1, d: 1} }

// affineCompose 返回 A∘B（先 B 后 A）。
func affineCompose(A, B affine) affine {
	return affine{
		a: A.a*B.a + A.c*B.b,
		b: A.b*B.a + A.d*B.b,
		c: A.a*B.c + A.c*B.d,
		d: A.b*B.c + A.d*B.d,
		e: A.a*B.e + A.c*B.f + A.e,
		f: A.b*B.e + A.d*B.f + A.f,
	}
}

// affineTrans 返回平移矩阵 T(tx,ty)。
func affineTrans(tx, ty float64) affine {
	m := affineIdent()
	m.e, m.f = tx, ty
	return m
}

// affineScale 返回以原点为中心的缩放矩阵（S 对角）。
func affineScale(sx, sy float64) affine {
	return affine{a: sx, d: sy}
}

// affineRot 返回旋转矩阵 R(θ)（弧度；数学正方向，y 向下坐标系中
// 视觉为顺时针，与 OOXML rot 正值一致）。
func affineRot(rad float64) affine {
	co, si := math.Cos(rad), math.Sin(rad)
	return affine{a: co, b: si, c: -si, d: co}
}

// affineFlipH 返回水平翻转矩阵（关于 y 轴镜像：x → -x）。
func affineFlipH() affine { return affine{a: -1, d: 1} }

// affineFlipV 返回垂直翻转矩阵（关于 x 轴镜像：y → -y）。
func affineFlipV() affine { return affine{a: 1, d: -1} }

// apply 把矩阵作用于点 (x,y)。
func (m affine) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// rotRadians 把 a:xfrm@rot 属性值（1/60000 度）转为弧度。
func rotRadians(rot int64) float64 {
	return float64(rot) * math.Pi / (float64(anglePerDegree) * 180.0)
}

// ---------- xfrm 解析 ----------

// shapeXfrm 是解析后的形状变换（a:xfrm / p:xfrm）。
// ch* 字段仅在组（grpSp）存在；叶子形状 hasCh=false。
type shapeXfrm struct {
	offX, offY EMU
	extCX      EMU
	extCY      EMU
	hasCh      bool
	chOffX     EMU
	chOffY     EMU
	chExtCX    EMU
	chExtCY    EMU
	rot        int64 // 1/60000 度
	flipH      bool
	flipV      bool
}

// flipRotAffine 返回围绕自身 ext 框中心 C=off+ext/2 的翻转与旋转
// 组合 T(C)·R·F·T(-C)。无翻转旋转时为单位矩阵。
func (x *shapeXfrm) flipRotAffine() affine {
	rad := rotRadians(x.rot)
	cx := float64(x.offX) + float64(x.extCX)/2
	cy := float64(x.offY) + float64(x.extCY)/2
	F := affineIdent()
	if x.flipH {
		F = affineCompose(affineFlipH(), F)
	}
	if x.flipV {
		F = affineCompose(affineFlipV(), F)
	}
	R := affineRot(rad)
	// T(C)·R·F·T(-C)
	m := affineTrans(cx, cy)
	m = affineCompose(m, R)
	m = affineCompose(m, F)
	m = affineCompose(m, affineTrans(-cx, -cy))
	return m
}

// groupAffine 返回组映射（把组内子坐标映射到组父坐标）：
//
//	Mgroup = T(C)·R·F·T(-C)·G，G = T(off)·S·T(-chOff)
//
// S 为非等比缩放 diag(ext.cx/chExt.cx, ext.cy/chExt.cy)；C 为组 ext
// 框中心 off+ext/2。chExt 任一维度为零时返回错误（不把除法结果默认为
// 0，§8）。
func (x *shapeXfrm) groupAffine() (affine, error) {
	if !x.hasCh {
		return affine{}, &OperationError{
			Op: "group transform", Message: "xfrm has no chOff/chExt (not a group)", Err: ErrMalformedPackage,
		}
	}
	if x.chExtCX == 0 || x.chExtCY == 0 {
		return affine{}, &OperationError{
			Op:      "group transform",
			Message: fmt.Sprintf("zero child extent chExt=(%d,%d) refuses division", x.chExtCX, x.chExtCY),
			Err:     ErrMalformedPackage,
		}
	}
	sx := float64(x.extCX) / float64(x.chExtCX)
	sy := float64(x.extCY) / float64(x.chExtCY)
	// G = T(off)·S·T(-chOff)
	G := affineCompose(affineTrans(float64(x.offX), float64(x.offY)),
		affineCompose(affineScale(sx, sy), affineTrans(-float64(x.chOffX), -float64(x.chOffY))))
	// M = T(C)·R·F·T(-C)·G
	rad := rotRadians(x.rot)
	cx := float64(x.offX) + float64(x.extCX)/2
	cy := float64(x.offY) + float64(x.extCY)/2
	F := affineIdent()
	if x.flipH {
		F = affineCompose(affineFlipH(), F)
	}
	if x.flipV {
		F = affineCompose(affineFlipV(), F)
	}
	M := affineCompose(affineTrans(cx, cy), affineCompose(affineRot(rad),
		affineCompose(F, affineCompose(affineTrans(-cx, -cy), G))))
	return M, nil
}

// bounds 返回形状本地框（off/ext，不含翻转旋转）。
func (x *shapeXfrm) bounds() Rect {
	return Rect{X: x.offX, Y: x.offY, W: x.extCX, H: x.extCY}
}

// xfrmNode 返回形状元素对应的变换容器节点：
//
//	p:grpSp        → grpSpPr/a:xfrm（含 chOff/chExt）
//	p:graphicFrame → p:xfrm 直接子元素（CT_Transform2D，无 ch）
//	p:sp/p:pic/p:cxnSp → spPr/a:xfrm
//
// 未知容器（ShapeOpaque 其它）尝试 spPr/a:xfrm；均无则返回 nil。
func xfrmNode(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	switch el.Local() {
	case "grpSp":
		gp := childOfKind(doc, el, nsPresentationML, "grpSpPr", 0)
		if gp == nil {
			return nil
		}
		return childOfKind(doc, gp, nsDrawingML, "xfrm", 0)
	case "graphicFrame":
		if n := childOfKind(doc, el, nsPresentationML, "xfrm", 0); n != nil {
			return n
		}
		return childOfKind(doc, el, nsDrawingML, "xfrm", 0)
	default:
		sp := childOfKind(doc, el, nsPresentationML, "spPr", 0)
		if sp == nil {
			return nil
		}
		return childOfKind(doc, sp, nsDrawingML, "xfrm", 0)
	}
}

// parseXfrm 从变换容器解析几何。isGroup 决定是否要求并解析 chOff/chExt。
// off/ext 缺失、属性缺失、ext 非正均视为包结构非法。
func parseXfrm(doc *xmlstore.XMLDocument, x *xmlstore.NodeRecord, isGroup bool) (*shapeXfrm, error) {
	out := &shapeXfrm{hasCh: isGroup}
	if x == nil {
		return nil, &OperationError{Op: "xfrm", Message: "shape has no transform", Err: ErrNotFound}
	}
	off := childOfKind(doc, x, nsDrawingML, "off", 0)
	ext := childOfKind(doc, x, nsDrawingML, "ext", 0)
	if off == nil || ext == nil {
		return nil, &OperationError{
			Op: "xfrm", Message: "transform missing a:off/a:ext", Err: ErrMalformedPackage,
		}
	}
	var err error
	if out.offX, out.offY, err = coordPair(off, "x", "y"); err != nil {
		return nil, err
	}
	if out.extCX, out.extCY, err = coordPair(ext, "cx", "cy"); err != nil {
		return nil, err
	}
	if out.extCX <= 0 || out.extCY <= 0 {
		return nil, &OperationError{
			Op: "xfrm", Message: fmt.Sprintf("non-positive extent (%d,%d)", out.extCX, out.extCY),
			Err: ErrMalformedPackage,
		}
	}
	if isGroup {
		chOff := childOfKind(doc, x, nsDrawingML, "chOff", 0)
		chExt := childOfKind(doc, x, nsDrawingML, "chExt", 0)
		if chOff == nil || chExt == nil {
			return nil, &OperationError{
				Op: "xfrm", Message: "group transform missing a:chOff/a:chExt", Err: ErrMalformedPackage,
			}
		}
		if out.chOffX, out.chOffY, err = coordPair(chOff, "x", "y"); err != nil {
			return nil, err
		}
		if out.chExtCX, out.chExtCY, err = coordPair(chExt, "cx", "cy"); err != nil {
			return nil, err
		}
		// chExt 为零不在此拒绝（Bounds 仍可用）；仅在计算组矩阵时拒绝。
	}
	if v, ok := x.Attr("", "rot"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.rot = n
		} else {
			return nil, &OperationError{
				Op: "xfrm", Message: fmt.Sprintf("invalid rot attribute %q", v), Err: ErrMalformedPackage,
			}
		}
	}
	if v, ok := x.Attr("", "flipH"); ok {
		out.flipH = v == "1"
	}
	if v, ok := x.Attr("", "flipV"); ok {
		out.flipV = v == "1"
	}
	return out, nil
}

// coordPair 解析两属性为 EMU（允许负坐标）。
func coordPair(n *xmlstore.NodeRecord, a1, a2 string) (EMU, EMU, error) {
	s1, ok1 := n.Attr("", a1)
	s2, ok2 := n.Attr("", a2)
	if !ok1 || !ok2 {
		return 0, 0, &OperationError{
			Op: "xfrm", Message: fmt.Sprintf("%s missing attributes %s/%s", n.Local(), a1, a2),
			Err: ErrMalformedPackage,
		}
	}
	v1, e1 := strconv.ParseInt(s1, 10, 64)
	v2, e2 := strconv.ParseInt(s2, 10, 64)
	if e1 != nil || e2 != nil {
		return 0, 0, &OperationError{
			Op: "xfrm", Message: fmt.Sprintf("invalid coordinate %q/%q", s1, s2), Err: ErrMalformedPackage,
		}
	}
	return EMU(v1), EMU(v2), nil
}

// shapeXfrmOf 定位并解析形状元素（自身）的变换。无 xfrm → ErrNotFound。
func shapeXfrmOf(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) (*shapeXfrm, error) {
	xn := xfrmNode(doc, el)
	if xn == nil {
		return nil, &OperationError{
			Op: "shape geometry", Message: fmt.Sprintf("shape %s has no transform", el.Local()), Err: ErrNotFound,
		}
	}
	return parseXfrm(doc, xn, el.Local() == "grpSp")
}

// ---------- 形状几何查询（GEOM-01） ----------

// ancestorGroups 自近（直接父）向远收集形状的全部祖先 p:grpSp。
func ancestorGroups(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) []*xmlstore.NodeRecord {
	var out []*xmlstore.NodeRecord
	cur := el
	for cur != nil && cur.Parent != xmlstore.NoNode {
		par := doc.Node(cur.Parent)
		if par == nil {
			break
		}
		if par.Namespace == nsPresentationML && par.Local() == "grpSp" {
			out = append(out, par)
		}
		cur = par
	}
	return out
}

// worldAffine 计算形状的世界矩阵。点变换链：形状自身内容坐标 →
// 直接父组（近）→ … → 最外层组（远）→ 页面坐标。组映射按父矩阵
// 左乘：p_world = M_最外 ∘ … ∘ M_直接父 ∘ M_自身（§8）。故从近到远
// 逐层把组矩阵组合到左侧。
func worldAffine(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord, self *shapeXfrm) (affine, error) {
	// 自身：先翻转旋转（叶子的本地旋转、翻转在其自身坐标阶段组合，
	// 不重复应用组变换；组自身作为被查询形状时同此处理）。
	M := self.flipRotAffine()
	anc := ancestorGroups(doc, el)
	for i := 0; i < len(anc); i++ { // 近（直接父）→ 远（最外层）
		xg, err := shapeXfrmOf(doc, anc[i])
		if err != nil {
			return affine{}, err
		}
		Mg, err := xg.groupAffine()
		if err != nil {
			return affine{}, err
		}
		M = affineCompose(Mg, M)
	}
	return M, nil
}

// shapeQuad 把本地框四角经矩阵 M 变换为世界四角。
func shapeQuad(M affine, r Rect) Quad {
	x0, y0 := float64(r.X), float64(r.Y)
	x1, y1 := float64(r.Right()), float64(r.Bottom())
	q := Quad{}
	px, py := M.apply(x0, y0)
	q.TopLeft = Point{X: EMU(math.Round(px)), Y: EMU(math.Round(py))}
	px, py = M.apply(x1, y0)
	q.TopRight = Point{X: EMU(math.Round(px)), Y: EMU(math.Round(py))}
	px, py = M.apply(x1, y1)
	q.BottomRight = Point{X: EMU(math.Round(px)), Y: EMU(math.Round(py))}
	px, py = M.apply(x0, y1)
	q.BottomLeft = Point{X: EMU(math.Round(px)), Y: EMU(math.Round(py))}
	return q
}

// Bounds 返回形状的本地框（直接父坐标系内、未经任何翻转旋转的
// off/ext 轴对齐矩形；§8）。形状无 a:xfrm 返回 ErrNotFound。
//
// "本地"相对形状直接所属的 spTree：顶层形状即页面坐标；组内形状为
// 组坐标系（经祖先组 G 映射后才是页面坐标，见 WorldQuad）。框边界
// 不含阴影/发光/描边膨胀。
func (s *shapeNode) Bounds() (Rect, error) {
	doc, el, err := s.locate()
	if err != nil {
		return Rect{}, Annotate(err, "shape.Bounds")
	}
	x, err := shapeXfrmOf(doc, el)
	if err != nil {
		return Rect{}, err
	}
	return x.bounds(), nil
}

// WorldQuad 返回形状内容框（本地框）在世界坐标系中的四角。世界指
// 页面坐标系：逐层应用祖先组的非等比缩放/平移（G）与组级翻转旋转
// （Mgroup），再组合形状自身翻转旋转。组内形状经完整链映射后即页面
// 坐标；四角顺序固定为左上/右上/右下/左下。
//
// 祖先组 chExt 为零时返回 ErrMalformedPackage（禁止除零，不默认为 0）。
func (s *shapeNode) WorldQuad() (Quad, error) {
	doc, el, err := s.locate()
	if err != nil {
		return Quad{}, Annotate(err, "shape.WorldQuad")
	}
	self, err := shapeXfrmOf(doc, el)
	if err != nil {
		return Quad{}, err
	}
	M, err := worldAffine(doc, el, self)
	if err != nil {
		return Quad{}, err
	}
	return shapeQuad(M, self.bounds()), nil
}

// WorldAABB 返回 WorldQuad 的轴对齐包围框（世界坐标 min/max）。
func (s *shapeNode) WorldAABB() (Rect, error) {
	q, err := s.WorldQuad()
	if err != nil {
		return Rect{}, err
	}
	return q.aabb(), nil
}
