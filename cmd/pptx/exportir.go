package main

import (
	"flag"
	"fmt"

	"github.com/F31/go-pptx/ir"
)

// cmdRunExportIR 导出 IR 到 --output；也可写 --stdout（默认 stdout）。
//
// JSON 仍写入 stdout。--output 仅在需要文件时使用；--stdout 与 --output
// 互斥（同时指定报错）。
func cmdRunExportIR(args []string) ExitCode {
	fs := flag.NewFlagSet("export-ir", flag.ContinueOnError)
	fs.Usage = func() { usageExportIR() }
	outPath := fs.String("output", "", "output path (default stdout)")
	useStdout := fs.Bool("stdout", false, "force writing JSON to stdout (mutually exclusive with --output)")
	overwrite := fs.Bool("overwrite", false, "allow replacing an existing --output file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "export-ir: missing input path")
		return ExitUsageError
	}
	if *outPath != "" && *useStdout {
		fmt.Fprintln(stderrW, "--output and --stdout are mutually exclusive")
		return ExitUsageError
	}
	input := fs.Arg(0)

	// 仍重定向 stdout 用于 --output 模式，避免泄漏 IR 到终端。
	origStdout := stdoutW
	defer func() { stdoutW = origStdout }()
	if *outPath != "" {
		target := resolveOutputPath(*outPath)
		if !*overwrite {
			if _, err := fileExist(target); err == nil {
				fmt.Fprintf(stderrW, "%s exists; pass --overwrite to replace\n", target)
				return ExitResource
			}
		}
		f, err := openFileForWrite(target, *overwrite)
		if err != nil {
			fmt.Fprintf(stderrW, "open %s: %v\n", target, err)
			return classifyError(err, ExitRuntimeError)
		}
		defer f.Close()
		stdoutW = f
	}

	p, closer, code := openPresentation(input)
	if code != ExitOK {
		return code
	}
	defer closer()

	doc, err := ir.FromPresentation(p, ir.DefaultOptions())
	if err != nil {
		fmt.Fprintf(stderrW, "export-ir: %v\n", err)
		return classifyError(err, ExitRuntimeError)
	}
	return writeJSON(doc)
}

func usageExportIR() {
	fmt.Fprintln(stderrW, "usage: pptx export-ir [--output out.json | --stdout] [--overwrite] input.pptx")
}
