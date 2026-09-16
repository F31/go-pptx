# go-pptx v1.0.6 Patch Release Notes

> 2026-09-16 · 随 `git tag -s v1.0.6`（SSH 签名）发布
>
> **v1.0.5 后的第六个 patch release，定位为"媒体 OOXML 合规模块修复"——纯 bug-fix，公共 API 表面零变化、binary-compat with v1.0.0–v1.0.5。** v1.0.5 → v1.0.6 仅含 ADR-025（配音）/ ADR-026（视频）两条修复线及其守门测试与文档。

## 版本定位

本版把 v1.0.5 之后落在 `main` 上、尚未进入任何 tag 的两条高危修复一并发布：

1. **ADR-025 配音产物被 PowerPoint 拒收（高危）**：为闭合 V2.6 §15.3 第 3 条语料缺口制作出首个含配音样本后，实测暴露 `0x80070570 文件或目录损坏`——根因是 audio `p:pic` 缺 `p:nvPr`、`a:audioFile` 缺 `r:link` 且位置错，外加 `SetAdvanceAfter` 看不见 `mc:AlternateContent` 包裹的既有 `p:transition` 而追加第二个（违反 `CT_Slide` 的 `maxOccurs=1`）。WPS 宽容接受掩盖了它。
2. **ADR-026 视频产物被 PowerPoint 拒收（高危，与 ADR-025 同源）**：顺带排查发现 video `p:pic` 存在**完全同构**缺陷（`p:videoFile` 命名空间错、缺 `r:link`、位置错）。同一套错误写法被复制到 audio/video 两处，读侧也各维护一份重复探测——本版一并修复并新增守门测试。

> **关于版本号**：本版**无任何 API 表面变化**（Stable 段/符号/方法、`// Experimental:`、公共 type 全部与 v1.0.5 一致），按 semver 与本项目"binary-compat 即 patch"口径，patch 准确。

## 关键不变量（v1.0.0 → v1.0.6 全程）

| 项 | v1.0.5 | **v1.0.6** | 结论 |
|---|---:|---:|---|
| `// Stable:` 段落数 | 40 | **40** | 不变 |
| Stable 符号数 | 60 | **60** | 不变 |
| `// Experimental:` 段落数 | 0 | **0** | 不变 |
| 公共 API type 总数 | 163 | **163** | 不变 |
| Stable 方法数 | 131 | **131** | 不变 |
| 错误哨兵语义 | 锁死 | 锁死 | ✅ 不变 |
| 黄金语料 B1 哈希 | PASS | PASS | ✅ 不变 |

## Fixed

- **含配音的产物在 PowerPoint 下被判"文件或目录损坏"（高危，ADR-025）**：见 CHANGELOG `[1.0.6]` 段。要点：补 `p:nvPr` + `a:audioFile r:link` 移入 `p:nvPr`；`SetAdvanceAfter` 展开 mc 两分支写 `advTm`；读取侧 `picMediaKind` / `classifyShape` / `Slide.AdvanceAfter` 同源修复；`lastProfileByMedia` 复用 `parseAudioProfile`（修复 `Profile().Duration` 恒为 0）。新增 `audio_ooxml_compliance_test.go` + 端到端 `TestNarratedDeckRoundTrip`。
- **含视频的产物在 PowerPoint 下被判"文件或目录损坏"（高危，ADR-026，同源）**：video `p:pic` 补 `p:nvPr` + 引用改为 `a:videoFile r:link` 移入 `p:nvPr`；读侧 `picMediaKind` **追加**正确命名空间探测并**保留**旧 `p:videoFile` 分支（兼容 v1.0.5 及更早产物）；`hasVideoFile` 复用 `picMediaKind`。新增 `video_ooxml_compliance_test.go`。
- **同类缺陷防御性排查（ADR-026 教训）**：扫描全部形状构建器，确认 `p:sp` / 普通图片 `p:pic` / 视频 poster `p:pic` / 图表 `p:graphicFrame` 均结构正确（含必需的 nvPr 族节点）；OLE 对象无生产构建器。仅 audio/video 两个媒体 `p:pic` 曾是缺陷载体，均已修复，**无残留同类缺陷**。

## L3 真机客户端矩阵（第三~五轮，配音/视频播放验证）

| 验证项 | 结果 |
|---|---|
| 配音产物（ADR-025） | PowerPoint 16.0.20326 + WPS 12.1.0.28599 均 `OPEN=ok` + `SAVE=ok`、音频识别 `type=16`；人工录屏含 `soun` 轨且人耳确认可闻 → §15.3 第 3 条闭合 |
| 视频产物（ADR-026） | 两家客户端均 `OPEN=ok`、`Video 11` 均 `type=16`；**2026-09-16 用户确认视频可正常播放** |

详见 [`docs/client-compat-matrix.md`](client-compat-matrix.md)（第三~五轮）。

## Verification（如何验证）

```bash
go test ./...                                     # 14/14 包 PASS
go test -tags=corpus ./...                        # 含 B1 金样比对
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v .   # 7/7 AST 守门

CGO_ENABLED=0 go vet ./...                        # 零警告
CGO_ENABLED=0 go vet -tags=corpus ./...           # 零警告
gofmt -l .                                        # 零输出
CGO_ENABLED=0 GOOS=js     GOARCH=wasm go build ./...
CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm go build ./...
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build ./...

# 覆盖率复核（应得 internal/opc 90.3%，root 合并口径 84.4%）
go test ./... -cover
```

## Compatibility

- **v1.0.5 → v1.0.6：binary-compatible + API-compatible（零变化）**——本次为纯 bug-fix，公共 API 表面与 v1.0.5 逐项一致；**输出 PPTX 字节布局不变**（无 Tier 2 类改动）。
- 下游升级动作：**建议**。任何生成含音频/视频 PPTX 的下游都应升级——否则产物在 PowerPoint 下会被判损坏（`0x80070570`）。
- ⚠️ **兼容性读取**：v1.0.6 可正常读取 v1.0.5 及更早生成的音频/视频产物（读侧保留旧 `p:videoFile` / `blipFill` 内 `a:audioFile` 探测分支）。

## 已知限制（本版未闭合）

- 无新增限制。V2.6 §15.3 五条发布硬门槛已全部闭合。

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 本版适用场景：所有生成含音频/视频 PPTX 的下游（修复 PowerPoint 拒收高危缺陷）。
