@echo off
REM Quick run script - assumes build.bat has been run at least once

if not exist weight.exe (
    echo weight.exe not found. Running build.bat first...
    call build.bat
    exit /b
)

REM Check if .env exists
if not exist .env (
    echo ERROR: .env file not found!
    echo Please run build.bat first to create it.
    pause
    exit /b 1
)

REM Load environment variables
for /f "usebackq tokens=1,* delims==" %%a in (.env) do (
    echo %%a | findstr /r "^#" >nul
    if errorlevel 1 (
        if not "%%a"=="" (
            set "%%a=%%b"
        )
    )
)

echo Starting server on http://localhost:%PORT%
echo Press Ctrl+C to stop the server
echo.

weight.exe
