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

	"github.com/F31/go-pptx"
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
	case errors.Is(err, pptx.ErrLimitExceeded),
		errors.Is(err, pptx.ErrOutputExists):
		return ExitResource
	}
	return fallback
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
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(stderrW, "%s exists; pass --overwrite to replace\n", path)
			return ExitResource
		}
	}
	if _, err := p.Save(ctx, path, pptx.WithSaveOverwrite(true)); err != nil {
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
