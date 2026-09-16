# go-pptx Architecture Baseline

Date: 2026-09-11

> **Update 2026-09-16**: 本基线记录的是抽取起步态。之后已完成 P3 文件拆分、文件名域前缀
> 归一化、以及 v2.0 架构试点（`text_util`/`bind_marker`/`theme_placeholder` 迁出 + 共享
> helper 下沉）。当前包清单与依赖方向见文末 [Update 2026-09-16](#update-2026-09-16)。
> 上文基线表**保留原样**以对照进展，勿当作当前值。

This document records the current architecture before the next refactoring phase. It is a baseline for measuring whether future internal package extraction improves maintainability without changing the public SDK surface.

## Package Layout

Current Go package inventory:

| Package | Role | Go files | Test files |
|---|---:|---:|---:|
| `github.com/F31/go-pptx` | Public SDK facade plus most domain implementation | 41 | 34 |
| `github.com/F31/go-pptx/internal/opc` | OPC package loading, relationships, content types, save planning | 9 | 5 |
| `github.com/F31/go-pptx/internal/xmlstore` | XML scanner, indexed tree, span patch engine | 6 | 5 |
| `github.com/F31/go-pptx/internal/document` | Minimal store interfaces for document edits | 1 | 0 |
| `github.com/F31/go-pptx/internal/editplan` | Single-part and multi-part edit plans | 2 | 2 |
| `github.com/F31/go-pptx/internal/textmap` | Text rune mapping and span location primitives | 1 | 1 |
| `github.com/F31/go-pptx/internal/audioprobe` | Audio container probing | 4 | 1 |
| `github.com/F31/go-pptx/internal/videoprobe` | Video container probing | 4 | 1 |
| `github.com/F31/go-pptx/ir` | Read-only intermediate representation, timing IR, semantic diff | 3 | 3 |
| `github.com/F31/go-pptx/cmd/pptx` | CLI workflow entry point | 12 | 5 |
| `github.com/F31/go-pptx/wasm/check` | Browser/WASM check facade | 1 | 0 |
| `github.com/F31/go-pptx/render` | Rendering placeholder package | 1 | 0 |
| `github.com/F31/go-pptx/scripts/perf/summarize` | Performance summary helper | 1 | 0 |

## Current Dependency Direction

The current dependency direction is mostly clean:

```text
cmd/pptx       -> pptx, ir
wasm/check     -> pptx, ir
ir             -> pptx, internal/xmlstore
pptx           -> internal/document, internal/editplan, internal/textmap,
                  internal/opc, internal/xmlstore, internal/audioprobe, internal/videoprobe
internal/editplan -> internal/document, internal/opc
internal/document -> internal/opc
internal/opc   -> internal/xmlstore
```

The root package is the largest package because it contains both public API and most domain logic. That is acceptable for a Go SDK facade, but it creates pressure on public interfaces and makes implementation-only refactoring harder.

## Root Package Responsibilities

The root package currently owns:

- Public object model: `Presentation`, `Slide`, `Shape`, `TextFrame`, `Paragraph`, `TextRun`.
- Document lifecycle: `New`, `Open`, `OpenReader`, `Save`, `Write`, `Close`.
- Transaction state: staged patches, added/deleted parts, revision, XML document cache.
- Text editing: rich text model, cross-run replacement, plain text replacement, fields.
- Shape model: shape classification, geometry, line/fill/effect reports.
- Tables: logical grid, merge/unmerge, cell text, style subset.
- Charts: chart XML model, workbook synchronization, advanced labels/error bars/trendlines/axes.
- Media: picture/audio/video insertion and relationship management.
- Timing and transitions: audio playback tree, transitions, timing raw access.
- Template binding and clone workflows.
- Capability and validation report construction.

## Current Edit Transaction Rule

Feature code should not directly call `stageAdd`, `stagePatch`, `stageDelete`, or `commit`.

Current boundary:

- `presentation.go` owns the low-level transaction primitives.
- `document_store.go` adapts those primitives to `internal/document` contracts.
- Single-part feature writes use `internal/editplan.SinglePartPatch` through `applySinglePartPatch`.
- Multi-part feature writes use `internal/editplan.MultiPartPlan` through `applyMultiPartPlan`, which restores `pending` on stage failure.

The expected direct references to low-level transaction primitives are therefore limited to `presentation.go` and `document_store.go`.

## Identified Architecture Pressure Points

| Area | Pressure | Preferred Direction |
|---|---|---|
| `Presentation` internals | Feature code touches transaction/cache details indirectly through root helpers | Introduce `internal/document` engine and `PartStore` facade |
| Text replacement | High-risk logic combines text mapping, safety checks, patch construction, and public API result types | Extract implementation-only text map/plan logic to `internal/textmap` |
| `Shape` interface | The interface has grown beyond identity into geometry/style/effect access | Add small capability interfaces before adding more methods |
| Chart implementation | XML read/write/canonical validation/workbook logic is concentrated in large files | **DONE (2026-09-12 ADR-017 r1+r2+r3)**：全部实现层已搬到 `internal/chart`——第一批 3 函数 + 4 常量、第二批 11 值对象 + `Optional[T]`（type alias move，公共 API 零变化）、第三批 parse / build / canonical / validate / fragment / workbook + `xmlUnescape`。**根包同时删除 31 个已无生产调用方的私有 facade**（保留 `chartIsCanonical` / `parseChartSpace` / `buildChartSpaceXML` / `validateChartData` / `nodeText` 等仍有调用方的适配器）。结果：chart.go 1319 → **481** 行、chartfrag.go 262 → **74**、chartbook.go 215 → **107**；白盒测试迁入 `internal/chart`（覆盖率 **91.4%**）；// Stable: 34 / // Experimental: 5 / root type 158 / 17 哨兵 全部锁死；全仓覆盖率维持 **84.4%**；corpus replay 36/36 + perf smoke PASS。**第四批（可选）**：文件归并与 `DefaultWorkbookBuilder` 实现下沉评估 |
| Compatibility evidence | Corpus manifests and smoke reports exist but are not yet indexed into a matrix | Add corpus index/report generation |

## Refactoring Rules

1. Public API stays in root `package pptx` unless an ADR explicitly allows a breaking change.
2. New internal packages must not import the root package.
3. Extract pure implementation first; keep root methods as facade functions.
4. Every migrated path must keep existing tests passing and preserve corpus gold diffs.
5. The first phase is additive and behavior-preserving except for bug fixes guarded by tests.
6. Each refactoring step must remove or avoid hard-coded assumptions when a typed constant, package-level helper, or manifest-driven value already exists.
7. Each refactoring step must delete dead code, unused adapters, stale comments, and obsolete compatibility branches introduced by the migration itself.
8. New abstraction layers must have at least one current caller or a compile-time assertion that proves the intended integration point; otherwise they are deferred.

## Completed First Batch Scope

The first batch intentionally avoids broad file moves:

1. Added this architecture baseline document.
2. Added `internal/document` with minimal document-store interfaces.
3. Extracted text span location primitives to `internal/textmap` and wired `ReplaceText` through them.
4. Added `internal/editplan` with `SinglePartPatch` and `MultiPartPlan`.
5. Migrated production edit paths from direct transaction primitives to plan helpers, including text, shape, transition, timing, table, page, notes, bind, media, chart, clone, and docProps paths.

Success criteria:

```bash
go test -run 'TestReplaceTextSuffixInLaterRunDoesNotCorruptPrefix|TestReplaceTextCrossRunFirstCharacter|TestTextFrameReplaceText' ./...
scripts/gen_corpus/run.sh validate testdata/corpus
```

Current broad verification:

```bash
go test ./...
scripts/gen_corpus/run.sh validate testdata/corpus
python3 -m py_compile scripts/gen_corpus/corpus.py
```

---

## Update 2026-09-16

基线（2026-09-11）之后的进展：P3 根包大文件拆分（ADR-029）、文件名域前缀归一化、
以及 **v2.0 架构试点**（ADR-030）——3 个「零导出/零方法」文件迁出根包，并把跨包
helper 下沉为共享 internal 包。

### 当前包清单

| Package | Role | Go files | Test files |
|---|---:|---:|---:|
| `github.com/F31/go-pptx` | Public SDK facade + domain implementation | 60 | 50 |
| `github.com/F31/go-pptx/internal/opc` | OPC package loading, relationships, content types, save planning | 9 | 11 |
| `github.com/F31/go-pptx/internal/xmlstore` | XML scanner, indexed tree, span patch engine, DOM helpers | 7 | 6 |
| `github.com/F31/go-pptx/internal/chart` | Chart XML model, workbook, canonical validation, fragments | 11 | 4 |
| `github.com/F31/go-pptx/internal/document` | Minimal store interfaces for document edits | 1 | 1 |
| `github.com/F31/go-pptx/internal/editplan` | Single-part and multi-part edit plans | 2 | 2 |
| `github.com/F31/go-pptx/internal/textmap` | Text rune mapping and span location primitives | 1 | 1 |
| `github.com/F31/go-pptx/internal/audioprobe` | Audio container probing | 4 | 1 |
| `github.com/F31/go-pptx/internal/videoprobe` | Video container probing | 4 | 1 |
| `github.com/F31/go-pptx/internal/textutil` | XML patch/escape helpers (v2.0 试点) | 1 | 2 |
| `github.com/F31/go-pptx/internal/bind` | Template marker scan/directive (v2.0 试点) | 1 | 1 |
| `github.com/F31/go-pptx/internal/style` | Placeholder key/class resolution (v2.0 试点) | 1 | 1 |
| `github.com/F31/go-pptx/internal/ooxmlns` | Shared OOXML/OPC namespace URIs | 1 | 0 |
| `github.com/F31/go-pptx/ir` | Read-only intermediate representation, timing IR, semantic diff | 3 | 4 |
| `github.com/F31/go-pptx/cmd/pptx` | CLI workflow entry point | 12 | 5 |
| `github.com/F31/go-pptx/wasm/check` | Browser/WASM check facade | 1 | 1 |
| `github.com/F31/go-pptx/render` | Rendering adapter **interface contract** (M8 RENDER-01; no impl) | 1 | 1 |
| `github.com/F31/go-pptx/scripts/perf/summarize` | Performance summary helper | 1 | 1 |

根包非测试文件：基线 41 → 现 **60**（P3 拆分把 `text`/`bind`/`geomadv`/`format`/`style`
各拆为多文件，新增数大于 3 个试点迁出数，故总数上升）；**平均行数 932 → 353**
（拆分与迁移共同下降），非测试总行数 21,163。

### 当前依赖方向（`go list` 实测，2026-09-16）

```text
pptx           -> internal/{audioprobe, bind, chart, document, editplan, ooxmlns,
                            opc, style, textmap, textutil, videoprobe, xmlstore}
cmd/pptx       -> pptx, ir
wasm/check     -> pptx, ir
render         -> pptx                       # 公共契约；门面不反向依赖 render
ir             -> pptx, internal/xmlstore    # 待 v2.0 改造为只吃 ooxml（ADR-030）
internal/chart -> internal/{ooxmlns, textutil, xmlstore}
internal/style -> internal/{ooxmlns, xmlstore}
internal/bind  -> internal/xmlstore
internal/textutil -> internal/xmlstore
internal/editplan -> internal/{document, opc}
internal/document -> internal/{opc, xmlstore}
internal/opc   -> internal/xmlstore
internal/ooxmlns / textmap / audioprobe / videoprobe -> (无 go-pptx 依赖)
```

规则不变：**internal 不得反向 import 根包**（现满足；`ir` 例外，属名义公共包，
v2.0 改造项）。v2.0 目标态见 [ADR-030](adr/ADR-030-v2-target-architecture.md)。

### 剩余「压力点」处置

基线所列 5 项：`internal/chart` 抽取 **DONE**（ADR-017）；`internal/textmap` **DONE**；
`internal/document` `PartStore` **DONE**（接口 + 编译期断言）；Shape capability 接口
**DONE**（ADR-021）；corpus matrix **DONE**（`docs/client-compat-matrix.md`）。
新增压力点：根包仍 60 文件（ADR-030 触发阈值 80，未命中）。
