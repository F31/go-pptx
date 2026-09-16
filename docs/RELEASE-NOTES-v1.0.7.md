# go-pptx v1.0.7 Patch Release Notes

> 2026-09-16 · 随 `git tag -s v1.0.7`（SSH 签名）发布
>
> **v1.0.6 后的第七个 patch release，定位为"音频形状可用性修复"——修复 6 处独立缺陷，并使 1 项默认行为更合理。唯一公共 API 变化是 `AudioSpec` 追加位置/尺寸字段（仅追加，binary-compat with v1.0.0–v1.0.6）。**

## 版本定位

本版把 v1.0.6 之后落在 `main` 上、尚未进入任何 tag 的音频修复线（ADR-027 及其续）一并发布。核心价值：**音频形状从"能生成但客户端三态各异"变为 PowerPoint 与 WPS 均可打开、图标可见、F5 放映自动出声。**

这轮修复源自一个真实的用户旅程：生成公开音频样本 `s004-audio`（闭合 V2.6 §15.3 第 3 条）后，逐步暴露 **6 处彼此独立的缺陷**——"能否打开 / 图标是否可见 / 是否可点击 / 是否有声"是**四套独立**的客户端约束，缺任一都表现为不同症状：

| # | 缺陷 | 症状 | 严重度 | ADR |
|---|---|---|---|---|
| 1 | 音频形状硬编码 `0×0` 几何 | 图标不可见 | 中 | ADR-027 |
| 2 | `a:blip` 指向音频文件（无 poster） | 无图标 | 中 | ADR-027 续 |
| 3 | `vol="80"` 量纲错（应为 80000） | 静音（0.08%） | 中 | ADR-027 续二 |
| 4 | 缺 `p14:media` 关联 | **PowerPoint 打不开** | 高 | ADR-027 续三 |
| 5 | 极简计时结构 | WPS 不自动播放 | 中 | ADR-027 续四 |
| 6 | 默认定位（0×0 → 右下角）| 行为改进 | — | ADR-027 续 |

## 关键不变量（v1.0.0 → v1.0.7）

| 项 | v1.0.6 | **v1.0.7** | 结论 |
|---|---:|---:|---|
| `// Stable:` 段落数 | 40 | **40** | 不变 |
| Stable 符号数 | 60 | **60** | 不变 |
| `// Experimental:` 段落数 | 0 | **0** | 不变 |
| 公共 API type 总数 | 163 | **163** | 不变 |
| Stable 方法数 | 131 | **131** | 不变 |
| 错误哨兵语义 | 锁死 | 锁死 | ✅ 不变 |
| 黄金语料 B1 哈希 | PASS | PASS | ✅ 不变 |

> 唯一 API 变化：`AudioSpec` 追加 `X/Y/Width/Height int64`（EMU，仅追加；零值走默认，既有调用方行为不变）。

## Added

- **`AudioSpec.X/Y/Width/Height`（EMU，仅追加）**：音频形状位置与尺寸；全零时默认 `1in×1in` 定位到**页面右下角**（距右/下边各 0.25in）。新增 `audioGeometry` 与 `Presentation.slideSize()`。
- **内置喇叭图标** `assets/audio-speaker.png`（64×64 PNG，`//go:embed`）：音频 `p:pic` 的 poster。
- **公开音频样本** `testdata/corpus/s004-audio/` + 生成脚本 `scripts/gen_audio/main.go`：公开语料 36 → **37**。

## Fixed

见 CHANGELOG `[1.0.7]` 段的 6 条。要点：

- `buildAudioPicFragment`：补几何框、poster 图标（`a:blip→image`）、`ppaction://media`、`picLocks`、`p14:media`（Microsoft 2007 media 关系）。
- `audioTimingFragment`：改原生媒体播放形态（`p:seq` + `p:cmd playFrom(0.0)` + `p:cMediaNode delay="indefinite"`），`vol="80000"`。
- `clone.go`：`classifyCloneRel` 归类 `.../2007/relationships/media`。
- 守门：`TestBuildAudioPicFragmentIsSchemaCompliant`（扩几何/poster/p14:media 断言）、`TestAudioGeometryDefaults`、`TestAudioShapeHasVisibleBounds`、`TestSetPlayback_AppendsTiming`（`vol="80000"`）。

## L3 真机客户端矩阵（第五轮）

| Client | Sample | Open | 图标可见 | F5 自动出声 |
|---|---|---|---|---|
| PowerPoint 16.0.20326 | `s004-audio` | ✅ | ✅ | ✅ |
| WPS 12.1.0.28599 | `s004-audio` | ✅ | ✅ | ✅ |

详见 [`docs/client-compat-matrix.md`](client-compat-matrix.md) 第五轮。

## Verification（如何验证）

```bash
go test ./...                                     # 全包 PASS
go test -tags=corpus ./...                        # 含 B1 金样比对（37 样本）
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v .   # 7/7 AST 守门

CGO_ENABLED=0 go vet ./...                        # 零警告
CGO_ENABLED=0 go vet -tags=corpus ./...           # 零警告
gofmt -l .                                        # 零输出
CGO_ENABLED=0 GOOS=js     GOARCH=wasm go build ./...
CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm go build ./...
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build ./...

go run ./scripts/gen_audio                        # 复现 s004-audio 样本
```

## Compatibility

- **v1.0.6 → v1.0.7：binary-compatible + API-compatible（仅 `AudioSpec` 追加字段）**——Stable 段/符号/方法/哨兵计数不变。
- 下游升级动作：**建议**。任何生成含音频 PPTX 的下游都应升级——否则产物在各客户端表现为：图标不可见 / 无 poster / 静音 / PowerPoint 拒开 / WPS 不自动播放。
- ⚠️ **读取兼容**：既有含音频产物仍可正常读取（形状分类与 `AudioProfile` 解析不变）。

## 已知限制（本版未闭合）

- 无新增限制。V2.6 §15.3 五条发布硬门槛全部闭合，且经公开样本 `s004-audio` 真机复现。
- 设计态不承诺（非缺陷）：原生渲染实现、OLE/SmartArt、批注/审阅、OOXML 数学、`custGeom` 顶点编辑。

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 本版适用场景：所有生成含音频 PPTX 的下游（修复图标不可见 / 静音 / PowerPoint 拒开 / WPS 不自动播放）。
