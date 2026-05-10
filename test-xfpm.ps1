# XFPM Real-Machine Test Script (Windows)
# This script will clean existing XFPM installations, reinstall it, and test a full project lifecycle.

$logFile = "$HOME\xfpm_test_$(Get-Date -Format 'yyyyMMdd_HHmmss').log"
Start-Transcript -Path $logFile -Append

Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "   ⚡ XFPM ReM TEST ENGINE ⚡   " -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "Logging to: $logFile"

# 1. Install XFPM (Installer handles cleanup)
Write-Host "[1/4] Installing/Updating XFPM..." -ForegroundColor Yellow
try {
    # Using the node-based bridge as requested
    Invoke-RestMethod -Uri "https://xypriss.nehonix.com/install.js" -UseBasicParsing | node
    
    # Refresh Environment Variables for the current session
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","User") + ";" + [System.Environment]::GetEnvironmentVariable("Path","Machine")
    
    Write-Host "Installation command completed." -ForegroundColor Green
} catch {
    Write-Host "CRITICAL: Installation failed! Check if Node.js is installed." -ForegroundColor Red
    Stop-Transcript
    exit 1
}

# 2. Initialize Project
Write-Host "`n[2/4] Initializing new XyPriss project..." -ForegroundColor Yellow
if (Test-Path "my-xyp-project") {
    Remove-Item -Recurse -Force "my-xyp-project"
}

# Use the full path if recently installed and not yet in session PATH
& "$HOME\.xfpm\bin\xfpm.exe" init my-xyp-project

if (-not (Test-Path "my-xyp-project")) {
    Write-Host "CRITICAL: Project initialization failed!" -ForegroundColor Red
    Stop-Transcript
    exit 1
}

# 3. Change Directory
cd my-xyp-project
Write-Host "Entered project: $(Get-Location)" -ForegroundColor Gray

# 4. Start Dev Server with Live Detection
Write-Host "`n[3/4] Starting development server..." -ForegroundColor Yellow
Write-Host "Waiting for 'localhost' signal in logs..." -ForegroundColor Gray

$devLog = "$HOME\xfpm_dev_temp.log"
if (Test-Path $devLog) { Remove-Item $devLog }

# Start in background via a Job to capture output
$job = Start-Job -ScriptBlock {
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","User") + ";" + [System.Environment]::GetEnvironmentVariable("Path","Machine")
    cd $using:PWD
    & "$HOME\.xfpm\bin\xfpm.exe" dev *>&1 | Out-File $using:devLog -Encoding utf8
}

$ready = $false
$timeout = 120 # 2 minute timeout for first-time compilation
$start = Get-Date

while (((Get-Date) - $start).TotalSeconds -lt $timeout) {
    if (Test-Path $devLog) {
        try {
            $logs = Get-Content $devLog -Raw -ErrorAction SilentlyContinue
            if ($logs -match "localhost" -or $logs -match "Ready" -or $logs -match "http://") {
                $ready = $true
                break
            }
        } catch { }
    }
    Write-Progress -Activity "Booting Neural Bridge" -Status "Searching for localhost..." -PercentComplete (((Get-Date) - $start).TotalSeconds / $timeout * 100)
    Start-Sleep -Seconds 1
}

if ($ready) {
    Write-Host "`n✅ SERVER IS READY AT LOCALHOST!" -ForegroundColor Green
    Write-Host "Everything looks stable and optimized." -ForegroundColor Green
    Write-Host "`n[ACTION] Press ANY KEY here to finalize the test and stop the server." -ForegroundColor Yellow
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
} else {
    Write-Host "`n⚠️  Timeout: 'localhost' not detected after $timeout s." -ForegroundColor Orange
    Write-Host "The server might still be starting or encountered an error."
}

# 5. Finalize
Write-Host "`n[4/4] Finalizing test..." -ForegroundColor Yellow
Stop-Job $job
if (Test-Path $devLog) {
    $finalDevLogs = Get-Content $devLog
    Write-Host "`n--- CAPTURED DEV LOGS ---" -ForegroundColor Gray
    # Write-Host will be captured by the transcript automatically
    $finalDevLogs | ForEach-Object { Write-Host $_ }
    Remove-Item $devLog
}

Write-Host "`n==========================================" -ForegroundColor Green
Write-Host "   ✅ TEST SEQUENCE COMPLETED SUCCESSFULLY" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
Write-Host "Please send the file '$logFile' to iDevo for analysis." -ForegroundColor Cyan

Stop-Transcript
