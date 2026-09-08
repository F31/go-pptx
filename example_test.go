package pptx_test

import (
	"errors"
	"fmt"

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
