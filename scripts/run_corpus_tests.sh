#!/usr/bin/env bash
# scripts/run_corpus_tests.sh — 本地 CORPUS-01 公开样本金样 replay 三段式：
#
#   1. 校验 testdata/corpus 下所有 manifest.json / actions.json 约定一致
#      （scripts/gen_corpus/run.sh validate）。
#   2. 跑 -tags=corpus 测试套件 + 详细日志到 corpus-replay.log。
#   3. 抽取 PASS/SKIP/FAIL 计数 + 退出码，便于本地 dev 与 CI 共同使用。
#
# 用法：
#   scripts/run_corpus_tests.sh                # 默认 testdata/corpus
#   scripts/run_corpus_tests.sh /path/to/dir   # 自定义语料根目录
#
# 退出码：
#   0 — 校验通过且测试无 FAIL（PASS + SKIP 都视为 OK；SKIP 来自私有样本无源）
#   1 — manifest 校验失败或测试 FAIL
#   2 — 环境异常（缺 go / 缺 gen_corpus 脚本）

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CORPUS_ROOT="${1:-$ROOT/testdata/corpus}"
LOG="$ROOT/corpus-replay.log"

# Windows + Git Bash 下 python3.exe 不识别 POSIX 路径（/e/projects/...），
# 需要 cygpath -w 转成 Win32 路径再传入。Linux/macOS 上 cygpath 不存在，
# 直接用 POSIX 路径。
to_win_path() {
    if command -v cygpath >/dev/null 2>&1; then
        cygpath -w "$1"
    else
        printf '%s' "$1"
    fi
}
CORPUS_ROOT_WIN="$(to_win_path "$CORPUS_ROOT")"

cd "$ROOT"

# 1. manifest 校验
# 直接调 corpus.py（绕开 scripts/gen_corpus/run.sh，避免跨平台路径适配
# 耦合到 opencode 维护的工具脚本）。Windows + Git Bash 下 python3.exe
# 不识别 POSIX 路径（/e/projects/...），用 cygpath -w 转 Win32。
PY_SCRIPT="$ROOT/scripts/gen_corpus/corpus.py"
if [[ ! -f "$PY_SCRIPT" ]]; then
    echo "FAIL: scripts/gen_corpus/corpus.py not found" >&2
    exit 2
fi
PY_SCRIPT_WIN="$(to_win_path "$PY_SCRIPT")"
echo "==> 1/3 validate manifests in $CORPUS_ROOT_WIN"
if ! python3 "$PY_SCRIPT_WIN" validate "$CORPUS_ROOT_WIN"; then
    echo "FAIL: corpus manifest validation failed" >&2
    exit 1
fi

# 2. 跑 corpus 测试
if ! command -v go >/dev/null 2>&1; then
    echo "FAIL: go toolchain not found in PATH" >&2
    exit 2
fi
echo "==> 2/3 go test -tags=corpus -v ./... (log: $LOG)"
if ! go test -tags=corpus -v ./... | tee "$LOG"; then
    # tee 已把测试输出复制到 stdout；保留测试退出码。
    TEST_RC=${PIPESTATUS[0]}
    echo "FAIL: go test exited $TEST_RC" >&2
    echo "FAIL: corpus-replay summary follows" >&2
    echo "----" >&2
    grep -E '^(=== RUN|--- (PASS|FAIL|SKIP):|PASS|FAIL|ok|FAIL)' "$LOG" | tail -40 >&2
    exit 1
fi

# 3. 抽取统计
echo "==> 3/3 summary"
pass=$(grep -cE '^    --- PASS:' "$LOG" || true)
fail=$(grep -cE '^    --- FAIL:' "$LOG" || true)
skip=$(grep -cE '^    --- SKIP:' "$LOG" || true)
echo "    PASS: $pass"
echo "    SKIP: $skip"
echo "    FAIL: $fail"

if (( fail > 0 )); then
    echo "FAIL: $fail failing subtest(s) in corpus-replay" >&2
    exit 1
fi
echo "OK: corpus-replay all green ($pass pass, $skip skip)"