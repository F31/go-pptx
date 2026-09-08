package xmlstore

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"
)

// 补丁引擎：对一份 XML 的原始字节做基于 span 的区间替换（方案 §18.1
// "已修改 Part 精确补丁优先"、§19.1 事务中的"节点补丁"）。
//
// 核心不变量：
//   - 补丁集合先整体校验（范围、重叠、锚定）再一次性应用，任一失败则
//     整体失败、原始数据不被修改（调用方拿到 error 时返回值不可用）；
//   - 应用按 Start 降序执行，避免区间偏移漂移；
//   - 替换内容必须由调用方经 EscapeText/EscapeAttrValue 转义（本文档
//     不隐式转义，防止双重转义语义歧义）。

var (
	// ErrPatchOverlap 表示补丁区间相互重叠（含相同区间重复声明）。
	ErrPatchOverlap = errors.New("xmlstore: patch ranges overlap")
	// ErrPatchRange 表示补丁区间越界或 Start > End。
	ErrPatchRange = errors.New("xmlstore: patch range out of bounds")
	// ErrPatchAnchor 表示锚定期望内容与实际字节不符（基线漂移防护）。
	ErrPatchAnchor = errors.New("xmlstore: patch anchor mismatch")
	// ErrEscapeInvalidRune 表示待转义内容含 XML 1.0 不允许的字符。
	ErrEscapeInvalidRune = errors.New("xmlstore: content contains characters not allowed in XML 1.0")
)

// PatchError 定位出错的补丁（序号在补丁集合中的原始位置）与原因。
type PatchError struct {
	// Index 是出错补丁在调用方切片中的下标。
	Index int
	// Op 是失败阶段："range" / "overlap" / "anchor" / "escape"。
	Op  string
	Err error
}

func (e *PatchError) Error() string {
	return fmt.Sprintf("xmlstore: patch[%d] %s: %v", e.Index, e.Op, e.Err)
}

// Unwrap 使 errors.Is(err, ErrPatch*) 成立。
func (e *PatchError) Unwrap() error { return e.Err }

// SpanPatch 描述一次区间替换：把原始字节 [Start, End) 替换为 Replacement。
// 区间为半开且按 UTF-8 原始字节计数（与 ByteRange 一致）。
//
// Expect 非空时为锚定校验：应用前要求 data[Start:End] 与 Expect 完全一致，
// 用于防范"补丁构造后、应用前"文档被并发修改（方案 §19.1 revision 语义
// 的最小实现）；为空表示不校验。
//
// Desc 仅用于诊断与冲突报告，不参与逻辑。
type SpanPatch struct {
	Start       int
	End         int
	Replacement []byte
	Expect      []byte
	Desc        string
}

// Size 返回替换造成的字节长度增量（负值为收缩）。
func (p SpanPatch) Size() int { return len(p.Replacement) - (p.End - p.Start) }

// validateRange 校验单个补丁的范围与锚定（相对 data）。
func (p SpanPatch) validateRange(data []byte) error {
	if p.Start < 0 || p.End > len(data) || p.Start > p.End {
		return fmt.Errorf("%w: range [%d,%d) outside document (%d bytes)",
			ErrPatchRange, p.Start, p.End, len(data))
	}
	if p.Expect != nil && !bytes.Equal(data[p.Start:p.End], p.Expect) {
		return fmt.Errorf("%w: expect %d bytes, got %d bytes at [%d,%d)",
			ErrPatchAnchor, len(p.Expect), p.End-p.Start, p.Start, p.End)
	}
	return nil
}

// validateSet 对补丁集合做统一校验：范围、锚定与两两重叠。返回按 Start
// 升序排列的下标视图（不改变调用方切片顺序，错误报告仍用原始下标）。
func validateSet(data []byte, patches []SpanPatch) ([]int, error) {
	order := make([]int, len(patches))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return patches[order[a]].Start < patches[order[b]].Start
	})
	for _, i := range order {
		if err := patches[i].validateRange(data); err != nil {
			return nil, &PatchError{Index: i, Op: opOf(err), Err: err}
		}
	}
	for k := 1; k < len(order); k++ {
		prev, cur := patches[order[k-1]], patches[order[k]]
		if cur.Start < prev.End {
			return nil, &PatchError{
				Index: order[k],
				Op:    "overlap",
				Err: fmt.Errorf("%w: patch[%d] [%d,%d) overlaps patch[%d] [%d,%d)",
					ErrPatchOverlap, order[k], cur.Start, cur.End,
					order[k-1], prev.Start, prev.End),
			}
		}
	}
	return order, nil
}

func opOf(err error) string {
	switch {
	case errors.Is(err, ErrPatchAnchor):
		return "anchor"
	default:
		return "range"
	}
}

// ApplyPatches 校验并一次性应用补丁集合，返回新字节切片。
//
// 语义：
//   - 空集合返回 data 本身（不拷贝）；
//   - 校验失败返回 PatchError，且不产生部分应用；
//   - 成功返回新切片（新建缓冲升序回放，与"降序原地替换"等价——新
//     缓冲不受偏移漂移影响；集合先按 Start 升序排序并校验不重叠，
//     保证任意替换顺序下结果一致），调用方可以 Index 重建索引。
func ApplyPatches(data []byte, patches []SpanPatch) ([]byte, error) {
	if len(patches) == 0 {
		return data, nil
	}
	order, err := validateSet(data, patches)
	if err != nil {
		return nil, err
	}
	delta := 0
	for _, p := range patches {
		delta += p.Size()
	}
	out := make([]byte, 0, len(data)+delta)
	pos := 0
	for _, k := range order { // order 按 Start 升序
		p := patches[k]
		out = append(out, data[pos:p.Start]...)
		out = append(out, p.Replacement...)
		pos = p.End
	}
	out = append(out, data[pos:]...)
	return out, nil
}

// ---- 转义 ----

// EscapeText 将文本节点内容转义为可嵌入 XML 的字节：&、<、> 实体化，
// 其余字符（含 \t\n\r 与全部非 ASCII）原样保留。出现 XML 1.0 禁止的
// 字符（多数 C0 控制符、U+FFFE/U+FFFF 等）返回 ErrEscapeInvalidRune，
// 由调用方决定清洗或拒绝——本包不做静默替换（方案 §19.3：不能在未知
// 情况下转码后仍声称词法保真）。
func EscapeText(s string) (string, error) {
	if !stringsContainsAny(s, "&<>") {
		if err := checkXMLChars(s); err != nil {
			return "", err
		}
		return s, nil
	}
	var b []byte
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("%w: invalid UTF-8 at byte %d", ErrEscapeInvalidRune, i)
		}
		switch r {
		case '&':
			b = append(b, "&amp;"...)
		case '<':
			b = append(b, "&lt;"...)
		case '>':
			b = append(b, "&gt;"...)
		default:
			if err := checkXMLRune(r, i); err != nil {
				return "", err
			}
			b = append(b, s[i:i+size]...)
		}
		i += size
	}
	return string(b), nil
}

// EscapeAttrValue 将属性值转义：在文本规则之上额外转义引号。quote 是
// 包裹该值的引号字符（'"' 或 '\”，应与目标位置实际引号一致），对应的
// 引号被实体化，另一侧与空白原样保留（空白属性语义：值内空白不折叠、
// 不裁剪，由引号定界保证逐字保真）。
func EscapeAttrValue(s string, quote byte) (string, error) {
	if quote != '"' && quote != '\'' {
		return "", fmt.Errorf("xmlstore: invalid attribute quote %q", quote)
	}
	if !stringsContainsAny(s, "&<>") && !stringsContainsByte(s, quote) {
		if err := checkXMLChars(s); err != nil {
			return "", err
		}
		return s, nil
	}
	var b []byte
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("%w: invalid UTF-8 at byte %d", ErrEscapeInvalidRune, i)
		}
		switch r {
		case '&':
			b = append(b, "&amp;"...)
		case '<':
			b = append(b, "&lt;"...)
		case '>':
			b = append(b, "&gt;"...)
		case '"':
			if quote == '"' {
				b = append(b, "&quot;"...)
			} else {
				b = append(b, '"')
			}
		case '\'':
			if quote == '\'' {
				b = append(b, "&apos;"...)
			} else {
				b = append(b, '\'')
			}
		default:
			if err := checkXMLRune(r, i); err != nil {
				return "", err
			}
			b = append(b, s[i:i+size]...)
		}
		i += size
	}
	return string(b), nil
}

// checkXMLChars 校验整串不含 XML 1.0 禁止字符。
func checkXMLChars(s string) error {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("%w: invalid UTF-8 at byte %d", ErrEscapeInvalidRune, i)
		}
		if err := checkXMLRune(r, i); err != nil {
			return err
		}
		i += size
	}
	return nil
}

// checkXMLRune 按 XML 1.0 Char 产生式校验单个字符：
// #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF]。
func checkXMLRune(r rune, at int) error {
	valid := r == 0x09 || r == 0x0A || r == 0x0D ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= utf8.MaxRune)
	if !valid {
		return fmt.Errorf("%w: U+%04X at byte %d", ErrEscapeInvalidRune, r, at)
	}
	return nil
}

func stringsContainsAny(s, chars string) bool {
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				return true
			}
		}
	}
	return false
}

func stringsContainsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// ---- 便捷构造 ----

// NewTextPatch 构造文本节点内容替换补丁：将 [start, end) 的原文替换为
// text 经 EscapeText 转义后的字节。start/end 通常取 Scanner 的
// TokenText span 或 textmap 层映射结果。
func NewTextPatch(start, end int, text string) (SpanPatch, error) {
	esc, err := EscapeText(text)
	if err != nil {
		return SpanPatch{}, &PatchError{Op: "escape", Err: err}
	}
	return SpanPatch{Start: start, End: end, Replacement: []byte(esc)}, nil
}

// SetAttrValuePatch 构造属性值替换补丁：定位 n 上首个 Namespace==ns 且
// 本地名为 local 的属性，替换其引号内原始值。引号从原文探测（保持
// 原文档的引号风格），新值经 EscapeAttrValue 转义；原值经解码后作为
// Expect 锚定，防止构造与应用之间文档漂移。
//
// ns 传 "" 匹配无前缀属性；找不到属性返回 PatchError（Op:"range"）。
func SetAttrValuePatch(d *XMLDocument, n *NodeRecord, ns, local, value string) (SpanPatch, error) {
	idx := -1
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Namespace == ns && splitQName(a.RawName).Local == local {
			idx = i
			break
		}
	}
	if idx < 0 {
		return SpanPatch{}, &PatchError{
			Op:  "range",
			Err: fmt.Errorf("attribute {%s}%s not found on <%s>", ns, local, n.Name()),
		}
	}
	a := &n.Attrs[idx]
	if a.ValueStart <= 0 || a.ValueStart > a.ValueEnd || a.ValueEnd > len(d.Original()) {
		return SpanPatch{}, &PatchError{
			Op:  "range",
			Err: fmt.Errorf("attribute {%s}%s has invalid value span [%d,%d)", ns, local, a.ValueStart, a.ValueEnd),
		}
	}
	quote := d.Original()[a.ValueStart-1] // 引号紧邻值区间之前
	if quote != '"' && quote != '\'' {
		return SpanPatch{}, &PatchError{
			Op:  "range",
			Err: fmt.Errorf("attribute {%s}%s value not quoted as expected", ns, local),
		}
	}
	esc, err := EscapeAttrValue(value, quote)
	if err != nil {
		return SpanPatch{}, &PatchError{Op: "escape", Err: err}
	}
	return SpanPatch{
		Start:       a.ValueStart,
		End:         a.ValueEnd,
		Replacement: []byte(esc),
		Expect:      d.Original()[a.ValueStart:a.ValueEnd],
		Desc:        fmt.Sprintf("set attr {%s}%s on <%s>", ns, local, n.Name()),
	}, nil
}
