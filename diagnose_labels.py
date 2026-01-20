"""
Diagnostic script to verify label extraction consistency between training and evaluation.
This script will help identify why accuracy drops from 91% to 17%.
"""

import sys
from pathlib import Path

# Add paths for imports
sys.path.append(str(Path(__file__).parent / 'edge'))
sys.path.append(str(Path(__file__).parent / 'main'))

from edge.model_utils import CustomImageDataset
from main.evaluate import ImageDataset, DatasetConfig
import torchvision.transforms as transforms

def test_label_extraction():
    """Test if both implementations extract labels consistently."""
    
    print("="*80)
    print("LABEL EXTRACTION DIAGNOSTIC TEST")
    print("="*80)
    
    data_dir = "./mnist_jpg"
    
    # Test 1: Load data with training implementation
    print("\n[TEST 1] Loading data with TRAINING implementation (model_utils.py)")
    print("-"*80)
    
    transform = transforms.Compose([
        transforms.Resize((224, 224)),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
    ])
    
    train_dataset = CustomImageDataset(data_dir, transform=None)
    
    print(f"✓ Loaded {len(train_dataset)} images")
    print(f"✓ Number of classes: {len(train_dataset.class_to_idx)}")
    print(f"✓ Class names: {sorted(train_dataset.class_to_idx.keys())}")
    print(f"✓ Class mapping: {train_dataset.class_to_idx}")
    
    # Test 2: Load data with evaluation implementation
    print("\n[TEST 2] Loading data with EVALUATION implementation (evaluate.py)")
    print("-"*80)
    
    config = DatasetConfig()
    config.auto_detect_from_data(data_dir)
    eval_dataset = ImageDataset(data_dir, transform=None, config=config)
    
    print(f"✓ Loaded {len(eval_dataset.samples)} images")
    print(f"✓ Number of classes: {config.num_classes}")
    print(f"✓ Class names: {config.class_names}")
    
    # Test 3: Compare first 20 samples
    print("\n[TEST 3] Comparing first 20 samples")
    print("-"*80)
    print(f"{'Index':<8} {'Filename':<40} {'Train Label':<12} {'Eval Label':<12} {'Match':<8}")
    print("-"*80)
    
    mismatches = 0
    for i in range(min(20, len(train_dataset), len(eval_dataset.samples))):
        train_path = train_dataset.image_paths[i]
        train_label = train_dataset.labels[i]
        
        eval_path, eval_label = eval_dataset.samples[i]
        
        filename = Path(train_path).name
        match = "✓" if train_label == eval_label else "✗ MISMATCH"
        
        if train_label != eval_label:
            mismatches += 1
        
        print(f"{i:<8} {filename:<40} {train_label:<12} {eval_label:<12} {match:<8}")
    
    # Test 4: Statistical comparison
    print("\n[TEST 4] Statistical Analysis")
    print("-"*80)
    
    # Count label distribution in training dataset
    train_label_counts = {}
    for label in train_dataset.labels[:1000]:  # First 1000 samples
        train_label_counts[label] = train_label_counts.get(label, 0) + 1
    
    # Count label distribution in eval dataset
    eval_label_counts = {}
    for _, label in eval_dataset.samples[:1000]:  # First 1000 samples
        eval_label_counts[label] = eval_label_counts.get(label, 0) + 1
    
    print(f"\nLabel distribution (first 1000 samples):")
    print(f"{'Label':<10} {'Train Count':<15} {'Eval Count':<15} {'Match':<8}")
    print("-"*80)
    
    all_labels = sorted(set(list(train_label_counts.keys()) + list(eval_label_counts.keys())))
    for label in all_labels:
        train_count = train_label_counts.get(label, 0)
        eval_count = eval_label_counts.get(label, 0)
        match = "✓" if train_count == eval_count else "✗"
        print(f"{label:<10} {train_count:<15} {eval_count:<15} {match:<8}")
    
    # Test 5: Check specific MNIST filename parsing
    print("\n[TEST 5] Testing MNIST filename parsing")
    print("-"*80)
    
    test_filenames = [
        "mnist_train_00001_label_5.jpg",
        "mnist_train_00123_label_0.jpg",
        "mnist_train_00456_label_9.jpg"
    ]
    
    for filename in test_filenames:
        # Simulate training extraction
        if 'label_' in filename:
            train_class_name = filename.split('label_')[-1].split('.')[0]
            train_label = train_dataset.class_to_idx.get(train_class_name, -1)
        else:
            train_label = -1
        
        # Simulate evaluation extraction
        if config.class_names:
            class_to_idx = {cls_name: idx for idx, cls_name in enumerate(config.class_names)}
        else:
            class_to_idx = {}
        
        # This mimics the buggy evaluation code
        if 'label_' in filename:
            label_str = filename.split('label_')[-1].split('.')[0]
            eval_label = class_to_idx.get(label_str, int(label_str) if label_str.isdigit() else 0)
        elif '_' in filename:  # BUG: This might execute for MNIST files
            potential_label = filename.split('_')[0]  # Gets "mnist"
            eval_label = class_to_idx.get(potential_label, int(potential_label) if potential_label.isdigit() else 0)
        else:
            eval_label = -1
        
        match = "✓" if train_label == eval_label else "✗ MISMATCH"
        print(f"{filename:<40} Train: {train_label:<5} Eval: {eval_label:<5} {match}")
    
    # Summary
    print("\n" + "="*80)
    print("DIAGNOSTIC SUMMARY")
    print("="*80)
    
    if mismatches == 0:
        print("✓ All labels match between training and evaluation implementations")
        print("✓ The accuracy issue is likely NOT due to label extraction")
    else:
        print(f"✗ Found {mismatches} label mismatches in first 20 samples")
        print("✗ This is the ROOT CAUSE of the accuracy drop!")
        print("\nRECOMMENDATION:")
        print("  1. Fix the label extraction logic in evaluate.py")
        print("  2. Ensure class_to_idx mapping is consistent")
        print("  3. Use the same CustomImageDataset for both training and evaluation")
    
    print("="*80)

if __name__ == "__main__":
    test_label_extraction()
