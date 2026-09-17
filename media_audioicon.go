package pptx

import _ "embed"

// audioSpeakerIconPNG 是音频形状的内置 poster 图标（喇叭 + 声波，64×64 PNG）。
//
// 背景（ADR-027 续）：PowerPoint 的音频形状是 `p:pic`，其
// `p:blipFill/a:blip@r:embed` **必须指向一张图片**（poster 图标），
// 由它渲染成可见的喇叭按钮；音频本身经 `p:nvPr/a:audioFile@r:link`
// 关联。早期实现把 `a:blip` 直接指向音频文件（WAV），PowerPoint
// 无法把音频当图片解码 → 图标不可见。
//
//go:embed assets/audio-speaker.png
var audioSpeakerIconPNG []byte

// audioSpeakerIconKind 描述内置图标的媒体类型与尺寸（供
// Presentation.planMedia 去重/命名使用）。
var audioSpeakerIconKind = imageKind{
	CT:    "image/png",
	Ext:   "png",
	Width: 64,
	Hgt:   64,
}

func init() {
	audioSpeakerIconKind.ByteSize = int64(len(audioSpeakerIconPNG))
}
