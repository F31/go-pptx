package pptx

import (
	"strings"
	"unicode/utf8"

	"github.com/F31/go-pptx/internal/textmap"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 TEXT-02 的跨 Run 替换与整批变更（方案 §7.2）：
// Paragraph.ReplaceText 与 TextFrame.ReplaceText。
//
// 确定性的替换规则（首版固定）：
//   - 大小写敏感、字面匹配、从左到右非重叠、基于初始文本快照、
//     不递归处理替换结果；空 old 返回 ErrInvalidArgument。
//   - 逻辑文本视图与 Paragraph.Text() 一致：普通 Run 的 a:t 解码文本
//     按文档序拼接；a:br/a:fld 不贡献文本且构成段内块边界——匹配
//     绝不跨越 br/fld，也不跨越"不同超链接/动作"的相邻 Run
//     （rPr 的 hlinkClick/hlinkMouseOver 目标不同即不可跨越）。
//   - replacement 含换行（U+000A）首版拒绝，返回 ErrUnsupportedEdit；
//     调用方使用显式段落 API（SetPlainText 等）。
//   - 复杂字素簇保护：匹配边界不得切开组合字符或 ZWJ 序列，违规命中
//     被跳过并记诊断（Hit.Reason=grapheme-cluster-boundary）。
//   - 安全与保真约束：重建或删除 Run 前检查——覆盖范围内 Run 含
//     rPr/a:t 之外的有语义直接子元素（如扩展）、或携带超链接/动作时
//     跳过该命中（Reason=unsafe-link-or-extension-run）；相邻命中因
//     共享 Run 而无法独立表达时跳过后出现者（Reason=
//     adjacent-match-run-conflict）。仅原地文本替换（不拆 Run）不受
//     链接约束，链接原样保留。
//   - 空 replacement 合法（删除匹配文本）；命中后 Run 变空则删除该
//     Run（受上述安全约束）。
//
// 三种格式策略（ReplaceMode）：
//   - ReplaceFirstCharacter（默认）：replacement 继承匹配首字符所在
//     Run 的显式字符格式；未命中的前后内容与格式完全保留。
//   - ReplaceEqualLengthPerRune：old 与 replacement 的 rune 数必须
//     一致（否则 ErrInvalidArgument）；逐 Run 继承各自原格式（仅改
//     文本，不拆 Run）。
//   - ReplaceExplicitStyle：调用方以 WithReplacementStyle 提供格式；
//     未提供返回 ErrInvalidArgument。
//
// 整批变更：段内所有命中先按"占用 Run 区间"贪心调度（先到先得，
// 保证补丁区间互不重叠），同一 Run 的多个原地文本变更合并为单次
// t 内容替换，随后一次 ApplyPatches + SinglePartPatch。

// ReplaceMode 是替换片段的格式策略（方案 §7.2 三种格式策略）。
//
// Stable: iota 枚举值（ReplaceFirstCharacter / ReplaceEqualLengthPerRune
// 等）在 v1.0 后锁死——下游 switch/case 完备性依赖此枚举。仅允许追加新
// 枚举值（追加到 iota 末尾），不可重命名或移除已有值。
type ReplaceMode int

const (
	// ReplaceFirstCharacter 默认策略：replacement 继承匹配首字符所在
	// Run 的显式字符格式。
	ReplaceFirstCharacter ReplaceMode = iota
	// ReplaceEqualLengthPerRune 逐位置继承：old 与 replacement 的 rune
	// 数必须一致，各 Run 保留自身格式。
	ReplaceEqualLengthPerRune
	// ReplaceExplicitStyle 调用方提供 replacement 的显式格式。
	ReplaceExplicitStyle
)

func (m ReplaceMode) String() string {
	switch m {
	case ReplaceFirstCharacter:
		return "FirstCharacterStyle"
	case ReplaceEqualLengthPerRune:
		return "EqualLengthPerRune"
	case ReplaceExplicitStyle:
		return "ExplicitStyle"
	}
	return "ReplaceMode(" + intString(int(m)) + ")"
}

// replaceOptions / ReplaceOption / WithReplaceMode / WithReplacementStyle 已迁出至 options.go。

// ReplaceHit 是一次命中的定位与结果。StartRune/EndRune 是逻辑文本
// 视图（= 所属段落 Text() 的 Unicode rune 序列）的半开区间 [Start,End)。
type ReplaceHit struct {
	StartRune int
	EndRune   int
	Replaced  bool
	Reason    string // 跳过原因（Replaced=false 时）
}

// ReplaceResult 是一次 ReplaceText 调用的汇总（方案 §7.2）。
type ReplaceResult struct {
	Matches  int // 初始快照上的匹配总数
	Replaced int // 实际完成替换数
	Skipped  int // 因边界/安全规则跳过的匹配数
	Hits     []ReplaceHit
}

// segRun 是段内一个普通 Run 的解析快照（ReplaceText 匹配基础）。
type segRun struct {
	node   *xmlstore.NodeRecord
	prefix string // run 元素 QName 前缀（缺省 "a"）
	text   string // a:t 解码后的逻辑文本
	link   string // 超链接/动作 key（空 = 无链接）
	rPr    *xmlstore.NodeRecord
	tNode  *xmlstore.NodeRecord
}

// segBlock 是段内可连续匹配的 Run 序列（块间不可跨越）。
// viewStart 是本块首个 Run 在段落逻辑视图（Text() 语义）中的 rune 偏移。
type segBlock struct {
	runs      []segRun
	runes     []rune // 块内 run 文本拼接的 rune 序列
	viewStart int
}

// paragraphBlocks 解析段落为顺序块。
func paragraphBlocks(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) []segBlock {
	var blocks []segBlock
	viewRune := 0
	var cur *segBlock
	flush := func() {
		if cur != nil && len(cur.runs) > 0 {
			var sb strings.Builder
			for i := range cur.runs {
				sb.WriteString(cur.runs[i].text)
			}
			cur.runes = []rune(sb.String())
			blocks = append(blocks, *cur)
		}
		cur = nil
	}
	for _, cid := range para.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "r":
			sr := snapshotRun(doc, c)
			linkChange := cur != nil && cur.runs[len(cur.runs)-1].link != sr.link
			if cur == nil || linkChange {
				flush()
				cur = &segBlock{viewStart: viewRune}
			}
			cur.runs = append(cur.runs, sr)
			viewRune += utf8.RuneCountInString(sr.text)
		case "br", "fld":
			flush() // 换行/字段为硬边界（不贡献文本）
		default:
			// pPr/endParaRPr/extLst 等不参与文本流。
		}
	}
	flush()
	return blocks
}

// snapshotRun 解析一个 a:r 的文本、前缀与链接 key（无 a:t 视作空文本）。
func snapshotRun(doc *xmlstore.XMLDocument, r *xmlstore.NodeRecord) segRun {
	sr := segRun{node: r, prefix: "a"}
	if r.QName.Prefix != "" {
		sr.prefix = r.QName.Prefix
	}
	for _, cid := range r.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "rPr":
			sr.rPr = c
			sr.link = runLinkKey(doc, c)
		case "t":
			sr.tNode = c
			if !c.SelfClosing() {
				sr.text = xmlUnescape(string(doc.Original()[c.OpenEnd:c.CloseStart]))
			}
		}
	}
	return sr
}

// runLinkKey 返回 rPr 中第一个超链接/动作目标的 key；无链接返回空串。
// 相邻 Run key 不同即不可跨越（方案 §7.2 不同超链接动作边界）。
func runLinkKey(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) string {
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		if c.Local() == "hlinkClick" || c.Local() == "hlinkMouseOver" {
			if v, ok := c.Attr(nsOfficeDocument, "id"); ok {
				return "r:" + v
			}
			if v, ok := c.Attr("", "action"); ok {
				return "a:" + v
			}
			return "h" // 无目标参数的独立动作
		}
	}
	return ""
}

// matchOcc 是块内一次命中的记录（rune 区间，初始快照）。
type matchOcc struct {
	blk *segBlock
	gsi int // 块内 rune 起始
	gei int // 块内 rune 结束（开）
	ri  int // 首个命中 run 下标
	rj  int // 末个命中 run 下标
	rsi int // run[ri] 内 rune 起始（局部）
	rej int // run[rj] 内 rune 结束（局部，开）
}

// runChange 是施加到单个 Run 文本上的原地变更（run 局部 rune 区间）。
type runChange struct {
	run  int // blk.runs 下标
	s, e int // run 内 rune 半开区间
	text string
}

// ReplaceText 在本段落内执行确定性替换（方案 §7.2，规则见本文件头
// 注释）。匹配基于初始文本快照、从左到右非重叠；段内全部命中在一次
// 隐式事务中批量提交。
func (p *Paragraph) ReplaceText(old, replacement string, opts ...ReplaceOption) (ReplaceResult, error) {
	var res ReplaceResult
	o := replaceOptions{mode: ReplaceFirstCharacter}
	for _, fn := range opts {
		fn(&o)
	}
	if old == "" {
		return res, Annotate(ErrInvalidArgument, "Paragraph.ReplaceText")
	}
	if strings.ContainsRune(replacement, '\n') {
		return res, &OperationError{
			Op:      "Paragraph.ReplaceText",
			Message: "replacement containing a line feed is not supported in this version; use explicit paragraph APIs",
			Err:     ErrUnsupportedEdit,
		}
	}
	if _, err := xmlstore.EscapeText(replacement); err != nil {
		return res, Annotate(mapXMLError(err), "Paragraph.ReplaceText")
	}
	if o.mode == ReplaceExplicitStyle && !o.styleSet {
		return res, &OperationError{
			Op:      "Paragraph.ReplaceText",
			Message: "ReplaceExplicitStyle requires WithReplacementStyle",
			Err:     ErrInvalidArgument,
		}
	}
	repRunes := []rune(replacement)
	oldRunes := []rune(old)
	if o.mode == ReplaceEqualLengthPerRune && len(oldRunes) != len(repRunes) {
		return res, &OperationError{
			Op:      "Paragraph.ReplaceText",
			Message: "ReplaceEqualLengthPerRune requires old and replacement to have equal rune counts",
			Err:     ErrInvalidArgument,
		}
	}

	doc, para, err := p.locatePara()
	if err != nil {
		return res, Annotate(err, "Paragraph.ReplaceText")
	}
	res, patches, err := replacePatches(doc, para, old, replacement, o)
	if err != nil {
		return res, Annotate(err, "Paragraph.ReplaceText")
	}
	if len(patches) == 0 {
		return res, nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return res, Annotate(mapXMLError(err), "Paragraph.ReplaceText")
	}
	if err := applySinglePartPatch(p.p, p.part, out); err != nil {
		return res, Annotate(err, "Paragraph.ReplaceText")
	}
	return res, nil
}

// replacePatches 在给定段落上计算替换补丁，不做暂存与提交——供
// Paragraph.ReplaceText（单段落事务）与 TPL-01 绑定引擎（按 Part
// 聚合多段落补丁、单事务提交）共用同一条保真替换路径（ADR 013：
// 不引入第二套编辑路径）。
//
// 返回的补丁均相对 doc.Original() 定位，调用方须在同一 revision 快照
// 上一次性 ApplyPatches。
func replacePatches(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord, old, replacement string, o replaceOptions) (ReplaceResult, []xmlstore.SpanPatch, error) {
	var res ReplaceResult
	repRunes := []rune(replacement)
	oldRunes := []rune(old)
	blocks := paragraphBlocks(doc, para)

	// 1) 初始快照上收集全部命中（非重叠、从左到右），做块内定位与
	//    字素簇边界检查；跳过项直接记入结果。
	var occs []matchOcc
	for b := range blocks {
		blk := &blocks[b]
		from := 0
		for {
			rel := textmap.IndexRunes(blk.runes[from:], oldRunes)
			if rel < 0 {
				break
			}
			gsi := from + rel
			gei := gsi + len(oldRunes)
			res.Matches++
			var occ matchOcc
			occ.blk, occ.gsi, occ.gei = blk, gsi, gei
			if !textmap.GraphemeSafe(blk.runes, gsi, gei) {
				res.Skipped++
				res.Hits = append(res.Hits, ReplaceHit{
					StartRune: blk.viewStart + gsi, EndRune: blk.viewStart + gei,
					Replaced: false, Reason: "grapheme-cluster-boundary",
				})
			} else if !locateBlockSpan(blk, gsi, gei, &occ) {
				res.Skipped++
				res.Hits = append(res.Hits, ReplaceHit{
					StartRune: blk.viewStart + gsi, EndRune: blk.viewStart + gei,
					Replaced: false, Reason: "unsupported-run-content",
				})
			} else {
				occs = append(occs, occ)
			}
			from = gei
		}
	}
	if len(occs) == 0 {
		return res, nil, nil
	}

	// 2) 贪心调度（前向、先到先得）：rebuild 命中占用 [ri..rj] 整段
	//    Run；inplace 命中只改所及 Run 的 t 内容（同 Run 多命中合并）。
	//    冲突、unsafe 与成功执行在此全部落账，保证 Hits 前向有序且
	//    计数一致。
	type runSpan struct{ lo, hi int } // 闭区间
	var rebuilt []runSpan             // 互不相交、升序
	var rebuildExec []matchOcc        // 实际执行的 rebuild 命中（前向序）
	inplaceByRun := map[int][]runChange{}
	inplaceMark := map[int]bool{}
	overlapsRebuilt := func(lo, hi int) bool {
		for _, s := range rebuilt {
			if lo <= s.hi && hi >= s.lo {
				return true
			}
		}
		return false
	}
	record := func(occ matchOcc, ok bool, reason string) {
		hit := ReplaceHit{
			StartRune: occ.blk.viewStart + occ.gsi,
			EndRune:   occ.blk.viewStart + occ.gei,
			Replaced:  ok,
			Reason:    reason,
		}
		res.Hits = append(res.Hits, hit)
		if ok {
			res.Replaced++
		} else {
			res.Skipped++
		}
	}

	for _, occ := range occs {
		ri, rj := occ.ri, occ.rj
		if occ.isRebuild(o.mode, repRunes) {
			if !runsSafelyRebuildable(doc, occ.blk, ri, rj) {
				record(occ, false, "unsafe-link-or-extension-run")
				continue
			}
			if overlapsRebuilt(ri, rj) {
				record(occ, false, "adjacent-match-run-conflict")
				continue
			}
			conflict := false
			for k := ri; k <= rj; k++ {
				if inplaceMark[k] {
					conflict = true
					break
				}
			}
			if conflict {
				record(occ, false, "adjacent-match-run-conflict")
				continue
			}
			rebuilt = append(rebuilt, runSpan{ri, rj})
			rebuildExec = append(rebuildExec, occ)
			record(occ, true, "")
			continue
		}
		// inplace：目标 Run 均未被 rebuild 占用即合并变更。
		conflict := false
		for k := ri; k <= rj; k++ {
			if overlapsRebuilt(k, k) {
				conflict = true
				break
			}
		}
		if conflict {
			record(occ, false, "adjacent-match-run-conflict")
			continue
		}
		splitInplace(occ, o.mode, repRunes, func(c runChange) {
			inplaceByRun[c.run] = append(inplaceByRun[c.run], c)
			inplaceMark[c.run] = true
		})
		record(occ, true, "")
	}

	// 3) 构造补丁：rebuild 从后向前；inplace 逐 run 合并为单次 t 内容
	//    替换。区间互不重叠，统一 ApplyPatches + 单次提交。
	var patches []xmlstore.SpanPatch
	for k := len(rebuildExec) - 1; k >= 0; k-- {
		occ := rebuildExec[k]
		repRPr := ""
		if o.mode == ReplaceExplicitStyle {
			if o.style.anySet() {
				frag, err := buildRPrFragment(occ.blk.runs[occ.ri].prefix, o.style)
				if err != nil {
					return res, nil, Annotate(err, "replacePatches")
				}
				repRPr = frag
			}
		} else {
			repRPr = rPrBytes(doc, occ.blk.runs[occ.ri].rPr)
		}
		patches = append(patches, rebuildSpan(doc, occ.blk, occ, repRPr, repRunes))
	}
	for r, changes := range inplaceByRun {
		patches = append(patches, inplaceRunPatch(doc, occs[0].blk, r, changes))
	}
	return res, patches, nil
}

// isRebuild 判定命中是否需要重建/删除 Run（true）或仅原地改 t。
func (o *matchOcc) isRebuild(mode ReplaceMode, rep []rune) bool {
	if mode == ReplaceEqualLengthPerRune {
		return false // 等长逐 Run 文本替换，永不拆 Run
	}
	if o.ri != o.rj {
		return true // 跨 Run：FirstCharacter/Explicit 均重建
	}
	if mode == ReplaceExplicitStyle {
		return true // replacement 需独立显式格式
	}
	// FirstCharacter 单 Run：替换后 Run 文本为空才需删除（重建）。
	rr := []rune(o.blk.runs[o.ri].text)
	hasText := o.rsi > 0 || len(rep) > 0 || o.rej < len(rr)
	return !hasText
}

// splitInplace 把 inplace 命中展开为逐 run 的文本变更。
func splitInplace(occ matchOcc, mode ReplaceMode, rep []rune, emit func(runChange)) {
	if mode == ReplaceEqualLengthPerRune {
		// 把 replacement 按各 Run 命中 rune 数切分。
		pos := 0
		for i := 0; i < occ.ri; i++ {
			pos += utf8.RuneCountInString(occ.blk.runs[i].text)
		}
		for i := occ.ri; i <= occ.rj; i++ {
			rr := []rune(occ.blk.runs[i].text)
			lo, hi := pos, pos+len(rr)
			pos = hi
			s := maxInt(occ.gsi-lo, 0)
			e := minInt(occ.gei-lo, len(rr))
			if s >= e || occ.blk.runs[i].tNode == nil {
				continue
			}
			n := e - s
			emit(runChange{run: i, s: s, e: e, text: string(rep[:n])})
			rep = rep[n:]
		}
		return
	}
	// FirstCharacter 单 Run（isRebuild=false 已保证替换后非空）。
	emit(runChange{run: occ.ri, s: occ.rsi, e: occ.rej, text: string(rep)})
}

// inplaceRunPatch 合并 run 的所有文本变更并生成单次 t 内容替换。
// 变更区间互不重叠（来自非重叠命中）。
func inplaceRunPatch(doc *xmlstore.XMLDocument, blk *segBlock, r int, changes []runChange) xmlstore.SpanPatch {
	run := blk.runs[r]
	rr := []rune(run.text)
	// 降序应用（区间不重叠）。
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		text := string(rr[:c.s]) + c.text + string(rr[c.e:])
		rr = []rune(text)
	}
	esc, err := xmlstore.EscapeText(string(rr))
	if err != nil {
		esc = string(rr) // 防御；正常不可达
	}
	return xmlstore.SpanPatch{
		Start:       run.tNode.OpenEnd,
		End:         run.tNode.CloseStart,
		Replacement: []byte(esc),
	}
}

// runsSafelyRebuildable 检查 [ri..rj] 内每个 Run 均可被删除/重建：
// 直接子元素仅限 rPr/a:t，且不携带超链接/动作。
func runsSafelyRebuildable(doc *xmlstore.XMLDocument, blk *segBlock, ri, rj int) bool {
	for i := ri; i <= rj; i++ {
		run := blk.runs[i]
		if run.link != "" {
			return false
		}
		for _, cid := range run.node.Children {
			c := doc.Node(cid)
			if c.Namespace != nsDrawingML {
				return false
			}
			if c.Local() != "rPr" && c.Local() != "t" {
				return false
			}
		}
	}
	return true
}

// rebuildSpan 用"前缀 Run + 替换 Run + 后缀 Run"重建被覆盖区间
// [run[ri] 起始, run[rj] 结束)；三者按需存在（空片段省略）。
func rebuildSpan(doc *xmlstore.XMLDocument, blk *segBlock, occ matchOcc, repRPr string, rep []rune) xmlstore.SpanPatch {
	first, last := blk.runs[occ.ri], blk.runs[occ.rj]
	fr := []rune(first.text)
	lr := []rune(last.text)
	prefix := string(fr[:occ.rsi])
	suffix := string(lr[occ.rej:])
	var sb strings.Builder
	if prefix != "" {
		sb.WriteString(buildRunXML(first.prefix, rPrBytes(doc, first.rPr), prefix))
	}
	if len(rep) > 0 {
		sb.WriteString(buildRunXML(first.prefix, repRPr, string(rep)))
	}
	if suffix != "" {
		sb.WriteString(buildRunXML(last.prefix, rPrBytes(doc, last.rPr), suffix))
	}
	return xmlstore.SpanPatch{
		Start:       first.node.Source.Start,
		End:         last.node.Source.End,
		Replacement: []byte(sb.String()),
	}
}

// buildRunXML 构造 <pfx:r>[rPrXML]<pfx:t>text</pfx:t></pfx:r>；
// rPrXML 为空则不输出 rPr。
func buildRunXML(prefix, rPrXML, text string) string {
	esc, err := xmlstore.EscapeText(text)
	if err != nil {
		esc = text // 防御；正常不可达（文本源合法）
	}
	var sb strings.Builder
	sb.WriteString("<" + prefix + ":r>")
	if rPrXML != "" {
		sb.WriteString(rPrXML)
	}
	sb.WriteString("<" + prefix + ":t>")
	sb.WriteString(esc)
	sb.WriteString("</" + prefix + ":t></" + prefix + ":r>")
	return sb.String()
}

// rPrBytes 取 rPr 节点的原始字节；rPr 为 nil 时返回空串。
func rPrBytes(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) string {
	if rPr == nil {
		return ""
	}
	return string(doc.Original()[rPr.Source.Start:rPr.Source.End])
}

// locateBlockSpan 把块内 rune 区间映射到命中的首/末 Run 及各自内部
// rune 偏移；命中完全落在块文本内时返回 true。
func locateBlockSpan(blk *segBlock, gsi, gei int, occ *matchOcc) bool {
	texts := make([]string, len(blk.runs))
	for i := range blk.runs {
		texts[i] = blk.runs[i].text
	}
	span, ok := textmap.LocateSpan(texts, gsi, gei)
	if !ok {
		return false
	}
	occ.ri, occ.rj = span.RunStart, span.RunEnd
	occ.rsi, occ.rej = span.StartInRun, span.EndInRun
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TextFrame.ReplaceText 遍历正文各段落执行 ReplaceText（段落间不匹配，
// 方案 §7.2 不跨段落）。Hits 的 rune 区间是各自所在段落的局部视图。
func (t *TextFrame) ReplaceText(old, replacement string, opts ...ReplaceOption) (ReplaceResult, error) {
	var res ReplaceResult
	paras, err := t.Paragraphs()
	if err != nil {
		return res, Annotate(err, "TextFrame.ReplaceText")
	}
	for _, para := range paras {
		r, err := para.ReplaceText(old, replacement, opts...)
		if err != nil {
			return res, Annotate(err, "TextFrame.ReplaceText")
		}
		res.Matches += r.Matches
		res.Replaced += r.Replaced
		res.Skipped += r.Skipped
		res.Hits = append(res.Hits, r.Hits...)
	}
	return res, nil
}
