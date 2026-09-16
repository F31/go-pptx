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
# 瘦身选项（体积敏感：浏览器需下载整个 .wasm）：
#   -trimpath 去掉本地绝对路径（同时消除构建机路径泄漏）
#   -ldflags="-s -w" 去掉符号表与 DWARF 调试信息
# 仅用于发布/分发构建；本地排错时可加 SLIM=0 保留调试信息。
SLIM="${SLIM:-1}"
if [[ "$SLIM" == "1" ]]; then
  LDFLAGS=(-trimpath -ldflags="-s -w")
else
  LDFLAGS=()
fi

echo "[check_wasm] building cmd/pptx_check → $OUT/pptx_check.wasm (slim=$SLIM)"
CGO_ENABLED=0 GOOS=js GOARCH=wasm go build "${LDFLAGS[@]}" -o "$OUT/pptx_check.wasm" ./cmd/pptx_check

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

Tip: enable gzip/brotli on the server for pptx_check.wasm (~6 MB → ~1.5 MB
over the wire; the -s -w strip above only saves ~2% of the raw size, transport
compression is what actually matters).

Then open http://localhost:8080/check.html in a modern browser.

Note: file:// may block fetch(); use a static server for tests.
EOF
