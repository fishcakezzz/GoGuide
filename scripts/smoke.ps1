param([string]$Base='http://127.0.0.1:8080')
$ErrorActionPreference='Stop'
function Request([string]$Path,[string]$Method='GET',$Body=$null){
  $headers=@{}; if($script:token){$headers.Authorization="Bearer $script:token"}
  $args=@{Uri="$Base$Path";Method=$Method;Headers=$headers;ContentType='application/json'}
  if($null -ne $Body){$args.Body=($Body|ConvertTo-Json -Depth 15 -Compress)}
  Invoke-RestMethod @args
}
$null=Request '/healthz'
$products=Request '/v1/products?in_stock=true'
if($products.items.Count -lt 3){throw 'catalog seed missing'}
$mail='smoke-'+[guid]::NewGuid().ToString('N')+'@example.test'
$auth=Request '/v1/auth/register' 'POST' @{email=$mail;password='SmokePassword123!'}
$script:token=$auth.token
$address=Request '/v1/addresses' 'POST' @{recipient='测试用户';phone='00000000000';detail='本地演示地址';default=$true}
$null=Request '/v1/cart/items' 'POST' @{sku_id=1;quantity=1}
$preview=Request '/v1/orders/preview' 'POST' @{address_id=$address.id}
$requestId=[guid]::NewGuid().ToString()
$order=Request '/v1/orders' 'POST' @{preview_id=$preview.preview_id;request_id=$requestId}
$again=Request '/v1/orders' 'POST' @{preview_id=$preview.preview_id;request_id=$requestId}
if($order.order_id -ne $again.order_id){throw 'order idempotency failed'}
$paid=Request "/v1/orders/$($order.order_id)/mock-pay" 'POST' @{}
$repeated=Request "/v1/orders/$($order.order_id)/mock-pay" 'POST' @{}
if($paid.status -ne 'PAID' -or $repeated.status -ne 'PAID'){throw 'payment failed'}
$session=Request '/v1/guide/sessions' 'POST' @{}
$guide=Request "/v1/guide/sessions/$($session.session_id)/messages" 'POST' @{text='预算 8000 元，买笔记本、显示器和扩展坞，希望一根线连接';expected_version=1}
if($guide.proposals.Count -lt 1){throw "guide returned no proposals: $($guide.message)"}
$p=$guide.proposals[0]
$confirmed=Request "/v1/guide/proposals/$($p.id)/confirm" 'POST' @{proposal_version=$p.version;expected_amount_cents=$p.amount_cents;items=@($p.items|ForEach-Object {@{sku_id=$_.sku_id;quantity=$_.quantity}})}
$confirmedAgain=Request "/v1/guide/proposals/$($p.id)/confirm" 'POST' @{proposal_version=$p.version;expected_amount_cents=$p.amount_cents;items=@($p.items|ForEach-Object {@{sku_id=$_.sku_id;quantity=$_.quantity}})}
if(-not $confirmed.confirmed -or -not $confirmedAgain.idempotent){throw 'proposal confirmation idempotency failed'}
$cart=Request '/v1/cart'
if($cart.items.Count -lt 3){throw 'proposal items missing from cart'}
[pscustomobject]@{Products=$products.items.Count;OrderStatus=$paid.status;OrderIdempotent=$again.idempotent;GuideMode=$guide.mode;Proposals=$guide.proposals.Count;ProposalIdempotent=$confirmedAgain.idempotent;CartItems=$cart.items.Count}
