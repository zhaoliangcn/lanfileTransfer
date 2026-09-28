param(
    [switch]$Dev,
    [switch]$Clean,
    [switch]$SkipFrontend,
    [switch]$SkipTests,
    [string]$Platform = "",
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$PROJECT_ROOT = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Definition)
$BUILD_DIR = Join-Path $PROJECT_ROOT "build\bin"

$GOOS = ""
$GOARCH = ""
$APP_NAME = "LanFileTransfer-Go.exe"

function Write-Banner {
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "  LanFileTransfer-Go Build Script" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host ""
}

function Show-Usage {
    Write-Host "Usage: .\scripts\build.ps1 [options]" -ForegroundColor White
    Write-Host ""
    Write-Host "Options:" -ForegroundColor Yellow
    Write-Host "  -Dev             Build in development mode (debug tools enabled)" -ForegroundColor White
    Write-Host "  -Clean           Clean build directory before building" -ForegroundColor White
    Write-Host "  -SkipFrontend    Skip frontend dependency installation" -ForegroundColor White
    Write-Host "  -SkipTests       Skip running Go tests before build" -ForegroundColor White
    Write-Host "  -Platform <str>  Cross-compile target (e.g.: windows/amd64, linux/amd64, darwin/amd64)" -ForegroundColor White
    Write-Host "  -Help            Show this help message" -ForegroundColor White
    Write-Host ""
    Write-Host "Examples:" -ForegroundColor Yellow
    Write-Host "  .\scripts\build.ps1                        # Production build" -ForegroundColor White
    Write-Host "  .\scripts\build.ps1 -Dev                   # Development build" -ForegroundColor White
    Write-Host "  .\scripts\build.ps1 -Clean -SkipTests      # Clean build, skip tests" -ForegroundColor White
    Write-Host "  .\scripts\build.ps1 -Platform linux/amd64  # Cross-compile for Linux" -ForegroundColor White
}

function Check-Command {
    param($Name, $Hint)
    if (!(Get-Command $Name -ErrorAction SilentlyContinue)) {
        Write-Host "[ERROR] $Name not found!" -ForegroundColor Red
        if ($Hint) { Write-Host "  Hint: $Hint" -ForegroundColor Yellow }
        exit 1
    }
    if ($Name -eq "go") {
        Write-Host "[OK] $Name found: $(& $Name version 2>&1 | Select-Object -First 1)" -ForegroundColor Green
    }
    else {
        Write-Host "[OK] $Name found: $(& $Name --version 2>&1 | Select-Object -First 1)" -ForegroundColor Green
    }
}

function Check-Prerequisites {
    Write-Host "--- Checking prerequisites ---" -ForegroundColor Yellow

    Check-Command "go" "Install Go 1.23+: https://go.dev/dl/"
    $goVersion = go version
    if ($goVersion -notmatch 'go1\.2[3-9]|go1\.[3-9]\d') {
        Write-Host "[WARN] Go version may be too old. Recommended: 1.23+" -ForegroundColor Yellow
    }

    Check-Command "node" "Install Node.js 18+: https://nodejs.org/"
    Check-Command "npm" "Node.js should include npm"

    $wails = Get-Command "wails" -ErrorAction SilentlyContinue
    if (-not $wails) {
        Write-Host "[INFO] Wails CLI not found, installing..." -ForegroundColor Yellow
        go install github.com/wailsapp/wails/v2/cmd/wails@latest
        $gopath = go env GOPATH
        $env:Path += ";$gopath\bin"
        if (!(Get-Command "wails" -ErrorAction SilentlyContinue)) {
            Write-Host "[ERROR] Wails CLI installation failed. Add GOPATH/bin to PATH and retry." -ForegroundColor Red
            exit 1
        }
    }
    Write-Host "[OK] Wails CLI: $(wails version)" -ForegroundColor Green
    Write-Host ""
}

function Install-FrontendDeps {
    if ($SkipFrontend) {
        Write-Host "[SKIP] Frontend dependency installation skipped" -ForegroundColor Yellow
        return
    }
    Write-Host "--- Installing frontend dependencies ---" -ForegroundColor Yellow
    $frontendDir = Join-Path $PROJECT_ROOT "frontend"
    if (-not (Test-Path (Join-Path $frontendDir "node_modules"))) {
        Push-Location $frontendDir
        try {
            npm install
            Write-Host "[OK] Frontend dependencies installed" -ForegroundColor Green
        }
        catch {
            Write-Host "[ERROR] Failed to install frontend dependencies: $_" -ForegroundColor Red
            exit 1
        }
        finally {
            Pop-Location
        }
    }
    else {
        Write-Host "[OK] Frontend dependencies already installed" -ForegroundColor Green
    }
    Write-Host ""
}

function Run-Tests {
    if ($SkipTests) {
        Write-Host "[SKIP] Tests skipped" -ForegroundColor Yellow
        return
    }
    Write-Host "--- Running tests ---" -ForegroundColor Yellow
    Push-Location $PROJECT_ROOT
    try {
        $env:GOOS = ""
        $env:GOARCH = ""
        $result = go test ./... -count=1 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[ERROR] Tests failed:" -ForegroundColor Red
            Write-Host $result
            exit 1
        }
        Write-Host "[OK] All tests passed" -ForegroundColor Green
    }
    finally {
        Pop-Location
    }
    Write-Host ""
}

function Clean-Build {
    Write-Host "--- Cleaning build directory ---" -ForegroundColor Yellow
    if (Test-Path $BUILD_DIR) {
        Remove-Item -Path "$BUILD_DIR\*" -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "[OK] Build directory cleaned" -ForegroundColor Green
    }
    else {
        New-Item -ItemType Directory -Path $BUILD_DIR -Force | Out-Null
        Write-Host "[OK] Build directory created" -ForegroundColor Green
    }
    Write-Host ""
}

function Resolve-Platform {
    if (-not $Platform) {
        return
    }

    $parts = $Platform -split '/'
    if ($parts.Count -ne 2) {
        Write-Host "[ERROR] Invalid platform format. Use: os/arch (e.g.: windows/amd64)" -ForegroundColor Red
        exit 1
    }

    $script:GOOS = $parts[0]
    $script:GOARCH = $parts[1]

    switch ($GOOS) {
        "windows" { $script:APP_NAME = "LanFileTransfer-Go.exe" }
        "linux"   { $script:APP_NAME = "LanFileTransfer-Go" }
        "darwin"  { $script:APP_NAME = "LanFileTransfer-Go" }
        default   {
            Write-Host "[ERROR] Unsupported OS: $GOOS" -ForegroundColor Red
            exit 1
        }
    }

    Write-Host "[INFO] Cross-compiling for: $GOOS/$GOARCH" -ForegroundColor Cyan
    Write-Host ""
}

function Build-App {
    Write-Host "--- Building application ---" -ForegroundColor Yellow

    Push-Location $PROJECT_ROOT
    try {
        $env:GOOS = $GOOS
        $env:GOARCH = $GOARCH

        $buildArgs = @("build")

        if ($Dev) {
            $buildArgs += "-debug"
            Write-Host "[INFO] Build mode: dev (debug enabled)" -ForegroundColor Cyan
        }
        else {
            if ($GOOS -eq "windows" -or (-not $GOOS)) {
                $buildArgs += "-tags"
                $buildArgs += "native_webview2loader"
            }
            Write-Host "[INFO] Build mode: production" -ForegroundColor Cyan
        }

        $buildArgs += "-o"
        $buildArgs += $APP_NAME

        & wails $buildArgs
        if ($LASTEXITCODE -ne 0) {
            throw "Build failed with exit code $LASTEXITCODE"
        }

        $outputPath = Join-Path $BUILD_DIR $APP_NAME
        if (Test-Path $outputPath) {
            $fileInfo = Get-Item $outputPath
            $sizeMB = [math]::Round($fileInfo.Length / 1MB, 2)
            Write-Host ""
            Write-Host "[OK] Build successful!" -ForegroundColor Green
            Write-Host "  Output: $outputPath" -ForegroundColor White
            Write-Host "  Size:   $sizeMB MB" -ForegroundColor White
        }
        else {
            Write-Host "[ERROR] Output file not found: $outputPath" -ForegroundColor Red
            exit 1
        }
    }
    finally {
        $env:GOOS = ""
        $env:GOARCH = ""
        Pop-Location
    }
    Write-Host ""
}

if ($Help) {
    Show-Usage
    exit 0
}

Write-Banner
Check-Prerequisites
Install-FrontendDeps
Run-Tests

if ($Clean) {
    Clean-Build
}

Resolve-Platform
Build-App

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Build completed!" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan