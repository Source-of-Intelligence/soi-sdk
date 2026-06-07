# Test-SandboxFeature.ps1 - 测试 Sandbox Uses 功能
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File TEST_USES.ps1

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Testing Sandbox Uses Feature" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$sdkDir = Split-Path -Parent $scriptDir
$pluginDir = "e:\code\soi\soi-plugin\word2md"

Write-Host "Testing soi-sync with Uses feature..." -ForegroundColor Yellow
Write-Host ""

# Run soi-sync
$syncResult = & go run . --dir $pluginDir 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "SUCCESS: soi-sync completed" -ForegroundColor Green
    Write-Host $syncResult -ForegroundColor Gray
} else {
    Write-Host "FAILED: soi-sync failed" -ForegroundColor Red
    Write-Host $syncResult -ForegroundColor Red
}

Write-Host ""
Write-Host "Checking generated skill.yaml..." -ForegroundColor Yellow
Write-Host ""

$yamlPath = Join-Path $pluginDir "skill.yaml"
if (Test-Path $yamlPath) {
    $content = Get-Content $yamlPath -Raw
    if ($content -match "uses:") {
        Write-Host "SUCCESS: 'uses' section found in skill.yaml" -ForegroundColor Green
        Write-Host ""
        Write-Host "Generated YAML preview:" -ForegroundColor Cyan
        $content | Select-String -Pattern "uses:" -Context 0,3 | ForEach-Object {
            Write-Host $_.Line -ForegroundColor White
            $_.Context.PostContext | ForEach-Object {
                Write-Host $_.TrimStart() -ForegroundColor Gray
            }
        }
    } else {
        Write-Host "FAILED: 'uses' section not found" -ForegroundColor Red
    }
} else {
    Write-Host "FAILED: skill.yaml not found" -ForegroundColor Red
}

Write-Host ""
Write-Host "Checking word2md plugin source..." -ForegroundColor Yellow
Write-Host ""

$mainGo = Join-Path $pluginDir "main.go"
if (Test-Path $mainGo) {
    $content = Get-Content $mainGo -Raw
    if ($content -match "WithSandbox") {
        Write-Host "SUCCESS: WithSandbox found in source" -ForegroundColor Green
        Write-Host ""
        Write-Host "Found uses declarations:" -ForegroundColor Cyan
        $content -split "`n" | Where-Object { $_ -match "WithSandbox" } | ForEach-Object {
            Write-Host "  $($_.Trim())" -ForegroundColor White
        }
    } else {
        Write-Host "FAILED: WithSandbox not found" -ForegroundColor Red
    }
} else {
    Write-Host "FAILED: main.go not found" -ForegroundColor Red
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Test Complete!" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
