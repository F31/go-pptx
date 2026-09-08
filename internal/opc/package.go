package opc

import (
	"fmt"
	"io"
	"strings"
)

// Package 是 OPC 层的包视图：ZIP 索引 + Content Types + 关系图。
//
// 主 Part 发现走包级 officeDocument 关系，不硬编码 presentation.xml
// 等固定名称（方案 §4.1）。关系图允许循环（如版式 ↔ 母版），遍历
// 用 visited 防护，不假设包图为 DAG。
type Package struct {
	index *Index
	ct    *ContentTypes
	// rels 按源 Part 的查找键（lookupKey）索引；包根关系集合的键是 ""。
	rels map[string]*RelationshipSet
}

// Load 扫描包并解析 Content Types 与全部关系流。
//
// 错误：ErrMalformedPackage（ZIP/Content Types/关系流不合法、缺少
// [Content_Types].xml 或 _rels/.rels）或 ErrLimitExceeded（预算超限）。
func Load(ra io.ReaderAt, size int64, budget Budget) (*Package, error) {
	ix, err := Scan(ra, size, budget)
	if err != nil {
		return nil, err
	}

	ctData, err := readXMLPart(ix, ContentTypesPartName)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedPackage,
			fmt.Errorf("content types part: %v", err))
	}
	ct, err := ParseContentTypes(ctData)
	if err != nil {
		return nil, err
	}

	pk := &Package{
		index: ix,
		ct:    ct,
		rels:  make(map[string]*RelationshipSet),
	}
	for _, name := range ix.PartNames() {
		if !isRelsEntry(name) {
			continue
		}
		src, err := relsSource(name)
		if err != nil {
			return nil, err
		}
		data, err := readXMLPart(ix, name)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalformedPackage,
				fmt.Errorf("relationships part %s: %v", name, err))
		}
		set, err := ParseRelationships(src, data)
		if err != nil {
			return nil, err
		}
		key := lookupKey(src)
		if prev, dup := pk.rels[key]; dup {
			return nil, fmt.Errorf("%w: two relationship parts map to source %s (%s)",
				ErrMalformedPackage, src, prev.Source)
		}
		pk.rels[key] = set
	}
	if _, ok := pk.rels[lookupKey("/")]; !ok {
		return nil, fmt.Errorf("%w: package root relationships part _rels/.rels missing",
			ErrMalformedPackage)
	}
	return pk, nil
}

// HasPart 报告 Part 是否存在于包中。
func (pk *Package) HasPart(name PartName) bool { return pk.index.HasPart(name) }

// PartNames 返回全部 Part 名（排序）。
func (pk *Package) PartNames() []PartName { return pk.index.PartNames() }

// ContentType 返回 Part 的内容类型（Override 优先于 Default）；
// 未命中返回 ok=false。
func (pk *Package) ContentType(name PartName) (string, bool) { return pk.ct.Lookup(name) }

// OpenPart 打开 Part 内容流（按实际字节计数与预算限制）。调用方负责 Close。
func (pk *Package) OpenPart(name PartName) (io.ReadCloser, error) { return pk.index.OpenPart(name) }

// Relationships 返回源 Part 的关系集合；该 Part 没有关系流时返回
// ok=false（OPC 语义：无关系流 = 无关系，不是错误）。
func (pk *Package) Relationships(source PartName) (*RelationshipSet, bool) {
	set, ok := pk.rels[lookupKey(source)]
	return set, ok
}

// MainPart 从包级 officeDocument 关系发现主 Part。
//
// 不假设主 Part 名称；多个 officeDocument 关系时取文档序第一条（确定性），
// 完整一致性诊断由 Validate 层负责。目标缺失或模式为外部时报错。
func (pk *Package) MainPart() (PartName, error) {
	root, ok := pk.rels[lookupKey("/")]
	if !ok {
		return "", fmt.Errorf("%w: package root relationships missing", ErrMalformedPackage)
	}
	for _, rel := range root.All() {
		if rel.Type != RelOfficeDocument {
			continue
		}
		if rel.Mode != TargetInternal {
			return "", fmt.Errorf("%w: officeDocument relationship at package root is external (%q)",
				ErrMalformedPackage, rel.Target)
		}
		if !pk.index.HasPart(rel.TargetPart) {
			return "", fmt.Errorf("%w: officeDocument target %s missing in package",
				ErrMalformedPackage, rel.TargetPart)
		}
		return rel.TargetPart, nil
	}
	return "", fmt.Errorf("%w: no officeDocument relationship at package root", ErrMalformedPackage)
}

// RelatedParts 返回源 Part 上指定类型关系的全部内部目标（文档序，可
// 为 nil）。不校验目标是否存在；存在性由调用方或 Validate 层处理。
func (pk *Package) RelatedParts(source PartName, relType string) []PartName {
	set, ok := pk.rels[lookupKey(source)]
	if !ok {
		return nil
	}
	var out []PartName
	for _, rel := range set.All() {
		if rel.Type == relType && rel.Mode == TargetInternal {
			out = append(out, rel.TargetPart)
		}
	}
	return out
}

// RelatedByIDs 返回源 Part 上指定类型关系的 (rId, 目标) 对（文档序），
// 供需要按 rId 回填 XML 引用的层（textmap/edit）使用。
func (pk *Package) RelatedByIDs(source PartName, relType string) [][2]string {
	set, ok := pk.rels[lookupKey(source)]
	if !ok {
		return nil
	}
	var out [][2]string
	for _, rel := range set.All() {
		if rel.Type == relType && rel.Mode == TargetInternal {
			out = append(out, [2]string{rel.ID, string(rel.TargetPart)})
		}
	}
	return out
}

// Walk 从 start 出发深度优先遍历内部关系图。visit 对每条关系调用一次
// （含外部关系，外部不展开）；对同一 Part 的关系集合只展开一次——循环
// （如版式 ↔ 母版）安全终止，不假设 DAG（方案 §4.1）。visit 返回错误
// 即中止遍历。
func (pk *Package) Walk(start PartName, visit func(source PartName, rel *Relationship) error) error {
	visited := make(map[PartName]bool)
	var walk func(src PartName) error
	walk = func(src PartName) error {
		if visited[src] {
			return nil
		}
		visited[src] = true
		set, ok := pk.rels[lookupKey(src)]
		if !ok {
			return nil
		}
		for _, rel := range set.All() {
			if err := visit(src, rel); err != nil {
				return err
			}
			if rel.Mode == TargetInternal {
				if err := walk(rel.TargetPart); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(start)
}

// readXMLPart 读取 XML 类 Part 全部字节（受 MaxXMLBytes 预算约束）。
func readXMLPart(ix *Index, name PartName) ([]byte, error) {
	rc, err := ix.OpenXMLPart(name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// isRelsEntry 报告条目是否为关系流：路径含 "_rels" 段且以 ".rels" 结尾
// （根包 "_rels/.rels" 或 "<dir>/_rels/<base>.rels"）。
func isRelsEntry(name PartName) bool {
	entry := string(name)
	if !strings.HasSuffix(entry, ".rels") {
		return false
	}
	for _, seg := range strings.Split(entry, "/") {
		if seg == "_rels" {
			return true
		}
	}
	return false
}

// relsSource 由关系流条目名推出其源 Part："/a/b/_rels/c.xml.rels" →
// "/a/b/c.xml"；"/_rels/.rels" → 包根 "/"。
func relsSource(name PartName) (PartName, error) {
	entry, err := name.EntryName()
	if err != nil {
		return "", err
	}
	base := entry[:len(entry)-len(".rels")]
	if i := strings.LastIndex(base, "/_rels/"); i >= 0 {
		src := "/" + base[:i] + "/" + base[i+len("/_rels/"):]
		return PartName(src), nil
	}
	if base == "_rels/" {
		// 根包关系流 "_rels/.rels"：源是包根本身。
		return "/", nil
	}
	return "", fmt.Errorf("%w: unrecognised relationships entry %q", ErrMalformedPackage, entry)
}
