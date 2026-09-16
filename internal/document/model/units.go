package model

// EMU 是 OOXML 长度单位（English Metric Unit，int64）。
// 1 in = 914400 EMU，1 pt = 12700 EMU；形状坐标、尺寸均以 EMU 表示。
// 作为共享度量单位，被 geometry / style / layout 等域共同引用。
type EMU int64

const (
	emuPerInch  = 914400
	emuPerPoint = 12700
)

// Inches 返回英寸表示的浮点值。
func (e EMU) Inches() float64 { return float64(e) / float64(emuPerInch) }

// Points 返回磅表示的浮点值。
func (e EMU) Points() float64 { return float64(e) / float64(emuPerPoint) }
