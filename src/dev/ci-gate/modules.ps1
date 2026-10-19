#Requires -Version 7
# Downloads only the go.sum-pinned modules for a CI job. The validation contract allows one
# unchanged retry for a confirmed transient infrastructure error, so the download is retried
# once only when every reported error is a recognised network failure; checksum, version and
# format errors fail at once, and the caller still runs go mod verify.
$ErrorActionPreference = 'Stop'
$transient = 'TLS handshake timeout|i/o timeout|connection reset by peer|connection refused|502 Bad Gateway|503 Service Unavailable|504 Gateway Timeout'
for ($attempt = 1; ; $attempt++) {
  $global:LASTEXITCODE = 1 # a launch failure sets no exit code and must not inherit an earlier 0
  $ErrorActionPreference = 'Continue'
  try {
    $output = @(go mod download -json 2>&1 | ForEach-Object { "$_" })
  } finally {
    $ErrorActionPreference = 'Stop'
  }
  $code = $LASTEXITCODE
  $output | Write-Output
  if ($code -eq 0) { break }
  $errors = @($output | Where-Object { $_ -match '"Error":|^go: ' })
  $networkOnly = $errors.Count -gt 0 -and -not ($errors | Where-Object { $_ -notmatch $transient })
  if ($attempt -ge 2 -or -not $networkOnly) { throw 'pinned module download failed' }
  Write-Output "transient module download failure on attempt $attempt; retrying once"
  Start-Sleep -Seconds 15
}
