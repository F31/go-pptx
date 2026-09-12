# go-pptx 性能基线报告（PERF-01）

> 本文件由 `scripts/perf/summarize` 从 `go test -bench` 原始日志自动生成，请勿手工编辑。
> 指标口径与限制见 `docs/PERF-01-性能基线.md`（设计方案 §15.3 / 实施计划 §9.3）。

## 环境

| 项 | 值 |
|---|---|
| 生成时间 (UTC) | 2026-09-12T00:14:59Z |
| Go 版本 | go1.27.0 |
| 平台 | windows/amd64 |
| 逻辑核数 | 24 |
| CPU | Intel(R) Core(TM) Ultra 9 275HX |
| 采样次数 (-count) | 10 |
| benchtime | default |

> 性能数据与硬件/工具链强相关；跨机器比较须先固定平台与 Go 版本。

## 打开（BenchmarkPerfOpen）

Open 全量解析 + Close。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 1.080 ms | 1.454 ms | 145.8 KiB | 1440 |
| 50p-image | 59.9 KiB | 3.734 ms | 4.007 ms | 611.7 KiB | 6092 |
| 100p-media | 33.90 MiB | 2.026 ms | 2.180 ms | 1.21 MiB | 12334 |

## 遍历（BenchmarkPerfTraverse）

打开一次后反复枚举全部页/形状/段落文本。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 287.82 µs | 308.95 µs | 277.5 KiB | 2823 |
| 50p-image | 59.9 KiB | 3.463 ms | 3.739 ms | 5.66 MiB | 51097 |
| 100p-media | 33.90 MiB | 11.706 ms | 12.948 ms | 21.60 MiB | 193396 |

## 单处替换（BenchmarkPerfReplace）

首页文本框内唯一标记的可逆替换（每轮恰好 1 处命中）。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 16.32 µs | 17.85 µs | 39.4 KiB | 257 |
| 50p-image | 59.9 KiB | 19.41 µs | 20.75 µs | 43.3 KiB | 325 |
| 100p-media | 33.90 MiB | 25.55 µs | 27.28 µs | 68.0 KiB | 395 |

## 保存·内存序列化（BenchmarkPerfSaveMem）

Write → io.Discard，不含文件系统。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 910.25 µs | 969.08 µs | 97.7 KiB | 853 |
| 50p-image | 59.9 KiB | 3.258 ms | 3.716 ms | 387.3 KiB | 3090 |
| 100p-media | 33.90 MiB | 40.277 ms | 42.288 ms | 84.45 MiB | 6908 |

## 保存·落盘原子替换（BenchmarkPerfSaveDisk）

Save 到临时路径，含原子替换与文件系统开销。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 1.984 ms | 2.125 ms | 120.6 KiB | 1047 |
| 50p-image | 59.9 KiB | 4.774 ms | 4.979 ms | 443.4 KiB | 3689 |
| 100p-media | 33.90 MiB | 143.191 ms | 152.388 ms | 84.77 MiB | 8114 |

## 全流程峰值内存（BenchmarkPerfPeakHeap）

Open → 遍历 → Write 的堆分配峰值；采样会 STW，故不呈现 ns/op。

| 语料 | 输入包 | 堆峰值 | 峰值/输入 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 1.90 MiB | 137.77× | 863.3 KiB | 7374 |
| 50p-image | 59.9 KiB | 10.54 MiB | 180.20× | 8.48 MiB | 74670 |
| 100p-media | 33.90 MiB | 62.11 MiB | 1.83× | 112.09 MiB | 242584 |

