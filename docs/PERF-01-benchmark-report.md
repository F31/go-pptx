# go-pptx 性能基线报告（PERF-01）

> 本文件由 `scripts/perf/summarize` 从 `go test -bench` 原始日志自动生成，请勿手工编辑。
> 指标口径与限制见 `docs/PERF-01-性能基线.md`（设计方案 §15.3 / 实施计划 §9.3）。

## 环境

| 项 | 值 |
|---|---|
| 生成时间 (UTC) | 2026-09-10T08:17:03Z |
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
| 10p-text | 14.1 KiB | 1.104 ms | 1.190 ms | 145.9 KiB | 1440 |
| 50p-image | 59.9 KiB | 3.929 ms | 4.187 ms | 611.5 KiB | 6092 |
| 100p-media | 33.90 MiB | 3.121 ms | 8.012 ms | 1.22 MiB | 12334 |

## 遍历（BenchmarkPerfTraverse）

打开一次后反复枚举全部页/形状/段落文本。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 875.10 µs | 921.40 µs | 277.7 KiB | 2824 |
| 50p-image | 59.9 KiB | 9.405 ms | 10.069 ms | 5.67 MiB | 51172 |
| 100p-media | 33.90 MiB | 31.330 ms | 32.006 ms | 21.66 MiB | 193899 |

## 单处替换（BenchmarkPerfReplace）

首页文本框内唯一标记的可逆替换（每轮恰好 1 处命中）。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 40.79 µs | 41.46 µs | 38.0 KiB | 256 |
| 50p-image | 59.9 KiB | 47.71 µs | 54.86 µs | 41.5 KiB | 324 |
| 100p-media | 33.90 MiB | 59.19 µs | 61.46 µs | 66.0 KiB | 394 |

## 保存·内存序列化（BenchmarkPerfSaveMem）

Write → io.Discard，不含文件系统。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 2.967 ms | 3.190 ms | 100.6 KiB | 853 |
| 50p-image | 59.9 KiB | 11.113 ms | 11.631 ms | 398.3 KiB | 3090 |
| 100p-media | 33.90 MiB | 100.193 ms | 105.920 ms | 84.50 MiB | 6912 |

## 保存·落盘原子替换（BenchmarkPerfSaveDisk）

Save 到临时路径，含原子替换与文件系统开销。

| 语料 | 输入包 | p50 | p95 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 5.751 ms | 6.311 ms | 122.2 KiB | 1047 |
| 50p-image | 59.9 KiB | 14.928 ms | 15.467 ms | 470.8 KiB | 3690 |
| 100p-media | 33.90 MiB | 155.177 ms | 163.444 ms | 84.83 MiB | 8118 |

## 全流程峰值内存（BenchmarkPerfPeakHeap）

Open → 遍历 → Write 的堆分配峰值；采样会 STW，故不呈现 ns/op。

| 语料 | 输入包 | 堆峰值 | 峰值/输入 | 分配总量/op | 分配次数/op |
|---|---:|---:|---:|---:|---:|
| 10p-text | 14.1 KiB | 1.93 MiB | 140.16× | 867.4 KiB | 7374 |
| 50p-image | 59.9 KiB | 10.41 MiB | 177.93× | 8.51 MiB | 74671 |
| 100p-media | 33.90 MiB | 59.67 MiB | 1.76× | 112.00 MiB | 242587 |

