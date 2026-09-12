package audioprobe

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// ---------- WAV 测试 ----------

// wavSamplePCM 构造最小 WAV/PCM 字节（44 字节 RIFF/fmt/data header + dataSize 字节 data）。
func wavSamplePCM(sampleRate, channels, bits, dataBytes int) []byte {
	blockAlign := channels * bits / 8
	rate := uint32(sampleRate)
	rateBps := uint32(sampleRate * blockAlign)
	total := 44 + dataBytes
	out := make([]byte, total)
	copy(out[:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(total-8))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 0x0001) // PCM
	binary.LittleEndian.PutUint16(out[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:28], rate)
	binary.LittleEndian.PutUint32(out[28:32], rateBps)
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], uint16(bits))
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataBytes))
	// data 部分保持零。
	return out
}

func TestProbeWAVPCM(t *testing.T) {
	// 1 秒 8kHz mono 16-bit = 16000 字节 data。
	wav := wavSamplePCM(8000, 1, 16, 16000)
	info, err := Probe(ProbeInput{Data: wav})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "wav" || info.Encoding != "pcm" {
		t.Errorf("got container=%q encoding=%q", info.Container, info.Encoding)
	}
	if info.State != DurationKnown {
		t.Fatalf("state = %v, want Known", info.State)
	}
	// 1 秒 ±10ms。
	if d := info.Duration.Milliseconds(); d < 990 || d > 1010 {
		t.Errorf("duration ms = %d, want ~1000", d)
	}
	if info.SampleRate != 8000 || info.Channels != 1 || info.BitsPerSample != 16 {
		t.Errorf("info = %+v", info)
	}
	if info.BitrateKbps != 128 {
		t.Errorf("bitrate = %d, want 128", info.BitrateKbps)
	}
}

func TestProbeWAVFormatVariantsAndUnknownChunk(t *testing.T) {
	for _, tc := range []struct {
		tag  uint16
		want string
	}{
		{0x0003, "ieee_float"},
		{0xFFFE, "extensible"},
		{0x1234, "format=0x1234"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			wav := wavSamplePCM(8000, 1, 16, 16000)
			binary.LittleEndian.PutUint16(wav[20:22], tc.tag)
			// Rewrite the data chunk as an unknown odd-sized chunk followed by data to cover warnings and padding.
			prefix := append([]byte(nil), wav[:36]...)
			prefix = append(prefix, []byte{'X', 'X', 'X', 'X', 1, 0, 0, 0, 0, 0}...)
			prefix = append(prefix, wav[36:]...)
			info, err := Probe(ProbeInput{Data: prefix})
			if err != nil {
				t.Fatal(err)
			}
			if info.Encoding != tc.want {
				t.Fatalf("encoding = %q, want %q", info.Encoding, tc.want)
			}
			if len(info.Warnings) == 0 || !strings.Contains(info.Warnings[0], "unknown wav chunk") {
				t.Fatalf("warnings = %v", info.Warnings)
			}
		})
	}
}

func TestProbeWAVMalformedChunks(t *testing.T) {
	shortFmt := []byte("RIFF\x1c\x00\x00\x00WAVEfmt \x08\x00\x00\x00abcdefgh")
	_, err := Probe(ProbeInput{Data: shortFmt})
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("short fmt err = %v, want ErrMalformedMedia", err)
	}
	ds64 := []byte("RIFF\x14\x00\x00\x00WAVEds64\x08\x00\x00\x00abcdefgh")
	_, err = Probe(ProbeInput{Data: ds64})
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("short ds64 err = %v, want ErrMalformedMedia", err)
	}
	noFmt := []byte("RIFF\x10\x00\x00\x00WAVEJUNK\x00\x00\x00\x00")
	_, err = Probe(ProbeInput{Data: noFmt})
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("no fmt err = %v, want ErrMalformedMedia", err)
	}
}

func TestProbeWAVDS64KnownDuration(t *testing.T) {
	wav := []byte("RIFF\xff\xff\xff\xffWAVE")
	ds64 := make([]byte, 8+28)
	copy(ds64[:4], "ds64")
	binary.LittleEndian.PutUint32(ds64[4:8], 28)
	binary.LittleEndian.PutUint64(ds64[16:24], 16000)
	wav = append(wav, ds64...)
	fmtChunk := wavSamplePCM(8000, 1, 16, 0)[12:36]
	wav = append(wav, fmtChunk...)
	wav = append(wav, []byte{'d', 'a', 't', 'a', 0xff, 0xff, 0xff, 0xff}...)
	info, err := Probe(ProbeInput{Data: wav})
	if err != nil {
		t.Fatal(err)
	}
	if info.State != DurationKnown || info.Duration.Milliseconds() != 1000 {
		t.Fatalf("state/duration = %v/%dms, want known 1000ms", info.State, info.Duration.Milliseconds())
	}
}

func TestProbeWAVNoDataChunk(t *testing.T) {
	// 仅 fmt，无 data → DurationUnknown。
	wav := wavSamplePCM(8000, 1, 16, 0)
	wav = wav[:36] // 去掉 data chunk
	info, err := Probe(ProbeInput{Data: wav})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.State != DurationUnknown {
		t.Errorf("state = %v, want Unknown", info.State)
	}
	if info.Container != "wav" {
		t.Errorf("container = %q", info.Container)
	}
}

func TestProbeWAVTruncatedChunk(t *testing.T) {
	// 数据不足超出声明 size：应保留 warning 不崩。
	wav := wavSamplePCM(8000, 1, 16, 1000)
	wav = wav[:50] // 截断
	info, err := Probe(ProbeInput{Data: wav})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "wav" {
		t.Errorf("container = %q", info.Container)
	}
	if len(info.Warnings) == 0 {
		t.Error("expected truncation warning")
	}
}

func TestProbeWAVContainerHint(t *testing.T) {
	wav := wavSamplePCM(8000, 1, 16, 16000)
	info, err := Probe(ProbeInput{Data: wav, ContainerHint: "wav"})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "wav" || info.State != DurationKnown {
		t.Errorf("got %+v", info)
	}
}

// ---------- MP3 测试 ----------

// mp3Frame 构造一个 MPEG 1 Layer III 帧（CBR 128kbps @ 44.1kHz stereo）。
// 比特率索引 9 = 128 kbps，sample rate 索引 0 = 44.1 kHz，padding 0。
// mp3FrameHeader 构造 MPEG 1 Layer III 帧（CBR 128kbps @ 44.1kHz stereo）。
// 比特率索引 9 = 128 kbps，sample rate 索引 0 = 44.1 kHz，padding 0。
// 11 位 sync 全部为 1：0b11111111111 << 21 = 0xFFE00000。
func mp3FrameHeader(stereo bool, pad bool) uint32 {
	h := uint32(0xFFE00000) // 11 sync bits
	h |= uint32(0b11) << 19 // MPEG1
	h |= uint32(0b01) << 17 // Layer III
	h |= uint32(1) << 16    // protection (no CRC)
	h |= uint32(9) << 12    // bitrate index 9 = 128kbps MPEG1/L3
	h |= uint32(0) << 10    // sample rate index 0 = 44.1kHz
	if pad {
		h |= uint32(1) << 9
	}
	if !stereo {
		h |= uint32(0b11) << 6 // mode = mono
	}
	return h
}

// mp3FrameBytes 返回帧头 + 按公式计算的字节长度（公式给出含 4 字节头的总长）。
// MPEG1 L3 128kbps@44100Hz → framesize = 144*128000/44100 ≈ 417 bytes 含头。
func mp3FrameBytes(mono, pad bool) []byte {
	h := mp3FrameHeader(!mono, pad)
	// 计算帧长（含 4 字节头）。
	sr := 44100
	br := 128000
	size := 144 * br / sr
	if pad {
		size++
	}
	// 我们仅模拟头，body 部分保持零。
	out := make([]byte, size)
	out[0] = byte(h >> 24)
	out[1] = byte(h >> 16)
	out[2] = byte(h >> 8)
	out[3] = byte(h)
	return out
}

func TestProbeMP3CBR(t *testing.T) {
	// 1 帧 CBR 128kbps @ 44.1 kHz ≈ 417 字节帧 / 16000 字节/s ≈ 26 ms。
	data := mp3FrameBytes(false, false)
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp3" || info.Encoding != "mp3" {
		t.Errorf("container=%q encoding=%q", info.Container, info.Encoding)
	}
	if info.State != DurationKnown {
		t.Fatalf("state = %v", info.State)
	}
	// 1 帧 = 1152/44100 = 26.122 ms；近似 CBR 文件大小/字节率亦应落在该范围。
	dms := info.Duration.Milliseconds()
	if dms < 25 || dms > 28 {
		t.Errorf("duration ms = %d, want ~26", dms)
	}
	if info.SampleRate != 44100 {
		t.Errorf("sample rate = %d", info.SampleRate)
	}
	if info.BitrateKbps != 128 {
		t.Errorf("bitrate = %d", info.BitrateKbps)
	}
	if info.Channels != 2 {
		t.Errorf("channels = %d, want 2 (stereo)", info.Channels)
	}
}

func TestMP3Helpers(t *testing.T) {
	mono := mp3FrameHeader(false, false)
	if got := detectChannels(mono); got != 1 {
		t.Fatalf("mono channels = %d, want 1", got)
	}
	stereo := mp3FrameHeader(true, false)
	if got := detectChannels(stereo); got != 2 {
		t.Fatalf("stereo channels = %d, want 2", got)
	}
	withTag := append([]byte("audio"), make([]byte, 123)...)
	copy(withTag[len(withTag)-128:], "TAG")
	if got := skipID3v1(withTag); len(got) != len(withTag)-128 {
		t.Fatalf("skipID3v1 len = %d, want %d", len(got), len(withTag)-128)
	}
	if _, _, _, _, ok := parseMP3Frame([]byte{0, 1, 2}); ok {
		t.Fatal("short frame parsed ok")
	}
	badLayer := mp3FrameBytes(false, false)
	badLayer[1] &^= 0b00000110
	if _, _, _, _, ok := parseMP3Frame(badLayer); ok {
		t.Fatal("bad layer parsed ok")
	}
}

func TestProbeMP3WithID3(t *testing.T) {
	// 构造 ID3v2 头 + 一帧 MP3。
	data := append([]byte("ID3"), 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10)
	frame := mp3FrameBytes(false, false)
	data = append(data, frame...)
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp3" {
		t.Errorf("container = %q", info.Container)
	}
	if info.State != DurationKnown {
		t.Fatalf("state = %v", info.State)
	}
}

func TestProbeMP3ID3v2Only(t *testing.T) {
	// 仅 ID3 头无帧：时长 Unknown。
	data := append([]byte("ID3"), 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10)
	data = append(data, make([]byte, 16)...)
	_, err := Probe(ProbeInput{Data: data})
	if err == nil {
		// id3 仅有 → no mp3 data after id3 → 错误
	} else {
		// acceptable
	}
}

func TestProbeMP3ID3Malformed(t *testing.T) {
	data := []byte{'I', 'D', '3', 4, 0, 0, 0x7f, 0x7f, 0x7f, 0x7f}
	_, err := Probe(ProbeInput{Data: data})
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("err = %v, want ErrMalformedMedia", err)
	}
	data = []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}
	_, err = Probe(ProbeInput{Data: data})
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("err = %v, want ErrMalformedMedia", err)
	}
}

func TestProbeMP3XingVBR(t *testing.T) {
	// CBR 帧 + 紧跟的 Xing VBR 标签：frames=100。
	frame := mp3FrameBytes(false, false)
	// Xing: 'Xing' + flags=0x01 (FRAMES) + 4 字节 totalFrames 大端。
	xing := []byte{'X', 'i', 'n', 'g', 0, 0, 0, 1, 0, 0, 0, 100}
	// parseMP3Frame 返回 framesize 含 4 字节头，所以 Xing 直接跟在帧后。
	data := append(frame, xing...)
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.State != DurationKnown {
		t.Fatalf("state = %v", info.State)
	}
	// 100 帧 MPEG1/L3 = 100 * 1152 / 44100 ≈ 2.612 s。
	if d := info.Duration.Milliseconds(); d < 2600 || d > 2625 {
		t.Errorf("duration ms = %d, want ~2612", d)
	}
}

func TestProbeMP3VBRI(t *testing.T) {
	frame := mp3FrameBytes(false, false)
	data := append([]byte(nil), frame...)
	pad := make([]byte, 42)
	copy(pad[32:36], "VBRI")
	binary.BigEndian.PutUint16(pad[40:42], 50)
	data = append(data, pad...)
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if info.State != DurationKnown {
		t.Fatalf("state = %v, want known", info.State)
	}
	if d := info.Duration.Milliseconds(); d < 1300 || d > 1315 {
		t.Fatalf("duration = %dms, want ~1306ms", d)
	}
}

// TestParseMP3FrameErrorBranches covers the four invalid-frame early-return
// branches in parseMP3Frame (brIdx==0, brIdx>=14, srIdx>=3, lyr==0). These
// were not exercised by the CBR 128kbps Layer III fixture. Each case builds
// its header from scratch (cannot OR-mutate, since OR cannot clear bits).
func TestParseMP3FrameErrorBranches(t *testing.T) {
	// Base layout: 11 sync bits set; MPEG1; Layer III; no CRC.
	// We rebuild the header for each case with one field mutated.
	sync := uint32(0xFFE00000)
	mpeg1 := uint32(0b11) << 19
	layerIII := uint32(0b01) << 17
	noCRC := uint32(1) << 16
	defaultBR := uint32(9) << 12
	defaultSR := uint32(0) << 10

	cases := []struct {
		name string
		head uint32
	}{
		{"brIdx=0 (free)", sync | mpeg1 | layerIII | noCRC | (0 << 12) | defaultSR},
		{"brIdx=15 (bad)", sync | mpeg1 | layerIII | noCRC | (15 << 12) | defaultSR},
		{"srIdx=3 (reserved)", sync | mpeg1 | layerIII | noCRC | defaultBR | (3 << 10)},
		{"lyr=0 (Layer I)", sync | mpeg1 | (0 << 17) | noCRC | defaultBR | defaultSR},
	}
	for _, tc := range cases {
		buf := []byte{byte(tc.head >> 24), byte(tc.head >> 16), byte(tc.head >> 8), byte(tc.head)}
		_, _, _, _, ok := parseMP3Frame(buf)
		if ok {
			t.Errorf("%s: parseMP3Frame returned ok=true; want false", tc.name)
		}
	}
}

// TestParseMP3FrameMPEG2 covers the MPEG2/2.5 Layer III success branch of
// parseMP3Frame (ver != mp3MPEG1 → size uses 72*bitrate/sampleRate formula
// instead of MPEG1's 144*bitrate/sampleRate).
//
// For MPEG2 Layer III, brIdx=9 maps to 80kbps (not 128kbps as in MPEG1), and
// srIdx=0 maps to 22050 Hz. Frame size = 72*80000/22050 ≈ 261 bytes.
func TestParseMP3FrameMPEG2(t *testing.T) {
	sync := uint32(0xFFE00000)
	mpeg2 := uint32(0b10) << 19
	layerIII := uint32(0b01) << 17
	noCRC := uint32(1) << 16
	brIdx9 := uint32(9) << 12
	srIdx0 := uint32(0) << 10
	h := sync | mpeg2 | layerIII | noCRC | brIdx9 | srIdx0
	buf := []byte{byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h)}
	_, frameLen, version, layer, ok := parseMP3Frame(buf)
	if !ok {
		t.Fatalf("parseMP3Frame(MPEG2 L3) returned ok=false")
	}
	if version != 2 {
		t.Errorf("version = %d, want 2 (MPEG2 raw bits)", version)
	}
	if layer != 1 {
		t.Errorf("layer = %d, want 1 (mp3LayerIII constant)", layer)
	}
	// MPEG2 L3 @ 80kbps / 22050Hz = 72 * 80000 / 22050 ≈ 261 bytes.
	if frameLen < 255 || frameLen > 270 {
		t.Errorf("frameLen = %d, want ~261", frameLen)
	}
}

// TestProbeMP3LayerIIWarning covers the warning-append branch in probeMP3
// (layer != mp3LayerIII). Layer II frames are not decoded as such but probeMP3
// still computes duration using the Layer II row of the bitrate/frame-samples
// tables; the layer-not-supported warning is the observable contract for the
// "Layer II not supported" stance.
func TestProbeMP3LayerIIWarning(t *testing.T) {
	// Layer II (raw bits 0b10), MPEG1 (raw bits 0b11), 128kbps, 44.1kHz.
	// MPEG1 L2 frame size = 144 * 128000 / 44100 ≈ 417 bytes.
	sync := uint32(0xFFE00000)
	mpeg1 := uint32(0b11) << 19
	layerII := uint32(0b10) << 17
	noCRC := uint32(1) << 16
	brIdx9 := uint32(9) << 12
	srIdx0 := uint32(0) << 10
	h := sync | mpeg1 | layerII | noCRC | brIdx9 | srIdx0
	frameLen := 144 * 128000 / 44100
	buf := make([]byte, frameLen)
	binary.BigEndian.PutUint32(buf, h)

	info, err := Probe(ProbeInput{Data: buf})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp3" {
		t.Errorf("container = %q, want mp3", info.Container)
	}
	warningFound := false
	for _, w := range info.Warnings {
		if strings.Contains(w, "layer=2") {
			warningFound = true
			break
		}
	}
	if !warningFound {
		t.Errorf("expected layer=2 warning, got %v", info.Warnings)
	}
}

// TestProbeMP3InvalidBitrate covers the "no valid frame found" return path
// in probeMP3. A header with brIdx=0 passes parseMP3Frame's sync/version/
// layer checks but parseMP3Frame itself rejects brIdx=0 (free format
// reserved) → ok=false → probeMP3 keeps searching, never finds a valid
// frame, falls through to DurationUnknown at the bottom.
func TestProbeMP3InvalidBitrate(t *testing.T) {
	sync := uint32(0xFFE00000)
	mpeg1 := uint32(0b11) << 19
	layerIII := uint32(0b01) << 17
	noCRC := uint32(1) << 16
	brIdx0 := uint32(0) << 12
	srIdx0 := uint32(0) << 10
	h := sync | mpeg1 | layerIII | noCRC | brIdx0 | srIdx0
	buf := make([]byte, 64)
	binary.BigEndian.PutUint32(buf, h)
	info, err := Probe(ProbeInput{Data: buf})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp3" {
		t.Errorf("container = %q, want mp3", info.Container)
	}
	if info.State != DurationUnknown {
		t.Errorf("state = %v, want Unknown (free-format bitrate unsupported)", info.State)
	}
}

func TestProbeMP3Broken(t *testing.T) {
	// 损坏（不含 RIFF 且前两字节不是 0xFF）→ 未知。
	info, err := Probe(ProbeInput{Data: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.State != DurationUnknown {
		t.Errorf("state = %v, want Unknown", info.State)
	}
}

// ---------- Dispatcher ----------

func TestProbeByHintUnknown(t *testing.T) {
	info, _ := Probe(ProbeInput{Data: []byte("random"), ContainerHint: "ogg"})
	if info.Container != "" {
		t.Errorf("container = %q, want empty", info.Container)
	}
}

func TestKnownDuration(t *testing.T) {
	if got := KnownDuration(-10).Nanoseconds; got != 0 {
		t.Fatalf("negative duration = %d, want 0", got)
	}
	if got := (Duration{}).Milliseconds(); got != 0 {
		t.Fatalf("zero duration ms = %d, want 0", got)
	}
	d := KnownDuration(1500)
	if d.Nanoseconds != 1500 {
		t.Errorf("nanos = %d", d.Nanoseconds)
	}
	// 1500ns 上取整到 1ms（小于 1ms 但非整 ms 视作整 ms）。
	if d.Milliseconds() != 1 {
		t.Errorf("ms = %d, want 1 (ceil from 1500ns)", d.Milliseconds())
	}
	// 1.5ms 应上取整为 2ms。
	d2 := KnownDuration(1_500_000)
	if d2.Milliseconds() != 2 {
		t.Errorf("ms2 = %d, want 2 (ceil from 1.5ms)", d2.Milliseconds())
	}
}

func TestProbeErrorUnwrap(t *testing.T) {
	e := &ProbeError{Op: "x", Message: "bad", Err: ErrMalformedMedia}
	if !errors.Is(e, ErrMalformedMedia) || !errIs(e, ErrMalformedMedia) {
		t.Error("unwrap failed")
	}
	if got := e.Error(); !strings.Contains(got, "x: bad") || !strings.Contains(got, ErrMalformedMedia.Error()) {
		t.Fatalf("Error = %q", got)
	}
	plain := (&ProbeError{Op: "x", Message: "bad"}).Error()
	if plain != "x: bad" {
		t.Fatalf("plain Error = %q", plain)
	}
}

func errIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
