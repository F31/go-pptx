// Package pptx 是 go-pptx 组件的公共入口（module 根包，方案 §3）。
//
// 本包提供 Presentation → Slide → Shape → TextFrame → Paragraph → Run
// 的对象模型，目标是提供类似 python-pptx 的使用体验，并满足模板报告生成、
// 已有文档精细编辑与带配音 PPT 合成三类场景（方案 §1）。
//
// 当前处于 M0 起步阶段：本包先落地稳定的错误码、ID 与诊断类型
// （方案 §12.1、§20.4），对象模型随 OPC/XML 底座工作包逐步开放。
// 设计基线：《go-pptx 完整设计方案 V2.6 开发实施版》。
package pptx
