// Command pptx 提供 go-pptx SDK 的命令行界面（TOOL-01，方案 §23.2）。
//
// 子命令：inspect / validate / replace / narrate / timing-plan / export-ir。
//   - JSON 写到 stdout；
//   - 进度/错误摘要写到 stderr；
//   - 退出码：0 成功，1 执行/I/O 错误，2 参数错误，3 校验或能力限制，
//     4 资源超限（§23.2）。
//   - 只读命令默认拒绝 --output；写命令要求 --output；--overwrite 为
//     显式覆盖开关。
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExitCode 定义（§23.2）。
type ExitCode int

const (
	ExitOK           ExitCode = 0
	ExitRuntimeError ExitCode = 1
	ExitUsageError   ExitCode = 2
	ExitCapability   ExitCode = 3
	ExitResource     ExitCode = 4
)

// osExit 把 exitCode 写入全局可重写变量以便测试。生产环境调用 os.Exit。
var osExit = func(code ExitCode) {
	os.Exit(int(code))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(stderrW, "pptx: missing subcommand")
		printUsage(false)
		osExit(ExitUsageError)
		return
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "inspect":
		exitRun(cmdRunInspect(args))
	case "validate":
		exitRun(cmdRunValidate(args))
	case "replace":
		exitRun(cmdRunReplace(args))
	case "narrate":
		exitRun(cmdRunNarrate(args))
	case "timing-plan":
		exitRun(cmdRunTimingPlan(args))
	case "export-ir":
		exitRun(cmdRunExportIR(args))
	case "-h", "--help", "help":
		printUsage(true)
		osExit(ExitOK)
	case "-v", "--version":
		fmt.Fprintln(stdoutW, "pptx", sdkVersionString())
		osExit(ExitOK)
	default:
		fmt.Fprintf(stderrW, "pptx: unknown subcommand %q\n\n", cmd)
		printUsage(false)
		osExit(ExitUsageError)
	}
}

func exitRun(c ExitCode) {
	if c == ExitOK {
		return
	}
	osExit(c)
}

// printUsage 输出精简帮助（详细帮助留子命令自管）。到 stdout/stderr。
func printUsage(toStdout bool) {
	var w io.Writer = stdoutW
	if !toStdout {
		w = stderrW
	}
	printUsageTo(w)
}

func printUsageTo(w io.Writer) {
	fmt.Fprintln(w, "Usage: pptx <subcommand> [options] input.pptx")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  inspect       read & summarize document structure (JSON)")
	fmt.Fprintln(w, "  validate      run structural validation (JSON)")
	fmt.Fprintln(w, "  replace       replace text in shape bodies (writes --output)")
	fmt.Fprintln(w, "  narrate       embed audio from tracks.json (writes --output)")
	fmt.Fprintln(w, "  timing-plan   preview timing sync plan (read-only)")
	fmt.Fprintln(w, "  export-ir     export intermediate representation (JSON)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Common flags:")
	fmt.Fprintln(w, "  --json                ensure single JSON object on stdout")
	fmt.Fprintln(w, "  --output <path>       required for write commands (replace, narrate, export-ir)")
	fmt.Fprintln(w, "  --overwrite           allow replacing an existing output file")
}

// sdkVersionString 返回 SDK 版本：嵌入版本号（空时标 dev）。
func sdkVersionString() string {
	if PPTXVersion != "" {
		return PPTXVersion
	}
	return "dev"
}

// basename 取文件基础名（仅主名，不含目录）。
func basename(path string) string { return filepath.Base(path) }

// 子命令签名由各 *_<subcommand>.go 文件提供：
//
//	cmdRunInspect(args []string) ExitCode
//	cmdRunValidate(args []string) ExitCode
//	cmdRunReplace(args []string) ExitCode
//	cmdRunNarrate(args []string) ExitCode
//	cmdRunTimingPlan(args []string) ExitCode
//	cmdRunExportIR(args []string) ExitCode
//
// 本文件仅做分发与退出码包装。
var _ = "TOOL-01 subcommand registrar"
