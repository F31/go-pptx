package opc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

// 本文件验证 ADR-018 Tier 2 raw 直通（生产路径 SavePlan.Write）：
//   - 未变 non-XML Part 走 raw 直通：输出压缩帧与源逐字节相同（跳过解压/重压缩）；
//   - 未变 XML Part 退回 Tier 1 流式：输出压缩帧与源不同（重新 Deflate）；
//   - 所有 Part 解压后内容逐字节一致（B1 语义不被 Tier 2 破坏）。
//
// 为干净区分 raw 直通与流式，源包一律用 Store 方法 + 自定义 Extra 标记写 Part：
// raw 直通原样保留帧（含标记），流式（zip.Writer.Create）重压缩为 Deflate 并生成自有
// Extra —— 二者输出帧必然不同，从而可判定实际走了哪条路径。

// rawMarkerExtra 是源包 Part 上的人造 Extra，用于检测是否被 raw 直通原样保留。
var rawMarkerExtra = []byte("T2-RAW-MARKER")

// rawTestBuildZip 构造媒体重包（全部 Part 以 Store + 标记 Extra 写入）。
func rawTestBuildZip(tb testing.TB, mediaSize, mediaCount int) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeRaw := func(name string, body []byte) {
		fh := &zip.FileHeader{
			Name:   name,
			Method: zip.Store,
			Extra:  append([]byte(nil), rawMarkerExtra...),
		}
		// CreateRaw 不重算大小，必须显式给出（Store：压缩 == 未压缩）。
		fh.UncompressedSize64 = uint64(len(body))
		fh.CompressedSize64 = uint64(len(body))
		w, err := zw.CreateRaw(fh)
		if err != nil {
			tb.Fatalf("CreateRaw %s: %v", name, err)
		}
		if _, err := w.Write(body); err != nil {
			tb.Fatalf("write %s: %v", name, err)
		}
	}
	writeRaw("[Content_Types].xml", []byte(miniContentTypes(
		`<Default Extension="bin" ContentType="application/octet-stream"/>`)))
	writeRaw("_rels/.rels", []byte(miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml"))))
	writeRaw("ppt/deck.xml", []byte(`<p:presentation xmlns:p="urn:p"/>`))
	payload := []byte(benchPayload(mediaSize))
	for i := 0; i < mediaCount; i++ {
		writeRaw(fmt.Sprintf("ppt/media/bench%d.bin", i), payload)
	}
	if err := zw.Close(); err != nil {
		tb.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// rawTestLoadPlan 返回源字节 + 已载入包 + 全量 CopyOriginal 计划（空变更集）。
func rawTestLoadPlan(tb testing.TB, mediaSize, mediaCount int) ([]byte, *Package, *SavePlan) {
	tb.Helper()
	data := rawTestBuildZip(tb, mediaSize, mediaCount)
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		tb.Fatalf("Load: %v", err)
	}
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		tb.Fatalf("BuildSavePlan: %v", err)
	}
	return data, pk, plan
}

// rawParts 返回 zip 各条目的原始压缩帧字节（经 OpenRaw，不做解压）。
func rawParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.OpenRaw()
		if err != nil {
			t.Fatalf("OpenRaw %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read raw %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

// decompressedParts 返回 zip 各条目解压后的内容（经 Open，做一次解压）。
func decompressedParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("Open %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

// TestSavePlanWriteTier2RawPassThrough 验证媒体 Part raw 直通 + XML 退回流式 + B1 内容一致。
func TestSavePlanWriteTier2RawPassThrough(t *testing.T) {
	data, pk, plan := rawTestLoadPlan(t, 1<<20, 3)

	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	srcRaw := rawParts(t, data)
	outRaw := rawParts(t, buf.Bytes())
	if len(outRaw) != len(srcRaw) {
		t.Fatalf("output parts = %d, want %d", len(outRaw), len(srcRaw))
	}

	usedRaw := 0
	for name, srcFrame := range srcRaw {
		outFrame, ok := outRaw[name]
		if !ok {
			t.Fatalf("part %s missing in output", name)
		}
		isMedia := !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels")
		if isMedia {
			// raw 直通：Store 帧 + 标记 Extra 必须原样保留。
			if !bytes.Equal(outFrame, srcFrame) {
				t.Errorf("media part %s NOT raw-passed (frame differs from source; Store+marker not preserved)", name)
			}
			usedRaw++
		} else {
			// XML 限定：必须退回流式（Create 重压缩为 Deflate，丢弃标记 Extra）。
			if bytes.Equal(outFrame, srcFrame) {
				t.Errorf("XML part %s was raw-passed; expected Tier 1 streaming (out of Tier 2 scope)", name)
			}
		}
	}
	if usedRaw == 0 {
		t.Fatalf("no media part used raw pass-through; Tier 2 not exercised")
	}

	// B1：解压后内容逐字节一致（无论 raw 还是流式，解压内容不变）。
	srcDec := decompressedParts(t, data)
	outDec := decompressedParts(t, buf.Bytes())
	for name, want := range srcDec {
		got, ok := outDec[name]
		if !ok {
			t.Fatalf("part %s missing in output (decompressed)", name)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("part %s decompressed content changed (B1 violation under Tier 2)", name)
		}
	}
	t.Logf("Tier 2 raw pass-through exercised on %d media parts; B1 decompressed content byte-identical", usedRaw)
}

// TestSavePlanWriteTier2RawPreservesSourceFrame 验证 raw 直通产物对媒体 Part 的
// 压缩帧（含 Store 方法 + 标记 Extra）与源包完全一致。
func TestSavePlanWriteTier2RawPreservesSourceFrame(t *testing.T) {
	data, pk, plan := rawTestLoadPlan(t, 512<<10, 4)

	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	srcRaw := rawParts(t, data)
	outRaw := rawParts(t, buf.Bytes())
	for name, src := range srcRaw {
		if strings.HasSuffix(name, ".bin") {
			if got := outRaw[name]; !bytes.Equal(got, src) {
				t.Errorf("media part %s frame differs from source (%d vs %d bytes)", name, len(got), len(src))
			}
		}
	}
}
