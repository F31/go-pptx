# go-pptx 实施状态跟踪

> 按《go-pptx 项目实施计划》§12 执行顺序滚动更新；工作包完成标准见计划 §5.2。
> 更新时机：每次 PR 合并 / 里程碑评审后由负责人更新本表。

## 当前阶段：M0 技术验证（执行中）

### 最近更新

| 日期 | 进展 | 备注 |
|---|---|---|
| 2026-09-08 | CORE-01 建仓首批：go.mod（占位 module `go-pptx`，go 1.24.0）、LICENSE（MIT 默认）、.gitignore、README、CI（三 OS × 1.24/1.27 + WASM 编译 job + race） | 正式 module path/许可待组织确认后替换 |
| 2026-09-08 | CORE-02：module path 正式化 `github.com/F31/go-pptx`；许可证切换为 Apache-2.0（与远程仓库 LICENSE 字节一致）；关联远程 origin `https://github.com/F31/go-pptx.git` | 远程 Initial commit（仅 LICENSE）已并入本地历史 |
| 2026-09-08 | 根包 pptx 落地：稳定错误码全集（§20.4）、SlideID/ShapeID、Diagnostic/ValidationReport | errors.go/ids.go/diagnostics.go |
| 2026-09-08 | OPC-01 首批：`internal/opc` ZIP 条目索引（Scan）、PartName 校验、Budget 预算、实际字节计数读取（countedReadCloser）+ 测试 | 路径/重复/超限/大小写重复/恶意名用例 |
| 2026-09-08 | XML-01 首批：`internal/xmlstore` 命名空间感知词法扫描器（span/属性实体解码/注释/CDATA/PI/DOCTYPE/闭合校验/非 UTF-8 拒绝）+ 测试 | 节点索引树为后续任务 |
| 2026-09-08 | XML-01 节点索引树：`XMLDocument`/`NodeRecord`（Source/OpenEnd/CloseStart 跨度、属性记录、子元素序）、`NamespaceScope` 前缀解析+默认 ns+遮蔽/回退、mc/高频 ns URI 常量、未知子树与任意 ns 一视同仁建树、`Elements`/`Attr` 查询、深度预算（ErrDepthLimit，默认 256） | index.go/namespace.go；覆盖率 83.8%；等价前缀异写同 URI 识别用例 |
| 2026-09-08 | 语料清单模板：`testdata/corpus/README.md`（QA-01 起步，24 条目标清单） | 待落实样本与许可 |

### M0 剩余任务（按实施计划 §6）

- [x] OPC-01 收口：真实 PPTX 样本冒烟（语料到位后）
- [x] XML-01 节点索引树：NodeRecord / 命名空间环境 / 未知子树保留 / 深度预算（MaxXMLDepth）
- [ ] XML-02：文本/属性补丁与结构插入（补丁冲突检测、转义、区间降序）
- [ ] M0 垂直验证程序：含动画与未知扩展样本 → 只改一个 Run → 保存 → B1 哈希 + 同节点未知区字节不变断言（**最高优先，失败即回方案调整保存策略**）
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
| XML-02 | 文本/属性补丁与结构插入 | M0 | 未开始 |
| OPC-02 | 关系图、Content Types、主 Part 发现 | M1 | 未开始 |
| SAVE-01 | 保存计划、未变 Part 复制 | M1 | 未开始 |
| SAVE-02 | 原子保存、失败恢复、输出检查 | M1 | 未开始 |
| MODEL-01 | 页面/形状/句柄/惰性读取 | M1 | 未开始 |
| TEXT-01 | 段落/Run、属性 patch、备注 | M2 | 未开始 |
| TEXT-02 | 跨 Run 替换与整批变更 | M2 | 未开始 |
| STYLE-01 | 基础占位符与样式解析 | M2 | 未开始 |
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
