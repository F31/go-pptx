// Package videoprobe 提供视频容器探测能力（VIDEO-01，方案 §21.2）。
//
// 当前内置探测支持：
//
//   - ISO BMFF（MP4/MOV 等）：通过文件首部 box size+type 字段识别 ftyp，
//     解析 major_brand 与 compatible_brands（白名单）。Video-01 不解析
//     完整 box 树推导时长（依赖 FFmpeg/MediaInfo 体系，§2.4 不作为核心
//     强依赖）；时长统一返回 DurationUnknown，由调用方按需提供或拒绝。
//   - WebM/Matroska：通过 EBML 头 0x1A45DFA3 签名识别；版本段与
//     Duration 元素涉及 VINT 解析，简化为"仅签名存在 + Container 已知 +
//     时长未知"返回。
//
// 探测结果区分时长状态（Known/Unknown）与容器来源（ByteSignature/
// CallerDeclared/Unknown）。VIDEO-01 E 档目标以"容器已识别 + 字节去重 +
// 关系分配 + p:videoFile 片段生成"为完成标准，时长推导属后续切分项。
//
// 核心不为探测自动调用 FFmpeg；扩展其他容器（MKV、3GP、AVI）由用户实现
// MediaProbe 接口注入。
package videoprobe

// MediaInfo 是探测结果（方案 §21.2）。
//
// Container/Encoding 仅描述结构；VIDEO-01 阶段 Duration 通常为
// DurationUnknown（推导留给后续切分），调用方可基于 State 决定是否
// 拒绝整个嵌入。
type MediaInfo struct {
	// Container 是容器标识（"mp4"/"webm" 等）；空表示未识别。
	Container string
	// Encoding 是编码格式（如 "h264"/"aac"/"vp9"）；空表示未识别。
	Encoding string
	// Brands 是容器兼容品牌列表（MP4 ftyp 的 major+compatible）。
	Brands []string
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
	// DurationUnknown 表示无法确定时长（VIDEO-01 阶段恒为 Unknown）。
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

// ProbeError 描述探测阶段检测到的容器不一致或结构损坏错误。
type ProbeError struct {
	Err        error  // 分类错误（ErrMalformedMedia 等）
	Op         string // 探测器名称
	ProbeInput string // 调用方声明的容器/类型
}

// Error 实现 error 接口。
func (e *ProbeError) Error() string {
	if e.Op == "" {
		return "videoprobe: " + e.ProbeInput
	}
	return "videoprobe: " + e.Op + ": " + e.ProbeInput
}

// Unwrap 返回分类错误。
func (e *ProbeError) Unwrap() error { return e.Err }

// MalformedError 构造容器结构损坏错误（探测器签名匹配但 box/树解析失败）。
func MalformedError(op, msg string) *ProbeError {
	return &ProbeError{Op: op, ProbeInput: msg, Err: ErrMalformedMedia}
}

// ErrMalformedMedia 标识签名匹配但容器结构无法解析（与 audioprobe 一致）。
var ErrMalformedMedia = sentinelError("video media: malformed")

// sentinelError 是包内哨兵错误，避免向调用方暴露 fmt.Errorf 的实现细节。
type sentinelError string

func (e sentinelError) Error() string { return string(e) }

// Probe 是分派器：按签名匹配对应探测器，全部失败返回 OriginUnknown + nil。
//
// 优先级：MP4 → WebM。调用方声明可作为补充（ContainerHint 或 Declared）。
func Probe(in ProbeInput) (MediaInfo, error) {
	if len(in.Data) == 0 {
		return MediaInfo{Origin: OriginUnknown, State: DurationUnknown}, nil
	}
	if info, ok, err := probeMP4(in); ok || err != nil {
		return info, err
	}
	if info, ok, err := probeWebM(in); ok || err != nil {
		return info, err
	}
	return MediaInfo{
		Origin: OriginUnknown,
		State:  DurationUnknown,
	}, nil
}
