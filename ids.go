package pptx

// SlideID 是演示文稿范围内的页面标识，取自 presentation.xml 中 sldId 的 id。
//
// 作用域与排序语义（方案 §19.2）：slide id 按规范合法区间分配并检查耗尽；
// 页面顺序由 presentation 中的列表顺序决定，不按 id 数值排序。
//
// Stable: 类型定义与文档语义（持久化到 OOXML、归属 presentation 级别、不
// 表示页面顺序）在 v1.0 后锁死；底层 uint32 表示不变。仅允许追加文档备注。
type SlideID uint32

// ShapeID 是形状树适用作用域内的形状标识，对应 OOXML ShapeID。
//
// 与库内部 NodeID（xmlstore 稳定句柄）不同，ShapeID 会写入并持久化到 OOXML。
// 计时（timing）与连接器等引用在重映射时必须同步更新（方案 §19.2）。
//
// Stable: 类型定义与文档语义（持久化到 OOXML、与 NodeID 互相独立、引用
// 重映射须同步）在 v1.0 后锁死；底层 uint32 表示不变。
type ShapeID uint32
