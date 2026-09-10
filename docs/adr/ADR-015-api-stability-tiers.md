# ADR-015：API 稳定性分级与升降档流程

- 状态：已采纳
- 日期：2026-09-10
- 适用范围：根包 `pptx` 的全部导出符号（含类型、函数、常量、变量）。
- 关联 ADR：ADR-014（内部包拆分策略，定义 v1.0 概念边界）。

## 背景

go-pptx 当前导出 158 个类型（含 2 个 `type alias`，732 个方法/常量/变量符号），分发前必须给出"哪些承诺兼容、哪些会改"的明确承诺——否则下游客户不敢升级，遇到突然变更会视为作者疏忽。

设计 V2.6 没有条款约定分级方法；跟踪文档第 47 行"待确认"项中提到"分层维护"但未落地机制。本 ADR 钉死三级标签、降档/升档规则、降级与冻结承诺。

## 决策

根包导出符号按三级分级，godoc 段落标签为唯一标记（不强制 struct tag，便于 godoc / pkg.go.dev 直接渲染）：

| 标签 | 形态 | 兼容性承诺 |
|---|---|---|
| **Stable** | `// Stable: <reason>` 紧贴 type/func 声明上方段落 | v1.0 后**承诺向后兼容**——只增不破；行为/语义可加严但不可放宽；允许弃用但必须保留替代入口至少两个小版本 |
| **API（默认）** | 无标签 | v1.0 锁定前可调整字段；锁定承诺由 ADR-015 §"v1.0 锁定流程"规定 |
| **Experimental** | `// Experimental: <reason>` 紧贴 type/func 声明上方段落 | 1.0 内**可能改**——可新增方法、新增字段、调整不兼容语义；事件必须发公告，不静默改 |
| **Deprecated** | Go 1.19+ 标准 `// Deprecated: <替代>` 标签 | v1.0 锁定前通常不引入；保留至少两个小版本 |

错误码（`Err*` 哨兵）、错误类型（`OperationError`、诊断 `Diagnostic` / `Severity`）默认 Stable——错误码是 API 兼容契约的硬约束。

## 升降档规则

### 升档（Experimental → API→ Stable）

- 升档提案 PR 必须包含：① 现状使用面调查（哪些下游代码已直接依赖）② 公共面契约收敛证明（删除/收紧了哪些变更可能性的开放口子）③ 失败回退路径（如果升档后发现必须变更，是否能迁回 Experimental）。
- 升档提交须 `docs/adr/ADR-015-api-stability-tiers.md#changelog` 加一行登记。

### 降档（Stable → Experimental 或 Deprecated）

- 视为**破坏性变更**，必须：① 走 ADR 评审流程（新建 ADR-0xx）② 在 `docs/go-pptx-实施状态跟踪.md` 顶部"破坏性变更公告"段写入日期与降档范围 ③ 至少两个小版本的过渡期内 `Deprecated` 与替代入口并存。
- 严禁未经 ADR 静默降档。

### 新增标 Stable

- 仅限**通用对象模型入口**（类比 `net/http` 的 `Handler` / `http.Request`）与**错误/诊断契约**。
- 加性工作（如新增 spec 类型、报告类型、辅助函数）一律默认 API 级；不在 PR 中讨论升 Stable。

## v1.0 锁定流程

v1.0 标签的发布日 T-2 周起：

1. 打开 ADR-015 的"v1.0 冻结清单"附录（按 PR 增量加新行），列出全部尚未标注的导出符号。
2. 每行由负责模块的审阅者补标 Stable / API / Experimental / Deprecated。
3. 在 `docs/go-pptx-实施状态跟踪.md` 公开通告后，v1.0 tag 同步冻结 API 级字段（不再接受字段重命名/类型调整 PR）。
4. Experimental 与 Deprecated 可继续演化为 v1.x。

## 当前快照（2026-09-10）

| 标签 | 数量 | 候选列表 |
|---|---|---|
| Stable | 3 + 28 + 6 = **37 个独立类型 / 21 个 Stable 段落** | 核心对象模型入口 3 个 + T-2 周首批加 Stable 段落 28 个（错误码 17 集合段 + OperationError 1 + 诊断契约 4 + 几何值对象 4 + 枚举 2）+ T-1 周收敛 6 个（Capability Output 契约 4 + 句柄 ID 类型 2） |
| Experimental | 5 | `ChartWorkbookBuilder`、`ChartDataBook`、`DefaultWorkbookBuilder`、`CustomPropertyKind`、`CustomPropertyValue` |
| Deprecated | 0 | — |
| API（默认） | 116 | 其余全部导出符号，含全部 `*Spec` / `*Option` 输入规格、`*Info` / `*Report` 只读报告 |

> **注**：本 ADR 不引入"冻结"以外的承诺标签——以减少语义摩擦。`// Deprecated:` 是 Go 1.19+ 标准注释惯用法，本 ADR 不重新约定。

## v1.0 冻结清单（2026-09-10 启动）

T-2 周启动文档：[`docs/v1.0-freeze-list.md`](../../v1.0-freeze-list.md)。要点：

- **Stable 候选**（17 项，ADR-015 §决策默认 Stable，已在 2026-09-10 T-2 周首批加 `// Stable:` 段落）：错误码 `Err*`（17 个，集合性段落 + OperationError + 4 个诊断契约 `Diagnostic` / `Severity` / `ValidationReport` / `CapabilityStatus`）。
- **Stable 候选**（额外审查可考虑，已在 2026-09-10 T-2 周首批加 `// Stable:` 段落）：`Point` / `Rect` / `EMU` / `Quad`（`geom.go`）/`ReplaceMode`（`replace.go`）/`MultiCellTextPolicy`（`table.go`）——值对象与枚举，对用户 switch/case 完备性有约束。
- **Experimental 保留**：5 项不变（已写明演化边界）。
- **API 默认**（149 项）：按类别聚合评审，详见 freeze list §C。

执行时间线（T-2 周 → T-0）见 freeze list §E；本 ADR §"v1.0 锁定流程" 与之同步。

## 关联

- doc.go 顶层 godoc 含"API 稳定性承诺"摘要，引用本 ADR。
- 跟踪文档 `docs/go-pptx-实施状态跟踪.md` 第 47 行"API 稳定性"待确认项已闭合。
