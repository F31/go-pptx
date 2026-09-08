package opc

import (
	"fmt"
	"strings"

	"github.com/F31/go-pptx/internal/xmlstore"
)

// Content Types（[Content_Types].xml）解析：Override（按 Part 精确覆盖）
// 优先于 Default（按扩展名兜底），查找顺序不可颠倒（方案 §4.1：
// "Content Type 先查对应 Part 的 Override，再查扩展名 Default；不能只
// 检查扩展名是否出现"）。
//
// 只做读取视图；写侧（新建/改动的 Content Types 再生成）属于 SAVE-01，
// 必须与关系集合基于同一变更集生成（方案 §18.2）。

// NsContentTypes 是 Content Types 流的 XML 命名空间。
const NsContentTypes = "http://schemas.openxmlformats.org/package/2006/content-types"

// ContentTypes 是已解析的包内容类型表。
type ContentTypes struct {
	overrides map[PartName]string
	// lowerOverrides 服务于大小写不一致的兜底匹配（Windows 工具产出的
	// 包偶发 Override 与实际 Part 大小写不一致）；精确匹配始终优先。
	lowerOverrides map[string]string
	defaults       map[string]string
}

// ParseContentTypes 解析 Content Types 字节流。
//
// 错误：ErrMalformedPackage（非 UTF-8/结构非法、根元素不符、重复
// Override/Default、必填属性缺失）。未知子元素被忽略（容错读取，
// 结构问题由 Validate 层诊断，不在读取路径拒绝整个包）。
func ParseContentTypes(data []byte) (*ContentTypes, error) {
	doc, err := xmlstore.Index(data)
	if err != nil {
		return nil, fmt.Errorf("%w: content types: %v", ErrMalformedPackage, err)
	}
	root := doc.Root()
	if root.Namespace != NsContentTypes || root.Local() != "Types" {
		return nil, fmt.Errorf("%w: content types root is {%s}%s, want {%s}Types",
			ErrMalformedPackage, root.Namespace, root.Local(), NsContentTypes)
	}

	ct := &ContentTypes{
		overrides:      make(map[PartName]string),
		lowerOverrides: make(map[string]string),
		defaults:       make(map[string]string),
	}
	for _, id := range root.Children {
		n := doc.Node(id)
		if n.Namespace != NsContentTypes {
			continue
		}
		switch n.Local() {
		case "Default":
			ext, _ := n.Attr("", "Extension")
			ctType, _ := n.Attr("", "ContentType")
			if ext == "" || ctType == "" {
				return nil, fmt.Errorf("%w: Default element missing Extension/ContentType",
					ErrMalformedPackage)
			}
			key := strings.ToLower(ext)
			if prev, dup := ct.defaults[key]; dup {
				return nil, fmt.Errorf("%w: duplicate Default for extension %q (%q, %q)",
					ErrMalformedPackage, key, prev, ctType)
			}
			ct.defaults[key] = ctType
		case "Override":
			part, _ := n.Attr("", "PartName")
			ctType, _ := n.Attr("", "ContentType")
			pn := PartName(part)
			if part == "" || ctType == "" || !pn.Valid() {
				return nil, fmt.Errorf("%w: Override has invalid PartName %q or missing ContentType",
					ErrMalformedPackage, part)
			}
			if prev, dup := ct.overrides[pn]; dup {
				return nil, fmt.Errorf("%w: duplicate Override for part %s (%q, %q)",
					ErrMalformedPackage, pn, prev, ctType)
			}
			ct.overrides[pn] = ctType
			ct.lowerOverrides[lookupKey(pn)] = ctType
		default:
			// 未知子元素：忽略。
		}
	}
	return ct, nil
}

// Lookup 返回 Part 的内容类型。顺序：Override 精确匹配 → Override
// 大小写兜底 → Default（扩展名，大小写不敏感）。三处都未命中返回
// ok=false（可能意味着包损坏，由调用方决定诊断级别）。
func (ct *ContentTypes) Lookup(name PartName) (string, bool) {
	if !name.Valid() {
		return "", false
	}
	if ctType, ok := ct.overrides[name]; ok {
		return ctType, true
	}
	if ctType, ok := ct.lowerOverrides[lookupKey(name)]; ok {
		return ctType, true
	}
	ext := extensionOf(string(name))
	if ext == "" {
		return "", false
	}
	ctType, ok := ct.defaults[strings.ToLower(ext)]
	return ctType, ok
}

// extensionOf 返回 Part 名的扩展名（不含点，保留原大小写）；无扩展名
// 或最后一段以点开头（隐藏文件风格）返回空。
func extensionOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 || i == len(name)-1 {
		return ""
	}
	seg := name[i+1:]
	if strings.Contains(seg, "/") {
		return ""
	}
	return seg
}
