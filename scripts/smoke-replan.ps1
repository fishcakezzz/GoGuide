param(
  [string]$Base = 'http://127.0.0.1:8080',
  [Parameter(Mandatory=$true)][string]$AdminEmail,
  [Parameter(Mandatory=$true)][string]$AdminPassword
)
$ErrorActionPreference = 'Stop'
function Request([string]$Path,[string]$Method='GET',$Body=$null,[string]$Token='') {
  $headers=@{}; if($Token){$headers.Authorization="Bearer $Token"}
  $args=@{Uri="$Base$Path";Method=$Method;Headers=$headers;ContentType='application/json'}
  if($null -ne $Body){$args.Body=($Body|ConvertTo-Json -Depth 18 -Compress)}
  Invoke-RestMethod @args
}
$mail='replan-'+[guid]::NewGuid().ToString('N')+'@example.test'
$user=Request '/v1/auth/register' 'POST' @{email=$mail;password='SmokePassword123!'}
$admin=Request '/v1/auth/login' 'POST' @{email=$AdminEmail;password=$AdminPassword}
$session=Request '/v1/guide/sessions' 'POST' @{} $user.token
$guide=Request "/v1/guide/sessions/$($session.session_id)/messages" 'POST' @{text='预算10000元装机，需要CPU、主板、32GB DDR5内存、显卡、1TB固态、散热器、机箱和电源，比较性能与性价比';expected_version=1} $user.token
if($guide.proposals.Count -lt 1){throw "no initial proposal: $($guide.message)"}
$old=$guide.proposals[0]
if($old.score.score -le 0 -or $old.score.value_per_thousand_yuan -le 0){throw 'score missing'}
$item=$old.items[0]
$price=$item.unit_price_cents+5000
$null=Request "/v1/admin/skus/$($item.sku_id)/price" 'PUT' @{price_cents=$price;reason='replan smoke test'} $admin.token
$event=$null
for($i=0;$i -lt 20;$i++){
  Start-Sleep -Seconds 3
  $history=Request "/v1/guide/sessions/$($session.session_id)/replans" 'GET' $null $user.token
  if($history.items.Count -gt 0 -and $history.items[0].status -eq 'DONE'){$event=$history.items[0];break}
}
if(-not $event){throw 'worker did not complete replan'}
$plans=Request "/v1/guide/sessions/$($session.session_id)/proposals" 'GET' $null $user.token
$current=@($plans.items|Where-Object {-not $_.stale})
if($current.Count -lt 1 -or $event.new_intent_version -le $event.old_intent_version){throw 'new version missing'}
$status=0
try {
  $null=Request "/v1/guide/proposals/$($old.id)/confirm" 'POST' @{proposal_version=$old.version;expected_amount_cents=$old.amount_cents;items=@($old.items|ForEach-Object{@{sku_id=$_.sku_id;quantity=$_.quantity}})} $user.token
} catch {
  $status=[int]$_.Exception.Response.StatusCode
}
if($status -ne 409){throw "old proposal confirmation should be 409, got $status"}
$stockItem=$current[0].items[0]
$null=Request "/v1/admin/skus/$($stockItem.sku_id)/stock" 'PUT' @{delta=-1;reason='replan smoke test';request_id=[guid]::NewGuid().ToString()} $admin.token
$stockEvent=$null
for($i=0;$i -lt 20;$i++){
  Start-Sleep -Seconds 3
  $history=Request "/v1/guide/sessions/$($session.session_id)/replans" 'GET' $null $user.token
  if($history.items.Count -gt 1 -and $history.items[0].status -eq 'DONE'){$stockEvent=$history.items[0];break}
}
if(-not $stockEvent -or $stockEvent.new_intent_version -le $event.new_intent_version){throw 'stock version did not trigger replan'}
[pscustomobject]@{OldVersion=$event.old_intent_version;PriceReplanVersion=$event.new_intent_version;StockReplanVersion=$stockEvent.new_intent_version;PriceReason=$event.reason;StockReason=$stockEvent.reason;NewPlans=$current.Count;OldConfirmStatus=$status;Score=$old.score.score;ValuePerThousand=$old.score.value_per_thousand_yuan}
