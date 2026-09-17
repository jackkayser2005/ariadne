#requires -Version 7.0
param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [ValidateRange(1, 5)][int]$Runs = 3
)
$ErrorActionPreference = 'Stop'
$binary = [IO.Path]::GetFullPath($Executable)
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'Build the Ariadne executable before measuring.' }
$repository = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$runDirectory = Join-Path $repository ('.cache/har-perf-' + [Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($runDirectory) | Out-Null
$utf8 = [Text.UTF8Encoding]::new($false)
$capture = Join-Path $runDirectory 'capture.har'
$rules = Join-Path $runDirectory 'rules.json'
$builder = [Text.StringBuilder]::new()
[void]$builder.Append('{"log":{"version":"1.2","entries":[')
for ($entry = 0; $entry -lt 10000; $entry++) {
    if ($entry -gt 0) { [void]$builder.Append(',') }
    [void]$builder.Append('{"request":{"url":"https://origin-').Append($entry).Append('.test/?x=test-person%40example.test"},"comment":"').Append(('x' * 650)).Append('"}')
}
[void]$builder.Append(']}}')
[IO.File]::WriteAllText($capture, $builder.ToString(), $utf8)
[IO.File]::WriteAllText($rules, '{"schema_version":1,"synthetic":true,"rules":[{"category":"email","value":"test-person@example.test"},{"category":"account-id","value":"account-12345"}]}', $utf8)
$measurements = @()
for ($iteration = 1; $iteration -le $Runs; $iteration++) {
    $output = Join-Path $runDirectory ('report-' + $iteration + '.html')
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $binary
    $start.WorkingDirectory = $repository
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in @('browser', 'inspect-har', '--origin', 'https://example.test', '--test-values', $rules, '--output', $output, $capture)) { $start.ArgumentList.Add($argument) }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $start
    $timer = [Diagnostics.Stopwatch]::StartNew()
    $peak = [long]0
    try {
        if (-not $process.Start()) { throw 'Measurement process did not start.' }
        while (-not $process.WaitForExit(10)) {
            $process.Refresh()
            $peak = [Math]::Max($peak, $process.PeakWorkingSet64)
            if ($timer.Elapsed.TotalSeconds -gt 30) { $process.Kill(); throw 'Measurement timed out.' }
        }
        $timer.Stop()
        try { $peak = [Math]::Max($peak, $process.PeakWorkingSet64) } catch { }
        if ($process.ExitCode -ne 0 -or $peak -eq 0) { throw 'Measurement failed or peak working set was unavailable.' }
        $measurements += [pscustomobject]@{
            elapsed_ms = [Math]::Round($timer.Elapsed.TotalMilliseconds, 1)
            peak_working_set_bytes = $peak
            input_bytes = (Get-Item -LiteralPath $capture).Length
            output_bytes = (Get-Item -LiteralPath $output).Length
        }
    } finally { $process.Dispose() }
}
$measurements | ConvertTo-Json
