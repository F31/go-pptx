# ADR-016: 渐进式 internal 实现层抽取

- **状态**: Accepted
- **日期**: 2026-09-11
- **关联 ADR**: ADR-014、ADR-015
- **关联基线**: `docs/architecture-current.md`

## 上下文

ADR-014 在发布前语境下决定暂不拆 `internal/textmap` / `internal/style` / `internal/geom` 等实现包，核心理由是避免目录搬迁冲击真实语料保真证据与 v1 API 收敛。

随后真实语料体系已从硬阻塞降级：`testdata/corpus/` 已有 36 个样本索引、3 个公开 LibreOffice 金样闭环、`ext-0024` 私有真实样本冒烟，并且真实样本暴露的 TEXT-02 NUL 字节缺陷已经由回归测试锁定。项目进入“功能继续推进 + 架构收敛并行”的阶段。

## 决策

采用**渐进式 internal 实现层抽取**：

1. 根包 `package pptx` 继续作为唯一公共 SDK 门面。
2. 不把 `TextFrame` / `ChartShape` / `TableShape` 等公共类型迁出根包。
3. 新增 internal 包只承载纯实现逻辑或编译期契约，不能导入根包。
4. 根包通过未导出的 adapter 向 internal 契约适配，避免新增公开 API。
5. 每次抽取必须保持公开 API、金样 diff、corpus validate 和全量测试不变。
6. 每次抽取同步清理硬编码与死代码：已有类型常量、关系类型、Part 命名 helper、manifest 字段可复用时，不新增散落字面量；迁移后无调用的 helper、过期注释、临时兼容分支必须删除。
7. 新增 internal 抽象必须有当前调用点、测试用例或编译期接口断言；否则视为超前抽象，不进入当次重构。

## 与 ADR-014 的关系

本 ADR 不推翻 ADR-014 的“不要做目录搬迁式重构”结论；它细化允许范围：

- **允许**：抽纯算法、纯解析、最小接口契约，例如 `internal/textmap` 的 rune 匹配 / 字素边界 / run span 定位。
- **允许**：新增 `internal/document` 的 `PartStore` 契约，由根包未导出 adapter 实现。
- **不允许**：把根包公共对象模型拆成公开子包，或用 type alias 重导出制造双份 API。
- **不允许**：为适配 internal 包而给 `Presentation` 增加新的公开方法。

## 第一批落地范围

1. `docs/architecture-current.md`：记录当前架构基线。
2. `internal/document`：定义最小 `PartStore` / `ReadStore` / `PatchStore` 契约。
3. `internal/textmap`：抽取 TEXT-02 的纯文本映射原语。
4. `internal/editplan`：定义 `SinglePartPatch` / `MultiPartPlan`，把生产写入路径收敛到 plan helper。
5. 根包 `replace.go`：继续保留 XML 安全规则和 patch 构造，只改为调用 `internal/textmap`。
6. 根包未导出 adapter：`document_store.go` 负责把 `Presentation` 的低层事务 primitive 适配为 `internal/document` 契约。

## 验收门槛

```bash
go test ./...
scripts/gen_corpus/run.sh validate testdata/corpus
```

涉及 TEXT-02 的迁移还必须覆盖：

```bash
go test -run 'TestReplaceText|TestTextFrameReplaceText|TestReplaceTextSuffixInLaterRunDoesNotCorruptPrefix' ./...
```

## 后果

### 正面

- 可以逐步降低根包实现复杂度，不冲击用户 API。
- internal 包获得编译期边界，后续 EditPlan / Document Engine 可平滑接入。
- 高风险文本算法获得独立单元测试。
- 硬编码和死代码被纳入每步验收，避免“抽包成功但维护负担上升”。

### 风险

- internal 包数量增加后，需要避免“小包过度拆分”。
- 根包 adapter 容易变成隐藏依赖层，必须保持最小。
- ADR-014 与 ADR-016 的语境不同，后续文档需要以 ADR-016 作为新阶段迁移准则。

## 当前落地状态

截至 2026-09-11，生产业务路径已不再直接调用 `stageAdd` / `stagePatch` / `stageDelete` / `commit`；直接调用保留在 `presentation.go` 低层 primitive 与 `document_store.go` adapter/helper 边界内。

已迁移到 plan helper 的代表路径包括：

- 单 Part：文本、形状属性、transition、audio timing、table、docProps 刷新等。
- 多 Part：AddSlide、RemoveSlide、notes 创建、Bind 聚合补丁、AddPicture、ReplaceImage、AddAudio、AddVideo、AddChart、Chart SetData、Clone、docProps core/app/custom/root rel 写入。
