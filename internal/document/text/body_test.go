package text

import (
	"errors"
	"strings"
	"testing"

	"github.com/F31/go-pptx/v2/internal/document/model"
	"github.com/F31/go-pptx/v2/internal/document/style"
	"github.com/F31/go-pptx/v2/internal/errs"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

func TestNSPrefix(t *testing.T) {
	if got := NSPrefix(&xmlstore.NodeRecord{QName: xmlstore.QName{Prefix: "p", Local: "sld"}}); got != "p" {
		t.Errorf("NSPrefix(p:sld) = %q, want p", got)
	}
	if got := NSPrefix(&xmlstore.NodeRecord{QName: xmlstore.QName{Local: "bodyPr"}}); got != "a" {
		t.Errorf("NSPrefix(bodyPr) = %q, want fallback a", got)
	}
}

func TestBodyPropsAnySetVertAllowed(t *testing.T) {
	if BodyPropsAnySet(BodyProps{}) {
		t.Error("empty BodyProps should not be set")
	}
	if !BodyPropsAnySet(BodyProps{Vertical: model.NewOptional("vert")}) {
		t.Error("vertical-props should be set")
	}
	if !VertAllowed("") || !VertAllowed("horz") || VertAllowed("bogus") {
		t.Error("VertAllowed mismatch")
	}
}

func TestParseBodyProps(t *testing.T) {
	doc := idx(t, `<a:bodyPr xmlns:a="`+dd+`" numCol="2" vert="vert" anchorCtr="1"/>`)
	p := ParseBodyProps(doc, doc.Root())
	if !p.Columns.Set || p.Columns.Value != 2 {
		t.Errorf("Columns = %+v", p.Columns)
	}
	if !p.Vertical.Set || p.Vertical.Value != "vert" {
		t.Errorf("Vertical = %+v", p.Vertical)
	}
	if !p.AnchorCenter.Set || !p.AnchorCenter.Value {
		t.Errorf("AnchorCenter = %+v", p.AnchorCenter)
	}
	// anchorCtr="true" 也识别为真。
	doc2 := idx(t, `<a:bodyPr xmlns:a="`+dd+`" anchorCtr="true"/>`)
	if p := ParseBodyProps(doc2, doc2.Root()); !p.AnchorCenter.Value {
		t.Errorf("anchorCtr=true not recognized: %+v", p.AnchorCenter)
	}
}

func TestApplyBodyPropsPatch(t *testing.T) {
	doc := idx(t, `<a:bodyPr xmlns:a="`+dd+`" numCol="2" vert="horz" anchorCtr="0"/>`)
	patches, err := ApplyBodyPropsPatch(doc, doc.Root(), "a", BodyProps{
		Columns:      model.NewOptional(0),      // 删除
		Vertical:     model.NewOptional("vert"), // 改值
		AnchorCenter: model.NewOptional(true),   // 改值
	})
	if err != nil || len(patches) != 3 {
		t.Fatalf("patches=%d err=%v", len(patches), err)
	}
	// 值为空/未变 → 无补丁。
	patches, err = ApplyBodyPropsPatch(doc, doc.Root(), "a", BodyProps{
		Vertical:     model.NewOptional(""),    // 删除
		AnchorCenter: model.NewOptional(false), // 与 "0" 等价 → 无补丁
	})
	if err != nil || len(patches) != 1 {
		t.Fatalf("delete vert: patches=%d err=%v", len(patches), err)
	}
	// Columns>=1 → 写值。
	doc2 := idx(t, `<a:bodyPr xmlns:a="`+dd+`"/>`)
	patches, err = ApplyBodyPropsPatch(doc2, doc2.Root(), "a", BodyProps{Columns: model.NewOptional(3)})
	if err != nil || len(patches) != 1 {
		t.Fatalf("set numCol: patches=%d err=%v", len(patches), err)
	}
}

func TestSetAttrPatchAndRemove(t *testing.T) {
	doc := idx(t, `<a:bodyPr xmlns:a="`+dd+`" numCol="2"/>`)
	bp := doc.Root()
	if p, err := SetAttrPatch(bp, "numCol", "2", "a"); err != nil || p != nil {
		t.Errorf("same: p=%v err=%v", p, err)
	}
	if p, err := SetAttrPatch(bp, "numCol", "3", "a"); err != nil || p == nil {
		t.Errorf("changed: p=%v err=%v", p, err)
	}
	if p, err := SetAttrPatch(bp, "vert", "vert", "a"); err != nil || p == nil {
		t.Errorf("insert: p=%v err=%v", p, err)
	}
	if p := RemoveAttrIfExists(doc, bp, "numCol"); p == nil {
		t.Error("remove existing should return patch")
	}
	if p := RemoveAttrIfExists(doc, bp, "missing"); p != nil {
		t.Error("remove missing should return nil")
	}
}

func TestBuildBodyPrFragment(t *testing.T) {
	frag, err := BuildBodyPrFragment("a", BodyProps{
		Columns:      model.NewOptional(2),
		Vertical:     model.NewOptional("vert"),
		AnchorCenter: model.NewOptional(true),
	})
	if err != nil {
		t.Fatalf("BuildBodyPrFragment: %v", err)
	}
	for _, want := range []string{`numCol="2"`, `vert="vert"`, `anchorCtr="1"`, "/>"} {
		if !strings.Contains(frag, want) {
			t.Errorf("frag missing %q: %s", want, frag)
		}
	}
}

func TestValidateFieldSpec(t *testing.T) {
	if err := ValidateFieldSpec(FieldSpec{Kind: FieldSlideNumber}); err != nil {
		t.Errorf("slidenum: %v", err)
	}
	if err := ValidateFieldSpec(FieldSpec{Kind: FieldSlideNumber, Guide: "X"}); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("slidenum guide err = %v", err)
	}
	if err := ValidateFieldSpec(FieldSpec{Kind: FieldDateTime, Guide: "YYYY-MM-DD"}); err != nil {
		t.Errorf("datetime: %v", err)
	}
	if err := ValidateFieldSpec(FieldSpec{Kind: FieldDateTime, Guide: "bogus"}); !errors.Is(err, errs.ErrInvalidArgument) {
		t.Errorf("datetime guide err = %v", err)
	}
	if err := ValidateFieldSpec(FieldSpec{Kind: "bad"}); !errors.Is(err, errs.ErrUnsupportedEdit) {
		t.Errorf("unknown kind err = %v", err)
	}
	if err := ValidateFieldSpec(FieldSpec{Kind: FieldSlideNumber, Text: "\x00"}); err == nil {
		t.Error("invalid xml char should error")
	}
}

func TestBuildFieldFragment(t *testing.T) {
	doc := idx(t, `<a:p xmlns:a="`+dd+`"><a:r/></a:p>`)
	frag, err := BuildFieldFragment(doc, doc.Root(), FieldSpec{
		Kind:  FieldDateTime,
		Guide: "YYYY-MM-DD",
		Text:  "2020",
		Style: style.FontStyle{Bold: model.NewOptional(true)},
	})
	if err != nil {
		t.Fatalf("BuildFieldFragment: %v", err)
	}
	for _, want := range []string{`type="datetime"`, `fldGuide="YYYY-MM-DD"`, `b="1"`, "</a:fld>"} {
		if !strings.Contains(frag, want) {
			t.Errorf("frag missing %q: %s", want, frag)
		}
	}
	frag2, err := BuildFieldFragment(doc, doc.Root(), FieldSpec{Kind: FieldSlideNumber, Text: "5"})
	if err != nil {
		t.Fatalf("BuildFieldFragment slidenum: %v", err)
	}
	if strings.Contains(frag2, "fldGuide") {
		t.Errorf("slidenum should have no guide: %s", frag2)
	}
}

func TestBodyRawShapeOK(t *testing.T) {
	ok := idx(t, `<a:txBody xmlns:a="`+dd+`"><a:bodyPr/><a:p/></a:txBody>`)
	if !BodyRawShapeOK(ok, ok.Root()) {
		t.Error("valid body should be OK")
	}
	bad := idx(t, `<a:txBody xmlns:a="`+dd+`" xmlns:x="urn:x"><a:bodyPr/><x:foo/></a:txBody>`)
	if BodyRawShapeOK(bad, bad.Root()) {
		t.Error("unknown child should fail")
	}
}

func TestParaRunPrefix(t *testing.T) {
	body := idx(t, `<a:txBody xmlns:a="`+dd+`"><a:p/></a:txBody>`)
	if got := ParaPrefix(body, body.Root()); got != "a" {
		t.Errorf("ParaPrefix = %q", got)
	}
	empty := idx(t, `<a:txBody xmlns:a="`+dd+`"/>`)
	if got := ParaPrefix(empty, empty.Root()); got != "a" {
		t.Errorf("ParaPrefix fallback = %q", got)
	}
	para := idx(t, `<a:p xmlns:a="`+dd+`"><a:r/></a:p>`)
	if got := RunPrefix(para, para.Root()); got != "a" {
		t.Errorf("RunPrefix = %q", got)
	}
	paraEmpty := idx(t, `<a:p xmlns:a="`+dd+`"/>`)
	if got := RunPrefix(paraEmpty, paraEmpty.Root()); got != "a" {
		t.Errorf("RunPrefix fallback = %q", got)
	}
}

func TestBuildPlainParagraphAndEndParaAnchor(t *testing.T) {
	if got := BuildPlainParagraph("a", "hi"); !strings.Contains(got, "<a:t>hi</a:t>") {
		t.Errorf("BuildPlainParagraph = %q", got)
	}
	if got := BuildPlainParagraph("a", "\x00"); got != "" {
		t.Errorf("invalid char should yield empty, got %q", got)
	}
	para := idx(t, `<a:p xmlns:a="`+dd+`"><a:r/><a:endParaRPr/></a:p>`)
	if n, i := EndParaAnchor(para, para.Root()); n == nil || i != 1 {
		t.Errorf("EndParaAnchor = %v,%d", n, i)
	}
	plain := idx(t, `<a:p xmlns:a="`+dd+`"><a:r/></a:p>`)
	if n, i := EndParaAnchor(plain, plain.Root()); n != nil || i != -1 {
		t.Errorf("EndParaAnchor none = %v,%d", n, i)
	}
}

func TestParagraphText(t *testing.T) {
	doc := idx(t, `<a:p xmlns:a="`+dd+`">`+
		`<a:r><a:t>hi</a:t></a:r>`+
		`<a:fld><a:t>f</a:t></a:fld>`+
		`<a:r><a:t/></a:r>`+
		`<a:br/>`+
		`</a:p>`)
	if got := ParagraphText(doc, doc.Root()); got != "hif" {
		t.Errorf("ParagraphText = %q, want hif", got)
	}
}
