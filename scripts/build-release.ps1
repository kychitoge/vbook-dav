param (
    [string]$Version = "v1.0.0"
)

$ErrorActionPreference = "Stop"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  vbook-dav - Multi-Platform Release Builder" -ForegroundColor Cyan
Write-Host "  Target Version: $Version" -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Cyan

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$distDir = Join-Path $rootDir "dist"

if (Test-Path $distDir) {
    Remove-Item -Recurse -Force $distDir
}
New-Item -ItemType Directory -Force -Path $distDir | Out-Null

$ldflags = "-s -w"

$targets = @(
    @{ OS = "linux";   Arch = "amd64"; Name = "linux_amd64";   Ext = "";     Archive = "tar.gz" },
    @{ OS = "linux";   Arch = "arm64"; Name = "linux_arm64";   Ext = "";     Archive = "tar.gz" },
    @{ OS = "windows"; Arch = "amd64"; Name = "windows_amd64"; Ext = ".exe"; Archive = "zip"    },
    @{ OS = "darwin";  Arch = "arm64"; Name = "darwin_arm64";  Ext = "";     Archive = "tar.gz" }
)

foreach ($t in $targets) {
    $binName = "vbook-dav" + $t.Ext
    $binPath = Join-Path $distDir $binName
    $archiveBase = "vbook-dav_${Version}_$($t.Name)"

    Write-Host "[build] Compiling for $($t.OS)/$($t.Arch)..." -ForegroundColor Yellow

    $env:CGO_ENABLED = "0"
    $env:GOOS = $t.OS
    $env:GOARCH = $t.Arch

    & go build "-ldflags=$ldflags" -trimpath -o $binPath ./cmd/dav
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build for $($t.OS)/$($t.Arch)"
    }

    if ($t.Archive -eq "zip") {
        $archivePath = Join-Path $distDir "$archiveBase.zip"
        tar.exe -a -c -f $archivePath -C $distDir $binName -C $rootDir README.md LICENSE .env.example
    } else {
        $archivePath = Join-Path $distDir "$archiveBase.tar.gz"
        tar.exe -czvf $archivePath -C $distDir $binName -C $rootDir README.md LICENSE .env.example
    }

    Remove-Item -Force $binPath
    Write-Host "[ok   ] Created: $archiveBase.$($t.Archive)" -ForegroundColor Green
}

# Reset environment variables
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue

# Generate SHA256 checksums
Write-Host "[info ] Computing SHA256 checksums..." -ForegroundColor Gray
$checksumFile = Join-Path $distDir "checksums.txt"
$files = Get-ChildItem -Path $distDir -File | Where-Object { $_.Name -ne "checksums.txt" }
$lines = @()
foreach ($f in $files) {
    $hash = (Get-FileHash -Path $f.FullName -Algorithm SHA256).Hash.ToLower()
    $lines += "$hash  $($f.Name)"
}
$lines | Out-File -FilePath $checksumFile -Encoding utf8

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  Build complete! Release assets ready in: dist/" -ForegroundColor Green
Get-ChildItem -Path $distDir | Select-Object Name, Length | Format-Table -AutoSize
Write-Host "============================================================" -ForegroundColor Cyan
