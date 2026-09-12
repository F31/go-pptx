# Changelog

All notable changes to go-pptx will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to a [Semantic API Stability](docs/adr/ADR-015-api-stability-tiers.md) model
(`// Stable:` / `// Experimental:` godoc tags). The per-type assignment is maintained in
[`docs/v1.0-freeze-list.md`](docs/v1.0-freeze-list.md).

## [Unreleased]

### Changed

- Consolidated production edit paths behind `SinglePartPatch` / `MultiPartPlan` helpers via `internal/document`, `internal/textmap`, and `internal/editplan`; low-level `stage*` / `commit` primitives are now confined to the transaction primitive and root adapter boundary.
- Updated architecture/status documentation for ADR-016 and QA-01 corpus status: 36 sample indexes plus 3 public LibreOffice gold samples; L3 PowerPoint/WPS real-machine matrix remains outstanding and is tracked in `docs/client-compat-matrix.md`.
- Added `docs/coverage-roadmap.md` to track the deferred 1.x coverage-improvement work toward the V2.6 90% target.
- Reached COV-01 (full coverprofile total >= 80%) on 2026-09-11: expanded stable enum stringers/value-object tests and `Slide` public accessors (root 78.1%), `internal/xmlstore` scanner/node accessor edges (90.2%), `internal/opc` `PartNameFromEntry`/`EntryNames`/`ChangeSet.IsEmpty` (86.7%), and `ir` pure page-score/timing-helper/core-projection tests (81.4%).
- COV-02 push on 2026-09-11: root moved to 81.5% and full total to 82.5% via capability standalone helpers, Save/Write/Open error guards, bind typed-value helpers, audio/video shape accessors and probe-error mapping, `SetFont` create/expand/replace-fill branches, `TimingTreeRaw` branches, chart `plotElement`/categories/values/canonical/`chartOfGraphic`, notes error branches, `mapOCError`/`parseUint32`, `appendSldIdPatch`, and table style helper edges. Removed never-called dead code (`findTiming`, `Field.pathToField`, `findAudioByShape`).
- COV-02 reached on 2026-09-11 (root 82.0%, full total 82.9%): white-box predicate tests for chart canonical ser/trendline/errBars/title/rich-text/axes, bind patch helpers (`emptyParaPatch`/`deletePatch`/`childElems`), `resolve`/`resolveItems` branches, `scanShapes` traversal/depth, row-loop strict-mode and unsupported-marker rejections, `sourceRef.Open` failure paths, `lastAudioHandle` not-found, `recordAudioProfile` corrupted-reset, and `chartWorkbookPartOf`.
- Closed the `ir` table-text projection gap on 2026-09-11 (`readTableText`/`cellText` were 0%) with a self-contained zip fixture deck via public `OpenReader`; `ir` moved to 86.2% and full total to 83.2%.
- ADR-014 quarterly trigger-condition review on 2026-09-11: no trigger hit (incremental compile 0.56s < 5s, no third-party extension demand, no merge-conflict hotspot); `internal/edit` stays un-extracted.
- COV-04 coverage-target review on 2026-09-11: a flat 90%-per-package target is abandoned; 90% applies only to low-level format packages (`opc`/`xmlstore`/`videoprobe`/`audioprobe`/`textmap`/`editplan`), root SDK target is 85%, command/helper stays 85%. L3 client-compat matrix samples are confirmed ready (`s001`/`s002`/`s003` include `.pptx/.edited.pptx/.odp/.actions.json`, `ext-0024` has manifest + smoke); actual PowerPoint/WPS runs remain blocked on a client-equipped environment.
- L3 PowerPoint/WPS client-compat matrix executed on 2026-09-11 (Windows 11 host, WSL-triggered COM automation): **8/8 combinations passed** — PowerPoint 16.0.20326 and WPS 演示 12.1.0.28599 both opened `s001-text`/`s002-table`/`s003-image`/`ext-0024` edited samples with no repair prompt, resaved as `.pptx`, and every resaved file re-opened by go-pptx with `Validate` errorCount=0 and the edited content preserved. New reproducible tooling `scripts/l3/run_client.sh` + `scripts/l3/ppt_open_resave.ps1`; results recorded in `docs/client-compat-matrix.md` (evidence hashes in `.l3-output/`, gitignored).

### Fixed

- Added a docProps regression covering same-call creation of `core.xml` and `app.xml` root relationships so root rel patches are merged rather than overwritten.

## [1.0.0] - 2026-09-11

The first stable release of go-pptx.

### Highlights

- **Three-tier API stability** — Stable (50 symbols: 33 independent types + 17 error sentinels sharing one aggregated section, 34 `// Stable:` sections total) / API default (120 types, additive evolution allowed) / Experimental (5 types, may change in 1.x). See [`docs/v1.0-freeze-list.md`](docs/v1.0-freeze-list.md) and [ADR-015](docs/adr/ADR-015-api-stability-tiers.md).
- **M0–M8 milestones complete** — full PPTX read/edit stack: OPC engine, XML store with span patches, slide/shape/text/table/chart/audio/video model, capability manifest, semantic diff, template binding.
- **Public corpus** — three LibreOffice-generated public samples (`s001-text` / `s002-table` / `s003-image`) gated by `//go:build corpus` CI job.
- **Cross-platform** — pure Go (CGO=0), CI-verified builds for `js/wasm`, `darwin/arm64`, `wasip1/wasm`, `linux/arm64`.

### Stable API (33 types + 17 error sentinels = 50 symbols)

- **Core entry points (6)** — `Presentation`, `Slide`, `Shape`, `TextFrame`, `Paragraph`, `TextRun`
- **Error sentinel family (17)** — one shared `// Stable:` section in `errors.go`; individual `Err*` constants append-only:
  `ErrClosed`, `ErrStaleHandle`, `ErrInvalidArgument`, `ErrOutOfRange`, `ErrNotFound`, `ErrForeignReference`,
  `ErrUnsupportedFormat`, `ErrUnsupportedEdit`, `ErrLimitExceeded`, `ErrMalformedPackage`, `ErrUnresolvedStyle`,
  `ErrValidationFailed`, `ErrTimingConflict`, `ErrDurationUnknown`, `ErrConcurrentModification`,
  `ErrOutputExists`, `ErrAtomicReplaceUnavailable`
- **Error type** — `OperationError`
- **Diagnostic contract (4)** — `Diagnostic`, `Severity`, `ValidationReport`, `CapabilityStatus`
- **Capability Output family (4 + 2 constants)** — `CapabilityManifest`, `CapabilityManifestSource`,
  `CapabilityDimension`, `CapabilityFeature`; JSON tag set locked, schema-version axis enforced via
  `CapabilityManifestSchemaVersion` / `CapabilityManifestDimensionKey`
- **Geometry value objects (4)** — `EMU`, `Point`, `Rect`, `Quad`
- **Handle ID types (2)** — `SlideID`, `ShapeID` (u32 + documented semantics)
- **Enums (3)** — `ReplaceMode`, `MultiCellTextPolicy`, `ShapeKind`
- **Shape handles (8)** — `GroupShape`, `AutoShape`, `OpaqueShape`, `PictureShape`, `TableShape`,
  `ChartShape`, `AudioShape`, `VideoShape`
- **Type alias** — `TextShape` (alias of `AutoShape`, same Stable contract)

### Experimental API (5, may change in 1.x)

- `ChartWorkbookBuilder` — adapter interface; 1.0 may add methods like `Close()` / `Validate()`
- `ChartDataBook` — workbook snapshot; fields may grow with builder extensions
- `DefaultWorkbookBuilder` — minimum xlsx output; may add fields later (no breakage for users not depending on them)
- `CustomPropertyKind` — OOXML variant enum; iota may grow in 1.x
- `CustomPropertyValue` — multi-field union; may be split by `Kind` in 1.x

### M0–M8 milestones

- **M0** — repo bootstrap (CI three-OS + WASM) + OPC ZIP index + XML scanner/index/patch + vertical validation
- **M1** — relationships/content-types/save plan/atomic save/Presentation skeleton
- **M2** — rich-text model + cross-Run replace + style resolution + image media
- **M3** — units/group matrix + table merge & style + format depth subset
- **M4** — media probe + audio embedding + narration playback
- **M5** — limited three chart types + same-document restricted page clone
- **M6** — transition animation + video shape + text advanced + chart extensions + layout diagnostic + theme style matrix + geometry R-tier
- **M7** — capability manifest + browser-native check tool + animation timing IR + template binding
- **M8** — DIFF-01 semantic diff + cross-document restricted clone + STALE-GUARD handle identity fix

### Implementation highlights

- **OPC**: ZIP index with budget + part discovery + relationships + content types (saving plan with byte-level preservation)
- **XML store**: namespace-aware scanner + node index tree + span patches (additive inserts, controlled namespace) — unknown subtrees preserved
- **Atomic save**: snapshot/restore on failure, no torn writes; default refuses overwrite (`ErrOutputExists`, `WithOverwrite` opt-in)
- **Concurrent-edit guard**: revision-counter snapshot rejects commits from stale sessions (`ErrConcurrentModification`)
- **STALE-GUARD**: shape/text/cell handle identity = cNvPr@id; survives `MoveShape` / `AddShape` / text edits; invalidates only on `RemoveShape`

### Tools

- `pptx capability` — emits 6-dimension capability manifest (`Inspect` / `Create` / `Edit` / `Preserve` / `Render` / `Play`)
- `pptx inspect` — read-only report (geometry / fill / effects / style matrix / layout info)
- `pptx diff` — semantic diff over two documents (`go-pptx.diff/1.0`)
- `pptx bind` — template data binding (`{{path}}` / `{{#if}}` / `{{#each}}`)
- `pptx validate` — diagnostic report (`ValidateOption` for level)
- `pptx check` (WASM) — browser-native privacy-preserving capability / inspect / validate

### Documentation

- [`docs/v1.0-freeze-list.md`](docs/v1.0-freeze-list.md) — full freeze list with 5-phase review trail
- [`docs/adr/ADR-014-root-internal-package-strategy.md`](docs/adr/ADR-014-root-internal-package-strategy.md) — internal package split policy
- [`docs/adr/ADR-015-api-stability-tiers.md`](docs/adr/ADR-015-api-stability-tiers.md) — three-tier stability model
- [`docs/corpus-入库指南.md`](docs/corpus-入库指南.md) — public sample corpus onboarding guide
- [`docs/M6-排序输入.md`](docs/M6-排序输入.md) / [`docs/M7-排序输入.md`](docs/M7-排序输入.md) / [`docs/M8-里程碑总结.md`](docs/M8-里程碑总结.md)
- [`docs/PERF-01-性能基线.md`](docs/PERF-01-性能基线.md) + [`docs/PERF-01-benchmark-report.md`](docs/PERF-01-benchmark-report.md) — performance baseline + CI gate

### Known limitations

- **L3 client matrix not verified** — no PowerPoint/WPS real-machine smoke. Tracked in 实施状态跟踪 §"当前阶段"; per V2.6 §26 P1 this is a hard release gap.
- **Coverage 86.6%** (`cmd/pptx`) / **77.1%** (root). Below V2.6 §15.3 90% target but not a hard release gate per ADR-015 §4.
- **Public corpus** — only 3 LibreOffice-generated samples. Private `ext-*` (33 files) indexed but not redistributed (WPS source / restricted license).

### CI / Build

- `lint` job — `gofmt -l .` + `go vet` (default + `corpus` build tag)
- `corpus-replay` job — public sample replay against latest code
- 4 cross-builds verified per push — `js/wasm`, `darwin/arm64`, `wasip1/wasm`, `linux/arm64`

[1.0.0]: https://github.com/F31/go-pptx/releases/tag/v1.0.0
