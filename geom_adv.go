package pptx

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
