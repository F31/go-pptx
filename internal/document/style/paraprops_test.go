package style

import (
	"testing"

	"github.com/F31/go-pptx/v2/internal/ooxmlns"
	"github.com/F31/go-pptx/v2/internal/xmlstore"
)

func mustIndex(t *testing.T, s string) *xmlstore.XMLDocument {
	t.Helper()
	d, err := xmlstore.Index([]byte(s))
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	return d
}

func pr(t *testing.T, inner, attrs string) ParagraphProps {
	t.Helper()
	doc := mustIndex(t, `<a:pPr xmlns:a="`+ooxmlns.DrawingML+`" `+attrs+`>`+inner+`</a:pPr>`)
	return ParseParagraphProps(doc, doc.Root())
}

func TestParseParagraphPropsAttrs(t *testing.T) {
	out := pr(t, "",
		`lvl="1" algn="ctr" indent="-100" marL="200" marR="300" rtl="1" eaLnBrk="true" latinLnBrk="0" hangingPunct="1" fontAlgn="b" defTabSz="400" bogus="x" a:ns="y"`)
	if !out.Specified || out.Level != 1 || out.Align != "ctr" {
		t.Fatalf("basic = %+v", out)
	}
	if out.Indent != -100 || out.MarginLeft != 200 || out.MarginRight != 300 {
		t.Fatalf("indent/margins = %+v", out)
	}
	if !out.RTL || !out.EastAsianLineBreak || out.LatinLineBreak || !out.HangingPunct {
		t.Fatalf("flags = %+v", out)
	}
	if out.FontAlign != "b" || out.DefaultTabSize != 400 {
		t.Fatalf("fontAlgn/defTabSz = %+v", out)
	}
	if len(out.Unknown) != 2 || out.Unknown[0] != "bogus" {
		t.Fatalf("unknown = %v", out.Unknown)
	}
}

func TestParseParagraphPropsSpacingTabs(t *testing.T) {
	out := pr(t,
		`<a:lnSpc><a:spcPct val="90000"/></a:lnSpc>`+
			`<a:spcBef><a:spcPts val="600"/></a:spcBef>`+
			`<a:spcAft><a:spcPct val="100"/></a:spcAft>`+
			`<a:tabLst><a:tab pos="100" algn="l"/><a:other/></a:tabLst>`, "")
	if out.LineSpacing == nil || out.LineSpacing.Kind != "pct" || out.LineSpacing.Value != 90000 {
		t.Fatalf("lineSpacing = %+v", out.LineSpacing)
	}
	if out.SpaceBefore == nil || out.SpaceBefore.Kind != "pts" || out.SpaceBefore.Value != 600 {
		t.Fatalf("spaceBefore = %+v", out.SpaceBefore)
	}
	if out.SpaceAfter == nil || out.SpaceAfter.Kind != "pct" {
		t.Fatalf("spaceAfter = %+v", out.SpaceAfter)
	}
	if len(out.Tabs) != 1 || out.Tabs[0].Position != 100 || out.Tabs[0].Align != "l" {
		t.Fatalf("tabs = %+v", out.Tabs)
	}
}

func TestParseParagraphPropsSpacingEmpty(t *testing.T) {
	out := pr(t, `<a:lnSpc><a:other/></a:lnSpc>`, "")
	if out.LineSpacing != nil {
		t.Fatalf("empty spacing should be nil: %+v", out.LineSpacing)
	}
}

func TestParseParagraphPropsBullets(t *testing.T) {
	char := pr(t, `<a:buChar char="&#8226;"/><a:buFont typeface="Arial"/><a:buSzPct val="80"/>`, "")
	if char.Bullet == nil || char.Bullet.Kind != BulletChar || char.Bullet.Char != "•" ||
		char.Bullet.Font != "Arial" || char.Bullet.SizePct != 80 {
		t.Fatalf("char bullet = %+v", char.Bullet)
	}
	none := pr(t, `<a:buNone/>`, "")
	if none.Bullet == nil || none.Bullet.Kind != BulletNone {
		t.Fatalf("none bullet = %+v", none.Bullet)
	}
	auto := pr(t, `<a:buAutoNum type="arabicPeriod" startAt="3"/><a:buSzPts val="250"/>`, "")
	if auto.Bullet == nil || auto.Bullet.Kind != BulletAutoNum ||
		auto.Bullet.AutoNumType != "arabicPeriod" || auto.Bullet.StartAt != 3 ||
		auto.Bullet.SizePts != 2.5 {
		t.Fatalf("autonum bullet = %+v", auto.Bullet)
	}
	blip := pr(t, `<a:buBlip/>`, "")
	if blip.Bullet == nil || blip.Bullet.Kind != BulletBlip {
		t.Fatalf("blip bullet = %+v", blip.Bullet)
	}
}

func TestParseParagraphPropsUnknownChild(t *testing.T) {
	out := pr(t, `<a:mystery/>`, "")
	if len(out.Unknown) != 1 || out.Unknown[0] != "mystery" {
		t.Fatalf("unknown child = %v", out.Unknown)
	}
}

func TestBulletKindString(t *testing.T) {
	for _, tc := range []struct {
		k    BulletKind
		want string
	}{
		{BulletNone, "none"},
		{BulletChar, "char"},
		{BulletAutoNum, "autonum"},
		{BulletBlip, "blip"},
		{BulletUnknown, "unknown"},
		{BulletKind(99), "unknown"},
	} {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("BulletKind(%d).String() = %q, want %q", tc.k, got, tc.want)
		}
	}
}
