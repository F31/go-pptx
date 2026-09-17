# go-pptx API Reference

> Public API of the **`pptx`** package — module `github.com/F31/go-pptx/pptx` (v2.0).
> Stability tiers: `Stable:` methods/types are frozen and backward compatible per ADR-015; all other exported symbols are management contract and may change.
> Live godoc: `go doc github.com/F31/go-pptx/pptx`

## Package Surface

| Item | Count |
|---|---|
| Exported types | 163 |
| Top-level functions | 36 |
| Exported methods | 143 |
| Constants | 149 |
| Package vars & error sentinels | 18 |

## Types

### `AnimationTimingPolicy`

Kind: `type`

> Strategy when the native timing tree cannot be parsed.

### `AudioProfile`

Kind: struct

> Metadata of an audio embedding (duration, codec, bitrate).

### `AudioRole`

Kind: `type`

> Semantic role of an audio on a page.

**Methods:**

- `func (AudioRole) String() string`

### `AudioShape`

Kind: struct

**Stability:** Stable

> An audio media shape; exposes profile, audio source, role, and playback timing.

**Methods:**

- `func (AudioShape) AudioSource() (MediaSource, error)`
- `func (AudioShape) Kind() ShapeKind`
- `func (AudioShape) Profile() AudioProfile`
- `func (AudioShape) Role() AudioRole`
- `func (AudioShape) SetPlayback(spec PlaybackSpec) error`

### `AudioSpec`

Kind: struct

### `AutoShape`

Kind: struct

**Stability:** Stable

> A `p:sp` shape (text box or auto shape) with optional placeholder role and editable text.

**Methods:**

- `func (AutoShape) Kind() ShapeKind`
- `func (AutoShape) Placeholder() (string, uint32, bool)`
- `func (AutoShape) SetAltText(text string) error`
- `func (AutoShape) SetDecorative(decorative bool) error`
- `func (AutoShape) TextFrame() (*TextFrame, error)`

### `AutoShapeSpec`

Kind: struct

### `Bevel3D`

Kind: `type`

> A 3-D bevel.

### `BindOption`

Kind: `type`

### `BindReport`

Kind: struct

> Result of a template data-binding run.

### `BlipFillInfo`

Kind: `type`

> An image (blip) fill with stretch/crop.

### `BodyProps`

Kind: `type`

> Text-body properties (margins, vertical orientation, wrap).

### `Bullet`

Kind: `type`

### `BulletKind`

Kind: `type`

### `Camera3D`

Kind: `type`

> A 3-D camera.

### `CapabilityDimension`

Kind: struct

### `CapabilityFeature`

Kind: struct

> A single feature line inside a capability manifest.

### `CapabilityManifest`

Kind: struct

> Six-dimension capability report (Inspect / Create / Edit / Preserve / Render / Play).

### `CapabilityManifestSource`

Kind: struct

### `CapabilityStatus`

Kind: `type`

> Capability status: Untested / Unsupported / Partial / Supported.

**Methods:**

- `func (CapabilityStatus) MarshalJSON() ([]byte, error)`
- `func (CapabilityStatus) String() string`
- `func (CapabilityStatus) UnmarshalJSON(b []byte) error`

### `Cell`

Kind: struct

> A single table cell with merge/continuation metadata and a text frame.

**Methods:**

- `func (Cell) Column() int`
- `func (Cell) EffectiveCellStyle() (EffectiveCellStyle, []Diagnostic, error)`
- `func (Cell) IsContinuation() (bool, error)`
- `func (Cell) IsMerged() (bool, error)`
- `func (Cell) Row() int`
- `func (Cell) Spans() (int, error)`
- `func (Cell) TextFrame() (*TextFrame, error)`

### `CellBorder`

Kind: `type`

> Border of a table cell (one side).

### `CellBorders`

Kind: `type`

> All four borders of a table cell.

### `CellFill`

Kind: `type`

> Fill of a table cell.

### `CellRange`

Kind: struct

> A rectangular region of table cells.

### `CellText`

Kind: `type`

> Text settings of a table cell.

### `ChartAxisOptions`

Kind: `type`

### `ChartData`

Kind: `type`

> Snapshot of a chart's data: type, title, categories, series, and axes.

### `ChartDataBook`

Kind: `type`

### `ChartDataLabel`

Kind: `type`

### `ChartErrorBars`

Kind: `type`

### `ChartErrorType`

Kind: `type`

### `ChartSeries`

Kind: `type`

> A chart data series (name + numeric values).

### `ChartShape`

Kind: struct

**Stability:** Stable

> A chart graphic frame; exposes typed chart data (bar/line/pie) with read/write access.

**Methods:**

- `func (ChartShape) Data() (ChartData, error)`
- `func (ChartShape) DataWithDiagnostics() (ChartData, []Diagnostic, error)`
- `func (ChartShape) Kind() ShapeKind`
- `func (ChartShape) SetAltText(text string) error`
- `func (ChartShape) SetData(cd ChartData) error`
- `func (ChartShape) SetDecorative(decorative bool) error`

### `ChartSpec`

Kind: `type`

> Specification for creating a new chart (size, type, categories, series).

### `ChartTrendType`

Kind: `type`

### `ChartTrendline`

Kind: `type`

### `ChartType`

Kind: `type`

> Supported chart types: bar, line, pie.

### `ChartWorkbookBuilder`

Kind: interface

### `ClonePolicy`

Kind: struct

### `ColorSpec`

Kind: `type`

> A color specification (base + transforms).

### `ColorTransform`

Kind: `type`

> A color transform applied to a base color.

### `CoreProperties`

Kind: struct

> Document core metadata (title, author, modified timestamp, etc.).

### `CorePropertiesPatch`

Alias of `CoreProperties`.

> Patch for updating core properties.

### `CustomPropertyKind`

Kind: `type`

### `CustomPropertyValue`

Kind: struct

> A custom property value (string, bool, integer, datetime, hyperlink).

### `CutOptions`

Kind: struct

> Options for cutting a range of pages.

### `DefaultWorkbookBuilder`

Kind: struct

**Methods:**

- `func (DefaultWorkbookBuilder) Build(book ChartDataBook) ([]byte, error)`

### `Diagnostic`

Kind: `type`

> A structured diagnostic entry (code, severity, part, message).

### `EMU`

Kind: `type`

> English Metric Unit — the OOXML length unit (1 inch = 914400 EMU).

### `Effect`

Kind: `type`

> A single visual effect.

### `EffectInfo`

Kind: `type`

> Parsed shape effects (shadow, glow, soft edge, 3-D).

### `EffectKind`

Kind: `type`

### `EffectiveCellStyle`

Kind: `type`

> Resolved effective style of a cell.

### `EffectsProvider`

Kind: interface

### `FadeOptions`

Kind: struct

> Options for a fade transition.

### `Field`

Kind: struct

> A dynamic field in paragraph text (slide number, datetime, etc.).

**Methods:**

- `func (Field) Guide() (string, error)`
- `func (Field) Kind() (FieldKind, error)`
- `func (Field) Remove() error`
- `func (Field) SetText(text string) error`
- `func (Field) Text() (string, error)`

### `FieldKind`

Kind: `type`

> Field kinds supported for insertion.

### `FieldSpec`

Kind: `type`

> Specification for inserting a field.

### `FillInfo`

Kind: `type`

> Parsed shape fill (solid, gradient, pattern, blip).

### `FillKind`

Kind: `type`

### `FillProvider`

Kind: interface

### `FontProperty`

Kind: `type`

### `FontSize`

Kind: `type`

> Font size value object in points (with %-relative mode).

### `FontStyle`

Kind: `type`

> A public font style (font face, size, bold, italic, color).

### `GeomAdjust`

Kind: `type`

> An adjustment value of a geometry.

### `GeomGuide`

Kind: `type`

> A geometry guide.

### `GeomPath`

Kind: `type`

> A geometry path.

### `GeometryInfo`

Kind: `type`

> Parsed shape geometry (adjusts, guides, paths, commands).

### `GeometryKind`

Kind: `type`

### `GeometryProvider`

Kind: interface

### `GradientFill`

Kind: `type`

> A gradient fill.

### `GradientStop`

Kind: `type`

> A color stop in a gradient fill.

### `GroupShape`

Kind: struct

**Stability:** Stable

> Read-side handle for a `p:grpSp` group; exposes recursive children.

**Methods:**

- `func (GroupShape) Children() ([]Shape, error)`
- `func (GroupShape) Kind() ShapeKind`

### `HandoutMasterInfo`

Kind: struct

### `IconMode`

Kind: `type`

> Mode for the poster icon of a media shape.

**Methods:**

- `func (IconMode) String() string`

### `KinsokuRule`

Kind: struct

### `LayoutEmbeddedFont`

Kind: struct

### `LayoutRef`

Kind: struct

**Methods:**

- `func (LayoutRef) Name() string`

### `LayoutReport`

Kind: struct

### `LayoutSection`

Kind: struct

### `LightRig3D`

Kind: `type`

> A 3-D lighting rig.

### `LineEnd`

Kind: `type`

### `LineProvider`

Kind: interface

### `LineStyle`

Kind: `type`

> A shape's line (stroke) style.

### `MatrixRefKind`

Kind: `type`

> Kind of a style-matrix reference.

### `MediaSource`

Kind: `type`

> Abstract media input: file, byte slice, function, or reader adapters.

### `MergeOption`

Kind: `type`

> Options for a cell merge.

### `MultiCellTextPolicy`

Kind: `type`

> Policy when merging cells that both contain text.

### `NewOption`

Kind: `type`

### `OpaqueShape`

Kind: struct

**Stability:** Stable

> Read-only handle for shape kinds not modeled for editing (groups, connectors, graphic frames).

**Methods:**

- `func (OpaqueShape) Kind() ShapeKind`

### `OpenOption`

Kind: `type`

### `OperationError`

Kind: `type`

> An error annotated with operation context (op, part, node path, ids).

### `Optional`

Kind: `type`

> Generic optional value wrapper.

### `PageTiming`

Kind: struct

> Timing duration of a single page.

### `Paragraph`

Kind: struct

**Stability:** Stable

> A paragraph inside a TextFrame. Exposes runs, text, fields, and bullet/tab/paragraph properties.

**Methods:**

- `func (Paragraph) AddRun(s string, style FontStyle) (*TextRun, error)`
- `func (Paragraph) AppendField(spec FieldSpec) (*Field, error)`
- `func (Paragraph) Fields() ([]*Field, error)`
- `func (Paragraph) InsertField(at *TextRun, spec FieldSpec) (*Field, error)`
- `func (Paragraph) Props() (ParagraphProps, []Diagnostic, error)`
- `func (Paragraph) ReplaceText(old, replacement string, opts ...ReplaceOption) (ReplaceResult, error)`
- `func (Paragraph) Runs() ([]*TextRun, error)`
- `func (Paragraph) Text() (string, error)`

### `ParagraphProps`

Kind: `type`

> Paragraph properties (spacing, bullets, tab stops).

### `ParagraphSpec`

Kind: struct

### `ParsedColor`

Kind: `type`

> A parsed color value.

### `PathCommand`

Kind: `type`

> A path command in a custom geometry.

### `PatternFill`

Kind: `type`

> A pattern fill.

### `PictureFitMode`

Kind: `type`

**Methods:**

- `func (PictureFitMode) String() string`

### `PictureShape`

Kind: struct

**Stability:** Stable

> An embedded picture (`p:pic`); exposes source media, fit mode, and alt text.

**Methods:**

- `func (PictureShape) Kind() ShapeKind`
- `func (PictureShape) ReplaceImage(ctx context.Context, src MediaSource) error`
- `func (PictureShape) SetAltText(text string) error`
- `func (PictureShape) SetDecorative(decorative bool) error`

### `PictureSpec`

Kind: struct

### `Placeholder`

Kind: struct

> Placeholder metadata of a shape (`p:ph`).

### `PlaybackSpec`

Kind: struct

> Specification for media playback.

### `PlaybackTrigger`

Kind: `type`

> Trigger for media playback.

**Methods:**

- `func (PlaybackTrigger) String() string`

### `Point`

Kind: `type`

> A 2-D point in EMU coordinates.

### `Presentation`

Kind: struct

> The main entry point. Create, open, save, validate, bind, and audit a PPTX document (OLE/OPC package). Methods cover slide management, media, charts, template binding, capability manifest, and timing plans.

**Methods:**

- `func (Presentation) AddSlide(layout *LayoutRef) (*Slide, error)`
- `func (Presentation) ApplyTimingPlan(_ context.Context, plan TimingPlan) (TimingSyncReport, error)`
- `func (Presentation) Bind(data map[string]any, opts ...BindOption) (BindReport, error)`
- `func (Presentation) Capability(sourcePath string) CapabilityManifest`
- `func (Presentation) Close() error`
- `func (Presentation) CopySlideFrom(src *Slide, policy *ClonePolicy) (*Slide, error)`
- `func (Presentation) CoreProperties() (CoreProperties, error)`
- `func (Presentation) CustomProperties() (map[string]CustomPropertyValue, error)`
- `func (Presentation) DebugAudioXML() string`
- `func (Presentation) DebugVideoXML() string`
- `func (Presentation) LayoutInfo() (*LayoutReport, error)`
- `func (Presentation) Layouts() ([]*LayoutRef, error)`
- `func (Presentation) MoveSlide(id SlideID, index int) error`
- `func (Presentation) PlanTimingSync(_ context.Context, opts TimingSyncOptions) (TimingPlan, error)`
- `func (Presentation) RemoveSlide(id SlideID) error`
- `func (Presentation) Revision() uint64`
- `func (Presentation) Save(ctx context.Context, path string, opts ...SaveOption) (SaveReport, error)`
- `func (Presentation) SetChartWorkbookBuilder(b ChartWorkbookBuilder) error`
- `func (Presentation) SetCoreProperties(patch CorePropertiesPatch) error`
- `func (Presentation) SetCustomProperty(name string, value CustomPropertyValue) error`
- `func (Presentation) Slide(index int) (*Slide, error)`
- `func (Presentation) SlideByID(id SlideID) (*Slide, error)`
- `func (Presentation) Slides() ([]*Slide, error)`
- `func (Presentation) SyncTimingToAudio(ctx context.Context, opts TimingSyncOptions) (TimingSyncReport, error)`
- `func (Presentation) Validate(ctx context.Context, opts ...ValidateOption) ValidationReport`
- `func (Presentation) Write(ctx context.Context, w io.Writer, opts ...SaveOption) (SaveReport, error)`

### `Quad`

Kind: struct

> A 4-corner quadrilateral in EMU coordinates.

### `Rect`

Kind: struct

> An axis-aligned rectangle in EMU coordinates.

**Methods:**

- `func (Rect) Bottom() EMU`
- `func (Rect) Contains(p Point) bool`
- `func (Rect) Right() EMU`

### `ReplaceHit`

Kind: struct

> Details of a single replacement location.

### `ReplaceMode`

Kind: `type`

> Replacement format strategy.

**Methods:**

- `func (ReplaceMode) String() string`

### `ReplaceOption`

Kind: `type`

### `ReplaceResult`

Kind: struct

> Result of a literal text replacement pass.

### `ResolveContext`

Kind: `type`

> Context captured for resolving styles.

### `ResolvedColor`

Kind: `type`

> A fully-resolved color (scheme/theme expanded).

### `ResolvedFont`

Kind: `type`

> A fully-resolved font.

### `ResolvedValue`

Kind: `type`

> A resolved optional value with source set.

### `RunProps`

Kind: `type`

> Character run properties.

### `RunSymbol`

Kind: `type`

### `SaveOption`

Kind: `type`

### `SaveReport`

Kind: struct

> Result of a save operation: revision, changed parts, diagnostics.

### `Scene3DInfo`

Kind: `type`

> 3-D scene settings.

### `Severity`

Kind: `type`

> Diagnostic severity level (Info / Warning / Error).

### `Shape`

Kind: interface

> Unified read-side interface for every shape type: ID, name, kind, bounds, geometry, fill, effects, and style-matrix references.

### `Shape3DInfo`

Kind: `type`

> 3-D shape settings.

### `ShapeID`

Kind: `type`

> A stable shape identifier (uint32, from `p:cNvPr@id`).

### `ShapeKind`

Kind: `type`

**Stability:** Stable

> Kind of a shape on the page (text box, picture, chart, etc.).

**Methods:**

- `func (ShapeKind) String() string`

### `Slide`

Kind: struct

> A single page of a presentation. Created via Presentation.AddSlide; exposes text boxes, auto shapes, pictures, charts, media, and page-level properties (id, name, hidden, advance time, notes).

**Methods:**

- `func (Slide) AddAudio(ctx context.Context, src MediaSource, spec AudioSpec) (*AudioShape, error)`
- `func (Slide) AddAutoShape(spec AutoShapeSpec) (*AutoShape, error)`
- `func (Slide) AddChart(ctx context.Context, spec ChartSpec) (*ChartShape, error)`
- `func (Slide) AddPicture(ctx context.Context, src MediaSource, spec PictureSpec) (*PictureShape, error)`
- `func (Slide) AddTextBox(spec TextBoxSpec) (*TextShape, error)`
- `func (Slide) AddVideo(ctx context.Context, src MediaSource, spec VideoSpec) (*VideoShape, error)`
- `func (Slide) AdvanceAfter() (time.Duration, bool, error)` `[Stable]`
- `func (Slide) Clone(policy *ClonePolicy) (*Slide, error)`
- `func (Slide) EnsureSpeakerNotes() (*TextFrame, error)`
- `func (Slide) HasTiming() bool`
- `func (Slide) Hidden() (bool, error)` `[Stable]`
- `func (Slide) ID() SlideID`
- `func (Slide) MoveShape(id ShapeID, zIndex int) error`
- `func (Slide) Name() string`
- `func (Slide) NotesPart() string`
- `func (Slide) PartName() string`
- `func (Slide) Placeholders() ([]*Placeholder, error)`
- `func (Slide) RemoveShape(id ShapeID) error`
- `func (Slide) RemoveTransition() error`
- `func (Slide) SetAdvanceAfter(d time.Duration) error`
- `func (Slide) SetSpeakerNotes(text string) error`
- `func (Slide) SetTransition(spec TransitionSpec) error`
- `func (Slide) Shapes() ([]Shape, error)`
- `func (Slide) SpeakerNotes() (*TextFrame, error)`
- `func (Slide) SpeakerNotesText() (string, error)`
- `func (Slide) TimingTreeRaw() ([]byte, []Diagnostic, error)`
- `func (Slide) Transition() (TransitionSpec, error)`
- `func (Slide) UpsertNarration(ctx context.Context, src MediaSource, spec AudioSpec, pb PlaybackSpec) (*AudioShape, string, error)`

### `SlideID`

Kind: `type`

> A stable page identifier (uint32).

### `Spacing`

Kind: `type`

### `SplitAxis`

Kind: `type`

> Split position on the main axis.

### `SplitDir`

Kind: `type`

> Page split direction (vertical / horizontal / both).

### `StyleMatrixRef`

Kind: `type`

> A reference into the theme style matrix chain.

### `StyleMatrixRefsProvider`

Kind: interface

### `StylePart`

Kind: `type`

### `StyleSource`

Kind: `type`

> Source of a style step in the resolution chain.

### `StyleStep`

Kind: `type`

> One step in the style resolution chain.

### `StyleToggle`

Kind: `type`

> Toggle for table style features.

### `TabStop`

Kind: `type`

### `TableShape`

Kind: struct

**Stability:** Stable

> A table on a page: row/column counts, cell access, merging, and dimension access.

**Methods:**

- `func (TableShape) Cell(row, col int) (*Cell, error)`
- `func (TableShape) ColumnCount() (int, error)`
- `func (TableShape) ColumnWidth(col int) (EMU, error)`
- `func (TableShape) Kind() ShapeKind`
- `func (TableShape) Merge(r CellRange, opts ...MergeOption) error`
- `func (TableShape) RowCount() (int, error)`
- `func (TableShape) RowHeight(row int) (EMU, error)`
- `func (TableShape) SetColumnWidth(col int, w EMU) error`
- `func (TableShape) SetRowHeight(row int, h EMU) error`
- `func (TableShape) StyleFlags() (TableStyleFlags, error)`
- `func (TableShape) StyleID() (string, error)`
- `func (TableShape) Unmerge(r CellRange) error`

### `TableStyleFlags`

Kind: `type`

> Feature flags of a table style.

### `TextBoxSpec`

Kind: struct

### `TextFrame`

Kind: struct

**Stability:** Stable

> Container for the paragraph/run text model of a text-bearing shape. Provides paragraphs, plain-text setters, and cross-run literal replacement.

**Methods:**

- `func (TextFrame) AddParagraph(spec ParagraphSpec) (*Paragraph, error)`
- `func (TextFrame) BodyProps() (BodyProps, error)`
- `func (TextFrame) Paragraphs() ([]*Paragraph, error)`
- `func (TextFrame) ReplaceText(old, replacement string, opts ...ReplaceOption) (ReplaceResult, error)`
- `func (TextFrame) SetBodyProps(p BodyProps) error`
- `func (TextFrame) SetPlainText(text string) error`

### `TextRun`

Kind: struct

**Stability:** Stable

> A single formatted text run. Read/write text, explicit font and effective font resolution.

**Methods:**

- `func (TextRun) AdvancedProps() (RunProps, []Diagnostic, error)`
- `func (TextRun) EffectiveFont(ctx ResolveContext) (ResolvedFont, []Diagnostic, error)`
- `func (TextRun) ExplicitFont() (FontStyle, error)`
- `func (TextRun) ResetFontProperty(prop FontProperty) error`
- `func (TextRun) SetFont(style FontStyle) error`
- `func (TextRun) SetText(text string) error`
- `func (TextRun) Text() (string, error)`

### `TextShape`

Alias of `AutoShape`.

**Stability:** Stable

> Alias-shaped handle for a text box (an AutoShape with body text).

### `ThemeFontSlot`

Kind: `type`

> Theme font slot (major / minor).

### `TimingPlan`

Kind: struct

> A prepared timing plan (page jumps, padding).

### `TimingSyncOptions`

Kind: struct

> Options for planning timing synchronization.

### `TimingSyncReport`

Kind: struct

> Report of a timing-sync apply.

### `TrackContribution`

Kind: struct

### `TransitionDir`

Kind: `type`

> Transition direction.

### `TransitionSpec`

Kind: struct

> Specification for a page transition.

### `TransitionSpeed`

Kind: `type`

> Transition speed (slow / medium / fast).

### `TransitionType`

Kind: `type`

> Transition kinds (fade, push, wipe, etc.).

### `UnknownDurationPolicy`

Kind: `type`

> Policy for unknown media durations in timing sync.

### `ValidateOption`

Kind: `type`

### `ValidationReport`

Kind: `type`

> Result of validation: mode and structured diagnostics.

### `VideoProfile`

Kind: struct

> Metadata of a video embedding (duration, dimension).

### `VideoRole`

Kind: `type`

> Semantic role of a video on a page.

**Methods:**

- `func (VideoRole) String() string`

### `VideoShape`

Kind: struct

**Stability:** Stable

> A video media shape; exposes video profile and source.

**Methods:**

- `func (VideoShape) HasPoster() bool`
- `func (VideoShape) Kind() ShapeKind`
- `func (VideoShape) PosterSource() (MediaSource, error)`
- `func (VideoShape) Profile() VideoProfile`
- `func (VideoShape) Role() VideoRole`
- `func (VideoShape) VideoSource() (MediaSource, error)`

### `VideoSpec`

Kind: struct

## Functions

- `func Annotate(err error, op string) error`
- `func BoolPtr(b bool) *bool`
- `func BooleanCustomProperty(v bool) CustomPropertyValue`
- `func BytesMedia(data []byte, contentType string) MediaSource`
- `func DateTimeCustomProperty(v time.Time) CustomPropertyValue`
- `func EMUFromInches(v float64) (EMU, error)`
- `func EMUFromPoints(v float64) (EMU, error)`
- `func FileMedia(path string) MediaSource`
- `func FuncMedia(open func(...), contentType string) MediaSource`
- `func IntegerCustomProperty(v int64) CustomPropertyValue`
- `func MainPartBytes(p *Presentation) ([]byte, bool)`
- `func MarshalManifest(m CapabilityManifest) ([]byte, error)`
- `func MarshalManifestIndent(m CapabilityManifest, indent string) ([]byte, error)`
- `func New(opts ...NewOption) (*Presentation, error)`
- `func NewCapabilityManifest() CapabilityManifest`
- `func NewOptional(v T) **ast.IndexExpr`
- `func Open(path string, opts ...OpenOption) (*Presentation, error)`
- `func OpenReader(r io.ReaderAt, size int64, opts ...OpenOption) (*Presentation, error)`
- `func PartBytes(p *Presentation, name opc.PartName) ([]byte, bool)`
- `func PopulateCapabilityDimensions(m *CapabilityManifest)`
- `func PopulateCapabilityFeatures(m *CapabilityManifest)`
- `func Pts(v float64) FontSize`
- `func ReaderMedia(r io.Reader, contentType string) (MediaSource, error)`
- `func SortCapabilityFeatures(m *CapabilityManifest)`
- `func StringCustomProperty(v string) CustomPropertyValue`
- `func UnmarshalManifest(b []byte) (CapabilityManifest, error)`
- `func WithBindReplaceMode(m ReplaceMode) BindOption`
- `func WithBindStrict(strict bool) BindOption`
- `func WithBudget(b opc.Budget) OpenOption`
- `func WithMergeTextPolicy(p MultiCellTextPolicy) MergeOption`
- `func WithNewBudget(b opc.Budget) NewOption`
- `func WithNewTemplate(parts map[string][]byte) NewOption`
- `func WithReplaceMode(m ReplaceMode) ReplaceOption`
- `func WithReplacementStyle(style FontStyle) ReplaceOption`
- `func WithSaveDurability(d opc.Durability) SaveOption`
- `func WithSaveOverwrite(v bool) SaveOption`

## Constants

- `AnimationTimingKeepExisting`
- `AnimationTimingReject`
- `AudioRoleBackground`
- `AudioRoleEffect`
- `AudioRoleNarration`
- `AxisHor`
- `AxisVert`
- `BulletAutoNum`
- `BulletBlip`
- `BulletChar`
- `BulletNone`
- `BulletUnknown`
- `CapabilityCreate`
- `CapabilityEdit`
- `CapabilityInspect`
- `CapabilityManifestSchemaVersion`
- `CapabilityPlay`
- `CapabilityPreserve`
- `CapabilityRender`
- `ChartBar`
- `ChartErrFixed`
- `ChartErrPercentage`
- `ChartErrStandardDeviation`
- `ChartErrStandardError`
- `ChartLine`
- `ChartPie`
- `ChartTrendExponential`
- `ChartTrendLinear`
- `ChartTrendLogarithmic`
- `ChartTrendMovingAverage`
- `ChartTrendPolynomial`
- `ChartTrendPower`
- `CustomPropertyBoolean`
- `CustomPropertyDateTime`
- `CustomPropertyInteger`
- `CustomPropertyString`
- `DirDown`
- `DirLeft`
- `DirRight`
- `DirUp`
- `EMUPerInch`
- `EMUPerPoint`
- `EffectBlur`
- `EffectFillOverlay`
- `EffectGlow`
- `EffectInnerShadow`
- `EffectNone`
- `EffectOuterShadow`
- `EffectReflection`
- `EffectSoftEdge`
- `EffectUnknown`
- `FieldDateTime`
- `FieldSlideNumber`
- `FillGradient`
- `FillGroup`
- `FillNone`
- `FillPattern`
- `FillPicture`
- `FillSolid`
- `FillUnspecified`
- `FitContain`
- `FitCover`
- `FitOriginalSize`
- `FitStretch`
- `FontPropBold`
- `FontPropColor`
- `FontPropComplexScript`
- `FontPropEastAsian`
- `FontPropItalic`
- `FontPropLatin`
- `FontPropSize`
- `FontSlotMajor`
- `FontSlotMinor`
- `FontSlotUnknown`
- `GeometryCustom`
- `GeometryPreset`
- `GeometryUnknown`
- `IconHiddenDuringShow`
- `IconVisible`
- `MergeKeepAnchorText`
- `MergeRejectMultipleText`
- `PartBand1H`
- `PartBand1V`
- `PartBand2H`
- `PartBand2V`
- `PartFirstCol`
- `PartFirstRow`
- `PartLastCol`
- `PartLastRow`
- `PartNECell`
- `PartNWCell`
- `PartSECell`
- `PartSWCell`
- `PartWholeTable`
- `PlaybackOnClick`
- `PlaybackOnSlideEnter`
- `RefEffect`
- `RefFill`
- `RefFont`
- `RefLine`
- `ReplaceEqualLengthPerRune`
- `ReplaceExplicitStyle`
- `ReplaceFirstCharacter`
- `SeverityError`
- `SeverityInfo`
- `SeverityWarning`
- `ShapeAudio`
- `ShapeAutoShape`
- `ShapeChart`
- `ShapeConnector`
- `ShapeGraphicFrame`
- `ShapeGroup`
- `ShapeOpaque`
- `ShapePicture`
- `ShapeTable`
- `ShapeTextBox`
- `ShapeVideo`
- `SourceCellExplicit`
- `SourceFallback`
- `SourceListStyle`
- `SourceParagraphDefault`
- `SourceRun`
- `SourceTableStyle`
- `SourceTheme`
- `SpeedFast`
- `SpeedMed`
- `SpeedSlow`
- `SplitIn`
- `SplitOut`
- `StatusPartial`
- `StatusSupported`
- `StatusUnsupported`
- `StatusUntested`
- `ToggleDefault`
- `ToggleOff`
- `ToggleOn`
- `TransitionCover`
- `TransitionCut`
- `TransitionDissolve`
- `TransitionFade`
- `TransitionNone`
- `TransitionPush`
- `TransitionSplit`
- `TransitionWipe`
- `UnknownDurationFail`
- `UnknownDurationSkip`
- `VideoRoleBackground`
- `VideoRoleMain`
- `VideoRoleTrim`

## Package Variables & Error Sentinels

- `ErrAtomicReplaceUnavailable`
- `ErrClosed`
- `ErrConcurrentModification`
- `ErrDurationUnknown`
- `ErrForeignReference`
- `ErrInvalidArgument`
- `ErrLimitExceeded`
- `ErrMalformedPackage`
- `ErrNotFound`
- `ErrOutOfRange`
- `ErrOutputExists`
- `ErrStaleHandle`
- `ErrTimingConflict`
- `ErrUnresolvedStyle`
- `ErrUnsupportedEdit`
- `ErrUnsupportedFormat`
- `ErrValidationFailed`
- `SDKVersion`

---

*Generated by `scripts/gen/apidoc`. Regenerate: `go run ./scripts/gen/apidoc`.*
