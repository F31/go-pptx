package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/F31/go-pptx"
)

// fileExist 报告路径是否已存在（用于 --overwrite 提示）。
func fileExist(path string) (struct{}, error) {
	_, err := os.Stat(path)
	if err == nil {
		return struct{}{}, nil
	}
	return struct{}{}, err
}

// openFileForWrite 按目标创建文件，覆盖由 caller 决定；不覆盖时返回
// ErrOutputExists 兼容（cmd 层先做 exists 检查）。
func openFileForWrite(path string, overwrite bool) (*os.File, error) {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return nil, fmt.Errorf("%w", pptx.ErrOutputExists)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// jsonUnmarshalStrict 拒绝未知字段的严格解码（用于外部 manifest 输入校验）。
func jsonUnmarshalStrict(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}

// contextFromArgs 解析 --ctx-timeout=10s 之类选项；当前保留 entry 用于
// 后续多命令一致性扩展。
func contextFromArgs(args []string) (context.Context, []string) { return context.Background(), args }

// notImplemented 标注尚未完成的子命令占位（避免静默 0 返回）。TOOL-01 范围
// 内六个子命令实现完整；此函数仅作为开发期辅助。
func notImplemented(name string) ExitCode {
	fmt.Fprintf(stderrW, "%s: not implemented in this build\n", name)
	return ExitCapability
}

var (
	_ = jsonUnmarshalStrict
	_ = notImplemented
	_ = errors.New
)
