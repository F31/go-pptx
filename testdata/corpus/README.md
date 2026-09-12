# testdata/corpus 语料库说明（QA-01）

本目录存放 go-pptx 的兼容性测试语料索引与金样约定。语料文件本身可能因许可原因
不直接入库，索引记录必须完整，金样按本文件约定存储。

## 当前落地状态（2026-09-10）

| 项 | 当前状态 |
|---|---|
| 公开可再分发样本 | 3 份 LibreOffice headless 导出样本：`s001-text` / `s002-table` / `s003-image` |
| 公开金样闭环 | 3/3 已完成修改前 `.pptx`、修改后 `.edited.pptx`、`.actions.json`、`compat-smoke.json` |
| 私有真实样本索引 | 33 份，`ext-0001`–`ext-0033`，只登记 manifest，不提交原始业务 PPTX |
| 动画+未知扩展真实样本 | `ext-0024`（WPS，私有索引）已完成本地冒烟，含 `animation.timing` / `animation.transition` / `xml.unknown_ext` |
| 语料校验 | `scripts/gen_corpus/run.sh validate testdata/corpus`：36 samples，0 errors |
| 客户端矩阵 | PowerPoint/WPS 真机打开验证仍待补；当前仅完成 go-pptx validate/diff 冒烟；执行矩阵见 `docs/client-compat-matrix.md` |

结论：QA-01 的“语料为空”硬阻塞已解除；发布级 L3 兼容报告仍需按 `docs/client-compat-matrix.md` 补 PowerPoint/WPS 真机证据。

## 每个样本必须记录的字段（方案 §15.1）

| 字段 | 说明 |
|---|---|
| 样本 ID | 唯一编号，用于金样与回归关联 |
| 来源许可 | 来源、许可类型、可否再分发 |
| 生成器与版本 | PowerPoint/WPS/LibreOffice/Google Slides/Keynote + 具体版本/平台 |
| 创建步骤 | 如何生成（含自动化脚本或手工步骤） |
| 字体环境 | 生成与验证所用字体环境 |
| OOXML 类型 | Transitional / Strict 等 |
| 特性标签 | 命中 2.3 能力矩阵的哪些特性（同一文件可多标签） |
| 预期断言 | 读取/编辑/保存后的预期结果 |
| 已知问题 | 客户端修复行为、已知偏差等 |

## 金样约定

- 每个原生样本保留「修改前 / 修改后」两份金样。
- 修改类金样命名：`<sample-id>.<ext>`（修改前）与 `<sample-id>.edited.<ext>`（修改后），
  修改动作记录在 `<sample-id>.actions.json`。
- 金样只增不改；发现既有金样错误时新增修订版本，不覆盖旧证据。

## 开工目标清单（第 0–2 周，≥20 份，来源许可优先落实）

| # | 生成器 | 特征标签 | 用途 |
|---|---|---|---|
| 1–6 | PowerPoint（记录具体版本/平台） | 纯文本 / 图文 / 备注 / 表格 / 图表 / 动画+未知扩展 | M0 垂直验证、M1 B1 |
| 7–10 | WPS | 文本 / 表格 / 私有节点 / 音频 | 客户端矩阵、私有扩展保留 |
| 11–14 | LibreOffice | 文本 / 图片 / 主题 / 版式变体 | 生成器差异回归 |
| 15–18 | Google Slides 导出 | 文本 / 形状 / 关系布局差异 | 关系解析健壮性 |
| 19–22 | Keynote | 文本 / 媒体 / 导出差异 | 后续矩阵扩充 |
| 23–24 | 手工构造最小样本 | 重复条目 / 越界路径 / 超深 XML / 非 UTF-8 | L0 安全用例 |

> 私有企业样本经脱敏并获授权后进受限回归库，不随开源分发（方案 §26）。
> 公开来源不自动意味着可自由再分发，逐项核实。
