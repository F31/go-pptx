package pptx

import (
	"errors"
	"strings"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 TEXT-01 的富文本模型（方案 §7.1/§20.2）：
// TextFrame → Paragraph → TextRun 三级受控句柄。
//
// 句柄不持有资源也不缓存 NodeID（xmlstore 节点 ID 在索引重建后变化）；
// 每次操作以"原始字节 → 当前 revision 索引"重定位句柄内记录的稳定
// 元素路径（从根元素逐层按 [ns,local,序号] 下降）。路径上的祖先或
// 目标被删除时返回 ErrStaleHandle；文档关闭返回 ErrClosed。
//
// 读侧语义（§7.1）：段落属性（a:pPr）与结束字符属性（a:endParaRPr）
// 在模型中保留，不扁平化成 []string；Text() 仅拼接普通 Run 的 a:t
// 文本，a:br/a:fld 等内联节点的展示规则随 TEXT-02 textmap 落地。

// nodeStep 是从根元素到目标元素路径上的一步：[ns,local] 匹配父元素
// 的第 nth 个同名单元素子节点（0 基序号）。
type nodeStep struct {
	ns    string
	local string
	nth   int
}

// textNode 是文本模型句柄的公共载体。
type textNode struct {
	p    *Presentation
	part opc.PartName
	path []nodeStep
}

// locate 解析句柄路径，返回当前 revision 索引与目标元素。NotFound
// 语义统一映射为 ErrStaleHandle（Part 或路径目标已不存在）。
func (h *textNode) locate() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	if h.p == nil || h.p.closed {
		return nil, nil, Annotate(ErrClosed, "textNode")
	}
	doc, err := h.p.docOf(h.part)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, Annotate(ErrStaleHandle, "textNode")
		}
		return nil, nil, err
	}
	n := resolvePath(doc, h.path)
	if n == nil {
		return nil, nil, Annotate(ErrStaleHandle, "textNode")
	}
	return doc, n, nil
}

// resolvePath 按路径步骤从根元素下降；任一步无匹配返回 nil。
func resolvePath(doc *xmlstore.XMLDocument, steps []nodeStep) *xmlstore.NodeRecord {
	cur := doc.Root()
	if cur == nil {
		return nil
	}
	for _, st := range steps {
		found := -1
		seen := 0
		for _, cid := range cur.Children {
			c := doc.Node(cid)
			if c.Namespace == st.ns && c.Local() == st.local {
				if seen == st.nth {
					found = int(cid)
					break
				}
				seen++
			}
		}
		if found < 0 {
			return nil
		}
		cur = doc.Node(xmlstore.NodeID(found))
	}
	return cur
}

// recordPath 把 doc 中元素 id 的祖先链录为路径（根元素本身返回空路径）。
func recordPath(doc *xmlstore.XMLDocument, id xmlstore.NodeID) []nodeStep {
	var rev []nodeStep
	cur := doc.Node(id)
	if cur == nil || cur.Parent == xmlstore.NoNode {
		return nil
	}
	for cur != nil && cur.Parent != xmlstore.NoNode {
		parent := doc.Node(cur.Parent)
		nth := 0
		for _, cid := range parent.Children {
			if cid == cur.ID {
				break
			}
			c := doc.Node(cid)
			if c.Namespace == cur.Namespace && c.Local() == cur.Local() {
				nth++
			}
		}
		rev = append(rev, nodeStep{ns: cur.Namespace, local: cur.Local(), nth: nth})
		cur = parent
	}
	// 反转成根→叶。
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// childOfKind 返回 parent 下第 nth 个（0 基）ns/local 匹配的子元素。
func childOfKind(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, ns, local string, nth int) *xmlstore.NodeRecord {
	seen := 0
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == ns && c.Local() == local {
			if seen == nth {
				return c
			}
			seen++
		}
	}
	return nil
}

// countKind 统计 parent 下 ns/local 匹配的子元素数。
func countKind(doc *xmlstore.XMLDocument, parent *xmlstore.NodeRecord, ns, local string) int {
	n := 0
	for _, cid := range parent.Children {
		c := doc.Node(cid)
		if c.Namespace == ns && c.Local() == local {
			n++
		}
	}
	return n
}

// kindIndex 返回 child 在其 parent 同名单元素兄弟中的序号（0 基）。
func kindIndex(doc *xmlstore.XMLDocument, child *xmlstore.NodeRecord) int {
	if child.Parent == xmlstore.NoNode {
		return 0
	}
	parent := doc.Node(child.Parent)
	n := 0
	for _, cid := range parent.Children {
		if cid == child.ID {
			return n
		}
		c := doc.Node(cid)
		if c.Namespace == child.Namespace && c.Local() == child.Local() {
			n++
		}
	}
	return n
}

// ---------- TextFrame ----------

// TextFrame 是文本框正文（p:txBody）的受控句柄。方法均以一次隐式事务
// 提交；读取基于当前 revision 索引。
type TextFrame struct {
	textNode
}

// Paragraphs 返回正文中的段落句柄切片（文档序；仅 a:p，忽略属性等
// 非段落内容）。
func (t *TextFrame) Paragraphs() ([]*Paragraph, error) {
	doc, body, err := t.locate()
	if err != nil {
		return nil, Annotate(err, "TextFrame.Paragraphs")
	}
	n := countKind(doc, body, nsDrawingML, "p")
	out := make([]*Paragraph, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &Paragraph{
			textNode: textNode{p: t.p, part: t.part, path: t.path},
			idx:      i,
		})
	}
	return out, nil
}

// SetPlainText 是明确的结构替换（方案 §20.2）：以 '\n' 分行生成段落，
// 移除被替换正文内的字符格式、字段与链接，但保留文本框级属性
// （a:bodyPr/a:lstStyle）。若 txBody 内存在不能安全删除的扩展元素，
// 返回 ErrUnsupportedEdit 且不产生部分修改。
func (t *TextFrame) SetPlainText(text string) error {
	doc, body, err := t.locate()
	if err != nil {
		return Annotate(err, "TextFrame.SetPlainText")
	}
	if !body.SelfClosing() && !bodyRawShapeOK(doc, body) {
		return &OperationError{
			Op: "TextFrame.SetPlainText", Part: string(t.part),
			Message: "txBody contains extensions that SetPlainText cannot safely delete",
			Err:     ErrUnsupportedEdit,
		}
	}
	prefix := paraPrefix(doc, body)

	var kept, ps []string
	for _, cid := range body.Children {
		c := doc.Node(cid)
		if c.Local() == "bodyPr" || c.Local() == "lstStyle" {
			kept = append(kept, string(doc.Slice(c.Source)))
		}
	}
	lines := strings.Split(text, "\n")
	for _, ln := range lines {
		ps = append(ps, buildPlainParagraph(prefix, ln))
	}
	if len(ps) == 0 {
		ps = append(ps, "<"+prefix+":p/>")
	}
	start, end := body.OpenEnd, body.CloseStart
	if body.SelfClosing() {
		// 自闭合 txBody 无可保留属性，也不会有 p；整体展开。
		start, end = body.Source.Start, body.Source.End
		kept = nil
	}
	repl := strings.Join(kept, "") + strings.Join(ps, "")
	patch := xmlstore.SpanPatch{Start: start, End: end, Replacement: []byte(repl)}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return Annotate(mapXMLError(err), "TextFrame.SetPlainText")
	}
	if err := t.p.stagePatch(t.part, out); err != nil {
		return Annotate(err, "TextFrame.SetPlainText")
	}
	t.p.commit()
	return nil
}

// bodyRawShapeOK 检查 txBody 的子元素只属于 {a:bodyPr, a:lstStyle, a:p}
// （未知命名空间/未知本地名的元素视为不可安全删除的扩展）。
func bodyRawShapeOK(doc *xmlstore.XMLDocument, body *xmlstore.NodeRecord) bool {
	for _, cid := range body.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML &&
			(c.Local() == "bodyPr" || c.Local() == "lstStyle" || c.Local() == "p") {
			continue
		}
		return false
	}
	return true
}

// paraPrefix 返回正文段落应使用的前缀（取首个 a:p 的前缀；txBody 至少
// 含一个 a:p 才合法，缺失时回退 "a" 依赖作用域）。
func paraPrefix(doc *xmlstore.XMLDocument, body *xmlstore.NodeRecord) string {
	for _, cid := range body.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML && c.Local() == "p" && c.QName.Prefix != "" {
			return c.QName.Prefix
		}
	}
	return "a"
}

// buildPlainParagraph 生成无字符格式的段落：<p:…><a:r><a:t>…</a:t></a:r></p:…>。
func buildPlainParagraph(prefix, text string) string {
	esc, err := xmlstore.EscapeText(text)
	if err != nil {
		return "" // 非法 XML 字符由上层 EscapeText 在 SetPlainText 内统一拒绝
	}
	return "<" + prefix + ":p><" + prefix + ":r><" + prefix + ":t>" + esc + "</" + prefix + ":t></" + prefix + ":r></" + prefix + ":p>"
}

// ---------- Paragraph ----------

// Paragraph 是段落（a:p）的受控句柄，保留段落属性（a:pPr）与结束字符
// 属性（a:endParaRPr）：本模型不把它们扁平化。
type Paragraph struct {
	textNode
	idx int // 在所属 txBody 的 a:p 兄弟中的序号
}

func (p *Paragraph) locatePara() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, body, err := p.locate()
	if err != nil {
		return nil, nil, err
	}
	para := childOfKind(doc, body, nsDrawingML, "p", p.idx)
	if para == nil {
		return nil, nil, Annotate(ErrStaleHandle, "Paragraph")
	}
	return doc, para, nil
}

// Text 返回本段落普通 Run 文本的拼接（a:r/a:t 按文档序）；a:br/a:fld
// 暂不贡献内容（固定规则：仅普通 Run 计入；展示字段规则随 TEXT-02）。
func (p *Paragraph) Text() (string, error) {
	doc, para, err := p.locatePara()
	if err != nil {
		return "", Annotate(err, "Paragraph.Text")
	}
	var sb strings.Builder
	for _, cid := range para.Children {
		r := doc.Node(cid)
		if r.Namespace != nsDrawingML || r.Local() != "r" {
			continue
		}
		for _, tid := range r.Children {
			tt := doc.Node(tid)
			if tt.Namespace == nsDrawingML && tt.Local() == "t" {
				raw := ""
				if !tt.SelfClosing() {
					raw = string(doc.Original()[tt.OpenEnd:tt.CloseStart])
				}
				sb.WriteString(xmlUnescape(raw))
			}
		}
	}
	return sb.String(), nil
}

// Runs 返回段落内普通 Run（a:r）句柄切片（文档序；br/fld 等非普通
// Run 不在此列，属 TEXT-02 的内联节点模型）。
func (p *Paragraph) Runs() ([]*TextRun, error) {
	doc, para, err := p.locatePara()
	if err != nil {
		return nil, Annotate(err, "Paragraph.Runs")
	}
	n := countKind(doc, para, nsDrawingML, "r")
	out := make([]*TextRun, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &TextRun{
			textNode: textNode{p: p.p, part: p.part, path: p.path},
			paraIdx:  p.idx,
			runIdx:   i,
		})
	}
	return out, nil
}

// AddRun 在段落末尾（a:endParaRPr 之前，若存在）追加一个普通 Run，
// 应用 style 中 Set=true 的字段。返回新 Run 句柄。
func (p *Paragraph) AddRun(text string, style FontStyle) (*TextRun, error) {
	doc, para, err := p.locatePara()
	if err != nil {
		return nil, Annotate(err, "Paragraph.AddRun")
	}
	prefix := runPrefix(doc, para)
	rPr := ""
	if style.anySet() {
		frag, err := buildRPrFragment(prefix, style)
		if err != nil {
			return nil, Annotate(err, "Paragraph.AddRun")
		}
		rPr = frag
	}
	esc, err := xmlstore.EscapeText(text)
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Paragraph.AddRun")
	}
	run := "<" + prefix + ":r>" + rPr + "<" + prefix + ":t>" + esc + "</" + prefix + ":t></" + prefix + ":r>"

	// 插入位置：endParaRPr 之前；否则段落末尾（AppendChild）。
	var patch xmlstore.SpanPatch
	anchor, endIdx := p.endParaAnchor(doc, para)
	if anchor != nil {
		patch, err = xmlstore.InsertBefore(anchor, []byte(run))
	} else {
		_ = endIdx
		patch, err = xmlstore.AppendChild(para, []byte(run))
	}
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Paragraph.AddRun")
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return nil, Annotate(mapXMLError(err), "Paragraph.AddRun")
	}
	if err := p.p.stagePatch(p.part, out); err != nil {
		return nil, Annotate(err, "Paragraph.AddRun")
	}
	p.p.commit()
	return p.lastRun(), nil
}

// endParaAnchor 返回段落末的 a:endParaRPr 元素（存在时），供插入定位。
func (p *Paragraph) endParaAnchor(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) (*xmlstore.NodeRecord, int) {
	for i := len(para.Children) - 1; i >= 0; i-- {
		c := doc.Node(para.Children[i])
		if c.Namespace == nsDrawingML && c.Local() == "endParaRPr" {
			return c, i
		}
	}
	return nil, -1
}

// lastRun 在提交后重新解析段落并返回最后一个 a:r 的句柄。
func (p *Paragraph) lastRun() *TextRun {
	doc, para, err := p.locatePara()
	if err != nil {
		return &TextRun{textNode: p.textNode, paraIdx: p.idx, runIdx: -1}
	}
	n := countKind(doc, para, nsDrawingML, "r")
	return &TextRun{textNode: p.textNode, paraIdx: p.idx, runIdx: n - 1}
}

// runPrefix 返回段落内 Run 应使用的前缀（取首个 a:r 前缀，缺省 "a"）。
func runPrefix(doc *xmlstore.XMLDocument, para *xmlstore.NodeRecord) string {
	for _, cid := range para.Children {
		c := doc.Node(cid)
		if c.Namespace == nsDrawingML && c.Local() == "r" && c.QName.Prefix != "" {
			return c.QName.Prefix
		}
	}
	return "a"
}

// ---------- TextRun ----------

// TextRun 是普通 Run（a:r）的受控句柄。
type TextRun struct {
	textNode
	paraIdx int
	runIdx  int // 在所属段落 a:r 兄弟中的序号（-1 表示无效句柄）
}

func (r *TextRun) locateRun() (*xmlstore.XMLDocument, *xmlstore.NodeRecord, error) {
	doc, para, err := (&Paragraph{textNode: r.textNode, idx: r.paraIdx}).locatePara()
	if err != nil {
		return nil, nil, err
	}
	run := childOfKind(doc, para, nsDrawingML, "r", r.runIdx)
	if run == nil || r.runIdx < 0 {
		return nil, nil, Annotate(ErrStaleHandle, "TextRun")
	}
	return doc, run, nil
}

// Text 返回 Run 的文本（a:t 内容解码后的 Unicode）。
func (r *TextRun) Text() (string, error) {
	doc, run, err := r.locateRun()
	if err != nil {
		return "", Annotate(err, "TextRun.Text")
	}
	for _, cid := range run.Children {
		tt := doc.Node(cid)
		if tt.Namespace == nsDrawingML && tt.Local() == "t" {
			if tt.SelfClosing() {
				return "", nil
			}
			return xmlUnescape(string(doc.Original()[tt.OpenEnd:tt.CloseStart])), nil
		}
	}
	return "", nil
}

// SetText 替换本 Run 的文本（a:t 内容，逐字符 XML 转义）。a:t 缺失时
// 在 Run 内追加（空 Run <a:r/> 或仅含 rPr 的 Run）。不影响其它 Run。
func (r *TextRun) SetText(text string) error {
	doc, run, err := r.locateRun()
	if err != nil {
		return Annotate(err, "TextRun.SetText")
	}
	esc, err := xmlstore.EscapeText(text)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.SetText")
	}
	var patch xmlstore.SpanPatch
	tNode := childOfKind(doc, run, nsDrawingML, "t", 0)
	if tNode != nil {
		patch = xmlstore.SpanPatch{Start: tNode.OpenEnd, End: tNode.CloseStart, Replacement: []byte(esc)}
	} else {
		pfx := run.QName.Prefix
		if pfx == "" {
			pfx = "a"
		}
		frag := "<" + pfx + ":t>" + esc + "</" + pfx + ":t>"
		ip, err := xmlstore.AppendChild(run, []byte(frag))
		if err != nil {
			return Annotate(mapXMLError(err), "TextRun.SetText")
		}
		patch = ip
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), []xmlstore.SpanPatch{patch})
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.SetText")
	}
	if err := r.p.stagePatch(r.part, out); err != nil {
		return Annotate(err, "TextRun.SetText")
	}
	r.p.commit()
	return nil
}

// ExplicitFont 返回 Run 的本地字符格式（a:rPr 中显式出现的属性）；
// 未出现的属性 Set=false（样式链解析属 STYLE-01）。
func (r *TextRun) ExplicitFont() (FontStyle, error) {
	doc, run, err := r.locateRun()
	if err != nil {
		return FontStyle{}, Annotate(err, "TextRun.ExplicitFont")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return FontStyle{}, nil
	}
	return parseLocalFont(doc, rPr), nil
}

// SetFont 以 patch 语义应用 style（方案 §20.2）：仅 Set=true 的字段被
// 写入，其余不变；恢复继承请用 ResetFontProperty。
func (r *TextRun) SetFont(style FontStyle) error {
	if !style.anySet() {
		return nil
	}
	doc, run, err := r.locateRun()
	if err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	prefix := run.QName.Prefix
	if prefix == "" {
		prefix = "a"
	}
	patches, err := r.buildFontPatches(doc, run, prefix, style)
	if err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	if len(patches) == 0 {
		return nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.SetFont")
	}
	if err := r.p.stagePatch(r.part, out); err != nil {
		return Annotate(err, "TextRun.SetFont")
	}
	r.p.commit()
	return nil
}

// ResetFontProperty 移除本地格式中的单个属性族并恢复继承；若 rPr 因
// 此变为空元素则一并移除 rPr。未知属性不受影响。
func (r *TextRun) ResetFontProperty(prop FontProperty) error {
	doc, run, err := r.locateRun()
	if err != nil {
		return Annotate(err, "TextRun.ResetFontProperty")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return nil // 无本地格式，恢复继承是 no-op
	}
	attrNames := map[FontProperty]string{}
	childNames := map[FontProperty]string{
		FontPropColor:         "solidFill",
		FontPropLatin:         "latin",
		FontPropEastAsian:     "ea",
		FontPropComplexScript: "cs",
	}
	attrs := map[FontProperty]string{FontPropBold: "b", FontPropItalic: "i", FontPropSize: "sz"}
	attrName, isAttr := attrs[prop]
	childName, isChild := childNames[prop]
	_ = attrNames

	var patches []xmlstore.SpanPatch
	if isAttr {
		for i := range rPr.Attrs {
			a := &rPr.Attrs[i]
			if a.Namespace == "" && a.RawName == attrName {
				patches = append(patches, removeAttrPatch(doc, rPr, i))
			}
		}
	}
	if isChild {
		for _, cid := range rPr.Children {
			c := doc.Node(cid)
			if c.Namespace == nsDrawingML && c.Local() == childName {
				patches = append(patches, removeElementPatch(doc, c))
			}
		}
	}
	if len(patches) == 0 {
		return nil
	}
	out, err := xmlstore.ApplyPatches(doc.Original(), patches)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.ResetFontProperty")
	}
	// rPr 若已无属性无子元素则整体删除（连同注释外），由 removeEmptyRPr 处理。
	out, err = r.removeEmptyRPr(out)
	if err != nil {
		return Annotate(mapXMLError(err), "TextRun.ResetFontProperty")
	}
	if err := r.p.stagePatch(r.part, out); err != nil {
		return Annotate(err, "TextRun.ResetFontProperty")
	}
	r.p.commit()
	return nil
}

// removeEmptyRPr 重新解析 out；若 run 的 rPr 已无属性与子元素则移除之
// （连同前导空白）。结构编辑不影响路径解析（路径按元素序）。
func (r *TextRun) removeEmptyRPr(out []byte) ([]byte, error) {
	doc, err := xmlstore.Index(out)
	if err != nil {
		return nil, err
	}
	runNode := resolvePath(doc, r.pathToRun())
	if runNode == nil {
		return out, nil
	}
	rPr := childOfKind(doc, runNode, nsDrawingML, "rPr", 0)
	if rPr == nil || len(rPr.Attrs) > 0 || len(rPr.Children) > 0 {
		return out, nil
	}
	patches := []xmlstore.SpanPatch{removeElementPatch(doc, rPr)}
	return xmlstore.ApplyPatches(out, patches)
}

// pathToRun 返回从根到本 run 的完整路径：句柄路径（到 txBody）+ p 序号
// + r 序号。removeEmptyRPr 在补丁后的重解析文档上以此稳定定位 run。
func (r *TextRun) pathToRun() []nodeStep {
	steps := make([]nodeStep, 0, len(r.path)+2)
	steps = append(steps, r.path...)
	steps = append(steps,
		nodeStep{ns: nsDrawingML, local: "p", nth: r.paraIdx},
		nodeStep{ns: nsDrawingML, local: "r", nth: r.runIdx},
	)
	return steps
}

// buildFontPatches 计算应用 style 到 run 的补丁集（不修改 doc）。
func (r *TextRun) buildFontPatches(doc *xmlstore.XMLDocument, run *xmlstore.NodeRecord, prefix string, style FontStyle) ([]xmlstore.SpanPatch, error) {
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		// 需要新建 rPr（属性或子元素），插入 run 内容开头。
		frag, err := buildRPrFragment(prefix, style)
		if err != nil {
			return nil, err
		}
		anchor := firstChildOf(doc, run)
		if anchor != nil {
			p, err := xmlstore.InsertBefore(anchor, []byte(frag))
			return []xmlstore.SpanPatch{p}, err
		}
		p, err := xmlstore.AppendChild(run, []byte(frag))
		return []xmlstore.SpanPatch{p}, err
	}
	return patchExistingRPr(doc, run, rPr, prefix, style)
}

// patchExistingRPr 在既有 rPr 上应用 style（属性就地改、子元素增删换）。
func patchExistingRPr(doc *xmlstore.XMLDocument, run, rPr *xmlstore.NodeRecord, prefix string, style FontStyle) ([]xmlstore.SpanPatch, error) {
	// 自闭合 rPr（仅属性）：整体重建为带子元素的展开形态，避免属性补丁
	// 与内容补丁在 '/>' 处重叠冲突。
	if rPr.SelfClosing() {
		expanded, err := expandSelfClosingRPr(doc, rPr, prefix, style)
		if err != nil {
			return nil, err
		}
		return []xmlstore.SpanPatch{{
			Start: rPr.Source.Start, End: rPr.Source.End, Replacement: []byte(expanded),
		}}, nil
	}
	var patches []xmlstore.SpanPatch

	// 属性类：b/i/sz。
	if style.Bold.Set {
		p, err := setRPrAttr(doc, rPr, "b", boolVal(style.Bold.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}
	if style.Italic.Set {
		p, err := setRPrAttr(doc, rPr, "i", boolVal(style.Italic.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}
	if style.Size.Set {
		p, err := setRPrAttr(doc, rPr, "sz", sizeCentipoints(style.Size.Value), prefix)
		if err != nil {
			return nil, err
		}
		if p != nil {
			patches = append(patches, *p)
		}
	}

	// 子元素类：color/latin/ea/cs。
	if style.Color.Set {
		if !style.Color.Value.Valid() {
			return nil, &OperationError{Message: "empty ColorSpec", Err: ErrInvalidArgument}
		}
		fill := fillChildOf(doc, rPr)
		frag := solidFillFragment(prefix, style.Color.Value)
		if fill != nil && fill.Local() == "solidFill" {
			// 就地替换 solidFill（区间一致，无顺序问题）。
			patches = append(patches, xmlstore.SpanPatch{
				Start: fill.Source.Start, End: fill.Source.End, Replacement: []byte(frag),
			})
		} else if fill != nil {
			// 其它填充类型被显式颜色替换。
			patches = append(patches, xmlstore.SpanPatch{
				Start: fill.Source.Start, End: fill.Source.End, Replacement: []byte(frag),
			})
		} else {
			p, err := insertRPrChild(doc, rPr, frag, fillRank)
			if err != nil {
				return nil, err
			}
			patches = append(patches, p)
		}
	}
	for _, fc := range []struct {
		set  Optional[string]
		kind string
		rank int
	}{
		{style.Latin, "latin", latinRank},
		{style.EastAsian, "ea", eaRank},
		{style.ComplexScript, "cs", csRank},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return nil, Annotate(mapXMLError(err), "SetFont")
		}
		frag := "<" + prefix + ":" + fc.kind + ` typeface="` + esc + `"/>`
		existing := childOfKind(doc, rPr, nsDrawingML, fc.kind, 0)
		if existing != nil {
			patches = append(patches, xmlstore.SpanPatch{
				Start: existing.Source.Start, End: existing.Source.End, Replacement: []byte(frag),
			})
		} else {
			p, err := insertRPrChild(doc, rPr, frag, fc.rank)
			if err != nil {
				return nil, err
			}
			patches = append(patches, p)
		}
	}
	return patches, nil
}

// setRPrAttr 设置 rPr 属性值；属性不存在时在开标签内追加。
// 返回 nil 表示值未变化无需补丁。
func setRPrAttr(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, name, value, prefix string) (*xmlstore.SpanPatch, error) {
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace == "" && a.RawName == name {
			if a.Value == value {
				return nil, nil
			}
			p := xmlstore.SpanPatch{Start: a.ValueStart, End: a.ValueEnd, Replacement: []byte(value)}
			return &p, nil
		}
	}
	// 追加到开标签末尾（'>' 或 '/>' 之前；自闭合时插到 '/' 前）。
	pos := rPr.OpenEnd - 1
	if rPr.SelfClosing() {
		pos = rPr.OpenEnd - 2
	}
	repl := []byte(" " + name + "=\"" + value + "\"")
	p := xmlstore.SpanPatch{Start: pos, End: pos, Replacement: repl}
	return &p, nil
}

// insertRPrChild 把 fragment 插入 rPr 的 schema 序位置。rPr 含无法
// 判定族别的未知子元素且目标排在其后时返回 ErrUnsupportedEdit
// （不做可能破坏顺序的猜测）。
func insertRPrChild(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, frag string, targetRank int) (xmlstore.SpanPatch, error) {
	insertPos := -1
	for i, cid := range rPr.Children {
		c := doc.Node(cid)
		rank, ok := rPrChildRank(c.Namespace, c.Local())
		if !ok {
			// 未知子元素：若其后还有已知/目标插入点将破坏顺序 → 拒绝。
			return xmlstore.SpanPatch{}, &OperationError{
				Op:      "TextRun.SetFont",
				Part:    "",
				Message: "cannot safely order new run property around unknown child <" + c.Name() + ">",
				Err:     ErrUnsupportedEdit,
			}
		}
		if rank >= targetRank {
			insertPos = c.Source.Start
			break
		}
		_ = i
	}
	if insertPos < 0 {
		// 追加到末尾（rPr 闭标签前）。
		insertPos = rPr.CloseStart
	}
	return xmlstore.SpanPatch{Start: insertPos, End: insertPos, Replacement: []byte(frag)}, nil
}

// expandSelfClosingRPr 把自闭合 rPr（保留全部原属性）展开为带 style
// 子元素的形态。
func expandSelfClosingRPr(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord, prefix string, style FontStyle) (string, error) {
	qp := rPr.QName.Prefix
	if qp == "" {
		qp = "a"
	}
	// 原开标签内属性（去掉 '<pfx:rPr' 与 '/>'），逐条重建。
	var sb strings.Builder
	sb.WriteString("<" + qp + ":rPr")
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if isNSDeclAttr(a.RawName) {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(a.Value, '"')
		if err != nil {
			return "", Annotate(mapXMLError(err), "SetFont")
		}
		sb.WriteString(" " + a.RawName + "=\"" + esc + "\"")
	}
	// 应用 style 属性（去重）。
	styleAttrs := map[string]string{}
	if style.Bold.Set {
		styleAttrs["b"] = boolVal(style.Bold.Value)
	}
	if style.Italic.Set {
		styleAttrs["i"] = boolVal(style.Italic.Value)
	}
	if style.Size.Set {
		styleAttrs["sz"] = sizeCentipoints(style.Size.Value)
	}
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if v, ok := styleAttrs[a.RawName]; ok {
			delete(styleAttrs, a.RawName)
			_ = v
		}
	}
	// 剩余 style 属性按固定序输出（b/i/sz）。
	ordered := []string{"b", "i", "sz"}
	for _, k := range ordered {
		if v, ok := styleAttrs[k]; ok {
			sb.WriteString(" " + k + "=\"" + v + "\"")
		}
	}
	if !style.anyChildSet() {
		sb.WriteString("/>")
		return sb.String(), nil
	}
	sb.WriteString(">")
	frag, err := rPrChildrenFragment(prefix, style)
	if err != nil {
		return "", err
	}
	sb.WriteString(frag)
	sb.WriteString("</" + qp + ":rPr>")
	return sb.String(), nil
}

// rPrChildrenFragment 生成 rPr 子元素（color/latin/ea/cs），供自闭合展开。
func rPrChildrenFragment(prefix string, style FontStyle) (string, error) {
	var children []string
	if style.Color.Set {
		if !style.Color.Value.Valid() {
			return "", &OperationError{Message: "empty ColorSpec", Err: ErrInvalidArgument}
		}
		children = append(children, solidFillFragment(prefix, style.Color.Value))
	}
	for _, fc := range []struct {
		set  Optional[string]
		kind string
	}{
		{style.Latin, "latin"},
		{style.EastAsian, "ea"},
		{style.ComplexScript, "cs"},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return "", Annotate(mapXMLError(err), "SetFont")
		}
		children = append(children, "<"+prefix+":"+fc.kind+` typeface="`+esc+`"/>`)
	}
	return strings.Join(children, ""), nil
}

// anyChildSet 报告 style 是否含子元素类字段。
func (f FontStyle) anyChildSet() bool {
	return f.Color.Set || f.Latin.Set || f.EastAsian.Set || f.ComplexScript.Set
}

// isNSDeclAttr 报告属性名是否为 xmlns 声明（rPr 重建时跳过）。
func isNSDeclAttr(raw string) bool {
	return raw == "xmlns" || strings.HasPrefix(raw, "xmlns:")
}

// rPrChildRank 返回 rPr 子元素族别的 schema 序号；未知返回 ok=false。
func rPrChildRank(ns, local string) (int, bool) {
	if ns != nsDrawingML {
		return 0, false
	}
	switch local {
	case "ln":
		return 0, true
	case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
		return 1, true
	case "effectLst", "effectDag":
		return 2, true
	case "highlight":
		return 3, true
	case "uLnTx", "uLn":
		return 4, true
	case "uFillTx", "uFill":
		return 5, true
	case "latin":
		return 6, true
	case "ea":
		return 7, true
	case "cs":
		return 8, true
	case "sym":
		return 9, true
	case "hlinkClick", "hlinkMouseOver":
		return 10, true
	case "rtl":
		return 11, true
	case "extLst":
		return 12, true
	}
	return 0, false
}

const (
	fillRank  = 1
	latinRank = 6
	eaRank    = 7
	csRank    = 8
)

// fillChildOf 返回 rPr 中填充族子元素（含 solidFill/noFill/grad 等）。
func fillChildOf(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill":
			return c
		}
	}
	return nil
}

// solidFillFragment 生成 solidFill 片段。
func solidFillFragment(prefix string, c ColorSpec) string {
	if c.Scheme != "" {
		esc, _ := xmlstore.EscapeAttrValue(c.Scheme, '"')
		return "<" + prefix + ":solidFill><" + prefix + ":schemeClr val=\"" + esc + "\"/></" + prefix + ":solidFill>"
	}
	return "<" + prefix + ":solidFill><" + prefix + ":srgbClr val=\"" + c.RGB + "\"/></" + prefix + ":solidFill>"
}

// buildRPrFragment 从 style 构造全新 rPr 片段（属性 + 子元素按序）。
func buildRPrFragment(prefix string, style FontStyle) (string, error) {
	var attrs []string
	if style.Bold.Set {
		attrs = append(attrs, `b="`+boolVal(style.Bold.Value)+`"`)
	}
	if style.Italic.Set {
		attrs = append(attrs, `i="`+boolVal(style.Italic.Value)+`"`)
	}
	if style.Size.Set {
		attrs = append(attrs, `sz="`+sizeCentipoints(style.Size.Value)+`"`)
	}
	var children []string
	if style.Color.Set {
		if !style.Color.Value.Valid() {
			return "", &OperationError{Message: "empty ColorSpec", Err: ErrInvalidArgument}
		}
		children = append(children, solidFillFragment(prefix, style.Color.Value))
	}
	for _, fc := range []struct {
		set  Optional[string]
		kind string
	}{
		{style.Latin, "latin"},
		{style.EastAsian, "ea"},
		{style.ComplexScript, "cs"},
	} {
		if !fc.set.Set {
			continue
		}
		esc, err := xmlstore.EscapeAttrValue(fc.set.Value, '"')
		if err != nil {
			return "", Annotate(mapXMLError(err), "SetFont")
		}
		children = append(children, "<"+prefix+":"+fc.kind+` typeface="`+esc+`"/>`)
	}
	var sb strings.Builder
	sb.WriteString("<" + prefix + ":rPr")
	for _, a := range attrs {
		sb.WriteString(" ")
		sb.WriteString(a)
	}
	if len(children) == 0 {
		sb.WriteString("/>")
	} else {
		sb.WriteString(">")
		for _, c := range children {
			sb.WriteString(c)
		}
		sb.WriteString("</" + prefix + ":rPr>")
	}
	return sb.String(), nil
}

// anySet 报告 FontStyle 是否含显式字段。
func (f FontStyle) anySet() bool {
	return f.Bold.Set || f.Italic.Set || f.Size.Set || f.Color.Set ||
		f.Latin.Set || f.EastAsian.Set || f.ComplexScript.Set
}

func boolVal(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// sizeCentipoints 把 pt 字号转 XML 的百分之一 pt 整数（四舍五入）。
func sizeCentipoints(sz FontSize) string {
	cp := int(float64(sz)*100 + 0.5)
	return intString(cp)
}

func intString(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// parseLocalFont 读取 rPr 中可安全表示的本地属性。
func parseLocalFont(doc *xmlstore.XMLDocument, rPr *xmlstore.NodeRecord) FontStyle {
	var f FontStyle
	for i := range rPr.Attrs {
		a := &rPr.Attrs[i]
		if a.Namespace != "" {
			continue
		}
		switch a.RawName {
		case "b":
			f.Bold = Optional[bool]{Value: a.Value != "0", Set: true}
		case "i":
			f.Italic = Optional[bool]{Value: a.Value != "0", Set: true}
		case "sz":
			if cp, err := parseCentipoints(a.Value); err == nil {
				f.Size = Optional[FontSize]{Value: FontSize(cp) / 100.0, Set: true}
			}
		}
	}
	for _, cid := range rPr.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "solidFill":
			if spec, ok := parseSolidFill(doc, c); ok {
				f.Color = Optional[ColorSpec]{Value: spec, Set: true}
			}
		case "latin":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.Latin = Optional[string]{Value: tf, Set: true}
			}
		case "ea":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.EastAsian = Optional[string]{Value: tf, Set: true}
			}
		case "cs":
			if tf, ok := c.Attr("", "typeface"); ok {
				f.ComplexScript = Optional[string]{Value: tf, Set: true}
			}
		}
	}
	return f
}

// parseSolidFill 只识别 a:schemeClr / a:srgbClr 两种可安全表示的形态。
func parseSolidFill(doc *xmlstore.XMLDocument, fill *xmlstore.NodeRecord) (ColorSpec, bool) {
	for _, cid := range fill.Children {
		c := doc.Node(cid)
		if c.Namespace != nsDrawingML {
			continue
		}
		switch c.Local() {
		case "schemeClr":
			if v, ok := c.Attr("", "val"); ok {
				return ColorSpec{Scheme: v}, true
			}
		case "srgbClr":
			if v, ok := c.Attr("", "val"); ok {
				return ColorSpec{RGB: v}, true
			}
		}
	}
	return ColorSpec{}, false
}

func parseCentipoints(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	v := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errors.New("non-digit")
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}

// removeAttrPatch 构造删除属性补丁：从属性名前导空白（若有）到值结束
// 引号之后（含引号）。NameStart/ValueEnd 来自 xmlstore 索引的精确
// 字节锚点；ValueEnd 指向闭合引号位置（值区间为引号内内容），因此
// 结束边界取 ValueEnd+1 以连同引号一并删除。
func removeAttrPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, attrIdx int) xmlstore.SpanPatch {
	a := &n.Attrs[attrIdx]
	orig := doc.Original()
	start := a.NameStart
	if start > n.Source.Start {
		switch orig[start-1] {
		case ' ', '\t', '\n', '\r':
			start--
		}
	}
	return xmlstore.SpanPatch{
		Start:       start,
		End:         a.ValueEnd + 1,
		Replacement: []byte(""),
	}
}

// removeElementPatch 构造删除元素补丁。
func removeElementPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) xmlstore.SpanPatch {
	return xmlstore.SpanPatch{
		Start:       n.Source.Start,
		End:         n.Source.End,
		Replacement: []byte(""),
	}
}

func firstChildOf(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *xmlstore.NodeRecord {
	if len(n.Children) == 0 {
		return nil
	}
	return doc.Node(n.Children[0])
}

// xmlUnescape 解码文本内容中出现的 XML 实体（含数字引用）。
func xmlUnescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		j := strings.IndexByte(s[i:], ';')
		if j < 0 {
			sb.WriteByte(s[i])
			i++
			continue
		}
		ent := s[i+1 : i+j]
		switch ent {
		case "amp":
			sb.WriteByte('&')
		case "lt":
			sb.WriteByte('<')
		case "gt":
			sb.WriteByte('>')
		case "quot":
			sb.WriteByte('"')
		case "apos":
			sb.WriteByte('\'')
		default:
			if len(ent) > 1 && ent[0] == '#' {
				var code rune = -1
				if ent[1] == 'x' || ent[1] == 'X' {
					code = parseHexRune(ent[2:])
				} else {
					code = parseDecRune(ent[1:])
				}
				if code >= 0 {
					sb.WriteRune(code)
				} else {
					sb.WriteString(s[i : i+j+1])
				}
			} else {
				sb.WriteString(s[i : i+j+1]) // 未知命名实体原样保留
			}
		}
		i += j + 1
	}
	return sb.String()
}

func parseHexRune(s string) rune {
	var v rune
	for i := 0; i < len(s); i++ {
		c := s[i]
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			v |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= rune(c-'A') + 10
		default:
			return -1
		}
		if v > 0x10FFFF {
			return -1
		}
	}
	return v
}

func parseDecRune(s string) rune {
	var v rune
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return -1
		}
		v = v*10 + rune(c-'0')
		if v > 0x10FFFF {
			return -1
		}
	}
	return v
}
