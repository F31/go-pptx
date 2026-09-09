package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 M3"格式深度子集"（方案 2.3 矩阵 M3 行）：
//
//	形状级 线条系统      a:ln（箭头、dash、join）                    E 档
//	文本级 段落属性全集  a:lnSpc、buChar/buAutoNum/buBlip、a:tabLst  E 档
//	文本级 Run 高级属性  baseline、spc、highlight、caps、sym         E 档
//	主题   样式矩阵      a:fmtScheme 与 styleMatrixReference 引用链   R 档
//	主题   颜色变换全集  lumMod/lumOff/shade/tint/satMod 等约 20 种   R 全集
//
// 本轮为"起步解析"：提供结构化读取与诊断输出，不提供写入 API（写入与
// 像素级一致随 M6 的 STYLE-02/GEOM-02/TEXT-03 收口）。每项解析遇到
// 未知形态时保留已知信息并输出诊断，不臆造取值（§6.1 契约）。

// ---------- 颜色变换全集 ----------

// ColorTransform 是单个颜色变换（ECMA EG_ColorTransform）。
// Value 为 val 属性的千分比值（0..100000；hue 单位为 1/60000 度）。
type ColorTransform struct {
	Kind  string
	Value int32
}

// ParsedColor 是颜色元素（a:srgbClr/a:schemeClr/…）的解析结果。
type ParsedColor struct {
	// Spec 保留原始颜色引用（scheme 名或 #RRGGBB），未解析时也尽量保留。
	Spec ColorSpec
	// RGB 是应用全部已知变换后的 RRGGBB；空串表示未解析出可呈现值。
	RGB string
	// Alpha 是变换后的不透明度（0..1；缺省 1）。
	Alpha float64
	// Transforms 是按文档序的全部变换（含未知变换）。
	Transforms []ColorTransform
	// Unknown 是未识别的变换名（顺序同文档），调用方据此判断部分解析。
	Unknown []string
	// Resolved 表示已得到最终可呈现 RGB（未知变换时为 false）。
	Resolved bool
}

// 已知变换名集合（ECMA EG_ColorTransform 全集 28 种；未知入诊断）。
//
// 分类（单位均为 val 属性的整数）：
//   - 相对量（*Mod = 乘 1/1000 百分比，*Off = 加 1/1000 百分比）：
//     lumMod/lumOff/satMod/satOff/hueMod/hueOff/redMod/redOff/
//     greenMod/greenOff/blueMod/blueOff/alphaMod/alphaOff
//   - 绝对量（直接赋值，单位同相对量）：hue（1/60000 度）、
//     sat/lum/red/green/blue/alpha（1/1000 百分比）
//   - 无参复合：comp（补色 +180°）、inv（反相）、gray（灰度）
//   - 曲线：gamma（伽马校正）、invGamma（逆伽马校正）
//   - 简写：tint（向白混合 = lumOff 的补）、shade（向黑混合 = lumMod 的补）
var knownTransformKinds = map[string]bool{
	// 相对量
	"lumMod": true, "lumOff": true, "satMod": true, "satOff": true,
	"hueMod": true, "hueOff": true,
	"redMod": true, "redOff": true, "greenMod": true, "greenOff": true,
	"blueMod": true, "blueOff": true, "alphaMod": true, "alphaOff": true,
	// 绝对量（STYLE-02 补齐）
	"hue": true, "sat": true, "lum": true,
	"red": true, "green": true, "blue": true, "alpha": true,
	// 无参复合
	"comp": true, "inv": true, "gray": true,
	// 曲线（STYLE-02 补齐）
	"gamma": true, "invGamma": true,
	// 简写
	"tint": true, "shade": true,
}

// parseColorNode 解析颜色元素节点（srgbClr/schemeClr/sysClr/prstClr/
// hslClr/scrgbClr）及其变换序列。env 用于 schemeClr 的主题展开；
// 不可用时（如无主题）RGB 留空并输出诊断。
func (p *Presentation) parseColorNode(doc *xmlstore.XMLDocument, env *styleEnv, part string,
	clr *xmlstore.NodeRecord, diags *[]Diagnostic) ParsedColor {

	out := ParsedColor{Alpha: 1}
	if clr == nil {
		return out
	}
	base := ""
	switch clr.Local() {
	case "srgbClr":
		v, _ := clr.Attr("", "val")
		base = strings.ToUpper(v)
		out.Spec = ColorSpec{RGB: base}
	case "scrgbClr":
		r, g, b := scrgbChannels(clr)
		base = fmt.Sprintf("%02X%02X%02X", r, g, b)
		out.Spec = ColorSpec{RGB: base}
	case "hslClr":
		h, s, l := hslChannels(clr)
		r, g, b := hslToRGB(h, s, l)
		base = fmt.Sprintf("%02X%02X%02X", r, g, b)
		out.Spec = ColorSpec{RGB: base}
	case "prstClr":
		v, _ := clr.Attr("", "val")
		out.Spec = ColorSpec{RGB: v}
		if rgb, ok := presetColorRGB(v); ok {
			base = rgb
		} else {
			*diags = append(*diags, Diagnostic{
				Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: part,
				Message: "unknown preset color: " + v,
			})
		}
	case "schemeClr":
		v, _ := clr.Attr("", "val")
		out.Spec = ColorSpec{Scheme: v}
		scheme := v
		if clrMapIndirect(scheme) {
			if m := masterClrMap(p.masterDoc(env)); m != nil {
				if mapped, ok := m[scheme]; ok {
					scheme = mapped
				}
			}
		}
		rgb, partial := schemeRGB(p.themeDoc(env), scheme)
		if rgb == "" || partial {
			*diags = append(*diags, Diagnostic{
				Code: "STYLE_PARTIAL", Severity: SeverityWarning, Part: part,
				Message: "scheme color not fully resolved: " + v,
			})
			return out
		}
		base = rgb
	case "sysClr":
		v, _ := clr.Attr("", "val")
		out.Spec = ColorSpec{Scheme: v}
		if lc, ok := clr.Attr("", "lastClr"); ok && lc != "" {
			base = strings.ToUpper(lc)
		} else if m, known := sysColorFallback[strings.ToLower(v)]; known {
			base = m
		} else {
			*diags = append(*diags, Diagnostic{
				Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: part,
				Message: "unknown system color: " + v,
			})
			return out
		}
	default:
		*diags = append(*diags, Diagnostic{
			Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: part,
			Message: "unsupported color element: " + clr.Local(),
		})
		return out
	}
	if !isHexRGB(base) {
		*diags = append(*diags, Diagnostic{
			Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: part,
			Message: "invalid base color: " + base,
		})
		return out
	}
	// 变换序列。
	for _, cid := range clr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		v, _ := c.Attr("", "val")
		val, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			val = 0
		}
		out.Transforms = append(out.Transforms, ColorTransform{Kind: c.Local(), Value: int32(val)})
	}
	rgb, alpha, unknown := applyColorTransforms(base, out.Transforms)
	out.RGB, out.Alpha, out.Unknown = rgb, alpha, unknown
	out.Resolved = len(unknown) == 0 && rgb != ""
	if len(unknown) > 0 {
		out.Resolved = false
		*diags = append(*diags, Diagnostic{
			Code: "STYLE_PARTIAL", Severity: SeverityWarning, Part: part,
			Message: "unsupported color transform(s): " + strings.Join(unknown, ","),
		})
	}
	return out
}

// applyColorTransforms 按序应用变换（ECMA：按 XML 文档序）。未知变换
// 记录于 unknown 并跳过该步（保留此前结果，不臆造）。
func applyColorTransforms(rgb string, ts []ColorTransform) (out string, alpha float64, unknown []string) {
	alpha = 1
	if !isHexRGB(rgb) {
		return rgb, alpha, unknown
	}
	r, g, b := int(hexByte(rgb[0:2])), int(hexByte(rgb[2:4])), int(hexByte(rgb[4:6]))
	h, s, l := rgbToHSL(r, g, b)
	for _, t := range ts {
		if !knownTransformKinds[t.Kind] {
			unknown = append(unknown, t.Kind)
			continue
		}
		v := int(t.Value)
		switch t.Kind {
		case "lumMod":
			r, g, b = r*v/100000, g*v/100000, b*v/100000
		case "lumOff":
			r, g, b = r+255*v/100000, g+255*v/100000, b+255*v/100000
		case "shade":
			r, g, b = r*(100000-v)/100000, g*(100000-v)/100000, b*(100000-v)/100000
		case "tint":
			r, g, b = r+(255-r)*v/100000, g+(255-g)*v/100000, b+(255-b)*v/100000
		case "redMod":
			r = r * v / 100000
		case "redOff":
			r = r + 255*v/100000
		case "greenMod":
			g = g * v / 100000
		case "greenOff":
			g = g + 255*v/100000
		case "blueMod":
			b = b * v / 100000
		case "blueOff":
			b = b + 255*v/100000
		case "alpha":
			alpha = float64(v) / 100000
		case "alphaMod":
			alpha = alpha * float64(v) / 100000
		case "alphaOff":
			alpha = alpha + float64(v)/100000
		// ---- STYLE-02 补齐：绝对量通道赋值（val 1/1000 百分比）----
		case "red":
			r = v * 255 / 100000
		case "green":
			g = v * 255 / 100000
		case "blue":
			b = v * 255 / 100000
		// ---- STYLE-02 补齐：伽马曲线（逐通道；c 归一化到 [0,1]）----
		// 语义按 ECMA-376 Part 1 §20.1.2.3.13/14 常见解释（与 LibreOffice、
		// POI 一致）：gamma 为 pow(c, 1/g)、invGamma 为 pow(c, g)，其中
		// g = val/100000。g<=0 或 val 缺失视为无操作（不臆造取值）。
		case "gamma", "invGamma":
			gv := float64(v) / 100000
			if gv > 0 {
				exp := gv
				if t.Kind == "gamma" {
					exp = 1 / gv
				}
				r = int(math.Pow(float64(r)/255, exp)*255 + 0.5)
				g = int(math.Pow(float64(g)/255, exp)*255 + 0.5)
				b = int(math.Pow(float64(b)/255, exp)*255 + 0.5)
			}
		case "inv":
			r, g, b = 255-r, 255-g, 255-b
		case "gray":
			gray := (299*r + 587*g + 114*b) / 1000
			r, g, b = gray, gray, gray
		default: // HSL 空间变换
			h, s, l = rgbToHSL(r, g, b)
			switch t.Kind {
			case "satMod":
				s = s * v / 100000
			case "satOff":
				s = s + v
			case "hueMod":
				h = h * v / 100000
			case "hueOff":
				h = h + v
			case "comp":
				h = (h + 10800000) % 21600000
			// ---- STYLE-02 补齐：HSL 绝对量赋值 ----
			// val 单位与内部表示一致（hue 1/60000 度，sat/lum 1/100000）。
			case "hue":
				h = v % 21600000
				if h < 0 {
					h += 21600000
				}
			case "sat":
				s = clampPct(v)
			case "lum":
				l = clampPct(v)
			}
			r, g, b = hslToRGB(h, s, l)
		}
		r, g, b = clamp8(r), clamp8(g), clamp8(b)
	}
	return fmt.Sprintf("%02X%02X%02X", r, g, b), clamp01(alpha), unknown
}

func clamp8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// clampPct 把百分比整数（0..100000 = 0%..100%）约束到合法区间；
// 用于 sat/lum 等绝对量赋值变换（STYLE-02）。
func clampPct(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100000 {
		return 100000
	}
	return v
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// scrgbChannels 解析 a:scrgbClr 的 r/g/b（千分比 0..100000）。
func scrgbChannels(n *xmlstore.NodeRecord) (r, g, b int) {
	for _, name := range []string{"r", "g", "b"} {
		v, _ := n.Attr("", name)
		i, err := strconv.Atoi(v)
		if err != nil {
			i = 0
		}
		i = i * 255 / 100000
		switch name {
		case "r":
			r = clamp8(i)
		case "g":
			g = clamp8(i)
		default:
			b = clamp8(i)
		}
	}
	return
}

// hslChannels 解析 a:hslClr 的 hue（1/60000 度）、sat/lum（千分比）。
func hslChannels(n *xmlstore.NodeRecord) (h, s, l int) {
	if v, ok := n.Attr("", "hue"); ok {
		h, _ = strconv.Atoi(v)
	}
	if v, ok := n.Attr("", "sat"); ok {
		s, _ = strconv.Atoi(v)
	}
	if v, ok := n.Attr("", "lum"); ok {
		l, _ = strconv.Atoi(v)
	}
	return
}

// presetColorRGB 是常用预设色映射（未知返回 ok=false）。
var presetColorRGBs = map[string]string{
	"black": "000000", "white": "FFFFFF", "red": "FF0000", "green": "00FF00",
	"blue": "0000FF", "yellow": "FFFF00", "cyan": "00FFFF", "magenta": "FF00FF",
	"gray": "808080", "dkGray": "404040", "ltGray": "C0C0C0",
	"orange": "FFA500", "purple": "800080", "brown": "A52A2A", "pink": "FFC0CB",
}

func presetColorRGB(name string) (string, bool) {
	v, ok := presetColorRGBs[strings.ToLower(name)]
	return v, ok
}

func rgbToHSL(r, g, b int) (h, s, l int) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	max, min := math.Max(rf, math.Max(gf, bf)), math.Min(rf, math.Min(gf, bf))
	lum := (max + min) / 2
	hue, sat := 0.0, 0.0
	if max != min {
		d := max - min
		if lum > 0.5 {
			sat = d / (2 - max - min)
		} else {
			sat = d / (max + min)
		}
		switch max {
		case rf:
			hue = (gf - bf) / d
			if gf < bf {
				hue += 6
			}
		case gf:
			hue = (bf-rf)/d + 2
		default:
			hue = (rf-gf)/d + 4
		}
		hue *= 60
	}
	return int(hue * 60000), int(sat * 100000), int(lum * 100000)
}

func hslToRGB(h, s, l int) (r, g, b int) {
	hue, sat, lum := float64(h)/60000, float64(s)/100000, float64(l)/100000
	if sat == 0 {
		v := int(lum*255 + 0.5)
		return clamp8(v), clamp8(v), clamp8(v)
	}
	q := lum * (1 + sat)
	if lum >= 0.5 {
		q = lum + sat - lum*sat
	}
	p := 2*lum - q
	// hue 单位为度（0..360），hue2rgb 的 t 参数要求归一化到 [0,1]，
	// 故除以 360（STYLE-02 修复：原实现误用 /60，得到 0..6 的量纲，
	// 使 satMod/hueMod/hueOff/comp/hue/sat/lum 全部 HSL 变换失真）。
	hf := hue / 360
	// conv 把 tc（归一化的色相段位置）映射为 [0,1] 通道值。
	//
	// 注意：p/q 是本函数的常量，conv 每次调用必须用**局部**变量 v 承接，
	// 绝不写回 p——否则三次调用会串行污染（STYLE-02 修复：原实现把
	// p 当作累加器，导致 satMod/hueMod/hue/hueOff/comp 等 HSL 变换的
	// 后两个通道沿用前一个通道已被修改的 p，结果失真）。
	conv := func(tc float64) int {
		if tc < 0 {
			tc += 1
		}
		if tc > 1 {
			tc -= 1
		}
		v := p
		switch {
		case tc < 1.0/6:
			v = p + (q-p)*6*tc
		case tc < 0.5:
			v = q
		case tc < 2.0/3:
			v = p + (q-p)*(2.0/3-tc)*6
		}
		return int(v*255 + 0.5)
	}
	return clamp8(conv(hf + 1.0/3)), clamp8(conv(hf)), clamp8(conv(hf - 1.0/3))
}

// ---------- 线条系统（a:ln） ----------

// LineStyle 是形状线条（a:ln）的解析结果（E 档常用子集）。
type LineStyle struct {
	// Specified 表示形状存在 a:ln（无线条时为 false）。
	Specified bool
	// Width 是线宽（EMU；a:ln@w 缺省 0 表示 hairline，按原值保留）。
	Width EMU
	// Cap 是端点形状（rnd/sq/flat）。
	Cap string
	// Compound 是复合线型（sng/dbl/thickThin/thinThick/tri）。
	Compound string
	// Align 是笔对齐（ctr/in）。
	Align string
	// Dash 是虚线类型（solid/dot/dash/... 或 custom）。
	Dash string
	// Join 是连接方式（round/bevel/miter）。
	Join string
	// MiterLimit 是斜接限制（a:miter@lim，千分比）。
	MiterLimit int32
	// Color 是线条颜色（含变换）。
	Color ParsedColor
	// HeadEnd / TailEnd 是箭头（a:headEnd/a:tailEnd）。
	HeadEnd LineEnd
	TailEnd LineEnd
	// Unknown 是未识别的线条子元素名。
	Unknown []string
}

// LineEnd 是线条端点（箭头）。
type LineEnd struct {
	// Specified 表示存在对应元素。
	Specified bool
	// Type 是端点类型（none/triangle/stealth/diamond/oval/arrow）。
	Type string
	// Width 与 Length 是尺寸档位（sm/med/lg）。
	Width  string
	Length string
}

// Line 返回形状的线条（spPr/a:ln）；形状无线条返回 Specified=false。
// 组合（grpSp）等无 spPr 的形状返回空 LineStyle 与 nil 错误。
func (s *shapeNode) Line() (LineStyle, []Diagnostic, error) {
	var out LineStyle
	doc, el, err := s.locate()
	if err != nil {
		return out, nil, Annotate(err, "shape.Line")
	}
	sp := childOfKind(doc, el, nsPresentationML, "spPr", 0)
	if sp == nil {
		return out, nil, nil
	}
	ln := childOfKind(doc, sp, nsDrawingML, "ln", 0)
	if ln == nil {
		return out, nil, nil
	}
	var diags []Diagnostic
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return out, nil, Annotate(err, "shape.Line")
	}
	out.Specified = true
	if v, ok := ln.Attr("", "w"); ok {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil {
			out.Width = EMU(n)
		}
	}
	out.Cap, _ = ln.Attr("", "cap")
	out.Compound, _ = ln.Attr("", "cmpd")
	out.Align, _ = ln.Attr("", "algn")
	for _, cid := range ln.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "prstDash":
			v, _ := c.Attr("", "val")
			out.Dash = v
		case "custDash":
			out.Dash = "custom"
		case "round":
			out.Join = "round"
		case "bevel":
			out.Join = "bevel"
		case "miter":
			out.Join = "miter"
			if v, ok := c.Attr("", "lim"); ok {
				if n, e := strconv.Atoi(v); e == nil {
					out.MiterLimit = int32(n)
				}
			}
		case "headEnd":
			out.HeadEnd = parseLineEnd(c)
		case "tailEnd":
			out.TailEnd = parseLineEnd(c)
		case "noFill", "solidFill", "gradFill", "pattFill":
			out.Color = s.p.parseColorNode(doc, env, string(s.part), colorChildOf(doc, c), &diags)
			if c.Local() != "solidFill" {
				out.Unknown = append(out.Unknown, c.Local())
			}
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out, diags, nil
}

func parseLineEnd(n *xmlstore.NodeRecord) LineEnd {
	e := LineEnd{Specified: true}
	e.Type, _ = n.Attr("", "type")
	e.Width, _ = n.Attr("", "w")
	e.Length, _ = n.Attr("", "len")
	return e
}

// colorChildOf 返回填充元素内的颜色元素（solidFill→srgbClr 等）。
func colorChildOf(doc *xmlstore.XMLDocument, fill *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML {
			return c
		}
	}
	return nil
}

// ---------- 段落属性全集 ----------

// Spacing 是间距值（百分比为千分比或磅值）。
type Spacing struct {
	// Kind 为 "pct"（千分比，100000=100%）或 "pts"（磅）。
	Kind string
	// Value 是数值。
	Value int32
}

// BulletKind 是项目符号类型。
type BulletKind int

const (
	// BulletNone 表示无项目符号（a:buNone）。
	BulletNone BulletKind = iota
	// BulletChar 表示字符项目符号（a:buChar）。
	BulletChar
	// BulletAutoNum 表示自动编号（a:buAutoNum）。
	BulletAutoNum
	// BulletBlip 表示图片项目符号（a:buBlip）。
	BulletBlip
	// BulletUnknown 表示未识别/缺失符号定义。
	BulletUnknown
)

func (k BulletKind) String() string {
	switch k {
	case BulletNone:
		return "none"
	case BulletChar:
		return "char"
	case BulletAutoNum:
		return "autonum"
	case BulletBlip:
		return "blip"
	}
	return "unknown"
}

// Bullet 是段落项目符号的解析结果。
type Bullet struct {
	Kind BulletKind
	// Char 是符号字符（a:buChar@char）。
	Char string
	// Font 是符号字体（a:buFont@typeface）。
	Font string
	// AutoNumType 是编号类型（a:buAutoNum@type）。
	AutoNumType string
	// StartAt 是起始编号（a:buAutoNum@startAt）。
	StartAt int32
	// SizePct / SizePts 是符号相对尺寸（千分比 / 磅）。
	SizePct int32
	SizePts float64
}

// TabStop 是制表位（a:tab）。
type TabStop struct {
	// Position 是位置（EMU）。
	Position EMU
	// Align 是对齐（l/ctr/r/dec）。
	Align string
}

// ParagraphProps 是段落属性（a:pPr）的解析结果。
type ParagraphProps struct {
	// Specified 表示存在 a:pPr（缺省继承时为 false）。
	Specified bool
	Level     int32
	Align     string
	// Indent 是首行/悬挂缩进（EMU，负值表示悬挂）。
	Indent      EMU
	MarginLeft  EMU
	MarginRight EMU
	LineSpacing *Spacing
	SpaceBefore *Spacing
	SpaceAfter  *Spacing
	Bullet      *Bullet
	Tabs        []TabStop
	// DefaultTabSize 是默认制表宽度（EMU）。
	DefaultTabSize EMU
	// RTL 表示从右到左段落。
	RTL bool
	// EastAsianLineBreak / LatinLineBreak / HangingPunct 是换行与标点规则。
	EastAsianLineBreak bool
	LatinLineBreak     bool
	HangingPunct       bool
	// FontAlign 是字体对齐（auto/t/ctr/b/base）。
	FontAlign string
	// Unknown 是未识别的 a:pPr 属性/子元素名。
	Unknown []string
}

// Props 返回段落属性（a:pPr）的解析结果（§2.3 矩阵"段落属性全集"
// 起步解析）。未知属性与子元素经诊断与 Unknown 输出，不臆造取值。
func (p *Paragraph) Props() (ParagraphProps, []Diagnostic, error) {
	var out ParagraphProps
	doc, para, err := p.locatePara()
	if err != nil {
		return out, nil, Annotate(err, "Paragraph.Props")
	}
	pr := childOfKind(doc, para, nsDrawingML, "pPr", 0)
	if pr == nil {
		return out, nil, nil
	}
	out.Specified = true
	known := map[string]bool{
		"lvl": true, "algn": true, "indent": true, "marL": true, "marR": true,
		"rtl": true, "eaLnBrk": true, "latinLnBrk": true, "hangingPunct": true,
		"fontAlgn": true, "defTabSz": true,
	}
	for i := range pr.Attrs {
		a := &pr.Attrs[i]
		if a.Namespace != "" {
			out.Unknown = append(out.Unknown, a.RawName)
			continue
		}
		switch a.RawName {
		case "lvl":
			out.Level = intAttr(a.Value)
		case "algn":
			out.Align = a.Value
		case "indent":
			out.Indent = EMU(intAttr(a.Value))
		case "marL":
			out.MarginLeft = EMU(intAttr(a.Value))
		case "marR":
			out.MarginRight = EMU(intAttr(a.Value))
		case "rtl", "eaLnBrk", "latinLnBrk", "hangingPunct":
			v := a.Value == "1" || a.Value == "true"
			switch a.RawName {
			case "rtl":
				out.RTL = v
			case "eaLnBrk":
				out.EastAsianLineBreak = v
			case "latinLnBrk":
				out.LatinLineBreak = v
			default:
				out.HangingPunct = v
			}
		case "fontAlgn":
			out.FontAlign = a.Value
		case "defTabSz":
			out.DefaultTabSize = EMU(intAttr(a.Value))
		default:
			if !known[a.RawName] {
				out.Unknown = append(out.Unknown, a.RawName)
			}
		}
	}
	for _, cid := range pr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "lnSpc":
			out.LineSpacing = parseSpacing(doc, c)
		case "spcBef":
			out.SpaceBefore = parseSpacing(doc, c)
		case "spcAft":
			out.SpaceAfter = parseSpacing(doc, c)
		case "tabLst":
			for _, tid := range c.Children {
				t := doc.Node(tid)
				if t.Namespace != nsDrawingML || t.Local() != "tab" {
					continue
				}
				pos, _ := t.Attr("", "pos")
				algn, _ := t.Attr("", "algn")
				out.Tabs = append(out.Tabs, TabStop{Position: EMU(intAttr(pos)), Align: algn})
			}
		case "buNone":
			out.Bullet = &Bullet{Kind: BulletNone}
		case "buChar":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Kind = BulletChar
			out.Bullet.Char, _ = c.Attr("", "char")
		case "buAutoNum":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Kind = BulletAutoNum
			out.Bullet.AutoNumType, _ = c.Attr("", "type")
			if v, ok := c.Attr("", "startAt"); ok {
				out.Bullet.StartAt = intAttr(v)
			}
		case "buBlip":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Kind = BulletBlip
		case "buFont":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			out.Bullet.Font, _ = c.Attr("", "typeface")
		case "buSzPct":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			if v, ok := c.Attr("", "val"); ok {
				out.Bullet.SizePct = intAttr(v)
			}
		case "buSzPts":
			if out.Bullet == nil {
				out.Bullet = &Bullet{}
			}
			if v, ok := c.Attr("", "val"); ok {
				out.Bullet.SizePts = float64(intAttr(v)) / 100
			}
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out, nil, nil
}

// parseSpacing 解析 a:lnSpc/a:spcBef/a:spcAft 的 spcPct/spcPts。
func parseSpacing(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *Spacing {
	if pct := childOfKind(doc, n, nsDrawingML, "spcPct", 0); pct != nil {
		v, _ := pct.Attr("", "val")
		return &Spacing{Kind: "pct", Value: intAttr(v)}
	}
	if pts := childOfKind(doc, n, nsDrawingML, "spcPts", 0); pts != nil {
		v, _ := pts.Attr("", "val")
		return &Spacing{Kind: "pts", Value: intAttr(v)}
	}
	return nil
}

func intAttr(s string) int32 {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(v)
}

// ---------- Run 高级属性 ----------

// RunProps 是 Run 字符属性（a:rPr）中 STYLE-01 之外的高级项解析结果。
type RunProps struct {
	// Baseline 是基线偏移（a:rPr@baseline，千分比；负为下标）。
	Baseline int32
	// Spacing 是字间距（a:rPr@spc，磅 = val/100）。
	Spacing float64
	// Highlight 是高亮颜色（a:highlight 内的颜色元素）。
	Highlight ParsedColor
	// Caps 是大小写形式（none/all/small）。
	Caps string
	// Strike 是删除线（noStrike/sglStrike/dblStrike）。
	Strike string
	// Underline 是下划线类型（a:rPr@u）。
	Underline string
	// Language / AltLanguage 是语言标签（a:rPr@lang/@altLang）。
	Language    string
	AltLanguage string
	// Kern 是字距调整（a:rPr@kern，磅 = val/100）。
	Kern float64
	// Symbol 是符号字体与字符（a:sym@font/@char）。
	Symbol *RunSymbol
	// Dirty 与 SpellError 表示待重新计算/拼写错误标记。
	Dirty      bool
	SpellError bool
	// Unknown 是未识别的 a:rPr 属性/子元素名。
	Unknown []string
}

// RunSymbol 是 a:sym 符号引用。
type RunSymbol struct {
	Font string
	Char string
}

// AdvancedProps 返回 Run 的高级字符属性（§2.3 矩阵"Run 高级属性"
// 起步解析）：baseline、spc、highlight、caps、strike、u、lang、sym 等。
// 未知项经 Unknown 输出，不臆造取值。
func (r *TextRun) AdvancedProps() (RunProps, []Diagnostic, error) {
	var out RunProps
	doc, run, err := r.locateRun()
	if err != nil {
		return out, nil, Annotate(err, "TextRun.AdvancedProps")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return out, nil, nil
	}
	var diags []Diagnostic
	env, err := r.p.styleEnv(r.part)
	if err != nil {
		return out, nil, Annotate(err, "TextRun.AdvancedProps")
	}
	known := map[string]bool{
		"lang": true, "altLang": true, "sz": true, "b": true, "i": true, "u": true,
		"strike": true, "cap": true, "spc": true, "baseline": true, "kern": true,
		"dirty": true, "err": true, "smtClean": true, "spellErr": true,
	}
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace != "" {
			out.Unknown = append(out.Unknown, a.RawName)
			continue
		}
		switch a.RawName {
		case "baseline":
			out.Baseline = intAttr(a.Value)
		case "spc":
			out.Spacing = float64(intAttr(a.Value)) / 100
		case "cap":
			out.Caps = a.Value
		case "strike":
			out.Strike = a.Value
		case "u":
			out.Underline = a.Value
		case "lang":
			out.Language = a.Value
		case "altLang":
			out.AltLanguage = a.Value
		case "kern":
			out.Kern = float64(intAttr(a.Value)) / 100
		case "dirty":
			out.Dirty = a.Value == "1" || a.Value == "true"
		case "spellErr":
			out.SpellError = a.Value == "1" || a.Value == "true"
		default:
			if !known[a.RawName] {
				out.Unknown = append(out.Unknown, a.RawName)
			}
		}
	}
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "highlight":
			out.Highlight = r.p.parseColorNode(doc, env, string(r.part), colorChildOf(doc, c), &diags)
		case "sym":
			s := &RunSymbol{}
			s.Font, _ = c.Attr("", "font")
			s.Char, _ = c.Attr("", "char")
			out.Symbol = s
		case "latin", "ea", "cs", "solidFill", "noFill", "gradFill", "ln",
			"effectLst", "effectDag", "uLnTx", "uLn", "uFillTx", "uFill",
			"blipFill", "pattFill", "grpFill", "hlinkClick", "hlinkMouseOver",
			"rtl", "extLst":
			// 已知但本包不展开的项（STYLE-01 或后续工作包负责）。
		default:
			out.Unknown = append(out.Unknown, c.Local())
		}
	}
	return out, diags, nil
}

// ---------- 主题样式矩阵引用链 ----------

// MatrixRefKind 是样式矩阵引用的目标类型。
type MatrixRefKind int

const (
	// RefFill 是填充样式引用（a:fillRef）。
	RefFill MatrixRefKind = iota
	// RefLine 是线条样式引用（a:lnRef）。
	RefLine
	// RefEffect 是效果样式引用（a:effectRef）。
	RefEffect
	// RefFont 是字体样式引用（a:fontRef）；STYLE-02 新增，解析到主题
	// a:fontScheme 的 majorFont/minorFont。
	RefFont
)

func (k MatrixRefKind) String() string {
	switch k {
	case RefFill:
		return "fillRef"
	case RefLine:
		return "lnRef"
	case RefEffect:
		return "effectRef"
	case RefFont:
		return "fontRef"
	}
	return "unknown"
}

// StyleMatrixRef 是形状样式矩阵引用（a:spPr/a:style 内的 fillRef/
// lnRef/effectRef/fontRef）的解析结果（R 档：解析并输出诊断，不提供写入）。
type StyleMatrixRef struct {
	// Kind 是引用类型；STYLE-02 起含 RefFont。
	Kind MatrixRefKind
	// Index 是引用下标（1 基；ECMA idx 从 1 起）。
	Index int32
	// Color 是引用内颜色（含变换）。
	Color ParsedColor
	// ThemeEntry 是主题 fmtScheme 中对应条目的元素名（如 solidFill）。
	ThemeEntry string
	// ThemeColor 是主题条目解析出的可呈现颜色（STYLE-02）。
	// 主题条目以 phClr 声明时，基色取 Color 并套用主题条目自身的变换；
	// 无法解析时 RGB 为空、Resolved=false。
	ThemeColor ParsedColor
	// ThemeTypeface 仅 RefFont 有效：主题 fontScheme majorFont/minorFont
	// 的字体名（latin 优先，退化 ea/cs）；未解析时为空（STYLE-02）。
	ThemeTypeface string
	// FontSlot 仅 RefFont 有效：idx 映射到的主题字体槽位。
	FontSlot ThemeFontSlot
	// Resolved 表示引用与主题条目均已解析。
	Resolved bool
}

// StyleMatrixRefs 返回形状的样式矩阵引用链（a:spPr/a:style）。
// 主题 fmtScheme 缺失或下标越界时输出诊断并保持 Resolved=false。
func (s *shapeNode) StyleMatrixRefs() ([]StyleMatrixRef, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return nil, nil, Annotate(err, "shape.StyleMatrixRefs")
	}
	sp := childOfKind(doc, el, nsPresentationML, "spPr", 0)
	if sp == nil {
		return nil, nil, nil
	}
	st := childOfKind(doc, sp, nsDrawingML, "style", 0)
	if st == nil {
		return nil, nil, nil
	}
	var diags []Diagnostic
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return nil, nil, Annotate(err, "shape.StyleMatrixRefs")
	}
	var out []StyleMatrixRef
	for _, cid := range st.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		var kind MatrixRefKind
		switch c.Local() {
		case "fillRef":
			kind = RefFill
		case "lnRef":
			kind = RefLine
		case "effectRef":
			kind = RefEffect
		case "fontRef":
			kind = RefFont // STYLE-02
		default:
			continue
		}
		ref := StyleMatrixRef{Kind: kind}
		idxRaw := ""
		if v, ok := c.Attr("", "idx"); ok {
			idxRaw = v
			ref.Index = intAttr(v)
		}
		ref.Color = s.p.parseColorNode(doc, env, string(s.part), colorChildOf(doc, c), &diags)
		ref.ThemeEntry, ref.Resolved = s.p.themeMatrixEntry(env, kind, ref.Index)
		if kind == RefFont {
			// a:fontRef：idx 为 "major"/"minor" 或 1/2，解析到主题字体槽位。
			ref.FontSlot, ref.ThemeTypeface = s.p.themeFontTypeface(env, idxRaw)
			ref.Resolved = ref.FontSlot != FontSlotUnknown && ref.ThemeTypeface != ""
		} else if kind == RefFill || kind == RefLine {
			// STYLE-02：把主题条目进一步解析为可呈现颜色（phClr 代入）。
			// 仅填充/线条条目含颜色；effectRef 无颜色元素，不产生诊断。
			tdoc, entryNode, ok := s.p.themeMatrixEntryNode(env, kind, ref.Index)
			if ok {
				tc, resolved := s.p.themeEntryColor(tdoc, env, string(s.part), entryNode, ref.Color, &diags)
				ref.ThemeColor = tc
				if !resolved {
					diags = append(diags, Diagnostic{
						Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: string(s.part),
						Message: "theme entry color not resolved for " + kind.String() + " idx=" + idxRaw,
					})
				}
			}
		}
		if !ref.Resolved {
			diags = append(diags, Diagnostic{
				Code: "STYLE_UNRESOLVED", Severity: SeverityInfo, Part: string(s.part),
				Message: fmt.Sprintf("%s idx=%d not found in theme fmtScheme", kind, ref.Index),
			})
		}
		out = append(out, ref)
	}
	return out, diags, nil
}

// themeMatrixEntry 返回主题 fmtScheme 中对应下标条目的填充元素名。
func (p *Presentation) themeMatrixEntry(env *styleEnv, kind MatrixRefKind, idx int32) (entry string, resolved bool) {
	tdoc := p.themeDoc(env)
	if tdoc == nil || idx < 1 {
		return "", false
	}
	elems := childOfKind(tdoc, tdoc.Root(), nsDrawingML, "themeElements", 0)
	if elems == nil {
		return "", false
	}
	fm := childOfKind(tdoc, elems, nsDrawingML, "fmtScheme", 0)
	if fm == nil {
		return "", false
	}
	listName := ""
	switch kind {
	case RefFill:
		listName = "fillStyleLst"
	case RefLine:
		listName = "lnStyleLst"
	case RefEffect:
		listName = "effectStyleLst"
	}
	lst := childOfKind(tdoc, fm, nsDrawingML, listName, 0)
	if lst == nil {
		return "", false
	}
	seen := int32(0)
	for _, cid := range lst.Children {
		c := tdoc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		seen++
		if seen != idx {
			continue
		}
		// 条目：fillStyleLst 为填充元素；lnStyleLst 为 a:ln；effectStyleLst 为 a:effectStyle。
		for _, gid := range c.Children {
			g := tdoc.Node(gid)
			if g.Namespace == nsDrawingML {
				return g.Local(), true
			}
		}
		return c.Local(), true
	}
	return "", false
}
