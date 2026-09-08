package audioprobe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ---------- WAV/RIFF ----------

// wavFmt 是 fmt chunk 的核心字段（wFormatTag 起）：
//
//	WORD  wFormatTag;       // 0x0001=PCM, 0x0003=IEEE float, 0xFFFE=extensible 等
//	WORD  nChannels;
//	DWORD nSamplesPerSec;
//	DWORD nAvgBytesPerSec;
//	WORD  nBlockAlign;
//	WORD  wBitsPerSample;  // 可选字段后还有 cbSize/exTra
type wavFmt struct {
	tag     uint16
	chanNum uint16
	rate    uint32
	rateBps uint32
	block   uint16
	bits    uint16
	extra   uint16 // cbSize
}

// probeWAV 解析 RIFF/WAVE：遍历 top-level chunks，对 fmt/data 按规范解析；
// 不解析嵌入 RIFX（big-endian） 文件，签名不匹配返回空 MediaInfo。
//
// 不调用 FFmpeg；data chunk 缺失或速率==0 → 时长 DurationUnknown。
func probeWAV(in ProbeInput) (MediaInfo, error) {
	if len(in.Data) < 12 {
		return MediaInfo{}, nil
	}
	if string(in.Data[:4]) != "RIFF" {
		return MediaInfo{}, nil
	}
	// big-endian WAV 使用 RIFX magic 我们暂不识别。
	if len(in.Data) < 12 || string(in.Data[8:12]) != "WAVE" {
		return MediaInfo{}, nil
	}
	info := MediaInfo{
		Container: "wav",
		Encoding:  "pcm",
		ByteSize:  int64(len(in.Data)),
		Origin:    OriginByteSignature,
	}
	pos := 12
	var foundFmt bool
	var dataBytes uint32
	var totalDataBytes uint64 // 仅 ds64 使用
	var hasDS64 bool
	for pos+8 <= len(in.Data) {
		id := string(in.Data[pos : pos+4])
		size := binary.LittleEndian.Uint32(in.Data[pos+4 : pos+8])
		pos += 8
		bodyEnd := int64(pos) + int64(size)
		if bodyEnd > int64(len(in.Data)) {
			// chunk 截断：返回已收集的诊断，停止遍历。
			info.Warnings = append(info.Warnings, fmt.Sprintf("wav chunk %q truncated", id))
			break
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return info, &ProbeError{
					Op: "wav probe", Message: "fmt chunk too short", Err: ErrMalformedMedia,
				}
			}
			f := wavFmt{
				tag:     binary.LittleEndian.Uint16(in.Data[pos : pos+2]),
				chanNum: binary.LittleEndian.Uint16(in.Data[pos+2 : pos+4]),
				rate:    binary.LittleEndian.Uint32(in.Data[pos+4 : pos+8]),
				rateBps: binary.LittleEndian.Uint32(in.Data[pos+8 : pos+12]),
				block:   binary.LittleEndian.Uint16(in.Data[pos+12 : pos+14]),
				bits:    binary.LittleEndian.Uint16(in.Data[pos+14 : pos+16]),
			}
			if size >= 18 {
				f.extra = binary.LittleEndian.Uint16(in.Data[pos+16 : pos+18])
			}
			info.SampleRate = int(f.rate)
			info.Channels = int(f.chanNum)
			info.BitsPerSample = int(f.bits)
			if f.rateBps > 0 {
				info.BitrateKbps = int((f.rateBps*8 + 500) / 1000)
			}
			switch f.tag {
			case 0x0001:
				info.Encoding = "pcm"
			case 0x0003:
				info.Encoding = "ieee_float"
			case 0xFFFE:
				info.Encoding = "extensible"
			default:
				info.Encoding = fmt.Sprintf("format=0x%04X", f.tag)
			}
			foundFmt = true
		case "data":
			dataBytes = size
		case "ds64":
			// RF64: 32-bit 表头，先低位 + 高位；为保持 MVP 简单，仅识别存在。
			if size < 28 {
				return info, &ProbeError{
					Op: "wav probe", Message: "ds64 chunk too short", Err: ErrMalformedMedia,
				}
			}
			totalDataBytes = binary.LittleEndian.Uint64(in.Data[pos+8 : pos+16])
			hasDS64 = true
		case "LIST", "INFO", "JUNK", "bext", "iXML", "fact":
			// 跳过；不进入主路径实现。
		default:
			// 未知 chunk：保留为 warning，不阻断。
			info.Warnings = append(info.Warnings, fmt.Sprintf("unknown wav chunk %q", id))
		}
		// WAV chunk 按偶对齐（除 RIFF 头自身外）。
		next := bodyEnd
		if next < int64(pos) {
			return info, &ProbeError{
				Op: "wav probe", Message: "wav chunk size underflow", Err: ErrMalformedMedia,
			}
		}
		if size%2 == 1 && next < int64(len(in.Data)) {
			next++
		}
		pos = int(next)
		if pos >= len(in.Data) {
			break
		}
	}
	if !foundFmt {
		return info, &ProbeError{
			Op: "wav probe", Message: "no fmt chunk", Err: ErrMalformedMedia,
		}
	}
	if hasDS64 {
		// RF64：data chunk 自身 size 字段为 -1 真实长度在 ds64。
		info.ByteSize = int64(totalDataBytes) + int64(len(in.Data)) // 近似
		dataBytes = uint32(totalDataBytes & 0xFFFFFFFF)
	}
	// 计算时长：rateBps=0 无法计算 dataBytes / rateBps。
	if info.SampleRate == 0 || info.BitsPerSample == 0 || info.Channels == 0 {
		info.State = DurationUnknown
		return info, nil
	}
	if dataBytes == 0 {
		// 缺少 a:data chunk 或长度为 0：时长不可确定。
		info.State = DurationUnknown
		return info, nil
	}
	bytesPerSec := int64(info.SampleRate) * int64(info.Channels) * int64(info.BitsPerSample) / 8
	if bytesPerSec <= 0 {
		info.State = DurationUnknown
		return info, nil
	}
	durNs := int64(dataBytes) * int64(1_000_000_000) / bytesPerSec
	info.Duration = KnownDuration(durNs)
	info.State = DurationKnown
	return info, nil
}

// ProbeError 是探测层的统一错误包装。
type ProbeError struct {
	Op      string
	Message string
	Err     error
}

func (e *ProbeError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Op, e.Message, e.Err)
}

// Unwrap 支持 errors.Is(err, ErrMalformedMedia) 等。
func (e *ProbeError) Unwrap() error { return e.Err }

// ErrMalformedMedia 表示媒体字节结构非法（容器头不匹配、长度不足、
// 必备 chunk 缺失）。
var ErrMalformedMedia = errors.New("audioprobe: malformed media")

// Compile-time guard：保证 io 包依赖项仍在链接图中。
var _ = io.EOF
