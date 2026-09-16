# ADR-027: 音频形状可见几何修复（AudioSpec 补齐 X/Y/Width/Height）

- 状态：**已实施**（2026-09-16）
- 严重级别：**中**——音频可播放但图标不可见（0×0），影响可用性
- 关联：[ADR-025](ADR-025-audio-ooxml-compliance-fix.md)（audio 合规修复）、[ADR-026](ADR-026-video-ooxml-compliance-fix.md)（video 同源修复）

## 上下文

`buildAudioPicFragment`（`audio.go`）在生成 audio 的 `p:pic` 片段时**硬编码几何框为 0×0**：

```xml
<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></a:xfrm>…</p:spPr>
```

原注释称其为"占位几何：0×0 EMU；调用方一般随后调 Geometry/Move"，但仓库中**不存在**设置形状几何的公开/内部写 API（`Shape` 接口只有只读的 `Bounds()`/`WorldQuad()`；`MoveShape` 仅调整 z-order）。因此音频形状一旦创建就永久停留在 0×0。

对比其它形状的 spec：

| Spec | 几何字段 |
|---|---|
| `TextBoxSpec` | `X, Y, Width, Height`（`create.go`） |
| `PictureSpec` | `X, Y, Width, Height`（`picture.go`） |
| `VideoSpec` | `X, Y, Width, Height`（**必填**，`video.go:74`） |
| `AudioSpec` | **无**（`audio.go`） |

**实证**：生成的公开样本 `s004-audio.pptx` 在 PowerPoint 中打开后**看不到任何音频图标**；解包核对确认 `p:pic` 的 `a:ext cx="0" cy="0"`。音频 Part、关系、`a:audioFile r:link`、`p:timing` 节点均正确（ADR-025 已闭合），唯独形状不可见。

## 决策

1. **`AudioSpec` 追加 `X, Y, Width, Height int64`**（EMU）——与 `VideoSpec` 同侧语义，但**允许零值**以保持向后兼容。
2. **新增 `audioGeometry(spec)` helper**：全零时补默认 `914400×914400 EMU`（1in×1in）@ `(914400, 914400)`（1in,1in，与 PowerPoint 插入音频的默认位置一致）；仅位置或仅尺寸为零时按"缺失维度补默认、已给维度保留"处理。
3. **`buildAudioPicFragment` 增加 `ox, oy, cx, cy int64` 参数**并写入真实 `a:xfrm`。
4. **不放宽任何校验**；不动 `p:timing` 与关系结构。

向后兼容性：`AudioSpec` 位于 default API 档（非 Stable），追加字段符合"仅追加"原则；全零走默认值，现有调用方（含全部既有测试）行为不变但**由不可见变为可见**。

## 验证

- 新增守门测试（`audio_ooxml_compliance_test.go`）：
  - `TestBuildAudioPicFragmentIsSchemaCompliant` 扩展断言——几何必为非零，且不得出现 `cx="0" cy="0"`。
  - `TestAudioGeometryDefaults` 表驱动（全零 / 全显式 / 仅位置 / 仅尺寸）。
  - `TestAudioShapeHasVisibleBounds` 端到端——`AddAudio` 后音频 `p:pic` 片段含非零 `a:ext`。
- 重新生成 `s004-audio`：`<a:off x="914400" y="2743200"/><a:ext cx="914400" cy="914400"/>`。
- `go test ./...`、`go test -tags=corpus ./...`、`corpus.py validate`（37 样本 0 错误）全绿。

## 后果

- 音频形状默认可见（1in×1in 喇叭图标），调用方可通过 `AudioSpec.X/Y/Width/Height` 精确定位。
- 这是**第三处 audio/video/media 系列的遗漏**（前两处为 ADR-025/026 的 schema 合规）。共同教训：**新增 media 形状类型时应与既有 spec 逐字段对齐**，`VideoSpec` 已含几何字段而 `AudioSpec` 没有，属复制/对齐时的漏项。
- 真机播放验证仍待在装有 PowerPoint/WPS 的 Windows 机器上执行（音频图标可见性已由 XML 断言覆盖，播放效果需真机）。

## 后续

1. 真机打开 `s004-audio.pptx` 确认图标可见、点击可播放（需 Office 环境）。
2. 排查其它 media 类形状是否同样缺几何字段（如未来新增的 OLE / `p:media`）。
