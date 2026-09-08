// Package opc 实现 OPC（Open Packaging Conventions）层：ZIP 条目、
// Part URI、Content Types、关系图与流式媒体访问（方案 §3 文档存储层之下）。
//
// 职责边界：本包不依赖任何 PresentationML 业务对象；Part 以逻辑名
// （OPC 风格 "/..."）标识，与磁盘路径、ZIP 条目名相互分离（方案 §4.1）。
//
// 当前交付：
//   - ZIP 条目索引、PartName 校验、资源预算、实际字节计数读取（OPC-01）
//   - Content Types 解析与查找（Override 优先于 Default）、关系集合
//     解析与目标解析（越根拒绝、外部模式保留、rId 唯一）、主 Part
//     发现（officeDocument 关系，非固定名称）、循环安全遍历（OPC-02）
//   - 保存计划（ChangeSet → PlannedEntry 四动作、CT 同源再生成）、
//     未变 Part 复制（B1）（SAVE-01）
//   - 原子落盘（同目录临时文件 → Close+校验 → 原子替换、失败保留旧
//     目标、WithOverwrite/WithDurability）（SAVE-02）
//
// 关系/Content Types 的增删改与主文档顺序再生成必须基于同一变更集
// （方案 §18.2）；公共 API 边界的错误映射由上层 DocumentStore 完成。
package opc
