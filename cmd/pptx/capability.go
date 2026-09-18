// pptx capability 子命令（CAP-01，M7 第一项）输出 capability manifest
// （§12.2 / §2.4 护城河第 2 项 / §23.2）。只读，不修改输入文件。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/F31/go-pptx/v2/internal/engine"
)

// cmdRunCapability 输出 capability manifest（§23.2，CAP-01）。
//
// usage: pptx capability [--json] input.pptx
//
// --json 是默认行为（输出已是 JSON）；保留标志用于其它格式扩展点。
// 加载文件并读取元信息（大小），不解析或修改 Part。
func cmdRunCapability(args []string) ExitCode {
	fs := flag.NewFlagSet("capability", flag.ContinueOnError)
	fs.Usage = func() { usageCapability() }
	jsonMode, rest := flagOrJSON(args)
	if code, ok := parseSubcommandFlags(fs, rest); !ok {
		return code
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderrW, "capability: missing input path")
		return ExitUsageError
	}
	if code := rejectOutputFlag(fs); code != ExitOK {
		return code
	}
	input := fs.Arg(0)

	_, closer, code := openPresentation(input)
	if code != ExitOK {
		return code
	}
	defer func() { _ = closer() }()

	var size int64
	if info, err := os.Stat(input); err == nil && !info.IsDir() {
		size = info.Size()
	}
	r, err := engine.Capability(input, size)
	if err != nil {
		fmt.Fprintf(stderrW, "manifest marshal: %v\n", err)
		return ExitRuntimeError
	}
	if _, err := stdoutW.Write(r.IndentJSON); err != nil {
		fmt.Fprintf(stderrW, "stdout write: %v\n", err)
		return ExitRuntimeError
	}
	if _, err := stdoutW.Write([]byte("\n")); err != nil {
		return ExitRuntimeError
	}
	_ = jsonMode // 保留标志位
	return ExitOK
}

// usageCapability 输出 capability 子命令的精简帮助到 stderr。
func usageCapability() {
	fmt.Fprintln(stderrW, "usage: pptx capability [--json] input.pptx")
}
