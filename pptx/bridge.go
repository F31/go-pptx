package pptx

import "github.com/F31/go-pptx/internal/opc"

// 本文件是 v2.0 门面 → 格式层的**只读桥**：让 internal 层能从
// *Presentation 读取 Part 原始字节，用于 internal/ooxml 的 schema 只读
// 投影（演进第 1 步长尾）。不参与任何写路径；保持 Part 字节恒等。
//
// 说明：这是包级函数而非方法，故不进入 api_surface 的方法黄金面。

// PartBytes 返回包内 Part 的原始字节（只读）。Part 不存在或读取失败时
// 返回 (nil, false)。明确只读：写路径不得通过本桥再写回。
func PartBytes(p *Presentation, name opc.PartName) ([]byte, bool) {
	if p == nil || name == "" {
		return nil, false
	}
	b, err := p.partBytes(name)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// MainPartBytes 返回主 Part（presentation.xml）原始字节（只读）。
// 主 Part 缺失或读取失败返回 (nil, false)。
func MainPartBytes(p *Presentation) ([]byte, bool) {
	if p == nil {
		return nil, false
	}
	return PartBytes(p, p.main)
}
