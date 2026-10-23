#Requires -Version 7
# Fixed GitHub transport; response bytes remain data for the offline Go validator.
function Get-CIGateFile {
  param(
    [Parameter(Mandatory)][Net.Http.HttpClient]$Client,
    [Parameter(Mandatory)][uri]$Uri,
    [Parameter(Mandatory)][string]$Out,
    [Parameter(Mandatory)][long]$Limit,
    [Parameter(Mandatory)][Threading.CancellationToken]$Cancellation,
    [string]$Token = '',
    [TimeSpan]$RetryDelay = [TimeSpan]::FromSeconds(2)
  )
  if ($Uri.Scheme -ne 'https' -or $Uri.Host -ne 'api.github.com' -or $Uri.UserInfo -or $Uri.Port -ne 443) { throw 'GitHub endpoint rejected' }
  # #275: a transient transport failure or 5xx is tried at most three times inside the
  # caller's deadline; every try restarts at the GitHub endpoint, so the credential still
  # goes only to its first hop. An existing output is never replaced.
  if (Test-Path -LiteralPath $Out) { throw 'GitHub download failed' }
  for ($try = 1; ; $try++) {
    $transient = $false
    $created = $false
    try {
      Receive-CIGateFile -Client $Client -Uri $Uri -Out $Out -Limit $Limit -Cancellation $Cancellation -Token $Token -Created ([ref]$created)
      return
    } catch {
      $transient = $try -lt 3 -and !$Cancellation.IsCancellationRequested -and (Test-CIGateTransient $_.Exception)
    }
    # HttpClient errors can contain signed redirect URLs; discard their messages.
    if (!$transient) { throw 'GitHub download failed' }
    if ($created) { Remove-Item -LiteralPath $Out } # this try's partial bytes only
    if ($Cancellation.WaitHandle.WaitOne([TimeSpan]::FromTicks($RetryDelay.Ticks * $try))) { throw 'GitHub download failed' }
  }
}

function Test-CIGateTransient([Exception]$Exception) {
  for ($e = $Exception; $null -ne $e; $e = $e.InnerException) {
    if ($e -is [Net.Http.HttpRequestException] -or $e -is [IO.IOException] -or $e -is [TimeoutException] -or $e.Message -in @('transient status', 'response incomplete')) { return $true }
  }
  return $false
}

function Receive-CIGateFile($Client, [uri]$Uri, [string]$Out, [long]$Limit, [Threading.CancellationToken]$Cancellation, [string]$Token, [ref]$Created) {
  for ($redirect = 0; $redirect -lt 4; $redirect++) {
    $Cancellation.ThrowIfCancellationRequested()
    $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Get, $Uri)
    $response = $null
    try {
      $request.Headers.Add('Accept', 'application/vnd.github+json')
      $request.Headers.Add('X-GitHub-Api-Version', '2026-03-10')
      $request.Headers.Add('User-Agent', 'tree-sitter-grammar-kit-ci-gate')
      if ($redirect -eq 0 -and $Token) { $request.Headers.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $Token) }
      $response = $Client.Send($request, [Net.Http.HttpCompletionOption]::ResponseHeadersRead, $Cancellation)
      $code = [int]$response.StatusCode
      if ($code -in @(301, 302, 303, 307, 308)) {
        $location = $response.Headers.Location
        if ($null -eq $location) { throw 'redirect missing' }
        $next = [uri]::new($Uri, $location)
        if ($next.Scheme -ne 'https' -or $next.UserInfo -or $next.Port -ne 443 -or
            !($next.Host -eq 'api.github.com' -or $next.Host.EndsWith('.blob.core.windows.net', [StringComparison]::Ordinal) -or $next.Host.EndsWith('.githubusercontent.com', [StringComparison]::Ordinal))) { throw 'redirect rejected' }
        $Uri = $next
        continue
      }
      if ($code -in @(500, 502, 503, 504)) { throw 'transient status' }
      if ($code -ne 200 -or ($null -ne $response.Content.Headers.ContentLength -and $response.Content.Headers.ContentLength -gt $Limit)) { throw 'response rejected' }
      $inputStream = $null
      $outputStream = $null
      try {
        $inputStream = $response.Content.ReadAsStream($Cancellation)
        $outputStream = [IO.File]::Open($Out, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        $Created.Value = $true
        $buffer = [byte[]]::new(65536)
        [long]$total = 0
        while ($true) {
          $Cancellation.ThrowIfCancellationRequested()
          $read = $inputStream.ReadAsync($buffer, 0, [int][Math]::Min($buffer.Length, $Limit - $total + 1), $Cancellation).GetAwaiter().GetResult()
          if ($read -eq 0) { break }
          $total += $read
          if ($total -gt $Limit) { throw 'response too large' }
          $outputStream.Write($buffer, 0, $read)
        }
        if ($null -ne $response.Content.Headers.ContentLength -and $total -ne $response.Content.Headers.ContentLength) { throw 'response incomplete' }
        $Cancellation.ThrowIfCancellationRequested()
        return
      } finally {
        if ($null -ne $outputStream) { $outputStream.Dispose() }
        if ($null -ne $inputStream) { $inputStream.Dispose() }
      }
    } finally {
      if ($null -ne $response) { $response.Dispose() }
      $request.Dispose()
    }
  }
  throw 'redirect limit'
}
