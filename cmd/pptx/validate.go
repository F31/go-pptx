package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/F31/go-pptx"
)

// cmdRunValidate 执行 L0 结构校验并输出诊断（§23.2）。
//
// 不修复原文件；不写输出；error 级诊断退出码 3。
func cmdRunValidate(args []string) ExitCode {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.Usage = func() { usageValidate() }
	level := fs.String("level", "structural", "validation level (structural)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "validate: missing input path")
		return ExitUsageError
	}
	if code := rejectOutputFlag(fs); code != ExitOK {
		return code
	}
	if !strings.EqualFold(*level, "structural") {
		fmt.Fprintf(stderrW, "validate: unsupported level %q (only structural)\n", *level)
		return ExitUsageError
	}
	input := fs.Arg(0)
	p, closer, code := openPresentation(input)
	if code != ExitOK {
		return code
	}
	defer closer()

	report := p.Validate(context.Background())
	out := validateSummary{
		Input:      input,
		Level:      "structural",
		Mode:       report.Mode,
		Diagnostic: report.Diagnostics,
		ErrorCount: 0,
		WarnCount:  0,
	}
	for _, d := range report.Diagnostics {
		switch d.Severity {
		case pptx.SeverityError:
			out.ErrorCount++
		case pptx.SeverityWarning:
			out.WarnCount++
		}
	}
	if code := writeJSON(out); code != ExitOK {
		return code
	}
	if out.ErrorCount > 0 {
		return ExitCapability
	}
	return ExitOK
}

func usageValidate() {
	fmt.Fprintln(stderrW, "usage: pptx validate [--level structural] input.pptx")
}

type validateSummary struct {
	Input      string            `json:"input"`
	Level      string            `json:"level"`
	Mode       string            `json:"mode"`
	ErrorCount int               `json:"errorCount"`
	WarnCount  int               `json:"warnCount"`
	Diagnostic []pptx.Diagnostic `json:"diagnostics"`
}
