package pptx

// 本文件实现 GEOM-02（方案 §2.3 V2.6 矩阵 M6 + 24 章工作包收口）：
//
//	a:prstGeom（190+ preset 及 adjust 公式全集）→ GeometryInfo    R 档
//	a:custGeom（路径、guide 求值）              → GeometryInfo    R 档
//	a:effectLst/effectDag（阴影/发光/反射/柔边）→ EffectInfo     R 档
//	a:scene3d / a:sp3d                          → EffectInfo     R 档
//	a:gradFill（gsLst + 线性/路径渐变）          → FillInfo       R 档
//	a:pattFill                                  → FillInfo       R 档
//	a:blipFill / a:grpFill / noFill / solidFill → FillInfo       R 档
//	线条系统（a:ln 已在 format.go 实现，M3）     ——               E 档
//
// 本文件不引入任何写入 API（方案 §6.1 契约 + V2.6 §1108 治理结论）；
// 仅通过 Shape 接口三个新方法输出只读数据与诊断：
//
//	Shape.Geometry()  → GeometryInfo
//	Shape.Fill()      → FillInfo
//	Shape.Effects()   → EffectInfo
//
// 解析期原则（与 LAYOUT-01 一致）：
//   - 解析失败绝不返回 error，只把问题降级为 Diagnostic（Warning/Error）。
//   - 缺失元素（无 prstGeom/custGeom/fill/effectLst）记为"空"而非异常。
//   - 未知子元素保留原始 Local 名记入对应结构的 Unknown 字段。

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- 几何 GeometryInfo ----------

// GeometryKind 描述 spPr/a:prstGeom 或 a:custGeom 的类型。
type GeometryKind int

const (
	// GeometryPreset 是 a:prstGeom（预置几何，含 adjust 公式）。
	GeometryPreset GeometryKind = iota
	// GeometryCustom 是 a:custGeom（自定义路径 + guide）。
	GeometryCustom
	// GeometryUnknown 是缺失或未识别的几何容器。
	GeometryUnknown
)

func (k GeometryKind) String() string {
	switch k {
	case GeometryPreset:
		return "preset"
	case GeometryCustom:
		return "custom"
	}
	return "unknown"
}

// GeometryInfo 是形状几何（a:prstGeom / a:custGeom）的解析结果。
// 零值即"未声明几何"——大多数容器（p:grpSp / 未知 OpaqueShape）适用。
type GeometryInfo struct {
	// Kind 是几何类别（prstGeom / custGeom / 未知）。
	Kind GeometryKind
	// Preset 是 a:prstGeom@prst 的预设名（如 "rect"、"ellipse"、"roundRect"）。
	// Kind=GeometryPreset 时有效；custom/unknown 时为空。
	Preset string
	// Adjusts 是 a:prstGeom/a:gdLst 内所有 a:gd 的 name→val（千分比/原始）。
	// 仅 prstGeom 有效；custGeom 不使用。
	Adjusts []GeomAdjust
	// Guides 是 a:custGeom/a:gdLst 内 a:gd 的全集（公式文本）。
	// 仅 custGeom 有效。
	Guides []GeomGuide
	// Paths 是 a:custGeom/a:pathLst 内每条 a:path 的命令序列。
	// 仅 custGeom 有效。
	Paths []GeomPath
	// Unknown 是未识别的子元素 Local 名（按文档序）。
	Unknown []string
	// Diagnostics 是解析期产生的诊断条目。
	Diagnostics []Diagnostic
}

// GeomAdjust 是预置几何调整值（a:gd）。
type GeomAdjust struct {
	// Name 是 a:gd@name（如 "adj"、"adj1"、"adj2"）。
	Name string
	// Fmla 是 a:gd@fmla 的原始公式文本（如 "val 25000"）。
	Fmla string
}

// GeomGuide 是自定义几何导引公式（a:gd / a:guide）。
type GeomGuide struct {
	// Name 是 a:gd@name（如 "myGuide"）。
	Name string
	// Fmla 是 a:gd@fmla 公式文本。
	Fmla string
}

// GeomPath 是 a:custGeom/a:pathLst/a:path 一条路径。
type GeomPath struct {
	// Width / Height 是路径所属虚拟框尺寸（EMU）。
	Width, Height EMU
	// Fill 是路径的填充模式（"none" / "solid" / "norm" / "lighten" / ...）；
	// 来自 a:path@fill。
	Fill string
	// Stroke 是路径描边模式（"none" / "solid" / ...）。
	Stroke string
	// Commands 是按文档序的全部命令（a:moveTo/a:lnTo/a:cubicBezTo/...）。
	Commands []PathCommand
	// Unknown 是路径下未识别子元素名。
	Unknown []string
}

// PathCommand 是 a:custGeom 单条命令。Kind 为命令元素 Local 名。
type PathCommand struct {
	// Kind 是命令元素 Local 名（moveTo / lnTo / arcTo / cubicBezTo /
	// quadBezTo / close）。
	Kind string
	// Points 是该命令坐标序列（EMU）。moveTo/lnTo/arcTo 各 1 个；
	// cubicBezTo 3 个（控制点 1、控制点 2、终点）；quadBezTo 2 个。
	Points []Point
}

// ---------- 填充 FillInfo ----------

// FillKind 在 tablestyle.go 已声明（FillUnspecified/FillNone/FillSolid/
// FillGradient/FillPattern/FillPicture/FillGroup）；本文件复用。形状
// 填充额外补充：FillUnspecified 兼作"未识别填充"语义，配合 FillInfo.Raw
// 记录底层元素 Local 名；FillPicture 对应 a:blipFill（图片填充，r:blip
// 关系未解析时仅保留 rId）。
//
// FillKind 描述 spPr/a:fill 容器内的填充类别。
// （本文件不重复声明；FillKind.String() 由 tablestyle.go 提供。）

// FillInfo 是形状填充（spPr/a:fill 内首元素）的解析结果。
// Kind=FillSolid 时 Color 有效；Kind=FillGradient 时 Gradient 有效；
// Kind=FillPattern 时 Pattern 有效；Kind=FillPicture 时 Blip 有效。
type FillInfo struct {
	// Kind 是填充类别。
	Kind FillKind
	// Color 是 solidFill 的颜色（Kind=FillSolid 时有效）。
	Color ParsedColor
	// Gradient 是渐变填充详情（Kind=FillGradient 时有效）。
	Gradient *GradientFill
	// Pattern 是图案填充详情（Kind=FillPattern 时有效）。
	Pattern *PatternFill
	// Blip 是图片填充占位（Kind=FillPicture 时有效）。
	Blip *BlipFillInfo
	// Raw 是底层填充元素 Local 名（Kind=FillUnspecified 且非未声明时记录）。
	Raw string
	// Unknown 是同一 a:fill 容器内其余未识别兄弟元素名。
	Unknown []string
	// Diagnostics 是解析期诊断条目。
	Diagnostics []Diagnostic
}

// GradientFill 是 a:gradFill 解析结果。
type GradientFill struct {
	// Flip / TileAlign / RotateWithShape 是 a:gradFill 上的三个属性
	//（默认分别为 nil/"ctr"/true）。
	Flip            string
	TileAlign       string
	RotateWithShape Optional[bool]
	// PathType 是 a:path@path 的"shape"/"rect"/"circle"之一；缺省"shape"。
	PathType string
	// PathCenter 是 a:path@a:fillToRect 的 l/r/t/b 各通道（千分比 0..100000）。
	PathLeft, PathRight, PathTop, PathBottom int32
	// Angle 是线性渐变角度（a:lin@ang，1/60000 度；线性渐变时有效）。
	Angle int64
	// Scaled 是 a:lin@scaled（默认 true）。
	Scaled Optional[bool]
	// Stops 是按文档序的 a:gs 内 a:stop 列表。
	Stops []GradientStop
	// Unknown 是未识别的子元素名。
	Unknown []string
}

// GradientStop 是 a:gs/a:stop 一个停止点。
type GradientStop struct {
	// Position 是 a:gs@pos 的千分比值（0..100000）。
	Position int32
	// Color 是该停止点的颜色。
	Color ParsedColor
}

// PatternFill 是 a:pattFill 解析结果。
type PatternFill struct {
	// Preset 是 a:patt@prst 的预设图案名（如 "pct5"、"ltHorz"、"dkHorz"）。
	Preset string
	// Foreground / Background 是前景/背景颜色（缺一即视为部分解析）。
	Foreground ParsedColor
	Background ParsedColor
	// Unknown 是未识别的子元素名。
	Unknown []string
}

// BlipFillInfo 是 a:blipFill 的占位结构（图片关系未解析）。
type BlipFillInfo struct {
	// RId 是 a:blip@r:embed 关系 ID；缺关系时为空。
	RId string
	// Dpi 是 a:blip@dpi（资源 DPI）；缺省 0。
	Dpi int32
	// RotateWithShape 是 a:blip@rotWithShape 的值。
	RotateWithShape Optional[bool]
	// SourceRect 是 a:srcRect 的 l/r/t/b（千分比 0..100000）。
	SrcLeft, SrcRight, SrcTop, SrcBottom int32
}

// ---------- 效果 EffectInfo ----------

// EffectKind 是效果类别（a:effectLst 或 a:effectDag 顶层元素）。
type EffectKind int

const (
	// EffectNone 表示无效果（effectLst 元素存在但无内容）。
	EffectNone EffectKind = iota
	// EffectOuterShadow 是 a:outerShdw。
	EffectOuterShadow
	// EffectInnerShadow 是 a:innerShdw。
	EffectInnerShadow
	// EffectGlow 是 a:glow。
	EffectGlow
	// EffectSoftEdge 是 a:softEdge。
	EffectSoftEdge
	// EffectReflection 是 a:reflection。
	EffectReflection
	// EffectFillOverlay 是 a:fillOverlay。
	EffectFillOverlay
	// EffectBlur 是 a:blur（a14 扩展）。
	EffectBlur
	// EffectUnknown 是未识别/缺失类别。
	EffectUnknown
)

func (k EffectKind) String() string {
	switch k {
	case EffectOuterShadow:
		return "outerShdw"
	case EffectInnerShadow:
		return "innerShdw"
	case EffectGlow:
		return "glow"
	case EffectSoftEdge:
		return "softEdge"
	case EffectReflection:
		return "reflection"
	case EffectFillOverlay:
		return "fillOverlay"
	case EffectBlur:
		return "blur"
	}
	return "unknown"
}

// EffectInfo 是形状效果（spPr/a:effectLst 或 a:effectDag）的解析结果。
// Scene3D 与 Shape3D 仅在 spPr/a:scene3d 与 a:scene3d/a:sp3d 出现时填入。
type EffectInfo struct {
	// Container 是效果容器名（"effectLst" / "effectDag" / ""）。
	Container string
	// Effects 是按文档序的全部效果条目。
	Effects []Effect
	// Scene3D 表示 a:scene3d 解析结果（nil 表示未声明）。
	Scene3D *Scene3DInfo
	// Shape3D 表示 a:scene3d/a:sp3d 解析结果（nil 表示未声明）。
	Shape3D *Shape3DInfo
	// Diagnostics 是解析期诊断条目。
	Diagnostics []Diagnostic
}

// Effect 是单个效果条目。
type Effect struct {
	// Kind 是效果类别。
	Kind EffectKind
	// Raw 是底层元素 Local 名（未识别时记录）。
	Raw string
	// Color 是阴影/发光/反射的颜色（softEdge 无颜色）；其它效果为空。
	Color ParsedColor
	// BlurRadius / OffsetX / OffsetY 是阴影/柔边的 EMU 半径与偏移。
	BlurRadius, OffsetX, OffsetY EMU
	// Angle 是阴影方向（1/60000 度）。
	Angle int64
	// Alpha 是阴影/反射等透明度（0..1）；缺省 1。
	Alpha float64
	// StandardDeviation 是高斯模糊标准差 EMU（glow/softEdge/reflection）。
	StandardDeviation EMU
	// Distance 是反射距离（EMU）。
	Distance EMU
	// Direction 是反射方向（a:reflection@dir，1/60000 度）。
	Direction int64
	// FadeDirection 是反射淡化方向（a:reflection@fadeDir，1/60000 度）。
	FadeDirection int64
	// Start / EndOpacity 是反射/阴影的起始/终止透明度（0..1）。
	StartOpacity, EndOpacity float64
	// BlendMode 是效果混合模式（"norm" / "mult" / "screen" 等）。
	BlendMode string
	// Hidden 是某些效果容器内的 @hideSelf="1"。
	Hidden bool
	// Unknown 是未识别的属性或子元素名。
	Unknown []string
}

// Scene3DInfo 是 a:scene3d 顶层解析（仅"声明存在"+关键子元素）。
type Scene3DInfo struct {
	// Camera 是 a:camera 集合（prst/zoom/rot）。
	Camera Camera3D
	// LightRig 是 a:lightRig 集合（rig/dir）。
	LightRig LightRig3D
	// BackdropPlane 是 a:backdrop 平面（可空）。
	BackdropPlane Optional[bool]
}

// Camera3D 是 a:scene3d/a:camera 解析。
type Camera3D struct {
	Preset string
	Zoom   Optional[float64]
	Rot    []int64 // yaw/pitch/roll 各 1/60000 度
}

// LightRig3D 是 a:scene3d/a:lightRig 解析。
type LightRig3D struct {
	Rig string
	Dir []int64 // latitude/longitude/revolution
}

// Shape3DInfo 是 a:sp3d 解析。
type Shape3DInfo struct {
	ExtrusionH         EMU
	ContourW           EMU
	PresetMaterial     string
	TopBevel, BotBevel Bevel3D
}

// Bevel3D 是 a:bevelT/a:bevelB 解析。
type Bevel3D struct {
	Preset string
	Width  EMU
	Height EMU
}

// ---------- Shape 接口扩展 ----------

// Geometry 返回形状几何（spPr/a:prstGeom 或 a:custGeom）的 R 档解析结果。
// 组合（grpSp）/未知 OpaqueShape 等无 spPr 的形状返回零值 + Warning 诊断。
func (s *shapeNode) Geometry() (GeometryInfo, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return GeometryInfo{}, nil, Annotate(err, "shape.Geometry")
	}
	return parseShapeGeometry(doc, el)
}

// Fill 返回形状填充（spPr/a:fill）的 R 档解析结果。
// 组合（grpSp）等无 spPr 的形状返回零值 + Warning 诊断。
func (s *shapeNode) Fill() (FillInfo, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return FillInfo{}, nil, Annotate(err, "shape.Fill")
	}
	return parseShapeFill(s.p, doc, el, string(s.part))
}

// Effects 返回形状效果（spPr/a:effectLst|a:effectDag + a:scene3d + a:sp3d）
// 的 R 档解析结果。
func (s *shapeNode) Effects() (EffectInfo, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return EffectInfo{}, nil, Annotate(err, "shape.Effects")
	}
	return parseShapeEffects(s.p, doc, el, string(s.part))
}

// ---------- 内部解析 ----------

// fillContainer 在形状元素下查找 spPr/a:fill 容器。spPr 不存在返回 nil。
func fillContainer(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	sp := childOfKind(doc, el, nsPresentationML, "spPr", 0)
	if sp == nil {
		return nil
	}
	return childOfKind(doc, sp, nsDrawingML, "fill", 0)
}

// spPrOf 查找形状元素下的 spPr。
func spPrOf(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	return childOfKind(doc, el, nsPresentationML, "spPr", 0)
}

// parseShapeGeometry 解析 spPr 下 a:prstGeom / a:custGeom。
func parseShapeGeometry(doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord) (GeometryInfo, []Diagnostic, error) {
	var diags []Diagnostic
	sp := spPrOf(doc, el)
	if sp == nil {
		// 组合（grpSp）等无 spPr 的情况：返回 unknown + Warning。
		diags = append(diags, Diagnostic{
			Code: "geom.spPr.missing", Severity: SeverityWarning,
			Message: "shape has no spPr; geometry not applicable",
		})
		return GeometryInfo{Kind: GeometryUnknown, Diagnostics: diags}, diags, nil
	}
	// 在 spPr 子节点中按文档序查找首个 a:prstGeom 或 a:custGeom。
	var node *xmlstore.NodeRecord
	var kind GeometryKind
	for _, cid := range sp.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
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
		if c.Namespace != nsDrawingML {
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
	info *GeometryInfo, diags *[]Diagnostic) {
	avLst := childOfKind(doc, node, nsDrawingML, "avLst", 0)
	if avLst == nil {
		return
	}
	for _, cid := range avLst.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML || c.Local() != "gd" {
			if c.Namespace == nsDrawingML {
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
	info *GeometryInfo, diags *[]Diagnostic) {
	gdLst := childOfKind(doc, node, nsDrawingML, "gdLst", 0)
	if gdLst != nil {
		for _, cid := range gdLst.Children {
			c := doc.Node(cid)
			if c.Namespace != nsDrawingML || c.Local() != "gd" {
				continue
			}
			name, _ := c.Attr("", "name")
			fmla, _ := c.Attr("", "fmla")
			info.Guides = append(info.Guides, GeomGuide{Name: name, Fmla: fmla})
		}
	}
	pathLst := childOfKind(doc, node, nsDrawingML, "pathLst", 0)
	if pathLst == nil {
		return
	}
	for _, cid := range pathLst.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML || c.Local() != "path" {
			continue
		}
		p := GeomPath{}
		if v, ok := c.Attr("", "w"); ok {
			if n, e := strconv.ParseInt(v, 10, 64); e == nil {
				p.Width = EMU(n)
			} else {
				*diags = append(*diags, Diagnostic{
					Code: "geom.custGeom.invalid_w", Severity: SeverityWarning,
					Message: "invalid w attribute on path: " + v,
				})
			}
		}
		if v, ok := c.Attr("", "h"); ok {
			if n, e := strconv.ParseInt(v, 10, 64); e == nil {
				p.Height = EMU(n)
			} else {
				*diags = append(*diags, Diagnostic{
					Code: "geom.custGeom.invalid_h", Severity: SeverityWarning,
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
	out *GeomPath, diags *[]Diagnostic) {
	for _, cid := range path.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "close":
			out.Commands = append(out.Commands, PathCommand{Kind: "close"})
		case "moveTo", "lnTo", "arcTo":
			pt, ok := parseSinglePoint(doc, c)
			if !ok {
				*diags = append(*diags, Diagnostic{
					Code: "geom.custGeom.missing_pt", Severity: SeverityWarning,
					Message: c.Local() + " missing/invalid pt",
				})
				continue
			}
			out.Commands = append(out.Commands, PathCommand{Kind: c.Local(), Points: []Point{pt}})
		case "cubicBezTo":
			pts := parseTriplePoints(doc, c)
			if len(pts) < 3 {
				*diags = append(*diags, Diagnostic{
					Code: "geom.custGeom.missing_pt", Severity: SeverityWarning,
					Message: "cubicBezTo requires 3 points",
				})
				continue
			}
			out.Commands = append(out.Commands, PathCommand{Kind: c.Local(), Points: pts[:3]})
		case "quadBezTo":
			pts := parseTriplePoints(doc, c)
			if len(pts) < 2 {
				*diags = append(*diags, Diagnostic{
					Code: "geom.custGeom.missing_pt", Severity: SeverityWarning,
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
	pt := childOfKind(doc, parent, nsDrawingML, "pt", 0)
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
		if c.Namespace != nsDrawingML || c.Local() != "pt" {
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

// ---------- Fill 解析 ----------

// parseShapeFill 解析 spPr/a:fill 内的首个有意义的填充元素。
func parseShapeFill(p *Presentation, doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord, part string) (FillInfo, []Diagnostic, error) {
	var diags []Diagnostic
	fc := fillContainer(doc, el)
	if fc == nil {
		// 形状无 spPr 或 spPr 无 a:fill：返回 unknown + Warning。
		diags = append(diags, Diagnostic{
			Code: "fill.spPr.missing", Severity: SeverityWarning,
			Message: "shape has no spPr/fill; fill not applicable",
		})
		return FillInfo{Kind: FillUnspecified, Diagnostics: diags}, diags, nil
	}
	// 准备 styleEnv 供 schemeClr 展开。
	var env *styleEnv
	if p != nil {
		if e, err := p.styleEnv(opc.PartName(part)); err == nil {
			env = e
		} else {
			diags = append(diags, Diagnostic{
				Code: "fill.theme.unavailable", Severity: SeverityInfo,
				Message: "theme unavailable for fill color resolution: " + err.Error(),
			})
		}
	}
	out := FillInfo{Diagnostics: diags}
	first := true
	for _, cid := range fc.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		// 仅第一个语义填充元素生效；其余视为冲突并记 Unknown。
		if !first {
			out.Unknown = append(out.Unknown, c.Local())
			continue
		}
		first = false
		switch c.Local() {
		case "noFill":
			out.Kind = FillNone
		case "solidFill":
			out.Kind = FillSolid
			out.Color = p.parseColorNode(doc, env, part, colorChildOf(doc, c), &diags)
		case "gradFill":
			out.Kind = FillGradient
			out.Gradient = parseGradFill(p, doc, c, part, env, &diags)
		case "pattFill":
			out.Kind = FillPattern
			out.Pattern = parsePattFill(p, doc, c, part, env, &diags)
		case "blipFill":
			out.Kind = FillPicture
			out.Blip = parseBlipFill(doc, c, &diags)
		case "grpFill":
			out.Kind = FillGroup
		default:
			out.Kind = FillUnspecified
			out.Raw = c.Local()
			diags = append(diags, Diagnostic{
				Code: "fill.unknown_kind", Severity: SeverityWarning,
				Message: "unknown fill kind: " + c.Local(),
			})
		}
	}
	out.Diagnostics = diags
	return out, diags, nil
}

// parseGradFill 解析 a:gradFill + a:gsLst/a:gs/a:stop + a:lin/a:path。
func parseGradFill(p *Presentation, doc *xmlstore.XMLDocument, grad *xmlstore.NodeRecord,
	part string, env *styleEnv, diags *[]Diagnostic) *GradientFill {
	g := &GradientFill{Flip: "none", TileAlign: "ctr", RotateWithShape: NewOptional(true)}
	if v, ok := grad.Attr("", "flip"); ok {
		g.Flip = v
	}
	if v, ok := grad.Attr("", "tileAlign"); ok {
		g.TileAlign = v
	}
	if v, ok := grad.Attr("", "rotWithShape"); ok {
		g.RotateWithShape = parseOptionalBool(v)
	}
	for _, cid := range grad.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "gsLst":
			for _, gid := range c.Children {
				gs := doc.Node(gid)
				if gs.Namespace != nsDrawingML || gs.Local() != "gs" {
					continue
				}
				stop := GradientStop{Position: -1}
				if v, ok := gs.Attr("", "pos"); ok {
					if n, e := strconv.ParseInt(v, 10, 32); e == nil {
						stop.Position = int32(n)
					} else {
						*diags = append(*diags, Diagnostic{
							Code: "fill.gradient.invalid_pos", Severity: SeverityWarning,
							Message: "invalid gs@pos: " + v,
						})
					}
				}
				// gs 内首个 a:DrawingML 子元素即为颜色。
				clr := firstDrawingChild(doc, gs)
				stop.Color = p.parseColorNode(doc, env, part, clr, diags)
				g.Stops = append(g.Stops, stop)
			}
		case "lin":
			if v, ok := c.Attr("", "ang"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					g.Angle = n
				}
			}
			if v, ok := c.Attr("", "scaled"); ok {
				g.Scaled = parseOptionalBool(v)
			}
		case "path":
			g.PathType, _ = c.Attr("", "path")
			if g.PathType == "" {
				g.PathType = "shape"
			}
			if fillToRect := childOfKind(doc, c, nsDrawingML, "fillToRect", 0); fillToRect != nil {
				g.PathLeft = percentAttr(doc, fillToRect, "l")
				g.PathRight = percentAttr(doc, fillToRect, "r")
				g.PathTop = percentAttr(doc, fillToRect, "t")
				g.PathBottom = percentAttr(doc, fillToRect, "b")
			}
		case "tileRect":
			// 路径下 a:tileRect（l/t/r/b）暂作 R 档细节可选保留；此处只跳过。
		default:
			g.Unknown = append(g.Unknown, c.Local())
		}
	}
	return g
}

// parsePattFill 解析 a:pattFill + fg/bg 颜色。
func parsePattFill(p *Presentation, doc *xmlstore.XMLDocument, patt *xmlstore.NodeRecord,
	part string, env *styleEnv, diags *[]Diagnostic) *PatternFill {
	out := &PatternFill{Preset: "pct5"}
	if v, ok := patt.Attr("", "prst"); ok {
		out.Preset = v
	}
	for _, cid := range patt.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		clr := colorChildOf(doc, c)
		col := p.parseColorNode(doc, env, part, clr, diags)
		switch c.Local() {
		case "fgClr":
			out.Foreground = col
		case "bgClr":
			out.Background = col
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out
}

// parseBlipFill 解析 a:blipFill 内 a:blip（rId/dpi/rotWithShape）与 a:srcRect。
func parseBlipFill(doc *xmlstore.XMLDocument, bf *xmlstore.NodeRecord, diags *[]Diagnostic) *BlipFillInfo {
	out := &BlipFillInfo{RotateWithShape: NewOptional(true)}
	for _, cid := range bf.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "blip":
			out.RId, _ = c.Attr(nsOfficeDocument, "embed")
			if v, ok := c.Attr("", "dpi"); ok {
				if n, e := strconv.ParseInt(v, 10, 32); e == nil {
					out.Dpi = int32(n)
				}
			}
			if v, ok := c.Attr("", "rotWithShape"); ok {
				out.RotateWithShape = parseOptionalBool(v)
			}
		case "srcRect":
			out.SrcLeft = percentAttr(doc, c, "l")
			out.SrcRight = percentAttr(doc, c, "r")
			out.SrcTop = percentAttr(doc, c, "t")
			out.SrcBottom = percentAttr(doc, c, "b")
		case "tile", "stretch":
			// 合法子元素，不记 Unknown。
		default:
			*diags = append(*diags, Diagnostic{
				Code: "fill.blip.unknown_child", Severity: SeverityInfo,
				Message: "unknown blipFill child: " + c.Local(),
			})
		}
	}
	return out
}

// percentAttr 解析百分比属性（千分比字符串），失败返回 -1 表示"无值"。
func percentAttr(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, name string) int32 {
	v, ok := n.Attr("", name)
	if !ok {
		return -1
	}
	num, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return -1
	}
	return int32(num)
}

// firstDrawingChild 返回容器内首个 a:DrawingML 命名空间下的子元素。
func firstDrawingChild(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML {
			return c
		}
	}
	return nil
}

// ---------- Effects 解析 ----------

// parseShapeEffects 解析 spPr/a:effectLst|a:effectDag + a:scene3d + a:sp3d。
func parseShapeEffects(p *Presentation, doc *xmlstore.XMLDocument, el *xmlstore.NodeRecord, part string) (EffectInfo, []Diagnostic, error) {
	var diags []Diagnostic
	sp := spPrOf(doc, el)
	if sp == nil {
		diags = append(diags, Diagnostic{
			Code: "effects.spPr.missing", Severity: SeverityWarning,
			Message: "shape has no spPr; effects not applicable",
		})
		return EffectInfo{Diagnostics: diags}, diags, nil
	}
	out := EffectInfo{}
	for _, cid := range sp.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "effectLst", "effectDag":
			out.Container = c.Local()
			parseEffectList(p, doc, c, part, &out, &diags)
		case "scene3d":
			out.Scene3D = parseScene3D(doc, c, &diags)
			// scene3d 内 a:sp3d 嵌套解析。
			if sp3 := childOfKind(doc, c, nsDrawingML, "sp3d", 0); sp3 != nil {
				out.Shape3D = parseShape3D(doc, sp3, &diags)
			}
		}
	}
	out.Diagnostics = diags
	return out, diags, nil
}

// parseEffectList 解析 a:effectLst / a:effectDag 内全部效果条目。
func parseEffectList(p *Presentation, doc *xmlstore.XMLDocument, cont *xmlstore.NodeRecord,
	part string, out *EffectInfo, diags *[]Diagnostic) {
	var env *styleEnv
	if p != nil {
		if e, err := p.styleEnv(opc.PartName(part)); err == nil {
			env = e
		}
	}
	for _, cid := range cont.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		eff := Effect{Raw: c.Local(), BlendMode: "norm", Color: ParsedColor{Alpha: 1}}
		switch c.Local() {
		case "outerShdw", "innerShdw":
			parseShadow(p, doc, c, part, env, &eff, diags)
			if c.Local() == "innerShdw" {
				eff.Kind = EffectInnerShadow
			} else {
				eff.Kind = EffectOuterShadow
			}
		case "glow":
			eff.Kind = EffectGlow
			if v, ok := c.Attr("", "rad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = EMU(n)
				}
			}
			clr := firstDrawingChild(doc, c)
			eff.Color = p.parseColorNode(doc, env, part, clr, diags)
		case "softEdge":
			eff.Kind = EffectSoftEdge
			if v, ok := c.Attr("", "rad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = EMU(n)
				}
			}
		case "reflection":
			eff.Kind = EffectReflection
			if v, ok := c.Attr("", "blurRad"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.StandardDeviation = EMU(n)
				}
			}
			if v, ok := c.Attr("", "stA"); ok {
				if f, e := strconv.ParseFloat(v, 64); e == nil {
					eff.StartOpacity = f / 100000.0
				}
			}
			if v, ok := c.Attr("", "stPos"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.Distance = EMU(n)
				}
			}
			if v, ok := c.Attr("", "endA"); ok {
				if f, e := strconv.ParseFloat(v, 64); e == nil {
					eff.EndOpacity = f / 100000.0
				}
			}
			if v, ok := c.Attr("", "endPos"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.OffsetY = EMU(n)
				}
			}
			if v, ok := c.Attr("", "dir"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.Direction = n
				}
			}
			if v, ok := c.Attr("", "fadeDir"); ok {
				if n, e := strconv.ParseInt(v, 10, 64); e == nil {
					eff.FadeDirection = n
				}
			}
			parseBlendModeAttrs(c, &eff)
		case "fillOverlay":
			eff.Kind = EffectFillOverlay
			clr := firstDrawingChild(doc, c)
			eff.Color = p.parseColorNode(doc, env, part, clr, diags)
			parseBlendModeAttrs(c, &eff)
		default:
			// a14:blur 等扩展暂归 unknown。
			eff.Kind = EffectUnknown
			*diags = append(*diags, Diagnostic{
				Code: "effects.unknown_kind", Severity: SeverityInfo,
				Message: "unrecognised effect element: " + c.Local(),
			})
		}
		// 通用 @hideSelf（a:effectLst 的常见属性）。
		if v, ok := c.Attr("", "hideSelf"); ok {
			eff.Hidden = v == "1"
		}
		out.Effects = append(out.Effects, eff)
	}
}

// parseShadow 解析 outerShdw / innerShdw 的公用字段。
func parseShadow(p *Presentation, doc *xmlstore.XMLDocument, c *xmlstore.NodeRecord,
	part string, env *styleEnv, eff *Effect, diags *[]Diagnostic) {
	if v, ok := c.Attr("", "blurRad"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.BlurRadius = EMU(n)
		}
	}
	if v, ok := c.Attr("", "dist"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.Distance = EMU(n)
		}
	}
	if v, ok := c.Attr("", "dir"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.Angle = n
		}
	}
	if v, ok := c.Attr("", "sx"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.OffsetX = EMU(n)
		}
	}
	if v, ok := c.Attr("", "sy"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			eff.OffsetY = EMU(n)
		}
	}
	if v, ok := c.Attr("", "algn"); ok {
		eff.Unknown = append(eff.Unknown, "algn="+v)
	}
	clr := firstDrawingChild(doc, c)
	eff.Color = p.parseColorNode(doc, env, part, clr, diags)
	eff.Alpha = eff.Color.Alpha
	parseBlendModeAttrs(c, eff)
}

// parseBlendModeAttrs 提取 @blend 属性。
func parseBlendModeAttrs(c *xmlstore.NodeRecord, eff *Effect) {
	if v, ok := c.Attr("", "blend"); ok {
		eff.BlendMode = v
	}
}

// parseScene3D 解析 a:scene3d 内 a:camera + a:lightRig。
func parseScene3D(doc *xmlstore.XMLDocument, scene *xmlstore.NodeRecord, diags *[]Diagnostic) *Scene3DInfo {
	out := &Scene3DInfo{}
	if cam := childOfKind(doc, scene, nsDrawingML, "camera", 0); cam != nil {
		out.Camera.Preset, _ = cam.Attr("", "prst")
		if v, ok := cam.Attr("", "zoom"); ok {
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				out.Camera.Zoom = NewOptional(f)
			}
		}
		if rot := childOfKind(doc, cam, nsDrawingML, "rot", 0); rot != nil {
			out.Camera.Rot = axisTriplet(rot, []string{"lat", "lng", "rev"})
		}
	}
	if rig := childOfKind(doc, scene, nsDrawingML, "lightRig", 0); rig != nil {
		out.LightRig.Rig, _ = rig.Attr("", "rig")
		if rot := childOfKind(doc, rig, nsDrawingML, "rot", 0); rot != nil {
			out.LightRig.Dir = axisTriplet(rot, []string{"lat", "lng", "rev"})
		}
	}
	if bd := childOfKind(doc, scene, nsDrawingML, "backdrop", 0); bd != nil {
		if v, ok := bd.Attr("", "plane"); ok {
			out.BackdropPlane = parseOptionalBool(v)
		}
	}
	return out
}

// parseShape3D 解析 a:sp3d（含 bevelT / bevelB / extrusionH / contourW）。
func parseShape3D(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord, diags *[]Diagnostic) *Shape3DInfo {
	out := &Shape3DInfo{PresetMaterial: "warmMatte"}
	if v, ok := sp.Attr("", "prstMaterial"); ok {
		out.PresetMaterial = v
	}
	if v, ok := sp.Attr("", "extrusionH"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.ExtrusionH = EMU(n)
		}
	}
	if v, ok := sp.Attr("", "contourW"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.ContourW = EMU(n)
		}
	}
	if b := childOfKind(doc, sp, nsDrawingML, "bevelT", 0); b != nil {
		out.TopBevel = parseBevel(doc, b)
	}
	if b := childOfKind(doc, sp, nsDrawingML, "bevelB", 0); b != nil {
		out.BotBevel = parseBevel(doc, b)
	}
	return out
}

// parseBevel 解析 a:bevelT/a:bevelB。
func parseBevel(doc *xmlstore.XMLDocument, b *xmlstore.NodeRecord) Bevel3D {
	out := Bevel3D{Preset: "round"}
	if v, ok := b.Attr("", "prst"); ok {
		out.Preset = v
	}
	if v, ok := b.Attr("", "w"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Width = EMU(n)
		}
	}
	if v, ok := b.Attr("", "h"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Height = EMU(n)
		}
	}
	return out
}

// axisTriplet 从 a:rot 元素读取三属性 lat/lng/rev（1/60000 度）。
func axisTriplet(rot *xmlstore.NodeRecord, keys []string) []int64 {
	out := make([]int64, 0, len(keys))
	for _, k := range keys {
		v, ok := rot.Attr("", k)
		if !ok {
			out = append(out, 0)
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

// parseOptionalBool 把 "1"/"0"/"true"/"false" 映射为 Optional[bool]。
// 非合法值返回未设置的 Optional（Set=false）。
func parseOptionalBool(v string) Optional[bool] {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true":
		return NewOptional(true)
	case "0", "false":
		return NewOptional(false)
	}
	return Optional[bool]{}
}
