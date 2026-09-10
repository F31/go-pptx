package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// Presentation 是一份演示文稿的受控入口（方案 §5）。
//
// 生命周期与约定：
//   - 单实例不保证并发安全（含会填充缓存的读取）；多个独立实例可并行；
//   - Open 持有文件资源由 Close 释放；OpenReader 不关闭调用方的 ReaderAt；
//   - 每次成功的公共修改隐式提交一次事务并递增 revision（§19.1）；
//   - Save(path) 默认拒绝与源文件同一文件实体的原位保存（源文件仍被
//     惰性读取）；输出先写同目录临时文件再原子替换（SAVE-02）。
type Presentation struct {
	pk   *opc.Package
	main opc.PartName

	// srcPath/srcFile 仅 Open 持有；OpenReader/New 为空。
	srcPath string
	srcFile *os.File

	rev    uint64
	closed bool

	// coreAutoModified 表示 Modified 时间戳处于"库代管"状态：最近一次
	// SetCoreProperties 未显式携带 Modified 时置位；Save/Write 前若置位
	// 自动把 core.xml 的 dcterms:modified 刷新为当前 UTC 时间
	//（方案 §5.1）。显式传入 Modified 后清除。
	coreAutoModified bool

	// DocumentStore 骨架（§18）：overrides/addedParts/deletedParts 保存
	// 已提交变更的最新字节与内容类型（读取视图优先于包内原始内容）；
	// pending 是当前隐式事务的暂存区，仅在公共修改方法执行期间非空
	//（提交即合并入上述三表）。addedParts 与 deletedParts 同时服务
	// 保存计划（ChangeSet.Added/Deleted），使增删在多次提交后仍可重放。
	overrides    map[opc.PartName][]byte
	addedParts   map[opc.PartName]opc.AddedPart
	deletedParts map[opc.PartName]bool
	pending      *opc.ChangeSet

	// partDocs 是按 Part 缓存的解析文档（惰性），键值保存其构建时的
	// revision；commit 递增 revision 后整体失效（Part 数量少，重建便宜）。
	partDocs map[opc.PartName]*partDocEntry

	// chartWorkbookBuilder 是图表嵌入工作簿适配器（CHART-01，方案
	// §9.2）；nil 时使用 DefaultWorkbookBuilder。
	chartWorkbookBuilder ChartWorkbookBuilder
}

// partDocEntry 是某 Part 最新 revision 下的解析缓存。
type partDocEntry struct {
	doc *xmlstore.XMLDocument
	rev uint64
}

// NewOption 是 New 的函数式选项。
type NewOption func(*newOptions)

type newOptions struct {
	budget   opc.Budget
	template map[opc.PartName][]byte
}

// WithNewBudget 覆盖 New 的资源预算（默认 DefaultBudget）。
func WithNewBudget(b opc.Budget) NewOption {
	return func(o *newOptions) { o.budget = b }
}

// WithNewTemplate 追加/替换模板 Part（键为 OPC 风格 "/..." 名）。
// 模板整体仍须经完整的 OPC 装载校验，非法输入在 New 返回错误。
func WithNewTemplate(parts map[string][]byte) NewOption {
	return func(o *newOptions) {
		if o.template == nil {
			o.template = make(map[opc.PartName][]byte, len(parts))
		}
		for k, v := range parts {
			o.template[opc.PartName(k)] = append([]byte(nil), v...)
		}
	}
}

// New 基于库内合法最小模板（或调用方模板）创建空演示文稿。
func New(opts ...NewOption) (*Presentation, error) {
	o := newOptions{budget: opc.DefaultBudget()}
	for _, fn := range opts {
		fn(&o)
	}
	parts := minimalTemplateParts()
	for name, content := range o.template {
		parts[name] = content
	}
	data, err := buildPackageZip(parts)
	if err != nil {
		return nil, Annotate(err, "Presentation.New")
	}
	pk, err := opc.Load(bytes.NewReader(data), int64(len(data)), o.budget)
	if err != nil {
		return nil, Annotate(mapOCError(err), "Presentation.New")
	}
	main, err := pk.MainPart()
	if err != nil {
		return nil, Annotate(mapOCError(err), "Presentation.New")
	}
	return &Presentation{
		pk:           pk,
		main:         main,
		overrides:    make(map[opc.PartName][]byte),
		addedParts:   make(map[opc.PartName]opc.AddedPart),
		deletedParts: make(map[opc.PartName]bool),
		partDocs:     make(map[opc.PartName]*partDocEntry),
	}, nil
}

// OpenOption 是 Open/OpenReader 的函数式选项。
type OpenOption func(*openOptions)

type openOptions struct {
	budget opc.Budget
}

// WithBudget 覆盖打开时的资源预算（默认 DefaultBudget）。
func WithBudget(b opc.Budget) OpenOption {
	return func(o *openOptions) { o.budget = b }
}

// Open 打开磁盘上的 PPTX 文件；文件资源由 Close 释放。
func Open(path string, opts ...OpenOption) (*Presentation, error) {
	o := openOptions{budget: opc.DefaultBudget()}
	for _, fn := range opts {
		fn(&o)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, Annotate(err, "Presentation.Open")
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Annotate(err, "Presentation.Open")
	}
	pk, err := opc.Load(f, st.Size(), o.budget)
	if err != nil {
		f.Close()
		return nil, Annotate(mapOCError(err), "Presentation.Open")
	}
	main, err := pk.MainPart()
	if err != nil {
		f.Close()
		return nil, Annotate(mapOCError(err), "Presentation.Open")
	}
	return &Presentation{
		pk:           pk,
		main:         main,
		srcPath:      path,
		srcFile:      f,
		overrides:    make(map[opc.PartName][]byte),
		addedParts:   make(map[opc.PartName]opc.AddedPart),
		deletedParts: make(map[opc.PartName]bool),
		partDocs:     make(map[opc.PartName]*partDocEntry),
	}, nil
}

// OpenReader 从调用方提供的 ReaderAt 打开演示文稿；不关闭 r。
// size 是底层流的总长度（ZIP 中央目录读取需要）。
func OpenReader(r io.ReaderAt, size int64, opts ...OpenOption) (*Presentation, error) {
	o := openOptions{budget: opc.DefaultBudget()}
	for _, fn := range opts {
		fn(&o)
	}
	pk, err := opc.Load(r, size, o.budget)
	if err != nil {
		return nil, Annotate(mapOCError(err), "Presentation.OpenReader")
	}
	// 主 Part 发现走包级 officeDocument 关系；Load 只保证根关系流存在，
	// 不保证 officeDocument 关系存在（或其为内部目标、目标 Part 落盘）——
	// 恶意包可在这些情形下让 Load 通过而 MainPart 失败，必须显式报错而
	// 非 panic（AT-14）。
	main, err := pk.MainPart()
	if err != nil {
		return nil, Annotate(mapOCError(err), "Presentation.OpenReader")
	}
	return &Presentation{
		pk:           pk,
		main:         main,
		overrides:    make(map[opc.PartName][]byte),
		addedParts:   make(map[opc.PartName]opc.AddedPart),
		deletedParts: make(map[opc.PartName]bool),
		partDocs:     make(map[opc.PartName]*partDocEntry),
	}, nil
}

// Close 释放资源。Open 打开的文件在此关闭；此后任何方法返回
// ErrClosed（Close 本身幂等返回 ErrClosed 语义之外的 nil 无必要，
// 重复 Close 返回 ErrClosed）。
func (p *Presentation) Close() error {
	if p.closed {
		return Annotate(ErrClosed, "Presentation.Close")
	}
	p.closed = true
	if p.srcFile != nil {
		err := p.srcFile.Close()
		p.srcFile = nil
		if err != nil {
			return Annotate(err, "Presentation.Close")
		}
	}
	return nil
}

// Revision 返回当前文档 revision；每次成功的公共修改提交后递增。
func (p *Presentation) Revision() uint64 {
	return p.rev
}

// Slides 返回页面受控句柄的新切片，顺序为 presentation.xml 的 sldIdLst
// 顺序（不按 id 排序，方案 §19.2）。不触发各页面的形状解析。
func (p *Presentation) Slides() ([]*Slide, error) {
	if p.closed {
		return nil, Annotate(ErrClosed, "Presentation.Slides")
	}
	doc, err := p.presentationDoc()
	if err != nil {
		return nil, Annotate(err, "Presentation.Slides")
	}
	lsts := doc.Elements(nsPresentationML, "sldIdLst")
	var out []*Slide
	for _, lstID := range lsts {
		lst := doc.Node(lstID)
		for _, cid := range lst.Children {
			n := doc.Node(cid)
			if n.Namespace != nsPresentationML || n.Local() != "sldId" {
				continue // 未知子节点：保留不解析（§4.3 未知保留）
			}
			idStr, ok := n.Attr("", "id")
			if !ok {
				return nil, &OperationError{
					Op:      "Presentation.Slides",
					Part:    string(p.main),
					Message: "p:sldId missing id attribute",
					Err:     ErrMalformedPackage,
				}
			}
			id, err := parseUint32(idStr)
			if err != nil || id < 256 || id > 2147483647 {
				return nil, &OperationError{
					Op: "Presentation.Slides", Part: string(p.main),
					Message: fmt.Sprintf("p:sldId id %q outside [256,2147483647]", idStr),
					Err:     ErrMalformedPackage,
				}
			}
			rid, ok := n.Attr(nsOfficeDocument, "id")
			if !ok {
				return nil, &OperationError{
					Op: "Presentation.Slides", Part: string(p.main),
					Message: "p:sldId missing r:id attribute",
					Err:     ErrMalformedPackage,
				}
			}
			rels, ok, err := p.relsOf(p.main)
			if err != nil || !ok {
				return nil, &OperationError{
					Op: "Presentation.Slides", Part: string(p.main),
					Message: "presentation part has no relationships",
					Err:     ErrMalformedPackage,
				}
			}
			var target opc.PartName
			for _, rel := range rels {
				if rel.ID == rid && rel.Mode == opc.TargetInternal {
					target = rel.TargetPart
					break
				}
			}
			if target == "" {
				return nil, &OperationError{
					Op: "Presentation.Slides", Part: string(p.main),
					SlideID: SlideID(id),
					Message: fmt.Sprintf("sldId %q has no internal slide relationship", rid),
					Err:     ErrNotFound,
				}
			}
			out = append(out, &Slide{p: p, id: SlideID(id), part: target, rev: p.rev})
		}
	}
	return out, nil
}

// SaveReport 是一次保存的结果。
type SaveReport struct {
	// Revision 是保存所基于的文档 revision。
	Revision uint64
	// ChangedParts 是输出中发生变化的 Part 名（排序）。
	ChangedParts []string
	// Diagnostics 是保存计划的非阻断诊断。
	Diagnostics []Diagnostic
}

// SaveOption 是 Save/Write 的函数式选项。
type SaveOption func(*saveOptions)

type saveOptions struct {
	overwrite  bool
	durability opc.Durability
}

// WithSaveOverwrite 显式允许覆盖已存在的目标文件（不绕过原子替换语义）。
func WithSaveOverwrite(v bool) SaveOption {
	return func(o *saveOptions) { o.overwrite = v }
}

// WithSaveDurability 设置持久性级别（默认只保证原子可见性）。
func WithSaveDurability(d opc.Durability) SaveOption {
	return func(o *saveOptions) { o.durability = d }
}

// Save 将当前文档原子保存到 path（同目录临时文件 → 校验 → 原子替换）。
//
// 默认拒绝覆盖已存在目标（WithSaveOverwrite 启用）；且拒绝与源文件
// 同一文件实体的原位保存——源文件仍被惰性读取，安全原位替换待后续
// 版本（方案 §5）。返回的 SaveReport 基于保存时的 revision 快照。
func (p *Presentation) Save(ctx context.Context, path string, opts ...SaveOption) (SaveReport, error) {
	o := saveOptions{}
	for _, fn := range opts {
		fn(&o)
	}
	if p.closed {
		return SaveReport{}, Annotate(ErrClosed, "Presentation.Save")
	}
	if err := ctx.Err(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Save")
	}
	if path == "" {
		return SaveReport{}, Annotate(ErrInvalidArgument, "Presentation.Save")
	}
	if p.sameSourceEntity(path) {
		return SaveReport{}, &OperationError{
			Op:      "Presentation.Save",
			Message: "saving to the source file is not supported; choose a new output path (in-place replace arrives in a later version)",
			Err:     ErrInvalidArgument,
		}
	}

	// Modified 未显式指定时由库代管：保存前刷新为当前时间。
	if err := p.flushAutoModified(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Save")
	}
	rev, plan, err := p.buildPlan()
	if err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Save")
	}
	if p.rev != rev { // 计划期间的变更检测（§18）
		return SaveReport{}, Annotate(ErrConcurrentModification, "Presentation.Save")
	}
	err = plan.SaveToFile(p.pk, path,
		opc.WithOverwrite(o.overwrite), opc.WithDurability(o.durability))
	if err != nil {
		return SaveReport{}, Annotate(mapOCError(err), "Presentation.Save")
	}
	return planReport(rev, plan), nil
}

// Write 将当前文档写入 w。已写入的字节无法回滚；I/O 失败时错误明确
// 表明输出可能不完整（方案 §5）。
func (p *Presentation) Write(ctx context.Context, w io.Writer, opts ...SaveOption) (SaveReport, error) {
	if p.closed {
		return SaveReport{}, Annotate(ErrClosed, "Presentation.Write")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Write")
	}
	if w == nil {
		return SaveReport{}, Annotate(ErrInvalidArgument, "Presentation.Write")
	}
	// Modified 未显式指定时由库代管：保存前刷新为当前时间。
	if err := p.flushAutoModified(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Write")
	}
	rev, plan, err := p.buildPlan()
	if err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Write")
	}
	if p.rev != rev {
		return SaveReport{}, Annotate(ErrConcurrentModification, "Presentation.Write")
	}
	if err := plan.Write(p.pk, w); err != nil {
		return SaveReport{}, &OperationError{
			Op:      "Presentation.Write",
			Message: "write failed; output may be incomplete and cannot be rolled back",
			Err:     mapOCError(err),
		}
	}
	return planReport(rev, plan), nil
}

// Validate 对当前文档执行 L0 结构校验（默认 Structural 模式）：每个
// Part 都有 Content Types 覆盖、页面关系目标存在。返回报告不自动失败；
// 保存路径是否把 error 级诊断升级为 ErrValidationFailed 由保存选项决定
// （后续 WP 扩展 ValidateOption）。
func (p *Presentation) Validate(ctx context.Context, opts ...ValidateOption) ValidationReport {
	report := ValidationReport{Mode: "Structural"}
	if p.closed {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{
			Code: "CLOSED", Severity: SeverityError, Message: "document is closed",
		})
		return report
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = opts // 预留：ValidationMode/外部严格校验属后续 WP
	for _, name := range p.pk.PartNames() {
		if ctx.Err() != nil {
			report.Diagnostics = append(report.Diagnostics, Diagnostic{
				Code: "CANCELLED", Severity: SeverityError, Message: ctx.Err().Error(),
			})
			return report
		}
		if _, ok := p.pk.ContentType(name); !ok {
			report.Diagnostics = append(report.Diagnostics, Diagnostic{
				Code:     "OPC_CT_MISSING",
				Severity: SeverityError,
				Part:     string(name),
				Message:  "part has no content type (neither Override nor Default)",
			})
		}
	}
	// 页面关系目标存在性（关系图在 Load 时已解析，这里核对 Part 实体）。
	for _, pair := range p.pk.RelatedByIDs(p.main, opc.RelSlide) {
		target := opc.PartName(pair[1])
		if !p.pk.HasPart(target) {
			report.Diagnostics = append(report.Diagnostics, Diagnostic{
				Code: "OPC_REL_TARGET_MISSING", Severity: SeverityError,
				Part: string(p.main), Message: "slide relationship target missing: " + pair[1],
			})
		}
	}
	return report
}

// ValidateOption 预留：校验选项（ValidationMode 等）随校验 WP 落地。
type ValidateOption func(*validateOptions)

type validateOptions struct{}

// ---------- 事务与读取视图骨架（§18/§19.1，供后续 WP 的 Setter 使用） ----------

// stagePatch 把一次节点补丁的结果暂存到当前隐式事务。失败（含重复
// 冲突）不改变对象状态。commit 成功前对 Slides/Save/Write 不可见。
func (p *Presentation) stagePatch(name opc.PartName, newBytes []byte) error {
	if p.closed {
		return Annotate(ErrClosed, "stagePatch")
	}
	if !name.Valid() {
		return &OperationError{Message: "invalid part name " + string(name), Err: ErrInvalidArgument}
	}
	if p.pending == nil {
		p.pending = &opc.ChangeSet{}
	}
	if p.pending.Patched == nil {
		p.pending.Patched = make(map[opc.PartName][]byte)
	}
	if _, clash := p.pending.Deleted[name]; clash {
		return &OperationError{Part: string(name), Message: "part is staged for deletion", Err: ErrInvalidArgument}
	}
	p.pending.Patched[name] = append([]byte(nil), newBytes...)
	return nil
}

// stageAdd 暂存新建 Part。name 必须尚不存在于包与已提交视图；contentType
// 为空时要求该扩展名已有 Default 覆盖，否则在保存计划阶段失败
// （不允许产出无类型的 Part，方案 §18.2）。字节与内容类型均拷贝。
func (p *Presentation) stageAdd(name opc.PartName, content []byte, contentType string) error {
	if p.closed {
		return Annotate(ErrClosed, "stageAdd")
	}
	if !name.Valid() {
		return &OperationError{Message: "invalid part name " + string(name), Err: ErrInvalidArgument}
	}
	if p.pk.HasPart(name) || p.addedParts[name].Content != nil {
		return &OperationError{Part: string(name), Message: "part already exists", Err: ErrInvalidArgument}
	}
	if p.pending == nil {
		p.pending = &opc.ChangeSet{}
	}
	if p.pending.Added == nil {
		p.pending.Added = make(map[opc.PartName]opc.AddedPart)
	}
	if _, clash := p.pending.Patched[name]; clash {
		return &OperationError{Part: string(name), Message: "part is staged for patching", Err: ErrInvalidArgument}
	}
	p.pending.Added[name] = opc.AddedPart{
		Content:     append([]byte(nil), content...),
		ContentType: contentType,
	}
	return nil
}

// stageDelete 暂存删除（连带关系流由保存计划层处理）。
func (p *Presentation) stageDelete(name opc.PartName) error {
	if p.closed {
		return Annotate(ErrClosed, "stageDelete")
	}
	if p.pending == nil {
		p.pending = &opc.ChangeSet{}
	}
	if p.pending.Deleted == nil {
		p.pending.Deleted = make(map[opc.PartName]bool)
	}
	if _, clash := p.pending.Patched[name]; clash {
		return &OperationError{Part: string(name), Message: "part is staged for patching", Err: ErrInvalidArgument}
	}
	p.pending.Deleted[name] = true
	return nil
}

// commit 把当前暂存合并为一次提交：更新读取视图、递增 revision、
// 失效全部解析缓存。空事务是 no-op。
func (p *Presentation) commit() {
	if p.pending == nil {
		return
	}
	for name := range p.pending.Deleted {
		delete(p.overrides, name)
		delete(p.addedParts, name)
		if p.pk.HasPart(name) {
			// 仅包内既有 Part 进入删除视图（保存计划可重放）；
			// 会话内新增（未落盘）Part 被删除 = 直接丢弃，输出从未含它。
			p.deletedParts[name] = true
		}
	}
	for name, b := range p.pending.Patched {
		p.overrides[name] = b
		// 会话内新增（addedParts）Part 被再次修改：同步其最新字节，
		// 使保存计划 Added 分支输出补丁后内容（buildPlan 中 Added 优先
		// 于 Patched，仅改 overrides 会被旧 Added 内容覆盖）。
		if a, ok := p.addedParts[name]; ok {
			a.Content = b
			p.addedParts[name] = a
		}
	}
	for name, a := range p.pending.Added {
		p.addedParts[name] = a
		p.overrides[name] = a.Content
	}
	p.pending = nil
	p.rev++
	p.partDocs = make(map[opc.PartName]*partDocEntry) // 全部缓存随 revision 失效
}

// partBytes 是读取视图：优先已提交补丁/新增，其次包内原始内容。
// 已删除 Part 返回 ErrNotFound。
func (p *Presentation) partBytes(name opc.PartName) ([]byte, error) {
	if p.deletedParts[name] {
		return nil, Annotate(ErrNotFound, "partBytes")
	}
	if b, ok := p.overrides[name]; ok {
		return append([]byte(nil), b...), nil
	}
	rc, err := p.pk.OpenPart(name)
	if err != nil {
		return nil, mapOCError(err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// buildPlan 基于当前已提交状态构造保存计划，返回 (revision, plan)。
// 计划是当前 revision 的只读快照（§18）。
func (p *Presentation) buildPlan() (uint64, *opc.SavePlan, error) {
	cs := &opc.ChangeSet{
		Patched: make(map[opc.PartName][]byte),
		Added:   make(map[opc.PartName]opc.AddedPart),
		Deleted: make(map[opc.PartName]bool),
	}
	for name, b := range p.overrides {
		cs.Patched[name] = b
	}
	for name, a := range p.addedParts {
		cs.Added[name] = a
		delete(cs.Patched, name) // Added 与 Patched 互斥（同一最新字节只走 Added）
	}
	for name := range p.deletedParts {
		cs.Deleted[name] = true
		delete(cs.Patched, name)
		delete(cs.Added, name)
	}
	plan, err := opc.BuildSavePlan(p.pk, cs)
	if err != nil {
		return 0, nil, mapOCError(err)
	}
	return p.rev, plan, nil
}

// docOf 惰性解析并缓存指定 Part 的 XML 索引（当前 revision 快照）。
// 缓存随 commit 递增 revision 整体失效。
func (p *Presentation) docOf(part opc.PartName) (*xmlstore.XMLDocument, error) {
	if e, ok := p.partDocs[part]; ok && e.rev == p.rev {
		return e.doc, nil
	}
	data, err := p.partBytes(part)
	if err != nil {
		return nil, err
	}
	doc, err := xmlstore.Index(data)
	if err != nil {
		return nil, &OperationError{
			Op: "Presentation.parse", Part: string(part),
			Message: "part is not well-formed XML", Err: mapXMLError(err),
		}
	}
	p.partDocs[part] = &partDocEntry{doc: doc, rev: p.rev}
	return doc, nil
}

// presentationDoc 惰性解析并缓存 presentation.xml（主 Part 便捷封装）。
func (p *Presentation) presentationDoc() (*xmlstore.XMLDocument, error) {
	return p.docOf(p.main)
}

// sameSourceEntity 判断 path 是否与 Open 的源文件是同一文件实体
// （路径相等或 os.SameFile）。
func (p *Presentation) sameSourceEntity(path string) bool {
	if p.srcPath == "" {
		return false
	}
	absA, errA := filepath.Abs(p.srcPath)
	absB, errB := filepath.Abs(path)
	if errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB) {
		return true
	}
	si, errA := os.Stat(p.srcPath)
	ti, errB := os.Stat(path)
	if errA == nil && errB == nil {
		return os.SameFile(si, ti)
	}
	return false
}

// ---------- 内部辅助 ----------

// buildPackageZip 把 Part 集合编码为内存 ZIP（条目名经 EntryName 转换，
// 按名排序保证确定性输出）。
func buildPackageZip(parts map[opc.PartName][]byte) ([]byte, error) {
	names := make([]opc.PartName, 0, len(parts))
	for n := range parts {
		names = append(names, n)
	}
	sortPartNames(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		entry, err := n.EntryName()
		if err != nil {
			return nil, err
		}
		f, err := zw.Create(entry)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(parts[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sortPartNames(names []opc.PartName) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}

func parseUint32(s string) (uint32, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	var v uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit %q", c)
		}
		v = v*10 + uint64(c-'0')
		if v > 1<<32-1 {
			return 0, fmt.Errorf("overflow")
		}
	}
	return uint32(v), nil
}

// mapOCError 把 opc 层错误映射为根包稳定错误码（§20.4）。
func mapOCError(err error) error {
	var target error
	switch {
	case errors.Is(err, opc.ErrOutputExists):
		target = ErrOutputExists
	case errors.Is(err, opc.ErrAtomicReplaceUnavailable):
		target = ErrAtomicReplaceUnavailable
	case errors.Is(err, opc.ErrPlanInvalid):
		target = ErrValidationFailed
	default:
		target = nil
	}
	if target == nil {
		// opc.ErrMalformedPackage / opc.ErrNotFound 与根包哨兵同名同义，
		// errors.Is 直接穿透（哨兵值不同则映射）。
		if errors.Is(err, opc.ErrMalformedPackage) && !errors.Is(err, ErrMalformedPackage) {
			return fmt.Errorf("%w: %w", ErrMalformedPackage, err)
		}
		if errors.Is(err, opc.ErrNotFound) && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		if errors.Is(err, opc.ErrLimitExceeded) && !errors.Is(err, ErrLimitExceeded) {
			return fmt.Errorf("%w: %w", ErrLimitExceeded, err)
		}
		return err
	}
	return fmt.Errorf("%w: %w", target, err)
}

// mapXMLError 预留：xmlstore 错误映射（当前词法/索引错误统一按
// ErrMalformedPackage 语义由调用方包装）。
func mapXMLError(err error) error { return err }

// planReport 从保存计划构造报告。
func planReport(rev uint64, plan *opc.SavePlan) SaveReport {
	r := SaveReport{Revision: rev, ChangedParts: make([]string, 0, len(plan.ChangedParts))}
	for _, name := range plan.ChangedParts {
		r.ChangedParts = append(r.ChangedParts, string(name))
	}
	for _, d := range plan.Diagnostics {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{
			Code: d.Code, Severity: SeverityInfo, Part: string(d.Part), Message: d.Message,
		})
	}
	return r
}
