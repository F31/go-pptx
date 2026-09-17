# go-pptx

[English](./README.md) | [中文](./README.zh.md)

A pure Go SDK for creating, editing, and auditing PowerPoint (OOXML / PPTX) files. Zero runtime dependencies. Runs on Linux, Windows, macOS, and browser WASM.

```text
Product    go-pptx
License    Apache-2.0
Go         >= 1.24
Module     github.com/F31/go-pptx/v2
Import     github.com/F31/go-pptx/v2/pptx
```

[![CI](https://github.com/F31/go-pptx/actions/workflows/ci.yml/badge.svg)](https://github.com/F31/go-pptx/actions/workflows/ci.yml)

---

## What is go-pptx?

go-pptx is a pure Go SDK for **programmatic PPTX processing** — creating slides, replacing text, embedding images/charts/audio/video, batch-rendering templates, and auditing document changes — **without** Office clients, LibreOffice, or any interpreter runtime.

Unlike libraries that read and rewrite documents entirely, go-pptx treats **OOXML raw bytes as the invariant**. It patches only the target regions; untouched parts and unknown extensions remain byte-identical (B1 fidelity). This minimizes side effects on documents produced by other tools — the core differentiator from generic serialization libraries.

**v2.0 architecture** (ADR-030): The codebase follows a layered structure with a thin public facade (`pptx/`), domain logic in `internal/`, and a schema-generated type model from the ECMA-376 standard. See [Architecture](#architecture-v20) for details.

---

## Install

```bash
go get github.com/F31/go-pptx/v2/pptx
```

---

## Quick Start

### Create and Edit

```go
package main

import (
	"context"
	"log"

	"github.com/F31/go-pptx/v2/pptx"
)

func main() {
	p, err := pptx.New()
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	layouts, _ := p.Layouts()
	slide, _ := p.AddSlide(layouts[0])

	box, _ := slide.AddTextBox(pptx.TextBoxSpec{
		X: 914400, Y: 914400, Width: 6096000, Height: 914400, // EMU units (1 in = 914400)
		Text: "Hello, go-pptx",
	})
	tf, _ := box.TextFrame()
	tf.ReplaceText("go-pptx", "World") // Cross-run replacement with format preservation

	if _, err := p.Save(context.Background(), "out.pptx"); err != nil {
		log.Fatal(err)
	}
}
```

### Template Data Binding

```go
// Inline placeholders: {{name}} / {{#if}} conditional / {{#each rows}} table row loop / chart data
rep, _ := p.Bind(map[string]any{
	"name":  "Q3 Revenue",
	"rows":  []any{map[string]any{"k": "East China", "v": 120}},
})
_ = rep // BindReport{...}
```

> Full API: `go doc github.com/F31/go-pptx/v2/pptx` · [Implementation Status](docs/go-pptx-实施状态跟踪.md)

---

## Core Features

| Category | Capabilities |
|---|---|
| **Create** | `New()` minimal template; `AddSlide` / `AddTextBox` / `AddAutoShape` / `AddPicture` / `AddChart` / `AddAudio` / `AddVideo`; `MoveSlide` / `RemoveSlide` / `MoveShape` / `RemoveShape` |
| **Text Editing** | Paragraph / Run object model; cross-run literal replacement (3 format strategies, grapheme cluster protection, br/field/link boundary safe); font/color/size patch and reset; text box properties and slidenum/datetime fields |
| **Images & Media** | PNG/JPEG embedding with 4 fit modes (stretch/Contain/Cover/original); content-hash dedup; MP3/WAV/MP4/WebM pure-Go probing (duration, signature, container brand); audio playback tree and timing plan |
| **Charts & Tables** | Bar/Line/Pie charts with embedded workbook generation; trendlines/error bars/data labels/axis extensions; rich text tables, cell merging, table style parsing and priority matrix |
| **Template Binding** | Inline `{{path}}`, conditional paragraphs, table row loops, chart data binding; plan-phase read-only validation + single-transaction atomic commit, **zero residue on failure** |
| **Read-only Audit** | Read-only IR (schema-versioned JSON), semantic `Diff`, `Validate`, `Capability` six-dimension manifest |
| **Byte Preservation** | SpanPatch precise byte patching, unknown subtree/extension preservation, revision concurrency protection, atomic save with failure recovery |
| **Browser / WASM** | WASM build target + offline static inspection page (files never leave the device, no internet, no CDN) |

---

## Why go-pptx?

### Comparison with Other Libraries

| Dimension | **go-pptx** | python-pptx | Apache POI (XSLF) | LibreOffice UNO | Aspose.Slides | Open XML SDK |
|---|---|---|---|---|---|---|
| Language / Runtime | **Go, zero runtime** | Python interpreter | JVM | C++/multi-lang | .NET/Java/cloud | .NET |
| Deployment | Single binary / WASM | Needs Python env | Needs JVM | Needs LibreOffice install | Needs runtime + commercial license | Needs .NET |
| Edit Fidelity | **Byte-level precise patching**, untouched parts unchanged | Read-then-rewrite package | Rebuild document | Rebuild document | Rebuild document (good fidelity) | Full rewrite |
| Unknown Extension Retention | **Raw byte preservation** (Opaque strategy) | Dependent on rewrite path, easily lost | Depends on parser coverage | Depends on parser coverage | Depends on version | Structural retention |
| Browser Read-only | **WASM, files never leave device** | None | None | None | Web API (requires upload) | None |
| Capability Self-description | **Yes (6-dimension Supported/Partial/...)** | None | None | None | None | None |
| Semantic Diff (page-aligned) | **Built-in (weighted LCS)** | None | None | None | Yes (comparison) | None |
| Template Data Binding | **Built-in** (inline/conditional/row loop/chart) | Needs jinja2 etc. | None | Yes (macros/scripts) | Yes (template API) | None |
| Media Duration/Signature Probe | **Pure Go** (WAV/MP3/MP4/WebM) | No built-in | No built-in | Yes | Yes | None |
| License | **Apache-2.0** | MIT | Apache-2.0 | MPL-2.0 | Commercial | MIT |

### Key Differentiators

1. **Only "byte-level fidelity" pure Go implementation**: When editing a single Run in real-world samples, all other Part bytes remain identical before/after (B1), and untouched regions within edited Parts converge to 1-byte difference — ideal for document processing where "documents must not be rewritten by tools."
2. **No comparable PPTX SDK in the Go ecosystem**: Single binary distribution, seamless integration with existing Go services/CI, WASM directly in the browser.
3. **Built-in audit and capability negotiation**: `Diff`, `Validate`, `Capability` let callers know which capabilities are available or limited before deployment, avoiding runtime surprises.
4. **Open source and self-sustaining**: Apache-2.0, no runtime/licensing costs, lower cost than commercial alternatives (Aspose.Slides) for batch processing and automation.

---

## Technical Innovations

### 1. Raw Byte + Namespace Dual Model

The custom `internal/xmlstore` lexical scanner preserves both the byte span and parsed namespace scope of every element. Unknown namespace subtrees still get nodes with original bytes preserved, enabling the "change one Run, keep everything else identical" fidelity goal. This also works around `encoding/xml` depth-sync deviations on certain self-closing tag combinations.

### 2. Transactional Editing

All write paths converge to `SinglePartPatch` / `MultiPartPlan` — first read-only validation (plan), then single-transaction apply, with zero residue on any failure; revision validation rejects concurrent modifications. Behavior aligns with "database transactions" rather than "in-place string rewriting."

### 3. Byte-Level Preservation Patch Engine (B1)

SpanPatch interval replacement + anchor validation + conflict detection; changesets are validated holistically then applied in descending offset order; unchanged Parts are copied verbatim; Content Types are regenerated from the same source as the changeset.

### 4. Capability Self-description and Safety Boundaries

`CapabilityManifest` reports six dimensions (Inspect/Create/Edit/Preserve/Render/Play) with per-work-package support levels and limitations. WASM inspection pages complete locally in the browser — files never leave the device.

### 5. Semantic Diff with Page Alignment

Weighted LCS + shape ID set similarity distinguishes "two revisions of the same page" from "delete page + add page"; high-threshold secondary pairing marks moves for residual ordering mismatches.

### 6. Corpus-driven Engineering Verification

36 real PPTX samples + 3 redistributable LibreOffice gold samples (with action replay and byte-level B1 assertions); edited files open in PowerPoint / WPS without repair prompts (L3 8/8).

---

## Architecture (v2.0)

```text
┌─────────────────────────────────────────────────────────┐
│                    Public Facade                         │
│                  github.com/F31/go-pptx/v2/pptx            │
│  Presentation · Slide · Shape · TextFrame · TableShape  │
│  ChartShape · PictureShape · AudioShape · VideoShape    │
│  (158 types · 131 Stable methods · 17 sentinels)        │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                  Domain Layer (internal/)                │
│                                                         │
│  document/geometry   Geometry/fill/effect parsing        │
│  document/style      Style resolution chain              │
│  document/text       Text field/body/fragment helpers    │
│  document/media      Media input contract + probing      │
│  document/table      Logical grid + cell helpers         │
│  document/model      Shared value types (EMU, IDs)       │
│                                                         │
│  chart               Chart XML parse/build/validate      │
│  bind                Template binding internals          │
│  ir                  Read-only IR + semantic diff        │
│  engine              Orchestration (CLI + WASM shared)   │
│  archlint            CI dependency direction linter      │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                  Format Layer                            │
│                                                         │
│  ooxml             OOXML read-only projection            │
│  ooxml/schema      Generated types from ECMA-376 XSD    │
│                    (8 files · 7,300+ lines)              │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────┴────────────────────────────────┐
│                  Transport Layer                         │
│                                                         │
│  opc               OPC package loading, relationships,   │
│                    content types, atomic save             │
│  xmlstore          XML scanner, node tree, SpanPatch     │
│                    engine                                │
└─────────────────────────────────────────────────────────┘
```

**Dependency rule**: `internal/*` must not import the public facade (exception: `internal/engine` as the orchestration layer). Enforced by `internal/archlint` in CI.

---

## Package Structure

```text
pptx/                   Public SDK facade (158 types, 131 Stable methods)
internal/
  opc/                  OPC package loading, ZIP index, relationships, content types, atomic save
  xmlstore/             XML scanner, indexed tree, namespace scope, SpanPatch engine
  ooxml/                OOXML read-only projection (shapes, notes, timing, chart)
  ooxml/schema/         Generated types from ECMA-376 Transitional XSD (8 namespaces, 7,300+ lines)
  document/
    model/              Shared value types: SlideID, ShapeID, EMU, Point, Rect
    geometry/           Geometry/fill/effect parsing (prstGeom, custGrad, fill kinds)
    style/              Style resolution chain (run/paragraph/list-level/theme, color transforms)
    text/               Text field parsing, body/fragment helpers, font resolution
    media/              Media input contract (MediaSource, bounded copy, image detection)
    table/              Logical grid, cell helpers, merge spans
  chart/                Chart XML model: parse/build/canonical/validate/fragment/workbook
  bind/                 Template data-binding internals (Member, AsSlice, Truthy)
  ir/                   Read-only intermediate representation + semantic diff
  engine/               Orchestration layer (CLI + WASM shared, ProjectIR adapter)
  archlint/             CI dependency direction linter (ADR-030 mechanism 4)
  editplan/             Single-part and multi-part edit plans (StageAdd/Delete/Patch)
  textmap/              Text rune mapping and span location primitives
  textutil/             Shared XML text utilities
  audioprobe/           Audio container probing (MP3 frame parsing, WAV format variants)
  videoprobe/           Video container probing (MP4 moov/mvex, WebM doc type)
  diag/                 Cross-layer diagnostic types (Severity, Diagnostic)
  errs/                 Stable error codes and OperationError
  ooxmls/               Namespace URI constants
cmd/
  pptx/                 CLI tool (9 subcommands)
  pptx_check/           WASM browser-check entry point
wasm/
  check/                Browser-side pure-function inspection
  site/                 Offline static inspection page
render/                 Rendering adapter contract (no implementation yet)
testdata/corpus/        Sample index and public gold samples
docs/                   Design docs, ADRs, coverage roadmap, compatibility matrix
```

---

## CLI Commands

| Command | Description | Mode |
|---|---|---|
| `inspect` | Read and summarize document structure (pages, media, notes, capability) as JSON | Read-only |
| `validate` | L0 structural validation with diagnostics; exit code 3 on errors | Read-only |
| `replace` | Literal text replacement across shape bodies (`--old`, `--new`, `--mode`, `--output`) | Write |
| `bind` | Render template with JSON data source (`--data`, `--output`, `--loose`) | Write |
| `diff` | Semantic diff of two presentations (JSON, with `--ignore-geometry/whitespace/notes`) | Read-only |
| `capability` | Emit capability manifest JSON (six-dimension status report) | Read-only |
| `narrate` | Embed audio from `tracks.json` manifest (`--manifest`, `--output`) | Write |
| `timing-plan` | Preview timing sync plan (page jumps, tail padding, strict/skip) | Read-only |
| `export-ir` | Export intermediate representation as JSON (`--output` or stdout) | Write |

Exit codes: `0` success · `1` runtime error · `2` usage error · `3` capability/validation error · `4` resource limit.

---

## WASM / Browser

go-pptx compiles to WebAssembly for browser-side read-only inspection. Files never leave the user's device.

```go
// Three pure functions exposed via syscall/js
wasm/check.Inspect(ctx, pptxBytes, fileName)    // → JSON with IR projection
wasm/check.Validate(ctx, pptxBytes, fileName)   // → JSON with validation diagnostics
wasm/check.Capability(fileName)                  // → JSON with capability manifest
```

**Build**:
```bash
GOOS=js GOARCH=wasm go build -o wasm/site/check.wasm ./cmd/pptx_check
```

**Static site**: `wasm/site/` contains `check.html`, `check.js`, and `wasm_exec.js`. Open `check.html` in any modern browser — no server required.

---

## Quality & Verification

| Metric | Value |
|---|---|
| **Coverage (full repo)** | 84.4% weighted total |
| **Coverage (pptx facade)** | 83.2% (threshold: 82%) |
| **Coverage (internal packages)** | 15 packages ≥ 85%, 9 packages ≥ 90% |
| **CI Gate** | `scripts/coverage/gate.sh` enforces per-package thresholds |
| **Corpus** | 36 real PPTX samples + 3 public LibreOffice gold samples |
| **L3 Compatibility** | PowerPoint 16.0 + WPS 12.1: 8/8 pass, no repair prompts |
| **API Surface** | 158 types · 40 Stable sections · 131 methods · 17 sentinels (golden-locked) |
| **Cross-compile** | `CGO_ENABLED=0` on Linux, `js/wasm`, `wasip1/wasm` |
| **Static Analysis** | `go vet` + `gofmt` (default + corpus build tags) |
| **Dependency Direction** | `internal/archlint` enforces R1-R5 rules in CI |

---

## Documentation

| Document | Description |
|---|---|
| [Architecture Baseline](docs/architecture-current.md) | Current package layout and dependency directions |
| [Coverage Roadmap](docs/coverage-roadmap.md) | Per-package thresholds and CI gate definition |
| [Client Compatibility Matrix](docs/client-compat-matrix.md) | L3 real-client evidence |
| [ADR-030: v2.0 Target Architecture](docs/adr/ADR-030-v2-target-architecture.md) | Layered architecture design decision |
| [Complete Design Spec V2.6](docs/go-pptx_完整设计方案_V2_6_开发实施版.md) | Full design document |
| [Technical Whitepaper](docs/go-pptx-技术白皮书.md) | Technical whitepaper |
| [Implementation Status](docs/go-pptx-实施状态跟踪.md) | Feature implementation tracking |
| [ADR Directory](docs/adr/) | All architecture decision records (ADR-014 through ADR-030) |

---

## Development

```bash
# Build
go build ./...                                     # CGO_ENABLED=0 (CI enforced)
go build ./pptx/                                   # Public facade only

# Test
go test ./...                                      # All unit + gold tests
go test -tags=corpus ./...                         # With corpus build tag
go test -cover ./...                               # With coverage report

# Lint & Vet
gofmt -l .                                         # Check formatting
go vet ./...                                       # Static analysis

# Coverage Gate
bash scripts/coverage/gate.sh                      # Enforce per-package thresholds
TOLERANCE=0.5 bash scripts/coverage/gate.sh        # With tolerance

# WASM
GOOS=js GOARCH=wasm go build ./cmd/pptx_check     # Build WASM binary
bash scripts/check_wasm.sh                         # Build browser check tool

# Corpus & L3
bash scripts/gen_corpus/run.sh validate testdata/corpus   # Validate corpus index
bash scripts/l3/run_client.sh ppt <src> <dst>             # PowerPoint/WPS real-client test
```

---

## License

[Apache-2.0](LICENSE) — free to use, modify, and distribute. No runtime or licensing costs.
