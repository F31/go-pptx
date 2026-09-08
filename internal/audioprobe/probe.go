// Package audioprobe 提供音频容器探测能力（方案 §21.2）。
//
// 当前内置探测支持：
//
//   - RIFF/WAVE（PCM）：遍历 top-level chunk 链表，支持填充字节；
//     从 a:fmt 块读取采样率、声道、位深、字节率；时长按
//     ceil(data_bytes / byteRate) 在有 a:data 块时返回 Known；
//     a:data 缺失回退 DurationUnknown。
//   - MPEG 1 Layer III（MP3）：处理 ID3v2 头（含 unsynchronisation + 扩展头）
//     与 ID3v1 尾标；解析 MPEG 帧头，支持常量比特率（CBR）、Xing/Info
//     VBR 头与 LAME/VBRI VBR 头；时长先后退到 FrameScanned 与
//     HeaderDerived，未知或损坏返回 DurationUnknown。
//
// 探测结果区分时长来源（CallerProvided/HeaderDerived/FrameScanned）与
// 容器来源（ByteSignature/CallerDeclared/Unknown），上层调用方可据此
// 选择是否接受 ErrDurationUnknown。
//
// 核心不为探测自动调用 FFmpeg；扩展其他容器（FLAC/AAC/OGG 等）由用户
// 实现 MediaProbe 接口注入。
package audioprobe

// MediaInfo 是探测结果（方案 §21.2）。
//
// Container/Encoding 仅描述结构；Duration 是核心字段，State 区分
// "已知"与"未知"——后者若上游需要必须明确返回 ErrDurationUnknown。
type MediaInfo struct {
	// Container 是容器标识（"wav"/"mp3" 等）；空表示未识别。
	Container string
	// Encoding 是编码格式（如 "pcm"/"mp3"）。
	Encoding string
	// SampleRate 是每秒采样数；缺失或为 0 表示未知。
	SampleRate int
	// Channels 是声道数；缺失或为 0 表示未知。
	Channels int
	// BitsPerSample 是每样本位深（WAV PCM）或 0（MP3）。
	BitsPerSample int
	// BitrateKbps 是平均比特率（kbps，含 1000 bps 整数）；缺失为 0。
	BitrateKbps int
	// ByteSize 是解码后字节数（媒体 Part 大小），整字节；0 为未知。
	ByteSize int64
	// Duration 是解析得到的时长；为零时含义见 State。
	Duration Duration
	// State 是时长确定性分类。
	State DurationState
	// Origin 标识 Container 的判定来源（签名 vs 调用方声明）。
	Origin Origin
	// Warnings 是非阻断诊断（如可疑字段、未知版本）；调用方可忽略。
	Warnings []string
}

// MediaProbe 是探测接口（方案 §21.2）。
type MediaProbe interface {
	// Probe 读取源前若干字节并返回 MediaInfo。错误仅在容器结构明确
	// 不一致或读取失败时返回；未知容器应返回 nil + MediaInfo{}。
	Probe(in ProbeInput) (MediaInfo, error)
}

// ProbeInput 为探测输入（媒体源 + 调用方声明）。
type ProbeInput struct {
	// Data 是已读到内存的媒体字节（探测器独立消费；完成后仍属调用方）。
	Data []byte
	// Declared 是调用方声明的 MIME / 文件扩展，可空。
	Declared string
	// ContainerHint 是上层基于文件扩展名或前次判定给出的容器提示，
	// 可空；探测器优先按签名判定。
	ContainerHint string
}

// Duration 是时长包装，区分"零值=已知"与"未知"。
type Duration struct {
	// Nanoseconds 是纳秒数（≥0）。State 为 Known 时有效。
	Nanoseconds int64
}

// DurationState 是时长确定性状态。
type DurationState int

const (
	// DurationUnknown 表示无法确定时长（损坏、容器头缺失、码率不可解）。
	DurationUnknown DurationState = iota
	// DurationKnown 表示时长已解析（Nanoseconds 有效）。
	DurationKnown
)

// KnownDuration 返回 State=Known 的 Duration。
func KnownDuration(nanos int64) Duration {
	if nanos < 0 {
		nanos = 0
	}
	return Duration{Nanoseconds: nanos}
}

// Milliseconds 返回时长毫秒（整毫秒，向上取整避免提前翻页）。
func (d Duration) Milliseconds() int64 {
	if d.Nanoseconds <= 0 {
		return 0
	}
	ns := d.Nanoseconds
	if rem := ns % int64(1_000_000); rem != 0 {
		ns += int64(1_000_000) - rem
	}
	return ns / int64(1_000_000)
}

// Origin 标识 Container/Encoding 的判定来源。
type Origin int

const (
	// OriginUnknown 表示容器未识别（MediaInfo.Container 为空）。
	OriginUnknown Origin = iota
	// OriginByteSignature 表示容器由字节签名（魔数/结构）确认。
	OriginByteSignature
	// OriginCallerDeclared 表示容器仅由调用方声明支持，未做签名确认。
	OriginCallerDeclared
)
