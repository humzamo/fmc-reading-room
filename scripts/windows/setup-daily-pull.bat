@echo off
rem One-time setup: registers a Windows Scheduled Task that runs pull.bat
rem every day at 11:00 local time. Re-running this is safe (/f overwrites
rem the existing task). The task only runs while you're logged in; see
rem README.md if you need it to run even when logged out.

schtasks /create /tn "FMC Reading Room Daily Pull" /tr "\"%~dp0pull.bat\"" /sc daily /st 11:00 /f

if %ERRORLEVEL% equ 0 (
    echo Scheduled. Run "schtasks /query /tn \"FMC Reading Room Daily Pull\"" to check it, or open Task Scheduler.
) else (
    echo Failed to create the scheduled task — see the error above.
)
