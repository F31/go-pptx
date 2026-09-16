# ADR-027: 音频形状可见性修复（几何框 + poster 图标）

- 状态：**已实施**（2026-09-16）
- 严重级别：**中**——音频可播放但图标不可见（0×0 且 blip 指向音频），影响可用性
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
2. **新增 `audioGeometry(spec, slideW, slideH)` helper**：全零时补默认 `914400×914400 EMU`（1in×1in）并定位到**页面右下角**（距右/下边各 0.25in 边距，`slideW/H` 读自 `presentation.xml` 的 `p:sldSz`，缺失回退 16:9 `12192000×6858000`）；仅给尺寸时定位到右下角，仅给位置时补默认尺寸。
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

- 音频形状默认可见（1in×1in 喇叭图标，位于页面右下角），调用方可通过 `AudioSpec.X/Y/Width/Height` 精确定位。
- 这是**第三处 audio/video/media 系列的遗漏**（前两处为 ADR-025/026 的 schema 合规）。共同教训：**新增 media 形状类型时应与既有 spec 逐字段对齐**，`VideoSpec` 已含几何字段而 `AudioSpec` 没有，属复制/对齐时的漏项。
- 真机播放验证仍待在装有 PowerPoint/WPS 的 Windows 机器上执行（音频图标可见性已由 XML 断言覆盖，播放效果需真机）。

## 后续

1. 真机打开 `s004-audio.pptx` 确认图标可见、点击可播放（需 Office 环境）。
2. 排查其它 media 类形状是否同样缺几何字段（如未来新增的 OLE / `p:media`）。

---

## 续：poster 图标（`a:blip` 指向音频而非图片）

几何修复后**仍不可见**——真机复查确认另有根因。

### 第二个根因

`buildAudioPicFragment` 把 `p:blipFill/a:blip@r:embed` 指向**音频关系**（`.../audio` → `audio1.wav`）：

```xml
<Relationship Id="rId2" Type=".../audio" Target="../media/audio1.wav"/>
...
<a:blip r:embed="rId2"/>     <!-- 把 WAV 当图片嵌入 → PowerPoint 无法解码 → 图标空白 -->
```

PowerPoint 的音频形状是 `p:pic`：`a:blip` 必须指向一张**图片（poster 图标）**作为可见的喇叭按钮，音频本身只经 `p:nvPr/a:audioFile@r:link` 关联。取证：PowerPoint 原生插入音频的实包（`.l3-output/ref-ppt-inserted.pptx`）：

```xml
<p:pic>
  <p:nvPicPr>
    <p:cNvPr id="2" name="ref-audio">
      <a:hlinkClick r:id="" action="ppaction://media"/>          <!-- 可点击播放 -->
    </p:cNvPr>
    <p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>
    <p:nvPr><a:audioFile r:link="rId2"/></p:nvPr>                <!-- 音频 -->
  </p:nvPicPr>
  <p:blipFill><a:blip r:embed="rId4"/></p:blipFill>              <!-- rId4 → image1.png (图标) -->
  <p:spPr>…<a:ext cx="406400" cy="406400"/></p:spPr>
</p:pic>
```

关系表：`rId2`=audio→wav、`rId4`=image→png。

### 决策（续）

1. **嵌入内置喇叭图标** `assets/audio-speaker.png`（64×64，1.8 KB，`//go:embed`）作为 poster；经 `planMedia`（IMAGE-01 同源去重/命名）与 `relImage` 关系嵌入。
2. **`a:blip` 改指向图标图片**，`a:audioFile` 仍指向音频。
3. **补 `<a:hlinkClick action="ppaction://media"/>`**（`p:cNvPr`）与 `<a:picLocks noChangeAspect="1"/>`（`p:cNvPicPr`）。
4. 不引入 `p14:media` 扩展（ADR-025 已证非必需）。

### 验证（续）

- 生成物关系：`rId2`(audio→wav) + `rId3`(image→png)，`a:blip r:embed="rId3"`、`a:audioFile r:link="rId2"`。
- `TestBuildAudioPicFragmentIsSchemaCompliant` 扩展断言：`a:blip` 必指向 iconRid 且**不得**指向 audioRid；含 `ppaction://media` 与 `picLocks`。
- 全量测试 + corpus（37 样本 0 错误）全绿。

### 后果（续）

- 音频/视频/media 系列至此共四处同类遗漏（ADR-025 schema、026 命名空间、027 几何、027 续 blip 目标）。共同教训：**media pic 片段必须逐一对照客户端原生产物**；"文件能打开"（ADR-025 的最小实验只验证了不判损）**不等于**"图标可见/可交互"。
- 内置图标是库级资源，后续若需主题化（深浅色/自定义）可扩展为可配置 poster（对应 `VideoSpec.PosterSource` 的音频版）。

---

## 续二：播放静音（`vol="80"` 而非 `80000`）

图标可见、位置正确后，**F5 放映仍无声音**（PowerPoint 中显示为静音）。

### 第三个根因

`audioTimingFragment`（`audiotiming.go`）生成 `p:cMediaNode` 时写：

```xml
<p:cMediaNode vol="80">      <!-- ← 错误 -->
```

`vol` 的 XSD 类型是 **`ST_PositiveFixedPercentage`（0..100000，千分比）**，`vol="80"` = **0.08%**，客户端表现为静音。取证：PowerPoint 原生音频（`.l3-output/ref-ppt-inserted.pptx`）为 `vol="80000"`（=80%）。

### 决策（续二）

- `vol="80"` → **`vol="80000"`**（80%，与原生一致）。
- 守门：`TestSetPlayback_AppendsTiming` 增加断言——必须含 `vol="80000"` 且不得含 `vol="80"`。

### 后果（续二）

- 这是 audio 系列的**第五处**遗漏，且性质升级：前四处只影响"能否打开/是否可见"，本处影响**功能可用性（有声/无声）**。
- 一般化教训：**枚举/百分比类属性必须核对 XSD 类型与量纲**；`vol` 的 0..100000 千分比与直觉的 0..100 相差三个数量级，`80` 与 `80000` 在文本上极不显眼，代码级测试若只断言"存在 cMediaNode"则完全抓不到。

---

## 续三：PowerPoint 打不开（缺 `p14:media` 关联）

poster 图标 + 右下角定位 + `vol=80000` 后，真机反馈：**WPS 能打开，PowerPoint 打不开**。

### 第四个根因

`p:nvPr` 只有 `<a:audioFile r:link>`，缺 **`p14:media` 扩展**——poster 图片与媒体文件的关联。取证对照：

| 样本 | `a:blip` 指向 | `p14:media` | PowerPoint |
|---|---|---|---|
| `s001-text.video.fixed.pptx`（ADR-026，已验） | 视频关系 | 无 | ✅ 打开 |
| `ref-ppt-inserted.pptx`（原生） | 图片 | **有** | ✅ 打开 |
| 本 ADR 续二版 s004-audio | 图片 | 无 | ❌ **打不开** |

规律：**当 `a:blip` 指向图片（poster）时，PowerPoint 要求 `p:nvPr/p:extLst/p14:media` 关联媒体**；缺它则判包不可用（WPS 宽容）。此前 ADR-026 决定"不引入 p14:media"仅对 `blip→媒体文件` 成立（video 路线），不适用于 `blip→图片`（audio 路线）。

### 决策（续三）

按原生结构补全（`audio.go`）：

```xml
<Relationship Id="rId2" Type=".../relationships/audio" Target="../media/audio1.wav"/>
<Relationship Id="rId3" Type="http://schemas.microsoft.com/office/2007/relationships/media" Target="../media/audio1.wav"/>
<Relationship Id="rId4" Type=".../relationships/image" Target="../media/image1.png"/>
...
<p:nvPr><a:audioFile r:link="rId2"/>
  <p:extLst><p:ext uri="{DAA4B4D4-6D71-4841-9C94-3DE7FCFB9230}">
    <p14:media xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main" r:embed="rId3"/>
  </p:ext></p:extLst></p:nvPr>
```

新增常量 `nsPowerPoint2010` / `relMedia2007` / `mediaExtURI`（`audio.go`）。**连带修复**：`clone.go` 的 `classifyCloneRel` 需把 `relMedia2007` 归入 media（否则克隆含音频页面时报"unsupported internal relationship type"）。

### 验证（续三）

- 生成物 rels：`rId2`(audio) + `rId3`(media 2007) + `rId4`(image)，pic 结构与原生逐项一致。
- 守门：`TestBuildAudioPicFragmentIsSchemaCompliant` 增加 `p14:media` 与 ext uri 断言。
- SDK 回读：`AudioShape` 仍正确分类（kind=audio、profile track/duration/role、`AudioSource` 可读）、`Validate` 0 错误。
- 全量 + corpus（37 样本 0 错误）全绿。

### 后果（续三）

- audio 系列累计**六处**遗漏。本处再次印证：**"能打开"与"可见/可交互/有声"是三套独立的客户端约束**，必须逐项对照原生实包；且 ADR 的"最小改动"结论有**适用边界**（`blip→媒体` vs `blip→图片`），换路线后需重新取证。
