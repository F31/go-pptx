// 本文件汇集 Presentation 各公共入口的"函数式选项"（New / Open / Save /
// Validate / Replace / Bind / Merge），对齐设计文档 §3"options.go"清单。
//
// 约定：所有 *Option 都是 `func(*xxxOptions)`，调用方通过 With* 构造器
// 累积配置；底层 xxxOptions 结构体小写不导出，确保对外配置面收敛到
// 显式声明的 With* 入口。结构体字段是配置真相，With* 是 API 入口，
// 内部消费方（New/Open/Save/...）把 options 显式落到具体动作上。

package pptx

import (
	"github.com/F31/go-pptx/internal/opc"
)

// ---------- New（库内最小模板创建）----------

// NewOption 是 New 的函数式选项。
type NewOption func(*newOptions)

type newOptions struct {
	budget   opc.Budget
	template map[opc.PartName][]byte
}

// WithNewBudget 覆盖 New 的资源预算（默认 DefaultBudget）。
func WithNewBudget(b opc.Budget) NewOption {
	return func(o *newOptions) { o.budget = b }
}

// WithNewTemplate 追加/替换模板 Part（键为 OPC 风格 "/..." 名）。
// 模板整体仍须经完整的 OPC 装载校验，非法输入在 New 返回错误。
func WithNewTemplate(parts map[string][]byte) NewOption {
	return func(o *newOptions) {
		if o.template == nil {
			o.template = make(map[opc.PartName][]byte, len(parts))
		}
		for k, v := range parts {
			o.template[opc.PartName(k)] = append([]byte(nil), v...)
		}
	}
}

// ---------- Open（磁盘 / ReaderAt 打开）----------

// OpenOption 是 Open/OpenReader 的函数式选项。
type OpenOption func(*openOptions)

type openOptions struct {
	budget opc.Budget
}

// WithBudget 覆盖打开时的资源预算（默认 DefaultBudget）。
func WithBudget(b opc.Budget) OpenOption {
	return func(o *openOptions) { o.budget = b }
}

// ---------- Save（原子保存 / Writer 写出）----------

// SaveOption 是 Save/Write 的函数式选项。
type SaveOption func(*saveOptions)

type saveOptions struct {
	overwrite  bool
	durability opc.Durability
}

// WithSaveOverwrite 显式允许覆盖已存在的目标文件（不绕过原子替换语义）。
func WithSaveOverwrite(v bool) SaveOption {
	return func(o *saveOptions) { o.overwrite = v }
}

// WithSaveDurability 设置持久性级别（默认只保证原子可见性）。
func WithSaveDurability(d opc.Durability) SaveOption {
	return func(o *saveOptions) { o.durability = d }
}

// ---------- Validate（结构校验）----------

// ValidateOption 预留：校验选项（ValidationMode 等）随校验 WP 落地。
type ValidateOption func(*validateOptions)

type validateOptions struct{}

// ---------- Replace（跨 Run 文本替换）----------

// replaceOptions 收集 ReplaceText 的函数式选项。
type replaceOptions struct {
	mode     ReplaceMode
	style    FontStyle
	styleSet bool
}

// ReplaceOption 是 ReplaceText 的函数式选项。
type ReplaceOption func(*replaceOptions)

// WithReplaceMode 选择格式策略；缺省 ReplaceFirstCharacter。
func WithReplaceMode(m ReplaceMode) ReplaceOption {
	return func(o *replaceOptions) { o.mode = m }
}

// WithReplacementStyle 提供 ExplicitStyle 策略下 replacement 的格式。
// 仅 ReplaceExplicitStyle 使用；其它策略下调用无效果。
func WithReplacementStyle(style FontStyle) ReplaceOption {
	return func(o *replaceOptions) {
		o.style = style
		o.styleSet = true
	}
}

// ---------- Bind（模板数据绑定）----------

// bindOptions 收集 Bind 的函数式选项。
type bindOptions struct {
	strict bool
	mode   ReplaceMode
}

// BindOption 是 Presentation.Bind 的函数式选项。
type BindOption func(*bindOptions)

// WithBindStrict 设置严格模式（默认 true）：数据源缺键或值类型不可
// 呈现时显式报错；关闭时未解析占位符保留原文并记 Warning 诊断。
func WithBindStrict(strict bool) BindOption {
	return func(o *bindOptions) { o.strict = strict }
}

// WithBindReplaceMode 设置占位符替换的格式策略
// （默认 ReplaceFirstCharacter，与 Paragraph.ReplaceText 一致）。
func WithBindReplaceMode(m ReplaceMode) BindOption {
	return func(o *bindOptions) { o.mode = m }
}

// ---------- Merge（表格单元格合并）----------

type mergeOptions struct {
	policy MultiCellTextPolicy
}

// MergeOption 配置单次合并行为。
type MergeOption func(*mergeOptions)

// WithMergeTextPolicy 指定多非空单元格合并策略（默认
// MergeRejectMultipleText）。
func WithMergeTextPolicy(p MultiCellTextPolicy) MergeOption {
	return func(o *mergeOptions) { o.policy = p }
}
