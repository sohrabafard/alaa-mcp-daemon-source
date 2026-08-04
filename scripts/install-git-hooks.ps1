[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    throw "git was not found in PATH"
}
if (-not (Get-Command python -ErrorAction SilentlyContinue) -and -not (Get-Command python3 -ErrorAction SilentlyContinue)) {
    throw "Python 3 is required by the repository pre-commit hook"
}

& git config --local core.hooksPath .githooks
if ($LASTEXITCODE -ne 0) {
    throw "could not configure core.hooksPath"
}
Write-Output "Installed repository hooks from .githooks."
