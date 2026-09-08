// Package pptx 是 go-pptx 组件的公共入口（module 根包，方案 §3）。
//
// 本包提供 Presentation → Slide → Shape → TextFrame → Paragraph → Run
// 的对象模型，目标是提供类似 python-pptx 的使用体验，并满足模板报告生成、
// 已有文档精细编辑与带配音 PPT 合成三类场景（方案 §1）。
//
// 已交付（M0+M1）：稳定错误码/ID/诊断类型（§12.1、§20.4）；
// Presentation 骨架（MODEL-01）——New/Open/OpenReader/Save/Write/
// Close/Validate、Slide 受控句柄、ErrClosed/ErrStaleHandle 语义、
// revision 事务骨架（隐式事务 stage/commit、保存计划快照与
// ErrConcurrentModification 守卫）、库内最小合法模板。
// 已交付（M2 文本模板 MVP）：富文本与备注（TEXT-01）、跨 Run 替换
// （TEXT-02）、有效样式解析（STYLE-01）、图片与媒体（IMAGE-01）、
// 文档级元数据（docProps 5.1）、页面 API（Slide(index)/Slides/Layouts/
// AddSlide/MoveSlide/RemoveSlide/Shapes/Placeholders，§20.1）与
// 无障碍替代文本（AltText 8.1，PictureShape/AutoShape）。
// 已交付（M3）：单位与几何（GEOM-01，§5.2/§8）——EMU 单位换算、
// Point/Rect/Quad 值类型、3×3 仿射矩阵（列向量）、组映射
// Mgroup=T(C)·R·F·T(-C)·G 与嵌套组左乘、Shape 的 Bounds/WorldQuad/
// WorldAABB、GroupShape.Children；表格（TABLE-01，§9.1）——TableShape
// 与逻辑网格、富文本 Cell、Merge/Unmerge（AT-09 多非空默认拒绝）、
// 行高列宽、样式子集与 EffectiveCellStyle。
// 待真实语料后的 M2 闭环一验收与格式深度子集等 M3 后续工作包。
// 设计基线：《go-pptx 完整设计方案 V2.6 开发实施版》。
package pptx
