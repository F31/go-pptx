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

## 当前状态（M2 已收口，M3 推进中）

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
- [x] docProps(5.1)：CoreProperties/CustomProperties 读写（Optional patch 语义、Modified 保存时自动更新、core/custom.xml 缺失按需建 Part/CT/根关系、lpwstr/i4/bool/filetime 四变体）
- [x] 页面 API 收口（M2 收口）：Slide(index)/Slides()（读视图）、Layouts/LayoutRef（绑定文档，跨文档 AddSlide 返回 ErrForeignReference）、AddSlide（新建 slide Part+rId+sldId 注册、最小空闲 id、含自闭合/缺失 sldIdLst 展开）、MoveSlide（index=最终位置语义、整元素字节搬移保真）、RemoveSlide（连带 notesSlide、未知依赖 ErrUnsupportedEdit 阻止、notesMaster 保留）
- [x] 形状枚举与 AltText(8.1)（M2 收口）：Slide.Shapes()/Placeholders()（z-order 枚举，nvGrpSpPr/grpSpPr 跳过；Shape 公共面 ID/Name/Kind/AltText/IsDecorative）、AutoShape 句柄（TextBox/AutoShape 判别、TextFrame 读写、占位符 Type/Index 规范化 obj/0）、OpaqueShape 只读回退、AutoShape/PictureShape §8.1 读写（装饰标记与空串语义互斥区分，共用 shapeNode 基元）、保存往返保真
- [x] M2 代码项全部收口（QA-01 语料冒烟与真实客户端验证待语料/环境到位）

## 当前状态（M3 格式与表格，GEOM-01/TABLE-01/格式深度子集已完成）

- [x] GEOM-01：EMU 单位与换算（舍入+溢出检查）、Point/Rect/Quad 几何值类型、3×3 仿射矩阵（列向量）、xfrm 解析（负坐标合法、rot=1/60000 度顺时针、flipH/flipV）、组映射 Mgroup=T(C)·R·F·T(-C)·G（非等比缩放 G=T(off)·S·T(-chOff)、chExt 零拒绝除法）、嵌套组父矩阵左乘、Shape 接口 Bounds/WorldQuad/WorldAABB（本地框=直接父坐标 off/ext；WorldQuad=页面坐标四角）、GroupShape 正式句柄（grpSp 从 OpaqueShape 升级，Children() 组内 z-order 枚举+嵌套递归）
- [x] TABLE-01：TableShape 句柄（含 a:tbl 的图形框）与逻辑网格（gridSpan/rowSpan + hMerge/vMerge continuation 映射）、富文本 Cell（复用 TextFrame）、Merge/Unmerge（矩形与跨边界校验、**AT-09 多非空单元格默认拒绝**、可明确保留锚点文本）、RowHeight/ColumnWidth 读写、样式子集（三态区域开关 + 12 个 band/first/last 优先级矩阵 + tableStyles.xml 解析 + EffectiveCellStyle 逐属性状态，未知样式 ID → unresolved）
- [x] 格式深度子集：颜色变换全集（19 种：lumMod/lumOff/shade/tint/{red,green,blue}{Mod,Off}/satMod/satOff/hueMod/hueOff/alpha/alphaMod/alphaOff/inv/gray/comp，整数除法 val/100000）、线条系统（a:ln 全属性：w/cap/cmpd/algn/prstDash/custDash/round/bevel/miter/headEnd/tailEnd+颜色+Unknown 子元素）、段落属性全集（a:pPr 全属性+lnSpc/spcBef/spcAft+tabLst/buChar/buAutoNum/buBlip/buFont/buSzPct/buSzPts）、Run 高级属性（a:rPr baseline/spc/cap/strike/u/lang/altLang/kern/highlight/sym 等）、主题样式矩阵引用链（fillRef/lnRef/effectRef→themeMatrixEntry 沿 fmtScheme 定位）、Shape 接口扩展 Line/StyleMatrixRefs；起步解析 R 档，未知项→Unknown 字段或诊断
- [x] M4 配音功能包：MEDIA-01 媒体探测（WAV chunk 遍历/MP3 ID3+Xing/VBRI VBR/三级时长来源）；AUDIO-01 嵌入与 AudioProfile（/docProps/audio.xml 自有扩展、SHA-256 去重、未知时长拒绝）；AUDIO-02 受限播放树（纯音频 p:timing 幂等重建、复杂树 ErrTimingConflict、SetAdvanceAfter）；AUDIO-03 UpsertNarration 幂等三态 + PlanTimingSync/ApplyTimingPlan（Ej 公式、revision 校验、未知时长策略）
- [ ] M3 后续（QA-01 语料冒烟与真实客户端验证待语料/环境到位）

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
