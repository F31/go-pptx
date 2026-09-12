# Coverage Roadmap

Date: 2026-09-11（snapshot refreshed 2026-09-12 morning; second refresh 2026-09-12 afternoon after 5-package behavior-first push）

This document tracks the 1.x coverage-improvement work that was deferred from the v1.0 hard gate. It is not a v1.0 release blocker, but it is the active plan for moving toward the V2.6 90% target.

## Current Snapshot

Command:

```bash
go test ./... -coverprofile=/tmp/opencode/go-pptx-cover.out
go tool cover -func=/tmp/opencode/go-pptx-cover.out
```

Package snapshot (2026-09-12 re-measured after `6361bdd`, Windows/amd64 dev box):

| Package | Coverage (2026-09-11) | Coverage (2026-09-12 morning) | Coverage (2026-09-12 afternoon, 5-package push) |
|---|---:|---:|---:|
| `github.com/F31/go-pptx` | 82.0% | **82.7%** | **82.8%** |
| `github.com/F31/go-pptx/cmd/pptx` | 85.6% | **87.2%** | 87.2% |
| `github.com/F31/go-pptx/internal/audioprobe` | 83.0% | **84.9%** | **88.4%** |
| `github.com/F31/go-pptx/internal/editplan` | 81.8% | 81.8% | **100.0%** |
| `github.com/F31/go-pptx/internal/opc` | 86.7% | 86.6% | **88.9%** |
| `github.com/F31/go-pptx/internal/textmap` | 82.2% | 82.2% | **100.0%** |
| `github.com/F31/go-pptx/internal/videoprobe` | 92.1% | **92.6%** | 92.6% |
| `github.com/F31/go-pptx/internal/xmlstore` | 90.2% | **90.8%** | 90.8% |
| `github.com/F31/go-pptx/ir` | 86.2% | **87.0%** | 87.0% |
| `github.com/F31/go-pptx/render` | 84.2% | 84.2% | 84.2% |
| `github.com/F31/go-pptx/scripts/perf/summarize` | 86.4% | **89.5%** | 89.5% |
| `github.com/F31/go-pptx/wasm/check` | 85.9% | **89.5%** | 89.5% |

**Full-repo weighted total: 84.4%** (re-measured 2026-09-12 afternoon; up from 83.2% on 2026-09-11 / 82.9% after COV-02).

Notes on the 2026-09-12 refreshes:

- **Morning refresh**: Re-measured after `6361bdd`（VideoShape 公开访问器与 probe 错误映射覆盖）on the same dev box; all packages are at or above the 2026-09-11 record, so no milestone regresses.
- **Afternoon refresh**: Five behavior-first push commits — `919b803` (`internal/editplan`: 81.8%→100%), `7eec993` (`internal/textmap`: 82.2%→100%), `02a2f92` (`internal/opc`: 86.6%→88.9%), `75c9a60` (`internal/audioprobe`: 84.9%→88.4%), `0587a4e` (root SDK: 82.7%→82.8%). 26 new behavior-priority tests; full-repo total 83.2%→84.4%. No regressions in any package.

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
| Done | COV-01 | full total 80.0% | Milestone reached; full total now 84.4% as of 2026-09-12 afternoon |
| Done | `scripts/perf/summarize` | 86.4% | Keep tests focused on parser/merge/report behavior; no further action unless the helper changes |
| Done | `internal/videoprobe` | 92.1% | Malformed error helpers, MP4/WebM edge variants, and duration wrappers covered |
| Done | `internal/audioprobe` | 88.4% | After 5-package push: deeper MP3 frame-scanner branches covered (`parseMP3Frame` 4 error paths, MPEG2 L3 / Layer II warning / free-format bitrate); remaining gaps are unreachable defensive post-frame code |
| Done | `wasm/check` | 89.5% | Error envelope and `AsError` edge coverage added; revisit only if wasm API changes |
| Done | `internal/xmlstore` | 90.8% | Scanner accessors, node helpers, and error formatting covered |
| Done | `internal/opc` | 88.9% | After 5-package push: `OpenPart`, `normalize` branches, `verifyOutput` (missing entry / bad zip), `Lookup` case-fallback, percent-escape 4 branches. Remaining ~1.1% is concentrated in I/O error paths that need a mocked filesystem |
| Done | `internal/editplan` | 100.0% | After 5-package push: `MultiPartPlan.All`/`StopOnError`, `StageAdd`/`StageDelete` error paths, `SinglePartPatch` 4 getters — over-delivers the 90% low-level format target |
| Done | root `pptx` | 82.8% | **COV-02 reached 2026-09-11**; 5-package push added 5 contract-valued audio/timing guards (+0.1%). 85% threshold deferred to 1.x — requires 30+ system-level tests with diminishing returns |
| Done | `ir` | 87.0% | Table-text projection (`readTableText`/`cellText`) closed with a self-contained zip fixture deck |
| Done | `internal/textmap` | 100.0% | After 5-package push: `LocateSpan` missing-entry, `IndexRunes` empty needle, `GraphemeSafe` OOB / ZWJ, max a>b symmetry. Over-delivers the 90% low-level format target |
| P2 | `cmd/pptx` | 87.2% | Subcommand JSON output variants and edge error paths; optional polish only |
| P2 | root `pptx` | 82.8% | 85% threshold requires 30+ system-level tests (audiotiming Plan*/Apply*, clone.go helpers, `docProps.leafTextPatch`, capability enum maps, etc.). Marginal value per test is low; defer to 1.x unless a contract-driven reason emerges |
| P3 | `internal/opc` | 88.9% | ~1.1% gap is I/O error simulation (corrupt on-disk part, fsync failure); requires an injected filesystem or temp-dir fault injection |

## Suggested Milestones

| Milestone | Target | Scope |
|---|---|---|
| COV-01 | full total >= 80% | **Reached 2026-09-11**; full total now **84.4%** as of 2026-09-12 afternoon |
| COV-02 | root package >= 82% | **Reached 2026-09-11** (82.0%); now **82.8%** as of 2026-09-12 afternoon |
| COV-03 | command/helper packages >= 85% | **Reached 2026-09-11**; all three packages now 87.2% / 89.5% / 89.5% |
| COV-04 | release target review | **Re-evaluated 2026-09-12 afternoon**: flat 90% per-package target abandoned; 90% applies to low-level format packages. After 5-package push: 4/6 format packages are at or above 90% (`videoprobe` 92.6%, `xmlstore` 90.8%, `editplan` 100%, `textmap` 100%), `opc` 88.9% and `audioprobe` 88.4% remain below 90% but each with marginal-gain-only remaining branches |

## Verification

Use this sequence for coverage PRs:

```bash
go test ./... -coverprofile=/tmp/opencode/go-pptx-cover.out
go tool cover -func=/tmp/opencode/go-pptx-cover.out
scripts/gen_corpus/run.sh validate testdata/corpus
python3 -m py_compile scripts/gen_corpus/corpus.py
```

## Current Conclusion

COV-01 (full total >= 80%), COV-02 (root >= 82%), and COV-03 (command/helper >= 85%) were reached on 2026-09-11. As of 2026-09-12 afternoon (after the 5-package behavior-first push), full-repo weighted total is **84.4%**, root is **82.8%**, all three command/helper packages are between 87.2% and 89.5%.

COV-04 evaluation (re-evaluated 2026-09-12 afternoon): the flat 90%-per-package target remains abandoned. Updated rationale:

- **90% stays for low-level format packages** whose behavior is enumerable and where coverage maps to format-safety: `internal/opc`, `internal/xmlstore`, `internal/videoprobe`, `internal/audioprobe`, `internal/textmap`, `internal/editplan`. After the 5-package push, four packages are at or above 90% — `videoprobe` 92.6%, `xmlstore` 90.8%, `editplan` 100.0%, `textmap` 100.0% — and two (`opc` 88.9%, `audioprobe` 88.4%) remain between 88% and 90% with only marginal or I/O-error branches left.
- **Root SDK target is 85%** (currently 82.8%), not 90%: the 158-export-type surface is breadth-driven and a 90% target pushes toward brittle serialization/implementation-detail tests, contradicting Rule 2.
- **Command/helper packages stay at 85%**: all three packages already exceed this (`cmd/pptx` 87.2%, `scripts/perf/summarize` 89.5%, `wasm/check` 89.5%).
- **`ir` and `render` use behavior-based targets**: `ir` at 87.0% after closing the table-text projection gap; further gains are optional and must stay behavior-first.

Next 1.x coverage work is optional polish only — `opc` and `audioprobe` remaining gaps are either I/O-error simulation (needs filesystem fault injection) or unreachable defensive code; root 85% requires 30+ system-level tests with diminishing returns per test.
