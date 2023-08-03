@echo off
setlocal enabledelayedexpansion

:menu
echo.
echo gen-api
echo gen-swagger [directory path]
echo.
echo Please enter your command:
set /p cmd=

if /i "%cmd%"=="gen-api" goto gen_api
if /i "%cmd:~0,11%"=="gen-swagger" goto gen_swagger
echo Invalid command. Please try again.
goto menu

:gen_api
set "scriptdir=%~dp0"
cd /d "%scriptdir%"
goctl api go -api "%scriptdir%api\powerx.api" -dir .
del powerx.go
echo gen-api has been executed successfully.
goto menu

:gen_swagger
set dir=%cmd:~12%
set "scriptdir=%~dp0"
cd /d "%scriptdir%"
for %%f in ("%scriptdir%%dir%\*.api") do (
    set "filename=%%~nf"
    goctl api plugin -plugin goctl-sw
