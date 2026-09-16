#!/usr/bin/env bash
# 覆盖率门槛守门（COV-01..04）——各包契约写成断言，进 CI 常态执行。
#
# 背景（ADR-018 Tier 2 回归）：`tryRawCopyOriginal` 的 4 条安全门限拒绝分支
# 落地时零测试覆盖，使 `internal/opc` 从 90.4% **静默跌到 89.5%**，跌破
# COV-04 的 90% 门槛**三天无人察觉**——因为 CI 当时只跑 build/test/vet，
# 从未跑覆盖率。本脚本把门槛固化为断言，让这类回归在 CI 立即变红。
#
# 依据 docs/coverage-roadmap.md：
#   COV-02  root（root SDK）           >= 82%   （85% 已决策推迟）
#   COV-03  command/helper 三包         >= 85%
#   COV-04  低层格式包                  >= 90%   （audioprobe 依 B-2 决策豁免 90）
#   ir / render / chart                 行为优先，门槛取防回归下界
#
# 完整性校验：`go list ./...` 中每个包必须出现在 FLOORS（百分比门槛）或
# SKIP（无可测语句 / 一次性工具包，须注明理由）。否则 FAIL——堵住「新包静默
# 不受检」（v2.0 试点期曾三次新增包后忘加门槛）。
#
# 用法：
#   scripts/coverage/gate.sh
#   TOLERANCE=0.5 scripts/coverage/gate.sh     # 放宽测量容差（默认 0）
#
# 退出码：0=全部达标；1=有包跌破门槛、有门槛包缺失、或有包未登记。
set -u

TOLERANCE="${TOLERANCE:-0.0}"
GOFLAGS_TAGS="${GOFLAGS_TAGS:-}"   # 可选：传 "corpus" 走 corpus 口径

# 包=门槛（%）—— 与 docs/coverage-roadmap.md 的门槛表一一对应。
FLOORS=(
  "github.com/F31/go-pptx=82"
  "github.com/F31/go-pptx/cmd/pptx=85"
  "github.com/F31/go-pptx/wasm/check=85"
  "github.com/F31/go-pptx/scripts/perf/summarize=85"
  "github.com/F31/go-pptx/internal/opc=90"
  "github.com/F31/go-pptx/internal/xmlstore=90"
  "github.com/F31/go-pptx/internal/videoprobe=90"
  "github.com/F31/go-pptx/internal/textmap=90"
  "github.com/F31/go-pptx/internal/editplan=90"
  "github.com/F31/go-pptx/internal/audioprobe=86"
  "github.com/F31/go-pptx/internal/chart=90"
  "github.com/F31/go-pptx/ir=85"
  "github.com/F31/go-pptx/internal/textutil=90"
  "github.com/F31/go-pptx/internal/bind=90"
  "github.com/F31/go-pptx/internal/style=90"
  "github.com/F31/go-pptx/render=84"
)

# 显式豁免：无需百分比门槛的包（须注明理由，完整性校验据此放行）。
SKIP=(
  "github.com/F31/go-pptx/internal/document"  # 纯接口/类型声明，无可测语句
  "github.com/F31/go-pptx/internal/ooxmlns"   # 纯命名空间常量，无可测语句
  "github.com/F31/go-pptx/scripts/gen_audio"  # 一次性生成工具（package main）
  "github.com/F31/go-pptx/scripts/gen_media"  # 一次性生成工具（package main）
)

tags_flag=()
if [ -n "$GOFLAGS_TAGS" ]; then
  tags_flag=(-tags="$GOFLAGS_TAGS")
fi

echo "== go test -cover ${tags_flag[*]:-} ./... =="
RAW=$(CGO_ENABLED=0 go test "${tags_flag[@]}" -cover ./... 2>&1)
RC=$?
if [ "$RC" -ne 0 ]; then
  echo "go test failed (rc=$RC):"
  echo "$RAW"
  exit 1
fi

# 解析 "ok  \t<pkg>\t<time>\tcoverage: X% of statements"。
declare -A COV
while IFS= read -r line; do
  pkg=$(printf '%s' "$line" | awk '{print $2}')
  cov=$(printf '%s' "$line" | grep -oE 'coverage: [0-9.]+' | awk '{print $2}')
  if [ -n "$pkg" ] && [ -n "$cov" ]; then
    COV["$pkg"]="$cov"
  fi
done < <(printf '%s\n' "$RAW" | grep -E '^ok[[:space:]]')

fail=0

# ── 完整性校验：go list ./... 中每个包必须出现在 FLOORS 或 SKIP ──────────
declare -A KNOWN
for entry in "${FLOORS[@]}"; do KNOWN["${entry%=*}"]=1; done
for pkg in "${SKIP[@]}"; do KNOWN["$pkg"]=1; done

missing=()
while IFS= read -r pkg; do
  [ -n "$pkg" ] || continue
  [ -n "${KNOWN[$pkg]:-}" ] || missing+=("$pkg")
done < <(go list ./... 2>/dev/null)

if [ "${#missing[@]}" -gt 0 ]; then
  echo
  echo "COVERAGE GATE: 以下包既未列入 FLOORS 也未登记 SKIP（静默不受检）："
  printf '  - %s\n' "${missing[@]}"
  echo "  请加入 FLOORS（有百分比门槛）或 SKIP（无可测语句/工具包，须注明理由）。"
  fail=1
fi

printf '%-46s %9s %9s  %s\n' "package" "cover" "floor" "status"
printf '%-46s %9s %9s  %s\n' "----------------------------------------------" "---------" "---------" "------"
for entry in "${FLOORS[@]}"; do
  pkg="${entry%=*}"
  floor="${entry##*=}"
  cov="${COV[$pkg]:-}"
  if [ -z "$cov" ]; then
    printf '%-46s %9s %8s%%  %s\n' "$pkg" "MISSING" "$floor" "FAIL"
    fail=1
    continue
  fi
  ok=$(awk -v c="$cov" -v f="$floor" -v tol="$TOLERANCE" 'BEGIN{print (c+tol>=f)?"1":"0"}')
  if [ "$ok" = "1" ]; then
    printf '%-46s %8s%% %8s%%  %s\n' "$pkg" "$cov" "$floor" "ok"
  else
    printf '%-46s %8s%% %8s%%  %s\n' "$pkg" "$cov" "$floor" "FAIL"
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo
  echo "COVERAGE GATE FAILED (tolerance=$TOLERANCE)."
  echo "详见 docs/coverage-roadmap.md 的门槛表；若为合理下移请同步更新门槛与文档。"
  exit 1
fi
echo
echo "COVERAGE GATE PASSED (tolerance=$TOLERANCE)."
