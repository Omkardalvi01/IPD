@echo off
REM IPD Enhanced System - Windows Run Script

echo ========================================
echo IPD Enhanced Image Processing System
echo ========================================
echo.

REM Check if Go is installed
go version >nul 2>&1
if errorlevel 1 (
    echo ERROR: Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl/
    pause
    exit /b 1
)

REM Build the application if it doesn't exist
if not exist "main\ipd-enhanced.exe" (
    echo Building IPD Enhanced System...
    cd main
    go build -o ipd-enhanced.exe
    if errorlevel 1 (
        echo ERROR: Build failed
        pause
        exit /b 1
    )
    cd ..
    echo Build completed successfully!
    echo.
)

REM Create sample data if it doesn't exist
if not exist "sample_images" (
    echo Creating sample test data...
    mkdir sample_images
    echo Sample image data 1 > sample_images\image1.txt
    echo Sample image data 2 > sample_images\image2.txt
    echo Sample image data 3 > sample_images\image3.txt
    echo Sample image data 4 > sample_images\image4.txt
    echo Sample image data 5 > sample_images\image5.txt
    echo Sample test data created in sample_images\
    echo.
)

REM Run the application
echo Starting IPD Enhanced System...
echo.
echo Instructions:
echo 1. When prompted for number of workers, enter a number (e.g., 2)
echo 2. The system will attempt to connect to a signaling server
echo 3. Connection errors are expected without a running signaling server
echo 4. Check the output for system behavior and logs
echo.

cd main
ipd-enhanced.exe ..\sample_images
cd ..

echo.
echo ========================================
echo IPD Enhanced System Execution Complete
echo ========================================
echo.
echo Check the following directories for output:
echo - shared_results\ : Aggregated worker results
echo - logs\ : System and worker log files
echo.
pause