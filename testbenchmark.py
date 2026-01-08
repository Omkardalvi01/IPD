import torch
import torch.nn as nn
import time
import psutil
import json
import socket
from typing import Dict, Any

class EdgeTrainingBenchmark:
    def __init__(self, sample_input_shape=(32, 3, 224, 224), aggregator_ip=None):
        self.device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
        self.input_shape = sample_input_shape
        self.aggregator_ip = aggregator_ip
        
    def _get_flatten_size(self, model_features):
        """Dynamically calculate flattened size to avoid shape errors"""
        dummy = torch.zeros(1, *self.input_shape[1:])
        with torch.no_grad():
            out = model_features(dummy)
        return out.numel()
    
    def benchmark_training_step(self, duration=3.0):
        """
        Measures training speed (forward + backward + optimizer step)
        Returns: training images per second
        """
        print(f"  Running TRAINING benchmark for {duration}s...")
        
        # Build model with correct dimensions
        conv_block = nn.Sequential(
            nn.Conv2d(3, 64, 3, padding=1), nn.BatchNorm2d(64), nn.ReLU(),
            nn.MaxPool2d(2),
            nn.Conv2d(64, 128, 3, padding=1), nn.BatchNorm2d(128), nn.ReLU(),
            nn.MaxPool2d(2),
            nn.Flatten()
        )
        
        flatten_size = self._get_flatten_size(conv_block)
        
        model = nn.Sequential(
            conv_block,
            nn.Linear(flatten_size, 256),
            nn.ReLU(),
            nn.Dropout(0.5),
            nn.Linear(256, 10)
        ).to(self.device)
        
        optimizer = torch.optim.SGD(model.parameters(), lr=0.01, momentum=0.9)
        criterion = nn.CrossEntropyLoss()
        
        # Prepare data
        data = torch.randn(*self.input_shape, device=self.device)
        target = torch.randint(0, 10, (self.input_shape[0],), device=self.device)
        
        # Warmup (important for GPU)
        model.train()
        for _ in range(5):
            optimizer.zero_grad()
            output = model(data)
            loss = criterion(output, target)
            loss.backward()
            optimizer.step()
            
        if torch.cuda.is_available(): 
            torch.cuda.synchronize()
        
        # Actual benchmark
        start = time.time()
        steps = 0
        
        while (time.time() - start) < duration:
            optimizer.zero_grad()
            output = model(data)
            loss = criterion(output, target)
            loss.backward()
            optimizer.step()
            steps += 1
            
        if torch.cuda.is_available(): 
            torch.cuda.synchronize()
        
        elapsed = time.time() - start
        training_ips = (steps * self.input_shape[0]) / elapsed
        
        # Cleanup
        del model, optimizer, data, target
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
        
        return training_ips
    
    def benchmark_network_latency(self):
        """
        Measure round-trip time to aggregator server
        Returns: latency in milliseconds
        """
        if not self.aggregator_ip:
            return 0.0  # No penalty if aggregator not specified
            
        print(f"  Measuring network latency to {self.aggregator_ip}...")
        
        try:
            # Simple TCP connection test
            import socket
            latencies = []
            
            for _ in range(5):
                start = time.time()
                sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                sock.settimeout(2)
                
                # Try to connect to aggregator (you'll need a listening port)
                result = sock.connect_ex((self.aggregator_ip, 8080))
                
                if result == 0:
                    elapsed = (time.time() - start) * 1000  # Convert to ms
                    latencies.append(elapsed)
                
                sock.close()
                time.sleep(0.1)
            
            return sum(latencies) / len(latencies) if latencies else 999.0
            
        except Exception as e:
            print(f"    Warning: Network test failed ({e}), using high latency")
            return 999.0  # High penalty if unreachable
    
    def get_system_health(self):
        """
        Returns health factor (0.1 to 1.0) based on system load
        """
        cpu_pct = psutil.cpu_percent(interval=0.5) / 100.0
        ram_pct = psutil.virtual_memory().percent / 100.0
        
        # Critical thresholds
        if ram_pct > 0.95:
            return 0.1  # System is thrashing
        if cpu_pct > 0.95:
            return 0.3  # CPU maxed out
        
        # Smooth penalty curve
        load_factor = max(cpu_pct, ram_pct)
        health = 1.0 - (load_factor * 0.7)  # Max 70% penalty
        
        return max(health, 0.1)  # Never go below 0.1
    
    def get_device_info(self) -> Dict[str, Any]:
        """Collect device metadata"""
        info = {
            'device_type': 'GPU' if torch.cuda.is_available() else 'CPU',
            'cpu_count': psutil.cpu_count(logical=False),
            'ram_gb': round(psutil.virtual_memory().total / (1024**3), 2),
        }
        
        if torch.cuda.is_available():
            info['gpu_name'] = torch.cuda.get_device_name(0)
            info['gpu_memory_gb'] = round(
                torch.cuda.get_device_properties(0).total_memory / (1024**3), 2
            )
        
        return info
    
    def get_full_report(self) -> Dict[str, Any]:
        """
        Complete benchmark for edge registration
        Returns comprehensive metrics for workload distribution
        """
        print("\n" + "="*60)
        print("EDGE DEVICE BENCHMARK - Federated Learning Profile")
        print("="*60 + "\n")
        
        # Run all benchmarks
        training_speed = self.benchmark_training_step(duration=3.0)
        health_factor = self.get_system_health()
        network_latency = self.benchmark_network_latency()
        device_info = self.get_device_info()
        
        # Calculate effective score
        # Score represents: "How many training images/sec considering current health"
        effective_score = training_speed * health_factor
        
        report = {
            'score': round(effective_score, 2),
            'raw_training_ips': round(training_speed, 2),
            'health_factor': round(health_factor, 3),
            'network_latency_ms': round(network_latency, 2),
            'device_info': device_info,
            'timestamp': time.time()
        }
        
        # Display summary
        print("\n" + "-"*60)
        print(f"EFFECTIVE TRAINING SCORE: {effective_score:.2f} img/s")
        print(f"  Raw Speed:      {training_speed:.2f} img/s")
        print(f"  Health Factor:  {health_factor:.2%}")
        print(f"  Network Latency: {network_latency:.1f} ms")
        print(f"  Device Type:    {device_info['device_type']}")
        print("-"*60 + "\n")
        
        return report
    
    def get_quick_score(self):
        """
        Fast re-check for periodic updates (< 2 seconds)
        Only checks system health, uses cached training speed estimate
        """
        health = self.get_system_health()
        # You'd cache the training_speed from initial benchmark
        # For now, return just health as adjustment factor
        return health


# Usage for edge device
if __name__ == "__main__":
    # Each edge runs this and reports to aggregator
    benchmark = EdgeTrainingBenchmark(
        sample_input_shape=(32, 3, 224, 224),
        aggregator_ip="192.168.1.100"  # Your aggregator server
    )
    
    report = benchmark.get_full_report()
    
    # Save locally
    with open("edge_benchmark.json", "w") as f:
        json.dump(report, f, indent=2)
    
    # Send to aggregator (you'll implement this)
    # send_to_aggregator(report)