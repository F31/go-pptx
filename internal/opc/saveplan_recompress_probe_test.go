package opc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"testing"
)

// 本文件是 ADR-018 Tier 2 重启条件①的取证探针：量化「媒体重档 Save 的 p50
// 中重压缩（解压 + 重压缩）占比」是否 > 50%。
//
// 方法：构造一个媒体重包（未变 Part 占绝对多数），分别走两条路径：
//   - Stream：生产路径 SavePlan.Write —— CopyOriginal 走 OpenPart 解压 + zip writer 重压缩；
//   - Raw   ：测试本地 raw 直通 —— OpenRaw 取压缩帧 + CreateRaw 原样写入，跳过解压/重压缩。
//
// 两者在**同一进程**内先后计时，ns/op 之比直接给出重压缩在 CopyOriginal（亦即媒体重档
// 整体 Save）耗时中的占比：share = (Stream_ns − Raw_ns) / Stream_ns。
//
// 该占比对墙钟抖动（PERF-01 §6 本机 3.5× 漂移）相对稳健——Δ 与 Stream 同源同进程。
// 若 share > 0.5，则满足重启条件①，应落地 Tier 2；否则维持「不实施」，并把本证据补记到 ADR-018。

// probeBuildZip 构造媒体重包（与 saveplan_bench_test.go 同构，参数化便于复用）。
func probeBuildZip(tb testing.TB, mediaSize, mediaCount int) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			tb.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			tb.Fatalf("write %s: %v", name, err)
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
		tb.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// probeMediaZip 返回源字节 + 已载入包 + 全量 CopyOriginal 计划（空变更集）。
func probeMediaZip(tb testing.TB, mediaSize, mediaCount int) ([]byte, *Package, *SavePlan) {
	tb.Helper()
	data := probeBuildZip(tb, mediaSize, mediaCount)
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

// rawWriteCopyOriginal 是 Tier 2 raw 直通的最小 faithful 实现（仅测试内，不改生产代码）：
// 未变 media Part 用 zip.File.OpenRaw + zip.Writer.CreateRaw 原样搬运压缩帧；
// Emit*/New 仍走正常路径（内存字节无从直通）。
func rawWriteCopyOriginal(tb testing.TB, srcData []byte, plan *SavePlan, w io.Writer) error {
	tb.Helper()
	zr, err := zip.NewReader(bytes.NewReader(srcData), int64(len(srcData)))
	if err != nil {
		return err
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	zw := zip.NewWriter(w)
	for _, e := range plan.Entries {
		if e.Action == Omit {
			continue
		}
		name, err := e.Name.EntryName()
		if err != nil {
			return err
		}
		switch e.Action {
		case EmitPatched, EmitNew:
			f, err := zw.Create(name)
			if err != nil {
				return err
			}
			if _, err := f.Write(e.Content); err != nil {
				return err
			}
		case CopyOriginal:
			src, ok := files[name]
			if !ok {
				return fmt.Errorf("raw: missing source part %s", name)
			}
			// 复制头部（含 Method/CRC32/大小/Modified/Flags/Extra），CreateRaw 据此
			// 原样写出压缩帧，不重算 CRC/压缩。
			fh := src.FileHeader
			dst, err := zw.CreateRaw(&fh)
			if err != nil {
				return err
			}
			rc, err := src.OpenRaw()
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, rc); err != nil {
				return err
			}
		}
	}
	return zw.Close()
}

// BenchmarkCopyOriginalRecompressShare 量化重压缩占比。
//
// 固定 -benchtime 以保证 Stream/Raw 同口径（PERF-01 §6：跨轮次比较须同 benchtime）。
func BenchmarkCopyOriginalRecompressShare(b *testing.B) {
	cases := []struct {
		name       string
		mediaSize  int
		mediaCount int
	}{
		{"3x8MiB", 8 << 20, 3},      // 24 MiB 媒体（ADR-018 已测档）
		{"100x768", 768 * 768, 100}, // 约 55 MiB 媒体，模拟 100p-media 形状
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			data, pk, plan := probeMediaZip(b, tc.mediaSize, tc.mediaCount)
			b.SetBytes(int64(tc.mediaSize) * int64(tc.mediaCount))
			b.ReportAllocs()
			b.Run("Stream", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if err := plan.Write(pk, io.Discard); err != nil {
						b.Fatalf("Write: %v", err)
					}
				}
			})
			b.Run("Raw", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if err := rawWriteCopyOriginal(b, data, plan, io.Discard); err != nil {
						b.Fatalf("rawWrite: %v", err)
					}
				}
			})
		})
	}
}

// TestRawPassThroughFaithful 确认 raw 直通产物可被 opc 重新装配，且解压内容与源一致
// （保真性锚点：Tier 2 不损失任何 Part 内容）。
func TestRawPassThroughFaithful(t *testing.T) {
	data, pk, plan := probeMediaZip(t, 1<<20, 2)
	var buf bytes.Buffer
	if err := rawWriteCopyOriginal(t, data, plan, &buf); err != nil {
		t.Fatalf("rawWrite: %v", err)
	}
	out, err := Load(bytes.NewReader(buf.Bytes()), int64(buf.Len()), Budget{})
	if err != nil {
		t.Fatalf("Load raw output: %v", err)
	}
	for _, name := range pk.PartNames() {
		src, err := pk.readAll(name)
		if err != nil {
			t.Fatalf("read src %s: %v", name, err)
		}
		got, err := out.readAll(name)
		if err != nil {
			t.Fatalf("read out %s: %v", name, err)
		}
		if !bytes.Equal(src, got) {
			t.Fatalf("part %s content mismatch after raw pass-through", name)
		}
	}
}
