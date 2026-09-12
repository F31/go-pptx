# ADR-014: 根包内部包拆分策略

- **状态**: Accepted
- **日期**: 2026-09-10
- **关联设计**: 《go-pptx 完整设计方案 V2.6 开发实施版》§3、§4
- **替代/废止**: 无
- **关联 ADR**: ADR-013（编辑路径必须经统一事务层）、ADR-016（渐进式 internal 实现层抽取）

## 上下文

go-pptx 根包 `package pptx` 当前承载全部公共 API（**2026-09-10 本 ADR 撰写时点：74 个 .go 文件、35,597 行、158 导出类型、737 导出符号**；ADR-016 重构后 2026-09-12 实测：非测试 41 文件 / 22,091 行，导出类型仍 158），M0–M8 全部落地，已进入发布前。设计文档 §3 规划了如下分层目录：

```
internal/opc/        # ✅ 已落地
internal/xmlstore/   # ✅ 已落地
internal/editplan/   # ✅ 已落地（ADR-016，编辑计划层）
internal/style/      # ❌ 根包内
internal/textmap/    # ✅ 已落地（ADR-016）
internal/geom/       # ❌ 根包内
internal/validate/   # ❌ 根包内
```

实测依赖方向是干净的——`root → internal/{opc,xmlstore,audio,videoprobe}`，无反向依赖、无循环；`ir` 只被 `cmd/pptx` 与 `wasm/check` 引用；`render` 只被自己引用（设计 §3"核心不反向依赖 render"已满足）。Go 反模式扫描：`fmt.Sprintf` 拼 XML 仅 2 处（已知 schema + 整型），裸 XML 常量 0 处；唯一的大函数 `populateCapabilityFeatures`（220 行）是纯声明式数据表，圈复杂度 ≈ 1。

## 决策

**对 5 个未落地的内部包分三档处理：**

### 1. 永久不拆：`internal/style`、`internal/textmap`、`internal/geom`、`internal/validate`

**理由：公共面必须留在根包。** 这 4 个包的"实现细节"与"公开类型"深度重叠：

- `internal/geom` 对外的值类型 `Point`/`Rect`/`Matrix`/`GeometryInfo` 是用户直接调用的公开 API，拆到 `internal/` 后调用方路径变为 `pptx.Point{}` 不变，但内部会退化为 `type Point = pptxgeom.Point` 的 type alias——纯搬运 + 双份文档负担，**收益 ≈ 0**。
- `internal/style` 的 `FillInfo`/`EffectInfo`/`StyleMatrixRef` 同理。
- `internal/textmap` 的 `replacePatches` 是 TEXT-02 跨 Run 替换的核心算法，深度耦合 DrawingML 语义，独立后还得为 root 留公开 facade。
- `internal/validate` 的 `ValidationReport`/`Diagnostic` 是 v0.x 的核心 API，独立后用户调用路径不变但维护成本翻倍。

**结论：拆出去只有两条路——改公开 API（破坏性）或 type alias re-export（纯搬运），都不是好交易。**

### 2. 已部分收敛、暂不深拆：`internal/editplan` / 低层事务 primitive

**ADR-016 后的状态。** 该层承载设计 §3 关键不变量——"不维护两个可独立修改的文档真相"。生产业务路径已收敛到 `internal/editplan.SinglePartPatch` / `MultiPartPlan`，根包通过未导出 `document_store.go` adapter 应用计划；直接 `stageAdd` / `stagePatch` / `stageDelete` / `commit` 调用仅保留在 `document_store.go` adapter/helper 边界（primitive 本体定义于 `presentation.go`）。

当前**不深拆**完整 `internal/edit`，因为：

- 深拆收益是"进一步强制力"而非"正确性"——大规模搬迁的回归风险仍高于收益；
- go-pptx 核心价值是"字节级保真"，真实语料垂直验证（`vertical_corpus_test.go` ext-0024 单 Run 替换差异区间收敛到 1 字节）是产品最强证据，发布前**不希望让核心编辑路径抖动**；
- 业务路径已经通过计划 helper 统一，剩余 primitive 深拆属于"代码组织"而非"功能边界"。

**触发拆分的条件**（任一出现即启动）：

1. **合并冲突热点**：多人并行开发时 `presentation.go` / `document_store.go` / `internal/editplan` 周边出现持续性合并冲突（>3 次/月），意味着对象层与事务层耦合度仍过高；
2. **第三方扩展需求**：有外部团队需要实现自定义语义服务（例如私有模板引擎 / 行业定制格式转换器），需要把 `internal/edit` 升级为 `pkg/edit` 公开 API；
3. **编译时间瓶颈**：根包增量编译超过 5s（当前 < 1.5s），且定位到 `replace.go`/`clone.go`/`bind.go` 三处热路径。

触发任一条件后，迁移范围限定为低层 primitive、`ChangeSet` 操作与冲突检测，不再包含已迁移的业务写入路径。

### 3. 已落地：`internal/opc`、`internal/xmlstore`

`internal/audioprobe`、`internal/videoprobe` 同样已落地，作为媒体探测隔离层。这三处不重复论证。

## 后果

### 正面

- 根包保持 Go 习惯形态（单包几十文件，如 `net/http`、`go/types`），API 调用路径稳定；
- 公共 API 收敛到 `pptx.` 命名空间，便于文档与 capability manifest 一致性维护；
- 真实语料证据不被重构冲击，发布前核心保真能力锁死；
- "何时拆"有明确触发条件，避免每轮重新讨论。

### 负面 / 风险

- "修改必经变更集"这条不变量靠 ADR 013 纪律 + code review 维持，不是编译器强制；新 contributor 误用 raw XML 拼接的可能仍存在（实测：根包内 `fmt.Sprintf` 拼 XML 0 处，当前无违规）；
- 一旦内部出现跨包公共 API 膨胀，根包 158 导出类型会继续增长，需要 v1.x 期间用 API 稳定性分级（`// Deprecated:` / 实验性标记）来管理。

### 后续动作

- 跟踪文档 `docs/go-pptx-实施状态跟踪.md` 移除"目录分歧（非阻塞）"待议项；
- 仓库根目录文件级整理（`options.go` / `save.go` / `shape.go` 命名对齐）于 2026-09-10 完成，**不属本 ADR 范围**——是降低观感成本，零 API 变更；
- 季度审视记录：2026-09-11 首次审视——三项触发条件均未命中（增量编译 0.56s < 5s；无第三方扩展迹象；无合并冲突热点），维持暂不深拆。

## 参考

- 设计文档 §3 总体架构与模块职责
- 设计文档 §4.2 三类写入路径
- ADR-013 模板数据绑定引擎构建在保真补丁之上
- 跟踪文档"目录分歧"待议项的连续讨论记录（2026-09-10 夜段）
