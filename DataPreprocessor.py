"""
Universal Smart Data Preprocessor for Federated Learning
Automatically detects and handles multiple dataset formats:
1. Nested directories (class/subclass/images)
2. Flat directories with CSV labels
3. Mixed structures
4. ImageNet-style folders
"""

import os
import shutil
import json
import random
import csv
from pathlib import Path
from collections import defaultdict

def load_csv_mapping(csv_path):
    """Load CSV file with flexible column detection"""
    if not os.path.exists(csv_path):
        return None
    
    mapping = {}
    classes = set()
    subclasses = defaultdict(set)
    
    with open(csv_path, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        header = reader.fieldnames
        
        if not header:
            return None
        
        print(f"  CSV columns: {', '.join(header)}")
        
        # Detect column names
        filename_col = None
        class_col = None
        subclass_col = None
        
        for col in header:
            col_lower = col.lower().strip()
            if col_lower in ['filename', 'image', 'image_name', 'file', 'name', 'image_id']:
                filename_col = col
            elif col_lower in ['class', 'label', 'category', 'main_class', 'primary_class', 'target']:
                class_col = col
            elif col_lower in ['subclass', 'subcategory', 'sub_class', 'secondary_class', 'sublabel']:
                subclass_col = col
        
        if not filename_col:
            print(f"  ⚠️  No filename column found")
            return None
        
        if not class_col:
            print(f"  ⚠️  No class/label column found - CSV has only filenames")
            return None
        
        print(f"  Detected: filename='{filename_col}', class='{class_col}'", end="")
        if subclass_col:
            print(f", subclass='{subclass_col}'")
        else:
            print()
        
        for row in reader:
            filename = row[filename_col].strip()
            class_name = row[class_col].strip()
            subclass_name = row[subclass_col].strip() if subclass_col and row.get(subclass_col) else None
            
            if filename and class_name:
                mapping[filename] = {
                    'class': class_name,
                    'subclass': subclass_name
                }
                classes.add(class_name)
                if subclass_name:
                    subclasses[class_name].add(subclass_name)
    
    return {
        'mapping': mapping,
        'classes': sorted(list(classes)),
        'subclasses': {k: sorted(list(v)) for k, v in subclasses.items()},
        'has_labels': True
    }

def match_filename_to_csv(filename, csv_mapping):
    """Match filename to CSV with flexible matching"""
    if not csv_mapping or not csv_mapping.get('has_labels'):
        return None
    
    mapping = csv_mapping['mapping']
    
    # Direct match
    if filename in mapping:
        return mapping[filename]
    
    # Without extension
    name_without_ext = os.path.splitext(filename)[0]
    if name_without_ext in mapping:
        return mapping[name_without_ext]
    
    # Match with any extension
    for mapped_name in mapping.keys():
        if os.path.splitext(mapped_name)[0] == name_without_ext:
            return mapping[mapped_name]
    
    return None

def detect_dataset_structure(data_dir):
    """
    Intelligently detect dataset structure
    Returns: structure type and relevant paths
    """
    image_extensions = ('.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp')
    
    train_dir = os.path.join(data_dir, "train")
    test_dir = os.path.join(data_dir, "test")
    
    has_train = os.path.exists(train_dir) and os.path.isdir(train_dir)
    has_test = os.path.exists(test_dir) and os.path.isdir(test_dir)
    
    # Find CSV files
    csv_files = [f for f in os.listdir(data_dir) if f.lower().endswith('.csv')]
    
    structure = {
        'type': None,
        'has_train': has_train,
        'has_test': has_test,
        'csv_files': csv_files,
        'train_nested': False,
        'test_nested': False,
        'needs_split': False
    }
    
    # Check if train/test are nested or flat
    if has_train:
        train_contents = os.listdir(train_dir)
        train_subdirs = [d for d in train_contents if os.path.isdir(os.path.join(train_dir, d))]
        train_images = [f for f in train_contents if f.lower().endswith(image_extensions)]
        
        if train_subdirs and not train_images:
            structure['train_nested'] = True
            print("  ✓ Train folder: NESTED (class folders)")
        elif train_images:
            structure['train_nested'] = False
            print("  ✓ Train folder: FLAT (images only)")
        else:
            print("  ⚠️  Train folder appears empty")
    
    if has_test:
        test_contents = os.listdir(test_dir)
        test_subdirs = [d for d in test_contents if os.path.isdir(os.path.join(test_dir, d))]
        test_images = [f for f in test_contents if f.lower().endswith(image_extensions)]
        
        if test_subdirs and not test_images:
            structure['test_nested'] = True
            print("  ✓ Test folder: NESTED (class folders)")
        elif test_images:
            structure['test_nested'] = False
            print("  ✓ Test folder: FLAT (images only)")
        else:
            print("  ⚠️  Test folder appears empty")
    
    # Determine structure type
    if has_train and has_test:
        if structure['train_nested'] and structure['test_nested']:
            structure['type'] = 'nested_split'
        elif not structure['train_nested'] and not structure['test_nested']:
            structure['type'] = 'flat_split'
        else:
            structure['type'] = 'mixed_split'
    elif has_train:
        structure['type'] = 'only_train'
        structure['needs_split'] = True
    else:
        # Check if data_dir itself has class folders
        all_items = os.listdir(data_dir)
        subdirs = [d for d in all_items if os.path.isdir(os.path.join(data_dir, d)) and d not in ['train', 'test']]
        images = [f for f in all_items if f.lower().endswith(image_extensions)]
        
        if subdirs and not images:
            structure['type'] = 'nested_nosplit'
        elif images:
            structure['type'] = 'flat_nosplit'
        else:
            structure['type'] = 'unknown'
        
        structure['needs_split'] = True
    
    print(f"  Detected structure: {structure['type']}")
    if csv_files:
        print(f"  Found CSV files: {', '.join(csv_files)}")
    
    return structure

def get_images_from_nested_dir(directory, csv_mapping=None):
    """Extract images from nested directory structure"""
    image_extensions = ('.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp')
    class_images = defaultdict(list)
    
    for root, dirs, files in os.walk(directory):
        rel_path = os.path.relpath(root, directory)
        
        for file in files:
            if file.lower().endswith(image_extensions):
                full_path = os.path.join(root, file)
                
                # Try CSV first, then directory structure
                if csv_mapping:
                    label_info = match_filename_to_csv(file, csv_mapping)
                    if label_info:
                        class_name = label_info['class']
                        subclass_name = label_info['subclass']
                    else:
                        # Fallback to directory
                        path_parts = Path(rel_path).parts
                        class_name = path_parts[0] if len(path_parts) >= 1 and rel_path != '.' else "unknown"
                        subclass_name = path_parts[1] if len(path_parts) >= 2 else None
                else:
                    # Use directory structure
                    path_parts = Path(rel_path).parts
                    class_name = path_parts[0] if len(path_parts) >= 1 and rel_path != '.' else "unknown"
                    subclass_name = path_parts[1] if len(path_parts) >= 2 else None
                
                class_images[class_name].append({
                    'path': full_path,
                    'filename': file,
                    'class': class_name,
                    'subclass': subclass_name
                })
    
    return dict(class_images)

def get_images_from_flat_dir(directory, csv_mapping):
    """Extract images from flat directory (requires CSV)"""
    image_extensions = ('.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp')
    class_images = defaultdict(list)
    unmapped = []
    
    for file in os.listdir(directory):
        if file.lower().endswith(image_extensions):
            full_path = os.path.join(directory, file)
            
            if csv_mapping and csv_mapping.get('has_labels'):
                label_info = match_filename_to_csv(file, csv_mapping)
                if label_info:
                    class_name = label_info['class']
                    subclass_name = label_info['subclass']
                else:
                    unmapped.append(file)
                    continue
            else:
                # No CSV or CSV has no labels
                class_name = "unknown"
                subclass_name = None
            
            class_images[class_name].append({
                'path': full_path,
                'filename': file,
                'class': class_name,
                'subclass': subclass_name
            })
    
    if unmapped:
        print(f"  ⚠️  {len(unmapped)} images not found in CSV mapping")
    
    return dict(class_images)

def create_labeled_filename(img_info):
    """Create filename: class#subclass#original.ext or class#original.ext"""
    base_name, ext = os.path.splitext(img_info['filename'])
    class_name = img_info['class']
    subclass_name = img_info.get('subclass')
    
    if subclass_name:
        return f"{class_name}#{subclass_name}#{base_name}{ext}"
    else:
        return f"{class_name}#{base_name}{ext}"

def copy_and_flatten(class_images, dest_dir):
    """Copy images to flat destination with labeled filenames"""
    os.makedirs(dest_dir, exist_ok=True)
    
    stats = {
        'count': 0,
        'by_class': defaultdict(int),
        'classes': set()
    }
    
    for class_name, images in class_images.items():
        stats['classes'].add(class_name)
        
        for img_info in images:
            new_filename = create_labeled_filename(img_info)
            dest_path = os.path.join(dest_dir, new_filename)
            
            # Handle conflicts
            counter = 1
            base_name, ext = os.path.splitext(new_filename)
            while os.path.exists(dest_path):
                dest_path = os.path.join(dest_dir, f"{base_name}_{counter}{ext}")
                counter += 1
            
            shutil.copy2(img_info['path'], dest_path)
            stats['count'] += 1
            stats['by_class'][class_name] += 1
            
            if stats['count'] % 100 == 0:
                print(f"    Copied {stats['count']} images...")
    
    stats['classes'] = sorted(list(stats['classes']))
    return stats

def split_images(class_images, train_ratio=0.8, seed=42):
    """Split images by class into train/test"""
    random.seed(seed)
    
    train_images = defaultdict(list)
    test_images = defaultdict(list)
    
    for class_name, images in class_images.items():
        shuffled = images.copy()
        random.shuffle(shuffled)
        
        split_idx = int(len(shuffled) * train_ratio)
        train_images[class_name] = shuffled[:split_idx]
        test_images[class_name] = shuffled[split_idx:]
    
    return dict(train_images), dict(test_images)

def universal_preprocess(data_dir="./data", output_dir=None, split_ratio=0.8, 
                        seed=42, cleanup=False):
    """
    Universal preprocessing - auto-detects and handles any dataset format
    
    Handles:
    - Nested directories (ImageNet-style: class/images)
    - Flat directories with CSV labels
    - Mixed nested/flat structures
    - Existing train/test splits
    - Single directory needing split
    
    Args:
        data_dir: Root data directory
        output_dir: Output directory (default: processed_data)
        split_ratio: Train/test ratio (default: 0.8)
        seed: Random seed
        cleanup: Remove original directories after processing
    """
    if not os.path.exists(data_dir):
        print(f"❌ Directory not found: {data_dir}")
        return {"error": "Directory not found"}
    
    print("=" * 70)
    print("UNIVERSAL DATA PREPROCESSOR")
    print("=" * 70)
    print(f"Source: {data_dir}")
    print(f"Split ratio: {int(split_ratio*100)}/{int((1-split_ratio)*100)}")
    
    # Create output directory
    if output_dir is None:
        parent = os.path.dirname(os.path.abspath(data_dir))
        output_dir = os.path.join(parent, "processed_data")
    
    os.makedirs(output_dir, exist_ok=True)
    print(f"Output: {output_dir}\n")
    
    # Detect structure
    print("🔍 Analyzing dataset structure...")
    structure = detect_dataset_structure(data_dir)
    
    # Load CSV files
    csv_mappings = {}
    for csv_file in structure['csv_files']:
        csv_path = os.path.join(data_dir, csv_file)
        print(f"\n📊 Loading CSV: {csv_file}")
        mapping = load_csv_mapping(csv_path)
        if mapping:
            csv_mappings[csv_file.lower()] = mapping
            print(f"  ✓ Loaded {len(mapping['mapping'])} mappings")
    
    # Find appropriate CSVs for train/test
    train_csv = None
    test_csv = None
    
    for filename, mapping in csv_mappings.items():
        if 'train' in filename:
            train_csv = mapping
        elif 'test' in filename:
            test_csv = mapping
    
    # Fallback: use any available CSV
    if not train_csv and csv_mappings:
        train_csv = list(csv_mappings.values())[0]
    
    output_train = os.path.join(output_dir, "train")
    output_test = os.path.join(output_dir, "test")
    
    overall_stats = {
        'structure_type': structure['type'],
        'train_stats': {},
        'test_stats': {}
    }
    
    # Process based on structure type
    if structure['type'] == 'nested_split':
        # Both train/test exist with nested class folders
        print("\n📁 Processing nested train/test structure...")
        
        print("\n  Processing train...")
        train_images = get_images_from_nested_dir(os.path.join(data_dir, "train"), train_csv)
        train_stats = copy_and_flatten(train_images, output_train)
        print(f"  ✓ Train: {train_stats['count']} images, {len(train_stats['classes'])} classes")
        
        print("\n  Processing test...")
        test_images = get_images_from_nested_dir(os.path.join(data_dir, "test"), test_csv or train_csv)
        test_stats = copy_and_flatten(test_images, output_test)
        print(f"  ✓ Test: {test_stats['count']} images, {len(test_stats['classes'])} classes")
        
        overall_stats['train_stats'] = train_stats
        overall_stats['test_stats'] = test_stats
    
    elif structure['type'] == 'flat_split':
        # Both train/test exist with flat images (need CSV)
        print("\n📁 Processing flat train/test structure...")
        
        if not train_csv or not train_csv.get('has_labels'):
            print("  ❌ Flat directories require CSV with labels!")
            return {"error": "CSV labels required"}
        
        print("\n  Processing train...")
        train_images = get_images_from_flat_dir(os.path.join(data_dir, "train"), train_csv)
        train_stats = copy_and_flatten(train_images, output_train)
        print(f"  ✓ Train: {train_stats['count']} images, {len(train_stats['classes'])} classes")
        
        print("\n  Processing test...")
        test_csv_final = test_csv if test_csv and test_csv.get('has_labels') else train_csv
        test_images = get_images_from_flat_dir(os.path.join(data_dir, "test"), test_csv_final)
        test_stats = copy_and_flatten(test_images, output_test)
        print(f"  ✓ Test: {test_stats['count']} images, {len(test_stats['classes'])} classes")
        
        overall_stats['train_stats'] = train_stats
        overall_stats['test_stats'] = test_stats
    
    elif structure['type'] == 'mixed_split':
        # Mixed nested/flat - handle each appropriately
        print("\n📁 Processing mixed structure...")
        
        print("\n  Processing train...")
        if structure['train_nested']:
            train_images = get_images_from_nested_dir(os.path.join(data_dir, "train"), train_csv)
        else:
            train_images = get_images_from_flat_dir(os.path.join(data_dir, "train"), train_csv)
        train_stats = copy_and_flatten(train_images, output_train)
        print(f"  ✓ Train: {train_stats['count']} images")
        
        print("\n  Processing test...")
        if structure['test_nested']:
            test_images = get_images_from_nested_dir(os.path.join(data_dir, "test"), test_csv or train_csv)
        else:
            test_images = get_images_from_flat_dir(os.path.join(data_dir, "test"), test_csv or train_csv)
        test_stats = copy_and_flatten(test_images, output_test)
        print(f"  ✓ Test: {test_stats['count']} images")
        
        overall_stats['train_stats'] = train_stats
        overall_stats['test_stats'] = test_stats
    
    elif structure['type'] in ['nested_nosplit', 'flat_nosplit', 'only_train']:
        # Need to create split
        print(f"\n🔀 Creating {int(split_ratio*100)}/{int((1-split_ratio)*100)} split...")
        
        if structure['type'] == 'only_train':
            source_dir = os.path.join(data_dir, "train")
            is_nested = structure['train_nested']
        else:
            source_dir = data_dir
            # Check if nested
            items = os.listdir(source_dir)
            subdirs = [d for d in items if os.path.isdir(os.path.join(source_dir, d)) and d not in ['train', 'test']]
            is_nested = bool(subdirs)
        
        # Collect all images
        if is_nested:
            all_images = get_images_from_nested_dir(source_dir, train_csv)
        else:
            all_images = get_images_from_flat_dir(source_dir, train_csv)
        
        print(f"  Found {sum(len(imgs) for imgs in all_images.values())} images")
        print(f"  Classes: {len(all_images)}")
        
        # Split
        train_images, test_images = split_images(all_images, split_ratio, seed)
        
        # Copy
        train_stats = copy_and_flatten(train_images, output_train)
        test_stats = copy_and_flatten(test_images, output_test)
        
        print(f"  ✓ Train: {train_stats['count']} images")
        print(f"  ✓ Test: {test_stats['count']} images")
        
        overall_stats['train_stats'] = train_stats
        overall_stats['test_stats'] = test_stats
    
    else:
        print("❌ Unknown dataset structure!")
        return {"error": "Unknown structure"}
    
    # Cleanup
    if cleanup:
        print("\n🧹 Cleaning up original files...")
        if structure['has_train']:
            shutil.rmtree(os.path.join(data_dir, "train"))
            print("  ✓ Removed train/")
        if structure['has_test']:
            shutil.rmtree(os.path.join(data_dir, "test"))
            print("  ✓ Removed test/")
    
    # Create label mappings
    print("\n📝 Creating label mappings...")
    create_label_files(output_dir)
    
    # Save report
    report_path = os.path.join(output_dir, "_preprocessing_report.json")
    with open(report_path, 'w') as f:
        json.dump(overall_stats, f, indent=2, default=str)
    
    print(f"\n✅ Complete! Report: {report_path}")
    print(f"\n📁 Output structure:")
    print(f"  {output_dir}/")
    print(f"  ├── train/ (flattened, labeled)")
    print(f"  └── test/  (flattened, labeled)")
    
    return overall_stats

def create_label_files(output_dir):
    """Create label mapping JSON files"""
    image_extensions = ('.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp')
    
    for split in ['train', 'test']:
        split_dir = os.path.join(output_dir, split)
        if not os.path.exists(split_dir):
            continue
        
        labels = {}
        class_to_idx = {}
        subclass_to_idx = {}
        
        for filename in os.listdir(split_dir):
            if filename.lower().endswith(image_extensions) and '#' in filename:
                parts = filename.split('#')
                class_name = parts[0]
                subclass_name = parts[1] if len(parts) > 2 else None
                
                labels[filename] = {
                    'class': class_name,
                    'subclass': subclass_name
                }
                
                if class_name not in class_to_idx:
                    class_to_idx[class_name] = len(class_to_idx)
                
                if subclass_name:
                    full = f"{class_name}_{subclass_name}"
                    if full not in subclass_to_idx:
                        subclass_to_idx[full] = len(subclass_to_idx)
        
        label_data = {
            "labels": labels,
            "class_to_idx": class_to_idx,
            "subclass_to_idx": subclass_to_idx,
            "num_classes": len(class_to_idx),
            "num_subclasses": len(subclass_to_idx)
        }
        
        with open(os.path.join(split_dir, f"labels_{split}.json"), 'w') as f:
            json.dump(label_data, f, indent=2)
        
        print(f"  ✓ {split}: {len(labels)} images, {len(class_to_idx)} classes")

def simple_in_place_split(data_dir):
    """
    Check if data_dir has 'train' and 'test' subdirectories.
    If yes, return the train directory path.
    If no, split the files in data_dir into 'train' and 'test' (80/20) IN-PLACE.
    """
    train_dir = os.path.join(data_dir, "train")
    test_dir = os.path.join(data_dir, "test")

    # Check if split already exists
    if os.path.exists(train_dir) and os.path.exists(test_dir):
        # We assume if they exist, they are valid
        return train_dir

    print(f"  Splitting data at {data_dir} into train/test (80/20)...")
    os.makedirs(train_dir, exist_ok=True)
    os.makedirs(test_dir, exist_ok=True)

    # Detect structure: Nested (Classes) or Flat (Images)?
    items = os.listdir(data_dir)
    image_extensions = ('.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp')
    
    subdirs = [d for d in items if os.path.isdir(os.path.join(data_dir, d)) and d not in ['train', 'test']]
    images = [f for f in items if f.lower().endswith(image_extensions)]

    if subdirs and not images:
        # Nested Structure: Split each class folder
        for class_name in subdirs:
            class_dir = os.path.join(data_dir, class_name)
            
            # Create class dirs in train/test
            os.makedirs(os.path.join(train_dir, class_name), exist_ok=True)
            os.makedirs(os.path.join(test_dir, class_name), exist_ok=True)
            
            class_images = [f for f in os.listdir(class_dir) if f.lower().endswith(image_extensions)]
            random.shuffle(class_images)
            
            split_idx = int(len(class_images) * 0.8)
            train_imgs = class_images[:split_idx]
            test_imgs = class_images[split_idx:]
            
            # Move files
            for img in train_imgs:
                shutil.move(os.path.join(class_dir, img), os.path.join(train_dir, class_name, img))
            for img in test_imgs:
                shutil.move(os.path.join(class_dir, img), os.path.join(test_dir, class_name, img))
            
            # Remove empty class dir
            try:
                os.rmdir(class_dir)
            except:
                pass

    elif images:
        # Flat Structure
        random.shuffle(images)
        split_idx = int(len(images) * 0.8)
        train_imgs = images[:split_idx]
        test_imgs = images[split_idx:]
        
        for img in train_imgs:
            shutil.move(os.path.join(data_dir, img), os.path.join(train_dir, img))
        for img in test_imgs:
            shutil.move(os.path.join(data_dir, img), os.path.join(test_dir, img))

    return train_dir


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Data Preprocessor")
    parser.add_argument("--data_dir", type=str, required=True, help="Path to data directory")
    parser.add_argument("--split", action="store_true", help="Split data into train/test if not already split")
    
    args = parser.parse_args()
    
    if args.split:
        # Use simple in-place split logic as requested by user
        train_path = simple_in_place_split(args.data_dir)
        print(train_path) # Print ONLY the path to stdout for Go to capture
    else:
        # Default behavior: check if 'train' exists
        train_dir = os.path.join(args.data_dir, "train")
        if os.path.exists(train_dir):
             print(train_dir)
        else:
             print(args.data_dir)