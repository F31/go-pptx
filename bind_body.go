package pptx

import (
	"fmt"
	"github.com/F31/go-pptx/internal/bind"
	"strings"
)

// 本文件是模板绑定的**形状扫描与正文绑定**：递归扫描形状/组（scanShapes）、
// 段落级绑定（bindBody）、空段删除（applyDeletions）与单段内联替换（substitute）。
// 表格绑定见 bindtable.go，图表见 bindchart.go。

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
		if kind, path, ok := bind.ParseDirective(trimmed); ok {
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
						"unmatched %s/if%s in shape %q", bind.TplMarkOpen, bind.TplMarkClose, label), Err: ErrInvalidArgument}
				}
				stack = stack[:len(stack)-1]
			case "each", "endeach":
				return &OperationError{Op: op, Message: fmt.Sprintf(
					"%s#each%s marker in shape %q must live in a table row cell", bind.TplMarkOpen, bind.TplMarkClose, label),
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
			"unclosed %s#if%s block in shape %q", bind.TplMarkOpen, bind.TplMarkClose, label), Err: ErrInvalidArgument}
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
			if p := bind.EmptyParaPatch(doc, node); p != nil {
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
	toks := bind.ScanInline(text)
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
		if seen[tok.Literal] {
			continue
		}
		seen[tok.Literal] = true
		v, found, err := s.resolve(scope, tok.Path)
		if err != nil {
			return err
		}
		if !found {
			if s.o.strict {
				return &OperationError{Op: op, Message: fmt.Sprintf(
					"placeholder %q not found in data (shape %q)", tok.Path, label), Err: ErrInvalidArgument}
			}
			s.diag("bind.key_missing", string(para.part),
				fmt.Sprintf("placeholder %q missing; left as-is", tok.Path))
			continue
		}
		rep, ok := formatBindValue(v)
		if !ok {
			return &OperationError{Op: op, Message: fmt.Sprintf(
				"placeholder %q has unsupported value type %T", tok.Path, v), Err: ErrInvalidArgument}
		}
		// 实际补丁在 apply 阶段生成（同段落多占位符需逐个重新索引）。
		s.addSub(para.part, offset, path, tok.Literal, rep)
	}
	return nil
}
