package pptx

import (
	"strings"

	"github.com/F31/go-pptx/internal/textutil"
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
// textNode 可选携带所属形状的 cNvPr@id（shapeHint；V2.6 §M8 收尾）：
// 解析 path 后向上找最近 p:sp 的 cNvPr@id，与 shapeHint 比对——
// 不等即 ErrStaleHandle。这把 shape 增删的失效检测从"path 解析到相邻
// 兄弟 shape 的同形 txBody"提升为"按 shape 身份识别"。shapeHint=0
// 走纯路径判定（向后兼容；老句柄/测试零值句柄/notes 等无 cNvPr 的
// 文本模型）。段落/Run 在所属 shape 内的增删仍是已知妥协——句柄只能
// 告诉"句柄仍属于原 shape"，不能告诉"句柄仍是原段落"。
//
// 读侧语义（§7.1）：段落属性（a:pPr）与结束字符属性（a:endParaRPr）
// 在模型中保留，不扁平化成 []string；Text() 仅拼接普通 Run 的 a:t
// 文本，a:br/a:fld 等内联节点的展示规则随 TEXT-02 textmap 落地。

// ---------- TextFrame ----------

// Stable: TextFrame 是文本操作三层入口（TextFrame → Paragraph → TextRun）
// 之一，类比 Presentation/Slide/Shape 同级别。0 公开字段（私有字段仅用于
// 句柄身份定位，不属于 API）；句柄失效语义由 textNode 嵌入 + shapeHint
// 锁定（V2.6 §M8 STALE-GUARD 修复点）。v1.x 内承诺：
//
//   - 类型签名不变；不新增/重命名/移除公开方法
//   - 现有方法签名与返回类型不变（Text/Runs/Paragraphs/SetText/SetFont 等）
//   - 仅允许在不破坏既有方法前提下**追加**新方法（如 AddParagraph 已在
//     create.go 补充；未来可加 InsertParagraph/RemoveParagraph 等）
//   - 句柄身份语义不变：cNvPr@id + path 双层校验
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
		// 嵌入复制 t.textNode 而非重建，让 Paragraph/TextRun 自动继承
		// shapeHint（V2.6 §M8 textNode 句柄失效语义修复点）。
		out = append(out, &Paragraph{
			textNode: t.textNode,
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
	if err := applySinglePartPatch(t.p, t.part, out); err != nil {
		return Annotate(err, "TextFrame.SetPlainText")
	}
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

// Stable: Paragraph 是段落（a:p）的受控句柄，类比 Presentation/Slide/Shape
// 同级别。0 公开字段（私有 idx 仅用于段落序号定位，不属于 API）；句柄失效
// 语义由 textNode 嵌入 + shapeHint 锁定（V2.6 §M8 STALE-GUARD 修复点）。
// v1.x 内承诺：
//
//   - 类型签名不变；不新增/重命名/移除公开方法
//   - 现有方法签名与返回类型不变（Text/Runs/AddRun/Props 等）
//   - 仅允许在不破坏既有方法前提下**追加**新方法
//   - 句柄身份语义不变：cNvPr@id + 段落序号双层校验
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

// Text 返回本段落普通 Run 与字段（a:fld）缓存文本的拼接（TEXT-03：
// 字段展示规则固定为缓存文本嵌入；a:r/a:t 与 a:fld/a:t 按文档序拼接）。
// a:br 不贡献文本。
func (p *Paragraph) Text() (string, error) {
	doc, para, err := p.locatePara()
	if err != nil {
		return "", Annotate(err, "Paragraph.Text")
	}
	return paragraphText(doc, para), nil
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
	if style.AnySet() {
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
	if err := applySinglePartPatch(p.p, p.part, out); err != nil {
		return nil, Annotate(err, "Paragraph.AddRun")
	}
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

// Stable: TextRun 是普通 Run（a:r）的受控句柄，类比 Presentation/Slide/
// Shape 同级别。0 公开字段（私有 paraIdx/runIdx 仅用于段落/Run 序号定位，
// 不属于 API）；句柄失效语义由 textNode 嵌入 + shapeHint 锁定（V2.6 §M8
// STALE-GUARD 修复点）。v1.x 内承诺：
//
//   - 类型签名不变；不新增/重命名/移除公开方法
//   - 现有方法签名与返回类型不变（Text/SetText/ExplicitFont/SetFont/
//     ResetFontProperty/AdvancedProps 等）
//   - 仅允许在不破坏既有方法前提下**追加**新方法
//   - 句柄身份语义不变：cNvPr@id + 段落序号 + Run 序号三层校验
//   - Run 字段集为空（无 a:rPr 字段直接暴露）；字体通过 ExplicitFont /
//     SetFont 间接操作；段落级属性通过 Paragraph.Props 暴露
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
			return textutil.XmlUnescape(string(doc.Original()[tt.OpenEnd:tt.CloseStart])), nil
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
	if err := applySinglePartPatch(r.p, r.part, out); err != nil {
		return Annotate(err, "TextRun.SetText")
	}
	return nil
}
