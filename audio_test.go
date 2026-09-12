package pptx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/F31/go-pptx/internal/audioprobe"
	"github.com/F31/go-pptx/internal/opc"
)

// minimalWAV 构造最小 WAV/PCM（16-bit mono 8 kHz，dataSize 字节）。
func minimalWAV(dataSize int) []byte {
	wav := []byte(`RIFF`)
	rest := make([]byte, 4)
	binary.LittleEndian.PutUint32(rest, uint32(36+dataSize))
	wav = append(wav, rest...)
	wav = append(wav, []byte(`WAVEfmt `)...)
	// fmt chunk: 4 字节 size（16） + data（fmt_ID, channels, rate, byte_rate, block, bits）
	ch := make([]byte, 20)
	binary.LittleEndian.PutUint32(ch[0:4], 16)      // fmt chunk size
	binary.LittleEndian.PutUint16(ch[4:6], 0x0001)  // PCM
	binary.LittleEndian.PutUint16(ch[6:8], 1)       // channels
	binary.LittleEndian.PutUint32(ch[8:12], 8000)   // sample rate
	binary.LittleEndian.PutUint32(ch[12:16], 16000) // byte rate
	binary.LittleEndian.PutUint16(ch[16:18], 2)     // block align
	binary.LittleEndian.PutUint16(ch[18:20], 16)    // bits per sample
	wav = append(wav, ch...)
	wav = append(wav, []byte(`data`)...)
	dh := make([]byte, 4)
	binary.LittleEndian.PutUint32(dh, uint32(dataSize))
	wav = append(wav, dh...)
	body := make([]byte, dataSize)
	return append(wav, body...)
}

// minimalMP3 构造 1 帧 MPEG1/L3 128kbps @ 44.1kHz CBR（含 4 字节头，全帧 417B）。
func minimalMP3() []byte {
	h := uint32(0xFFE00000) |
		uint32(0b11)<<19 |
		uint32(0b01)<<17 |
		uint32(1)<<16 |
		uint32(9)<<12 |
		uint32(0)<<10
	out := make([]byte, 144*128000/44100)
	out[0] = byte(h >> 24)
	out[1] = byte(h >> 16)
	out[2] = byte(h >> 8)
	out[3] = byte(h)
	return out
}

// ---------- AUDIO-01 测试 ----------

func TestAudioProbeWrapper(t *testing.T) {
	mp3 := minimalMP3()
	info, err := audioprobe.Probe(audioprobe.ProbeInput{Data: mp3})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if info.Container != "mp3" || info.State != audioprobe.DurationKnown {
		t.Errorf("got %+v", info)
	}
}

func TestAudioAddAudio_MP3(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, err := p.Slides()
	if err != nil || len(slides) == 0 {
		t.Fatalf("Slides: %v len=%d", err, len(slides))
	}
	s := slides[0]
	mp3 := minimalMP3()
	spec := AudioSpec{
		TrackKey: "voice1",
		Role:     AudioRoleNarration,
		Source:   BytesMedia(mp3, "audio/mpeg"),
		Duration: NewOptional[time.Duration](time.Second),
	}
	as, err := s.AddAudio(context.Background(), spec.Source, spec)
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	if as.ID() == 0 {
		t.Error("id not assigned")
	}
	if as.Role() != AudioRoleNarration {
		t.Errorf("role = %v", as.Role())
	}
	if got := as.Profile().TrackKey; got != "voice1" {
		t.Errorf("trackkey = %q", got)
	}
	// Source 探出并写出 Part。
	if as.profile.MediaPart == "" {
		t.Fatal("no media part")
	}
	if !p.pk.HasPart(as.profile.MediaPart) && p.addedParts[as.profile.MediaPart].Content == nil {
		t.Errorf("media part %s not present after commit", as.profile.MediaPart)
	}
}

func TestAudioAddAudio_WAV(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	wav := minimalWAV(16000) // 1s
	spec := AudioSpec{
		TrackKey: "voiceW",
		Role:     AudioRoleBackground,
		Source:   BytesMedia(wav, "audio/wav"),
	}
	as, err := slides[0].AddAudio(context.Background(), spec.Source, spec)
	if err != nil {
		t.Fatalf("AddAudio WAV: %v", err)
	}
	if as.profile.MediaPart == "" {
		t.Fatal("no media part")
	}
}

func TestAudioAddAudio_InvalidArgs(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	// 空 track key
	_, err := slides[0].AddAudio(context.Background(), BytesMedia(minimalMP3(), "audio/mpeg"),
		AudioSpec{TrackKey: "", Source: BytesMedia(minimalMP3(), ""), Role: AudioRoleNarration})
	if err == nil {
		t.Error("expected error on empty track key")
	}
	// 源为 nil
	_, err = slides[0].AddAudio(context.Background(), nil, AudioSpec{TrackKey: "x", Source: nil, Role: AudioRoleNarration})
	if err == nil {
		t.Error("expected error on nil source")
	}
}

func TestAudioAddAudio_DuplicateTrackKey(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	if _, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "dup", Source: BytesMedia(mp3, "audio/mpeg"), Role: AudioRoleNarration}); err != nil {
		t.Fatalf("first AddAudio: %v", err)
	}
	_, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "dup", Source: BytesMedia(mp3, "audio/mpeg"), Role: AudioRoleNarration})
	if !errors.Is(err, ErrUnsupportedEdit) {
		t.Errorf("err = %v, want ErrUnsupportedEdit", err)
	}
}

func TestAudioAddAudio_DurationUnknownRejected(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	// 故意构造 WAV 但 data 长度为零 → probe 返回 DurationUnknown。
	wav := minimalWAV(0) // data chunk 长度为 0
	spec := AudioSpec{TrackKey: "unknown", Source: BytesMedia(wav, "audio/wav")}
	_, err := slides[0].AddAudio(context.Background(), spec.Source, spec)
	if !errors.Is(err, ErrDurationUnknown) {
		t.Errorf("err = %v, want ErrDurationUnknown", err)
	}
}

func TestAudioShape_AudioSourceReopensBytes(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "ref", Source: BytesMedia(mp3, "audio/mpeg"), Role: AudioRoleNarration})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	src, err := as.AudioSource()
	if err != nil {
		t.Fatalf("AudioSource: %v", err)
	}
	r, err := src.Open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()
	got := sha256.New()
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			got.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	if !bytes.Equal(got.Sum(nil)[:8], sha256Bytes(mp3)[:8]) {
		t.Errorf("source bytes mismatch")
	}
}

func sha256Bytes(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func TestAudioProfile_RecordsAllFields(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "fields", Source: BytesMedia(mp3, "audio/mpeg"), Role: AudioRoleNarration})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	ap := as.Profile()
	if ap.MediaPart == "" || ap.ContentSHA256 == "" {
		t.Errorf("ap = %+v", ap)
	}
	if ap.Version != 1 {
		t.Errorf("version = %d", ap.Version)
	}
	// Part 必须存在 + Content Type 正确。
	if p.pk.HasPart(ap.MediaPart) {
		ct, _ := p.pk.ContentType(ap.MediaPart)
		if ct != "audio/mpeg" {
			t.Errorf("ct = %q", ct)
		}
	} else if got := p.addedParts[ap.MediaPart].ContentType; got != "audio/mpeg" {
		t.Errorf("added ct = %q", got)
	}
}

func TestAudioShape_KindAndSourceAccessors(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "acc", Source: BytesMedia(mp3, "audio/mpeg"), Role: AudioRoleBackground})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	if as.Kind() != ShapeAudio {
		t.Fatalf("Kind = %v, want ShapeAudio", as.Kind())
	}
	src, err := as.AudioSource()
	if err != nil {
		t.Fatalf("AudioSource: %v", err)
	}
	if got := src.DeclaredType(); got != "audio/mpeg" {
		t.Fatalf("DeclaredType = %q", got)
	}
	if got := audioCTForExt("WAV"); got != "audio/wav" {
		t.Fatalf("audioCTForExt(WAV) = %q", got)
	}
	if got := audioCTForExt("ogg"); got != "application/octet-stream" {
		t.Fatalf("audioCTForExt(ogg) = %q", got)
	}
	if got := extOfMedia("/ppt/media/audio1.mp3"); got != "mp3" {
		t.Fatalf("extOfMedia = %q", got)
	}
	if got := extOfMedia("/ppt/media/noext"); got != "" {
		t.Fatalf("extOfMedia(noext) = %q", got)
	}
	if got := p.DebugAudioXML(); !strings.Contains(got, "AudioProfile") {
		t.Fatalf("DebugAudioXML missing profile: %.200s", got)
	}
	var nilP *Presentation
	if got := nilP.DebugAudioXML(); got != "" {
		t.Fatalf("nil DebugAudioXML = %q", got)
	}
}

func TestAudioShape_AudioSourceErrorBranches(t *testing.T) {
	p := audioDeck(t)
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	as, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "err", Source: BytesMedia(mp3, "audio/mpeg"), Role: AudioRoleNarration})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := as.AudioSource(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed AudioSource: %v, want ErrClosed", err)
	}
	// 空 profile → ErrNotFound。
	p2 := audioDeck(t)
	defer p2.Close()
	empty := &AudioShape{shapeNode: shapeNode{p: p2}}
	if _, err := empty.AudioSource(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty profile AudioSource: %v, want ErrNotFound", err)
	}
}

func TestAudioAddAudio_ProbeErrorMapped(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	slides, _ := p.Slides()
	// 损坏 ID3 头：probe 失败 → ErrUnsupportedFormat（mapProbeError 映射）。
	bad := []byte{'I', 'D', '3', 4, 0, 0, 0x7f, 0x7f, 0x7f, 0x7f}
	_, err := slides[0].AddAudio(context.Background(), BytesMedia(bad, "audio/mpeg"),
		AudioSpec{TrackKey: "bad", Source: BytesMedia(bad, "audio/mpeg"),
			Role: AudioRoleNarration, Duration: NewOptional[time.Duration](time.Second)})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
	if err := mapProbeError(nil); err != nil {
		t.Fatalf("mapProbeError(nil) = %v", err)
	}
}

func TestAudioDebugAddedParts(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	// 空文档：无 added/override。
	if got := p.debugAddedParts(); len(got) != 0 {
		t.Fatalf("empty debugAddedParts = %v", got)
	}
	// 添加一个 Part 后出现。
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	if _, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"),
		AudioSpec{TrackKey: "dbg", Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](time.Second)}); err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	got := p.debugAddedParts()
	if len(got) == 0 {
		t.Fatal("debugAddedParts empty after AddAudio")
	}
	var sawAudio bool
	for _, n := range got {
		if strings.HasPrefix(n, "/docProps/audio.xml") {
			sawAudio = true
		}
	}
	if !sawAudio {
		t.Fatalf("audio.xml not staged: %v", got)
	}
}

func TestAudioHelperEdges(t *testing.T) {
	if got := roleFromString(" narration "); got != AudioRoleNarration {
		t.Fatalf("role narration = %v", got)
	}
	if got := roleFromString("BACKGROUND"); got != AudioRoleBackground {
		t.Fatalf("role background = %v", got)
	}
	if got := roleFromString("Effect"); got != AudioRoleEffect {
		t.Fatalf("role effect = %v", got)
	}
	if got := roleFromString("weird"); got != AudioRoleNarration {
		t.Fatalf("role unknown = %v", got)
	}
	if got := audioContentType(audioprobe.MediaInfo{Container: "wav"}); got != "audio/wav" {
		t.Fatalf("ct wav = %q", got)
	}
	if got := audioContentType(audioprobe.MediaInfo{Container: "mp3"}); got != "audio/mpeg" {
		t.Fatalf("ct mp3 = %q", got)
	}
	if got := audioContentType(audioprobe.MediaInfo{Container: "ogg"}); got != "application/octet-stream" {
		t.Fatalf("ct ogg = %q", got)
	}
	if got := audioExt(audioprobe.MediaInfo{Container: "wav"}); got != "wav" {
		t.Fatalf("ext wav = %q", got)
	}
	if got := audioExt(audioprobe.MediaInfo{Container: "ogg"}); got != "bin" {
		t.Fatalf("ext ogg = %q", got)
	}
	if got := hintFromName("/ppt/media/x.mp3"); got != "mp3" {
		t.Fatalf("hint = %q", got)
	}
	if got := hintFromName("noext"); got != "" {
		t.Fatalf("hint noext = %q", got)
	}
	var sb strings.Builder
	xmlEscapeAttr(&sb, `a"b&c<d>e`)
	if got := sb.String(); got != `a&quot;b&amp;c&lt;d&gt;e` {
		t.Fatalf("xmlEscapeAttr = %q", got)
	}
	sb.Reset()
	xmlEscapeAttr(&sb, "plain")
	if got := sb.String(); got != "plain" {
		t.Fatalf("xmlEscapeAttr plain = %q", got)
	}
}

func TestAudioPlaybackSpecDefaults(t *testing.T) {
	var spec PlaybackSpec
	if got := spec.Trigger.String(); got != "onSlideEnter" {
		t.Fatalf("default trigger = %q", got)
	}
}

func TestAudioDedupSharedMediaPart(t *testing.T) {
	p := audioDeck(t)
	slides, _ := p.Slides()
	mp3 := minimalMP3()
	spec := func(key string) AudioSpec {
		return AudioSpec{TrackKey: key, Source: BytesMedia(mp3, "audio/mpeg"),
			Role:     AudioRoleNarration,
			Duration: NewOptional[time.Duration](time.Second)}
	}
	as1, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"), spec("a1"))
	if err != nil {
		t.Fatalf("AddAudio a1: %v", err)
	}
	as2, err := slides[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"), spec("a2"))
	if err != nil {
		t.Fatalf("AddAudio a2: %v", err)
	}
	if as1.profile.MediaPart != as2.profile.MediaPart {
		t.Fatalf("dedup failed: %s vs %s", as1.profile.MediaPart, as2.profile.MediaPart)
	}
	// 保存 → 重开 → 再同字节 AddAudio：复用已提交 Part（pk 分支）。
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p.Close()
	p2 := openFixture(t, buf.Bytes())
	defer p2.Close()
	slides2, _ := p2.Slides()
	as3, err := slides2[0].AddAudio(context.Background(), BytesMedia(mp3, "audio/mpeg"), spec("a3"))
	if err != nil {
		t.Fatalf("AddAudio a3: %v", err)
	}
	if as3.profile.MediaPart != as1.profile.MediaPart {
		t.Fatalf("pk dedup failed: %s vs %s", as3.profile.MediaPart, as1.profile.MediaPart)
	}
	// 不同内容 → 新 Part。
	mp3b := append([]byte(nil), mp3...)
	mp3b[4] ^= 0xff
	as4, err := slides2[0].AddAudio(context.Background(), BytesMedia(mp3b, "audio/mpeg"), spec("a4"))
	if err != nil {
		t.Fatalf("AddAudio a4: %v", err)
	}
	if as4.profile.MediaPart == as1.profile.MediaPart {
		t.Fatalf("different content reused part %s", as4.profile.MediaPart)
	}
}

func TestAudioSourceRefOpenFailure(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	// 指向不存在的 part → Open 返回 ErrNotFound。
	src := sourceRef{part: "/ppt/media/nope.mp3", ct: "audio/mpeg", p: p}
	if _, err := src.Open(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open missing part err = %v, want ErrNotFound", err)
	}
	// 已关闭 → ErrClosed。
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := src.Open(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Open closed err = %v, want ErrClosed", err)
	}
}

func TestLastAudioHandleNotFound(t *testing.T) {
	// 无音频 pic 的页面 → ErrNotFound。
	p := slideWithBody(t, `<a:bodyPr/><a:p><a:r><a:t>X</a:t></a:r></a:p>`)
	defer p.Close()
	s := mustSlide(t, p)
	if _, err := s.lastAudioHandle(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lastAudioHandle err = %v, want ErrNotFound", err)
	}
}

func TestRecordAudioProfileCorruptedReset(t *testing.T) {
	p := audioDeck(t)
	defer p.Close()
	// 既有 audio.xml 无 </AudioProfiles> 闭标签 → 重置为仅含本条目。
	corrupt := []byte(`<?xml version="1.0"?><AudioProfiles xmlns="x"><Profile trackKey="stale"/></AudioProfiles`)
	if err := p.stagePatch("/docProps/audio.xml", corrupt); err != nil {
		t.Fatalf("stagePatch: %v", err)
	}
	p.commit()
	if err := p.recordAudioProfile(AudioProfile{TrackKey: "fresh", MediaPart: "/ppt/media/a.mp3", ContentSHA256: "s", Version: 1}); err != nil {
		t.Fatalf("recordAudioProfile: %v", err)
	}
	if got := p.DebugAudioXML(); !strings.Contains(got, "fresh") || strings.Contains(got, "stale") {
		t.Fatalf("reset audio.xml = %s", got)
	}
	// 无 Part → 新建全量。
	p2 := audioDeck(t)
	defer p2.Close()
	if err := p2.recordAudioProfile(AudioProfile{TrackKey: "new", MediaPart: "/ppt/media/b.mp3"}); err != nil {
		t.Fatalf("recordAudioProfile new: %v", err)
	}
	if got := p2.DebugAudioXML(); !strings.Contains(got, "new") {
		t.Fatalf("new audio.xml = %s", got)
	}
}

// audioDeck 是一个含 spTree 的最小可扩展模板。
func audioDeck(t *testing.T) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	const nsP = nsPresentationML
	const nsA = nsDrawingML
	parts["/ppt/slides/slide1.xml"] = []byte(xmlDecl +
		`<p:sld xmlns:a="` + nsA + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>` +
		`</p:sld>`)
	// 注册 slide Content-Type 与父 rels。
	ct := []byte(xmlDecl + `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	_ = ct
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rIdS1" Type="` + opc.RelSlide + `" Target="slides/slide1.xml"/>` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/slides/_rels/slide1.xml.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideLayout + `" Target="../slideLayouts/slideLayout1.xml"/>` +
		`</Relationships>`)
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsP + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst><p:sldId id="256" r:id="rIdS1"/></p:sldIdLst>` +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	parts["/[Content_Types].xml"] = append(parts["/[Content_Types].xml"][:len(parts["/[Content_Types].xml"])-len("</Types>")],
		[]byte(`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`)...)
	zipBytes, err := buildPackageZip(parts)
	if err != nil {
		t.Fatalf("buildPackageZip: %v", err)
	}
	p, err := OpenReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	return p
}

// Suppress unused-import checks if some helper above isn't used.
var _ = strings.TrimSpace
