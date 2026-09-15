# ADR-025: 含配音产物的 OOXML 合规修复（PowerPoint 拒收 0x80070570）

- 状态：**已实施**（2026-09-16）
- 严重级别：**高**——含配音的产物在 PowerPoint 下完全不可用
- 关联：[ADR-018](ADR-018-save-streaming-copy.md)（Tier 2 输出字节）、[ADR-024](ADR-024-saveplan-duplicate-entry-fix.md)（同为"真机才暴露"的产物缺陷）
- 影响面：`Slide.AddAudio` / `UpsertNarration` 写出的音频形状；`Slide.SetAdvanceAfter` 的过渡节点写入

## 上下文

V2.6 §15.3 第 3 条要求"PowerPoint/WPS 的受支持关键用例实际打开无修复提示；**配音功能必须有播放记录**"。该条一直记为**环境型缺口**（语料 36 个 manifest 无任何 audio 标签样本），`docs/release-readiness-2026-09-12.md` 更明确写着"性质是环境型缺口……**不是代码缺陷**"。

2026-09-16 制作出第一个含配音样本（合成 WAV + `AddAudio` + `SetPlayback` + `SetAdvanceAfter`）后，**该判断被实测推翻**：

| 文件 | PowerPoint 16.0.20326 | WPS 12.1.0.28599 |
|---|---|---|
| `s001-text.pptx`（原样本） | `OPEN=ok` | `OPEN=ok` |
| `s001-text.t2.pptx`（Tier 2 产物） | `OPEN=ok` | `OPEN=ok` |
| **`s001-text.narrated.pptx`（配音产物）** | **`ERR 0x80070570 文件或目录损坏`** | `OPEN=ok`（形状 type=13） |

同机、同 COM 会话的对照实验排除了环境因素。**WPS 宽容接受掩盖了缺陷**——这也是代码级测试与 WPS 验证都没能发现它的原因。

## 取证过程（最小实验矩阵）

第一轮猜测（`p:nvPicPr` 缺 `p:nvPr` + `a:audioFile` 缺 `r:link`）经补丁变体实测**被证伪**：单独补这两处仍失败。后续用补丁工具（对生成物做定点增删、不碰生产代码）做减法定位：

| 变体 | 操作 | PowerPoint |
|---|---|---|
| v1/v2/v3 | 修 `p:pic` 结构（三种写法） | ✗ |
| e3 | 只删 `mc:AlternateContent` | ✗ |
| e4 | 只删外层 `advTm` transition | ✗ |
| e5 | 修 pic + 删 `p:timing` | ✗ |
| **e2** | **修 pic + 删 `mc:AlternateContent`** | **✅** |
| **v6** | **修 pic + `advTm` 写入 mc 两个分支** | **✅** |
| **v7** | **修 pic + 合并为单个 transition** | **✅** |

结论：**两处缺陷都是必要条件**（"与"关系），缺任何一个 PowerPoint 都拒收。v1/v2/v3/e3/e4/e5 全失败恰好逐一排除了其它假设。

## 根因

### 缺陷 1：audio `p:pic` 结构不合规（`buildAudioPicFragment`）

原实现产出：

```xml
<p:nvPicPr><p:cNvPr id="11" name="Audio 11"/><p:cNvPicPr/></p:nvPicPr>  <!-- 缺 p:nvPr -->
<p:blipFill><a:blip r:embed="rId2"/><a:audioFile contentType="audio/wav"/></p:blipFill>
```

两处 OOXML schema 违反：

1. `p:nvPicPr` 缺必需的 **`p:nvPr`**——`CT_PictureNonVisual` 的 `cNvPr`/`cNvPicPr`/`nvPr` 三项均为 `minOccurs=1`。对照：同文件生成的普通图片 `p:pic` 与所有 `p:nvSpPr` 都带 `<p:nvPr/>`（`shapes_test.go:36`），**只有音频 pic 漏写**——是遗漏而非设计选择。
2. `a:audioFile` 缺必需的 **`r:link`**，且位置错误：`CT_AudioFile` 的 `r:link` 为必需；音频引用属于 **`p:nvPr`** 而非 `p:blipFill`。`p:blipFill` 的 `a:blip` 应指向封面图。

代码注释原写"powerpoint 接受的 audio 形状表达之一"——该假设**从未经真机验证**。

### 缺陷 2：`SetAdvanceAfter` 写入重复 `p:transition`（`audiotiming.go`）

源模板（如 `s001-text`）常见形态是把过渡包在标记兼容块里：

```xml
<mc:AlternateContent>
  <mc:Choice Requires="p14"><p:transition spd="slow" p14:dur="2000"/></mc:Choice>
  <mc:Fallback><p:transition spd="slow"/></mc:Fallback>
</mc:AlternateContent>
```

`SetAdvanceAfter` 只遍历 `p:sld` 的**直接子元素**且要求命名空间为 PresentationML，因此**看不见**被 `mc:AlternateContent` 包裹的 transition → 判定"无 transition" → 追加 `<p:transition advTm="2500"/>` → 同一 slide 出现两个 `p:transition`，违反 `CT_Slide` 的 `maxOccurs=1`。

## 决策

1. **修 `buildAudioPicFragment`**：补 `p:nvPr`，把 `<a:audioFile r:link="rIdX"/>` 移入 `p:nvPr`；`p:blipFill` 保留 `a:blip` 引用并补 `<a:stretch><a:fillRect/></a:stretch>`；不再写 `contentType`（媒体类型由 Part 关系 + `[Content_Types].xml` 承载）。
2. **修 `SetAdvanceAfter`**：新增 `alternateContentTransitions` helper 展开 `mc:AlternateContent` 的 Choice/Fallback，把 `advTm` 写给**全部既有 transition**；仅当确实不存在任何 transition 时才新增节点。
   - 选 v6（保真）而非 v7（合并）：保留源模板的 `spd`/`p14:dur` 与 mc 结构，符合"未触碰区域不动"的 B1 原则。
   - mc 的两个分支语义等价，故**两者都写**同一份 `advTm`——否则客户端走降级分支时会丢自动翻页设置。
3. **不放宽任何校验**：既未让 `Load`/`Validate` 忽略结构问题，也未把失败降级为警告。

## 验证

**真机复验（修复后重新生成同一产物）**：

| 客户端 | 修复前 | 修复后 |
|---|---|---|
| PowerPoint 16.0.20326 | `ERR 0x80070570` | ✅ `OPEN=ok slides=1`；识别 `Audio 11` **type=16 (msoMedia)**；`advanceTime=2.5`；`SAVE=ok` |
| WPS 12.1.0.28599 | `OPEN=ok`（type=**13**） | ✅ `OPEN=ok`；识别 `Audio 11` **type=16**；`advanceTime=2.5`；`SAVE=ok` |

副证据：**WPS 的形状类型从 13 变为 16**——修复前 WPS 只把它当作某种占位图片，修复后两个客户端都识别为真正的媒体形状（msoMedia = 16），语义达成一致。

**守门（`audio_ooxml_compliance_test.go`）**：

- `TestBuildAudioPicFragmentIsSchemaCompliant`：断言 `p:nvPr` 存在、`a:audioFile r:link` 位于 `p:nvPr` 内且不在 `blipFill` 内。
- `TestSetAdvanceAfterKeepsSingleTransition`：在自带 `mc:AlternateContent` 的公开样本上断言 `p:transition` 恰好 2 个（Choice + Fallback）、`advTm` 写在 2 处、且**没有**被追加的直接子 transition。
- **守门有效性已验证**：临时停用 `mc:AlternateContent` 展开分支 → 测试必红并精确报出 `p:transition count = 3, want 2` 与 "a direct-child `<p:transition advTm=.../>` was appended"。

## 后果

- §15.3 第 3 条的**性质正式修正**：从"环境型缺口（缺语料）"改为"**真实代码缺陷**（已修复，待补播放记录证据）"。此前"不是代码缺陷"的判断源于推理而非实测。
- 该缺陷自 AUDIO-01/02 落地起就存在，历经多个版本未被发现，原因是**三层验证同时缺失**：
  1. 无 audio 语料 → 真机矩阵从未覆盖音频用例；
  2. 代码级测试只断言自身生成的 XML 形态，不校验 OOXML 合规性；
  3. **WPS 比 PowerPoint 宽容**，WPS 通过被当作"客户端兼容"证据。
- 教训（已写入项目记忆）：**凡改动 OOXML 结构，必须用 PowerPoint 真机验证**；WPS 通过不能替代。

## 后续

1. 样本正式入 corpus（`audio` 标签 + 可再分发合成音）需与 opencode 协调。
2. §15.3 第 3 条的"**播放记录**"仍需人工录屏或 Windows 音频会话枚举作为最终证据——本次已证明产物可被两家客户端正确打开并识别为媒体形状，但"音频确实播放出声"尚未留下证据。
3. `AudioShape.Profile()` 走 `lastProfileByMedia`（手写内联解析）漏读 `durMs`/`stMs`/`trigger`，与 `PlanTimingSync` 所用的 `parseAudioProfile` 两条路径不同步——**独立缺陷，另行处置**。
