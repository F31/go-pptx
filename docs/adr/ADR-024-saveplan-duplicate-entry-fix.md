# ADR-024: `SavePlan.Write` 重复条目缺陷（ext-0024 暴露）

- **状态**: Accepted（已修复）
- **日期**: 2026-09-16
- **严重级别**: **高**（`SaveToFile` 对任何含未变非 XML Part 的文档必然失败）
- **关联 ADR**: ADR-018（Save 流式复制 / Tier 2 raw 直通）——**本缺陷是 Tier 2 的回归**
- **关联条目**: `docs/adr/ADR-018-save-streaming-copy.md` §验收清单第 4 项（唯一未打勾项）
- **登记号**: `BUG-002` → 归 `docs/v1.0-bug-registry.md` §1（已修历史）

## 摘要

ADR-018 留下的唯一未打勾验收项记为「`ext-0024` 存在既有 B1 缺陷——"output has 96
entries, plan wants 75"」，并推测"与 Tier 2 无关"。

**该推测是错的。** 这句话是 `verifyOutput`（`internal/opc/save.go:190`）的真实错误
消息，即 `SaveToFile` **确实失败**；缺陷正是 ADR-018 **Tier 2 引入的回归**：
`SavePlan.Write` 在循环开头无条件 `zw.Create(entry)`，而 `tryRawCopyOriginal` 走通时
又 `zw.CreateRaw` 注册**同名第二个条目**。

**影响面**：任何包含「未变 + 非 XML」Part 的文档（即含图片/视频/音频/嵌入对象的
绝大多数真实 PPTX）经 `Save`/`SaveToFile` 均产出重复条目 → `verifyOutput` 失败、
`opc.Load` 报 `duplicate entry`。合成 `Store` 媒体语料同样中招（已用注入实验证实，
12 条 vs 期望 6 条），故**任何含媒体的 deck 都受影响**，不止 ext-0024。

## 取证过程（三轮，前两轮结论已被推翻，留痕）

### 第 1 轮：误判为"目录条目口径问题"（错）

观察：源 ZIP 91 条目 = 75 文件 + 16 目录；`plan.Entries` = 75；输出 96 条目。
当时推断 `96 = 91 + 15`，归因于「不再复制目录条目（-16）」+「`archive/zip` 的
Stream Data Descriptor 溢出（+31，净 +15）」。

复核 `git show b00df5d:internal/opc/{saveplan.go,zipindex.go}` 发现 SAVE-01 初版起
`Index.Scan` 就已 `HasSuffix(f.Name,"/") → continue`，目录条目**从不**进入
`plan.Entries`。**91/16/15 的数值巧合是误导线索**，与根因无关。

### 第 2 轮：发现真实重复条目（关键转折）

收紧断言后跑 `TestRealExt0024_*`，直接报：

```
Load output: opc: malformed package: duplicate entry "ppt/media/image1.jpeg" (also listed as "ppt/media/image1.jpeg")
SaveToFile: verify output: output has 96 entries, plan wants 75
```

诊断输出包 96 个物理条目：**21 个 media 条目各出现 2 次**，其余 75 - 21 = 54 个正常。
**21 恰好等于源包 non-XML 条目数**（即全部走 Tier 2 raw 直通的 Part）。

→ `96 = 75 + 21`。**与目录条目完全无关**。

### 第 3 轮：隔离定位（根因确认）

`TestZZIsolateCreateRaw` 单独复刻 `tryRawCopyOriginal` 的动作（值拷贝 `f.FileHeader`
→ `OpenRaw` → `CreateRaw` → `io.Copy`），产物 **1 个条目、解压内容保真**，
4 个变体（改 CRC / 清 flags / 清零 sizes）**均正常**。

→ **`CreateRaw` 用法本身没错**。重复只能来自**同一 Part 被注册两次**。

回看 `Write` 循环（`saveplan.go:267`，修复前）：

```go
f, err := zw.Create(entry)        // ← 无条件注册条目 #1
...
case CopyOriginal:
    if raw, err := plan.tryRawCopyOriginal(pk, zw, e.Name); err != nil {
    } else if !raw {
        copyPart(pk, f, e.Name, copyBuf)   // 复用上面 Create 的 writer → 正常
    }
```

- **非 raw 路径**（XML / 不满足门限）：复用 `f`，只注册一次 —— **但注入实验显示 XML
  也曾重复**，因为 `zw.Create` 无条件执行，raw 路径下这个 `f` 被**完全丢弃**（无人写入）
  而 `CreateRaw` 又注册了第二个同名条目 → **两条**。故实为**每条 CopyOriginal 都产出两条**，
  raw 与非 raw 皆然。
- 注入实验（把 `zw.Create` 加回 `CopyOriginal` 分支内）复现：`duplicate entry` ×6、
  `output entries = 12, want 6`、`Load rejected: duplicate entry "[Content_Types].xml"`。

## 根因

`SavePlan.Write` 的条目注册职责在两条路径间**重复**：

| 路径 | 注册动作 | 结果 |
|---|---|---|
| `EmitPatched` / `EmitNew` | `zw.Create` | 正常（1 条）|
| `CopyOriginal` + raw 直通成功 | `zw.Create`（丢弃）+ `zw.CreateRaw` | **2 条** |
| `CopyOriginal` + 回退流式 | `zw.Create`（复用）| 正常（1 条）|

`archive/zip` **允许**同名重复条目注册，不会报错，因此问题一路静默到输出校验。

**为什么四层测试都没抓住**：

1. 既有 Tier 2 测试用 `map[name][]byte` 收集条目（`rawParts` / `decompressedParts`），
   **同名条目互相覆盖**，重复被静默吞掉；`len(outRaw) == len(srcRaw)` 恒成立。
2. 合成测试包**无目录条目、条目数少**，`plan.Entries` 与输出条目数的差异不被断言。
3. `TestRealExt0024_*` 在 CI 上因私有语料缺席**恒 Skip**，且当时末段只有 `t.Logf`
   而**无条目数断言**。
4. `verifyOutput` 的失败只在 `SaveToFile` 路径暴露；`Write` 单测直接写 `bytes.Buffer`
   并自行 `zip.NewReader`（不校验重复），故未被发现。

## 决策

**修 `SavePlan.Write` 的注册职责**（唯一改动点，`internal/opc/saveplan.go`）：
把 `zw.Create(entry)` 从循环开头**下沉到真正需要它的分支**——

```go
switch e.Action {
case EmitPatched, EmitNew:
    f, err := zw.Create(entry)      // 需要 f 写内容
    ...
case CopyOriginal:
    if raw, err := plan.tryRawCopyOriginal(pk, zw, e.Name); err != nil {
        return err
    } else if !raw {
        f, err := zw.Create(entry)  // 仅回退路径创建，raw 路径由 CreateRaw 独占注册
        ...
        copyPart(pk, f, e.Name, copyBuf)
    }
}
```

并加注释说明"raw 路径由 `tryRawCopyOriginal` 自行 `CreateRaw`，**不得**预先 `Create`"，
防止再次被"顺手重构"回去。

**不放宽 `verifyOutput`**：它是正确的守门者，本次正是它先报出问题。也**不放宽 Tier 2
安全门限**（门限工作正常——21 个 media 全部合规地走了直通）。

## 兼容性

- **零公共 API 变化**：改动全在 `internal/opc`，`api_surface_test.go` golden 不变。
- **输出字节变化（修复方向）**：修复前输出含 21 个重复条目即**损坏产物**，修复后为
  ADR-018 承诺的形态（75 条，Part 集合与源一致，解压内容逐字节恒等）。
- **B1 语义不变**：`TestRealExt0024_SaveUnchangedB1` 修复后 75 Part 全部字节恒等。

## 验收与守门

```bash
# 修复验证（需源样本可达）
go test -tags=corpus -run 'TestRealExt0024' -count=1 -v ./internal/opc/
#   → SaveUnchangedB1: 75 plan parts, 75 physical entries；75 parts byte-identical
#   → SaveToFileRoundTrip: OK（修复前为 "output has 96 entries, plan wants 75"）
# 全量
go test ./... && go test -tags=corpus ./... && go vet ./...
```

**新增守门测试 `TestSavePlanWriteNoDuplicateEntries`**（`saveplan_raw_test.go`）：

- 直接按 `zr.File` **逐条计数**（不经 map），断言无同名条目；
- 断言 `len(zr.File) == 计划非 Omit 条目数`（3 media raw + 3 XML = 6）；
- 断言输出可被 `opc.Load` 接受（重复条目会在此报错）。

**守门有效性已验证**（注入退化必红）：把 `zw.Create` 加回后该测试报
`duplicate entry "ppt/media/bench2.bin" appears 2 times` / `output entries = 12, want 6` /
`Load rejected: duplicate entry`。

**收紧既有断言**：`TestRealExt0024_SaveUnchangedB1` 新增 `assertEntryCountInvariant`
——输出 Part 名字集合与 plan 逐项相等，且物理条目数 ≥ Part 数（允许 SPD 溢出，不允许丢条目）。

## 后果

### 正面

- **修复一个会导致产物损坏的高危缺陷**：此前任何含媒体的文档 `Save` 均产出重复条目，
  `SaveToFile` 直接报错；`Write` 直写路径则静默产出 `opc.Load` 拒绝的包。
- ADR-018 唯一未打勾验收项闭合为"已修复"。
- 暴露并修补了 4 层测试盲区：map 去重吞重复、合成包无目录条目、私有语料恒 Skip、
  `Write` 单测不校验重复。新增守门补上第 1、4 项。

### 风险

- **Tier 2 自落地（`752fb95`）即损坏**，而 Tier 2 的性能收益（重压缩占比 50–73%）
  是在**未校验输出结构**的基准里测得的；收益结论本身仍成立（raw 帧直通确实跳过了重压缩），
  但**当时没有端到端验证产物可被接受**。教训：性能优化改动必须同时跑通 `SaveToFile`
  这类带输出校验的端到端路径，不能只跑 `Write` + 自建 `zip.NewReader`。
- `SaveToFile` 对 ext-0024 的失败此前被登记为"既有 B1 缺陷"而**搁置**，若未在本次立项
  取证，缺陷会随 v1.1.0 一同发布。**"搁置未验证项"的代价在此显现**。

## 当前落地状态

- 修复：**已完成**（`internal/opc/saveplan.go`）。
- 守门：**已完成**（`TestSavePlanWriteNoDuplicateEntries` + `assertEntryCountInvariant`）。
- 验证：默认 14 包 + corpus 14 包 + `go vet`（两 tag）+ ext-0024 三测试 全绿。
- 本 ADR 取代初版同名文件（初版"非缺陷/口径问题"结论**已被推翻并作废**）。
