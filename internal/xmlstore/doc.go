// Package xmlstore 实现可保留词法信息的 XML 存储层（方案 §3、§4.3、§18.1）。
//
// 目标能力：保留原始字节、元素与属性跨度、命名空间作用域、未知子树与节点
// 顺序；用 URI/local name 识别元素，同时保留前缀声明及 mc:Ignorable、
// mc:AlternateContent 等上下文。不使用正则定位 XML，不把整个文档
// Unmarshal→修改结构体→Marshal 作为模板编辑默认路径（方案 §4.3）。
//
// 已交付：
//   - 命名空间感知的词法扫描器：Token 与字节跨度、属性实体解码、
//     注释/CDATA/PI/DOCTYPE、闭合校验、非 UTF-8 拒绝（scanner.go）
//   - 节点索引树：XMLDocument/NodeRecord（字节跨度 Source/OpenEnd/
//     CloseStart、属性记录、子元素顺序）、NamespaceScope 前缀解析与
//     默认命名空间、mc 上下文常量、未知子树与任意 ns 元素一视同仁建树、
//     深度预算（index.go / namespace.go）
//
// 后续：XML-02 文本/属性补丁引擎基于 span 工作（见
// docs/go-pptx-实施状态跟踪.md）。
package xmlstore
