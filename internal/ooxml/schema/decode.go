package schema

import (
	"encoding/xml"
	"fmt"
)

// 本文件提供只读投影的解码入口（ADR-030 机制 2）：生成类型仅用于
// encoding/xml 反序列化，写入路径不得使用。

// Unmarshal 解码 data 到 v（生成类型的只读投影）。统一错误前缀，便于
// 调用方区分「格式层解析失败」与业务错误。不做任何写入。
func Unmarshal(data []byte, v any) error {
	if err := xml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("ooxml schema: %w", err)
	}
	return nil
}

// DecodePresentation 解码 ppt/presentation.xml 的 p:presentation 根。
func DecodePresentation(data []byte) (*P_CT_Presentation, error) {
	v := &P_CT_Presentation{}
	if err := Unmarshal(data, v); err != nil {
		return nil, err
	}
	return v, nil
}

// DecodeSlide 解码 ppt/slides/slideN.xml 的 p:sld 根。
func DecodeSlide(data []byte) (*P_CT_Slide, error) {
	v := &P_CT_Slide{}
	if err := Unmarshal(data, v); err != nil {
		return nil, err
	}
	return v, nil
}

// DecodeSlideLayout 解码 ppt/slideLayouts/slideLayoutN.xml 的根。
func DecodeSlideLayout(data []byte) (*P_CT_SlideLayout, error) {
	v := &P_CT_SlideLayout{}
	if err := Unmarshal(data, v); err != nil {
		return nil, err
	}
	return v, nil
}

// DecodeSlideMaster 解码 ppt/slideMasters/slideMasterN.xml 的根。
func DecodeSlideMaster(data []byte) (*P_CT_SlideMaster, error) {
	v := &P_CT_SlideMaster{}
	if err := Unmarshal(data, v); err != nil {
		return nil, err
	}
	return v, nil
}
