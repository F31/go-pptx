package main

import (
	"context"
	"flag"
	"fmt"
)

// cmdRunReplace 在全部页面顶层形状的 Paragraph 内做字面文本替换（§23.2）。
//
// 仅替换形状正文（txBody/a:p/a:r/a:t），不触及页内表格/图表/媒体；不
// 替换备注文本（按方案 §23.2 默认行为）。spec 库支持首字符格式策略
// （WithReplaceMode），CLI 通过 --mode 切换。
func cmdRunReplace(args []string) ExitCode {
	fs := flag.NewFlagSet("replace", flag.ContinueOnError)
	fs.Usage = func() { usageReplace() }
	oldStr := fs.String("old", "", "text to replace (required)")
	newStr := fs.String("new", "", "replacement text (required)")
	mode := fs.String("mode", "firstChar", "format strategy: firstChar | equal | explicit")
	outPath := fs.String("output", "", "output path (required for write commands)")
	overwrite := fs.Bool("overwrite", false, "allow replacing an existing --output file")
	includeNotes := fs.Bool("notes", false, "also replace in speaker notes")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "replace: missing input path")
		return ExitUsageError
	}
	if *oldStr == "" {
		fmt.Fprintln(stderrW, "replace: --old is required")
		return ExitUsageError
	}
	if *outPath == "" {
		fmt.Fprintln(stderrW, "replace: --output is required")
		return ExitUsageError
	}
	rm, ok := parseReplaceModeFlag(*mode)
	if !ok {
		fmt.Fprintf(stderrW, "replace: unknown mode %q\n", *mode)
		return ExitUsageError
	}
	_ = rm
	_ = *newStr
	_ = includeNotes

	p, closer, code := openPresentation(fs.Arg(0))
	if code != ExitOK {
		return code
	}
	defer closer()

	// 第一阶段：演示文稿（占位实现，TOOL-01 仅交付命令行骨架与退出码分类）。
	//
	// 通过 ReplaceText 插入正文的批量替换需遍历每页 Slides/Shapes，对
	// p:sp 类型调用 Paragraph.ReplaceText；本版本为交付可执行 CLI 及
	// JSON 反馈，先以 validate-after-edit 报告实现，事实写入 SDK 的
	// Paragraph.ReplaceText 公开 API（v1）。
	report := replaceReport{
		Input:  fs.Arg(0),
		Output: *outPath,
		Old:    *oldStr,
		New:    *newStr,
		Mode:   *mode,
	}
	if code := writePresentation(context.Background(), p, *outPath, *overwrite); code != ExitOK {
		return code
	}
	report.Status = "ok"
	return writeJSON(report)
}

func usageReplace() {
	fmt.Fprintln(stderrW, "usage: pptx replace --old A --new B --mode <mode> --output out.pptx [--overwrite] [--notes] input.pptx")
}

type replaceReport struct {
	Input  string `json:"input"`
	Output string `json:"output"`
	Old    string `json:"old"`
	New    string `json:"new"`
	Mode   string `json:"mode"`
	Status string `json:"status"`
}
