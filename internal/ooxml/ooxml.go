// Package ooxml 是 v2.0 格式层（ADR-030 目标依赖链中的
// `internal/ooxml`）：在 encoding/xml 生成的 schema 类型之上提供只读
// 投影助手。当前覆盖：OPC Part 读取桥 + p:sld 形状/文本/表格投影。
//
// 硬约束：本层只做**只读投影**（替换手写 parse / 门面句柄读取），不得
// 进入写路径（写路径维持 xmlstore span 补丁 + 未修改 Part 字节拷贝）。
// 本层不得 import 门面（archlint R1）与 internal/ir。
package ooxml

import (
	"io"

	"github.com/F31/go-pptx/internal/opc"
)

// Open 以只读方式加载 OPC 包（包装 opc.Load，默认预算）。调用方负责关闭。
func Open(r io.ReaderAt, size int64) (*opc.Package, error) {
	return opc.Load(r, size, opc.Budget{})
}

// Bytes 返回包内 Part 的原始字节；不存在返回 (nil, false)。
func Bytes(p *opc.Package, name opc.PartName) ([]byte, bool) {
	if p == nil || !p.HasPart(name) {
		return nil, false
	}
	rc, err := p.OpenPart(name)
	if err != nil {
		return nil, false
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}
