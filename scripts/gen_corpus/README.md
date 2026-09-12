# scripts/gen_corpus

QA-01 语料/金样辅助工具。主入口：

```bash
scripts/gen_corpus/run.sh <command> [args]
```

## 1. 扫描现有本地 PPTX

适合把私有目录登记为外部真实语料索引。默认只写 `manifest.json`，不复制 PPTX，避免把许可未确认的文件误提交。

```bash
scripts/gen_corpus/run.sh scan "/mnt/e/work/2026/服务器" \
  --out testdata/corpus \
  --license "private/internal; redistribution not approved"
```

如已确认可入库，可加 `--copy --redistributable`：

```bash
scripts/gen_corpus/run.sh scan "/mnt/e/work/2026/服务器" --copy --redistributable
```

扫描会为每个 PPTX 生成：

```text
testdata/corpus/ext-0001/manifest.json
```

manifest 中会记录来源路径、SHA-256、大小、`docProps/app.xml` 的生成器信息，并按结构探测打标签，例如：

- `generator.wps`
- `generator.powerpoint`
- `generator.pptxgenjs`
- `table`
- `chart`
- `media.image`
- `animation.timing`
- `animation.transition`
- `xml.unknown_ext`

## 2. 用 LibreOffice headless 生成样本

需要本机安装 `soffice` 或 `libreoffice`：

```bash
scripts/gen_corpus/run.sh generate --out testdata/corpus
```

没有 LibreOffice 时可只生成 ODP 源和 manifest：

```bash
scripts/gen_corpus/run.sh generate --no-convert --out testdata/corpus
```

样本定义放在 `scripts/gen_corpus/samples/*.json`。工具会先生成最小 ODP，再通过 LibreOffice 导出 PPTX，因此导出的 PPTX 属于外部客户端产物，而不是 go-pptx 自己生成的文件。

## 3. 校验 manifest/actions 约定

```bash
scripts/gen_corpus/run.sh validate testdata/corpus
```

修改类金样仍按 `testdata/corpus/README.md`：

```text
<sample-id>.pptx
<sample-id>.edited.pptx
<sample-id>.actions.json
```

`validate` 会检查：

- `manifest.json` 必填字段是否齐全
- `sample_id` 是否与目录名一致
- `<sample-id>.edited.pptx` 与 `<sample-id>.actions.json` 是否成对出现
- `actions.json` 是否包含支持的动作字段

当前支持的动作：

- `ReplaceText`：必填 `old`, `new`
- `SetPlainText`：必填 `text`
- `SetNotes`：必填 `text`
- `Bind`：必填 `data`

## 4. 对你现有服务器目录的建议

`E:\work\2026\服务器` 在 WSL 下通常对应：

```bash
/mnt/e/work/2026/服务器
```

这批文件适合作为真实语料，尤其覆盖了 WPS、PowerPoint、PptxGenJS、表格、图表、图片、未知扩展。扫描后优先挑：

- `拓扑方案.pptx`：含 `p:timing`、`p:transition`、`extLst`，是 M0 垂直验证候选。
- `GPU_AI规格对比_V7.pptx`：WPS、多页、表格密集。
- `英伟达GPU显卡技术解析*.pptx`：大页数、多媒体/图表，适合性能和关系闭包回归。

但这些文件若含业务内容，默认按 private/internal 登记，不建议直接提交 PPTX 原文件。
