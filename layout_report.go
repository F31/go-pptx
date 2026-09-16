package pptx

import (
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 LAYOUT-01（方案 §2.3 + V2.6 §941 工作包收口）：
//
//	章节 / 嵌入字体识别 / 讲义母版 + 视图属性 / 避头尾规则
//
// 设计上四项均达 R 档："可解析并输出诊断，不提供写入 API"。本文件
// 严格不引入任何对 p14:sectionLst / p:embeddedFontLst / p:handoutMaster
// 或 kinsoku 的写入路径——所有方法仅返回只读数据与诊断。
//
// 输出布局：
//
//	LayoutReport {
//	    Sections      []LayoutSection       // p14:sectionLst
//	    EmbeddedFonts []LayoutEmbeddedFont  // 各 sldMaster 的 p:embeddedFontLst
//	    HandoutMaster *HandoutMasterInfo    // 主关系 RelHandoutMaster（如有）
//	    Kinsoku       []KinsokuRule         // a:kinsoku/kumimoji/lang 在母版文本样式的汇总
//	    Diagnostics   []Diagnostic          // 解析期发现（缺失关系、未引用 sldId 等）
//	}
//
// 解析期原则：
//   - 解析失败绝不返回 error，只把问题降级为 Diagnostic；调用方按需
//     走 Presentation.Validate() 升级到 SeverityError。
//   - 缺失元素（无 p14:sectionLst / 无 p:embeddedFontLst / 无关系目标 Part）
//     记为 Warning 级诊断（layout.<feature>.broken_ref）。

// LayoutReport 是母版/版式/章节/嵌入字体/讲义母版/避头尾规则四项 R 档
// 解析结果。零值即"全部不存在"，调用方无需 nil 判空即可正常遍历。
type LayoutReport struct {
	// Sections 是 p14:sectionLst 章节列表（按文档序）。
	Sections []LayoutSection
	// EmbeddedFonts 是各 slideMaster 下所有 p:embeddedFontLst 条目
	//（按 master 序 × 文档序）。
	EmbeddedFonts []LayoutEmbeddedFont
	// HandoutMaster 描述 p:handoutMasterIdLst + 主关系中的讲义母版
	// 引用；nil 表示文档未声明讲义母版。
	HandoutMaster *HandoutMasterInfo
	// Kinsoku 是 a:kinsoku / a:lang / a:altLang / a:kumimoji 在母版
	// 文本样式（p:txStyles）中检测到的（lang → parts）映射，按 lang
	// 文档序聚合。
	Kinsoku []KinsokuRule
	// Diagnostics 是解析期发现的诊断条目（Warning / Error 级）。
	Diagnostics []Diagnostic
}

// LayoutSection 描述 p14:sectionLst 中的一节（V2.6 §2.3 章节读写）。
type LayoutSection struct {
	// ID 是 p14:section 的 id 属性（由 p14:section@id）。
	ID string
	// Name 是 p14:section 的 name 属性（可空）。
	Name string
	// Type 是 p14:section@type（如 "nextPage" / "continuous"），可空。
	Type string
	// SlideIDs 是 p14:sldIdLst 中按文档序的 p:sldId 引用。
	SlideIDs []SlideID
}

// LayoutEmbeddedFont 描述 p:embeddedFontLst 中的一行嵌入字体。
type LayoutEmbeddedFont struct {
	// MasterPart 是所属母版 Part 路径。
	MasterPart opc.PartName
	// Typeface 是 p:font@typeface。
	Typeface string
	// HasRegular / HasBold / HasItalic / HasBoldItalic 标记四种 R-ID
	// 是否存在。
	HasRegular, HasBold, HasItalic, HasBoldItalic bool
	// RegularTargetPart / BoldTargetPart / ItalicTargetPart /
	// BoldItalicTargetPart 在 r:id 关系可达时为字体 Part 名，不可达
	// （关系不存在 / TargetPart 空）时为空字符串。
	RegularTargetPart    opc.PartName
	BoldTargetPart       opc.PartName
	ItalicTargetPart     opc.PartName
	BoldItalicTargetPart opc.PartName
}

// HandoutMasterInfo 描述文档的讲义母版绑定。
type HandoutMasterInfo struct {
	// Part 是讲义母版 Part 名（关系 TargetPart 解析结果）。
	Part opc.PartName
	// RelationID 是 p:handoutMasterIdLst 条目引用的主关系 r:id。
	RelationID string
	// Present 表示 Part 是否在包内实际存在（Part 不存在记 Diagnostics
	// 并保留 Part 路径以便客户端追查）。
	Present bool
}

// KinsokuRule 描述母版文本样式中聚合的一种语言 / kinsoku 标记。
type KinsokuRule struct {
	// Lang 是 a:lang / a:altLang 的值（如 "en-US"、"ja-JP"、"zh-CN"）。
	Lang string
	// AltLang 是配套的 a:altLang 值（可空）。
	AltLang string
	// Kumimoji 表示在母版文本样式任一处检测到 a:kumimoji="1"。
	Kumimoji bool
	// KinsokuFlag 表示在母版文本样式任一处检测到 a:kinsoku="1"（OOXML
	// 实际可能不出现此属性，作为 R 档探测保留）。
	KinsokuFlag bool
	// Parts 是包含此语言的母版 Part 列表。
	Parts []opc.PartName
}

// ---------- Presentation API ----------

// LayoutInfo 返回 LAYOUT-01 R 档只读报告（章节/嵌入字体/讲义母版/
// 避头尾规则）。解析全程不返回 error：缺失与畸形均降级为 Diagnostic。
// 文档关闭时调用返回空报告 + 一条 ErrClosed 诊断。
func (p *Presentation) LayoutInfo() (*LayoutReport, error) {
	if p.closed {
		return nil, Annotate(ErrClosed, "Presentation.LayoutInfo")
	}
	rep := &LayoutReport{}
	masters, err := p.listMasterParts()
	if err != nil {
		return rep, Annotate(err, "Presentation.LayoutInfo")
	}

	// 1) 章节 p14:sectionLst。
	if err := p.parseSectionsInto(rep); err != nil {
		return rep, Annotate(err, "Presentation.LayoutInfo")
	}

	// 2) 嵌入字体 p:embeddedFontLst（各 master）。
	for _, m := range masters {
		if err := p.parseEmbeddedFonts(m, rep); err != nil {
			return rep, Annotate(err, "Presentation.LayoutInfo")
		}
	}

	// 3) 讲义母版（主关系 RelHandoutMaster）。
	if err := p.parseHandoutMaster(rep); err != nil {
		return rep, Annotate(err, "Presentation.LayoutInfo")
	}

	// 4) kinsoku 规则（母版文本样式 lang/altLang/kumimoji 聚合）。
	for _, m := range masters {
		if err := p.parseMasterKinsoku(m, rep); err != nil {
			return rep, Annotate(err, "Presentation.LayoutInfo")
		}
	}

	return rep, nil
}

// ---------- 章节解析 ----------

// parseSectionsInto 解析 p14:sectionLst，写入 rep.Sections。缺失列表
// 不报错，引用不存在的 sldId 产生 layout.section.broken_ref 诊断。
func (p *Presentation) parseSectionsInto(rep *LayoutReport) error {
	doc, err := p.presentationDoc()
	if err != nil {
		return err
	}
	root := doc.Root()
	if root == nil || root.Local() != "presentation" || root.Namespace != nsPresentationML {
		// 非 presentation 根（如未知封装）按"无章节"处理。
		return nil
	}
	lst := childOfKind(doc, root, nsP14ML, "sectionLst", 0)
	if lst == nil {
		// 不发出 absent 诊断——大部份文档没有章节，保持安静。
		return nil
	}
	// 仅收集根 sldIdLst（直接子元素），不进入 p14:sectionLst 嵌套列表，
	// 避免把 section 内部的 sldId 误计为"页面已用"。
	used := map[SlideID]bool{}
	for _, cid := range root.Children {
		sl := doc.Node(cid)
		if sl.Namespace != nsPresentationML || sl.Local() != "sldIdLst" {
			continue
		}
		for _, sid := range sl.Children {
			s := doc.Node(sid)
			if s.Namespace == nsPresentationML && s.Local() == "sldId" {
				if v, ok := s.Attr("", "id"); ok {
					if n, err := parseUint32(v); err == nil {
						used[SlideID(n)] = true
					}
				}
			}
		}
	}
	for _, cid := range lst.Children {
		sec := doc.Node(cid)
		if sec.Namespace != nsP14ML || sec.Local() != "section" {
			continue
		}
		idv, _ := sec.Attr("", "id")
		name, _ := sec.Attr("", "name")
		typ, _ := sec.Attr("", "type")
		item := LayoutSection{ID: idv, Name: name, Type: typ}
		sldList := childOfKind(doc, sec, nsPresentationML, "sldIdLst", 0)
		if sldList != nil {
			for _, sid := range sldList.Children {
				s := doc.Node(sid)
				if s.Namespace != nsPresentationML || s.Local() != "sldId" {
					continue
				}
				v, ok := s.Attr("", "id")
				if !ok {
					continue
				}
				n, err := parseUint32(v)
				if err != nil {
					continue
				}
				sidVal := SlideID(n)
				item.SlideIDs = append(item.SlideIDs, sidVal)
				if !used[sidVal] {
					rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
						Code:     "layout.section.broken_ref",
						Severity: SeverityWarning,
						Part:     string(p.main),
						Message:  "section " + idv + " references unknown slide id " + v,
					})
				}
			}
		}
		rep.Sections = append(rep.Sections, item)
	}
	return nil
}

// ---------- 嵌入字体解析 ----------

// listMasterParts 收集主关系下的全部 slideMaster Part（按文档序，去重）。
func (p *Presentation) listMasterParts() ([]opc.PartName, error) {
	rels, ok, err := p.relsOf(p.main)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	seen := map[opc.PartName]bool{}
	var out []opc.PartName
	for _, rel := range rels {
		if rel.Mode != opc.TargetInternal || rel.Type != opc.RelSlideMaster {
			continue
		}
		if seen[rel.TargetPart] {
			continue
		}
		seen[rel.TargetPart] = true
		out = append(out, rel.TargetPart)
	}
	return out, nil
}

// parseEmbeddedFonts 解析单个母版的 p:embeddedFontLst 写入 rep。
func (p *Presentation) parseEmbeddedFonts(master opc.PartName, rep *LayoutReport) error {
	doc, err := p.docOf(master)
	if err != nil {
		// 母版损坏不在 LAYOUT-01 范围；仅提示。
		rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
			Code:     "layout.master.unreadable",
			Severity: SeverityWarning,
			Part:     string(master),
			Message:  "slide master XML cannot be parsed: " + err.Error(),
		})
		return nil
	}
	mrels, _, _ := p.relsOf(master)
	for _, lid := range doc.Elements(nsPresentationML, "embeddedFontLst") {
		lst := doc.Node(lid)
		for _, eid := range lst.Children {
			ef := doc.Node(eid)
			if ef.Namespace != nsPresentationML || ef.Local() != "embeddedFont" {
				continue
			}
			fontNode := childOfKind(doc, ef, nsPresentationML, "font", 0)
			typeface := ""
			if fontNode != nil {
				typeface, _ = fontNode.Attr("", "typeface")
			}
			item := LayoutEmbeddedFont{MasterPart: master, Typeface: typeface}
			if v, ok := sectionVariantRel(doc, ef, "regular"); ok {
				item.HasRegular = true
				if rid, ok2 := v.Attr(nsOfficeDocument, "id"); ok2 {
					item.RegularTargetPart = lookupRelTarget(mrels, rid)
					if item.RegularTargetPart == "" {
						rep.Diagnostics = append(rep.Diagnostics, diagBrokenFont(master, typeface, "regular", rid))
					}
				}
			}
			if v, ok := sectionVariantRel(doc, ef, "bold"); ok {
				item.HasBold = true
				if rid, ok2 := v.Attr(nsOfficeDocument, "id"); ok2 {
					item.BoldTargetPart = lookupRelTarget(mrels, rid)
					if item.BoldTargetPart == "" {
						rep.Diagnostics = append(rep.Diagnostics, diagBrokenFont(master, typeface, "bold", rid))
					}
				}
			}
			if v, ok := sectionVariantRel(doc, ef, "italic"); ok {
				item.HasItalic = true
				if rid, ok2 := v.Attr(nsOfficeDocument, "id"); ok2 {
					item.ItalicTargetPart = lookupRelTarget(mrels, rid)
					if item.ItalicTargetPart == "" {
						rep.Diagnostics = append(rep.Diagnostics, diagBrokenFont(master, typeface, "italic", rid))
					}
				}
			}
			if v, ok := sectionVariantRel(doc, ef, "boldItalic"); ok {
				item.HasBoldItalic = true
				if rid, ok2 := v.Attr(nsOfficeDocument, "id"); ok2 {
					item.BoldItalicTargetPart = lookupRelTarget(mrels, rid)
					if item.BoldItalicTargetPart == "" {
						rep.Diagnostics = append(rep.Diagnostics, diagBrokenFont(master, typeface, "boldItalic", rid))
					}
				}
			}
			rep.EmbeddedFonts = append(rep.EmbeddedFonts, item)
		}
	}
	return nil
}

// sectionVariantRel 在 p:embeddedFont 内查找 p:<name> 变体（regular/bold/
// italic/boldItalic）；要求节点命名空间为 PresentationML，local==name。
func sectionVariantRel(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, name string) (*xmlstore.NodeRecord, bool) {
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == nsPresentationML && c.Local() == name {
			return c, true
		}
	}
	return nil, false
}

func lookupRelTarget(rels []*opc.Relationship, rid string) opc.PartName {
	for _, r := range rels {
		if r.ID == rid && r.Mode == opc.TargetInternal {
			return r.TargetPart
		}
	}
	return ""
}

func diagBrokenFont(master opc.PartName, typeface, variant, rid string) Diagnostic {
	return Diagnostic{
		Code:     "layout.font.broken_ref",
		Severity: SeverityWarning,
		Part:     string(master),
		Message:  "embeddedFont " + typeface + " (" + variant + ") references missing relationship " + rid,
	}
}

// ---------- 讲义母版解析 ----------

// parseHandoutMaster 解析 p:handoutMasterIdLst + 主关系 RelHandoutMaster。
func (p *Presentation) parseHandoutMaster(rep *LayoutReport) error {
	doc, err := p.presentationDoc()
	if err != nil {
		return err
	}
	root := doc.Root()
	if root == nil || root.Namespace != nsPresentationML {
		return nil
	}
	hmLst := childOfKind(doc, root, nsPresentationML, "handoutMasterIdLst", 0)
	if hmLst == nil {
		return nil
	}
	mainRels, _, _ := p.relsOf(p.main)
	for _, cid := range hmLst.Children {
		c := doc.Node(cid)
		if c.Namespace != nsPresentationML || c.Local() != "handoutMasterId" {
			continue
		}
		rid, ok := c.Attr(nsOfficeDocument, "id")
		if !ok {
			rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
				Code:     "layout.handout.broken_ref",
				Severity: SeverityWarning,
				Part:     string(p.main),
				Message:  "handoutMasterId missing r:id",
			})
			continue
		}
		rel := findRel(mainRels, rid)
		if rel == nil || rel.Mode != opc.TargetInternal || rel.Type != opc.RelHandoutMaster {
			rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
				Code:     "layout.handout.broken_ref",
				Severity: SeverityWarning,
				Part:     string(p.main),
				Message:  "handoutMasterId " + rid + " has no internal RelHandoutMaster relationship",
			})
			continue
		}
		info := &HandoutMasterInfo{
			Part:       rel.TargetPart,
			RelationID: rid,
			Present:    p.hasPartCurrent(rel.TargetPart),
		}
		if !info.Present {
			rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
				Code:     "layout.handout.broken_ref",
				Severity: SeverityWarning,
				Part:     string(rel.TargetPart),
				Message:  "handout master part referenced but not present in package",
			})
		}
		rep.HandoutMaster = info
	}
	return nil
}

func findRel(rels []*opc.Relationship, rid string) *opc.Relationship {
	for _, r := range rels {
		if r.ID == rid {
			return r
		}
	}
	return nil
}

// ---------- 避头尾（kinsoku）解析 ----------

// parseMasterKinsoku 扫描母版 p:txStyles 各级样式（titleStyle/bodyStyle/
// otherStyle）的 defRPr / rPr / endParaRPr，聚合 lang/altLang/kumimoji。
func (p *Presentation) parseMasterKinsoku(master opc.PartName, rep *LayoutReport) error {
	doc, err := p.docOf(master)
	if err != nil {
		return nil
	}
	// 在 master 中查找 <p:txStyles>；该节点下含 titleStyle/bodyStyle/otherStyle。
	for _, tid := range doc.Elements(nsPresentationML, "txStyles") {
		ts := doc.Node(tid)
		for _, sid := range ts.Children {
			style := doc.Node(sid)
			if style.Namespace != nsPresentationML {
				continue
			}
			walkKinsokuProbes(doc, style, master, rep)
		}
	}
	return nil
}

// walkKinsokuProbes 递归扫描样式子树（覆盖 defRPr / rPr / endParaRPr /
// lstStyle 多级定义），提取语言与 kumimoji/kinsoku 标记。
func walkKinsokuProbes(doc *xmlstore.XMLDocument, root *xmlstore.NodeRecord, master opc.PartName, rep *LayoutReport) {
	var visit func(n *xmlstore.NodeRecord)
	visit = func(n *xmlstore.NodeRecord) {
		if n == nil {
			return
		}
		switch n.Local() {
		case "defRPr", "rPr", "endParaRPr":
			collectRPrLang(doc, n, master, rep)
		}
		for _, cid := range n.Children {
			visit(doc.Node(cid))
		}
	}
	visit(root)
}

func collectRPrLang(doc *xmlstore.XMLDocument, rpr *xmlstore.NodeRecord, master opc.PartName, rep *LayoutReport) {
	lang, _ := rpr.Attr("", "lang")
	altLang, _ := rpr.Attr("", "altLang")
	kumimojiRaw, hasK := rpr.Attr("", "kumimoji")
	kinsokuRaw, hasKin := rpr.Attr("", "kinsoku")
	if lang == "" && altLang == "" && !hasK && !hasKin {
		return
	}
	// 聚合到或新建 KinsokuRule。
	merged := false
	for i := range rep.Kinsoku {
		if rep.Kinsoku[i].Lang == lang && rep.Kinsoku[i].AltLang == altLang {
			rep.Kinsoku[i].Kumimoji = rep.Kinsoku[i].Kumimoji || isTrue(kumimojiRaw)
			rep.Kinsoku[i].KinsokuFlag = rep.Kinsoku[i].KinsokuFlag || isTrue(kinsokuRaw)
			rep.Kinsoku[i].Parts = appendPartUnique(rep.Kinsoku[i].Parts, master)
			merged = true
			break
		}
	}
	if !merged {
		rep.Kinsoku = append(rep.Kinsoku, KinsokuRule{
			Lang:        lang,
			AltLang:     altLang,
			Kumimoji:    isTrue(kumimojiRaw),
			KinsokuFlag: isTrue(kinsokuRaw),
			Parts:       []opc.PartName{master},
		})
	}
}

func isTrue(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true":
		return true
	}
	return false
}

func appendPartUnique(parts []opc.PartName, p opc.PartName) []opc.PartName {
	for _, q := range parts {
		if q == p {
			return parts
		}
	}
	return append(parts, p)
}
