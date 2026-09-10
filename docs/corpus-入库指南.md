# 公开样本数据入库指南（CORPUS-01 / QA-01）

> 适用对象：想把新样本加入 `testdata/corpus/` 的协作者（dev / QA / opencode 工具）。
> 目标：让语料在 CI 上**可重放、可校验、可追溯**——任何一份公开样本入库后，CI
> 会自动把它接入 replay 与垂直验证测试。

---

## 1. 三条入库路径（按数据来源选择）

| 路径 | 命令 | 适用场景 | 是否提交 PPTX |
|---|---|---|---|
| 路径 A：LibreOffice 生成（公开） | `scripts/gen_corpus/run.sh generate` | **首选**。完全自动生成 ODP 源 + LibreOffice headless 导出 PPTX，PPT X 属外部客户端产物，可再分发。 | 是 |
| 路径 B：扫描本地 PPTX（私有索引） | `scripts/gen_corpus/run.sh scan <dir>` | 把本机已存在的 PPTX 登记为 manifest，**不复制**原文件。 | 否（只 manifest） |
| 路径 C：扫描 + 复制 + 标记可再分发 | `scripts/gen_corpus/run.sh scan <dir> --copy --redistributable` | 经授权可再分发的本地 PPTX；写入样本目录 + manifest 标注 `redistributable: true`。 | 是 |

### 路径 A 的最小操作（首推公开样本入库）

```bash
# 1. 准备样本定义（samples/<id>.json）
cat > scripts/gen_corpus/samples/s004-chart.json <<EOF
{
  "sample_id": "s004-chart",
  "title": "图表样本",
  "slides": [{"title": "...", "layout": "table", "table": [[...]]}],
  "feature_tags": ["chart", "generator.libreoffice"],
  "redistributable": true
}
EOF

# 2. 生成 ODP + LibreOffice 转 PPTX + 写 manifest
scripts/gen_corpus/run.sh generate --only s004-chart

# 3. 准备金样 actions.json + 跑一轮生成 edited.pptx
scripts/gen_corpus/run.sh generate --only s004-chart
#   ↓ 手动或脚本：
#     - Open(s004-chart.pptx) → ReplaceText(...) → Save(...edited.pptx)
#     - 记录 expected {matches, replaced, validate_error_count}
#     - 写到 s004-chart.actions.json + s004-chart.compat-smoke.json

# 4. 校验
scripts/gen_corpus/run.sh validate testdata/corpus
```

### 路径 B 的最小操作（私有样本登记）

```bash
# 仅写 manifest.json（按你给的 license 标记）
scripts/gen_corpus/run.sh scan /path/to/private/pptx \
    --license "private/internal; redistribution not approved" \
    --font-environment "Windows-11 + Microsoft YaHei"

# CI / corpus_replay job 会自动 SKIP 本地无源的样本；本机挂载源文件后自动启用。
```

---

## 2. 校验契约（corpus.py validate 自动执行）

每次 PR 触发 `corpus-replay` job 时，CI 先跑 `validate testdata/corpus`，必查项：

1. `manifest.json` 必填字段齐全：`sample_id` / `source_license` / `generator` / `feature_tags` / `expectations` / `files`。
2. 目录名与 `sample_id` 一致（`ext-0001/manifest.json` 的 `sample_id == "ext-0001"`）。
3. `<id>.actions.json` 与 `<id>.edited.pptx` 成对出现；任一缺失即报错。
4. `actions.json` 中每个 action 必须声明支持的类型与必填字段：
   - `ReplaceText` → `old`, `new`
   - `SetPlainText` → `text`
   - `SetNotes` → `text`
   - `Bind` → `data`
5. `compat-smoke.json` 记录 `gold_action.result.{matches, replaced, shapes_seen, shapes_tried}` 与 `validate.{before,after}_error_count`，replay 测试用其作断言基准。

本地手动校验：

```bash
scripts/gen_corpus/run.sh validate testdata/corpus
# 期望输出: validated 36 sample(s), errors=0
```

---

## 3. CI 接入（公开样本的"价值兑现"）

CI 的 `corpus-replay` job（`.github/workflows/ci.yml`）按以下顺序跑：

1. **Validate**：`scripts/gen_corpus/run.sh validate testdata/corpus`——manifest 损坏在此 fail。
2. **Build**：`-tags=corpus ./...`——确保 `corpus_replay_test.go` / `vertical_corpus_test.go` 编译通过。
3. **Vet**：`-tags=corpus ./...`——静态检查覆盖 build-tag 代码路径。
4. **Test**：`-tags=corpus -v ./...`——公开样本全部跑通；私有样本在 CI 无源则自动 SKIP。
5. **Artifact**：`corpus-replay.log` 上传 14 天，公开样本的 PASS/SKIP 计数可追。

公开样本（`s00*`）必须在 CI 上 PASS，否则 PR 不允许合并。
私有样本（`ext-*`）SKIP 视为正常——它们的 source 在协作者本机，不在 CI 沙箱。

---

## 4. 本地 helper（不走 CI 也能跑）

```bash
# 三段式：validate → test → 统计 PASS/SKIP/FAIL
scripts/run_corpus_tests.sh

# 自定义语料根目录（CI 多语料库场景）
scripts/run_corpus_tests.sh /path/to/another/corpus
```

输出示例：

```text
==> 1/3 validate manifests in /e/projects/go-pptx/testdata/corpus
validated 36 sample(s), errors=0
==> 2/3 go test -tags=corpus -v ./... (log: /e/projects/go-pptx/corpus-replay.log)
... (测试日志)
==> 3/3 summary
    PASS: 125
    SKIP: 1
    FAIL: 0
OK: corpus-replay all green (125 pass, 1 skip)
```

退出码：`0` 校验通过且无 FAIL；`1` manifest 损坏或测试 FAIL；`2` 环境异常（缺 go / 缺 gen_corpus）。

---

## 5. 与 build tag 的关系

`corpus_replay_test.go` 与 `vertical_corpus_test.go` 都声明 `//go:build corpus`——

- `go test ./...`（默认）**不**编译也不跑这两个文件，保证 dev 内循环速度。
- `go test -tags=corpus ./...` 启用 build tag，CI 守门使用。

任何新增的公开样本相关测试文件，必须在文件顶部加：

```go
//go:build corpus

// file-level comment 说明本文件角色与样本依赖
```

否则会污染默认测试套件。

---

## 6. 跨平台路径处理（私有样本的 WSL / Git Bash / Windows 互通）

`corpus_replay_test.go::corpusAbsPathCandidates` 把绝对路径转成三种形态候选，
保证同一份 manifest 在三种环境下都能找到源文件：

| manifest.path 形态 | 派生候选 |
|---|---|
| `/mnt/e/work/.../foo.pptx`（WSL） | `/e/work/.../foo.pptx`（Git Bash）+ `E:\work\.../foo.pptx`（Win32） |
| `/e/work/.../foo.pptx`（Git Bash） | `/mnt/e/work/.../foo.pptx`（WSL）+ `E:\work\.../foo.pptx`（Win32） |
| `E:\work\.../foo.pptx`（Win32） | （未派生，避免 Linux 误识别盘符） |

新增样本时：manifest.path 优先用 WSL `/mnt/<drive>/...` 形态，可同时覆盖 Git Bash
与 Windows（通过候选转换）。Linux / macOS dev 上 path 直传即可。

---

## 7. 已知约束与不在本指南范围

- **客户端矩阵**：PowerPoint/WPS 真机打开验证是发布级硬缺口（参见
  `docs/v1.0-freeze-list.md` §F）；CI 上仅完成 go-pptx validate/diff 冒烟。
- **大文件策略**：私有样本源 PPTX 不入库（路径驱动）；公开样本建议 ≤ 1 MB，
  超过时考虑用 LibreOffice 重新生成更紧凑版本（关闭嵌入字体 / 压缩图像）。
- **许可登记**：扫描路径 B/C 时，`--license` 字符串会原样写入 manifest 并随
  PR 公开，**不要**写"MIT"或"public domain"等未经授权的许可。