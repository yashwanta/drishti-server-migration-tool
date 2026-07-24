#Requires -Version 5.1
<#
.SYNOPSIS
  Builds and tests all DRISHTI HyperShift components and reports results.
#>
$ErrorActionPreference = "Continue"
Set-StrictMode -Version 3.0

$script:results = @()

function Step($name, $script) {
    Write-Host ""
    Write-Host "=== $name ===" -ForegroundColor Cyan
    $ok = $true
    try {
        & $script
        if ($LASTEXITCODE -ne 0) { $ok = $false }
    } catch {
        Write-Host "ERROR: $_" -ForegroundColor Red
        $ok = $false
    }
    $script:results += [pscustomobject]@{ Step = $name; Result = $(if ($ok) { "PASS" } else { "FAIL" }) }
    if (-not $ok) { Write-Host "FAILED" -ForegroundColor Red } else { Write-Host "OK" -ForegroundColor Green }
}

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Step "Backend build"      { Set-Location "$root\backend"; go build ./... }
Step "Backend go vet"     { Set-Location "$root\backend"; go vet ./... }
Step "Backend tests"      { Set-Location "$root\backend"; go test ./... }
Step "Worker build"       { Set-Location "$root\worker"; go build ./... }
Step "Worker go vet"      { Set-Location "$root\worker"; go vet ./... }
Step "Worker tests"       { Set-Location "$root\worker"; go test ./... }
Step "Frontend typecheck" { Set-Location "$root\frontend"; npx tsc --noEmit }
Step "Frontend build"     { Set-Location "$root\frontend"; npm run build }

Write-Host ""
Write-Host "==================== SUMMARY ====================" -ForegroundColor Yellow
$script:results | Format-Table -AutoSize
$failed = @($script:results | Where-Object { $_.Result -eq "FAIL" }).Count
if ($failed -gt 0) {
    Write-Host "$failed step(s) FAILED." -ForegroundColor Red
    exit 1
} else {
    Write-Host "All steps PASSED." -ForegroundColor Green
    exit 0
}