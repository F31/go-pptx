# L3 客户端兼容矩阵

日期：2026-09-11（**真机首轮执行完成**）/ 2026-09-16（**第二轮：ADR-018 Tier 2 产物验证**）；自动化脚本见 `scripts/l3/`

本文记录 PowerPoint / WPS 真机兼容验证。验证方式：Windows 宿主机（WSL 侧触发）通过 COM 自动化打开样本、判定无修复提示并另存为 .pptx，再交由 go-pptx 重开回验。所有步骤可由 `scripts/l3/run_client.sh <ppt|wpp> <src> <dst>` 复现。

- **PowerPoint**：Microsoft Office 16.0.20326（ProgID `PowerPoint.Application`）
- **WPS 演示**：12.1.0.28599（ProgID `Kwpp.Application`）
- **平台**：Windows 11 家庭中文版（NT 10.0.26200）

判定语义：COM `Presentations.Open` 在 50s 超时窗口内成功返回 ⇒ 无修复提示（需修复时客户端弹对话框会阻塞 Open，被超时判定为 `TIMEOUT_REPAIR_PROMPT`）；随后 `SaveAs(24)` 成功 ⇒ 重存 .pptx 成功。

## 判定标准

每个样本 × 客户端组合至少记录以下检查项：

| 检查项 | 通过标准 |
|---|---|
| 打开 | 客户端打开原始/编辑后 PPTX 无“修复/恢复/内容损坏”提示 |
| 保存 | 客户端重存为 PPTX 成功，无报错 |
| 重开 | go-pptx 可重新打开客户端重存文件，`Validate` 无新增错误 |
| 内容 | 预期文本/表格/图片/图表/媒体变更仍存在 |
| 保真 | 非目标区域无明显视觉损坏；含未知扩展/动画样本不丢关键结构 |
| 播放 | 音频/视频/计时/翻页相关样本按预期播放或明确记录偏差 |

## 证据约定

建议把不能公开的截图/录屏存放在受限证据库；本仓库只记录索引和摘要。

| 字段 | 说明 |
|---|---|
| client | 客户端名称，例如 PowerPoint / WPS |
| version | 精确版本号与构建号 |
| platform | Windows/macOS/Linux + 版本 |
| sample | `testdata/corpus` 样本 ID |
| source | 原始、go-pptx 编辑后、客户端重存后 |
| result | pass / fail / skip |
| repair_prompt | 是否出现修复提示 |
| resave_validate | 客户端重存后 go-pptx validate 结果 |
| evidence | 截图/录屏/日志的受限路径或 issue/PR 链接 |
| notes | 已知偏差、字体替换、客户端自动改写等 |

## 首批必跑样本

| 样本 | 原因 | 当前状态 |
|---|---|---|
| `s001-text` | 公开文本替换金样，多段文本结构 | ✅ 2026-09-11 两端真机通过 |
| `s002-table` | 公开表格文本替换金样 | ✅ 2026-09-11 两端真机通过 |
| `s003-image` | 公开图片页文本替换金样，含 opaque region | ✅ 2026-09-11 两端真机通过 |
| `ext-0024` | 私有 WPS 真实样本，含 timing/transition/unknown_ext | ✅ 2026-09-11 两端真机通过（原文件与重存文件不入库，证据记 hash） |

## 结果矩阵

| Client | Version | Platform | Sample | Source | Open | Repair Prompt | Resave | go-pptx Validate After Resave | Evidence | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| PowerPoint | 16.0.20326 | Win11 26200 | `s001-text` | edited | pass | 无 | pass | errorCount=0，Q3-2026 保留 | `.l3-output/s001-text.ppt-resaved.pptx` (b58faf81…) | COM 自动化 |
| PowerPoint | 16.0.20326 | Win11 26200 | `s002-table` | edited | pass | 无 | pass | errorCount=0，AI 表格文本保留 | `.l3-output/s002-table.ppt-resaved.pptx` (c72621d1…) | COM 自动化 |
| PowerPoint | 16.0.20326 | Win11 26200 | `s003-image` | edited | pass | 无 | pass | errorCount=0，Generated PNG 保留 | `.l3-output/s003-image.ppt-resaved.pptx` (fd9813f6…) | COM 自动化 |
| PowerPoint | 16.0.20326 | Win11 26200 | `ext-0024` | go-pptx edited | pass | 无 | pass | errorCount=0，89145 保留 | `.l3-output/ext-0024.ppt-resaved.pptx` (65ee0c10…) | 私有样本，不入库 |
| WPS | 12.1.0.28599 | Win11 26200 | `s001-text` | edited | pass | 无 | pass | errorCount=0，Q3-2026 保留 | `.l3-output/s001-text.wps-resaved.pptx` (d4b43feb…) | COM 自动化 |
| WPS | 12.1.0.28599 | Win11 26200 | `s002-table` | edited | pass | 无 | pass | errorCount=0，AI 表格文本保留 | `.l3-output/s002-table.wps-resaved.pptx` (3c991836…) | COM 自动化 |
| WPS | 12.1.0.28599 | Win11 26200 | `s003-image` | edited | pass | 无 | pass | errorCount=0，Generated PNG 保留 | `.l3-output/s003-image.wps-resaved.pptx` (2adeec79…) | COM 自动化 |
| WPS | 12.1.0.28599 | Win11 26200 | `ext-0024` | go-pptx edited | pass | 无 | pass | errorCount=0，89145 保留 | `.l3-output/ext-0024.wps-resaved.pptx` (a97cf728…) | 私有样本，不入库 |

## 第二轮：ADR-018 Tier 2 产物验证（2026-09-16）

**触发**：ADR-018 Tier 2（未变非 XML Part 走 `CreateRaw`/`OpenRaw` 原始帧直通）改变了 Save 的输出字节布局——媒体 Part 不再被重压缩，源压缩帧与 Extra 字段原样保留。ADR-018 验收清单第 5 项据此要求**重跑 8 组合**（Tier 1 曾因输出字节不变而豁免）。

**输入**：用本轮 HEAD（含 Tier 2）经 SDK `ReplaceText` + `Save` 重新生成编辑后产物到 `.l3-output/t2/*.t2.pptx`——**不是**入库的 `*.edited.pptx`（后者由 Tier 2 之前的代码生成）。生成器 `scripts/gen_corpus/gold_replace.go`。

| Sample | 编辑 | Tier 2 产物 sha8 | 尺寸 |
|---|---|---|---:|
| `s001-text` | `REPORT_BODY` → `Q3-2026` | 9771BB47 | 8748 |
| `s002-table` | `GPU` → `AI` | 7C998323 | 9026 |
| `s003-image` | `Synthetic PNG` → `Generated PNG` | 208210DA | 9014 |
| `ext-0024` | `89144` → `89145` | BFF349A9 | 5964704 |

**结果：8/8 全部通过**（`OPEN=ok` 无修复提示 + `SAVE=ok`）。

| Client | Sample | Open | Repair Prompt | Resave | go-pptx Validate After Resave | Evidence sha8 |
|---|---|---|---|---|---|---|
| PowerPoint 16.0.20326 | `s001-text` | pass | 无 | pass | errorCount=0 | 038C7BDC |
| PowerPoint 16.0.20326 | `s002-table` | pass | 无 | pass | errorCount=0 | 506665A0 |
| PowerPoint 16.0.20326 | `s003-image` | pass | 无 | pass | errorCount=0 | 3783C1F0 |
| PowerPoint 16.0.20326 | `ext-0024` | pass | 无 | pass | errorCount=0 | F041CB5A |
| WPS 12.1.0.28599 | `s001-text` | pass | 无 | pass | errorCount=0 | 03DEAC86 |
| WPS 12.1.0.28599 | `s002-table` | pass | 无 | pass | errorCount=0 | D51DB2EE |
| WPS 12.1.0.28599 | `s003-image` | pass | 无 | pass | errorCount=0 | 91C94AFF |
| WPS 12.1.0.28599 | `ext-0024` | pass | 无 | pass | errorCount=0 | 6F45FE60 |

> 执行方式：本机为原生 Windows（非 WSL），故用等价的一次性 runner 驱动 `scripts/l3/ppt_open_resave.ps1`（`Start-Job` + 55s 超时保护，判定语义与 `run_client.sh` 一致）。总耗时 13.3s。`ext-0024` 重存后 2 页（`SLIDES=2`），其余样本 1 页，均与源一致。

**意义**：这是 Tier 2 安全门限（非 XML / 仅 Store 或 Deflate / 排除加密位与 data-descriptor 位 / 尺寸已知）**在真机上的端到端背书**——raw 直通保留的原始压缩帧被 PowerPoint 与 WPS 正常接受，无修复提示。门限的全部意义就是不让"客户端判损"发生，本轮矩阵是其直接证据。

## 第三轮：配音播放证据链（2026-09-16，CreateVideo 对照实验）

**目的**：为 V2.6 §15.3 第 3 条"配音功能必须有播放记录"取证（该条此前因缺 audio 语料一直未闭合）。

**方法**：用 PowerPoint COM `Presentation.CreateVideo(file, UseTimingsAndNarrations = -1, DefaultSlideDuration = 2, 720p, 30fps, 85)` 把含配音的文档渲染为 `.mp4`，再用 `zz_mp4probe` 扫描 MP4 的 box 结构判定音频轨是否存在。

**结果**：

| 输入 | 音频来源 | mp4 时长 | 音频轨 | 形状 type |
|---|---|---|---|---|
| `s001-text.narrated.final.pptx` | go-pptx `AddAudio` | **2.508s** | **无**（`soun`/`mp4a`/`esds` 均 false） | 16 |
| `ref-ppt-inserted.pptx` | **PowerPoint 原生插入**（对照组） | **2.508s** | **无**（box 结构逐项一致） | 16 |

**结论**：

1. **`CreateVideo` 不把页内音频对象渲染进视频** —— 原生对照组同样无声，故**与本库无关**（`UseTimingsAndNarrations` 的 "Narrations" 仅指 PowerPoint「录制旁白」，不含「插入的音频对象」）。**该路线不能作为"配音可播放"的证据。**
2. 但该实验意外产出两条**可自动获得**的强证据：
   - **计时被采用**：两段 mp4 时长均为 **2.508s**，等于写入的 `advanceTime="2500"`，而非 `CreateVideo` 的默认时长参数（2s）→ PowerPoint 确实解析并执行了 go-pptx 写入的自动翻页计时（含 mc:AlternateContent 两个分支上的 `advTm`）；
   - **形状与原生等价**：go-pptx 产物与 PowerPoint 原生插入的音频形状，在 COM 视角下同为 `type=16 (msoMedia)`。
3. **"音频确实出声"仍需人工**：放映（F5）时页内音频对象会播放，但 COM 无法采集音频输出。最终证据需人工录屏（`.mp4`，`Win+Shift+S` 或 `Win+G`）或 Windows 音频会话枚举（本机 `Add-Type` 被安全策略禁，纯脚本不可行）。

**复现**：

```bash
go run .l3-output/zz_genaudio.go -input testdata/corpus/s001-text/s001-text.pptx -output .l3-output/x.pptx
# 用 PowerPoint COM CreateVideo 渲染为 mp4 后：
go run .l3-output/zz_mp4probe.go -f x.mp4     # AUDIO_TRACK_PRESENT=false 属预期
```

## 第四轮：人工录屏（2026-09-16，§15.3 第 3 条最终证据）

**方法**：在真机上放映 `s001-text.narrated.final.pptx` 并录屏（`.mp4`），再用 `zz_mp4probe` 扫描录屏文件的 MP4 box 结构。

**结果**：

| 项 | 值 |
|---|---|
| 文件 | `.l3-output/s001-text.narrated.mp4`（643923 B；覆盖了先前的 CreateVideo 产物） |
| 总时长 | 26.816s |
| 轨数 | **2** |
| 轨 1 | `hdlr handler_type=vide`（400 帧 / 26.606s） |
| 轨 2 | **`hdlr handler_type=soun`** + `smhd` + AAC（`mp4a` / `esds` 命中） |
| 音频样本 | 838 帧 / 423191 B / 平均 505 B / 最大 572 B |
| 等效码率 | **≈126 kbps** |
| `AUDIO_TRACK_PRESENT` | **true** |

**结论与边界**：

1. ✅ 录屏**确实包含音频轨**且带实际数据（非空轨）——录屏期间有音频被捕获。
2. ⚠️ **但不能仅凭容器层判定"非静音"**：该音轨是 **CBR AAC**（逐秒平均帧大小恒为 505±2 字节）。恒定码率下静音帧同样会被填充到固定大小，**帧大小/码率不能作为"有声"的判据**（这是本节分析的自我修正——最初的"逐秒分析"假设被数据推翻）。
3. 因此最终判定需要**播放确认**（人耳）与**音频源确认**（系统声音 vs 麦克风）。

**验收标准（供 QA / 后续复现）**：

| 步骤 | 期望 |
|---|---|
| 1. 用 PowerPoint 打开 `.l3-output/s001-text.narrated.final.pptx` | 无修复提示，页面出现 `Audio 11` 音频图标 |
| 2. F5 放映 | 无修复提示；`advanceTime=2500` 生效（2.5s 后自动翻页/结束） |
| 3. 放映期间 | 可听到 2.0s 的 440 Hz 提示音 |
| 4. 录屏（`Win+Shift+S` 或 `Win+G`，输出 `.mp4`） | 音频源选**系统声音**（而非麦克风），以获得"客户端确实输出了音频"的直接证据 |
| 5. `go run .l3-output/zz_mp4probe.go -f <录屏>.mp4` | `AUDIO_TRACK_PRESENT=true`（音轨存在） |
| 6. 人耳确认 | 录屏中能听到该提示音 |

> 步骤 4 的**音频源选择是关键**：录制"系统声音"时，音轨内容即客户端输出，是"配音确实播放"的直接证据；若录的是麦克风，则需扬声器外放才能捕获，证据强度降为间接。

## 执行步骤
1. 运行 `scripts/gen_corpus/run.sh validate testdata/corpus` 确认本地语料索引有效。
2. 对公开样本使用 `*.edited.pptx` 作为客户端打开输入。
3. 对私有样本按 `manifest.json.files.pptx.path` 定位原文件，在受限环境生成 go-pptx 编辑后文件。
4. 用目标客户端打开输入文件，记录是否出现修复提示。
5. 在客户端中执行另存或保存为新文件。
6. 用 go-pptx 打开客户端重存文件并运行 `Validate`。
7. 将结果填入矩阵，证据只填路径/链接，不提交受限文件。

自动化执行（Windows 宿主机 + WSL）：

```bash
# PowerPoint（公开样本直接使用 corpus 中 edited 文件）
bash scripts/l3/run_client.sh ppt \
  'E:\projects\go-pptx\testdata\corpus\s001-text\s001-text.edited.pptx' \
  'E:\projects\go-pptx\.l3-output\s001-text.ppt-resaved.pptx'

# WPS
bash scripts/l3/run_client.sh wpp \
  'E:\projects\go-pptx\testdata\corpus\s002-table\s002-table.edited.pptx' \
  'E:\projects\go-pptx\.l3-output\s002-table.wps-resaved.pptx'
```

## 当前结论

2026-09-11 首轮真机执行完成：**4 样本 × 2 客户端 = 8 组合全部通过**——PowerPoint 16.0.20326 与 WPS 演示 12.1.0.28599 均无修复提示打开、重存成功，重存文件经 go-pptx `Validate` 全部 errorCount=0 且编辑内容保留。L3 发布级缺口闭合（COM 自动化为无窗口代理；如需 GUI 截图证据，可后续在真机补录）。

**2026-09-16 第二轮（Tier 2 产物）同样 8/8 通过**：这是 Tier 2 改变输出字节后的兼容性背书，也是 v1.0.5 的发布门禁之一。两轮合计 16 组合、零修复提示。
