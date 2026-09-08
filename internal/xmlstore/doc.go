// Package xmlstore 实现可保留词法信息的 XML 存储层（方案 §3、§4.3、§18.1）。
//
// 目标能力：保留原始字节、元素与属性跨度、命名空间作用域、未知子树与节点
// 顺序；用 URI/local name 识别元素，同时保留前缀声明及 mc:Ignorable、
// mc:AlternateContent 等上下文。不使用正则定位 XML，不把整个文档
// Unmarshal→修改结构体→Marshal 作为模板编辑默认路径（方案 §4.3）。
//
// 当前首批交付：命名空间感知的词法扫描器（Token 与字节跨度、属性实体解码、
// 注释/CDATA/PI/DOCTYPE、闭合校验、非 UTF-8 拒绝）。节点索引树
// （NodeRecord/命名空间环境/未知子树保留/深度预算）为 XML-01 的后续任务，
// 见 docs/go-pptx-实施状态跟踪.md。
package xmlstore
