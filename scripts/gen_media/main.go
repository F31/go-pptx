// gen_media 生成 L3 真机媒体兼容矩阵所需的 fixture（非语料金样）：
//
//   - audio-auto.pptx   音频 + 幻灯片进入时自动播放
//   - audio-click.pptx  音频 + 单击图标播放
//   - video.pptx        视频
//
// 用法：
//
//	go run ./scripts/gen_media -out .l3-output/media
//
// 目的：把 ADR-027 的"媒体四态"（能打开 / 图标可见 / 可点击 / 有声）
// 在两家客户端上逐一验证——尤其覆盖 click 触发路径（语料 s004-audio 走 auto）。
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"

	pptx "github.com/F31/go-pptx/pptx"
)

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
	write(uint16(1))
	write(uint16(1))
	write(uint32(sampleRate))
	write(uint32(sampleRate * 2))
	write(uint16(2))
	write(uint16(16))
	buf.WriteString("data")
	write(uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

// minimalMP4 最小 ftyp box。
func minimalMP4() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p',
		'i', 's', 'o', 'm', 0, 0, 0, 0,
		'm', 'p', '4', '2', 'a', 'v', 'c', '1',
	}
}

func newSlide(label string) (*pptx.Presentation, *pptx.Slide, error) {
	p, err := pptx.New()
	if err != nil {
		return nil, nil, err
	}
	layouts, err := p.Layouts()
	if err != nil {
		return nil, nil, err
	}
	sl, err := p.AddSlide(layouts[0])
	if err != nil {
		return nil, nil, err
	}
	if _, err := sl.AddTextBox(pptx.TextBoxSpec{
		X: 914400, Y: 914400, Width: 6096000, Height: 914400, Text: label,
	}); err != nil {
		return nil, nil, err
	}
	return p, sl, nil
}

func genAudio(outDir, name string, trigger pptx.PlaybackTrigger) error {
	p, sl, err := newSlide("Media matrix: " + name)
	if err != nil {
		return err
	}
	defer p.Close()
	wav := synthWAV(2.0, 44100)
	as, err := sl.AddAudio(context.Background(), pptx.BytesMedia(wav, "audio/wav"), pptx.AudioSpec{
		TrackKey: "narr", Role: pptx.AudioRoleNarration, Source: pptx.BytesMedia(wav, "audio/wav"),
	})
	if err != nil {
		return err
	}
	if err := as.SetPlayback(pptx.PlaybackSpec{Trigger: trigger}); err != nil {
		return err
	}
	path := filepath.Join(outDir, name+".pptx")
	if _, err := p.Save(context.Background(), path); err != nil {
		return err
	}
	fmt.Println("created", path)
	return nil
}

func genVideo(outDir, name string) error {
	p, sl, err := newSlide("Media matrix: " + name)
	if err != nil {
		return err
	}
	defer p.Close()
	if _, err := sl.AddVideo(context.Background(), pptx.BytesMedia(minimalMP4(), "video/mp4"), pptx.VideoSpec{
		TrackKey: "clip", Role: pptx.VideoRoleMain,
		X: 914400, Y: 2743200, Width: 5486400, Height: 3086100,
	}); err != nil {
		return err
	}
	path := filepath.Join(outDir, name+".pptx")
	if _, err := p.Save(context.Background(), path); err != nil {
		return err
	}
	fmt.Println("created", path)
	return nil
}

func main() {
	out := flag.String("out", ".l3-output/media", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		panic(err)
	}
	if err := genAudio(*out, "audio-auto", pptx.PlaybackOnSlideEnter); err != nil {
		panic(err)
	}
	if err := genAudio(*out, "audio-click", pptx.PlaybackOnClick); err != nil {
		panic(err)
	}
	if err := genVideo(*out, "video"); err != nil {
		panic(err)
	}
}
