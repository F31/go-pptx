# go-pptx v2.0.1 Release Notes

> 2026-09-18 · 随 `git tag -s v2.0.1`（SSH 签名）发布
>
> **v2.0.0 后的第一个 patch release，定位为"评审后加固"——把 `docs/code-review-2026-09-17.md`
> 的安全性 / 性能 / 易用性发现项全部落地。公共 API 为纯追加：`pptx` 包新增 3 个类型别名 +
> 1 个构造函数 + 2 个取值常量；唯一的语义放宽是 `Presentation.Close()` 改幂等。**

## Table of Contents

- [版本定位](#版本定位)
- [迁移指南 v2.0.0 → v2.0.1](#迁移指南-v200--v201)
- [关键不变量 v2.0.0 → v2.0.1](#关键不变量-v200--v201)
- [Added](#added)
- [Fixed](#fixed)
- [Changed](#changed)
- [Verification](#verification)
- [Compatibility](#compatibility)
- [Known Limitations](#known-limitations)
- [Feedback](#feedback)

## 版本定位

本版发布 v2.0.0 之后落在 `main` 上、尚未进入任何 tag 的 9 个提交。它们源自一轮四维代码评审
（安全 / 稳定 / 易用 / 性能，见 `docs/code-review-2026-09-17.md`），不引入新特性，目标是**把
v2.0.0 里"承诺了却做不到"和"会被单文件打崩"的几处**补上：

- **能拧动的旋钮**：`WithBudget` / `WithNewBudget` / `WithSaveDurability` 此前直接引用
  `internal/opc` 的类型，Go 的 internal 规则让外部模块**根本无法引用**，文档里承诺的资源预算
  实际不可设置 → 以 `pptx` 包类型别名补齐。
- **不可信输入的边界**：图表 `c:pt/@idx` 与 MP4 `ftyp` 品牌列表两处无界分配，可被单个恶意
  `.pptx` 驱动到 `makeslice` panic 或数十 GB 分配 → 加上界。
- **热路径**：`Slides()` 的循环不变量 + 媒体哈希 O(N²) → 实测遍历 −75.8% 耗时、−95.6% 内存。
- **契约一致性**：`Save(nil)` 与同包 `Write` 行为不一致（panic vs 归一化）、CLI `--help` 退出码
  与顶层不一致、覆盖保护存在 TOCTOU → 统一。

## 迁移指南 v2.0.0 → v2.0.1

**无需改动即可升级**（仅追加 + 语义放宽）。两处可选动作：

1. **此前想设资源预算却编译不过**的调用方，现在可以直接写：

   ```go
   p, err := pptx.Open("in.pptx",
       pptx.WithBudget(pptx.Budget{MaxEntries: 5000, MaxUncompressedBytes: 256 << 20}),
       pptx.WithSaveDurability(pptx.DurabilityFull),
   )
   ```

   此前必须写成 `opc.Budget{…}`（会触发 `use of internal package not allowed`）。
   别名与 `internal/opc` 的实体是**同一类型**，零转换成本、零语义漂移。

2. **依赖"二次 `Close()` 返回 `ErrClosed`"的断言**需要改成 `nil`。示例：

   ```go
   // v2.0.0：第二次 Close 返回 ErrClosed
   // v2.0.1：重复 Close 一律返回 nil；Close 之后的业务方法仍返回 ErrClosed
   _ = p.Close()
   if err := p.Close(); err != nil { /* 现在不会走到这里 */ }
   _, err = p.Slides() // 仍然是 ErrClosed
   ```

> `opc.Budget.MaxXMLDepth` 被移除。该字段**全仓无消费点**（真实深度限制在 `internal/xmlstore`
> 的 256），调用方设置了以为深度受控、实际没有。它此前随 `internal/opc` 无法被外部引用，
> 故移除不构成对外的破坏性变更。

## 关键不变量 v2.0.0 → v2.0.1

| 项 | v2.0.0 | **v2.0.1** | 结论 |
|---|---:|---:|---|
| 公共 API type 总数 | 163 | **166** | +3（别名，纯追加） |
| `// Stable:` 段落数 | 40 | **42** | +2（别名分组段 + `Durability` 取值段） |
| Stable 符号数 | 60 | **65** | +5 |
| Stable 方法数 | 131 | **134** | +3（`PartName.String/Valid/EntryName`） |
| `// Experimental:` 段落数 | 0 | **0** | 不变 |
| 错误哨兵数 | 17 | **17** | 不变 |
| 黄金语料 B1 哈希 | PASS | **PASS** | ✅ 不变 |

> 计数以 `pptx/api_surface_test.go` 的金样为准；`docs/api-reference.md` 已随别名重新生成
> （166 types / 37 顶层函数 / 143 导出方法 / 151 常量 / 18 哨兵）。

## Added

- **`pptx.Budget` / `pptx.Durability` / `pptx.PartName`（type alias，Stable）**：新文件
  `pptx/aliases.go`，并配套导出 `pptx.DefaultBudget()` 与 `pptx.DurabilityDefault` /
  `pptx.DurabilityFull`。修的是"承诺了却拧不动"：`WithBudget` / `WithNewBudget` /
  `WithSaveDurability` / `PartBytes` 以及 `AudioProfile.MediaPart`、`VideoProfile.MediaPart`、
  `LayoutReport.Part/Parts` 等导出字段原本直接引用 `internal/opc` 的类型。
- `pptx.PartName.String()` / `.Valid()` / `.EntryName()` 随之进入 Stable 方法面。
- 回归测试：`internal/chart/cache_bounds_test.go`、`internal/videoprobe/brand_bounds_test.go`、
  `pptx/save_contract_test.go`。
- `docs/api-reference.md` 重新生成（166 / 37 / 143 / 151 / 18）。

## Fixed

- **【安全】图表 `c:pt/@idx` 无界分配**（`internal/chart/parse.go`）：`idx` 直接取自文件内容，
  `make([]string, max+1)` 可被单文件驱动到 `makeslice` panic 或约 16 GB 分配，且经公共 API
  `ChartShape.Data()` 可达、库内无 recover。现加上界（Excel 32k 点 ×2 余量），并保留
  "缺号补空串"语义。
- **【安全】MP4 `ftyp` 兼容品牌列表无上限**（`internal/videoprobe/mp4.go`）：每 4 字节 append
  一个 string，可放大 21×；现收集上限 64 个。
- **【性能】`Presentation.Slides()` 每页重复读取并解析主关系流**：该调用是循环不变量，
  提到环外并按 revision 缓存。
- **【性能】`AddPicture` 去重 O(N²)**：`findExistingMedia` 对每个已存媒体都全量读字节再 SHA256；
  新增 part→hash 缓存（随 revision 失效）。
  实测 `BenchmarkPerfTraverse/100p-media`（`-benchtime=6x`）：
  16.40 ms → **3.96 ms**（−75.8%）、26.77 MB → **1.17 MB**（−95.6%）、
  223,284 → **10,713** allocs（−95.2%）。
- **【工具】`scripts/perf/{run,smoke}.sh` 目标包写成 `.`**：ADR-029 后模块根已无 Go 文件，
  脚本长期 `no Go files … FAIL`（CI 两个性能任务因此长期红）。改为 `./pptx/`。
- **【易用】v2 迁移指南事实错误**（`docs/RELEASE-NOTES-v2.0.0.md`）：把 v1.0.x 的模块路径写成
  `github.com/F31/go-pptx/v2`（实为 `github.com/F31/go-pptx`），给出的 `grep|sed` 对任何 v1
  用户都匹配不到——跑完以为迁完，实际一行没改。同时补报此前漏记的破坏性变更：
  v1 的公共包 `ir/` 在 v2 变为 `internal/ir` 且门面不再暴露。
- **【稳定性】`Save(nil, …)` 直接 panic**，而同包的 `Write` 早已做 nil 归一化、`Validate` 容忍
  nil → 统一为 nil → `context.Background()`。
- **【易用】CLI**：子命令 `--help` 此前退出码 2 且只写 stderr（顶层 `--help` 却是 0 且写 stdout），
  `pptx inspect --help | less` 之类常规用法失效 → 统一为 usage 写 stdout、退出 0；真正的参数
  错误仍走 stderr + 退出 2。
- **【易用】CLI 覆盖保护存在 TOCTOU**：`writePresentation` 先 `os.Stat` 再无条件传
  `WithSaveOverwrite(true)`，既绕开库自身的 `ErrOutputExists` 守卫，也有 Stat 与 Save 之间
  目标被他人创建的竞态 → 改由库在一次原子检查内判定（退出码 4 → 2，与 §23.2 对齐）。
- **【文档】** README(中/英) 三处 `158 types / 131 methods / 40 Stable 段` 与金样不符 →
  166 / 134 / 42；`PictureShape` 的 Stable godoc 列举了三个从未存在的方法
  （`SetPictureFit` / `PictureFit` / `PictureSource`）；`doc.go` 仍称"根包"且只列六子命令
  （实为九）；`SDKVersion` 的 ldflags 路径仍是 v1 的模块根。

## Changed

- ⚠️ **`Presentation.Close()` 改为幂等**：此前二次调用返回 `ErrClosed`，与 `defer p.Close()`
  惯用法冲突（显式 Close 后再 defer，会拿到一个无法区分"真失败 / 已关闭"的错误）。现在重复
  调用一律返回 `nil`；**Close 之后的业务方法仍然返回 `ErrClosed`**，句柄失效的可观测性未削弱。
  这是一次语义放宽，不影响任何合理调用方；唯一的适配点是此前依赖"二次 Close 返回错误"的断言
  （`pptx/presentation_test.go:TestErrClosedSemantics` 已同步）。
- ⚠️ **`opc.Budget` 移除 `MaxXMLDepth` 字段**：该字段全仓只有定义 / normalize / 自身测试，
  **没有任何解析器消费**——调用方设置了以为深度受控，实际没有（典型"假能力"）。真实深度限制
  在 `internal/xmlstore`（256）。此字段此前随 `internal/opc` 无法被外部引用，故移除不构成
  对外的破坏性变更。

## Verification

```bash
go build ./...                                   # PASS
go vet ./...                                     # 零警告
gofmt -l .                                       # 零输出
go test ./...                                    # 28 包 PASS
go test -tags=corpus ./...                       # 28 包 PASS（含 B1 金样比对）
go vet -tags=corpus ./...                        # 零警告

# 金样门禁（API 冻结面）
go test -run 'TestAPIFrozen|TestErrorSentinels|TestNoBuildConstraints' -v ./pptx/   # 7/7

# 多平台交叉构建
CGO_ENABLED=0 GOOS=js     GOARCH=wasm   go build ./...
CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm   go build ./...
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64  go build ./...
CGO_ENABLED=0 GOOS=linux  GOARCH=arm64  go build ./...

# 性能回归（修后基线）
go test -run '^$' -bench BenchmarkPerfTraverse -benchtime=6x ./pptx/
```

## Compatibility

- **v2.0.0 → v2.0.1：API-compatible（纯追加）+ source-compatible**。无签名移除、无改名、
  无返回值类型变化；新增的 3 个类型是 alias，与 `internal/opc` 的实体同一类型。
- 唯一的语义放宽是 `Close()` 幂等；**任何合理调用方都不受影响**（只可能是此前断言"二次 Close
  报错"的测试需要改）。
- 下游升级动作：**建议**。生成/处理**不可信来源** `.pptx` 的下游应尽快升级——v2.0.0 在
  图表 `@idx` 与 MP4 `ftyp` 两处存在可被单文件触发的资源耗尽路径。
- 读取兼容：既有产物读写行为不变（金样语料 B1 哈希 PASS）。

## Known Limitations

- 评审中的"超长文件 / 超长函数拆分"项**未做**——需先接入 golangci-lint 才有客观阈值，
  否则容易沦为人肉格式化。
- `Open` 系列未补 `ctx` 参数——属签名级变更，留给 minor 版本。
- Tier 2 raw 直通的命中率仍偏低，未纳入本版。
- 金样守卫存在维度盲区：`loadAPISurface` 只遍历 `*ast.GenDecl`，顶层导出**函数**的新增或删除
  不会让任何门禁失败（本次新增的 `DefaultBudget()` 即属此类）。

## Feedback

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 本版适用场景：所有下游（尤其处理不可信 `.pptx` 的服务端，以及受 `Slides()` / `AddPicture`
  性能影响的大批量生成场景）。
