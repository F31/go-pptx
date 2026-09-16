# ADR-028: 媒体兼容性契约工程化

- 状态：**已实施**（2026-09-16）
- 关联：[ADR-025](ADR-025-audio-ooxml-compliance-fix.md)（audio schema）、[ADR-026](ADR-026-video-ooxml-compliance-fix.md)（video）、[ADR-027](ADR-027-audio-visible-geometry-fix.md)（可见性/播放/几何）

## 上下文

ADR-027 为修复单个音频形状的可用性，连续暴露 **6 处彼此独立**的缺陷：

| # | 缺陷 | 症状 |
|---|---|---|
| 1 | 形状几何硬编码 `0×0` | 图标不可见 |
| 2 | `a:blip` 指向音频文件（无 poster） | 无图标 |
| 3 | `vol="80"` 量纲错（应 `80000`） | 静音（0.08%） |
| 4 | 缺 `p14:media` 关联 | **PowerPoint 打不开** |
| 5 | 极简计时结构 | WPS 不自动播放 |
| 6 | 默认定位不可见 | 行为改进 |

根因不是"某一个 bug"，而是**缺少媒体形状的书面契约与可执行守门**。既有测试只断言"存在某元素"（如 `cMediaNode` 存在），因此：

- 抓不到**量纲错**（`vol="80"` 与 `vol="80000"` 都"存在"）；
- 抓不到**指向错**（`a:blip r:embed` 指向音频还是图片，都"存在 blip"）；
- 抓不到**缺关联**（`p14:media` 缺失时结构仍合法）；
- 抓不到**触发错**（`delay="0"` 与 `playFrom` 命令都"是计时"）。

同时暴露两个一般化教训：

1. **媒体"四态"是四套独立约束**：能打开 / 图标可见 / 可点击 / 有声。缺任一不影响其余，"文件能打开"（ADR-025 最小实验只验证了不判损）**不等于**"可用"。
2. **"最小改动"结论有适用边界**：ADR-026 的"不需要 `p14:media`"只对 `blip→媒体文件` 成立；换成 `blip→图片` 就必须补（ADR-027 续三）。
3. **同类缺陷会被复制**：ADR-025( audio)/ADR-026(video) 是同一错误写法在两处；`AudioSpec` 漏 `X/Y/Width/Height` 是"未逐字段对齐 `VideoSpec`"。

## 决策

把上述教训固化为**契约 + 可执行守门 + 真机矩阵**三件套：

1. **书面契约** [`docs/media-compat-checklist.md`](media-compat-checklist.md)：
   - 媒体四态定义与真实反例；
   - 逐类型必需元素表（通用 / audio / video / picture）；
   - **关系类型匹配矩阵**（属性 → 关系类型）；
   - 新增媒体形状的逐项清单（含"逐字段对齐既有 spec"与"原生实包取证"）。

2. **可执行守门** `media_compat_test.go`（进 CI）：
   - `TestMediaRelationshipTargetsMatchAttribute`：断言 `a:blip→image`、`a:audioFile→audio`、`a:videoFile→video`、`p14:media→2007/media`，并做**悬空引用**检查；显式反例断言（`a:blip` 不得指向音频、不得出现 `p:videoFile`）。
   - `TestAudioTimingStructureByTrigger`：auto（`mainSeq`/`withEffect`）与 click（`interactiveSeq`/`clickEffect`）两条触发路径的形状，含 `playFrom(0.0)`、`vol="80000"`、`onStopAudio`。
   - `TestMediaDeckRoundTrip`：生成含 audio(auto)+audio(click)+video 的文稿 → 保存 → 重开 → `Validate` 0 error → 形状分类正确。

3. **守门有效性（变异验证）**：三条守门均经注入回归验证——`blip→audio`、`vol=80`、删 `p14:media` 时必须**变红**（防止"测试恒绿"）。命令记入契约文档 §6。

4. **真机矩阵** `scripts/l3/media_matrix.sh`：生成 `{audio-auto, audio-click, video}` fixture（`scripts/gen_media`）→ `{PowerPoint, WPS}` COM 打开/重存 → 打印人工核对清单（播放效果无法自动化）。覆盖在语料 `s004-audio` 之外的 **click 触发路径**。

5. **规则化**：新增任何媒体形状类型，必须（a）`<Type>Spec` 逐字段对齐既有 spec；（b）补 §2/§3 契约与守门；（c）用客户端原生产物取证；（d）进 L3 矩阵。

## 验证

- 三条新守门全绿；变异注入后分别变红（已实测）。
- 全量 `go test ./...` + `-tags=corpus`（37 样本）全绿。
- `scripts/gen_media` 产出 3 个 fixture，`audio-click` 结构断言通过（`interactiveSeq`/`clickEffect`/`onClick`/`playFrom`/`vol=80000`/`p14:media`/`blip→image`）。

## 后果

- 媒体缺陷从"靠真机逐轮试错发现"变为"契约层拦截 + 真机只做最终背书"。
- 新增媒体类型的成本前置：契约文档 + 守门 + 矩阵，避免再出现 ADR-027 式的 6 连缺陷。
- 真机部分仍不可完全自动化（播放效果需人耳/人工），但**scope 被压缩到最小**（矩阵只做最终确认，结构正确性由 CI 保证）。
- 未覆盖：`VideoSpec.PosterSource` 的 poster `p:pic` 独立契约、图表/表格等非 `p:pic` 媒体容器（后续按需扩展 §2）。
