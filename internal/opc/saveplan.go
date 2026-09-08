package opc

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"sort"
)

// 保存计划（SAVE-01，方案 §18.2）：对当前包的变更集产出唯一、确定的
// 输出条目清单。保证：
//   - 同一输出 Part 只能存在一个 PlannedEntry；
//   - 未修改 Part 走 CopyOriginal（B1：解压后内容字节一致）；
//   - Content Types 与变更集同源生成，不允许分次保存；
//   - 删除 Part 自动连带其关系流（关系流脱离源 Part 无意义）。
//
// 原子落盘（临时文件/替换/失败恢复）属 SAVE-02，不在此处。

// ErrPlanInvalid 表示保存计划无法从给定变更集构造（输入一致性问题，
// 区别于包本身损坏的 ErrMalformedPackage）。
var ErrPlanInvalid = errors.New("opc: save plan invalid")

// EntryAction 是输出条目动作。
type EntryAction string

const (
	// CopyOriginal 未修改：原样复制解压内容（B1 保证）。
	CopyOriginal EntryAction = "CopyOriginal"
	// EmitPatched 已修改：写入变更集提供的新字节（通常来自补丁）。
	EmitPatched EntryAction = "EmitPatched"
	// EmitNew 新建 Part。
	EmitNew EntryAction = "EmitNew"
	// Omit 删除：不出现在输出中。
	Omit EntryAction = "Omit"
)

// PlannedEntry 是输出 ZIP 中的一个条目计划。
type PlannedEntry struct {
	Name   PartName
	Action EntryAction
	// Content 仅 EmitPatched/EmitNew 有值（新字节）；CopyOriginal/Omit 为空。
	Content []byte
}

// PlanDiagnostic 是计划期诊断（opc 内部形态；公共边界由上层映射为
// 根包 Diagnostic）。
type PlanDiagnostic struct {
	Code    string // 稳定码，如 "OPC_DANGLING_REL"
	Part    PartName
	Message string
}

// SavePlan 是输出条目的只读快照。
type SavePlan struct {
	// BaseRevision 预留：接入 DocumentStore revision 后由上层填充
	//（opc 层无 revision 概念，恒为 0）。
	BaseRevision uint64
	// Entries 按 Part 名排序（确定性输出）。
	Entries []PlannedEntry
	// Diagnostics 为非阻断诊断（如悬空关系）；阻断问题以错误返回。
	Diagnostics []PlanDiagnostic
	// ChangedParts 是 EmitPatched/EmitNew/Omit 的 Part 名列表（排序）。
	ChangedParts []PartName
}

// AddedPart 是变更集中新建 Part 的载荷。
type AddedPart struct {
	// Content 是新 Part 的完整字节。
	Content []byte
	// ContentType 是新 Part 的 Override 内容类型；为空时要求该扩展名
	// 已有 Default 覆盖，否则计划失败（不允许产出无类型的 Part）。
	ContentType string
}

// ChangeSet 是一次保存的变更集合（同一变更集生成 CT/关系/条目，§18.2）。
type ChangeSet struct {
	// Patched 是已修改 Part 的新字节（Part 必须已存在）。
	Patched map[PartName][]byte
	// Added 是新建 Part（Part 必须不存在）。
	Added map[PartName]AddedPart
	// Deleted 是删除的 Part（Part 必须已存在）。
	Deleted map[PartName]bool
}

// IsEmpty 报告变更集是否为空（空变更集 → 全量 CopyOriginal，B1 输出）。
func (cs *ChangeSet) IsEmpty() bool {
	return len(cs.Patched) == 0 && len(cs.Added) == 0 && len(cs.Deleted) == 0
}

// BuildSavePlan 校验变更集并产出保存计划。
//
// 错误：ErrMalformedPackage（Part 名非法）、ErrNotFound（修改/删除的
// Part 不存在）、ErrPlanInvalid（新建 Part 已存在、内容类型无法确定、
// 变更集交叉冲突）。悬空关系等非阻断问题进 Diagnostics。
func BuildSavePlan(pk *Package, cs *ChangeSet) (*SavePlan, error) {
	if cs == nil {
		cs = &ChangeSet{}
	}
	// 1) 名称与交叉冲突校验。
	for name := range cs.Patched {
		if !name.Valid() {
			return nil, fmt.Errorf("%w: patched part name %q", ErrMalformedPackage, string(name))
		}
	}
	for name := range cs.Added {
		if !name.Valid() {
			return nil, fmt.Errorf("%w: added part name %q", ErrMalformedPackage, string(name))
		}
	}
	for name := range cs.Deleted {
		if !name.Valid() {
			return nil, fmt.Errorf("%w: deleted part name %q", ErrMalformedPackage, string(name))
		}
	}
	if err := crossCheck(pk, cs); err != nil {
		return nil, err
	}

	// 2) Content Types 与变更集同源再生成（仅增删时；纯补丁不动 CT）。
	ctPlan := pk.ct.clone()
	ctChanged := false
	for name := range cs.Deleted {
		if _, has := ctPlan.overrides[name]; has {
			ctPlan.removeOverride(name)
			ctChanged = true
		}
	}
	addedNames := make([]PartName, 0, len(cs.Added))
	for name := range cs.Added {
		addedNames = append(addedNames, name)
	}
	sort.Slice(addedNames, func(i, j int) bool { return addedNames[i] < addedNames[j] })
	for _, name := range addedNames {
		ap := cs.Added[name]
		if ap.ContentType != "" {
			if err := ctPlan.addOverride(name, ap.ContentType); err != nil {
				return nil, err
			}
			ctChanged = true
			continue
		}
		if _, ok := ctPlan.Lookup(name); !ok {
			return nil, fmt.Errorf("%w: added part %s has no ContentType and no Default covers its extension",
				ErrPlanInvalid, name)
		}
	}

	// 3) 条目规划：既有 Part 逐个决定动作，再追加新建。
	deleted := make(map[PartName]bool, len(cs.Deleted))
	for name := range cs.Deleted {
		deleted[name] = true
		// 连带关系流：删除 Part 时其 .rels 一并 Omit。
		if rp, ok := relsPartOf(name); ok && pk.HasPart(rp) && !deleted[rp] {
			deleted[rp] = true
		}
	}

	entries := make([]PlannedEntry, 0, pk.index.Count()+len(cs.Added)+1)
	changed := make([]PartName, 0, len(cs.Patched)+len(cs.Added))
	for _, name := range pk.PartNames() {
		switch {
		case deleted[name]:
			entries = append(entries, PlannedEntry{Name: name, Action: Omit})
			changed = append(changed, name)
		case name == ContentTypesPartName && ctChanged:
			entries = append(entries, PlannedEntry{
				Name: name, Action: EmitPatched, Content: ctPlan.serialize(),
			})
			changed = append(changed, name)
		default:
			if data, ok := cs.Patched[name]; ok {
				entries = append(entries, PlannedEntry{Name: name, Action: EmitPatched, Content: data})
				changed = append(changed, name)
			} else {
				entries = append(entries, PlannedEntry{Name: name, Action: CopyOriginal})
			}
		}
	}
	for _, name := range addedNames {
		entries = append(entries, PlannedEntry{
			Name: name, Action: EmitNew, Content: cs.Added[name].Content,
		})
		changed = append(changed, name)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	sort.Slice(changed, func(i, j int) bool { return changed[i] < changed[j] })

	plan := &SavePlan{
		Entries:      entries,
		ChangedParts: changed,
	}

	// 4) 非阻断诊断：幸存 Part 的内部关系指向被删除的目标 → 悬空。
	omit := make(map[PartName]bool)
	for _, e := range entries {
		if e.Action == Omit {
			omit[e.Name] = true
		}
	}
	for _, set := range pk.rels {
		for _, rel := range set.All() {
			if rel.Mode == TargetInternal && omit[rel.TargetPart] {
				plan.Diagnostics = append(plan.Diagnostics, PlanDiagnostic{
					Code: "OPC_DANGLING_REL",
					Part: set.Source,
					Message: fmt.Sprintf("relationship %s (type %s) targets deleted part %s",
						rel.ID, rel.Type, rel.TargetPart),
				})
			}
		}
	}
	return plan, nil
}

// crossCheck 校验变更集与包现状的一致性。
func crossCheck(pk *Package, cs *ChangeSet) error {
	for name := range cs.Patched {
		if !pk.HasPart(name) {
			return fmt.Errorf("%w: patched part %s does not exist", ErrNotFound, name)
		}
	}
	for name := range cs.Deleted {
		if !pk.HasPart(name) {
			return fmt.Errorf("%w: deleted part %s does not exist", ErrNotFound, name)
		}
	}
	for name := range cs.Added {
		if pk.HasPart(name) {
			return fmt.Errorf("%w: added part %s already exists", ErrPlanInvalid, name)
		}
	}
	for name := range cs.Patched {
		if cs.Deleted[name] {
			return fmt.Errorf("%w: part %s both patched and deleted", ErrPlanInvalid, name)
		}
		if _, ok := cs.Added[name]; ok {
			return fmt.Errorf("%w: part %s both patched and added", ErrPlanInvalid, name)
		}
	}
	for name := range cs.Added {
		if cs.Deleted[name] {
			return fmt.Errorf("%w: part %s both added and deleted", ErrPlanInvalid, name)
		}
	}
	return nil
}

// Write 执行保存计划：把输出条目写入 ZIP（未变 Part 从源包读取复制）。
//
// B1 语义：CopyOriginal 条目写入的是源包解压内容，逐字节一致；输出条目
// 按 Part 名排序（确定性）。写入完成后输出仍可通过 Load 重新装配。
func (plan *SavePlan) Write(pk *Package, w io.Writer) error {
	zw := zip.NewWriter(w)
	for _, e := range plan.Entries {
		if e.Action == Omit {
			continue
		}
		entry, err := e.Name.EntryName()
		if err != nil {
			return fmt.Errorf("entry name %s: %w", e.Name, err)
		}
		f, err := zw.Create(entry)
		if err != nil {
			return fmt.Errorf("create entry %s: %w", e.Name, err)
		}
		switch e.Action {
		case EmitPatched, EmitNew:
			if _, err := f.Write(e.Content); err != nil {
				return fmt.Errorf("write entry %s: %w", e.Name, err)
			}
		case CopyOriginal:
			data, err := pk.readAll(e.Name)
			if err != nil {
				return fmt.Errorf("copy entry %s: %w", e.Name, err)
			}
			if _, err := f.Write(data); err != nil {
				return fmt.Errorf("write entry %s: %w", e.Name, err)
			}
		default:
			return fmt.Errorf("%w: unknown action %q for %s", ErrPlanInvalid, e.Action, e.Name)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}
	return nil
}

// readAll 读取 Part 全部解压内容（受该 Part 的预算限制）。
func (pk *Package) readAll(name PartName) ([]byte, error) {
	rc, err := pk.index.OpenPart(name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// relsPartOf 返回 Part 的关系流 Part 名；Part 在包根时无关系流返回 ok=false。
func relsPartOf(name PartName) (PartName, bool) {
	entry := string(name)
	if !name.Valid() || entry == "/" {
		return "", false
	}
	i := lastIndexByte(entry, '/')
	dir, base := entry[:i], entry[i+1:] // base 非空（Valid 保证无空段）
	return PartName(dir + "/_rels/" + base + ".rels"), true
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
