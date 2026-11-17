@echo off
rem katyoti bootstrap (Windows) -- DOUBLE-CLICK this file from the channel folder.
rem
rem Windows opens a .ps1 in Notepad; this launcher runs install.ps1 from its own
rem folder (%~dp0, a local path, a mapped drive or a UNC share alike). The
rem policy bypass is per-process: it changes nothing on the machine. The
rem installer derives the channel from that folder, so nothing is typed.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1"
if errorlevel 1 (
    echo.
    echo install failed -- see above.
    pause
)
