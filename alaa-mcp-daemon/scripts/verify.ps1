[CmdletBinding()]
param(
    [switch]$RunSchedulerIntegration
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Stop-CouldNotRun([string]$Message) {
    [Console]::Error.WriteLine("could-not-run: $Message")
    exit 2
}

function Invoke-NativeGate([string]$Name, [scriptblock]$Command) {
    Write-Output "==> $Name"
    & $Command
    if ($LASTEXITCODE -ne 0) {
        [Console]::Error.WriteLine("finding: $Name exited with code $LASTEXITCODE")
        exit 1
    }
}

if (-not $IsWindows) {
    Stop-CouldNotRun "the v1 release gate must run on Windows"
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Stop-CouldNotRun "go was not found in PATH"
}
if (-not (Get-Command govulncheck -ErrorAction SilentlyContinue)) {
    Stop-CouldNotRun "govulncheck was not found; install golang.org/x/vuln/cmd/govulncheck"
}

$goos = (& go env GOOS).Trim()
$goarch = (& go env GOARCH).Trim()
$cgo = (& go env CGO_ENABLED).Trim()
if ($goos -ne "windows" -or $goarch -ne "amd64") {
    Stop-CouldNotRun "expected GOOS=windows and GOARCH=amd64, got GOOS=$goos GOARCH=$goarch"
}
if ($cgo -ne "1") {
    Stop-CouldNotRun "go test -race requires CGO_ENABLED=1 for this gate"
}
$cc = (& go env CC).Trim()
if ([string]::IsNullOrWhiteSpace($cc) -or -not (Get-Command $cc -ErrorAction SilentlyContinue)) {
    Stop-CouldNotRun "the configured C compiler '$cc' was not found; it is required by the race gate"
}

Write-Output "==> gofmt cleanliness"
$unformatted = @(& gofmt -l .)
if ($LASTEXITCODE -ne 0) {
    [Console]::Error.WriteLine("finding: gofmt could not inspect the tree")
    exit 1
}
if ($unformatted.Count -gt 0) {
    [Console]::Error.WriteLine("finding: unformatted Go files")
    $unformatted | ForEach-Object { [Console]::Error.WriteLine($_) }
    exit 1
}

Invoke-NativeGate "go test ./..." { & go test -count=1 ./... }
Invoke-NativeGate "go vet ./..." { & go vet ./... }
Invoke-NativeGate "go test -race ./..." { & go test -race -count=1 ./... }
Invoke-NativeGate "govulncheck ./..." { & govulncheck ./... }

if ($RunSchedulerIntegration) {
    if (-not (Get-Command schtasks.exe -ErrorAction SilentlyContinue)) {
        Stop-CouldNotRun "schtasks.exe was not found"
    }
    Write-Output "==> Task Scheduler create/remove integration"
    $previous = $env:ALAA_MCP_DAEMON_RUN_SCHEDULER_TESTS
    try {
        $env:ALAA_MCP_DAEMON_RUN_SCHEDULER_TESTS = "1"
        & go test -count=1 -run '^TestTaskSchedulerInstallRemoveOptIn$' ./internal/autostart
        if ($LASTEXITCODE -ne 0) {
            [Console]::Error.WriteLine("finding: Task Scheduler integration failed")
            exit 1
        }
    }
    finally {
        if ($null -eq $previous) {
            Remove-Item Env:ALAA_MCP_DAEMON_RUN_SCHEDULER_TESTS -ErrorAction SilentlyContinue
        }
        else {
            $env:ALAA_MCP_DAEMON_RUN_SCHEDULER_TESTS = $previous
        }
    }
}

$tempOutput = Join-Path ([System.IO.Path]::GetTempPath()) ("alaa-mcp-daemon-" + [Guid]::NewGuid().ToString("N") + ".exe")
try {
    Invoke-NativeGate "Windows production build" { & go build -trimpath -o $tempOutput ./cmd/alaa-mcp-daemon }
}
finally {
    Remove-Item $tempOutput -Force -ErrorAction SilentlyContinue
}

Write-Output "clean: all requested verification gates passed"
exit 0
