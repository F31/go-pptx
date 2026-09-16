# go-pptx 项目长期约定（MEMORY.md）

## 项目定位
纯 Go（CGO_ENABLED=0）PPTX 创建/编辑组件。设计基线 V2.6，M0–M8 阶段门禁制推进。

## 发布状态
- 已发布 tag：v1.0.0–v1.0.5（SSH 签名 annotated tag，push origin）。最新 v1.0.5（2026-09-16）= v1.0.1 以来首个含生产代码改动的 patch。
- binary-compat 表面（v1.0.5 起）：40 Stable 段 / 0 Experimental / 60 Stable 符号 / 163 type / 131 Stable 方法，全只增不改。守门于 `api_surface_test.go` 7 个 AST 测试（golden 名单 + parser.ParseDir）；改动公共 API 表面须同步 golden 清单；根包非测试文件禁 `//go:build`。
- 覆盖率（2026-09-16 复测）：root 合并口径 84.4%（COV-02 门槛 82%，贴线）；per-package：opc 90.3% / xmlstore 90.8% / videoprobe 92.6% / summarize 91.0% / chart 91.2% / audioprobe 88.4% / editplan·textmap 100% / ir 86.1% / render 84.2%。
- **V2.6 §15.3 五条发布硬门槛：全部闭合（2026-09-16）**。第 3 条（配音播放）原判"环境型缺口、非代码缺陷"被实测推翻（PowerPoint 拒收 → ADR-025）；video 同源缺陷 ADR-026 已修并真机验证可播放。

## 工程约定（已落地，稳定）
- module：github.com/F31/go-pptx（go 1.24.0）；Apache-2.0；origin SSH git@github.com:F31/go-pptx.git；git 本地身份 go-pptx-dev <go-pptx-dev@local>（仓库级）。
- CI 必检：CGO_ENABLED=0 构建 + vet + test + GOOS=js GOARCH=wasm 编译；race 在 ubuntu job。
- 错误分层：internal 自定义内部错误（不反向 import 根包），公共边界映射根包稳定错误码；用 error 不 panic；文档/commit 中文；md 在 docs/。
- 目录：根包 pptx；internal/{opc,xmlstore,edit,style,textmap,geom,validate,document,editplan,chart}；render/、cmd/pptx/、testdata/corpus/。
- 新增导出符号升 Stable 须 freeze list §D checklist；降档须新 ADR 禁止 PR 直降。
- 句柄身份（STALE-GUARD）= 形状 cNvPr@id；hint=0 走纯路径；仅 RemoveShape 使句柄失效。三层：shapeNode/textNode/Cell。
- opencode 协作分界：testdata/corpus/ 与 scripts/gen_corpus/ 由 opencode 维护，主代理不纳入。
- 状态跟踪：docs/go-pptx-实施状态跟踪.md 每里程碑更新；日志按日追加于 .workbuddy/memory/YYYY-MM-DD.md。

## 可复用技术经验（高层 evergreen）
- **OOXML 结构改动必须真机验 PowerPoint**（WPS 宽容会漏掉拒收类缺陷）。修一处后须主动排查同类复制点（audio/video 同源缺陷 ADR-025/026 = 复制粘贴同构错误，读侧各维护一份重复探测）。
- **对照实验定性归属**：单边结果易误判为本库缺陷（CreateVideo 不渲染页内音频对象，原生对照组同样无声）。
- **先验证判据本身**：CBR AAC 静音帧同样填充到固定大小 → 逐秒帧大小判据作废。
- **产物流式复制**：io.Copy 每次新分配 32KiB → 循环外用 io.CopyBuffer 复用缓冲。性能/IO 层改动完成定义必须含：SaveToFile 端到端（Load 接受 + 条目集逐条计数不经 map）+ 覆盖率门槛复核。
- **API 表面守门**：改生产代码后必须 -tags=corpus 跑测试；删 facade 须与迁测试同 commit（否则 per-package 覆盖率断裂）。
- chart 抽 internal（ADR-017）：区分零依赖根包类型 vs 接收根包值对象（须 type alias move）；方法随类型搬走，私有方法须公开。
- 详细取证技巧（ZIP raw 直通门限构造、MP4 box 解析、定点增删二分）见 2026-09-16 日日志。

## 事故记录（evergreen 教训）
- git merge/rebase 前 cp -r .git 备份（曾整 .git 消失）。
- `git commit -F <msgfile>` 偶发 fatal: could not read log file（exit 128）但 commit 实际成功 → 先 git log 核实再 push，勿重复 commit，push 单独执行。
- `git commit -m "\u2192"` 转义字面落盘 → 用 Write 写 UTF-8 临时文件 + commit -F。
- Edit 工具偶报成功但改动未落盘 → 重要 Edit 后 Read 复核。
- `go fmt ./...` 把 CRLF 检出文件整体重写为 LF → diff 确认零内容后 git checkout -- 还原。
- 顺手改 B 被测试拦下（ADR-025 视频命名空间）→ 兼容探测要"追加"不要"替换"。

## 待确认
1. 团队人力基线（计划默认 2 开发 + 0.5–1 测试/语料）。
2. ppts 真机门禁（FEAT-001）；图表 numFmt 需 ADR-019、阅读顺序需 ADR+Inspect 扩展。
