@echo off
REM Build and run Weight application

echo Building Weight application...
go build -o weight.exe .
if %ERRORLEVEL% NEQ 0 (
    echo Build failed!
    pause
    exit /b 1
)

REM Check if .env exists, create from .env.example if missing
if not exist .env (
    echo WARNING: .env file not found!
    copy .env.example .env
    echo Created .env from .env.example. Edit if needed, then run again.
    pause
    exit /b 1
)

REM Load environment variables from .env
for /f "usebackq tokens=1,* delims==" %%a in (.env) do (
    REM Skip comments and empty lines
    echo %%a | findstr /r "^#" >nul
    if errorlevel 1 (
        if not "%%a"=="" (
            set "%%a=%%b"
        )
    )
)

echo Production mode: %PROD%
echo.
echo Starting server on http://localhost:%PORT%
echo Press Ctrl+C to stop the server
echo.

weight.exe
