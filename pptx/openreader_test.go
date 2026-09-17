package pptx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

// TestOpenReader_MissingOfficeDocument 验证：opc.Load 成功（Content Types 与
// 根关系流均合法）但根关系流缺少 officeDocument 关系时，OpenReader 必须
// 返回 ErrMalformedPackage 而非 panic（AT-14：恶意包预算内失败、无 panic）。
func TestOpenReader_MissingOfficeDocument(t *testing.T) {
	parts := minimalTemplateParts()
	// 篡改根关系流：仅保留 core-properties 关系，移除 officeDocument。
	parts["/_rels/.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId2" Type="` + opc.RelTypePrefix + `core-properties" Target="docProps/core.xml"/>` +
		`</Relationships>`)
	data := buildPackageZipPanic(parts)

	_, err := OpenReader(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatal("OpenReader with no officeDocument relationship: expected error, got nil")
	}
	if !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("OpenReader error = %v, want ErrMalformedPackage", err)
	}
}

// TestOpenReader_ExternalOfficeDocument 验证：officeDocument 关系存在但为
// 外部目标（TargetMode=External）时，OpenReader 同样应返回错误而非 panic。
func TestOpenReader_ExternalOfficeDocument(t *testing.T) {
	parts := minimalTemplateParts()
	parts["/_rels/.rels"] = []byte(xmlDecl +
		`<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelOfficeDocument + `" Target="https://evil.example/deck.xml" TargetMode="External"/>` +
		`</Relationships>`)
	data := buildPackageZipPanic(parts)

	_, err := OpenReader(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatal("OpenReader with external officeDocument: expected error, got nil")
	}
	if !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("OpenReader error = %v, want ErrMalformedPackage", err)
	}
}

func TestOpenFileErrorBranches(t *testing.T) {
	// 路径不存在 → os.Open 失败（Annotate 包装，不含错误分类哨兵）。
	if _, err := Open(filepath.Join(t.TempDir(), "missing.pptx")); err == nil {
		t.Fatal("Open missing path succeeded")
	}
	// 目录路径 → os.Open 成功但 Stat/读失败路径。
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("Open directory succeeded")
	}
	// 非 ZIP 内容 → opc.Load 失败映射 ErrMalformedPackage。
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.pptx")
	if err := os.WriteFile(bad, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(bad); !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("Open bad file: %v, want ErrMalformedPackage", err)
	}
	// 合法 Open 路径仍可用。
	good := filepath.Join(dir, "good.pptx")
	if err := os.WriteFile(good, newDocBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Open(good)
	if err != nil {
		t.Fatalf("Open good file: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
