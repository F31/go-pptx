# PERF-01 performance baseline driver (design doc sec 15.3 / plan sec 9.3).
#
# Usage (run from the repository root):
#   pwsh -File scripts/perf/run.ps1
#
# Environment variables:
#   COUNT      sample count for p50/p95 (default 10)
#   BENCH      benchmark filter regex (default BenchmarkPerf)
#   BENCHTIME  benchtime override (default empty = auto calibrate)
#   RAW        raw log path (default scripts/perf/raw-bench.log)
#   OUT        report path (default docs/PERF-01-benchmark-report.md)
$ErrorActionPreference = 'Stop'

$Count = if ($env:COUNT) { $env:COUNT } else { '10' }
$Bench = if ($env:BENCH) { $env:BENCH } else { 'BenchmarkPerf' }
$Benchtime = if ($env:BENCHTIME) { $env:BENCHTIME } else { '' }
$Raw = if ($env:RAW) { $env:RAW } else { 'scripts/perf/raw-bench.log' }
$Out = if ($env:OUT) { $env:OUT } else { 'docs/PERF-01-benchmark-report.md' }

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
    "# bench: $Bench"
)
Set-Content -Path $Raw -Value $header -Encoding UTF8

$testArgs = @('-run', '^$', '-bench', $Bench, '-benchmem', '-count', $Count)
if ($Benchtime) { $testArgs += @('-benchtime', $Benchtime) }
$testArgs += '.'

& go test @testArgs 2>&1 | Add-Content -Path $Raw -Encoding UTF8

$report = Get-Content -Raw -Path $Raw | & go run ./scripts/perf/summarize
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText((Join-Path (Get-Location) $Out), ($report -join "`n"), $utf8NoBom)

Write-Output "raw log: $Raw"
Write-Output "report : $Out"
