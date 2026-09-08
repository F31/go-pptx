package opc

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// 原子落盘（SAVE-02，方案 §5/§27/实施计划 §6）：
//
//	同目录临时文件 → ZIP Close + 输出校验 → 原子替换目标。
//
// 约束：
//   - 默认禁止覆盖已存在目标（ErrOutputExists）；WithOverwrite(true) 显式启用；
//   - 替换用平台原子机制（POSIX rename(2) / Windows MoveFileEx
//     REPLACE_EXISTING，os.Rename 两者皆覆盖）；替换失败保留旧目标，
//     绝不退化成"删旧再写"（ErrAtomicReplaceUnavailable）；
//   - 提交前任何失败删除临时文件、保留旧目标（AT-11）；
//   - WithDurability(DurabilityFull) 在支持的平台执行文件 fsync 与
//     目录 fsync；默认只保证错误处理与原子可见性，不承诺断电持久。
var (
	// ErrOutputExists 表示目标已存在且未显式允许覆盖。
	ErrOutputExists = errors.New("opc: output file exists")
	// ErrAtomicReplaceUnavailable 表示无法安全完成原子替换（旧目标保留）。
	ErrAtomicReplaceUnavailable = errors.New("opc: atomic replace unavailable")
)

// Durability 控制保存持久性级别。
type Durability int

const (
	// DurabilityDefault 只保证错误处理与原子可见性，不承诺断电持久。
	DurabilityDefault Durability = iota
	// DurabilityFull 执行文件 fsync 与（支持的平台的）目录 fsync。
	DurabilityFull
)

// SaveOptions 是 SaveToFile 的选项集合。
type SaveOptions struct {
	Overwrite  bool
	Durability Durability
}

// SaveOption 是 SaveToFile 的函数式选项。
type SaveOption func(*SaveOptions)

// WithOverwrite 显式允许覆盖已存在的目标（不绕过原子替换语义）。
func WithOverwrite(v bool) SaveOption {
	return func(o *SaveOptions) { o.Overwrite = v }
}

// WithDurability 设置持久性级别。
func WithDurability(d Durability) SaveOption {
	return func(o *SaveOptions) { o.Durability = d }
}

// tempPattern 是临时文件名模式（同目录、隐藏前缀、固定后缀便于清理检查）。
const tempPattern = ".go-pptx-save-*.tmp"

// SaveToFile 将保存计划原子写入 path。
//
// 流程：目标检查 → 同目录临时文件 → 写入 → fsync（可选）→ Close →
// 输出校验（ZIP 结构与条目集）→ 原子替换 → 目录 fsync（可选）。
// 提交前失败：删除临时文件，旧目标不变；清理失败以 errors.Join 附加，
// 不掩盖主错误也不误报目标已提交。
func (plan *SavePlan) SaveToFile(pk *Package, path string, opts ...SaveOption) error {
	o := SaveOptions{}
	for _, fn := range opts {
		fn(&o)
	}

	// 0) 目标检查在写临时文件之前（快速失败，不产生临时文件）。
	if _, err := os.Lstat(path); err == nil {
		if !o.Overwrite {
			return fmt.Errorf("%w: %s", ErrOutputExists, path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat target %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanupErr := func() error {
		if rmErr := os.Remove(tmpName); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return fmt.Errorf("remove temp file %s: %w", tmpName, rmErr)
		}
		return nil
	}

	// 1) 写入。I/O 失败时输出不完整，但仅存在于临时文件中。
	if err := plan.Write(pk, tmp); err != nil {
		tmp.Close()
		if rmErr := cleanupErr(); rmErr != nil {
			return errors.Join(fmt.Errorf("write plan: %w", err), rmErr)
		}
		return fmt.Errorf("write plan: %w", err)
	}
	// 2) fsync + Close。
	if o.Durability == DurabilityFull {
		if err := tmp.Sync(); err != nil {
			tmp.Close()
			if rmErr := cleanupErr(); rmErr != nil {
				return errors.Join(fmt.Errorf("sync temp file: %w", err), rmErr)
			}
			return fmt.Errorf("sync temp file: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		if rmErr := cleanupErr(); rmErr != nil {
			return errors.Join(fmt.Errorf("close temp file: %w", err), rmErr)
		}
		return fmt.Errorf("close temp file: %w", err)
	}

	// 3) 输出校验：ZIP 结构可打开、条目集与计划一致。
	if err := verifyOutput(tmpName, plan); err != nil {
		if rmErr := cleanupErr(); rmErr != nil {
			return errors.Join(fmt.Errorf("verify output: %w", err), rmErr)
		}
		return fmt.Errorf("verify output: %w", err)
	}

	// 4) 原子替换。失败保留旧目标；不删旧再写。
	if err := os.Rename(tmpName, path); err != nil {
		// rename 失败时临时文件仍在；按提交前失败语义清理。
		if rmErr := cleanupErr(); rmErr != nil {
			return errors.Join(fmt.Errorf("%w: rename to %s: %v (old target preserved)",
				ErrAtomicReplaceUnavailable, path, err), rmErr)
		}
		return fmt.Errorf("%w: rename to %s: %v (old target preserved)",
			ErrAtomicReplaceUnavailable, path, err)
	}

	// 5) 目录 fsync（DurabilityFull；Windows 无可移植目录句柄 fsync，跳过）。
	if o.Durability == DurabilityFull && runtime.GOOS != "windows" {
		if d, derr := os.Open(dir); derr == nil {
			if serr := d.Sync(); serr != nil {
				d.Close()
				// 目标已提交；目录 fsync 失败不伪装成保存失败。
				return fmt.Errorf("output committed but directory sync failed: %w", serr)
			}
			d.Close()
		}
	}
	return nil
}

// verifyOutput 重新打开输出文件做结构校验：ZIP Central Directory 可解析、
// 非遗漏条目集与计划一致。
func verifyOutput(name string, plan *SavePlan) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedPackage, err)
	}
	want := make([]string, 0, len(plan.Entries))
	for _, e := range plan.Entries {
		if e.Action == Omit {
			continue
		}
		en, err := e.Name.EntryName()
		if err != nil {
			return err
		}
		want = append(want, en)
	}
	sort.Strings(want)
	got := make([]string, 0, len(zr.File))
	for _, zf := range zr.File {
		got = append(got, zf.Name)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		return fmt.Errorf("output has %d entries, plan wants %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("output entry %q, want %q", got[i], want[i])
		}
	}
	return nil
}
