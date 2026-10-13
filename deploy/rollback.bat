@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0rollback.ps1" %*
if errorlevel 1 pause