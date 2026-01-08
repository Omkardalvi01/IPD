import torch
import torch.nn as nn
import time
import psutil
import json
import socket
import os
import math
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
    
    def get_power_status(self):
        """
        Check if device is plugged in.
        Returns: 1.0 if plugged in, 0.5 if on battery (Power Saving penalty)
        """
        try:
            battery = psutil.sensors_battery()
            if battery is None: return 1.0 # Desktop/Server usually returns None
            return 1.0 if battery.power_plugged else 0.5
        except:
            return 1.0

    def benchmark_training_stability(self, duration=5.0):
        """
        Measures training speed AND stability (throttling detection).
        Returns: (images_per_second, stability_score)
        """
        print(f"  Running SMART TRAINING benchmark for {duration}s...")
        
        # --- Model Setup ---
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
        
        data = torch.randn(*self.input_shape, device=self.device)
        target = torch.randint(0, 10, (self.input_shape[0],), device=self.device)
        
        # --- Warmup ---
        model.train()
        for _ in range(5):
            optimizer.zero_grad()
            loss = criterion(model(data), target)
            loss.backward()
            optimizer.step()
            
        if torch.cuda.is_available(): 
            torch.cuda.synchronize()
        
        # --- Benchmark Loop with Phase Timing ---
        batch_times = []
        start_global = time.time()
        
        while (time.time() - start_global) < duration:
            t0 = time.time()
            
            optimizer.zero_grad()
            output = model(data)
            loss = criterion(output, target)
            loss.backward()
            optimizer.step()
            
            if torch.cuda.is_available(): 
                torch.cuda.synchronize()
            
            batch_times.append(time.time() - t0)
            
        # --- Analysis ---
        total_images = len(batch_times) * self.input_shape[0]
        total_time = sum(batch_times)
        avg_ips = total_images / total_time
        
        # Stability Check: Compare first 25% speed vs last 25% speed
        # If the device throttles, the last 25% will be much slower (higher time)
        quarter = len(batch_times) // 4
        if quarter > 0:
            start_avg_time = sum(batch_times[:quarter]) / quarter
            end_avg_time = sum(batch_times[-quarter:]) / quarter
            
            # stability = start_time / end_time. 
            # If end_time is higher (slower), stability < 1.0
            stability = start_avg_time / end_avg_time 
        else:
            stability = 1.0
            
        # Cap stability at 1.0 (we don't care if it gets faster/warms up)
        stability = min(stability, 1.0)
        
        # Cleanup
        del model, optimizer, data, target
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
        
        return avg_ips, stability
    
    def benchmark_network_latency(self):
        """
        Measure round-trip time to aggregator server
        Returns: latency in milliseconds
        """
        if not self.aggregator_ip:
            return 0.0   
            
        print(f"  Measuring network latency to {self.aggregator_ip}...")
        
        try:
            import socket
            latencies = []
            
            # Reduced to 3 attempts to save time during benchmarks
            for _ in range(3):
                start = time.time()
                sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                sock.settimeout(2)
                
                # Assuming aggregator listens on 8080 or similar control port
                result = sock.connect_ex((self.aggregator_ip, 8080))
                
                if result == 0:
                    elapsed = (time.time() - start) * 1000 
                    latencies.append(elapsed)
                
                sock.close()
                time.sleep(0.1)
            
            return sum(latencies) / len(latencies) if latencies else 999.0
            
        except Exception as e:
            print(f"    Warning: Network test failed ({e}), using high latency")
            return 999.0 
    
    def get_system_health(self):
        """
        Returns health factor (0.1 to 1.0) based on system load
        """
        cpu_pct = psutil.cpu_percent(interval=0.5) / 100.0
        ram_pct = psutil.virtual_memory().percent / 100.0
        
        if ram_pct > 0.95:
            return 0.1  
        if cpu_pct > 0.95:
            return 0.3 
        
        load_factor = max(cpu_pct, ram_pct)
        health = 1.0 - (load_factor * 0.7) 
        
        return max(health, 0.1) 
    
    def get_device_info(self) -> Dict[str, Any]:
        """Collect device metadata"""
        info = {
            'device_type': 'GPU' if torch.cuda.is_available() else 'CPU',
            'cpu_count': psutil.cpu_count(logical=False),
            'ram_gb': round(psutil.virtual_memory().total / (1024**3), 2),
            'power_plugged': self.get_power_status() == 1.0
        }
        
        if torch.cuda.is_available():
            info['gpu_name'] = torch.cuda.get_device_name(0)
            info['gpu_memory_gb'] = round(
                torch.cuda.get_device_properties(0).total_memory / (1024**3), 2
            )
        
        return info
    
    def get_full_report(self) -> Dict[str, Any]:
        """
        Complete benchmark for edge registration with Smart Scoring
        """
        print("\n" + "="*60)
        print("EDGE DEVICE BENCHMARK - Research Profile (Smart)")
        print("="*60 + "\n")
        
        # 1. Run Benchmarks
        raw_ips, stability = self.benchmark_training_stability(duration=5.0)
        health_factor = self.get_system_health()
        latency_ms = self.benchmark_network_latency()
        power_factor = self.get_power_status()
        device_info = self.get_device_info()
        
        # 2. Advanced Scoring Formula
        # Start with raw speed
        score = raw_ips 
        # Penalize for system load
        score *= health_factor 
        # Penalize for thermal throttling (stability)
        score *= stability
        # Penalize for battery usage
        score *= power_factor
        
        # Network Penalty (Logarithmic decay)
        # Prevents assigning heavy tasks to high-latency nodes
        if self.aggregator_ip and latency_ms > 0:
            # Multiplier starts at 1.0 and drops as latency grows
            # e.g., 20ms -> ~0.9, 200ms -> ~0.5
            net_penalty = 1.0 / (1.0 + (latency_ms / 200.0))
            score *= net_penalty
        
        report = {
            'final_score': round(score, 2),
            'metrics': {
                'raw_training_ips': round(raw_ips, 2),
                'stability_index': round(stability, 3),
                'health_factor': round(health_factor, 2),
                'power_factor': power_factor,
                'network_latency_ms': round(latency_ms, 2)
            },
            'device_info': device_info,
            'timestamp': time.time()
        }
        
        print("\n" + "-"*60)
        print(f"FINAL SMART SCORE: {score:.2f}")
        print(f"  Raw Speed:       {raw_ips:.2f} img/s")
        print(f"  Stability:       {stability:.2f} (1.0=Stable)")
        print(f"  Power Mode:      {'Plugged In' if power_factor==1.0 else 'Battery (0.5x Penalty)'}")
        print(f"  Network Latency: {latency_ms:.1f} ms")
        print(f"  Device Type:     {device_info['device_type']}")
        print("-"*60 + "\n")
        
        return report
    
    def get_quick_score(self):
        """
        Fast re-check for periodic updates
        """
        health = self.get_system_health()
        return health


if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description='Edge Device Benchmark')
    parser.add_argument('--score-only', action='store_true', help='Output only the effective score')
    parser.add_argument('--aggregator-ip', type=str, default="192.168.1.100", help='Aggregator IP address')
    
    args = parser.parse_args()
    
    benchmark = EdgeTrainingBenchmark(
        sample_input_shape=(32, 3, 224, 224),
        aggregator_ip=args.aggregator_ip
    )
    
    if args.score_only:
        import sys
        original_stdout = sys.stdout
        sys.stdout = open(os.devnull, 'w')
        
        try:
            report = benchmark.get_full_report()
        finally:
            sys.stdout.close()
            sys.stdout = original_stdout
            
        print(report['final_score'])
    else:
        report = benchmark.get_full_report()
        
        with open("edge_benchmark.json", "w") as f:
            json.dump(report, f, indent=2)