@echo off
REM IPD Worker Node Startup Script for Windows
REM Usage: start-worker-node.bat [worker-id] [coordinator-url] [signaling-url]

setlocal enabledelayedexpansion

REM Configuration
set WORKER_ID=%1
if "%WORKER_ID%"=="" set WORKER_ID=worker-%COMPUTERNAME%-%RANDOM%

set COORDINATOR_URL=%2
if "%COORDINATOR_URL%"=="" set COORDINATOR_URL=ws://localhost:8000

set SIGNALING_URL=%3
if "%SIGNALING_URL%"=="" set SIGNALING_URL=ws://localhost:8000

echo ==========================================
echo IPD Enhanced Worker Node
echo ==========================================
echo Worker ID: %WORKER_ID%
echo Coordinator: %COORDINATOR_URL%
echo Signaling Server: %SIGNALING_URL%
echo ==========================================

REM Check prerequisites
go version >nul 2>&1
if errorlevel 1 (
    echo ❌ Error: Go is not installed
    pause
    exit /b 1
)

python --version >nul 2>&1
if errorlevel 1 (
    python3 --version >nul 2>&1
    if errorlevel 1 (
        echo ❌ Error: Python is not installed
        pause
        exit /b 1
    )
)

REM Build worker application if needed
if not exist "main\ipd-worker-node.exe" (
    echo 🔨 Building worker node application...
    cd main
    go build -o ipd-worker-node.exe worker-node.go worker.go config.go result_collector.go file_transmitter.go script_executor.go error_handling.go retry_logic.go graceful_degradation.go enhanced_logging.go error_recovery_integration.go task.go
    if errorlevel 1 (
        echo ❌ Build failed
        pause
        exit /b 1
    )
    cd ..
    echo ✅ Build completed
)

REM Create worker directories
if not exist "worker_input" mkdir worker_input
if not exist "worker_output" mkdir worker_output
if not exist "logs\worker" mkdir logs\worker

REM Set environment variables
set IPD_MODE=worker-node
set IPD_WORKER_ID=%WORKER_ID%
set IPD_COORDINATOR_URL=%COORDINATOR_URL%
set IPD_SIGNALING_SERVER_URL=%SIGNALING_URL%
set IPD_LOG_LEVEL=INFO
set IPD_INPUT_BASE_DIRECTORY=./worker_input
set IPD_OUTPUT_BASE_DIRECTORY=./worker_output

REM Create worker configuration
echo { > worker-config.json
echo   "system": { >> worker-config.json
echo     "heartbeat_interval": "30s", >> worker-config.json
echo     "processing_timeout": "10m", >> worker-config.json
echo     "connection_timeout": "60s", >> worker-config.json
echo     "retry_attempts": 3, >> worker-config.json
echo     "external_script_path": "./scripts/process.py", >> worker-config.json
echo     "input_base_directory": "./worker_input", >> worker-config.json
echo     "output_base_directory": "./worker_output", >> worker-config.json
echo     "log_level": "INFO" >> worker-config.json
echo   }, >> worker-config.json
echo   "network": { >> worker-config.json
echo     "stun_servers": [ >> worker-config.json
echo       "stun:stun.l.google.com:19302", >> worker-config.json
echo       "stun:stun1.l.google.com:19302" >> worker-config.json
echo     ], >> worker-config.json
echo     "data_channel_timeout": "60s", >> worker-config.json
echo     "max_reconnect_attempts": 10, >> worker-config.json
echo     "reconnect_delay": "5s" >> worker-config.json
echo   } >> worker-config.json
echo } >> worker-config.json

set IPD_CONFIG_PATH=./worker-config.json

echo 🚀 Starting worker node...
echo 📝 Logs will be written to: logs\worker\
echo 📁 Input directory: worker_input\
echo 📁 Output directory: worker_output\
echo.
echo Press Ctrl+C to stop the worker node
echo.

REM Start worker node
cd main
ipd-worker-node.exe

echo.
echo 🛑 Worker node stopped
pause