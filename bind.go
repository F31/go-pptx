package pptx

import (
	"fmt"
	"sort"

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
