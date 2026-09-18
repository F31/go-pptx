# go-pptx 对《AI 辅助开发编码规范 V1.4》的符合性评估

> 评估对象：`github.com/F31/go-pptx/v2`，HEAD `e8229a2`（v2.0.0 之后），工作区干净
> 评估依据：《AI 辅助开发编码规范》V1.4（用户提供的外部规范全文）
> 评估方式：**只读**。仓库代码零改动；结论以本机实跑（gofmt/vet/test/覆盖率/bench/git 历史）+ 静态度量（正则统计，口径见 §6）为依据
> 评估日期：2026-09-17

## 1. 项目画像判定（规范"使用说明"表）

| 判定项 | 结论 | 依据 |
|---|---|---|
| 主形态 | **纯 Go 库/组件型**（无独立进程，被其他 Go 项目 import） | `go.mod` module `github.com/F31/go-pptx/v2`；公共门面 `pptx/`；无 server/监听 |
| 次形态 | 同仓含 **CLI**（`cmd/pptx`）与 **WASM 构建目标**（`wasm/check`） | 二者均复用门面，不改变主画像 |
| **生效章节** | 0、1.1、**1.3**、2（含 **2.10**）、4、5、6、7、8、10 | 库/组件型行 |
| **跳过章节** | 第 3 章（Web 前端）、第 9 章（多端，wasm 属同语言多目标） | — |
| 服务型专属跳过 | 1.2（服务分层）、2.7（HTTP/gRPC API）、2.8（日志实现）、2.9（DB/事务）、4.6（配置中心）、4.7（指标/追踪） | 画像不匹配 |

**一句话结论**：按"纯 Go 库/组件"画像衡量，当前代码**整体高于规范下限**——尤其在"依赖最小化、架构边界自动化、错误契约、测试与并发纪律"四项上明显超标；差距集中在三处：**①规范本身未落地进仓库（规则入口/AI 标注/PR 模板缺失）②复杂度门禁未进 CI（导致文件/函数长度超标无人拦）③公开 API 契约的两处执行偏差**（导出签名引用 `internal/` 类型、破坏性变更的迁移说明有误）。

---

## 2. 逐章符合性结论

### 0. 总则

| 条款 | 结论 | 证据 |
|---|---|---|
| 小步提交、可回滚 | **符合** | `git log` 为单主题粒度：`fix(video): …`、`docs(sync): …`、`release(v2.0): …`，每个 commit 可独立 revert |
| 禁止臆造依赖与接口 | **符合** | 全仓**零第三方依赖**（`go.mod` 仅 module + go 指令）；ADR 与代码注释大量引用具体文件行号 |
| 安全默认关闭 / 不可信输入 | **部分符合** | 已有 `Budget` 预算层、路径穿越拒绝（`internal/opc/partname.go:48`）、XXE 不可达；但存在可被单文件触发的内存放大（图表 `@idx`、`walkGroup` 路径串），详见本报告同批的 `docs/code-review-2026-09-17.md` |
| 先读后写 / 架构上下文优先 | **符合** | 17 篇 ADR（ADR-014…030）+ `docs/architecture-current.md`；跨层改动均先引 ADR |
| 最小改动面 | **符合** | 提交历史未见"顺手重排 import"类噪音 |
| **AI 标注** | **不符合** | 近 300 个 commit 的 body 中 `AI-assisted:` 出现 **0 次**（`git log --format=%B -300 \| grep -ic AI-assisted` → `0`），而该仓库实际为重度 AI 辅助开发 |
| 组件化/分层不是目标 | **符合** | ADR-030 明确以"降低变化成本"为理由引入分层；`archlint` 有临时例外清单而非一刀切 |

### 1.1 / 1.3 项目结构（库/组件型）

| 条款 | 结论 | 证据 |
|---|---|---|
| 按 1.3 扁平结构，不套服务分层 | **符合** | `pptx/`（106 文件/34.5k 行）+ `internal/*`；无 `biz/data` 服务分层 |
| `option.go`/`config.go`（Functional Options） | **符合** | `pptx/options.go` 全集（New/Open/Save/Validate/Replace/Bind/Merge 各一组 `With*`） |
| 按职责拆多文件，不塞进一个大文件 | **基本符合** | 已按职责拆（`media_audio/audiotiming/picture/video`、`shape/geom/style/table/text_*`…）；但**21 个非生成文件超 500 行**（见 §5.4） |
| `example_test.go` | **符合** | `pptx/example_test.go` 存在 |
| `internal/` / `testdata/` / `README` / `CHANGELOG` | **符合** | 四项齐备 |
| 公开 API 视为契约，破坏性变更走大版本号 + CHANGELOG 说明迁移 | **部分符合** | **版本号规则执行正确**（v2.0.0 迁 `/v2` 路径）；但迁移说明事实错误：`RELEASE-NOTES-v2.0.0.md:28-37` 把 v1.0.x 模块写成 `github.com/F31/go-pptx/v2`，而 `git show v1.0.7:go.mod` 实为 `github.com/F31/go-pptx`，导致其 sed 命令对任何 v1 用户**静默不生效**；且漏报 `ir/` 公共包被收编为 `internal/ir` |
| 多外部依赖实现做成可选子包 | **不适用** | 零外部依赖 |

### 1.5 架构与设计文档

| 条款 | 结论 | 证据 |
|---|---|---|
| 有架构文档 | **基本符合** | `docs/architecture-current.md`：包清单 / 依赖方向（附 `go list` 实测）/ 编辑事务规则 / 压力点 / 重构规则 / v2 门面收敛 |
| 有 ADR 目录 | **符合** | `docs/adr/ADR-014…030`（17 篇，含 v2 目标架构 ADR-030） |
| 必备内容完整性 | **部分符合** | 缺"系统上下文（使用者是谁、典型集成方式）"与"非功能要求（安全/并发/性能/可用性）"专章；并发声明散落在 `pptx/presentation.go:20` |
| 文件命名 `ARCHITECTURE.md` / `CODING_STANDARDS.md` | **不符合** | 现有 `architecture-current.md`；**仓库内无 `docs/CODING_STANDARDS.md`**（见 §6/§10） |

### 2.1 基础风格（库型）

| 条款 | 结论 | 证据 |
|---|---|---|
| gofmt/goimports，CI 检查 | **符合** | `gofmt -l .` 空；CI `ci.yml:29` 有 gofmt 检查步骤；无第三方依赖故 import 分组天然合规 |
| go vet | **符合** | CI `ci.yml:36-39` 两轮（默认 tag + corpus tag） |
| **golangci-lint（含 staticcheck/errcheck/revive/gocyclo/funlen…）** | **不符合** | CI 中**无** golangci-lint 步骤；`.github/workflows/ci.yml` 只有 gofmt/vet/build/test/race/cross-build/corpus |
| 包名小写、简短、无下划线、非复数 | **符合** | `pptx/opc/xmlstore/ir/engine/errs/diag/chart/ooxmlns/archlint/textmap/textutil/audioprobe/videoprobe` |
| 禁止包级可变全局变量 | **符合** | 全仓包级 `var` 均为只读表与哨兵错误；唯一 `init()` 仅填充静态字段（`media_audioicon.go:25`） |
| `go mod tidy` 干净 | **符合** | `go.mod` 无 require 段 |

### 2.2 命名

**符合**。17 个 `Err*` 哨兵（`errors.go`）；接口按行为命名（`Shape`/`Renderer`/`Store`/`DocFunc`/`ChartWorkbookBuilder`），无 `I` 前缀；无 `usr/cfg1` 类缩写；通用缩写 `ctx/err/id/w/r` 使用一致。

### 2.3 错误处理

| 条款 | 结论 | 证据 |
|---|---|---|
| 不吞错误、必须带上下文 `%w` | **符合** | 公共边界统一 `OperationError{Op,Part,Err}` + `%w`；17 哨兵全部 `errors.Is` 可判定，且有 `TestErrorSentinelsFrozen` 锁字符串 |
| 业务错误用哨兵/自定义类型，不用字符串匹配 | **符合** | 同上 |
| panic 仅用于程序员错误 | **部分符合** | 显式 `panic(` 共 48 处**全部在 `scripts/`**，库路径 0 处；但**运行时 panic 可达**：`internal/chart/parse.go:421` 的 `@idx` 无界分配可触发 `makeslice` panic；`pptx/save.go:62` `Save(nil,…)` 触发 nil 解引用（我实测复现）。虽非 `panic()` 调用，但与"不对正常输入崩溃"的条款精神冲突 |
| 错误信息不含敏感信息 | **符合** | 错误串只含 OPC Part 名与操作名 |

### 2.4 并发

**符合**（且强于规范）：生产代码 `go func` **0 处**；`context.Context` 仅用于取消，未塞业务参数；CI 有**独立 race job**（`ci.yml:68-79` `go test -race ./...`）；共享状态假设与实现自洽（无 Mutex、无可变全局态）。

### 2.5 接口与依赖注入

**符合**。接口定义在**使用方**：`presentationPartStore` 实现 `document.Store`（接口在 `internal/document/*`，实现在门面）；`style.DocFunc` 由门面注入解析器；`ChartWorkbookBuilder` + `SetChartWorkbookBuilder` 提供可替换实现；ADR-021 把大接口拆成 5 个窄能力接口（ISP）。

### 2.6 测试

**符合**。覆盖率 `pptx 84.1% / opc 90.3% / xmlstore 91.2%`（我实测）；表驱动 **62 处**；异常/边界覆盖充分（`*_error_test.go`、`*_stale_test.go`、并发 revision 冲突、原子性字节不变）；fuzz **8 个目标**；CI 跑 `-race`。

### 2.7–2.9 服务型条款 → **不适用**（画像不匹配）。

### 2.10 纯 Go 库/组件补充规范（重点章）

| 条款 | 结论 | 证据 |
|---|---|---|
| 耗时/可取消的导出函数首参 `ctx` | **部分符合** | **已带 ctx**：`Save`/`Write`/`Validate`/`AddPicture`/`AddAudio`/`AddVideo`/`AddChart`（`pptx/save.go:54,97`、`media_*.go:170,198,243`、`chart.go:202`）。**未带 ctx**：`Open(path)`（`presentation.go:105`）、`OpenReader(r,size)`（`:143`）、`New()`（`:72`）、`Bind(data)`（`bind.go:78`）、`Shapes()`（`slide.go`/`shape.go:557`）——其中 `Open/OpenReader` 涉及磁盘读取与整包索引，属"可能耗时且应可取消" |
| `init()` 无副作用 | **符合** | 唯一 `init()` 仅写静态字段 |
| 禁止 `log.Fatal`/`os.Exit`/篡改全局状态 | **符合** | `os.Exit` 仅 `cmd/pptx/main.go:33` 与 scripts；库代码**未 import 任何日志实现**（`log`/`log/slog`/zap 零命中） |
| 返回可判定错误，不透内部细节 | **符合** | 17 哨兵 + `OperationError`；不回传底层 SQL/zip 原始文本 |
| 不强制依赖日志实现 | **符合** | 通过 `Diagnostic`/`SaveReport`/`CapabilityManifest` 暴露结构化结果，由调用方决定如何记录 |
| **并发安全性在 godoc 中声明** | **符合** | `pptx/presentation.go:20`："单实例不保证并发安全（含会填充缓存的读取）；多个独立实例可并行" |
| **每个导出符号有 godoc + 至少一个 Example** | **基本符合** | Example：`pptx/example_test.go` ✓。godoc：门面**公共面**仅 **6 处**缺口（5 个 `String()` + `table.go Column`）；另有 10 处为**非导出接收者上的方法**（如 `presentationPartStore`），不计入公共面。全仓（含 internal）口径下 func 注释覆盖率约 88.4%，type/var **100%** |
| 发布前跑 golangci-lint + vet + -race + API diff | **部分符合** | vet ✓、`-race` ✓（CI 独立 job）、**API diff 以自建金样替代**（`pptx/api_surface_test.go`：163 类型/40 Stable 段/131 方法/17 哨兵，等效于 apidiff 的自证，且随 `go test` 跑）；**golangci-lint 缺失** ✗ |

### 4.1 Git 提交规范

| 条款 | 结论 | 证据 |
|---|---|---|
| Conventional Commits | **符合** | 实测类型齐全：`feat/fix/docs/chore/refactor/perf/test/build/ci`；带 scope：`fix(video):`、`docs(sync):`、`release(v2.0):` |
| 一次 commit 一件事 | **符合** | 未混格式化/功能/依赖升级 |
| **AI 生成提交须在 body 标注 `AI-assisted:`** | **不符合** | 近 300 commit **0 处**（见 §0） |
| 禁止 force push 保护分支 | 无法从仓库侧验证 | 需仓库设置侧确认 |

### 4.2 Code Review / 4.3 依赖管理 / 4.4 安全基线

- **4.2**：规范要求"AI 生成代码必经人工 review"——无法从产物验证，但仓库有 `docs/` 自证链（ADR + RELEASE-NOTES + 验证命令），**推断符合流程意图**；仓库无 PR 模板（见 §7.3）。
- **4.3**：**超标符合**——零第三方依赖，"能否用标准库实现"这一问天然满分；`govulncheck`/SBOM 对零依赖项目仅具形式意义，CI 未跑属可接受（建议仍加一条零成本门禁以防未来引入依赖时漏掉）。
- **4.4**：无硬编码密钥 ✓；用户输入校验与白名单 ✓（`Budget`、Part 名校验、媒体类型白名单）；**残留缺口**是资源耗尽类（内存放大），见 §0 与同批代码评审。AI 工具禁止读取密钥——本仓库无 `.env`/密钥文件 ✓。

### 4.5 文档与注释

**符合**。godoc 注释以标识符开头 ✓；注释解释"为什么"（大量引用 ADR/方案章节号）；`docs/adr/` 17 篇 ✓；`docs/api-reference.md`（与生成器输出逐字节一致）✓；`CHANGELOG.md` 逐版本 ✓；README 中英双版 ✓。**小瑕疵**：README 三处仍写 158 类型（金样 163）、`pptx/doc.go` 仍称"根包"/"六子命令"。

### 5.1 / 5.4 复杂度与行数（本轮最大差距项）

规范：嵌套 ≤3 层、圈复杂度 ≤15（理想 ≤10）、函数 ≤50（>80 强制拆）、单文件 ≤500；CI 用 gocyclo/cyclop/funlen 检查。

| 度量项 | 实测 | 判定 |
|---|---|---|
| CI 是否有复杂度门禁 | **无**（golangci-lint 未接入） | **不符合** |
| 非生成/非测试文件 > 500 行 | **21 个**（`docProps.go` 960、`media_audio.go` 914、`shape.go` 872、`media_video.go` 872、`ir/timingir.go` 862、`clone.go` 804、`style/table.go` 707…） | **不符合**（阈值本身注明"非绝对红线"，但数量偏多且无登记豁免） |
| 函数 > 80 行 | **≥12 个**（`capability.go populateCapabilityFeatures` 220、`media_video.go AddVideo` 172、`media_audio.go AddAudio` 149、`text_replace.go replacePatches` 146、`xmlstore Index` 146、`table.go Merge` 140、`docProps.go coreFieldsOperation` 138…） | **不符合** |
| 嵌套深度 | 粗测（花括号深度，含复合字面量与闭包）最深 **9**（`text_node.go shapeIDFromAncestors`）、8（`bind_table.go renderRow`）、7（`bind_body.go bindBody`、`xmlstore Index`） | **需人工复核**：该口径高估控制流嵌套，但结合函数长度看，5.1 的"超 3 层先重构"大概率未系统执行 |
| 函数参数个数 ≤5 | 多数符合；长参数走 `Spec` 结构体（`PictureSpec/AudioSpec/VideoSpec/ChartSpec`）与 Option | **符合** |

### 5.2 / 5.3 设计原则与模式

**符合**。SRP：域拆分到 `internal/document/{geometry,style,text,media,table}`；OCP：`With*` 选项与可替换 `ChartWorkbookBuilder`；DIP/ISP：接口在使用方 + ADR-021 五个窄接口；模式使用克制（函数式选项、适配器式 `DocFunc`、策略式媒体探测分派），**未见为炫技引入的模式包装**；纯组合，无继承式写法。

### 5.5–5.8

- **5.5 安全/并发/可靠/性能**：安全见 §4.4；并发 ✓；可靠性（原子保存、单事务、零残留）✓ 且实测过失败分支；性能**先度量后优化** ✓（仓库有 6+2 个 benchmark 与语料回放）——但**性能看守链路当前断了**（`scripts/perf/run.sh:52,54` 目标包写成 `.`，ADR-029 后模块根无 Go 文件 → 实测 `no Go files … FAIL`），这使"先度量"这一条实际失效。
- **5.6 依赖最小化**：**超标符合**（零外部依赖）。
- **5.7 三次原则/抽象时机**：静态难以判定；从 106 文件规模看存在"按形态而非按复用拆文件"的倾向，建议 review 时抽查。
- **5.8 可测试性**：依赖注入 ✓；**时间不可注入**——`pptx/docProps.go` 直接使用 `time.Now()`（"Modified 由库代管"），导致跨秒保存产物字节不同、可复现构建失败（`capability.go` 的 `GeneratedAt` 属合理用例）。

### 5.9 / 5.10 组件化与分层边界

| 条款 | 结论 | 证据 |
|---|---|---|
| 库/组件：公开 API 与内部实现隔离（`internal/`） | **基本符合** | `internal/` 隔离成立；**但导出签名引用了 internal 类型**：`options.go:26,53,73`（`WithNewBudget/WithBudget/WithSaveDurability`）、`bridge.go:13`（`PartBytes`）、`layout_report.go:68/77-80/106`、`media_audio.go:632,634`、`media_video.go:705-707`。外部模块实测编译报 `use of internal package … not allowed` → 违反"不强制上层调用者感知内部依赖细节" |
| 禁止循环依赖、反向依赖 | **符合（且超出规范）** | 自建 `internal/archlint` 强制 R1–R5（internal 不得 import 门面/工具层、leaf 包只依赖 std-lib…），随 `go test` 运行 |
| 跨层传递用明确类型，禁止 `any`/`map[string]interface{}` 泛滥 | **部分符合** | 全仓 `any` 命中 179 处；公共面集中在 `Bind(data map[string]any)` 等数据绑定入口（语义上合理），但应确认没有泛滥到内部契约 |
| 模块间通过公开接口通信 | **符合** | `document.Store`、`style.DocFunc`、`editplan.Operation` |

### 5.11 架构适应度检查（CI）

**符合且超出规范**。规范要求"建议纳入 CI"，本仓库自建 `internal/archlint`（依赖方向规则 + 测试）+ `go list` 包清单门禁（`RELEASE-NOTES-v2.0.0.md:76`：新包未登记即 FAIL），覆盖"禁止反向依赖/循环依赖/跨层 import"。

### 6. AI 辅助编码通用约束

| 条款 | 结论 | 证据 |
|---|---|---|
| 生成代码必须能通过既有 CI | **符合** | 我复跑全绿：build/vet/gofmt/28 包 ×2 模式 |
| 验证证据、不伪造 | **符合** | 各 RELEASE-NOTES 附验证命令与门禁表 |
| **各客户端规则入口引用同一份 `docs/CODING_STANDARDS.md`** | **不符合** | 仓库根目录仅有 `README.md`、`README.zh.md`、`CHANGELOG.md`、`LICENSE`、`go.mod`、`.gitignore`、`.gitattributes`；**无 `docs/CODING_STANDARDS.md`、无 `AGENTS.md`/`CLAUDE.md`/`.cursor/rules`/`.github/copilot-instructions.md`** → AI 助手每次会话无法自动读取本规范 |
| 提交标注 `AI-assisted:` | **不符合** | 0/300（同 §0/§4.1） |
| 禁止读取密钥/`.env` | **符合** | 仓库无此类文件 |
| 输出 PR 含动机/变更/风险/验证/回滚 | **部分符合** | RELEASE-NOTES 有等价内容；但**无 PR 模板**（`git ls-files` 无 `pull_request_template.md`） |

### 7. 质量门禁与自检清单

| 规范要求（7.1 Go 后端/组件） | 现状 | 判定 |
|---|---|---|
| `gofmt -l .` 无输出 | ✓ `ci.yml:29` | 符合 |
| `go vet ./...` | ✓ `ci.yml:36,38`（两种 tag） | 符合 |
| **`golangci-lint run`** | **✗ 未接入** | 不符合 |
| `go test -race ./...` | ✓ `ci.yml:68-79` 独立 job | 符合 |
| **`govulncheck ./...`** | **✗ 未接入**（零依赖，形式意义） | 建议补（零成本） |
| **API diff（apidiff/gorelease）** | **部分**：未用官方工具，但有自建金样 `api_surface_test.go`（163 类型/40 段/131 方法/17 哨兵），随 `go test` 生效 | 等效符合 |
| 集成/语料测试 | ✓ `ci.yml:89-111` corpus-replay job | 符合（超出） |
| 交叉编译矩阵 | ✓ `ci.yml:145-168`（含 js/wasm） | 符合（超出） |
| **7.2 自检清单 / 7.3 PR 模板** | 仓库无落地文件 | 不符合 |

### 8. 通用 AI Coding Skill 规范

**不适用/未落地**：规范要求 `skills/ai-coding-standards/`（SKILL.md + references + scripts + adapters）。本仓库无 `skills/` 目录。若团队希望 AI 客户端自动加载规范，这是缺失项；若仅以本文档为外部规范，则属不适用。注意 8.6 的边界提示：**Skill 不能替代 CI 门禁与人工 Review**——本仓库正好相反，把门禁放在 CI（强），把规范留在仓外（弱）。

### 9. 多端与跨平台 → **不适用**（wasm 是同一 Go 代码的构建目标，非独立技术栈）。

### 10. 文档维护

| 条款 | 结论 | 证据 |
|---|---|---|
| 规范放 `docs/CODING_STANDARDS.md` 并被引用 | **不符合** | 不存在（见 §6） |
| ADR 放 `docs/adr/` | **符合**（命名 `ADR-0NN-*.md`，与规范示例 `0001-*.md` 风格不同，实质等价） | `docs/adr/` 17 篇 |
| 用户可见变更更新 CHANGELOG | **符合** | `CHANGELOG.md` 至 1.0.7/v2.0.0 |
| API 变更同步文档 | **基本符合** | `docs/api-reference.md` 由生成器维护；但 v2 的 `ir` 包移除未在迁移指南体现 |

---

## 3. 汇总：符合 / 部分 / 不符合 / 不适用

| 判定 | 条款 |
|---|---|
| **符合（或超出）** | 0 小步提交/先读后写/最小改动面 · 1.1/1.3 库型结构（含 example_test、internal、CHANGELOG）· 2.1 gofmt/包名/无可变全局/tidy · 2.2 命名 · 2.3 错误契约 · 2.4 并发（含 CI -race）· 2.5 接口归属与 DI · 2.6 测试（84.1%、62 处表驱动、fuzz 8）· 2.10 日志不绑定/init 无副作用/无 os.Exit/并发安全已声明/错误可判定 · 4.1 Conventional Commits · 4.3 依赖最小化（**零依赖**）· 4.4 密钥与输入校验 · 4.5 文档与注释 · 5.2/5.3 SOLID 与模式克制 · 5.6 依赖最小化 · 5.9/5.10 边界与依赖方向 · 5.11 架构适应度 CI（自建 archlint，超出要求） |
| **部分符合** | 0 安全默认关闭（内存放大残留）· 1.3 破坏性变更迁移说明有误 · 1.5 架构文档缺系统上下文与非功能专章 · 2.10 部分耗时入口缺 ctx（Open/OpenReader/New/Bind/Shapes）· 2.10 公共面 6 处 godoc 缺口 · 2.10 发布前 lint/API diff（无 golangci-lint；API diff 由金样等效替代）· 2.3 运行时 panic 可达 · 5.8 时间不可注入 · 7.1 无 govulncheck/官方 apidiff · 10 API 变更文档不完整 |
| **不符合** | **2.1 / 5.1 / 5.4：CI 无 golangci-lint，复杂度门禁缺失 → 21 文件 >500 行、≥12 函数 >80 行未被拦** · **4.1 / 6：提交 body 无 `AI-assisted:` 标注（0/300）** · **6 / 10：仓库无 `docs/CODING_STANDARDS.md`，无 AGENTS.md/CLAUDE.md/cursor/copilot 规则入口** · **7.3：无 PR 模板** · **5.10.3：导出签名引用 `internal/` 类型，库外不可用** |
| **不适用** | 1.2 服务分层 · 2.7 HTTP/gRPC API · 2.8 日志实现 · 2.9 DB/事务 · 3 前端全章 · 4.6 配置中心 · 4.7 指标/追踪（库按 2.10 暴露可选能力）· 9 多端 |

---

## 4. 建议的整改顺序（只改文档/CI/签名，不动行为）

| 优先级 | 动作 | 对应条款 | 成本 |
|---|---|---|---|
| **P0** | 把本规范落库为 `docs/CODING_STANDARDS.md`，并加 `AGENTS.md` / `CLAUDE.md` / `.cursor/rules` 等**只引用不复制**的入口 | 6、10 | 低 |
| **P0** | 提交 body 统一加 `AI-assisted: <tool>`（或对历史不做追溯、从下个 commit 起执行并在 CHANGELOG 说明） | 0、4.1、6 | 低 |
| **P0** | CI 接入 `golangci-lint`（先启 govet/staticcheck/errcheck/revive/gocyclo/funlen，阈值按现状放行 + 存量豁免清单），让 5.1/5.4 有门禁 | 2.1、5.1、5.4、7.1 | 中 |
| **P1** | 修 `internal` 类型外露：`pptx` 内加 `type Budget = opc.Budget` 等别名并改导出签名；同时给 `archlint`/金样加"禁止导出签名引用 internal 类型"规则 | 5.10.3 | 低 |
| **P1** | 修 v2 迁移说明（模块路径起点、sed 表达式、补 `ir` 包移除），并给金样不变量表加"公共包清单"一行 | 1.3、10 | 低 |
| **P1** | 修 `scripts/perf/run.sh|smoke.sh` 目标包 `.` → `./pptx/`（当前 CI 两个性能任务失败），恢复"先度量后优化"的闭环 | 5.5、7 | 低 |
| **P2** | 拆分或登记豁免超长文件/函数（`docProps.go`、`media_audio.go`、`capability.go populateCapabilityFeatures` 等） | 5.4 | 中 |
| **P2** | 为 `Open/OpenReader` 提供带 ctx 的入口（如 `OpenContext` 或选项注入），避免 v3 才能修 | 2.10 | 中 |
| **P2** | 补架构文档"系统上下文 + 非功能要求"两章；把并发声明上浮到包 doc | 1.5、2.10 | 低 |
| **P3** | `time.Now` 注入化（测试可固定时间 → 产物字节可复现）；补 `govulncheck`、PR 模板、README 数字同步 | 5.8、7.3、4.5 | 低 |

---

## 5. 亮点（可反向输出到规范或 README）

1. **零第三方依赖**——把 5.6"依赖最小化"做到极致，同时天然消解了供应链/SBOM/license 三类风险。
2. **把架构约束做成可执行门禁**（`internal/archlint` R1–R5 + `go list` 包清单门禁），而不是文档里的口号；5.11 要求的是"建议纳入 CI"，这里是自建实现。
3. **API 冻结金样**（163 类型/40 Stable 段/131 方法/17 哨兵 + 错误字符串冻结）替代了 apidiff 的作用，且随单元测试运行——"公开 API 即契约"有了自证手段。
4. **错误契约完整**：`OperationError{Op,Part,Err}` + `%w` + 哨兵，调用方只用 `errors.Is` 即可判定，无字符串匹配。
5. **并发纪律**：生产代码 0 goroutine、CI 独立 `-race` job、godoc 明确声明单实例非并发安全。
6. **失败路径实测过**：原子保存的目标已存在/目标是目录两种失败后无临时文件残留——这是 5.5"可靠性"里最难验证的一条，仓库做到了。

---

## 6. 本次评估的口径与局限

- 度量脚本为**正则估算**（非 go/ast）：函数长度按"顶层 `func` 到配平 `}`"计；嵌套深度按花括号计（**含复合字面量与闭包，会高估控制流嵌套**，故 §5.1 结论标注"需人工复核"）；godoc 覆盖按"紧邻上一非空行是否为 `//`"判定（已人工区分公共面与非导出接收者方法）。
- CI 结论读自 `.github/workflows/ci.yml` 文本，未真实触发 GitHub Actions。
- 未验证项：仓库设置（分支保护）、PR 历史中的 AI 标注是否曾存在于 squash 前的 commit、5.7 抽象时机的抽样、govulncheck 实际结果。
- 与本报告同批的 `docs/code-review-2026-09-17.md` 提供了安全性/性能问题的完整证据链，本评估只引用结论，不重复展开。
