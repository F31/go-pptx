# PERF-01 performance baseline driver (design doc sec 15.3 / plan sec 9.3).
#
# Usage (run from the repository root):
#   pwsh -File scripts/perf/run.ps1
#
# Environment variables:
#   COUNT      sample count for p50/p95 (default 10)
#   BENCH      benchmark filter regex (default BenchmarkPerf)
#   BENCHTIME  benchtime override (default empty = auto calibrate)
#   TIMEOUT    go test -timeout (default 30m; raise for large COUNT / slow runners)
#   RAW        raw log path (default perf-out/raw-bench.log; gitignored, local-only)
#   OUT        report path (default docs/PERF-01-benchmark-report.md)
#   OPC_BENCH      internal/opc save-copy benchmark filter
#                  (default BenchmarkSavePlanWriteCopyOriginal)
#   OPC_BENCHTIME  benchtime for that group (default 20x, fixed iterations)
#   OPC_COUNT      sample count for that group (default 3)
#   SKIP_OPC       set to 1 to skip the internal/opc group
$ErrorActionPreference = 'Stop'

$Count = if ($env:COUNT) { $env:COUNT } else { '10' }
$Bench = if ($env:BENCH) { $env:BENCH } else { 'BenchmarkPerf' }
$Benchtime = if ($env:BENCHTIME) { $env:BENCHTIME } else { '' }
$Timeout = if ($env:TIMEOUT) { $env:TIMEOUT } else { '30m' }
$Raw = if ($env:RAW) { $env:RAW } else { 'perf-out/raw-bench.log' }
$Out = if ($env:OUT) { $env:OUT } else { 'docs/PERF-01-benchmark-report.md' }
$OpcBench = if ($env:OPC_BENCH) { $env:OPC_BENCH } else { 'BenchmarkSavePlanWriteCopyOriginal' }
$OpcBenchtime = if ($env:OPC_BENCHTIME) { $env:OPC_BENCHTIME } else { '20x' }
$OpcCount = if ($env:OPC_COUNT) { $env:OPC_COUNT } else { '3' }
$SkipOpc = if ($env:SKIP_OPC) { $env:SKIP_OPC } else { '0' }

foreach ($p in @($Raw, $Out)) {
    $dir = Split-Path -Parent $p
    if ($dir -and -not (Test-Path $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
}

$stamp = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$goVer = (& go version)
$benchtimeLabel = if ($Benchtime) { $Benchtime } else { 'default' }
$header = @(
    '# go-pptx PERF-01 raw benchmark log',
    "# timestamp: $stamp",
    "# go-version: $goVer",
    "# count: $Count",
    "# benchtime: $benchtimeLabel",
    "# timeout: $Timeout",
    "# bench: $Bench",
    "# opc-bench: $OpcBench (benchtime=$OpcBenchtime, count=$OpcCount)"
)
Set-Content -Path $Raw -Value $header -Encoding UTF8

$testArgs = @('-run', '^$', '-bench', $Bench, '-benchmem', '-count', $Count, '-timeout', $Timeout)
if ($Benchtime) { $testArgs += @('-benchtime', $Benchtime) }
$testArgs += '.'

& go test @testArgs 2>&1 | Add-Content -Path $Raw -Encoding UTF8
$status = $LASTEXITCODE

# ADR-018 收益锚点：internal/opc 的「未变 Part 复制」基准（分配 / 峰值堆）。
if ($SkipOpc -ne '1') {
    Add-Content -Path $Raw -Value '# --- internal/opc save copy bench (ADR-018) ---' -Encoding UTF8
    & go test -run '^$' -bench $OpcBench -benchmem -count $OpcCount -benchtime $OpcBenchtime `
        -timeout $Timeout ./internal/opc/ 2>&1 | Add-Content -Path $Raw -Encoding UTF8
    if ($status -eq 0) { $status = $LASTEXITCODE }
}

$report = Get-Content -Raw -Path $Raw | & go run ./scripts/perf/summarize
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText((Join-Path (Get-Location) $Out), ($report -join "`n"), $utf8NoBom)

Write-Output "raw log: $Raw"
Write-Output "report : $Out"
