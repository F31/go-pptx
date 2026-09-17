package pptx

// 本文件是 M3 的 **主题样式矩阵**访问器薄委托：解析已迁至
// internal/document/style 的 ParseStyleMatrixRefs（v2.0 域搬迁）。

import (
	"github.com/F31/go-pptx/internal/document/style"
)

// StyleMatrixRefs 返回形状的样式矩阵引用链（a:spPr/a:style）。
// 主题 fmtScheme 缺失或下标越界时输出诊断并保持 Resolved=false。
func (s *shapeNode) StyleMatrixRefs() ([]StyleMatrixRef, []Diagnostic, error) {
	doc, el, err := s.locate()
	if err != nil {
		return nil, nil, Annotate(err, "shape.StyleMatrixRefs")
	}
	env, err := s.p.styleEnv(s.part)
	if err != nil {
		return nil, nil, Annotate(err, "shape.StyleMatrixRefs")
	}
	out, diags := style.ParseStyleMatrixRefs(doc, el, env, s.p.styleDocs(), string(s.part))
	return out, diags, nil
}
