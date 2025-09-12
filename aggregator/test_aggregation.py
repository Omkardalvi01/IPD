import os
import sys
import json
import time
import shutil
import requests
import torch
from pathlib import Path
from typing import List, Dict, Any
from generate_mobilenetv2_weights import generate_mobilenetv2_sample

# Configuration
API_URL = "http://localhost:5000"
OUTPUT_DIR = "output"
GLOBAL_MODEL_DIR = "global_models"
NUM_CLIENTS = 3

class TestAggregation:
    def __init__(self):
        self.test_dir = Path(__file__).parent
        self.output_dir = self.test_dir / OUTPUT_DIR
        self.global_model_dir = self.test_dir / GLOBAL_MODEL_DIR
        self.client_ids = [f"client_{i+1}" for i in range(NUM_CLIENTS)]
        self.setup_directories()

    def setup_directories(self):
        """Create necessary directories and clean up old test data"""
        self.output_dir.mkdir(exist_ok=True)
        self.global_model_dir.mkdir(exist_ok=True)
        
        # Clean up old test files
        for file in self.output_dir.glob("*.txt"):
            file.unlink()

    def generate_test_weights(self):
        """Generate sample weight files for testing"""
        print("\n1. Generating sample weight files...")
        for client_id in self.client_ids:
            generate_mobilenetv2_sample(str(self.output_dir), client_id)
            print(f"  ✓ Generated weights for {client_id}")
        
        # Verify files were created
        weight_files = list(self.output_dir.glob("*.txt"))
        if len(weight_files) != NUM_CLIENTS:
            raise RuntimeError(f"Expected {NUM_CLIENTS} weight files, found {len(weight_files)}")

    def check_api_server(self) -> bool:
        """Check if API server is running"""
        print("\n2. Checking API server status...")
        max_retries = 5
        for i in range(max_retries):
            try:
                response = requests.get(f"{API_URL}/available_clients", timeout=5)
                if response.status_code == 200:
                    print("  ✓ API server is running")
                    return True
            except (requests.exceptions.ConnectionError, requests.exceptions.Timeout):
                if i < max_retries - 1:
                    print(f"  - Waiting for API server to start... (Attempt {i+1}/{max_retries})")
                    time.sleep(2)
                else:
                    print("  ✗ API server not responding")
                    return False
        return False

    def send_aggregation_request(self) -> Dict[str, Any]:
        """Send UIDs to API for aggregation"""
        print("\n3. Sending UIDs to API for aggregation...")
        try:
            response = requests.post(
                f"{API_URL}/receive_uids",
                json={"uids": self.client_ids},
                timeout=30
            )
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            print(f"  ✗ Error sending request to API: {e}")
            return {"status": "error", "message": str(e)}

    def verify_global_model(self) -> bool:
        """Verify that the global model was created and is valid"""
        print("\n4. Verifying global model...")
        model_files = list(self.global_model_dir.glob("**/*.pth"))
        
        if not model_files:
            print("  ✗ No global model files found")
            return False
            
        latest_model = max(model_files, key=os.path.getmtime)
        print(f"  ✓ Found global model: {latest_model}")
        
        # Load and verify the model
        try:
            model_weights = torch.load(latest_model, map_location='cpu')
            if not isinstance(model_weights, dict):
                print("  ✗ Invalid model format: expected dictionary")
                return False
                
            print(f"  ✓ Model contains {len(model_weights)} parameters")
            for name, tensor in list(model_weights.items())[:3]:  # Print first 3 params as sample
                print(f"     - {name}: {tuple(tensor.shape)}")
            if len(model_weights) > 3:
                print(f"     - ... and {len(model_weights) - 3} more parameters")
                
            return True
            
        except Exception as e:
            print(f"  ✗ Error loading model: {e}")
            return False

def main():
    print("=== Federated Learning Aggregator Test ===")
    print(f"Testing with {NUM_CLIENTS} clients")
    print(f"API URL: {API_URL}")
    
    test = TestAggregation()
    
    try:
        # 1. Generate test weights
        test.generate_test_weights()
        
        # 2. Check API server
        if not test.check_api_server():
            print("\n❌ Test failed: Could not connect to API server")
            print("Make sure the API server is running with:")
            print("  python -m IPD.aggregator.api_server")
            return 1
        
        # 3. Send aggregation request
        response = test.send_aggregation_request()
        
        print("\nAggregation Response:")
        print(json.dumps(response, indent=2))
        
        if response.get('status') != 'success':
            print("\n❌ Test failed: Aggregation was not successful")
            return 1
        
        # 4. Verify global model
        if not test.verify_global_model():
            print("\n❌ Test failed: Global model verification failed")
            return 1
        
        print("\n✅ Test completed successfully!")
        return 0
        
    except Exception as e:
        print(f"\n❌ Test failed with error: {e}")
        import traceback
        traceback.print_exc()
        return 1
    
if __name__ == "__main__":
    sys.exit(main())
