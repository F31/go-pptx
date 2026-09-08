# go-pptx 实施状态跟踪

> 按《go-pptx 项目实施计划》§12 执行顺序滚动更新；工作包完成标准见计划 §5.2。
> 更新时机：每次 PR 合并 / 里程碑评审后由负责人更新本表。

## 当前阶段：M2 富文本与基础样式（执行中，TEXT-01 完成）

### 最近更新

| 日期 | 进展 | 备注 |
|---|---|---|
| 2026-09-08 | CORE-01 建仓首批：go.mod（占位 module `go-pptx`，go 1.24.0）、LICENSE（MIT 默认）、.gitignore、README、CI（三 OS × 1.24/1.27 + WASM 编译 job + race） | 正式 module path/许可待组织确认后替换 |
| 2026-09-08 | CORE-02：module path 正式化 `github.com/F31/go-pptx`；许可证切换为 Apache-2.0（与远程仓库 LICENSE 字节一致）；关联远程 origin `https://github.com/F31/go-pptx.git` | 远程 Initial commit（仅 LICENSE）已并入本地历史 |
| 2026-09-08 | 根包 pptx 落地：稳定错误码全集（§20.4）、SlideID/ShapeID、Diagnostic/ValidationReport | errors.go/ids.go/diagnostics.go |
| 2026-09-08 | OPC-01 首批：`internal/opc` ZIP 条目索引（Scan）、PartName 校验、Budget 预算、实际字节计数读取（countedReadCloser）+ 测试 | 路径/重复/超限/大小写重复/恶意名用例 |
| 2026-09-08 | XML-01 首批：`internal/xmlstore` 命名空间感知词法扫描器（span/属性实体解码/注释/CDATA/PI/DOCTYPE/闭合校验/非 UTF-8 拒绝）+ 测试 | 节点索引树为后续任务 |
| 2026-09-08 | XML-01 节点索引树：`XMLDocument`/`NodeRecord`（Source/OpenEnd/CloseStart 跨度、属性记录、子元素序）、`NamespaceScope` 前缀解析+默认 ns+遮蔽/回退、mc/高频 ns URI 常量、未知子树与任意 ns 一视同仁建树、`Elements`/`Attr` 查询、深度预算（ErrDepthLimit，默认 256） | index.go/namespace.go；覆盖率 83.8%；等价前缀异写同 URI 识别用例 |
| 2026-09-08 | XML-02 补丁与受控插入：`SpanPatch` 区间替换（统一校验+升序重建、重叠/范围/锚定冲突检测 ErrPatch*）、`EscapeText`/`EscapeAttrValue`（XML 1.0 非法字符显式拒绝、空白属性逐字保留）、`SetAttrValuePatch`/`NewTextPatch`、`InsertBefore/After/AppendChild`（片段单根 well-formed + 前缀自足或插入点作用域可解析，否则 ErrFragmentNamespace；自闭合 AppendChild 拒绝） | patch.go/insert.go；覆盖率 86.2%；含垂直验证单元级雏形 TestPatchPreservesUntouchedRegions（改一个 Run，动画分支/未知子树字节逐字不变） |
| 2026-09-08 | 语料清单模板：`testdata/corpus/README.md`（QA-01 起步，24 条目标清单） | 待落实样本与许可 |
| 2026-09-08 | OPC-02 关系图/Content Types/主 Part 发现（M1 首项）：`ParseContentTypes`（Override 精确→大小写兜底→Default；重复/缺属性拒绝）、`ParseRelationships`（rId 唯一、目标相对源 Part 解析、越出包根拒绝、TargetMode 缺省 Internal + 绝对 URI 自动 External、百分号解码）、`Package.Load`（装配 + 全部 .rels 解析 + 双源冲突检测）、`MainPart`（officeDocument 关系、非固定名称）、`RelatedParts`/`RelatedByIDs`/`Walk`（visited 防循环） | package.go/relationships.go/contenttypes.go；非固定名称/循环关系/Override 金样三项验收均过 |
| 2026-09-08 | MODEL-01 公共 API 骨架：`Presentation`（New/Open/OpenReader/Save/Write/Close/Validate）、`Slide` 受控句柄、ErrClosed/ErrStaleHandle 语义、revision 事务骨架（stagePatch/stageDelete/commit、保存计划 revision 快照 + ErrConcurrentModification 守卫）、库内最小合法模板（presentation+master+layout+theme+docProps，原创生成无第三方素材）、原位保存拒绝（同路径/同文件实体） | presentation.go/slide.go/template.go；根包覆盖率 70.7%；M1 代码项全部完成 |
| 2026-09-08 | SAVE-01 保存计划与未变 Part 复制：`BuildSavePlan`（ChangeSet{Patched/Added/Deleted} → 唯一 PlannedEntry 清单；交叉冲突/存在性/名称校验）、CT 与变更集同源再生成（确定性序列化）、删除自动连带关系流、悬空关系 OPC_DANGLING_REL 诊断、`SavePlan.Write`（CopyOriginal 复制源解压内容，条目按名排序）；**B1 哈希回归全绿**（空变更集逐 Part 哈希一致 = AT-01 等价、补丁保存仅目标 Part 变化、增删后其余 Part 一致） | saveplan.go + contenttypes.go 写侧；opc 覆盖率 86.2%；原子落盘/失败恢复属 SAVE-02 |
| 2026-09-08 | SAVE-02 原子保存与失败恢复：`SavePlan.SaveToFile`（目标检查 → 同目录临时文件 → 写入 → fsync(可选) → Close → 输出校验（ZIP 结构+条目集一致）→ 原子替换（POSIX rename / Windows MoveFileEx REPLACE_EXISTING）→ 目录 fsync(可选)）；默认禁覆盖（ErrOutputExists），`WithOverwrite(true)` 显式启用；`WithDurability(DurabilityFull)` 文件+目录 fsync；替换失败保留旧目标 ErrAtomicReplaceUnavailable，**绝不删旧再写**；提交前失败清理临时文件（清理失败 errors.Join 附加不掩盖主错误）；writer 故障注入（zip.Writer 全程缓冲、Close 单次落盘） | save.go；**AT-11 等价用例全绿**（写入失败/目录不可写/默认禁覆盖三态旧目标不变、零临时残留）；opc 覆盖率 84.0% |
| 2026-09-08 | TEXT-01 富文本模型与备注（M2 首项）：DocumentStore 扩展（addedParts/deletedParts/partDocs 三表、`docOf` 泛型替代仅主 Part 视图、commit 合并三类变更整体失效缓存）、`Optional[T]`/`FontStyle`/`ColorSpec` 字体模型、TextFrame/Paragraph/TextRun 三级受控句柄（nodeStep 稳定路径定位）、`SetPlainText`（结构替换保 bodyPr/lstStyle、未知扩展拒绝）、`SetText`/`AddRun`、`SetFont`/`ResetFontProperty`（rPr 自闭合展开、属性/子元素精确补丁、空 rPr 自动移除）、备注四 API（SpeakerNotesText/SpeakerNotes/EnsureSpeakerNotes/SetSpeakerNotes，notesMaster 依赖真实关系解析）、Save/Write/Validate nil ctx 兜底 | presentation.go 扩展 + font.go/text.go/notes.go/text_test.go；**修复 xmlstore 属性删除边界 bug（AttributeRecord 补 NameStart/NameEnd，删除含闭合引号）与 pathToRun 缺 r 步骤**；根包覆盖率 49.1%；WASM/Linux 交叉构建通过 |
| 2026-09-08 | TEXT-02 跨 Run 替换与整批变更（M2 第二项）：`Paragraph.ReplaceText`（大小写敏感/字面/左到右非重叠/初始快照/不递归；空 old 拒绝；replacement 含换行拒绝）、三种格式策略 `ReplaceMode`（FirstCharacter 默认继承命中首字符 Run 格式；EqualLengthPerRune 逐 Run 等长文本替换不拆 Run；ExplicitStyle + WithReplacementStyle）、逻辑文本视图与 Text() 一致（rune 索引）、块边界（br/fld 硬边界 + 不同链接/动作 key 不可跨越）、字素簇保护（组合字符/ZWJ 序列边界拒绝）、安全保真（重建/删除 Run 前检查未知直接子元素与链接；unsafe/相邻共享 Run 冲突记 skipped 诊断）、`ReplaceResult`（Matches/Replaced/Skipped/Hits 段落 rune 定位）、`TextFrame.ReplaceText` 遍历段落；**占用 Run 区间贪心调度 + 同 Run 多命中合并单补丁 + 一次事务批量提交** | replace.go/replace_test.go；修复 `locateBlockSpan` 终点判定（gei<=hi）、`OperationError.Error()` 遗漏 Message 字段；根包覆盖率 49.1%→57.3%；vet/全仓测试/WASM/js/Linux 交叉构建通过 |
| 2026-09-08 | STYLE-01 基础占位符与样式解析（M2 第三项）：`TextRun.EffectiveFont(ctx ResolveContext)`（方案 §6.1 契约）逐属性返回 `ResolvedValue/ResolvedColor`（Value/Resolved/Fallback/Trace 三要素 + 来源链 StyleStep）；解析链 L1 run rPr → L2 pPr defRPr → L3 列表级别样式（占位符: layout 匹配占位符 lstStyle → master 匹配占位符 lstStyle → master txStyles[class]；非占位符: otherStyle）→ L4 主题缺省字体（title 类 majorFont，其余 minorFont）；**占位符匹配 (type,idx) 规范化全等（type 缺省 obj、idx 缺省 0，ECMA CT_Placeholder）不按坐标/名称**；主题沿真实关系图导航（slide→layout→master→theme / notes→notesMaster→theme，**新增 p.relsOf 读视图含已提交 rels 补丁**）；颜色保留原始 ColorSpec 同时输出 RGB（srgbClr 直出 / sysClr lastClr 优先 / clrMap bg1..tx2 间接映射 / lumMod lumOff shade tint 按序应用，phClr 与未知变换 STYLE_PARTIAL）；"+" 主题字体引用展开（+mj-*/+mn-*）；无法解析 STYLE_UNRESOLVED、仅 ctx.Fallback 采用并标 Fallback、ctx.Strict → ErrUnresolvedStyle | style.go/style_test.go（完整链 fixture：title/body idx1/body idx2/普通文本框 × lvl0/lvl2/run rPr 覆盖/notes 冒烟）；**修复 notesPartOf 改读视图致 createNotes 后同会话可再读备注**；根包覆盖率 57.3%→73.1%；vet/全仓测试/WASM(js/wasip1)/Linux 交叉构建通过 |

### M0 剩余任务（按实施计划 §6）

- [x] OPC-01 收口：真实 PPTX 样本冒烟（语料到位后）
- [x] XML-01 节点索引树：NodeRecord / 命名空间环境 / 未知子树保留 / 深度预算（MaxXMLDepth）
- [x] XML-02：文本/属性补丁与结构插入（补丁冲突检测、转义、区间降序）
- [ ] M0 垂直验证程序：含动画与未知扩展样本 → 只改一个 Run → 保存 → B1 哈希 + 同节点未知区字节不变断言（**最高优先，失败即回方案调整保存策略**；单元级雏形已就绪，待真实语料）
- [ ] QA-01：落实 ≥20 份语料样本与许可记录、首批修改前后金样入库

### 待确认事项（阻塞性）

1. ~~正式 module path~~（已定：`github.com/F31/go-pptx`，CORE-02）
2. ~~正式许可证~~（已定：Apache-2.0，与远程仓库一致，CORE-02）
3. 首批客户端 PowerPoint / WPS 的具体版本/平台（语料 README 待登记）
4. 团队人力（计划基线：2 开发 + 0.5–1 测试/语料）

## 工作包总表（34 项，来源：实施计划附录 A）

| WP | 名称 | 阶段 | 状态 |
|---|---|---|---|
| CORE-01 | 初始化 module、CI、许可、模板资源 | M0 | 进行中（首批已提交） |
| OPC-01 | ZIP 索引、PartName、资源预算 | M0 | 进行中（首批已提交） |
| XML-01 | NS 感知解析与原始跨度 | M0 | 已完成（代码+单测；样本冒烟待语料） |
| XML-02 | 文本/属性补丁与结构插入 | M0 | 已完成（代码+单测） |
| OPC-02 | 关系图、Content Types、主 Part 发现 | M1 | 已完成（代码+单测；真实样本冒烟待语料） |
| SAVE-01 | 保存计划、未变 Part 复制 | M1 | 已完成（代码+单测，B1 回归全绿；接 DocumentStore 后回填 BaseRevision） |
| SAVE-02 | 原子保存、失败恢复、输出检查 | M1 | 已完成（代码+单测，AT-11 等价用例全绿；平台真机测试随 CI 三 OS 扩展） |
| MODEL-01 | Presentation/Slide 句柄与惰性读取、事务/revision 骨架 | M1 | 已完成（代码+单测） |
| TEXT-01 | 段落/Run、属性 patch、备注 | M2 | 已完成（代码+单测；真实样本冒烟待语料） |
| TEXT-02 | 跨 Run 替换与整批变更 | M2 | 已完成（代码+单测） |
| STYLE-01 | 基础占位符与样式解析 | M2 | 已完成（代码+单测） |
| IMAGE-01 | PNG/JPEG 与媒体暂存 | M2 | 未开始 |
| GEOM-01 | 单位、组矩阵、四角边界 | M3 | 未开始 |
| TABLE-01 | 富文本表格、合并、样式子集 | M3 | 未开始 |
| MEDIA-01 | 媒体输入、类型检查、probe | M4 | 未开始 |
| AUDIO-01/02/03 | 配音/播放树/计时计划 | M4 | 未开始 |
| CHART-01 | 受限图表与数据源适配 | M5 | 未开始 |
| CLONE-01 | 同文档受限复制 | M5 | 未开始 |
| TOOL-01 | CLI 与 IR | M5/M6 | 未开始 |
| GEOM-02/STYLE-02/LAYOUT-01/ANIM-02/VIDEO-01/CHART-02/TEXT-03 | M6 扩展七包 | M6 | 未开始 |
| CAP-01/TIMIR-01/TPL-01/TOOL-02 | M7 创新 I | M7 | 未开始 |
| DIFF-01 | M8 创新 II | M8 | 未开始 |
| QA-01 | 语料、fuzz、兼容报告 | 持续 | 进行中（模板已建） |
