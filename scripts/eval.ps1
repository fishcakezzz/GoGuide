param(
  [string]$Base='http://127.0.0.1:8080',
  [string]$Tasks='eval/tasks.json',
  [string]$Output='eval/results.json'
)
$ErrorActionPreference='Stop'
$data=Get-Content -LiteralPath $Tasks -Raw -Encoding UTF8 | ConvertFrom-Json
$email='eval-'+[guid]::NewGuid().ToString('N')+'@example.test'
$auth=Invoke-RestMethod -Uri "$Base/v1/auth/register" -Method POST -ContentType 'application/json' -Body (@{email=$email;password='EvalPassword123!'}|ConvertTo-Json)
$headers=@{Authorization="Bearer $($auth.token)"}
$results=@()
foreach($task in $data.tasks){
  $session=Invoke-RestMethod -Uri "$Base/v1/guide/sessions" -Method POST -Headers $headers -ContentType 'application/json' -Body '{}'
  $version=1
  $answer=$null
  foreach($turn in $task.turns){
    $body=@{text=$turn;expected_version=$version}|ConvertTo-Json -Depth 10 -Compress
    $answer=Invoke-RestMethod -Uri "$Base/v1/guide/sessions/$($session.session_id)/messages" -Method POST -Headers $headers -ContentType 'application/json' -Body ([System.Text.Encoding]::UTF8.GetBytes($body))
    $version=$answer.intent_version
  }
  $checks=@()
  $proposals=@($answer.proposals)
  if($task.expect -eq 'proposal'){
    if($proposals.Count -eq 0){$checks+='expected proposal, got none'}
  } elseif($proposals.Count -gt 0){$checks+='expected no proposal'}
  if($task.budget_cents -gt 0 -and $answer.intent.budget_cents -ne $task.budget_cents){$checks+='budget extraction mismatch'}
  foreach($cat in $task.categories){if($cat -notin @($answer.intent.categories)){$checks+="missing intent category: $cat"}}
  foreach($p in $proposals){
    if($p.amount_cents -gt $task.budget_cents){$checks+='proposal exceeds budget'}
    if($p.compatibility.status -ne 'SUPPORTED'){$checks+='unverified compatibility'}
    if(@($p.items).Count -ne @($task.categories).Count){$checks+='wrong proposal item count'}
    foreach($cat in $task.categories){if($cat -notin @($p.items|ForEach-Object {$_.category})){$checks+="missing proposal category: $cat"}}
    if(@($p.evidence).Count -ne @($p.items).Count){$checks+='missing SKU evidence'}
  }
  $results+=[pscustomobject]@{id=$task.id;passed=($checks.Count -eq 0);checks=$checks;proposal_count=$proposals.Count;mode=$answer.mode;intent=$answer.intent}
  Write-Host "$($task.id): $(if($checks.Count -eq 0){'PASS'}else{'FAIL '+($checks -join '; ')})"
}
$passed=@($results|Where-Object passed).Count
$report=[pscustomobject]@{dataset_version=$data.dataset_version;time_utc=[DateTime]::UtcNow.ToString('o');total=$results.Count;passed=$passed;failed=$results.Count-$passed;results=$results}
$report|ConvertTo-Json -Depth 20|Set-Content -LiteralPath $Output -Encoding UTF8
Write-Host "Result: $passed/$($results.Count), saved to $Output"
if($passed -ne $results.Count){exit 1}
