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
import sys
from pathlib import Path
from collections import defaultdict
import re

def natural_sort_key(s):
    """Key for natural/alphanumeric sorting (e.g., '2' comes before '10')"""
    return [int(text) if text.isdigit() else text.lower()
            for text in re.split('([0-9]+)', str(s))]

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
        'classes': sorted(list(classes), key=natural_sort_key),
        'subclasses': {k: sorted(list(v), key=natural_sort_key) for k, v in subclasses.items()},
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

def preprocess_universal(data_dir="./data", output_dir=None, split_ratio=0.8, 
                        seed=42, cleanup=False):
    """
    Universal preprocessing - auto-detects and handles any dataset format
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
    
    # Avoid recursion if output_dir is inside data_dir
    if os.path.abspath(output_dir).startswith(os.path.abspath(data_dir)):
        # If the input IS the processed_data, we should just return it
        if os.path.exists(os.path.join(data_dir, "train")):
            print("  ✓ Data already appears processed and split.")
            return {"type": "already_processed", "output_dir": data_dir}
    
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
        print("\n📁 Processing nested train/test structure...")
        train_images = get_images_from_nested_dir(os.path.join(data_dir, "train"), train_csv)
        train_stats = copy_and_flatten(train_images, output_train)
        test_images = get_images_from_nested_dir(os.path.join(data_dir, "test"), test_csv or train_csv)
        test_stats = copy_and_flatten(test_images, output_test)
        overall_stats['train_stats'] = train_stats
        overall_stats['test_stats'] = test_stats
    
    elif structure['type'] == 'flat_split':
        print("\n📁 Processing flat train/test structure...")
        if not train_csv or not train_csv.get('has_labels'):
            return {"error": "CSV labels required for flat split"}
        train_images = get_images_from_flat_dir(os.path.join(data_dir, "train"), train_csv)
        train_stats = copy_and_flatten(train_images, output_train)
        test_csv_final = test_csv if test_csv and test_csv.get('has_labels') else train_csv
        test_images = get_images_from_flat_dir(os.path.join(data_dir, "test"), test_csv_final)
        test_stats = copy_and_flatten(test_images, output_test)
        overall_stats['train_stats'] = train_stats
        overall_stats['test_stats'] = test_stats
    
    elif structure['type'] in ['nested_nosplit', 'flat_nosplit', 'only_train', 'mixed_split']:
        # Simplified for brevity, handle mixed and no-split
        print(f"\n📁 Processing {structure['type']}...")
        source_dir = os.path.join(data_dir, "train") if structure['type'] == 'only_train' else data_dir
        
        is_nested = structure['train_nested'] if structure['type'] == 'only_train' else 'nested' in structure['type']
        if is_nested:
            all_images = get_images_from_nested_dir(source_dir, train_csv)
        else:
            all_images = get_images_from_flat_dir(source_dir, train_csv)
            
        train_images, test_images = split_images(all_images, split_ratio, seed)
        train_stats = copy_and_flatten(train_images, output_train)
        test_stats = copy_and_flatten(test_images, output_test)
        overall_stats['train_stats'] = train_stats
        overall_stats['test_stats'] = test_stats
    
    # Create label mappings
    print("\n📝 Creating label mappings...")
    create_label_files(output_dir)
    
    # Save report
    report_path = os.path.join(output_dir, "_preprocessing_report.json")
    with open(report_path, 'w') as f:
        json.dump(overall_stats, f, indent=2, default=str)
    
    print(f"\n✅ Complete! Report: {report_path}")
    return overall_stats

def create_label_files(output_dir):
    """Create label mapping JSON files with NATURAL SORTING"""
    image_extensions = ('.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp')
    
    for split in ['train', 'test']:
        split_dir = os.path.join(output_dir, split)
        if not os.path.exists(split_dir):
            continue
        
        labels_info = {}
        classes_found = set()
        subclasses_found = defaultdict(set)
        
        # 1. First pass: Collect all unique classes
        for filename in os.listdir(split_dir):
            if filename.lower().endswith(image_extensions) and '#' in filename:
                parts = filename.split('#')
                class_name = parts[0]
                subclass_name = parts[1] if len(parts) > 2 else None
                
                labels_info[filename] = {
                    'class': class_name,
                    'subclass': subclass_name
                }
                classes_found.add(class_name)
                if subclass_name:
                    subclasses_found[class_name].add(subclass_name)
        
        # 2. Sort classes NATURALLY
        sorted_classes = sorted(list(classes_found), key=natural_sort_key)
        class_to_idx = {cls: i for i, cls in enumerate(sorted_classes)}
        
        # 3. Handle subclasses
        subclass_to_idx = {}
        for cls_name in sorted_classes:
            if cls_name in subclasses_found:
                sorted_subs = sorted(list(subclasses_found[cls_name]), key=natural_sort_key)
                for sub in sorted_subs:
                    full = f"{cls_name}_{sub}"
                    if full not in subclass_to_idx:
                        subclass_to_idx[full] = len(subclass_to_idx)
        
        label_data = {
            "labels": labels_info,
            "class_to_idx": class_to_idx,
            "subclass_to_idx": subclass_to_idx,
            "num_classes": len(class_to_idx),
            "num_subclasses": len(subclass_to_idx)
        }
        
        with open(os.path.join(split_dir, f"labels_{split}.json"), 'w') as f:
            json.dump(label_data, f, indent=2)
        
        print(f"  ✓ {split}: {len(labels_info)} images, {len(class_to_idx)} classes")

def simple_in_place_split(data_dir, train_ratio=0.8, seed=42):
    """
    Simple in-place splitting of images into flat train and test folders.
    All images are dumped directly into train/ and test/ without class subdirectories.
    """
    import random
    random.seed(seed)
    
    image_extensions = ('.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp')
    train_dir = os.path.join(data_dir, "train")
    test_dir = os.path.join(data_dir, "test")
    
    # If already split and populated, skip
    if os.path.exists(train_dir) and os.path.exists(test_dir):
        train_images = [f for f in os.listdir(train_dir) if f.lower().endswith(image_extensions)]
        test_images = [f for f in os.listdir(test_dir) if f.lower().endswith(image_extensions)]
        if train_images and test_images:
            print(f"  ✓ Data already split in {data_dir}")
            return train_dir

    os.makedirs(train_dir, exist_ok=True)
    os.makedirs(test_dir, exist_ok=True)
    
    # Collect all images
    all_images = []
    print(f"🔍 Scanning {data_dir} for images...")
    
    for root, dirs, files in os.walk(data_dir):
        # Skip existing train/test dirs
        rel_root = os.path.relpath(root, data_dir)
        if rel_root.startswith("train") or rel_root.startswith("test"):
            continue
            
        for file in files:
            if file.lower().endswith(image_extensions):
                full_path = os.path.join(root, file)
                all_images.append(full_path)
    
    if not all_images:
        print(f"  ⚠️  No images found to split in {data_dir}")
        return train_dir
        
    print(f"  Splitting {len(all_images)} images...")
    
    random.shuffle(all_images)
    split_idx = int(len(all_images) * train_ratio)
    
    train_set = all_images[:split_idx]
    test_set = all_images[split_idx:]
    
    for img_path in train_set:
        dest = os.path.join(train_dir, os.path.basename(img_path))
        if img_path != dest:
            shutil.move(img_path, dest)
            
    for img_path in test_set:
        dest = os.path.join(test_dir, os.path.basename(img_path))
        if img_path != dest:
            shutil.move(img_path, dest)

    # Cleanup empty original subdirectories
    for root, dirs, files in os.walk(data_dir, topdown=False):
        rel_root = os.path.relpath(root, data_dir)
        if rel_root == "." or rel_root.startswith("train") or rel_root.startswith("test"):
            continue
        try:
            if not os.listdir(root):
                os.rmdir(root)
        except OSError:
            pass

    print(f"✅ Split complete: {len(train_set)} train, {len(test_set)} test")
    return train_dir


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Universal Data Preprocessor")
    parser.add_argument("--data_dir", type=str, required=True, help="Path to data directory")
    parser.add_argument("--split", action="store_true", help="Split data into train/test")
    parser.add_argument("--universal", action="store_true", help="Force universal processing")
    
    args = parser.parse_args()
    
    # ORCHESTRATOR COMPATIBILITY: If it looks like it's already processed,
    # just print path and exit (unless --split or --universal is explicitly passed)
    train_dir = os.path.join(args.data_dir, "train")
    if os.path.exists(train_dir) and not args.universal and not args.split:
        print(train_dir)
        sys.exit(0)
        
    if args.universal:
        preprocess_universal(args.data_dir)
        print(os.path.join(args.data_dir, "train") if "processed_data" in args.data_dir else os.path.join(os.path.dirname(args.data_dir), "processed_data", "train"))
    elif args.split:
        print(simple_in_place_split(args.data_dir))
    else:
        # Default go-style behavior
        if os.path.exists(train_dir):
            print(train_dir)
        else:
            print(args.data_dir)