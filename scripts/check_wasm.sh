#!/usr/bin/env bash
# TOOL-02 (WAS-02) 构建脚本：编译 pptx_check.wasm 并把 wasm_exec.js 一并
# 拷贝到 wasm/site/。该目录可直接由静态 HTTP server（或 file://）托
# 管检查工具页面，PPT 完全不离开用户设备。
#
# 用法：
#   ./scripts/check_wasm.sh                 # 默认构建到 ./wasm/site/
#   OUT=/path/to/site ./scripts/check_wasm.sh
#
# 依赖：GOROOT 内的 wasm_exec.js（Go 标准库自带的浏览器 runtime）。
set -euo pipefail

cd "$(dirname "$0")/.."

OUT="${OUT:-./wasm/site}"
mkdir -p "$OUT"

# 1) 编译 WASM。
echo "[check_wasm] building cmd/pptx_check → $OUT/pptx_check.wasm"
CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -o "$OUT/pptx_check.wasm" ./cmd/pptx_check

# 2) 复制 wasm_exec.js 到站点目录。
if [[ -z "${GOROOT:-}" ]]; then
  GOROOT="$(go env GOROOT)"
fi
if [[ ! -f "$GOROOT/lib/wasm/wasm_exec.js" ]]; then
  echo "[check_wasm] error: wasm_exec.js not found at $GOROOT/lib/wasm/"
  exit 1
fi

echo "[check_wasm] copying wasm_exec.js → $OUT/wasm_exec.js"
cp -f "$GOROOT/lib/wasm/wasm_exec.js" "$OUT/wasm_exec.js"

# 3) 提示用法。
cat <<EOF

[check_wasm] build complete:
  $(ls -la "$OUT/pptx_check.wasm" | awk '{print $9, $5}')
  $(ls -la "$OUT/wasm_exec.js" | awk '{print $9, $5}')

Serve the site directory with any static HTTP server:
  cd "$OUT" && python -m http.server 8080
Then open http://localhost:8080/check.html in a modern browser.

Note: file:// may block fetch(); use a static server for tests.
EOF
