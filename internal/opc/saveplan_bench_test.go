package opc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"runtime"
	"testing"
	"time"
)

// 本文件是 ADR-018 Tier 1 的收益锚点：量化「未变 Part 复制」从全缓冲
// （readAll）改为流式（io.Copy）后的分配与峰值内存差异。
//
// 设计要点：
//   - 用例用**未变 Part 占绝大多数**的形态（媒体重文档是真实痛点场景）；
//   - 输出写 io.Discard，测量的是 SavePlan.Write 本身的成本，不含落盘；
//   - 断言输出内容仍逐字节正确（B1 语义不被性能优化破坏）。
//
// 本文件刻意只依赖两个 commit 都存在的 helper（BuildSavePlan / Load /
// Budget 等），以便复制到 Tier 1 之前的 worktree 做前后对比。

// benchPayload 生成确定性的弱可压缩大块内容（模拟媒体 Part）。
func benchPayload(n int) string {
	b := make([]byte, n)
	x := uint32(0x1234567)
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return string(b)
}

// benchBuildZip 构造一个含 mediaCount 个 mediaSize 大小媒体 Part 的 OPC 包。
func benchBuildZip(b *testing.B, mediaSize, mediaCount int) []byte {
	b.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			b.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			b.Fatalf("write %s: %v", name, err)
		}
	}
	write("[Content_Types].xml", miniContentTypes(
		`<Default Extension="bin" ContentType="application/octet-stream"/>`))
	write("_rels/.rels", miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml")))
	write("ppt/deck.xml", `<p:presentation xmlns:p="urn:p"/>`)
	payload := benchPayload(mediaSize)
	for i := 0; i < mediaCount; i++ {
		write(fmt.Sprintf("ppt/media/bench%d.bin", i), payload)
	}
	if err := zw.Close(); err != nil {
		b.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// benchLoadPlan 载入包并产出「全量 CopyOriginal」计划（空变更集）。
func benchLoadPlan(b *testing.B, mediaSize, mediaCount int) (*Package, *SavePlan) {
	b.Helper()
	data := benchBuildZip(b, mediaSize, mediaCount)
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		b.Fatalf("Load: %v", err)
	}
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		b.Fatalf("BuildSavePlan: %v", err)
	}
	return pk, plan
}

// BenchmarkSavePlanWriteCopyOriginal 测量空变更集保存（全部走 CopyOriginal）
// 的分配成本。B/op 与 allocs/op 是 Tier 1 的核心收益指标：
// 全缓冲路径会为每个未变 Part 分配 O(Part 大小) 的切片，流式路径只保留
// io.Copy 的 32 KiB 缓冲。
func BenchmarkSavePlanWriteCopyOriginal(b *testing.B) {
	cases := []struct {
		name       string
		mediaSize  int
		mediaCount int
	}{
		{"1x1MiB", 1 << 20, 1},
		{"4x1MiB", 1 << 20, 4},
		{"3x8MiB", 8 << 20, 3},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			pk, plan := benchLoadPlan(b, tc.mediaSize, tc.mediaCount)
			b.SetBytes(int64(tc.mediaSize) * int64(tc.mediaCount))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := plan.Write(pk, io.Discard); err != nil {
					b.Fatalf("Write: %v", err)
				}
			}
		})
	}
}

// BenchmarkSavePlanWriteCopyOriginalPeakHeap 测量单次保存过程中的**堆峰值增量**
// （近似值：以 100µs 为间隔采样 HeapAlloc，取最大值减基线）。
//
// 这是比 B/op 更贴近「媒体重文档 OOM」痛点的指标——全缓冲路径会把
// 最大单个 Part 完整驻留堆上，流式路径不会。
func BenchmarkSavePlanWriteCopyOriginalPeakHeap(b *testing.B) {
	const mediaSize = 8 << 20
	const mediaCount = 3
	pk, plan := benchLoadPlan(b, mediaSize, mediaCount)

	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	baseline := ms.HeapAlloc

	peak := uint64(0)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Microsecond)
		defer ticker.Stop()
		var s runtime.MemStats
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				runtime.ReadMemStats(&s)
				if s.HeapAlloc > peak {
					peak = s.HeapAlloc
				}
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := plan.Write(pk, io.Discard); err != nil {
			close(done)
			b.Fatalf("Write: %v", err)
		}
	}
	b.StopTimer()
	close(done)

	delta := int64(peak) - int64(baseline)
	if delta < 0 {
		delta = 0
	}
	b.ReportMetric(float64(delta), "peak_heap_B")
}

// TestSavePlanWriteCopyOriginalStillByteExact 基准用例的内容正确性锚点：
// 性能优化不得改变 B1 语义——保存后的媒体 Part 与源包逐字节一致。
func TestSavePlanWriteCopyOriginalStillByteExact(t *testing.T) {
	const mediaSize = 512 << 10
	entries := map[string]string{
		"[Content_Types].xml": miniContentTypes(
			`<Default Extension="bin" ContentType="application/octet-stream"/>`),
		"_rels/.rels":      miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml")),
		"ppt/deck.xml":     `<p:presentation xmlns:p="urn:p"/>`,
		"ppt/media/b0.bin": benchPayload(mediaSize),
		"ppt/media/b1.bin": benchPayload(mediaSize * 2),
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	data := buf.Bytes()

	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	var out bytes.Buffer
	if err := plan.Write(pk, &out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := outputPartMap(t, out.Bytes())
	for name, want := range entries {
		pn := PartName("/" + name)
		g, ok := got[pn]
		if !ok {
			t.Fatalf("output missing %s", pn)
		}
		if !bytes.Equal(g, []byte(want)) {
			t.Fatalf("part %s differs: got %d bytes, want %d bytes", pn, len(g), len(want))
		}
	}
}
