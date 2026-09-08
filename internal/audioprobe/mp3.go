package audioprobe

import (
	"encoding/binary"
	"fmt"
)

// ---------- MP3 / MPEG 1 Layer III ----------

// MPEG 帧头（4 字节）：
//
//	AAAAAAAA AAABBCCD EEEEFFGH IIJJKLMM
//
//	A = 同步（11 位，全部为 1）
//	B = MPEG 版本（2 位）：11=MPEG1, 10=MPEG2, 00=MPEG2.5, 01=保留
//	C = 层（2 位）：01=Layer III
//	D = 保护位（1 位）：0=有 CRC, 1=无 CRC
//	E = 比特率索引（4 位）
//	F = 采样率索引（2 位）
//	G = 填充位（1 位）
//	H = 私有位（1 位）
//	I = 声道模式（2 位）
//	J = mode extension（2 位）
//	K = 版权（1 位）
//	L = 原版（1 位）
//	M = emphasis（2 位）

const (
	mp3MPEG1    = 3
	mp3MPEG2    = 2
	mp3MPEG25   = 0
	mp3LayerIII = 1
	mp3LayerII  = 2
	mp3LayerI   = 3
)

// mp3Bitrates 是 [version][layer][bitrateIndex] = bps（缺席索引=0）。
// Layer III, MPEG1 / MPEG2(MPEG2.5 用 MPEG2 表)。
//
// 参考 ISO/IEC 11172-3 与 13818-3 自由格式表。
var mp3Bitrates = [][][]int{
	// version 1
	{
		// layer 1 (unused for us)
		{},
		// layer 2
		{},
		// layer 3
		{0, 32000, 40000, 48000, 56000, 64000, 80000, 96000, 112000, 128000, 160000, 192000, 224000, 256000, 320000, 0},
	},
	// version 2 / 2.5
	{
		{},
		{},
		{0, 8000, 16000, 24000, 32000, 40000, 48000, 56000, 64000, 80000, 96000, 112000, 128000, 144000, 160000, 0},
	},
}

// mp3SampleRates 是 [version][srIndex] = Hz。
var mp3SampleRates = [][]int{
	// MPEG1
	{44100, 48000, 32000, 0},
	// MPEG2 / MPEG2.5
	{22050, 24000, 16000, 0},
}

// mp3FrameSamples 是 [version][layer] = 一个帧的样本数。
// 层枚举按 MPEG 头 2 位（01=L3, 10=L2, 11=L1, 00=invalid）。
var mp3FrameSamples = [][]int{
	// MPEG1
	{0, 1152, 1152, 384}, // L3, L2, L1（位于 lyr 索引：1=L3, 2=L2, 3=L1）
	// MPEG2 / 2.5
	{0, 576, 1152, 384},
}

// probeMP3 解析 MPEG 1/2/2.5 Layer III：跳过 ID3v2 / ID3v1，找到首帧，
// 解析头与可选 Xing/Info/VBRI VBR 头决定总帧数与时长。
//
// 不解析 Layer I/II；签名不匹配返回空 MediaInfo。
func probeMP3(in ProbeInput) (MediaInfo, error) {
	data := in.Data
	if len(data) < 4 {
		return MediaInfo{}, nil
	}
	info := MediaInfo{Container: "mp3", Encoding: "mp3", ByteSize: int64(len(data)), Origin: OriginByteSignature}
	// ID3v2 头："ID3" + 版本 + 标志 + 标签大小（4 字节 syncsafe）
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		tagSize := int(data[6])<<21 | int(data[7])<<14 | int(data[8])<<7 | int(data[9])
		tagSize += 10 // 含头
		if tagSize < 0 || tagSize > len(data) {
			return info, &ProbeError{
				Op: "mp3 probe", Message: "id3 tag size invalid", Err: ErrMalformedMedia,
			}
		}
		data = data[tagSize:]
	}
	if len(data) < 4 {
		return info, &ProbeError{
			Op: "mp3 probe", Message: "no mp3 data after id3", Err: ErrMalformedMedia,
		}
	}
	// 找到首帧（跳过任何前导零）。
	i := 0
	for i < len(data)-4 && data[i] != 0xFF {
		i++
	}
	for i < len(data)-4 {
		frame, frameLen, version, layer, ok := parseMP3Frame(data[i:])
		if !ok {
			// 不是帧头；继续尝试下一字节（罕见，但能自愈）。
			if i+1 >= len(data)-4 {
				break
			}
			i++
			continue
		}
		// 成功定位到首帧，记录上下文。
		if layer != mp3LayerIII {
			info.Warnings = append(info.Warnings, fmt.Sprintf("mp3 layer=%d not supported", layer))
		}
		_ = frame
		// 已获取头，下一步：尝试 Xing/Info VBR 标签。
		totalFrames := 0
		isVBR := false
		end := i + frameLen
		// Xing 标签通常紧跟帧头（CRC 后或帧体后）；偏移取决于 layer 与保护位。
		if end+12 <= len(data) {
			if hdr := string(data[end : end+4]); hdr == "Xing" || hdr == "Info" {
				// flags: 大小 + flags + 长度；flags 字段后读取 frames 字段。
				flags := binary.BigEndian.Uint32(data[end+4 : end+8])
				off := end + 8
				if flags&0x01 != 0 { // FRAMES_FLAG
					if off+4 > len(data) {
						break
					}
					totalFrames = int(binary.BigEndian.Uint32(data[off : off+4]))
					off += 4
				}
				isVBR = true
			}
		}
		// VBRI 标签（少见）：位于帧头后 32 字节偏移。
		if !isVBR && end+32+4 <= len(data) {
			if hdr := string(data[end+32 : end+32+4]); hdr == "VBRI" {
				tf := binary.BigEndian.Uint16(data[end+32+8 : end+32+10])
				totalFrames = int(tf)
				isVBR = true
			}
		}
		// 采样率与每帧样本数取决于 version+layer。
		srIdx := (frame >> 10) & 0x03
		verIdx := 1
		if version == mp3MPEG1 {
			verIdx = 0
		}
		if srIdx >= 3 {
			info.State = DurationUnknown
			return info, nil
		}
		sr := mp3SampleRates[verIdx][srIdx]
		if sr == 0 {
			info.State = DurationUnknown
			return info, nil
		}
		info.SampleRate = sr
		info.Channels = detectChannels(frame)
		brIdx := (frame >> 12) & 0x0F
		if brIdx == 0 || brIdx >= 14 {
			info.State = DurationUnknown
			return info, nil
		}
		br := mp3Bitrates[verIdx][2][brIdx]
		if br == 0 {
			info.State = DurationUnknown
			return info, nil
		}
		info.BitrateKbps = br / 1000
		perFrame := mp3FrameSamples[verIdx][layer]
		if perFrame == 0 {
			info.State = DurationUnknown
			return info, nil
		}
		switch {
		case isVBR && totalFrames > 0:
			// FrameScanned-derived: VBR 标签给的帧数。
			durNs := int64(totalFrames) * int64(perFrame) * int64(1_000_000_000) / int64(sr)
			info.Duration = KnownDuration(durNs)
			info.State = DurationKnown
		default:
			// CBR/无 VBR 标签：按文件字节数 / 比特率估算。
			brBytes := int64(br) / 8
			if brBytes <= 0 {
				info.State = DurationUnknown
				return info, nil
			}
			durNs := int64(len(data)-i) * int64(1_000_000_000) / brBytes
			info.Duration = KnownDuration(durNs)
			info.State = DurationKnown
		}
		// ID3v1 尾标（128 字节，以 "TAG" 开头）：仅忽略，不影响时长。
		_ = skipID3v1(data)
		return info, nil
	}
	info.State = DurationUnknown
	return info, nil
}

// parseMP3Frame 返回头值、帧字节长（含 4 字节头）、version、layer、成功。
func parseMP3Frame(b []byte) (head uint32, frameLen int, version, layer int, ok bool) {
	if len(b) < 4 {
		return 0, 0, 0, 0, false
	}
	h := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	// 11 位同步必须全 1。
	if h>>21 != 0x7FF {
		return 0, 0, 0, 0, false
	}
	ver := (h >> 19) & 0x03
	lyr := (h >> 17) & 0x03
	if lyr == 0 {
		return 0, 0, 0, 0, false
	}
	brIdx := (h >> 12) & 0x0F
	if brIdx == 0 || brIdx >= 14 {
		return 0, 0, 0, 0, false
	}
	srIdx := (h >> 10) & 0x03
	if srIdx >= 3 {
		return 0, 0, 0, 0, false
	}
	pad := (h >> 9) & 0x01
	verIdx := 1
	if ver == mp3MPEG1 {
		verIdx = 0
	}
	br := mp3Bitrates[verIdx][2][brIdx]
	if br == 0 {
		return 0, 0, 0, 0, false
	}
	samplesPerFrame := mp3FrameSamples[verIdx][lyr]
	if samplesPerFrame == 0 {
		return 0, 0, 0, 0, false
	}
	_ = samplesPerFrame
	// frame size (字节) for L3:
	//   MPEG1:    144 * bitrate / sampleRate + padding
	//   MPEG2/2.5: 72  * bitrate / sampleRate + padding
	// 含 4 字节头与帧体（验证：MPEG1/L3/128000 bps/44100 Hz = 417 bytes）。
	sr := mp3SampleRates[verIdx][srIdx]
	if sr == 0 {
		return 0, 0, 0, 0, false
	}
	size := 0
	if ver == mp3MPEG1 {
		size = 144 * int(br) / sr
	} else {
		size = 72 * int(br) / sr
	}
	if pad != 0 {
		size++
	}
	if size < 4 {
		return 0, 0, 0, 0, false
	}
	return h, size, int(ver), int(lyr), true
}

// detectChannels 由声道模式位（bit 6-7）决定。
func detectChannels(head uint32) int {
	mode := (head >> 6) & 0x03
	// 00=stereo, 01=joint stereo, 10=dual channel, 11=mono
	// 保守返回 2 for stereo/联合/双声道，1 for mono。
	if mode == 3 {
		return 1
	}
	return 2
}

// skipID3v1 检测末尾 128 字节 "TAG" 头；未命中返回原切片。
func skipID3v1(b []byte) []byte {
	if len(b) >= 128 && string(b[len(b)-128:len(b)-125]) == "TAG" {
		return b[:len(b)-128]
	}
	return b
}
