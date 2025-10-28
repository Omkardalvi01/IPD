#!/usr/bin/env python3
"""
Simple Federated Learning Aggregator
Parses JSON files from client_payloads directory and performs FedAvg aggregation
"""

import os
import json
import torch
import numpy as np
from typing import Dict, List, Any
import argparse
from datetime import datetime

def load_client_payloads(client_payloads_dir: str) -> Dict[str, Dict[str, Any]]:
    """Load all client weight files from the output directory"""
    client_weights = {}
    
    if not os.path.exists(client_payloads_dir):
        print(f"Error: Directory {client_payloads_dir} does not exist")
        return {}
    
    txt_files = [f for f in os.listdir(client_payloads_dir) if f.endswith('.txt')]
    
    if not txt_files:
        print(f"No TXT files found in {client_payloads_dir}")
        return {}
    
    print(f"Found {len(txt_files)} TXT files: {txt_files}")
    
    for txt_file in txt_files:
        file_path = os.path.join(client_payloads_dir, txt_file)
        try:
            with open(file_path, 'r') as f:
                # Assume the txt file contains a JSON-like dictionary as text
                data = json.loads(f.read())
            # Extract client ID from filename
            client_id = txt_file.replace('.txt', '').replace('edge_', '').replace('client_', '')
            # Skip metadata or chunk files
            if 'metadata' in txt_file:
                print(f"Skipping metadata file: {txt_file}")
                continue
            if 'chunk_' in txt_file:
                print(f"Skipping chunk file: {txt_file}")
                continue
            # The file should directly contain the weights dict
            weights = {}
            for layer_name, layer_weights in data.items():
                weights[layer_name] = torch.tensor(layer_weights, dtype=torch.float32)
            client_weights[client_id] = weights
            print(f"Loaded weights for client {client_id}: {len(weights)} layers")
        except Exception as e:
            print(f"Error loading {txt_file}: {e}")
            continue
    return client_weights

def fedavg_aggregate(client_weights: Dict[str, Dict[str, torch.Tensor]]) -> Dict[str, torch.Tensor]:
    """Perform Federated Averaging aggregation"""
    
    if not client_weights:
        raise ValueError("No client weights provided")
    
    client_ids = list(client_weights.keys())
    first_client_weights = client_weights[client_ids[0]]
    
    print(f"Aggregating weights from {len(client_ids)} clients: {client_ids}")
    
    # Initialize global parameters with zeros
    global_weights = {}
    for param_name, param_tensor in first_client_weights.items():
        global_weights[param_name] = torch.zeros_like(param_tensor, dtype=torch.float32)
    
    # Aggregate parameters across all clients
    for client_id in client_ids:
        client_weight_dict = client_weights[client_id]
        
        # Weighted sum of parameters
        for param_name in client_weight_dict:
            if param_name in global_weights:
                client_param = client_weight_dict[param_name].float()
                global_param = global_weights[param_name].float()
                global_weights[param_name] = global_param + client_param
            else:
                print(f"Warning: Parameter {param_name} not found in global weights")
    
    # Average the weights
    num_clients = len(client_ids)
    for param_name in global_weights:
        global_weights[param_name] = global_weights[param_name] / num_clients
    
    return global_weights

def save_global_model(global_weights: Dict[str, torch.Tensor], output_dir: str) -> Dict[str, Any]:
    """Save the aggregated global model weights and return metadata"""
    
    os.makedirs(output_dir, exist_ok=True)
    
    # Save as PyTorch state dict
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    model_path = os.path.join(output_dir, f"global_model_{timestamp}.pth")
    
    # Save the global weights in a format compatible with PyTorch models
    torch.save(global_weights, model_path)
    print(f"Global model saved to: {model_path}")
    
    # Save as JSON for inspection and API response
    json_path = os.path.join(output_dir, f"global_model_{timestamp}.json")
    
    # Convert tensors to lists for JSON serialization
    json_weights = {}
    for param_name, param_tensor in global_weights.items():
        json_weights[param_name] = {
            "shape": list(param_tensor.shape),
            "dtype": str(param_tensor.dtype),
            "mean": float(param_tensor.mean().item()),
            "std": float(param_tensor.std().item()),
            "values": param_tensor.tolist()  # Include values for compatibility
        }
    
    with open(json_path, 'w') as f:
        json.dump(json_weights, f, indent=2)
    
    print(f"Global model metadata saved to: {json_path}")
    
    return {
        "model_path": model_path,
        "json_path": json_path,
        "timestamp": timestamp,
        "metadata": json_weights
    }

def main():
    parser = argparse.ArgumentParser(description='Federated Learning Aggregator')
    parser.add_argument('--client_payloads_dir', type=str, default='aggregator/client_payloads',
                       help='Directory containing client JSON files')
    parser.add_argument('--output_dir', type=str, default='aggregator/global_models',
                       help='Directory to save aggregated model')
    parser.add_argument('--min_clients', type=int, default=1,
                       help='Minimum number of clients required for aggregation')
    
    args = parser.parse_args()
    
    print("=== Federated Learning Aggregator ===")
    print(f"Client payloads directory: {args.client_payloads_dir}")
    print(f"Output directory: {args.output_dir}")
    print(f"Minimum clients required: {args.min_clients}")
    
    # Load client payloads
    client_weights = load_client_payloads(args.client_payloads_dir)
    
    if len(client_weights) < args.min_clients:
        print(f"Error: Only {len(client_weights)} clients found, need at least {args.min_clients}")
        return 1
    
    if not client_weights:
        print("Error: No valid client weights found")
        return 1
    
    # Perform aggregation
    try:
        global_weights = fedavg_aggregate(client_weights)
        print(f"Successfully aggregated weights from {len(client_weights)} clients")
        
        # Save global model
        save_global_model(global_weights, args.output_dir)
        
        print("Aggregation completed successfully!")
        return 0
        
    except Exception as e:
        print(f"Error during aggregation: {e}")
        return 1

if __name__ == "__main__":
    exit(main())
