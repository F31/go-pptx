# go-pptx

纯 Go、`CGO_ENABLED=0`、无外部运行时强依赖的 PPTX 创建与编辑组件（Presentation → Slide → Shape → TextFrame → Paragraph → Run 对象模型）。

- 远程仓库：`https://github.com/F31/go-pptx`
- 设计基线：《go-pptx 完整设计方案 V2.6 开发实施版》（`docs/go-pptx_完整设计方案_V2_6_开发实施版.md`）
- 实施计划：《go-pptx 项目实施计划》（`docs/go-pptx-项目实施计划.md`）
- 状态跟踪：`docs/go-pptx-实施状态跟踪.md`

## 仓库基线记录（CORE-01，建仓提交）

| 配置项 | 当前值 | 状态 |
|---|---|---|
| module path | `github.com/F31/go-pptx` | 已随 CORE-02 由占位 `go-pptx` 正式化 |
| 许可证 | Apache-2.0 | 与远程仓库 LICENSE（Initial commit）一致 |
| Go 工具链 | go.mod 声明 `go 1.24.0`（最低），本地开发 1.27.0 | CI 双版本验证 |
| 平台 | Windows 主开发；CI 覆盖 Windows/Linux/macOS | 原子替换等平台行为后续真机验证 |
| 首批客户端 | PowerPoint / WPS（具体版本待登记） | 待补充到 `testdata/corpus/README.md` |

> 模块名、正式许可、工具链与客户端版本应在仓库初始化提交中记录（方案 §27）。

## 当前状态（M2 富文本与基础样式）

- [x] CORE-01：module/目录/许可/CI 骨架（含 `GOOS=js GOARCH=wasm` 编译验证 job）
- [x] OPC-01 首批：ZIP 条目索引、PartName 校验、资源预算、实际字节计数读取（`internal/opc`）
- [x] XML-01：命名空间感知扫描器 + 节点索引树（NodeRecord/ns 环境/未知子树保留/深度预算，`internal/xmlstore`）
- [x] XML-02：文本/属性补丁与结构插入（SpanPatch 冲突检测/转义/受控插入，含垂直验证单元级雏形）
- [x] OPC-02：关系图、Content Types（Override 优先）、主 Part 发现（非固定名称）、循环安全遍历（`internal/opc`）
- [x] SAVE-01：保存计划（PlannedEntry 四动作）、未变 Part 复制、CT 同源再生成、B1 哈希回归全绿（`internal/opc`）
- [x] SAVE-02：原子落盘（临时文件→Close+校验→原子替换）、失败保留旧目标、WithOverwrite/WithDurability（`internal/opc`）
- [ ] M0 垂直验证：真实语料 B1 哈希比对（**阻塞于语料收集**）
- [x] MODEL-01：Presentation 骨架（New/Open/OpenReader/Save/Write/Close/Validate）、Slide 受控句柄、revision 事务骨架、库内最小合法模板
- [x] TEXT-01：DocumentStore 增删/文档缓存扩展、Optional/FontStyle/ColorSpec、TextFrame/Paragraph/TextRun（SetPlainText/SetText/AddRun/SetFont/ResetFontProperty）、备注四 API（SpeakerNotes*）
- [x] TEXT-02：Paragraph/TextFrame.ReplaceText（跨 Run 字面替换、三格式策略、br/fld/链接边界、字素簇保护、ReplaceResult 报告、整批单事务提交）
- [x] STYLE-01：占位符 (type,idx) 规范化匹配、EffectiveFont 样式链（Run→段落→占位符/版式/母版→主题）、每属性 Value/Resolved/SourceTrace、主题色/字体解析（clrMap、sysClr、lumMod/lumOff/shade/tint、+mj-*/+mn-*）、未决/部分解析诊断与回退/Strict 语义
- [x] IMAGE-01：MediaSource 适配器与 PNG/JPEG 探测、Slide.AddPicture 四 Fit 模式（原尺寸/拉伸/Contain/Cover-crop）、媒体内容哈希去重、ReplaceImage 共享引用保护、AltText/IsDecorative
- [ ] M2 剩余（页面 API/docProps）：按实施计划 §12 推进

详情见 `docs/go-pptx-实施状态跟踪.md`。

## 目录结构（目标形态，方案 §3）

```text
go-pptx/
  *.go                 # 公共对象层（根包 pptx）
  internal/opc/        # ZIP 条目、Part URI、Content Types、关系图、流式媒体
  internal/xmlstore/   # 原始字节、token/节点跨度、命名空间环境
  internal/edit/       # 变更集、冲突检测、事务提交（后续）
  internal/style/      # 属性继承、颜色变换、表格样式（后续）
  internal/textmap/    # 文本逻辑位置与 XML 节点映射（后续）
  internal/geom/       # 单位、矩阵、边界计算（后续）
  internal/validate/   # 结构与语义规则（后续）
  render/              # 渲染接口，适配实现按需拆分（后续）
  cmd/pptx/            # inspect、validate、replace 等便捷入口（后续）
  testdata/corpus/     # 生成器与特性双标签索引
  docs/                # API 契约、兼容矩阵、ADR
```

## 开发命令

```bash
go build ./...                  # CGO_ENABLED=0 构建（CI 强制）
go vet ./...                    # 静态检查
go test ./...                   # 单元与金样测试
GOOS=js GOARCH=wasm go build ./...   # WASM 可编译性验证（方案 V2.6 §26）
```
