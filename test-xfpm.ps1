# XFPM Real-Machine Test Script (Windows)
# This script will clean existing XFPM installations, reinstall it, and test a full project lifecycle.

$logFile = "$HOME\xfpm_test_$(Get-Date -Format 'yyyyMMdd_HHmmss').log"
Start-Transcript -Path $logFile -Append

Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "   ⚡ XFPM RM TEST ENGINE ⚡   " -ForegroundColor Cyan
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

# 4. Change Directory
cd my-xyp-project
Write-Host "Entered project: $(Get-Location)" -ForegroundColor Gray

# 4. Start Dev Server
Write-Host "`n[3/4] Starting development server..." -ForegroundColor Yellow
Write-Host "The server will run for 30 seconds for testing, then exit." -ForegroundColor Gray

# Start xfpm dev in a separate process to avoid blocking and capture output
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = "$HOME\.xfpm\bin\xfpm.exe"
$psi.Arguments = "dev"
$psi.WorkingDirectory = (Get-Location).Path
$psi.UseShellExecute = $false
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true

$proc = [System.Diagnostics.Process]::Start($psi)

Write-Host "Server started. Observing performance for 30s..." -ForegroundColor DarkGray
$seconds = 30
for ($i = 0; $i -lt $seconds; $i++) {
    if ($proc.HasExited) { break }
    Write-Progress -Activity "Testing XFPM Dev" -Status "Server running ($($seconds - $i)s left)" -PercentComplete (($i / $seconds) * 100)
    Start-Sleep -Seconds 1
}

if (-not $proc.HasExited) {
    Write-Host "Stopping server..." -ForegroundColor Gray
    $proc.Kill()
}

$output = $proc.StandardOutput.ReadToEnd()
$errors = $proc.StandardError.ReadToEnd()

Write-Host "--- SERVER OUTPUT ---" -ForegroundColor Gray
Write-Host $output
if ($errors) {
    Write-Host "--- SERVER ERRORS ---" -ForegroundColor Red
    Write-Host $errors
}

Write-Host "`n==========================================" -ForegroundColor Green
Write-Host "   ✅ TEST SEQUENCE COMPLETED SUCCESSFULLY" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
Write-Host "Please send the file '$logFile' to iDevo for analysis." -ForegroundColor Cyan

Stop-Transcript
