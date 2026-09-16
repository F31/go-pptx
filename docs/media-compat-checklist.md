# 媒体形状兼容性契约（Media Compatibility Checklist）

> 建立背景：音频形状一个特性连续暴露 **6 处彼此独立**的缺陷（ADR-027 系列），
> 症状各不相同。本文把教训固化为**可执行契约**——供新增任何媒体形状时逐项对照。
>
> 关联：[ADR-025](adr/ADR-025-audio-ooxml-compliance-fix.md)（audio schema）、
> [ADR-026](adr/ADR-026-video-ooxml-compliance-fix.md)（video）、
> [ADR-027](adr/ADR-027-audio-visible-geometry-fix.md)（可见性/播放）、
> [ADR-028](adr/ADR-028-media-compatibility-contract.md)（工程化）、
> 守门测试 `media_compat_test.go` / `audio_ooxml_compliance_test.go` / `video_ooxml_compliance_test.go`。

---

## 1. 核心教训：媒体"四态"是四套独立约束

一个媒体形状在客户端有 **4 个独立的可用性维度**。缺任一都不影响其余，因此
"能打开"**不代表**"图标可见/可点击/有声"；代码级测试只断言"存在某元素"会
漏掉整类缺陷。

| 维度 | 缺失/错误的典型症状 | 反例（本项目真实缺陷） |
|---|---|---|
| **能打开** | 客户端报"文件损坏"（PowerPoint `0x80070570`） | 缺 `p:nvPr`（ADR-025）；缺 `p14:media`（ADR-027 续三） |
| **图标可见** | 形状存在但空白/不可见 | 几何 `a:ext 0×0`（ADR-027）；`a:blip` 指向音频而非图片（ADR-027 续） |
| **可点击** | 图标可见但点击无反应 | 缺 `ppaction://media` 超链接 |
| **有声/播放** | 打开正常、图标正常，但放映无声 | `vol="80"`（=0.08%，ADR-027 续二）；缺 `p:cmd playFrom`（WPS，ADR-027 续四） |

> **WPS 比 PowerPoint 宽容**：同一致命缺陷往往只在 PowerPoint 暴露（ADR-025/026/027 续三），
> 或只在 WPS 暴露（ADR-027 续四）。**两家都必须测**。

---

## 2. 逐类型必需元素契约

### 2.1 通用（所有 `p:pic` 媒体形状）

| # | 要求 | 依据 |
|---|---|---|
| C1 | `p:nvPicPr` 含 `p:cNvPr` + `p:cNvPicPr` + `p:nvPr`（三者均 `minOccurs=1`） | CT_PictureNonVisual |
| C2 | `p:spPr/a:xfrm` 的 `a:ext` 非零（可见） | ADR-027 |
| C3 | `p:blipFill/a:blip@r:embed` 指向的关系**类型正确**（见 §3） | ADR-027 续 |
| C4 | XML 中每个非空 `r:embed`/`r:link`/`r:id` 都能解析到关系（无悬空引用） | 本契约 |

### 2.2 音频（`a:audioFile`）

| # | 要求 | 依据 |
|---|---|---|
| A1 | `p:nvPr` 内 `<a:audioFile r:link="rIdX"/>`（DrawingML 命名空间 + 必需 `r:link`） | ADR-025 |
| A2 | `p:nvPr` 内 `<p:extLst><p:ext uri="{DAA4B4D4-…}"><p14:media r:embed="rIdY"/></p:ext></p:extLst>`，`rIdY` 是 **2007 media 关系**且**≠** `rIdX` | ADR-027 续三 |
| A3 | `p:cNvPr` 内 `<a:hlinkClick r:id="" action="ppaction://media"/>` | ADR-027 续 |
| A4 | `p:cNvPicPr` 含 `<a:picLocks noChangeAspect="1"/>` | ADR-027 续 |
| A5 | `a:blip@r:embed` 指向 **图片（poster 图标）**，绝不可指向音频关系 | ADR-027 续 |
| A6 | `p:timing` 含原生媒体播放结构：`p:seq`（`mainSeq`/`interactiveSeq`）→ `presetClass="mediacall"` 效果 → `<p:cmd type="call" cmd="playFrom(0.0)">`，外加 `p:audio/p:cMediaNode`（`delay="indefinite"` + `onStopAudio`） | ADR-027 续四（WPS） |
| A7 | `p:cMediaNode@vol` 是 **`ST_PositiveFixedPercentage`（0..100000）**，80% 写作 `vol="80000"` | ADR-027 续二 |

### 2.3 视频（`a:videoFile`）

| # | 要求 | 依据 |
|---|---|---|
| V1 | `p:nvPr` 内 `<a:videoFile r:link="rIdX"/>`（**DrawingML** 命名空间，**不是** `p:videoFile`） | ADR-026 |
| V2 | 读侧需**追加**旧 `p:videoFile` 探测以兼容 v1.0.5 及更早产物 | ADR-026 |
| V3 | 当前设计 `a:blip@r:embed` 指向视频关系（PowerPoint 可渲染视频封面）；提供 `PosterSource` 时另叠一张 poster `p:pic` | VIDEO-01 |

### 2.4 普通图片（`p:blipFill`）

| # | 要求 | 依据 |
|---|---|---|
| P1 | `p:nvPr` 存在（可为空） | IMAGE-01 已合规 |

---

## 3. 关系类型匹配矩阵（守门核心）

`media_compat_test.go` 逐条断言"属性 → 关系类型"匹配：

| XML 属性 | 必须指向的关系类型 |
|---|---|
| `a:blip@r:embed`（媒体 pic） | `…/relationships/image` |
| `a:audioFile@r:link` | `…/relationships/audio` |
| `a:videoFile@r:link` | `…/relationships/video` |
| `p14:media@r:embed` | `http://schemas.microsoft.com/office/2007/relationships/media` |

> 这条矩阵直接守"`a:blip` 指向音频文件"（ADR-027 续的根因）——这是最隐蔽的一类：
> XML 合法、文件能打开（PowerPoint 旧版）或不可见，代码级断言"blip 存在"完全抓不到。

---

## 4. 验证分层（都进 CI）

| 层 | 载体 | 覆盖 |
|---|---|---|
| 片段结构 | `audio_ooxml_compliance_test.go` / `video_ooxml_compliance_test.go` | 元素存在性、位置、命名空间、几何、vol |
| 关系图一致性 | `media_compat_test.go::TestMediaRelationshipTargetsMatchAttribute` | 类型匹配 + 悬空引用 |
| 触发路径 | `media_compat_test.go::TestAudioTimingStructureByTrigger` | auto（`mainSeq`/`withEffect`）vs click（`interactiveSeq`/`clickEffect`） |
| 端到端往返 | `media_compat_test.go::TestMediaDeckRoundTrip` | 生成→保存→重开→Validate→分类 |
| 真机矩阵 | `scripts/l3/media_matrix.sh`（Windows 宿主） | {audio,video} × {auto,click} × {PowerPoint,WPS} × {打开,可见,播放} |

**守门有效性**：契约测试均经**变异验证**——注入回归（blip→audio、vol=80、删 p14:media）
必须变红，防止"测试恒绿"。

---

## 5. 新增媒体形状的清单

1. 定义 `<Type>Spec` 时，**逐字段对照**既有 spec（`TextBoxSpec`/`PictureSpec`/`VideoSpec`/`AudioSpec`）——
   `AudioSpec` 曾漏掉 `X/Y/Width/Height`，代价是"形状永久 0×0 不可见"。
2. 构建 `p:pic`/`p:graphicFrame` 片段时，对照 §2 的通用 + 类型专属契约。
3. 在 `media_compat_test.go` 补该类型的：片段结构、关系类型匹配、端到端往返。
4. **用客户端原生插入的实包取证**（PowerPoint `AddMediaObject2` 等），
   不要凭记忆或仅凭"文件能打开"。
5. 在 `scripts/l3/media_matrix.sh` 加该类型 × 两客户端 × 两触发。
6. **不要**把某条"最小改动"结论当作普适规律——ADR-026 的"不需要 `p14:media`"
   只对 `blip→媒体文件` 成立，换成 `blip→图片` 就必须补（ADR-027 续三）。

---

## 6. 复现命令

```bash
# 契约层（CI 常态）
go test -run 'TestMedia|TestAudio|TestBuildAudioPic|TestBuildVideoPic|TestSetPlayback' -v .

# 端到端
go test ./... && go test -tags=corpus ./...

# 变异验证（手动，确认守门非恒绿）：注入回归后应 FAIL
#   1) audio.go: blip 改指向 audioRid       → TestMediaRelationshipTargetsMatchAttribute 必红
#   2) audiotiming.go: vol=80000 → vol=80   → TestAudioTimingStructureByTrigger 必红
#   3) audio.go: 删 p14:media               → TestMediaRelationshipTargetsMatchAttribute 必红

# 真机矩阵（Windows 宿主）
bash scripts/l3/media_matrix.sh
```
