package pptx

import (
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// p14 命名空间 URI（Microsoft PowerPoint 2010 扩展）。仅在 morph 等
// V2.6 后切分项使用时引入；本轮白名单不涉及 p14 元素，仅记录常量。
const nsP14ML = "http://schemas.microsoft.com/office/powerpoint/2010/main"

// ECMA-376 ST_TransitionType 的受控子集（ANIM-02 基础子集）。morph
// （p14:morph）等 V2.6 后切分项不在白名单内；本包不在过渡子树写入时
// 接收 p14 前缀元素，检测到整体返回 ErrUnsupportedEdit（不做部分合并）。
var transitionTypeURIs = map[string]string{
	"":         "none",
	"fade":     "fade",
	"push":     "push",
	"wipe":     "wipe",
	"split":    "split",
	"cover":    "cover",
	"cut":      "cut",
	"dissolve": "dissolve",
}

// transitionTypeURI 反查：none/fade/.../dissolve 全部映射到合法子元素 URI。
func transitionTypeURI(t TransitionType) (string, bool) {
	s := string(t)
	for uri, name := range transitionTypeURIs {
		if name == s {
			if uri == "" {
				return "", true // none 表示 p:transition 自闭合/无子元素
			}
			return uri, true
		}
	}
	return "", false
}

// TransitionSpeed 是 p:transition@spd 的三档枚举。
type TransitionSpeed string

const (
	SpeedSlow TransitionSpeed = "slow"
	SpeedMed  TransitionSpeed = "med"
	SpeedFast TransitionSpeed = "fast"
)

// PushDir/WipeDir/CoverDir 共享 dir 枚举（l/r/u/d）。
type TransitionDir string

const (
	DirLeft  TransitionDir = "l"
	DirRight TransitionDir = "r"
	DirUp    TransitionDir = "u"
	DirDown  TransitionDir = "d"
)

// SplitDir 是 split 专用 dir 枚举（in/out）。
type SplitDir string

const (
	SplitIn  SplitDir = "in"
	SplitOut SplitDir = "out"
)

// SplitAxis 是 split 专用 axis 枚举（hor/vert）。
type SplitAxis string

const (
	AxisHor  SplitAxis = "hor"
	AxisVert SplitAxis = "vert"
)

// FadeOptions 描述 fade 的可选 throughBlack。
type FadeOptions struct {
	ThroughBlack bool
}

// CutOptions 描述 cut 的可选 throughBlack（语义为"瞬切"）。
type CutOptions struct {
	ThroughBlack bool
}

// TransitionType 枚举对应 8 个 ECMA ST_TransitionType 子集。
type TransitionType string

const (
	TransitionNone     TransitionType = "none"
	TransitionFade     TransitionType = "fade"
	TransitionPush     TransitionType = "push"
	TransitionWipe     TransitionType = "wipe"
	TransitionSplit    TransitionType = "split"
	TransitionCover    TransitionType = "cover"
	TransitionCut      TransitionType = "cut"
	TransitionDissolve TransitionType = "dissolve"
)

// TransitionSpec 是受限过渡的对外配置。
//
//   - Type 必须为白名单值；其他 ErrInvalidArgument。
//   - Fade/Cut 字段只在 Type==Fade/Cut 时使用，其余忽略。
//   - Dir 字段用于 push/wipe/cover；split 字段仅在 Type==Split 时使用。
//   - Speed 默认 Fast；空值写入会被规范化为 fast。
//   - AdvanceClick 控制 p:transition@advClick（nil 默认 true = 允许点击翻页；
//     BoolPtr(false) 显式写入 advClick="0"）；用指针区分"未设置"与"显式关闭"。
//   - AdvanceAfter 不在本结构——由 Slide.SetAdvanceAfter 维护（避免与
//     AUDIO-02 计时计划双源；SetTransition 不触碰 advTm）。
type TransitionSpec struct {
	Type         TransitionType
	Speed        TransitionSpeed
	AdvanceClick *bool
	Fade         FadeOptions
	Cut          CutOptions
	Dir          TransitionDir
	SplitDir     SplitDir
	SplitAxis    SplitAxis
}

// BoolPtr 返回 bool 的指针字面量（便于测试与调用方构造）。
func BoolPtr(b bool) *bool { return &b }

// Transition 返回当前页面的过渡设置；无 p:transition 节点返回 ErrNotFound。
//
// 读取规则：
//   - spd 缺省视为 fast。
//   - advClick 缺省视为 true（PowerPoint 默认）。
//   - 子元素仅识别白名单内的 8 种；识别失败返回 ErrUnsupportedEdit。
//   - 出现 p14 子元素（morph 等本轮未支持）返回 ErrUnsupportedEdit。
func (s *Slide) Transition() (TransitionSpec, error) {
	if err := s.alive(); err != nil {
		return TransitionSpec{}, Annotate(err, "Slide.Transition")
	}
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return TransitionSpec{}, Annotate(err, "Slide.Transition")
	}
	root := doc.Root()
	if root == nil {
		return TransitionSpec{}, Annotate(ErrStaleHandle, "Slide.Transition")
	}
	tr := findTransition(doc, root)
	if tr == nil {
		return TransitionSpec{}, Annotate(ErrNotFound, "Slide.Transition")
	}
	spec, err := readTransitionSpec(doc, tr)
	if err != nil {
		return TransitionSpec{}, Annotate(err, "Slide.Transition")
	}
	return spec, nil
}

// SetTransition 受限写入 p:transition 节点。
//
// 行为：
//   - 容器不存在则按 CT_Slide 子元素序（cSld/clrMapOvr 之后、timing 之前）创建。
//   - 容器已存在则原地重建内容：保留 advTm（不动 SetAdvanceAfter 写入），
//     其余子元素与属性按 spec 重写。
//   - 子元素若非白名单（含 p14:morph），整体拒绝 ErrUnsupportedEdit，
//     不做部分合并。
//   - 单事务提交，失败恢复 pending。
func (s *Slide) SetTransition(spec TransitionSpec) error {
	if err := s.alive(); err != nil {
		return Annotate(err, "Slide.SetTransition")
	}
	if !isKnownTransitionType(spec.Type) {
		return &OperationError{
			Op: "Slide.SetTransition", Err: ErrInvalidArgument,
			Message: "unsupported transition type: " + string(spec.Type),
		}
	}
	if spec.Speed == "" {
		spec.Speed = SpeedFast
	} else if !isKnownSpeed(spec.Speed) {
		return &OperationError{
			Op: "Slide.SetTransition", Err: ErrInvalidArgument,
			Message: "unsupported transition speed: " + string(spec.Speed),
		}
	}
	if err := validateTransitionSpec(spec); err != nil {
		return Annotate(err, "Slide.SetTransition")
	}

	doc, err := s.p.docOf(s.part)
	if err != nil {
		return Annotate(err, "Slide.SetTransition")
	}
	root := doc.Root()
	if root == nil {
		return Annotate(ErrStaleHandle, "Slide.SetTransition")
	}
	tr := findTransition(doc, root)
	advTm := ""
	if tr != nil {
		if v, ok := tr.Attr("", "advTm"); ok {
			advTm = v
		}
	}
	containerXML, err := buildTransitionContainer(spec, advTm)
	if err != nil {
		return Annotate(err, "Slide.SetTransition")
	}

	var patches []xmlstore.SpanPatch
	if tr == nil {
		// 创建：插在 cSld/clrMapOvr 之后、timing 之前。
		var anchor *xmlstore.NodeRecord
		for _, cid := range root.Children {
			c := doc.Node(cid)
			if c == nil || c.Namespace != nsPresentationML {
				continue
			}
			if c.Local() == "timing" {
				anchor = c
				break
			}
		}
		var ap xmlstore.SpanPatch
		if anchor != nil {
			ap, err = xmlstore.InsertBefore(anchor, []byte(containerXML))
		} else {
			ap, err = xmlstore.AppendChild(root, []byte(containerXML))
		}
		if err != nil {
			return Annotate(mapXMLError(err), "Slide.SetTransition")
		}
		patches = append(patches, ap)
	} else {
		// 替换：删除旧容器整段，在同位置插入新容器。保留同一 OpenStart 之前
		// 与 CloseEnd 之后的字节；整段 SpanPatch 完成原子替换。
		patches = append(patches, xmlstore.SpanPatch{
			Start:       tr.Source.Start,
			End:         tr.Source.End,
			Replacement: []byte(containerXML),
		})
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "Slide.SetTransition")
	}
	if err := s.p.stagePatch(s.part, out); err != nil {
		return Annotate(err, "Slide.SetTransition")
	}
	s.p.commit()
	return nil
}

// RemoveTransition 删除整个 p:transition 节点（无论子元素与属性）。
// 不影响 p:timing 树（按 §24 ANIM-02 验收"不破坏原 timing 树"）。
// 页面无 transition 视为 no-op（不报错）。
func (s *Slide) RemoveTransition() error {
	if err := s.alive(); err != nil {
		return Annotate(err, "Slide.RemoveTransition")
	}
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return Annotate(err, "Slide.RemoveTransition")
	}
	root := doc.Root()
	if root == nil {
		return Annotate(ErrStaleHandle, "Slide.RemoveTransition")
	}
	tr := findTransition(doc, root)
	if tr == nil {
		return nil
	}
	patch := xmlstore.SpanPatch{
		Start:       tr.Source.Start,
		End:         tr.Source.End,
		Replacement: []byte(""),
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return Annotate(mapXMLError(err), "Slide.RemoveTransition")
	}
	if err := s.p.stagePatch(s.part, out); err != nil {
		return Annotate(err, "Slide.RemoveTransition")
	}
	s.p.commit()
	return nil
}

// ---------- 内部辅助 ----------

// findTransition 定位 p:sld 的 p:transition 直接子元素（按 CT_Slide 序）。
func findTransition(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c != nil && c.Namespace == nsPresentationML && c.Local() == "transition" {
			return c
		}
	}
	return nil
}

// findTiming 定位 p:sld 的 p:timing 直接子元素（若存在）。
func findTiming(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c != nil && c.Namespace == nsPresentationML && c.Local() == "timing" {
			return c
		}
	}
	return nil
}

// readTransitionSpec 解析受限 p:transition 容器为 TransitionSpec。
//
// 拒绝策略：
//   - 子元素非白名单（含 p14:*）→ ErrUnsupportedEdit。
//   - 子元素的属性非白名单 → ErrUnsupportedEdit。
//   - 容器属性 advTm 仅作为占位读出（不进入 spec，由 SetAdvanceAfter 管理）。
func readTransitionSpec(doc *xmlstore.XMLDocument, tr *xmlstore.NodeRecord) (TransitionSpec, error) {
	spec := TransitionSpec{
		Speed:        SpeedFast,
		AdvanceClick: BoolPtr(true),
	}
	if v, ok := tr.Attr("", "spd"); ok {
		switch TransitionSpeed(v) {
		case SpeedSlow, SpeedMed, SpeedFast:
			spec.Speed = TransitionSpeed(v)
		default:
			return TransitionSpec{}, &OperationError{
				Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
				Message: "unsupported transition spd: " + v,
			}
		}
	}
	if v, ok := tr.Attr("", "advClick"); ok {
		switch v {
		case "0":
			spec.AdvanceClick = BoolPtr(false)
		case "1":
			spec.AdvanceClick = BoolPtr(true)
		default:
			return TransitionSpec{}, &OperationError{
				Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
				Message: "unsupported transition advClick: " + v,
			}
		}
	}

	// 解析子元素：必须正好 0（none）或 1（其余类型）。
	typeChild := -1
	for i, cid := range tr.Children {
		c := doc.Node(cid)
		if c == nil {
			continue
		}
		if c.Namespace != nsPresentationML {
			// 含 mc:AlternateContent / p14:* 等未支持命名空间。
			return TransitionSpec{}, &OperationError{
				Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
				Message: "unsupported transition child namespace: " + c.Namespace,
			}
		}
		switch c.Local() {
		case "fade", "push", "wipe", "split", "cover", "cut", "dissolve":
			typeChild = i
		default:
			return TransitionSpec{}, &OperationError{
				Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
				Message: "unsupported transition child: " + c.Local(),
			}
		}
	}
	if typeChild < 0 {
		// 没有子元素视为 none；若无属性也允许（自闭合或空 transition）。
		// 含 advTm/spd/advClick 但无 type 子元素时，按"自定义 none"对待
		// （保留 spd/advClick）。
		spec.Type = TransitionNone
		return spec, nil
	}
	childNode := doc.Node(tr.Children[typeChild])

	switch childNode.Local() {
	case "fade":
		spec.Type = TransitionFade
		if v, ok := childNode.Attr("", "throughBlack"); ok {
			if v != "0" && v != "1" {
				return TransitionSpec{}, &OperationError{
					Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
					Message: "unsupported fade throughBlack: " + v,
				}
			}
			spec.Fade.ThroughBlack = v == "1"
		}
	case "push", "wipe", "cover":
		spec.Type = TransitionType(childNode.Local())
		spec.Dir, _ = parseDir(childNode)
	case "cut":
		spec.Type = TransitionCut
		if v, ok := childNode.Attr("", "throughBlack"); ok {
			if v != "0" && v != "1" {
				return TransitionSpec{}, &OperationError{
					Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
					Message: "unsupported cut throughBlack: " + v,
				}
			}
			spec.Cut.ThroughBlack = v == "1"
		}
	case "dissolve":
		spec.Type = TransitionDissolve
	case "split":
		spec.Type = TransitionSplit
		if v, ok := childNode.Attr("", "dir"); ok {
			if SplitDir(v) != SplitIn && SplitDir(v) != SplitOut {
				return TransitionSpec{}, &OperationError{
					Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
					Message: "unsupported split dir: " + v,
				}
			}
			spec.SplitDir = SplitDir(v)
		}
		if v, ok := childNode.Attr("", "axis"); ok {
			if SplitAxis(v) != AxisHor && SplitAxis(v) != AxisVert {
				return TransitionSpec{}, &OperationError{
					Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
					Message: "unsupported split axis: " + v,
				}
			}
			spec.SplitAxis = SplitAxis(v)
		}
	}
	// 检查子元素是否含未知属性（白名单外的属性视为非 spec）。
	for _, a := range childNode.Attrs {
		if !isKnownChildAttr(childNode.Local(), a.RawName) {
			return TransitionSpec{}, &OperationError{
				Op: "readTransitionSpec", Err: ErrUnsupportedEdit,
				Message: "unsupported transition child attribute: " + a.RawName,
			}
		}
	}
	return spec, nil
}

// parseDir 解析 dir 属性，缺失返回空。
func parseDir(n *xmlstore.NodeRecord) (TransitionDir, bool) {
	v, ok := n.Attr("", "dir")
	if !ok {
		return "", false
	}
	switch TransitionDir(v) {
	case DirLeft, DirRight, DirUp, DirDown:
		return TransitionDir(v), true
	}
	return "", false
}

// isKnownTransitionType 报告 t 是否在白名单内。
func isKnownTransitionType(t TransitionType) bool {
	switch t {
	case TransitionNone, TransitionFade, TransitionPush, TransitionWipe,
		TransitionSplit, TransitionCover, TransitionCut, TransitionDissolve:
		return true
	}
	return false
}

// isKnownSpeed 报告 spd 是否在白名单内。
func isKnownSpeed(s TransitionSpeed) bool {
	switch s {
	case SpeedSlow, SpeedMed, SpeedFast:
		return true
	}
	return false
}

// isKnownChildAttr 返回 child 上的属性是否在白名单内。
func isKnownChildAttr(child, attr string) bool {
	switch child {
	case "fade":
		return attr == "throughBlack"
	case "push", "wipe", "cover":
		return attr == "dir"
	case "cut":
		return attr == "throughBlack"
	case "split":
		return attr == "dir" || attr == "axis"
	case "dissolve":
		return false // dissolve 不带属性
	}
	return false
}

// validateTransitionSpec 校验 Spec 字段组合（类型+dir/split 等）。
func validateTransitionSpec(spec TransitionSpec) error {
	switch spec.Type {
	case TransitionFade:
		// Fade 不需要 dir
	case TransitionPush, TransitionWipe, TransitionCover:
		if spec.Dir != "" {
			switch spec.Dir {
			case DirLeft, DirRight, DirUp, DirDown:
			default:
				return &OperationError{
					Op: "validateTransitionSpec", Err: ErrInvalidArgument,
					Message: "unsupported dir: " + string(spec.Dir),
				}
			}
		}
	case TransitionSplit:
		if spec.SplitDir != "" {
			if spec.SplitDir != SplitIn && spec.SplitDir != SplitOut {
				return &OperationError{
					Op: "validateTransitionSpec", Err: ErrInvalidArgument,
					Message: "unsupported split dir: " + string(spec.SplitDir),
				}
			}
		}
		if spec.SplitAxis != "" {
			if spec.SplitAxis != AxisHor && spec.SplitAxis != AxisVert {
				return &OperationError{
					Op: "validateTransitionSpec", Err: ErrInvalidArgument,
					Message: "unsupported split axis: " + string(spec.SplitAxis),
				}
			}
		}
	}
	return nil
}

// buildTransitionContainer 生成完整 p:transition 元素字节。
// advTm 为空时不写入该属性；spd/advClick 始终写入（规范化默认）。
func buildTransitionContainer(spec TransitionSpec, advTm string) (string, error) {
	var sb strings.Builder
	sb.WriteString(`<p:transition`)
	sb.WriteString(` spd="`)
	sb.WriteString(string(spec.Speed))
	sb.WriteString(`"`)
	click := true
	if spec.AdvanceClick != nil {
		click = *spec.AdvanceClick
	}
	if click {
		sb.WriteString(` advClick="1"`)
	} else {
		sb.WriteString(` advClick="0"`)
	}
	if advTm != "" {
		sb.WriteString(` advTm="`)
		sb.WriteString(advTm)
		sb.WriteString(`"`)
	}
	uri, ok := transitionTypeURI(spec.Type)
	if !ok {
		return "", &OperationError{
			Op: "buildTransitionContainer", Err: ErrInvalidArgument,
			Message: "unknown transition type: " + string(spec.Type),
		}
	}
	if uri == "" {
		// none：自闭合。
		sb.WriteString(`/>`)
		return sb.String(), nil
	}
	sb.WriteString(`>`)
	sb.WriteString(`<p:`)
	sb.WriteString(uri)
	switch spec.Type {
	case TransitionFade:
		if spec.Fade.ThroughBlack {
			sb.WriteString(` throughBlack="1"`)
		}
	case TransitionPush, TransitionWipe, TransitionCover:
		if spec.Dir != "" {
			sb.WriteString(` dir="`)
			sb.WriteString(string(spec.Dir))
			sb.WriteString(`"`)
		}
	case TransitionSplit:
		if spec.SplitDir != "" {
			sb.WriteString(` dir="`)
			sb.WriteString(string(spec.SplitDir))
			sb.WriteString(`"`)
		}
		if spec.SplitAxis != "" {
			sb.WriteString(` axis="`)
			sb.WriteString(string(spec.SplitAxis))
			sb.WriteString(`"`)
		}
	case TransitionCut:
		if spec.Cut.ThroughBlack {
			sb.WriteString(` throughBlack="1"`)
		}
	case TransitionDissolve:
		// 无属性
	}
	sb.WriteString(`/>`)
	sb.WriteString(`</p:transition>`)
	// 防御性检查：确认无非法字符。
	if strings.ContainsRune(sb.String(), 0) {
		return "", &OperationError{
			Op: "buildTransitionContainer", Err: ErrInvalidArgument,
			Message: "null byte in transition container",
		}
	}
	_ = strconv.Itoa // 保留 strconv 以备扩展使用
	return sb.String(), nil
}
