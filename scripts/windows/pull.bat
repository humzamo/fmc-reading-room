@echo off
rem Pulls the latest commits for whatever repo this script lives in.
rem Meant to be run daily (see setup-daily-pull.bat) to keep a local clone
rem of fmc-reading-room in sync with the nightly GitHub Actions job.

for /f "delims=" %%i in ('git -C "%~dp0" rev-parse --show-toplevel 2^>nul') do set REPO_DIR=%%i
if not defined REPO_DIR (
    echo Could not find a git repo above %~dp0 — is git installed and on PATH?
    exit /b 1
)

set LOG_FILE=%REPO_DIR%\pull.log
echo [%date% %time%] Pulling... >> "%LOG_FILE%"
git -C "%REPO_DIR%" pull --ff-only origin main >> "%LOG_FILE%" 2>&1
echo [%date% %time%] Exit code %ERRORLEVEL% >> "%LOG_FILE%"
