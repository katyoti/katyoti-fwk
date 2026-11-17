# katyoti bootstrap (Windows).
#
#   piped (URL):   $u='<channel-url>'; $env:KTW_INFRA_CHANNEL=$u; irm "$u/install.ps1" | iex
#   from a folder: & '<channel-folder>\install.ps1'        (or double-click install-on-windows.cmd)
#
# Makes an installation: a folder of its own (%LOCALAPPDATA%\Programs\Katyoti\
# <solution>-<version>, or KTW_INSTALLATION_DIR) whose katyoti.yml says where its
# roots are and how it starts -- read by every katyoti program of it, so it runs
# beside any other installation without touching it. Fetches only katyoti.exe,
# verifies it against the channel's signed SHA256SUMS, then lets katyoti pull
# ktw-hub + ktw-gui from the same channel. No PATH or user variable is written.
$ErrorActionPreference = 'Stop'

# The channel: the variable when given, else the folder this script sits in.
# $PSScriptRoot is empty for a piped script -- it has no folder to be in.
$channel = $env:KTW_INFRA_CHANNEL
if (-not $channel) { $channel = $PSScriptRoot }
if (-not $channel) {
    throw "Set KTW_INFRA_CHANNEL to the infra channel URL or folder before running this (a piped script cannot know where it came from)."
}
$channel = $channel.TrimEnd('/', '\')

function Get-ChannelText($rel) {
    if ($channel -match '^https?://') { return (Invoke-WebRequest -UseBasicParsing "$channel/$rel").Content }
    return Get-Content -Raw (Join-Path $channel $rel)
}

$version = ((Get-ChannelText 'channel.yaml') -split "`n" |
    Where-Object { $_ -match '^\s*version\s*:' } |
    ForEach-Object { ($_ -split ':', 2)[1].Trim() } | Select-Object -First 1)
if (-not $version) { throw "channel.yaml at $channel declares no version" }

$arch = if ([Environment]::Is64BitOperatingSystem) { 'amd64' } else { '386' }
$platform = "windows-$arch"
Write-Host "katyoti $version ($platform) from $channel"

# The installation: named after the solution the channel opens on (its
# `solution:`, `demo:` in an older channel) and the version, so another version
# or solution is another folder. Installing the same one again repairs it.
# Nothing of it is passed to setup: setup reads the channel itself.
$pointer = (Get-ChannelText 'channel.yaml') -split "`n"
$solution = ($pointer | Where-Object { $_ -match '^\s*solution\s*:' } |
    ForEach-Object { ($_ -split ':', 2)[1].Trim() } | Select-Object -First 1)
if (-not $solution) {
    $solution = ($pointer | Where-Object { $_ -match '^\s*demo\s*:' } |
        ForEach-Object { ($_ -split ':', 2)[1].Trim() } | Select-Object -First 1)
}
$name = if ($solution) { "$(($solution -split '@')[0]) $(($solution -split '@')[1])" } else { "katyoti $version" }
$slug = ($name.ToLower() -replace '[^a-z0-9._-]+', '-').Trim('-')
$dir = $env:KTW_INSTALLATION_DIR
if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA "Programs\Katyoti\$slug" }
$root = Join-Path $dir 'root'
New-Item -ItemType Directory -Force -Path $root | Out-Null
$file = Join-Path $dir 'katyoti.yml'
if (-not (Test-Path $file)) {
    $yml = "# A katyoti installation: every katyoti program of it reads this file first.`n" +
        "name: $name`n"
    if ($solution) { $yml += "solution: $solution`n" }
    $yml += "roots:`n  config: root`n  data: data`n  cache: cache`n"
    [IO.File]::WriteAllText($file, $yml)
}
# The channel, recorded in the installation (setup pins its key on first use).
$record = Join-Path $root 'infra-channel.yaml'
if (-not (Test-Path $record)) { [IO.File]::WriteAllText($record, "root: $($channel -replace '\\', '/')`n") }

$dest = Join-Path $root 'control-plane'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
$exe = Join-Path $dest 'katyoti.exe'

$src = "$channel/$version/$platform/katyoti.exe"
if ($channel -match '^https?://') {
    Invoke-WebRequest -UseBasicParsing $src -OutFile "$exe.new"
} else {
    Copy-Item (Join-Path $channel "$version\$platform\katyoti.exe") "$exe.new" -Force
}

# Integrity: the channel's SHA256SUMS is signed, so a swapped binary is caught
# here rather than after it has installed everything else.
$want = ((Get-ChannelText "$version/$platform/SHA256SUMS") -split "`n" |
    Where-Object { $_ -match 'katyoti\.exe' } |
    ForEach-Object { ($_ -split '\s+')[0] } | Select-Object -First 1)
$got = (Get-FileHash "$exe.new" -Algorithm SHA256).Hash.ToLower()
if ($want -and $got -ne $want.ToLower()) {
    Remove-Item "$exe.new" -Force
    throw "katyoti.exe checksum mismatch (want $want, got $got)"
}
Move-Item "$exe.new" $exe -Force

Write-Host "installed $exe (installation $dir)"

# `setup` finishes the job: the rest of the control plane, remembering this
# channel, subscribing the marketplace beside it, the installation's Start-menu
# and Desktop shortcuts « KaTyoti <name> », and opening the GUI. One
# instruction in, a running app out -- no follow-up homework. It reads the
# installation's katyoti.yml above it; the channel is the one recorded above.
# KTW_SETUP_ARGS passes options to setup (`--no-gui` for a headless check).
$setupArgs = @()
if ($env:KTW_SETUP_ARGS) { $setupArgs = $env:KTW_SETUP_ARGS -split '\s+' }
& $exe setup @setupArgs
if ($LASTEXITCODE -ne 0) { throw "katyoti setup failed (exit $LASTEXITCODE)" }
