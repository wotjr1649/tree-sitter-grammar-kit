#Requires -Version 7
# Downloads only the go.sum-pinned modules for a CI job. The validation contract allows one
# unchanged retry for a confirmed transient infrastructure error, so only a recognised network
# failure is retried once; checksum, version and format errors fail at once, and the caller
# still runs go mod verify.
$ErrorActionPreference = 'Stop'
$transient = 'TLS handshake timeout|i/o timeout|connection reset by peer|connection refused|unexpected EOF|502 Bad Gateway|503 Service Unavailable|504 Gateway Timeout'
for ($attempt = 1; ; $attempt++) {
  $ErrorActionPreference = 'Continue'
  $output = @(go mod download -json 2>&1 | ForEach-Object { "$_" })
  $code = $LASTEXITCODE
  $ErrorActionPreference = 'Stop'
  $output | Write-Output
  if ($code -eq 0) { break }
  if ($attempt -ge 2 -or ($output -join "`n") -notmatch $transient) { throw 'pinned module download failed' }
  Write-Output "transient module download failure on attempt $attempt; retrying once"
  Start-Sleep -Seconds 15
}
