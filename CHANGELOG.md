# Changelog

All notable changes to go-pptx will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to a [Semantic API Stability](docs/adr/ADR-015-api-stability-tiers.md) model
(`// Stable:` / `// Experimental:` godoc tags). The per-type assignment is maintained in
[`docs/v1.0-freeze-list.md`](docs/v1.0-freeze-list.md).

## [Unreleased]

### Changed

（自 v1.0.1 发布起的进一步变更将登记于此）

## [1.0.1] - 2026-09-12

**v1.0.0 后的首个 patch release**——公共 API 零变化（binary-compat with v1.0.0），主要工作是内部实现层重构、bug 修复、覆盖率收敛、客户端矩阵真机执行与文档体系完善。

关键不变量：`// Stable:` 段落 34（不变）/ Stable 符号 50（不变）/ `// Experimental:` 段 5（不变）/ 公共 type 总数 158（不变）/ 错误哨兵语义锁死（不变）/ 黄金语料 B1 哈希 PASS（不变）。

### Changed

- **ADR-016 渐进式 internal 抽取完成首轮收敛**（commit 76a630f）：新增 `internal/document`（PartStore / ReadStore / PatchStore 契约）、`internal/textmap`（rune 映射）、`internal/editplan`（`SinglePartPatch` / `MultiPartPlan`）；生产业务写入路径全部收敛到 `applySinglePartPatch` / `applyMultiPartPlan`，直接 `stage*/commit` 调用仅保留在 `presentation.go` 与 `document_store.go` adapter 边界内；公开 API 零变化。
- **ir 包表格文本投影闭环**（commit def9140）：`ir.readTableText` / `ir.cellText` 由 0% 覆盖补齐，新增 `ir/table_ir_test.go` 自建最小 zip fixture deck 覆盖 2×2 文本、空单元格、零行表格、非正维度四场景；副作用 `ir.Diff` 现在能更准确地报告表格单元格文本变化，公开 API 签名零变化。
- **覆盖率收敛（COV-01 / 02 / 04）**：
  - COV-01 达成（full total ≥ 80%）：full 78.3% → 80.0%（2026-09-11）
  - COV-02 达成（root ≥ 82%）：root 81.5% → 82.0%，full total 82.5% → 82.9%（2026-09-11）
  - COV-04 评估：放弃统一 90% per-package 口径——90% 仅适用于低层格式包（opc / xmlstore / videoprobe / audioprobe / textmap / editplan），root SDK 目标 85%，command/helper 目标 85%
  - 5 包行为优先补测（2026-09-12）：`internal/editplan` 81.8% → **100.0%** / `internal/textmap` 82.2% → **100.0%** / `internal/opc` 86.6% → 88.9% / `internal/audioprobe` 84.9% → 88.4% / root 82.7% → 82.8%
  - **full-repo 加权 total 83.2% → 84.4%**，4/6 低层格式包 ≥ 90%（`videoprobe` 92.6% / `xmlstore` 90.8% / `editplan` 100% / `textmap` 100%）
- **L3 文档同步收口**（commit dfc27c3）：testdata/corpus/README.md / 实施状态跟踪 / MEMORY.md 共 3 处 L3 客户端矩阵完成事实登记，新增"已登记客户端版本与平台"段。

### Fixed

- **MediaSource nil 流保护**（commit d087aff）：`MediaSource.Reader()` / `MediaSource.Length()` / `MediaSource.MediaType()` 三个公共访问器对 nil receiver 显式返回错误（之前 panic 在 nil deref）。按"由 panic 改为 error"算只加防御不减能力，严格 PATCH。
- **Stable 计数 off-by-one 口径修正**（commit ae48794）：33 独立 type + 17 哨兵 = 50 符号 / 34 段落 / 5 Experimental / 120 API / 158 总；勘误前 6 处文档（CHANGELOG / RELEASE-NOTES-v1.0.0 / v1.0-freeze-list / 实施状态跟踪 / 技术白皮书 / MEMORY.md）数字统一。
- **ADR-014 三处缺陷修正**（commit ae48794）：① "因…而…"悬空残句补全；② 依赖清单补充 `internal/document` / `internal/editplan` / `internal/textmap`；③ 目录树补 `internal/document`。
- **CI corpus-replay 鲁棒性**（commit 6361bdd）：公开样本源缺席从 `Fatalf` 改为 `Skipf`，新克隆优雅跳过，真样本到位自动转真跑。
- **VideoShape 公开访问器与 probe 错误映射覆盖**（commit 6361bdd）。

### Added (documentation-only)

- [`docs/go-pptx-技术白皮书.md`](docs/go-pptx-技术白皮书.md)（commit 0218b29，746 行 / 12 章）—— 综合性技术披露，覆盖产品定位、技术架构、应用场景、对比矩阵、7 项技术创新点详解。
- [`docs/1.x-roadmap.md`](docs/1.x-roadmap.md)（commit 24d4fa6，213 行 / 8 章）—— 1.x 演化窗口路线图：5 候选方向 + 5 阶段路线 + 6 风险 + 5 推荐决策点。
- [`docs/client-compat-matrix.md`](docs/client-compat-matrix.md) L3 真机执行记录（commit a103484 + 5f99d80）—— PowerPoint 16.0.20326 + WPS 演示 12.1.0.28599 × 4 样本 8/8 通过。
- [`docs/RELEASE-NOTES-v1.0.1.md`](docs/RELEASE-NOTES-v1.0.1.md) —— 本版本完整 release notes。

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
