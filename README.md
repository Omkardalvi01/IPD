# IPD - Enhanced Image Processing Distributed System

A robust distributed image processing system with persistent worker connections, bidirectional communication, and comprehensive monitoring capabilities.

## 🚀 Features

- **Persistent Worker Connections**: Workers maintain connections throughout the entire processing lifecycle
- **Bidirectional Communication**: Full message passing between workers and main server
- **Result Aggregation**: Centralized collection and validation of all worker outputs
- **Health Monitoring**: Real-time connection and worker health tracking
- **Configuration Management**: Flexible, validated configuration system
- **Error Recovery**: Comprehensive error handling with retry mechanisms
- **Enhanced Logging**: Detailed progress reporting and status updates
- **Graceful Shutdown**: Proper resource cleanup and connection closure

## 📋 Prerequisites

### System Requirements
- **Go**: Version 1.19 or higher
- **Python**: Version 3.7 or higher (for processing scripts)
- **Operating System**: Windows, Linux, or macOS
- **Memory**: Minimum 4GB RAM (8GB recommended for multiple workers)
- **Network**: WebRTC-compatible network environment

### Check Prerequisites
```bash
# Check Go version
go version

# Check Python version
python --version
# or
python3 --version
```

## 🛠️ Installation and Setup

### Method 1: Automated Setup (Recommended)

#### For Windows:
```cmd
# Simply run the automated setup script
run.bat
```

#### For Linux/macOS:
```bash
# Make script executable and run
chmod +x run.sh
./run.sh
```

### Method 2: Manual Setup

#### Step 1: Clone and Navigate
```bash
git clone <repository-url>
cd IPD
```

#### Step 2: Build the Application
```bash
cd main
go mod tidy
go build -o ipd-enhanced
```

#### Step 3: Create Test Data
```bash
# Go back to IPD root directory
cd ..

# Create sample images directory
mkdir sample_images

# Add some test files
echo "Sample image data 1" > sample_images/image1.txt
echo "Sample image data 2" > sample_images/image2.txt
echo "Sample image data 3" > sample_images/image3.txt
echo "Sample image data 4" > sample_images/image4.txt
echo "Sample image data 5" > sample_images/image5.txt
```

#### Step 4: Run the System
```bash
cd main
./ipd-enhanced ../sample_images
```

## 🏃‍♂️ How to Run the System

### Quick Start (5 Minutes)

1. **Download and Build**
   ```bash
   cd IPD/main
   go build -o ipd-enhanced
   ```

2. **Create Test Data**
   ```bash
   mkdir ../test_images
   echo "test data" > ../test_images/test1.txt
   echo "test data" > ../test_images/test2.txt
   ```

3. **Run with Interactive Prompt**
   ```bash
   ./ipd-enhanced ../test_images
   ```
   
4. **When Prompted, Enter Number of Workers**
   ```
   Enter number of workers (recommended less than 4 for your device, max 4 from config)
   Workers: 2
   ```

### Non-Interactive Run
```bash
# Run with 2 workers automatically
echo "2" | ./ipd-enhanced ../test_images
```

### Custom Input Directory
```bash
# Process images from a specific directory
./ipd-enhanced /path/to/your/images
```

## 📊 What You'll See When Running

### Successful Startup
```
IPD Enhanced Worker System
Configuration loaded from: ./config.json
Max workers: 4, Heartbeat interval: 30s
Enter number of workers (recommended less than 4 for your device, max 4 from config)
Workers: 2
Connection_id: 1234
🔗 Setting up component integration...
🚀 Starting 2 persistent workers...
✅ All 2 persistent workers initialized
📂 Distributing 5 images to workers...
📡 Image distribution complete. Workers maintaining connections for processing...
```

### Real-time Monitoring
```
💓 Heartbeat status: 2/2 connections healthy
🔄 Worker states: PROCESSING:2
📤 Worker: 0 status: SUCCESS uploaded: 1/5
📤 Worker: 1 status: SUCCESS uploaded: 2/5
📤 Worker: 0 status: SUCCESS uploaded: 3/5
📤 Worker: 1 status: SUCCESS uploaded: 4/5
📤 Worker: 0 status: SUCCESS uploaded: 5/5
✓ Worker 0 completed result transmission (1/2)
✓ Worker 1 completed result transmission (2/2)
🎉 All workers completed! Results stored in: ./shared_results
```

### Final Statistics
```
📈 Final Processing Statistics:
  Total workers: 2
  Completed workers: 2
  Healthy workers: 2
  Workers with validation errors: 0
  Worker 0: COMPLETE (input:3 output:3)
  Worker 1: COMPLETE (input:2 output:2)
  Results directory: ./shared_results
✅ Shutdown complete
```

## ⚙️ Configuration

### Default Configuration
The system automatically creates a `config.json` file with default settings:

```json
{
  "system": {
    "max_workers": 4,
    "heartbeat_interval": "30s",
    "processing_timeout": "10m",
    "connection_timeout": "30s",
    "retry_attempts": 3,
    "external_script_path": "./scripts/process.py",
    "input_base_directory": "./data/input",
    "output_base_directory": "./data/output",
    "signaling_server_url": "ws://localhost:8000",
    "log_level": "INFO",
    "enable_metrics": false,
    "metrics_port": 9090
  }
}
```

### Environment Variables Override
```bash
# Override max workers
export IPD_MAX_WORKERS=8

# Override heartbeat interval
export IPD_HEARTBEAT_INTERVAL=15s

# Override processing timeout
export IPD_PROCESSING_TIMEOUT=20m

# Override log level
export IPD_LOG_LEVEL=DEBUG

# Run with overrides
./ipd-enhanced ../sample_images
```

### Custom Configuration File
```bash
# Use custom config file
export IPD_CONFIG_PATH=./my-config.json
./ipd-enhanced ../sample_images
```

## 📁 Output Structure

After running, you'll find:

```
IPD/
├── shared_results/              # Main output directory
│   ├── worker_0/               # Results from worker 0
│   │   ├── result_metadata.json
│   │   ├── processed_image1.txt
│   │   ├── processed_image2.txt
│   │   └── completion_time.txt
│   ├── worker_1/               # Results from worker 1
│   │   ├── result_metadata.json
│   │   ├── processed_image3.txt
│   │   └── completion_time.txt
│   └── collection_summary.json # Overall summary
├── logs/                       # Log files
│   ├── worker_0/
│   │   ├── worker.log
│   │   └── error.log
│   ├── worker_1/
│   │   └── worker.log
│   └── system.log
└── config.json                # Auto-generated config
```

## 🔧 Troubleshooting

### Common Issues and Solutions

#### 1. Connection Refused Error
```
Error: dial tcp [::1]:8000: connectex: No connection could be made
```
**This is Normal!** The system is trying to connect to a WebRTC signaling server. Without a running signaling server, you'll see connection errors, but the system will still demonstrate its enhanced features.

**Solution**: For full functionality, set up a WebRTC signaling server or use the system in offline mode.

#### 2. Build Errors
```
go: module not found
```
**Solution**:
```bash
cd main
go mod init github.com/Omkardalvi01/IPD
go mod tidy
go build -o ipd-enhanced
```

#### 3. Permission Denied (Linux/macOS)
```
permission denied: ./ipd-enhanced
```
**Solution**:
```bash
chmod +x ipd-enhanced
```

#### 4. Python Script Not Found
```
Error: external script not found
```
**Solution**:
```bash
# Make sure Python script is executable
chmod +x ../scripts/process.py

# Or set custom script path
export IPD_EXTERNAL_SCRIPT_PATH=/path/to/your/script.py
```

#### 5. Out of Memory
```
Worker 2 connection error: out of memory
```
**Solution**:
```bash
# Reduce number of workers
echo "1" | ./ipd-enhanced ../sample_images

# Or set lower max workers in config
export IPD_MAX_WORKERS=2
```

## 🧪 Testing the System

### Basic Functionality Tests
```bash
# Create test data
mkdir test_data
echo "test content 1" > test_data/file1.txt
echo "test content 2" > test_data/file2.txt

# Run with 1 worker
echo "1" | ./ipd-enhanced ../test_data

# Check results
ls -la ../shared_results/
```

### Performance Test
```bash
# Create larger dataset
mkdir large_test
for i in {1..20}; do
    echo "Large test file $i content" > large_test/file$i.txt
done

# Run with multiple workers
echo "4" | ./ipd-enhanced ../large_test
```

### Configuration Test
```bash
# Test with custom settings
export IPD_MAX_WORKERS=2
export IPD_LOG_LEVEL=DEBUG
export IPD_HEARTBEAT_INTERVAL=10s

echo "2" | ./ipd-enhanced ../sample_images
```

## 📈 Performance Optimization

### Optimal Worker Count
- **For your system**: Start with number of CPU cores
- **For testing**: Use 2-4 workers
- **For production**: Tune based on memory and network capacity

### Memory Optimization
```bash
# For low memory systems
export IPD_MAX_WORKERS=2
export IPD_CHUNK_SIZE=32768

# For high memory systems
export IPD_MAX_WORKERS=8
export IPD_CHUNK_SIZE=131072
```

### Network Optimization
```bash
# For slow networks
export IPD_CONNECTION_TIMEOUT=120s
export IPD_HEARTBEAT_INTERVAL=60s

# For fast networks
export IPD_CONNECTION_TIMEOUT=15s
export IPD_HEARTBEAT_INTERVAL=10s
```

## 🚀 Advanced Usage

### Batch Processing
```bash
# Process multiple directories
for dir in ./batch1 ./batch2 ./batch3; do
    echo "Processing $dir..."
    echo "4" | ./ipd-enhanced "$dir"
done
```

### Automated Processing
```bash
#!/bin/bash
# automated_processing.sh

INPUT_DIR="$1"
WORKERS="$2"

if [ -z "$INPUT_DIR" ] || [ -z "$WORKERS" ]; then
    echo "Usage: $0 <input_directory> <num_workers>"
    exit 1
fi

echo "Starting automated processing..."
echo "Input: $INPUT_DIR"
echo "Workers: $WORKERS"

cd main
echo "$WORKERS" | ./ipd-enhanced "$INPUT_DIR"

echo "Processing complete. Results in: ./shared_results"
```

### Custom Processing Script
```python
# custom_processor.py
import sys
import os

def process_file(input_file, output_file):
    # Your custom processing logic here
    with open(input_file, 'r') as f:
        content = f.read()
    
    # Process content
    processed = content.upper() + " [PROCESSED]"
    
    with open(output_file, 'w') as f:
        f.write(processed)

# Use with:
# export IPD_EXTERNAL_SCRIPT_PATH=./custom_processor.py
```

## 🔍 Monitoring and Debugging

### Enable Debug Logging
```bash
export IPD_LOG_LEVEL=DEBUG
./ipd-enhanced ../sample_images
```

### Monitor Log Files
```bash
# Watch system logs in real-time
tail -f logs/system.log

# Watch worker logs
tail -f logs/worker_0/worker.log

# Search for errors
grep -i error logs/*.log
```

### Enable Metrics
```bash
export IPD_ENABLE_METRICS=true
export IPD_METRICS_PORT=9090
./ipd-enhanced ../sample_images

# In another terminal, check metrics
curl http://localhost:9090/metrics
```

## 📚 Additional Resources

- **QUICKSTART.md** - 5-minute setup guide
- **DEPLOYMENT.md** - Production deployment guide
- **config-example.json** - Complete configuration template
- **scripts/process.py** - Sample processing script

## 🤝 Support

If you encounter issues:

1. **Check the troubleshooting section above**
2. **Enable debug logging**: `export IPD_LOG_LEVEL=DEBUG`
3. **Check log files** in the `logs/` directory
4. **Verify prerequisites** (Go, Python versions)
5. **Try with fewer workers** if you see memory issues

## 🎯 Quick Commands Reference

```bash
# Build
cd main && go build -o ipd-enhanced

# Run with 2 workers
echo "2" | ./ipd-enhanced ../sample_images

# Run with debug logging
IPD_LOG_LEVEL=DEBUG echo "2" | ./ipd-enhanced ../sample_images

# Run with custom config
IPD_CONFIG_PATH=./my-config.json ./ipd-enhanced ../sample_images

# Check results
ls -la ../shared_results/

# View logs
tail -f ../logs/system.log
```

---

**Ready to get started?** Run the automated setup script (`run.bat` on Windows or `./run.sh` on Linux/macOS) or follow the manual setup steps above! 🚀
