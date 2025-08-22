#!/usr/bin/env python3
"""
Example training script for worker edge nodes.
This script receives a batch directory and outputs trained weights.
"""

import argparse
import os
import sys
import time
import json
import numpy as np
from pathlib import Path

def main():
    parser = argparse.ArgumentParser(description='Train ML model on image batch')
    parser.add_argument('--batch-dir', required=True, help='Directory containing image batch')
    parser.add_argument('--weights-output', required=True, help='Path to output weights file')
    
    args = parser.parse_args()
    
    print(f"Starting training on batch: {args.batch_dir}")
    print(f"Weights will be saved to: {args.weights_output}")
    
    # Validate batch directory
    if not os.path.exists(args.batch_dir):
        print(f"Error: Batch directory does not exist: {args.batch_dir}")
        sys.exit(1)
    
    # Count images in batch
    image_files = []
    for ext in ['*.jpg', '*.jpeg', '*.png', '*.bmp']:
        image_files.extend(Path(args.batch_dir).glob(ext))
    
    if not image_files:
        print(f"Error: No image files found in batch directory: {args.batch_dir}")
        sys.exit(1)
    
    print(f"Found {len(image_files)} images in batch")
    
    # Simulate training process
    print("Starting training simulation...")
    for i in range(5):
        time.sleep(1)  # Simulate training time
        progress = (i + 1) * 20
        print(f"Training progress: {progress}%")
    
    # Generate mock weights (in real scenario, this would be actual model weights)
    weights = {
        'layer1': np.random.randn(128, 64).tolist(),
        'layer2': np.random.randn(64, 32).tolist(),
        'layer3': np.random.randn(32, 10).tolist(),
        'metadata': {
            'batch_size': len(image_files),
            'training_time': 5.0,
            'accuracy': 0.85 + np.random.random() * 0.1
        }
    }
    
    # Ensure output directory exists
    os.makedirs(os.path.dirname(args.weights_output), exist_ok=True)
    
    # Save weights to file
    weights_file = args.weights_output + '.json'
    with open(weights_file, 'w') as f:
        json.dump(weights, f, indent=2)
    
    print(f"Training completed successfully!")
    print(f"Weights saved to: {weights_file}")
    print(f"Model accuracy: {weights['metadata']['accuracy']:.3f}")

if __name__ == '__main__':
    main()