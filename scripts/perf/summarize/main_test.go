package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseBenchLine(t *testing.T) {
	res, err := parseBenchLine("BenchmarkPerfOpen/s001-text-8 10 1234 ns/op 2048 B/op 7 allocs/op 4096 pkg-bytes 8192 peak-heap-B")
	if err != nil {
		t.Fatal(err)
	}
	if res.name != "BenchmarkPerfOpen/s001-text-8" {
		t.Fatalf("name = %q", res.name)
	}
	if got := res.metrics["ns/op"].vals[0]; got != 1234 {
		t.Fatalf("ns/op = %v", got)
	}
	if _, err := parseBenchLine("BenchmarkBad 10 not-a-number ns/op"); err == nil {
		t.Fatal("expected bad value error")
	}
}

func TestMergeAndAggregateMetrics(t *testing.T) {
	a, err := parseBenchLine("BenchmarkPerfSaveMem/deck-8 1 100 ns/op 10 B/op 1 allocs/op 1000 peak-heap-B")
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseBenchLine("BenchmarkPerfSaveMem/deck-8 1 300 ns/op 30 B/op 3 allocs/op 2000 peak-heap-B")
	if err != nil {
		t.Fatal(err)
	}
	mergeMetrics(a, b)
	if got, ok := a.metricOr("B/op", true); !ok || got != 20 {
		t.Fatalf("median B/op = %v %v, want 20 true", got, ok)
	}
	if got, ok := a.metricOr("peak-heap-B", false); !ok || got != 2000 {
		t.Fatalf("max peak = %v %v, want 2000 true", got, ok)
	}
	if got := percentile([]float64{1, 2, 3, 4}, 0.95); got != 4 {
		t.Fatalf("p95 = %v, want 4", got)
	}
}

func TestRunWritesReport(t *testing.T) {
	input := strings.Join([]string{
		"# timestamp: 2026-09-11T00:00:00Z",
		"# count: 2",
		"# benchtime: 100ms",
		"# go-version: go version go1.27.0 linux/amd64",
		"goos: linux",
		"goarch: amd64",
		"cpu: test-cpu",
		"pkg: github.com/F31/go-pptx",
		"BenchmarkPerfOpen/s001-text-8 1 1000 ns/op 1024 B/op 2 allocs/op 4096 pkg-bytes",
		"BenchmarkPerfOpen/s001-text-8 1 2000 ns/op 2048 B/op 4 allocs/op 4096 pkg-bytes",
		"BenchmarkPerfPeakHeap/s001-text-8 1 999 ns/op 3072 B/op 6 allocs/op 4096 pkg-bytes 8192 peak-heap-B",
	}, "\n")
	var out bytes.Buffer
	if err := run(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	for _, want := range []string{
		"# go-pptx 性能基线报告",
		"| Go 版本 | go1.27.0 |",
		"## 打开（BenchmarkPerfOpen）",
		"| s001-text | 4.0 KiB | 1.00 µs | 2.00 µs | 1.5 KiB | 3 |",
		"## 全流程峰值内存（BenchmarkPerfPeakHeap）",
		"| s001-text | 4.0 KiB | 8.0 KiB | 2.00× | 3.0 KiB | 6 |",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q\n%s", want, report)
		}
	}
}

func TestRunWritesSavePlanSection(t *testing.T) {
	input := strings.Join([]string{
		"# timestamp: 2026-09-12T00:00:00Z",
		"# count: 1",
		"# go-version: go version go1.27.0 windows/amd64",
		"goos: windows",
		"goarch: amd64",
		"pkg: github.com/F31/go-pptx/internal/opc",
		"BenchmarkSavePlanWriteCopyOriginal/3x8MiB-24 20 6428050 ns/op 206296 B/op 148 allocs/op",
		"BenchmarkSavePlanWriteCopyOriginal/PeakHeap-24 20 6974470 ns/op 3189984 peak_heap_B 319030 B/op 150 allocs/op",
	}, "\n")
	var out bytes.Buffer
	if err := run(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	for _, want := range []string{
		"## 保存·未变 Part 复制（internal/opc，ADR-018）",
		"| 3x8MiB | 6.428 ms | 6.428 ms | 201.5 KiB | 148 | — |",
		"| PeakHeap | — | — | 311.6 KiB | 150 | 3.04 MiB |",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q\n%s", want, report)
		}
	}
}

func TestRunNoBenchmarks(t *testing.T) {
	var out bytes.Buffer
	err := run(strings.NewReader("PASS\n"), &out)
	if err == nil || !strings.Contains(err.Error(), "no benchmark lines") {
		t.Fatalf("err = %v, want no benchmark lines", err)
	}
}

func TestFormattingHelpers(t *testing.T) {
	if op, deck := splitName("BenchmarkPerfReplace/s003-image-16"); op != "PerfReplace" || deck != "s003-image" {
		t.Fatalf("splitName = %q %q", op, deck)
	}
	if got := normalizeGoVersion("go version go1.27.0 windows/amd64"); got != "go1.27.0" {
		t.Fatalf("normalizeGoVersion = %q", got)
	}
	if got := fmtDur(2_500_000); got != "2.500 ms" {
		t.Fatalf("fmtDur = %q", got)
	}
	if got := fmtBytes(2 << 20); got != "2.00 MiB" {
		t.Fatalf("fmtBytes = %q", got)
	}
}
