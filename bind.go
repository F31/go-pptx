package pptx

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/F31/go-pptx/internal/editplan"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// 本文件实现 TPL-01 模板数据绑定引擎（方案 §2.4 创新方向，ADR 013）。
//
// 声明式模板层：占位符 + 数据源 → 整套文档。**构建在保真补丁与跨 Run
// 替换之上**（ADR 013）——不引入第二套编辑路径：内联占位符复用
// replacePatches（TEXT-02 同一条保真替换路径），行复制/段落删除复用
// xmlstore.SpanPatch，与 Save/Validate 共用同一 revision 语义。
//
// 模板语法（v1 固定，真实语料回填后可能扩展）：
//   - 内联占位符 `{{ path }}`：点分路径（如 `title`、`user.name`、
//     `items.0.name`），可出现在形状正文与表格单元格的任意段落内；
//     跨 Run 匹配由 replacePatches 保证（占位符被拆到多个 Run 也能命中）。
//   - 条件段落 `{{#if path}}` … `{{/if}}`：标记必须独占段落；条件为假时
//     删除标记与块内全部段落。支持嵌套（栈判定）。
//   - 表格行循环 `{{#each path}}` … `{{/each}}`：标记必须位于同一表格行
//     的单元格内且独占段落；该行作为模板行按数据条数复制，块内
//     `{{field}}` 以条目为作用域解析（`{{.}}` 表示标量条目自身）。
//     数据为空切片时删除模板行（表格仅剩该行时拒绝，避免产出空表）。
//   - 图表数据绑定：数据源中值为 ChartData 且键等于图表形状名
//     （p:cNvPr@name）时绑定，走 CHART-01 的 ChartShape.SetData。
//
// 条件真值：nil / false / 空串 / 零数值 / 空切片 → 假；其余为真。
//
// 原子性（"绑定失败显式报错且无部分写入"）：
//   - 阶段 1 plan：纯读取——扫描全文档、解析数据、校验图表可写性，
//     任何语义错误在此返回，不产生任何补丁；
//   - 阶段 2 apply：补丁按 Part 聚合，每 Part 一次 ApplyPatches 后通过
//     MultiPartPlan 单次提交（无部分写入）；图表绑定在 plan 阶段已预检，
//     随后逐图提交。
//
// 严格模式（默认开启 WithBindStrict(true)）：数据源缺键或值类型不可
// 呈现时显式报错（ErrInvalidArgument）；关闭时未解析占位符保留原文并
// 记 Warning 诊断。不支持的构造（如行循环内嵌条件段落）一律
// ErrUnsupportedEdit，不做部分合并。

// 模板标记与行循环临时包装（声明 DrawingML 为默认命名空间，兼容
// 源文档使用前缀或默认命名空间的两种写法）。
const (
	tplMarkOpen  = "{{"
	tplMarkClose = "}}"
	tplWrapOpen  = `<tplwrap xmlns="` + nsDrawingML + `" xmlns:a="` + nsDrawingML +
		`" xmlns:p="` + nsPresentationML + `" xmlns:r="` + nsOfficeDocument + `">`
	tplWrapClose = `</tplwrap>`
)

// ---------- 公共 API ----------

// BindOption / bindOptions / WithBindStrict / WithBindReplaceMode 已迁出至 options.go。

// BindReport 是一次模板绑定的执行汇总。
type BindReport struct {
	Slides            int          // 参与绑定的页数
	Placeholders      int          // 文档中识别到的占位符出现次数
	Substituted       int          // 实际完成替换的占位符出现次数
	RowsGenerated     int          // 表格行循环生成的数据行数
	RowsRemoved       int          // 因数据为空而删除的模板行数
	ParagraphsRemoved int          // 因条件不成立而删除的段落数
	ChartsBound       int          // 完成数据绑定的图表数
	Parts             []string     // 被修改的 Part（去重升序）
	Revision          uint64       // 提交后的 revision
	Diagnostics       []Diagnostic // 非致命问题（跳过项等）
}

// Bind 按数据源渲染当前文档中的模板标记（TPL-01）。
//
// data 为绑定数据源（键为顶层名称）；只读语义之外的修改遵循 §19.1
// revision 语义。任何绑定失败（缺键、类型不符、不支持的构造）都显式
// 返回错误且不产生部分写入——plan 阶段纯读取校验，apply 阶段按 Part
// 聚合后单次提交。
func (p *Presentation) Bind(data map[string]any, opts ...BindOption) (BindReport, error) {
	const op = "Presentation.Bind"
	o := bindOptions{strict: true, mode: ReplaceFirstCharacter}
	for _, fn := range opts {
		fn(&o)
	}
	if p == nil {
		return BindReport{}, &OperationError{Op: op, Message: "nil presentation", Err: ErrInvalidArgument}
	}
	if p.closed {
		return BindReport{}, Annotate(ErrClosed, op)
	}
	if data == nil {
		data = map[string]any{}
	}

	s := &bindScanner{
		p:     p,
		data:  data,
		o:     o,
		parts: map[opc.PartName]*bindPart{},
	}

	// 阶段 1：plan（纯读取，失败即返回，不修改任何字节）。
	// 扫描期错误已自带 Op（OperationError），不再二次包裹。
	if err := s.scan(); err != nil {
		return BindReport{}, err
	}

	// 阶段 2：apply（按 Part 聚合，单事务提交）。
	names := make([]opc.PartName, 0, len(s.parts))
	for name := range s.parts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return string(names[i]) < string(names[j]) })
	ops := make([]editplan.Operation, 0, len(names))
	for _, name := range names {
		out, err := s.applyPart(name, s.parts[name])
		if err != nil {
			return BindReport{}, err
		}
		ops = append(ops, editplan.Patch(name, out))
		s.rep.Parts = append(s.rep.Parts, string(name))
	}
	if len(ops) > 0 {
		if err := applyMultiPartPlan(p, editplan.NewMultiPartPlan(ops...)); err != nil {
			return BindReport{}, err
		}
	}

	// 图表数据绑定（plan 阶段已预检，此处仅执行；每个图表自身事务）。
	for _, cb := range s.charts {
		if err := cb.shape.SetData(cb.data); err != nil {
			return BindReport{}, Annotate(err, op)
		}
		s.rep.ChartsBound++
	}
	s.rep.Revision = p.Revision()
	return s.rep, nil
}

// ---------- 应用 ----------

// applyPart 在单个 Part 上回放全部绑定操作，返回新字节。
//
// 顺序：按起始偏移降序（后改先应用，前面的偏移不漂移）；同一段落的
// 多个占位符逐个重新索引后应用（避免 rebuild 补丁区间重叠）。
func (s *bindScanner) applyPart(name opc.PartName, bp *bindPart) ([]byte, error) {
	const op = "Presentation.Bind"
	doc, err := s.p.docOf(name)
	if err != nil {
		return nil, Annotate(err, op)
	}
	bp.base = doc.Original()
	work := bp.base
	if len(bp.subs)+len(bp.dels)+len(bp.rows) == 0 {
		return work, nil
	}

	type unit struct {
		start int
		kind  int // 0=sub 1=del 2=row
		key   int // sub: 段落起始偏移；del/row: 切片下标
	}
	units := make([]unit, 0, len(bp.subs)+len(bp.dels)+len(bp.rows))
	for off := range bp.subs {
		units = append(units, unit{start: off, kind: 0, key: off})
	}
	for i, d := range bp.dels {
		units = append(units, unit{start: d.start, kind: 1, key: i})
	}
	for i, r := range bp.rows {
		units = append(units, unit{start: r.start, kind: 2, key: i})
	}
	sort.SliceStable(units, func(a, b int) bool { return units[a].start > units[b].start })

	for _, u := range units {
		switch u.kind {
		case 1:
			d := bp.dels[u.key]
			work, err = xmlstore.ApplyPatches(work, []xmlstore.SpanPatch{{
				Start: d.start, End: d.end, Expect: d.expect, Desc: "bind-delete",
			}})
		case 2:
			r := bp.rows[u.key]
			work, err = xmlstore.ApplyPatches(work, []xmlstore.SpanPatch{{
				Start: r.start, End: r.end, Replacement: r.repl, Expect: r.expect, Desc: "bind-row-loop",
			}})
		default:
			sub := bp.subs[u.key]
			for _, pr := range sub.pairs {
				cur, err := xmlstore.Index(work)
				if err != nil {
					return nil, Annotate(mapXMLError(err), op)
				}
				node := resolvePath(cur, sub.path)
				if node == nil {
					return nil, &OperationError{Op: op, Part: string(name),
						Message: "bound paragraph no longer resolvable", Err: ErrStaleHandle}
				}
				res, patches, err := replacePatches(cur, node, pr.needle, pr.value, replaceOptions{mode: s.o.mode})
				if err != nil {
					return nil, Annotate(err, op)
				}
				if res.Replaced < res.Matches {
					s.diag("bind.replace_skipped", string(name), fmt.Sprintf(
						"placeholder %q: %d of %d occurrences skipped by fidelity rules",
						pr.needle, res.Skipped, res.Matches))
				}
				s.rep.Substituted += res.Replaced
				if len(patches) == 0 {
					continue
				}
				work, err = xmlstore.ApplyPatches(work, patches)
				if err != nil {
					return nil, Annotate(mapXMLError(err), op)
				}
			}
			continue
		}
		if err != nil {
			return nil, Annotate(mapXMLError(err), op)
		}
	}
	return work, nil
}

// ---------- 扫描与绑定 ----------

// chartBind 是一条已预检的图表绑定。
type chartBind struct {
	shape *ChartShape
	data  ChartData
}

// bindScanner 是绑定上下文：扫描文档、解析数据、累积待应用操作。
type bindScanner struct {
	p      *Presentation
	data   map[string]any
	o      bindOptions
	rep    BindReport
	parts  map[opc.PartName]*bindPart
	charts []chartBind
}

// bindPair 是一个占位符的字面量与替换值。
type bindPair struct{ needle, value string }

// bindSub 是同一段落上的一组占位符替换（按段落起始偏移聚合）。
type bindSub struct {
	path   []nodeStep
	offset int
	pairs  []bindPair
}

// bindDel 是一次区间删除（段落删除或段落内容清空）。
type bindDel struct {
	start  int
	end    int
	expect []byte
}

// bindRow 是一次行循环替换（模板行区间 → 生成的 N 行）。
type bindRow struct {
	start  int
	end    int
	expect []byte
	repl   []byte
}

// bindPart 聚合单个 Part 上的全部绑定操作。
//
// 应用顺序为"起始偏移降序"：先改后面的字节，前面的偏移不漂移；
// 同一段落的多个占位符在应用时逐个重新索引（replacePatches 的
// rebuild 补丁可能覆盖整段 Run，逐个应用才能避免区间重叠）。
type bindPart struct {
	base []byte
	subs map[int]*bindSub
	dels []bindDel
	rows []bindRow
}

// part 取（或创建）某 Part 的操作聚合器。
func (s *bindScanner) part(name opc.PartName) *bindPart {
	if bp, ok := s.parts[name]; ok {
		return bp
	}
	bp := &bindPart{subs: map[int]*bindSub{}}
	s.parts[name] = bp
	return bp
}

// addSub 登记一次占位符替换（同一段落按 offset 合并）。
func (s *bindScanner) addSub(part opc.PartName, offset int, path []nodeStep, needle, value string) {
	bp := s.part(part)
	sub, ok := bp.subs[offset]
	if !ok {
		sub = &bindSub{path: path, offset: offset}
		bp.subs[offset] = sub
	}
	sub.pairs = append(sub.pairs, bindPair{needle: needle, value: value})
}

// addDel 登记一次区间删除（expect 为原文锚定）。
func (s *bindScanner) addDel(part opc.PartName, start, end int, expect []byte) {
	bp := s.part(part)
	bp.dels = append(bp.dels, bindDel{start: start, end: end, expect: expect})
}

// addRow 登记一次行循环替换。
func (s *bindScanner) addRow(part opc.PartName, start, end int, expect, repl []byte) {
	bp := s.part(part)
	bp.rows = append(bp.rows, bindRow{start: start, end: end, expect: expect, repl: repl})
}

// diag 记录一条非致命诊断。
func (s *bindScanner) diag(code, part, msg string) {
	s.rep.Diagnostics = append(s.rep.Diagnostics, Diagnostic{
		Code: code, Severity: SeverityWarning, Part: part, Message: msg,
	})
}

// scan 遍历全部页面与形状，构建补丁计划。
func (s *bindScanner) scan() error {
	slides, err := s.p.Slides()
	if err != nil {
		return err
	}
	s.rep.Slides = len(slides)
	for _, sl := range slides {
		shapes, err := sl.Shapes()
		if err != nil {
			return err
		}
		if err := s.scanShapes(shapes, 0); err != nil {
			return err
		}
	}
	return nil
}

// scanShapes 递归扫描形状（含组形状，深度上限 8）。
func (s *bindScanner) scanShapes(shapes []Shape, depth int) error {
	if depth > 8 {
		return nil
	}
	for _, sh := range shapes {
		switch v := sh.(type) {
		case *GroupShape:
			children, err := v.Children()
			if err != nil {
				return err
			}
			if err := s.scanShapes(children, depth+1); err != nil {
				return err
			}
		case *AutoShape:
			tf, err := v.TextFrame()
			if err != nil || tf == nil {
				continue
			}
			paras, err := tf.Paragraphs()
			if err != nil {
				return err
			}
			if err := s.bindBody(sh.Name(), paras); err != nil {
				return err
			}
		case *TableShape:
			if err := s.bindTable(v); err != nil {
				return err
			}
		case *ChartShape:
			if err := s.bindChart(v); err != nil {
				return err
			}
		}
	}
	return nil
}

// bindBody 处理一个文本体（形状正文或表格单元格）内的条件段落与
// 内联占位符。scope 非 nil 时为行循环条目作用域（当前仅行内使用）。
func (s *bindScanner) bindBody(label string, paras []*Paragraph) error {
	const op = "Presentation.Bind"
	del := make([]bool, len(paras))
	var stack []bool // {{#if}} 条件栈（全部为真才保留内容）
	allTrue := func() bool {
		for _, b := range stack {
			if !b {
				return false
			}
		}
		return true
	}
	for i, para := range paras {
		text, err := para.Text()
		if err != nil {
			return err
		}
		trimmed := strings.TrimSpace(text)
		if kind, path, ok := parseDirective(trimmed); ok {
			switch kind {
			case "if":
				v, found, err := s.resolve(nil, path)
				if err != nil {
					return err
				}
				if !found {
					if s.o.strict {
						return &OperationError{Op: op, Message: fmt.Sprintf(
							"condition key %q not found in data (shape %q)", path, label), Err: ErrInvalidArgument}
					}
					s.diag("bind.key_missing", "", fmt.Sprintf("condition key %q missing; treated as false", path))
				}
				stack = append(stack, found && truthy(v))
			case "endif":
				if len(stack) == 0 {
					return &OperationError{Op: op, Message: fmt.Sprintf(
						"unmatched %s/if%s in shape %q", tplMarkOpen, tplMarkClose, label), Err: ErrInvalidArgument}
				}
				stack = stack[:len(stack)-1]
			case "each", "endeach":
				return &OperationError{Op: op, Message: fmt.Sprintf(
					"%s#each%s marker in shape %q must live in a table row cell", tplMarkOpen, tplMarkClose, label),
					Err: ErrUnsupportedEdit}
			}
			del[i] = true // 标记段落始终不保留
			continue
		}
		if len(stack) > 0 && !allTrue() {
			del[i] = true // 条件为假：块内段落删除
			continue
		}
		if err := s.substitute(para, text, nil, label); err != nil {
			return err
		}
	}
	if len(stack) != 0 {
		return &OperationError{Op: op, Message: fmt.Sprintf(
			"unclosed %s#if%s block in shape %q", tplMarkOpen, tplMarkClose, label), Err: ErrInvalidArgument}
	}
	return s.applyDeletions(label, paras, del)
}

// applyDeletions 生成段落删除补丁；若文本体会被清空，则保留首段并
// 清空其内容（避免产出无段落的 txBody）。
func (s *bindScanner) applyDeletions(label string, paras []*Paragraph, del []bool) error {
	n := 0
	for _, d := range del {
		if d {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	if n == len(paras) {
		// 全删保护：首段改为清空内容（保留 a:p 元素）。
		for i, d := range del {
			if !d {
				continue
			}
			doc, node, err := paras[i].locatePara()
			if err != nil {
				return err
			}
			if p := emptyParaPatch(doc, node); p != nil {
				s.addDel(paras[i].part, p.Start, p.End, p.Expect)
			}
			del[i] = false
			break
		}
	}
	for i, d := range del {
		if !d {
			continue
		}
		doc, node, err := paras[i].locatePara()
		if err != nil {
			return err
		}
		s.addDel(paras[i].part, node.Source.Start, node.Source.End, doc.Slice(node.Source))
		s.rep.ParagraphsRemoved++
	}
	return nil
}

// substitute 在段落内替换全部内联占位符（同一字面量只调一次
// replacePatches，其内部会替换该字面量的所有出现）。
func (s *bindScanner) substitute(para *Paragraph, text string, scope any, label string) error {
	const op = "Presentation.Bind"
	toks := scanInline(text)
	if len(toks) == 0 {
		return nil
	}
	s.rep.Placeholders += len(toks)
	seen := map[string]bool{}
	doc, node, err := para.locatePara()
	if err != nil {
		return err
	}
	path := recordPath(doc, node.ID)
	offset := node.Source.Start
	for _, tok := range toks {
		if seen[tok.literal] {
			continue
		}
		seen[tok.literal] = true
		v, found, err := s.resolve(scope, tok.path)
		if err != nil {
			return err
		}
		if !found {
			if s.o.strict {
				return &OperationError{Op: op, Message: fmt.Sprintf(
					"placeholder %q not found in data (shape %q)", tok.path, label), Err: ErrInvalidArgument}
			}
			s.diag("bind.key_missing", string(para.part),
				fmt.Sprintf("placeholder %q missing; left as-is", tok.path))
			continue
		}
		rep, ok := formatBindValue(v)
		if !ok {
			return &OperationError{Op: op, Message: fmt.Sprintf(
				"placeholder %q has unsupported value type %T", tok.path, v), Err: ErrInvalidArgument}
		}
		// 实际补丁在 apply 阶段生成（同段落多占位符需逐个重新索引）。
		s.addSub(para.part, offset, path, tok.literal, rep)
	}
	return nil
}

// ---------- 表格行循环 ----------

// bindTable 处理一个表格：先识别行循环模板行，再逐个单元格绑定。
func (s *bindScanner) bindTable(t *TableShape) error {
	const op = "Presentation.Bind"
	rows, err := t.RowCount()
	if err != nil {
		return err
	}
	cols, err := t.ColumnCount()
	if err != nil {
		return err
	}
	type cellParas struct {
		paras []*Paragraph
	}
	byRow := make([][]cellParas, rows)
	for r := 0; r < rows; r++ {
		byRow[r] = make([]cellParas, cols)
		for c := 0; c < cols; c++ {
			cell, err := t.Cell(r, c)
			if err != nil {
				continue
			}
			tf, err := cell.TextFrame()
			if err != nil || tf == nil {
				continue
			}
			paras, err := tf.Paragraphs()
			if err != nil {
				continue
			}
			byRow[r][c] = cellParas{paras: paras}
		}
	}

	for r := 0; r < rows; r++ {
		path, hasEach := "", false
		hasEnd := false
		for c := 0; c < cols; c++ {
			for _, para := range byRow[r][c].paras {
				txt, err := para.Text()
				if err != nil {
					return err
				}
				kind, p, ok := parseDirective(strings.TrimSpace(txt))
				if !ok {
					continue
				}
				switch kind {
				case "each":
					if hasEach {
						return &OperationError{Op: op, Message: fmt.Sprintf(
							"multiple %s#each%s markers in one table row (%d)", tplMarkOpen, tplMarkClose, r),
							Err: ErrUnsupportedEdit}
					}
					hasEach, path = true, p
				case "endeach":
					hasEnd = true
				case "if", "endif":
					return &OperationError{Op: op, Message: fmt.Sprintf(
						"conditional blocks are not supported inside a row loop (row %d)", r),
						Err: ErrUnsupportedEdit}
				}
			}
		}
		if hasEach != hasEnd {
			return &OperationError{Op: op, Message: fmt.Sprintf(
				"%s#each%s/%s markers must be paired within table row %d", tplMarkOpen, tplMarkClose, tplMarkOpen, r),
				Err: ErrInvalidArgument}
		}
		if !hasEach {
			for c := 0; c < cols; c++ {
				if err := s.bindBody(fmt.Sprintf("table row %d col %d", r, c), byRow[r][c].paras); err != nil {
					return err
				}
			}
			continue
		}
		// 该行为模板行：整行被生成行替换，其单元格不再单独绑定。
		items, err := s.resolveItems(path)
		if err != nil {
			return err
		}
		if len(items) == 0 && rows == 1 {
			return &OperationError{Op: op, Message: fmt.Sprintf(
				"row loop %q produced no rows and the table would be empty", path), Err: ErrUnsupportedEdit}
		}
		if err := s.buildRowPatch(t, r, path, items); err != nil {
			return err
		}
	}
	return nil
}

// buildRowPatch 用数据条目渲染模板行，并用生成行整体替换模板行区间。
func (s *bindScanner) buildRowPatch(t *TableShape, row int, path string, items []any) error {
	doc, tr, err := t.rowNode(row)
	if err != nil {
		return err
	}
	rowBytes := doc.Slice(tr.Source)
	var out [][]byte
	for _, item := range items {
		rb, err := s.renderRow(rowBytes, item)
		if err != nil {
			return err
		}
		out = append(out, rb)
		s.rep.RowsGenerated++
	}
	if len(items) == 0 {
		s.rep.RowsRemoved++
	}
	s.addRow(t.part, tr.Source.Start, tr.Source.End, rowBytes, bytes.Join(out, nil))
	return nil
}

// renderRow 在临时文档中渲染一行：删除 each 标记段落、按条目作用域
// 替换占位符，返回渲染后的 a:tr 字节。
//
// 行内多个占位符与标记删除按"起始偏移降序"回放（同 applyPart），
// 同一段落的多个占位符逐个重新索引后应用，避免 rebuild 补丁重叠。
func (s *bindScanner) renderRow(rowBytes []byte, item any) ([]byte, error) {
	const op = "Presentation.Bind"
	wrapped := make([]byte, 0, len(tplWrapOpen)+len(rowBytes)+len(tplWrapClose))
	wrapped = append(wrapped, tplWrapOpen...)
	wrapped = append(wrapped, rowBytes...)
	wrapped = append(wrapped, tplWrapClose...)
	tmp, err := xmlstore.Index(wrapped)
	if err != nil {
		return nil, Annotate(mapXMLError(err), op)
	}
	root := tmp.Root()
	if root == nil {
		return nil, &OperationError{Op: op, Message: "row loop: cannot index template row", Err: ErrMalformedPackage}
	}
	tr := childOfKind(tmp, root, nsDrawingML, "tr", 0)
	if tr == nil || tr.Namespace != nsDrawingML {
		return nil, &OperationError{Op: op, Message: "row loop: template row is not a DrawingML a:tr", Err: ErrMalformedPackage}
	}

	// 1) 收集段落级操作（标记删除 / 占位符替换）。
	type paraUnit struct {
		start int
		del   bool
		path  []nodeStep
		pairs []bindPair
	}
	var units []paraUnit
	for _, tc := range childElems(tmp, tr, "tc") {
		for _, tx := range childElems(tmp, tc, "txBody") {
			for _, p := range childElems(tmp, tx, "p") {
				text := paragraphText(tmp, p)
				if kind, _, ok := parseDirective(strings.TrimSpace(text)); ok {
					switch kind {
					case "each", "endeach":
						units = append(units, paraUnit{start: p.Source.Start, del: true})
					default:
						return nil, &OperationError{Op: op, Message: fmt.Sprintf(
							"unsupported marker %q inside row loop", strings.TrimSpace(text)), Err: ErrUnsupportedEdit}
					}
					continue
				}
				toks := scanInline(text)
				if len(toks) == 0 {
					continue
				}
				s.rep.Placeholders += len(toks)
				u := paraUnit{start: p.Source.Start, path: recordPath(tmp, p.ID)}
				seen := map[string]bool{}
				for _, tok := range toks {
					if seen[tok.literal] {
						continue
					}
					seen[tok.literal] = true
					v, found, err := s.resolve(item, tok.path)
					if err != nil {
						return nil, err
					}
					if !found {
						if s.o.strict {
							return nil, &OperationError{Op: op, Message: fmt.Sprintf(
								"placeholder %q not found in row item", tok.path), Err: ErrInvalidArgument}
						}
						s.diag("bind.key_missing", "", fmt.Sprintf(
							"placeholder %q missing in row item; left as-is", tok.path))
						continue
					}
					rep, ok := formatBindValue(v)
					if !ok {
						return nil, &OperationError{Op: op, Message: fmt.Sprintf(
							"placeholder %q has unsupported value type %T", tok.path, v), Err: ErrInvalidArgument}
					}
					u.pairs = append(u.pairs, bindPair{needle: tok.literal, value: rep})
				}
				if len(u.pairs) > 0 {
					units = append(units, u)
				}
			}
		}
	}

	// 2) 按起始偏移降序回放。
	sort.SliceStable(units, func(a, b int) bool { return units[a].start > units[b].start })
	work := tmp.Original()
	for _, u := range units {
		if u.del {
			work, err = xmlstore.ApplyPatches(work, []xmlstore.SpanPatch{deletePatch(tmp, tmpNodeAt(tmp, u.start), "each-marker")})
			if err != nil {
				return nil, Annotate(mapXMLError(err), op)
			}
			continue
		}
		for _, pr := range u.pairs {
			cur, err := xmlstore.Index(work)
			if err != nil {
				return nil, Annotate(mapXMLError(err), op)
			}
			node := resolvePath(cur, u.path)
			if node == nil {
				return nil, &OperationError{Op: op, Message: "row loop: paragraph no longer resolvable", Err: ErrStaleHandle}
			}
			res, patches, err := replacePatches(cur, node, pr.needle, pr.value, replaceOptions{mode: s.o.mode})
			if err != nil {
				return nil, err
			}
			s.rep.Substituted += res.Replaced
			if len(patches) == 0 {
				continue
			}
			work, err = xmlstore.ApplyPatches(work, patches)
			if err != nil {
				return nil, Annotate(mapXMLError(err), op)
			}
		}
	}
	if len(work) < len(tplWrapOpen)+len(tplWrapClose) {
		return nil, &OperationError{Op: op, Message: "row loop: rendered row is empty", Err: ErrMalformedPackage}
	}
	return work[len(tplWrapOpen) : len(work)-len(tplWrapClose)], nil
}

// tmpNodeAt 在（未修改的）临时文档中按起始偏移定位节点。
func tmpNodeAt(doc *xmlstore.XMLDocument, start int) *xmlstore.NodeRecord {
	for i := 0; i < doc.Len(); i++ {
		if n := doc.Node(xmlstore.NodeID(i)); n != nil && n.Source.Start == start {
			return n
		}
	}
	return nil
}

// ---------- 图表绑定 ----------

// bindChart 若数据源中存在与图表同名的 ChartData，则预检并登记绑定。
func (s *bindScanner) bindChart(c *ChartShape) error {
	const op = "Presentation.Bind"
	name := c.Name()
	if name == "" {
		return nil
	}
	v, found, err := s.resolve(nil, name)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	cd, ok := v.(ChartData)
	if !ok {
		return &OperationError{Op: op, Message: fmt.Sprintf(
			"chart %q expects pptx.ChartData in the data source, got %T", name, v), Err: ErrInvalidArgument}
	}
	if err := chartBindPreflight(c, cd); err != nil {
		return err
	}
	s.charts = append(s.charts, chartBind{shape: c, data: cd})
	return nil
}

// chartBindPreflight 复刻 ChartShape.SetData 的前置校验（只读），使
// plan 阶段即可判定该图表能否绑定——避免 apply 阶段中途失败。
func chartBindPreflight(c *ChartShape, cd ChartData) error {
	const op = "Presentation.Bind"
	if err := c.alive(); err != nil {
		return Annotate(err, op)
	}
	if err := validateChartData(cd, op); err != nil {
		return err
	}
	part, err := c.chartPartOf()
	if err != nil {
		return Annotate(err, op)
	}
	doc, err := c.p.docOf(part)
	if err != nil {
		return Annotate(err, op)
	}
	root := doc.Root()
	if root == nil || root.Namespace != nsChartML || root.Local() != "chartSpace" {
		return &OperationError{Op: op, Part: string(part),
			Message: "chart part root is not c:chartSpace", Err: ErrMalformedPackage}
	}
	if !chartIsCanonical(doc, root) {
		return &OperationError{Op: op, Part: string(part),
			Message: "chart " + c.Name() + " layout is not the library-canonical form; refused to avoid partial merge",
			Err:     ErrUnsupportedEdit}
	}
	cur, err := parseChartSpace(doc, root)
	if err != nil {
		return Annotate(err, op)
	}
	if cur.Type != cd.Type {
		return &OperationError{Op: op, Part: string(part), Message: fmt.Sprintf(
			"chart %q type change (%s → %s) is not a supported edit", c.Name(), cur.Type, cd.Type),
			Err: ErrUnsupportedEdit}
	}
	if _, ok := c.p.chartWorkbookPartOf(part); !ok {
		return &OperationError{Op: op, Part: string(part),
			Message: "chart " + c.Name() + " has no embedded workbook", Err: ErrUnsupportedEdit}
	}
	return nil
}

// ---------- 数据解析 ----------

// resolve 解析点分路径：首段先在 scope（行循环条目）中查找，未命中则
// 回退到根数据源；支持 map 键、切片下标与结构体字段。
func (s *bindScanner) resolve(scope any, path string) (any, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false, &OperationError{
			Op: "Presentation.Bind", Message: "empty placeholder path", Err: ErrInvalidArgument}
	}
	if path == "." {
		if scope == nil {
			return nil, false, nil
		}
		return scope, true, nil
	}
	segs := strings.Split(path, ".")
	var cur any
	start := 0
	if scope != nil {
		if v, ok := member(scope, segs[0]); ok {
			cur, start = v, 1
		}
	}
	if start == 0 {
		v, ok := member(s.data, segs[0])
		if !ok {
			return nil, false, nil
		}
		cur, start = v, 1
	}
	for _, seg := range segs[start:] {
		v, ok := member(cur, seg)
		if !ok {
			return nil, false, nil
		}
		cur = v
	}
	return cur, true, nil
}

// resolveItems 解析行循环数据，要求值为切片/数组。
func (s *bindScanner) resolveItems(path string) ([]any, error) {
	const op = "Presentation.Bind"
	v, found, err := s.resolve(nil, path)
	if err != nil {
		return nil, err
	}
	if !found {
		if s.o.strict {
			return nil, &OperationError{Op: op, Message: fmt.Sprintf(
				"row loop key %q not found in data", path), Err: ErrInvalidArgument}
		}
		s.diag("bind.key_missing", "", fmt.Sprintf("row loop key %q missing; no rows generated", path))
		return nil, nil
	}
	items, ok := asSlice(v)
	if !ok {
		return nil, &OperationError{Op: op, Message: fmt.Sprintf(
			"row loop key %q must be a slice, got %T", path, v), Err: ErrInvalidArgument}
	}
	return items, nil
}

// member 取 map 键 / 切片下标 / 结构体字段（反射兜底支持
// []map[string]any 等具体类型）。
func member(v any, key string) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		x, ok := t[key]
		return x, ok
	case []any:
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i >= len(t) {
			return nil, false
		}
		return t[i], true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Map:
		kt := rv.Type().Key()
		if kt.Kind() != reflect.String {
			return nil, false
		}
		x := rv.MapIndex(reflect.ValueOf(key).Convert(kt))
		if !x.IsValid() {
			return nil, false
		}
		return x.Interface(), true
	case reflect.Slice, reflect.Array:
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i >= rv.Len() {
			return nil, false
		}
		return rv.Index(i).Interface(), true
	case reflect.Struct:
		f := rv.FieldByName(key)
		if !f.IsValid() || !f.CanInterface() {
			// 大小写不敏感回退（首字母小写字段名）。
			f = rv.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, key) })
			if !f.IsValid() || !f.CanInterface() {
				return nil, false
			}
		}
		return f.Interface(), true
	}
	return nil, false
}

// asSlice 把任意切片/数组值展开为 []any。
func asSlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	if t, ok := v.([]any); ok {
		return t, true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = rv.Index(i).Interface()
		}
		return out, true
	}
	return nil, false
}

// truthy 判定条件真值：nil / false / 空串 / 零数值 / 空容器 → 假。
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.Len() > 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() > 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.Bool:
		return rv.Bool()
	}
	return true
}

// formatBindValue 把数据值呈现为占位符文本（不可呈现类型返回 false）。
func formatBindValue(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case int:
		return strconv.Itoa(t), true
	case int8:
		return strconv.FormatInt(int64(t), 10), true
	case int16:
		return strconv.FormatInt(int64(t), 10), true
	case int32:
		return strconv.FormatInt(int64(t), 10), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case uint:
		return strconv.FormatUint(uint64(t), 10), true
	case uint8:
		return strconv.FormatUint(uint64(t), 10), true
	case uint16:
		return strconv.FormatUint(uint64(t), 10), true
	case uint32:
		return strconv.FormatUint(uint64(t), 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case time.Time:
		return t.Format("2006-01-02"), true
	case fmt.Stringer:
		return t.String(), true
	}
	return "", false
}

// ---------- 模板语法 ----------

// tplToken 是一个内联占位符：literal 为文档中的原始字面量。
type tplToken struct {
	literal string
	path    string
}

// parseDirective 解析独占段落的块标记；ok=false 表示不是块标记。
// kind 取值：if / endif / each / endeach。
func parseDirective(s string) (kind, path string, ok bool) {
	if !strings.HasPrefix(s, tplMarkOpen) || !strings.HasSuffix(s, tplMarkClose) {
		return "", "", false
	}
	inner := strings.TrimSpace(s[len(tplMarkOpen) : len(s)-len(tplMarkClose)])
	switch {
	case inner == "/if":
		return "endif", "", true
	case inner == "/each":
		return "endeach", "", true
	case strings.HasPrefix(inner, "#if "):
		p := strings.TrimSpace(strings.TrimPrefix(inner, "#if "))
		if p == "" {
			return "", "", false
		}
		return "if", p, true
	case strings.HasPrefix(inner, "#each "):
		p := strings.TrimSpace(strings.TrimPrefix(inner, "#each "))
		if p == "" {
			return "", "", false
		}
		return "each", p, true
	}
	return "", "", false
}

// scanInline 扫描文本中的内联占位符（跳过块标记形态）。
func scanInline(s string) []tplToken {
	var out []tplToken
	for i := 0; i+len(tplMarkClose) <= len(s); {
		at := strings.Index(s[i:], tplMarkOpen)
		if at < 0 {
			break
		}
		start := i + at
		end := strings.Index(s[start+len(tplMarkOpen):], tplMarkClose)
		if end < 0 {
			break
		}
		end = start + len(tplMarkOpen) + end + len(tplMarkClose)
		lit := s[start:end]
		inner := strings.TrimSpace(lit[len(tplMarkOpen) : len(lit)-len(tplMarkClose)])
		i = end
		if inner == "" || strings.HasPrefix(inner, "#") || strings.HasPrefix(inner, "/") {
			continue // 块标记由 parseDirective 处理
		}
		out = append(out, tplToken{literal: lit, path: inner})
	}
	return out
}

// ---------- 补丁辅助 ----------

// deletePatch 构造删除整个元素的补丁（带原文锚定）。
func deletePatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, desc string) xmlstore.SpanPatch {
	return xmlstore.SpanPatch{
		Start:  n.Source.Start,
		End:    n.Source.End,
		Expect: doc.Slice(n.Source),
		Desc:   desc,
	}
}

// emptyParaPatch 构造"清空段落内容"的补丁（保留 a:p 元素本身）；
// 无子元素时返回 nil。
func emptyParaPatch(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord) *xmlstore.SpanPatch {
	if n.SelfClosing() || len(n.Children) == 0 {
		return nil
	}
	first := doc.Node(n.Children[0])
	last := doc.Node(n.Children[len(n.Children)-1])
	return &xmlstore.SpanPatch{
		Start:  first.Source.Start,
		End:    last.Source.End,
		Expect: doc.Slice(xmlstore.ByteRange{Start: first.Source.Start, End: last.Source.End}),
		Desc:   "empty-paragraph",
	}
}

// childElems 返回按文档序的直接子元素中本地名为 local 的节点。
func childElems(doc *xmlstore.XMLDocument, n *xmlstore.NodeRecord, local string) []*xmlstore.NodeRecord {
	var out []*xmlstore.NodeRecord
	for _, id := range n.Children {
		c := doc.Node(id)
		if c.Local() == local {
			out = append(out, c)
		}
	}
	return out
}
