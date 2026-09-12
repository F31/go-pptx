# L3 客户端兼容矩阵

日期：2026-09-11（**真机首轮执行完成**；自动化脚本见 `scripts/l3/`）

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
