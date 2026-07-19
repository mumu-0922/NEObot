[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet("start", "stop", "status")]
    [string]$Action = "status",

    [string]$Python = "python.exe",

    [ValidateRange(1, 65535)]
    [int]$Port = 18081
)

$ErrorActionPreference = "Stop"
$StatePath = Join-Path $env:TEMP "mm-chat-mineru-result-proxy.json"
$ProxyScript = Join-Path $PSScriptRoot "mineru_result_proxy_wsl.py"

function Get-ProxyState {
    if (-not (Test-Path -LiteralPath $StatePath -PathType Leaf)) {
        return $null
    }
    try {
        return Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
    }
    catch {
        Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
        return $null
    }
}

function Get-TrackedProcess($State) {
    if ($null -eq $State -or $State.pid -notmatch "^[1-9][0-9]*$") {
        return $null
    }
    $Process = Get-Process -Id ([int]$State.pid) -ErrorAction SilentlyContinue
    if ($null -eq $Process) {
        return $null
    }
    try {
        $StartedAt = $Process.StartTime.ToUniversalTime().ToFileTimeUtc()
        $ProcessPath = $Process.Path
    }
    catch {
        return $null
    }
    if (
        $ProcessPath -ne $State.pythonPath -or
        $StartedAt -ne [long]$State.startedAtFileTimeUtc
    ) {
        return $null
    }
    return $Process
}

$State = Get-ProxyState
$Tracked = Get-TrackedProcess $State

switch ($Action) {
    "status" {
        if ($null -eq $Tracked) {
            Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
            Write-Output "MinerU result proxy: stopped"
            exit 1
        }
        Write-Output "MinerU result proxy: running pid=$($Tracked.Id) port=$($State.port)"
    }
    "stop" {
        if ($null -ne $Tracked) {
            Stop-Process -Id $Tracked.Id -ErrorAction Stop
            $Tracked.WaitForExit()
        }
        Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
        Write-Output "MinerU result proxy: stopped"
    }
    "start" {
        if ($null -ne $Tracked) {
            Write-Output "MinerU result proxy: already running pid=$($Tracked.Id) port=$($State.port)"
            exit 0
        }
        Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
        if (-not (Test-Path -LiteralPath $ProxyScript -PathType Leaf)) {
            throw "MinerU result proxy script is unavailable"
        }
        $PythonPath = (Get-Command $Python -ErrorAction Stop).Source
        $Arguments = '"{0}" --host 0.0.0.0 --port {1}' -f $ProxyScript, $Port
        $Process = Start-Process `
            -FilePath $PythonPath `
            -ArgumentList $Arguments `
            -WindowStyle Hidden `
            -PassThru
        Start-Sleep -Milliseconds 500
        if ($Process.HasExited) {
            throw "MinerU result proxy failed to start"
        }
        $NewState = [ordered]@{
            pid = $Process.Id
            port = $Port
            pythonPath = $Process.Path
            startedAtFileTimeUtc = $Process.StartTime.ToUniversalTime().ToFileTimeUtc()
        }
        $Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText(
            $StatePath,
            ($NewState | ConvertTo-Json -Compress),
            $Utf8NoBom
        )
        Write-Output "MinerU result proxy: running pid=$($Process.Id) port=$Port"
    }
}
