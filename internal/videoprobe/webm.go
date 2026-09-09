package videoprobe

// WebM/Matroska 文件以 EBML 头开始，前 4 字节为 0x1A 0x45 0xDF 0xA3。
// 后续 VINT 编码的版本、读/写权限、DocType、DocTypeVersion、DocTypeReadVersion
// 等段（视频包 V_INT_WIDTH）。VIDEO-01 E 档仅做签名确认 + Container 已知 +
// 时长未知；完整 VINT/Segment 解析属后续切分项。
//
// 参考：Matroska / WebM Container Guidelines（Xiph.Org）。

// probeWebM 探测 WebM/Matroska。返回 ok=true 表示签名匹配。
func probeWebM(in ProbeInput) (MediaInfo, bool, error) {
	d := in.Data
	if len(d) < 4 {
		return MediaInfo{}, false, nil
	}
	// EBML 头签名 0x1A45DFA3（前 4 字节）。
	if d[0] != 0x1A || d[1] != 0x45 || d[2] != 0xDF || d[3] != 0xA3 {
		return MediaInfo{}, false, nil
	}
	// 简化把 EBML 容器视作 webm（Matroska 子集）；VIDEO-01 不区分 webm/mkv。
	info := MediaInfo{
		Container: "webm",
		Encoding:  "",
		Brands:    nil,
		ByteSize:  int64(len(d)),
		Duration:  Duration{},
		State:     DurationUnknown,
		Origin:    OriginByteSignature,
	}
	// 检查 DocType（"webm"/"matroska"）是否出现；仅在签名匹配后做软验证。
	if docType := extractEBMLDocType(d); docType != "" {
		info.Brands = []string{docType}
	}
	return info, true, nil
}

// extractEBMLDocType 在前若干字节中查找 "webm" 或 "matroska" 字串（DocType
// 元素按 VINT 长度前缀 + ASCII；最多扫描前 256 字节）。
func extractEBMLDocType(d []byte) string {
	end := len(d)
	if end > 256 {
		end = 256
	}
	for i := 0; i+4 <= end; i++ {
		if bytesEq(d[i:i+4], "webm") {
			return "webm"
		}
	}
	for i := 0; i+8 <= end; i++ {
		if bytesEq(d[i:i+8], "matroska") {
			return "matroska"
		}
	}
	return ""
}
