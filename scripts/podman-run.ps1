#Requires -Version 5.1
<#
.SYNOPSIS
  Build, run, and manage the DRISHTI HyperShift stack on Podman (no compose needed).

.DESCRIPTION
  Uses a Podman pod to host the backend, frontend, and worker containers on a
  shared network. Backend runs in mock mode so no external DB or platform is
  required for a smoke test.

  Usage:
    .\scripts\podman-run.ps1 build   - build all images
    .\scripts\podman-run.ps1 up      - create pod + start containers
    .\scripts\podman-run.ps1 down    - stop and remove pod + containers
    .\scripts\podman-run.ps1 status  - show container status
    .\scripts\podman-run.ps1 logs    - tail logs from all containers
    .\scripts\podman-run.ps1 test    - curl the endpoints
#>
param(
    [Parameter(Position = 0)]
    [ValidateSet("build", "up", "down", "status", "logs", "test", "restart")]
    [string]$Action = "up"
)

$ErrorActionPreference = "Continue"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$PodName   = "hypershift"
$BackC     = "hs-backend"
$FrontC    = "hs-frontend"
$WorkerC   = "hs-worker"
$BackTag   = "localhost/hypershift-backend:0.1"
$FrontTag  = "localhost/hypershift-frontend:0.1"
$WorkerTag = "localhost/hypershift-worker:0.1"

function Build {
    Write-Host "Building images..." -ForegroundColor Cyan
    podman build -t $BackTag   ./backend
    podman build -t $FrontTag  ./frontend
    podman build -t $WorkerTag ./worker
}

function Down {
    Write-Host "Stopping pod '$PodName'..." -ForegroundColor Cyan
    podman pod stop $PodName 2>$null | Out-Null
    podman pod rm   $PodName --force 2>$null | Out-Null
    # Remove any orphaned containers
    foreach ($c in @($BackC, $FrontC, $WorkerC)) {
        podman rm $c --force 2>$null | Out-Null
    }
    Write-Host "Done." -ForegroundColor Green
}

function Up {
    Write-Host "Creating pod '$PodName' with published ports..." -ForegroundColor Cyan
    podman pod create --name $PodName -p 8180:8080 -p 5173:80 -p 8090:8090 2>$null | Out-Null

    Write-Host "Starting backend..." -ForegroundColor Cyan
    $backendEnv = @("-e", "DRISHTI_MODE=mock")
    if (Test-Path ".env") {
        $backendEnv = @("--env-file", ".env")
        Write-Host "  Loading backend configuration from .env" -ForegroundColor Yellow
    }
    podman run -d --pod $PodName --name $BackC @backendEnv `
        -e DRISHTI_HTTP_ADDR=:8080 -e DRISHTI_LOG_LEVEL=info $BackTag | Out-Null

    Write-Host "Starting worker..." -ForegroundColor Cyan
    podman run -d --pod $PodName --name $WorkerC `
        -e DRISHTI_WORKER_ADDR=:8090 `
        -e DRISHTI_LOG_LEVEL=info `
        $WorkerTag | Out-Null

    Write-Host "Starting frontend..." -ForegroundColor Cyan
    podman run -d --pod $PodName --name $FrontC $FrontTag | Out-Null

    Write-Host ""
    Write-Host "Stack is up:" -ForegroundColor Green
    Write-Host "  Frontend:  http://localhost:5173"
    Write-Host "  Backend:   http://localhost:8180"
    Write-Host "  Worker:    http://localhost:8090"
    Write-Host ""
    Write-Host "Run '.\scripts\podman-run.ps1 status' to inspect, '.\scripts\podman-run.ps1 test' to smoke-test."
}

function Status {
    podman pod ps --filter "name=$PodName"
    Write-Host ""
    podman ps -a --filter "pod=$PodName" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
}

function Logs {
    podman logs $BackC
    Write-Host "----- worker -----" -ForegroundColor Yellow
    podman logs $WorkerC
    Write-Host "----- frontend -----" -ForegroundColor Yellow
    podman logs $FrontC
}

function Test-Stack {
    Write-Host "Waiting for containers to start..." -ForegroundColor Cyan
    Start-Sleep -Seconds 3

    Write-Host "`n[backend] GET /healthz" -ForegroundColor Yellow
    try { (Invoke-WebRequest "http://localhost:8180/healthz" -UseBasicParsing).Content } catch { Write-Host "FAIL: $_" -ForegroundColor Red }

    Write-Host "`n[backend] GET /api/v1/connections" -ForegroundColor Yellow
    try { (Invoke-WebRequest "http://localhost:8180/api/v1/connections" -UseBasicParsing).Content } catch { Write-Host "FAIL: $_" -ForegroundColor Red }

    Write-Host "`n[worker] GET /healthz" -ForegroundColor Yellow
    try { (Invoke-WebRequest "http://localhost:8090/healthz" -UseBasicParsing).Content } catch { Write-Host "FAIL: $_" -ForegroundColor Red }

    Write-Host "`n[frontend] GET / (expect 200 / nginx)" -ForegroundColor Yellow
    try {
        $r = Invoke-WebRequest "http://localhost:5173/" -UseBasicParsing
        Write-Host "HTTP $($r.StatusCode) - $($r.Content.Length) bytes"
    } catch { Write-Host "FAIL: $_" -ForegroundColor Red }

    Write-Host "`n[backend] POST /api/v1/plans (creates DRAFT only)" -ForegroundColor Yellow
    try {
        $body = '{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab"}'
        (Invoke-WebRequest "http://localhost:8180/api/v1/plans" -Method POST -Body $body -ContentType "application/json" -UseBasicParsing).Content
    } catch { Write-Host "FAIL: $_" -ForegroundColor Red }
}

switch ($Action) {
    "build"   { Build }
    "up"      { Down; Up }
    "down"    { Down }
    "status"  { Status }
    "logs"    { Logs }
    "test"    { Test-Stack }
    "restart" { Down; Up }
    default   { Up }
}
