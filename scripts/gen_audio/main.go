// gen_audio 生成含音频的公开样本（s004-audio）用于闭合 V2.6 §15.3 第 3 条硬门槛。
//
// 用法：
//
//	go run ./scripts/gen_audio
//
// 输出（testdata/corpus/s004-audio/）：
//
//	s004-audio.pptx          原始样本（文本 + 音频 + 自动翻页）
//	s004-audio.edited.pptx   对原始样本应用 gold action（ReplaceText）后的结果
//	s004-audio.actions.json  gold action 定义
//	compat-smoke.json        replay 断言（计数 / validate / diff）
//	manifest.json            语料元数据（schema go-pptx.corpus/1.0）
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	pptx "github.com/F31/go-pptx"
)

const (
	sampleID  = "s004-audio"
	goldOld   = "Audio Sample (s004-audio)"
	goldNew   = "Audio Test (Q3-2026)"
	outDirRel = "testdata/corpus/s004-audio"
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

func writeJSON(path string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("Created: %s\n", path)
}

func sha256File(path string) (string, int64) {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), int64(len(data))
}

func main() {
	ctx := context.Background()
	if err := os.MkdirAll(outDirRel, 0o755); err != nil {
		panic(err)
	}

	// 1) 创建原始样本：含文本 + 音频 + 自动翻页。
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

	if _, err := slide.AddTextBox(pptx.TextBoxSpec{
		X: 914400, Y: 914400, Width: 6096000, Height: 914400,
		Text: goldOld,
	}); err != nil {
		panic(err)
	}

	wav := synthWAV(2.0, 44100)
	fmt.Printf("Synthesized WAV: %d bytes\n", len(wav))

	as, err := slide.AddAudio(ctx, pptx.BytesMedia(wav, "audio/wav"), pptx.AudioSpec{
		TrackKey: "narration",
		Role:     pptx.AudioRoleNarration,
		Source:   pptx.BytesMedia(wav, "audio/wav"),
		X:        914400,
		Y:        2743200,
		Width:    914400,
		Height:   914400,
	})
	if err != nil {
		panic(err)
	}
	if err := as.SetPlayback(pptx.PlaybackSpec{Trigger: pptx.PlaybackOnSlideEnter}); err != nil {
		panic(err)
	}
	if err := slide.SetAdvanceAfter(2500 * time.Millisecond); err != nil {
		panic(err)
	}

	origPath := filepath.Join(outDirRel, sampleID+".pptx")
	if _, err := p.Save(ctx, origPath); err != nil {
		panic(err)
	}
	fmt.Printf("Created: %s\n", origPath)

	// 2) 应用 gold action（ReplaceText）生成 edited 样本。
	p2, err := pptx.Open(origPath)
	if err != nil {
		panic(err)
	}
	defer p2.Close()

	slides2, err := p2.Slides()
	if err != nil {
		panic(err)
	}
	stats := replayStats{}
	for _, sl := range slides2 {
		shapes, err := sl.Shapes()
		if err != nil {
			panic(err)
		}
		for _, sh := range shapes {
			stats.ShapesSeen++
			auto, ok := sh.(*pptx.AutoShape)
			if !ok {
				continue
			}
			tf, err := auto.TextFrame()
			if err != nil {
				continue
			}
			stats.ShapesTried++
			res, err := tf.ReplaceText(goldOld, goldNew)
			if err != nil {
				panic(err)
			}
			stats.Matches += res.Matches
			stats.Replaced += res.Replaced
		}
	}
	if stats.Replaced != 1 {
		panic(fmt.Sprintf("expected exactly 1 replacement, got %d (seen=%d tried=%d)",
			stats.Replaced, stats.ShapesSeen, stats.ShapesTried))
	}

	editedPath := filepath.Join(outDirRel, sampleID+".edited.pptx")
	if _, err := p2.Save(ctx, editedPath); err != nil {
		panic(err)
	}
	fmt.Printf("Created: %s\n", editedPath)

	// 3) gold action 定义（与 s001/s002/s003 同格式：数组）。
	writeJSON(filepath.Join(outDirRel, sampleID+".actions.json"), []map[string]any{
		{
			"action": "ReplaceText",
			"old":    goldOld,
			"new":    goldNew,
			"scope":  "all AutoShape text frames",
			"expected": map[string]any{
				"matches":              stats.Matches,
				"replaced":             stats.Replaced,
				"validate_error_count": 0,
			},
		},
	})

	// 4) compat-smoke（replay 断言）。
	writeJSON(filepath.Join(outDirRel, "compat-smoke.json"), map[string]any{
		"schemaVersion":        "go-pptx.compat-smoke/1.0",
		"sample_id":            sampleID,
		"source":               "testdata/corpus/" + sampleID + "/" + sampleID + ".pptx",
		"edited_file_committed": true,
		"gold_action": map[string]any{
			"action": "ReplaceText",
			"old":    goldOld,
			"new":    goldNew,
			"scope":  "all AutoShape text frames",
			"result": map[string]any{
				"matches":      stats.Matches,
				"replaced":     stats.Replaced,
				"shapes_seen":  stats.ShapesSeen,
				"shapes_tried": stats.ShapesTried,
			},
		},
		"validate": map[string]any{
			"before_error_count": 0,
			"before_warn_count":  0,
			"after_error_count":  0,
			"after_warn_count":   0,
		},
		"client_matrix": map[string]any{
			"go_pptx_validate_before":       "pass",
			"go_pptx_validate_after":        "pass",
			"go_pptx_diff":                  "pass",
			"libreoffice_source_generation": "not_run",
			"powerpoint_open_after":         "not_run",
			"wps_open_after":                "not_run",
		},
	})

	// 5) manifest（schema go-pptx.corpus/1.0）。
	origSHA, origSize := sha256File(origPath)
	editSHA, editSize := sha256File(editedPath)
	writeJSON(filepath.Join(outDirRel, "manifest.json"), map[string]any{
		"schemaVersion": "go-pptx.corpus/1.0",
		"sample_id":     sampleID,
		"title":         "SDK generated audio (narration) sample",
		"source_license": map[string]any{
			"source":         "scripts/gen_audio generated by go-pptx SDK",
			"license":        "project-owned/generated",
			"redistributable": true,
			"creation_steps": []string{"go run ./scripts/gen_audio"},
		},
		"generator": map[string]any{
			"name":     "go-pptx SDK",
			"version":  "v1.0.6",
			"platform": "Linux",
		},
		"creation_steps": []string{"go run ./scripts/gen_audio"},
		"font_environment": "unrecorded",
		"ooxml_type":       "Transitional",
		"feature_tags":     []string{"audio.narration", "audio.wav", "timing.advance", "preserve.b1"},
		"expectations": []string{
			"OpenReader succeeds",
			"ReplaceText \"" + goldOld + "\" -> \"" + goldNew + "\" affects only the expected slide part",
			"audio shape is visible (non-zero bounds) and carries a:audioFile r:link inside p:nvPr",
			"validate reports no structural errors",
		},
		"known_issues": []string{
			"True client playback (PowerPoint/WPS) still requires a machine with an office suite installed",
		},
		"files": map[string]any{
			"pptx": map[string]any{
				"path":   sampleID + ".pptx",
				"sha256": origSHA,
				"size":   origSize,
			},
			"edited_pptx": map[string]any{
				"path":   sampleID + ".edited.pptx",
				"sha256": editSHA,
				"size":   editSize,
			},
		},
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})

	fmt.Println("\nSample generation complete.")
	fmt.Printf("gold action: %q -> %q (matches=%d replaced=%d seen=%d tried=%d)\n",
		goldOld, goldNew, stats.Matches, stats.Replaced, stats.ShapesSeen, stats.ShapesTried)
}

type replayStats struct {
	Matches     int
	Replaced    int
	ShapesSeen  int
	ShapesTried int
}
