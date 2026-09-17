package pptx

// 本文件是 M3 的 **Run 高级属性**访问器薄委托：解析已迁至
// internal/document/style 的 ParseRunProps（v2.0 域搬迁）。

import (
	"github.com/F31/go-pptx/v2/internal/document/style"
)

// AdvancedProps 返回 Run 的高级字符属性（§2.3 矩阵"Run 高级属性"
// 起步解析）：baseline、spc、highlight、caps、strike、u、lang、sym 等。
// 未知项经 Unknown 输出，不臆造取值。
func (r *TextRun) AdvancedProps() (RunProps, []Diagnostic, error) {
	doc, run, err := r.locateRun()
	if err != nil {
		return RunProps{}, nil, Annotate(err, "TextRun.AdvancedProps")
	}
	rPr := childOfKind(doc, run, nsDrawingML, "rPr", 0)
	if rPr == nil {
		return RunProps{}, nil, nil
	}
	env, err := r.p.styleEnv(r.part)
	if err != nil {
		return RunProps{}, nil, Annotate(err, "TextRun.AdvancedProps")
	}
	out, diags := style.ParseRunProps(doc, rPr, env, r.p.styleDocs(), string(r.part))
	return out, diags, nil
}
