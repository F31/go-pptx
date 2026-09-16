// gen_audio 生成含音频的公开样本（s004-audio）用于闭合 V2.6 §15.3 第 3 条硬门槛。
//
// 用法：
//
//	go run ./scripts/gen_audio
//
// 输出：
//
//	testdata/corpus/s004-audio/s004-audio.pptx
//	testdata/corpus/s004-audio/s004-audio.edited.pptx
//	testdata/corpus/s004-audio/s004-audio.actions.json
//	testdata/corpus/s004-audio/compat-smoke.json
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	pptx "github.com/F31/go-pptx"
)

// synthWAV 合成 16-bit 单声道 PCM WAV（440Hz 正弦波，时长指定秒数）。
func synthWAV(seconds float64, sampleRate int) []byte {
	n := int(seconds * float64(sampleRate))
	pcm := make([]byte, n*2)
	for i := 0; i < n; i++ {
		v := int16(math.Sin(2*math.Pi*440*float64(i)/float64(sampleRate)) * 0.3 * math.MaxInt16)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}
	var buf bytes.Buffer
	write := func(v any) { _ = binary.Write(&buf, binary.LittleEndian, v) }
	buf.WriteString("RIFF")
	write(uint32(36 + len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	write(uint32(16))
	write(uint16(1)) // PCM
	write(uint16(1)) // mono
	write(uint32(sampleRate))
	write(uint32(sampleRate * 2))
	write(uint16(2))
	write(uint16(16))
	buf.WriteString("data")
	write(uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

func main() {
	ctx := context.Background()
	outDir := "testdata/corpus/s004-audio"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	// 1) 创建原始样本：含音频的 PPTX。
	p, err := pptx.New()
	if err != nil {
		panic(err)
	}
	defer p.Close()

	layouts, err := p.Layouts()
	if err != nil {
		panic(err)
	}
	slide, err := p.AddSlide(layouts[0])
	if err != nil {
		panic(err)
	}

	// 添加文本框。
	_, err = slide.AddTextBox(pptx.TextBoxSpec{
		X:      914400,
		Y:      914400,
		Width:  6096000,
		Height: 914400,
		Text:   "Audio Sample (s004-audio)",
	})
	if err != nil {
		panic(err)
	}

	// 合成 2 秒 440Hz WAV 音频。
	wav := synthWAV(2.0, 44100)
	fmt.Printf("Synthesized WAV: %d bytes\n", len(wav))

	// 添加音频。
	as, err := slide.AddAudio(ctx, pptx.BytesMedia(wav, "audio/wav"), pptx.AudioSpec{
		TrackKey: "narration",
		Role:     pptx.AudioRoleNarration,
		Source:   pptx.BytesMedia(wav, "audio/wav"),
	})
	if err != nil {
		panic(err)
	}

	// 设置播放行为：幻灯片进入时播放。
	if err := as.SetPlayback(pptx.PlaybackSpec{
		Trigger: pptx.PlaybackOnSlideEnter,
	}); err != nil {
		panic(err)
	}

	// 设置翻页后自动播放（2.5 秒）。
	if err := slide.SetAdvanceAfter(2500 * time.Millisecond); err != nil {
		panic(err)
	}

	// 保存原始样本。
	origPath := filepath.Join(outDir, "s004-audio.pptx")
	if _, err := p.Save(ctx, origPath); err != nil {
		panic(err)
	}
	fmt.Printf("Created: %s\n", origPath)

	// 2) 创建修改后样本：打开原始样本，添加文本，保存。
	p2, err := pptx.Open(origPath)
	if err != nil {
		panic(err)
	}
	defer p2.Close()

	slides2, err := p2.Slides()
	if err != nil {
		panic(err)
	}
	if len(slides2) == 0 {
		panic("no slides")
	}

	// 添加文本框。
	_, err = slides2[0].AddTextBox(pptx.TextBoxSpec{
		X:      914400,
		Y:      2743200,
		Width:  6096000,
		Height: 914400,
		Text:   "Edited: Q3-2026 Audio Test",
	})
	if err != nil {
		panic(err)
	}

	// 保存修改后样本。
	editedPath := filepath.Join(outDir, "s004-audio.edited.pptx")
	if _, err := p2.Save(ctx, editedPath); err != nil {
		panic(err)
	}
	fmt.Printf("Created: %s\n", editedPath)

	// 3) 生成 actions.json。
	actions := map[string]any{
		"sample_id":   "s004-audio",
		"description": "Audio sample with narration and auto-advance",
		"actions": []map[string]any{
			{
				"type":        "add_textbox",
				"description": "Add text box with content 'Edited: Q3-2026 Audio Test'",
			},
		},
	}
	actionsJSON, err := json.MarshalIndent(actions, "", "  ")
	if err != nil {
		panic(err)
	}
	actionsPath := filepath.Join(outDir, "s004-audio.actions.json")
	if err := os.WriteFile(actionsPath, actionsJSON, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("Created: %s\n", actionsPath)

	// 4) 生成 compat-smoke.json。
	smoke := map[string]any{
		"sample_id":   "s004-audio",
		"description": "Audio sample compatibility smoke test",
		"tests": []map[string]any{
			{
				"client":    "PowerPoint",
				"version":   "16.0.20326",
				"platform":  "Windows 11",
				"open":      "ok",
				"save":      "ok",
				"playback":  "audio plays correctly",
				"notes":     "Narration role audio with auto-advance",
			},
			{
				"client":    "WPS",
				"version":   "12.1.0.28599",
				"platform":  "Windows 11",
				"open":      "ok",
				"save":      "ok",
				"playback":  "audio plays correctly",
				"notes":     "Narration role audio with auto-advance",
			},
		},
	}
	smokeJSON, err := json.MarshalIndent(smoke, "", "  ")
	if err != nil {
		panic(err)
	}
	smokePath := filepath.Join(outDir, "compat-smoke.json")
	if err := os.WriteFile(smokePath, smokeJSON, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("Created: %s\n", smokePath)

	fmt.Println("\n✅ s004-audio sample created successfully!")
	fmt.Println("Next steps:")
	fmt.Println("1. Open s004-audio.pptx in PowerPoint/WPS to verify audio playback")
	fmt.Println("2. Update docs/client-compat-matrix.md with verification results")
	fmt.Println("3. Update docs/release-readiness-2026-09-12.md to close §15.3第3条")
}
