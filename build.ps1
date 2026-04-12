# build.ps1 — One-step build for MultiSnek on Windows.
# Produces a single portable exe with no external DLL dependencies.
# Usage: .\build.ps1

$ErrorActionPreference = "Stop"

# Ensure MinGW is on PATH for CGo
$mingw = "C:\msys64\mingw64\bin"
if (Test-Path $mingw) {
    $env:PATH = "$mingw;$env:PATH"
} else {
    Write-Error "MinGW not found at $mingw — install via: winget install MSYS2.MSYS2"
    exit 1
}

$env:CGO_ENABLED = "1"

# Static-link libopus, libopusfile, libogg so the exe is fully self-contained.
$env:CGO_LDFLAGS = "-static -LC:/msys64/mingw64/lib -lopus -lopusfile -logg -lm -lstdc++ -lws2_32 -lssp"

Write-Host "Building Multisnek (static, single exe)..." -ForegroundColor Cyan
$wails = Get-Command wails -ErrorAction SilentlyContinue
if (-not $wails) {
    $candidatePaths = @(
        (Join-Path $env:USERPROFILE "go\bin\wails.exe"),
        $(if ($env:GOPATH) { Join-Path $env:GOPATH "bin\wails.exe" })
    ) | Where-Object { $_ -and (Test-Path $_) }
    if ($candidatePaths.Count -gt 0) {
        $wails = Get-Item $candidatePaths[0]
    }
}
if (-not $wails) {
    Write-Error "Wails CLI not found. Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
}
$wailsPath = if ($wails.PSObject.Properties.Name -contains "Source" -and $wails.Source) {
    $wails.Source
} else {
    $wails.FullName
}
& $wailsPath build

if ($LASTEXITCODE -eq 0) {
    $exe = Get-Item "build\bin\Multisnek.exe"
    $sizeMB = "{0:N1}" -f ($exe.Length / 1MB)
    Write-Host "`nDone: $($exe.FullName)  ($sizeMB MB)" -ForegroundColor Green
} else {
    Write-Host "`nBuild failed." -ForegroundColor Red
    exit 1
}
