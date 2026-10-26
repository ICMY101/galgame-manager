@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo ============================================
echo   Galgame 管理器 - 一键打包
echo ============================================
echo.

where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 未找到 Go，请先安装 Go 1.21+ 并加入 PATH
    pause & exit /b 1
)

where node >nul 2>nul
if errorlevel 1 (
    echo [错误] 未找到 Node.js，请先安装 Node 18+ 并加入 PATH
    pause & exit /b 1
)

where wails >nul 2>nul
if errorlevel 1 (
    echo [提示] 未找到 wails CLI，正在安装...
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
    set "PATH=%USERPROFILE%\go\bin;%PATH%"
)

echo [1/3] 安装前端依赖...
pushd frontend
if not exist node_modules (
    call npm install || (echo [错误] npm install 失败 & popd & pause & exit /b 1)
)
popd

echo [2/3] 构建应用...
wails build -clean -o GalgameManager.exe || (echo [错误] 构建失败 & pause & exit /b 1)

echo.
echo [3/3] 完成
echo 产物: %cd%\build\bin\GalgameManager.exe
echo.
pause
