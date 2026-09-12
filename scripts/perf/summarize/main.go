// Command summarize 把 `go test -bench` 原始日志聚合为 PERF-01 性能基线报告
// （设计方案 §15.3：报告硬件、Go 版本、输入尺寸与 p50/p95/峰值内存）。
//
// 用法：
//
//	go test -run '^$' -bench BenchmarkPerf -benchmem -count 10 . | \
//	    go run ./scripts/perf/summarize > docs/PERF-01-benchmark-report.md
//
// 输入解析：
//   - 以 '#' 开头的行视为驱动脚本写入的头部（key: value），识别 timestamp /
//     count / benchtime / go-version；
//   - `go test` 自身输出的 goos / goarch / pkg / cpu 行用于填写环境信息；
//   - 以 "Benchmark" 开头的行是数据行，形如
//     `Name-N  <ns/op> ns/op  <MB/s> MB/s  <B/op> B/op  <allocs/op> allocs/op
//     <pkg-bytes> pkg-bytes  <peak-heap-B> peak-heap-B`（度量单位成对出现）。
//
// 聚合口径：同一基准名的多次 -count 采样取 ns/op 的 p50 / p95（最近秩法），
// 其余度量取中位数（peak-heap-B 取最大值，因其为峰值语义）。峰值内存采样会
// STW，BenchmarkPerfPeakHeap 的 ns/op 不具参考意义，报告中不予呈现。
package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// metric 是某基准名下的一个度量聚合结果（按单位归档）。
type metric struct {
	unit string
	vals []float64
}

// result 是单个基准名的聚合结果。
type result struct {
	name    string
	order   int // 首次出现次序，保持报告稳定排序
	metrics map[string]*metric
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "summarize: %v\n", err)
		os.Exit(1)
	}
}

// header 保存从输入中提取的环境与驱动信息。
type header struct {
	goVersion string
	timestamp string
	count     string
	benchtime string
	goos      string
	goarch    string
	cpu       string
	pkg       string
}

func run(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	hdr := header{}
	order := 0
	byName := map[string]*result{}

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			applyHeader(&hdr, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
		case strings.HasPrefix(trimmed, "goos:"):
			hdr.goos = value(trimmed)
		case strings.HasPrefix(trimmed, "goarch:"):
			hdr.goarch = value(trimmed)
		case strings.HasPrefix(trimmed, "cpu:"):
			hdr.cpu = value(trimmed)
		case strings.HasPrefix(trimmed, "pkg:"):
			hdr.pkg = value(trimmed)
		case strings.HasPrefix(trimmed, "Benchmark"):
			res, err := parseBenchLine(trimmed)
			if err != nil {
				return err
			}
			if cur, ok := byName[res.name]; ok {
				mergeMetrics(cur, res)
				continue
			}
			res.order = order
			order++
			byName[res.name] = res
		default:
			// PASS / ok / FAIL / --- 等行忽略。
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(byName) == 0 {
		return fmt.Errorf("no benchmark lines found in input")
	}

	results := make([]*result, 0, len(byName))
	for _, res := range byName {
		results = append(results, res)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].order < results[j].order })

	writeReport(w, hdr, results)
	return nil
}

// applyHeader 解析驱动脚本写入的 "# key: value" 头部行。
func applyHeader(h *header, s string) {
	key := s
	if i := strings.IndexByte(s, ':'); i >= 0 {
		key = strings.TrimSpace(s[:i])
	}
	val := value(s)
	switch key {
	case "go-version":
		h.goVersion = val
	case "timestamp", "generated-utc":
		h.timestamp = val
	case "count":
		h.count = val
	case "benchtime":
		h.benchtime = val
	}
}

// value 返回 "key: value" 行中冒号后的内容。
func value(s string) string {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return ""
}

// parseBenchLine 解析单行 benchmark 输出。
//
// 行格式：`Name-N  <iters>  <value> <unit>  [<value> <unit> ...]`——名称后紧跟
// 迭代次数列，其余度量以「值 单位」成对出现。
func parseBenchLine(line string) (*result, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return nil, fmt.Errorf("malformed benchmark line: %q", line)
	}
	res := &result{name: fields[0], metrics: map[string]*metric{}}
	// 跳过迭代次数列（若存在）。
	start := 1
	if _, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
		start = 2
	}
	for i := start; i+1 < len(fields); i += 2 {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return nil, fmt.Errorf("benchmark %s: bad value %q", fields[0], fields[i])
		}
		unit := fields[i+1]
		m, ok := res.metrics[unit]
		if !ok {
			m = &metric{unit: unit}
			res.metrics[unit] = m
		}
		m.vals = append(m.vals, v)
	}
	return res, nil
}

// mergeMetrics 把同一基准名的后续采样合并进首个 result。
func mergeMetrics(dst, src *result) {
	for unit, sm := range src.metrics {
		dm, ok := dst.metrics[unit]
		if !ok {
			dm = &metric{unit: unit}
			dst.metrics[unit] = dm
		}
		dm.vals = append(dm.vals, sm.vals...)
	}
}

// percentile 用最近秩法返回排序后的分位数（p ∈ [0,1]）。
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func maxOf(vals []float64) float64 {
	m := 0.0
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

// metricOr 返回指定单位度量的聚合值（medianVals 为真取中位数，否则取最大值）。
func (r *result) metricOr(unit string, medianVals bool) (float64, bool) {
	m, ok := r.metrics[unit]
	if !ok || len(m.vals) == 0 {
		return 0, false
	}
	if medianVals {
		return median(m.vals), true
	}
	return maxOf(m.vals), true
}

// splitName 把基准名拆成 (操作, 语料) 两段。
func splitName(name string) (op, deck string) {
	base := strings.TrimPrefix(name, "Benchmark")
	// 去掉 GOMAXPROCS 后缀（形如 -8）。
	if i := strings.LastIndexByte(base, '-'); i >= 0 {
		if _, err := strconv.Atoi(base[i+1:]); err == nil {
			base = base[:i]
		}
	}
	if i := strings.IndexByte(base, '/'); i >= 0 {
		return base[:i], base[i+1:]
	}
	return base, ""
}

// opTitle 把基准操作名映射为中文分组标题与说明。
func opTitle(op string) (string, string) {
	switch op {
	case "PerfOpen":
		return "打开（BenchmarkPerfOpen）", "Open 全量解析 + Close。"
	case "PerfTraverse":
		return "遍历（BenchmarkPerfTraverse）", "打开一次后反复枚举全部页/形状/段落文本。"
	case "PerfReplace":
		return "单处替换（BenchmarkPerfReplace）", "首页文本框内唯一标记的可逆替换（每轮恰好 1 处命中）。"
	case "PerfSaveMem":
		return "保存·内存序列化（BenchmarkPerfSaveMem）", "Write → io.Discard，不含文件系统。"
	case "PerfSaveDisk":
		return "保存·落盘原子替换（BenchmarkPerfSaveDisk）", "Save 到临时路径，含原子替换与文件系统开销。"
	case "PerfPeakHeap":
		return "全流程峰值内存（BenchmarkPerfPeakHeap）", "Open → 遍历 → Write 的堆分配峰值；采样会 STW，故不呈现 ns/op。"
	case "SavePlanWriteCopyOriginal":
		return "保存·未变 Part 复制（internal/opc，ADR-018）",
			"空变更集保存，未变媒体 Part 全部走 CopyOriginal（媒体重文档的真实热点）。" +
				"**分配总量/op 应与媒体体积解耦**——这是 ADR-018 Tier 1 的收益锚点；" +
				"PeakHeap 行的 ns/op 含采样 STW，不具参考意义。"
	default:
		return op, ""
	}
}

func writeReport(w io.Writer, hdr header, results []*result) {
	goVer := normalizeGoVersion(hdr.goVersion)
	goos := hdr.goos
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := hdr.goarch
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	fmt.Fprintln(w, "# go-pptx 性能基线报告（PERF-01）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "> 本文件由 `scripts/perf/summarize` 从 `go test -bench` 原始日志自动生成，请勿手工编辑。")
	fmt.Fprintln(w, "> 指标口径与限制见 `docs/PERF-01-性能基线.md`（设计方案 §15.3 / 实施计划 §9.3）。")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## 环境")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| 项 | 值 |")
	fmt.Fprintln(w, "|---|---|")
	fmt.Fprintf(w, "| 生成时间 (UTC) | %s |\n", orDash(hdr.timestamp))
	fmt.Fprintf(w, "| Go 版本 | %s |\n", goVer)
	fmt.Fprintf(w, "| 平台 | %s/%s |\n", goos, goarch)
	fmt.Fprintf(w, "| 逻辑核数 | %d |\n", runtime.NumCPU())
	fmt.Fprintf(w, "| CPU | %s |\n", orDash(hdr.cpu))
	fmt.Fprintf(w, "| 采样次数 (-count) | %s |\n", orDash(hdr.count))
	fmt.Fprintf(w, "| benchtime | %s |\n", orDash(hdr.benchtime))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "> 性能数据与硬件/工具链强相关；跨机器比较须先固定平台与 Go 版本。")

	groups := []string{"PerfOpen", "PerfTraverse", "PerfReplace", "PerfSaveMem", "PerfSaveDisk",
		"PerfPeakHeap", "SavePlanWriteCopyOriginal"}
	for _, op := range groups {
		var rows []*result
		for _, r := range results {
			if o, _ := splitName(r.name); o == op {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			continue
		}
		title, desc := opTitle(op)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "## %s\n\n", title)
		if desc != "" {
			fmt.Fprintf(w, "%s\n\n", desc)
		}
		if op == "PerfPeakHeap" {
			fmt.Fprintln(w, "| 语料 | 输入包 | 堆峰值 | 峰值/输入 | 分配总量/op | 分配次数/op |")
			fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|")
			for _, r := range rows {
				_, deck := splitName(r.name)
				pkgB, _ := r.metricOr("pkg-bytes", true)
				peak, _ := r.metricOr("peak-heap-B", false)
				bOp, _ := r.metricOr("B/op", true)
				allocs, _ := r.metricOr("allocs/op", true)
				ratio := 0.0
				if pkgB > 0 {
					ratio = peak / pkgB
				}
				fmt.Fprintf(w, "| %s | %s | %s | %.2f× | %s | %.0f |\n",
					deck, fmtBytes(pkgB), fmtBytes(peak), ratio, fmtBytes(bOp), allocs)
			}
			continue
		}
		// internal/opc 的 Save 复制基准：没有"输入包"概念（合成媒体），
		// 故不呈现 pkg-bytes 列，改为呈现堆峰值增量。
		if op == "SavePlanWriteCopyOriginal" {
			fmt.Fprintln(w, "| 场景 | p50 | p95 | 分配总量/op | 分配次数/op | 堆峰值增量 |")
			fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|")
			for _, r := range rows {
				_, deck := splitName(r.name)
				bOp, _ := r.metricOr("B/op", true)
				allocs, _ := r.metricOr("allocs/op", true)
				peak, hasPeak := r.metricOr("peak_heap_B", false)
				peakS := "—"
				if hasPeak {
					peakS = fmtBytes(peak)
				}
				p50S, p95S := "—", "—"
				if deck != "PeakHeap" {
					if m, ok := r.metrics["ns/op"]; ok && len(m.vals) > 0 {
						s := append([]float64(nil), m.vals...)
						sort.Float64s(s)
						p50S = fmtDur(percentile(s, 0.50))
						p95S = fmtDur(percentile(s, 0.95))
					}
				}
				fmt.Fprintf(w, "| %s | %s | %s | %s | %.0f | %s |\n",
					deck, p50S, p95S, fmtBytes(bOp), allocs, peakS)
			}
			continue
		}
		fmt.Fprintln(w, "| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |")
		fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|")
		for _, r := range rows {
			_, deck := splitName(r.name)
			pkgB, _ := r.metricOr("pkg-bytes", true)
			bOp, _ := r.metricOr("B/op", true)
			allocs, _ := r.metricOr("allocs/op", true)
			p50, p95 := 0.0, 0.0
			if m, ok := r.metrics["ns/op"]; ok && len(m.vals) > 0 {
				s := append([]float64(nil), m.vals...)
				sort.Float64s(s)
				p50 = percentile(s, 0.50)
				p95 = percentile(s, 0.95)
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %.0f |\n",
				deck, fmtBytes(pkgB), fmtDur(p50), fmtDur(p95), fmtBytes(bOp), allocs)
		}
	}
	fmt.Fprintln(w)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// normalizeGoVersion 把 `go version go1.27.0 windows/amd64` 归一为 `go1.27.0`；
// 无法识别时退回运行时版本。
func normalizeGoVersion(s string) string {
	fields := strings.Fields(s)
	for _, f := range fields {
		if strings.HasPrefix(f, "go1") || strings.HasPrefix(f, "go2") {
			return f
		}
	}
	return runtime.Version()
}

// fmtDur 把纳秒时长格式化为易读单位。
func fmtDur(ns float64) string {
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.3f s", ns/1e9)
	case ns >= 1e6:
		return fmt.Sprintf("%.3f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.2f µs", ns/1e3)
	default:
		return fmt.Sprintf("%.0f ns", ns)
	}
}

// fmtBytes 把字节数格式化为 KiB/MiB。
func fmtBytes(b float64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.2f MiB", b/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", b/(1<<10))
	default:
		return fmt.Sprintf("%.0f B", b)
	}
}
