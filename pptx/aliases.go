package pptx

// 本文件把门面签名里用到的 internal 类型以**别名**形式暴露到公共包。
//
// 背景：pptx 的导出签名与导出结构体字段此前直接引用 internal/opc 的
// Budget / Durability / PartName。Go 的 internal 规则禁止外部模块 import
// 这些包，因此调用方**连参数类型都写不出来**——例如 WithBudget 承诺了资源
// 预算可配置，实际却无法调用（外部实测编译报 "use of internal package …
// not allowed"）。同类问题还让 AudioProfile.MediaPart、LayoutReport.Part 等
// 导出字段在库外不可读。
//
// 修法用 type alias 而非重新定义：别名与内部类型**完全同一类型**，零转换成本、
// 零语义漂移，也不产生第二套需要同步维护的定义。

import "github.com/F31/go-pptx/v2/internal/opc"

// Stable: 以下三个别名自 v2.0.x 起是公共契约的一部分；删除或改语义需走大版本。
type (
	// Budget 是处理不可信 PPTX 时的资源预算（条目数、单 Part 字节、声明总量）。
	//
	// 用法：`pptx.Open(path, pptx.WithBudget(pptx.Budget{MaxEntries: 2000}))`。
	// 任一字段 <=0 时回退到库内默认初值（0 不代表"无限"）。
	Budget = opc.Budget

	// Durability 控制保存的持久性级别（见 DurabilityDefault / DurabilityFull）。
	Durability = opc.Durability

	// PartName 是 OPC 风格的 Part 名（以 "/" 开头的包内路径，如
	// "/ppt/slides/slide1.xml"）。Valid 报告其是否合法，EntryName 返回其在
	// ZIP 中的条目名（去掉前导 "/"）。
	PartName = opc.PartName
)

// DefaultBudget 返回库内置的资源预算基线。
//
// 常见用法是**局部覆盖**：取基线后只改关心的字段，其余保持默认——
// `Budget` 的每个字段在 <=0 时都会回退到这里的值，所以零值不代表"不限制"。
//
//	b := pptx.DefaultBudget()
//	b.MaxEntries = 2000
//	p, err := pptx.Open(path, pptx.WithBudget(b))
//
// Stable: DefaultBudget 自 v2.0.x 起是公共契约的一部分。
func DefaultBudget() Budget { return opc.DefaultBudget() }

// Durability 取值（与 internal/opc 同一组常量，便于调用方不必引用内部包）。
//
// Stable: 这两个取值自 v2.0.x 起是公共契约的一部分。
const (
	// DurabilityDefault 只保证错误处理与原子可见性，不承诺断电持久。
	DurabilityDefault = opc.DurabilityDefault
	// DurabilityFull 执行文件 fsync 与（支持的平台的）目录 fsync。
	DurabilityFull = opc.DurabilityFull
)
