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
| `github.com/F31/go-pptx/v2` | Public SDK facade plus most domain implementation | 41 | 34 |
| `github.com/F31/go-pptx/v2/internal/opc` | OPC package loading, relationships, content types, save planning | 9 | 5 |
| `github.com/F31/go-pptx/v2/internal/xmlstore` | XML scanner, indexed tree, span patch engine | 6 | 5 |
| `github.com/F31/go-pptx/v2/internal/document` | Minimal store interfaces for document edits | 1 | 0 |
| `github.com/F31/go-pptx/v2/internal/editplan` | Single-part and multi-part edit plans | 2 | 2 |
| `github.com/F31/go-pptx/v2/internal/textmap` | Text rune mapping and span location primitives | 1 | 1 |
| `github.com/F31/go-pptx/v2/internal/audioprobe` | Audio container probing | 4 | 1 |
| `github.com/F31/go-pptx/v2/internal/videoprobe` | Video container probing | 4 | 1 |
| `github.com/F31/go-pptx/v2/ir` | Read-only intermediate representation, timing IR, semantic diff | 3 | 3 |
| `github.com/F31/go-pptx/v2/cmd/pptx` | CLI workflow entry point | 12 | 5 |
| `github.com/F31/go-pptx/v2/wasm/check` | Browser/WASM check facade | 1 | 0 |
| `github.com/F31/go-pptx/v2/render` | Rendering placeholder package | 1 | 0 |
| `github.com/F31/go-pptx/v2/scripts/perf/summarize` | Performance summary helper | 1 | 0 |

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
| `github.com/F31/go-pptx/v2` | Public SDK facade + domain implementation | 60 | 50 |
| `github.com/F31/go-pptx/v2/internal/opc` | OPC package loading, relationships, content types, save planning | 9 | 11 |
| `github.com/F31/go-pptx/v2/internal/xmlstore` | XML scanner, indexed tree, span patch engine, DOM helpers | 7 | 6 |
| `github.com/F31/go-pptx/v2/internal/chart` | Chart XML model, workbook, canonical validation, fragments | 11 | 4 |
| `github.com/F31/go-pptx/v2/internal/document` | Minimal store interfaces for document edits | 1 | 1 |
| `github.com/F31/go-pptx/v2/internal/editplan` | Single-part and multi-part edit plans | 2 | 2 |
| `github.com/F31/go-pptx/v2/internal/textmap` | Text rune mapping and span location primitives | 1 | 1 |
| `github.com/F31/go-pptx/v2/internal/audioprobe` | Audio container probing | 4 | 1 |
| `github.com/F31/go-pptx/v2/internal/videoprobe` | Video container probing | 4 | 1 |
| `github.com/F31/go-pptx/v2/internal/textutil` | XML patch/escape helpers (v2.0 试点) | 1 | 2 |
| `github.com/F31/go-pptx/v2/internal/bind` | Template marker scan/directive (v2.0 试点) | 1 | 1 |
| `github.com/F31/go-pptx/v2/internal/style` | Placeholder key/class resolution (v2.0 试点) | 1 | 1 |
| `github.com/F31/go-pptx/v2/internal/ooxmlns` | Shared OOXML/OPC namespace URIs | 1 | 0 |
| `github.com/F31/go-pptx/v2/ir` | Read-only intermediate representation, timing IR, semantic diff | 3 | 4 |
| `github.com/F31/go-pptx/v2/cmd/pptx` | CLI workflow entry point | 12 | 5 |
| `github.com/F31/go-pptx/v2/wasm/check` | Browser/WASM check facade | 1 | 1 |
| `github.com/F31/go-pptx/v2/render` | Rendering adapter **interface contract** (M8 RENDER-01; no impl) | 1 | 1 |
| `github.com/F31/go-pptx/v2/scripts/perf/summarize` | Performance summary helper | 1 | 1 |

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

规则不变：**internal 不得反向 import 门面**。v2.0 门面收敛后 `pptx/` 为公共
门面；`internal/*` 只依赖更低层。**唯一临时例外**：`internal/engine`（编排层，
承载门面→IR 适配器与 Open/Save 编排，需门面公共句柄）。`internal/ir` 已于
2026-09-17 完成门面解耦（改吃 `internal/document/model` + `xmlstore`；投影
适配器 `ProjectIR` 移入 engine）——R1 由 `internal/archlint`（Step 6）守护。
v2.0 目标态见 [ADR-030](adr/ADR-030-v2-target-architecture.md)。

### 剩余「压力点」处置

基线所列 5 项：`internal/chart` 抽取 **DONE**（ADR-017）；`internal/textmap` **DONE**；
`internal/document` `PartStore` **DONE**（接口 + 编译期断言）；Shape capability 接口
**DONE**（ADR-021）；corpus matrix **DONE**（`testdata/corpus/README.md`，复现
`scripts/l3/run_client.sh`）。
新增压力点：根包仍 60 文件（ADR-030 触发阈值 80，未命中）。

### Update 2026-09-17：v2.0 门面收敛（ADR-030 Step 3）

根包 `package pptx` **整体移入 `pptx/` 子包**——模块根不再含任何 `.go`
文件（`go.mod` / `LICENSE` / `README.md` 为元仓库根）。唯一正式公共导入
路径变为 `github.com/F31/go-pptx/v2/pptx`。

- 迁移：105 个根 `.go`（104 `pptx` + 1 `pptx_test`）`git mv` 入 `pptx/`；
  `assets/audio-speaker.png` 同迁 `pptx/assets/`（`//go:embed` 不能用
  `..`）；`api_surface_test.go` 随包迁移，`addAliasMethods` 解析模块内
  包改以 `..`（模块根）为基准
- 引用更新：24 个 importer（`cmd/pptx`、`wasm/check`、`ir`、`render`、
  `scripts/gen_*`、`pptx/example_test.go`）由 `.../go-pptx` → `.../go-pptx/pptx`
- 测试相对路径：6 个 corpus 测试的 `testdata/corpus` → `../testdata/corpus`
  （`testdata/` 按 ADR-030 目标布局保留在仓库根）
- CI：`scripts/coverage/gate.sh` 门槛项 root → `github.com/F31/go-pptx/v2/pptx=82`；
  `fuzz.yml` 的 4 个 root fuzz 目标 `pkg: '.'` → `'./pptx'`
- 守恒：api_surface golden 零变更（163 type / 40 Stable 段 / 60 符号 /
  131 方法 / 17 哨兵）；`go test ./...` 24/24 + corpus 24；go vet/gofmt
  clean；coverage gate PASS（`pptx` 83.3% ≥ 82）

- **Step 4（同日）**：顶层 `ir/` → `internal/ir`（importer 与 gate 门槛同步；
  实测 85.2%）；`render/` 保留。同日完成**门面解耦**：`FromPresentation`
  投影适配器移入 `internal/engine.ProjectIR`，`internal/ir` 不再 import
  `pptx`（改吃 `internal/document/model` + `xmlstore`）；投影测试随迁 engine

- **Step 5（同日）**：新建 `internal/engine`（编排层起步）——统一 CLI 与
  WASM 的 Inspect/Validate/Capability 核心；`wasm/check` 与
  `cmd/pptx/{inspect,validate,capability}` 改委托（实测 92.3%）；承接
  `ProjectIR` 适配器后 86.6%
- **Step 6（同日）**：新建 `internal/archlint`（std-lib 依赖方向校验），
  CI 经 `go test` 执行；当前模块全合规（临时例外收敛为仅 `internal/engine`）
- **Step 1（同日）**：ooxml 生成管线落地——`scripts/gen/schema`（std-lib
  XSD→Go）+ `internal/ooxml/schema`（Transitional schema，只读投影，100%）。
   输入源更正为 ECMA-376 **Part 4 Transitional**（`schemas.openxmlformats.org`），
   非 Part 1 的 Strict（`purl.oclc.org`）。**格式层真改造（续）**：新增
`internal/ooxml`（`Open`/`Bytes` + `SlideShapes` schema 只读投影），engine
   `ProjectIR` 的形状/文本/表格改由 `pptx.PartBytes` 只读桥 + `ooxml.SlideShapes`
    驱动。**长尾（同日）**：notes/timing/hidden 也改由 `internal/ooxml` 只读投影；
    **图表投影收尾**：`chart.go`（`ChartPartOf`/`ChartData`，slide rels→chart part→
    schema 解码 C_CT_ChartSpace 抽 type/title/categories），engine 不再走门面
    `ChartShape.DataWithDiagnostics()`/`chartShapesByID`——`projectShapes` 全路径
    零门面句柄读取。engine 覆盖率 91.1%，ooxml 覆盖率 92.7%。

当前导入路径：**`github.com/F31/go-pptx/v2/pptx`**（breaking change，
v2.0 一次性迁移）。
