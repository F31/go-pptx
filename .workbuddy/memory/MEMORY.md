# go-pptx 项目长期约定（MEMORY.md）

## 项目定位
纯 Go（CGO_ENABLED=0、无外部运行时强依赖）PPTX 创建/编辑组件。设计基线《go-pptx 完整设计方案 V2.6 开发实施版》，实施依《go-pptx 项目实施计划》M0–M8 阶段门禁制推进。

## 发布状态（2026-09-12）
- **v1.0.0 已发布**：`git tag -s v1.0.0`（SSH 签名，仓库级 `gpg.format=ssh` + `user.signingkey=~/.ssh/id_ed25519`）→ commit 44b9ba7；tag 对象 92dc8622。
- **v1.0.1 已发布**：`git tag -s v1.0.1`（SSH 签名）→ commit 9f24866（commit 9f24866 即"docs(release): 准备 v1.0.1 patch release 文档"）；tag 对象 13d1376。**binary-compat with v1.0.0**——公共 API 零变化，// Stable: 34 / // Experimental: 5 / 50 Stable 符号 / 158 总 type / 17 哨兵 全部锁死不变。范围：v1.0.0..HEAD 共 17 commit（ADR-016 渐进式 internal 抽取首轮收敛 / ir 表格文本投影闭环 / 5 包行为优先补测 / L3 文档同步 / MediaSource nil 流保护 / Stable 计数 off-by-one 修正 / ADR-014 三处缺陷 / 技术白皮书 / 1.x 路线图方案）。
- **v1.0 冻结清单已闭合（2026-09-12 勘误口径）**：34 Stable 段落（33 独立 type 段 + 1 个 17 哨兵聚合段；Stable 符号合计 50）/ 5 Experimental / 0 Deprecated / 120 API type / 158 总 type。**勘误原因**：曾误记"51 Stable（34 独立 + 17）/ 102 API"——把段落 grep 数 34 误作独立 type 数（段落含 1 聚合段），且 158−51−5 算式把 var 哨兵错从 type 总数扣除。符号级承诺不变，仅计数修正；详见 freeze list §"2026-09-12 — 口径勘误"。
- **新增导出符号流程**：按 freeze list §D 评审 checklist——升 Stable 须给"为什么 Stable"+ 不允许扩展方向 + 使用面举例；降档禁止 PR 直降必须新 ADR。
- **1.x 已知缺口（更新于 2026-09-12 下午，09-12 深夜实测修正）**：L3 客户端矩阵已闭合（真机 8/8，`docs/client-compat-matrix.md`）；覆盖率补测后 editplan 100% / textmap 100% / **opc 90.0%（实测，旧记 88.9% 已过时）** / audioprobe 88.4% / root 82.8%，full total **84.4%**（COV-04 评估放弃统一 90% 口径）；PERF-01 基线已在 ADR-016 重构后重跑（p50 全链路 -60%~-65%，无回归）。1.x 路线图见 `docs/1.x-roadmap.md`（5 方向 + 5 阶段 + 6 风险 + 5 决策点）。

## 工程约定（已落地）
- module path：`github.com/F31/go-pptx`（CORE-02 已由占位 `go-pptx` 正式化；go.mod `go 1.24.0`）。
- 许可证：Apache-2.0（与远程仓库 LICENSE 字节一致，CORE-02；模板资源许可另行登记）。
- 远程仓库：origin = `git@github.com:F31/go-pptx.git`（SSH 推送；本机 ed25519 已绑定 GitHub 账号 jinfeng105）。
- git 本地身份：go-pptx-dev <go-pptx-dev@local>（仓库级，勿外发）。
- **git tag 签名**：仓库级 SSH 签名已配置（`gpg.format=ssh`）；本机无 GPG 密钥（keyboxd 不可用），勿用 GPG 签名。
- CI 必检：`CGO_ENABLED=0` 构建 + vet + test + `GOOS=js GOARCH=wasm` 编译（V2.6 §26 P1）；race 在 ubuntu job（本机 Windows 无 gcc）。
- 错误分层：internal 包各自定义内部错误（不反向 import 根包防循环），公共 API 边界映射为根包 pptx 稳定错误码。
- 代码风格：错误用 error 不 panic；文档/状态文件用中文；commit message 关联 WP 编号；相关 md 置于 docs/。
- 目录：根包 pptx；internal/{opc,xmlstore,edit,style,textmap,geom,validate,document,editplan,**chart**(2026-09-12 新增)}；render/、cmd/pptx/、testdata/corpus/ 后续。
- **chart 抽 internal 经验（ADR-017 r1+r2，2026-09-12 落地）**：**严格区分"真零依赖根包类型"与"接收根包值对象"**——前者可直接搬（`BuildChartFrameFragment` / `ChartNumber` / `WorkbookColumn` + 4 常量），后者会反向 import 根包违反 ADR-014，必须先 type alias move 值对象。const 在 Go 中必须是编译期常量，**不能直接引用 var 包常量**——保留 const 在根包 / 函数实现搬到 internal 是干净路径。第一批仅 3 函数 + 4 常量；完整抽取需 4 批（零依赖 → type alias → 全搬迁 → 清理），第二批值对象 move 触及公共 API 表面需用户二次审批。**ChartNumber 精度保真**：`strconv.FormatFloat(v, 'g', -1, 64)` 是 chartbook.go 历史行为，搬到 internal/chart 时极易误写为 `'f', 1` 破坏 B1 黄金语料哈希——实施前必查原实现，测试断言用真实输出值。**type alias 与方法定义的根本冲突**（第二批踩坑）：`type X = pkg.X` 后**不能**在 alias 上定义新方法（Go 编译错误 "cannot define new methods on non-local type X"），所有方法必须定义在类型所在包（internal/chart），根包只是 alias 引用。这意味着第二批必须把 String/PlotElement 等方法一并搬到 internal/chart；私有方法（如原 `plotElement()`）必须公开为 `PlotElement()`（这是 binary-compat 表面的微调：私有 → 公开；调用方零修改）。
- **ADR-018 Save 流式复制（2026-09-12 落地 + 量化）**：`SavePlan.Write` 的 `CopyOriginal` 改 `Package.OpenPart` + `io.Copy`（`copyPart`）。实测 24 MiB 未变媒体：B/op −99.7%、峰值堆 −88.4%、ns/op −44.7%。**Tier 2（OpenRaw/CreateRaw 原始帧直通）已验证可行但不实施**（仅再加 1.34×，却需新增 internal raw API + 输出字节变化须重跑 B1/L3），重启条件见 ADR-018。收益锚点 `internal/opc/saveplan_bench_test.go` 已接入 PERF-01（`OPC_*` 环境变量）与 `scripts/perf/smoke.sh` ①b 守门。
- **chart 抽 internal 第三批完成（ADR-017 r3，commit a3abfca，2026-09-12）**：实现层（parse/build/canonical/validate/fragment/workbook + `xmlUnescape`）全部搬到 `internal/chart`；根包 **chart.go 1319 → 481 行**、chartfrag.go 262 → 74、chartbook.go 215 → 107；`internal/chart` 覆盖率 **91.4%**；不变量 Stable 34 / Exp 5 / root type 158 / 17 哨兵 锁死；全仓 total **84.4%**。**关键决定：删除 31 个已无生产调用方的根包私有 facade**——薄包装的原意是"调用方零修改"，但调用方自己已搬走，facade 遂成死代码（虚增根包行数 + 让覆盖率只落根包一侧）。仍保留 `chartIsCanonical` / `parseChartSpace` / `buildChartSpaceXML` / `validateChartData` / `nodeText`（有调用方）。**搬迁期 5 个可复用缺陷**：① `c:v`/`a:t` 的值在**标签之间**不在属性上，写成 `Attr("", "val")` 会永远读空（表现为标题/系列名/类别/数值全空）→ 取 `Original()[OpenEnd:CloseStart]` + `xmlUnescape`；② `CachePoints` 必须按 `pt@idx` 排序补空位，不是按文档顺序追加；③ **`dLbls` 是图表组（barChart/lineChart/pieChart）的子元素**，不是 `c:plotArea` 直接子元素，读侧须与 build 侧对称；④ **`xl/workbook.xml` 的 `xmlns:r` 不能漏**（`<sheet r:id>` 依赖它，缺则产出非法 XML 且改变 xlsx 字节 → B1 金样失败）；⑤ 抽取时**不得"顺手放宽" canonical 白名单**（WIP 版多放行 spPr/trendlineLbl/5 个 errBars 元素并改写 CanonicalRichText），会静默改变 SetData 的拒收语义 → 逐条回归 HEAD 原实现。
- **覆盖率归属陷阱（抽取类重构的通用坑）**：实现搬到 internal 包后若白盒测试仍留根包，**per-package 覆盖率不再归属被执行语句**——本次 `internal/chart` 记 5.5%、全仓 total 从 84.4% 掉到 **79.4%**（跌破 COV-01 的 80% 里程碑）。两种错法都掉分：测试留根包保 facade（facade 100%、internal 0%），或测试迁走但 facade 留（facade 0%）。**正解：迁测试 + 删死 facade 一起做**（本次新增 `internal/chart/migrated_test.go` + `codec_test.go` 后回到 84.4%）。判断死 facade 的方法：逐个 grep `[^a-zA-Z0-9_]<name>(` 并排除自身定义与 `_test.go`，计数为 0 即可删。
- **跨实现比较 ZIP 产物的铁律**：必须**按条目名对齐**，不能按位置——不同实现的条目顺序可能不同（如 raw 直通保留源顺序 vs 按 Part 名排序输出）。
- **基准测量坑**：`B/op` 随 `-benchtime` 变化（1x 时一次性分配摊到单次迭代会显著抬高），前后对比必须固定同一 benchtime。
- **本机 gofmt 假阳性**：`core.autocrlf=true` 检出为 CRLF 的 .go 文件会被 `gofmt -l` 标记；git 内的 LF 版本是干净的，**不要整体重排**（用 `git diff --stat` 行数判断是否为行尾差异）。
- **基准形状必须覆盖「多而小」（可复用铁律，ADR-018 血泪教训）**：任何改造 per-item 处理的优化，收益锚点**必须同时有「少而大」与「多而小」两档**。「少而大」的用例结构上不可能暴露 per-item **固定开销**——ADR-018 Tier 1 用 `io.Copy`（每次新分配 32 KiB，与 Part 大小无关）替换按体积分配的 `readAll`，微基准（3×8MiB 媒体）显示 B/op −99.7%，但真实正文型文档（十几个几 KB Part）付出 `nParts × 32 KiB` 净亏损，**保存分配量反向劣化 10.7×**，只有端到端 PERF-01 能抓到。修法是在循环外**复用同一缓冲**（`io.CopyBuffer`），固定开销 O(n)→O(1)，之后三档才全面优于改前。已补 `200x4KiB` 哨兵档（无复用 7.24MB vs 复用 231KB，31× 灵敏度）。**推论：在循环里对每个 item 调 `io.Copy` 就是 O(n) 次 32KiB 分配。**
- **本机墙钟时间不可用于跨轮次比较**：Intel Core Ultra 9 275HX 是 **P/E 混合核**，Windows 线程调度漂移使**同一份代码**的 `ns/op` 在不同轮次可差 **3.5×**（实测 `Open/100p-media` 6.577ms vs 1.9ms）。回归判定**只能用 B/op / allocs / peak-heap 等确定性指标**（跨轮次稳定到小数点后 2 位）；需要墙钟结论须在同一进程内 A/B 或用 CI 的 Linux runner。已写入 PERF-01 §6。
- **归因不要只比头尾**：中间夹了别的 commit 时，用**中间 commit 的独立 worktree** 跑同一命令隔离变量（本次用 `9bfe44d`=Tier 1 单独 commit，直接排除 chart 重构干扰）。
- **断言绑在 CI 取不到的输入上 = 死断言**：B1「发布硬门槛」曾只挂在私有样本 ext-0024（源文件未入库 → CI 恒 Skip）与合成 fixture 上，等于从未在 CI 生效。已迁到库内可再分发的公开样本（`corpus_b1_test.go`）。**新增门禁前先确认输入在 CI 上可得。**
- **B1 比对走 OPC Part 视角，不是 ZIP 条目**：用 `p.pk.PartNames() + partBytes()` 比解压内容 SHA256；条目级（压缩方式/顺序/时间戳）不在 B1 承诺范围。B1-AFTER 口径：只有 `SaveReport.ChangedParts` 声明的 Part 可变，且纯文本替换的变更集合应只落在 `/ppt/slides/` 下。
- **`pwsh` 本机不可用**（PowerShell 工具是 5.1），但 `go` 在 bash/PowerShell 下均可用 → 跑 `scripts/perf/run.ps1` 请改用 `bash scripts/perf/run.sh`（完整一轮约 5 分钟）。
- **状态跟踪**：docs/go-pptx-实施状态跟踪.md（负责人每 PR/里程碑后更新）；记忆日志按日追加。
- **句柄身份约定（STALE-GUARD，M8 落地，三层已闭环）**：所有句柄身份 = 所属形状的 cNvPr@id（`shapeNode.idHint` / `textNode.shapeHint` / `Cell.shapeHint`），path 仅作"在哪个 spTree/grpSp 下查找"的父容器提示。locate 先按 path 解析出目标元素，再向上遍历找最近 p:sp/p:cxnSp/p:graphicFrame/p:grpSp 的 cNvPr@id 与 hint 比对——不等/找不到返回 ErrStaleHandle；hint=0 走纯路径判定（向后兼容 notes/老句柄）。语义：MoveShape / AddShape（兄弟增）/ 编辑文本后句柄仍有效（cNvPr@id 未变），只有 RemoveShape 让目标 cNvPr@id 消失时句柄才失效。三层覆盖：shapeNode（形状）、textNode（Paragraph/TextRun/TextFrame）、Cell（表格单元格）。
- **opencode 协作分界**：`testdata/corpus/`（含 README.md、s00*、ext-*）与 `scripts/gen_corpus/` 由 opencode 维护，主代理提交不纳入这些路径。

## 事故记录（重要）
- 2026-09-08：git merge 触发内部 stash 失败后 `.git` 目录整体消失（工作区完好）。教训：① 本机 git 大操作（merge/rebase）前先 `cp -r .git` 备份；② merge 前务必保证 working tree clean，避免 autostash 路径。
- 2026-09-10/11（两次复现）：`git commit -F .commit-msg-*.txt` 报 `fatal: could not read log file`（exit 128）但 **commit 实际创建成功**（Windows Git Bash 与 `.` 开头隐藏文件竞态）。处理：fatal 后先 `git log` 核实，已创建则直接 push，勿重复 commit。`.gitignore` 已加 `/.commit-msg-*.txt` 规则。
- 2026-09-12 深夜：项目清理期间 **scripts/ 整目录从磁盘消失**（非删除目标；清理前 Grep 仍可读）。同期 chart.go/chartbook.go/chartfrag.go/internal/chart/types.go 出现未预期大量修改 + internal/chart 新增 6 个未跟踪 .go（形态符合 A-1 第三批 WIP，疑似并行会话在制品）。处置：`git restore -- scripts/` 恢复（纯 D 零丢失）；chart WIP 未触碰。教训：① 批量删除后必须 git status 全量核对；② 沙箱 safe-delete 钩子报 "Some operations were aborted" 时立即检查无关路径；③ 本机 bash coreutils（ls/sed/grep/dirname）损坏，文件操作走 PowerShell/专用工具，git 命令可用。

## 待确认（阻塞/排期敏感）
1. ~~正式 module path~~（已定 github.com/F31/go-pptx）；~~许可证~~（已定 Apache-2.0）；~~v1.0 冻结~~（2026-09-11 已发布）；~~L3 客户端矩阵~~（2026-09-11 8/8 通过，PowerPoint 16.0.20326 + WPS 12.1.0.28599 / Windows 11 10.0.26200 已登记 `testdata/corpus/README.md` §"已登记客户端版本与平台"）。
2. 团队人力基线（计划默认 2 开发 + 0.5–1 测试/语料）。
3. M0 垂直验证需要含动画+未知扩展的真实 PPTX 语料——已由 ext-0024（WPS 私有索引）落地，含 `animation.timing` / `animation.transition` / `xml.unknown_ext`，本地冒烟 + 真机矩阵通过。
