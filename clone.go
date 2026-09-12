package pptx

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/editplan"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 CLONE-01（方案 §11）：同文档内受限页面复制。
//
// 复制页面不是复制一个 slide XML 文件：必须遍历依赖闭包（visited 防循环）、
// 建立 Part 映射、重写已知引用、验证备注与图表依赖，一次事务提交。对
// 无法识别的关系类型整体拒绝（ErrUnsupportedEdit，AT-13 同文档口径），
// 绝不产出引用悬空的残缺目标页。
//
// 引用重写策略（同文档受限范围的化简）：
//   - 关系 ID：目标页/目标 chart/notes 的关系流是全新 Part，逐条保留源
//     rId 与 TargetMode/Type 原文，仅重写被克隆 Part 的 Target——rId 与
//     页内 r:embed/r:link/r:id 引用恒等成立，无需改写 slide XML；
//   - ShapeID 与 timing 节点 ID：均为 slide Part 作用域，整页字节级复制
//     后内部一致性保持（p:spTgt@spid 等引用同步成立），跨页无引用；
//   - notesSlide 对 slide 的回引经 Part 映射重写到目标页。
//
// 复用/独立策略（ClonePolicy）：
//   - 版式与母版：复用（同文档，无复制必要）；
//   - 图表与嵌入工作簿：总是独立复制（数据隔离硬性要求，M5 退出标准）；
//   - 媒体（图片/音频/视频）：默认共享，策略可改为独立复制；
//   - 外部关系（超链接等）：逐字节保留，不解析不下载。

// ---------- 策略 ----------

// ClonePolicy 是同文档页面复制策略（方案 §11）。
//
// 零值即默认策略：媒体共享、图表与嵌入工作簿独立复制、版式与母版复用。
type ClonePolicy struct {
	// IndependentMedia 为 true 时媒体 Part（图片/音频/视频）独立复制，
	// 目标页与源页媒体数据隔离；默认 false（共享同一 Part，引用同一份
	// 字节）。图表及其嵌入工作簿不受本开关影响，总是独立复制。
	IndependentMedia bool
}

// ---------- 公共 API ----------

// Clone 在同一文档内复制页面 s：源页全部内容（形状、文本、表格、图表、
// 音频、备注、计时树、切换）复制为追加到 sldIdLst 末尾的新页面，返回
// 新页面句柄。policy 为 nil 时使用默认策略（媒体共享）。
//
// 受限范围（超出整体拒绝 ErrUnsupportedEdit，零残留）：
//   - 源页关系流中的未知内部关系类型（OLE、SmartArt、外部生成图表的
//     颜色/样式 Part 等）——无法安全重映射的引用宁可拒绝（AT-13）；
//   - notesSlide 回引非源页、复用目标（版式/母版）缺失、嵌入工作簿
//     携带自身关系流等结构异常。
//
// 复制后保证：新页面 SlideID 唯一；图表数据与源隔离（改一方不影响
// 另一方）；库创建音轨的 AudioProfile 以派生 TrackKey 记录到新页面。
func (s *Slide) Clone(policy *ClonePolicy) (*Slide, error) {
	const op = "Slide.Clone"
	if err := s.alive(); err != nil {
		return nil, Annotate(err, op)
	}
	pol := ClonePolicy{}
	if policy != nil {
		pol = *policy
	}
	plan, err := s.p.buildClonePlan(s.p, s, pol)
	if err != nil {
		return nil, Annotate(err, op)
	}
	if err := s.p.applyClonePlan(plan); err != nil {
		return nil, Annotate(err, op)
	}
	return &Slide{p: s.p, id: plan.newID, part: plan.dst, rev: s.p.rev}, nil
}

// CopySlideFrom 把属于另一文档的页面 src 复制到当前文档（CLONE-02，
// 跨文档受限复制）。源页全部内容（形状、文本、表格、图表、音频、备注、
// 计时树、切换）复制为追加到当前文档 sldIdLst 末尾的新页面，返回新页
// 面句柄。policy 为 nil 时使用默认策略。
//
// 与同文档 Clone 的语义差异：
//   - 媒体（图片/音频/视频）总是独立复制（跨包无法共享源字节），
//     policy.IndependentMedia 字段被忽略；
//   - 版式/母版按**内容字节完全一致**匹配复用：目标文档已有相同
//     slideLayout / notesMaster 则复用，否则整体拒绝 ErrUnsupportedEdit
//     （AT-13 口径：不克隆版式/母版链，不生成残缺目标页）。
//
// 受限范围（超出整体拒绝 ErrUnsupportedEdit，零残留）：源页关系流中的
// 未知内部关系类型（OLE、SmartArt 等）、版式/母版内容不匹配、notesSlide
// 回引非源页、嵌入工作簿携带自身关系流等结构异常。
func (dst *Presentation) CopySlideFrom(src *Slide, policy *ClonePolicy) (*Slide, error) {
	const op = "Presentation.CopySlideFrom"
	if dst == nil {
		return nil, &OperationError{Op: op, Message: "nil destination presentation", Err: ErrInvalidArgument}
	}
	if src == nil {
		return nil, &OperationError{Op: op, Message: "nil source slide", Err: ErrInvalidArgument}
	}
	if err := src.alive(); err != nil {
		return nil, Annotate(err, op)
	}
	if dst.closed {
		return nil, Annotate(ErrClosed, op)
	}
	if src.p == dst {
		// 同文档：直接走 Clone，语义更完整（媒体可共享）。
		return src.Clone(policy)
	}
	pol := ClonePolicy{}
	if policy != nil {
		pol = *policy
	}
	plan, err := dst.buildClonePlan(src.p, src, pol)
	if err != nil {
		return nil, Annotate(err, op)
	}
	if err := dst.applyClonePlan(plan); err != nil {
		return nil, Annotate(err, op)
	}
	return &Slide{p: dst, id: plan.newID, part: plan.dst, rev: dst.rev}, nil
}

// ---------- 计划（纯读取，无任何暂存副作用） ----------

// clonePartPlan 是一个待克隆附属 Part（notes/chart/workbook/media）。
type clonePartPlan struct {
	src, dst opc.PartName
	ct       string
	kind     string // "notes" | "chart" | "workbook" | "media"
}

// cloneRelsPlan 记录克隆容器（slide/notes/chart）的关系流处理结果：
// exists=false 表示源无关系流（目标同样不创建）。
type cloneRelsPlan struct {
	exists bool
	bytes  []byte // 需要时已重写 Target 的完整关系流字节
}

// clonePlan 是一次页面复制的完整计划（验证与暂存两阶段共享）。
//
// srcDoc 是源文档（读视图），plan 的构造 receiver 是目标文档（写视图）；
// 同文档复制时二者为同一 *Presentation。crossDoc 由 srcDoc != 目标文档
// 判定，驱动跨文档专有语义（版式/母版内容匹配、媒体强制独立）。
type clonePlan struct {
	src, dst opc.PartName
	newID    SlideID
	mainRID  string

	srcDoc   *Presentation
	crossDoc bool

	independentMedia bool

	// cloned 按发现序记录全部附属克隆 Part；remap 是源→目标 Part 映射
	//（含 src slide 自身，供 notes 回引重写）。
	cloned []clonePartPlan
	remap  map[opc.PartName]opc.PartName

	// rels 是每个克隆容器的关系流计划；allocSeq/allocUsed 是本事务内
	// 的名字分配去重状态。
	rels      map[opc.PartName]*cloneRelsPlan
	allocSeq  map[string]int
	allocUsed map[opc.PartName]bool

	profileAdds   []AudioProfile
	profileTaken  map[string]bool
	profileAddsV  []VideoProfile
	profileTakenV map[string]bool
}

// buildClonePlan 遍历依赖闭包并构造复制计划。本阶段只读：任何拒绝都
// 发生在暂存之前，保证零残留。
func (dst *Presentation) buildClonePlan(srcDoc *Presentation, s *Slide, policy ClonePolicy) (*clonePlan, error) {
	const op = "clone.plan"
	doc, err := dst.presentationDoc()
	if err != nil {
		return nil, err
	}
	used, err := usedSlideIDs(doc)
	if err != nil {
		return nil, err
	}
	newID, err := allocSlideID(used)
	if err != nil {
		return nil, err
	}
	mainRels, err := relsXML(dst, dst.main)
	if err != nil {
		return nil, err
	}

	plan := &clonePlan{
		src: s.part, dst: "", newID: newID, mainRID: nextRID(mainRels),
		srcDoc:           srcDoc,
		crossDoc:         srcDoc != dst,
		independentMedia: policy.IndependentMedia,
		remap:            map[opc.PartName]opc.PartName{},
		rels:             map[opc.PartName]*cloneRelsPlan{},
		allocSeq:         map[string]int{},
		allocUsed:        map[opc.PartName]bool{},
		profileTaken:     dst.allAudioTrackKeys(),
		profileTakenV:    dst.allVideoTrackKeys(),
	}
	// 源页 Part 必须在源文档读视图存在。
	if !srcDoc.hasPartCurrent(plan.src) {
		return nil, &OperationError{Op: op, Part: string(plan.src),
			Message: "source slide part does not exist", Err: ErrNotFound}
	}
	// 克隆源页容器（分配目标名 + 遍历其关系闭包）。
	if err := dst.cloneWalkContainer(srcDoc, plan.src, "slide", plan); err != nil {
		return nil, err
	}
	// 库创建音轨的 Profile 克隆（派生 TrackKey、指向目标页）。
	for _, prof := range srcDoc.audioProfilesOfSlide(plan.src) {
		np := prof
		np.TrackKey = uniqueTrackKey(prof.TrackKey+"-c", plan.profileTaken)
		np.SlidePart = plan.dst
		if dstM, hit := plan.remap[prof.MediaPart]; hit {
			np.MediaPart = dstM
		}
		plan.profileAdds = append(plan.profileAdds, np)
	}
	// 库创建视频的 Profile 克隆（派生 TrackKey、指向目标页；独立媒体时同步重映射）。
	for _, vp := range srcDoc.videoProfilesOfSlide(plan.src) {
		np := vp
		np.TrackKey = uniqueTrackKey(vp.TrackKey+"-c", plan.profileTakenV)
		np.SlidePart = plan.dst
		if dstM, hit := plan.remap[vp.MediaPart]; hit {
			np.MediaPart = dstM
		}
		if vp.PosterPart != "" {
			if dstM, hit := plan.remap[vp.PosterPart]; hit {
				np.PosterPart = dstM
			}
		}
		plan.profileAddsV = append(plan.profileAddsV, np)
	}
	return plan, nil
}

// cloneWalkContainer 克隆一个容器 Part（slide/notes/chart）：分配目标名、
// 分类其关系流、递归依赖闭包。remap 在入口登记，天然防循环（visited）。
//
// dst 是目标文档（receiver，负责名字分配与暂存），srcDoc 是源文档（负责
// 读取 Part 字节/关系/内容类型）。同文档复制时二者相同。
func (dst *Presentation) cloneWalkContainer(srcDoc *Presentation, src opc.PartName, kind string, plan *clonePlan) error {
	const op = "clone.walk"
	dstPart := dst.allocCloneName(src, plan)
	plan.remap[src] = dstPart
	plan.cloned = append(plan.cloned, clonePartPlan{src: src, dst: dstPart, ct: srcDoc.contentTypeOf(src), kind: kind})
	// 源页自身记录目标名（供 notes 回引与 Profile 重写）。
	if kind == "slide" {
		plan.dst = dstPart
	}

	rels, ok, err := srcDoc.relsOf(src)
	if err != nil {
		return &OperationError{Op: op, Part: string(src),
			Message: "cannot read relationships", Err: err}
	}
	if !ok {
		plan.rels[src] = &cloneRelsPlan{exists: false}
		return nil
	}
	for _, rel := range rels {
		if rel.Mode == opc.TargetExternal {
			continue // 外部关系逐字节保留
		}
		target := rel.TargetPart
		if !srcDoc.hasPartCurrent(target) {
			return &OperationError{Op: op, Part: string(src),
				Message: fmt.Sprintf("relationship %q target %s does not exist", rel.ID, target),
				Err:     ErrNotFound}
		}
		switch classifyCloneRel(rel.Type, kind) {
		case "reuse":
			// 版式/母版：同文档直接复用（不进 remap）；跨文档按内容
			// 字节匹配复用，匹配不到整体拒绝（不克隆版式/母版链）。
			if plan.crossDoc {
				match, found, err := dst.matchReusePart(srcDoc, target, rel.Type)
				if err != nil {
					return err
				}
				if !found {
					return &OperationError{Op: op, Part: string(src),
						Message: fmt.Sprintf("reuse relationship %q target %s has no byte-identical counterpart in destination document",
							rel.Type, target),
						Err: ErrUnsupportedEdit}
				}
				plan.remap[target] = match
			}
		case "backref":
			// notesSlide 对 slide 的回引：必须指向克隆源页，经 remap 重写。
			if target != plan.src {
				return &OperationError{Op: op, Part: string(src),
					Message: fmt.Sprintf("notesSlide back-references slide %s, not the cloned slide %s", target, plan.src),
					Err:     ErrUnsupportedEdit}
			}
		case "media":
			if plan.independentMedia || plan.crossDoc {
				// 独立复制（跨文档强制：跨包无法共享源字节）。
				if _, done := plan.remap[target]; !done {
					dstM := dst.allocCloneName(target, plan)
					plan.remap[target] = dstM
					plan.cloned = append(plan.cloned, clonePartPlan{
						src: target, dst: dstM, ct: srcDoc.contentTypeOf(target), kind: "media"})
				}
			}
		case "notes", "chart":
			if _, done := plan.remap[target]; !done {
				if err := dst.cloneWalkContainer(srcDoc, target, classifyCloneRel(rel.Type, kind), plan); err != nil {
					return err
				}
			}
		case "workbook":
			// chart 的嵌入工作簿：独立复制；不得携带自身关系流
			//（无法安全重映射时宁可拒绝）。
			if wbRels, ok2, _ := srcDoc.relsOf(target); ok2 && len(wbRels) > 0 {
				return &OperationError{Op: op, Part: string(target),
					Message: "embedded workbook carries its own relationships; cannot remap safely",
					Err:     ErrUnsupportedEdit}
			}
			if _, done := plan.remap[target]; !done {
				dstW := dst.allocCloneName(target, plan)
				plan.remap[target] = dstW
				plan.cloned = append(plan.cloned, clonePartPlan{
					src: target, dst: dstW, ct: srcDoc.contentTypeOf(target), kind: "workbook"})
			}
		default: // "unknown"
			return &OperationError{Op: op, Part: string(src),
				Message: fmt.Sprintf("unsupported internal relationship type %q (target %s); refusing to clone",
					rel.Type, target),
				Err: ErrUnsupportedEdit}
		}
	}
	// 生成重写 Target 后的关系流字节（无命中则逐字节保留原文）。
	relsBytes, err := dst.patchedCloneRels(srcDoc, src, plan.remap)
	if err != nil {
		return err
	}
	plan.rels[src] = &cloneRelsPlan{exists: true, bytes: relsBytes}
	return nil
}

// matchReusePart 跨文档复用匹配：在目标文档中查找与源 Part 内容字节完全
// 一致的同类型 Part（slideLayout / notesMaster）。命中返回目标 PartName 与
// true；未命中返回 false（调用方拒绝 ErrUnsupportedEdit）。
func (dst *Presentation) matchReusePart(srcDoc *Presentation, srcPart opc.PartName, relType string) (opc.PartName, bool, error) {
	const op = "clone.match_reuse"
	srcBytes, err := srcDoc.partBytes(srcPart)
	if err != nil {
		return "", false, &OperationError{Op: op, Part: string(srcPart),
			Message: "cannot read reuse part", Err: err}
	}
	var candidates []opc.PartName
	switch relType {
	case opc.RelSlideLayout:
		for _, master := range dst.pk.RelatedParts(dst.main, opc.RelSlideMaster) {
			candidates = append(candidates, dst.pk.RelatedParts(master, opc.RelSlideLayout)...)
		}
	case relNotesMaster:
		candidates = dst.pk.RelatedParts(dst.main, relNotesMaster)
	default:
		return "", false, nil
	}
	for _, cand := range candidates {
		if !dst.hasPartCurrent(cand) {
			continue
		}
		b, err := dst.partBytes(cand)
		if err != nil {
			return "", false, &OperationError{Op: op, Part: string(cand),
				Message: "cannot read candidate reuse part", Err: err}
		}
		if bytes.Equal(b, srcBytes) {
			return cand, true, nil
		}
	}
	return "", false, nil
}

// classifyCloneRel 按容器种类分类关系类型。返回值：
// reuse/backref/media/notes/chart/workbook/unknown。
func classifyCloneRel(relType, container string) string {
	switch relType {
	case opc.RelSlideLayout, relNotesMaster:
		return "reuse"
	case relImage, relAudio, relVideo, relMedia:
		return "media"
	case opc.RelNotesSlide:
		if container == "slide" {
			return "notes"
		}
		return "unknown"
	case relChart:
		if container == "slide" {
			return "chart"
		}
		return "unknown"
	case relPackage:
		if container == "chart" {
			return "workbook"
		}
		return "unknown"
	case opc.RelSlide:
		if container == "notes" {
			return "backref"
		}
		return "unknown"
	}
	return "unknown"
}

// 媒体关系类型常量（relImage 见 media.go、relNotesMaster 见 notes.go）。
const (
	relAudio = opc.RelTypePrefix + "audio"
	relVideo = opc.RelTypePrefix + "video"
	relMedia = opc.RelTypePrefix + "media"
)

// patchedCloneRels 返回容器 src 的关系流字节：命中 remap 的内部关系
// 仅重写 Target 属性（Id/Type/TargetMode 与转义原文保留），其余逐字节
// 保留。无命中返回原文。
func (dst *Presentation) patchedCloneRels(srcDoc *Presentation, src opc.PartName, remap map[opc.PartName]opc.PartName) ([]byte, error) {
	const op = "clone.rels"
	raw, err := relsXML(srcDoc, src)
	if err != nil {
		return nil, err
	}
	rels, ok, err := srcDoc.relsOf(src)
	if err != nil || !ok {
		return nil, &OperationError{Op: op, Part: string(src),
			Message: "relationships disappeared during planning", Err: ErrNotFound}
	}
	byID := make(map[string]*opc.Relationship, len(rels))
	needPatch := false
	for _, rel := range rels {
		byID[rel.ID] = rel
		if rel.Mode == opc.TargetInternal {
			if _, hit := remap[rel.TargetPart]; hit {
				needPatch = true
			}
		}
	}
	if !needPatch {
		return append([]byte(nil), raw...), nil
	}
	doc, err := xmlstore.Index(raw)
	if err != nil {
		return nil, &OperationError{Op: op, Part: string(relsPart(src)),
			Message: "relationships stream is not well-formed XML", Err: mapXMLError(err)}
	}
	root := doc.Root()
	var patches []xmlstore.SpanPatch
	for _, cid := range root.Children {
		n := doc.Node(cid)
		if n == nil || n.Namespace != nsPkgRels || n.Local() != "Relationship" {
			continue
		}
		id, _ := n.AttrLocal("Id")
		rel, ok := byID[id]
		if !ok || rel.Mode != opc.TargetInternal {
			continue
		}
		dst, hit := remap[rel.TargetPart]
		if !hit {
			continue
		}
		patch, err := xmlstore.SetAttrValuePatch(doc, n, "", "Target", retargetRel(rel.Target, dst))
		if err != nil {
			return nil, &OperationError{Op: op, Part: string(relsPart(src)),
				Message: "cannot rewrite relationship target", Err: mapXMLError(err)}
		}
		patches = append(patches, patch)
	}
	out, err := xmlstore.ApplyPatches(raw, patches)
	if err != nil {
		return nil, &OperationError{Op: op, Part: string(relsPart(src)),
			Message: "cannot apply relationship patches", Err: mapXMLError(err)}
	}
	return out, nil
}

// retargetRel 由旧 Target（相对或绝对）推导指向 dst 的新 Target。
// 克隆 Part 总是与源同目录，目录前缀保持不变。
func retargetRel(oldTarget string, dst opc.PartName) string {
	if strings.HasPrefix(oldTarget, "/") {
		return string(dst)
	}
	if i := strings.LastIndex(oldTarget, "/"); i >= 0 {
		return oldTarget[:i+1] + slideName(dst)
	}
	return slideName(dst)
}

// ---------- 名字分配 ----------

// allocCloneName 为克隆 Part 分配与源同目录的目标名：
//   - 惯用数字序名（chart1.xml、notesSlide2.xml、image3.png 等）→
//     同前缀 nextPartSeq 递增（含本事务已分配序号去重）；
//   - 非数字结尾名 → stem-N 后缀，直到空闲。
func (p *Presentation) allocCloneName(src opc.PartName, plan *clonePlan) opc.PartName {
	s := string(src)
	i := strings.LastIndex(s, "/")
	dir, base := "", s
	if i >= 0 {
		dir, base = s[:i+1], s[i+1:]
	}
	if stem, ext, _, ok := splitTrailingDigits(base); ok {
		prefix := dir + stem
		n := nextPartSeq(p, prefix, ext)
		if v := plan.allocSeq[prefix]; v > n {
			n = v
		}
		plan.allocSeq[prefix] = n + 1
		for {
			cand := opc.PartName(prefix + strconv.Itoa(n) + ext)
			if !p.hasPartCurrent(cand) && !plan.allocUsed[cand] {
				plan.allocUsed[cand] = true
				return cand
			}
			n++
			plan.allocSeq[prefix] = n + 1
		}
	}
	ext := ""
	stem := base
	if j := strings.LastIndex(base, "."); j >= 0 {
		ext, stem = base[j:], base[:j]
	}
	for n := 2; ; n++ {
		cand := opc.PartName(dir + stem + "-" + strconv.Itoa(n) + ext)
		if !p.hasPartCurrent(cand) && !plan.allocUsed[cand] {
			plan.allocUsed[cand] = true
			return cand
		}
	}
}

// splitTrailingDigits 拆出文件名结尾数字：chart1.xml → ("chart", ".xml", 1, true)。
func splitTrailingDigits(base string) (stem, ext string, digits int, ok bool) {
	j := strings.LastIndex(base, ".")
	if j <= 0 {
		return "", "", 0, false
	}
	ext = base[j:]
	head := base[:j]
	k := len(head)
	for k > 0 && head[k-1] >= '0' && head[k-1] <= '9' {
		k--
	}
	if k == len(head) || k == 0 {
		// 无数字 / 全数字（无 stem）都不按惯用名处理。
		return "", "", 0, false
	}
	n, err := strconv.Atoi(head[k:])
	if err != nil {
		return "", "", 0, false
	}
	return head[:k], ext, n, true
}

// ---------- 应用（单事务，失败恢复暂存区） ----------

// applyClonePlan 把计划落为一次提交。MultiPartPlan 负责暂存失败时恢复
// pending 快照，避免复杂克隆出现半暂存状态。
func (p *Presentation) applyClonePlan(plan *clonePlan) error {
	const op = "clone.apply"
	ops := make([]editplan.Operation, 0, 4+len(plan.cloned)*2)

	// 1) 目标页 Part（源字节级复制）与其关系流。
	// 跨文档复制时源 Part 只在 srcDoc 里可读，plan.src 由 srcDoc 持有。
	srcDoc := plan.srcDoc
	slideBytes, err := srcDoc.partBytes(plan.src)
	if err != nil {
		return &OperationError{Op: op, Part: string(plan.src), Message: "cannot read source slide", Err: err}
	}
	ops = append(ops, editplan.Add(plan.dst, slideBytes, ctSlide))
	if rp := plan.rels[plan.src]; rp != nil && rp.exists {
		ops = append(ops, editplan.Add(relsPart(plan.dst), rp.bytes, ""))
	}
	// 2) 附属 Part（notes/chart/workbook/media）与其关系流。
	for _, cp := range plan.cloned {
		b, err := srcDoc.partBytes(cp.src)
		if err != nil {
			return &OperationError{Op: op, Part: string(cp.src), Message: "cannot read cloned part", Err: err}
		}
		ops = append(ops, editplan.Add(cp.dst, b, fallbackCloneCT(cp)))
		if rp := plan.rels[cp.src]; rp != nil && rp.exists {
			ops = append(ops, editplan.Add(relsPart(cp.dst), rp.bytes, ""))
		}
	}
	// 3) presentation.xml 追加 p:sldId + 主关系新增 rId。
	presDoc, err := p.presentationDoc()
	if err != nil {
		return err
	}
	frag := `<p:sldId id="` + strconv.FormatUint(uint64(plan.newID), 10) + `" r:id="` + plan.mainRID + `"/>`
	patches, err := appendSldIdPatch(presDoc, frag)
	if err != nil {
		return err
	}
	out, err := xmlstore.ApplyPatches(presDoc.Original(), patches)
	if err != nil {
		return &OperationError{Op: op, Part: string(p.main),
			Message: "cannot append sldId", Err: mapXMLError(err)}
	}
	ops = append(ops, editplan.Patch(p.main, out))
	mainRels, err := relsXML(p, p.main)
	if err != nil {
		return err
	}
	entry := `<Relationship Id="` + plan.mainRID + `" Type="` + opc.RelSlide +
		`" Target="slides/` + slideName(plan.dst) + `"/>`
	ops = append(ops, relsPlanOp(p, p.main, insertRel(mainRels, entry)))
	// 4) 库创建音轨的 Profile 追加（同一事务）。
	if len(plan.profileAdds) > 0 {
		profileOp, err := p.appendAudioProfilesOperation(plan.profileAdds)
		if err != nil {
			return err
		}
		ops = append(ops, profileOp)
	}
	// 5) 库创建视频的 Profile 追加（同一事务）。
	if len(plan.profileAddsV) > 0 {
		profileOp, err := p.appendVideoProfilesOperation(plan.profileAddsV)
		if err != nil {
			return err
		}
		ops = append(ops, profileOp)
	}
	return applyMultiPartPlan(p, editplan.NewMultiPartPlan(ops...))
}

// fallbackCloneCT 给克隆 Part 的内容类型兜底（源读不到时按类型常量）。
func fallbackCloneCT(cp clonePartPlan) string {
	if cp.ct != "" {
		return cp.ct
	}
	switch cp.kind {
	case "notes":
		return ctNotesSlide
	case "chart":
		return ctChartPart
	case "workbook":
		return ctWorkbook
	}
	return ""
}

// contentTypeOf 返回 Part 当前读视图的内容类型（会话新增优先）。
func (p *Presentation) contentTypeOf(name opc.PartName) string {
	if a, ok := p.addedParts[name]; ok && a.Content != nil {
		return a.ContentType
	}
	if ct, ok := p.pk.ContentType(name); ok {
		return ct
	}
	return ""
}

func (p *Presentation) appendAudioProfilesOperation(adds []AudioProfile) (editplan.Operation, error) {
	const partName opc.PartName = "/docProps/audio.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return editplan.Operation{}, &OperationError{Op: "clone.profiles", Part: string(partName),
			Message: "audio profile part disappeared", Err: err}
	}
	idx := bytes.Index(b, []byte("</AudioProfiles>"))
	if idx < 0 {
		return editplan.Operation{}, &OperationError{Op: "clone.profiles", Part: string(partName),
			Message: "audio profile part is malformed (no closing root)", Err: ErrMalformedPackage}
	}
	var buf bytes.Buffer
	buf.Write(b[:idx])
	for _, prof := range adds {
		buf.WriteString("<Profile " + profileXMLAttrs(prof) + "/>")
	}
	buf.Write(b[idx:])
	return editplan.Patch(partName, buf.Bytes()), nil
}

// allAudioTrackKeys 返回文档内全部已用 TrackKey（克隆派生键去重用）。
func (p *Presentation) allAudioTrackKeys() map[string]bool {
	taken := map[string]bool{}
	const partName opc.PartName = "/docProps/audio.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return taken
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return taken
	}
	root := doc.Root()
	if root == nil {
		return taken
	}
	for _, id := range root.Children {
		n := doc.Node(id)
		if n == nil {
			continue
		}
		if prof, ok := parseAudioProfile(n); ok {
			taken[prof.TrackKey] = true
		}
	}
	return taken
}

func (p *Presentation) appendVideoProfilesOperation(adds []VideoProfile) (editplan.Operation, error) {
	const partName opc.PartName = "/docProps/video.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return editplan.Operation{}, &OperationError{Op: "clone.profiles", Part: string(partName),
			Message: "video profile part disappeared", Err: err}
	}
	idx := bytes.Index(b, []byte("</VideoProfiles>"))
	if idx < 0 {
		return editplan.Operation{}, &OperationError{Op: "clone.profiles", Part: string(partName),
			Message: "video profile part is malformed (no closing root)", Err: ErrMalformedPackage}
	}
	var buf bytes.Buffer
	buf.Write(b[:idx])
	for _, prof := range adds {
		buf.WriteString("<Profile " + videoProfileXMLAttrs(prof) + "/>")
	}
	buf.Write(b[idx:])
	return editplan.Patch(partName, buf.Bytes()), nil
}

// allVideoTrackKeys 返回文档内全部已用 TrackKey（视频 Profile 派生键去重用）。
func (p *Presentation) allVideoTrackKeys() map[string]bool {
	taken := map[string]bool{}
	const partName opc.PartName = "/docProps/video.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return taken
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return taken
	}
	root := doc.Root()
	if root == nil {
		return taken
	}
	for _, id := range root.Children {
		n := doc.Node(id)
		if n == nil {
			continue
		}
		if vp, ok := parseVideoProfile(n); ok {
			taken[vp.TrackKey] = true
		}
	}
	return taken
}

// videoProfilesOfSlide 返回挂在指定 slide 上的全部 VideoProfile。
func (p *Presentation) videoProfilesOfSlide(slide opc.PartName) []VideoProfile {
	const partName opc.PartName = "/docProps/video.xml"
	b, err := p.partBytes(partName)
	if err != nil {
		return nil
	}
	doc, err := xmlstore.Index(b)
	if err != nil {
		return nil
	}
	root := doc.Root()
	var out []VideoProfile
	for _, id := range root.Children {
		n := doc.Node(id)
		if n == nil {
			continue
		}
		if vp, ok := parseVideoProfile(n); ok && vp.SlidePart == slide {
			out = append(out, vp)
		}
	}
	return out
}

// uniqueTrackKey 在 taken 约束下生成唯一派生键：base、base-1、base-2…
// 命中后登记，供同事务多次派生互不冲突。
func uniqueTrackKey(base string, taken map[string]bool) string {
	if !taken[base] {
		taken[base] = true
		return base
	}
	for n := 1; ; n++ {
		cand := base + "-" + strconv.Itoa(n)
		if !taken[cand] {
			taken[cand] = true
			return cand
		}
	}
}
