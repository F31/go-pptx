# Coverage Roadmap

Date: 2026-09-11

This document tracks the 1.x coverage-improvement work that was deferred from the v1.0 hard gate. It is not a v1.0 release blocker, but it is the active plan for moving toward the V2.6 90% target.

## Current Snapshot

Command:

```bash
go test ./... -coverprofile=/tmp/opencode/go-pptx-cover.out
go tool cover -func=/tmp/opencode/go-pptx-cover.out
```

Package snapshot (2026-09-11, COV-02 reached):

| Package | Coverage |
|---|---:|
| `github.com/F31/go-pptx` | 82.0% |
| `github.com/F31/go-pptx/cmd/pptx` | 85.6% |
| `github.com/F31/go-pptx/internal/audioprobe` | 83.0% |
| `github.com/F31/go-pptx/internal/editplan` | 81.8% |
| `github.com/F31/go-pptx/internal/opc` | 86.7% |
| `github.com/F31/go-pptx/internal/textmap` | 82.2% |
| `github.com/F31/go-pptx/internal/videoprobe` | 92.1% |
| `github.com/F31/go-pptx/internal/xmlstore` | 90.2% |
| `github.com/F31/go-pptx/ir` | 86.2% |
| `github.com/F31/go-pptx/render` | 84.2% |
| `github.com/F31/go-pptx/scripts/perf/summarize` | 86.4% |
| `github.com/F31/go-pptx/wasm/check` | 85.9% |
| full coverprofile total | 83.2% |

Recent movement:

- 2026-09-11: added `scripts/perf/summarize` unit tests for benchmark parsing, metric aggregation, report generation, and formatting helpers; package coverage moved from 0.0% to 86.4%, full total from 76.2% to 77.6%.
- 2026-09-11: added IR projection tests for text shape projection and diagnostic ordering; `ir` moved from 72.7% to 75.6%, full total from 77.6% to 77.8%.
- 2026-09-11: added `cmd/pptx` helper tests for bind usage, context preservation, output file creation/refusal, and output path resolution; `cmd/pptx` moved from 84.9% to 85.6%.
- 2026-09-11: added `wasm/check` error-envelope tests for `FailJSON` and `AsError` edge cases; `wasm/check` moved from 78.9% to 85.9%, full total from 77.8% to 77.9%.
- 2026-09-11: added root SDK media adapter tests for `FuncMedia`, `ReaderMedia`, `readBounded`, and nil stream handling; root moved from 76.6% to 76.7%, full total from 77.9% to 78.0%.
- 2026-09-11: added `internal/videoprobe` tests for malformed error wrapping, duration normalization, MP4 malformed branches, and WebM doc type detection; package moved from 74.2% to 92.1%, full total from 78.0% to 78.1%.
- 2026-09-11: added `internal/audioprobe` tests for WAV format variants, malformed chunks, RF64/ds64 duration, MP3 ID3 malformed cases, VBRI duration, and helper edges; package moved from 70.4% to 83.0%, full total from 78.1% to 78.3%.
- 2026-09-11: added root SDK tests for stable option constructors and enum stringers (`ReplaceMode`, `ShapeKind`); root moved from 76.7% to 77.1%, full total from 78.3% to 78.6%.
- 2026-09-11: added root SDK tests for the remaining stable enum stringers (audio/video roles, chart types, bullets, style parts, value-object strings) and `Slide` public accessors; root moved from 77.1% to 78.1%.
- 2026-09-11: added `internal/xmlstore` tests for node/attribute accessors, scanner `ErrOffset`/`Depth`/`TokenKind.String`, and error formatting; package moved from 86.4% to 90.2%.
- 2026-09-11: added `internal/opc` tests for `PartNameFromEntry`, `EntryNames` copy semantics, and `ChangeSet.IsEmpty`; package moved from 84.0% to 86.7%.
- 2026-09-11: added `ir` tests for pure page-score/box helpers, timing projection helper edges, and core-properties projection; `ir` moved from 75.6% to 81.4%, full total to 80.0%.
- 2026-09-11: COV-02 push in root: capability standalone helpers, Save/Write/Open error guards, bind typed-member/truthy/format/directive helpers, audio/video shape accessors + probe error mapping, `SetFont` create/expand/replace-fill branches, `TimingTreeRaw` branches, chart `plotElement`/categories/values/canonical/`chartOfGraphic`, notes error branches, `mapOCError`/`parseUint32`, `appendSldIdPatch`, table style helper edges; root moved from 78.1% to 81.5%, full total to 82.5%. Removed dead code `findTiming`, `Field.pathToField`, `findAudioByShape` (never called), which also reduced statement denominator.
- 2026-09-11: COV-02 reached (root >= 82%): additional white-box predicate tests for chart canonical ser/trendline/errBars/title/rich-text/axes, bind patch helpers (`emptyParaPatch`/`deletePatch`/`childElems`), `resolve`/`resolveItems` branches, `scanShapes` traversal/depth, row-loop strict-mode and unsupported-marker rejections, `sourceRef.Open` failure paths, `lastAudioHandle` not-found, `recordAudioProfile` corrupted-reset, `chartWorkbookPartOf`; root moved from 81.5% to 82.0%, full total to 82.9%.
- 2026-09-11: closed the `ir` table-text projection gap (`readTableText`/`cellText` were 0%) with a self-contained zip fixture deck (public `OpenReader`) covering 2×2 text, empty cell, zero-row, and non-positive dimension edges; `ir` moved from 81.4% to 86.2%, full total to 83.2%.

## Rules

1. Prefer behavior tests over tests that only call trivial getters for coverage.
2. Do not add brittle tests that assert full OOXML serialization unless the behavior is already a public guarantee.
3. When a package is a command/helper package, either add useful command tests or explicitly exclude it from release coverage accounting; do not let a 0% helper distort SDK coverage without a decision.
4. Coverage work must keep `go test ./...` and `scripts/gen_corpus/run.sh validate testdata/corpus` green.

## Priority Backlog

| Priority | Area | Current Signal | Suggested Tests |
|---|---|---|---|
| Done | COV-01 | full total 80.0% | Milestone reached; full total now 82.5% |
| Done | `scripts/perf/summarize` | 86.4% | Keep tests focused on parser/merge/report behavior; no further action unless the helper changes |
| Done | `internal/videoprobe` | 92.1% | Malformed error helpers, MP4/WebM edge variants, and duration wrappers covered |
| Done | `internal/audioprobe` | 83.0% | First pass complete; remaining gains are deeper MP3 frame-scanner and malformed-header branches |
| Done | `wasm/check` | 85.9% | Error envelope and `AsError` edge coverage added; revisit only if wasm API changes |
| Done | `internal/xmlstore` | 90.2% | Scanner accessors, node helpers, and error formatting covered |
| Done | `internal/opc` | 86.7% | `PartNameFromEntry`, `EntryNames`, `ChangeSet.IsEmpty` covered |
| Done | root `pptx` | 82.0% | **COV-02 reached 2026-09-11** |
| Done | `ir` | 86.2% | Table-text projection (`readTableText`/`cellText`) closed with a self-contained zip fixture deck |
| P2 | `cmd/pptx` | 85.6% | Near COV-03 target; remaining subcommand edge paths and JSON output variants |
| P3 | `internal/audioprobe` | 83.0% | Deeper MP3 frame-scanner and malformed-header branches |

## Suggested Milestones

| Milestone | Target | Scope |
|---|---|---|
| COV-01 | full total >= 80% | **Reached 2026-09-11**; full total now 83.2% |
| COV-02 | root package >= 82% | **Reached 2026-09-11** (82.0%); verified with `go test ./...` and corpus validate |
| COV-03 | command/helper packages >= 85% | **Effectively reached 2026-09-11** (`cmd/pptx` 85.6%, `scripts/perf/summarize` 86.4%, `wasm/check` 85.9%); optional remaining polish on subcommand JSON variants |
| COV-04 | release target review | **Evaluated 2026-09-11**: a flat 90% per-package target is abandoned; 90% applies only to low-level format packages (see Conclusion) |

## Verification

Use this sequence for coverage PRs:

```bash
go test ./... -coverprofile=/tmp/opencode/go-pptx-cover.out
go tool cover -func=/tmp/opencode/go-pptx-cover.out
scripts/gen_corpus/run.sh validate testdata/corpus
python3 -m py_compile scripts/gen_corpus/corpus.py
```

## Current Conclusion

COV-01 (full total >= 80%), COV-02 (root >= 82%), and COV-03 (command/helper >= 85%) are reached on 2026-09-11; full total is 83.2% and root is 82.0%.

COV-04 evaluation (2026-09-11): a flat 90%-per-package target is abandoned. Rationale:

- **90% stays for low-level format packages** whose behavior is enumerable and where coverage maps to format-safety: `internal/opc`, `internal/xmlstore`, `internal/videoprobe`, `internal/audioprobe`, `internal/textmap`, `internal/editplan`. These are already at 81.8%–92.1% and the marginal tests are behavior-first, not brittle.
- **Root SDK target is 85%** (currently 82.0%), not 90%: the 158-export-type surface is breadth-driven and a 90% target pushes toward brittle serialization/implementation-detail tests, contradicting Rule 2.
- **Command/helper packages stay at 85%**: `cmd/pptx` (85.6%), `scripts/perf/summarize` (86.4%), `wasm/check` (85.9%) already satisfy this.
- **`ir` and `render` use behavior-based targets**: `ir` at 86.2% after closing the table-text projection gap; further gains are optional and must stay behavior-first.

Next 1.x coverage work is optional polish only; coverage is not a v1.0 hard gate.
