# TOOL-02 (WAS-02) 构建脚本（PowerShell）：编译 pptx_check.wasm 并把
# wasm_exec.js 一并拷贝到 wasm/site/。该目录可由任意静态 HTTP 服务器
# （或 file://）托管，PPT 完全不离开用户设备。
#
# 用法（仓库根目录执行）：
#   pwsh -File scripts/check_wasm.ps1
#   $env:OUT='E:/custom/site'; pwsh -File scripts/check_wasm.ps1
#
# 依赖：GOROOT 内的 wasm_exec.js（Go 标准库自带的浏览器 runtime）。
[CmdletBinding()]
param(
    [string]$Out = "$PSScriptRoot\..\wasm\site"
)

$ErrorActionPreference = 'Stop'

Set-Location (Join-Path $PSScriptRoot '..')
New-Item -ItemType Directory -Path $Out -Force | Out-Null

# 1) 编译 WASM。
Write-Host "[check_wasm] building cmd/pptx_check -> $Out\pptx_check.wasm"
$env:CGO_ENABLED = '0'
$env:GOOS = 'js'
$env:GOARCH = 'wasm'
try {
    go build -o (Join-Path $Out 'pptx_check.wasm') .\cmd\pptx_check
}
finally {
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}

# 2) 复制 wasm_exec.js 到站点目录。
if (-not $env:GOROOT) {
    $env:GOROOT = go env GOROOT
}
$wasmExecSrc = Join-Path $env:GOROOT 'lib\wasm\wasm_exec.js'
if (-not (Test-Path $wasmExecSrc)) {
    Write-Host "[check_wasm] error: wasm_exec.js not found at $wasmExecSrc" -ForegroundColor Red
    exit 1
}

Write-Host "[check_wasm] copying wasm_exec.js -> $Out\wasm_exec.js"
Copy-Item -Path $wasmExecSrc -Destination (Join-Path $Out 'wasm_exec.js') -Force

# 3) 提示用法。
$wasmFile = Get-Item (Join-Path $Out 'pptx_check.wasm')
$execFile = Get-Item (Join-Path $Out 'wasm_exec.js')
Write-Host ""
Write-Host "[check_wasm] build complete:"
Write-Host ("  {0,-60} {1,12}" -f $wasmFile.FullName, $wasmFile.Length)
Write-Host ("  {0,-60} {1,12}" -f $execFile.FullName, $execFile.Length)
Write-Host ""
Write-Host "Serve the site directory with any static HTTP server:"
Write-Host ("  cd {0}; python -m http.server 8080" -f $Out)
Write-Host "Then open http://localhost:8080/check.html in a modern browser."
Write-Host "Note: file:// may block fetch(); use a static server for tests."
