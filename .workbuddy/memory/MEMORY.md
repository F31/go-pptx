# go-pptx 项目长期约定（MEMORY.md）

## 项目定位
纯 Go（CGO_ENABLED=0、无外部运行时强依赖）PPTX 创建/编辑组件。设计基线《go-pptx 完整设计方案 V2.6 开发实施版》，实施依《go-pptx 项目实施计划》M0–M8 阶段门禁制推进。

## 工程约定（已落地）
- module path：`github.com/F31/go-pptx`（CORE-02 已由占位 `go-pptx` 正式化；go.mod `go 1.24.0`）。
- 许可证：Apache-2.0（与远程仓库 LICENSE 字节一致，CORE-02；模板资源许可另行登记）。
- 远程仓库：origin = `git@github.com:F31/go-pptx.git`（SSH 推送；本机 ed25519 已绑定 GitHub 账号 jinfeng105）。
- git 本地身份：go-pptx-dev <go-pptx-dev@local>（仓库级，勿外发）。
- CI 必检：`CGO_ENABLED=0` 构建 + vet + test + `GOOS=js GOARCH=wasm` 编译（V2.6 §26 P1）；race 在 ubuntu job（本机 Windows 无 gcc）。
- 错误分层：internal 包各自定义内部错误（不反向 import 根包防循环），公共 API 边界映射为根包 pptx 稳定错误码。
- 代码风格：错误用 error 不 panic；文档/状态文件用中文；commit message 关联 WP 编号；相关 md 置于 docs/。
- 目录：根包 pptx；internal/{opc,xmlstore,edit,style,textmap,geom,validate}；render/、cmd/pptx/、testdata/corpus/ 后续。
- 状态跟踪：docs/go-pptx-实施状态跟踪.md（负责人每 PR/里程碑后更新）；记忆日志按日追加。
- **句柄身份约定（STALE-GUARD，M8 落地）**：`shapeNode` 句柄身份 = cNvPr@id；path 仅作"在哪个 spTree/grpSp 下查找"的父容器提示。locate 先按 path[:-1] 解析出 spTree/grpSp（跨兄弟操作稳定），再在该父容器子元素中按 cNvPr@id 线性查找——找不到返回 ErrStaleHandle。MoveShape 后句柄仍有效（cNvPr@id 未变），只有 RemoveShape 让 cNvPr@id 消失时句柄才失效。文本节点（textNode）暂无等价稳定标识符，仍按 path 解析——这是"编辑后重新取句柄"建议保留的唯一场景。

## 事故记录（重要）
- 2026-09-08：git merge 触发内部 stash 失败后 `.git` 目录整体消失（工作区完好）。教训：① 本机 git 大操作（merge/rebase）前先 `cp -r .git` 备份；② merge 前务必保证 working tree clean，避免 autostash 路径。

## 待确认（阻塞/排期敏感）
1. ~~正式 module path~~（已定 github.com/F31/go-pptx）；~~许可证~~（已定 Apache-2.0）。
2. 首批客户端 PowerPoint/WPS 版本平台（登记到 testdata/corpus/README.md）。
3. 团队人力基线（计划默认 2 开发 + 0.5–1 测试/语料）。
4. M0 垂直验证需要含动画+未知扩展的真实 PPTX 语料（当前缺，最高优先收集，许可记录齐全）。
