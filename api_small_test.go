package pptx

import (
	"testing"

	"github.com/F31/go-pptx/internal/opc"
)

func TestOptionConstructors(t *testing.T) {
	newOpt := newOptions{}
	WithNewBudget(opc.Budget{MaxEntries: 7})(&newOpt)
	if newOpt.budget.MaxEntries != 7 {
		t.Fatalf("new budget = %+v", newOpt.budget)
	}
	template := map[string][]byte{"/ppt/slides/slide1.xml": []byte("abc")}
	WithNewTemplate(template)(&newOpt)
	template["/ppt/slides/slide1.xml"][0] = 'z'
	if string(newOpt.template["/ppt/slides/slide1.xml"]) != "abc" {
		t.Fatalf("template was not defensively copied: %q", newOpt.template["/ppt/slides/slide1.xml"])
	}
	openOpt := openOptions{}
	WithBudget(opc.Budget{MaxTotalBytes: 11})(&openOpt)
	if openOpt.budget.MaxTotalBytes != 11 {
		t.Fatalf("open budget = %+v", openOpt.budget)
	}
	saveOpt := saveOptions{}
	WithSaveOverwrite(true)(&saveOpt)
	WithSaveDurability(opc.DurabilityFull)(&saveOpt)
	if !saveOpt.overwrite || saveOpt.durability != opc.DurabilityFull {
		t.Fatalf("save options = %+v", saveOpt)
	}
	replOpt := replaceOptions{}
	WithReplaceMode(ReplaceExplicitStyle)(&replOpt)
	WithReplacementStyle(FontStyle{Bold: NewOptional(true)})(&replOpt)
	if replOpt.mode != ReplaceExplicitStyle || !replOpt.styleSet || !replOpt.style.Bold.Set || !replOpt.style.Bold.Value {
		t.Fatalf("replace options = %+v", replOpt)
	}
	bindOpt := bindOptions{}
	WithBindStrict(false)(&bindOpt)
	WithBindReplaceMode(ReplaceEqualLengthPerRune)(&bindOpt)
	if bindOpt.strict || bindOpt.mode != ReplaceEqualLengthPerRune {
		t.Fatalf("bind options = %+v", bindOpt)
	}
	mergeOpt := mergeOptions{}
	WithMergeTextPolicy(MergeKeepAnchorText)(&mergeOpt)
	if mergeOpt.policy != MergeKeepAnchorText {
		t.Fatalf("merge options = %+v", mergeOpt)
	}
}

func TestStableEnumStringers(t *testing.T) {
	replace := map[ReplaceMode]string{
		ReplaceFirstCharacter:     "FirstCharacterStyle",
		ReplaceEqualLengthPerRune: "EqualLengthPerRune",
		ReplaceExplicitStyle:      "ExplicitStyle",
		ReplaceMode(99):           "ReplaceMode(99)",
	}
	for mode, want := range replace {
		if got := mode.String(); got != want {
			t.Fatalf("ReplaceMode(%d).String = %q, want %q", mode, got, want)
		}
	}
	shapes := map[ShapeKind]string{
		ShapeTextBox:      "textbox",
		ShapeAutoShape:    "autoshape",
		ShapePicture:      "picture",
		ShapeGroup:        "group",
		ShapeConnector:    "connector",
		ShapeGraphicFrame: "graphic-frame",
		ShapeTable:        "table",
		ShapeChart:        "chart",
		ShapeAudio:        "audio",
		ShapeVideo:        "video",
		ShapeOpaque:       "opaque",
		ShapeKind(99):     "ShapeKind(99)",
	}
	for kind, want := range shapes {
		if got := kind.String(); got != want {
			t.Fatalf("ShapeKind(%d).String = %q, want %q", kind, got, want)
		}
	}
	for role, want := range map[AudioRole]string{
		AudioRoleNarration:  "narration",
		AudioRoleBackground: "background",
		AudioRoleEffect:     "effect",
		AudioRole(99):       "AudioRole(99)",
	} {
		if got := role.String(); got != want {
			t.Fatalf("AudioRole(%d).String = %q, want %q", role, got, want)
		}
	}
	for role, want := range map[VideoRole]string{
		VideoRoleBackground: "background",
		VideoRoleMain:       "main",
		VideoRoleTrim:       "trim",
		VideoRole(99):       "VideoRole(99)",
	} {
		if got := role.String(); got != want {
			t.Fatalf("VideoRole(%d).String = %q, want %q", role, got, want)
		}
	}
	for mode, want := range map[PictureFitMode]string{
		FitOriginalSize:    "FitOriginalSize",
		FitStretch:         "FitStretch",
		FitContain:         "FitContain",
		FitCover:           "FitCover",
		PictureFitMode(99): "PictureFitMode(99)",
	} {
		if got := mode.String(); got != want {
			t.Fatalf("PictureFitMode(%d).String = %q, want %q", mode, got, want)
		}
	}
	for trigger, want := range map[PlaybackTrigger]string{
		PlaybackOnSlideEnter: "onSlideEnter",
		PlaybackOnClick:      "onClick",
		PlaybackTrigger(99):  "PlaybackTrigger(99)",
	} {
		if got := trigger.String(); got != want {
			t.Fatalf("PlaybackTrigger(%d).String = %q, want %q", trigger, got, want)
		}
	}
	for mode, want := range map[IconMode]string{
		IconVisible:          "visible",
		IconHiddenDuringShow: "hiddenDuringShow",
		IconMode(99):         "IconMode(99)",
	} {
		if got := mode.String(); got != want {
			t.Fatalf("IconMode(%d).String = %q, want %q", mode, got, want)
		}
	}
	for severity, want := range map[Severity]string{
		SeverityInfo:    "info",
		SeverityWarning: "warning",
		SeverityError:   "error",
		Severity(99):    "unknown",
	} {
		if got := severity.String(); got != want {
			t.Fatalf("Severity(%d).String = %q, want %q", severity, got, want)
		}
	}
	for source, want := range map[StyleSource]string{
		SourceRun:              "run",
		SourceParagraphDefault: "paragraph-default",
		SourceListStyle:        "list-style",
		SourceTheme:            "theme",
		SourceFallback:         "fallback",
		SourceCellExplicit:     "cell-explicit",
		SourceTableStyle:       "table-style",
		StyleSource(99):        "unknown",
	} {
		if got := source.String(); got != want {
			t.Fatalf("StyleSource(%d).String = %q, want %q", source, got, want)
		}
	}
	for slot, want := range map[ThemeFontSlot]string{
		FontSlotUnknown:   "unknown",
		FontSlotMajor:     "major",
		FontSlotMinor:     "minor",
		ThemeFontSlot(99): "unknown",
	} {
		if got := slot.String(); got != want {
			t.Fatalf("ThemeFontSlot(%d).String = %q, want %q", slot, got, want)
		}
	}
	for toggle, want := range map[StyleToggle]string{
		ToggleDefault:   "def",
		ToggleOn:        "on",
		ToggleOff:       "off",
		StyleToggle(99): "def",
	} {
		if got := toggle.String(); got != want {
			t.Fatalf("StyleToggle(%d).String = %q, want %q", toggle, got, want)
		}
	}
	for fill, want := range map[FillKind]string{
		FillUnspecified: "unspecified",
		FillNone:        "none",
		FillSolid:       "solid",
		FillGradient:    "gradient",
		FillPattern:     "pattern",
		FillPicture:     "picture",
		FillGroup:       "group",
		FillKind(99):    "unspecified",
	} {
		if got := fill.String(); got != want {
			t.Fatalf("FillKind(%d).String = %q, want %q", fill, got, want)
		}
	}
	for typ, want := range map[ChartType]string{
		ChartBar:      "bar",
		ChartLine:     "line",
		ChartPie:      "pie",
		ChartType(99): "ChartType(99)",
	} {
		if got := typ.String(); got != want {
			t.Fatalf("ChartType(%d).String = %q, want %q", typ, got, want)
		}
	}
	for typ, want := range map[ChartErrorType]string{
		ChartErrStandardDeviation: "stdDev",
		ChartErrStandardError:     "stdErr",
		ChartErrFixed:             "fixed",
		ChartErrPercentage:        "percentage",
		ChartErrorType(99):        "",
	} {
		if got := typ.String(); got != want {
			t.Fatalf("ChartErrorType(%d).String = %q, want %q", typ, got, want)
		}
	}
	for typ, want := range map[ChartTrendType]string{
		ChartTrendLinear:        "linear",
		ChartTrendLogarithmic:   "log",
		ChartTrendExponential:   "exp",
		ChartTrendPolynomial:    "poly",
		ChartTrendPower:         "power",
		ChartTrendMovingAverage: "movingAvg",
		ChartTrendType(99):      "",
	} {
		if got := typ.String(); got != want {
			t.Fatalf("ChartTrendType(%d).String = %q, want %q", typ, got, want)
		}
	}
	for kind, want := range map[BulletKind]string{
		BulletNone:     "none",
		BulletChar:     "char",
		BulletAutoNum:  "autonum",
		BulletBlip:     "blip",
		BulletUnknown:  "unknown",
		BulletKind(99): "unknown",
	} {
		if got := kind.String(); got != want {
			t.Fatalf("BulletKind(%d).String = %q, want %q", kind, got, want)
		}
	}
	for kind, want := range map[MatrixRefKind]string{
		RefFill:           "fillRef",
		RefLine:           "lnRef",
		RefEffect:         "effectRef",
		RefFont:           "fontRef",
		MatrixRefKind(99): "unknown",
	} {
		if got := kind.String(); got != want {
			t.Fatalf("MatrixRefKind(%d).String = %q, want %q", kind, got, want)
		}
	}
	for part, want := range map[StylePart]string{
		PartWholeTable: "wholeTbl",
		PartBand1H:     "band1H",
		PartBand2H:     "band2H",
		PartBand1V:     "band1V",
		PartBand2V:     "band2V",
		PartFirstRow:   "firstRow",
		PartLastRow:    "lastRow",
		PartFirstCol:   "firstCol",
		PartLastCol:    "lastCol",
		PartNWCell:     "nwCell",
		PartNECell:     "neCell",
		PartSWCell:     "swCell",
		PartSECell:     "seCell",
		StylePart(99):  "StylePart(99)",
	} {
		if got := part.String(); got != want {
			t.Fatalf("StylePart(%d).String = %q, want %q", part, got, want)
		}
	}
}

func TestStableValueObjectStrings(t *testing.T) {
	for spec, want := range map[ColorSpec]string{
		{Scheme: "accent1"}: "scheme:accent1",
		{RGB: "FF0000"}:     "#FF0000",
		{}:                  "",
	} {
		if got := spec.String(); got != want {
			t.Fatalf("ColorSpec(%+v).String = %q, want %q", spec, got, want)
		}
		if got := spec.Valid(); got != (want != "") {
			t.Fatalf("ColorSpec(%+v).Valid = %v", spec, got)
		}
	}
	style := FontStyle{
		Bold:          NewOptional(true),
		Italic:        NewOptional(false),
		Size:          NewOptional(Pts(12.5)),
		Color:         NewOptional(ColorSpec{RGB: "00FF00"}),
		Latin:         NewOptional("Arial"),
		EastAsian:     NewOptional("MS PGothic"),
		ComplexScript: NewOptional("Arial Unicode MS"),
	}
	if got := style.String(); got == "" {
		t.Fatal("FontStyle.String returned empty string")
	}
}
