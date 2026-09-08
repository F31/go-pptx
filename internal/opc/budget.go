package opc

// Budget 是处理不可信 PPTX 时的资源预算（方案 §13）。
//
// 阈值可配置；任意字段为 0 时表示未配置，Scan 会回退到默认初值
// （默认初值是"可调整工程初值"，须以实际语料确定服务默认值，方案 §13）。
// 注意：0 不代表"无限"——需要收紧限制时显式给正值。
type Budget struct {
	// MaxEntries 限制 ZIP 条目总数（含目录条目）。
	MaxEntries int
	// MaxXMLBytes 限制单个 XML Part 解压后的上限（单 XML 32 MiB 初值）。
	MaxXMLBytes int64
	// MaxMediaBytes 限制单个非 XML Part（媒体等）解压后的上限。
	MaxMediaBytes int64
	// MaxTotalBytes 限制全部条目声明的解压大小总和（总解压 1 GiB 初值）。
	MaxTotalBytes int64
	// MaxXMLDepth 限制 XML 嵌套深度（初值 256）。
	MaxXMLDepth int
}

const (
	defaultMaxEntries    = 10_000
	defaultMaxXMLBytes   = int64(32 << 20) // 32 MiB
	defaultMaxMediaBytes = int64(512 << 20)
	defaultMaxTotalBytes = int64(1 << 30) // 1 GiB
	defaultMaxXMLDepth   = 256
)

// DefaultBudget 返回方案 §13 建议的起始预算。
func DefaultBudget() Budget {
	return Budget{
		MaxEntries:    defaultMaxEntries,
		MaxXMLBytes:   defaultMaxXMLBytes,
		MaxMediaBytes: defaultMaxMediaBytes,
		MaxTotalBytes: defaultMaxTotalBytes,
		MaxXMLDepth:   defaultMaxXMLDepth,
	}
}

// normalize 将未配置（<=0）字段回退到默认初值。
func (b Budget) normalize() Budget {
	d := DefaultBudget()
	if b.MaxEntries <= 0 {
		b.MaxEntries = d.MaxEntries
	}
	if b.MaxXMLBytes <= 0 {
		b.MaxXMLBytes = d.MaxXMLBytes
	}
	if b.MaxMediaBytes <= 0 {
		b.MaxMediaBytes = d.MaxMediaBytes
	}
	if b.MaxTotalBytes <= 0 {
		b.MaxTotalBytes = d.MaxTotalBytes
	}
	if b.MaxXMLDepth <= 0 {
		b.MaxXMLDepth = d.MaxXMLDepth
	}
	return b
}

// partLimit 返回给定条目适用的单 Part 预算。
func (b Budget) partLimit(entry string) int64 {
	if isXMLName(entry) {
		return b.MaxXMLBytes
	}
	return b.MaxMediaBytes
}
