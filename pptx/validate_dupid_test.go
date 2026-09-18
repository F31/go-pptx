package pptx

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corruptSlideID 把已保存包中某 slide 的 from id 改为 to id（制造重复
// cNvPr@id 的畸形文件），用于验证 Validate 的重复 id 诊断。
func corruptSlideID(t *testing.T, path string, from, to ShapeID) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer zr.Close()
	files := make(map[string][]byte)
	var slideName string
	for _, f := range zr.File {
		rc, rerr := f.Open()
		if rerr != nil {
			t.Fatalf("open %s: %v", f.Name, rerr)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = b
		if strings.Contains(f.Name, "slides/slide") && strings.Contains(string(b), "cNvPr") {
			slideName = f.Name
		}
	}
	if slideName == "" {
		t.Fatal("saved package has no slide part")
	}
	oldS := fmt.Sprintf(`<p:cNvPr id="%d"`, from)
	newS := fmt.Sprintf(`<p:cNvPr id="%d"`, to)
	files[slideName] = []byte(strings.Replace(string(files[slideName]), oldS, newS, 1))

	wf, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	defer wf.Close()
	zw := zip.NewWriter(wf)
	for name, b := range files {
		w, werr := zw.Create(name)
		if werr != nil {
			t.Fatalf("zw.Create: %v", werr)
		}
		if _, werr := w.Write(b); werr != nil {
			t.Fatalf("write %s: %v", name, werr)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zw.Close: %v", err)
	}
}

// TestValidate_DuplicateShapeID：不可信/畸形第三方文件可能在同一 slide
// spTree 内重复 cNvPr@id（OOXML 规范要求 spTree 内唯一）。Validate 必须产出
// DRAWING_ID_DUPLICATE / SeverityWarning 诊断，而非静默通过——否则以 id 为
// 身份的句柄会定位到错误形状而不自知。默认仅诊断、不拒绝打开（与白皮书
// "未知内容冲突返回诊断而非静默丢弃"一致）。
func TestValidate_DuplicateShapeID(t *testing.T) {
	p, s := createSlide(t)
	a, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"})
	b, _ := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "B"})
	aID, bID := a.ID(), b.ID()
	if aID == bID {
		t.Fatalf("precondition: two shapes must have distinct ids (a=%d b=%d)", aID, bID)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.pptx")
	if _, err := p.Save(context.Background(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p.Close()
	// 把 b 的 id 改为 a 的 id，制造同 slide 内重复 cNvPr@id。
	corruptSlideID(t, path, bID, aID)

	p2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p2.Close()
	rep := p2.Validate(context.Background())
	found := false
	for _, d := range rep.Diagnostics {
		if d.Code == "DRAWING_ID_DUPLICATE" && d.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected DRAWING_ID_DUPLICATE/SeverityWarning diagnostic, got %+v", rep.Diagnostics)
	}
}

// TestValidate_UniqueShapeIDOK：正常文档（无重复 id）不应报 DRAWING_ID_DUPLICATE，
// 作为上述用例的反向基线。
func TestValidate_UniqueShapeIDOK(t *testing.T) {
	p, s := createSlide(t)
	if _, err := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "A"}); err != nil {
		t.Fatalf("AddTextBox A: %v", err)
	}
	if _, err := s.AddTextBox(TextBoxSpec{Width: 10, Height: 10, Name: "B"}); err != nil {
		t.Fatalf("AddTextBox B: %v", err)
	}
	defer p.Close()
	rep := p.Validate(context.Background())
	for _, d := range rep.Diagnostics {
		if d.Code == "DRAWING_ID_DUPLICATE" {
			t.Errorf("unexpected DRAWING_ID_DUPLICATE on clean document: %+v", d)
		}
	}
}
