package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/go-pptx"
)

// cliBindDeck 生成一页含 `{{ title }}` 占位符形状的最小 PPTX。
//
// 本库尚无 AddTextbox 类创建型形状 API（M2/M3 未覆盖），故以
// New()+AddSlide 产出合法包后做一次 zip 手术：替换该页 slide XML 为
// 含占位符正文的页面。仅供 CLI 端到端验证使用。
func cliBindDeck(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p, err := pptx.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	layouts, _ := p.Layouts()
	if _, err := p.AddSlide(layouts[0]); err != nil {
		p.Close()
		t.Fatalf("AddSlide: %v", err)
	}
	base := filepath.Join(dir, "base.pptx")
	if _, err := p.Save(context.Background(), base); err != nil {
		p.Close()
		t.Fatalf("Save: %v", err)
	}
	p.Close()

	const sld = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
		` xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>Report: {{ title }}</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:sld>`

	out := filepath.Join(dir, "tpl.pptx")
	if err := rewriteSlidePart(base, out, sld); err != nil {
		t.Fatalf("rewriteSlidePart: %v", err)
	}
	return out
}

// rewriteSlidePart 复制 src 包并把首个 slide Part 替换为给定 XML。
func rewriteSlidePart(src, dst, slideXML string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	done := false
	for _, f := range zr.File {
		w, err := zw.Create(f.Name)
		if err != nil {
			return err
		}
		if !done && strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			done = true
			if _, err := io.WriteString(w, slideXML); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("no slide part found in %s", src)
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}

func cliWriteData(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestCLI_Bind_HappyPath(t *testing.T) {
	in := cliBindDeck(t)
	data := cliWriteData(t, `{"title":"Q3 Review"}`)
	out := filepath.Join(t.TempDir(), "out.pptx")
	code, stdout, stderr := runCLI([]string{"bind", "--data", data, "--output", out, in})
	if code != ExitOK {
		t.Fatalf("bind exit = %v, stderr = %s", code, stderr)
	}
	var rep bindReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("report JSON: %v\nstdout=%s", err, stdout)
	}
	if rep.Status != "ok" || rep.Substituted != 1 {
		t.Errorf("report = %+v", rep)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output file missing: %v", err)
	}
	// 渲染结果可读回：占位符已被替换。
	p, err := pptx.Open(out)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer p.Close()
	slides, _ := p.Slides()
	shapes, _ := slides[0].Shapes()
	for _, sh := range shapes {
		if as, ok := sh.(*pptx.AutoShape); ok {
			tf, err := as.TextFrame()
			if err != nil || tf == nil {
				continue
			}
			paras, _ := tf.Paragraphs()
			for _, para := range paras {
				txt, _ := para.Text()
				if strings.Contains(txt, "Q3 Review") {
					return
				}
			}
		}
	}
	t.Error("bound text not found in output deck")
}

func TestCLI_Bind_MissingKeyNoOutput(t *testing.T) {
	in := cliBindDeck(t)
	data := cliWriteData(t, `{"other":"x"}`)
	out := filepath.Join(t.TempDir(), "out.pptx")
	// 数据缺键按 classifyError 归为输入/用法错误（§23.2 退出码 2）。
	code, _, stderr := runCLI([]string{"bind", "--data", data, "--output", out, in})
	if code != ExitUsageError {
		t.Fatalf("bind exit = %v, want %v (stderr=%s)", code, ExitUsageError, stderr)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("output file must not be written on binding failure")
	}
	// --loose 保留原文并成功写出。
	code, _, stderr = runCLI([]string{"bind", "--data", data, "--output", out, "--loose", in})
	if code != ExitOK {
		t.Fatalf("loose bind exit = %v, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("loose output missing: %v", err)
	}
}

func TestCLI_Bind_MissingFlags(t *testing.T) {
	in := cliBindDeck(t)
	if code, _, _ := runCLI([]string{"bind", in}); code != ExitUsageError {
		t.Errorf("missing --data exit = %v, want usage", code)
	}
	data := cliWriteData(t, `{}`)
	if code, _, _ := runCLI([]string{"bind", "--data", data, in}); code != ExitUsageError {
		t.Errorf("missing --output exit = %v, want usage", code)
	}
}

func TestCLI_Bind_InvalidDataJSON(t *testing.T) {
	in := cliBindDeck(t)
	data := cliWriteData(t, `{"title":`)
	out := filepath.Join(t.TempDir(), "out.pptx")
	if code, _, _ := runCLI([]string{"bind", "--data", data, "--output", out, in}); code != ExitUsageError {
		t.Errorf("invalid JSON exit = %v, want usage", code)
	}
}
