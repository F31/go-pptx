package pptx

// 本文件是 v1.0 冻结清单的**自动化守门**（ADR-015 / docs/v1.0-freeze-list.md）。
//
// 背景：v1.0 的公共 API 不变量（Stable 段数 / Stable 符号数 / Experimental 段数 /
// 公共 type 总数 / 哨兵数）此前只能靠人工 grep 统计，v1.0.0 发布时就出过一次
// 口径错误（把"段落 grep 数"误作"独立 type 数"），事后连改 6 处文档。更糟的是，
// 手工口径 `grep '^type [A-Z]'` 本身就漏掉了分组 `type ( ... )` 声明——实测只
// 数出 149，真实值是 158。
//
// 因此这里改用 go/ast 解析根包非测试文件，把这组数字连同**完整符号名单**固化
// 为断言。任何公共 API 表面的变化（新增 / 删除 / 重命名 / 误加 Stable 标记）
// 都会让本文件测试失败，并且在 code review 中体现为 golden 清单的一行 diff——
// 这正是 freeze list §D 要求的" conscious change"。
//
// 更新本文件的正确姿势：确属有意的 API 演化（且已按 ADR-015 走完评审）→ 同步
// 修改下方 golden 清单并在 PR 说明中给出理由；否则应修改代码而不是改这里。

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// v1.0 冻结的计数不变量（grep 无法可靠复现，一律以 AST 口径为准）。
const (
	wantExportedTypes        = 158 // 根包导出 type 总数（含 2 个 type alias）
	wantStableSections       = 34  // 带 "// Stable:" 段的顶层声明数（33 type + 1 哨兵聚合段）
	wantStableSymbols        = 50  // 上述段落覆盖的符号数（33 type + 17 哨兵）
	wantExperimentalSections = 5   // 带 "// Experimental:" 段的顶层声明数
	wantStableMethods        = 129 // Stable type 上的导出方法数（v1.0.2=127；FEAT-003 读侧补全 +2）
	wantSentinels            = 17  // 导出 Err* 哨兵数
)

// goldenExportedTypes 是 v1.0 冻结的根包导出 type 名单（排序后）。
var goldenExportedTypes = []string{
	"AnimationTimingPolicy", "AudioProfile", "AudioRole", "AudioShape", "AudioSpec",
	"AutoShape", "AutoShapeSpec", "Bevel3D", "BindOption", "BindReport",
	"BlipFillInfo", "BodyProps", "Bullet", "BulletKind", "Camera3D",
	"CapabilityDimension", "CapabilityFeature", "CapabilityManifest", "CapabilityManifestSource", "CapabilityStatus",
	"Cell", "CellBorder", "CellBorders", "CellFill", "CellRange",
	"CellText", "ChartAxisOptions", "ChartData", "ChartDataBook", "ChartDataLabel",
	"ChartErrorBars", "ChartErrorType", "ChartSeries", "ChartShape", "ChartSpec",
	"ChartTrendType", "ChartTrendline", "ChartType", "ChartWorkbookBuilder", "ClonePolicy",
	"ColorSpec", "ColorTransform", "CoreProperties", "CorePropertiesPatch", "CustomPropertyKind",
	"CustomPropertyValue", "CutOptions", "DefaultWorkbookBuilder", "Diagnostic", "EMU",
	"Effect", "EffectInfo", "EffectKind", "EffectiveCellStyle", "FadeOptions",
	"Field", "FieldKind", "FieldSpec", "FillInfo", "FillKind",
	"FontProperty", "FontSize", "FontStyle", "GeomAdjust", "GeomGuide",
	"GeomPath", "GeometryInfo", "GeometryKind", "GradientFill", "GradientStop",
	"GroupShape", "HandoutMasterInfo", "IconMode", "KinsokuRule", "LayoutEmbeddedFont",
	"LayoutRef", "LayoutReport", "LayoutSection", "LightRig3D", "LineEnd",
	"LineStyle", "MatrixRefKind", "MediaSource", "MergeOption", "MultiCellTextPolicy",
	"NewOption", "OpaqueShape", "OpenOption", "OperationError", "Optional",
	"PageTiming", "Paragraph", "ParagraphProps", "ParagraphSpec", "ParsedColor",
	"PathCommand", "PatternFill", "PictureFitMode", "PictureShape", "PictureSpec",
	"Placeholder", "PlaybackSpec", "PlaybackTrigger", "Point", "Presentation",
	"Quad", "Rect", "ReplaceHit", "ReplaceMode", "ReplaceOption",
	"ReplaceResult", "ResolveContext", "ResolvedColor", "ResolvedFont", "ResolvedValue",
	"RunProps", "RunSymbol", "SaveOption", "SaveReport", "Scene3DInfo",
	"Severity", "Shape", "Shape3DInfo", "ShapeID", "ShapeKind",
	"Slide", "SlideID", "Spacing", "SplitAxis", "SplitDir",
	"StyleMatrixRef", "StylePart", "StyleSource", "StyleStep", "StyleToggle",
	"TabStop", "TableShape", "TableStyleFlags", "TextBoxSpec", "TextFrame",
	"TextRun", "TextShape", "ThemeFontSlot", "TimingPlan", "TimingSyncOptions",
	"TimingSyncReport", "TrackContribution", "TransitionDir", "TransitionSpec", "TransitionSpeed",
	"TransitionType", "UnknownDurationPolicy", "ValidateOption", "ValidationReport", "VideoProfile",
	"VideoRole", "VideoShape", "VideoSpec",
}

// goldenStableSymbols 是 v1.0 冻结的 Stable 符号集合（排序后）。
var goldenStableSymbols = []string{
	"AudioShape", "AutoShape", "CapabilityDimension", "CapabilityFeature",
	"CapabilityManifest", "CapabilityManifestSource", "CapabilityStatus", "ChartShape",
	"Diagnostic", "EMU", "ErrAtomicReplaceUnavailable", "ErrClosed",
	"ErrConcurrentModification", "ErrDurationUnknown", "ErrForeignReference", "ErrInvalidArgument",
	"ErrLimitExceeded", "ErrMalformedPackage", "ErrNotFound", "ErrOutOfRange",
	"ErrOutputExists", "ErrStaleHandle", "ErrTimingConflict", "ErrUnresolvedStyle",
	"ErrUnsupportedEdit", "ErrUnsupportedFormat", "ErrValidationFailed", "GroupShape",
	"MultiCellTextPolicy", "OpaqueShape", "OperationError", "Paragraph",
	"PictureShape", "Point", "Presentation", "Quad",
	"Rect", "ReplaceMode", "Severity", "Shape",
	"ShapeID", "ShapeKind", "Slide", "SlideID",
	"TableShape", "TextFrame", "TextRun", "TextShape",
	"ValidationReport", "VideoShape",
}

// goldenExperimentalSymbols 是 v1.0 冻结的 Experimental 符号集合（排序后）。
var goldenExperimentalSymbols = []string{
	"ChartDataBook", "ChartWorkbookBuilder", "CustomPropertyKind",
	"CustomPropertyValue", "DefaultWorkbookBuilder",
}

// goldenStableMethods 是 Stable type 上的导出方法集合，形如 "Type.Method"（排序后）。
// 这是 binary-compat 的真实表面：删除或重命名其中任何一条都会破坏下游编译。
//
// 变更记录（FEAT-003 读侧补全，2026-09-22）：
//   - v1.0.2 = 127 项；本版 = 129 项（+2）
//   - +Slide.AdvanceAfter：p:transition@advTm 读侧，与 SetAdvanceAfter 写入对偶（仅追加只读）
//   - +Slide.Hidden：p:sldId@show="0" 读侧（仅追加只读）
//     两方法均为追加式只读公开方法，binary-compat with v1.0.0..v1.0.2；列入 Stable 段。
var goldenStableMethods = []string{
	"AudioShape.AudioSource", "AudioShape.Kind", "AudioShape.Profile",
	"AudioShape.Role", "AudioShape.SetPlayback", "AutoShape.Kind",
	"AutoShape.Placeholder", "AutoShape.SetAltText", "AutoShape.SetDecorative",
	"AutoShape.TextFrame", "CapabilityStatus.MarshalJSON", "CapabilityStatus.String",
	"CapabilityStatus.UnmarshalJSON", "ChartShape.Data", "ChartShape.Kind",
	"ChartShape.SetAltText", "ChartShape.SetData", "ChartShape.SetDecorative",
	"EMU.Inches", "EMU.Points", "GroupShape.Children", "GroupShape.Kind",
	"OpaqueShape.Kind", "OperationError.Error", "OperationError.Unwrap",
	"Paragraph.AddRun", "Paragraph.AppendField", "Paragraph.Fields",
	"Paragraph.InsertField", "Paragraph.Props", "Paragraph.ReplaceText",
	"Paragraph.Runs", "Paragraph.Text", "PictureShape.Kind",
	"PictureShape.ReplaceImage", "PictureShape.SetAltText", "PictureShape.SetDecorative",
	"Presentation.AddSlide", "Presentation.ApplyTimingPlan", "Presentation.Bind",
	"Presentation.Capability", "Presentation.Close", "Presentation.CopySlideFrom",
	"Presentation.CoreProperties", "Presentation.CustomProperties", "Presentation.DebugAudioXML",
	"Presentation.DebugVideoXML", "Presentation.LayoutInfo", "Presentation.Layouts",
	"Presentation.MoveSlide", "Presentation.PlanTimingSync", "Presentation.RemoveSlide",
	"Presentation.Revision", "Presentation.Save", "Presentation.SetChartWorkbookBuilder",
	"Presentation.SetCoreProperties", "Presentation.SetCustomProperty", "Presentation.Slide",
	"Presentation.SlideByID", "Presentation.Slides", "Presentation.SyncTimingToAudio",
	"Presentation.Validate", "Presentation.Write", "Rect.Bottom",
	"Rect.Contains", "Rect.Right", "ReplaceMode.String", "Severity.String", "ShapeKind.String", "Slide.AddAudio",
	"Slide.AddAutoShape", "Slide.AddChart", "Slide.AddPicture",
	"Slide.AddTextBox", "Slide.AddVideo", "Slide.AdvanceAfter",
	"Slide.Clone", "Slide.EnsureSpeakerNotes", "Slide.HasTiming",
	"Slide.Hidden", "Slide.ID",
	"Slide.MoveShape", "Slide.Name", "Slide.NotesPart",
	"Slide.PartName", "Slide.Placeholders", "Slide.RemoveShape",
	"Slide.RemoveTransition", "Slide.SetAdvanceAfter", "Slide.SetSpeakerNotes",
	"Slide.SetTransition", "Slide.Shapes", "Slide.SpeakerNotes",
	"Slide.SpeakerNotesText", "Slide.TimingTreeRaw", "Slide.Transition",
	"Slide.UpsertNarration", "TableShape.Cell", "TableShape.ColumnCount",
	"TableShape.ColumnWidth", "TableShape.Kind", "TableShape.Merge",
	"TableShape.RowCount", "TableShape.RowHeight", "TableShape.SetColumnWidth",
	"TableShape.SetRowHeight", "TableShape.StyleFlags", "TableShape.StyleID",
	"TableShape.Unmerge", "TextFrame.AddParagraph", "TextFrame.BodyProps",
	"TextFrame.Paragraphs", "TextFrame.ReplaceText", "TextFrame.SetBodyProps",
	"TextFrame.SetPlainText", "TextRun.AdvancedProps", "TextRun.EffectiveFont",
	"TextRun.ExplicitFont", "TextRun.ResetFontProperty", "TextRun.SetFont",
	"TextRun.SetText", "TextRun.Text", "ValidationReport.HasErrors",
	"VideoShape.HasPoster", "VideoShape.Kind", "VideoShape.PosterSource",
	"VideoShape.Profile", "VideoShape.Role", "VideoShape.VideoSource",
}

// apiSurface 是根包公共 API 表面的一次性 AST 快照。
type apiSurface struct {
	files        int
	types        []string // 导出 type 名
	stableSec    int      // 带 "// Stable:" 的顶层声明数
	stableSyms   []string // 上述声明覆盖的符号名
	expSec       int      // 带 "// Experimental:" 的顶层声明数
	expSyms      []string // 上述声明覆盖的符号名
	stableMethod []string // "Type.Method"，仅 Stable type
	sentinels    []string // 导出 Err* 名
}

// loadAPISurface 解析根包（排除 _test.go）并返回其公共 API 表面。
//
// 用 AST 而非反射：反射无法枚举包级类型；用 AST 而非 grep：grep 漏掉分组
// `type ( ... )` 声明（实测 149 vs 真实 158），且不受注释缩进影响。
func loadAPISurface(t *testing.T) apiSurface {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse root package: %v", err)
	}
	pkg, ok := pkgs["pptx"]
	if !ok {
		t.Fatalf(`root package "pptx" not found (found: %v); `+
			`if the package was renamed, update api_surface_test.go deliberately`, pkgNames(pkgs))
	}

	var s apiSurface
	names := make([]string, 0, len(pkg.Files))
	for n := range pkg.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	s.files = len(names)

	for _, fname := range names {
		for _, d := range pkg.Files[fname].Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			var decl []string
			switch gd.Tok {
			case token.TYPE:
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					if ts.Name.IsExported() {
						decl = append(decl, ts.Name.Name)
					}
				}
				s.types = append(s.types, decl...)
			case token.VAR, token.CONST:
				for _, spec := range gd.Specs {
					for _, n := range spec.(*ast.ValueSpec).Names {
						if n.IsExported() {
							decl = append(decl, n.Name)
						}
					}
				}
			}
			switch markerOf(gd.Doc) {
			case "Stable":
				s.stableSec++
				s.stableSyms = append(s.stableSyms, decl...)
			case "Experimental":
				s.expSec++
				s.expSyms = append(s.expSyms, decl...)
			}
			if gd.Tok == token.VAR {
				for _, n := range decl {
					if strings.HasPrefix(n, "Err") {
						s.sentinels = append(s.sentinels, n)
					}
				}
			}
		}
	}

	stable := map[string]bool{}
	for _, n := range s.stableSyms {
		stable[n] = true
	}
	for _, fname := range names {
		for _, d := range pkg.Files[fname].Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || !fd.Name.IsExported() || len(fd.Recv.List) == 0 {
				continue
			}
			recv := strings.TrimPrefix(recvTypeName(fd.Recv.List[0].Type), "*")
			if stable[recv] {
				s.stableMethod = append(s.stableMethod, recv+"."+fd.Name.Name)
			}
		}
	}

	for _, sl := range [][]string{s.types, s.stableSyms, s.expSyms, s.stableMethod, s.sentinels} {
		sort.Strings(sl)
	}
	return s
}

// markerOf 取顶层声明 doc 中的稳定性标记行（"Stable" / "Experimental" / ""）。
func markerOf(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	for _, c := range doc.List {
		switch {
		case strings.HasPrefix(c.Text, "// Stable:"):
			return "Stable"
		case strings.HasPrefix(c.Text, "// Experimental:"):
			return "Experimental"
		}
	}
	return ""
}

func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + recvTypeName(t.X)
	case *ast.IndexExpr: // 泛型接收者，如 Optional[T]
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	}
	return ""
}

func pkgNames(m map[string]*ast.Package) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// diffSorted 给出 want 与 got 的差集，用于产出可读的失败信息。
func diffSorted(want, got []string) (missing, added []string) {
	w, g := map[string]int{}, map[string]int{}
	for _, x := range want {
		w[x]++
	}
	for _, x := range got {
		g[x]++
	}
	for _, x := range want {
		if g[x] == 0 {
			missing = append(missing, x)
		}
	}
	for _, x := range got {
		if w[x] == 0 {
			added = append(added, x)
		}
	}
	return missing, added
}

// assertSet 断言两个已排序集合相等，失败时打印增删明细。
func assertSet(t *testing.T, what string, want, got []string) {
	t.Helper()
	missing, added := diffSorted(want, got)
	if len(missing) == 0 && len(added) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s changed (v1.0 frozen API surface):\n", what)
	if len(missing) > 0 {
		fmt.Fprintf(&b, "  removed/renamed (%d): %v\n", len(missing), missing)
	}
	if len(added) > 0 {
		fmt.Fprintf(&b, "  added (%d): %v\n", len(added), added)
	}
	fmt.Fprintf(&b, "  %s", freezeHint)
	t.Fatal(b.String())
}

const freezeHint = "This is a v1.0 freeze guard: updating the golden list is only allowed " +
	"after an ADR-015 review; otherwise fix the code, not this file."

// TestNoBuildConstraintsInRootPackage 保证根包公共 API 表面不随 GOOS/构建标签漂移。
//
// 若允许根包出现平台条件文件（如 `shape_windows.go`），则本文件的 AST 快照测的是
// **各平台 API 的并集**——在 Linux CI 上会拿"Windows 的表面"去比对真实 Linux 表面，
// 守门形同虚设。v1.0 承诺单一跨平台 API，故直接禁止（分平台逻辑请放 internal 包）。
func TestNoBuildConstraintsInRootPackage(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse root package: %v", err)
	}
	pkg, ok := pkgs["pptx"]
	if !ok {
		t.Fatalf(`root package "pptx" not found (found: %v)`, pkgNames(pkgs))
	}
	for name, f := range pkg.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "//go:build") || strings.HasPrefix(c.Text, "// +build") {
					t.Errorf("%s carries build constraint %q; the root package must expose one "+
						"identical API on every platform — move platform-specific code to an internal package",
						name, c.Text)
				}
			}
		}
	}
}

// TestAPIFrozenCounts 锁死 v1.0 冻结清单的五个计数不变量。
func TestAPIFrozenCounts(t *testing.T) {
	s := loadAPISurface(t)
	if got := len(s.types); got != wantExportedTypes {
		t.Errorf("exported types = %d, want %d (%s)", got, wantExportedTypes, freezeHint)
	}
	if got := s.stableSec; got != wantStableSections {
		t.Errorf(`"// Stable:" sections = %d, want %d (%s)`, got, wantStableSections, freezeHint)
	}
	if got := len(s.stableSyms); got != wantStableSymbols {
		t.Errorf("stable symbols = %d, want %d (%s)", got, wantStableSymbols, freezeHint)
	}
	if got := s.expSec; got != wantExperimentalSections {
		t.Errorf(`"// Experimental:" sections = %d, want %d (%s)`, got, wantExperimentalSections, freezeHint)
	}
	if got := len(s.sentinels); got != wantSentinels {
		t.Errorf("error sentinels = %d, want %d (%s)", got, wantSentinels, freezeHint)
	}
}

// TestAPIFrozenExportedTypes 锁死根包导出 type 的完整名单。
func TestAPIFrozenExportedTypes(t *testing.T) {
	assertSet(t, "exported types", goldenExportedTypes, loadAPISurface(t).types)
}

// TestAPIFrozenStableSymbols 锁死 Stable 符号名单（升/降档必须走 ADR-015）。
func TestAPIFrozenStableSymbols(t *testing.T) {
	assertSet(t, "stable symbols", goldenStableSymbols, loadAPISurface(t).stableSyms)
}

// TestAPIFrozenExperimentalSymbols 锁死 Experimental 符号名单。
func TestAPIFrozenExperimentalSymbols(t *testing.T) {
	assertSet(t, "experimental symbols", goldenExperimentalSymbols, loadAPISurface(t).expSyms)
}

// TestAPIFrozenStableMethods 锁死 Stable type 的导出方法集（binary-compat 表面）。
func TestAPIFrozenStableMethods(t *testing.T) {
	s := loadAPISurface(t)
	if got := len(s.stableMethod); got != wantStableMethods {
		t.Errorf("stable methods = %d, want %d (%s)", got, wantStableMethods, freezeHint)
	}
	assertSet(t, "stable methods", goldenStableMethods, s.stableMethod)
}

// sentinelValues 把 17 个哨兵的**运行时值**暴露给断言，用于锁定错误字符串。
func sentinelValues() map[string]error {
	return map[string]error{
		"ErrClosed":                   ErrClosed,
		"ErrStaleHandle":              ErrStaleHandle,
		"ErrInvalidArgument":          ErrInvalidArgument,
		"ErrOutOfRange":               ErrOutOfRange,
		"ErrNotFound":                 ErrNotFound,
		"ErrForeignReference":         ErrForeignReference,
		"ErrUnsupportedFormat":        ErrUnsupportedFormat,
		"ErrUnsupportedEdit":          ErrUnsupportedEdit,
		"ErrLimitExceeded":            ErrLimitExceeded,
		"ErrMalformedPackage":         ErrMalformedPackage,
		"ErrUnresolvedStyle":          ErrUnresolvedStyle,
		"ErrValidationFailed":         ErrValidationFailed,
		"ErrTimingConflict":           ErrTimingConflict,
		"ErrDurationUnknown":          ErrDurationUnknown,
		"ErrConcurrentModification":   ErrConcurrentModification,
		"ErrOutputExists":             ErrOutputExists,
		"ErrAtomicReplaceUnavailable": ErrAtomicReplaceUnavailable,
	}
}

// goldenSentinelMessages 锁死 17 个哨兵的 error 字符串（下游按 errors.Is 分支，
// 字符串一旦变更即视为破坏性变更）。
var goldenSentinelMessages = map[string]string{
	"ErrClosed":                   "pptx: document closed",
	"ErrStaleHandle":              "pptx: stale handle",
	"ErrInvalidArgument":          "pptx: invalid argument",
	"ErrOutOfRange":               "pptx: index out of range",
	"ErrNotFound":                 "pptx: not found",
	"ErrForeignReference":         "pptx: foreign reference",
	"ErrUnsupportedFormat":        "pptx: unsupported format",
	"ErrUnsupportedEdit":          "pptx: unsupported edit",
	"ErrLimitExceeded":            "pptx: resource limit exceeded",
	"ErrMalformedPackage":         "pptx: malformed package",
	"ErrUnresolvedStyle":          "pptx: unresolved style",
	"ErrValidationFailed":         "pptx: validation failed",
	"ErrTimingConflict":           "pptx: timing conflict",
	"ErrDurationUnknown":          "pptx: duration unknown",
	"ErrConcurrentModification":   "pptx: concurrent modification",
	"ErrOutputExists":             "pptx: output file exists",
	"ErrAtomicReplaceUnavailable": "pptx: atomic replace unavailable",
}

// TestErrorSentinelsFrozen 锁死哨兵的名称集合、错误字符串与可检测性。
func TestErrorSentinelsFrozen(t *testing.T) {
	s := loadAPISurface(t)
	vals := sentinelValues()

	if len(vals) != wantSentinels {
		t.Fatalf("sentinelValues() covers %d sentinels, want %d", len(vals), wantSentinels)
	}
	// AST 名单与运行时名单必须逐一对应，防止新增哨兵后忘了登记到这里。
	assertSet(t, "sentinel names", s.sentinels, sortedKeys(vals))

	seen := map[string]string{}
	for name, err := range vals {
		if err == nil {
			t.Errorf("sentinel %s is nil", name)
			continue
		}
		want, ok := goldenSentinelMessages[name]
		if !ok {
			t.Errorf("sentinel %s has no frozen message registered (%s)", name, freezeHint)
			continue
		}
		if got := err.Error(); got != want {
			t.Errorf("sentinel %s message = %q, want %q (%s)", name, got, want, freezeHint)
		}
		// 语义锁死：包装后仍须可判定，且不同哨兵不得共用字符串。
		wrapped := fmt.Errorf("context: %w", err)
		if !errors.Is(wrapped, err) {
			t.Errorf("sentinel %s is not detectable after wrapping", name)
		}
		if prev, dup := seen[err.Error()]; dup {
			t.Errorf("sentinels %s and %s share message %q", prev, name, err.Error())
		}
		seen[err.Error()] = name
	}
}

func sortedKeys(m map[string]error) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
