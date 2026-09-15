package opc

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"strings"
	"testing"
)

// 本文件守门 ADR-018 Tier 2 的**安全门限**与 `SavePlan.Write` 的错误传播。
//
// 背景（2026-09-16 COV-04 回归取证）：Tier 2（commit 752fb95）新增的
// `tryRawCopyOriginal` 有 4 条安全门限拒绝分支（非 XML / 仅 Store|Deflate /
// 排除加密与 data-descriptor 帧 / 尺寸必须已知），落地时**零测试覆盖** ——
// 直接把 `internal/opc` 覆盖率从 90.4% 拉到 89.5%，跌破 COV-04 的 90% 门槛，
// 且三天无人发现（当时只跑了 build/test/vet，没跑覆盖率门）。
//
// 这些门限是**产物安全的关键防线**：一旦某条失效，源包中的加密帧 / 尺寸未知帧
// 会被原样带进输出，PowerPoint/WPS 可能判为损坏。故必须有直接守门。
//
// 为什么必须手工构造 ZIP：`zip.Writer` 无法产出「未知 compression method」
// 「置位加密位 / data-descriptor 位」「声明尺寸为 0」的帧，而这正是门限要挡的输入。

// rawHdrEntry 描述一条手工构造的 ZIP 条目（可精确控制中央目录字段）。
type rawHdrEntry struct {
	name     string
	content  []byte
	method   uint16
	flags    uint16
	sizeSet  bool // true 时使用 compSz/uncompSz（可为 0，模拟尺寸未知）；否则按 content 长度
	compSz   uint32
	uncompSz uint32
}

// rawHdrZip 手工构造 ZIP（local header + central directory + EOCD）。
func rawHdrZip(entries []rawHdrEntry) []byte {
	var body bytes.Buffer
	offsets := make([]uint32, 0, len(entries))
	for _, e := range entries {
		offsets = append(offsets, uint32(body.Len()))
		comp, uncomp := rawHdrSizes(e)
		var lh bytes.Buffer
		lh.Write([]byte{'P', 'K', 3, 4})
		rawHdrU16(&lh, 20) // version needed
		rawHdrU16(&lh, e.flags)
		rawHdrU16(&lh, e.method)
		rawHdrU16(&lh, 0) // mod time
		rawHdrU16(&lh, 0) // mod date
		rawHdrU32(&lh, crc32.ChecksumIEEE(e.content))
		rawHdrU32(&lh, comp)
		rawHdrU32(&lh, uncomp)
		rawHdrU16(&lh, uint16(len(e.name)))
		rawHdrU16(&lh, 0) // extra len
		lh.WriteString(e.name)
		body.Write(lh.Bytes())
		body.Write(e.content)
	}
	cdStart := uint32(body.Len())
	var cd bytes.Buffer
	for i, e := range entries {
		comp, uncomp := rawHdrSizes(e)
		cd.Write([]byte{'P', 'K', 1, 2})
		rawHdrU16(&cd, 20) // version made by
		rawHdrU16(&cd, 20) // version needed
		rawHdrU16(&cd, e.flags)
		rawHdrU16(&cd, e.method)
		rawHdrU16(&cd, 0) // mod time
		rawHdrU16(&cd, 0) // mod date
		rawHdrU32(&cd, crc32.ChecksumIEEE(e.content))
		rawHdrU32(&cd, comp)
		rawHdrU32(&cd, uncomp)
		rawHdrU16(&cd, uint16(len(e.name)))
		rawHdrU16(&cd, 0) // extra len
		rawHdrU16(&cd, 0) // comment len
		rawHdrU16(&cd, 0) // disk start
		rawHdrU16(&cd, 0) // internal attrs
		rawHdrU32(&cd, 0) // external attrs
		rawHdrU32(&cd, offsets[i])
		cd.WriteString(e.name)
	}
	body.Write(cd.Bytes())
	var eocd bytes.Buffer
	eocd.Write([]byte{'P', 'K', 5, 6})
	rawHdrU16(&eocd, 0) // disk
	rawHdrU16(&eocd, 0) // cd start disk
	rawHdrU16(&eocd, uint16(len(entries)))
	rawHdrU16(&eocd, uint16(len(entries)))
	rawHdrU32(&eocd, uint32(cd.Len()))
	rawHdrU32(&eocd, cdStart)
	rawHdrU16(&eocd, 0) // comment len
	body.Write(eocd.Bytes())
	return body.Bytes()
}

func rawHdrSizes(e rawHdrEntry) (comp, uncomp uint32) {
	if e.sizeSet {
		return e.compSz, e.uncompSz
	}
	n := uint32(len(e.content))
	return n, n
}

func rawHdrU16(b *bytes.Buffer, v uint16) {
	var t [2]byte
	binary.LittleEndian.PutUint16(t[:], v)
	b.Write(t[:])
}

func rawHdrU32(b *bytes.Buffer, v uint32) {
	var t [4]byte
	binary.LittleEndian.PutUint32(t[:], v)
	b.Write(t[:])
}

// rawGuardPackage 构造最小可 Load 的 OPC 包，媒体条目使用给定的（可能异常的）header。
func rawGuardPackage(t *testing.T, media rawHdrEntry) *Package {
	t.Helper()
	entries := []rawHdrEntry{
		{name: "[Content_Types].xml", content: []byte(miniContentTypes(
			`<Default Extension="bin" ContentType="application/octet-stream"/>`)), method: zip.Store},
		{name: "_rels/.rels", content: []byte(miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml"))), method: zip.Store},
		{name: "ppt/deck.xml", content: []byte(`<p:presentation xmlns:p="urn:p"/>`), method: zip.Store},
		media,
	}
	data := rawHdrZip(entries)
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return pk
}

// TestTryRawCopyOriginalSafetyGate 逐条验证 Tier 2 安全门限：
// 只有「非 XML + Store|Deflate + 无加密位 + 无 data-descriptor 位 + 尺寸已知」才允许 raw 直通。
func TestTryRawCopyOriginalSafetyGate(t *testing.T) {
	const (
		flagEncrypted      = 1 << 0
		flagDataDescriptor = 1 << 3
	)
	cases := []struct {
		label   string
		media   rawHdrEntry
		part    PartName
		wantRaw bool
	}{
		{
			label:   "eligible-store-media",
			media:   rawHdrEntry{name: "ppt/media/ok.bin", content: []byte("payload"), method: zip.Store},
			part:    "/ppt/media/ok.bin",
			wantRaw: true,
		},
		{
			label:   "reject-xml-suffix",
			media:   rawHdrEntry{name: "ppt/notes.xml", content: []byte(`<a/>`), method: zip.Store},
			part:    "/ppt/notes.xml",
			wantRaw: false,
		},
		{
			label:   "reject-rels-suffix",
			media:   rawHdrEntry{name: "ppt/slides/_rels/slide1.xml.rels", content: []byte(miniRels("")), method: zip.Store},
			part:    "/ppt/slides/_rels/slide1.xml.rels",
			wantRaw: false,
		},
		{
			label:   "reject-missing-part",
			media:   rawHdrEntry{name: "ppt/media/other.bin", content: []byte("x"), method: zip.Store},
			part:    "/ppt/media/absent.bin",
			wantRaw: false,
		},
		{
			label:   "reject-exotic-method",
			media:   rawHdrEntry{name: "ppt/media/odd.bin", content: []byte("payload"), method: 99},
			part:    "/ppt/media/odd.bin",
			wantRaw: false,
		},
		{
			label:   "reject-encrypted-frame",
			media:   rawHdrEntry{name: "ppt/media/enc.bin", content: []byte("payload"), method: zip.Store, flags: flagEncrypted},
			part:    "/ppt/media/enc.bin",
			wantRaw: false,
		},
		{
			label:   "reject-data-descriptor-frame",
			media:   rawHdrEntry{name: "ppt/media/dd.bin", content: []byte("payload"), method: zip.Store, flags: flagDataDescriptor},
			part:    "/ppt/media/dd.bin",
			wantRaw: false,
		},
		{
			label: "reject-unknown-size",
			media: rawHdrEntry{
				name: "ppt/media/zero.bin", content: []byte("payload"),
				method: zip.Store, sizeSet: true, compSz: 0, uncompSz: 0,
			},
			part:    "/ppt/media/zero.bin",
			wantRaw: false,
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			pk := rawGuardPackage(t, c.media)
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			raw, err := new(SavePlan).tryRawCopyOriginal(pk, zw, c.part)
			if err != nil {
				t.Fatalf("tryRawCopyOriginal(%s) unexpected error: %v", c.part, err)
			}
			if raw != c.wantRaw {
				t.Fatalf("raw = %v, want %v (safety gate %s)", raw, c.wantRaw, c.label)
			}
			if err := zw.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatalf("zip.NewReader: %v", err)
			}
			// 被拒的帧**不得**在输出里注册条目（否则调用方回退会产出重复条目——
			// 正是 ADR-024 修复的缺陷形态）。注意空 ZIP 的 EOCD 本身占 22 字节，
			// 故断言的是条目数而不是字节长度。
			wantEntries := 0
			if c.wantRaw {
				wantEntries = 1
			}
			if len(zr.File) != wantEntries {
				t.Errorf("output entries = %d, want %d (rejected frames must register nothing)", len(zr.File), wantEntries)
			}
		})
	}
}

// TestSavePlanWriteFallsBackOnUnsafeFrame 端到端验证：源包媒体帧带 data-descriptor 位
// （尺寸可能不在头部 → raw 直通不安全）时，Write 退回 Tier 1 流式复制，
// 产物仍正确（条目数、解压内容、可重新 Load）。
//
// 构造说明：`zip.Writer.Create` 产出的条目天然带 flags bit3（Go 流式写入用
// data descriptor 回填尺寸），因此用 zipBytes 构造的源包正是"不安全帧"的真实形态；
// 而 Tier 2 raw 直通测试必须改用 CreateRaw + 显式尺寸才能产出合格帧。
func TestSavePlanWriteFallsBackOnUnsafeFrame(t *testing.T) {
	const flagDataDescriptor = 1 << 3

	payload := strings.Repeat("median-bytes-", 64)
	data := zipBytes(t, map[string]string{
		"[Content_Types].xml": miniContentTypes(`<Default Extension="bin" ContentType="application/octet-stream"/>`),
		"_rels/.rels":         miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml")),
		"ppt/deck.xml":        `<p:presentation xmlns:p="urn:p"/>`,
		"ppt/media/dd.bin":    payload,
	})

	zrSrc, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader(source): %v", err)
	}
	var mediaFlags uint16
	for _, f := range zrSrc.File {
		if f.Name == "ppt/media/dd.bin" {
			mediaFlags = f.Flags
		}
	}
	if mediaFlags&flagDataDescriptor == 0 {
		t.Fatalf("precondition failed: source media frame has no data-descriptor bit (flags=%d)", mediaFlags)
	}

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
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader(output): %v", err)
	}
	want := 0
	for _, e := range plan.Entries {
		if e.Action != Omit {
			want++
		}
	}
	if len(zr.File) != want {
		t.Errorf("output entries = %d, want %d", len(zr.File), want)
	}
	for _, f := range zr.File {
		if f.Name != "ppt/media/dd.bin" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open media: %v", err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read media: %v", err)
		}
		if string(got) != payload {
			t.Errorf("media content changed on fallback path (len %d vs %d)", len(got), len(payload))
		}
	}
	if _, err := Load(bytes.NewReader(buf.Bytes()), int64(buf.Len()), Budget{}); err != nil {
		t.Errorf("Load(output) rejected: %v", err)
	}
}

// TestSavePlanWritePropagatesRawCopyError 验证 raw 直通的写入阶段错误被上抛
// （条目已提交，不可静默退回）。
func TestSavePlanWritePropagatesRawCopyError(t *testing.T) {
	big := bytes.Repeat([]byte("z"), 1<<16) // 超过 zip.Writer 内部缓冲，强制底层写入
	pk := rawGuardPackage(t, rawHdrEntry{name: "ppt/media/big.bin", content: big, method: zip.Store})
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	fw := &failingWriter{}
	err = plan.Write(pk, fw)
	if err == nil {
		t.Fatalf("Write succeeded despite failing writer")
	}
	if !strings.Contains(err.Error(), "raw entry") {
		t.Errorf("error = %v, want mention of raw entry copy", err)
	}
}

// TestSavePlanWritePropagatesStreamCopyError 验证回退（Tier 1 流式）路径的写入错误被上抛。
func TestSavePlanWritePropagatesStreamCopyError(t *testing.T) {
	// 大 XML Part：XML 后缀使其必然走回退路径。
	bigXML := []byte(`<p:presentation xmlns:p="urn:p">` + strings.Repeat("x", 1<<16) + `</p:presentation>`)
	pk := rawGuardPackage(t, rawHdrEntry{name: "ppt/slides/slide1.xml", content: bigXML, method: zip.Store})
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	err = plan.Write(pk, &failingWriter{})
	if err == nil {
		t.Fatalf("Write succeeded despite failing writer")
	}
}

// TestSavePlanWriteFailsOnClose 验证 zip.Writer.Close（central directory flush）失败被上抛：
// 条目内容小于内部缓冲时，前段写入不触达底层，错误只在 Close 阶段显现。
func TestSavePlanWriteFailsOnClose(t *testing.T) {
	pk := rawGuardPackage(t, rawHdrEntry{name: "ppt/media/tiny.bin", content: []byte("t"), method: zip.Store})
	plan, err := BuildSavePlan(pk, &ChangeSet{})
	if err != nil {
		t.Fatalf("BuildSavePlan: %v", err)
	}
	err = plan.Write(pk, &failingWriter{})
	if err == nil {
		t.Fatalf("Write succeeded despite failing writer")
	}
}
