package ooxml

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

const ctNS = "http://schemas.openxmlformats.org/package/2006/content-types"
const relNS = "http://schemas.openxmlformats.org/package/2006/relationships"
const mainRel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"

func miniZipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
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

func designDocBytes(t *testing.T) []byte {
	t.Helper()
	contentTypes := `<Types xmlns="` + ctNS + `">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`</Types>`
	rels := `<Relationships xmlns="` + relNS + `">` +
		`<Relationship Id="rId1" Type="` + mainRel + `" Target="ppt/presentation.xml"/>` +
		`</Relationships>`
	pres := `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`
	slide := wrapSlide(sp("1", "S", "hello", "1"))
	return miniZipBytes(t, map[string]string{
		"[Content_Types].xml":   contentTypes,
		"_rels/.rels":           rels,
		"ppt/presentation.xml":  pres,
		"ppt/slides/slide1.xml": slide,
	})
}

func TestOpenAndBytes(t *testing.T) {
	data := designDocBytes(t)
	pkg, err := Open(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	b, ok := Bytes(pkg, "/ppt/slides/slide1.xml")
	if !ok {
		t.Fatal("Bytes(missing? existing failed)")
	}
	if len(b) == 0 {
		t.Fatal("empty slide bytes")
	}

	if _, ok := Bytes(pkg, "/ppt/nope.xml"); ok {
		t.Error("missing part should report false")
	}
	if _, ok := Bytes(nil, "/ppt/slides/slide1.xml"); ok {
		t.Error("nil package should report false")
	}
}

func TestOpenBadZip(t *testing.T) {
	_, err := Open(bytes.NewReader([]byte("not a zip at all")), 16)
	if err == nil {
		t.Fatal("want error for bad zip")
	}
}
