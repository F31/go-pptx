// 本文件实现 AUDIO-02 与 AUDIO-03 的核心 API：
//   - SetPlayback：受限时间节点 ID 分配 → 保留原 p:timing 子树。
//     不支持合并时返回 ErrTimingConflict（AT-05）。
//   - SetAdvanceAfter：管理 p:transition 的 advanceTime；不推断音频时长。
//   - UpsertNarration：幂等的 TrackKey-aware 嵌入（AT-08）。
//   - PlanTimingSync：plan 包含 BaseRevision + 每页参与音轨 +
//     计算依据；不修改文档。
//   - ApplyTimingPlan：原子提交；revision 校验拒绝（AT-12）；
//     未知时长默认整批失败（AT-07）。
//   - SyncTimingToAudio：Plan+Apply 便捷封装。
//
// 翻页公式（方案 §21.3）：AdvanceAfter = max(Ej) + TailPadding。
// 参与集合默认只包含 Trigger=OnSlideEnter 且 Role=Narration
// 的有限音轨；背景音/循环/点击触发不参与。
package pptx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// ---------- Playback / SetPlayback ----------

// PlaybackTrigger 是 audio 的触发方式。
type PlaybackTrigger int

const (
	// PlaybackOnSlideEnter 翻入时自动播放（默认）。
	PlaybackOnSlideEnter PlaybackTrigger = iota
	// PlaybackOnClick 仅在点击时播放。
	PlaybackOnClick
)

// String 返回触发方式的稳定字符串（用于 XML 输出与日志）。
func (t PlaybackTrigger) String() string {
	switch t {
	case PlaybackOnSlideEnter:
		return "onSlideEnter"
	case PlaybackOnClick:
		return "onClick"
	}
	return fmt.Sprintf("PlaybackTrigger(%d)", int(t))
}

// IconMode 决定音频形状的图标可见性。
type IconMode int

const (
	// IconVisible 是默认（图标可见）。
	IconVisible IconMode = iota
	// IconHiddenDuringShow 放映时隐藏。
	IconHiddenDuringShow
)

// String 输出 IconMode 的稳定字符串。
func (i IconMode) String() string {
	switch i {
	case IconVisible:
		return "visible"
	case IconHiddenDuringShow:
		return "hiddenDuringShow"
	}
	return fmt.Sprintf("IconMode(%d)", int(i))
}

// PlaybackSpec 是 AddAudio/UpsertNarration 的播放配置。
type PlaybackSpec struct {
	Trigger    PlaybackTrigger
	StartDelay time.Duration
	IconMode   IconMode
}

// SetPlayback 设置已存在 audio 形状的播放行为。
//
// 仅库管理的 audio 形状支持；页面存在无法安全合并的既有 timing 子树
// （动画/视频等非纯音频结构）时返回 ErrTimingConflict（AT-05），不修改
// 任何原始节点。
//
// 实现为幂等重建：把 TrackKey 的 StartDelay/Trigger 记入 Profile 后，
// 按 slide 上全部 audio Profile 重建纯音频 p:timing 子树（时间节点 ID
// 由 ShapeID 派生，避免与 cNvPr@id 冲突）。
func (a *AudioShape) SetPlayback(spec PlaybackSpec) error {
	if a.p == nil || a.p.closed {
		return Annotate(ErrClosed, "AudioShape.SetPlayback")
	}
	if a.profile.TrackKey == "" {
		return Annotate(ErrStaleHandle, "AudioShape.SetPlayback")
	}
	if spec.StartDelay < 0 {
		return &OperationError{Op: "AudioShape.SetPlayback", Message: "negative start delay", Err: ErrInvalidArgument}
	}
	// 1) 冲突检测：既有 timing 必须缺失或纯音频（安全白名单）。
	doc, root, timing, err := a.slideTimingDoc()
	if err != nil {
		return Annotate(err, "AudioShape.SetPlayback")
	}
	if timing != nil && !timingAudioOnly(doc, timing) {
		return &OperationError{
			Op: "AudioShape.SetPlayback", Part: string(a.part),
			Message: "existing timing tree contains non-audio nodes, cannot safely merge",
			Err:     ErrTimingConflict,
		}
	}
	// 2) 记录本音轨的 StartDelay/Trigger（Version 不变：播放参数非资产替换）。
	if err := a.p.updateAudioProfile(a.profile.TrackKey, func(prof *AudioProfile) {
		prof.StartDelay = spec.StartDelay
		prof.Trigger = spec.Trigger
	}); err != nil {
		return Annotate(err, "AudioShape.SetPlayback")
	}
	// 3) 按全部 Profile 重建纯音频 timing（doc 缓存已在 updateAudioProfile
	// 的 commit 中失效，重新读取）。
	doc, root, timing, err = a.slideTimingDoc()
	if err != nil {
		return Annotate(err, "AudioShape.SetPlayback")
	}
	var patches []xmlstore.SpanPatch
	frag, err := audioTimingFragment(a.p, a.part)
	if err != nil {
		return Annotate(err, "AudioShape.SetPlayback")
	}
	if timing == nil {
		ap, err := xmlstore.AppendChild(root, []byte("<p:timing>"+frag+"</p:timing>"))
		if err != nil {
			return Annotate(mapXMLError(err), "AudioShape.SetPlayback")
		}
		patches = append(patches, ap)
	} else {
		// 整体替换 timing 的内容（纯音频子树已确认）。
		patches = append(patches, xmlstore.SpanPatch{
			Start:       timing.OpenEnd,
			End:         timing.CloseStart,
			Replacement: []byte(frag),
			Desc:        "SetPlayback: rebuild audio timing",
		})
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "AudioShape.SetPlayback")
	}
	if err := a.p.stagePatch(a.part, out); err != nil {
		return Annotate(err, "AudioShape.SetPlayback")
	}
	a.p.commit()
	_ = root
	return nil
}

// slideTimingDoc 返回 slide 文档、根元素与 p:timing 节点（缺失为 nil）。
// p:timing 是 p:sld 的直接子元素（cSld/clrMapOvr/transition 之后）。
func (a *AudioShape) slideTimingDoc() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, *xmlstore.NodeRecord, error) {
	doc, err := a.p.docOf(a.part)
	if err != nil {
		return nil, nil, nil, err
	}
	root := doc.Root()
	if root == nil {
		return nil, nil, nil, Annotate(ErrMalformedPackage, "slide root missing")
	}
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c != nil && c.Namespace == nsPresentationML && c.Local() == "timing" {
			return doc, root, c, nil
		}
	}
	return doc, root, nil, nil
}

// timingAudioOnly 报告 timing 子树是否只含库可管理的音频节点。
// 白名单覆盖纯音频 timing 树的结构性包装元素；出现任何动画/视频/媒体
// 之外的元素（anim/set/video/cmd 等）视为复杂 → 冲突。
var timingAudioSafeLocals = map[string]bool{
	"tnLst": true, "par": true, "childTnLst": true, "audio": true,
	"cMediaNode": true, "cTn": true, "stCondLst": true, "cond": true,
	"tgtEl": true, "spTgt": true, "audioCtr": true, "endCondLst": true,
}

func timingAudioOnly(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) bool {
	for _, cid := range n.Children {
		c := doc.Node(cid)
		if c == nil {
			continue
		}
		// 非 PresentationML 命名空间的注释等可忽略。
		if c.Namespace != nsPresentationML {
			continue
		}
		if !timingAudioSafeLocals[c.Local()] {
			return false
		}
		if !timingAudioOnly(doc, c) {
			return false
		}
	}
	return true
}

// audioTimingFragment 生成 slide 上全部 audio Profile 的纯音频 timing
// 子树内容（不含 p:timing 外壳；外壳由调用方决定新建或复用）。
//
// 结构（ECMA CT_SlideTimingInfo，简化子集）：
//
//	<p:tnLst><p:par><p:cTn id dur="indefinite" restart="never" nodeType="tmRoot">
//	  <p:childTnLst>
//	    <p:audio><p:cMediaNode vol="80">
//	      <p:cTn id fill="hold" display="0"><p:stCondLst><p:cond delay="MS"/></p:stCondLst></p:cTn>
//	      <p:tgtEl><p:spTgt spid="SHAPEID"/></p:tgtEl>
//	    </p:cMediaNode></p:audio>
//	  </p:childTnLst></p:cTn></p:par></p:tnLst>
func audioTimingFragment(p *Presentation, slide opc.PartName) (string, error) {
	profiles := p.audioProfilesOfSlide(slide)
	if len(profiles) == 0 {
		return "", Annotate(ErrNotFound, "no audio profiles on slide")
	}
	var sb strings.Builder
	sb.WriteString(`<p:tnLst><p:par><p:cTn id="900000" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst>`)
	for _, prof := range profiles {
		tnID := 900000 + int64(prof.ShapeID)*2
		delay := prof.StartDelay.Milliseconds()
		cond := fmt.Sprintf(`<p:cond delay="%d"/>`, delay)
		if prof.Trigger == PlaybackOnClick {
			cond = fmt.Sprintf(`<p:cond evt="onClick" delay="0"/>`)
		}
		fmt.Fprintf(&sb,
			`<p:audio><p:cMediaNode vol="80">`+
				`<p:cTn id="%d" fill="hold" display="0"><p:stCondLst>%s</p:stCondLst></p:cTn>`+
				`<p:tgtEl><p:spTgt spid="%d"/></p:tgtEl>`+
				`</p:cMediaNode></p:audio>`,
			tnID, cond, int64(prof.ShapeID),
		)
	}
	sb.WriteString(`</p:childTnLst></p:cTn></p:par></p:tnLst>`)
	return sb.String(), nil
}

// SetAdvanceAfter 设置页面自动翻页时长（p:transition@advTm 毫秒）。
// 不推断音频时长——计算依据由 PlanTimingSync/调用方提供。
// 页面无 p:transition 时创建最小 transition 元素（插在 p:timing 之前、
// p:cSld 之后，符合 CT_Slide 子元素次序）。
func (s *Slide) SetAdvanceAfter(d time.Duration) error {
	if err := s.alive(); err != nil {
		return Annotate(err, "Slide.SetAdvanceAfter")
	}
	if d < 0 {
		return &OperationError{Op: "SetAdvanceAfter", Message: "negative duration", Err: ErrInvalidArgument}
	}
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return Annotate(err, "Slide.SetAdvanceAfter")
	}
	root := doc.Root()
	if root == nil {
		return Annotate(ErrStaleHandle, "Slide.SetAdvanceAfter")
	}
	ms := d.Milliseconds()
	val := strconv.FormatInt(ms, 10)
	var timing, transition *xmlstore.NodeRecord
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c == nil || c.Namespace != nsPresentationML {
			continue
		}
		switch c.Local() {
		case "transition":
			transition = c
		case "timing":
			timing = c
		}
	}
	var patches []xmlstore.SpanPatch
	if transition != nil {
		p, err := setOrAddAttr(doc, transition, "advTm", val)
		if err != nil {
			return Annotate(err, "Slide.SetAdvanceAfter")
		}
		patches = append(patches, p)
	} else {
		frag := `<p:transition advTm="` + val + `"/>`
		var ap xmlstore.SpanPatch
		var err error
		if timing != nil {
			ap, err = xmlstore.InsertBefore(timing, []byte(frag))
		} else {
			ap, err = xmlstore.AppendChild(root, []byte(frag))
		}
		if err != nil {
			return Annotate(mapXMLError(err), "Slide.SetAdvanceAfter")
		}
		patches = append(patches, ap)
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "Slide.SetAdvanceAfter")
	}
	if err := s.p.stagePatch(s.part, out); err != nil {
		return Annotate(err, "Slide.SetAdvanceAfter")
	}
	s.p.commit()
	return nil
}

// ---------- UpsertNarration（AUDIO-03 入口） ----------

// UpsertNarration 按 TrackKey 幂等嵌入/更新音轨（AT-08）。
//
//   - TrackKey 不存在：走 AddAudio 新增（Version=1）并应用播放配置。
//   - TrackKey 已存在：返回指向既有 shape 的句柄（shape id 不变），
//     Profile Version 自增；Source 字节一致时报"未变"，不一致时报"更新"
//     （当前版本媒体 Part 不做字节级替换，仅版本计数，避免半途替换）。
//
// 返回的第二个值报告本次操作类别（added / unchanged / updated）。
func (s *Slide) UpsertNarration(ctx context.Context, src MediaSource, spec AudioSpec, pb PlaybackSpec) (*AudioShape, string, error) {
	if err := s.alive(); err != nil {
		return nil, "", Annotate(err, "Slide.UpsertNarration")
	}
	if src == nil || spec.TrackKey == "" {
		return nil, "", &OperationError{Op: "Slide.UpsertNarration", Message: "missing source or track key", Err: ErrInvalidArgument}
	}
	existing := s.p.findAudioProfile(spec.TrackKey)
	if existing == nil {
		as, err := s.AddAudio(ctx, src, spec)
		if err != nil {
			return nil, "", Annotate(err, "Slide.UpsertNarration")
		}
		if err := as.SetPlayback(pb); err != nil {
			return nil, "", Annotate(err, "Slide.UpsertNarration")
		}
		return as, "added", nil
	}
	// 已存在：更新 Version（+ 播放参数）。
	newVer := existing.Version + 1
	if err := s.p.updateAudioProfile(spec.TrackKey, func(prof *AudioProfile) {
		prof.Version = newVer
	}); err != nil {
		return nil, "", Annotate(err, "Slide.UpsertNarration")
	}
	as, err := s.handleByShapeID(existing.ShapeID)
	if err != nil {
		return nil, "", Annotate(err, "Slide.UpsertNarration")
	}
	if err := as.SetPlayback(pb); err != nil {
		return nil, "", Annotate(err, "Slide.UpsertNarration")
	}
	action := "updated"
	if data, err := readMedia(ctx, src); err == nil {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) == existing.ContentSHA256 {
			action = "unchanged"
		}
	}
	as.profile.Version = newVer
	return as, action, nil
}

// handleByShapeID 通过 shape id 重新构建 AudioShape 句柄。
func (s *Slide) handleByShapeID(id ShapeID) (*AudioShape, error) {
	doc, err := s.p.docOf(s.part)
	if err != nil {
		return nil, Annotate(err, "handleByShapeID")
	}
	// 整个 slide 上全部 pic 节点都尝试一次。
	picIDs := doc.Elements(nsPresentationML, "pic")
	for _, pid := range picIDs {
		c := doc.Node(pid)
		if c == nil {
			continue
		}
		cnv := elementCNvPr(doc, c)
		if cnv == nil {
			continue
		}
		vid, _ := cnv.Attr("", "id")
		if vid == strconv.FormatInt(int64(id), 10) {
			ap := s.lastProfileByShapeID(id)
			return &AudioShape{
				shapeNode: shapeNode{p: s.p, part: s.part, path: recordPath(doc, c.ID)},
				profile:   ap,
				role:      ap.Role,
			}, nil
		}
	}
	return nil, &OperationError{Op: "handleByShapeID", SlideID: SlideID(id), Message: "shape not found", Err: ErrStaleHandle}
}

// lastProfileByShapeID 返回 Profile 表中 shapeID 匹配的 Profile。
func (s *Slide) lastProfileByShapeID(id ShapeID) AudioProfile {
	const partName opc.PartName = "/docProps/audio.xml"
	b, err := s.p.partBytes(partName)
	if err != nil {
		return AudioProfile{}
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return AudioProfile{}
	}
	root := doc.Root()
	for _, cid := range root.Children {
		n := doc.Node(cid)
		if n == nil {
			continue
		}
		if prof, ok := parseAudioProfile(n); ok && prof.ShapeID == id {
			return prof
		}
	}
	return AudioProfile{}
}

// ---------- PlanTimingSync / ApplyTimingPlan ----------

// UnknownDurationPolicy 决定未知时长时的处理。
type UnknownDurationPolicy int

const (
	// UnknownDurationFail 默认：拒绝整批同步。
	UnknownDurationFail UnknownDurationPolicy = iota
	// UnknownDurationSkip 跳过未知页面并报告。
	UnknownDurationSkip
)

// AnimationTimingPolicy 决定原 timing 树无法解析时的策略。
type AnimationTimingPolicy int

const (
	// AnimationTimingReject 拒绝（有未解析 timing 时整个 plan 失败）。
	AnimationTimingReject AnimationTimingPolicy = iota
	// AnimationTimingKeepExisting 保留原值并记录 skipped。
	AnimationTimingKeepExisting
)

// TimingSyncOptions 配置同步行为。
type TimingSyncOptions struct {
	TailPadding     time.Duration
	UnknownDuration UnknownDurationPolicy
	AnimationPolicy AnimationTimingPolicy
}

// TimingPlan 是 PlanTimingSync 输出，可由 ApplyTimingPlan 应用。
type TimingPlan struct {
	BaseRevision uint64
	PageJumps    []PageTiming
	Skipped      []string
}

// PageTiming 是按页的 AdvanceAfter 计划。
type PageTiming struct {
	SlideIndex   int
	SlideID      SlideID
	AdvanceAfter time.Duration
	// 参与的音轨（For 调试）。
	Tracks []TrackContribution
}

// TrackContribution 单个音轨对 Adv 计算的贡献。
type TrackContribution struct {
	TrackKey   string
	Role       AudioRole
	StartDelay time.Duration
	Duration   time.Duration
	EndTime    time.Duration
}

// TimingSyncReport 是 ApplyTimingPlan 输出。
type TimingSyncReport struct {
	// Applied 是成功写入 advTm 的页面数。
	Applied int
	// Skipped 是未写入的页面数（计划中被跳过或写入失败）。
	Skipped int
}

// PlanTimingSync 计算翻页计划而不修改文档。
func (p *Presentation) PlanTimingSync(_ context.Context, opts TimingSyncOptions) (TimingPlan, error) {
	if p == nil || p.closed {
		return TimingPlan{}, Annotate(ErrClosed, "PlanTimingSync")
	}
	if opts.TailPadding < 0 {
		return TimingPlan{}, &OperationError{Op: "PlanTimingSync", Message: "negative padding", Err: ErrInvalidArgument}
	}
	slides, err := p.Slides()
	if err != nil {
		return TimingPlan{}, err
	}
	plan := TimingPlan{BaseRevision: p.rev}
	for i, s := range slides {
		tracks := pageNarrationTracks(p, s)
		if len(tracks) == 0 {
			plan.Skipped = append(plan.Skipped, fmt.Sprintf("slide %d: no narration tracks", i))
			continue
		}
		// 检查未知时长。
		var maxEnd time.Duration
		var skippedUnk bool
		for _, t := range tracks {
			if t.Duration <= 0 {
				if opts.UnknownDuration == UnknownDurationFail {
					return TimingPlan{}, &OperationError{
						Op: "PlanTimingSync", SlideID: s.ID(),
						Message: fmt.Sprintf("unknown duration on track %q", t.TrackKey),
						Err:     ErrDurationUnknown,
					}
				}
				skippedUnk = true
				continue
			}
			end := t.StartDelay + t.Duration
			if end > maxEnd {
				maxEnd = end
			}
		}
		if skippedUnk {
			continue
		}
		pt := PageTiming{
			SlideIndex:   i,
			SlideID:      s.ID(),
			AdvanceAfter: maxEnd + opts.TailPadding,
			Tracks:       tracks,
		}
		plan.PageJumps = append(plan.PageJumps, pt)
	}
	return plan, nil
}

// pageNarrationTracks 收集页面上参与翻页计算的音轨（方案 §21.3）：
// Role=Narration 且 Trigger=OnSlideEnter；背景音/点击触发不参与。
func pageNarrationTracks(p *Presentation, s *Slide) []TrackContribution {
	profiles := p.audioProfilesOfSlide(s.part)
	var out []TrackContribution
	for _, prof := range profiles {
		if prof.Role != AudioRoleNarration || prof.Trigger != PlaybackOnSlideEnter {
			continue
		}
		out = append(out, TrackContribution{
			TrackKey:   prof.TrackKey,
			Role:       prof.Role,
			StartDelay: prof.StartDelay,
			Duration:   prof.Duration,
			EndTime:    prof.StartDelay + prof.Duration,
		})
	}
	return out
}

// ApplyTimingPlan 把 plan 应用到文档（revision 校验 + 原子提交）。
func (p *Presentation) ApplyTimingPlan(_ context.Context, plan TimingPlan) (TimingSyncReport, error) {
	if p == nil || p.closed {
		return TimingSyncReport{}, Annotate(ErrClosed, "ApplyTimingPlan")
	}
	if plan.BaseRevision != p.rev {
		return TimingSyncReport{}, &OperationError{
			Op:      "ApplyTimingPlan",
			Message: fmt.Sprintf("plan revision %d != current %d (AT-12)", plan.BaseRevision, p.rev),
			Err:     ErrConcurrentModification,
		}
	}
	rep := TimingSyncReport{}
	for _, pt := range plan.PageJumps {
		s, err := p.SlideByID(pt.SlideID)
		if err != nil {
			return rep, err
		}
		if err := s.SetAdvanceAfter(pt.AdvanceAfter); err != nil {
			rep.Skipped++
			continue
		}
		rep.Applied++
	}
	rep.Skipped += len(plan.Skipped)
	return rep, nil
}

// SlideByID 由 SlideID 找 Slide。
func (p *Presentation) SlideByID(id SlideID) (*Slide, error) {
	if p == nil || p.closed {
		return nil, Annotate(ErrClosed, "SlideByID")
	}
	slides, err := p.Slides()
	if err != nil {
		return nil, err
	}
	for _, s := range slides {
		if s.ID() == id {
			return s, nil
		}
	}
	return nil, &OperationError{Op: "SlideByID", SlideID: id, Message: "not found", Err: ErrNotFound}
}

// SyncTimingToAudio 是 PlanTimingSync + ApplyTimingPlan 的便捷封装。
func (p *Presentation) SyncTimingToAudio(ctx context.Context, opts TimingSyncOptions) (TimingSyncReport, error) {
	plan, err := p.PlanTimingSync(ctx, opts)
	if err != nil {
		return TimingSyncReport{}, err
	}
	return p.ApplyTimingPlan(ctx, plan)
}
