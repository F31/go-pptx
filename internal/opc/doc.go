// Package opc 实现 OPC（Open Packaging Conventions）层：ZIP 条目、
// Part URI、Content Types、关系图与流式媒体访问（方案 §3 文档存储层之下）。
//
// 职责边界：本包不依赖任何 PresentationML 业务对象；Part 以逻辑名
// （OPC 风格 "/..."）标识，与磁盘路径、ZIP 条目名相互分离（方案 §4.1）。
// 关系图 / Content Types / 主 Part 发现属 OPC-02，不在本包当前首批范围。
package opc
