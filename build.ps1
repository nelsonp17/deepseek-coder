#Requires -Version 7
<#
.SYNOPSIS
    Compila deekseep-coder con la versión estampada en el binario.

.DESCRIPTION
    Sin -All produce solo el binario de esta máquina. Con -All genera los tres
    objetivos habituales (Windows/amd64, Linux/amd64, macOS/arm64).

.PARAMETER Version
    Versión que reportará `deekseep --version`. Por defecto "dev", que es lo que
    dice un binario sin estampar.

.PARAMETER OutDir
    Carpeta de salida. Por defecto "dist".

.EXAMPLE
    ./build.ps1 -Version v0.1.0
    ./build.ps1 -Version v0.1.0 -All
#>
param(
    [string]$Version = "dev",
    [string]$OutDir = "dist",
    [switch]$All
)

$ErrorActionPreference = "Stop"

if ($Version -match '\s') {
    throw "La versión no puede contener espacios: '$Version'"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# -s -w quitan símbolos de depuración; -trimpath quita rutas absolutas.
$ldflags = "-s -w -X main.version=$Version"

function Build-Target {
    param([string]$GoOS, [string]$GoArch, [string]$Name)

    Write-Host "-> $GoOS/$GoArch  $Name"
    $env:GOOS = $GoOS
    $env:GOARCH = $GoArch
    go build -trimpath -ldflags $ldflags -o (Join-Path $OutDir $Name) ./cli
    if ($LASTEXITCODE -ne 0) {
        throw "fallo la compilacion para $GoOS/$GoArch"
    }
}

if ($All) {
    # El de Windows es también el que se usa en esta máquina, así que lleva el
    # nombre corto: es el binario que el usuario va a ejecutar.
    Build-Target "windows" "amd64" "deekseep.exe"
    Build-Target "linux" "amd64" "deekseep-linux-amd64"
    Build-Target "darwin" "arm64" "deekseep-macos-arm64"
}
else {
    Build-Target "windows" "amd64" "deekseep.exe"
}

Write-Host ""
Get-ChildItem $OutDir -File | ForEach-Object {
    "{0,-32} {1,6:N1} MB" -f $_.Name, ($_.Length / 1MB)
}
Write-Host "`nVersion estampada: $Version"
