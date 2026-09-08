package pptx

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

func writeFileBytes(path string, b []byte) error {
	return os.WriteFile(path, b, 0o644)
}

// ---------- IMAGE-01 图片测试 ----------

// testPNG 生成 w×h 纯色 PNG 字节（rgb 为 RGB 三通道值）。
func testPNG(w, h int, rgb byte) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		if i%4 == 3 {
			img.Pix[i] = 255
		} else {
			img.Pix[i] = rgb
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// testJPEG 生成 w×h 纯色 JPEG 字节。
func testJPEG(w, h int, rgb byte) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c := color.RGBA{R: rgb, G: rgb, B: rgb, A: 255}
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// emptySlideXML 生成含空 spTree 的页面 XML。
func emptySlideXML() string {
	return `<p:sld xmlns:a="` + nsDrawingML + `" xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr/>` +
		`</p:spTree></p:cSld></p:sld>`
}

// imageDeckFixture 构造 n 页空页面的包（无 slide 关系流——覆盖 AddPicture
// 为缺失关系流新建的场景）。
func imageDeckFixture(t *testing.T, n int) *Presentation {
	t.Helper()
	parts := minimalTemplateParts()
	var rels, ids, cts strings.Builder
	rels.WriteString(xmlDecl + `<Relationships xmlns="` + nsPkgRels + `">` +
		`<Relationship Id="rId1" Type="` + opc.RelSlideMaster + `" Target="slideMasters/slideMaster1.xml"/>`)
	ids.WriteString(`<p:sldIdLst>`)
	for i := 1; i <= n; i++ {
		name := "/ppt/slides/slide" + strconv.Itoa(i) + ".xml"
		parts[opc.PartName(name)] = []byte(xmlDecl + emptySlideXML())
		rels.WriteString(`<Relationship Id="rId` + strconv.Itoa(i+1) + `" Type="` + opc.RelSlide +
			`" Target="slides/slide` + strconv.Itoa(i) + `.xml"/>`)
		ids.WriteString(`<p:sldId id="` + strconv.Itoa(255+i) + `" r:id="rId` + strconv.Itoa(i+1) + `"/>`)
		cts.WriteString(`<Override PartName="` + name + `" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`)
	}
	rels.WriteString(`</Relationships>`)
	ids.WriteString(`</p:sldIdLst>`)
	parts["/ppt/_rels/presentation.xml.rels"] = []byte(rels.String())
	parts["/ppt/presentation.xml"] = []byte(xmlDecl +
		`<p:presentation xmlns:r="` + nsOfficeDocument + `" xmlns:p="` + nsPresentationML + `">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		ids.String() +
		`<p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>` +
		`</p:presentation>`)
	parts["/[Content_Types].xml"] = bytes.Replace(parts["/[Content_Types].xml"],
		[]byte("</Types>"), []byte(cts.String()+`</Types>`), 1)
	return openFixture(t, buildPackageZipPanic(parts))
}

func deckSlides(t *testing.T, p *Presentation) []*Slide {
	t.Helper()
	slides, err := p.Slides()
	if err != nil {
		t.Fatalf("Slides: %v", err)
	}
	return slides
}

// picView 是从页面解析出的图片关键信息（测试断言用）。
type picView struct {
	id         int64
	embed      string
	descr      string
	decorative bool
	offX, offY int64
	extCx      int64
	extCy      int64
	srcRect    string // l/t/r/b 拼接；无裁剪为空串
}

// slidePics 返回页面全部 p:pic 视图（文档序）。
func slidePics(t *testing.T, s *Slide) []picView {
	t.Helper()
	doc, err := s.p.docOf(s.part)
	if err != nil {
		t.Fatalf("docOf: %v", err)
	}
	var out []picView
	for _, tid := range doc.Elements(nsPresentationML, "spTree") {
		tree := doc.Node(tid)
		for _, cid := range tree.Children {
			pic := doc.Node(cid)
			if pic.Namespace != nsPresentationML || pic.Local() != "pic" {
				continue
			}
			v := picView{}
			if c := cNvPrOf(doc, pic); c != nil {
				if id, ok := c.Attr("", "id"); ok {
					v.id, _ = strconv.ParseInt(id, 10, 64)
				}
				v.descr, _ = c.Attr("", "descr")
				d, _ := c.Attr("", "decorative")
				v.decorative = d == "1"
			}
			if b := picBlipOf(doc, pic); b != nil {
				v.embed, _ = b.Attr(nsOfficeDocument, "embed")
			}
			if sr := picSrcRectOf(doc, pic); sr != nil {
				l, _ := sr.Attr("", "l")
				tt, _ := sr.Attr("", "t")
				r, _ := sr.Attr("", "r")
				bb, _ := sr.Attr("", "b")
				v.srcRect = l + "/" + tt + "/" + r + "/" + bb
			}
			xfrm := childOfKind(doc, pic, nsPresentationML, "spPr", 0)
			if xfrm != nil {
				xf := childOfKind(doc, xfrm, nsDrawingML, "xfrm", 0)
				if xf != nil {
					if off := childOfKind(doc, xf, nsDrawingML, "off", 0); off != nil {
						v.offX = attrInt64(t, off, "x")
						v.offY = attrInt64(t, off, "y")
					}
					if ext := childOfKind(doc, xf, nsDrawingML, "ext", 0); ext != nil {
						v.extCx = attrInt64(t, ext, "cx")
						v.extCy = attrInt64(t, ext, "cy")
					}
				}
			}
			out = append(out, v)
		}
	}
	return out
}

func attrInt64(t *testing.T, n interface {
	Attr(ns, name string) (string, bool)
}, name string) int64 {
	t.Helper()
	v, ok := n.Attr("", name)
	if !ok {
		t.Fatalf("missing attr %s", name)
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		t.Fatalf("attr %s=%q: %v", name, v, err)
	}
	return i
}

// mediaParts 列出当前文档中的媒体 Part 名。
func mediaParts(p *Presentation) []string {
	seen := map[opc.PartName]bool{}
	var out []string
	for _, n := range p.pk.PartNames() {
		if strings.HasPrefix(string(n), "/ppt/media/") {
			seen[n] = true
		}
	}
	for n := range p.addedParts {
		if strings.HasPrefix(string(n), "/ppt/media/") {
			seen[n] = true
		}
	}
	for n := range seen {
		out = append(out, string(n))
	}
	return out
}

func slideImageRels(t *testing.T, s *Slide) map[string]string { // rid → target
	t.Helper()
	rels, ok, err := s.p.relsOf(s.part)
	if err != nil || !ok {
		t.Fatalf("relsOf: ok=%v err=%v", ok, err)
	}
	out := map[string]string{}
	for _, r := range rels {
		if r.Mode == opc.TargetInternal && r.Type == relImage {
			out[r.ID] = string(r.TargetPart)
		}
	}
	return out
}

// ---------- 用例 ----------

func TestAddPictureOriginalSizeAndRels(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	data := testPNG(4, 2, 200)

	ps, err := s.AddPicture(context.Background(), BytesMedia(data, ""), PictureSpec{})
	if err != nil {
		t.Fatalf("AddPicture: %v", err)
	}
	if ps == nil {
		t.Fatal("nil handle")
	}
	pics := slidePics(t, s)
	if len(pics) != 1 {
		t.Fatalf("pics = %d", len(pics))
	}
	v := pics[0]
	if v.offX != 0 || v.offY != 0 || v.extCx != 4*emuPerPixel96 || v.extCy != 2*emuPerPixel96 {
		t.Errorf("original-size geometry = (%d,%d,%d,%d)", v.offX, v.offY, v.extCx, v.extCy)
	}
	rels := slideImageRels(t, s)
	target, ok := rels[v.embed]
	if !ok {
		t.Fatalf("no rel for embed %s", v.embed)
	}
	if target != "/ppt/media/image1.png" {
		t.Errorf("media target = %s", target)
	}
	b, err := p.partBytes(opc.PartName(target))
	if err != nil || !bytes.Equal(b, data) {
		t.Errorf("media bytes mismatch (err=%v)", err)
	}
	if v.id < 2 || v.descr != "" || v.decorative {
		t.Errorf("cNvPr: id=%d descr=%q dec=%v", v.id, v.descr, v.decorative)
	}
}

func TestAddPictureFitGeometry(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	data := testPNG(4, 2, 100) // 2:1
	base := PictureSpec{X: 1000, Y: 2000, Width: 100000, Height: 100000}

	cases := []struct {
		fit      PictureFitMode
		wantOffX int64
		wantOffY int64
		wantCx   int64
		wantCy   int64
		srcRect  string
	}{
		{FitOriginalSize, 1000, 2000, 38100, 19050, ""},
		{FitStretch, 1000, 2000, 100000, 100000, ""},
		{FitContain, 1000, 2000 + 25000, 100000, 50000, ""},
		{FitCover, 1000, 2000, 100000, 100000, "25000/0/25000/0"},
	}
	for _, c := range cases {
		spec := base
		spec.Fit = c.fit
		if _, err := s.AddPicture(context.Background(), BytesMedia(data, ""), spec); err != nil {
			t.Fatalf("%s: %v", c.fit, err)
		}
	}
	pics := slidePics(t, s)
	if len(pics) != len(cases) {
		t.Fatalf("pics = %d", len(pics))
	}
	for i, c := range cases {
		v := pics[i]
		if v.offX != c.wantOffX || v.offY != c.wantOffY || v.extCx != c.wantCx || v.extCy != c.wantCy {
			t.Errorf("%s: geom=(%d,%d,%d,%d) want (%d,%d,%d,%d)",
				c.fit, v.offX, v.offY, v.extCx, v.extCy, c.wantOffX, c.wantOffY, c.wantCx, c.wantCy)
		}
		if v.srcRect != c.srcRect {
			t.Errorf("%s: srcRect=%q want %q", c.fit, v.srcRect, c.srcRect)
		}
	}
}

func TestAddPictureMissingSizeRejected(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	for _, fit := range []PictureFitMode{FitStretch, FitContain, FitCover} {
		_, err := s.AddPicture(context.Background(), BytesMedia(testPNG(2, 2, 1), ""),
			PictureSpec{Fit: fit})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: err=%v want ErrInvalidArgument", fit, err)
		}
	}
}

func TestAddPictureAltTextAndDecorative(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	ps, err := s.AddPicture(context.Background(), BytesMedia(testPNG(1, 1, 90), ""),
		PictureSpec{AltText: `red & "quoted" chart`})
	if err != nil {
		t.Fatal(err)
	}
	if got := ps.AltText(); got != `red & "quoted" chart` {
		t.Errorf("AltText = %q", got)
	}
	if ps.IsDecorative() {
		t.Error("IsDecorative = true initially")
	}
	// SetDecorative(true)：装饰标记替换替代文本。
	if err := ps.SetDecorative(true); err != nil {
		t.Fatal(err)
	}
	v := slidePics(t, s)[0]
	if v.decorative != true || v.descr != "" {
		t.Errorf("after decorative: dec=%v descr=%q", v.decorative, v.descr)
	}
	if !ps.IsDecorative() || ps.AltText() != "" {
		t.Errorf("read after decorative: dec=%v alt=%q", ps.IsDecorative(), ps.AltText())
	}
	// SetAltText：清除装饰标记并写回 descr。
	if err := ps.SetAltText("new alt"); err != nil {
		t.Fatal(err)
	}
	v = slidePics(t, s)[0]
	if v.decorative || v.descr != "new alt" {
		t.Errorf("after alt: dec=%v descr=%q", v.decorative, v.descr)
	}
	// 空文本清除 descr。
	if err := ps.SetAltText(""); err != nil {
		t.Fatal(err)
	}
	if v := slidePics(t, s)[0]; v.descr != "" || v.decorative {
		t.Errorf("after clear: descr=%q dec=%v", v.descr, v.decorative)
	}
}

func TestAddPictureJPEG(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	data := testJPEG(6, 3, 40)
	if _, err := s.AddPicture(context.Background(), BytesMedia(data, "image/jpeg"),
		PictureSpec{Fit: FitStretch, Width: 60000, Height: 30000}); err != nil {
		t.Fatalf("AddPicture: %v", err)
	}
	v := slidePics(t, s)[0]
	rels := slideImageRels(t, s)
	if got := rels[v.embed]; got != "/ppt/media/image1.jpg" {
		t.Errorf("jpeg media = %q", got)
	}
	if v.extCx != 60000 || v.extCy != 30000 {
		t.Errorf("jpeg ext=(%d,%d)", v.extCx, v.extCy)
	}
}

func TestAddPictureDedupSameSlideAndAcross(t *testing.T) {
	p := imageDeckFixture(t, 2)
	slides := deckSlides(t, p)
	data := testPNG(3, 3, 150)
	// 同页加两次：复用媒体 Part 与 rId。
	if _, err := slides[0].AddPicture(context.Background(), BytesMedia(data, ""), PictureSpec{}); err != nil {
		t.Fatal(err)
	}
	if _, err := slides[0].AddPicture(context.Background(), BytesMedia(data, ""), PictureSpec{}); err != nil {
		t.Fatal(err)
	}
	// 跨页再加一次：仍复用媒体 Part。
	if _, err := slides[1].AddPicture(context.Background(), BytesMedia(data, ""), PictureSpec{}); err != nil {
		t.Fatal(err)
	}
	if m := mediaParts(p); len(m) != 1 {
		t.Fatalf("media parts = %v (want 1)", m)
	}
	p1 := slidePics(t, slides[0])
	if len(p1) != 2 || p1[0].embed != p1[1].embed {
		t.Errorf("slide1 embeds not shared: %+v", p1)
	}
	rels1 := slideImageRels(t, slides[0])
	if len(rels1) != 1 {
		t.Errorf("slide1 image rels = %v", rels1)
	}
	rels2 := slideImageRels(t, slides[1])
	if len(rels2) != 1 {
		t.Errorf("slide2 image rels = %v", rels2)
	}
	// 不同内容生成第二个 Part。
	if _, err := slides[1].AddPicture(context.Background(), BytesMedia(testPNG(3, 3, 200), ""), PictureSpec{}); err != nil {
		t.Fatal(err)
	}
	if m := mediaParts(p); len(m) != 2 {
		t.Errorf("media parts after distinct add = %v (want 2)", m)
	}
}

func TestReplaceImageSharedReferenceProtected(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	imgA := testPNG(3, 3, 10)
	imgB := testPNG(3, 3, 20)
	imgC := testPNG(3, 3, 30)

	p1, err := s.AddPicture(context.Background(), BytesMedia(imgA, ""), PictureSpec{})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.AddPicture(context.Background(), BytesMedia(imgA, ""), PictureSpec{})
	if err != nil {
		t.Fatal(err)
	}
	partA := slideImageRels(t, s)[slidePics(t, s)[0].embed]
	if partA == "" {
		t.Fatal("no part A")
	}
	// p1 换图 → 新增 Part B；p2 仍引用 A，A 不得被删。
	if err := p1.ReplaceImage(context.Background(), BytesMedia(imgB, "")); err != nil {
		t.Fatalf("ReplaceImage: %v", err)
	}
	pics := slidePics(t, s)
	if len(pics) != 2 {
		t.Fatalf("pics = %d", len(pics))
	}
	if pics[0].embed == pics[1].embed {
		t.Errorf("embeds should differ after replace")
	}
	rels := slideImageRels(t, s)
	partA2 := rels[pics[1].embed]
	partB := rels[pics[0].embed]
	if partA2 != partA || partB == partA {
		t.Errorf("shared ref broken: A=%s A2=%s B=%s", partA, partA2, partB)
	}
	if _, err := p.partBytes(opc.PartName(partA)); err != nil {
		t.Errorf("shared media A deleted while in use: %v", err)
	}
	// p2 换图且 A 无其它引用 → A 删除；B/C 保留。
	if err := p2.ReplaceImage(context.Background(), BytesMedia(imgC, "")); err != nil {
		t.Fatalf("ReplaceImage 2: %v", err)
	}
	if _, err := p.partBytes(opc.PartName(partA)); !errors.Is(err, ErrNotFound) {
		t.Errorf("orphan media A not deleted: %v", err)
	}
	if m := mediaParts(p); len(m) != 2 {
		t.Errorf("media parts = %v (want B,C)", m)
	}
}

func TestReplaceImageSingleConsumerOrphanCleanup(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	imgA := testPNG(2, 2, 60)
	imgB := testJPEG(4, 2, 70)
	ps, err := s.AddPicture(context.Background(), BytesMedia(imgA, ""), PictureSpec{})
	if err != nil {
		t.Fatal(err)
	}
	partA := slideImageRels(t, s)[slidePics(t, s)[0].embed]
	if err := ps.ReplaceImage(context.Background(), BytesMedia(imgB, "")); err != nil {
		t.Fatalf("ReplaceImage: %v", err)
	}
	if _, err := p.partBytes(opc.PartName(partA)); !errors.Is(err, ErrNotFound) {
		t.Errorf("old media A not removed: %v", err)
	}
	pics := slidePics(t, s)
	rels := slideImageRels(t, s)
	if len(pics) != 1 || len(rels) != 1 {
		t.Fatalf("pics=%d rels=%v", len(pics), rels)
	}
	if got := rels[pics[0].embed]; got != "/ppt/media/image2.jpg" {
		t.Errorf("replaced target = %s", got)
	}
	b, err := p.partBytes(opc.PartName(rels[pics[0].embed]))
	if err != nil || !bytes.Equal(b, imgB) {
		t.Errorf("new media bytes mismatch (err=%v)", err)
	}
}

func TestReplaceImageSameContentNoop(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	data := testPNG(2, 2, 88)
	ps, err := s.AddPicture(context.Background(), BytesMedia(data, ""), PictureSpec{})
	if err != nil {
		t.Fatal(err)
	}
	rev := p.Revision()
	if err := ps.ReplaceImage(context.Background(), BytesMedia(data, "")); err != nil {
		t.Fatalf("ReplaceImage: %v", err)
	}
	if p.Revision() != rev {
		t.Error("revision changed on content-identical replace")
	}
	if m := mediaParts(p); len(m) != 1 {
		t.Errorf("media parts = %v", m)
	}
}

func TestAddPictureErrors(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	// 未知格式（GIF 魔数）。
	if _, err := s.AddPicture(context.Background(), BytesMedia([]byte("GIF89a..."), ""), PictureSpec{}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("gif: err=%v want ErrUnsupportedFormat", err)
	}
	// 随机字节。
	if _, err := s.AddPicture(context.Background(), BytesMedia([]byte("not an image"), ""), PictureSpec{}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("junk: err=%v want ErrUnsupportedFormat", err)
	}
	// 损坏的 PNG（仅魔数）。
	if _, err := s.AddPicture(context.Background(), BytesMedia([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 1, 2}, ""), PictureSpec{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("truncated png: err=%v want ErrInvalidArgument", err)
	}
	// 声明类型与实际不符。
	if _, err := s.AddPicture(context.Background(), BytesMedia(testPNG(1, 1, 1), "image/gif"), PictureSpec{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("declared mismatch: err=%v want ErrInvalidArgument", err)
	}
	// 等价声明（image/jpg ↔ image/jpeg）应通过。
	if _, err := s.AddPicture(context.Background(), BytesMedia(testJPEG(1, 1, 1), "image/jpg"), PictureSpec{}); err != nil {
		t.Errorf("jpg equivalent: %v", err)
	}
	// nil 源。
	if _, err := s.AddPicture(context.Background(), nil, PictureSpec{}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("nil src: err=%v", err)
	}
}

func TestAddPictureClosedDocument(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	ps, err := s.AddPicture(context.Background(), BytesMedia(testPNG(1, 1, 5), ""), PictureSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddPicture(context.Background(), BytesMedia(testPNG(1, 1, 6), ""), PictureSpec{}); !errors.Is(err, ErrClosed) {
		t.Errorf("add after close: %v", err)
	}
	if err := ps.SetAltText("x"); !errors.Is(err, ErrClosed) {
		t.Errorf("set alt after close: %v", err)
	}
	if err := ps.ReplaceImage(context.Background(), BytesMedia(testPNG(1, 1, 7), "")); !errors.Is(err, ErrClosed) {
		t.Errorf("replace after close: %v", err)
	}
}

func TestAddPictureSaveRoundTrip(t *testing.T) {
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	imgA := testPNG(5, 4, 120)
	imgB := testJPEG(3, 2, 55)
	if _, err := s.AddPicture(context.Background(), BytesMedia(imgA, ""), PictureSpec{
		X: 100, Y: 200, Fit: FitStretch, Width: 50000, Height: 40000,
	}); err != nil {
		t.Fatal(err)
	}
	// 再次添加相同 PNG → 去重后仅 1 个 PNG 媒体；再加 JPEG。
	if _, err := s.AddPicture(context.Background(), BytesMedia(imgA, ""), PictureSpec{X: 3000, Y: 0}); err != nil {
		t.Fatal(err)
	}
	sh, err := s.AddPicture(context.Background(), BytesMedia(imgB, ""), PictureSpec{Fit: FitCover, Width: 40000, Height: 20000})
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.SetAltText("round trip alt"); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	report, err := p.Write(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(report.ChangedParts) == 0 {
		t.Error("no changed parts reported")
	}
	// 重开校验：媒体/CT/关系/图片结构一致。
	p2, err := OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer p2.Close()
	vr := p2.Validate(context.Background())
	for _, d := range vr.Diagnostics {
		if d.Severity == SeverityError {
			t.Errorf("reopen validate: %+v", d)
		}
	}
	s2 := deckSlides(t, p2)[0]
	pics := slidePics(t, s2)
	if len(pics) != 3 {
		t.Fatalf("pics after reopen = %d", len(pics))
	}
	if pics[0].extCx != 50000 || pics[0].extCy != 40000 || pics[0].offX != 100 {
		t.Errorf("pic0 geom after reopen = %+v", pics[0])
	}
	if pics[2].srcRect == "" || pics[2].descr != "round trip alt" {
		t.Errorf("pic2 after reopen = %+v", pics[2])
	}
	if m := mediaParts(p2); len(m) != 2 {
		t.Errorf("media parts after reopen = %v", m)
	}
}

func TestFileMediaStagedAtCallTime(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/src.png"
	if err := writeFileBytes(path, testPNG(2, 2, 33)); err != nil {
		t.Fatal(err)
	}
	p := imageDeckFixture(t, 1)
	s := deckSlides(t, p)[0]
	if _, err := s.AddPicture(context.Background(), FileMedia(path), PictureSpec{}); err != nil {
		t.Fatalf("AddPicture: %v", err)
	}
	// 调用后改写源文件不影响已暂存内容（修改事务完成前已固化）。
	if err := writeFileBytes(path, testPNG(9, 9, 44)); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := p.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	p2, err := OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	for _, name := range p2.pk.PartNames() {
		if !strings.HasPrefix(string(name), "/ppt/media/") {
			continue
		}
		rc, err := p2.pk.OpenPart(name)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		if !bytes.Equal(b.Bytes(), testPNG(2, 2, 33)) {
			t.Errorf("media %s does not match staged bytes", name)
		}
	}
}
