// Package ir 提供 PPTX 的可选只读中间表示（方案 §18.3）。
//
// 关键约束：
//   - IR 是只读快照，不可作为 PPTX → PPTX 默认覆盖保存的输入；未来导入
//     只通过明确的有损新建接口（§18.3 顶部）；
//   - IR 不嵌入媒体二进制；仅通过 Part 名 + Content Type 引用（v1）；
//   - IR 的 schemaVersion 与 SDK 版本独立管理（SchemaVersion 常量），后续
//     兼容性按 schemaVersion 而非 go-pptx 版本号判定；
//   - 核心包（github.com/F31/go-pptx/v2）不得导入本包；本包仅依赖公共 pptx
//     与标准库。
//
// 典型用途：内容提取、调试、预览输入、跨格式适配（§18.3）、两份文档
// 语义 diff（DIFF-01）的输入视图；动画时序投影属 TIMIR-01（M7）。
//
// v1 范围（TOOL-01）：
//   - 文档元数据摘要（core.xml）；
//   - 页面顺序、SlideID、名称、备注文本；
//   - 顶层形状：ID/Name/Kind/AltText/Decorative/Bounds/WorldBox；
//   - ShapeChart 类型与首组 Categories（ChartShape.Data）；
//   - ShapeTable 行/列/纯文本拼接（Cell.TextFrame → Paragraph.Text）；
//   - ShapeTextBox/ShapeAutoShape 文本提取（AutoShape.TextFrame →
//     Paragraph.Text）；
//   - 媒体 Part 名 + ContentType 列表（不嵌入二进制）。
//
// 暂未覆盖（本版本）：形状矩阵变换详情、动画时序投影（TIMIR-01）、
// 主题/版式/母版内容、嵌入对象的解析、占位符索引。
package ir

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/F31/go-pptx/v2/internal/document/model"
)

// SchemaVersion 是 IR 的当前 schema 版本号。本字段独立于 SDK 版本演进。
// 新增可选字段必须保持向后兼容；语义变化（删除/重命名/类型变更）必须
// 递增 schemaVersion，并保留旧 SchemaVersion 的版本入口。
const SchemaVersion = "go-pptx.ir/1.0"

// SDKVersion 是构建本库时的语义版本号（独立于 SchemaVersion）。
// 默认空串；发布构建可由 CI 注入。
var SDKVersion = ""

// SeverityString 是 Diagnostic.Severity 的字符串值，与门面 Diagnostic
// 在序列化层对齐（"info"/"warning"/"error"）。
type SeverityString string

const (
	SevInfo    SeverityString = "info"
	SevWarning SeverityString = "warning"
	SevError   SeverityString = "error"
)

// Diagnostics 是导出过程中的非阻断诊断。
type Diagnostics []Diagnostic

// Diagnostic 单条诊断项。
type Diagnostic struct {
	Code     string         `json:"code"`
	Severity SeverityString `json:"severity"`
	Part     string         `json:"part,omitempty"`
	Message  string         `json:"message"`
}

// Document 是 IR 顶层对象。
type Document struct {
	SchemaVersion string      `json:"schemaVersion"`
	SDKVersion    string      `json:"sdkVersion,omitempty"`
	DocumentID    string      `json:"documentID,omitempty"`
	Core          *Core       `json:"core,omitempty"`
	Pages         []Page      `json:"pages"`
	Diagnostics   Diagnostics `json:"diagnostics,omitempty"`
}

// Core 是文档核心属性（§20.3 docProps）摘要。
type Core struct {
	Title       string     `json:"title,omitempty"`
	Subject     string     `json:"subject,omitempty"`
	Creator     string     `json:"creator,omitempty"`
	Keywords    string     `json:"keywords,omitempty"`
	Description string     `json:"description,omitempty"`
	Created     *time.Time `json:"created,omitempty"`
	Modified    *time.Time `json:"modified,omitempty"`
}

// Page 是单页 IR 视图。
type Page struct {
	Index     int           `json:"index"`
	SlideID   model.SlideID `json:"slideID"`
	Part      string        `json:"part"`
	Name      string        `json:"name,omitempty"`
	Shapes    []Shape       `json:"shapes"`
	NotesText string        `json:"notesText,omitempty"`
	HasTiming bool          `json:"hasTiming,omitempty"`
	// Hidden 是页面是否被标记隐藏（p:sldId@show="0"）。
	//
	// 三态语义：
	//   - nil：未读取或未确定（IncludeHidden=false 时一致）；
	//   - &false：已读，确认可见（show 缺省或非 "0"）；
	//   - &true：已读，确认被隐藏。
	//
	// ppts 等客户端可据此 default-skip=true 过滤讲稿源或导入页。
	//
	// Experimental: 字段新增，binary-compat；写侧随 1.x Hidden() 公开方法成熟
	// 后可能扩展为 IR 多端（JSON 协议）稳定档，需走新 ADR-019+ 走评审。
	Hidden *bool `json:"hidden,omitempty"`
	// Timing 是 PageTiming 投影（TIMIR-01）。当 IncludeTimingIR=false 或
	// 页无 timing 时为空。
	Timing      *PageTiming `json:"timing,omitempty"`
	Diagnostics Diagnostics `json:"diagnostics,omitempty"`
}

// Shape 是单个形状的只读摘要。
type Shape struct {
	ID   model.ShapeID `json:"id"`
	Name string        `json:"name,omitempty"`
	Kind string        `json:"kind"`
	// NodePath 是形状在所属 Part 内的元素路径（形如
	// p:sld/p:cSld/p:spTree/p:sp[2]）——DIFF-01 报告据此回溯到
	// Part/NodePath（设计 §18.3）。句柄失效时为空。
	NodePath   string  `json:"nodePath,omitempty"`
	AltText    string  `json:"altText,omitempty"`
	Decorative bool    `json:"decorative,omitempty"`
	Bounds     *Box    `json:"bounds,omitempty"`
	WorldBox   *Box    `json:"worldBox,omitempty"`
	Text       string  `json:"text,omitempty"`
	Children   []Shape `json:"children,omitempty"`
	// Chart 专属字段（ShapeChart）。
	ChartType string `json:"chartType,omitempty"`
	// Table 专属字段（ShapeTable）。
	TableRows int `json:"tableRows,omitempty"`
	TableCols int `json:"tableCols,omitempty"`
	// Diagnostics 是本形状读取过程中产生的非阻断诊断（如图表轴未提取字段的
	// 降置信信号，FEAT-002 项 3，ADR-020）。Experimental: 字段新增，
	// binary-compat；走 ir schemaVersion 兼容（omitempty，不影响既有消费方）。
	Diagnostics Diagnostics `json:"diagnostics,omitempty"`
}

// Box 是轴对齐矩形（EMU）。
type Box struct {
	X      int64 `json:"x"`
	Y      int64 `json:"y"`
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

// Options 控制 IR 投影的行为（由调用方的 builder 解释）。
type Options struct {
	// IncludeNotes 收集 SpeakerNotesText（默认 true）。
	IncludeNotes bool
	// IncludeTimingNode 仅在 HasTiming=true 时置位；本页是否含任何 timing
	// 子树（不做投影，TIMIR-01）。
	IncludeTimingNode bool
	// IncludeTimingIR 控制是否把 p:timing 投影为 PageTiming（TIMIR-01）。
	IncludeTimingIR bool
	// IncludeHidden 控制 Page.Hidden 三态字段的填充：
	//   - true（默认）：调用 Slide.Hidden()，三态字段正确填充；
	//   - false：跳过 Page.Hidden 填充，输出 nil，节省 p:sldIdLst 解析。
	IncludeHidden bool
}

// DefaultOptions 默认 IR 投影参数。
func DefaultOptions() Options {
	return Options{
		IncludeNotes:      true,
		IncludeTimingNode: true,
		IncludeTimingIR:   true,
		IncludeHidden:     true,
	}
}

// Marshal 把 IR 写为 JSON。序列化失败直接返回错误。
func (d *Document) Marshal() ([]byte, error) {
	return json.Marshal(d)
}

// Unmarshal 把 JSON 解析回 IR。附带 schemaVersion 校验。
func Unmarshal(data []byte) (*Document, error) {
	var d Document
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("ir: unmarshal: %w", err)
	}
	if d.SchemaVersion == "" {
		return nil, fmt.Errorf("ir: schemaVersion missing")
	}
	if d.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("ir: schemaVersion %q not supported (current %q)",
			d.SchemaVersion, SchemaVersion)
	}
	return &d, nil
}

func SortDiagnostics(diags Diagnostics) {
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Severity != diags[j].Severity {
			return severityRank(diags[i].Severity) > severityRank(diags[j].Severity)
		}
		if diags[i].Code != diags[j].Code {
			return diags[i].Code < diags[j].Code
		}
		return diags[i].Part < diags[j].Part
	})
}

func severityRank(s SeverityString) int {
	switch s {
	case SevError:
		return 3
	case SevWarning:
		return 2
	case SevInfo:
		return 1
	}
	return 0
}
