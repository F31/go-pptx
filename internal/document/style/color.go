package style

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/v2/internal/diag"
	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// ColorSpec 是字体颜色的原始规格（方案 §6.1：颜色保留原始 ColorSpec）。
//
// 二选一：Scheme 非空表示 a:schemeClr（主题色引用，如 "accent1"）；
// 否则 RGB 是 sRGB 十六进制 RRGGBB（无 '#' 前缀）。两者都为空表示
// "无颜色信息"（读取到无法安全表示的颜色形态时保持 Set=false，不臆测）。
type ColorSpec struct {
	Scheme string
	RGB    string
}

func (c ColorSpec) String() string {
	if c.Scheme != "" {
		return "scheme:" + c.Scheme
	}
	if c.RGB != "" {
		return "#" + c.RGB
	}
	return ""
}

// Valid 报告 ColorSpec 是否携带可写出的颜色。
func (c ColorSpec) Valid() bool { return c.Scheme != "" || c.RGB != "" }

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

// ParseColorNode 解析颜色元素节点（srgbClr/schemeClr/sysClr/prstClr/
// hslClr/scrgbClr）及其变换序列。themeDoc/masterDoc 用于 schemeClr 的
// 主题展开；不可用时（如无主题）RGB 留空并输出诊断。
func ParseColorNode(doc *xmlstore.XMLDocument, clr *xmlstore.NodeRecord,
	themeDoc, masterDoc *xmlstore.XMLDocument, part string, diags *[]diag.Diagnostic) ParsedColor {

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
			*diags = append(*diags, diag.Diagnostic{
				Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
				Message: "unknown preset color: " + v,
			})
		}
	case "schemeClr":
		v, _ := clr.Attr("", "val")
		out.Spec = ColorSpec{Scheme: v}
		scheme := v
		if ClrMapIndirect(scheme) {
			// MasterClrMap 恒非 nil：直接查表。
			if mapped, ok := MasterClrMap(masterDoc)[scheme]; ok {
				scheme = mapped
			}
		}
		rgb, partial := SchemeRGB(themeDoc, scheme)
		if rgb == "" || partial {
			*diags = append(*diags, diag.Diagnostic{
				Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning, Part: part,
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
			*diags = append(*diags, diag.Diagnostic{
				Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
				Message: "unknown system color: " + v,
			})
			return out
		}
	default:
		*diags = append(*diags, diag.Diagnostic{
			Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
			Message: "unsupported color element: " + clr.Local(),
		})
		return out
	}
	if !isHexRGB(base) {
		*diags = append(*diags, diag.Diagnostic{
			Code: "STYLE_UNRESOLVED", Severity: diag.SeverityInfo, Part: part,
			Message: "invalid base color: " + base,
		})
		return out
	}
	// 变换序列。
	for _, cid := range clr.Children {
		c := doc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		v, _ := c.Attr("", "val")
		val, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			val = 0
		}
		out.Transforms = append(out.Transforms, ColorTransform{Kind: c.Local(), Value: int32(val)})
	}
	rgb, alpha, unknown := ApplyColorTransforms(base, out.Transforms)
	out.RGB, out.Alpha, out.Unknown = rgb, alpha, unknown
	out.Resolved = len(unknown) == 0 && rgb != ""
	if len(unknown) > 0 {
		out.Resolved = false
		*diags = append(*diags, diag.Diagnostic{
			Code: "STYLE_PARTIAL", Severity: diag.SeverityWarning, Part: part,
			Message: "unsupported color transform(s): " + strings.Join(unknown, ","),
		})
	}
	return out
}

// clrMapIndirect 判断 scheme 名是否需经母版 clrMap 间接映射。
func ClrMapIndirect(scheme string) bool {
	switch scheme {
	case "bg1", "bg2", "tx1", "tx2":
		return true
	}
	return false
}

// SchemeRGB 解析主题 clrScheme 条目的可呈现 RGB。partial=true 表示
// 条目存在但无法完全解析（未知变换/未知系统色/未知 scheme/非
// srgbClr|sysClr 形态）；此时 rgb 可能为基础色（可部分参考）。
func SchemeRGB(tdoc *xmlstore.XMLDocument, scheme string) (rgb string, partial bool) {
	if tdoc == nil {
		return "", true
	}
	root := tdoc.Root()
	if root == nil {
		return "", true
	}
	elems := xmlstore.ChildOfKind(tdoc, root, ooxmlns.DrawingML, "themeElements", 0)
	if elems == nil {
		return "", true
	}
	cs := xmlstore.ChildOfKind(tdoc, elems, ooxmlns.DrawingML, "clrScheme", 0)
	if cs == nil {
		return "", true
	}
	entry := xmlstore.ChildOfKind(tdoc, cs, ooxmlns.DrawingML, scheme, 0)
	if entry == nil {
		return "", true // 未知 scheme 名
	}
	var colorNode *xmlstore.NodeRecord
	for _, cid := range entry.Children {
		c := tdoc.Node(cid)
		if c.Namespace == ooxmlns.DrawingML {
			colorNode = c
			break
		}
	}
	if colorNode == nil {
		return "", true
	}
	base := ""
	switch colorNode.Local() {
	case "srgbClr":
		v, _ := colorNode.Attr("", "val")
		base = strings.ToUpper(v)
	case "sysClr":
		v, _ := colorNode.Attr("", "val")
		if lc, ok := colorNode.Attr("", "lastClr"); ok && lc != "" {
			base = strings.ToUpper(lc)
		} else if m, known := sysColorFallback[strings.ToLower(v)]; known {
			base = m
		} else {
			return "", true
		}
	default:
		return "", true
	}
	if !isHexRGB(base) {
		return "", true
	}
	// 变换序列（GEOM/M3 颜色变换全集：由 applyColorTransforms 统一处理）。
	rgb = base
	var ts []ColorTransform
	for _, cid := range colorNode.Children {
		c := tdoc.Node(cid)
		if c.Namespace != ooxmlns.DrawingML {
			continue
		}
		v, _ := c.Attr("", "val")
		val, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			val = 0
		}
		ts = append(ts, ColorTransform{Kind: c.Local(), Value: int32(val)})
	}
	out, _, unknown := ApplyColorTransforms(rgb, ts)
	if len(unknown) > 0 {
		// 未知变换：保留基础色并标记部分解析（不臆造取值）。
		if isHexRGB(out) {
			return out, true
		}
		return base, true
	}
	// alpha 变换无法在 RGB 输出中表达 → 标记部分解析。
	for _, t := range ts {
		if t.Kind == "alpha" || t.Kind == "alphaMod" || t.Kind == "alphaOff" {
			return out, true
		}
	}
	return out, false
}

// IsHexRGB 报告 s 是否为 6 位大写十六进制 RRGGBB。
func IsHexRGB(s string) bool { return isHexRGB(s) }

func isHexRGB(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func hexByte(s string) uint8 {
	v, _ := strconv.ParseUint(s, 16, 8)
	return uint8(v)
}

// 已知系统色（sysClr）映射表；有 lastClr 属性时优先于本表。
var sysColorFallback = map[string]string{
	"windowtext": "000000",
	"window":     "FFFFFF",
}

// ApplyColorTransforms 按序应用变换（ECMA：按 XML 文档序）。未知变换
// 记录于 unknown 并跳过该步（保留此前结果，不臆造）。
func ApplyColorTransforms(rgb string, ts []ColorTransform) (out string, alpha float64, unknown []string) {
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
	return fmt.Sprintf("%02X%02X%02X", r, g, b), Clamp01(alpha), unknown
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

func Clamp01(v float64) float64 {
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

// presetColorRGBs 是常用预设色映射（未知返回 ok=false）。
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
