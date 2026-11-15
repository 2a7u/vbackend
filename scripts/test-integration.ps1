# scripts/test-integration.ps1
# Runs real PostgreSQL integration tests on Windows or Docker
param(
    [switch]$Docker
)

$ErrorActionPreference = "Stop"

if ($Docker) {
    if (-not (Get-Command "docker" -ErrorAction SilentlyContinue)) {
        Write-Error "Docker is not installed or not in PATH."
    }
    Write-Host "==> Starting Docker Compose PostgreSQL 16 Test Suite..." -ForegroundColor Cyan
    docker compose -f docker-compose.test.yml down -v --remove-orphans
    docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from test-runner
    docker compose -f docker-compose.test.yml down -v
} else {
    Write-Host "==> Running Real PostgreSQL Concurrency & Integration Tests (REQUIRE_PG=1)..." -ForegroundColor Cyan
    $env:REQUIRE_PG = "1"
    $env:CI = "true"
    go test -v -count=1 ./...
}
