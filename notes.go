package pptx

import (
	"errors"
	"strconv"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 TEXT-01 的备注 API（方案 §20.3/§10）：备注是独立 Part 与
// 母版关系体系。SetSpeakerNotes 定位备注正文占位符做结构替换，保留
// 页眉页脚、其他备注形状与未知节点——不通过重建整张 notesSlide 实现
// 普通讲稿替换；notesMaster 一律保留。
//
// 语义：没有 notes Part 时 SpeakerNotesText 返回空串和 nil；SpeakerNotes
// 返回 ErrNotFound（读取不创建 Part）；EnsureSpeakerNotes 按需创建
// notesSlide + notesMaster + 关系并注册到 presentation（notesMasterIdLst）；
// SetSpeakerNotes = Ensure + 正文结构替换（字符格式被清除，精细编辑走
// SpeakerNotes 的富文本 API）。

const (
	// ctNotesSlide/ctNotesMaster 是备注相关 Part 的内容类型。
	ctNotesSlide  = "application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"
	ctNotesMaster = "application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml"
	// relNotesMaster 是 notesMaster 关系类型 URI。
	relNotesMaster = opc.RelTypePrefix + "notesMaster"
)

// notesPartOf 返回页面关联的 notesSlide Part（首个内部关系）；无则
// ("", false)。
func (s *Slide) notesPartOf() (opc.PartName, bool) {
	set, ok := s.p.pk.Relationships(s.part)
	if !ok {
		return "", false
	}
	for _, rel := range set.All() {
		if rel.Type == opc.RelNotesSlide && rel.Mode == opc.TargetInternal {
			return rel.TargetPart, true
		}
	}
	return "", false
}

// notesBodyRef 定位 notesSlide 正文占位符（p:sp/p:txBody）。规则：
// sp 的 nvSpPr/nvPr/ph 存在且 type 缺省或为 "body"；不按坐标/名称猜测。
// 找不到返回 ErrNotFound。
func notesBodyRef(p *Presentation, part opc.PartName) ([]nodeStep, error) {
	doc, err := p.docOf(part)
	if err != nil {
		return nil, err
	}
	spTree := doc.Elements(nsPresentationML, "spTree")
	for _, tid := range spTree {
		tree := doc.Node(tid)
		for _, sid := range tree.Children {
			sp := doc.Node(sid)
			if sp.Namespace != nsPresentationML || sp.Local() != "sp" {
				continue
			}
			phType, isBody := notesPhType(doc, sp)
			if !isBody {
				continue
			}
			tx := childOfKind(doc, sp, nsPresentationML, "txBody", 0)
			if tx == nil {
				continue
			}
			_ = phType
			return recordPath(doc, tx.ID), nil
		}
	}
	return nil, &OperationError{
		Op: "notes body", Part: string(part),
		Message: "no body placeholder (p:ph type=body) with p:txBody found",
		Err:     ErrNotFound,
	}
}

// notesPhType 判断 sp 是否为备注正文占位符。
func notesPhType(doc *xmlstore.XMLDocument, sp *xmlstore.NodeRecord) (string, bool) {
	nvPr := childOfKind(doc, sp, nsPresentationML, "nvSpPr", 0)
	if nvPr == nil {
		return "", false
	}
	nvPr2 := childOfKind(doc, nvPr, nsPresentationML, "nvPr", 0)
	if nvPr2 == nil {
		return "", false
	}
	ph := childOfKind(doc, nvPr2, nsPresentationML, "ph", 0)
	if ph == nil {
		return "", false
	}
	typ, has := ph.Attr("", "type")
	if !has || typ == "body" {
		return typ, true
	}
	return typ, false
}

// SpeakerNotesText 返回页面讲稿纯文本（段落以 '\n' 连接）；无 notes
// Part 时返回空串和 nil。
func (s *Slide) SpeakerNotesText() (string, error) {
	if err := s.alive(); err != nil {
		return "", Annotate(err, "Slide.SpeakerNotesText")
	}
	notes, ok := s.notesPartOf()
	if !ok {
		return "", nil
	}
	tf, err := s.notesTextFrame(notes)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil // 有 Part 但无正文占位符：按无正文读
		}
		return "", Annotate(err, "Slide.SpeakerNotesText")
	}
	paras, err := tf.Paragraphs()
	if err != nil {
		return "", Annotate(err, "Slide.SpeakerNotesText")
	}
	var lines []string
	for _, p := range paras {
		txt, err := p.Text()
		if err != nil {
			return "", Annotate(err, "Slide.SpeakerNotesText")
		}
		lines = append(lines, txt)
	}
	return strings.Join(lines, "\n"), nil
}

// SpeakerNotes 返回页面讲稿正文 TextFrame；无 notes Part 或正文占位符
// 时返回 ErrNotFound（读取不创建）。
func (s *Slide) SpeakerNotes() (*TextFrame, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.SpeakerNotes")
	}
	notes, ok := s.notesPartOf()
	if !ok {
		return nil, Annotate(ErrNotFound, "Slide.SpeakerNotes")
	}
	return s.notesTextFrame(notes)
}

// notesTextFrame 构造备注正文 TextFrame（path 记录自当前索引）。
func (s *Slide) notesTextFrame(notes opc.PartName) (*TextFrame, error) {
	path, err := notesBodyRef(s.p, notes)
	if err != nil {
		return nil, err
	}
	return &TextFrame{textNode: textNode{p: s.p, part: notes, path: path}}, nil
}

// EnsureSpeakerNotes 返回讲稿正文 TextFrame，需要时创建 notesSlide、
// notesMaster、双方关系与 presentation 注册（notesMasterIdLst）；
// 已有合法对象直接复用。
func (s *Slide) EnsureSpeakerNotes() (*TextFrame, error) {
	if err := s.alive(); err != nil {
		return nil, Annotate(err, "Slide.EnsureSpeakerNotes")
	}
	if notes, ok := s.notesPartOf(); ok {
		return s.notesTextFrame(notes)
	}
	if err := s.createNotes(); err != nil {
		return nil, Annotate(err, "Slide.EnsureSpeakerNotes")
	}
	notes, _ := s.notesPartOf()
	return s.notesTextFrame(notes)
}

// SetSpeakerNotes 明确替换讲稿正文（字符格式清除）；无 notes Part 时
// 先 Ensure。notesMaster 与页眉页脚等其它形状保留。
func (s *Slide) SetSpeakerNotes(text string) error {
	if err := s.alive(); err != nil {
		return Annotate(err, "Slide.SetSpeakerNotes")
	}
	tf, err := s.EnsureSpeakerNotes()
	if err != nil {
		return Annotate(err, "Slide.SetSpeakerNotes")
	}
	return tf.SetPlainText(text)
}

// createNotes 在单次事务中装配 notesSlide + notesMaster + 关系 +
// presentation 注册。
func (s *Slide) createNotes() error {
	p := s.p
	slide := s.part
	main := p.main

	notesIdx := nextPartSeq(p, "/ppt/notesSlides/notesSlide", ".xml")
	masterIdx := nextPartSeq(p, "/ppt/notesMasters/notesMaster", ".xml")
	notes := opc.PartName("/ppt/notesSlides/notesSlide" + strconv.Itoa(notesIdx) + ".xml")
	master := opc.PartName("/ppt/notesMasters/notesMaster" + strconv.Itoa(masterIdx) + ".xml")
	notesRels := opc.PartName("/ppt/notesSlides/_rels/notesSlide" + strconv.Itoa(notesIdx) + ".xml.rels")
	masterRels := opc.PartName("/ppt/notesMasters/_rels/notesMaster" + strconv.Itoa(masterIdx) + ".xml.rels")

	// 1) 新建 Part 字节。
	notesXML := xmlDecl + buildNotesSlideXML()
	masterXML := xmlDecl + buildNotesMasterXML()
	notesRelsXML := xmlDecl + `<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + relNotesMaster + `" Target="../notesMasters/notesMaster` + strconv.Itoa(masterIdx) + `.xml"/>` +
		`<Relationship Id="rId2" Type="` + opc.RelSlide + `" Target="../slides/` + slideName(slide) + `"/>` +
		`</Relationships>`
	themeTarget, smTarget := notesMasterDeps(p, main)
	masterRelsXML := xmlDecl + `<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelTheme + `" Target="` + themeTarget + `"/>` +
		`<Relationship Id="rId2" Type="` + opc.RelSlideMaster + `" Target="` + smTarget + `"/>` +
		`</Relationships>`

	if err := p.stageAdd(notes, []byte(notesXML), ctNotesSlide); err != nil {
		return err
	}
	if err := p.stageAdd(master, []byte(masterXML), ctNotesMaster); err != nil {
		return err
	}
	if err := p.stageAdd(notesRels, []byte(notesRelsXML), ""); err != nil {
		return err
	}
	if err := p.stageAdd(masterRels, []byte(masterRelsXML), ""); err != nil {
		return err
	}

	// 2) slide 关系补 notesSlide（rId 按当前关系流分配）。
	slideRels, err := relsXML(p, slide)
	if err != nil {
		return err
	}
	slideRID := nextRID(slideRels)
	entry := `<Relationship Id="` + slideRID + `" Type="` + opc.RelNotesSlide +
		`" Target="../notesSlides/` + slideName(notes) + `"/>`
	newRels := insertRel(slideRels, entry)
	if err := p.stagePatch(relsPart(slide), []byte(newRels)); err != nil {
		return err
	}

	// 3) presentation.xml 补 notesMasterIdLst + 关系。
	presDoc, err := p.docOf(main)
	if err != nil {
		return err
	}
	presXML := presDoc.Original()
	presRels, err := relsXML(p, main)
	if err != nil {
		return err
	}
	rid := nextRID(presRels)
	presPatches, err := addNotesMasterToPresentation(presDoc, rid)
	if err != nil {
		return err
	}
	if len(presPatches) > 0 {
		out, err := xmlstore.ApplyPatches(presXML, presPatches)
		if err != nil {
			return err
		}
		if err := p.stagePatch(main, out); err != nil {
			return err
		}
	}
	presRels2 := insertRel(presRels,
		`<Relationship Id="`+rid+`" Type="`+relNotesMaster+`" Target="notesMasters/notesMaster`+strconv.Itoa(masterIdx)+`.xml"/>`)
	if err := p.stagePatch(relsPart(main), []byte(presRels2)); err != nil {
		return err
	}

	p.commit()
	return nil
}

// relsPart 返回 Part 的关系流名（包根 "/" 返回 "/_rels/.rels"）。
func relsPart(part opc.PartName) opc.PartName {
	if part == "/" {
		return "/_rels/.rels"
	}
	// /ppt/x.xml → /ppt/_rels/x.xml.rels
	trim := strings.TrimPrefix(string(part), "/")
	i := strings.LastIndex(trim, "/")
	if i < 0 {
		return opc.PartName("/_rels/" + trim + ".rels")
	}
	dir, base := trim[:i], trim[i+1:]
	return opc.PartName("/" + dir + "/_rels/" + base + ".rels")
}

func relsXML(p *Presentation, part opc.PartName) ([]byte, error) {
	rp := relsPart(part)
	if p.pk.HasPart(rp) {
		return p.partBytes(rp)
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<Relationships xmlns="` + nsPkgRels + `"/>`), nil
}

// insertRel 在 </Relationships> 前插入一条关系。
func insertRel(rels []byte, entry string) []byte {
	s := string(rels)
	i := strings.LastIndex(s, "</Relationships>")
	if i < 0 {
		// 空/畸形：整体替换为合法关系流。
		return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
			`<Relationships xmlns="` + nsPkgRels + `">` + entry + `</Relationships>`)
	}
	return []byte(s[:i] + entry + s[i:])
}

// nextRID 计算关系流里下一个 rId（rIdN 最大序号 +1）。
func nextRID(rels []byte) string {
	max := 0
	for _, tok := range strings.Fields(string(rels)) {
		if strings.HasPrefix(tok, `Id="rId`) {
			rest := strings.TrimSuffix(strings.TrimPrefix(tok, `Id="rId`), `"`)
			if n, err := strconv.Atoi(rest); err == nil && n > max {
				max = n
			}
		}
	}
	return "rId" + strconv.Itoa(max+1)
}

// slideName 取 Part 名最后一段（"slide1.xml"）。
func slideName(part opc.PartName) string {
	s := string(part)
	i := strings.LastIndex(s, "/")
	return s[i+1:]
}

// addNotesMasterToPresentation 构造 presentation.xml 的补丁：
// 若无 notesMasterIdLst，在 sldMasterIdLst 之后插入含 r:id 的列表。
func addNotesMasterToPresentation(doc *xmlstore.XMLDocument, rid string) ([]xmlstore.SpanPatch, error) {
	root := doc.Root()
	if root == nil || root.Namespace != nsPresentationML || root.Local() != "presentation" {
		return nil, &OperationError{
			Op: "EnsureSpeakerNotes", Part: "presentation",
			Message: "presentation root element is not p:presentation",
			Err:     ErrMalformedPackage,
		}
	}
	// 已有 notesMasterIdLst：跳过（只补关系）。
	for _, cid := range root.Children {
		c := doc.Node(cid)
		if c.Namespace == nsPresentationML && c.Local() == "notesMasterIdLst" {
			return nil, nil
		}
	}
	anchor := childOfKind(doc, root, nsPresentationML, "sldMasterIdLst", 0)
	frag := `<p:notesMasterIdLst><p:notesMasterId r:id="` + rid + `"/></p:notesMasterIdLst>`
	if anchor != nil {
		// notesMasterIdLst 必须紧跟 sldMasterIdLst。
		patch, err := xmlstore.InsertAfter(anchor, []byte(frag))
		if err != nil {
			return nil, err
		}
		return []xmlstore.SpanPatch{patch}, nil
	}
	// 无 sldMasterIdLst（非常规）：插到根内容最前。
	patch, err := xmlstore.InsertBefore(firstChildOf(doc, root), []byte(frag))
	if err != nil {
		return nil, err
	}
	return []xmlstore.SpanPatch{patch}, nil
}

// buildNotesSlideXML 生成 notesSlide 主体（含正文占位符 sp）。
func buildNotesSlideXML() string {
	return `<p:notes xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`<p:sp>` +
		`<p:nvSpPr><p:cNvPr id="2" name="Notes Body"/><p:cNvSpPr/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p/></p:txBody>` +
		`</p:sp>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:notes>`
}

// buildNotesMasterXML 生成最小 notesMaster（页眉页脚等占位符形状在
// 需要时由版式继承；本库保证 notesMaster 存在且被保留，不重建）。
func buildNotesMasterXML() string {
	return `<p:notesMaster xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`</p:spTree></p:cSld>` +
		`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`</p:notesMaster>`
}

// notesMasterDeps 返回 notesMaster 应引用的 theme 与 slideMaster 目标
// （相对 /ppt/notesMasters/ 目录）：解析真实关系链
// presentation → slideMaster → theme，不硬编码 Part 名。
func notesMasterDeps(p *Presentation, main opc.PartName) (themeTarget, smTarget string) {
	masters := p.pk.RelatedParts(main, opc.RelSlideMaster)
	if len(masters) == 0 {
		return "../theme/theme1.xml", "../slideMasters/slideMaster1.xml"
	}
	sm := masters[0]
	smTarget = "../slideMasters/" + slideName(sm)
	themes := p.pk.RelatedParts(sm, opc.RelTheme)
	if len(themes) == 0 {
		return "../theme/theme1.xml", smTarget
	}
	return "../theme/" + slideName(themes[0]), smTarget
}

// nextPartSeq 计算目录内同类 Part 的下一个序号（max+1，至少 1）。
func nextPartSeq(p *Presentation, prefix, suffix string) int {
	max := 0
	for _, name := range p.pk.PartNames() {
		s := string(name)
		if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, suffix) {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(s, prefix), suffix)
		if n, err := strconv.Atoi(mid); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}
