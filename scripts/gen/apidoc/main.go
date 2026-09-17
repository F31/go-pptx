// Command apidoc 生成 pptx 门面的英文 API 接口参考文档（纯 AST 驱动，
// 结构化输出 Markdown）。计数口径与 pptx/api_surface_test.go 的 golden
// 一致（163 types / 36 顶层函数 / 167 导出方法 / 149 常量 / 18 哨兵）。
// 仅 std-lib。产物：docs/api-reference.md
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type typeEntry struct {
	name    string
	kind    string // struct / interface / alias / type
	alias   string // alias 目标（kind=alias 时）
	stable  bool
	exp     bool
	methods []methodEntry
}

type methodEntry struct {
	name   string
	sig    string
	stable bool
}

func main() {
	dir := flag.String("pkg", "pptx", "package directory to scan")
	out := flag.String("out", "docs/api-reference.md", "output markdown")
	flag.Parse()

	fs := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(*dir, "*.go"))
	if err != nil {
		log.Fatal(err)
	}

	type pkgInfo struct {
		types  map[string]*typeEntry
		funcs  map[string]string // name -> sig
		consts map[string]string
		vars   map[string]string
		docs   map[string]string // name -> first doc line
	}
	pi := &pkgInfo{
		types:  map[string]*typeEntry{},
		funcs:  map[string]string{},
		consts: map[string]string{},
		vars:   map[string]string{},
		docs:   map[string]string{},
	}
	methodSigs := map[string]map[string]string{} // type -> method -> sig
	methodStable := map[string]map[string]bool{}
	// methodDocs 已并入 methodStable

	for _, fn := range files {
		if strings.HasSuffix(fn, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fs, fn, nil, parser.ParseComments)
		if err != nil {
			log.Fatalf("parse %s: %v", fn, err)
		}
		for _, d := range f.Decls {
			switch v := d.(type) {
			case *ast.GenDecl:
				doc := ""
				if v.Doc != nil {
					doc = firstDocLine(v.Doc.Text())
				}
				for _, s := range v.Specs {
					switch sp := s.(type) {
					case *ast.ValueSpec:
						for _, n := range sp.Names {
							if !n.IsExported() {
								continue
							}
							if v.Tok == token.CONST {
								pi.consts[n.Name] = ""
							} else {
								pi.vars[n.Name] = ""
							}
							if d := strings.TrimSpace(doc); d != "" && !strings.HasPrefix(d, "Stable:") && !strings.HasPrefix(d, "Experimental:") {
								pi.docs[n.Name] = d
							}
						}
					case *ast.TypeSpec:
						if !sp.Name.IsExported() {
							continue
						}
						te := &typeEntry{name: sp.Name.Name, stable: strings.Contains(doc, "Stable:"), exp: strings.Contains(doc, "Experimental:")}
						switch ut := sp.Type.(type) {
						case *ast.StructType:
							te.kind = "struct"
						case *ast.InterfaceType:
							te.kind = "interface"
						default:
							te.kind = "type"
							if sp.Assign.IsValid() {
								if id, ok := ut.(*ast.Ident); ok {
									te.kind = "alias"
									te.alias = id.Name
								}
							}
						}
						pi.types[sp.Name.Name] = te
					}
				}
				if strings.HasPrefix(doc, "Stable:") || strings.HasPrefix(doc, "Experimental:") {
					for _, s := range v.Specs {
						if sp, ok := s.(*ast.TypeSpec); ok && sp.Name.IsExported() {
							pi.docs[sp.Name.Name] = firstDocLine(strings.TrimPrefix(strings.TrimPrefix(doc, "Stable:"), "Experimental:"))
						}
					}
				}
			case *ast.FuncDecl:
				if !v.Name.IsExported() {
					continue
				}
				sig := funcSig(v)
				doc := ""
				if v.Doc != nil {
					doc = v.Doc.Text()
				}
				if v.Recv == nil {
					pi.funcs[v.Name.Name] = sig
					pi.docs[v.Name.Name] = firstDocLine(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(doc, "Stable:"), "Experimental:")))
				} else {
					recv := recvType(v.Recv)
					if methodSigs[recv] == nil {
						methodSigs[recv] = map[string]string{}
						methodStable[recv] = map[string]bool{}
					}
					methodSigs[recv][v.Name.Name] = sig
					methodStable[recv][v.Name.Name] = strings.Contains(doc, "Stable:")
				}
			}
		}
	}

	// 方法挂到类型上
	for name, te := range pi.types {
		for m, sig := range methodSigs[name] {
			te.methods = append(te.methods, methodEntry{name: m, sig: sig, stable: methodStable[name][m]})
		}
		sort.Slice(te.methods, func(i, j int) bool { return te.methods[i].name < te.methods[j].name })
	}

	// 排序
	typeNames := make([]string, 0, len(pi.types))
	for n := range pi.types {
		typeNames = append(typeNames, n)
	}
	sort.Strings(typeNames)
	funcNames := make([]string, 0, len(pi.funcs))
	for n := range pi.funcs {
		funcNames = append(funcNames, n)
	}
	sort.Strings(funcNames)
	constNames := make([]string, 0, len(pi.consts))
	for n := range pi.consts {
		constNames = append(constNames, n)
	}
	sort.Strings(constNames)
	varNames := make([]string, 0, len(pi.vars))
	for n := range pi.vars {
		varNames = append(varNames, n)
	}
	sort.Strings(varNames)

	// ---- write ----
	var b strings.Builder
	w := func(s string, a ...any) { fmt.Fprintf(&b, s+"\n", a...) }

	w("# go-pptx API Reference")
	w("")
	w("> Public API of the **`pptx`** package — module `github.com/F31/go-pptx/pptx` (v2.0).")
	w("> Stability tiers: `Stable:` methods/types are frozen and backward compatible per ADR-015; all other exported symbols are management contract and may change.")
	w("> Live godoc: `go doc github.com/F31/go-pptx/pptx`")
	w("")
	w("## Package Surface")
	w("")
	w("| Item | Count |")
	w("|---|---|")
	w("| Exported types | %d |", len(typeNames))
	w("| Top-level functions | %d |", len(funcNames))
	w("| Exported methods | %d |", countMethods(pi.types))
	w("| Constants | %d |", len(constNames))
	w("| Package vars & error sentinels | %d |", len(varNames))
	w("")
	w("## Types")
	w("")
	for _, name := range typeNames {
		te := pi.types[name]
		w("### `%s`", name)
		w("")
		switch te.kind {
		case "alias":
			w("Alias of `%s`.", te.alias)
		case "struct":
			w("Kind: struct")
		case "interface":
			w("Kind: interface")
		default:
			w("Kind: `%s`", te.kind)
		}
		if te.stable {
			w("")
			w("**Stability:** Stable")
		}
		if d := typeDescription(name, pi.docs[name]); d != "" {
			w("")
			w("> %s", d)
		}
		if len(te.methods) > 0 {
			w("")
			w("**Methods:**")
			w("")
			for _, m := range te.methods {
				flag := ""
				if m.stable {
					flag = " `[Stable]`"
				}
				w("- `func (%s) %s`%s", name, m.sig, flag)
			}
		}
		w("")
	}

	w("## Functions")
	w("")
	for _, n := range funcNames {
		w("- `func %s`", pi.funcs[n])
	}
	w("")
	w("## Constants")
	w("")
	for _, n := range constNames {
		if d := enDescOrNil(n, pi.docs[n]); d != "" {
			w("- `%s` — %s", n, d)
		} else {
			w("- `%s`", n)
		}
	}
	w("")
	w("## Package Variables & Error Sentinels")
	w("")
	for _, n := range varNames {
		if d := enDescOrNil(n, pi.docs[n]); d != "" {
			w("- `%s` — %s", n, d)
		} else {
			w("- `%s`", n)
		}
	}
	w("")
	w("---")
	w("")
	w("*Generated by `scripts/gen/apidoc`. Regenerate: `go run ./scripts/gen/apidoc`.*")

	if err := os.WriteFile(*out, []byte(b.String()), 0o644); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", *out, len(b.String()))
}

func countMethods(types map[string]*typeEntry) int {
	n := 0
	for _, t := range types {
		n += len(t.methods)
	}
	return n
}

// enDescOrNil 返回适合英文文档的描述：优先英文映射表；无映射中英文且
// 不含 CJK 的原样保留；含 CJK 返回空串（英文文档不混入中文）。
func enDescOrNil(name, src string) string {
	if d, ok := enTypeDesc[name]; ok {
		return d
	}
	if src != "" && !containsCJK(src) {
		return src
	}
	return ""
}

// typeDescription 返回类型的英文描述（含 encapsulating Stable 段说明）。
func typeDescription(name, srcDoc string) string {
	if d, ok := enTypeDesc[name]; ok {
		return d
	}
	if srcDoc == "" || containsCJK(srcDoc) {
		return ""
	}
	return srcDoc
}

// containsCJK 判断字符串是否含 CJK 统一表意文字（中文注释过滤）。
func containsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// enTypeDesc 关键公共类型的英文描述（其余类型以签名/结构呈现）。
var enTypeDesc = map[string]string{
	"Presentation":          "The main entry point. Create, open, save, validate, bind, and audit a PPTX document (OLE/OPC package). Methods cover slide management, media, charts, template binding, capability manifest, and timing plans.",
	"Slide":                 "A single page of a presentation. Created via Presentation.AddSlide; exposes text boxes, auto shapes, pictures, charts, media, and page-level properties (id, name, hidden, advance time, notes).",
	"Shape":                 "Unified read-side interface for every shape type: ID, name, kind, bounds, geometry, fill, effects, and style-matrix references.",
	"TextFrame":             "Container for the paragraph/run text model of a text-bearing shape. Provides paragraphs, plain-text setters, and cross-run literal replacement.",
	"Paragraph":             "A paragraph inside a TextFrame. Exposes runs, text, fields, and bullet/tab/paragraph properties.",
	"TextRun":               "A single formatted text run. Read/write text, explicit font and effective font resolution.",
	"AutoShape":             "A `p:sp` shape (text box or auto shape) with optional placeholder role and editable text.",
	"TextShape":             "Alias-shaped handle for a text box (an AutoShape with body text).",
	"GroupShape":            "Read-side handle for a `p:grpSp` group; exposes recursive children.",
	"PictureShape":          "An embedded picture (`p:pic`); exposes source media, fit mode, and alt text.",
	"AudioShape":            "An audio media shape; exposes profile, audio source, role, and playback timing.",
	"VideoShape":            "A video media shape; exposes video profile and source.",
	"OpaqueShape":           "Read-only handle for shape kinds not modeled for editing (groups, connectors, graphic frames).",
	"ChartShape":            "A chart graphic frame; exposes typed chart data (bar/line/pie) with read/write access.",
	"TableShape":            "A table on a page: row/column counts, cell access, merging, and dimension access.",
	"Cell":                  "A single table cell with merge/continuation metadata and a text frame.",
	"EMU":                   "English Metric Unit — the OOXML length unit (1 inch = 914400 EMU).",
	"Point":                 "A 2-D point in EMU coordinates.",
	"Rect":                  "An axis-aligned rectangle in EMU coordinates.",
	"Quad":                  "A 4-corner quadrilateral in EMU coordinates.",
	"SlideID":               "A stable page identifier (uint32).",
	"ShapeID":               "A stable shape identifier (uint32, from `p:cNvPr@id`).",
	"Severity":              "Diagnostic severity level (Info / Warning / Error).",
	"Diagnostic":            "A structured diagnostic entry (code, severity, part, message).",
	"OperationError":        "An error annotated with operation context (op, part, node path, ids).",
	"ChartData":             "Snapshot of a chart's data: type, title, categories, series, and axes.",
	"ChartType":             "Supported chart types: bar, line, pie.",
	"ChartSpec":             "Specification for creating a new chart (size, type, categories, series).",
	"ChartSeries":           "A chart data series (name + numeric values).",
	"MediaSource":           "Abstract media input: file, byte slice, function, or reader adapters.",
	"CapabilityManifest":    "Six-dimension capability report (Inspect / Create / Edit / Preserve / Render / Play).",
	"CapabilityStatus":      "Capability status: Untested / Unsupported / Partial / Supported.",
	"CapabilityFeature":     "A single feature line inside a capability manifest.",
	"SaveReport":            "Result of a save operation: revision, changed parts, diagnostics.",
	"ValidationReport":      "Result of validation: mode and structured diagnostics.",
	"BindReport":            "Result of a template data-binding run.",
	"GeometryInfo":          "Parsed shape geometry (adjusts, guides, paths, commands).",
	"FillInfo":              "Parsed shape fill (solid, gradient, pattern, blip).",
	"EffectInfo":            "Parsed shape effects (shadow, glow, soft edge, 3-D).",
	"LineStyle":             "A shape's line (stroke) style.",
	"StyleMatrixRef":        "A reference into the theme style matrix chain.",
	"FontStyle":             "A public font style (font face, size, bold, italic, color).",
	"FontSize":              "Font size value object in points (with %-relative mode).",
	"RunProps":              "Character run properties.",
	"ParagraphProps":        "Paragraph properties (spacing, bullets, tab stops).",
	"BodyProps":             "Text-body properties (margins, vertical orientation, wrap).",
	"Field":                 "A dynamic field in paragraph text (slide number, datetime, etc.).",
	"FieldKind":             "Field kinds supported for insertion.",
	"FieldSpec":             "Specification for inserting a field.",
	"CoreProperties":        "Document core metadata (title, author, modified timestamp, etc.).",
	"CorePropertiesPatch":   "Patch for updating core properties.",
	"CustomPropertyValue":   "A custom property value (string, bool, integer, datetime, hyperlink).",
	"Placeholder":           "Placeholder metadata of a shape (`p:ph`).",
	"AudioProfile":          "Metadata of an audio embedding (duration, codec, bitrate).",
	"VideoProfile":          "Metadata of a video embedding (duration, dimension).",
	"AudioRole":             "Semantic role of an audio on a page.",
	"VideoRole":             "Semantic role of a video on a page.",
	"ClipTextKind":          "Placeholder content classification.",
	"Optional":              "Generic optional value wrapper.",
	"SplitDir":              "Page split direction (vertical / horizontal / both).",
	"SplitAxis":             "Split position on the main axis.",
	"UnknownDurationPolicy": "Policy for unknown media durations in timing sync.",
	"TimingSyncOptions":     "Options for planning timing synchronization.",
	"TimingSyncReport":      "Report of a timing-sync apply.",
	"TimingPlan":            "A prepared timing plan (page jumps, padding).",
	"PageTiming":            "Timing duration of a single page.",
	"AnimationTimingPolicy": "Strategy when the native timing tree cannot be parsed.",
	"TransitionSpec":        "Specification for a page transition.",
	"TransitionType":        "Transition kinds (fade, push, wipe, etc.).",
	"TransitionDir":         "Transition direction.",
	"TransitionSpeed":       "Transition speed (slow / medium / fast).",
	"ShapeKind":             "Kind of a shape on the page (text box, picture, chart, etc.).",
	"TextFontStyle":         "Font style value object used in text replacement.",
	"ReplaceResult":         "Result of a literal text replacement pass.",
	"ReplaceHit":            "Details of a single replacement location.",
	"ReplaceMode":           "Replacement format strategy.",
	"MultiCellTextPolicy":   "Policy when merging cells that both contain text.",
	"MergeOption":           "Options for a cell merge.",
	"CellRange":             "A rectangular region of table cells.",
	"StyleToggle":           "Toggle for table style features.",
	"TableStyleFlags":       "Feature flags of a table style.",
	"EffectiveCellStyle":    "Resolved effective style of a cell.",
	"CellFill":              "Fill of a table cell.",
	"CellBorder":            "Border of a table cell (one side).",
	"CellBorders":           "All four borders of a table cell.",
	"CellText":              "Text settings of a table cell.",
	"StyleSource":           "Source of a style step in the resolution chain.",
	"StyleStep":             "One step in the style resolution chain.",
	"MatrixRefKind":         "Kind of a style-matrix reference.",
	"ThemeFontSlot":         "Theme font slot (major / minor).",
	"GeomAdjust":            "An adjustment value of a geometry.",
	"GeomGuide":             "A geometry guide.",
	"GeomPath":              "A geometry path.",
	"PathCommand":           "A path command in a custom geometry.",
	"GradientStop":          "A color stop in a gradient fill.",
	"GradientFill":          "A gradient fill.",
	"PatternFill":           "A pattern fill.",
	"BlipFillInfo":          "An image (blip) fill with stretch/crop.",
	"Effect":                "A single visual effect.",
	"Scene3DInfo":           "3-D scene settings.",
	"Shape3DInfo":           "3-D shape settings.",
	"Bevel3D":               "A 3-D bevel.",
	"Camera3D":              "A 3-D camera.",
	"LightRig3D":            "A 3-D lighting rig.",
	"ParsedColor":           "A parsed color value.",
	"ResolvedColor":         "A fully-resolved color (scheme/theme expanded).",
	"ColorTransform":        "A color transform applied to a base color.",
	"ColorSpec":             "A color specification (base + transforms).",
	"ResolvedFont":          "A fully-resolved font.",
	"ResolvedValue":         "A resolved optional value with source set.",
	"ResolveContext":        "Context captured for resolving styles.",
	"CutOptions":            "Options for cutting a range of pages.",
	"FadeOptions":           "Options for a fade transition.",
	"IconMode":              "Mode for the poster icon of a media shape.",
	"PlaybackSpec":          "Specification for media playback.",
	"PlaybackTrigger":       "Trigger for media playback.",
}

func firstDocLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func funcSig(f *ast.FuncDecl) string {
	var buf strings.Builder
	buf.WriteString(f.Name.Name + "(")
	results := formatFieldList(f.Type.Results)
	for i, p := range f.Type.Params.List {
		if i > 0 {
			buf.WriteString(", ")
		}
		if len(p.Names) > 0 {
			buf.WriteString(joinNames(p.Names) + " ")
		}
		buf.WriteString(formatType(p.Type))
	}
	buf.WriteString(")")
	if results != "" {
		buf.WriteString(" " + results)
	}
	return buf.String()
}

func joinNames(names []*ast.Ident) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n.Name)
	}
	return strings.Join(parts, ", ")
}

func recvType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	s := formatType(recv.List[0].Type)
	return strings.TrimPrefix(s, "*")
}

func formatType(e ast.Expr) string {
	switch t := e.(type) {
	case nil:
		return ""
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.X.(*ast.Ident).Name + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + formatType(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + formatType(t.Elt)
		}
		return "[" + typeStr(t.Len) + "]" + formatType(t.Elt)
	case *ast.MapType:
		return "map[" + formatType(t.Key) + "]" + formatType(t.Value)
	case *ast.FuncType:
		return "func(...)"
	case *ast.InterfaceType:
		return "any"
	case *ast.Ellipsis:
		return "..." + formatType(t.Elt)
	default:
		return "*" + fmt.Sprintf("%T", e)
	}
}

func formatFieldList(e *ast.FieldList) string {
	if e == nil || len(e.List) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.List))
	for _, f := range e.List {
		parts = append(parts, formatType(f.Type))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func typeStr(e ast.Expr) string {
	if id, ok := e.(*ast.BasicLit); ok {
		return id.Value
	}
	return "N"
}
