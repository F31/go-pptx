package videoprobe

// ISO BMFF (ISO/IEC 14496-12) 文件由一系列 box 构成：每个 box 以 4 字节 size
// + 4 字节 type（ASCII）开头。ftyp box 是文件第一个 box，含 major_brand +
// minor_version + compatible_brands。
//
// VIDEO-01 E 档实现：
//   - 通过 size+type 校验 ftyp 签名；
//   - 提取 major_brand 与 compatible_brands 做白名单确认；
//   - 不解析完整 box 树推导时长（依赖 moov/mvex/trak 等，V2.6 不实现）。
// 已知兼容品牌：isom/mp41/mp42/dash/avc1/iso2/iso5/iso6/M4V / M4A / qt 等。

// mp4KnownBrands 是容器白名单（与 PowerPoint 接受的常见品牌对齐）。
// 未在此名单的品牌视为不兼容但仍可作 raw mp4（不强制）。
var mp4KnownBrands = map[string]bool{
	"isom": true, "iso2": true, "iso5": true, "iso6": true,
	"mp41": true, "mp42": true, "mp4v": true, "mp4h": true,
	"dash": true, "avc1": true, "hev1": true,
	"qt": true, "qtim": true,
	"M4V ": true, "M4A ": true, "M4VH": true, "M4VP": true,
}

// probeMP4 探测 ISO BMFF。返回 ok=true 表示匹配；err 非 nil 表示签名匹配但
// 结构损坏；否则为"非 MP4 容器"。
func probeMP4(in ProbeInput) (MediaInfo, bool, error) {
	d := in.Data
	if len(d) < 8 {
		return MediaInfo{}, false, nil
	}
	// box size（4 字节大端）+ type（4 字节 ASCII）。size 允许 =1（extended
	// 8 字节 size）或 =0（box 延伸至文件末，本实现不处理）。
	boxSize := uint32(d[0])<<24 | uint32(d[1])<<16 | uint32(d[2])<<8 | uint32(d[3])
	boxType := string(d[4:8])
	if boxSize == 1 || boxSize == 0 {
		// 64 位 size 模式：跳过 8 字节后 4 字节 type。
		if len(d) < 12 {
			return MediaInfo{}, false, nil
		}
		if !bytesEq(d[8:12], "ftyp") {
			return MediaInfo{}, false, nil
		}
		boxType = string(d[8:12])
	} else if boxType != "ftyp" {
		return MediaInfo{}, false, nil
	}
	_ = boxSize
	_ = boxType
	// 签名匹配；尝试解析 brand 列表（需要至少 16 字节：size+type+major+minor）。
	if len(d) < 16 {
		return mp4Info(in, nil, false), true, nil
	}
	end := int(boxSize)
	if end > len(d) {
		end = len(d)
	}
	if end < 16 {
		return MediaInfo{}, true, MalformedError("mp4", "ftyp box too short")
	}
	major := string(d[8:12])
	brands := []string{major}
	for i := 16; i+4 <= end; i += 4 {
		brands = append(brands, string(d[i:i+4]))
	}
	known := mp4KnownBrands[major]
	for _, b := range brands[1:] {
		if mp4KnownBrands[b] {
			known = true
			break
		}
	}
	return mp4Info(in, brands, known), true, nil
}

func mp4Info(in ProbeInput, brands []string, known bool) MediaInfo {
	origin := OriginByteSignature
	if !known && (in.ContainerHint != "" || in.Declared != "") {
		// 签名已确认是 MP4；白名单外的 brand 仍视作"签名已知"。
	}
	warnings := []string(nil)
	if !known {
		warnings = append(warnings, "mp4 brand not in known list (still treating as mp4)")
	}
	return MediaInfo{
		Container: "mp4",
		Encoding:  "", // E 档不解析 codec；调用方若有需要走 MediaProbe 注入
		Brands:    brands,
		ByteSize:  int64(len(in.Data)),
		Duration:  Duration{},
		State:     DurationUnknown,
		Origin:    origin,
		Warnings:  warnings,
	}
}

func bytesEq(a []byte, s string) bool {
	if len(a) != len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if a[i] != s[i] {
			return false
		}
	}
	return true
}
