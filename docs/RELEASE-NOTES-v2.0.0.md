# go-pptx v2.0.0 Release Notes

> 2026-09-17 · released as `git tag v2.0.0`
>
> **v2.0 目标架构正式发布（ADR-030 全闭环）。唯一 breaking 点是公共导入路径迁移：`github.com/F31/go-pptx/v2` → `github.com/F31/go-pptx/v2/pptx`。公共 API 签名零变化（api_surface golden 不变），迁移为一次性 import 路径替换。同时发布英文 API 接口参考文档 `docs/api-reference.md`。**

## Table of Contents

- [Version Positioning](#version-positioning)
- [Migration Guide (v1.0.x → v2.0.0)](#migration-guide-v10x--v200)
- [Key Invariants (v1.0.7 → v2.0.0)](#key-invariants-v107--v200)
- [Architecture (ADR-030 Closed)](#architecture-adr-030-closed)
- [New: English API Reference](#new-english-api-reference)
- [Verification](#verification)
- [Compatibility](#compatibility)
- [Known Limitations](#known-limitations)

## Version Positioning

v2.0.0 交付 v2.0 目标架构（ADR-030）：把 go-pptx 从「根包即实现」的单一无层包重构为**分层元仓库**——公共门面 `pptx/` + 域逻辑 `internal/document/*` + 格式层 `internal/ooxml`（schema 只读投影）+ 传输层 `internal/{opc,xmlstore}`。同时收编顶层包（`ir/` → `internal/ir`）并新建编排层 `internal/engine`。

这是 go-pptx 历史上**唯一一次破坏性发布**；后续 v2.x 将维持公共面稳定（ADR-015 Stable 契约）。

## Migration Guide (v1.0.x → v2.0.0)

**变更仅一处：import 路径。**

| Component | v1.0.x | v2.0.0 |
|---|---|---|
| Module | `github.com/F31/go-pptx/v2` | `github.com/F31/go-pptx/v2/pptx` |
| Go code | `import "github.com/F31/go-pptx/v2"` | `import "github.com/F31/go-pptx/v2/pptx"` |
| `go get` | `go get github.com/F31/go-pptx/v2@v1.0.7` | `go get github.com/F31/go-pptx/v2/pptx@v2.0.0` |

```bash
# 一次性替换所有 importer：
grep -rl '"github.com/F31/go-pptx/v2"' --include='*.go' <your-project> | xargs sed -i \
  's|"github.com/F31/go-pptx/v2"|"github.com/F31/go-pptx/v2/pptx"|g'
```

**无 API 签名变化**：类型、方法、常量、哨兵逐一对应（golden 锁定）。`render` 包仍为公共契约（`render → pptx/pptx`）。

## Key Invariants (v1.0.7 → v2.0.0)

| Item | v1.0.7 | **v2.0.0** | Conclusion |
|---|---:|---:|---|
| `// Stable:` sections | 40 | **40** | unchanged |
| Stable symbols | 60 | **60** | unchanged |
| `// Experimental:` sections | 0 | **0** | unchanged |
| Exported API types | 163 | **163** | unchanged |
| Stable methods | 131 | **131** | unchanged |
| Error sentinels | 17 | **17** | unchanged |
| Corpus B1 hashes | PASS | **PASS** | unchanged |

## Architecture (ADR-030 Closed)

```
pptx (public facade) ─→ internal/document (domain) ─→ internal/ooxml (format) ─→ internal/opc + internal/xmlstore (transport)
                             ↑
              internal/engine orchestrates cross-domain (Open/Save/Bind/Clone)
cmd/pptx ─→ internal/engine
wasm/check ─→ internal/engine (pure-function subset)
internal/ir / diff ─→ internal/ooxml (read-only projection, no facade)
render ─→ pptx (public contract only; facade never depends on render)
```

**All DR-030 evolution steps closed:**

| Step | Delivery |
|---|---|
| 1. ooxml/schema pipeline | ECMA-376 Transitional XSD → generated Go types (8 namespaces, ~7,300 lines), read-only projection only |
| 2. Domain migration | geometry / style / text / media / table / bind → `internal/document/*` + `internal/{chart,bind}` |
| 3. Facade convergence | root → `pptx/` (the only breaking point); golden unchanged |
| 4. Top-level pack consolidation | `ir/` → `internal/ir` + facade decoupling (eats model + xmlstore only) |
| 5. Engine orchestration | `internal/engine` unifies CLI + WASM Inspect/Validate/Capability; hosts `ProjectIR` |
| 6. CI dependency linter | `internal/archlint` enforces R1–R5 (exception: only `internal/engine`) |
| 7. Gate completeness | `go list ./...` ⊄ FLOORS∪SKIP → FAIL; new packages registered |

**Internal/projection fidelity**: engine `projectShapes` reads shapes, text, tables, notes, timing, hidden flags, and **charts** via `internal/ooxml` schema projection (`ChartPartOf`/`ChartData`) — zero facade handle reads in the IR build path.

## New: English API Reference

新增 `docs/api-reference.md`——由纯 std-lib AST 生成器 `scripts/gen/apidoc` 生成，口径与 `api_surface_test` golden 一致：

```
Exported types      163
Top-level functions 36
Exported methods    143   (methods on exported types)
Constants           149
Vars & sentinels    18
```

Regenerate: `go run ./scripts/gen/apidoc`

## Verification

```bash
go test ./...                                     # all packages PASS
go test -tags=corpus ./...                        # corpus gold samples (B1)
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v ./pptx   # AST gates
bash scripts/coverage/gate.sh                     # per-package coverage FLOORS
go test ./internal/archlint/                      # dependency-direction R1–R5
gofmt -l .                                        # empty
go vet ./... && go vet -tags=corpus ./...         # zero warnings
CGO_ENABLED=0 GOOS=js GOARCH=wasm go build ./...          # WASM target
CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm go build ./...      # WASI target
CGO_ENABLED=0 go build ./...                              # native
```

## Compatibility

- **v1.0.7 → v2.0.0：BREAKING（仅 import 路径迁移）**。API 签名、stable 段、golden、corpus B1 全不变。
- 下游升级动作：**必须**。迁移 = sed 替换 import 路径（见上），零代码逻辑改动。
- 读取兼容：v1.0.7 产物与 API 完全兼容；编辑功能与保真保证不变。

## Known Limitations

- 与 v1.0.7 相同的设计态不承诺（非缺陷）：原生渲染实现、OLE/SmartArt、批注/审阅、OOXML 数学、`custGeom` 顶点编辑。
- 媒体/profile 后续切片、逐步以生成类型替换手写 parse、补齐 `xsd:any`/p14/morph 扩展 → v2.1+ 增量，不在 v2.0.0 范围。

## Feedback

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 适用场景：所有在 v1.0.x 上构建的下游（升级至 `pptx/` 导入路径）；新项目直接 `go get github.com/F31/go-pptx/v2/pptx@v2.0.0`。