#!/usr/bin/env pwsh
<#
.SYNOPSIS
  XFPM (XyPriss Fast Package Manager) - Windows Installer

.DESCRIPTION
  Downloads bridge.js from GitHub and executes it via Node.js / Deno / Bun
  to fetch the correct XFPM binary for your CPU architecture.
  Registers the installation in Windows Programs, adds it to PATH,
  and installs PowerShell completions.

.PARAMETER Version
  Version to install (e.g. "1.0.0", "v1.0.0", "latest", "canary"). Default: "latest"

.PARAMETER ForceBaseline
  Forces the baseline build (x64 without AVX2) even if your CPU supports AVX2.

.PARAMETER NoPathUpdate
  Skips adding xfpm to the user's %PATH%.

.PARAMETER NoRegisterInstallation
  Skips registering XFPM in Add/Remove Programs.

.PARAMETER NoCompletions
  Skips installing PowerShell completions.

.PARAMETER DownloadWithoutCurl
  Uses Invoke-RestMethod instead of curl.exe (slower, but useful as a fallback).

.EXAMPLE
  irm https://dll.nehonix.com/repo/xypriss/xfpm/scripts/install.ps1 | iex
  powershell -c "irm https://dll.nehonix.com/repo/xypriss/xfpm/scripts/install.ps1 | iex" -Version 1.2.0
#>
param(
  [String]$Version                = "latest",
  [String]$Action                 = "install",
  [Switch]$Xru                    = $false,
  [Switch]$ForceBaseline          = $false,
  [Switch]$NoPathUpdate           = $false,
  [Switch]$NoRegisterInstallation = $false,
  [Switch]$NoCompletions          = $false,
  [Switch]$DownloadWithoutCurl    = $false
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ─────────────────────────────────────────────────────────────
# Constants
# ─────────────────────────────────────────────────────────────
$REPO        = "Nehonix-Team/XFPM"
$BRIDGE_URL  = "https://dll.nehonix.com/repo/xypriss/xfpm/scripts/bridge.js"
$TOOL_NAME   = "xfpm"
$DISPLAY     = "XFPM"

$C_RESET  = [char]27 + "[0m"
$C_GREEN  = [char]27 + "[1;32m"
$C_RED    = [char]27 + "[1;31m"
$C_CYAN   = [char]27 + "[1;36m"
$C_YELLOW = [char]27 + "[1;33m"
$C_BOLD   = [char]27 + "[1m"
$C_DIM    = [char]27 + "[2m"

# ─────────────────────────────────────────────────────────────
# Display helpers
# ─────────────────────────────────────────────────────────────
function Write-Info  ([string]$msg) { Write-Host "${C_CYAN}info${C_RESET}  $msg" }
function Write-Ok    ([string]$msg) { Write-Host "${C_GREEN}ok${C_RESET}    $msg" }
function Write-Warn  ([string]$msg) { Write-Warning $msg }
function Write-Fatal ([string]$msg) {
  Write-Host "`n${C_RED}${C_BOLD}Install Failed${C_RESET}"
  Write-Host "  $msg`n"
  exit 1
}

# ─────────────────────────────────────────────────────────────
# PATH / registry helpers (no variable expansion side-effects)
# ─────────────────────────────────────────────────────────────
function Publish-Env {
  if (-not ("Win32.NativeMethods" -as [Type])) {
    Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @"
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(
    IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam,
    uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
"@
  }
  $HWND_BROADCAST   = [IntPtr]0xffff
  $WM_SETTINGCHANGE = 0x1a
  $result           = [UIntPtr]::Zero
  [Win32.NativeMethods]::SendMessageTimeout(
    $HWND_BROADCAST, $WM_SETTINGCHANGE,
    [UIntPtr]::Zero, "Environment",
    2, 5000, [ref]$result
  ) | Out-Null
}

function Write-Env {
  param([String]$Key, [String]$Value)
  $regEnv = (Get-Item -Path 'HKCU:').OpenSubKey('Environment', $true)
  if ($null -eq $Value) {
    $regEnv.DeleteValue($Key)
  } else {
    $kind = if ($Value.Contains('%')) {
      [Microsoft.Win32.RegistryValueKind]::ExpandString
    } elseif ($regEnv.GetValue($Key)) {
      $regEnv.GetValueKind($Key)
    } else {
      [Microsoft.Win32.RegistryValueKind]::String
    }
    $regEnv.SetValue($Key, $Value, $kind)
  }
  Publish-Env
}

function Get-Env {
  param([String]$Key)
  $regEnv = (Get-Item -Path 'HKCU:').OpenSubKey('Environment')
  $regEnv.GetValue($Key, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
}

# ─────────────────────────────────────────────────────────────
# Pre-flight system checks
# ─────────────────────────────────────────────────────────────
function Assert-SystemRequirements {
  # CPU arch — via registry, reliable even under WoW64 emulation
  $Arch = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment').PROCESSOR_ARCHITECTURE
  if ($Arch -notin @("AMD64", "ARM64")) {
    Write-Fatal "$DISPLAY is only available for x86 64-bit and ARM64 Windows. (Detected: $Arch)"
  }

  # Minimum Windows version: Windows 10 1809 / Server 2019 (build 17763)
  $MinBuild     = 17763
  $MinBuildName = "Windows 10 1809 / Windows Server 2019"
  $WinVer = [System.Environment]::OSVersion.Version
  if ($WinVer.Major -lt 10 -or ($WinVer.Major -eq 10 -and $WinVer.Build -lt $MinBuild)) {
    Write-Fatal "$DISPLAY requires $MinBuildName or newer.`n  Detected: $($WinVer.ToString())"
  }

  # Minimum PowerShell version: 5.1
  if ($PSVersionTable.PSVersion.Major -lt 5) {
    Write-Fatal "PowerShell 5.1 or newer is required (detected: $($PSVersionTable.PSVersion))."
  }

  # Write access check
  try {
    $testFile = Join-Path $env:USERPROFILE ".xfpm_write_test_$([System.IO.Path]::GetRandomFileName())"
    [System.IO.File]::WriteAllText($testFile, "test")
    Remove-Item $testFile -Force
  } catch {
    Write-Fatal "Cannot write to $env:USERPROFILE. Please check your permissions."
  }

  return $Arch
}

# ─────────────────────────────────────────────────────────────
# Detect available JS runtime (node → deno → bun)
# ─────────────────────────────────────────────────────────────
function Get-JsRuntime {
  # Node.js — must be v14+
  try {
    $nodePath = (Get-Command node -ErrorAction Stop).Source
    $nodeVer  = & node -e "process.stdout.write(process.versions.node)" 2>$null
    $nodeMajor = [int]($nodeVer -split '\.')[0]
    if ($nodeMajor -ge 14) {
      return @{ Runtime = "node"; Path = $nodePath }
    }
    Write-Warn "Node.js $nodeVer is too old (need v14+). Trying other runtimes..."
  } catch {}

  # Deno
  try {
    $denoPath = (Get-Command deno -ErrorAction Stop).Source
    return @{ Runtime = "deno"; Path = $denoPath }
  } catch {}

  # Bun
  try {
    $bunPath = (Get-Command bun -ErrorAction Stop).Source
    return @{ Runtime = "bun"; Path = $bunPath }
  } catch {}

  return $null
}

# ─────────────────────────────────────────────────────────────
# Download with fallback (curl.exe → Invoke-RestMethod)
# ─────────────────────────────────────────────────────────────
function Invoke-Download {
  param([string]$URL, [string]$DestPath, [bool]$WithoutCurl, [int]$MaxRetries = 3)

  $attempt   = 0
  $lastError = $null

  while ($attempt -lt $MaxRetries) {
    $attempt++
    $usedCurl = $false

    if (-not $WithoutCurl) {
      curl.exe "--retry" "3" "--retry-connrefused" "-fsSLo" "$DestPath" "$URL" 2>$null
      if ($LASTEXITCODE -eq 0) { $usedCurl = $true }
      else {
        Write-Warn "curl.exe failed (attempt $attempt/$MaxRetries, code $LASTEXITCODE). Trying Invoke-RestMethod..."
      }
    }

    if (-not $usedCurl) {
      try {
        Invoke-RestMethod -Uri $URL -OutFile $DestPath
        $usedCurl = $true  # download succeeded
      } catch {
        $lastError = $_
        Write-Warn "Invoke-RestMethod failed (attempt $attempt/$MaxRetries): $_"
        if ($attempt -lt $MaxRetries) { Start-Sleep -Seconds 2 }
        continue
      }
    }

    # Validate the file
    if (-not (Test-Path $DestPath)) {
      $lastError = "File missing after download (possible antivirus or firewall)"
      Write-Warn $lastError
      if ($attempt -lt $MaxRetries) { Start-Sleep -Seconds 2 }
      continue
    }
    $size = (Get-Item $DestPath).Length
    if ($size -eq 0) {
      Remove-Item $DestPath -Force -ErrorAction SilentlyContinue
      $lastError = "Downloaded file is empty (0 bytes)"
      Write-Warn $lastError
      if ($attempt -lt $MaxRetries) { Start-Sleep -Seconds 2 }
      continue
    }

    return  # success
  }

  Write-Fatal "Download failed after $MaxRetries attempts.`n  URL: $URL`n  Last error: $lastError"
}

# ─────────────────────────────────────────────────────────────
# Execute bridge.js via a JS runtime
# ─────────────────────────────────────────────────────────────
function Invoke-Bridge {
  param(
    [string]$BridgePath,
    [hashtable]$JsRuntime,
    [string]$OutPath
  )

  $env:XFPM_OUT     = $OutPath
  $env:XFPM_VERSION = $Version

  $bridgeArgs = @("--out", $OutPath)
  if ($Version -ne "latest") { $bridgeArgs += @("--version", $Version) }

  Write-Info "Running bridge via $($JsRuntime.Runtime)..."

  switch ($JsRuntime.Runtime) {
    "node" {
      & node $BridgePath @bridgeArgs
    }
    "deno" {
      & deno run --allow-net --allow-write --allow-read --allow-env $BridgePath @bridgeArgs
    }
    "bun" {
      & bun run $BridgePath @bridgeArgs
    }
  }

  $env:XFPM_OUT     = $null
  $env:XFPM_VERSION = $null

  if ($LASTEXITCODE -ne 0) {
    Write-Fatal "bridge.js exited with code $LASTEXITCODE.`n  Check the output above for details."
  }
}

# ─────────────────────────────────────────────────────────────
# Fallback: direct binary download without bridge (no JS runtime)
# ─────────────────────────────────────────────────────────────
function Invoke-DirectDownload {
  param([string]$Arch, [string]$OutPath)

  Write-Warn "No JS runtime found (node/deno/bun). Falling back to direct binary download..."

  $IsARM64    = ($Arch -eq "ARM64")
  $XfpmArch   = if ($IsARM64) { "arm64" } else { "amd64" }

  # AVX2 detection for baseline build
  $IsBaseline = $ForceBaseline
  if (-not $IsBaseline -and -not $IsARM64) {
    try {
      $IsBaseline = -not (
        Add-Type -MemberDefinition '[DllImport("kernel32.dll")] public static extern bool IsProcessorFeaturePresent(int ProcessorFeature);' `
          -Name 'Kernel32XfpmFb' -Namespace 'Win32XfpmFb' -PassThru -ErrorAction Stop
      )::IsProcessorFeaturePresent(40)
    } catch { $IsBaseline = $false }
  }

  $Suffix     = if ($IsBaseline) { "-baseline" } else { "" }
  $BinaryName = "xfpm-windows-${XfpmArch}${Suffix}.exe"

  # Normalize version
  $ver = $Version
  if ($ver -match "^\d+\.\d+\.\d+$")  { $ver = "xfpm-v$ver" }
  if ($ver -match "^v\d+\.\d+\.\d+$") { $ver = "xfpm-$ver"  }

  $segment = if ($ver -eq "latest") { "latest/download" } else { "download/$ver" }
  $url     = "https://github.com/$REPO/releases/$segment/$BinaryName"
  $tmp     = "$OutPath.tmp"

  Write-Info "Downloading: $url"
  Invoke-Download -URL $url -DestPath $tmp -WithoutCurl $DownloadWithoutCurl

  try {
    if (Test-Path $OutPath) { Remove-Item $OutPath -Force }
    Move-Item $tmp $OutPath -Force
  } catch {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    Write-Fatal "Cannot move binary to '$OutPath': $_"
  }
}

# ─────────────────────────────────────────────────────────────
# Verify the installed binary runs
# ─────────────────────────────────────────────────────────────
function Assert-BinaryWorks {
  param([string]$BinPath)

  if (-not (Test-Path $BinPath)) {
    Write-Fatal "Binary not found at '$BinPath' after installation."
  }

  $revision = & "$BinPath" --version 2>&1
  $exitCode = $LASTEXITCODE

  switch ($exitCode) {
    0 { return $revision.ToString().Trim() }

    1073741795 {  # STATUS_ILLEGAL_INSTRUCTION
      Write-Fatal "$DISPLAY is not compatible with your CPU.`n  Please open an issue: https://github.com/$REPO/issues/new"
    }

    { $_ -in @(3221225781, -1073741515) } {  # STATUS_DLL_NOT_FOUND
      $vcUrl = "https://aka.ms/vs/17/release/vc_redist.x64.exe"
      Write-Fatal "A required DLL is missing.`n  Install Visual C++ Redistributable: $vcUrl"
    }

    default {
      Write-Fatal "Binary verification failed (exit code $exitCode).`n  Command: '$BinPath --version'"
    }
  }
}

# ─────────────────────────────────────────────────────────────
# Remove existing binary cleanly
# ─────────────────────────────────────────────────────────────
function Remove-ExistingBinary {
  param([string]$BinPath)
  if (-not (Test-Path $BinPath)) { return }
  try {
    Remove-Item $BinPath -Force
  } catch [System.UnauthorizedAccessException] {
    $open = Get-Process -Name $TOOL_NAME -ErrorAction SilentlyContinue |
            Where-Object { $_.Path -eq $BinPath }
    if (@($open).Count -gt 0) {
      # Wait up to 3 seconds for the process to exit naturally (since xfpm upgrade now exits after spawn)
      $waitTimeout = 3
      while (@($open).Count -gt 0 -and $waitTimeout -gt 0) {
          Start-Sleep -Seconds 1
          $open = Get-Process -Name $TOOL_NAME -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $BinPath }
          $waitTimeout--
      }

      if (@($open).Count -gt 0) {
        Write-Host "`n${C_YELLOW}[!] Existing $DISPLAY process still running at: $BinPath${C_RESET}"
        Write-Host "    Installation cannot proceed while the binary is in use."
        
        $choice = Read-Host "    Would you like to terminate the process and continue? (y/N)"
        if ($choice -match "^[yY]$") {
          Write-Info "Terminating process..."
          Stop-Process -InputObject $open -Force
          Start-Sleep -Milliseconds 500
          try {
            Remove-Item $BinPath -Force
            return
          } catch {
            Write-Fatal "Still unable to remove '$BinPath'. Please close it manually."
          }
        } else {
          Write-Fatal "Installation aborted by user (process still running)."
        }
      } else {
          # Process exited on its own, try removing again
          try {
            Remove-Item $BinPath -Force
            return
          } catch {
            # Fall through to generic error
          }
      }
    }
    Write-Fatal "Cannot remove '$BinPath' (access denied). Close any program using $TOOL_NAME."
  } catch {
    Write-Fatal "Unexpected error removing existing installation:`n  $_"
  }
}

# ─────────────────────────────────────────────────────────────
# Install symlink aliases (xfpmc → create, xfpmx → exec, dev -> xfpm dev)
# ─────────────────────────────────────────────────────────────
function Install-Aliases {
  param([string]$XfpmBin, [string]$BinPath)

  $aliases = @{
    "xfpmc" = "xfpm create"
    "xfpmx" = "xfpm exec"
    "dev" = "xfpm dev"
  }

  foreach ($alias in $aliases.GetEnumerator()) {
    $aliasExe = Join-Path $XfpmBin "$($alias.Key).cmd"
    $content  = "@echo off`r`n$($alias.Value) %*`r`n"
    try {
      [System.IO.File]::WriteAllText($aliasExe, $content)
      Write-Ok "Alias created: $($alias.Key) -> $($alias.Value)"
    } catch {
      Write-Warn "Could not create alias '$($alias.Key)': $_"
    }
  }
}

# ─────────────────────────────────────────────────────────────
# Install PowerShell completions
# ─────────────────────────────────────────────────────────────
function Install-Completions {
  $profilePaths = @(
    $PROFILE.CurrentUserCurrentHost,
    $PROFILE.CurrentUserAllHosts
  )
  $targetProfile = $null
  foreach ($p in $profilePaths) {
    if ($p -and (Test-Path (Split-Path $p -Parent) -PathType Container)) {
      $targetProfile = $p; break
    }
  }
  if (-not $targetProfile) {
    Write-Warn "Could not locate a PowerShell profile. Completions were not installed."
    return
  }

  $block = @"

# XFPM completions — added automatically by the installer
if (Get-Command $TOOL_NAME -ErrorAction SilentlyContinue) {
    $TOOL_NAME completion powershell | Out-String | Invoke-Expression
}
"@

  if (Test-Path $targetProfile) {
    $existing = Get-Content $targetProfile -Raw -ErrorAction SilentlyContinue
    if ($existing -and $existing.Contains("XFPM completions")) {
      Write-Info "PowerShell completions already present in profile."
      return
    }
  }

  try {
    $dir = Split-Path $targetProfile -Parent
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    Add-Content -Path $targetProfile -Value $block -Encoding UTF8
    Write-Ok "PowerShell completions installed in: $targetProfile"
  } catch {
    Write-Warn "Could not write to profile '$targetProfile': $_"
  }
}

# ─────────────────────────────────────────────────────────────
# Register in Add/Remove Programs
# ─────────────────────────────────────────────────────────────
function Register-Installation {
  param([string]$XfpmRoot, [string]$BinPath, [string]$DisplayVersion)
  $regKey          = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\XFPM"
  $uninstallScript = Join-Path $XfpmRoot "uninstall.ps1"
  try {
    New-Item -Path $regKey -Force | Out-Null
    # Write the uninstaller script
    Set-Content -Path $uninstallScript -Value @"
`$ErrorActionPreference = 'SilentlyContinue'
`$regKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\XFPM'
if (Test-Path `$regKey) { Remove-Item -Path `$regKey -Recurse -Force }
`$bin = Join-Path `$PSScriptRoot 'bin'
`$currentPath = [Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { `$_ -ne '' -and `$_ -ne `$bin }
[Environment]::SetEnvironmentVariable('Path', (`$currentPath -join ';'), 'User')
Get-Process -Name '$TOOL_NAME','xru' | Stop-Process -Force
Start-Sleep -Seconds 1
Remove-Item -Path `$PSScriptRoot -Recurse -Force
Write-Host 'XFPM uninstalled successfully.'
"@ -Force -ErrorAction SilentlyContinue

    $props = @{
      DisplayName     = "$DISPLAY $DisplayVersion"
      DisplayIcon     = $BinPath
      InstallLocation = $XfpmRoot
      Publisher       = "Nehonix Team"
      URLInfoAbout    = "https://github.com/$REPO"
      UninstallString = "powershell -ExecutionPolicy Bypass -File `"$uninstallScript`""
      NoModify        = 1
      NoRepair        = 1
    }
    foreach ($kv in $props.GetEnumerator()) {
      $type = if ($kv.Value -is [int]) { "DWord" } else { "String" }
      New-ItemProperty -Path $regKey -Name $kv.Key -Value $kv.Value -PropertyType $type -Force | Out-Null
    }
    Write-Ok "Registered in Add/Remove Programs."
  } catch {
    Write-Warn "Registry registration failed (non-blocking): $_"
    if (Test-Path $regKey) { Remove-Item -Path $regKey -Force -ErrorAction SilentlyContinue }
  }
}

# ─────────────────────────────────────────────────────────────
# ─────────────────────────────────────────────────────────────
# Update user PATH
# ─────────────────────────────────────────────────────────────
function Update-UserPath {
  param([string]$XfpmBin)

  $currentPath = [Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -ne '' }
  if ($currentPath -contains $XfpmBin) {
    Write-Info "$XfpmBin is already in PATH."
    return
  }

  $newPath  = ($currentPath + $XfpmBin) -join ';'
  [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
  $env:Path = "$env:Path;$XfpmBin"
  Write-Ok "Added $XfpmBin to user PATH."
}

# ─────────────────────────────────────────────────────────────
# Main install flow
# ─────────────────────────────────────────────────────────────
function Install-Xfpm {
  Write-Host ""
  Write-Host "${C_CYAN}${C_BOLD}XFPM Installer (Windows)${C_RESET}"
  Write-Host "${C_CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"

  # 1. Detect platform
  $Arch = Get-TargetArch -ForceBaseline $ForceBaseline
  Write-Info "Platform   : windows/$Arch"
  Write-Info "Version    : $Version"

  # 2. Paths
  $XfpmRoot = if ($env:XFPM_INSTALL) { $env:XFPM_INSTALL } else { Join-Path $HOME ".xfpm" }
  $XfpmBin  = Join-Path $XfpmRoot "bin"
  $BinPath  = Join-Path $XfpmBin  "xfpm.exe"

  try { New-Item -ItemType Directory -Path $XfpmBin -Force | Out-Null }
  catch { Write-Fatal "Cannot create directory '$XfpmBin': $_" }

  # 3. Handle existing installation
  if (Test-Path $BinPath) {
    Write-Warn "Existing $DISPLAY installation found at $BinPath"
    Remove-ExistingBinary -BinPath $BinPath
  }

  # 4. Detect JS runtime
  $JsRuntime = Get-JsRuntime

  if ($null -ne $JsRuntime) {
    Write-Info "JS runtime : $($JsRuntime.Runtime) ($($JsRuntime.Path))"

    # 5a. Download bridge.js into a temp directory
    $TmpDir     = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
    $BridgePath = Join-Path $TmpDir "bridge.js"

    Write-Info "Downloading bridge from: $BRIDGE_URL"
    Invoke-Download -URL $BRIDGE_URL -DestPath $BridgePath -WithoutCurl $DownloadWithoutCurl

    # 5b. Run bridge (places the binary at $BinPath)
    try {
      Invoke-Bridge -BridgePath $BridgePath -JsRuntime $JsRuntime -OutPath $BinPath
    } finally {
      Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
  } else {
    # 5c. No JS runtime — fall back to direct download
    Invoke-DirectDownload -Arch $Arch -OutPath $BinPath
  }

  # 6. Verify binary
  Write-Info "Verifying binary..."
  $InstalledVersion = Assert-BinaryWorks -BinPath $BinPath

  # 7. Install aliases (xfpmc, xfpmx, dev)
  Install-Aliases -XfpmBin $XfpmBin -BinPath $BinPath

  # 8. PowerShell completions
  if (-not $NoCompletions) { Install-Completions }

  # 9. Register in Windows Programs
  if (-not $NoRegisterInstallation) {
    Register-Installation -XfpmRoot $XfpmRoot -BinPath $BinPath -DisplayVersion $InstalledVersion
  }

  # 10. Update PATH
  if (-not $NoPathUpdate) {
    Update-UserPath -XfpmBin $XfpmBin
  } else {
    Write-Info "Skipping PATH update (--NoPathUpdate)."
  }

  # 11. PATH conflict check
  try {
    $existing = Get-Command $TOOL_NAME -ErrorAction Stop
    if ($existing.Source -ne $BinPath) {
      Write-Warn "Note: Another '$TOOL_NAME' is already in PATH at $($existing.Source)`n  Restart your terminal to use the newly installed version."
    }
  } catch {}

  # 12. Deploy native XRU engine & stdlib by default
  Install-Xru -Standalone $false

  # ─── Success ──────────────────────────────────────────────
  Write-Host ""
  Write-Host "${C_GREEN}${C_BOLD}${DISPLAY} ${InstalledVersion} was installed successfully!${C_RESET}"
  Write-Host "  The binary is located at ${C_CYAN}$BinPath${C_RESET}"
  Write-Host "  XRU      : ${C_CYAN}$(Join-Path $XfpmBin 'xru.exe')${C_RESET} ${C_DIM}(stdlib: $(Join-Path $XfpmRoot 'lib\xru\std'))${C_RESET}"
  Write-Host "  Aliases  : ${C_DIM}xfpmc (create), xfpmx (exec)${C_RESET}, dev (xfpm dev)"
  Write-Host ""
  Write-Host "  To get started, restart your terminal and type:"
  Write-Host "  ${C_BOLD}xfpm --help${C_RESET}"
  Write-Host "  ${C_BOLD}xru --help${C_RESET}"
  Write-Host ""

  $global:LASTEXITCODE = 0
}
 
# ─────────────────────────────────────────────────────────────
# XRU (XyPriss Rule Unit) & Stdlib installer flow
# ─────────────────────────────────────────────────────────────
function Install-Xru {
  param([bool]$Standalone = $true)

  if ($Standalone) {
    Write-Host ""
    Write-Host "${C_CYAN}${C_BOLD}XRU Standalone Installer (Windows)${C_RESET}"
    Write-Host "${C_CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"
  } else {
    Write-Info "Deploying native XRU engine & standard library..."
  }

  $arch = "amd64"
  if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }

  $stdDest = Join-Path $XfpmRoot "lib\xru\std"
  Ensure-Directories -Paths @($XfpmBin, $stdDest)
  $destExe = Join-Path $XfpmBin "xru.exe"

  $xruVer = $Version
  if (-not $xruVer -or $xruVer -eq "latest") {
    try {
      $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/Nehonix-Team/xru/releases/latest" -UseBasicParsing -ErrorAction Stop
      $xruVer = $rel.tag_name -replace '^v',''
    } catch {
      $xruVer = "2.0.0"
    }
  } else {
    $xruVer = $xruVer -replace '^v',''
  }
  $xruZipName = "xru-v$xruVer-windows-$arch.zip"
  $xruUrl = "https://github.com/Nehonix-Team/xru/releases/download/v$xruVer/$xruZipName"
  $tmpZip = [System.IO.Path]::GetTempFileName() + ".zip"
  $tmpExtract = Join-Path ([System.IO.Path]::GetTempPath()) ("xru_ext_" + [System.IO.Path]::GetRandomFileName())

  Write-Info "Downloading XRU bundle from: $xruUrl"
  $downloadSuccess = $false
  try {
    Invoke-WebRequest -Uri $xruUrl -OutFile $tmpZip -UseBasicParsing -ErrorAction Stop
    Expand-Archive -Path $tmpZip -DestinationPath $tmpExtract -Force
    $downloadSuccess = $true
  } catch {
    # Fallback to direct raw .exe download
    $rawExeUrl = "https://github.com/Nehonix-Team/xru/releases/latest/download/xru-windows-$arch.exe"
    try {
      Invoke-WebRequest -Uri $rawExeUrl -OutFile $destExe -UseBasicParsing -ErrorAction Stop
    } catch {}
  }

  if ($downloadSuccess) {
    $extractedExe = Join-Path $tmpExtract "bin\xru.exe"
    if (Test-Path $extractedExe) {
      Copy-Item $extractedExe $destExe -Force
    }
    $extractedStd = Join-Path $tmpExtract "std"
    if (Test-Path $extractedStd) {
      if (Test-Path $stdDest) { Remove-Item $stdDest -Recurse -Force -ErrorAction SilentlyContinue }
      Ensure-Directories -Paths @($stdDest)
      Copy-Item (Join-Path $extractedStd "*") $stdDest -Recurse -Force
    }
  }

  # Cleanup temp files
  if (Test-Path $tmpZip) { Remove-Item $tmpZip -Force -ErrorAction SilentlyContinue }
  if (Test-Path $tmpExtract) { Remove-Item $tmpExtract -Recurse -Force -ErrorAction SilentlyContinue }

  # Clean legacy Windows binary locations to prevent PATH precedence conflicts
  $legacyExes = @(
    (Join-Path $env:USERPROFILE ".xru\bin\xru.exe"),
    (Join-Path $env:USERPROFILE ".local\bin\xru.exe"),
    (Join-Path $env:USERPROFILE "bin\xru.exe")
  )
  foreach ($leg in $legacyExes) {
    if (Test-Path $leg -and $leg -ne $destExe) {
      Remove-Item $leg -Force -ErrorAction SilentlyContinue
    }
  }

  if ($Standalone) {
    if (-not $NoPathUpdate) {
      Update-UserPath -XfpmBin $XfpmBin
    }
    Write-Host ""
    Write-Host "${C_GREEN}${C_BOLD}✓ XRU installed successfully!${C_RESET}"
    Write-Host "  Binary : ${C_CYAN}$destExe${C_RESET}"
    Write-Host "  Stdlib : ${C_CYAN}$stdDest${C_RESET}"
    Write-Host "  Usage  : ${C_YELLOW}xru <script.xru>${C_RESET}"
    Write-Host ""
  } else {
    Write-Ok "XRU engine & standard library installed at: ${C_CYAN}$stdDest${C_RESET}"
  }
}

# ─────────────────────────────────────────────────────────────
# Uninstall
# ─────────────────────────────────────────────────────────────
function Uninstall-Xfpm {
  Write-Info "Uninstalling XFPM..."

  # Remove Registry Key
  $regKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\XFPM"
  if (Test-Path $regKey) {
    Remove-Item -Path $regKey -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "Removed Windows Registry entries."
  }

  # Remove from PATH
  $currentPath = [Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -ne '' -and $_ -ne $XfpmBin }
  $newPath = $currentPath -join ';'
  [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
  Publish-Env
  Write-Ok "Cleaned PATH environment."

  # Remove Files
  if (Test-Path $XfpmRoot) {
    Get-Process -Name $TOOL_NAME, "xru" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    Remove-Item -Path $XfpmRoot -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "Removed installation folder: $XfpmRoot"
  }

  Write-Host ""
  Write-Host "${C_GREEN}${C_BOLD}${DISPLAY} was successfully uninstalled.${C_RESET}"
  Write-Host ""
}

# ─────────────────────────────────────────────────────────────
# Entry point
# ─────────────────────────────────────────────────────────────
if ($Action -eq "uninstall") {
  Uninstall-Xfpm
} elseif ($Action -eq "xru" -or $Xru) {
  Install-Xru -Standalone $true
} else {
  Install-Xfpm
}
