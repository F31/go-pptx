package pptx

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

// minimalMP4 构造最小 ftyp box（major=isom + minor=0 + compatible=mp42/avc1）。
// 仅含 ftyp；VIDEO-01 不解析完整 box 树，签名匹配即视为 mp4。
func minimalMP4() []byte {
	// ftyp box size = 16 + 2*4 = 24 字节（含 major + 2 compatible）。
	buf := []byte{
		0x00, 0x00, 0x00, 0x18, // size = 24
		'f', 't', 'y', 'p', // type=ftyp
		'i', 's', 'o', 'm', // major
		0x00, 0x00, 0x00, 0x00, // minor_version
		'm', 'p', '4', '2', // compatible
		'a', 'v', 'c', '1', // compatible
	}
	return buf
}

// minimalWebM 构造 EBML 头（含 0x1A45DFA3 + DocType="webm"）。
func minimalWebM() []byte {
	// 最小 EBML：4B 头 + DocType 元素 ID(0x4282)+size(VINT=4)+"webm"
	buf := []byte{
		0x1A, 0x45, 0xDF, 0xA3, // EBML signature
		0x84, // DocType element ID
		0x84, // size VINT (4 bytes payload)
		'w', 'e', 'b', 'm',
	}
	return buf
}

// minimalJPEG 生成最小 1×1 JPEG（VIDEO-01 poster frame 测试用）。
func minimalJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		panic("jpeg.Encode: " + err.Error())
	}
	return buf.Bytes()
}

func TestSlideAddVideo_MP4Basic(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides := SlidesOf(t, p)
	if len(slides) == 0 {
		t.Fatal("no slides")
	}
	s := slides[0]
	shape, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "v-clip-1",
		Role:     VideoRoleMain,
		X:        914400, Y: 914400,
		Width: 5486400, Height: 3086100,
		AltText: "Demo video",
	})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	if shape == nil {
		t.Fatal("shape nil")
	}
	if shape.Kind() != ShapeVideo {
		t.Errorf("Kind = %v, want ShapeVideo", shape.Kind())
	}
	if shape.Role() != VideoRoleMain {
		t.Errorf("Role = %v, want Main", shape.Role())
	}
	// media Part 已暂存
	if !p.hasPartCurrent("/ppt/media/video1.mp4") {
		t.Errorf("video media part not staged")
	}
	// /docProps/video.xml 已落
	if !p.hasPartCurrent("/docProps/video.xml") {
		t.Errorf("video.xml not staged")
	}
	// Profile 包含 TrackKey
	vpStr := p.DebugVideoXML()
	if !strings.Contains(vpStr, `trackKey="v-clip-1"`) {
		t.Errorf("video.xml missing trackKey: %s", vpStr)
	}
}

func TestSlideAddVideo_RepeatTrackKeyRejected(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "dup",
		Role:     VideoRoleMain,
		Width:    100, Height: 100,
	}); err != nil {
		t.Fatalf("first AddVideo: %v", err)
	}
	_, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "dup",
		Role:     VideoRoleMain,
		Width:    100, Height: 100,
	})
	if err == nil {
		t.Fatal("expected error for duplicate TrackKey")
	}
	if !strings.Contains(err.Error(), "TrackKey") {
		t.Errorf("error = %v, want TrackKey mention", err)
	}
}

func TestSlideAddVideo_WebMSupported(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	shape, err := s.AddVideo(context.Background(), BytesMedia(minimalWebM(), "video/webm"), VideoSpec{
		TrackKey: "w-clip",
		Width:    200, Height: 200,
	})
	if err != nil {
		t.Fatalf("AddVideo webm: %v", err)
	}
	if shape.Kind() != ShapeVideo {
		t.Errorf("Kind = %v, want ShapeVideo", shape.Kind())
	}
	if !p.hasPartCurrent("/ppt/media/video1.webm") {
		t.Errorf("webm media part not staged")
	}
}

func TestSlideAddVideo_UnknownFormatRejected(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	_, err := s.AddVideo(context.Background(), BytesMedia([]byte("not a video"), "video/x"), VideoSpec{
		TrackKey: "bad",
		Width:    100, Height: 100,
	})
	if err == nil {
		t.Fatal("expected error for unknown container")
	}
}

func TestSlideAddVideo_EmptyBytesRejected(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	_, err := s.AddVideo(context.Background(), BytesMedia(nil, "video/mp4"), VideoSpec{
		TrackKey: "empty",
		Width:    100, Height: 100,
	})
	if err == nil {
		t.Fatal("expected error for empty bytes")
	}
}

func TestSlideAddVideo_InvalidBoundsRejected(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	_, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "bad-bounds",
		Width:    0, Height: 100,
	})
	if err == nil {
		t.Fatal("expected error for zero width")
	}
}

func TestSlideAddVideo_TrackKeyValidation(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	_, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "bad/key",
		Width:    100, Height: 100,
	})
	if err == nil {
		t.Fatal("expected error for path separator in TrackKey")
	}
	_, err = s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "",
		Width:    100, Height: 100,
	})
	if err == nil {
		t.Fatal("expected error for empty TrackKey")
	}
}

func TestSlideAddVideo_PosterFrame(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	shape, err := s.AddVideo(context.Background(),
		BytesMedia(minimalMP4(), "video/mp4"),
		VideoSpec{
			TrackKey: "with-poster",
			Width:    400, Height: 300,
			PosterSource: BytesMedia(minimalJPEG(), "image/jpeg"),
			PosterAlt:    "Poster",
		})
	if err != nil {
		t.Fatalf("AddVideo with poster: %v", err)
	}
	if !shape.HasPoster() {
		t.Errorf("HasPoster = false, want true")
	}
	// Poster image Part 也已暂存。
	if !p.hasPartCurrent("/ppt/media/image1.jpg") {
		t.Errorf("poster image part not staged")
	}
	// VideoShape.Kind() 仍为 video（poster p:pic 不会被分类成 ShapeVideo）。
	if shape.Kind() != ShapeVideo {
		t.Errorf("Kind = %v, want ShapeVideo", shape.Kind())
	}
}

func TestSlideAddVideo_AltTextAndDecorative(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	shape, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "alt",
		Width:    100, Height: 100,
		AltText: "Demo clip",
	})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	if shape.AltText() != "Demo clip" {
		t.Errorf("AltText = %q, want Demo clip", shape.AltText())
	}
	if shape.IsDecorative() {
		t.Errorf("IsDecorative = true with AltText set")
	}

	// Decorative 优先于 AltText（空 alt 也允许；装饰性声明）。
	shape2, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "dec",
		Width:    100, Height: 100,
		IsDecorative: true,
	})
	if err != nil {
		t.Fatalf("AddVideo decorative: %v", err)
	}
	if !shape2.IsDecorative() {
		t.Errorf("IsDecorative = false")
	}
}

func TestSlideAddVideo_ClosedSessionRejected(t *testing.T) {
	p := audioDeck(t)
	slides := SlidesOf(t, p)
	if len(slides) == 0 {
		t.Fatal("no slides")
	}
	s := slides[0]
	p.Close()
	_, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "after-close",
		Width:    100, Height: 100,
	})
	if err == nil {
		t.Fatal("expected error after Close")
	}
}

func TestSlideAddVideo_DuplicateContentDedupe(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	// 同一字节嵌入两次（不同 TrackKey）→ 应复用 media Part。
	_, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "first",
		Width:    100, Height: 100,
	})
	if err != nil {
		t.Fatalf("first AddVideo: %v", err)
	}
	_, err = s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "second",
		Width:    100, Height: 100,
	})
	if err != nil {
		t.Fatalf("second AddVideo: %v", err)
	}
	if !p.hasPartCurrent("/ppt/media/video1.mp4") {
		t.Errorf("expected deduplication of media Part")
	}
	if p.hasPartCurrent("/ppt/media/video2.mp4") {
		t.Errorf("unexpected second media Part (should dedupe)")
	}
}

func TestSlideAddVideo_BoundsReflected(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	shape, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "geom",
		X:        100, Y: 200,
		Width:  500,
		Height: 400,
	})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	b, err := shape.Bounds()
	if err != nil {
		t.Fatalf("Bounds: %v", err)
	}
	if b.X != 100 || b.Y != 200 || b.W != 500 || b.H != 400 {
		t.Errorf("Bounds = %+v, want {100,200,500,400}", b)
	}
}

func TestSlideAddVideo_VideoSourceAccess(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	data := minimalMP4()
	shape, err := s.AddVideo(context.Background(), BytesMedia(data, "video/mp4"), VideoSpec{
		TrackKey: "vs",
		Width:    100, Height: 100,
	})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	src, err := shape.VideoSource()
	if err != nil {
		t.Fatalf("VideoSource: %v", err)
	}
	rc, err := src.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Errorf("video bytes mismatch")
	}
}

func TestSlideAddVideo_PosterSourceAccess(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	poster := minimalJPEG()
	shape, err := s.AddVideo(context.Background(),
		BytesMedia(minimalMP4(), "video/mp4"),
		VideoSpec{
			TrackKey: "ps",
			Width:    100, Height: 100,
			PosterSource: BytesMedia(poster, "image/jpeg"),
		})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	src, err := shape.PosterSource()
	if err != nil {
		t.Fatalf("PosterSource: %v", err)
	}
	rc, err := src.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), poster) {
		t.Errorf("poster bytes mismatch")
	}
}

func TestSlideClone_PreservesVideoProfile(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	_, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "orig",
		Width:    100, Height: 100,
	})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	c, err := s.Clone(nil)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	// 克隆页应含视频形状（独立复用默认下媒体共享）。
	cShapes, err := c.Shapes()
	if err != nil {
		t.Fatalf("Clone Shapes: %v", err)
	}
	var videoCount int
	for _, sh := range cShapes {
		if sh.Kind() == ShapeVideo {
			videoCount++
		}
	}
	if videoCount == 0 {
		t.Errorf("cloned slide has no video shape")
	}
	// /docProps/video.xml 含派生 TrackKey（"-c" 后缀）。
	vp := p.DebugVideoXML()
	if !strings.Contains(vp, `trackKey="orig-c"`) {
		t.Errorf("video.xml missing derived trackKey: %s", vp)
	}
}

func TestVideoShapeKind_Registered(t *testing.T) {
	if ShapeVideo.String() != "video" {
		t.Errorf("ShapeVideo.String = %q, want video", ShapeVideo.String())
	}
	if ShapeAudio.String() != "audio" {
		t.Errorf("ShapeAudio.String = %q, want audio", ShapeAudio.String())
	}
}

func TestVideoAddVideo_ProbeErrorMapped(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	// 损坏 MP4（extended size 的 ftyp）→ ErrUnsupportedFormat。
	bad := []byte{0, 0, 0, 1, 'x', 'x', 'x', 'x', 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	_, err := s.AddVideo(context.Background(), BytesMedia(bad, "video/mp4"),
		VideoSpec{TrackKey: "bad", Width: 100, Height: 100})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
	if err := mapVideoProbeError(nil); err != nil {
		t.Fatalf("mapVideoProbeError(nil) = %v", err)
	}
}

func TestVideoShape_ProfileAndAccessors(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	data := minimalMP4()
	shape, err := s.AddVideo(context.Background(), BytesMedia(data, "video/mp4"), VideoSpec{
		TrackKey: "prof",
		Role:     VideoRoleMain,
		Width:    100, Height: 100,
	})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	if shape.Kind() != ShapeVideo {
		t.Fatalf("Kind = %v", shape.Kind())
	}
	if shape.Role() != VideoRoleMain {
		t.Fatalf("Role = %v", shape.Role())
	}
	prof := shape.Profile()
	if prof.TrackKey != "prof" || prof.MediaPart == "" || prof.ContentSHA256 == "" {
		t.Fatalf("profile = %+v", prof)
	}
	src, err := shape.VideoSource()
	if err != nil {
		t.Fatalf("VideoSource: %v", err)
	}
	if got := src.DeclaredType(); got != "video/mp4" {
		t.Fatalf("DeclaredType = %q", got)
	}
	if got := videoCTForExt("WEBM"); got != "video/webm" {
		t.Fatalf("videoCTForExt(WEBM) = %q", got)
	}
	if got := videoCTForExt("avi"); got != "application/octet-stream" {
		t.Fatalf("videoCTForExt(avi) = %q", got)
	}
	if shape.HasPoster() {
		t.Fatalf("HasPoster = true, want false")
	}
	if _, err := shape.PosterSource(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PosterSource without poster: %v, want ErrNotFound", err)
	}
	if got := p.DebugVideoXML(); !strings.Contains(got, "VideoProfile") {
		t.Fatalf("DebugVideoXML missing profile: %.200s", got)
	}
}

func TestVideoShape_ErrorBranches(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	shape, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"),
		VideoSpec{TrackKey: "err", Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := shape.VideoSource(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed VideoSource: %v, want ErrClosed", err)
	}
	if _, err := shape.PosterSource(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed PosterSource: %v, want ErrClosed", err)
	}
	// 空 profile → ErrNotFound。
	p2 := audioDeck(t)
	defer p2.Close()
	empty := &VideoShape{shapeNode: shapeNode{p: p2}}
	if _, err := empty.VideoSource(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty profile VideoSource: %v, want ErrNotFound", err)
	}
}

func TestSlideAddVideo_PicClassifiedAsVideo(t *testing.T) {

	p := audioDeck(t)
	defer p.Close()
	s := SlidesOf(t, p)[0]
	if _, err := s.AddVideo(context.Background(), BytesMedia(minimalMP4(), "video/mp4"), VideoSpec{
		TrackKey: "classify",
		Width:    100, Height: 100,
	}); err != nil {
		t.Fatalf("AddVideo: %v", err)
	}
	shapes, err := s.Shapes()
	if err != nil {
		t.Fatalf("Shapes: %v", err)
	}
	var sawVideo bool
	for _, sh := range shapes {
		if sh.Kind() == ShapeVideo {
			sawVideo = true
		}
		if sh.Kind() == ShapePicture {
			t.Errorf("ShapePicture detected for video p:pic (should be ShapeVideo)")
		}
	}
	if !sawVideo {
		t.Errorf("no ShapeVideo after AddVideo")
	}
}
