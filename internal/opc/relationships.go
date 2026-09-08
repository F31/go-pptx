package opc

import (
	"fmt"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// 关系集合（.rels）解析与关系目标解析。
//
// 规则（方案 §4.1 / OPC ECMA-376 Part 2）：
//   - rId 在单个源 Part 的关系集合内唯一；
//   - 内部目标按源 Part URI 解析（允许 "../media/..."），解析后越出包根
//     的路径拒绝；
//   - TargetMode=External 保留原始 Target，不解析为 Part、不下载；
//   - TargetMode 缺省为 Internal；其他值拒绝。

// NsRelationships 是关系流的 XML 命名空间。
const NsRelationships = "http://schemas.openxmlformats.org/package/2006/relationships"

// 常用关系类型 URI（OPC-02/M1 需要的子集；全部为
// http://schemas.openxmlformats.org/officeDocument/2006/relationships/ 前缀）。
const (
	RelOfficeDocument = RelTypePrefix + "officeDocument"
	RelSlide          = RelTypePrefix + "slide"
	RelSlideLayout    = RelTypePrefix + "slideLayout"
	RelSlideMaster    = RelTypePrefix + "slideMaster"
	RelTheme          = RelTypePrefix + "theme"
	RelNotesSlide     = RelTypePrefix + "notesSlide"
)

// RelTypePrefix 是 officeDocument 关系类型的公共前缀。
const RelTypePrefix = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"

// TargetMode 区分包内目标与外部目标。
type TargetMode string

const (
	// TargetInternal 目标是包内 Part（Relationship.TargetPart 已解析）。
	TargetInternal TargetMode = "Internal"
	// TargetExternal 目标在包外（Target 为原样 URI/路径，不下载）。
	TargetExternal TargetMode = "External"
)

// Relationship 是单条关系记录。
type Relationship struct {
	// ID 是源 Part 关系集合内的 rId（如 "rId7"）。
	ID string
	// Type 是关系类型 URI。
	Type string
	// Target 是 Target 属性原样值（内部与外部均保留原文）。
	Target string
	// Mode 是目标模式；缺省 Internal。
	Mode TargetMode
	// TargetPart 仅 Internal 有效：Target 按源 Part 解析后的包内名称。
	// 外部关系为空值。
	TargetPart PartName
}

// RelationshipSet 是一个源 Part 的关系集合（文档序）。
type RelationshipSet struct {
	// Source 是关系所属 Part（包根关系集合为 "/"）。
	Source PartName
	rels   []*Relationship
	byID   map[string]*Relationship
}

// ParseRelationships 解析源 Part（或包根）的关系流。
//
// 错误：ErrMalformedPackage（结构/命名空间不符、必填属性缺失、rId 重复、
// TargetMode 非法、内部目标解析失败或越出包根）。
func ParseRelationships(source PartName, data []byte) (*RelationshipSet, error) {
	doc, err := xmlstore.Index(data)
	if err != nil {
		return nil, fmt.Errorf("%w: relationships of %s: %v", ErrMalformedPackage, source, err)
	}
	root := doc.Root()
	if root.Namespace != NsRelationships || root.Local() != "Relationships" {
		return nil, fmt.Errorf("%w: relationships root of %s is {%s}%s",
			ErrMalformedPackage, source, root.Namespace, root.Local())
	}

	set := &RelationshipSet{
		Source: source,
		byID:   make(map[string]*Relationship),
	}
	for _, id := range root.Children {
		n := doc.Node(id)
		if n.Namespace != NsRelationships || n.Local() != "Relationship" {
			continue // 未知子元素忽略
		}
		relID, _ := n.Attr("", "Id")
		relType, _ := n.Attr("", "Type")
		target, _ := n.Attr("", "Target")
		if relID == "" || relType == "" || target == "" {
			return nil, fmt.Errorf("%w: Relationship of %s missing Id/Type/Target",
				ErrMalformedPackage, source)
		}
		mode := TargetInternal
		if raw, ok := n.Attr("", "TargetMode"); ok {
			switch TargetMode(raw) {
			case TargetInternal, TargetExternal:
				mode = TargetMode(raw)
			default:
				return nil, fmt.Errorf("%w: relationship %q of %s has invalid TargetMode %q",
					ErrMalformedPackage, relID, source, raw)
			}
		} else if hasURIScheme(target) {
			// OPC 惯例：Target 是带 scheme 的绝对 URI 而未显式声明
			// TargetMode 时，按 External 处理（真实包中超链接常省略该属性）。
			mode = TargetExternal
		}

		rel := &Relationship{ID: relID, Type: relType, Target: target, Mode: mode}
		if mode == TargetInternal {
			pn, err := resolveTarget(source, target)
			if err != nil {
				return nil, err
			}
			rel.TargetPart = pn
		}

		if prev, dup := set.byID[relID]; dup {
			return nil, fmt.Errorf("%w: duplicate relationship Id %q of %s (%q, %q)",
				ErrMalformedPackage, relID, source, prev.Target, target)
		}
		set.byID[relID] = rel
		set.rels = append(set.rels, rel)
	}
	return set, nil
}

// All 按文档序返回全部关系（内部切片只读约定，勿修改）。
func (s *RelationshipSet) All() []*Relationship { return s.rels }

// ByID 按 rId 查找关系。
func (s *RelationshipSet) ByID(id string) (*Relationship, bool) {
	r, ok := s.byID[id]
	return r, ok
}

// Len 返回关系条数。
func (s *RelationshipSet) Len() int { return len(s.rels) }

// hasURIScheme 报告 target 是否为带 scheme 的绝对 URI
// （RFC 3986：scheme = ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ) ":"，
// 且 ":" 前不含 "/"）。
func hasURIScheme(target string) bool {
	for i := 0; i < len(target); i++ {
		c := target[i]
		switch {
		case c == ':':
			return i > 0
		case c == '/':
			return false
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return false
}

// resolveTarget 把内部关系 Target 按源 Part 解析为包内 PartName。
//
// 规则：以 "/" 开头为绝对 Part 名（直接校验）；否则相对源 Part 所在目录
// 逐段解析，"." 段消除，".." 段上跳；在包根继续上跳即拒绝（越出包根，
// 方案 §4.1）。空段（"//"）跳过。解析前对百分号编码做最小解码。
func resolveTarget(source PartName, target string) (PartName, error) {
	if strings.HasPrefix(target, "/") {
		pn := PartName(target)
		if !pn.Valid() {
			return "", fmt.Errorf("%w: absolute relationship target %q is not a valid part name",
				ErrMalformedPackage, target)
		}
		return pn, nil
	}

	path, err := unescapePercent(target)
	if err != nil {
		return "", fmt.Errorf("%w: relationship target %q of %s: %v",
			ErrMalformedPackage, target, source, err)
	}

	base := string(source) // "/ppt/slides/slide1.xml"
	// 源为包根 "/" 时，相对目标直接位于根下。
	var stack []string
	if trimmed := strings.Trim(base, "/"); trimmed != "" {
		stack = strings.Split(trimmed, "/")
		// 源 Part 的最后一段是文件名，相对目标基于其目录。
		stack = stack[:len(stack)-1]
	}
	for _, seg := range strings.Split(path, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(stack) == 0 {
				return "", fmt.Errorf("%w: relationship target %q of %s escapes package root",
					ErrMalformedPackage, target, source)
			}
			stack = stack[:len(stack)-1]
		default:
			stack = append(stack, seg)
		}
	}
	pn := PartName("/" + strings.Join(stack, "/"))
	if !pn.Valid() {
		return "", fmt.Errorf("%w: resolved relationship target %q of %s is not a valid part name",
			ErrMalformedPackage, target, source)
	}
	return pn, nil
}

// unescapePercent 最小百分号解码：%XX（十六进制）→ 字节；非法序列报错。
// 交叉编码、超范围 UTF-8 由调用方结构校验兜底。
func unescapePercent(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("incomplete percent escape at offset %d", i)
		}
		hi, ok1 := hexVal(s[i+1])
		lo, ok2 := hexVal(s[i+2])
		if !ok1 || !ok2 {
			return "", fmt.Errorf("invalid percent escape at offset %d", i)
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String(), nil
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
