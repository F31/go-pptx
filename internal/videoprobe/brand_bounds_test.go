package videoprobe

import "testing"

// 本文件锁定 MP4 ftyp compatible_brands 的收集上限——不可信输入的内存放大防护。
//
// 缺陷原型：compatible_brands 的长度由文件声明决定，旧实现为每 4 字节生成一个
// string（每个 string 另有 16 字节头开销），8 MiB 的 ftyp box 会放大到约 172 MiB
// （约 21×），在 512 MiB 媒体暂存上限下最坏可到十 GiB 级。现在收集上限为
// major + 64 个兼容品牌，与声明长度无关。

func TestProbeMP4_BrandsBounded(t *testing.T) {
	// 5000 个兼容品牌（约 20 KB 输入）：按旧实现会产生约 5001 个 string。
	many := make([]string, 0, 5000)
	many = append(many, "mp42") // 已知品牌置于上界内，应被正常识别
	for i := 0; i < 4999; i++ {
		many = append(many, "junk")
	}
	info, err := Probe(ProbeInput{Data: buildMP4FTyp("isom", many...)})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp4" {
		t.Fatalf("Container = %q, want mp4", info.Container)
	}
	// 品牌总数（major + 兼容品牌）上限 64；与输入里声明了多少个品牌无关。
	if len(info.Brands) != 64 {
		t.Fatalf("Brands = %d, want 64 (total brand cap); input declared %d compatible brands",
			len(info.Brands), len(many))
	}
	if info.Brands[0] != "isom" || info.Brands[1] != "mp42" {
		t.Fatalf("Brands head = %v, want [isom mp42]", info.Brands[:2])
	}
}

// TestProbeMP4_BrandBeyondCapNotScanned 记录上界的已知取舍：位于 64 之后的兼容
// 品牌不再参与白名单判定。真实 ftyp 的兼容品牌通常 ≤ 20 个，因此这只影响畸形
// 或刻意构造的文件——此时仍按签名识别为 mp4，不报错。
func TestProbeMP4_BrandBeyondCapNotScanned(t *testing.T) {
	many := make([]string, 0, 100)
	for i := 0; i < 99; i++ {
		many = append(many, "junk")
	}
	many = append(many, "avc1") // 已知品牌但位于上界之外
	info, err := Probe(ProbeInput{Data: buildMP4FTyp("xxxx", many...)})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp4" {
		t.Fatalf("Container = %q, want mp4 (signature still matches)", info.Container)
	}
	if len(info.Brands) != 64 {
		t.Fatalf("Brands = %d, want 64", len(info.Brands))
	}
}
