# ADR-026: 视频形状的 OOXML 合规修复（与 ADR-025 同源）

- 状态：**已实施**（2026-09-16）
- 严重级别：**高**——含视频的产物在 PowerPoint 下完全不可用
- 关联：[ADR-025](ADR-025-audio-ooxml-compliance-fix.md)（audio 的同源缺陷；本 ADR 是其系统性排查的延伸）

## 上下文

ADR-025 修复 audio 的合规缺陷后，顺带排查发现 `buildVideoPicFragment`（`video.go`）产出的视频形状**与 audio 的原缺陷完全同构**：

```xml
<p:nvPicPr><p:cNvPr .../><p:cNvPicPr/></p:nvPicPr>       <!-- 缺必需的 p:nvPr -->
<p:blipFill><a:blip r:embed="rId2"/><p:videoFile contentType="video/mp4"/></p:blipFill>
                                    <!-- p:videoFile：命名空间错 + 位置错 + 缺 r:link -->
```

实测确认（`s001-text.video.pptx` = SDK `AddVideo` 嵌入 mp4）：

| 客户端 | 结果 |
|---|---|
| PowerPoint 16.0.20326 | **`ERR 0x80070570 文件或目录损坏`** |
| WPS 12.1.0.28599 | `OPEN=ok`，`Video 11` **type=13** |

与 audio 修复前**完全一致**（PowerPoint 拒收 + WPS 宽容且只认 type=13）。

## 取证：PowerPoint 原生视频形状的真实形式

用 COM `Shapes.AddMediaObject2` 让 PowerPoint 自己插入同一段 mp4，保存后读其 `slide1.xml`（`zz_dumpzip.go -find videoFile`）：

```xml
<p:pic>
  <p:nvPicPr>
    <p:cNvPr id="2" name="s001-text.narrated">
      <a:hlinkClick r:id="" action="ppaction://media"/>
      <a:extLst>…</a:extLst>
    </p:cNvPr>
    <p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>
    <p:nvPr>
      <a:videoFile r:link="rId2"/>                                    <!-- ← 正确形式 -->
      <p:extLst><p:ext uri="{DAA4B4D4-…}"><p14:media r:embed="rId1"/></p:ext></p:extLst>
    </p:nvPr>
  </p:nvPicPr>
  <p:blipFill><a:blip r:embed="rId4"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>
  <!-- blip → 封面图（PowerPoint 另存了 image1.png 作为 poster） -->
  <p:spPr>…</p:spPr>
</p:pic>
```

要点：**`a:videoFile`（DrawingML）+ 必需 `r:link`，位于 `p:nvPr` 内**。

## 决策

1. **修 `buildVideoPicFragment`**：补 `p:nvPr`；视频引用改为 **`<a:videoFile r:link="rIdX"/>` 并移入 `p:nvPr`**（原为 `p:videoFile` 置于 `blipFill` 内）；`blipFill` 保留 `a:blip` 并补 `<a:stretch><a:fillRect/></a:stretch>`。
2. **读侧同步**（`picMediaKind`）：先探 `p:nvPr` 内的 `a:videoFile`，**再回退** `blipFill` 内的 `p:videoFile` —— 保留旧格式分支以读回 v1.0.5 及更早产物。
   - 这次是**追加**而非替换。ADR-025 期间曾直接替换命名空间，立即被 `TestSlideAddVideo_PicClassifiedAsVideo` 拦下（旧样本写 `p:videoFile`）——教训：**兼容探测要追加，不要替换**。
3. **消除重复实现**：`hasVideoFile` 改为复用 `picMediaKind`（原为独立的手写 blipFill 探测）。与 ADR-025 中 `lastProfileByMedia` 复用 `parseAudioProfile` 属同一模式。
4. **不引入 `p14:media` 扩展**：PowerPoint 原生带该扩展，但 ADR-025 的最小实验（audio v6/v7）证明不带也被接受；保持最小改动，待实测需要再加。
5. **不放宽任何校验**。

## 验证

| 文件 | PowerPoint 16.0.20326 | WPS 12.1.0.28599 |
|---|---|---|
| 修复前 `s001-text.video.pptx` | **`ERR 0x80070570`** | `OPEN=ok`，`Video 11` type=**13** |
| 修复后 `s001-text.video.fixed.pptx` | ✅ **`OPEN=ok`**，`Video 11` **type=16 (msoMedia)** | ✅ `OPEN=ok`，`Video 11` **type=16** |

与 audio 的修复结果**逐项一致**（含 WPS 的 type 13 → 16 变化）。

`go test ./...` 与 `-tags=corpus ./...` 全绿 —— **含既有的 video 测试**，证明兼容探测（追加式）没有破坏旧格式识别。

## 后果

- **这是一类缺陷，不是两个独立事故**：audio 与 video 的 pic 片段生成函数各自复制了同一套错误写法，读侧也各自维护一份重复探测。ADR-025 修了 audio，本 ADR 补齐 video。
- 一般化教训：**同一模式被复制到多个函数时，真机验证必须覆盖每一个**。代码级测试只断言"自己生成的形态"，无法发现"生成的形态本身不合规"。
- `p:videoFile`（错误写法）长期同时存在于实现与测试中，且注释也写作 `p:videoFile` —— 说明该命名空间**从未被真机校验过**。
- **建议后续排查同类复制点**：`p:videoFile`/`p:audioFile` 之外，`picture.go` 的普通图片 pic 片段已含 `p:nvPr`（`shapes_test.go:36` 可证），但其它 media 类形状（如 `p:media` / OLE 对象）值得同样核对。

## 后续

1. **video 侧的真机播放验证**尚未做——本次只验证到"能打开 + 识别为 type=16"。若要达到 audio 同等强度，需补放映/录屏证据（video 的验证成本高于 audio，且本轮 audio 已完成该闭环）。
2. `p14:media` 扩展是否必要，待有真机播放需求时评估。
