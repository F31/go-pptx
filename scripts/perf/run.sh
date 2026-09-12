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
#   OPC_BENCH       internal/opc Save 复制基准筛选（默认 BenchmarkSavePlanWriteCopyOriginal）
#   OPC_BENCHTIME   该组 benchtime（默认 20x，固定迭代数便于跨机比较）
#   OPC_COUNT       该组采样次数（默认 3）
#   SKIP_OPC        设为 1 时跳过 internal/opc 组（根包不可用等场合）
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
OPC_BENCH="${OPC_BENCH:-BenchmarkSavePlanWriteCopyOriginal}"
OPC_BENCHTIME="${OPC_BENCHTIME:-20x}"
OPC_COUNT="${OPC_COUNT:-3}"
SKIP_OPC="${SKIP_OPC:-0}"

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
	printf '# opc-bench: %s (benchtime=%s, count=%s)\n' "$OPC_BENCH" "$OPC_BENCHTIME" "$OPC_COUNT"
} > "$RAW"

set +e
if [ -n "$BENCHTIME" ]; then
	go test -run '^$' -bench "$BENCH" -benchmem -count "$COUNT" -benchtime="$BENCHTIME" -timeout "$TIMEOUT" . >> "$RAW" 2>&1
else
	go test -run '^$' -bench "$BENCH" -benchmem -count "$COUNT" -timeout "$TIMEOUT" . >> "$RAW" 2>&1
fi
STATUS=$?

# ADR-018 收益锚点：internal/opc 的「未变 Part 复制」基准（分配 / 峰值堆）。
# 该组不依赖根包，根包暂不可编译时可用 SKIP_OPC=1 之外的组合单独观察。
if [ "$SKIP_OPC" != "1" ]; then
	printf '# --- internal/opc save copy bench (ADR-018) ---\n' >> "$RAW"
	go test -run '^$' -bench "$OPC_BENCH" -benchmem -count "$OPC_COUNT" \
		-benchtime="$OPC_BENCHTIME" -timeout "$TIMEOUT" ./internal/opc/ >> "$RAW" 2>&1
	OPC_STATUS=$?
	if [ "$STATUS" -eq 0 ]; then
		STATUS="$OPC_STATUS"
	fi
fi
set -e

go run ./scripts/perf/summarize < "$RAW" > "$OUT"
printf 'raw log: %s\n' "$RAW"
printf 'report : %s\n' "$OUT"

exit "$STATUS"
