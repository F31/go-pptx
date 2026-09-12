package xmlstore

import (
	"errors"
	"strings"
	"testing"
)

// scanAll 扫描全部 token 并按类型收集"事件"（忽略纯空白文本）。
func scanAll(t *testing.T, doc string) ([]string, error) {
	t.Helper()
	s, err := NewScanner([]byte(doc))
	if err != nil {
		return nil, err
	}
	var events []string
	for s.Next() {
		tok := s.Token()
		switch tok.Kind {
		case TokenStart:
			name := tok.RawName
			if tok.SelfClosing {
				name += "/"
			}
			events = append(events, "S:"+name)
		case TokenEnd:
			events = append(events, "E:"+tok.RawName)
		case TokenText:
			if strings.TrimSpace(tok.Text) != "" {
				events = append(events, "T:"+tok.Text)
			}
		case TokenComment:
			events = append(events, "C:"+tok.Text)
		case TokenPI:
			events = append(events, "P:"+tok.Text)
		case TokenCDATA:
			events = append(events, "CD:"+tok.Text)
		case TokenDoctype:
			events = append(events, "D:"+tok.Text)
		}
	}
	return events, s.Err()
}

func mustEvents(t *testing.T, doc string) []string {
	t.Helper()
	events, err := scanAll(t, doc)
	if err != nil {
		t.Fatalf("scan %q: %v", doc, err)
	}
	return events
}

func TestScanPresentationSlice(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:p="urn:ooxml:main" xmlns:a="urn:ooxml:drawing">
  <p:sldIdLst><p:sldId id="256"/></p:sldIdLst>
  <!-- keep <raw> inside comment -->
  <p:body>text &amp; more &#x4E2D;</p:body>
</p:presentation>`

	events := mustEvents(t, doc)
	want := []string{
		"S:p:presentation",
		"S:p:sldIdLst",
		"S:p:sldId/",
		"E:p:sldIdLst",
		"C: keep <raw> inside comment ",
		"S:p:body",
		"T:text & more 中",
		"E:p:body",
		"E:p:presentation",
	}
	// PI 是否出现取决于首个事件，单独校验（事件 0 应为 PI）。
	if len(events) == 0 || !strings.HasPrefix(events[0], "P:") {
		t.Fatalf("first event = %v, want PI; events=%v", events, events)
	}
	rest := events[1:]
	if len(rest) != len(want) {
		t.Fatalf("events = %v\nwant    %v", rest, want)
	}
	for i := range want {
		if rest[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q", i, rest[i], want[i])
		}
	}
}

func TestStartTagAttrsAndSpans(t *testing.T) {
	doc := `<p:sldIdLst xmlns:p="urn:main"><p:sldId id="256" other='x&amp;y'/></p:sldIdLst>`
	s, err := NewScanner([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var found *Token
	for s.Next() {
		tok := s.Token()
		if tok.Kind == TokenStart && tok.RawName == "p:sldId" {
			tok := tok
			found = &tok
			break
		}
	}
	if s.Err() != nil {
		t.Fatalf("scan: %v", s.Err())
	}
	if found == nil {
		t.Fatal("p:sldId token not found")
	}
	if !found.SelfClosing {
		t.Error("SelfClosing = false")
	}
	if len(found.Attrs) != 2 {
		t.Fatalf("attrs = %v, want 2", found.Attrs)
	}
	var a *Attr
	for i := range found.Attrs {
		if found.Attrs[i].RawName == "id" {
			a = &found.Attrs[i]
		}
	}
	if a == nil {
		t.Fatal("attr id not found")
	}
	if a.Value != "256" {
		t.Fatalf("attr = %+v", a)
	}
	if got := doc[a.ValueStart:a.ValueEnd]; got != "256" {
		t.Errorf("span value = %q, want 256", got)
	}
	if got := doc[a.NameStart:a.NameEnd]; got != "id" {
		t.Errorf("span name = %q, want id", got)
	}
	// 另一个属性的实体解码：other='x&amp;y' → x&y。
	if other := found.Attrs[1]; other.RawName != "other" || other.Value != "x&y" {
		t.Errorf("other attr = %+v, want x&y", other)
	}

	// xmlns 声明出现在 presentation token 上（带前缀的 attribute）。
	s2, _ := NewScanner([]byte(doc))
	nsOK := false
	for s2.Next() {
		tok := s2.Token()
		if tok.Kind == TokenStart && tok.RawName == "p:sldIdLst" {
			for _, at := range tok.Attrs {
				if at.RawName == "xmlns:p" && at.Value == "urn:main" {
					nsOK = true
				}
			}
		}
	}
	if !nsOK {
		t.Error("xmlns:p declaration not captured")
	}
}

func TestScanCommentCDATA(t *testing.T) {
	events := mustEvents(t, `<a><!-- c --><![CDATA[raw <x> & not-markup]]></a>`)
	want := []string{"S:a", "C: c ", "CD:raw <x> & not-markup", "E:a"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q", i, events[i], want[i])
		}
	}
}

func TestScanAttrEntities(t *testing.T) {
	doc := `<e a='it&apos;s &quot;q&quot;' b="x"/>`
	s, _ := NewScanner([]byte(doc))
	var a *Attr
	for s.Next() {
		tok := s.Token()
		if tok.Kind == TokenStart && tok.RawName == "e" {
			for i := range tok.Attrs {
				if tok.Attrs[i].RawName == "a" {
					a = &tok.Attrs[i]
				}
			}
		}
	}
	if s.Err() != nil {
		t.Fatalf("scan: %v", s.Err())
	}
	if a == nil || a.Value != `it's "q"` {
		t.Fatalf("attr value = %+v", a)
	}
}

func TestScanUnknownEntityPreserved(t *testing.T) {
	events := mustEvents(t, `<a>&nbsp;</a>`)
	if len(events) != 3 || events[0] != "S:a" || events[1] != "T:&nbsp;" || events[2] != "E:a" {
		t.Fatalf("events = %v", events)
	}
}

func TestScanDoctypeSkipped(t *testing.T) {
	events := mustEvents(t, `<!DOCTYPE a [<!ENTITY x "y">]><a/>`)
	want := []string{"D:DOCTYPE a [<!ENTITY x \"y\">]", "S:a/"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q", i, events[i], want[i])
		}
	}
}

func TestScannerMismatchedEndTag(t *testing.T) {
	for _, doc := range []string{"<a><b></a>", "<a></b></a>", "</a>", "<a><b>"} {
		_, err := scanAll(t, doc)
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("doc %q: err = %v, want ErrMalformed", doc, err)
		}
	}
}

func TestScannerUnterminated(t *testing.T) {
	for _, doc := range []string{"<a", "<a x='1", "<!-- no close", "<![CDATA[no close", "<?pi"} {
		_, err := scanAll(t, doc)
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("doc %q: err = %v, want ErrMalformed", doc, err)
		}
	}
}

func TestScannerOffsetReported(t *testing.T) {
	doc := "<root>\n  <a></root>"
	s, _ := NewScanner([]byte(doc))
	for s.Next() {
	}
	if s.Err() == nil {
		t.Fatal("expected error")
	}
	var se *SyntaxError
	if !errors.As(s.Err(), &se) {
		t.Fatalf("err type = %T, want *SyntaxError", s.Err())
	}
	if se.Offset <= 0 || se.Offset > len(doc) {
		t.Errorf("offset = %d out of range", se.Offset)
	}
}

func TestScannerRejectsNonUTF8(t *testing.T) {
	if _, err := NewScanner([]byte{0xff, 0xfe, '<', 'a'}); !errors.Is(err, ErrEncoding) {
		t.Fatalf("err = %v, want ErrEncoding", err)
	}
	// 合法中文 UTF-8 应通过。
	if _, err := NewScanner([]byte("<p>中文</p>")); err != nil {
		t.Fatalf("valid utf-8 rejected: %v", err)
	}
}

func TestScannerEmptyAndSimple(t *testing.T) {
	if _, err := NewScanner([]byte("")); err != nil {
		t.Fatalf("empty input: %v", err)
	}
	events := mustEvents(t, "<a><b/><c>hi</c></a>")
	want := []string{"S:a", "S:b/", "S:c", "T:hi", "E:c", "E:a"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q", i, events[i], want[i])
		}
	}
}

func TestScannerAccessorsAndTokenKindStrings(t *testing.T) {
	want := map[TokenKind]string{
		TokenStart:    "Start",
		TokenEnd:      "End",
		TokenText:     "Text",
		TokenComment:  "Comment",
		TokenPI:       "PI",
		TokenCDATA:    "CDATA",
		TokenDoctype:  "Doctype",
		TokenKind(99): "Unknown",
	}
	for kind, str := range want {
		if got := kind.String(); got != str {
			t.Fatalf("TokenKind(%d).String = %q, want %q", kind, got, str)
		}
	}
	s, err := NewScanner([]byte(`<a><b/></a>`))
	if err != nil {
		t.Fatal(err)
	}
	if s.ErrOffset() != -1 || s.Depth() != 0 {
		t.Fatalf("initial offset/depth = %d/%d", s.ErrOffset(), s.Depth())
	}
	if !s.Next() || s.Token().RawName != "a" || s.Depth() != 1 {
		t.Fatalf("after root start token=%+v depth=%d", s.Token(), s.Depth())
	}
	if !s.Next() || s.Token().RawName != "b" || !s.Token().SelfClosing || s.Depth() != 1 {
		t.Fatalf("after self-closing token=%+v depth=%d", s.Token(), s.Depth())
	}
	for s.Next() {
	}
	if s.Err() != nil || s.ErrOffset() != -1 || s.Depth() != 0 {
		t.Fatalf("final err=%v offset=%d depth=%d", s.Err(), s.ErrOffset(), s.Depth())
	}
}

func TestSyntaxErrorFormatting(t *testing.T) {
	s, err := NewScanner([]byte(`<a><b></a>`))
	if err != nil {
		t.Fatal(err)
	}
	for s.Next() {
	}
	var se *SyntaxError
	if !errors.As(s.Err(), &se) {
		t.Fatalf("err = %v, want SyntaxError", s.Err())
	}
	if s.ErrOffset() != se.Offset {
		t.Fatalf("ErrOffset = %d, want %d", s.ErrOffset(), se.Offset)
	}
	if !strings.Contains(se.Error(), "byte offset") {
		t.Fatalf("SyntaxError string = %q", se.Error())
	}
}
