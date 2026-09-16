// Package geometry 定义几何领域的纯值类型与只读解析（度量单位、prstGeom /
// custGeom 路径）。本包不依赖根包——写入侧仍由根包 / xmlstore 承担。
package geometry

import (
	"github.com/F31/go-pptx/internal/diag"
	"github.com/F31/go-pptx/internal/document/model"
)

// EMU 是共享度量单位，定义在 internal/document/model。
type EMU = model.EMU

// Point 是 EMU 坐标点（y 向下为正）。
type Point struct {
	X EMU
	Y EMU
}

// GeometryKind 描述 spPr/a:prstGeom 或 a:custGeom 的类型。
type GeometryKind int

const (
	// GeometryPreset 是 a:prstGeom（预置几何，含 adjust 公式）。
	GeometryPreset GeometryKind = iota
	// GeometryCustom 是 a:custGeom（自定义路径 + guide）。
	GeometryCustom
	// GeometryUnknown 是缺失或未识别的几何容器。
	GeometryUnknown
)

func (k GeometryKind) String() string {
	switch k {
	case GeometryPreset:
		return "preset"
	case GeometryCustom:
		return "custom"
	}
	return "unknown"
}

// GeometryInfo 是形状几何（a:prstGeom / a:custGeom）的解析结果。
// 零值即"未声明几何"——大多数容器（p:grpSp / 未知 OpaqueShape）适用。
type GeometryInfo struct {
	// Kind 是几何类别（prstGeom / custGeom / 未知）。
	Kind GeometryKind
	// Preset 是 a:prstGeom@prst 的预设名（如 "rect"、"ellipse"、"roundRect"）。
	// Kind=GeometryPreset 时有效；custom/unknown 时为空。
	Preset string
	// Adjusts 是 a:prstGeom/a:gdLst 内所有 a:gd 的 name→val（千分比/原始）。
	// 仅 prstGeom 有效；custGeom 不使用。
	Adjusts []GeomAdjust
	// Guides 是 a:custGeom/a:gdLst 内 a:gd 的全集（公式文本）。
	// 仅 custGeom 有效。
	Guides []GeomGuide
	// Paths 是 a:custGeom/a:pathLst 内每条 a:path 的命令序列。
	// 仅 custGeom 有效。
	Paths []GeomPath
	// Unknown 是未识别的子元素 Local 名（按文档序）。
	Unknown []string
	// Diagnostics 是解析期产生的诊断条目。
	Diagnostics []diag.Diagnostic
}

// GeomAdjust 是预置几何调整值（a:gd）。
type GeomAdjust struct {
	// Name 是 a:gd@name（如 "adj"、"adj1"、"adj2"）。
	Name string
	// Fmla 是 a:gd@fmla 的原始公式文本（如 "val 25000"）。
	Fmla string
}

// GeomGuide 是自定义几何导引公式（a:gd / a:guide）。
type GeomGuide struct {
	// Name 是 a:gd@name（如 "myGuide"）。
	Name string
	// Fmla 是 a:gd@fmla 公式文本。
	Fmla string
}

// GeomPath 是 a:custGeom/a:pathLst/a:path 一条路径。
type GeomPath struct {
	// Width / Height 是路径所属虚拟框尺寸（EMU）。
	Width, Height EMU
	// Fill 是路径的填充模式（"none" / "solid" / "norm" / "lighten" / ...）；
	// 来自 a:path@fill。
	Fill string
	// Stroke 是路径描边模式（"none" / "solid" / ...）。
	Stroke string
	// Commands 是按文档序的全部命令（a:moveTo/a:lnTo/a:cubicBezTo/...）。
	Commands []PathCommand
	// Unknown 是路径下未识别子元素名。
	Unknown []string
}

// PathCommand 是 a:custGeom 单条命令。Kind 为命令元素 Local 名。
type PathCommand struct {
	// Kind 是命令元素 Local 名（moveTo / lnTo / arcTo / cubicBezTo /
	// quadBezTo / close）。
	Kind string
	// Points 是该命令坐标序列（EMU）。moveTo/lnTo/arcTo 各 1 个；
	// cubicBezTo 3 个（控制点 1、控制点 2、终点）；quadBezTo 2 个。
	Points []Point
}
