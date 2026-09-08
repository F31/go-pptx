package opc

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"strings"
	"testing"
)

// zipBytes 用 zip.Writer 构造内存 ZIP（正常输入场景）。
func zipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	// 确定性顺序（排序写入便于断言 EntryNames）。
	sortStrings(names)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("Create(%q): %v", name, err)
		}
		if _, err := io.WriteString(f, entries[name]); err != nil {
			t.Fatalf("Write(%q): %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.Bytes()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

type rawEntry struct {
	name     string
	content  []byte
	declared int64 // 0 表示按 content 长度
}

// rawZipBytes 手工构造 ZIP（store 方法），允许重复/恶意条目名，
// 用于 L0 安全用例（zip.Writer 会阻止部分非法名）。
func rawZipBytes(entries []rawEntry) []byte {
	var body bytes.Buffer
	type centralInfo struct{ offset uint32 }
	var centrals []centralInfo

	for _, e := range entries {
		size := e.declared
		if size <= 0 {
			size = int64(len(e.content))
		}
		crc := crc32.ChecksumIEEE(e.content)
		offset := uint32(body.Len())

		var lh bytes.Buffer
		lh.Write([]byte{'P', 'K', 0x03, 0x04})
		writeU16(&lh, 20) // version needed
		writeU16(&lh, 0)  // flags
		writeU16(&lh, 0)  // method: store
		writeU16(&lh, 0)  // mod time
		writeU16(&lh, 0)  // mod date
		writeU32(&lh, crc)
		writeU32(&lh, uint32(size))
		writeU32(&lh, uint32(size))
		writeU16(&lh, uint16(len(e.name)))
		writeU16(&lh, 0) // extra len
		lh.WriteString(e.name)
		body.Write(lh.Bytes())
		body.Write(e.content)

		centrals = append(centrals, centralInfo{offset: offset})
	}

	cdStart := uint32(body.Len())
	var cd bytes.Buffer
	for i, e := range entries {
		size := e.declared
		if size <= 0 {
			size = int64(len(e.content))
		}
		cd.Write([]byte{'P', 'K', 0x01, 0x02})
		writeU16(&cd, 20) // version made by
		writeU16(&cd, 20) // version needed
		writeU16(&cd, 0)  // flags
		writeU16(&cd, 0)  // method
		writeU16(&cd, 0)  // mod time
		writeU16(&cd, 0)  // mod date
		writeU32(&cd, crc32.ChecksumIEEE(e.content))
		writeU32(&cd, uint32(size))
		writeU32(&cd, uint32(size))
		writeU16(&cd, uint16(len(e.name)))
		writeU16(&cd, 0) // extra len
		writeU16(&cd, 0) // comment len
		writeU16(&cd, 0) // disk start
		writeU16(&cd, 0) // internal attrs
		writeU32(&cd, 0) // external attrs
		writeU32(&cd, centrals[i].offset)
		cd.WriteString(e.name)
	}
	body.Write(cd.Bytes())

	var eocd bytes.Buffer
	eocd.Write([]byte{'P', 'K', 0x05, 0x06})
	writeU16(&eocd, 0)                    // disk
	writeU16(&eocd, 0)                    // cd start disk
	writeU16(&eocd, uint16(len(entries))) // entries this disk
	writeU16(&eocd, uint16(len(entries))) // total entries
	writeU32(&eocd, uint32(cd.Len()))     // cd size
	writeU32(&eocd, cdStart)              // cd offset
	writeU16(&eocd, 0)                    // comment len
	return body.Bytes()
}

func writeU16(b *bytes.Buffer, v uint16) {
	_ = binary.Write(b, binary.LittleEndian, v)
}

func writeU32(b *bytes.Buffer, v uint32) {
	_ = binary.Write(b, binary.LittleEndian, v)
}

func scanBytes(t *testing.T, data []byte, budget Budget) (*Index, error) {
	t.Helper()
	return Scan(bytes.NewReader(data), int64(len(data)), budget)
}

func TestScanValidPackage(t *testing.T) {
	data := zipBytes(t, map[string]string{
		"[Content_Types].xml":   `<Types/>`,
		"_rels/.rels":           `<Relationships/>`,
		"ppt/presentation.xml":  `<p:presentation/>`,
		"ppt/slides/slide1.xml": `<p:sld/>`,
		"ppt/media/image1.png":  "fake-png",
	})
	ix, err := scanBytes(t, data, Budget{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if ix.Count() != 5 {
		t.Fatalf("Count = %d, want 5", ix.Count())
	}
	for _, name := range []PartName{
		"/[Content_Types].xml", "/_rels/.rels", "/ppt/presentation.xml",
		"/ppt/slides/slide1.xml", "/ppt/media/image1.png",
	} {
		if !ix.HasPart(name) {
			t.Errorf("HasPart(%q) = false", name)
		}
	}
	if got := ix.TotalDeclared(); got == 0 {
		t.Error("TotalDeclared = 0")
	}
}

func TestScanDuplicateEntries(t *testing.T) {
	data := rawZipBytes([]rawEntry{
		{name: "a.xml", content: []byte("<a/>")},
		{name: "a.xml", content: []byte("<a/>")},
	})
	_, err := scanBytes(t, data, Budget{})
	if !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("err = %v, want ErrMalformedPackage", err)
	}
}

func TestScanCaseInsensitiveDuplicate(t *testing.T) {
	data := rawZipBytes([]rawEntry{
		{name: "ppt/Slide1.xml", content: []byte("<a/>")},
		{name: "ppt/slide1.xml", content: []byte("<a/>")},
	})
	_, err := scanBytes(t, data, Budget{})
	if !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("err = %v, want ErrMalformedPackage", err)
	}
}

func TestScanHostileNames(t *testing.T) {
	for _, name := range []string{
		"../evil.xml",
		"a/../../evil.xml",
		"/abs.xml",
		`ppt\slide1.xml`,
		"",
	} {
		entries := []rawEntry{{name: name, content: []byte("<a/>")}}
		_, err := scanBytes(t, rawZipBytes(entries), Budget{})
		if err == nil {
			t.Errorf("name %q: Scan succeeded, want error", name)
			continue
		}
		if !errors.Is(err, ErrMalformedPackage) && !errors.Is(err, ErrLimitExceeded) {
			t.Errorf("name %q: err = %v, want malformed/limit classification", name, err)
		}
	}
}

func TestScanTooManyEntries(t *testing.T) {
	data := zipBytes(t, map[string]string{
		"a.xml": "<a/>",
		"b.xml": "<b/>",
		"c.xml": "<c/>",
	})
	budget := DefaultBudget()
	budget.MaxEntries = 2
	_, err := scanBytes(t, data, budget)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
}

func TestScanDeclaredXMLSizeOverLimit(t *testing.T) {
	// 用合法 ZIP + 收紧预算验证"单 Part 超限"预检路径（构造真实大数据成本高）。
	data := zipBytes(t, map[string]string{
		"ppt/slides/slide1.xml": strings.Repeat("<p:sld/>", 40), // 实际约 440 字节
	})
	budget := DefaultBudget()
	budget.MaxXMLBytes = 100
	_, err := scanBytes(t, data, budget)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
}

func TestScanTotalSizeOverLimit(t *testing.T) {
	data := zipBytes(t, map[string]string{
		"ppt/a.xml": "abcdef",
		"ppt/b.xml": "ghijkl",
	})
	budget := DefaultBudget()
	budget.MaxTotalBytes = 10
	_, err := scanBytes(t, data, budget)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
}

func TestOpenXMLPart(t *testing.T) {
	data := zipBytes(t, map[string]string{
		"ppt/presentation.xml": "<p:presentation/>",
	})
	ix, err := scanBytes(t, data, Budget{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	rc, err := ix.OpenXMLPart("/ppt/presentation.xml")
	if err != nil {
		t.Fatalf("OpenXMLPart: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "<p:presentation/>" {
		t.Fatalf("content = %q", got)
	}
}

func TestOpenPartNotFound(t *testing.T) {
	data := zipBytes(t, map[string]string{"a.xml": "<a/>"})
	ix, err := scanBytes(t, data, Budget{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if _, err := ix.OpenXMLPart("/nope.xml"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCountedReadCloserExceeds(t *testing.T) {
	c := &countedReadCloser{
		r:     io.NopCloser(strings.NewReader(strings.Repeat("a", 4096))),
		limit: 10,
		name:  "test",
	}
	_, err := io.ReadAll(c)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
}

func TestCountedReadCloserWithinLimit(t *testing.T) {
	c := &countedReadCloser{
		r:     io.NopCloser(strings.NewReader("hello")),
		limit: 100,
		name:  "test",
	}
	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q", got)
	}
}

func TestPartName(t *testing.T) {
	valid := []PartName{
		"/[Content_Types].xml",
		"/ppt/presentation.xml",
		"/ppt/media/image1.png",
	}
	for _, n := range valid {
		if !n.Valid() {
			t.Errorf("%q: Valid() = false", n)
		}
	}
	invalid := []PartName{
		"", "ppt/presentation.xml", "/a/../b.xml", `/a\b.xml`, "//a.xml", "/./a.xml",
	}
	for _, n := range invalid {
		if n.Valid() {
			t.Errorf("%q: Valid() = true, want false", n)
		}
	}
	if got, _ := ContentTypesPartName.EntryName(); got != "[Content_Types].xml" {
		t.Errorf("EntryName = %q", got)
	}
}
