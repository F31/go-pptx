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
// 解析策略：完整 streaming xml.Decoder.Token()，避免 std xml 包对
// 重复子元素名的处理限制。
//
// 已知限制（TIMIR-01 遗留）：自闭合 <cond/> 列表之后紧邻空的
// <childTnLst></childTnLst> 的组合，在 std encoding/xml 的
// token 流上可能触发 "element closed by" 解析错位——此类输入会
// 以 TIMIR_PARSE_ERR 诊断报告，而不是崩溃或静默截断。后续计划用
// internal/xmlstore（自闭合语义处理正确）重写解析器彻底修复。
package ir

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
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
	TimeEventOnDoubleClick   TimeEventKind = "onDblClick"
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

func newAttrs(t xml.StartElement) tmlAttr {
	out := make(tmlAttr, len(t.Attr))
	for _, a := range t.Attr {
		out[a.Name.Local] = a.Value
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
	Opaque   xml.Name // 不识别时
	Set      bool     // a:set 标记
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

// tmlMedia 包装 audio/video 节点内的 cTn + tgtEl。
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
func projectTimingTree(part string, raw []byte) (PageTiming, error) {
	pt := PageTiming{}
	if len(raw) == 0 {
		return pt, nil
	}
	root, tnLstCount, diags, err := parseTimingRoot(bytes.NewReader(raw))
	pt.Diagnostics = append(pt.Diagnostics, diags...)
	if err != nil && !errors.Is(err, io.EOF) {
		pt.Diagnostics = append(pt.Diagnostics, Diagnostic{
			Code: "TIMIR_PARSE_ERR", Severity: SevError,
			Part:    part,
			Message: "p:timing 不是良构 XML：" + err.Error(),
		})
		return pt, fmt.Errorf("ir: timing parse: %w", err)
	}
	if tnLstCount > 1 {
		pt.Diagnostics = append(pt.Diagnostics, Diagnostic{
			Code: "TIMIR_MULTI_TNLST", Severity: SevWarning,
			Part:    part,
			Message: "multiple <p:tnLst> elements; only par children kept",
		})
	}
	if root == nil {
		return pt, nil
	}
	for _, par := range root {
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

// parseTimingRoot 进入 <timing> 后走全部 <tnLst> 段，收集所有 par。
//
// 返回值：par 数组（顺序保留）+ tnLst count + 诊断（多 tnLst / 错位 par）。
// tnLst count 用于 projectTimingTree 输出 TIMIR_MULTI_TNLST 诊断。
func parseTimingRoot(r io.Reader) ([]*tmlCTn, int, Diagnostics, error) {
	diags := Diagnostics{}
	dec := xml.NewDecoder(r)
	inTnLst := false
	tnLstCount := 0
	var out []*tmlCTn
	docEnded := false
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, tnLstCount, diags, err
			}
			return out, tnLstCount, diags, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if docEnded {
				return out, tnLstCount, diags, nil
			}
			switch t.Name.Local {
			case "tnLst":
				inTnLst = true
				tnLstCount++
				continue
			case "par":
				if !inTnLst {
					diags = append(diags, Diagnostic{
						Code: "TIMIR_PAR_OUTSIDE_TNLST", Severity: SevWarning,
						Message: "par outside tnLst; ignored",
					})
					if err := skipElementSkipBody(dec); err != nil {
						return out, tnLstCount, diags, err
					}
					continue
				}
				cn, err := readCTnElement(dec, t)
				if err != nil {
					return out, tnLstCount, diags, err
				}
				out = append(out, cn)
			case "timing":
				continue
			default:
				if err := skipElementSkipBody(dec); err != nil {
					return out, tnLstCount, diags, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "timing" {
				docEnded = true
				return out, tnLstCount, diags, nil
			}
			if t.Name.Local == "tnLst" {
				inTnLst = false
			}
		}
	}
}

// readCTnElement 在收到 <cTn>/<par>/<seq>/<set> 等的起始 token 后读取整个子树。
func readCTnElement(dec *xml.Decoder, start xml.StartElement) (*tmlCTn, error) {
	cn := &tmlCTn{
		Attrs: newAttrs(start),
		Kind:  tkCTn,
	}
	closeOn := start.Name.Local
	return cn, readCTnBody(dec, cn, closeOn)
}

// readCTnBody 在已知已进入特定元素 StartElement 后消费其内部。
// 参数 parentClose 是其容器元素的本地名（par/cTn/seq/set/audio/video/
// cmd/animmotion 等）；调用方在调用前已 consume 自身 StartElement。
func readCTnBody(dec *xml.Decoder, cn *tmlCTn, parentClose string) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "childTnLst":
				if err := readCTnLstChildren(dec, t.Name.Local, cn); err != nil {
					return err
				}
			case "cTn":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				cn.Children = append(cn.Children, inner)
			case "par":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				inner.Kind = tkPar
				cn.Children = append(cn.Children, inner)
			case "seq":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				inner.Kind = tkSeq
				cn.Children = append(cn.Children, inner)
			case "audio", "video":
				body, err := readMediaBody(dec, t)
				if err != nil {
					return err
				}
				child := &tmlCTn{Kind: tkAudio}
				if t.Name.Local == "video" {
					child.Kind = tkVideo
				}
				child.Body = body
				cn.Children = append(cn.Children, child)
			case "cmd":
				cmd, err := readCmdBody(dec)
				if err != nil {
					return err
				}
				inner := &tmlCTn{Kind: tkCMD, CMD: cmd}
				cn.Children = append(cn.Children, inner)
			case "anim", "animmotion", "animcolor", "animscale", "animrotate":
				anim, err := readAnimBody(dec, t.Name.Local)
				if err != nil {
					return err
				}
				cn.Children = append(cn.Children, &tmlCTn{Kind: tkAnim, Anim: anim})
			case "set":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				inner.Kind = tkSet
				cn.Children = append(cn.Children, inner)
			case "stCondLst":
				conds, err := readCondList(dec)
				if err != nil {
					return err
				}
				cn.Begin = conds
			case "endCondLst":
				conds, err := readCondList(dec)
				if err != nil {
					return err
				}
				cn.End = conds
			default:
				// 不识别：吞整段当 opaque 落记。
				if err := skipElementSkipBody(dec); err != nil {
					return err
				}
				cn.Children = append(cn.Children, &tmlCTn{
					Kind:   tkOpaque,
					Opaque: t.Name,
				})
			}
		case xml.EndElement:
			if t.Name.Local == parentClose {
				return nil
			}
		}
	}
}

// readMediaBody 处理 audio/video 内的 cMediaNode > cTn / tgtEl。
func readMediaBody(dec *xml.Decoder, start xml.StartElement) (*tmlMedia, error) {
	body := &tmlMedia{}
	for _, a := range start.Attr {
		if a.Name.Local == "vol" {
			if v, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
				body.Vol = v
			}
		}
	}
	inCMedia := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "cMediaNode" {
				inCMedia = true
				for _, a := range t.Attr {
					if a.Name.Local == "vol" {
						if v, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
							body.Vol = v
						}
					}
				}
				continue
			}
			if !inCMedia {
				if err := skipElementSkipBody(dec); err != nil {
					return nil, err
				}
				continue
			}
			switch t.Name.Local {
			case "cTn":
				cn, err := readCTnElement(dec, t)
				if err != nil {
					return nil, err
				}
				body.CTn = cn
			case "tgtEl":
				tgt, err := readTgtEl(dec)
				if err != nil {
					return nil, err
				}
				body.Tgt = tgt
			default:
				if err := skipElementSkipBody(dec); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "audio" || t.Name.Local == "video" || t.Name.Local == "cMediaNode" {
				if t.Name.Local == "cMediaNode" {
					inCMedia = false
				}
				if t.Name.Local == "audio" || t.Name.Local == "video" {
					return body, nil
				}
			}
		}
	}
}

// readCmdBody 在 cmd 起始 token 后读取 cTn 子节点。
func readCmdBody(dec *xml.Decoder) (*tmlCmd, error) {
	cmd := &tmlCmd{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "cTn":
				cn, err := readCTnElement(dec, t)
				if err != nil {
					return nil, err
				}
				cmd.CTn = cn
			default:
				if err := skipElementSkipBody(dec); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "cmd" {
				return cmd, nil
			}
		}
	}
}

// readAnimBody 处理 p:anim* 节点——可能是 anim/animmotion/animcolor/
// animscale/animrotate；本函数读 cTn, tgtEl, animateEffect 等子节点。
func readAnimBody(dec *xml.Decoder, parent string) (*tmlAnim, error) {
	anim := &tmlAnim{ParentLocal: parent}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "cTn":
				cn, err := readCTnElement(dec, t)
				if err != nil {
					return nil, err
				}
				anim.CTn = cn
			case "tgtEl":
				tgt, err := readTgtEl(dec)
				if err != nil {
					return nil, err
				}
				anim.Tgt = tgt
			case "animateEffect", "animateMotion", "animateColor", "animateScale", "animateRotate":
				eff := tmlEffect{Local: t.Name.Local}
				for _, a := range t.Attr {
					if a.Name.Local == "effectId" {
						if v, err := strconv.ParseInt(a.Value, 10, 64); err == nil {
							eff.EffectID = v
						}
					}
				}
				anim.Effect = eff
				if err := skipElementSkipBody(dec); err != nil {
					return nil, err
				}
			default:
				if err := skipElementSkipBody(dec); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == parent {
				return anim, nil
			}
		}
	}
}

// readTgtEl 处理 p:tgtEl 中的 spTgt/setTgt/inkTgt。
func readTgtEl(dec *xml.Decoder) (*tmlTgt, error) {
	tgt := &tmlTgt{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			raw := t.Name.Local
			tgt.ElementRaw = raw
			for _, a := range t.Attr {
				switch a.Name.Local {
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
			if err := skipElementSkipBody(dec); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if t.Name.Local == "tgtEl" {
				return tgt, nil
			}
		}
	}
}

// readCondList 读取 stCondLst / endCondLst 内的 cond 子节点。
//
// 调用方已消费了外层 StartElement（stCondLst/endCondLst）；本函数消费
// 内部直到对应的 EndElement。
func readCondList(dec *xml.Decoder) ([]tmlCond, error) {
	var out []tmlCond
	for {
		tok, err := dec.Token()
		if err != nil {
			return out, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "cond" {
				c := tmlCond{}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "evt":
						c.Evt = a.Value
					case "delay":
						c.Delay = a.Value
					}
				}
				// 跳过 cond 子树（自闭合或展开均可——self-closing 时
				// std 仍发射 EE 来对齐 depth）。
				if err := skipElementSkipBody(dec); err != nil {
					return out, err
				}
				out = append(out, c)
			} else {
				if err := skipElementSkipBody(dec); err != nil {
					return out, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "stCondLst" || t.Name.Local == "endCondLst" {
				return out, nil
			}
		}
	}
}

// readCTnLstChildren 处理 childTnLst 内的子节点，与 readCTnBody 行为一致
// 但不含 childTnLst 这个外层包裹（cTn 的 Children 是 childTnLst 的内部）。
func readCTnLstChildren(dec *xml.Decoder, parent string, cn *tmlCTn) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "cTn":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				cn.Children = append(cn.Children, inner)
			case "par":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				inner.Kind = tkPar
				cn.Children = append(cn.Children, inner)
			case "seq":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				inner.Kind = tkSeq
				cn.Children = append(cn.Children, inner)
			case "audio", "video":
				body, err := readMediaBody(dec, t)
				if err != nil {
					return err
				}
				child := &tmlCTn{Kind: tkAudio}
				if t.Name.Local == "video" {
					child.Kind = tkVideo
				}
				child.Body = body
				cn.Children = append(cn.Children, child)
			case "cmd":
				cmd, err := readCmdBody(dec)
				if err != nil {
					return err
				}
				cn.Children = append(cn.Children, &tmlCTn{Kind: tkCMD, CMD: cmd})
			case "anim", "animmotion", "animcolor", "animscale", "animrotate":
				anim, err := readAnimBody(dec, t.Name.Local)
				if err != nil {
					return err
				}
				cn.Children = append(cn.Children, &tmlCTn{Kind: tkAnim, Anim: anim})
			case "set":
				inner, err := readCTnElement(dec, t)
				if err != nil {
					return err
				}
				inner.Kind = tkSet
				cn.Children = append(cn.Children, inner)
			default:
				if err := skipElementSkipBody(dec); err != nil {
					return err
				}
				cn.Children = append(cn.Children, &tmlCTn{
					Kind:   tkOpaque,
					Opaque: t.Name,
				})
			}
		case xml.EndElement:
			if t.Name.Local == parent {
				return nil
			}
		}
	}
}

// skipElementSkipBody 跳过当前已读取 StartElement 的元素子树。
//
// 设计说明：encoding/xml 包对 `<foo/>` 仅发射一个 StartElement，没有
// EndElement，因此"等待 EndElement for foo"的策略会越界。本函数采
// 用 depth 计数，从 1 开始：StartElement 增加 / EndElement 减少；任一
// 点 depth<=0 时已"关闭"足够多。我们不依赖 name 匹配：外层循环由
// 调用方的"已知上下文"负责。
func skipElementSkipBody(dec *xml.Decoder) error {
	depth := 1
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth <= 0 {
				_ = t
				return nil
			}
		}
	}
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
				Opaque: opaqueFromXMLName(child.Opaque),
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

// opaqueFromXMLName 把一个未识别的任意元素名封装成 OpaqueNode。
func opaqueFromXMLName(name xml.Name) *OpaqueNode {
	if name.Local == "" {
		return nil
	}
	ns := ""
	if name.Space != "" {
		ns = name.Space
	}
	return &OpaqueNode{
		LocalName: name.Local,
		Meta:      map[string]string{"__ns": ns},
	}
}
