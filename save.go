// 本文件汇集"保存链路"的公共入口——SaveReport 类型 + Presentation 的
// Close / Save / Write 方法——对齐设计文档 §3"save.go"清单。
//
// Save 是原子保存的对外门面：默认拒绝覆盖与原位保存（同 §5 源文件仍
// 惰性读取），底层走 opc.SavePlan 的同目录临时文件 + 原子替换语义
//（SAVE-02）。Write 把同一份 SavePlan 落到 io.Writer，I/O 失败时
// 错误明确表明输出可能不完整。
//
// Close 负责释放 Open 持有的文件资源；重复 Close 返回 ErrClosed（与
// 其它公共方法一致的 closed-guard 语义）。

package pptx

import (
	"context"
	"io"

	"github.com/F31/go-pptx/internal/opc"
)

// SaveReport 是一次保存的结果。
type SaveReport struct {
	// Revision 是保存所基于的文档 revision。
	Revision uint64
	// ChangedParts 是输出中发生变化的 Part 名（排序）。
	ChangedParts []string
	// Diagnostics 是保存计划的非阻断诊断。
	Diagnostics []Diagnostic
}

// Close 释放资源。Open 打开的文件在此关闭；此后任何方法返回
// ErrClosed（Close 本身幂等返回 ErrClosed 语义之外的 nil 无必要，
// 重复 Close 返回 ErrClosed）。
func (p *Presentation) Close() error {
	if p.closed {
		return Annotate(ErrClosed, "Presentation.Close")
	}
	p.closed = true
	if p.srcFile != nil {
		err := p.srcFile.Close()
		p.srcFile = nil
		if err != nil {
			return Annotate(err, "Presentation.Close")
		}
	}
	return nil
}

// Save 将当前文档原子保存到 path（同目录临时文件 → 校验 → 原子替换）。
//
// 默认拒绝覆盖已存在目标（WithSaveOverwrite 启用）；且拒绝与源文件
// 同一文件实体的原位保存——源文件仍被惰性读取，安全原位替换待后续
// 版本（方案 §5）。返回的 SaveReport 基于保存时的 revision 快照。
func (p *Presentation) Save(ctx context.Context, path string, opts ...SaveOption) (SaveReport, error) {
	o := saveOptions{}
	for _, fn := range opts {
		fn(&o)
	}
	if p.closed {
		return SaveReport{}, Annotate(ErrClosed, "Presentation.Save")
	}
	if err := ctx.Err(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Save")
	}
	if path == "" {
		return SaveReport{}, Annotate(ErrInvalidArgument, "Presentation.Save")
	}
	if p.sameSourceEntity(path) {
		return SaveReport{}, &OperationError{
			Op:      "Presentation.Save",
			Message: "saving to the source file is not supported; choose a new output path (in-place replace arrives in a later version)",
			Err:     ErrInvalidArgument,
		}
	}

	// Modified 未显式指定时由库代管：保存前刷新为当前时间。
	if err := p.flushAutoModified(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Save")
	}
	rev, plan, err := p.buildPlan()
	if err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Save")
	}
	if p.rev != rev { // 计划期间的变更检测（§18）
		return SaveReport{}, Annotate(ErrConcurrentModification, "Presentation.Save")
	}
	err = plan.SaveToFile(p.pk, path,
		opc.WithOverwrite(o.overwrite), opc.WithDurability(o.durability))
	if err != nil {
		return SaveReport{}, Annotate(mapOCError(err), "Presentation.Save")
	}
	return planReport(rev, plan), nil
}

// Write 将当前文档写入 w。已写入的字节无法回滚；I/O 失败时错误明确
// 表明输出可能不完整（方案 §5）。
func (p *Presentation) Write(ctx context.Context, w io.Writer, opts ...SaveOption) (SaveReport, error) {
	if p.closed {
		return SaveReport{}, Annotate(ErrClosed, "Presentation.Write")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Write")
	}
	if w == nil {
		return SaveReport{}, Annotate(ErrInvalidArgument, "Presentation.Write")
	}
	// Modified 未显式指定时由库代管：保存前刷新为当前时间。
	if err := p.flushAutoModified(); err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Write")
	}
	rev, plan, err := p.buildPlan()
	if err != nil {
		return SaveReport{}, Annotate(err, "Presentation.Write")
	}
	if p.rev != rev {
		return SaveReport{}, Annotate(ErrConcurrentModification, "Presentation.Write")
	}
	if err := plan.Write(p.pk, w); err != nil {
		return SaveReport{}, &OperationError{
			Op:      "Presentation.Write",
			Message: "write failed; output may be incomplete and cannot be rolled back",
			Err:     mapOCError(err),
		}
	}
	return planReport(rev, plan), nil
}
