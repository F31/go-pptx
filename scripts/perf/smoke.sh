#!/usr/bin/env sh
# PERF-01 冒烟守门（确定性，不含任何时序/内存阈值断言）。
#
# 用途：在 CI（每次 push/PR）与本地快速验证 PERF-01 交付物仍然完好。共享
# runner 的耗时抖动大，拿 ns/op 做门禁必然 flaky，故本脚本只验三类**确定性**
# 事实：
#
#   1. 基准套件可编译，且 6 组基准 × 3 档语料、以及 internal/opc 的
#      Save 复制锚点基准（4 个子基准）全部实际执行（防基准被误删/改名/
#      静默跳过）；
#   2. scripts/perf/summarize 能解析当前工具链的 `go test -bench` 输出
#      （防未来 Go 版本变更 bench 行格式导致报告生成器静默失效）；
#   3. 三档语料的输入包字节数（pkg-bytes）落在预期下界之上——其中
#      100p-media 的下界专门守住「大媒体被媒体内容哈希去重合并」这类
#      使语料名不副实的静默回归（见方法论文档 §2）。
#
# 阈值取「下界 + 宽裕余量」而非等值：PNG 编码输出可能随 Go 版本微变，
# 等值断言会误报；下界足以捕获量级坍塌（如 34 MiB → 885 KiB）。
#
# 用法（仓库根执行）：
#   scripts/perf/smoke.sh
#
# 环境变量：
#   RAW  原始日志路径（默认临时文件）。CI 传仓库内相对路径以便归档。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

RAW="${RAW:-}"
if [ -z "$RAW" ]; then
	RAW="$(mktemp "${TMPDIR:-/tmp}/go-pptx-perf-smoke.XXXXXX.log")"
fi

printf 'perf-smoke: running benchmark suite once (-benchtime=1x)\n'
status=0
go test -run '^$' -bench 'BenchmarkPerf' -benchmem -benchtime=1x -timeout 10m ./ >"$RAW" 2>&1 || status=$?
if [ "$status" -ne 0 ]; then
	printf 'perf-smoke: benchmark run FAILED (exit %s); log: %s\n' "$status" "$RAW" >&2
	tail -n 40 "$RAW" >&2 || true
	exit "$status"
fi

# ① 6 组基准 × 3 档语料全部执行。
missing=0
for group in PerfOpen PerfTraverse PerfReplace PerfSaveMem PerfSaveDisk PerfPeakHeap; do
	for deck in 10p-text 50p-image 100p-media; do
		if ! grep -q "^Benchmark${group}/${deck}" "$RAW"; then
			printf 'perf-smoke: missing benchmark %s/%s\n' "$group" "$deck" >&2
			missing=1
		fi
	done
done
if [ "$missing" -ne 0 ]; then
	exit 1
fi
# ①b ADR-018 收益锚点（internal/opc）：4 个子基准全部执行，且其输出一并喂给
#     报告生成器（防 Save 复制基准被误删/改名，导致分配回归无人看守）。
#     该组不依赖根包，独立一次 go test。
status=0
go test -run '^$' -bench 'BenchmarkSavePlanWriteCopyOriginal' -benchmem -benchtime=1x \
	-timeout 10m ./internal/opc/ >>"$RAW" 2>&1 || status=$?
if [ "$status" -ne 0 ]; then
	printf 'perf-smoke: opc save-copy benchmark FAILED (exit %s); log: %s\n' "$status" "$RAW" >&2
	tail -n 40 "$RAW" >&2 || true
	exit "$status"
fi
for sub in 1x1MiB 4x1MiB 3x8MiB PeakHeap; do
	if ! grep -q "^BenchmarkSavePlanWriteCopyOriginal/${sub}" "$RAW"; then
		printf 'perf-smoke: missing benchmark SavePlanWriteCopyOriginal/%s\n' "$sub" >&2
		missing=1
	fi
done
if [ "$missing" -ne 0 ]; then
	exit 1
fi
printf 'perf-smoke: 6 groups x 3 decks + opc save-copy 4 sub-benches executed OK\n'

# ② 报告生成器可解析当前工具链输出（解析失败会非零退出）。
report="$(mktemp "${TMPDIR:-/tmp}/go-pptx-perf-smoke.XXXXXX.md")"
go run ./scripts/perf/summarize <"$RAW" >"$report"
# 锚点一律用 ASCII 子串/正则，避免不同 locale 下 grep 对中文的字节处理差异。
# （报告正文的表格列名是中文，故只用生成器横幅、环境表与字节格式化单元做锚点。）
grep -q 'scripts/perf/summarize' "$report" || {
	printf 'perf-smoke: report missing generator banner: %s\n' "$report" >&2
	exit 1
}
grep -q '(UTC)' "$report" || {
	printf 'perf-smoke: report missing environment table: %s\n' "$report" >&2
	exit 1
}
grep -Eq '[0-9]+(\.[0-9]+)? (MiB|KiB|B)' "$report" || {
	printf 'perf-smoke: report has no formatted byte cells: %s\n' "$report" >&2
	exit 1
}
# ADR-018 锚点分组已渲染（ASCII 锚点，避免中文 locale 差异）。
grep -q 'ADR-018' "$report" || {
	printf 'perf-smoke: report missing ADR-018 save-copy section: %s\n' "$report" >&2
	exit 1
}
printf 'perf-smoke: summarizer parsed current toolchain output OK\n'

# ③ 语料输入尺寸下界（防语料量级坍塌）。
pkg_bytes_of() {
	awk -v pat="^BenchmarkPerfOpen/$1" '
		$0 ~ pat { for (i = 1; i < NF; i++) if ($i == "pkg-bytes") { print $(i - 1); exit } }
	' "$RAW"
}

check_floor() {
	deck="$1"
	floor="$2"
	got="$(pkg_bytes_of "$deck")"
	if [ -z "$got" ]; then
		printf 'perf-smoke: pkg-bytes for %s not found in %s\n' "$deck" "$RAW" >&2
		exit 1
	fi
	if [ "$got" -lt "$floor" ]; then
		printf 'perf-smoke: %s pkg-bytes=%s below floor %s (corpus shrank; media dedup regression?)\n' \
			"$deck" "$got" "$floor" >&2
		exit 1
	fi
	printf 'perf-smoke: %s pkg-bytes=%s (floor %s) OK\n' "$deck" "$got" "$floor"
}

check_floor 10p-text 8192
check_floor 50p-image 32768
check_floor 100p-media 20971520

printf 'perf-smoke: PASS (log: %s)\n' "$RAW"
