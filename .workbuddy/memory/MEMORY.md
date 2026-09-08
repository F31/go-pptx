# go-pptx 项目长期约定（MEMORY.md）

## 项目定位
纯 Go（CGO_ENABLED=0、无外部运行时强依赖）PPTX 创建/编辑组件。设计基线《go-pptx 完整设计方案 V2.6 开发实施版》，实施依《go-pptx 项目实施计划》M0–M8 阶段门禁制推进。

## 工程约定（已落地）
- module path：占位 `go-pptx`（go.mod `go 1.24.0`），正式仓库建立后 `go mod edit -module` 替换，README 同步更新。
- 许可证：默认 MIT（LICENSE 已提交），正式发布前需组织确认。
- git 本地身份：go-pptx-dev <go-pptx-dev@local>（仓库级，勿外发）。
- CI 必检：`CGO_ENABLED=0` 构建 + vet + test + `GOOS=js GOARCH=wasm` 编译（V2.6 §26 P1）；race 在 ubuntu job（本机 Windows 无 gcc）。
- 错误分层：internal 包各自定义内部错误（不反向 import 根包防循环），公共 API 边界映射为根包 pptx 稳定错误码。
- 代码风格：错误用 error 不 panic；文档/状态文件用中文；commit message 关联 WP 编号；相关 md 置于 docs/。
- 目录：根包 pptx；internal/{opc,xmlstore,edit,style,textmap,geom,validate}；render/、cmd/pptx/、testdata/corpus/ 后续。
- 状态跟踪：docs/go-pptx-实施状态跟踪.md（负责人每 PR/里程碑后更新）；记忆日志按日追加。

## 待确认（阻塞/排期敏感）
1. 正式 module path 与许可证。
2. 首批客户端 PowerPoint/WPS 版本平台（登记到 testdata/corpus/README.md）。
3. 团队人力基线（计划默认 2 开发 + 0.5–1 测试/语料）。
4. M0 垂直验证需要含动画+未知扩展的真实 PPTX 语料（当前缺，最高优先收集，许可记录齐全）。
