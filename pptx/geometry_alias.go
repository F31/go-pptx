package pptx

import (
	"github.com/F31/go-pptx/v2/internal/document/geometry"
	"github.com/F31/go-pptx/v2/internal/document/model"
)

// v2.0：几何值类型定义在 internal/document/geometry，此处以 alias 暴露
// （唯一公共导入路径仍为 github.com/F31/go-pptx/v2/pptx）。

// EMU 是 OOXML 长度单位（English Metric Unit，int64）。
// 1 in = 914400 EMU，1 pt = 12700 EMU；形状坐标、尺寸均以 EMU 表示。
//
// Stable: int64 别名类型——基本类型不变即契约稳定。所有几何运算（Point /
// Rect / Quad）均以 EMU 为基本单位；EMU/EMU/Pt/Inch/Cm 等构造器与常数
// 在 v1.0 后锁死。下游代码可直接以 EMU 字面量做算术（em := EMU(12700)）。
type EMU = model.EMU

// Point 是 EMU 坐标点（y 向下为正）。
//
// Stable: 字段集（X / Y）在 v1.0 后保持稳定——下游几何运算（如两点的距离、
// 矩形包含判断、变换矩阵）均依赖此结构。仅允许追加新字段（向后兼容）。
type Point = geometry.Point

// GeometryKind 描述 spPr/a:prstGeom 或 a:custGeom 的类型。
type GeometryKind = geometry.GeometryKind

// 几何类别常量。
const (
	// GeometryPreset 是 a:prstGeom（预置几何，含 adjust 公式）。
	GeometryPreset = geometry.GeometryPreset
	// GeometryCustom 是 a:custGeom（自定义路径 + guide）。
	GeometryCustom = geometry.GeometryCustom
	// GeometryUnknown 是缺失或未识别的几何容器。
	GeometryUnknown = geometry.GeometryUnknown
)

// GeometryInfo 是形状几何（a:prstGeom / a:custGeom）的解析结果。
type GeometryInfo = geometry.GeometryInfo

// GeomAdjust 是预置几何调整值（a:gd）。
type GeomAdjust = geometry.GeomAdjust

// GeomGuide 是自定义几何导引公式（a:gd / a:guide）。
type GeomGuide = geometry.GeomGuide

// GeomPath 是 a:custGeom/a:pathLst/a:path 一条路径。
type GeomPath = geometry.GeomPath

// PathCommand 是 a:custGeom 单条命令。Kind 为命令元素 Local 名。
type PathCommand = geometry.PathCommand
