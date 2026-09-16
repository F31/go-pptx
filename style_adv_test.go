package pptx

import (
	"bytes"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// 本文件覆盖 STYLE-02：颜色变换全集（ECMA EG_ColorTransform 28 种）与
// 主题样式矩阵引用链深化（a:fontRef + 主题条目可呈现颜色/phClr 代入）。

// ---------- 颜色变换全集 ----------

// ecmaColorTransformSet 是 ECMA-376 Part 1 EG_ColorTransform 全集 28 种。
var ecmaColorTransformSet = []string{
	// 相对量（14）
	"lumMod", "lumOff", "satMod", "satOff", "hueMod", "hueOff",
	"redMod", "redOff", "greenMod", "greenOff", "blueMod", "blueOff",
	"alphaMod", "alphaOff",
	// 绝对量（7）
	"hue", "sat", "lum", "red", "green", "blue", "alpha",
	// 无参复合（3）
	"comp", "inv", "gray",
	// 曲线（2）
	"gamma", "invGamma",
	// 简写（2）
	"tint", "shade",
}

func TestStyleAdv_TransformSetComplete(t *testing.T) {
	if len(ecmaColorTransformSet) != 28 {
		t.Fatalf("test fixture set size = %d, want 28", len(ecmaColorTransformSet))
	}
	for _, k := range ecmaColorTransformSet {
		if !knownTransformKinds[k] {
			t.Errorf("knownTransformKinds missing %q (STYLE-02 全集要求)", k)
		}
	}
	// 反向：白名单不应含集合外的名字（防止拼写漂移）。
	set := map[string]bool{}
	for _, k := range ecmaColorTransformSet {
		set[k] = true
	}
	for k := range knownTransformKinds {
		if !set[k] {
			t.Errorf("knownTransformKinds has extra entry %q", k)
		}
	}
}

// TestStyleAdv_AbsoluteTransforms 覆盖 STYLE-02 补齐的绝对量变换。
func TestStyleAdv_AbsoluteTransforms(t *testing.T) {
	cases := []struct {
		name string
		kind string
		val  int32
		base string
		want string
	}{
		// red/green/blue：val 为 1/1000 百分比（100000 = 100% → 255）。
		{"red full", "red", 100000, "000000", "FF0000"},
		{"red half", "red", 50000, "000000", "7F0000"},
		{"green full", "green", 100000, "000000", "00FF00"},
		{"blue full", "blue", 100000, "000000", "0000FF"},
		{"red zero", "red", 0, "FFFFFF", "00FFFF"},
		// lum：绝对亮度（HSL 的 L）。100000 → 白；0 → 黑。
		{"lum white", "lum", 100000, "FF0000", "FFFFFF"},
		{"lum black", "lum", 0, "FF0000", "000000"},
		{"lum mid", "lum", 50000, "000000", "808080"},
		// sat：绝对饱和度。0 → 灰度。
		{"sat zero grays", "sat", 0, "FF0000", "808080"},
		// hue：绝对色相（1/60000 度）。红色 FF0000 的 H=0；设为 120°（绿）。
		{"hue to green", "hue", 120 * 60000, "FF0000", "00FF00"},
		{"hue to blue", "hue", 240 * 60000, "FF0000", "0000FF"},
	}
	for _, tc := range cases {
		got, _, unknown := applyColorTransforms(tc.base, []ColorTransform{{Kind: tc.kind, Value: tc.val}})
		if len(unknown) != 0 {
			t.Errorf("%s: unexpected unknown %v", tc.name, unknown)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: applyColorTransforms(%s, %s=%d) = %s, want %s",
				tc.name, tc.base, tc.kind, tc.val, got, tc.want)
		}
	}
}

// TestStyleAdv_AbsoluteTransformsClamped 验证越界 val 被约束而非回绕。
func TestStyleAdv_AbsoluteTransformsClamped(t *testing.T) {
	// red = 200%（越界）→ 约束到 100%（255），不得回绕为负数。
	got, _, _ := applyColorTransforms("000000", []ColorTransform{{Kind: "red", Value: 200000}})
	if got != "FF0000" {
		t.Errorf("red 200%% = %s, want FF0000 (clamped)", got)
	}
	// lum = -50%（越界）→ 约束到 0（黑）。
	got, _, _ = applyColorTransforms("FF0000", []ColorTransform{{Kind: "lum", Value: -50000}})
	if got != "000000" {
		t.Errorf("lum -50%% = %s, want 000000 (clamped)", got)
	}
	// sat 越界 → 约束到 100%。
	got, _, _ = applyColorTransforms("FF0000", []ColorTransform{{Kind: "sat", Value: 150000}})
	if got != "FF0000" {
		t.Errorf("sat 150%% = %s, want FF0000 (clamped)", got)
	}
}

func TestStyleAdv_GammaTransforms(t *testing.T) {
	// gamma 取 pow(c, 1/g)；invGamma 取 pow(c, g)。g=1 时两者均为恒等。
	for _, kind := range []string{"gamma", "invGamma"} {
		got, _, unknown := applyColorTransforms("4080C0", []ColorTransform{{Kind: kind, Value: 100000}})
		if len(unknown) != 0 {
			t.Errorf("%s: unexpected unknown %v", kind, unknown)
		}
		if got != "4080C0" {
			t.Errorf("%s g=1.0 = %s, want 4080C0 (identity)", kind, got)
		}
	}
	// g=2：invGamma 取平方 → 各通道压暗。
	got, _, _ := applyColorTransforms("FFFFFF", []ColorTransform{{Kind: "invGamma", Value: 200000}})
	if got != "FFFFFF" {
		t.Errorf("invGamma g=2 on white = %s, want FFFFFF", got)
	}
	// 0.5 平方（g=2）≈ 0.25 → 64 (0x40)。
	got, _, _ = applyColorTransforms("808080", []ColorTransform{{Kind: "invGamma", Value: 200000}})
	if got != "404040" {
		t.Errorf("invGamma g=2 on 808080 = %s, want 404040", got)
	}
	// gamma g=2 为 pow(c, 0.5) → 提亮。0x80=128 → 128/255≈0.50196，
	// pow(0.50196, 0.5)≈0.70849 → ×255≈180.66 → 四舍五入 181 (0xB5)。
	got, _, _ = applyColorTransforms("808080", []ColorTransform{{Kind: "gamma", Value: 200000}})
	if got != "B5B5B5" {
		t.Errorf("gamma g=2 on 808080 = %s, want B5B5B5", got)
	}
	// g<=0 → 无操作（不臆造）。
	got, _, _ = applyColorTransforms("4080C0", []ColorTransform{{Kind: "gamma", Value: 0}})
	if got != "4080C0" {
		t.Errorf("gamma g=0 = %s, want 4080C0 (no-op)", got)
	}
}

// TestStyleAdv_UnknownTransformNotFabricated 验证未知变换仅入 Unknown，
// 不影响已应用步骤的结果（§6.1 不臆造取值）。
func TestStyleAdv_UnknownTransformNotFabricated(t *testing.T) {
	got, alpha, unknown := applyColorTransforms("FF0000", []ColorTransform{
		{Kind: "lumMod", Value: 50000},
		{Kind: "futureTransform", Value: 30000},
	})
	if len(unknown) != 1 || unknown[0] != "futureTransform" {
		t.Errorf("unknown = %v, want [futureTransform]", unknown)
	}
	if got != "7F0000" {
		t.Errorf("got %s, want 7F0000 (lumMod applied, unknown skipped)", got)
	}
	if alpha != 1 {
		t.Errorf("alpha = %v, want 1", alpha)
	}
}

// ---------- 主题样式矩阵：a:fontRef ----------

// styleAdvParts 构造 STYLE-02 专用夹具：默认模板主题（含 fontScheme 的
// majorFont=Calibri Light / minorFont=Calibri，fillStyleLst 为 phClr），
// 并把 fillStyleLst 第 1 项改为带 lumMod 的 phClr，便于验证代入与变换。
func styleAdvParts(styleBody string) map[opc.PartName][]byte {
	parts := minimalTemplateParts()
	theme := parts["/ppt/theme/theme1.xml"]
	// 首个 fillStyleLst 条目加 lumMod 50%，用于验证 phClr 代入后的变换。
	theme = bytes.Replace(theme,
		[]byte(`<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`),
		[]byte(`<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"><a:lumMod val="50000"/></a:schemeClr></a:solidFill>`),
		1)
	parts["/ppt/theme/theme1.xml"] = theme

	slide := `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="StyleShape"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
		`<a:style>` + styleBody + `</a:style>` +
		`</p:spPr>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>s</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl + slide)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
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
	ct := string(parts["/[Content_Types].xml"])
	ct = strings.Replace(ct, "</Types>",
		`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`, 1)
	parts["/[Content_Types].xml"] = []byte(ct)
	return parts
}

func styleAdvDeck(t *testing.T, styleBody string) *Presentation {
	t.Helper()
	return openFixture(t, buildPackageZipPanic(styleAdvParts(styleBody)))
}

// styleAdvRefOf 返回夹具形状上指定类型的样式矩阵引用。
func styleAdvRefOf(t *testing.T, p *Presentation, kind MatrixRefKind) *StyleMatrixRef {
	t.Helper()
	shapes, err := mustSlides(t, p)[0].Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	refs, _, err := findShapeByID(t, shapes, 2).StyleMatrixRefs()
	if err != nil {
		t.Fatalf("StyleMatrixRefs: %v", err)
	}
	for i := range refs {
		if refs[i].Kind == kind {
			return &refs[i]
		}
	}
	t.Fatalf("no %v in refs %+v", kind, refs)
	return nil
}

func TestStyleAdv_MatrixRefKindStringFontRef(t *testing.T) {
	if got := RefFont.String(); got != "fontRef" {
		t.Errorf("RefFont.String() = %q, want fontRef", got)
	}
}

func TestStyleAdv_FontRef_Minor(t *testing.T) {
	p := styleAdvDeck(t, `<a:fontRef idx="minor"/>`)
	defer p.Close()
	ref := styleAdvRefOf(t, p, RefFont)
	if ref.FontSlot != FontSlotMinor {
		t.Errorf("FontSlot = %v, want minor", ref.FontSlot)
	}
	if ref.ThemeTypeface != "Calibri" {
		t.Errorf("ThemeTypeface = %q, want Calibri", ref.ThemeTypeface)
	}
	if !ref.Resolved {
		t.Errorf("Resolved = false, want true")
	}
}

func TestStyleAdv_FontRef_Major(t *testing.T) {
	p := styleAdvDeck(t, `<a:fontRef idx="major"/>`)
	defer p.Close()
	ref := styleAdvRefOf(t, p, RefFont)
	if ref.FontSlot != FontSlotMajor {
		t.Errorf("FontSlot = %v, want major", ref.FontSlot)
	}
	if ref.ThemeTypeface != "Calibri Light" {
		t.Errorf("ThemeTypeface = %q, want Calibri Light", ref.ThemeTypeface)
	}
}

// TestStyleAdv_FontRef_NumericIdx 验证数字 idx（1=major / 2=minor）与
// 越界值（3）的映射行为。
func TestStyleAdv_FontRef_NumericIdx(t *testing.T) {
	cases := []struct {
		idx     string
		slot    ThemeFontSlot
		face    string
		resolve bool
	}{
		{"1", FontSlotMajor, "Calibri Light", true},
		{"2", FontSlotMinor, "Calibri", true},
		{"3", FontSlotUnknown, "", false},
	}
	for _, tc := range cases {
		p := styleAdvDeck(t, `<a:fontRef idx="`+tc.idx+`"/>`)
		ref := styleAdvRefOf(t, p, RefFont)
		if ref.FontSlot != tc.slot {
			t.Errorf("idx=%s FontSlot = %v, want %v", tc.idx, ref.FontSlot, tc.slot)
		}
		if ref.ThemeTypeface != tc.face {
			t.Errorf("idx=%s ThemeTypeface = %q, want %q", tc.idx, ref.ThemeTypeface, tc.face)
		}
		if ref.Resolved != tc.resolve {
			t.Errorf("idx=%s Resolved = %v, want %v", tc.idx, ref.Resolved, tc.resolve)
		}
		p.Close()
	}
}

// ---------- 主题条目可呈现颜色（phClr 代入）----------

// TestStyleAdv_ThemeColorPhClrSubstitution 验证主题条目以 phClr 声明时，
// 基色取 fillRef 自身颜色并套用主题条目变换（lumMod 50%）。
func TestStyleAdv_ThemeColorPhClrSubstitution(t *testing.T) {
	// fillRef 颜色 FF0000；主题第 1 项 = phClr + lumMod 50% → 7F0000。
	p := styleAdvDeck(t, `<a:fillRef idx="1"><a:srgbClr val="FF0000"/></a:fillRef>`)
	defer p.Close()
	ref := styleAdvRefOf(t, p, RefFill)
	if ref.Color.RGB != "FF0000" {
		t.Fatalf("ref color = %s, want FF0000", ref.Color.RGB)
	}
	if ref.ThemeColor.RGB != "7F0000" {
		t.Errorf("ThemeColor = %+v, want RGB 7F0000 (phClr 代入 + lumMod 50%%)", ref.ThemeColor)
	}
	if !ref.ThemeColor.Resolved {
		t.Errorf("ThemeColor.Resolved = false; Unknown=%v", ref.ThemeColor.Unknown)
	}
}

// TestStyleAdv_ThemeColorNoPhClr 验证主题条目为实色（非 phClr）时按常规
// 路径解析，不使用 fillRef 颜色。
func TestStyleAdv_ThemeColorNoPhClr(t *testing.T) {
	parts := styleAdvParts(`<a:fillRef idx="1"><a:srgbClr val="FF0000"/></a:fillRef>`)
	// 把主题第 1 项改为实色 0000FF（去掉 phClr）。
	theme := parts["/ppt/theme/theme1.xml"]
	theme = bytes.Replace(theme,
		[]byte(`<a:solidFill><a:schemeClr val="phClr"><a:lumMod val="50000"/></a:schemeClr></a:solidFill>`),
		[]byte(`<a:solidFill><a:srgbClr val="0000FF"/></a:solidFill>`),
		1)
	parts["/ppt/theme/theme1.xml"] = theme
	p := openFixture(t, buildPackageZipPanic(parts))
	defer p.Close()
	ref := styleAdvRefOf(t, p, RefFill)
	if ref.ThemeColor.RGB != "0000FF" {
		t.Errorf("ThemeColor = %+v, want 0000FF (实色条目不代入 ref 颜色)", ref.ThemeColor)
	}
}

// TestStyleAdv_ThemeColorUnresolvedWhenRefColorMissing 验证引用方颜色
// 无法解析时，phClr 条目不臆造取值（ThemeColor.RGB 为空）。
func TestStyleAdv_ThemeColorUnresolvedWhenRefColorMissing(t *testing.T) {
	// fillRef 用 schemeClr 指向不存在的 scheme → ref.Color 无法解析。
	p := styleAdvDeck(t, `<a:fillRef idx="1"><a:schemeClr val="noSuchScheme"/></a:fillRef>`)
	defer p.Close()
	ref := styleAdvRefOf(t, p, RefFill)
	if ref.Color.RGB != "" {
		t.Fatalf("ref color should be unresolved, got %s", ref.Color.RGB)
	}
	if ref.ThemeColor.RGB != "" {
		t.Errorf("ThemeColor.RGB = %q, want empty (不臆造)", ref.ThemeColor.RGB)
	}
	if ref.ThemeColor.Resolved {
		t.Errorf("ThemeColor.Resolved = true, want false")
	}
}

// TestStyleAdv_EffectRefNoColorDiagnostic 验证 effectRef 不产生颜色诊断
// （效果条目无颜色元素，属正常形态而非缺陷）。
func TestStyleAdv_EffectRefNoColorDiagnostic(t *testing.T) {
	p := styleAdvDeck(t, `<a:effectRef idx="1"/>`)
	defer p.Close()
	shapes, _ := mustSlides(t, p)[0].Shapes()
	_, diags, err := findShapeByID(t, shapes, 2).StyleMatrixRefs()
	if err != nil {
		t.Fatalf("StyleMatrixRefs: %v", err)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, "theme entry color not resolved") {
			t.Errorf("effectRef 不应产生颜色诊断: %+v", d)
		}
	}
}
