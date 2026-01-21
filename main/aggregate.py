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
            percentage = 1.0 
            weights = {}
            
            with open(file_path, 'r') as f:
                lines = f.readlines()
                
            # Parse custom text format
            i = 0
            while i < len(lines):
                line = lines[i].strip()
                if not line:
                    i += 1
                    continue
                if line.startswith("# WEIGHT_PERCENTAGE:"):
                    percentage = float(line.split(":")[1].strip())
                    i += 1
                    continue
                
                # Parse layer info
                parts = line.split()
                if len(parts) >= 2:
                    layer_name = parts[0]
                    shape = [int(s) for s in parts[1:]]
                    i += 1
                    if i < len(lines):
                        weight_values = [float(v) for v in lines[i].strip().split()]
                        weights[layer_name] = torch.tensor(weight_values).view(shape)
                i += 1
            
            # Extract client ID from filename
            client_id = txt_file.replace('.txt', '').replace('edge_', '').replace('client_', '')
            
            if weights:
                client_weights[client_id] = {
                    'weights': weights,
                    'factor': percentage
                }
                print(f"Loaded weights for client {client_id}: {len(weights)} layers (Weight Factor: {percentage})")
        except Exception as e:
            print(f"Error loading {txt_file}: {e}")
            continue
    return client_weights

def fedavg_aggregate(client_weights: Dict[str, Dict[str, Any]]) -> Dict[str, torch.Tensor]:
    """Perform Federated Averaging aggregation"""
    
    if not client_weights:
        raise ValueError("No client weights provided")
    
    client_ids = list(client_weights.keys())
    first_client_entry = client_weights[client_ids[0]]
    if 'weights' in first_client_entry:
        first_client_weights = first_client_entry['weights']
    else:
        # Fallback for legacy format or validation
        first_client_weights = first_client_entry
    
    print(f"Aggregating weights from {len(client_ids)} clients: {client_ids}")
    
    # Calculate total weight factor for normalization
    total_weight_factor = sum(client_weights[cid]['factor'] for cid in client_ids)
    if total_weight_factor == 0:
        print("Warning: Total weight factor is 0, falling back to equal weighting")
        total_weight_factor = len(client_ids)
        for cid in client_ids:
            client_weights[cid]['factor'] = 1.0
            
    print(f"Total weight factor (normalization constant): {total_weight_factor}")

    # Initialize global parameters with zeros
    global_weights = {}
    for param_name, param_tensor in first_client_weights.items():
        global_weights[param_name] = torch.zeros_like(param_tensor, dtype=torch.float32)
    
    # Aggregate parameters across all clients
    for client_id in client_ids:
        client_entry = client_weights[client_id]
        client_weight_dict = client_entry['weights']
        client_factor = client_entry['factor']
        
        # Normalized weight for this client
        normalized_weight = client_factor / total_weight_factor
        
        # Weighted sum of parameters
        for param_name in client_weight_dict:
            if param_name in global_weights:
                client_param = client_weight_dict[param_name].float()
                # Add weighted contribution: W_k * (n_k / N)
                global_weights[param_name] += client_param * normalized_weight
            else:
                print(f"Warning: Parameter {param_name} not found in global weights")
    
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
