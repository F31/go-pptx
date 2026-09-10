// Package render 定义外部渲染适配器的接口契约（设计方案 §23.1）。
//
// 接口独立于核心包：render 依赖 github.com/F31/go-pptx，而核心包
// 不反向依赖 render。Presentation 上不提供 RenderSlide 方法，避免
// 核心 SDK 被渲染实现拖入字体/shaping/布局等重依赖。
//
// 原生实现（标注覆盖度的"高质量缩略图"，ADR 014）后续单独立项；
// 本包仅定义契约，不含任何渲染实现。任何基于本接口的预览能力都必须
// 明确标注为"有限预览"，不得用于证明 PowerPoint 视觉一致。
package render

import (
	"context"
	"io"

	"github.com/F31/go-pptx"
)

// FontStrategy 描述缺字/字体替代策略。仅作为元数据声明，实际行为
// 由适配器决定（§23.1 要求适配器声明是否支持字体替代）。
type FontStrategy struct {
	// Substitute 是否允许缺字时回退到系统字体。
	Substitute bool
	// EmbedFallback 是否内嵌回退字体（仅声明，实现由适配器负责）。
	EmbedFallback bool
}

// RenderOptions 描述单页渲染的约束。当像素尺寸与 DPI 冲突时，以像素
// 尺寸为输出约束，DPI 仅作为元数据/换算输入（§23.1）。
type RenderOptions struct {
	// WidthPX / HeightPX 输出目标像素尺寸；为 0 表示由适配器按幻灯片
	// 原始纵横比与 DPI 推导。
	WidthPX  int
	HeightPX int
	// DPI 仅作为换算/元数据输入，不决定输出像素尺寸。
	DPI float64
	// Background 背景策略："transparent"（透明）或 "slide"（使用幻灯片
	// 自身背景）。空字符串等价于 "transparent"。
	Background string
	// Fonts 字体策略。
	Fonts FontStrategy
}

// RenderCapabilities 声明适配器支持的能力范围（§23.1）。调用方据此
// 决定是否可在目标场景使用该适配器。
type RenderCapabilities struct {
	// Animation 是否支持动画帧渲染。
	Animation bool
	// FontSubstitution 是否支持缺字替代。
	FontSubstitution bool
	// HiddenSlides 是否支持渲染隐藏页。
	HiddenSlides bool
	// OfficeFeatures 是否支持 Office 特有特性（SmartArt / 艺术字等）。
	OfficeFeatures bool
}

// RenderedSlide 是一次渲染的产物：可流式读取的页面图像 + 诊断信息。
type RenderedSlide struct {
	// SlideID 被渲染页面的 ID。
	SlideID pptx.SlideID
	// MIMEType 图像格式（如 "image/png"）。
	MIMEType string
	// Width / Height 实际输出像素尺寸。
	Width  int
	Height int
	// Data 页面图像字节流，由消费方负责 Close。
	Data io.ReadCloser
	// Diagnostics 渲染过程中的诊断/警告。
	Diagnostics []string
}

// Close 释放底层图像流（若存在）。
func (r RenderedSlide) Close() error {
	if r.Data == nil {
		return nil
	}
	return r.Data.Close()
}

// Renderer 是外部渲染适配器必须实现的接口（设计方案 §23.1）。
type Renderer interface {
	// Capabilities 返回适配器支持的能力声明。
	Capabilities() RenderCapabilities
	// RenderSlide 渲染单个页面。适配器对内存文档先保存临时副本再渲染，
	// 临时副本无权覆盖输入；进程超时、内存限制及临时文件清理由适配器
	// 负责。ctx 取消时适配器应尽快返回。
	RenderSlide(ctx context.Context, doc *pptx.Presentation, slideID pptx.SlideID, opts RenderOptions) (RenderedSlide, error)
}

// RenderAll 逐页渲染，通过回调消费每页产物，避免一次返回全部页面图
// 占用大内存（§23.1）。fn 在每次回调返回后其 RenderedSlide.Data 即被
// Close——fn 不得在返回后继续持有该流。fn 返回错误时中止渲染并向上传播，
// 已消费的页面不再重试。
func RenderAll(ctx context.Context, r Renderer, doc *pptx.Presentation, opts RenderOptions, fn func(RenderedSlide) error) error {
	slides, err := doc.Slides()
	if err != nil {
		return err
	}
	for _, s := range slides {
		if err := ctx.Err(); err != nil {
			return err
		}
		rs, err := r.RenderSlide(ctx, doc, s.ID(), opts)
		if err != nil {
			return err
		}
		err = fn(rs)
		cerr := rs.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
		}
	}
	return nil
}
