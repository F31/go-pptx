# ADR-021: A-2 Shape 能力窄接口拆分

- **状态**: Accepted（已实施）
- **日期**: 2026-09-12
- **关联 ADR**: ADR-014（内部包拆分）、ADR-015（稳定性分级）、ADR-016（渐进式抽取）
- **关联路线图**: `docs/1.x-roadmap.md` A-2（1 周 / 低）

## 上下文

`Shape` 接口聚合了几何、填充、效果、样式矩阵引用、线条等能力 getter。许多调用方只关心其中
一两项能力（如"只要能取几何就接受任意形状"），却被迫依赖完整 `Shape` 接口——这阻碍了按能力
维度的组合与测试替身构造，也不利于 ppts 等下游做窄契约断言。A-2 工作包决定把 `Shape` 按能力
维度切出一组窄接口。

## 决策

在根包新增 5 个能力窄接口（仅新增，不修改 `Shape` 接口、不移除既有 getter）：

```go
type (
    GeometryProvider        interface { Geometry() (GeometryInfo, []Diagnostic, error) }
    FillProvider            interface { Fill() (FillInfo, []Diagnostic, error) }
    EffectsProvider         interface { Effects() (EffectInfo, []Diagnostic, error) }
    StyleMatrixRefsProvider interface { StyleMatrixRefs() ([]StyleMatrixRef, []Diagnostic, error) }
    LineProvider            interface { Line() (LineStyle, []Diagnostic, error) }
)
```

约束（ADR-016 / 021）：

- 接口方法签名与 `Shape` 完全一致，语义一致；
- 所有具体形状类型（AutoShape / TextShape / PictureShape / ChartShape / GroupShape /
  TableShape / AudioShape / VideoShape / OpaqueShape）已通过 `shapeNode` 或自身方法满足这些
  接口——声明即满足，不改动既有类型；
- 新 caller 走窄接口断言，既有 caller 继续用 `Shape`，双轨并存；
- 接口按 ADR-015 标 `// Stable:`，承诺 v1.x 内稳定（仅允许追加新接口/方法）；进入
  goldenStableSymbols 由 `api_surface_test.go` 守门。

## 与 ADR-014 / 016 的关系

- 不推翻 ADR-014 的"不拆公共对象模型"结论——`Shape` 接口本体保持不变，仅在其上叠加可组合的
  窄接口；
- 符合 ADR-016 "新增 internal 抽象必须有当前调用点/测试断言"：本 ADR 以
  `shape_capability_test.go` 的编译期断言（`var _ GeometryProvider = (*AutoShape)(nil)` 等
  8 × 5 矩阵）作为存在性证明。

## 落地范围

1. `shape_capability.go`：5 个窄接口定义（`// Stable:` 标注）。
2. `shape_capability_test.go`：8 个具体形状 × 5 接口 的编译期断言。
3. `api_surface_test.go`：goldenExportedTypes 增加 `GeometryProvider`；goldenStableSymbols
   增加 `FillProvider / GeometryProvider / LineProvider / StyleMatrixRefsProvider`（55 项）；
   `EffectsProvider` 此前已在 goldenStableSymbols。

## 验收门槛

```bash
go test ./...
go test . -run 'TestAPIFrozen'
```

## 后果

### 正面

- 下游可按能力维度做窄契约断言，降低耦合；
- 测试替身只需实现关心的接口；
- 零破坏：所有既有调用方不变。

### 风险

- 接口数量增加后需注意"过度拆分"——后续新增能力接口须有真实 caller 或测试断言；
- `// Stable:` 承诺在 v1.1.0 冻结审查（freeze-list §D）时最终确认。

## 当前落地状态

- 代码已实施并通过全量测试。
- 改动文件：`shape_capability.go`（新增）、`shape_capability_test.go`（新增）、
  `api_surface_test.go`（golden 同步）。
- commit：本批次（随 ADR-019/020 一并提交）。
