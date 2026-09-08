package audioprobe

import "strings"

// Probe 利用按签名/扩展名的内置探测器解析音频字节。
//
// 默认实现：WAV（RIFF）+ MP3（MPEG 1/2/2.5 Layer III，含 Xing/Info
// VBR 与 VBRI VBR 头）。未知容器返回 MediaInfo{} 与 nil 错误。
//
// 容器选择规则：
//
//  1. 若 ContainerHint 命中已知容器名，使用对应探测器（可减少误识别）。
//  2. 否则按字节签名：RIFF → WAVE；sync 0xFFEx → MP3。
//  3. 都不匹配返回空 MediaInfo（调用方应改用自身 MediaProbe 或报错）。
func Probe(in ProbeInput) (MediaInfo, error) {
	if hint := strings.ToLower(strings.TrimSpace(in.ContainerHint)); hint != "" {
		switch hint {
		case "wav", "audio/wav", "audio/x-wav", "audio/wave":
			return probeWAV(in)
		case "mp3", "audio/mpeg", "audio/mp3":
			return probeMP3(in)
		}
	}
	// 签名探测：仅前 4 字节足以区分 WAV vs MP3 与 ID3。
	if len(in.Data) >= 12 && string(in.Data[:4]) == "RIFF" && string(in.Data[8:12]) == "WAVE" {
		return probeWAV(in)
	}
	if len(in.Data) >= 3 && string(in.Data[:3]) == "ID3" {
		return probeMP3(in)
	}
	if len(in.Data) >= 2 && in.Data[0] == 0xFF && (in.Data[1]&0xE0) == 0xE0 {
		// MP3 sync: 11 位全 1。
		return probeMP3(in)
	}
	return MediaInfo{}, nil
}
