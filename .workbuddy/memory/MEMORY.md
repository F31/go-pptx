# go-pptx 项目长期约定（MEMORY.md）

## 项目定位
纯 Go（CGO_ENABLED=0、无外部运行时强依赖）PPTX 创建/编辑组件。设计基线《go-pptx 完整设计方案 V2.6 开发实施版》，实施依《go-pptx 项目实施计划》M0–M8 阶段门禁制推进。

## 发布状态（2026-09-13 精简版）
- 已发布 tag：**v1.0.0 / v1.0.1 / v1.0.2 / v1.0.3**（均 `git tag -s` SSH 签名 + push origin；签名用仓库级 `gpg.format=ssh` + `user.signingkey=~/.ssh/id_ed25519`，本机无 GPG 勿用）。
- **全系列 binary-compat**：// Stable 34 / Experimental 5 / 50 Stable 符号 / 158 总 type / 17 哨兵（含错误字符串）全部锁死；v1.0.3 仅追加 2 Stable 只读方法（`Slide.Hidden` / `Slide.AdvanceAfter`），**Stable 方法 127 → 129**。
- **不变量由 `api_surface_test.go` 7 个 AST 测试守门**（golden 名单优于计数、parser.ParseDir 优于 grep——`grep '^type [A-Z]'` 只数 149 漏分组声明）。**改动公共 API 表面必须同步 golden 清单**；根包非测试文件禁止 `//go:build`。
- v1.0 冻结清单已闭合（34 Stable 段=33 独立 type+1 哨兵聚合段；曾误记 51/102，系段落 grep 数误当 type 数，详见 freeze list §2026-09-12 勘误）。
- 覆盖率现状（2026-09-13）：root 合并口径 **84.4%**（第 5 轮零覆盖清零后；COV-02 门槛 82%，合并口径=根包测试 binary 对 -coverpkg=./. 的覆盖）/ opc 90.4% / chart 91.4% / audioprobe 88.4% / editplan·textmap 100%。**根包 0% 函数清单已清空**（2026-09-13 全仓扫描）。PERF-01 已在 ADR-016/018 双重构后重跑。
- **唯一真缺口**：V2.6 §15.3 第 3 条后半"配音须有播放记录"——语料无 audio 标签样本，L3 从未真机验证音频播放（环境型，AUDIO-01/02/03 有代码级测试）。L3 客户端矩阵已闭合（真机 8/8，`docs/client-compat-matrix.md`）。
- 1.x 路线图 `docs/1.x-roadmap.md`（5 方向+5 阶段+6 风险+5 决策点）。v1.0 backlog：`docs/v1.0-bug-registry.md` §4。

## 工程约定（已落地）
- module：`github.com/F31/go-pptx`（go 1.24.0）；Apache-2.0；origin = `git@github.com:F31/go-pptx.git`（SSH）；git 本地身份 go-pptx-dev <go-pptx-dev@local>（仓库级，勿外发）。
- CI 必检：`CGO_ENABLED=0` 构建 + vet + test + `GOOS=js GOARCH=wasm` 编译；race 在 ubuntu job（本机 Windows 无 gcc）。
- 错误分层：internal 包自定义内部错误（不反向 import 根包），公共边界映射根包稳定错误码。错误用 error 不 panic；文档/commit 用中文；md 置于 docs/。
- 目录：根包 pptx；internal/{opc,xmlstore,edit,style,textmap,geom,validate,document,editplan,chart}；render/、cmd/pptx/、testdata/corpus/。
- **新增导出符号**：升 Stable 须 freeze list §D checklist（为什么 Stable+不允许扩展方向+使用面举例）；降档必须新 ADR 禁止 PR 直降。
- **句柄身份（STALE-GUARD）**：句柄身份 = 形状 cNvPr@id（path 仅作父容器提示）；hint=0 走纯路径（兼容 notes/老句柄）；只有 RemoveShape 使句柄失效。三层：shapeNode/textNode/Cell。
- **opencode 协作分界**：`testdata/corpus/` 与 `scripts/gen_corpus/` 由 opencode 维护，主代理提交不纳入。
- 状态跟踪：docs/go-pptx-实施状态跟踪.md 每 PR/里程碑后更新；日志按日追加。

## 可复用技术经验
- **chart 抽 internal（ADR-017 三批）**：严格区分"真零依赖根包类型"（可直接搬）与"接收根包值对象"（须先 type alias move，否则反向 import 违反 ADR-014）。const 不能引用 var 包常量。type alias 上不能定义新方法——方法必须随类型搬到 internal，私有方法须公开（binary-compat 表面微调）。ChartNumber 精度保真：`strconv.FormatFloat(v,'g',-1,64)` 历史行为，误写 'f',1 会破 B1 金样哈希。搬迁 5 坑：① `c:v`/`a:t` 值在标签之间不在属性上（取 `Original()[OpenEnd:CloseStart]`+xmlUnescape）；② CachePoints 按 `pt@idx` 排序补空位；③ `dLbls` 是图表组子元素非 plotArea 直接子元素；④ `xl/workbook.xml` 的 `xmlns:r` 不能漏；⑤ 不得顺手放宽 canonical 白名单（会静默改 SetData 拒收语义）。
- **死 facade 判定**：逐个 grep `[^a-zA-Z0-9_]<name>(` 排除自身定义与 _test.go，计数 0 即可删；删 facade 必须与迁测试同 commit（否则 per-package 覆盖率归属断裂，全仓曾 84.4%→79.4% 跌破 COV-01）。
- **ADR-018 Save 流式复制**：`io.Copy` 每次新分配 32KiB 与 Part 大小无关——循环内 per-item 用 `io.Copy` 就是 O(n) 次 32KiB 分配，正解是循环外复用缓冲（`io.CopyBuffer`）。微基准「少而大」不可能暴露 per-item 固定开销，**收益锚点必须同时有「少而大」+「多而小」两档**（已补 200x4KiB 哨兵档）。Tier 2 raw 直通不实施（重启条件见 ADR-018）。
- **跨实现比较 ZIP 产物**：必须按条目名对齐，不能按位置。B1 比对走 OPC Part 视角（PartNames+SHA256）；B1-AFTER 口径：只有 SaveReport.ChangedParts 声明的 Part 可变。**断言绑在 CI 取不到的输入上 = 死断言**（新增门禁先确认输入 CI 可得）。
- **基准测量**：B/op 随 -benchtime 变化，对比须固定同 benchtime；本机 P/E 混合核墙钟漂移 3.5×，跨轮次判定只能用 B/op/allocs/peak-heap 确定性指标；归因须用中间 commit 独立 worktree 隔离变量。
- **本机环境**：bash coreutils（ls/sed/grep/dirname）损坏——文件操作走 PowerShell/专用工具，go 用 `D:/Go/bin/go.exe`（go1.27.0），git 可用；`pwsh` 不可用（PowerShell 工具是 5.1），perf 脚本走 `bash scripts/perf/run.sh`；gofmt 假阳性：CRLF 检出被标记，git 内 LF 是干净的，勿整体重排（用 `git diff --stat` 判断）。**`go fmt ./...` 会把 CRLF 检出文件整体重写为 LF**（git status 全标 M 但 diff 无内容 hunk）——跑完必须 `git checkout --` 还原无内容 diff 的文件，只留真实格式修复；勿直接全量提交行尾噪音。
- **测试策略**：① 纯函数 helper 优先表驱动单测不走 fixture（100× 体积小）；② 100% 覆盖≠好测试——整数溢出等退化分支本质测 stdlib，放过更诚实，**覆盖追逻辑分支不追退化安全网**；③ 扫覆盖率主动查所有 <90% 同文件函数（bug-registry 可能漏列）；④ 零覆盖公开函数必须消除（即使无生产调用方也是 API 盲区）；单 commit 多函数回报是高 ROI 模式。

## 事故记录（重要）
- **2026-09-08**：git merge 内部 stash 失败后 `.git` 整体消失。教训：大操作（merge/rebase）前 `cp -r .git` 备份；保证 working tree clean。
- **2026-09-10/11**：`git commit -F .commit-msg-*.txt` 报 fatal 但 commit 实际创建成功（Windows Git Bash 隐藏文件竞态）——fatal 后先 `git log` 核实勿重复 commit。`.gitignore` 已加 `/.commit-msg-*.txt`。
- **2026-09-12 深夜**：清理期间 scripts/ 整目录从磁盘消失（非删除目标）。教训：批量删除后必须 git status 全量核对；沙箱钩子报 aborted 立即检查无关路径。
- **2026-09-12**：`git commit -m "...\u2192..."` 的 `\u` 转义不被 bash 解析，字面落盘。**唯一可靠**：Write 工具写 `.commitmsg_*.txt`（UTF-8）+ `git commit -F`（不要用 `.commit-msg-*` 命名，见上条）。修复：`git commit --amend -F <新文件>`。
- **2026-09-13**：本会话两次出现 **Edit 工具报成功但改动未落盘**（tablestyle_test.go 的 helper 名替换，编译时发现；MEMORY.md 覆盖率行更新，commit 后才发现丢失）。原因未明（疑似与并行会话/文件监视有关）。对策：**重要 Edit 后立即 Read 验证落盘**；commit 前对关键文件 diff 复核。

## 待确认（阻塞/排期敏感）
1. 团队人力基线（计划默认 2 开发 + 0.5–1 测试/语料）。
2. ppts 真机门禁（FEAT-001，需 PowerPoint/WPS 真机）；FEAT-002 项 3 图表 numFmt 需 ADR-019、项 2 阅读顺序需 ADR+Inspect 扩展。
