// 动画时序只读 IR（TIMIR-01，方案 §21.5）。
//
// 本文件实现 §21.5：把 p:timing 投影为只读时序 IR，供自动讲解视频
// 与 AI 内容理解场景回答"这页 PPT 的动画时间轴是什么"。**不提供
// 任何动画编辑 API**（R 档——只读报告）。
//
// 关键约束：
//   - 估计值 Duration 不承诺与 Office 客户端播放帧一致（方案 §21.5）；
//   - 未识别的动画子元素输出为 OpaqueNode 并计入 Diagnostic，不猜测、
//     不省略，如实报告 Partial/Untested 状态（方案 §21.5 实现约束）；
//   - 时序 IR 的估计值不得作为 AdvanceAfter 的计算输入（§21.5 不改变
//     21.3 PlanTimingSync 结论）。
//
// SchemaVersion 与基础 IR 共用 "go-pptx.ir/1.0"，JSON 字段在 v1 范围
// 向后兼容。
//
// 解析策略：以 internal/xmlstore.Scanner（自闭合语义处理正确：`<x/>`
// 仅发射一条 Start token 并带 SelfClosing=true，不发射独立 End）为底座
// 构建节点索引树，再按本地名规则把元素投影为 tmlCTn 内部 AST。前一版
// 直接驱动 std encoding/xml.Decoder，在 std 自闭合 cond 紧邻空
// childTnLst 的组合上触发 depth 同步偏差；xmlstore 的闭合校验依赖
// 显式元素栈，自闭合不再产生 End 事件，从而彻底避免该边缘 case。
package ir

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- §21.5 公共类型 ----------

// TimingNodeKind 描述 p:timing 下子节点的语义角色。
type TimingNodeKind string

const (
	TimeNodeRoot          TimingNodeKind = "root"
	TimeNodeParallel      TimingNodeKind = "parallel"
	TimeNodeSequence      TimingNodeKind = "sequence"
	TimeNodePrevious      TimingNodeKind = "previous"
	TimeNodeAudio         TimingNodeKind = "audio"
	TimeNodeVideo         TimingNodeKind = "video"
	TimeNodeAnimateEffect TimingNodeKind = "effect"
	TimeNodeAnimateMotion TimingNodeKind = "motion"
	TimeNodeAnimateColor  TimingNodeKind = "color"
	TimeNodeAnimateScale  TimingNodeKind = "scale"
	TimeNodeAnimateRotate TimingNodeKind = "rotate"
	TimeNodeCommand       TimingNodeKind = "command"
	TimeNodeSet           TimingNodeKind = "set"
	TimeNodeOpaqueKind    TimingNodeKind = "opaque"
)

// EffectClass 描述动画语义类别（入场 / 强调 / 退出）。
type EffectClass string

const (
	EffectClassEntrance EffectClass = "entrance"
	EffectClassEmphasis EffectClass = "emphasis"
	EffectClassExit     EffectClass = "exit"
	EffectClassMedia    EffectClass = "media"
	EffectClassOther    EffectClass = "other"
)

// TimeEventKind 是条件触发事件（p:cond@evt / endCondLst@evt）。
type TimeEventKind string

const (
	TimeEventBegin           TimeEventKind = "begin"
	TimeEventNext            TimeEventKind = "next"
	TimeEventEnd             TimeEventKind = "end"
	TimeEventOnClick         TimeEventKind = "onClick"
	TimeEventOnDoubleClick   TimeEventKind = "onDoubleClick"
	TimeEventOnMouseOver     TimeEventKind = "onMouseOver"
	TimeEventOnMouseOut      TimeEventKind = "onMouseOut"
	TimeEventOnStopAudio     TimeEventKind = "onStopAudio"
	TimeEventOnMediaBookmark TimeEventKind = "onMediaBookmark"
	TimeEventOnTrigger       TimeEventKind = "onTrigger"
	TimeEventOther           TimeEventKind = "other"
)

// Estimate 描述一个时间值是否可信。Estimated=true 标注"客户端实际帧
// 可能与本估计不同步"。
type Estimate struct {
	Value      int64  `json:"value"`
	Indefinite bool   `json:"indefinite,omitempty"`
	Estimated  bool   `json:"estimated,omitempty"`
	Source     string `json:"source,omitempty"`
}

// Condition 是条件触发（p:cond 或 endSync 子元素）。
type Condition struct {
	Event       TimeEventKind     `json:"event"`
	Delay       Estimate          `json:"delay,omitempty"`
	Trigger     *TriggerRef       `json:"trigger,omitempty"`
	OpaqueAttrs map[string]string `json:"opaqueAttrs,omitempty"`
}

// TriggerRef 表示 sender（trgt/target）或本节点的触发引用。
type TriggerRef struct {
	ShapeID  int64  `json:"shapeID,omitempty"`
	EffectID int64  `json:"effectID,omitempty"`
	AttrName string `json:"attrName,omitempty"`
	AttrVal  string `json:"attrVal,omitempty"`
}

// Target 描述该时间节点作用的对象（spTgt / setTgt / inkTgt）。
type Target struct {
	ShapeID     int64  `json:"shapeID,omitempty"`
	SubShapeID  int64  `json:"subShapeID,omitempty"`
	ElementName string `json:"elementName,omitempty"`
	EffectID    int64  `json:"effectID,omitempty"`
	Raw         string `json:"raw,omitempty"`
}

// AnimationEffect 描述一段动画的可呈现参数（a:animateEffect 等）。
type AnimationEffect struct {
	Class     EffectClass `json:"class"`
	Name      string      `json:"name,omitempty"`
	Duration  Estimate    `json:"duration,omitempty"`
	Direction string      `json:"direction,omitempty"`
	EffectID  int64       `json:"effectID,omitempty"`
}

// TimingNode 是 p:timing 的递归节点视图。
type TimingNode struct {
	ID          int64            `json:"id"`
	Kind        TimingNodeKind   `json:"kind"`
	PresetClass string           `json:"presetClass,omitempty"`
	Duration    Estimate         `json:"duration,omitempty"`
	Restart     string           `json:"restart,omitempty"`
	Fill        string           `json:"fill,omitempty"`
	Display     string           `json:"display,omitempty"`
	NodeType    string           `json:"nodeType,omitempty"`
	Begin       []Condition      `json:"begin,omitempty"`
	End         []Condition      `json:"end,omitempty"`
	Target      *Target          `json:"target,omitempty"`
	Effect      *AnimationEffect `json:"effect,omitempty"`
	Children    []*TimingNode    `json:"children,omitempty"`
	Opaque      *OpaqueNode      `json:"opaque,omitempty"`
}

// OpaqueNode 是无法解析的子树快照——如实报告 Partial/Untested。
type OpaqueNode struct {
	LocalName string            `json:"localName"`
	Meta      map[string]string `json:"meta,omitempty"`
	Children  []*OpaqueNode     `json:"children,omitempty"`
}

// PageTiming 是 Page 级别的时序摘要（根 + 概要统计）。
type PageTiming struct {
	Root        *TimingNode  `json:"root,omitempty"`
	Totals      TimingTotals `json:"totals"`
	Diagnostics Diagnostics  `json:"diagnostics,omitempty"`
}

// TimingTotals 是按页统计字段集合。
type TimingTotals struct {
	NodeCount      int `json:"nodeCount"`
	EstimatedCount int `json:"estimatedCount"`
	OpaqueCount    int `json:"opaqueCount"`
	AudioNodes     int `json:"audioNodes"`
	VideoNodes     int `json:"videoNodes"`
	EffectNodes    int `json:"effectNodes"`
}

// ---------- 内部：底层解析 AST ----------

// tmlAttr 是 cTn 简化属性。读完后仍以字符串承载，projectCTn 时再
// 解析为数字 / 枚举。
type tmlAttr map[string]string

func newAttrsFromNode(n *xmlstore.NodeRecord) tmlAttr {
	out := make(tmlAttr, len(n.Attrs))
	for _, a := range n.Attrs {
		out[a.Local()] = a.Value
	}
	return out
}

// tmlCTn 是 cTn 的内部表示，包含 cTn 自身属性 + 一个嵌套子节点数组。
type tmlCTn struct {
	Attrs    tmlAttr
	Children []*tmlCTn // 含所有子节点（par/seq/cTn/audio/video/...）
	Kind     tmlKind
	Body     *tmlMedia // 仅 audio/video 时
	Anim     *tmlAnim  // 仅 anim* 时
	CMD      *tmlCmd   // 仅 cmd 时
	Begin    []tmlCond
	End      []tmlCond
	Opaque   xmlstore.QName // 不识别时
	Set      bool           // a:set 标记
}

type tmlKind int

const (
	tkPar tmlKind = iota + 1
	tkSeq
	tkCTn
	tkAudio
	tkVideo
	tkCMD
	tkAnim
	tkSet
	tkOpaque
)

// tmlMedia 包装 audio/video 节点内的 cMediaNode > cTn / tgtEl。
type tmlMedia struct {
	Vol int64
	CTn *tmlCTn
	Tgt *tmlTgt
}

// tmlAnim 包装 anim / animmotion / animcolor / animscale / animrotate，
// 外层 Local 名（"anim"/"animmotion"/...）+ 内嵌的 cTn / tgtEl / 子 effect。
type tmlAnim struct {
	ParentLocal string
	Effect      tmlEffect
	CTn         *tmlCTn
	Tgt         *tmlTgt
}

// tmlEffect 描述一个效果的具体 preset 名（如 a:animateEffect）。
type tmlEffect struct {
	Local    string
	EffectID int64
}

// tmlCmd 包装 cmd 节点的 cTn。
type tmlCmd struct {
	CTn *tmlCTn
}

// tmlTgt 描述 p:tgtEl 中的 spTgt/setTgt/inkTgt。
type tmlTgt struct {
	SpShapeID  int64
	SetEffect  int64
	AttrName   string
	AttrVal    string
	ElementRaw string // raw tag local name
}

// tmlCond 描述 p:cond 子节点。
type tmlCond struct {
	Evt      string
	Delay    string
	TrigSpid int64 // 触发形状（onTrigger 时）
}

// ---------- 投影入口 ----------

// projectTimingTree 解析 p:timing 原始字节为 PageTiming。
//
// 行为：
//   - 输入空 → 返回 (PageTiming{}, nil)；
//   - 解析失败 → 返回 PageTiming{Diagnostics:[TIMIR_PARSE_ERR]} + 错误；
//   - 未识别子树 → 落入 OpaqueNode + 累计到 OpaqueCount。
//
// 实现：以 internal/xmlstore.Scanner 构建节点索引树，再按本地名规则
// 把每个元素投影到 tmlCTn AST。该路径无 std encoding/xml 自闭合导致的
// 深度同步问题——xmlstore 对 `<x/>` 仅发射 SelfClosing=true 的 Start，
// 不发射独立的 End 事件。
func projectTimingTree(part string, raw []byte) (PageTiming, error) {
	pt := PageTiming{}
	if len(raw) == 0 {
		return pt, nil
	}
	doc, err := xmlstore.Index(raw)
	if err != nil {
		pt.Diagnostics = append(pt.Diagnostics, Diagnostic{
			Code: "TIMIR_PARSE_ERR", Severity: SevError,
			Part:    part,
			Message: timingParseErrMessage(err),
		})
		return pt, fmt.Errorf("ir: timing parse: %w", err)
	}
	if doc == nil || doc.Root() == nil {
		return pt, nil
	}
	root := doc.Root()
	if root.QName.Local != "timing" {
		pt.Diagnostics = append(pt.Diagnostics, Diagnostic{
			Code: "TIMIR_BAD_ROOT", Severity: SevError,
			Part:    part,
			Message: "p:timing 根元素不是 timing，实际为 " + root.QName.String(),
		})
		return pt, fmt.Errorf("ir: timing parse: unexpected root <%s>", root.QName.String())
	}
	var pars []*tmlCTn
	tnLstCount := 0
	for _, childID := range root.Children {
		child := doc.Node(childID)
		switch child.QName.Local {
		case "tnLst":
			tnLstCount++
			// 收集该 tnLst 内的全部 par；保留文档序。
			for _, tnID := range child.Children {
				tn := doc.Node(tnID)
				if tn.QName.Local != "par" {
					pt.Diagnostics = append(pt.Diagnostics, Diagnostic{
						Code: "TIMIR_NON_PAR_IN_TNLST", Severity: SevWarning,
						Part:    part,
						Message: "tnLst 下出现非 par 子元素 <" + tn.QName.String() + ">；忽略",
					})
					continue
				}
				cn := readCTnFromNode(doc, tn, tkPar)
				pars = append(pars, cn)
			}
		default:
			// timing 根下的其他直接子元素（如 buildList）：如实记录。
			pt.Diagnostics = append(pt.Diagnostics, Diagnostic{
				Code: "TIMIR_TIMING_NON_TNLST", Severity: SevWarning,
				Part:    part,
				Message: "<timing> 下出现非 tnLst 直接子元素 <" + child.QName.String() + ">；忽略",
			})
		}
	}
	if tnLstCount > 1 {
		pt.Diagnostics = append(pt.Diagnostics, Diagnostic{
			Code: "TIMIR_MULTI_TNLST", Severity: SevWarning,
			Part:    part,
			Message: "multiple <p:tnLst> elements; only par children kept",
		})
	}
	for _, par := range pars {
		// par 的实际属性 / 子节点来自它内部的 cTn（OOXML 规范）：
		// <p:par><p:cTn id="..." dur="...">...</p:cTn></p:par>
		par.Kind = tkPar
		cn := par
		if len(par.Children) == 1 && par.Children[0].Kind == tkCTn {
			cn = par.Children[0]
		}
		tn := projectCTn(cn, &pt.Totals, pt.Diagnostics)
		tn.Kind = TimeNodeRoot
		if pt.Root == nil {
			pt.Root = tn
		} else {
			if pt.Root.Children == nil {
				pt.Root.Children = []*TimingNode{}
			}
			pt.Root.Children = append(pt.Root.Children, tn)
		}
	}
	sortDiags(pt.Diagnostics)
	return pt, nil
}

// timingParseErrMessage 把 xmlstore 错误展开为含字节偏移的可读消息。
func timingParseErrMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, io.EOF) {
		return "p:timing 输入截断（EOF）"
	}
	var se *xmlstore.SyntaxError
	if errors.As(err, &se) {
		return "p:timing 不是良构 XML：" + se.Error()
	}
	var de *xmlstore.DepthError
	if errors.As(err, &de) {
		return "p:timing 嵌套深度超限：" + de.Error()
	}
	return "p:timing 不是良构 XML：" + err.Error()
}

// readCTnFromNode 把一个 xmlstore 元素节点投影为 tmlCTn。
//
// 默认 kind 由 defaultKind 决定（<par>/<seq>/<set>/... 经调用方显式传入
// 对应 tmlKind；裸 cTn 走 tkCTn）。childTnLst 的子元素会被"扁平化"：
// 即 childTnLst 内部的 par/seq/cTn/audio/... 直接挂在父 tmlCTn 的
// Children 下，与前一版 parser 行为一致。
func readCTnFromNode(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, defaultKind tmlKind) *tmlCTn {
	cn := &tmlCTn{
		Attrs: newAttrsFromNode(n),
		Kind:  defaultKind,
	}
	for _, childID := range n.Children {
		child := doc.Node(childID)
		local := child.QName.Local
		switch local {
		case "cTn":
			inner := readCTnFromNode(doc, child, tkCTn)
			cn.Children = append(cn.Children, inner)
		case "par":
			inner := readCTnFromNode(doc, child, tkPar)
			cn.Children = append(cn.Children, inner)
		case "seq":
			inner := readCTnFromNode(doc, child, tkSeq)
			cn.Children = append(cn.Children, inner)
		case "set":
			inner := readCTnFromNode(doc, child, tkSet)
			cn.Children = append(cn.Children, inner)
		case "childTnLst":
			// childTnLst 是 cTn 的子节点列表包裹；语义上其内部子节点
			// 就是父 cTn 的时间子节点（OOXML：cTn.stCondLst /
			// cTn.endCondLst / cTn.childTnLst 平行），因此这里把
			// childTnLst 的直接子元素提升到父 cn.Children。
			for _, subID := range child.Children {
				sub := doc.Node(subID)
				switch sub.QName.Local {
				case "cTn":
					cn.Children = append(cn.Children, readCTnFromNode(doc, sub, tkCTn))
				case "par":
					cn.Children = append(cn.Children, readCTnFromNode(doc, sub, tkPar))
				case "seq":
					cn.Children = append(cn.Children, readCTnFromNode(doc, sub, tkSeq))
				case "set":
					cn.Children = append(cn.Children, readCTnFromNode(doc, sub, tkSet))
				case "audio":
					cn.Children = append(cn.Children, &tmlCTn{Kind: tkAudio, Body: readMediaFromNode(doc, sub)})
				case "video":
					cn.Children = append(cn.Children, &tmlCTn{Kind: tkVideo, Body: readMediaFromNode(doc, sub)})
				case "cmd":
					cn.Children = append(cn.Children, &tmlCTn{Kind: tkCMD, CMD: readCmdFromNode(doc, sub)})
				case "anim", "animmotion", "animcolor", "animscale", "animrotate":
					cn.Children = append(cn.Children, &tmlCTn{Kind: tkAnim, Anim: readAnimFromNode(doc, sub, sub.QName.Local)})
				default:
					cn.Children = append(cn.Children, &tmlCTn{
						Kind:   tkOpaque,
						Opaque: sub.QName,
					})
				}
			}
		case "stCondLst":
			cn.Begin = append(cn.Begin, readCondListFromNode(doc, child)...)
		case "endCondLst":
			cn.End = append(cn.End, readCondListFromNode(doc, child)...)
		case "audio":
			cn.Children = append(cn.Children, &tmlCTn{Kind: tkAudio, Body: readMediaFromNode(doc, child)})
		case "video":
			cn.Children = append(cn.Children, &tmlCTn{Kind: tkVideo, Body: readMediaFromNode(doc, child)})
		case "cmd":
			cn.Children = append(cn.Children, &tmlCTn{Kind: tkCMD, CMD: readCmdFromNode(doc, child)})
		case "anim", "animmotion", "animcolor", "animscale", "animrotate":
			cn.Children = append(cn.Children, &tmlCTn{Kind: tkAnim, Anim: readAnimFromNode(doc, child, local)})
		default:
			cn.Children = append(cn.Children, &tmlCTn{
				Kind:   tkOpaque,
				Opaque: child.QName,
			})
		}
	}
	return cn
}

// readMediaFromNode 处理 audio/video 内的 cMediaNode > cTn / tgtEl。
func readMediaFromNode(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *tmlMedia {
	body := &tmlMedia{}
	for _, a := range n.Attrs {
		if a.Local() == "vol" {
			if v, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
				body.Vol = v
			}
		}
	}
	for _, childID := range n.Children {
		child := doc.Node(childID)
		if child.QName.Local != "cMediaNode" {
			// cMediaNode 之外的直接子元素：如实保留为 OpaqueNode 不合
			// 适（tmlMedia 不承载 Children 字段），故此处静默丢弃。
			// 该路径只在损坏 / 异常文档上出现。
			continue
		}
		// cMediaNode 上的 vol 属性可覆盖外层 audio/video@vol。
		for _, a := range child.Attrs {
			if a.Local() == "vol" {
				if v, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
					body.Vol = v
				}
			}
		}
		for _, subID := range child.Children {
			sub := doc.Node(subID)
			switch sub.QName.Local {
			case "cTn":
				body.CTn = readCTnFromNode(doc, sub, tkCTn)
			case "tgtEl":
				body.Tgt = readTgtElFromNode(doc, sub)
			}
		}
	}
	return body
}

// readCmdFromNode 在 cmd 节点上读取其 cTn 子节点。
func readCmdFromNode(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *tmlCmd {
	cmd := &tmlCmd{}
	for _, childID := range n.Children {
		child := doc.Node(childID)
		if child.QName.Local == "cTn" {
			cmd.CTn = readCTnFromNode(doc, child, tkCTn)
		}
	}
	return cmd
}

// readAnimFromNode 处理 p:anim* 节点（anim/animmotion/animcolor/
// animscale/animrotate），读取 cTn、tgtEl 与 animateEffect 等子节点。
// parent 即外层本地名（"anim"/"animmotion"/...）。
func readAnimFromNode(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, parent string) *tmlAnim {
	anim := &tmlAnim{ParentLocal: parent}
	for _, childID := range n.Children {
		child := doc.Node(childID)
		switch child.QName.Local {
		case "cTn":
			anim.CTn = readCTnFromNode(doc, child, tkCTn)
		case "tgtEl":
			anim.Tgt = readTgtElFromNode(doc, child)
		case "animateEffect", "animateMotion", "animateColor", "animateScale", "animateRotate":
			eff := tmlEffect{Local: child.QName.Local}
			if v, ok := child.AttrLocal("effectId"); ok {
				if id, err := strconv.ParseInt(v, 10, 64); err == nil {
					eff.EffectID = id
				}
			}
			anim.Effect = eff
		}
	}
	return anim
}

// readTgtElFromNode 读取 p:tgtEl 内的 spTgt / setTgt / inkTgt。
//
// tgtEl 内部通常只有一个子元素，但 OOXML 允许复合形式；这里按子元素
// 出现顺序投影——后者覆盖前者，与解析顺序无歧义。
func readTgtElFromNode(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *tmlTgt {
	tgt := &tmlTgt{}
	for _, childID := range n.Children {
		child := doc.Node(childID)
		raw := child.QName.Local
		tgt.ElementRaw = raw
		for _, a := range child.Attrs {
			switch a.Local() {
			case "spid":
				if v, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
					tgt.SpShapeID = v
				}
			case "effectId":
				if v, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
					tgt.SetEffect = v
				}
			case "attrName":
				tgt.AttrName = a.Value
			case "attrVal":
				tgt.AttrVal = a.Value
			}
		}
	}
	return tgt
}

// readCondListFromNode 读取 stCondLst / endCondLst 内的 cond 子节点。
func readCondListFromNode(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) []tmlCond {
	var out []tmlCond
	for _, childID := range n.Children {
		child := doc.Node(childID)
		if child.QName.Local != "cond" {
			continue
		}
		c := tmlCond{}
		if v, ok := child.AttrLocal("evt"); ok {
			c.Evt = v
		}
		if v, ok := child.AttrLocal("delay"); ok {
			c.Delay = v
		}
		out = append(out, c)
	}
	return out
}

// ---------- 投影：tmlCTn 树 → TimingNode 树 ----------

// projectCTn 把单个 cTn 投影为 TimingNode 并累计 totals。
func projectCTn(cn *tmlCTn, totals *TimingTotals, diags Diagnostics) *TimingNode {
	if cn == nil {
		return nil
	}
	node := &TimingNode{
		Duration: durationsFromProto(cn.Attrs["dur"], "attribute"),
		Restart:  cn.Attrs["restart"],
		Fill:     cn.Attrs["fill"],
		Display:  cn.Attrs["display"],
		NodeType: cn.Attrs["nodeType"],
		Kind:     TimeNodePrevious,
	}
	if v, err := strconv.ParseInt(cn.Attrs["id"], 10, 64); err == nil {
		node.ID = v
	}
	if node.Duration.Estimated {
		totals.EstimatedCount++
	}
	if strings.EqualFold(strings.TrimSpace(node.NodeType), "tmRoot") {
		node.Kind = TimeNodeRoot
	}
	// Begin / End
	for _, c := range cn.Begin {
		node.Begin = append(node.Begin, projectCond(c))
	}
	for _, c := range cn.End {
		node.End = append(node.End, projectCond(c))
	}
	// 子节点
	for _, child := range cn.Children {
		switch child.Kind {
		case tkPar:
			sub := projectCTn(child, totals, diags)
			if sub != nil {
				sub.Kind = TimeNodeParallel
				node.Children = append(node.Children, sub)
			}
		case tkSeq:
			sub := projectCTn(child, totals, diags)
			if sub != nil {
				sub.Kind = TimeNodeSequence
				node.Children = append(node.Children, sub)
			}
		case tkCTn:
			node.Children = append(node.Children, projectCTn(child, totals, diags))
		case tkAudio:
			if sub := projectMedia(child, TimeNodeAudio); sub != nil {
				totals.AudioNodes++
				node.Children = append(node.Children, sub)
			}
		case tkVideo:
			if sub := projectMedia(child, TimeNodeVideo); sub != nil {
				totals.VideoNodes++
				node.Children = append(node.Children, sub)
			}
		case tkCMD:
			if child.CMD != nil && child.CMD.CTn != nil {
				sub := projectCTn(child.CMD.CTn, totals, diags)
				if sub != nil {
					sub.Kind = TimeNodeCommand
					node.Children = append(node.Children, sub)
				}
			}
		case tkAnim:
			if sub := projectAnim(child, totals, diags); sub != nil {
				node.Children = append(node.Children, sub)
				// p:anim / animmotion / animcolor / animscale / animrotate
				// 都被视为"动画节点"，计入 EffectNodes。
				totals.EffectNodes++
			}
		case tkSet:
			sub := projectCTn(child, totals, diags)
			if sub != nil {
				sub.Kind = TimeNodeSet
				node.Children = append(node.Children, sub)
			}
		case tkOpaque:
			node.Children = append(node.Children, &TimingNode{
				Kind:   TimeNodeOpaqueKind,
				Opaque: opaqueFromQName(child.Opaque),
			})
			totals.OpaqueCount++
		}
	}
	totals.NodeCount++
	return node
}

// projectMedia 把 audio/video 节点投影。
func projectMedia(cn *tmlCTn, kind TimingNodeKind) *TimingNode {
	if cn == nil || cn.Body == nil || cn.Body.CTn == nil {
		return nil
	}
	// audio/video 没有专用 totals，单独累加由调用方做。
	node := &TimingNode{Kind: kind}
	if v, err := strconv.ParseInt(cn.Attrs["id"], 10, 64); err == nil {
		node.ID = v
	}
	if body := cn.Body; body != nil {
		if body.CTn != nil {
			sub := projectCTn(body.CTn, &TimingTotals{}, Diagnostics{})
			if sub != nil {
				node.Duration = sub.Duration
				node.Restart = sub.Restart
				node.Fill = sub.Fill
				node.Display = sub.Display
				node.NodeType = sub.NodeType
				node.ID = sub.ID
				node.Begin = sub.Begin
				node.End = sub.End
			}
		}
		if body.Tgt != nil {
			node.Target = projectTgt(body.Tgt)
		}
	}
	return node
}

// projectAnim 把 anim* 节点投影。
func projectAnim(cn *tmlCTn, totals *TimingTotals, diags Diagnostics) *TimingNode {
	node := &TimingNode{Kind: TimeNodeAnimateEffect}
	if cn.Anim == nil {
		return nil
	}
	switch cn.Anim.ParentLocal {
	case "anim":
		node.Kind = TimeNodeAnimateEffect
	case "animmotion":
		node.Kind = TimeNodeAnimateMotion
	case "animcolor":
		node.Kind = TimeNodeAnimateColor
	case "animscale":
		node.Kind = TimeNodeAnimateScale
	case "animrotate":
		node.Kind = TimeNodeAnimateRotate
	}
	if cn.Anim.CTn != nil {
		sub := projectCTn(cn.Anim.CTn, totals, diags)
		if sub != nil {
			node.ID = sub.ID
			node.Duration = sub.Duration
			node.Begin = sub.Begin
			node.End = sub.End
		}
	}
	if cn.Anim.Effect.Local != "" {
		preset := cn.Anim.Effect.Local
		node.PresetClass = preset
		node.Effect = &AnimationEffect{
			Name:     preset,
			Class:    classifyAnim(preset),
			EffectID: cn.Anim.Effect.EffectID,
		}
	}
	if cn.Anim.Tgt != nil {
		node.Target = projectTgt(cn.Anim.Tgt)
	}
	totals.NodeCount++
	return node
}

// projectTgt 把 tmlTgt 投影为 *Target。
func projectTgt(tgt *tmlTgt) *Target {
	if tgt == nil {
		return nil
	}
	out := &Target{Raw: tgt.ElementRaw}
	switch tgt.ElementRaw {
	case "spTgt":
		out.ElementName = "sp"
		out.ShapeID = tgt.SpShapeID
	case "setTgt":
		out.ElementName = "set"
		out.EffectID = tgt.SetEffect
	case "inkTgt":
		out.ElementName = "ink"
	default:
		out.ElementName = "other"
	}
	return out
}

// projectCond 把 tmlCond 投影为 Condition。
func projectCond(c tmlCond) Condition {
	out := Condition{Event: eventFromString(c.Evt)}
	if v := strings.TrimSpace(c.Delay); v != "" {
		out.Delay = delaysFromProto(v)
	}
	if c.TrigSpid != 0 {
		out.Trigger = &TriggerRef{ShapeID: c.TrigSpid}
	}
	return out
}

// classifyAnim 简化映射。
func classifyAnim(name string) EffectClass {
	n := strings.ToLower(name)
	switch n {
	case "fly", "fade", "wipe", "split", "reveal", "push", "cover",
		"glide", "rise", "peel", "barn", "fall", "warp":
		return EffectClassEntrance
	case "wheel", "pinwheel", "pulse", "blink", "grow", "shrink",
		"highlight", "wave", "bounce", "spiral", "boil", "swing",
		"flash", "teeter", "flicker", "stretch":
		return EffectClassEmphasis
	}
	if strings.HasSuffix(n, "out") {
		return EffectClassExit
	}
	return EffectClassOther
}

// eventFromString 名称 → 枚举。
func eventFromString(s string) TimeEventKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "begin":
		return TimeEventBegin
	case "next":
		return TimeEventNext
	case "end":
		return TimeEventEnd
	case "onclick":
		return TimeEventOnClick
	case "ondblclick":
		return TimeEventOnDoubleClick
	case "onmouseover":
		return TimeEventOnMouseOver
	case "onmouseout":
		return TimeEventOnMouseOut
	case "onstopaudio":
		return TimeEventOnStopAudio
	case "onmediabookmark":
		return TimeEventOnMediaBookmark
	case "ontrigger":
		return TimeEventOnTrigger
	}
	return TimeEventOther
}

// durationsFromProto / delaysFromProto 是 §21.5 时间估计的转换。
func durationsFromProto(raw, source string) Estimate {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Estimate{Indefinite: true, Source: source + "-missing"}
	}
	if strings.EqualFold(raw, "indefinite") {
		return Estimate{Indefinite: true, Source: source}
	}
	if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return Estimate{Value: v, Source: source}
	}
	return Estimate{Source: source + "-malformed", Estimated: true}
}

func delaysFromProto(raw string) Estimate {
	return durationsFromProto(raw, "delay")
}

// opaqueFromQName 把一个未识别的任意元素名封装成 OpaqueNode。
//
// 与上一版的区别：xmlstore.NodeRecord 上的 Namespace 才是元素 URI 解析
// 结果；tmlCTn.Opaque 只携带 QName（前缀 + 本地名）足够供审计/诊断使用，
// URI 信息可由调用方按需从 NodeRecord.Namespace 检索（TIMIR-01 不需要）。
func opaqueFromQName(name xmlstore.QName) *OpaqueNode {
	if name.Local == "" {
		return nil
	}
	return &OpaqueNode{
		LocalName: name.Local,
		Meta:      map[string]string{"__prefix": name.Prefix},
	}
}

// 保留 bytes 包引用以备未来扩展（避免 lint 警告 unused import）。
var _ = bytes.NewReader

// 保留 bytes 包引用以备未来扩展（避免 lint 警告 unused import）。
var _ = bytes.NewReader
