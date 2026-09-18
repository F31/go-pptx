package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/F31/go-pptx/v2/pptx"
)

// PPTXVersion 可由 CI 注入（-ldflags '-X main.PPTXVersion=v0.x.y'）。
var PPTXVersion = ""

// 输出到 stdout/stderr 的辅助函数（测试可重写）。

var (
	stdoutW io.Writer = os.Stdout
	stderrW io.Writer = os.Stderr
)

// writeJSON 把 v 序列化为缩进 JSON 写到 stdoutW。失败返回 ExitRuntimeError。
func writeJSON(v any) ExitCode {
	enc := json.NewEncoder(stdoutW)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderrW, "json: %v\n", err)
		return ExitRuntimeError
	}
	return ExitOK
}

// stderrErr 写一条错误到 stderr 并返回对应 exit code。
func stderrErr(op string, err error, defaultCode ExitCode) ExitCode {
	if err == nil {
		return ExitOK
	}
	fmt.Fprintf(stderrW, "%s: %v\n", op, err)
	return classifyError(err, defaultCode)
}

// classifyError 把根包错误分类为退出码（§23.2 统一约定）。
func classifyError(err error, fallback ExitCode) ExitCode {
	switch {
	case errors.Is(err, pptx.ErrInvalidArgument),
		errors.Is(err, pptx.ErrOutOfRange):
		return ExitUsageError
	case errors.Is(err, pptx.ErrValidationFailed),
		errors.Is(err, pptx.ErrUnsupportedEdit),
		errors.Is(err, pptx.ErrUnsupportedFormat),
		errors.Is(err, pptx.ErrClosed):
		return ExitCapability
	case errors.Is(err, pptx.ErrLimitExceeded):
		return ExitResource
	case errors.Is(err, pptx.ErrOutputExists):
		// 退出码 4 在 §23.2 / README 里是"资源超限"；"输出已存在"是调用方式问题
		// （补 --overwrite 即可），归到 2 才与文档一致。
		return ExitUsageError
	}
	return fallback
}

// parseSubcommandFlags 统一子命令的 flag 解析分支。返回 (退出码, 是否继续)。
//
// 为什么集中处理：此前每个子命令都是
// `if err := fs.Parse(...); err != nil { fmt.Fprintln(stderrW, err); return ExitUsageError }`，
// 于是 `pptx <sub> --help` 与"参数写错"共用同一条路径 —— 帮助被写到 stderr 且退出码 2，
// 与顶层 `pptx --help`（stdout + 退出码 0）不一致，`pptx inspect --help | less` 之类的
// 常规用法直接失效，还会多打出一行 "flag: help requested"。
//
// 现在：--help/-h（flag.ErrHelp）→ 用法写 stdout、退出码 0；其它解析错误 → 错误与用法
// 写 stderr、退出码 2。输出一律经 stdoutW/stderrW（fs 自身输出设为 io.Discard），
// 便于测试捕获。
func parseSubcommandFlags(fs *flag.FlagSet, args []string) (ExitCode, bool) {
	fs.SetOutput(io.Discard) // 输出去向由本函数统一决定
	// 解析期间临时摘掉 fs.Usage：flag 在遇到 --help 或参数错误时会主动调用它，
	// 而各 usageX() 直接写 stderrW —— 那样帮助会先往 stderr 打一份（exit 0 的帮助
	// 不该出现在 stderr）。摘掉后 flag 走 defaultUsage（写 io.Discard），由本函数在
	// 正确的流上补打一份。
	usage := fs.Usage
	fs.Usage = nil
	err := fs.Parse(args)
	fs.Usage = usage
	if err == nil {
		return ExitOK, true
	}
	if errors.Is(err, flag.ErrHelp) {
		usageTo(stdoutW, fs)
		return ExitOK, false
	}
	fmt.Fprintln(stderrW, err.Error())
	usageTo(stderrW, fs)
	return ExitUsageError, false
}

// usageTo 调用 fs.Usage，但把本次用法输出临时重定向到 w。
//
// 各子命令的 usageX() 直接写 stderrW（无 writer 形参），这里临时替换 stderrW 变量
// 即可把同一份文本送到 stdout，无需改动 9 个 usage 函数。
func usageTo(w io.Writer, fs *flag.FlagSet) {
	if fs.Usage == nil {
		return
	}
	if w == stderrW {
		fs.Usage()
		return
	}
	prev := stderrW
	stderrW = w
	defer func() { stderrW = prev }()
	fs.Usage()
}

// flagOrJSON 解析 --json 标记（只读命令默认写 JSON 缩进格式）。
func flagOrJSON(args []string) (jsonMode bool, rest []string) {
	for _, a := range args {
		if a == "--json" {
			jsonMode = true
			continue
		}
		rest = append(rest, a)
	}
	return jsonMode, rest
}

// parseReplaceModeFlag 把 --mode=... 解析为 pptx.ReplaceMode。识别失败返回
// ("", false)。
func parseReplaceModeFlag(s string) (pptx.ReplaceMode, bool) {
	switch strings.ToLower(s) {
	case "firstchar", "first-char", "firstcharstyle":
		return pptx.ReplaceFirstCharacter, true
	case "equal", "equal-length", "equallength":
		return pptx.ReplaceEqualLengthPerRune, true
	case "explicit", "explicitstyle":
		return pptx.ReplaceExplicitStyle, true
	}
	var zero pptx.ReplaceMode
	return zero, false
}

// openPresentation 按 path 打开只读演示文稿，统一错误处理。
func openPresentation(path string) (*pptx.Presentation, func() error, ExitCode) {
	if path == "" {
		fmt.Fprintln(stderrW, "missing input path")
		return nil, nil, ExitUsageError
	}
	p, err := pptx.Open(path)
	if err != nil {
		fmt.Fprintf(stderrW, "open %s: %v\n", path, err)
		return nil, nil, classifyError(err, ExitRuntimeError)
	}
	return p, p.Close, ExitOK
}

// writePresentation 把 p 写入 path，检查覆盖并尊重 --overwrite。
func writePresentation(ctx context.Context, p *pptx.Presentation, path string, overwrite bool) ExitCode {
	if path == "" {
		fmt.Fprintln(stderrW, "missing --output")
		return ExitUsageError
	}
	// 覆盖判定交给库（原子保存内部检查），不再先 Stat 再无条件
	// WithSaveOverwrite(true)：那既绕开了库自身的 ErrOutputExists 守卫，也存在
	// Stat 与 Save 之间目标被他人创建的竞态（TOCTOU）。
	if _, err := p.Save(ctx, path, pptx.WithSaveOverwrite(overwrite)); err != nil {
		if errors.Is(err, pptx.ErrOutputExists) {
			fmt.Fprintf(stderrW, "%s exists; pass --overwrite to replace\n", path)
			return ExitUsageError
		}
		fmt.Fprintf(stderrW, "save %s: %v\n", path, err)
		return classifyError(err, ExitRuntimeError)
	}
	return ExitOK
}

// rejectOutputFlag 拒绝只读子命令中误传的 --output。
func rejectOutputFlag(fs *flag.FlagSet) ExitCode {
	if fs.Lookup("output") != nil {
		fmt.Fprintln(stderrW, "--output not supported by this read-only command")
		return ExitUsageError
	}
	return ExitOK
}

// sliceFlag 提取重复 --foo 标记的剩余 args（处理 -k=v 与 -k v 两种形态）。
func sliceFlag(args []string, names ...string) (values []string, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		matched := false
		for _, n := range names {
			eq := n + "="
			if a == n && i+1 < len(args) {
				values = append(values, args[i+1])
				i++
				matched = true
				break
			}
			if strings.HasPrefix(a, eq) {
				values = append(values, a[len(eq):])
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		rest = append(rest, a)
	}
	return values, rest
}

// resolveOutputPath 规范化 --output（去除尾部分隔符，相对路径基于 cwd）。
func resolveOutputPath(s string) string {
	if s == "" {
		return ""
	}
	if abs, err := filepath.Abs(s); err == nil {
		return abs
	}
	return s
}
