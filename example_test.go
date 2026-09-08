package pptx_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	pptx "github.com/F31/go-pptx"
)

// ExampleErrNotFound 展示稳定错误码的 errors.Is 用法（方案 §20.4）：
// 所有可检查错误都支持 errors.Is，具体上下文通过 OperationError 提供。
func ExampleErrNotFound() {
	err := pptx.ErrNotFound

	var opErr *pptx.OperationError
	annotated := pptx.Annotate(err, "Presentation.Open")
	if errors.As(annotated, &opErr) {
		fmt.Println(opErr.Op)
	}
	if errors.Is(annotated, pptx.ErrNotFound) {
		fmt.Println("classified as not found")
	}

	// Output:
	// Presentation.Open
	// classified as not found
}

// ExampleNew 展示最小闭环（MODEL-01）：新建空演示文稿 → 原子保存 →
// 重新打开读取页面列表。Save 默认拒绝覆盖已存在目标，也拒绝写回
// 仍在读取的源文件；输出经同目录临时文件原子替换（方案 §5）。
func ExampleNew() {
	dir, err := os.MkdirTemp("", "go-pptx-example")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)

	// 1) 新建（库内最小合法模板）。
	p, err := pptx.New()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer p.Close()

	// 2) 校验（L0 结构：Content Types 覆盖、关系目标存在）。
	if rep := p.Validate(context.Background()); rep.HasErrors() {
		fmt.Println("validation failed")
		return
	}

	// 3) 原子保存。
	path := filepath.Join(dir, "deck.pptx")
	report, err := p.Save(context.Background(), path)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("saved at revision %d, changed parts: %d\n",
		report.Revision, len(report.ChangedParts))

	// 4) 重新打开并读取页面（空模板为 0 页）。
	p2, err := pptx.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer p2.Close()
	slides, err := p2.Slides()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("slides:", len(slides))

	// Output:
	// saved at revision 0, changed parts: 0
	// slides: 0
}
