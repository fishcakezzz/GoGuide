param([string]$Output = 'data/public-listing-captures.json')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$source = Get-Content (Join-Path $root 'cmd/seed/pc_parts.go') -Raw
$urls = [regex]::Matches($source, 'https://tao\.hooos\.com/[^"\s]+') | ForEach-Object Value | Sort-Object -Unique
$captures = foreach ($url in $urls) {
  try {
    $response = Invoke-WebRequest $url -TimeoutSec 15
    $html = if ($response.Content -is [byte[]]) { [Text.Encoding]::UTF8.GetString($response.Content) } else { [string]$response.Content }
    $title = [regex]::Match($html, '<title[^>]*>(.*?)</title>', 'IgnoreCase,Singleline').Groups[1].Value
    if (-not $title) { $title = [regex]::Match($html, '<h1[^>]*>(.*?)</h1>', 'IgnoreCase,Singleline').Groups[1].Value }
    [pscustomobject]@{
      source_url = $url
      page_title = [Net.WebUtility]::HtmlDecode(($title -replace '<[^>]+>', '')).Trim()
      http_status = [int]$response.StatusCode
      captured_at = (Get-Date).ToUniversalTime().ToString('o')
    }
  } catch {
    [pscustomobject]@{source_url=$url;page_title='';http_status=0;captured_at=(Get-Date).ToUniversalTime().ToString('o');error=$_.Exception.Message}
  }
  Start-Sleep -Milliseconds 1200
}
$target = Join-Path $root $Output
New-Item -ItemType Directory -Force (Split-Path $target) | Out-Null
[IO.File]::WriteAllText($target, ($captures | ConvertTo-Json -Depth 4), [Text.UTF8Encoding]::new($false))
$captures | Select-Object http_status,page_title,source_url
