// Package schema 是从 ECMA-376 Transitional XSD **生成**的 OOXML 类型
// 模型（ADR-030 机制 2 / 演进第 1 步目标：`internal/ooxml/schema/`）。
//
// 硬约束（ADR-030 机制 2）：生成类型**只用于只读投影**（encoding/xml
// 反序列化，替换手写 parse），**不得进入写路径**——写路径维持
// `xmlstore` span 补丁 + 未修改 Part 字节拷贝，以保字节级保真证据。
//
// 生成：
//
//	go generate ./internal/ooxml/schema
//	# 等价于：
//	scripts/gen/schema/fetch.sh
//	go run ./scripts/gen/schema -xsd internal/ooxml/schema/.xsd -out internal/ooxml/schema
//
// XSD 输入来自 ECMA-376 第 5 版 Part 4 的 Transitional schema 集
// （schemas.openxmlformats.org 命名空间，与真实 PPTX 一致）；下载物落在
// `.xsd/`（.gitignore 忽略），生成物 `zz_generated_*.go` 入库。
//
// 生成器（scripts/gen/schema）只依赖 std-lib；类型名以命名空间短前缀
// 区分（P_/A_/S_/R_/C_…）避免同包重名。
package schema

import "encoding/xml"

//go:generate bash ../../../scripts/gen/schema/fetch.sh
//go:generate go run ../../../scripts/gen/schema -xsd .xsd -out .

// RawElem 承载未能映射到具体生成类型的元素（跨命名空间未加载的复杂
// 类型、xsd:any、匿名内联 complexType）。只读投影时保留原始 innerxml 与
// 属性，由调用方按需二次解析。
type RawElem struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   string     `xml:",innerxml"`
}
