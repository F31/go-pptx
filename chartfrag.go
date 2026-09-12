package pptx

import (
	"errors"

	chartinternal "github.com/F31/go-pptx/internal/chart"
)

// CHART-02 fragment / validator helpers。
//
// ADR-017 第三批：实现搬到 internal/chart，根包保留薄包装（调用方零修改）。
// 错误透传：internal/chart 返回 *chartinternal.ValidationError；根包 wrapper
// 用 *pptx.OperationError{Op, Message, Err} 包装并保留 errors.Is 链。

// 数据标签位置白名单：内部定义在 internal/chart.dataLabelPositionAllowed。

// validateChartData 入口适配器（chart.go 中的入口）；内部已搬。
//
// 错误透传：internal/chart 返回 *chartinternal.ValidationError；根包 wrapper
// 调用 ConvertToOperationError 把它包成 *pptx.OperationError，保留 errors.Is 链；
// 外层调用方拿到的是 *OperationError，行为与原实现一致。
func validateChartData(cd ChartData, op string) error {
	err := chartinternal.ValidateChartData(cd, op)
	if err == nil {
		return nil
	}
	return convertValidationError(err, op)
}

// convertValidationError 把 *chartinternal.ValidationError / *BuildError
// 转回 *pptx.OperationError（保持公共 API 错误链稳定）。
//
// 关键点：chartinternal 的 sentinel (ErrInvalidArgument 等) 与根包
// 不同 errors.New 实例，错误链不能直接穿透。convertValidationError
// 按 Sentinel 字符串映射到根包同名 sentinel —— 避免破坏 tests 与
// errors.Is(err, pptx.ErrInvalidArgument) 调用方契约。
func convertValidationError(err error, fallbackOp string) error {
	if err == nil {
		return nil
	}
	var ve *chartinternal.ValidationError
	if errors.As(err, &ve) {
		return &OperationError{Op: ve.Op, Message: ve.Message, Err: translateSentinel(ve.Sentinel)}
	}
	var be *chartinternal.BuildError
	if errors.As(err, &be) {
		return &OperationError{Op: be.Op, Message: be.Message, Err: translateSentinel(be.Sentinel)}
	}
	return &OperationError{Op: fallbackOp, Message: err.Error(), Err: translateSentinel(err)}
}

// translateSentinel 把 chartinternal 哨兵映射到根包同名 sentinel。
// 字符串匹配：chartinternal.ErrInvalidArgument.Error() = "chart: invalid argument"
// → 根包 ErrInvalidArgument。
func translateSentinel(s error) error {
	if s == nil {
		return ErrInvalidArgument
	}
	switch s.Error() {
	case chartinternal.ErrInvalidArgument.Error():
		return ErrInvalidArgument
	case chartinternal.ErrStaleHandle.Error():
		return ErrStaleHandle
	case chartinternal.ErrClosed.Error():
		return ErrClosed
	case chartinternal.ErrNotFound.Error():
		return ErrNotFound
	case chartinternal.ErrUnsupportedFormat.Error():
		return ErrUnsupportedFormat
	case chartinternal.ErrMalformedPackage.Error():
		return ErrMalformedPackage
	}
	return s
}
