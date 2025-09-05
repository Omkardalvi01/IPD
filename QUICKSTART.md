# IPD Quick Start Guide

Get up and running with the Enhanced IPD system in 5 minutes!

## 🚀 Quick Setup

### 1. Build the Application
```bash
cd IPD/main
go build -o ipd-enhanced
```

### 2. Create Test Data
```bash
mkdir -p test_images
echo "Sample image data" > test_images/image1.txt
echo "Sample image data" > test_images/image2.txt
echo "Sample image data" > test_images/image3.txt
```

### 3. Run with Default Settings
```bash
echo "2" | ./ipd-enhanced test_images
```

## 📋 What You'll See

```
IPD Enhanced Worker System
Configuration loaded from: ./config.json
Max workers: 4, Heartbeat interval: 30s
Enter number of workers (recommended less than 4 for your device, max 4 from config)
Workers: 🚀 Starting 2 persistent workers...
Connection_id: 1234
🔗 Setting up component integration...
✅ All 2 persistent workers initialized
📂 Distributing 3 images to workers...
📡 Image distribution complete. Workers maintaining connections for processing...
⏳ Waiting for all workers to complete processing and result transmission...
```

## ⚠️ Expected Behavior

Since there's no signaling server running, you'll see connection errors:
```
Error with peer connection in worker 0: dial tcp [::1]:8000: connectex: No connection could be made
```

This is normal! The system is trying to connect to a WebRTC signaling server.

## 🎯 Next Steps

1. **Set up a signaling server** for full functionality
2. **Customize configuration** in `config.json`
3. **Add real image processing scripts**
4. **Scale up with more workers**

## 🔧 Quick Configuration

Create a minimal `config.json`:
```json
{
  "system": {
    "max_workers": 4,
    "heartbeat_interval": "30s",
    "processing_timeout": "5m"
  }
}
```

## 📁 Output Structure

After running, check:
- `./shared_results/` - Aggregated results
- `./logs/` - System and worker logs
- `./config.json` - Auto-generated default config

That's it! You're ready to explore the enhanced IPD system! 🎉