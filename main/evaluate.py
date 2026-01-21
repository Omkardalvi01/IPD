import torch
import torch.nn as nn
from torch.utils.data import Dataset, DataLoader
from torchvision import transforms, models
from PIL import Image
from typing import Optional, Dict, List, Any, Union
import os
import sys
import json
import glob
import shutil
import logging
import argparse
from datetime import datetime
import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import seaborn as sns
from sklearn.metrics import confusion_matrix, precision_recall_fscore_support

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# 1. SETUP DEVICE
device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
logger.info(f"--- Using device: {device} ---")

import re

def natural_sort_key(s):
    """Key for natural/alphanumeric sorting (e.g., '2' comes before '10')"""
    return [int(text) if text.isdigit() else text.lower()
            for text in re.split('([0-9]+)', str(s))]

# 2. CUSTOM DATASET
class UniversalFolderDataset(Dataset):
    def __init__(self, folder_path, transform=None):
        self.folder_path = folder_path
        self.transform = transform
        
        if not os.path.exists(folder_path):
            raise FileNotFoundError(f"Data directory not found: {folder_path}")
            
        # Get all valid images
        self.file_names = sorted([f for f in os.listdir(folder_path) if f.lower().endswith(('.png', '.jpg', '.jpeg'))])
        
        if not self.file_names:
            logger.warning(f"No images found in {folder_path}")

        # DISCOVER CLASSES (Alphanumeric/Natural sorting)
        discovered_classes = set()
        for f in self.file_names:
            label = self._extract_label(f)
            if label is not None:
                discovered_classes.add(label)
        
        self.classes = sorted(list(discovered_classes), key=natural_sort_key)
        self.class_to_idx = {cls_name: i for i, cls_name in enumerate(self.classes)}
        
        logger.info(f"Discovered classes (Naturally Sorted): {self.classes}")
        logger.info(f"Class-to-Idx mapping: {self.class_to_idx}")

    def _extract_label(self, img_name):
        """Extract label from filename: 'class#img.jpg' or 'mnist_..._label_X.jpg'"""
        try:
            if '_label_' in img_name:
                return img_name.split('_label_')[1].split('.')[0]
            elif '#' in img_name:
                return img_name.split('#')[0]
            return None
        except Exception:
            return None

    def __len__(self):
        return len(self.file_names)

    def __getitem__(self, idx):
        img_name = self.file_names[idx]
        img_path = os.path.join(self.folder_path, img_name)
        image = Image.open(img_path).convert('RGB') 
        
        label_str = self._extract_label(img_name)
        label = self.class_to_idx.get(label_str, 0)
            
        if self.transform:
            image = self.transform(image)
        return image, label

def get_model(num_classes: int, device: torch.device) -> torch.nn.Module:
    """Get MobileNetV2 with specific output classes"""
    # DO NOT USE PRETRAINED WEIGHTS as requested by user
    model = models.mobilenet_v2(pretrained=False)
    in_features = model.classifier[1].in_features
    model.classifier[1] = nn.Linear(in_features, num_classes)
    return model.to(device)

def plot_confusion_matrix(y_true, y_pred, classes, output_path):
    """Plot and save confusion matrix using seaborn"""
    plt.figure(figsize=(12, 10))
    cm = confusion_matrix(y_true, y_pred)
    # Normalize by row (true labels)
    cm_norm = cm.astype('float') / cm.sum(axis=1)[:, np.newaxis]
    
    sns.heatmap(cm_norm, annot=True, fmt='.2f', cmap='Blues',
                xticklabels=classes, yticklabels=classes)
    plt.title('Confusion Matrix (Recall/Accuracy per Class)')
    plt.ylabel('True label')
    plt.xlabel('Predicted label')
    plt.tight_layout()
    plt.savefig(output_path, dpi=300)
    plt.close()

def plot_per_class_metrics(results, classes, output_path):
    """Plot Bar chart for Precision, Recall, and F1 (Per Class)"""
    x = np.arange(len(classes))
    width = 0.25
    
    # Extract values
    precision = [results['per_class_metrics'][cls]['precision'] for cls in classes]
    recall = [results['per_class_metrics'][cls]['recall'] for cls in classes]
    f1 = [results['per_class_metrics'][cls]['f1_score'] for cls in classes]
    
    plt.figure(figsize=(14, 7))
    plt.bar(x - width, precision, width, label='Precision', color='#3498db', alpha=0.8)
    plt.bar(x, recall, width, label='Recall', color='#2ecc71', alpha=0.8)
    plt.bar(x + width, f1, width, label='F1 Score', color='#e74c3c', alpha=0.8)
    
    plt.xlabel('Class Label')
    plt.ylabel('Score (0.0 - 1.0)')
    plt.title('Model Performance Metrics by Class')
    plt.xticks(x, classes)
    plt.ylim(0, 1.1)
    plt.legend(loc='lower right')
    plt.grid(axis='y', linestyle='--', alpha=0.6)
    
    # Add values on top
    for i in range(len(classes)):
        plt.text(i - width, precision[i] + 0.01, f"{precision[i]:.2f}", ha='center', va='bottom', fontsize=8)
        plt.text(i, recall[i] + 0.01, f"{recall[i]:.2f}", ha='center', va='bottom', fontsize=8)
        plt.text(i + width, f1[i] + 0.01, f"{f1[i]:.2f}", ha='center', va='bottom', fontsize=8)
        
    plt.tight_layout()
    plt.savefig(output_path, dpi=300)
    plt.close()

def find_latest_model(model_dir: str = "./global_models") -> Optional[str]:
    """Find the latest model checkpoint"""
    potential_dirs = [model_dir, "../global_models", "main/global_models"]
    
    for d in potential_dirs:
        if os.path.exists(d):
            model_files = []
            for ext in ['.pth', '.pt']:
                model_files.extend(glob.glob(os.path.join(d, f'*{ext}')))
            
            if model_files:
                model_files.sort(key=os.path.getmtime, reverse=True)
                return model_files[0]
    return None

def save_detailed_report(results, output_dir):
    """Save JSON and TXT reports with enhanced metrics"""
    os.makedirs(output_dir, exist_ok=True)
    
    # Save JSON
    with open(os.path.join(output_dir, 'evaluation_results.json'), 'w') as f:
        json.dump(results, f, indent=4)
    
    # Save Summary TXT
    with open(os.path.join(output_dir, 'evaluation_summary.txt'), 'w') as f:
        f.write("="*70 + "\n")
        f.write("FEDERATED GLOBAL MODEL EVALUATION REPORT\n")
        f.write("="*70 + "\n")
        f.write(f"Timestamp:      {results['timestamp']}\n")
        f.write(f"Global Accuracy: {results['global_metrics']['accuracy']:.2f}%\n")
        f.write(f"Macro Precision: {results['global_metrics']['macro_precision']:.4f}\n")
        f.write(f"Macro Recall:    {results['global_metrics']['macro_recall']:.4f}\n")
        f.write(f"Macro F1 Score:  {results['global_metrics']['macro_f1']:.4f}\n")
        f.write(f"Total Samples:   {results['total_samples']}\n")
        f.write("-" * 70 + "\n")
        f.write(f"{'Class':<8} | {'Accuracy':<10} | {'Precision':<10} | {'Recall':<8} | {'F1':<8}\n")
        f.write("-" * 70 + "\n")
        
        for cls_name in sorted(results['per_class_metrics'].keys(), key=lambda x: int(x)):
            m = results['per_class_metrics'][cls_name]
            f.write(f"{cls_name:<8} | {m['accuracy']:>8.2f}% | {m['precision']:>9.4f} | {m['recall']:>8.4f} | {m['f1_score']:>8.4f}\n")
        
        f.write("="*70 + "\n")
        f.write(f"Model: {results['model_used']}\n")
        f.write(f"Data:  {results['data_used']}\n")

# 4. MAIN EXECUTION
if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Federated Learning Global Model Evaluator")
    parser.add_argument('--data-dir', type=str, help='Path to test data directory')
    parser.add_argument('--test-dir', type=str, help='Alias for --data-dir') # For backward compatibility
    parser.add_argument('--model-path', type=str, default=None, help='Path to model weights')
    parser.add_argument('--output-dir', type=str, default='./evaluation_results', help='Output directory for results')
    parser.add_argument('--num-classes', type=int, default=10, help='Number of classes')
    
    args, unknown = parser.parse_known_args()
    
    # Resolve dynamic data directory
    val_folder = args.data_dir or args.test_dir
    if not val_folder:
        val_folder = "/home/mihir/Desktop/Final_IPD/IPD-F/processed_data/test"
        if not os.path.exists(val_folder): val_folder = "processed_data/test"

    os.makedirs(args.output_dir, exist_ok=True)
    logger.info(f"Targeting evaluation data at: {val_folder}")
    
    # Transforms (224x224 RGB as requested for MobileNetV2)
    transform = transforms.Compose([
        transforms.Resize((224, 224)),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
    ])

    try:
        val_dataset = UniversalFolderDataset(val_folder, transform=transform)
        val_loader = DataLoader(val_dataset, batch_size=32, shuffle=False)
        logger.info(f"Loaded {len(val_dataset)} test samples.")
    except Exception as e:
        logger.error(f"Failed to load dataset: {e}")
        sys.exit(1)

    # Initialize model and load weights
    num_classes = args.num_classes
    discovered_count = len(val_dataset.classes)
    
    # Auto-adjust if discovered more than default, and user hasn't explicitly overridden to something else
    if discovered_count > 0 and num_classes == 10 and discovered_count != 10:
        logger.info(f"Auto-adjusting num_classes to {discovered_count} (discovered from test data)")
        num_classes = discovered_count
        
    model = get_model(num_classes=num_classes, device=device)
    final_model_path = args.model_path or find_latest_model()
        
    if final_model_path and os.path.exists(final_model_path):
        logger.info(f"Loading weights from {final_model_path}")
        try:
            checkpoint = torch.load(final_model_path, map_location=device)
            state_dict = checkpoint['model_state_dict'] if isinstance(checkpoint, dict) and 'model_state_dict' in checkpoint else checkpoint
            
            # Clean state_dict
            new_state_dict = { (k[7:] if k.startswith('module.') else k): v for k, v in state_dict.items() }
            
            # Detailed mismatch check before loading
            model_dict = model.state_dict()
            mismatches = []
            for k, v in new_state_dict.items():
                if k in model_dict:
                    if v.shape != model_dict[k].shape:
                        mismatches.append(f"  - {k}: architecture has {model_dict[k].shape}, but checkpoint has {v.shape}")
            
            if mismatches:
                logger.error(f"FATAL: ARCHITECTURE MISMATCH DETECTED for {len(mismatches)} layers:")
                for m in mismatches[:10]: logger.error(m)
                if len(mismatches) > 10: logger.error(f"  ... and {len(mismatches)-10} more")
                logger.error("Suggestion: Delete old 'global_models/' and 'outputs/' and restart training for a clean MobileNetV2 run.")
                sys.exit(1)

            # Enforce STRICT weight loading
            model.load_state_dict(new_state_dict, strict=True)
            logger.info("Successfully loaded model weights (STRICT MODE).")
            
            # --- PROOF OF WEIGHT LOADING ---
            # Print statistics of the classifier layer to show it's not random/empty
            if 'classifier.1.weight' in new_state_dict:
                w = new_state_dict['classifier.1.weight']
                logger.info(f"Loaded Weight Stats [classifier.1.weight]: Mean={w.mean():.6f}, Std={w.std():.6f}, Max={w.max():.6f}")
            # -------------------------------
            
        except Exception as e:
            logger.error(f"Failed to load state dict (STRICT): {e}")
            sys.exit(1)
    else:
        logger.error("No model weights found.")
        sys.exit(1)

    # EVALUATION
    model.eval()
    all_preds, all_targets = [], []
    
    logger.info("Starting evaluation...")
    with torch.no_grad():
        for images, labels in val_loader:
            images, labels = images.to(device), labels.to(device)
            outputs = model(images)
            _, predicted = torch.max(outputs.data, 1)
            all_preds.extend(predicted.cpu().numpy())
            all_targets.extend(labels.cpu().numpy())
    
    if not all_targets:
        logger.error("No samples evaluated."); sys.exit(1)
        
    # CALCULATE METRICS
    y_true = np.array(all_targets)
    y_pred = np.array(all_preds)
    
    # Global Metrics
    precision, recall, f1, _ = precision_recall_fscore_support(y_true, y_pred, average='weighted')
    macro_precision, macro_recall, macro_f1, _ = precision_recall_fscore_support(y_true, y_pred, average='macro')
    accuracy = 100 * (y_true == y_pred).sum() / len(y_true)
    
    # Per-Class Metrics
    p_class, r_class, f1_class, _ = precision_recall_fscore_support(y_true, y_pred, labels=list(range(args.num_classes)))
    cm = confusion_matrix(y_true, y_pred, labels=list(range(args.num_classes)))
    
    per_class_metrics = {}
    for i in range(args.num_classes):
        class_total = np.sum(cm[i, :])
        class_acc = (100 * cm[i, i] / class_total) if class_total > 0 else 0.0
        per_class_metrics[str(i)] = {
            "accuracy": class_acc,
            "precision": float(p_class[i]),
            "recall": float(r_class[i]),
            "f1_score": float(f1_class[i])
        }

    now = datetime.now()
    results = {
        "timestamp": now.strftime("%Y-%m-%d %H:%M:%S"),
        "total_samples": len(y_true),
        "global_metrics": {
            "accuracy": accuracy,
            "weighted_precision": float(precision),
            "weighted_recall": float(recall),
            "weighted_f1": float(f1),
            "macro_precision": float(macro_precision),
            "macro_recall": float(macro_recall),
            "macro_f1": float(macro_f1)
        },
        "per_class_metrics": per_class_metrics,
        "model_used": os.path.abspath(final_model_path),
        "data_used": os.path.abspath(val_folder)
    }
    
    # SAVE AND PLOT
    timestamp_str = now.strftime("%Y%m%d_%H%M%S")
    target_output_dir = os.path.join(args.output_dir, f"run_{timestamp_str}")
    os.makedirs(target_output_dir, exist_ok=True)
    
    classes = [str(i) for i in range(args.num_classes)]
    save_detailed_report(results, target_output_dir)
    plot_confusion_matrix(y_true, y_pred, classes, os.path.join(target_output_dir, 'confusion_matrix.png'))
    plot_per_class_metrics(results, classes, os.path.join(target_output_dir, 'per_class_performance.png'))
    
    # Update latest folder
    latest_dir = os.path.join(args.output_dir, 'latest')
    if os.path.exists(latest_dir): shutil.rmtree(latest_dir)
    shutil.copytree(target_output_dir, latest_dir)
    
    logger.info(f"Evaluation complete. Reports generated in '{target_output_dir}'")
    logger.info(f"Summary: Accuracy={accuracy:.2f}%, F1={macro_f1:.4f}")