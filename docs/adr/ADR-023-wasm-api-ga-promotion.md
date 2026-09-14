# ADR-023: WASM API GA 化 — 5 个 Experimental 段升 Stable

- 状态：Accepted（2026-09-14）
- 关联：ADR-015（稳定性分级与升降档流程）、`docs/1.x-roadmap.md` D-5（S 档）、`docs/ppts-sync/` 跟踪
- 决策：将根包 5 个 `// Experimental:` 段
  （`ChartWorkbookBuilder` / `ChartDataBook` / `DefaultWorkbookBuilder` /
  `CustomPropertyKind` / `CustomPropertyValue`）全部升为 `// Stable:`，
  纳入 v1.1.0 公共 API 稳定契约。

## 背景 / 问题

v1.0 冻结时，5 个适配层 / 值对象类型标 `// Experimental:`（1.x 内可能改）。
但浏览器端 WASM 嵌入（路线图 P3 分发）正是这些类型的主要消费方——
WASM 消费者无法接受 1.x 内的静默变更，否则二进制契约漂移会破坏已发布的
wasm 模块。路线图 D-5（S 档）明确将其列为"1.x 升 GA 候选"，且前置条件
C-1（binary-compat 失守门禁，即 `api_surface_test.go` AST 守门）已于
2026-09-12 落地，升档所需的可观测性已具备。

## 升档提案（ADR-015 §升档 三要素）

### ① 现状使用面调查

- 5 个类型均为根包公开 API 的既有导出符号，自 v1.0.0 起**未发生任何
  签名 / 字段变更**（v1.0.1 ~ v1.0.4 均为追加式只读方法或测试 / 文档增量，
  零生产代码改动）。
- 下游直接依赖面：
  - `ChartWorkbookBuilder` / `DefaultWorkbookBuilder` / `ChartDataBook`：
    经 `Presentation.SetChartWorkbookBuilder` 与 `Slide.AddChart` /
    `ChartShape.SetData` 进入图表工作簿生成路径；WASM 嵌入场景据此提供
    自定义 xlsx 适配器。
  - `CustomPropertyKind` / `CustomPropertyValue`：经
    `Presentation.SetCustomProperty` / `CustomProperties` 进入自定义属性
    读写路径。
- 这些类型在 v1.0 周期内已积累真实集成经验：ADR-017 第三批已将
  `ChartDataBook` 定义下沉 `internal/chart`，但根包 `type alias` 公共表面
  零变化；契约形态已稳定，无待收敛的开放口子。

### ② 公共面契约收敛证明

升档后，以下"可能变更"的开放口子被关闭（即承诺向后兼容，**只增不破**）：

- `ChartWorkbookBuilder.Build(book ChartDataBook) ([]byte, error)`
  签名与错误契约锁定。
- `ChartDataBook`（Categories + ChartSeries 数组顺序 / 对应关系）锁定为
  v1.0 契约。
- `DefaultWorkbookBuilder.Build` 签名与 "SheetName 必须 Sheet1" 约束锁定。
- `CustomPropertyKind` iota 顺序锁定（新增值类型仅追加 iota 末尾，
  不重用、不重排已有常量）。
- `CustomPropertyValue` "Kind + 四字段" 布局锁定。
- 方法面：`DefaultWorkbookBuilder.Build` 进入 `goldenStableMethods`
  （binary-compat 真实表面）。`ChartWorkbookBuilder.Build` 为接口方法，
  不计入方法锁，但其签名随接口升 Stable 一同锁定。
- 不变量守门：`api_surface_test.go` 计数由 `35 / 55 / 5 / 130`
  调整为 `40 / 60 / 0 / 131`，golden 名单同步迁移 5 符号 + 1 方法；
  `go test ./...` 与 `go vet ./...` 全绿。

### ③ 失败回退路径

- 若升档后发现确需破坏性变更（如 `DefaultWorkbookBuilder` 需支持多 sheet
  而现有约束无法容纳）：
  - **禁止静默改**；必须按 ADR-015 §降档 新建 ADR 走评审，先 `Deprecated`
    再提供替代入口，至少两个小版本过渡。
  - 由于本次仅改 godoc 标记、未触碰任何运行时字段 / 签名，回退成本极低：
    仅需将标记改回 `// Experimental:` 并配套新 ADR，二进制契约在过渡期内
    保持兼容。
- 已知取舍：`DefaultWorkbookBuilder` 升 Stable 意味着 ADR-017 规划中的
  "第四批 `DefaultWorkbookBuilder` 迁入 internal" 路径被取代——根包导出
  构建器类型已成为稳定契约，后续内部实现重构须保持该公开类型表面不变
  （实现可下沉 internal，但类型留在根包）。此取舍符合 D-5"GA 优先于内部
  重构"的取向，已在此记录。

## 影响

- 公共 API 表面分类：Experimental 5 → 0；Stable 段 35 → 40、Stable 符号
  55 → 60、Stable 方法 130 → 131；导出 type 总数 163、哨兵 17 不变。
- 二进制兼容：无签名 / 字段变更，binary-compat with v1.0.x；WASM 消费者
  获得稳定契约。
- 文档同步：`docs/1.x-roadmap.md` D-5 标记完成；
  `docs/go-pptx-实施状态跟踪.md` Experimental 计数更新；
  `docs/v1.0-freeze-list.md` 追加 post-v1.0 升档注记；
  `CHANGELOG.md` 增补 `[Unreleased]` 段。

## 不在本次范围

- `ir.Page.Hidden` / `ir.Shape.Diagnostics` 的 `// Experimental:` **字段**
  标记（属 `ir` 包，不计入根包 Experimental 段计数）：前者升 Stable 需
  ADR-019 评审 JSON 协议稳定性（见 FEAT-003 文档），后者为 ADR-020 降置信
  子项 interim 字段，均留待各自 ADR 评审，不随本次 D-5 一并升档。

## 关联

- ADR-015 `#changelog` 加一行登记（2026-09-14，ADR-023，5 Experimental → Stable）。
