package main

import (
	"flag"
	"fmt"

	"github.com/F31/go-pptx/ir"
)

// cmdRunDiff 比较两个 PPTX 的语义 diff（DIFF-01，只读）。
//
// 两侧各自导出 IR（同一份 Options，含时序 IR），Diff 报告以 JSON
// 输出（stdout 或 --output）。定位字段 Part/NodePath 可回溯到
// 两侧的元素位置（设计 §18.3）。
func cmdRunDiff(args []string) ExitCode {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.Usage = func() { usageDiff() }
	outPath := fs.String("output", "", "output path (default stdout)")
	useStdout := fs.Bool("stdout", false, "force writing JSON to stdout (mutually exclusive with --output)")
	overwrite := fs.Bool("overwrite", false, "allow replacing an existing --output file")
	ignoreGeometry := fs.Bool("ignore-geometry", false, "skip shape position/size comparison")
	ignoreWhitespace := fs.Bool("ignore-whitespace", false, "normalize whitespace before text comparison")
	ignoreNotes := fs.Bool("ignore-notes", false, "skip notes text comparison")
	maxEntries := fs.Int("max-entries", 0, "cap on reported entries (0 = default 10000)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderrW, err.Error())
		return ExitUsageError
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderrW, "diff: need two input paths (old new)")
		return ExitUsageError
	}
	if *outPath != "" && *useStdout {
		fmt.Fprintln(stderrW, "--output and --stdout are mutually exclusive")
		return ExitUsageError
	}
	oldPath, newPath := fs.Arg(0), fs.Arg(1)

	// stdout 重定向，避免 --output 模式泄漏 JSON 到终端。
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

	pa, closerA, code := openPresentation(oldPath)
	if code != ExitOK {
		return code
	}
	defer closerA()
	pb, closerB, code := openPresentation(newPath)
	if code != ExitOK {
		return code
	}
	defer closerB()

	// 两侧使用同一份 IR 导出选项，确保投影维度一致。
	opts := ir.DefaultOptions()
	da, err := ir.FromPresentation(pa, opts)
	if err != nil {
		fmt.Fprintf(stderrW, "diff: %s: %v\n", oldPath, err)
		return classifyError(err, ExitRuntimeError)
	}
	db, err := ir.FromPresentation(pb, opts)
	if err != nil {
		fmt.Fprintf(stderrW, "diff: %s: %v\n", newPath, err)
		return classifyError(err, ExitRuntimeError)
	}

	dOpts := []ir.DiffOption{
		ir.WithIgnoreGeometry(*ignoreGeometry),
		ir.WithIgnoreWhitespace(*ignoreWhitespace),
		ir.WithIgnoreNotes(*ignoreNotes),
	}
	if *maxEntries > 0 {
		dOpts = append(dOpts, ir.WithMaxEntries(*maxEntries))
	}
	return writeJSON(ir.Diff(da, db, dOpts...))
}

func usageDiff() {
	fmt.Fprintln(stderrW, "usage: pptx diff [--output out.json | --stdout] [--overwrite] old.pptx new.pptx")
	fmt.Fprintln(stderrW, "                [--ignore-geometry] [--ignore-whitespace] [--ignore-notes] [--max-entries n]")
}
