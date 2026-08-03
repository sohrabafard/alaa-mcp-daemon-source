[CmdletBinding()]
param(
    [string]$Version = "dev",
    [string]$Commit = "unknown",
    [string]$Date = "",
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "go was not found in PATH"
}
if ($Version -match '\s' -or $Commit -match '\s') {
    throw "Version and Commit may not contain whitespace"
}
if ([string]::IsNullOrWhiteSpace($Date)) {
    $Date = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
}

$output = Join-Path $root $OutputDir
New-Item -ItemType Directory -Path $output -Force | Out-Null
$binary = Join-Path $output "alaa-mcp-daemon.exe"
$ldflags = "-s -w -X alaa-mcp-daemon/internal/buildinfo.Version=$Version -X alaa-mcp-daemon/internal/buildinfo.Commit=$Commit -X alaa-mcp-daemon/internal/buildinfo.Date=$Date"

& go build -trimpath -ldflags $ldflags -o $binary ./cmd/alaa-mcp-daemon
if ($LASTEXITCODE -ne 0) {
    throw "go build failed with exit code $LASTEXITCODE"
}

Copy-Item .\alaa-mcp-daemon.schema.json (Join-Path $output "alaa-mcp-daemon.schema.json") -Force
$example = Get-Content .\examples\serena.windows.example.json -Raw
$example = $example.Replace('"$schema": "../alaa-mcp-daemon.schema.json"', '"$schema": "./alaa-mcp-daemon.schema.json"')
Set-Content -Path (Join-Path $output "alaa-mcp-daemon.example.json") -Value $example -Encoding utf8NoBOM

Write-Output "Built $binary"
