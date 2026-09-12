package opc

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const ctNS = NsContentTypes
const relNS = NsRelationships

// miniContentTypes 构造最小 Content Types 流。
func miniContentTypes(overrides string) string {
	return `<Types xmlns="` + ctNS + `">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		overrides + `</Types>`
}

// TestContentTypesOverridePrecedence 是 Override 优先于 Default 的金样
// 断言（OPC-02 验收：Override 金样通过）。
func TestContentTypesOverridePrecedence(t *testing.T) {
	ct, err := ParseContentTypes([]byte(miniContentTypes(
		`<Override PartName="/ppt/deck.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`)))
	if err != nil {
		t.Fatalf("ParseContentTypes: %v", err)
	}
	// Override 命中优先于 xml 扩展名的 Default。
	got, ok := ct.Lookup("/ppt/deck.xml")
	if !ok || got != "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml" {
		t.Fatalf("Lookup = %q, %v; want presentation main type", got, ok)
	}
	// 无 Override 的 XML Part 落到 Default。
	got, ok = ct.Lookup("/ppt/slides/slide1.xml")
	if !ok || got != "application/xml" {
		t.Fatalf("default lookup = %q, %v", got, ok)
	}
	// 扩展名大小写不敏感（.XML 命中 Default 的 xml）；未知扩展名未命中。
	if _, ok := ct.Lookup("/ppt/slides/slide1.XML"); !ok {
		t.Error("case-insensitive extension lookup failed")
	}
	if _, ok := ct.Lookup("/ppt/media/blob.bin"); ok {
		t.Error("unknown extension should not resolve")
	}
	// 无扩展名 Part（如 some packages 的根流）不命中 Default。
	if _, ok := ct.Lookup("/weird"); ok {
		t.Error("extensionless part should not resolve via Default")
	}
}

func TestContentTypesMalformed(t *testing.T) {
	cases := map[string]string{
		"dup override": miniContentTypes(
			`<Override PartName="/a.xml" ContentType="t1"/><Override PartName="/a.xml" ContentType="t2"/>`),
		"dup default": `<Types xmlns="` + ctNS + `"><Default Extension="xml" ContentType="t1"/><Default Extension="XML" ContentType="t2"/></Types>`,
		"bad part":    miniContentTypes(`<Override PartName="a.xml" ContentType="t"/>`),
		"bad root":    `<Wrong xmlns="` + ctNS + `"/>`,
		"bad ns":      `<Types xmlns="urn:wrong"/>`,
	}
	for name, data := range cases {
		if _, err := ParseContentTypes([]byte(data)); !errors.Is(err, ErrMalformedPackage) {
			t.Errorf("%s: err = %v, want ErrMalformedPackage", name, err)
		}
	}
}

// miniRels 构造最小关系流。
func miniRels(body string) string {
	return `<Relationships xmlns="` + relNS + `">` + body + `</Relationships>`
}

func rel(id, typ, target string) string {
	return `<Relationship Id="` + id + `" Type="` + typ + `" Target="` + target + `"/>`
}

func TestRelationshipsParseAndResolve(t *testing.T) {
	src := PartName("/ppt/deck.xml")
	set, err := ParseRelationships(src, []byte(miniRels(
		rel("rId1", RelSlide, "slides/slide1.xml")+
			rel("rId2", RelSlideMaster, "../slideMasters/slideMaster1.xml")+
			rel("rId3", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink", "https://example.com/")+
			rel("rId4", RelTheme, "/ppt/theme/theme1.xml")+
			`<Relationship Id="rId5" Type="`+RelSlide+`" Target="a%20b.xml"/>`)))
	if err != nil {
		t.Fatalf("ParseRelationships: %v", err)
	}
	if set.Len() != 5 {
		t.Fatalf("Len = %d, want 5", set.Len())
	}
	if set.Source != src {
		t.Errorf("Source = %s, want %s", set.Source, src)
	}

	// 相对目标：基于源 Part 所在目录。
	r1, _ := set.ByID("rId1")
	if r1.TargetPart != "/ppt/slides/slide1.xml" {
		t.Errorf("rId1 = %s, want /ppt/slides/slide1.xml", r1.TargetPart)
	}
	if r1.Mode != TargetInternal {
		t.Errorf("rId1 default mode = %s, want Internal", r1.Mode)
	}
	// ".." 段上跳：/ppt/deck.xml 的目录是 /ppt/，上跳一级到包根。
	r2, _ := set.ByID("rId2")
	if r2.TargetPart != "/slideMasters/slideMaster1.xml" {
		t.Errorf("rId2 = %s", r2.TargetPart)
	}
	// 外部目标：原样保留，不解析为 Part。
	r3, _ := set.ByID("rId3")
	if r3.Mode != TargetExternal || r3.Target != "https://example.com/" || r3.TargetPart != "" {
		t.Errorf("rId3 = %+v", r3)
	}
	// 绝对目标（以 "/" 开头）。
	r4, _ := set.ByID("rId4")
	if r4.TargetPart != "/ppt/theme/theme1.xml" {
		t.Errorf("rId4 = %s", r4.TargetPart)
	}
	// 百分号解码。
	r5, _ := set.ByID("rId5")
	if r5.TargetPart != "/ppt/a b.xml" {
		t.Errorf("rId5 = %s, want /ppt/a b.xml", r5.TargetPart)
	}
}

func TestRelationshipsMalformed(t *testing.T) {
	src := PartName("/ppt/deck.xml")
	cases := map[string]struct {
		data string
		want error
	}{
		"escape root":  {miniRels(rel("rId1", RelSlide, "../../out.xml")), ErrMalformedPackage},
		"dup id":       {miniRels(rel("rId1", RelSlide, "a.xml") + rel("rId1", RelTheme, "b.xml")), ErrMalformedPackage},
		"bad mode":     {miniRels(`<Relationship Id="rId1" Type="t" Target="http://x/" TargetMode="Weird"/>`), ErrMalformedPackage},
		"missing attr": {miniRels(`<Relationship Id="rId1" Target="a.xml"/>`), ErrMalformedPackage},
		"bad absolute": {miniRels(rel("rId1", RelSlide, "/../x.xml")), ErrMalformedPackage},
		"bad root ns":  {`<Relationships xmlns="urn:x"/>`, ErrMalformedPackage},
		"bad percent":  {miniRels(rel("rId1", RelSlide, "a%zz.xml")), ErrMalformedPackage},
	}
	for name, tc := range cases {
		_, err := ParseRelationships(src, []byte(tc.data))
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

// TestRelationshipsPercentEscapes covers all three valid hexVal branches
// (digits, lowercase a-f, uppercase A-F) via unescapePercent, which is
// invoked during ParseRelationships → resolveTarget. The "bad percent" case
// in TestRelationshipsMalformed covers the invalid branch; this test covers
// the three success branches and the "incomplete escape" branch.
//
// Note: Relationship.Target retains the raw attribute value; unescapePercent
// is applied to TargetPart (the package-resolved PartName).
func TestRelationshipsPercentEscapes(t *testing.T) {
	src := PartName("/ppt/deck.xml")
	cases := []struct {
		name string
		data string
		want string
	}{
		{"digits", miniRels(rel("rId1", RelSlide, "file%20name.xml")), "/ppt/file name.xml"},
		{"lower", miniRels(rel("rId1", RelSlide, "file%ab.xml")), "/ppt/file\xab.xml"},
		{"upper", miniRels(rel("rId1", RelSlide, "file%AB.xml")), "/ppt/file\xab.xml"},
		{"mixed", miniRels(rel("rId1", RelSlide, "a%2Bb%3Dc.xml")), "/ppt/a+b=c.xml"},
	}
	for _, tc := range cases {
		set, err := ParseRelationships(src, []byte(tc.data))
		if err != nil {
			t.Errorf("%s: ParseRelationships: %v", tc.name, err)
			continue
		}
		if len(set.All()) != 1 {
			t.Errorf("%s: %d rels", tc.name, len(set.All()))
			continue
		}
		got := string(set.All()[0].TargetPart)
		if got != tc.want {
			t.Errorf("%s: TargetPart = %q, want %q", tc.name, got, tc.want)
		}
	}
	// Incomplete escape (% at end with no hex digits after) → wrapped
	// ErrMalformedPackage via ParseRelationships.
	if _, err := ParseRelationships(src, []byte(miniRels(rel("rId1", RelSlide, "abc%")))); !errors.Is(err, ErrMalformedPackage) {
		t.Errorf("incomplete percent escape err = %v, want ErrMalformedPackage", err)
	}
}

// loadMiniPackage 构造含循环关系（版式 ↔ 母版）与非固定名称主 Part
// （ppt/deck.xml）的迷你包，验证 Load / MainPart / Walk（OPC-02 验收：
// 非固定名称、循环关系）。
func loadMiniPackage(t *testing.T) *Package {
	t.Helper()
	entries := map[string]string{
		"[Content_Types].xml": miniContentTypes(
			`<Override PartName="/ppt/deck.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>` +
				`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`),
		"_rels/.rels":  miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml")),
		"ppt/deck.xml": `<p:presentation xmlns:p="urn:p"/>`,
		"ppt/_rels/deck.xml.rels": miniRels(
			rel("rId1", RelSlide, "slides/slide1.xml") +
				rel("rId2", RelSlideMaster, "slideMasters/slideMaster1.xml") +
				rel("rId3", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink", "https://example.com/")),
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="urn:p"><p:t>原始</p:t></p:sld>`,
		"ppt/slides/_rels/slide1.xml.rels": miniRels(
			rel("rId1", RelSlideLayout, "../slideLayouts/slideLayout1.xml")),
		"ppt/slideLayouts/slideLayout1.xml": `<p:sldLayout xmlns:p="urn:p"/>`,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": miniRels(
			rel("rId1", RelSlideMaster, "../slideMasters/slideMaster1.xml")),
		"ppt/slideMasters/slideMaster1.xml": `<p:sldMaster xmlns:p="urn:p"/>`,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": miniRels(
			rel("rId1", RelSlideLayout, "../slideLayouts/slideLayout1.xml")),
	}
	data := zipBytes(t, entries)
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return pk
}

func TestPackageMainPartNonFixedName(t *testing.T) {
	pk := loadMiniPackage(t)
	main, err := pk.MainPart()
	if err != nil {
		t.Fatalf("MainPart: %v", err)
	}
	// 主 Part 名称非固定（这里故意不是 presentation.xml）。
	if main != "/ppt/deck.xml" {
		t.Fatalf("MainPart = %s, want /ppt/deck.xml", main)
	}
	// 内容类型走 Override 金字塔。
	if ct, ok := pk.ContentType(main); !ok ||
		ct != "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml" {
		t.Errorf("ContentType(main) = %q, %v", ct, ok)
	}
}

func TestPackageRelsAccessors(t *testing.T) {
	pk := loadMiniPackage(t)
	main := PartName("/ppt/deck.xml")

	slides := pk.RelatedParts(main, RelSlide)
	if len(slides) != 1 || slides[0] != "/ppt/slides/slide1.xml" {
		t.Fatalf("slides = %v", slides)
	}
	masters := pk.RelatedParts(main, RelSlideMaster)
	if len(masters) != 1 || masters[0] != "/ppt/slideMasters/slideMaster1.xml" {
		t.Fatalf("masters = %v", masters)
	}
	// rId 映射。
	byID := pk.RelatedByIDs(main, RelSlide)
	if len(byID) != 1 || byID[0] != [2]string{"rId1", "/ppt/slides/slide1.xml"} {
		t.Fatalf("RelatedByIDs = %v", byID)
	}
	// 无关系流的 Part 返回空集合，不是错误。
	set, ok := pk.Relationships("/ppt/deck.xml")
	if !ok || set.Len() != 3 {
		t.Fatalf("deck rels = %v, %v", set, ok)
	}
	if set, ok := pk.Relationships("/ppt/unknown.xml"); ok || set != nil {
		t.Errorf("unknown part rels = %v, %v", set, ok)
	}
	// 包根关系。
	rootSet, ok := pk.Relationships("/")
	if !ok || rootSet.Len() != 1 {
		t.Fatalf("root rels = %v, %v", rootSet, ok)
	}
}

func TestPackageWalkCycleSafe(t *testing.T) {
	pk := loadMiniPackage(t)

	visited := make(map[PartName]int)
	visits := 0
	err := pk.Walk("/ppt/deck.xml", func(src PartName, rel *Relationship) error {
		visits++
		visited[PartName(src)]++
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	// 版式 ↔ 母版互指成环：visit 恰好对每条关系执行一次（6 条：
	// deck3 + slide1 + layout1 + master1），且遍历终止。
	const wantVisits = 6
	if visits != wantVisits {
		t.Fatalf("visits = %d, want %d", visits, wantVisits)
	}
	if got := visited["/ppt/slideLayouts/slideLayout1.xml"]; got != 1 {
		t.Errorf("layout expanded %d times, want 1 (cycle guard)", got)
	}
	// 外部关系出现在遍历中但不展开（无崩溃即通过）。
	deckRels, _ := pk.Relationships("/ppt/deck.xml")
	foundExternal := false
	for _, r := range deckRels.All() {
		if r.Mode == TargetExternal {
			foundExternal = true
		}
	}
	if !foundExternal {
		t.Error("external relationship missing in deck rels")
	}

	// visit 返回错误时中止并透传。
	sentinel := errors.New("stop")
	if err := pk.Walk("/ppt/deck.xml", func(src PartName, rel *Relationship) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Errorf("visit error = %v, want sentinel", err)
	}
}

func TestPackageMalformedCases(t *testing.T) {
	t.Run("missing content types", func(t *testing.T) {
		entries := map[string]string{
			"_rels/.rels":  miniRels(rel("rId1", RelOfficeDocument, "ppt/deck.xml")),
			"ppt/deck.xml": `<p/>`,
		}
		mustFailLoad(t, entries, ErrMalformedPackage)
	})
	t.Run("missing root rels", func(t *testing.T) {
		entries := map[string]string{
			"[Content_Types].xml": miniContentTypes(""),
			"ppt/deck.xml":        `<p/>`,
		}
		mustFailLoad(t, entries, ErrMalformedPackage)
	})
	t.Run("officeDocument target missing", func(t *testing.T) {
		entries := map[string]string{
			"[Content_Types].xml": miniContentTypes(""),
			"_rels/.rels":         miniRels(rel("rId1", RelOfficeDocument, "ppt/gone.xml")),
		}
		// Load 本身只做结构解析；目标缺失在 MainPart 发现时报错。
		pk := mustLoad(t, entries)
		if _, err := pk.MainPart(); !errors.Is(err, ErrMalformedPackage) {
			t.Fatalf("MainPart err = %v, want ErrMalformedPackage", err)
		}
	})
	t.Run("external officeDocument rejected", func(t *testing.T) {
		entries := map[string]string{
			"[Content_Types].xml": miniContentTypes(""),
			"_rels/.rels": miniRels(`<Relationship Id="rId1" Type="` + RelOfficeDocument +
				`" Target="https://example.com/p.pptx" TargetMode="External"/>`),
		}
		pk := mustLoad(t, entries)
		if _, err := pk.MainPart(); !errors.Is(err, ErrMalformedPackage) {
			t.Fatalf("MainPart err = %v, want ErrMalformedPackage", err)
		}
	})
	t.Run("no officeDocument", func(t *testing.T) {
		entries := map[string]string{
			"[Content_Types].xml": miniContentTypes(""),
			"_rels/.rels":         miniRels(rel("rId1", RelSlide, "ppt/deck.xml")),
			"ppt/deck.xml":        `<p/>`,
		}
		pk := mustLoad(t, entries)
		_, err := pk.MainPart()
		if err == nil || !strings.Contains(err.Error(), "officeDocument") {
			t.Fatalf("MainPart err = %v, want officeDocument failure", err)
		}
	})
}

func mustLoad(t *testing.T, entries map[string]string) *Package {
	t.Helper()
	data := zipBytes(t, entries)
	pk, err := Load(bytes.NewReader(data), int64(len(data)), Budget{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return pk
}

func mustFailLoad(t *testing.T, entries map[string]string, want error) {
	t.Helper()
	data := zipBytes(t, entries)
	if _, err := Load(bytes.NewReader(data), int64(len(data)), Budget{}); !errors.Is(err, want) {
		t.Fatalf("Load err = %v, want %v", err, want)
	}
}

// TestPackageOpenPart covers the OpenPart wrapper on Package: loads a minimal
// package and confirms OpenPart returns a readable stream for an existing part
// and a "not found" error for a missing one. The underlying index.OpenPart
// path was previously 0% covered.
func TestPackageOpenPart(t *testing.T) {
	pk := mustLoad(t, map[string]string{
		"[Content_Types].xml":  miniContentTypes(`<Override PartName="/ppt/presentation.xml" ContentType="application/xml"/>`),
		"_rels/.rels":          miniRels(rel("rId1", RelOfficeDocument, "ppt/presentation.xml")),
		"ppt/presentation.xml": `<p:presentation/>`,
	})
	rc, err := pk.OpenPart("/ppt/presentation.xml")
	if err != nil {
		t.Fatalf("OpenPart existing: %v", err)
	}
	defer rc.Close()
	buf := make([]byte, 8)
	n, _ := rc.Read(buf)
	if n == 0 {
		t.Fatal("OpenPart returned empty stream")
	}
	if _, err := pk.OpenPart("/ppt/nonexistent.xml"); err == nil {
		t.Fatal("OpenPart missing part must return error")
	}
}

// TestBudgetNormalizeAllZero covers the normalize() fallback: a zero Budget
// (every field <= 0) must be replaced field-by-field with DefaultBudget().
func TestBudgetNormalizeAllZero(t *testing.T) {
	got := (Budget{}).normalize()
	def := DefaultBudget()
	if got.MaxEntries != def.MaxEntries ||
		got.MaxXMLBytes != def.MaxXMLBytes ||
		got.MaxMediaBytes != def.MaxMediaBytes ||
		got.MaxTotalBytes != def.MaxTotalBytes ||
		got.MaxXMLDepth != def.MaxXMLDepth {
		t.Fatalf("normalize(zero) = %+v, want all defaults %+v", got, def)
	}
}

// TestBudgetNormalizePartialOverride covers the mixed case: a budget with one
// non-zero field keeps that field; the remaining four fields fall back to
// defaults. This exercises the per-field if-branches that an all-zero budget
// would compress.
func TestBudgetNormalizePartialOverride(t *testing.T) {
	got := Budget{MaxXMLDepth: 42}.normalize()
	def := DefaultBudget()
	if got.MaxXMLDepth != 42 {
		t.Fatalf("MaxXMLDepth override lost: %d", got.MaxXMLDepth)
	}
	if got.MaxEntries != def.MaxEntries ||
		got.MaxXMLBytes != def.MaxXMLBytes ||
		got.MaxMediaBytes != def.MaxMediaBytes ||
		got.MaxTotalBytes != def.MaxTotalBytes {
		t.Fatalf("non-overridden fields not defaulted: %+v", got)
	}
}

// TestContentTypesAddOverride covers the addOverride happy path and the three
// rejection branches (invalid PartName, empty contentType, duplicate name).
// The existing ParseContentTypes-driven tests do not exercise addOverride
// directly because they only round-trip through the XML serializer.
func TestContentTypesAddOverride(t *testing.T) {
	ct := &ContentTypes{
		overrides:      map[PartName]string{},
		lowerOverrides: map[string]string{},
		defaults:       map[string]string{},
	}
	if err := ct.addOverride("/ppt/slides/slide1.xml", "ct-slide"); err != nil {
		t.Fatalf("addOverride valid: %v", err)
	}
	if got, ok := ct.Lookup("/ppt/slides/slide1.xml"); !ok || got != "ct-slide" {
		t.Fatalf("Lookup after add = %q, %v", got, ok)
	}
	// Duplicate → wrapped ErrMalformedPackage.
	if err := ct.addOverride("/ppt/slides/slide1.xml", "ct-slide"); !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("dup addOverride err = %v, want ErrMalformedPackage", err)
	}
	// Invalid PartName → wrapped ErrMalformedPackage.
	if err := ct.addOverride("not valid", "ct-x"); !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("invalid PartName err = %v, want ErrMalformedPackage", err)
	}
	// Empty contentType → wrapped ErrMalformedPackage.
	if err := ct.addOverride("/ppt/slides/slide2.xml", ""); !errors.Is(err, ErrMalformedPackage) {
		t.Fatalf("empty ct err = %v, want ErrMalformedPackage", err)
	}
}

// TestContentTypesLookupLowercaseFallback covers the Lookup precedence chain:
// (1) exact-match overrides, (2) case-insensitive fallback (lowerOverrides),
// (3) extension defaults. The middle branch was uncovered; we seed the
// maps directly so we can exercise all three fallbacks without a parse round
// trip.
func TestContentTypesLookupLowercaseFallback(t *testing.T) {
	ct := &ContentTypes{
		overrides:      map[PartName]string{"/ppt/slides/slide1.xml": "ct-slide-exact"},
		lowerOverrides: map[string]string{"ppt/slides/slide2.xml": "ct-slide-lower"},
		defaults:       map[string]string{"xml": "application/xml"},
	}
	// Exact match → overrides branch.
	if got, ok := ct.Lookup("/ppt/slides/slide1.xml"); !ok || got != "ct-slide-exact" {
		t.Fatalf("exact Lookup = %q, %v, want ct-slide-exact/true", got, ok)
	}
	// Case mismatch on registered name → lowerOverrides branch.
	if got, ok := ct.Lookup("/ppt/slides/SLIDE2.XML"); !ok || got != "ct-slide-lower" {
		t.Fatalf("case-insensitive Lookup = %q, %v, want ct-slide-lower/true", got, ok)
	}
	// No override, has extension → default branch.
	if got, ok := ct.Lookup("/ppt/notes/notesSlide1.xml"); !ok || got != "application/xml" {
		t.Fatalf("default Lookup = %q, %v, want application/xml/true", got, ok)
	}
	// No override, no extension (trailing dot) → ok=false.
	if _, ok := ct.Lookup("/ppt/notes/weird."); ok {
		t.Fatal("trailing-dot Lookup must return false")
	}
	// Invalid PartName → ok=false (Lookup early-return guard).
	if _, ok := ct.Lookup("not valid"); ok {
		t.Fatal("invalid PartName Lookup must return false")
	}
}
