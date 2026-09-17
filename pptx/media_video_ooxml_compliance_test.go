package pptx

import (
	"strings"
	"testing"

	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

// 本文件守门 ADR-026：video 形状的 OOXML 合规性（与 ADR-025 的 audio 同源）。
//
// 背景：`buildVideoPicFragment` 产出的视频 pic 有两处 schema 违反——
//  1. p:nvPicPr 缺必需的 p:nvPr；
//  2. 视频引用写成 `p:videoFile`（命名空间错）、缺必需的 r:link、且位置错
//     （应在 p:nvPr 内，正确写法是 `<a:videoFile r:link="rIdX"/>`）。
// 后果：含视频的产物在 PowerPoint 下被判"文件或目录损坏"（0x80070570），
// 而 WPS 宽容接受。两处均为必要条件，且正确形式经 PowerPoint 原生
// `AddMediaObject2` 的实包取证确认。

// TestBuildVideoPicFragmentIsSchemaCompliant 断言视频形状片段的结构合规性。
func TestBuildVideoPicFragmentIsSchemaCompliant(t *testing.T) {
	frag := buildVideoPicFragment(11, "Video 11", "rId2", "mp4", 0, 0, 914400, 514350, "", false)

	if !strings.Contains(frag, "<p:nvPr>") {
		t.Errorf("fragment lacks required <p:nvPr>: %s", frag)
	}
	nvPr := strings.Index(frag, "<p:nvPr>")
	video := strings.Index(frag, `<a:videoFile r:link="rId2"/>`)
	blipStart := strings.Index(frag, "<p:blipFill>")
	blipEnd := strings.Index(frag, "</p:blipFill>")
	if nvPr < 0 || video < 0 {
		t.Fatalf("fragment lacks nvPr or <a:videoFile r:link>: %s", frag)
	}
	if video < nvPr {
		t.Errorf("a:videoFile must live inside p:nvPr: %s", frag)
	}
	if blipStart >= 0 && blipEnd >= 0 && video > blipStart && video < blipEnd {
		t.Errorf("a:videoFile must not live inside p:blipFill: %s", frag)
	}
	if strings.Contains(frag, "p:videoFile") {
		t.Errorf("fragment must not use the wrong-namespace p:videoFile: %s", frag)
	}
	if !strings.Contains(frag, `<a:blip r:embed="rId2"/>`) {
		t.Errorf("fragment lacks a:blip reference: %s", frag)
	}
}

// TestPicMediaKindAcceptsBothVideoForms 断言读侧**同时**识别正确形式
// （p:nvPr 内的 a:videoFile）与旧形式（p:blipFill 内的 p:videoFile）。
//
// 兼容探测必须"追加"而非"替换"：ADR-025 期间曾直接改命名空间，立即被
// 既有测试拦下（旧样本写 p:videoFile）——保留旧分支才能读回 v1.0.5 及
// 更早产物。
func TestPicMediaKindAcceptsBothVideoForms(t *testing.T) {
	const head = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"` +
		` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<p:cSld><p:spTree>`
	const tail = `</p:spTree></p:cSld></p:sld>`

	cases := []struct {
		name string
		pic  string
		want string
	}{
		{
			name: "correct: a:videoFile inside p:nvPr",
			pic: `<p:pic><p:nvPicPr><p:cNvPr id="11" name="Video 11"/><p:cNvPicPr/>` +
				`<p:nvPr><a:videoFile r:link="rId2"/></p:nvPr></p:nvPicPr>` +
				`<p:blipFill><a:blip r:embed="rId2"/></p:blipFill></p:pic>`,
			want: "video",
		},
		{
			name: "legacy: p:videoFile inside p:blipFill",
			pic: `<p:pic><p:nvPicPr><p:cNvPr id="11" name="Video 11"/><p:cNvPicPr/></p:nvPicPr>` +
				`<p:blipFill><a:blip r:embed="rId2"/><p:videoFile contentType="video/mp4"/></p:blipFill></p:pic>`,
			want: "video",
		},
		{
			name: "correct: a:audioFile inside p:nvPr",
			pic: `<p:pic><p:nvPicPr><p:cNvPr id="12" name="Audio 12"/><p:cNvPicPr/>` +
				`<p:nvPr><a:audioFile r:link="rId3"/></p:nvPr></p:nvPicPr>` +
				`<p:blipFill><a:blip r:embed="rId3"/></p:blipFill></p:pic>`,
			want: "audio",
		},
		{
			name: "plain picture",
			pic: `<p:pic><p:nvPicPr><p:cNvPr id="13" name="Picture 13"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr>` +
				`<p:blipFill><a:blip r:embed="rId4"/></p:blipFill></p:pic>`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := xmlstore.Index([]byte(head + tc.pic + tail))
			if err != nil {
				t.Fatalf("xmlstore.Index: %v", err)
			}
			ids := doc.Elements(nsPresentationML, "pic")
			if len(ids) == 0 {
				t.Fatal("pic element not indexed")
			}
			pic := doc.Node(ids[0])
			if pic == nil {
				t.Fatal("pic node nil")
			}
			if got := picMediaKind(doc, pic); got != tc.want {
				t.Errorf("picMediaKind = %q, want %q", got, tc.want)
			}
		})
	}
}
