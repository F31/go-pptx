package opc

import (
	"errors"
	"fmt"
	"strings"
)

// PartName 是包内 Part 的逻辑名称（OPC 规范风格，以 "/" 开头），
// 与磁盘路径和 ZIP 条目名分离（方案 §4.1）。
type PartName string

// ContentTypesPartName 是内容类型流的标准 Part 名。发现逻辑不依赖
// 硬编码路径（走 officeDocument 关系），此处仅作为已知常量。
const ContentTypesPartName PartName = "/[Content_Types].xml"

func (n PartName) String() string { return string(n) }

// Valid 报告 PartName 是否合法。
func (n PartName) Valid() bool {
	s := string(n)
	if len(s) == 0 || s[0] != '/' {
		return false
	}
	if strings.Contains(s, "\\") || strings.Contains(s, "//") {
		return false
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." || seg == "." {
			return false
		}
	}
	return true
}

// EntryName 返回对应的 ZIP 条目名（去掉前导 "/"）。
func (n PartName) EntryName() (string, error) {
	if !n.Valid() {
		return "", fmt.Errorf("%w: invalid part name %q", ErrMalformedPackage, string(n))
	}
	return strings.TrimPrefix(string(n), "/"), nil
}

// validateEntryName 校验 ZIP 条目名（用于包输入安全，L0）。
//
// 拒绝：绝对路径、反斜杠、"."/".." 段、空名。目录条目（以 "/" 结尾）允许
// 出现但不会被当作 Part（见 Index.Scan）。
func validateEntryName(name string) error {
	if name == "" {
		return errors.New("empty entry name")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("absolute entry name %q", name)
	}
	if strings.Contains(name, "\\") {
		return fmt.Errorf("backslash in entry name %q", name)
	}
	trimmed := strings.TrimSuffix(name, "/")
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." {
			return fmt.Errorf("path traversal in entry name %q", name)
		}
		if seg == "." {
			return fmt.Errorf("dot segment in entry name %q", name)
		}
		if seg == "" {
			return fmt.Errorf("empty segment in entry name %q", name)
		}
	}
	return nil
}

// PartNameFromEntry 将合法 ZIP 条目名转换为 PartName。
func PartNameFromEntry(entry string) (PartName, error) {
	if err := validateEntryName(entry); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMalformedPackage, err)
	}
	if strings.HasSuffix(entry, "/") {
		return "", fmt.Errorf("%w: directory entry %q is not a part", ErrMalformedPackage, entry)
	}
	return PartName("/" + entry), nil
}

// isXMLName 报告条目名是否为 XML Part（内容类型流与 .rels 均按 XML 预算限制）。
func isXMLName(entry string) bool {
	lower := strings.ToLower(entry)
	return strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".rels")
}
