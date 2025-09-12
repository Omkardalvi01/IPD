import numpy as np
import os

def generate_mobilenetv2_sample(output_path: str, client_id: str):
    """Generate a sample weight file for MobileNetV2 architecture"""
    
    # Define the layer structure
    layers = {
        # First Conv2d layer
        'features.0.0.weight': (32, 3, 3, 3),
        'features.0.1.weight': (32,),
        'features.0.1.bias': (32,),
        'features.0.1.running_mean': (32,),
        'features.0.1.running_var': (32,),
        
        # First bottleneck's depthwise conv
        'features.1.conv.0.0.weight': (32, 1, 3, 3),
        'features.1.conv.1.weight': (32,),
        'features.1.conv.1.bias': (32,),
        'features.1.conv.1.running_mean': (32,),
        'features.1.conv.1.running_var': (32,),
        
        # First bottleneck's pointwise conv
        'features.1.conv.3.weight': (16, 32, 1, 1),
        'features.1.conv.4.weight': (16,),
        'features.1.conv.4.bias': (16,),
        'features.1.conv.4.running_mean': (16,),
        'features.1.conv.4.running_var': (16,),
        
        # Example of expanded bottleneck
        'features.2.conv.0.0.weight': (96, 16, 1, 1),
        'features.2.conv.1.weight': (96,),
        'features.2.conv.1.bias': (96,),
        
        # Final conv layer
        'features.18.0.weight': (1280, 320, 1, 1),
        'features.18.1.weight': (1280,),
        'features.18.1.bias': (1280,),
        
        # Classifier (modified for MNIST)
        'classifier.1.weight': (10, 1280),
        'classifier.1.bias': (10,)
    }
    
    os.makedirs(output_path, exist_ok=True)
    file_path = os.path.join(output_path, f"{client_id}.txt")
    
    with open(file_path, 'w') as f:
        for name, shape in layers.items():
            # Write layer name and shape
            f.write(f"{name} {' '.join(map(str, shape))}\n")
            
            # Generate random weights
            size = np.prod(shape)
            # Use smaller initialization for weights
            if 'weight' in name:
                weights = np.random.normal(0, 0.02, size=size)
            # Use zeros for biases and running stats
            else:
                weights = np.zeros(size)
            
            # Write weights as space-separated values
            f.write(' '.join(map(lambda x: f"{x:.6f}", weights)))
            f.write('\n')
    
    print(f"Generated MobileNetV2 sample weight file: {file_path}")
    print("\nLayer shapes:")
    total_params = 0
    for name, shape in layers.items():
        params = np.prod(shape)
        total_params += params
        print(f"  {name}: {shape} ({params:,} parameters)")
    print(f"\nTotal parameters: {total_params:,}")

if __name__ == "__main__":
    # Generate a sample file
    generate_mobilenetv2_sample("output", "client1")
