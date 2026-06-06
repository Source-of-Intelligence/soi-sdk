# build.ps1 — Build & package xlsx2md SOI plugin
param(
    [switch]$Clean,
    [switch]$Package
)
$ErrorActionPreference = "Stop"

# ----- helpers -----
function _B { param($M) Write-Host "=== $M ===" -ForegroundColor Cyan }
function _S { param($M) Write-Host "  [>] $M" -ForegroundColor Yellow }
function _OK { param($M) Write-Host "  [+] $M" -ForegroundColor Green }
function _W  { param($M) Write-Host "  [!] $M" -ForegroundColor Yellow }
function _E  { param($M) Write-Host "  [-] $M" -ForegroundColor Red; exit 1 }

# ----- locate tinygo -----
$tinygo = Get-Command tinygo -ErrorAction SilentlyContinue | ForEach-Object Source
if (-not $tinygo) {
    @("$env:LOCALAPPDATA\tinygo\bin\tinygo.exe","C:\tinygo\bin\tinygo.exe","D:\sdk\tinygo\bin\tinygo.exe") | ForEach-Object {
        if (Test-Path $_) { $tinygo = $_ }
    }
}
if (-not $tinygo) { _E "TinyGo not found" }

# ----- locate wasm-opt -----
$wasmOpt = Get-Command wasm-opt -ErrorAction SilentlyContinue | ForEach-Object Source
if (-not $wasmOpt) {
    @("$env:LOCALAPPDATA\binaryen\bin\wasm-opt.exe","$env:ProgramFiles\binaryen\bin\wasm-opt.exe","$env:ProgramFiles\Binaryen\bin\wasm-opt.exe",
      "C:\binaryen\bin\wasm-opt.exe","D:\binaryen\bin\wasm-opt.exe",
      "E:\code\soi\soi-sdk\binaryen-version_121\bin\wasm-opt.exe") | ForEach-Object {
        if (Test-Path $_) { $wasmOpt = $_ }
    }
}
if ($wasmOpt) {
    $env:WASMOPT = $wasmOpt
    $env:Path = "$(Split-Path $wasmOpt);$env:Path"
    _OK "wasm-opt: $wasmOpt"
} else {
    _W "wasm-opt not found — TinyGo may fail without Binaryen"
}

# ----- clean -----
if ($Clean) {
    _B "Clean"
    Remove-Item -Recurse -Force wasm -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force dist -ErrorAction SilentlyContinue
    _OK "Done"
    return
}

# ----- build -----
_B "Build xlsx2md"
New-Item -ItemType Directory -Force -Path wasm | Out-Null
$soi   = "wasm\plugin.soi"
$name  = "xlsx2md"
$ver   = "1.0.0"

if (Test-Path skill.yaml) {
    $y = Get-Content skill.yaml -Raw
    if ($y -match '(?m)name:\s*"?(\S[^"#\r\n]*)') { $name = $Matches[1].Trim().Trim('"') }
    if ($y -match '(?m)version:\s*"?(\S[^"#\r\n]*)') { $ver = $Matches[1].Trim().Trim('"') }
}

Write-Host "  compiler : $tinygo" -ForegroundColor Gray
Write-Host "  target   : $soi" -ForegroundColor Gray
Push-Location $PSScriptRoot
& $tinygo build -target=wasi -o $soi .
$ok = ($LASTEXITCODE -eq 0)
Pop-Location
if (-not $ok) { _E "TinyGo build failed" }

$sz = [math]::Round((Get-Item $soi).Length / 1024, 2)
_OK "$soi ($sz KB)"

# ----- verify -----
_S "Verify symbols"
$bytes = [System.IO.File]::ReadAllBytes((Resolve-Path $soi))
$text  = [System.Text.Encoding]::UTF8.GetString($bytes)
if ($text -match "execute")         { _OK "execute"       } else { _E "execute MISSING"       }
if ($text -match "registerTools")   { _OK "registerTools" } else { _E "registerTools MISSING — add //export to tools.go" }

if (-not $Package) { Write-Host "Done!" -ForegroundColor Green; return }

# ----- zip -----
_B "Package"
$dist = "dist"
$pkg  = "$dist\$name-$ver"
New-Item -ItemType Directory -Force -Path "$pkg\wasm" | Out-Null
Copy-Item $soi "$pkg\wasm\"
if (Test-Path skill.yaml) { Copy-Item skill.yaml $pkg\ }
if (Test-Path README.md)  { Copy-Item README.md  $pkg\ }
$zip = "$pkg.zip"
Compress-Archive -Path "$pkg\*" -DestinationPath $zip -Force
Remove-Item -Recurse -Force $pkg
$zsz = [math]::Round((Get-Item $zip).Length / 1024, 2)
_OK "$zip ($zsz KB)"
