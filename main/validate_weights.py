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
        print("❌ Error: No edge files provided")
        return False, {}

    # Store weights from all files
    all_weights = {}
    reference_shapes = None
    validation_errors = []

    print(f"\n{'='*50}")
    print(f"Validating {len(edge_files)} weight files...")
    print(f"{'='*50}")

    for file_path in edge_files:
        file_name = os.path.basename(file_path)
        print(f"\n🔍 Validating {file_name}...")
        
        # Check if file exists
        if not os.path.exists(file_path):
            error_msg = f"❌ File not found: {file_path}"
            print(error_msg)
            validation_errors.append(error_msg)
            continue
            
        # Check if file is empty
        if os.path.getsize(file_path) == 0:
            error_msg = f"❌ File is empty: {file_path}"
            print(error_msg)
            validation_errors.append(error_msg)
            continue
            
        # Validate weight file
        success, weights = validate_weight_file(file_path)
        if not success:
            error_msg = f"❌ Validation failed for {file_path}"
            print(error_msg)
            validation_errors.append(error_msg)
            continue

        # Get client ID from filename
        client_id = os.path.basename(file_path).replace('.txt', '')
        all_weights[client_id] = weights
        print(f"✅ Successfully validated {file_name}")
        
        # Check for consistent shapes across clients
        if reference_shapes is None:
            reference_shapes = {k: v.shape for k, v in weights.items()}
            print(f"\nReference shapes set from {file_name}:")
            for k, v in reference_shapes.items():
                print(f"  - {k}: {v}")
        else:
            current_shapes = {k: v.shape for k, v in weights.items()}
            if current_shapes != reference_shapes:
                error_msg = f"❌ Shape mismatch in {file_name}:"
                for k in reference_shapes:
                    if k not in current_shapes:
                        error_msg += f"\n     - Missing layer: {k}"
                    elif current_shapes[k] != reference_shapes[k]:
                        error_msg += f"\n     - {k}: expected {reference_shapes[k]}, got {current_shapes[k]}"
                print(error_msg)
                validation_errors.append(error_msg)
                continue
    
    if validation_errors:
        print(f"\n❌ Validation completed with {len(validation_errors)} error(s):")
        for i, error in enumerate(validation_errors, 1):
            print(f"{i}. {error}")
        return False, {}
    
    if not all_weights:
        print("❌ No valid weight files found")
        return False, {}
        
    print(f"\n✅ Successfully validated {len(all_weights)} weight files")
    
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
