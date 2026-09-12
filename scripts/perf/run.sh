#!/usr/bin/env sh
# PERF-01 性能基线驱动脚本（设计方案 §15.3 / 实施计划 §9.3）。
#
# 用法（在仓库根执行）：
#   scripts/perf/run.sh
#
# 环境变量：
#   COUNT      采样次数，用于 p50/p95（默认 10）
#   BENCH      基准筛选正则（默认 BenchmarkPerf）
#   BENCHTIME  benchtime 覆盖（默认空，由框架自动定标）
#   TIMEOUT    go test -timeout（默认 30m；COUNT 大或 runner 慢时上调）
#   RAW        原始日志路径（默认 perf-out/raw-bench.log，gitignore 不入库）
#   OUT        报告输出路径（默认 docs/PERF-01-benchmark-report.md）
#
# 产物：
#   - RAW：含环境头 + `go test -bench` 原始输出，本地运行日志（perf-out/
#     已 gitignore，避免每次重跑污染 git status；聚合证据以 OUT 报告为准）；
#   - OUT：由 scripts/perf/summarize 聚合的 markdown 报告。
set -eu

COUNT="${COUNT:-10}"
BENCH="${BENCH:-BenchmarkPerf}"
BENCHTIME="${BENCHTIME:-}"
TIMEOUT="${TIMEOUT:-30m}"
RAW="${RAW:-perf-out/raw-bench.log}"
OUT="${OUT:-docs/PERF-01-benchmark-report.md}"

mkdir -p "$(dirname "$RAW")" "$(dirname "$OUT")"

STAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
{
	printf '# go-pptx PERF-01 raw benchmark log\n'
	printf '# timestamp: %s\n' "$STAMP"
	printf '# go-version: %s\n' "$(go version)"
	printf '# count: %s\n' "$COUNT"
	printf '# benchtime: %s\n' "${BENCHTIME:-default}"
	printf '# timeout: %s\n' "$TIMEOUT"
	printf '# bench: %s\n' "$BENCH"
} > "$RAW"

set +e
if [ -n "$BENCHTIME" ]; then
	go test -run '^$' -bench "$BENCH" -benchmem -count "$COUNT" -benchtime="$BENCHTIME" -timeout "$TIMEOUT" . >> "$RAW" 2>&1
else
	go test -run '^$' -bench "$BENCH" -benchmem -count "$COUNT" -timeout "$TIMEOUT" . >> "$RAW" 2>&1
fi
STATUS=$?
set -e

go run ./scripts/perf/summarize < "$RAW" > "$OUT"
printf 'raw log: %s\n' "$RAW"
printf 'report : %s\n' "$OUT"

exit "$STATUS"
