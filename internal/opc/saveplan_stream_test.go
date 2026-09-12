package opc

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// 本文件是 ADR-018 Tier 1（CopyOriginal 改流式 io.Copy）的行为锚点：
// 流式路径必须产出与旧 readAll 路径**逐字节相同**的内容，且不能绕过读取侧预算。

// pseudoBytes 生成确定性的大块内容（非简单重复，避免被压缩成平凡形态）。
func pseudoBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i%251) + byte(i/251%7)
	}
	return string(b)
}

// TestSavePlanCopyOriginalLargePartIsByteExact 用远大于 io.Copy 内部缓冲
// （32 KiB）的未变 Part 断言 B1：保存后解压内容与源包逐字节一致。
//
// 这是 Tier 1 的核心回归——旧路径把整个 Part 读进内存再写出，新路径边读边写，
// 内容必须完全相同。
func TestSavePlanCopyOriginalLargePartIsByteExact(t *testing.T) {
	const mediaName = PartName("/ppt/media/big.bin")
	body := pseudoBytes(1 << 20) // 1 MiB

	entries := map[string]string{
		"[Content_Types].xml": miniContentTypes(
			`<Default Extension="bin" ContentType="application/octet-stream"/>`),
		"_rels/.rels":       miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml")),
		"ppt/deck.xml":      `<p:presentation xmlns:p="urn:p"/>`,
		"ppt/media/big.bin": body,
	}
	data := zipBytes(t, entries)
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	var buf bytes.Buffer
	if err := plan.Write(pk, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	out := outputPartMap(t, buf.Bytes())
	got, ok := out[mediaName]
	if !ok {
		t.Fatalf("output missing %s (entries=%d)", mediaName, len(out))
	}
	if !bytes.Equal(got, []byte(body)) {
		t.Fatalf("streamed copy differs: got %d bytes hash %x, want %d bytes hash %x",
			len(got), hashBytes(got), len(body), hashBytes([]byte(body)))
	}
}

// errWriter 在任何写入时返回错误，用于覆盖流式复制的写失败分支。
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

// TestCopyPartErrors 覆盖 copyPart 的错误分支（ADR-018 Tier 1）：
// 源 Part 缺失 → OpenPart 错误；写入失败 → io.Copy 错误。两者都必须保留
// "copy entry %s" 前缀（错误契约不变）并关闭已打开的读取器。
func TestCopyPartErrors(t *testing.T) {
	pk := loadMiniPackage(t)

	t.Run("missing source part", func(t *testing.T) {
		var buf bytes.Buffer
		err := copyPart(pk, &buf, "/ppt/missing.xml")
		if err == nil {
			t.Fatal("copyPart: want error for missing part, got nil")
		}
		if !strings.Contains(err.Error(), "copy entry /ppt/missing.xml") {
			t.Fatalf("copyPart error = %q, want copy entry prefix", err)
		}
	})

	t.Run("write failure", func(t *testing.T) {
		want := errors.New("boom")
		err := copyPart(pk, errWriter{err: want}, "/ppt/slides/slide1.xml")
		if err == nil {
			t.Fatal("copyPart: want error for failing writer, got nil")
		}
		if !errors.Is(err, want) {
			t.Fatalf("copyPart error = %v, want wrapped %v", err, want)
		}
	})
}

// TestSavePlanCopyOriginalStreamingIsIdempotent 连续两次保存同一未变大 Part，
// 输出字节必须完全一致（流式路径不得引入缓冲边界相关的抖动）。
func TestSavePlanCopyOriginalStreamingIsIdempotent(t *testing.T) {
	entries := map[string]string{
		"[Content_Types].xml": miniContentTypes(
			`<Default Extension="bin" ContentType="application/octet-stream"/>`),
		"_rels/.rels":       miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml")),
		"ppt/deck.xml":      `<p:presentation xmlns:p="urn:p"/>`,
		"ppt/media/big.bin": pseudoBytes(300 << 10), // 300 KiB，跨多个缓冲块
	}
	data := zipBytes(t, entries)
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}

	var first, second bytes.Buffer
	if err := plan.Write(pk, &first); err != nil {
		t.Fatalf("Write #1: %v", err)
	}
	if err := plan.Write(pk, &second); err != nil {
		t.Fatalf("Write #2: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("streaming copy is not idempotent: %d vs %d bytes",
			first.Len(), second.Len())
	}
}
