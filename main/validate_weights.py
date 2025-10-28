import torch
import sys
import os
import numpy as np
from typing import Dict, List, Tuple, Any

def validate_weight_file(file_path) -> Tuple[bool, Dict[str, torch.Tensor]]:
    """
    Validate that a weight file is in the correct format and return its tensors
    Returns:
        Tuple of (success: bool, tensors: Dict[str, torch.Tensor])
    """
    try:
        # Read the file
        weights_dict = {}
        with open(file_path, 'r') as f:
            lines = f.readlines()
            
        # Expected format:
        # layer_name shape
        # value1 value2 value3 ...
        i = 0
        while i < len(lines):
            # Skip empty lines
            if not lines[i].strip():
                i += 1
                continue
                
            # Parse layer info
            layer_info = lines[i].strip().split()
            if len(layer_info) < 2:
                raise ValueError(f"Invalid layer info format at line {i+1}")
                
            layer_name = layer_info[0]
            shape = [int(x) for x in layer_info[1:]]
            
            # Next line should contain the weights
            i += 1
            if i >= len(lines):
                raise ValueError(f"Missing weights for layer {layer_name}")
                
            # Parse weights
            weights_str = lines[i].strip().split()
            weights = [float(x) for x in weights_str]
            
            # Verify number of weights matches shape
            expected_size = np.prod(shape)
            if len(weights) != expected_size:
                raise ValueError(f"Weight count mismatch for {layer_name}: expected {expected_size}, got {len(weights)}")
                
            # Reshape weights according to the shape
            weights_array = np.array(weights).reshape(shape)
            weights_dict[layer_name] = weights_array
            
            i += 1
        
        # Convert numpy arrays to tensors
        tensors = {}
        for name, weights in weights_dict.items():
            try:
                tensor = torch.tensor(weights, dtype=torch.float32)
                tensors[name] = tensor
                print(f"✓ {name}: shape {tensor.shape}, dtype {tensor.dtype}")
            except Exception as e:
                print(f"✗ Error in parameter {name}: {str(e)}")
                raise
        
        print("\nValidation successful! All weights could be converted to tensors.")
        return True, tensors
        
    except Exception as e:
        print(f"\nValidation failed: {str(e)}")
        return False, {}

def validate_edge_weights(edge_files: List[str]) -> Tuple[bool, Dict[str, Dict[str, torch.Tensor]]]:
    """
    Validate multiple edge weight files and ensure they have consistent parameters
    Args:
        edge_files: List of paths to edge weight files
    Returns:
        Tuple of (success: bool, weights: Dict[str, Dict[str, torch.Tensor]])
    """
    if not edge_files:
        print("No edge files provided")
        return False, {}

    # Store weights from all files
    all_weights = {}
    reference_shapes = None

    for file_path in edge_files:
        print(f"\nValidating {file_path}...")
        success, weights = validate_weight_file(file_path)
        if not success:
            return False, {}

        # Get client ID from filename
        client_id = os.path.basename(file_path).replace('.txt', '')
        all_weights[client_id] = weights

        # Check parameter consistency
        if reference_shapes is None:
            # First file becomes reference
            reference_shapes = {name: tensor.shape for name, tensor in weights.items()}
            reference_layers = set(weights.keys())
        else:
            # Check against reference
            current_layers = set(weights.keys())
            if current_layers != reference_layers:
                print(f"Error: Layer mismatch in {file_path}")
                print(f"Expected layers: {reference_layers}")
                print(f"Found layers: {current_layers}")
                return False, {}

            for name, tensor in weights.items():
                if tensor.shape != reference_shapes[name]:
                    print(f"Error: Shape mismatch in {file_path}, layer {name}")
                    print(f"Expected shape: {reference_shapes[name]}")
                    print(f"Found shape: {tensor.shape}")
                    return False, {}

    print("\nAll edge weight files validated successfully!")
    print(f"Number of clients: {len(all_weights)}")
    print("Layer shapes:")
    for name, shape in reference_shapes.items():
        print(f"  {name}: {shape}")

    return True, all_weights

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python validate_weights.py <weight_file1.txt> [weight_file2.txt ...]")
        sys.exit(1)
        
    edge_files = sys.argv[1:]
    for file_path in edge_files:
        if not os.path.exists(file_path):
            print(f"Error: File {file_path} does not exist")
            sys.exit(1)
        
    success, _ = validate_edge_weights(edge_files)
    sys.exit(0 if success else 1)
