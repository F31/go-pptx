package style

// StyleSource 标识 StyleStep 的来源层。
type StyleSource int

const (
	// SourceRun 是 run 本地 rPr（L1）。
	SourceRun StyleSource = iota
	// SourceParagraphDefault 是段落默认字符格式 pPr/defRPr（L2）。
	SourceParagraphDefault
	// SourceListStyle 是列表级别样式（占位符 lstStyle 或母版 txStyles；L3）。
	SourceListStyle
	// SourceTheme 是主题（fontScheme/clrScheme 展开或缺省；L4）。
	SourceTheme
	// SourceFallback 是调用方 ResolveContext.Fallback 回退值。
	SourceFallback
	// SourceCellExplicit 是单元格显式样式覆盖（a:tcPr；TABLE-01）。
	SourceCellExplicit
	// SourceTableStyle 是表格样式库的区域部分（tableStyles.xml；
	// TABLE-01），如 firstRow/band1H/nwCell 等。
	SourceTableStyle
)

func (s StyleSource) String() string {
	switch s {
	case SourceRun:
		return "run"
	case SourceParagraphDefault:
		return "paragraph-default"
	case SourceListStyle:
		return "list-style"
	case SourceTheme:
		return "theme"
	case SourceFallback:
		return "fallback"
	case SourceCellExplicit:
		return "cell-explicit"
	case SourceTableStyle:
		return "table-style"
	default:
		return "unknown"
	}
}

// StyleStep 是单个属性的一个解析来源步。Trace 以最近来源在前排列；
// 颜色/字体经主题展开时在其后追加 SourceTheme 步。
type StyleStep struct {
	// Source 是该步的来源层。
	Source StyleSource
	// Part 是提供该值的 Part（空串表示 run 所在 Part）。
	Part string
	// Detail 是人类可读的补充（如 "ph type=body idx=1 lvl=0"、
	// "clrScheme accent3 lumMod 60%"）。
	Detail string
}
