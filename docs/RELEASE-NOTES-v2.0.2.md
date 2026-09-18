# go-pptx v2.0.2 Release Notes

> 2026-09-18 · 随 `git tag -s v2.0.2`（SSH 签名）发布
>
> **句柄身份体系（STALE-GUARD）加固 patch：修复"复用外部 ID 做身份"方案的经典 ABA 陷阱，
> 并补齐不可信输入下 `cNvPr@id` 唯一性的诊断。两项均为追加式行为修正，公共 API 无移除、无改名。**

## Table of Contents

- [版本定位](#版本定位)
- [迁移指南 v2.0.1 → v2.0.2](#迁移指南-v201--v202)
- [关键不变量 v2.0.1 → v2.0.2](#关键不变量-v201--v202)
- [Changed](#changed)
- [Added](#added)
- [Verification](#verification)
- [Compatibility](#compatibility)
- [Known Limitations](#known-limitations)
- [Feedback](#feedback)

## 版本定位

本版落地代码评审中对白皮书 §3.6「句柄身份体系」的两点严谨性质疑：

1. **ABA 复活**：原分配器取"当前 spTree 内 `max(id)+1`"，删除拥有最大 id 的形状后新增形状会
   **复用同一 id**；旧句柄按 id 懒定位（`locateByIDHint` 第一个匹配即返回）会静默"复活"，
   返回错误形状的几何/文本而非 `ErrStaleHandle`。
2. **不可信输入下 `cNvPr@id` 全局唯一前提未必成立**：OOXML 规范要求 id 全局唯一，但第三方畸形/
   不可信文件可能违反；一旦违反，"比对最近祖先 cNvPr@id"的定位逻辑可能匹配到错误形状而不自知。

两项修复都遵循"诊断而非静默丢弃"的工程哲学，对公共 API 零破坏。

## 迁移指南 v2.0.1 → v2.0.2

**无需改动即可升级**（纯行为修正，无签名变更）：

1. 新分配器为单调只增，**不再保证 id 连续紧凑**。依赖"新增形状 id == 旧最大 id + 1"这类内部
   约定的下游需要改为不依赖具体数值（形状 id 仅作句柄身份，不应作为业务序号）。
2. 处理不可信/畸形 `.pptx` 的下游，现在可在 `Open` 后调用 `Validate` 读取
   `ValidationReport`，对 `DRAWING_ID_DUPLICATE`（`SeverityWarning`）自行决定拒绝：

   ```go
   p, err := pptx.Open("untrusted.pptx")
   if err != nil { /* … */ }
   rep := p.Validate(pptx.ValidationModeStrict)
   for _, d := range rep.Diagnostics {
       if d.Code == "DRAWING_ID_DUPLICATE" {
           // 业务侧决策：拒绝打开 / 告警后继续
       }
   }
   ```

   > 默认 `Open` **不拒绝**重复 id 的文件——与"诊断而非静默丢弃"一致。是否拒绝由调用方决定。

## 关键不变量 v2.0.1 → v2.0.2

| 项 | v2.0.1 | **v2.0.2** | 结论 |
|---|---:|---:|---|
| 公共 API type 总数 | 166 | **166** | 不变 |
| `// Stable:` 段落数 | 42 | **42** | 不变 |
| Stable 方法数 | 134 | **134** | 不变 |
| 错误哨兵数 | 17 | **17** | 不变 |
| 黄金语料 B1 哈希 | PASS | **PASS** | ✅ 不变 |

> 本版仅新增未导出的分配器方法与一个诊断 Code 字符串常量，未触及 `pptx/api_surface_test.go`
> 金样；`ShapeID` 仍为底层 `uint32` 的稳定身份。

## Changed

- **形状 ID 分配器改为单调只增、永不复用**（`pptx/presentation.go`）：原 `nextShapeID` 取
  "当前 spTree 内 `max(id)+1`"，删除拥有最大 id 的形状后新增形状会复用同一 id，旧句柄按 id 懒
  定位（`locateByIDHint`）会静默"复活"、返回错误形状的几何/文本而非 `ErrStaleHandle`。现由
  `Presentation.allocShapeID()` 维护单调上界（`maxShapeID` 只增），`New` / `Open` / `OpenReader`
  后 `seedShapeIDAlloc()` 扫描全文档所有 slide 的 spTree 取全局最大值初始化；耗尽
  （`xsd:unsignedInt` 上限 4294967295）返回 `ErrOutOfRange`。`RemoveShape` 不再使分配器回落，
  旧句柄经定位比对失败返回 `ErrStaleHandle`，不复活。OOXML 允许 `cNvPr@id` 在区间内任意跳跃，
  不违反规范。
- **`Validate` 新增重复 `cNvPr@id` 诊断**：同 slide spTree 内出现重复 `cNvPr@id` 时产出
  `DRAWING_ID_DUPLICATE` / `SeverityWarning` 诊断（见白皮书 §6.4.1）。默认仅诊断、不拒绝打开。

## Added

- 回归测试：`pptx/create_test.go`（`TestStaleGuard_ReuseAfterRemoveNoABA`）、
  `pptx/validate_dupid_test.go`（`TestValidate_DuplicateShapeID` / `TestValidate_UniqueShapeIDOK`）。
- 白皮书 §3.6.1（单调分配器语义）与 §6.4.1（诊断登记）同步。

## Verification

```bash
go build ./...                                   # PASS
go vet ./...                                     # 零警告
gofmt -l .                                       # 零输出
go test ./...                                    # 全包 PASS
go test -tags=corpus ./...                       # 全包 PASS（含 B1 金样比对）
bash scripts/coverage/gate.sh                    # COVERAGE GATE PASSED (tolerance=0.0)

# 金样门禁（API 冻结面，行为不变）
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v ./pptx/

# 行为验证
go test -run 'TestStaleGuard_ReuseAfterRemoveNoABA|TestValidate_DuplicateShapeID|TestValidate_UniqueShapeIDOK' -v ./pptx/

# fuzz 冒烟（8 目标，无 crasher）
go test -run='^$' -fuzz='FuzzLoad'        -fuzztime=5s ./internal/opc
go test -run='^$' -fuzz='FuzzScan'        -fuzztime=5s ./internal/opc
go test -run='^$' -fuzz='FuzzScanner'     -fuzztime=5s ./internal/xmlstore
go test -run='^$' -fuzz='FuzzIndex'       -fuzztime=5s ./internal/xmlstore
go test -run='^$' -fuzz='FuzzBind'        -fuzztime=5s ./pptx
go test -run='^$' -fuzz='FuzzReplaceText' -fuzztime=5s ./pptx
go test -run='^$' -fuzz='FuzzSetPlainText'-fuzztime=5s ./pptx
go test -run='^$' -fuzz='FuzzOpenReader'  -fuzztime=5s ./pptx
```

## Compatibility

- **v2.0.1 → v2.0.2：API-compatible + source-compatible**。无签名移除、无改名、无返回值类型变化；
  无新增导出符号，金样门禁不受影响。
- 唯一的行为变化：形状 id 分配从"复用型（max+1）"变为"单调型（只增）"。
  **任何把形状 id 当业务序号/连续编号使用的下游需要改为不依赖具体数值**——这从来不是承诺的契约。
- 下游升级动作：**建议**处理不可信/畸形 `.pptx` 的下游升级，以获得 `DRAWING_ID_DUPLICATE` 诊断能力。

## Known Limitations

- `Validate` 的重复 id 检测范围为**单 slide spTree 内**；跨 Part（如 notesMaster 与 slide 共用
  id 空间）的唯一性未做全局校验——OOXML 要求每 Part 内唯一，跨 Part 不同 id 是合法的。
- `seedShapeIDAlloc` 仅扫描 slide 的 spTree；若未来支持 notes/comment/master 形状分配，需同步
  扩展扫描范围（当前这些形状不通过 `allocShapeID` 创建）。
- 严格模式拒绝逻辑由调用方实现，库本身不提供"打开即拒绝"的开关。

## Feedback

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 本版适用场景：处理不可信/第三方 `.pptx` 的下游、以及任何依赖 `RemoveShape` 后旧句柄可观测失效
  的场景。
