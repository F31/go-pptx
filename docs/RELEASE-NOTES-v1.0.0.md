# go-pptx v1.0.0 Release Notes

> 2026-09-10 · T-0 草稿（待 `git tag -s v1.0.0` 后随 GitHub Release 发出）

我们很高兴发布 **go-pptx v1.0.0**——这是该项目的第一个稳定版本。M0–M8 全部里程碑收口，
**51 个 Stable API 段落**（覆盖核心对象模型入口、错误码、诊断、几何值对象、句柄 ID、
枚举、Shape 句柄、Capability Output 契约族）+ **5 个 Experimental 段落**（明确标注演化
边界）+ **102 个 API 默认类型**（v1.0 冻结已有字段，仅追加）。

## Highlights

- **完整 PPTX 读写栈**：OPC ZIP 索引 / XML 命名空间感知扫描器 / span 补丁（增量插入，
  受控命名空间，未知子树保留）/ 富文本模型 / 跨 Run 替换 / 样式解析 / 图片媒体 / 单位
  转换 / 组矩阵 / 表格合并样式 / 格式深度子集 / 媒体探测 / 音频嵌入 / 配音播放 /
  受限三类图表 / 同文档受限页面复制 / 过渡动画 / 视频形状 / 文本高级 / 图表扩展 /
  版式诊断 / 主题样式矩阵 / 几何 R 档 / capability manifest / 浏览器原生检查工具 /
  动画时序只读 IR / 模板数据绑定 / 语义 diff / 跨文档受限复制 / STALE-GUARD 句柄失
  效修复。

- **三级 API 稳定性体系**（[ADR-015](adr/ADR-015-api-stability-tiers.md)）：
  - **Stable** 51 段落——v1.0 后承诺向后兼容，仅追加新方法/字段
  - **API 默认** 102 类型——v1.0 后冻结已有字段，仅追加新字段
  - **Experimental** 5 段落——1.x 内可能改，godoc 段注明演化边界

- **跨平台**：纯 Go（`CGO_ENABLED=0`），CI 验证 `js/wasm` / `darwin/arm64` /
  `wasip1/wasm` / `linux/arm64` 四档构建。

- **公开样本语料入 CI 守门**：`s001-text` / `s002-table` / `s003-image` 三份
  LibreOffice 生成公开样本自动 replay；本地 helper `scripts/run_corpus_tests.sh`
  与 CI 同语义。详见 [`corpus-入库指南.md`](corpus-入库指南.md)。

- **CI 三 OS + WASM + 跨平台 + 公开语料 replay** 守门完整——`lint` /
  `corpus-replay` / `cross-build` 三个 job 覆盖日常 PR。

## Stable API（51 types / 35 sections）

### 核心入口（6）

- `Presentation` / `Slide` / `Shape` —— 文档级对象模型入口
- `TextFrame` / `Paragraph` / `TextRun` —— 文本操作三层入口（段落 / Run 受控句柄）

### 错误-Capability 契约族（26）

- **错误码 17 哨兵**（`errors.go` 顶部共享 Stable 段落，append-only）：
  `ErrClosed` / `ErrStaleHandle` / `ErrInvalidArgument` / `ErrOutOfRange` /
  `ErrNotFound` / `ErrForeignReference` / `ErrUnsupportedFormat` /
  `ErrUnsupportedEdit` / `ErrLimitExceeded` / `ErrMalformedPackage` /
  `ErrUnresolvedStyle` / `ErrValidationFailed` / `ErrTimingConflict` /
  `ErrDurationUnknown` / `ErrConcurrentModification` / `ErrOutputExists` /
  `ErrAtomicReplaceUnavailable`

- **`OperationError`** —— 结构字段 `Op` / `Part` / `NodePath` / `SlideID` /
  `ShapeID` / `Message` / `Err` + `Unwrap` / `Error()` 契约锁定

- **诊断契约 4**：`Diagnostic` / `Severity` / `ValidationReport` /
  `CapabilityStatus`

- **Capability Output 契约族 4**：`CapabilityManifest` /
  `CapabilityManifestSource` / `CapabilityDimension` / `CapabilityFeature`
  ——JSON 标签锁死 + `CapabilityManifestSchemaVersion` 大版本号轴双重稳定边界

### 值对象 / ID / 枚举（9）

- **几何值对象 4**：`EMU` / `Point` / `Rect` / `Quad`
- **句柄 ID 类型 2**：`SlideID` / `ShapeID`（u32 + 文档语义）
- **枚举 3**：`ReplaceMode` / `MultiCellTextPolicy` / `ShapeKind`

### Shape 句柄族（8 + 1 别名）

- `GroupShape` / `AutoShape` / `OpaqueShape` / `PictureShape` / `TableShape` /
  `ChartShape` / `AudioShape` / `VideoShape`
- `TextShape` 别名（与 `AutoShape` 共享 Stable 语义）

详细稳定契约见 [`v1.0-freeze-list.md`](v1.0-freeze-list.md)。

## Experimental API（5 types，1.x 内可能改）

| 类型 | 文件 | 演化边界 | 1.x 升 Stable 路径 |
|---|---|---|---|
| `ChartWorkbookBuilder` | `chartbook.go` | 适配接口，1.0 内可能新增 `Close()` / `Validate()` 等方法 | 视第三方扩展面调查决定 |
| `ChartDataBook` | `chartbook.go` | 数据快照结构，可能随 `WorkbookBuilder` 接口扩展而加字段 | 视 `WorkbookBuilder` 演化而定 |
| `DefaultWorkbookBuilder` | `chartbook.go` | 默认实现（受限最小，无公式 / 命名范围），后续可能扩字段 | 1.0 后扩字段不影响用户代码（不依赖的字段） |
| `CustomPropertyKind` | `docProps.go` | OOXML 变体枚举，可能新增 iota（`lpwstr` / `bstr` / `filetime` 等） | 1.x 增 iota 不破坏兼容性 |
| `CustomPropertyValue` | `docProps.go` | 多字段 union，未来可能重构为按 `Kind` 类型分流 | 1.x 重构时需 deprecate 与迁移路径 |

godoc 段落已注明演化边界与未来升档时间窗；详情见
[`v1.0-freeze-list.md` §B](v1.0-freeze-list.md)。

## 已知限制（v1.0.0）

- **L3 客户端矩阵未验证**：本机无 PowerPoint/WPS 真机环境，未跑"打开无修复提示 +
  编辑后重存"端到端验证。按 V2.6 §26 P1 发布级硬缺口，单独跟踪。
- **测试覆盖率 86.6%**（`cmd/pptx`）/ 77.1%（根包）：低于 V2.6 §15.3 90% 门槛
  但**非硬性要求**——按 ADR-015 §4 覆盖率门槛待 1.x 收敛。
- **公开样本语料 3 份**：LibreOffice 生成（可再分发）；私有 `ext-*` 33 份仅索引
  未入库（WPS 源 / 受限许可）。

## 安装

```bash
go get github.com/F31/go-pptx@v1.0.0
```

## 升级指引

v1.0.0 是首个稳定版本，无破坏性变更路径：

1. **from pre-1.0**：任何 `go-pptx < v1.0.0` 升级到 v1.0.0，按 ADR-015 §"API 默认"
   承诺增量扩展的字段保持兼容；破坏性变更（如有）会在 `CHANGELOG.md` 段标注
   `BREAKING`。
2. **依赖 ADR-015 三级体系**：
   - 业务代码可直接信赖 Stable 段落（51 个）的承诺——不需版本适配
   - API 默认（102 个）按需检查 godoc 段
   - Experimental（5 个）明确标注演化边界，建议业务代码若有依赖加
     deprecation wrapper

## 工具 CLI（`cmd/pptx`）

- `pptx capability` —— 6 维 capability manifest 输出（`Inspect` / `Create` /
  `Edit` / `Preserve` / `Render` / `Play`）
- `pptx inspect` —— 只读 R 档报告（几何 / 填充 / 效果 / 样式矩阵 / 版式信息）
- `pptx diff` —— 语义 diff（`go-pptx.diff/1.0` schema）
- `pptx bind` —— 模板数据绑定（`{{path}}` / `{{#if}}` / `{{#each}}`）
- `pptx validate` —— 诊断报告（`ValidateOption` 调级别）
- `pptx check`（WASM）—— 浏览器原生 capability / inspect / validate（隐私优先）

## 文档

- [`v1.0-freeze-list.md`](v1.0-freeze-list.md) —— 完整冻结清单 + 5 阶段评审执行进度
- [`adr/ADR-014-root-internal-package-strategy.md`](adr/ADR-014-root-internal-package-strategy.md)
  —— 内部包拆分策略
- [`adr/ADR-015-api-stability-tiers.md`](adr/ADR-015-api-stability-tiers.md)
  —— 三级稳定性模型
- [`corpus-入库指南.md`](corpus-入库指南.md) —— 公开样本语料入库指南
- [`../CHANGELOG.md`](../CHANGELOG.md) —— 完整变更记录
- [`go-pptx-实施状态跟踪.md`](go-pptx-实施状态跟踪.md) —— 项目实施状态跟踪
- [`M8-里程碑总结.md`](M8-里程碑总结.md) —— M8 里程碑总结
- [`go-pptx_完整设计方案_V2_6_开发实施版.md`](go-pptx_完整设计方案_V2_6_开发实施版.md)
  —— V2.6 设计方案

## 反馈

- GitHub Issues: https://github.com/F31/go-pptx/issues
- 模板数据绑定、`pptx check`（WASM）、capability manifest 等实验性功能欢迎反馈
- L3 客户端矩阵缺口需要 PowerPoint/WPS 真机接入——如果您有真机环境，欢迎贡献
  冒烟脚本

## Acknowledgments

本版本基于以下里程碑全部收口：

- **M0** —— 建仓首批（CI 三 OS + WASM）+ OPC ZIP 索引 + XML 扫描器 / 索引 / 补丁 + 垂直验证
- **M1** —— 关系图 / 内容类型 / 保存计划 / 原子保存 / Presentation 骨架
- **M2** —— 富文本模型 + 跨 Run 替换 + 样式解析 + 图片媒体
- **M3** —— 单位 / 组矩阵 + 表格合并样式 + 格式深度子集
- **M4** —— 媒体探测 + 音频嵌入 + 配音播放
- **M5** —— 受限三类图表 + 同文档受限页面复制
- **M6** —— 过渡动画 + 视频形状 + 文本高级 + 图表扩展 + 版式诊断 + 主题样式矩阵 + 几何 R 档
- **M7** —— capability manifest + 浏览器原生检查工具 + 动画时序只读 IR + 模板数据绑定
- **M8** —— 语义 diff + 跨文档受限复制 + STALE-GUARD 句柄失效修复