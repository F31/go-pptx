# L3 客户端自动化：通过 COM 打开 PPTX、检查无修复提示、另存为 .pptx。
#
# 用法（Windows PowerShell，从 WSL 调用见 run_client.sh）：
#   powershell -File ppt_open_resave.ps1 -App ppt -Src <file> -Dst <file>
#
# -App 取值：
#   ppt  -> PowerPoint.Application（Microsoft PowerPoint，Office 16）
#   wpp  -> Kwpp.Application（WPS 演示）
#
# 判定语义：
#   OPEN=ok  表示 COM Open 在超时窗口内成功返回（若文件需修复，客户端会
#             弹修复对话框并阻塞 Open → 由 run_client.sh 的超时判定为
#             TIMEOUT_REPAIR_PROMPT，而非 ok）。
#   SAVE=ok  表示 SaveAs(.pptx) 成功。
param(
  [ValidateSet('ppt', 'wpp')][string]$App,
  [string]$Src,
  [string]$Dst
)
$ErrorActionPreference = 'Stop'
$progID = if ($App -eq 'ppt') { 'PowerPoint.Application' } else { 'Kwpp.Application' }
$result = @{ open = 'fail'; slides = 0; save = 'fail'; err = '' }
try {
  $client = New-Object -ComObject $progID
  try {
    $pres = $client.Presentations.Open($Src)
    $result.open = 'ok'
    $result.slides = $pres.Slides.Count
    $pres.SaveAs($Dst, 24)  # 24 = ppSaveAsOpenXMLPresentation (.pptx)
    $result.save = 'ok'
    $pres.Close()
  } finally {
    $client.Quit()
    [System.Runtime.Interopservices.Marshal]::ReleaseComObject($client) | Out-Null
  }
} catch {
  $result.err = $_.Exception.Message
}
Write-Output ("OPEN=" + $result.open + " SLIDES=" + $result.slides + " SAVE=" + $result.save)
if ($result.err -ne '') { Write-Output ("ERR=" + $result.err) }
if ($result.open -ne 'ok' -or $result.save -ne 'ok') { exit 1 }
