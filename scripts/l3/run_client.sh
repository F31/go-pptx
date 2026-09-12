#!/bin/bash
# L3 客户端自动化 runner：在 Start-Job + 超时保护下调用 PowerPoint/WPS COM。
#
# 用法（WSL，Windows 宿主机）：
#   scripts/l3/run_client.sh <ppt|wpp> <src_win_path> <dst_win_path>
#
# 输出：
#   OPEN=ok SLIDES=N SAVE=ok          —— 打开成功（无修复提示）、重存成功
#   RESULT_TIMEOUT_REPAIR_PROMPT      —— Open 在超时窗口内未完成（疑似修复对话框）
#   ERR=...                           —— COM 调用异常
set -u
APP="$1"
SRC="$2"
DST="$3"
ROOT="E:\\projects\\go-pptx"
PS="/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"

WORK="$(dirname "$0")/../.l3-output"
mkdir -p "$WORK"

cat > "$WORK/_run_job.ps1" <<EOF
param([string]\$Src,[string]\$Dst)
\$ErrorActionPreference = 'Stop'
\$job = Start-Job -ScriptBlock {
  param(\$s,\$d)
  & '$ROOT\\scripts\\l3\\ppt_open_resave.ps1' -App '$APP' -Src \$s -Dst \$d
} -ArgumentList \$Src,\$Dst
if (Wait-Job \$job -Timeout 50) {
  Receive-Job \$job
  Remove-Job \$job -Force
} else {
  Stop-Job \$job -Force
  Remove-Job \$job -Force
  Write-Output 'RESULT_TIMEOUT_REPAIR_PROMPT'
}
EOF

"$PS" -NoProfile -ExecutionPolicy Bypass -File "$ROOT\\.l3-output\\_run_job.ps1" -Src "$SRC" -Dst "$DST" 2>&1
