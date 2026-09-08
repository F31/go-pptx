package pptx

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现页面 API 收口（方案 §20.1 + 实施计划 M2"页面 API 收口"）：
//
//	Presentation.Slide(index)       0 基下标；越界 ErrOutOfRange
//	Presentation.Slides()           （既有，读视图化）
//	Presentation.Layouts()          沿 master 关系收集版式（LayoutRef）
//	Presentation.AddSlide(layout)   新建页面 + sldId/rId 注册（layout 绑定文档）
//	Presentation.MoveSlide(id,index) index 为最终列表目标位置；事务修改顺序
//	Presentation.RemoveSlide(id)    处理已知引用（notesSlide/自身关系流），
//	                               未知依赖阻止删除
//
// LayoutRef 绑定所属文档：把其它文档的版式句柄直接传给 AddSlide 返回
// ErrForeignReference；不存在"自动用第一个版式替代"的回退行为。slide id
// 在合法区间 [256, 2147483647] 分配并检查耗尽；页面顺序始终由
// presentation.xml 的 sldIdLst 决定（§19.2），不按 id 数值排序。

const ctSlide = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"

// ---------- Slide(index) ----------

// Slide 返回零基下标对应的页面句柄；越界返回 ErrOutOfRange。
// 页面顺序为 presentation.xml 的 sldIdLst 顺序（与 Slides 一致）。
func (p *Presentation) Slide(index int) (*Slide, error) {
	if p.closed {
		return nil, Annotate(ErrClosed, "Presentation.Slide")
	}
	slides, err := p.Slides()
	if err != nil {
		return nil, Annotate(err, "Presentation.Slide")
	}
	if index < 0 || index >= len(slides) {
		return nil, &OperationError{
			Op:      "Presentation.Slide",
			Message: fmt.Sprintf("slide index %d out of range [0,%d)", index, len(slides)),
			Err:     ErrOutOfRange,
		}
	}
	return slides[index], nil
}

// ---------- Layouts / LayoutRef / AddSlide ----------

// LayoutRef 是版式（p:sldLayout）的受控引用，绑定所属文档。
// 仅可用于同一文档的 AddSlide；跨文档使用返回 ErrForeignReference。
type LayoutRef struct {
	p    *Presentation
	part opc.PartName
}

// Name 返回版式名称（p:cSld@name）；读取失败或文档关闭返回空串。
func (l *LayoutRef) Name() string {
	if l == nil || l.p == nil || l.p.closed {
		return ""
	}
	doc, err := l.p.docOf(l.part)
	if err != nil {
		return ""
	}
	for _, cid := range doc.Root().Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML || c.Local() != "cSld" {
			continue
		}
		v, _ := c.Attr("", "name")
		return v
	}
	return ""
}

// Layouts 返回文档全部版式引用：沿主 Part → 各 slideMaster 的关系流
// 收集 slideLayout（master 序 × 关系序，去重）。无版式返回空切片。
func (p *Presentation) Layouts() ([]*LayoutRef, error) {
	if p.closed {
		return nil, Annotate(ErrClosed, "Presentation.Layouts")
	}
	masters, ok, err := p.relsOf(p.main)
	if err != nil || !ok {
		return nil, Annotate(err, "Presentation.Layouts")
	}
	seen := make(map[opc.PartName]bool)
	var out []*LayoutRef
	for _, rel := range masters {
		if rel.Mode != opc.TargetInternal || rel.Type != opc.RelSlideMaster {
			continue
		}
		m := rel.TargetPart
		if seen[m] {
			continue
		}
		seen[m] = true
		lrels, lok, lerr := p.relsOf(m)
		if lerr != nil {
			return nil, Annotate(lerr, "Presentation.Layouts")
		}
		if !lok {
			continue
		}
		for _, lr := range lrels {
			if lr.Mode != opc.TargetInternal || lr.Type != opc.RelSlideLayout {
				continue
			}
			if seen[lr.TargetPart] {
				continue
			}
			seen[lr.TargetPart] = true
			out = append(out, &LayoutRef{p: p, part: lr.TargetPart})
		}
	}
	return out, nil
}

// hasPartCurrent 判断 Part 在当前读视图存在（包内或会话内新增且未删除）。
func (p *Presentation) hasPartCurrent(name opc.PartName) bool {
	if p.deletedParts[name] {
		return false
	}
	return p.pk.HasPart(name) || p.addedParts[name].Content != nil
}

// AddSlide 基于指定版式新增一页：新建 slide Part（含到 layout 的关系）
// 并注册到 presentation.xml（sldIdLst 追加 + 主关系新增 rId）。layout
// 必须属于本文档（跨文档 ErrForeignReference）；新增页面为空 spTree
// （占位符继承自版式，客户端显示版式背景与占位符）。
//
// slide id 在 [256, 2147483647] 取最小空闲值；耗尽返回
// ErrLimitExceeded。返回页面句柄（一次事务提交后有效）。
func (p *Presentation) AddSlide(layout *LayoutRef) (*Slide, error) {
	if p.closed {
		return nil, Annotate(ErrClosed, "Presentation.AddSlide")
	}
	if layout == nil {
		return nil, &OperationError{
			Op: "Presentation.AddSlide", Message: "layout is nil", Err: ErrInvalidArgument,
		}
	}
	if layout.p != p {
		return nil, &OperationError{
			Op: "Presentation.AddSlide", Part: string(layout.part),
			Message: "layout belongs to another document; use Layouts() of this document",
			Err:     ErrForeignReference,
		}
	}
	if !p.hasPartCurrent(layout.part) {
		return nil, &OperationError{
			Op: "Presentation.AddSlide", Part: string(layout.part),
			Message: "layout part does not exist", Err: ErrNotFound,
		}
	}

	doc, err := p.presentationDoc()
	if err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}
	used, err := usedSlideIDs(doc)
	if err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}
	id, err := allocSlideID(used)
	if err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}

	idx := nextPartSeq(p, "/ppt/slides/slide", ".xml")
	slide := opc.PartName("/ppt/slides/slide" + strconv.Itoa(idx) + ".xml")
	slideRels := relsPart(slide)

	// 1) slide Part 与自身关系流（stageAdd；ContentType 走 Override 计划）。
	slideXML := xmlDecl + buildSlideXML()
	if err := p.stageAdd(slide, []byte(slideXML), ctSlide); err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}
	relsXMLBody := xmlDecl + `<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/` + slideName(layout.part) + `"/>` +
		`</Relationships>`
	if err := p.stageAdd(slideRels, []byte(relsXMLBody), ""); err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}

	// 2) presentation.xml 追加 p:sldId。
	mainRels, err := relsXML(p, p.main)
	if err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}
	rid := nextRID(mainRels)
	sldIdFrag := `<p:sldId id="` + strconv.FormatUint(uint64(id), 10) + `" r:id="` + rid + `"/>`
	patches, err := appendSldIdPatch(doc, sldIdFrag)
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Presentation.AddSlide")
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Presentation.AddSlide")
	}
	if err := p.stagePatch(p.main, out); err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}

	// 3) 主关系流新增 rId（part 存在走补丁；缺失按新增注册）。
	entry := `<Relationship Id="` + rid + `" Type="` + opc.RelSlide +
		`" Target="slides/` + slideName(slide) + `"/>`
	updated := insertRel(mainRels, entry)
	if err := stageRelsBytes(p, p.main, updated); err != nil {
		return nil, Annotate(err, "Presentation.AddSlide")
	}

	p.commit()
	return &Slide{p: p, id: id, part: slide, rev: p.rev}, nil
}

// usedSlideIDs 收集 sldIdLst 已用 slide id。
func usedSlideIDs(doc *xmlstore.XMLDocument) ([]SlideID, error) {
	var out []SlideID
	for _, lid := range doc.Elements(nsPresentationML, "sldIdLst") {
		lst := doc.Node(lid)
		for _, cid := range lst.Children {
			c := doc.Node(cid)
			if c.Namespace != nsPresentationML || c.Local() != "sldId" {
				continue
			}
			s, ok := c.Attr("", "id")
			if !ok {
				return nil, &OperationError{
					Op: "sldId", Message: "p:sldId missing id attribute", Err: ErrMalformedPackage,
				}
			}
			v, err := parseUint32(s)
			if err != nil || v < 256 || v > 2147483647 {
				return nil, &OperationError{
					Op: "sldId", Message: fmt.Sprintf("p:sldId id %q outside [256,2147483647]", s),
					Err: ErrMalformedPackage,
				}
			}
			out = append(out, SlideID(v))
		}
	}
	return out, nil
}

// allocSlideID 在合法区间取最小空闲值；耗尽 ErrLimitExceeded。
func allocSlideID(used []SlideID) (SlideID, error) {
	sort.Slice(used, func(i, j int) bool { return used[i] < used[j] })
	cand := uint64(256)
	for _, u := range used {
		if uint64(u) == cand {
			cand++
		} else if uint64(u) > cand {
			break
		}
	}
	if cand > 2147483647 {
		return 0, &OperationError{
			Op: "AddSlide", Message: "slide id space exhausted", Err: ErrLimitExceeded,
		}
	}
	return SlideID(cand), nil
}

// appendSldIdPatch 构造 presentation.xml 追加单个 p:sldId 的补丁。
// 已有 sldIdLst（含自闭合）原地展开；缺失时按 schema 顺序在
// handoutMasterIdLst/notesMasterIdLst/sldMasterIdLst 之后新建列表。
func appendSldIdPatch(doc *xmlstore.XMLDocument, sldIdFrag string) ([]xmlstore.SpanPatch, error) {
	root := doc.Root()
	if root == nil || root.Namespace != nsPresentationML || root.Local() != "presentation" {
		return nil, &OperationError{
			Op: "AddSlide", Part: "presentation",
			Message: "presentation root element is not p:presentation", Err: ErrMalformedPackage,
		}
	}
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML || c.Local() != "sldIdLst" {
			continue
		}
		if !c.SelfClosing() {
			p, err := xmlstore.AppendChild(c, []byte(sldIdFrag))
			if err != nil {
				return nil, err
			}
			return []xmlstore.SpanPatch{p}, nil
		}
		// 自闭合 <p:sldIdLst/> → 展开为 <p:sldIdLst>...</p:sldIdLst>。
		return []xmlstore.SpanPatch{{
			Start:       c.OpenEnd - 2,
			End:         c.OpenEnd,
			Replacement: []byte(">" + sldIdFrag + "</p:sldIdLst>"),
		}}, nil
	}
	// 缺失：按 schema 顺序在 handout/notes/sldMaster 列表之后插入。
	lstFrag := `<p:sldIdLst>` + sldIdFrag + `</p:sldIdLst>`
	var anchor *xmlstore.NodeRecord
	for _, want := range []string{"handoutMasterIdLst", "notesMasterIdLst", "sldMasterIdLst"} {
		if a := childOfKind(doc, root, nsPresentationML, want, 0); a != nil {
			anchor = a
		}
	}
	if anchor != nil {
		p, err := xmlstore.InsertAfter(anchor, []byte(lstFrag))
		if err != nil {
			return nil, err
		}
		return []xmlstore.SpanPatch{p}, nil
	}
	p, err := xmlstore.InsertBefore(firstChildOf(doc, root), []byte(lstFrag))
	if err != nil {
		return nil, err
	}
	return []xmlstore.SpanPatch{p}, nil
}

// buildSlideXML 生成新建页面主体（空 spTree；占位符由版式继承）。
func buildSlideXML() string {
	return `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sld>`
}

// ---------- MoveSlide ----------

// sldIdEntry 是 sldIdLst 中一个页面条目（解析自 presentation.xml）。
type sldIdEntry struct {
	id   SlideID
	rid  string
	node *xmlstore.NodeRecord
}

// collectSldIds 返回 sldIdLst 中按文档序的页面条目；无列表返回空切片
// （不报错——文档可合法为空，由调用方决定语义）。
func collectSldIds(doc *xmlstore.XMLDocument) ([]sldIdEntry, error) {
	var out []sldIdEntry
	for _, lid := range doc.Elements(nsPresentationML, "sldIdLst") {
		lst := doc.Node(lid)
		for _, cid := range lst.Children {
			c := doc.Node(cid)
			if c.Namespace != nsPresentationML || c.Local() != "sldId" {
				continue
			}
			idStr, ok := c.Attr("", "id")
			if !ok {
				return nil, &OperationError{
					Op: "sldId", Message: "p:sldId missing id attribute", Err: ErrMalformedPackage,
				}
			}
			id, err := parseUint32(idStr)
			if err != nil || id < 256 || id > 2147483647 {
				return nil, &OperationError{
					Op: "sldId", Message: fmt.Sprintf("p:sldId id %q outside [256,2147483647]", idStr),
					Err: ErrMalformedPackage,
				}
			}
			rid, ok := c.Attr(nsOfficeDocument, "id")
			if !ok {
				return nil, &OperationError{
					Op: "sldId", Message: "p:sldId missing r:id attribute", Err: ErrMalformedPackage,
				}
			}
			out = append(out, sldIdEntry{id: SlideID(id), rid: rid, node: c})
		}
	}
	return out, nil
}

// MoveSlide 把 id 对应页面移动到 index（0 基）作为最终列表目标位置：
// 移动后该页在 sldIdLst 中的位置即 index。index 越界 ErrOutOfRange；
// id 不存在 ErrNotFound；移动到当前位置为 no-op。以一次事务修改顺序，
// 未知非 sldId 子元素原地保留。
func (p *Presentation) MoveSlide(id SlideID, index int) error {
	if p.closed {
		return Annotate(ErrClosed, "Presentation.MoveSlide")
	}
	doc, err := p.presentationDoc()
	if err != nil {
		return Annotate(err, "Presentation.MoveSlide")
	}
	entries, err := collectSldIds(doc)
	if err != nil {
		return Annotate(err, "Presentation.MoveSlide")
	}
	n := len(entries)
	if index < 0 || index >= n {
		return &OperationError{
			Op: "Presentation.MoveSlide", Message: fmt.Sprintf("index %d out of range [0,%d)", index, n),
			Err: ErrOutOfRange,
		}
	}
	i := -1
	for k := range entries {
		if entries[k].id == id {
			i = k
			break
		}
	}
	if i < 0 {
		return &OperationError{
			Op: "Presentation.MoveSlide", Message: fmt.Sprintf("slide id %d not found", id),
			Err: ErrNotFound,
		}
	}
	if i == index {
		return nil
	}
	orig := doc.Original()
	moved := entries[i].node
	bytes := doc.Slice(moved.Source)
	var patches []xmlstore.SpanPatch
	if index < i {
		// 前移：插到目标 index 元素（原列表）之前。
		patches = append(patches, xmlstore.SpanPatch{
			Start: entries[index].node.Source.Start, End: entries[index].node.Source.Start,
			Replacement: bytes,
		})
	} else {
		// 后移：插到原 index+1 元素之前；无则列表末尾。
		if index+1 < n {
			patches = append(patches, xmlstore.SpanPatch{
				Start: entries[index+1].node.Source.Start, End: entries[index+1].node.Source.Start,
				Replacement: bytes,
			})
		} else {
			last := entries[n-1].node.Source.End
			patches = append(patches, xmlstore.SpanPatch{
				Start: last, End: last, Replacement: bytes,
			})
		}
	}
	patches = append(patches, xmlstore.SpanPatch{
		Start: moved.Source.Start, End: moved.Source.End, Replacement: []byte(""),
	})
	// 删除区间与插入点互不重叠：确保补丁按升序提交（ApplyPatches 校验）。
	sort.SliceStable(patches, func(a, b int) bool { return patches[a].Start < patches[b].Start })
	out, err := xmlstore.ApplyPatches(orig, patches)
	if err != nil {
		return Annotate(mapXMLError(err), "Presentation.MoveSlide")
	}
	if err := p.stagePatch(p.main, out); err != nil {
		return Annotate(err, "Presentation.MoveSlide")
	}
	p.commit()
	return nil
}

// ---------- RemoveSlide ----------

// RemoveSlide 删除 id 对应页面：移除 sldIdLst 条目与主关系，删除 slide
// Part 及其关系流；关联的 notesSlide 一并删除（notesMaster 保留）。
// 若存在未知 Part 仍引用该 slide，拒绝删除并返回 ErrUnsupportedEdit
// （保守口径：不解析未知消费方，绝不破坏共享引用）。媒体等共享资源
// 不在此删除（引用归零清理属资源 GC 语义，随后续工作包）。
func (p *Presentation) RemoveSlide(id SlideID) error {
	if p.closed {
		return Annotate(ErrClosed, "Presentation.RemoveSlide")
	}
	doc, err := p.presentationDoc()
	if err != nil {
		return Annotate(err, "Presentation.RemoveSlide")
	}
	entries, err := collectSldIds(doc)
	if err != nil {
		return Annotate(err, "Presentation.RemoveSlide")
	}
	var ent *sldIdEntry
	for k := range entries {
		if entries[k].id == id {
			ent = &entries[k]
			break
		}
	}
	if ent == nil {
		return &OperationError{
			Op: "Presentation.RemoveSlide", Message: fmt.Sprintf("slide id %d not found", id),
			Err: ErrNotFound,
		}
	}

	// 解析 rId → slide Part（读视图）。
	mainRels, _, err := p.relsOf(p.main)
	if err != nil {
		return Annotate(err, "Presentation.RemoveSlide")
	}
	var slidePart opc.PartName
	for _, rel := range mainRels {
		if rel.ID == ent.rid && rel.Mode == opc.TargetInternal {
			slidePart = rel.TargetPart
			break
		}
	}
	if slidePart == "" {
		return &OperationError{
			Op: "Presentation.RemoveSlide", Part: string(p.main),
			Message: fmt.Sprintf("sldId %d has no internal slide relationship %q", id, ent.rid),
			Err:     ErrNotFound,
		}
	}

	// 已知依赖：本页关联的 notesSlide（删除时连带）。
	var notesPart opc.PartName
	if rels, ok, err := p.relsOf(slidePart); err == nil && ok {
		for _, rel := range rels {
			if rel.Mode == opc.TargetInternal && rel.Type == opc.RelNotesSlide {
				notesPart = rel.TargetPart
				break
			}
		}
	}

	// 未知依赖阻止：除主 Part（sldId 关系）与待删 notesSlide（回引）外，
	// 任何内部关系指向本 slide 都拒绝（不猜测消费方）。
	if src, dep := p.unknownSlideRef(slidePart, notesPart); dep {
		return &OperationError{
			Op: "Presentation.RemoveSlide", Part: string(slidePart),
			Message: "slide is referenced by unknown part " + string(src) + "; refusing to delete",
			Err:     ErrUnsupportedEdit,
		}
	}

	// 应用变更（单事务）：presentation.xml 移除条目、主关系移除 rId、
	// 删除 slide/notes Part 与其关系流。
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{{
		Start: ent.node.Source.Start, End: ent.node.Source.End, Replacement: []byte(""),
	}})
	if err != nil {
		return Annotate(mapXMLError(err), "Presentation.RemoveSlide")
	}
	if err := p.stagePatch(p.main, out); err != nil {
		return Annotate(err, "Presentation.RemoveSlide")
	}
	relsBytes, err := relsXML(p, p.main)
	if err != nil {
		return Annotate(err, "Presentation.RemoveSlide")
	}
	if r := removeRelEntry(relsBytes, ent.rid); !bytes.Equal(r, relsBytes) {
		if err := stageRelsBytes(p, p.main, r); err != nil {
			return Annotate(err, "Presentation.RemoveSlide")
		}
	}
	if err := p.stageDelete(slidePart); err != nil {
		return Annotate(err, "Presentation.RemoveSlide")
	}
	if p.pk.HasPart(relsPart(slidePart)) || p.addedParts[relsPart(slidePart)].Content != nil {
		if err := p.stageDelete(relsPart(slidePart)); err != nil {
			return Annotate(err, "Presentation.RemoveSlide")
		}
	}
	if notesPart != "" {
		if err := p.stageDelete(notesPart); err != nil {
			return Annotate(err, "Presentation.RemoveSlide")
		}
		np := relsPart(notesPart)
		if p.pk.HasPart(np) || p.addedParts[np].Content != nil {
			if err := p.stageDelete(np); err != nil {
				return Annotate(err, "Presentation.RemoveSlide")
			}
		}
	}
	p.commit()
	return nil
}

// unknownSlideRef 返回第一个未知引用 source：遍历全部 Part（含会话新增，
// 排除 .rels 源、已删除与主 Part），内部关系指向 slidePart 且源不是
// 待删 notesSlide 的视为未知依赖。
func (p *Presentation) unknownSlideRef(slidePart, notesPart opc.PartName) (opc.PartName, bool) {
	seen := map[opc.PartName]bool{}
	consider := func(name opc.PartName) opc.PartName {
		if seen[name] {
			return ""
		}
		seen[name] = true
		if name == p.main || name == notesPart || p.deletedParts[name] {
			return ""
		}
		if strings.HasSuffix(string(name), ".rels") {
			return ""
		}
		rels, ok, err := p.relsOf(name)
		if err != nil || !ok {
			return ""
		}
		for _, rel := range rels {
			if rel.Mode == opc.TargetInternal && rel.TargetPart == slidePart {
				return name
			}
		}
		return ""
	}
	for _, name := range p.pk.PartNames() {
		if src := consider(name); src != "" {
			return src, true
		}
	}
	for name := range p.addedParts {
		if src := consider(name); src != "" {
			return src, true
		}
	}
	return "", false
}
