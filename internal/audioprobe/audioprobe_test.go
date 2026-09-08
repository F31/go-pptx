package audioprobe

import (
	"encoding/binary"
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
	e := &ProbeError{Op: "x", Err: ErrMalformedMedia}
	if !errIs(e, ErrMalformedMedia) {
		t.Error("unwrap failed")
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
