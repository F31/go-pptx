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
# 用法：
#   scripts/coverage/gate.sh
#   TOLERANCE=0.5 scripts/coverage/gate.sh     # 放宽测量容差（默认 0）
#
# 退出码：0=全部达标；1=有包跌破门槛或有门槛包缺失。
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
  "github.com/F31/go-pptx/internal/textutil=82"
  "github.com/F31/go-pptx/internal/bind=90"
  "github.com/F31/go-pptx/render=84"
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
