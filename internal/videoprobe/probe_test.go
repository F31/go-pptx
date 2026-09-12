package videoprobe

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// buildMP4FTyp 构造最小 ftyp box（含 major+minor+两个 compatible brand）。
func buildMP4FTyp(major string, compatible ...string) []byte {
	size := 16 + 4*len(compatible)
	buf := make([]byte, 0, size)
	buf = append(buf, 0, 0, 0, 0) // size 占位
	binary.BigEndian.PutUint32(buf, uint32(size))
	buf = append(buf, "ftyp"...)
	buf = append(buf, major...)
	buf = append(buf, 0, 0, 0, 0) // minor_version
	for _, c := range compatible {
		if len(c) != 4 {
			panic("brand must be 4 bytes")
		}
		buf = append(buf, c...)
	}
	// 填充 size（已写入）。
	return buf
}

func TestProbeMP4_Ftyp(t *testing.T) {
	data := buildMP4FTyp("isom", "mp42", "avc1")
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp4" {
		t.Errorf("Container = %q, want mp4", info.Container)
	}
	if info.Origin != OriginByteSignature {
		t.Errorf("Origin = %v, want ByteSignature", info.Origin)
	}
	if info.State != DurationUnknown {
		t.Errorf("State = %v, want Unknown", info.State)
	}
}

func TestProbeMP4_UnknownBrandStillMP4(t *testing.T) {
	data := buildMP4FTyp("xxxx", "yyyy")
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp4" {
		t.Errorf("Container = %q, want mp4 (签名仍视为 MP4)", info.Container)
	}
	if len(info.Warnings) == 0 {
		t.Errorf("expected warning for unknown brand")
	}
}

func TestProbeWebM_Signature(t *testing.T) {
	data := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01}
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "webm" {
		t.Errorf("Container = %q, want webm", info.Container)
	}
	if info.Origin != OriginByteSignature {
		t.Errorf("Origin = %v, want ByteSignature", info.Origin)
	}
}

func TestProbe_UnknownReturnsEmpty(t *testing.T) {
	info, err := Probe(ProbeInput{Data: []byte("not a video")})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "" {
		t.Errorf("Container = %q, want empty", info.Container)
	}
	if info.Origin != OriginUnknown {
		t.Errorf("Origin = %v, want Unknown", info.Origin)
	}
}

func TestProbe_EmptyData(t *testing.T) {
	info, err := Probe(ProbeInput{Data: nil})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "" {
		t.Errorf("Container = %q, want empty", info.Container)
	}
}

func TestMP4_TruncatedFtyp_NoError(t *testing.T) {
	// ftyp 签名匹配但 size 异常；应返回 mp4 但无 error（签名已知）。
	data := []byte{0, 0, 0, 4, 'f', 't', 'y', 'p'} // size=4 截断
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp4" {
		t.Errorf("Container = %q, want mp4", info.Container)
	}
}

func TestMP4_NonFtypFirstBox(t *testing.T) {
	// 首 box size=8, type=moov；不是 ftyp → 签名不匹配，走其他探测器。
	data := []byte{0, 0, 0, 8, 'm', 'o', 'o', 'v', 0, 0, 0, 0}
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "" {
		t.Errorf("Container = %q, want empty", info.Container)
	}
}

func TestKnownDuration_Normalization(t *testing.T) {
	if got := KnownDuration(-1).Nanoseconds; got != 0 {
		t.Fatalf("negative KnownDuration = %d, want 0", got)
	}
	if got := (Duration{}).Milliseconds(); got != 0 {
		t.Fatalf("zero milliseconds = %d, want 0", got)
	}
	d := KnownDuration(1500000000) // 1.5s
	if d.Milliseconds() != 1500 {
		t.Errorf("Milliseconds = %d, want 1500", d.Milliseconds())
	}
	d2 := KnownDuration(1500500000)
	if d2.Milliseconds() != 1501 {
		t.Errorf("Milliseconds = %d, want 1501 (向上取整)", d2.Milliseconds())
	}
}

func TestIsKnownContainer(t *testing.T) {
	if !IsKnownContainer("mp4") {
		t.Error("mp4 should be known")
	}
	if !IsKnownContainer("webm") {
		t.Error("webm should be known")
	}
	if IsKnownContainer("mov") {
		t.Error("mov should not be known")
	}
}

func TestMP4Info_EncodingAndDurationEmpty(t *testing.T) {
	data := buildMP4FTyp("isom", "mp42")
	info, _ := Probe(ProbeInput{Data: data})
	if info.Encoding != "" {
		t.Errorf("Encoding = %q, want empty (E 档不解析)", info.Encoding)
	}
	if info.State != DurationUnknown {
		t.Errorf("State = %v, want Unknown", info.State)
	}
}

func TestWebM_NoDocTypeStillContainerKnown(t *testing.T) {
	// 仅有 EBML 签名，无 webm/matroska 字符串。
	data := []byte{0x1A, 0x45, 0xDF, 0xA3, 0xA3, 0x42, 0x86, 0x81, 0x01, 0x00}
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "webm" {
		t.Errorf("Container = %q, want webm", info.Container)
	}
}

func TestProbeErrorAndMalformedError(t *testing.T) {
	err := MalformedError("mp4", "ftyp box too short")
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("errors.Is = false for ErrMalformedMedia: %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "videoprobe: mp4: ftyp box too short") {
		t.Fatalf("Error = %q", got)
	}
	plain := (&ProbeError{ProbeInput: "bad input", Err: ErrMalformedMedia}).Error()
	if plain != "videoprobe: bad input" {
		t.Fatalf("plain Error = %q", plain)
	}
	if got := ErrMalformedMedia.Error(); got != "video media: malformed" {
		t.Fatalf("sentinel Error = %q", got)
	}
}

func TestMP4_ExtendedSizeFtypIsMalformed(t *testing.T) {
	data := []byte{0, 0, 0, 1, 'x', 'x', 'x', 'x', 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	_, err := Probe(ProbeInput{Data: data})
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("err = %v, want ErrMalformedMedia", err)
	}
}

func TestMP4_MalformedTooShortBox(t *testing.T) {
	data := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}
	_, err := Probe(ProbeInput{Data: data})
	if !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("err = %v, want ErrMalformedMedia", err)
	}
}

func TestWebM_DocTypeBrands(t *testing.T) {
	data := append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x42, 0x82, 0x84}, []byte("webm")...)
	info, err := Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(info.Brands) != 1 || info.Brands[0] != "webm" {
		t.Fatalf("Brands = %v, want [webm]", info.Brands)
	}
	data = append([]byte{0x1A, 0x45, 0xDF, 0xA3}, []byte("matroska")...)
	info, err = Probe(ProbeInput{Data: data})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(info.Brands) != 1 || info.Brands[0] != "matroska" {
		t.Fatalf("Brands = %v, want [matroska]", info.Brands)
	}
}
