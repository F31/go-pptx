package opc

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// 本包错误（internal）。公共入口 Open/Save 在边界上映射为根包稳定错误码
// （ErrMalformedPackage / ErrLimitExceeded / ErrNotFound，方案 §20.4），
// internal 包不反向 import 根包，避免循环依赖。
var (
	ErrMalformedPackage = errors.New("opc: malformed package")
	ErrLimitExceeded    = errors.New("opc: resource limit exceeded")
	ErrNotFound         = errors.New("opc: part not found")
)

// Index 是 ZIP 条目的只读索引：Scan 阶段校验路径、重复与预算（L0），
// Part 内容按需惰性读取（方案 §13：不能只信 ZIP 元数据，读取时按实际
// 字节计数）。索引不持有文件句柄，调用方负责 ReaderAt 生命周期。
type Index struct {
	budget  Budget
	byEntry map[string]*zip.File // key: 小写化条目名
	names   []string             // 非目录条目名（原始顺序）
	total   uint64               // 全部条目声明解压大小之和
}

// Scan 从 ReaderAt 建立包索引。
//
// budget 为零值时使用 DefaultBudget。校验失败返回 ErrMalformedPackage 或
// ErrLimitExceeded（均可 errors.Is）。
func Scan(ra io.ReaderAt, size int64, budget Budget) (*Index, error) {
	if budget == (Budget{}) {
		budget = DefaultBudget()
	}
	budget = budget.normalize()

	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedPackage, err)
	}
	if len(zr.File) > budget.MaxEntries {
		return nil, fmt.Errorf("%w: %d entries exceeds limit %d",
			ErrLimitExceeded, len(zr.File), budget.MaxEntries)
	}

	ix := &Index{
		budget:  budget,
		byEntry: make(map[string]*zip.File, len(zr.File)),
	}
	seen := make(map[string]string, len(zr.File))
	var total uint64

	for _, f := range zr.File {
		if err := validateEntryName(f.Name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalformedPackage, err)
		}
		total += f.UncompressedSize64

		if strings.HasSuffix(f.Name, "/") {
			continue // 目录条目：参与条目计数与总量预算，但不作为 Part
		}
		key := strings.ToLower(f.Name)
		if prev, dup := seen[key]; dup {
			return nil, fmt.Errorf("%w: duplicate entry %q (also listed as %q)",
				ErrMalformedPackage, f.Name, prev)
		}
		seen[key] = f.Name

		// 元数据预检（读取时仍按实际字节复核）。
		if limit := budget.partLimit(f.Name); f.UncompressedSize64 > uint64(limit) {
			return nil, fmt.Errorf("%w: part %q declares %d bytes, limit %d",
				ErrLimitExceeded, f.Name, f.UncompressedSize64, limit)
		}

		ix.byEntry[key] = f
		ix.names = append(ix.names, f.Name)
	}

	if total > uint64(budget.MaxTotalBytes) {
		return nil, fmt.Errorf("%w: total declared size %d bytes exceeds limit %d",
			ErrLimitExceeded, total, budget.MaxTotalBytes)
	}
	ix.total = total
	return ix, nil
}

// Count 返回非目录条目数。
func (ix *Index) Count() int { return len(ix.names) }

// TotalDeclared 返回全部条目声明的解压字节总数。
func (ix *Index) TotalDeclared() uint64 { return ix.total }

// EntryNames 返回非目录条目名（原始顺序）的副本。
func (ix *Index) EntryNames() []string {
	out := make([]string, len(ix.names))
	copy(out, ix.names)
	return out
}

// HasPart 报告 PartName 是否存在。
func (ix *Index) HasPart(name PartName) bool {
	if !name.Valid() {
		return false
	}
	_, ok := ix.byEntry[lookupKey(name)]
	return ok
}

// PartNames 返回全部 Part 名（按名称排序，确定性输出）。
func (ix *Index) PartNames() []PartName {
	out := make([]PartName, 0, len(ix.names))
	for _, n := range ix.names {
		out = append(out, PartName("/"+n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func lookupKey(name PartName) string {
	return strings.ToLower(strings.TrimPrefix(string(name), "/"))
}

func (ix *Index) lookup(name PartName) (*zip.File, error) {
	if !name.Valid() {
		return nil, fmt.Errorf("%w: invalid part name %q", ErrMalformedPackage, string(name))
	}
	f, ok := ix.byEntry[lookupKey(name)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return f, nil
}

// OpenXMLPart 打开 XML Part 并返回按实际读取字节计数的流；读取超过
// MaxXMLBytes 时返回 ErrLimitExceeded。调用方负责 Close。
func (ix *Index) OpenXMLPart(name PartName) (io.ReadCloser, error) {
	return ix.openPart(name, ix.budget.MaxXMLBytes)
}

// OpenPart 打开任意 Part；XML 按 XML 预算、媒体按媒体预算计数。
func (ix *Index) OpenPart(name PartName) (io.ReadCloser, error) {
	f, err := ix.lookup(name)
	if err != nil {
		return nil, err
	}
	return ix.openFile(f)
}

func (ix *Index) openPart(name PartName, limit int64) (io.ReadCloser, error) {
	f, err := ix.lookup(name)
	if err != nil {
		return nil, err
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedPackage, err)
	}
	return &countedReadCloser{r: rc, limit: limit, name: f.Name}, nil
}

func (ix *Index) openFile(f *zip.File) (io.ReadCloser, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedPackage, err)
	}
	return &countedReadCloser{
		r:     rc,
		limit: ix.budget.partLimit(f.Name),
		name:  f.Name,
	}, nil
}

// countedReadCloser 按实际读取字节计数（方案 §13：不能只信 ZIP 元数据）。
//
// 读取总量超过 limit 时返回 ErrLimitExceeded（可能伴随最后一次部分读取的
// 数据，调用方应丢弃本次读取结果）。与 io.LimitReader 的静默截断不同，
// 超限必须以错误暴露，不允许悄悄截断恶意输入。
type countedReadCloser struct {
	r     io.ReadCloser
	used  int64
	limit int64
	name  string
}

func (c *countedReadCloser) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.used += int64(n)
	if c.used > c.limit {
		return n, fmt.Errorf("%w: part %q exceeded %d bytes (read %d)",
			ErrLimitExceeded, c.name, c.limit, c.used)
	}
	return n, err
}

func (c *countedReadCloser) Close() error { return c.r.Close() }
