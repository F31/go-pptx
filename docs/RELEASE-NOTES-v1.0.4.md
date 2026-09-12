# go-pptx v1.0.4 Patch Release Notes

> 2026-09-13 · 随 `git tag -s v1.0.4`（SSH 签名）发布
>
> **本版本是 v1.0.3 后的第四个 patch release——质量里程碑版**。v1.0.3 → v1.0.4 共 10 个 commit，**全部为测试与文档增量，零生产代码改动**。公共 API 表面与 v1.0.3 逐项一致，**binary-compat with v1.0.0 / v1.0.1 / v1.0.2 / v1.0.3**。

## 版本定位

- **不是功能版**：无新 API、无行为变化、无 bug 修复（生产代码零改动）。
- **是质量锚点**：6 轮覆盖率补测将根包合并口径覆盖率从 82.3% 提升到 **84.4%**，**零覆盖函数清单全部清零**（含 `Rect.Contains` 这类"全仓无生产调用方的挂着的公开 API"），测试资产 **+1251 行 / 15 个测试文件**。此后任何覆盖率回退、API 盲区复活都会被这批回归测试在 `go test` 阶段拦截。
- **下游动作**：无需任何升级动作——`go get` 升到 v1.0.4 与留在 v1.0.3 行为完全一致；升级的唯一收益是依赖一个经过更充分回归验证的基线。

## 关键不变量（v1.0.0 → v1.0.4 全程不变项）

| 项 | v1.0.0 | v1.0.1 | v1.0.2 | v1.0.3 | v1.0.4 | 结论 |
|---|---:|---:|---:|---:|---:|---|
| `// Stable:` 段落数 | 34 | 34 | 34 | 34 | 34 | ✅ 不变 |
| Stable 符号数（type + 哨兵） | 50 | 50 | 50 | 50 | 50 | ✅ 不变 |
| `// Experimental:` 段落数 | 5 | 5 | 5 | 5 | 5 | ✅ 不变 |
| 公共 API type 总数 | 158 | 158 | 158 | 158 | 158 | ✅ 不变 |
| Stable 方法数 | 127 | 127 | 127 | 129 | 129 | ✅ 不变（129 为 v1.0.3 追加的 2 只读方法） |
| 错误哨兵语义 | 锁死 | 锁死 | 锁死 | 锁死 | 锁死 | ✅ 不变 |
| 黄金语料 B1 哈希 | PASS | PASS | PASS | PASS | PASS | ✅ 不变 |

> `api_surface_test.go` 的 7 个 AST 断言（golden 名单优于计数、`parser.ParseDir` 优于 grep）全程 PASS——公共 API 任何意外漂移都会在 `go test` 失败。

## 覆盖率提升明细（6 轮补测）

| 轮次 | commit | 目标函数 | 覆盖率 |
|---|---|---|---|
| 1 | `120c1b3` | `OpaqueShape.Kind()` 三路径 | 0% → 100% |
| 2 | `a2efe4a` | `Rect.Contains`（22 子测：边界 / 越界 / 负宽高 / 退化 / 负坐标） | 0% → 100% |
| 3 | `dbaf0d3` | `classifyCloneRel` / `retargetRel` / `fallbackCloneCT` | 75% / 60% / 28.6% → 100% |
| | | `splitTrailingDigits` | 78.6% → 92.9%* |
| 4 | `968110a` | `leafTextPatch` / `translateSentinel` / `clamp01` | 40% / 30% / 60% → 100% |
| | | `resolveColorSpec`（srgbClr / sysClr / schemeClr / clrMap 间接映射） | 39.1% → 95.7% |
| 5 | `315e077` | 10 个 0% 函数（`chartTypeFromPlot` / `GeometryKind.String` / `EffectKind.String` / `appendPartUnique` / `inferCols` / `sizeCentipoints` / `parseHexRune` / `parseDecRune` / `nsPrefix` → 100%，`kindIndex` → 90.9%） | 0% → 100% |
| | | `cloneChangeSet`（深拷贝回滚语义）/ `rPrChildRank` / `xmlUnescape` | 11.8% / 23.5% / 54.8% → 100% |
| | | `timingReferencesShape` | 15.8% → 94.7% |

\* 剩余 1 行为 `strconv.Atoi` 整数溢出退化分支（需构造 19+ 位全数字串），本质测 stdlib 而非本库逻辑——按"覆盖追逻辑分支，不追退化安全网"原则明确不追。

**合计**：根包合并口径（`-coverpkg=./.`）**82.3% → 84.4%**；零覆盖公开函数清零；同文件 <90% 函数主动扫描后全部补齐。

## 测试方法论（本版沉淀，已入项目约定）

- 纯函数 helper 优先表驱动单元测试，不走 PPTX fixture（体积小约 100 倍、回归快）；
- 零覆盖公开函数必须消除（即使无生产调用方也是 API 盲区）；
- bug-registry 扫描时主动查所有同文件 <90% 函数（主表可能漏列）；
- 单 commit 多函数回报是高 ROI 模式。

## Verification（如何验证）

```bash
go test ./...                                   # 全包 PASS
go test -tags=corpus ./...                      # 含 B1 金样比对
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v .  # 7/7 AST 守门 PASS

CGO_ENABLED=0 go vet ./...                      # 零警告
CGO_ENABLED=0 GOOS=js GOARCH=wasm go build ./...  # WASM 编译通过

# 覆盖率复核（应得 root 合并口径 ≈ 84.4%）
CGO_ENABLED=0 go test -coverprofile=/tmp/cover.out -coverpkg=./. ./...
go tool cover -func=/tmp/cover.out | tail -1
```

## Compatibility

- v1.0.0 → v1.0.1 → v1.0.2：binary-compatible（公共 API 零变化）
- v1.0.2 → v1.0.3：binary-compatible + API-compatible（仅追加 2 Stable 只读方法 + 1 Experimental IR 字段）
- **v1.0.3 → v1.0.4：binary-compatible + API-compatible（零生产代码改动；v1.0.3..v1.0.4 的 20 个变更文件均为 `*_test.go` 与 `.workbuddy` 文档）**
- v1.0.4 → v1.1.0（未来）：待 ADR-019 图表 numFmt / FEAT-002 项 2 阅读顺序，按 `docs/1.x-roadmap.md` 决定

## Fixed (documentation)

- CHANGELOG `[1.0.3]` 段日期笔误修正：`2026-09-22` → `2026-09-12`（tag 实际创建日；早前会话时钟漂移所致）。

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 本版适用场景：希望依赖一个覆盖率与回归资产更充分的质量基线的下游项目（如 ppts）；其余调用方可按需停留在 v1.0.3，行为完全一致。
