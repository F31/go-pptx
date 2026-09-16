package pptx

import (
	"errors"
	"fmt"
	"testing"

	chartinternal "github.com/F31/go-pptx/internal/chart"
)

// TestTranslateSentinel 覆盖 chartinternal 哨兵 → 根包哨兵的全部分支：
// nil 输入、6 个已知哨兵逐一映射、未知错误原样透传。
func TestTranslateSentinel(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   error
		want error
	}{
		{"nil", nil, ErrInvalidArgument},
		{"invalid-argument", chartinternal.ErrInvalidArgument, ErrInvalidArgument},
		{"stale-handle", chartinternal.ErrStaleHandle, ErrStaleHandle},
		{"closed", chartinternal.ErrClosed, ErrClosed},
		{"not-found", chartinternal.ErrNotFound, ErrNotFound},
		{"unsupported-format", chartinternal.ErrUnsupportedFormat, ErrUnsupportedFormat},
		{"malformed-package", chartinternal.ErrMalformedPackage, ErrMalformedPackage},
	} {
		got := translateSentinel(tc.in)
		if !errors.Is(got, tc.want) || got == nil {
			t.Errorf("%s: translateSentinel(%v) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
	// 未知错误：原样透传（同一实例）。
	custom := fmt.Errorf("chart: something else")
	if got := translateSentinel(custom); got != custom {
		t.Errorf("unknown error must pass through unchanged, got %v", got)
	}
}

// TestConvertValidationError 验证三类输入的包装行为与 errors.Is 链：
// *chartinternal.ValidationError / *BuildError / 普通错误（fallbackOp）。
func TestConvertValidationError(t *testing.T) {
	// 1) ValidationError：Op/Message 透传，Sentinel 经 translateSentinel
	//    映射回根包哨兵，errors.Is 链对根包调用方成立。
	ve := &chartinternal.ValidationError{
		Op: "SetData", Message: "series count mismatch",
		Sentinel: chartinternal.ErrInvalidArgument,
	}
	err := convertValidationError(ve, "unused")
	var oe *OperationError
	if !errors.As(err, &oe) {
		t.Fatalf("err = %T, want *OperationError", err)
	}
	if oe.Op != "SetData" || oe.Message != "series count mismatch" {
		t.Errorf("op/message = %q/%q, want SetData/series count mismatch", oe.Op, oe.Message)
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("errors.Is(err, ErrInvalidArgument) failed: %v", err)
	}

	// 2) BuildError：同形包装。
	be := &chartinternal.BuildError{
		Op: "BuildChartSpace", Message: "escape failed",
		Sentinel: chartinternal.ErrUnsupportedFormat,
	}
	err = convertValidationError(be, "unused")
	if !errors.As(err, &oe) || oe.Op != "BuildChartSpace" {
		t.Fatalf("BuildError wrap: %v", err)
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("errors.Is(err, ErrUnsupportedFormat) failed: %v", err)
	}

	// 3) 普通错误：fallbackOp 兜底，未知 sentinel 原样透传。
	plain := fmt.Errorf("plain failure")
	err = convertValidationError(plain, "ParseChartSpace")
	if !errors.As(err, &oe) || oe.Op != "ParseChartSpace" {
		t.Fatalf("plain wrap: %v", err)
	}
	if oe.Message != "plain failure" {
		t.Errorf("message = %q, want plain failure", oe.Message)
	}
}
