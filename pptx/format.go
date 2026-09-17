package pptx

// 本文件是 M3 的 **线条系统**访问器薄委托：解析已迁至
// internal/document/style 的 ParseShapeLine（v2.0 域搬迁）。

import (
	"github.com/F31/go-pptx/v2/internal/document/style"
)

// Line 返回形状的线条（spPr/a:ln）；形状无线条返回 Specified=false。
// 组合（grpSp）等无 spPr 的形状返回空 LineStyle 与 nil 错误。
func (s *shapeNode) Line() (LineStyle, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return LineStyle{}, nil, Annotate(err, "shape.Line")
	}
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return LineStyle{}, nil, Annotate(err, "shape.Line")
	}
	out, diags := style.ParseShapeLine(doc, el, env, s.p.styleDocs(), string(s.part))
	return out, diags, nil
}
