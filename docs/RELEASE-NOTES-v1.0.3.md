# go-pptx v1.0.3 Patch Release Notes

> 2026-09-22 · 将随 `git tag -s v1.0.3`（SSH 签名）发布
>
> **本版本是 v1.0.2 后的第三个 patch release**——公共 API **仅追加** 2 个 Stable 公开只读方法 + 1 个 Experimental IR 字段 + 1 个 `ir.Options` 开关（**binary-compat with v1.0.0 / v1.0.1 / v1.0.2**；零删除、零签名变更），主要工作是按 ppts 项目《go-pptx 特性与 bug 跟踪计划》V1.0 §7 实施 FEAT-003 读侧补全（隐藏页 + advTm）、FEAT-002 项 1 备注过滤契约文档化、BUG-001 自愈登记。

## 关键不变量（v1.0.0 / v1.0.1 / v1.0.2 → v1.0.3 不变项）

| 项 | v1.0.0 | v1.0.1 | v1.0.2 | v1.0.3 | 结论 |
|---|---:|---:|---:|---:|---|
| `// Stable:` 段落数 | 34 | 34 | 34 | 34 | ✅ 不变 |
| Stable 符号数（type + 哨兵） | 50 | 50 | 50 | 50 | ✅ 不变 |
| `// Experimental:` 段落数 | 5 | 5 | 5 | 5 | ✅ 不变 |
| 公共 API type 总数 | 158 | 158 | 158 | 158 | ✅ 不变 |
| 错误哨兵语义 | 锁死 | 锁死 | 锁死 | 锁死 | ✅ 不变 |
| 黄金语料 B1 哈希 | PASS | PASS | PASS | PASS | ✅ 不变 |
| Stable 方法数 | 127 | 127 | 127 | **129** | ⚠️ +2（仅追加，只读） |

> **关键**：v1.0.3 的 +2 Stable 方法是**仅追加的只读公开方法**——旧调用方零修改；新调用方（ppts 等）可按需使用。`api_surface_test.go` 的 `wantStableMethods` 已由 127 → 129 自动化守门，未来再追加仍会通过。
>
> v1.0.3 在 v1.0.2 之上**复用** `api_surface_test.go` 的 7 个 AST 断言——任何意外漂移（删方法、改签名、删字段）都会在 `go test` 失败；本版的 +2 方法已在 commit `945dc37` 中更新至 golden 名单。

## Added（公共 API 扩展——仅追加）

### FEAT-003 读侧补全（commit `945dc37`）

按 ppts《go-pptx 特性与 bug 跟踪计划》V1.0 §7/FEAT-003 实施——隐藏页 / advTm / 页序 三项 G0 已实测但公开读侧未完整化的内容，**本版补齐**。

#### `Slide.Hidden() (bool, error)` —— 公开 Stable 方法

- **语义**：读 `p:sldIdLst/p:sldId[@id=slideID]/@show` 属性。OOXML 语义：`show="0"` 表示该页被显式隐藏；缺省 / `show="1"` / 空串一律视为可见。
- **错误**：句柄失效返回 `ErrStaleHandle` / `ErrClosed`；文档操作失败由 `Annotate` 包装。
- **用例**：ppts 自动讲解器过滤隐藏页；编辑器 UI "隐藏/取消隐藏" 按钮读取当前状态；导入器按 `default-skip` 跳过隐藏页。
- **测试**：`TestSlide_Hidden_*` 3 条（可见 / 隐藏 / 句柄失效）。

#### `Slide.AdvanceAfter() (time.Duration, bool, error)` —— 公开 Stable 方法

- **语义**：读 `p:slide/p:transition/@advTm` 毫秒值并转为 `time.Duration`。第二个返回值 `ok` 区分"显式设定"（`true`）与"未设或被外部清空"（`false`）。第三个返回值为错误。
- **与 SetAdvanceAfter 对偶**：写入路径 `Slide.SetAdvanceAfter(d)` 在 v1.0 已存在；本版补齐读侧，ppts 无需自行解析 XML 即可拿到自动切页时长。
- **错误**：句柄失效返回 `ErrStaleHandle` / `ErrClosed`；advTm 非整型或负数返回 `ErrMalformedPackage`；包装层含 `OperationError{Op: "Slide.AdvanceAfter"}`。
- **测试**：`TestSlide_AdvanceAfter_*` 3 条（显式 / 缺省 / 非法值）。

#### `ir.Page.Hidden *bool` —— Experimental IR 字段

- **三态语义**：
  - `nil`：未读取或未确定（`IncludeHidden=false` 时一致）；
  - `&false`：已读，确认可见；
  - `&true`：已读，确认被隐藏。
- **为什么三态**：与 `NotesText string` / `Timing *PageTiming` 等 IR 现有字段的零值即"未设"语义一致；调用方可据此正确处理"隐藏 / 可见 / 跳过判断"三态。
- **同期新增**：`ir.Options.IncludeHidden bool`（默认 `true`）—— `false` 时跳过 `p:sldIdLst` 解析（节省 N 张幻灯片 × 1 次 sldIdLst 扫描），ppts 投影大量幻灯片时可选择性关闭。
- **用例**：ppts IR JSON 协议输出 `hidden` 字段；调用方可按 `*hidden == true` 跳过该页讲稿生成。
- **测试**：`TestFromPresentation_PageHiddenProjection` 2 子测试（visible default / hidden via show="0"） + `TestOptions_Defaults` 验证 `IncludeHidden` 默认值 + `TestFromPresentation_PageHiddenProjection` opt-out 子测试。

#### Frozen 表面增量清单

| 项 | 数量 | 说明 |
|---|---:|---|
| Stable 段落数 | 34（+0） | 不变 |
| Stable 符号数 | 50（+0） | 不变 |
| Stable 方法数 | **129（+2）** | 新增 `Slide.Hidden` + `Slide.AdvanceAfter` |
| Experimental 段落数 | 5（+0） | 不变 |
| 公共 API type 总数 | 158（+0） | 不变 |
| 错误哨兵数 | 17（+0） | 不变 |
| **IR 字段增量** | **1（Hidden）** | 实验性字段，1.x 期间可能按 ADR 调整 |

## Documented（文档体系完善）

### FEAT-002 项 1 备注过滤契约注记（commit `945dc37`）

- **现状**：`SpeakerNotesText` / `SpeakerNotes` / `SetSpeakerNotes` 早在 v1.0 即**仅取** `p:ph type="body"` 占位符（`notesPhType` 实现，notes.go:84-102 严格匹配 `type=body` 或缺省 type），自动跳过 `hdr / ftr / sldNum / dt` 等模板占位符。
- **缺口**：ppts《go-pptx 特性与bug跟踪计划》§7/FEAT-002 项 1 要求把该隐式规则**写明**为契约。
- **修复**：`notes.go` 顶部文件级注释新增"隐式过滤契约"段，明确：
  - 哪些占位符会被 `SpeakerNotesText` 返回；
  - 哪些不会（hdr / ftr / sldNum / dt 继承自 notesMaster，不在合同面上单独保留）；
  - 调用方无需自行判断哪些占位符是讲稿 vs 页脚。
- **代码行为零变化**，仅为契约文档化。

### ppts 同步文档（commit `945dc37`，新增目录 `docs/ppts-sync/`）

为 ppts 项目方验收方便，新增 4 份文档：

- [`docs/ppts-sync/README.md`](docs/ppts-sync/README.md) —— go-pptx 仓库对 ppts《go-pptx 特性与bug跟踪计划》V1.0 §7 的实施索引 + 可直接复制给 ppts 方的同步文本。
- [`docs/ppts-sync/BUG-001-self-healed.md`](docs/ppts-sync/BUG-001-self-healed.md) —— BUG-001 自愈状态说明 + 自愈路径证据链（commit a3abfca 等）。
- [`docs/ppts-sync/FEAT-002-notes-filtering-contract.md`](docs/ppts-sync/FEAT-002-notes-filtering-contract.md) —— FEAT-002 项 1 备注过滤契约的代码路径 + 行为边界。
- [`docs/ppts-sync/FEAT-003-hidden-advtm-read.md`](docs/ppts-sync/FEAT-003-hidden-advtm-read.md) —— FEAT-003 读侧补全的 2 个新公开方法 + IR 字段详细契约。

## Fixed（行为修复 / 自愈）

### BUG-001 自愈登记（无代码改动，仅文档）

- **ppts《go-pptx 特性与bug跟踪计划》§7/BUG-001**：`chart.go` 与 `chartfrag.go` 同时存在 `validateChartData` 函数体——ppts 报告时（基于"当前 working tree 为 3537adf 之后未提交改动"的假设）描述的是 ADR-017 r0 时期的状态。
- **自愈路径**：
  - commit `46c2f2d`（ADR-017 r1）开始搬迁 chart 实现；
  - commit `a3abfca`（ADR-017 r3）删除根包 `validateChartData` 旧实现，**仅 `chartfrag.go:22` 一处声明保留**；
  - 当前 HEAD `945dc37` 经 `git grep "func validateChartData"` 仅一处声明。
- **验证**：`go build ./...` 全绿；默认 14/14 包 + corpus tag 14/14 包全绿。
- **ppts 侧动作**：可直接把该 BUG-001 条目标记为 **CLOSED（自愈）**。go-pptx 侧无代码改动，不需新 commit。

## Verification（如何验证）

```bash
# 全测试（含语料 replay + 冻结守门 + B1 + 新增 6 个）
go test ./...                                        # 14/14 ok（默认）
go test -tags=corpus ./...                           # 14/14 ok（含 B1）

# 冻结守门（AST 断言 7 个测试 + B1 公开语料 + 新功能）
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v .     # 7/7 PASS（wantStableMethods 129）
go test -run 'TestSlide_Hidden|TestSlide_AdvanceAfter' -v .                    # 6/6 PASS
go test -run 'TestFromPresentation_PageHiddenProjection|TestOptions_Defaults' -v ./ir  # 3+/3+ PASS

# 覆盖率（口径同步到 2026-09-22 实测）
CGO_ENABLED=0 go test ./... -coverprofile=/tmp/cover.out
# full total ≈ 84.4% / root ≈ 82.3% / opc 90.4% / chart 91.4% / ir ≈ 87.0%+

# 交叉构建
GOOS=js GOARCH=wasm go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=wasip1 GOARCH=wasm go build ./...
GOOS=linux GOARCH=arm64 go build ./...

# v1.0.0 / v1.0.1 / v1.0.2 → v1.0.3 binary-compat 断言
#   1. TestAPIFrozenExportedTypes 测试通过 → 公共 type 集合零漂移（158 不变）
#   2. TestAPIFrozenStableMethods 测试通过 → 129 个 Stable 方法签名零漂移
#   3. TestCorpusB1UnchangedSave 通过 → Part 字节级保真

# fuzz 定时冒烟（CI 周一 04:17 UTC 自动）
gh workflow run fuzz.yml
```

## Compatibility

- **v1.0.0 → v1.0.1**：binary-compatible（公共 API 零变化）
- **v1.0.1 → v1.0.2**：binary-compatible（公共 API 零变化）
- **v1.0.2 → v1.0.3**：binary-compatible + API-compatible（**仅追加** Stable 公开只读方法 + Experimental IR 字段；无删除、无签名变更、无行为变化。旧调用方零修改；新调用方可按需使用 `Slide.Hidden()` / `Slide.AdvanceAfter()` / `ir.Page.Hidden`）
- **v1.0.3 → v1.1.0**（未来）：待 ADR-017 第四批、ADR-019 图表 numFmt/MajorUnit/MinorUnit、FEAT-002 项 2 阅读顺序建议 1.x 路线图决定

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- v1.0.3 的主要场景适用：ppts 自动讲解器过滤隐藏页 + 读取自动切页时长 + 接收 IR hidden 字段；编辑器 UI 显示隐藏状态；导入器 default-skip 跳过隐藏页

## Acknowledgments

本 patch 由 ppts 项目《go-pptx 特性与bug跟踪计划》V1.0 §7 直接驱动——FEAT-001 / FEAT-002 / FEAT-003 / BUG-001 四条验收契约全部得到代码侧或文档侧应答（详见 `docs/ppts-sync/README.md`）。全部增量在 CHANGELOG `## [1.0.3] - 2026-09-22` 段累积。
