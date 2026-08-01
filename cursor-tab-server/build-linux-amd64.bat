@echo off
setlocal
REM lyh用cursor修改 2026-08-01：在 Windows 一键生成无 CGO 依赖的 Linux amd64 产物，供宝塔直接上传部署。

cd /d "%~dp0"
set "OUTPUT_DIR=%CD%\dist"
set "OUTPUT_FILE=%OUTPUT_DIR%\cursor-tab-server-linux-amd64"

where go >nul 2>nul
if errorlevel 1 (
    echo [ERROR] Go was not found. Install Go and add it to PATH.
    exit /b 1
)

if not exist "go.mod" (
    echo [ERROR] go.mod was not found in %CD%.
    exit /b 1
)

if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"
if errorlevel 1 (
    echo [ERROR] Failed to create %OUTPUT_DIR%.
    exit /b 1
)

echo [1/3] Checking dependencies...
go mod verify
if errorlevel 1 goto :failed

echo [2/3] Running tests for the current Windows host...
go test ./...
if errorlevel 1 goto :failed

REM lyh用cursor修改 2026-08-01：测试完成后再切换 Linux 目标，避免 Windows 执行交叉编译的测试程序。
set "CGO_ENABLED=0"
set "GOOS=linux"
set "GOARCH=amd64"

echo [3/3] Building Linux amd64 binary...
go build -trimpath -ldflags="-s -w" -o "%OUTPUT_FILE%" .
if errorlevel 1 goto :failed

copy /Y "config.example.yaml" "%OUTPUT_DIR%\config.example.yaml" >nul
if errorlevel 1 goto :failed
copy /Y "DEPLOY_BT.md" "%OUTPUT_DIR%\DEPLOY_BT.md" >nul
if errorlevel 1 goto :failed

echo.
echo [OK] Build completed:
echo      %OUTPUT_FILE%
exit /b 0

:failed
echo.
echo [ERROR] Linux build failed.
exit /b 1