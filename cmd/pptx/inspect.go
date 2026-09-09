package main

import (
	"flag"
	"fmt"

	"github.com/F31/go-pptx/ir"
)

// cmdRunInspect 输出页面、媒体、备注及能力信息（§23.2，方案 §18.3）。
//
// 仅对已加载 IR 受支持字段投影；其余 schema 行作为后续工作包的扩展点。
func cmdRunInspect(args []string) ExitCode {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.Usage = func() { usageInspect() }
	jsonMode, rest := flagOrJSON(args)
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "inspect: missing input path")
		return ExitUsageError
	}
	if code := rejectOutputFlag(fs); code != ExitOK {
		return code
	}
	input := fs.Arg(0)

	p, closer, code := openPresentation(input)
	if code != ExitOK {
		return code
	}
	defer closer()

	slides, err := p.Slides()
	if err != nil {
		fmt.Fprintf(stderrW, "slides: %v\n", err)
		return classifyError(err, ExitRuntimeError)
	}

	doc, err := ir.FromPresentation(p, ir.DefaultOptions())
	if err != nil {
		fmt.Fprintf(stderrW, "ir: %v\n", err)
		return classifyError(err, ExitRuntimeError)
	}

	out := inspectSummary{
		Input:    input,
		Pages:    len(slides),
		Schemas:  PPTXVersion,
		Document: doc,
		ReadOnly: true,
	}
	_ = jsonMode
	return writeJSON(out)
}

func usageInspect() {
	fmt.Fprintln(stderrW, "usage: pptx inspect [--json] input.pptx")
}

// inspectSummary 是 inspect 命令的输出形态（§23.2 仅 — 这里是稳定 schema 的最小集）。
type inspectSummary struct {
	Input    string       `json:"input"`
	Pages    int          `json:"pages"`
	Schemas  string       `json:"sdkVersion"`
	Document *ir.Document `json:"document"`
	ReadOnly bool         `json:"readOnly"`
}
