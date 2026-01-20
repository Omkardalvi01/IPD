import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import Dataset, DataLoader, Subset
from torchvision import transforms
from PIL import Image
from typing import Optional, Dict, List, Any, Union
from sklearn.model_selection import train_test_split
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
from sklearn.metrics import confusion_matrix

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# 1. SETUP DEVICE
device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
logger.info(f"--- Using device: {device} ---")

def natural_sort_key(s):
    """Sort strings with numbers in a way that humans expect (1, 2, 10 instead of 1, 10, 2)"""
    import re
    return [int(text) if text.isdigit() else text.lower()
            for text in re.split('([0-9]+)', str(s))]

# 2. CUSTOM DATASET
class MNISTFolderDataset(Dataset):
    def __init__(self, folder_path, transform=None):
        self.folder_path = folder_path
        self.transform = transform
        # Get all valid images
        self.file_names = sorted([f for f in os.listdir(folder_path) if f.lower().endswith(('.png', '.jpg', '.jpeg'))])

    def __len__(self):
        return len(self.file_names)

    def __getitem__(self, idx):
        img_name = self.file_names[idx]
        img_path = os.path.join(self.folder_path, img_name)
        image = Image.open(img_path).convert('L') 
        
        # Label extraction: 'mnist_train_00004_label_9.jpg' -> 9
        try:
            # Extract the label from 'label_X' pattern
            label = int(img_name.split('_label_')[1].split('.')[0])
        except Exception as e:
            print(f"Warning: Could not extract label from {img_name}, defaulting to 0. Error: {e}")
            label = 0
            
        if self.transform:
            image = self.transform(image)
        return image, label

# 3. MEDIUM CNN MODEL
class MediumCNN(nn.Module):
    def __init__(self):
        super(MediumCNN, self).__init__()
        self.features = nn.Sequential(
            nn.Conv2d(1, 32, kernel_size=3, padding=1),
            nn.ReLU(),
            nn.MaxPool2d(2),
            nn.Conv2d(32, 64, kernel_size=3, padding=1),
            nn.ReLU(),
            nn.MaxPool2d(2)
        )
        self.classifier = nn.Sequential(
            nn.Flatten(),
            nn.Linear(64 * 7 * 7, 128),
            nn.ReLU(),
            nn.Dropout(0.5),
            nn.Linear(128, 10)
        )

    def forward(self, x):
        x = self.features(x)
        x = self.classifier(x)
        return x

def plot_confusion_matrix(y_true, y_pred, classes, output_path):
    """Plot and save confusion matrix using seaborn"""
    plt.figure(figsize=(10, 8))
    cm = confusion_matrix(y_true, y_pred)
    # Normalize
    cm_norm = cm.astype('float') / cm.sum(axis=1)[:, np.newaxis]
    
    sns.heatmap(cm_norm, annot=True, fmt='.2f', cmap='Blues',
                xticklabels=classes, yticklabels=classes)
    plt.title('Confusion Matrix (Normalized)')
    plt.ylabel('True label')
    plt.xlabel('Predicted label')
    plt.tight_layout()
    plt.savefig(output_path)
    plt.close()

def find_latest_model(model_dir: str = "./global_models") -> Optional[str]:
    """Find the latest model checkpoint"""
    if not os.path.exists(model_dir):
        # Fallback to current dir if not found (common in dev)
        if os.path.exists("../global_models"):
            model_dir = "../global_models"
        else:
            return None
    
    model_files = []
    for ext in ['.pth', '.pt']:
        model_files.extend(glob.glob(os.path.join(model_dir, f'*{ext}')))
    
    if not model_files:
        return None
    
    model_files.sort(key=os.path.getmtime, reverse=True)
    return model_files[0]

def save_detailed_report(results, output_dir):
    """Save JSON and TXT reports"""
    # Save JSON
    with open(os.path.join(output_dir, 'evaluation_results.json'), 'w') as f:
        json.dump(results, f, indent=4)
    
    # Save Summary TXT
    with open(os.path.join(output_dir, 'evaluation_summary.txt'), 'w') as f:
        f.write("="*50 + "\n")
        f.write("DETAILED EVALUATION SUMMARY\n")
        f.write("="*50 + "\n")
        f.write(f"Timestamp: {results['timestamp']}\n")
        f.write(f"Accuracy:  {results['accuracy']:.2f}%\n")
        f.write(f"Total Samples: {results['total_samples']}\n")
        f.write("-" * 30 + "\n")
        f.write("PER-CLASS ACCURACY:\n")
        for cls_name, acc in results['per_class_accuracy'].items():
            f.write(f"Class {cls_name}: {acc:.2f}%\n")
        f.write("="*50 + "\n")

# 4. MAIN EXECUTION
if __name__ == "__main__":
    # Set seeds for reproducibility
    torch.manual_seed(42)
    np.random.seed(42)
    
    transform = transforms.Compose([
        transforms.Resize((28, 28)),
        transforms.ToTensor(),
        transforms.Normalize((0.1307,), (0.3081,))
    ])

    train_folder = 'mnist_600/train' # Main training data
    val_folder = 'mnist_600/test'   # Validation data
    
    # Check if folders exist and adjust for running location
    potential_base = os.path.dirname(os.path.abspath(__file__))
    
    def resolve_path(p):
        if os.path.exists(p):
            return p
        # Check relative to script
        script_relative = os.path.join(potential_base, p)
        if os.path.exists(script_relative):
            return script_relative
        # Check parent (if run from inside main)
        parent_relative = os.path.join(os.path.dirname(potential_base), p)
        if os.path.exists(parent_relative):
            return parent_relative
        return p

    train_folder = resolve_path(train_folder)
    val_folder = resolve_path(val_folder)
    
    if not os.path.exists(train_folder):
        print(f"Error: Training folder '{train_folder}' not found.")
        exit()
             
    if not os.path.exists(val_folder):
        print(f"Error: Validation folder '{val_folder}' not found.")
        exit()
    
    # Load training data and split into train/test
    train_dataset = MNISTFolderDataset(train_folder, transform=transform)
    
    # Split training data into train and test (80/20)
    indices = list(range(len(train_dataset)))
    train_idx, test_idx = train_test_split(indices, test_size=0.2, random_state=42, shuffle=True)
    
    # Load validation data from separate folder
    val_dataset = MNISTFolderDataset(val_folder, transform=transform)
    
    print(f"Training set size: {len(train_idx)} (from {train_folder})")
    print(f"Test set size: {len(test_idx)} (from {train_folder})")
    print(f"Validation set size: {len(val_dataset)} (from {val_folder})")
    
    # Check label distributions
    train_labels = [train_dataset[i][1] for i in train_idx]
    test_labels = [train_dataset[i][1] for i in test_idx]
    val_labels = [val_dataset[i][1] for i in range(len(val_dataset))]
    
    print(f"\nLabel distributions:")
    print(f"Train: {np.bincount(train_labels)}")
    print(f"Test:  {np.bincount(test_labels)}")
    print(f"Val:   {np.bincount(val_labels)}")
    print()

    # Create data loaders
    train_loader = DataLoader(Subset(train_dataset, train_idx), batch_size=64, shuffle=True)
    test_loader = DataLoader(Subset(train_dataset, test_idx), batch_size=64, shuffle=False)
    val_loader = DataLoader(val_dataset, batch_size=64, shuffle=False)

    model = MediumCNN().to(device)
    criterion = nn.CrossEntropyLoss()
    optimizer = optim.Adam(model.parameters(), lr=0.001)

    # TRAINING PHASE
    print("=" * 50)
    print("TRAINING PHASE")
    print("=" * 50)
    for epoch in range(5):
        model.train()
        running_loss = 0.0
        train_correct = 0
        train_total = 0
        
        for images, labels in train_loader:
            images, labels = images.to(device), labels.to(device)
            
            optimizer.zero_grad()
            outputs = model(images)
            loss = criterion(outputs, labels)
            loss.backward()
            optimizer.step()
            running_loss += loss.item()
            
            # Track training accuracy
            _, predicted = torch.max(outputs.data, 1)
            train_total += labels.size(0)
            train_correct += (predicted == labels).sum().item()
        
        # Test set evaluation during training
        model.eval()
        test_correct = 0
        test_total = 0
        with torch.no_grad():
            for images, labels in test_loader:
                images, labels = images.to(device), labels.to(device)
                outputs = model(images)
                _, predicted = torch.max(outputs.data, 1)
                test_total += labels.size(0)
                test_correct += (predicted == labels).sum().item()
        
        avg_loss = running_loss / len(train_loader)
        train_accuracy = 100 * train_correct / train_total
        test_accuracy = 100 * test_correct / test_total
        
        print(f"Epoch {epoch+1} | Loss: {avg_loss:.4f} | Train Acc: {train_accuracy:.2f}% | Test Acc: {test_accuracy:.2f}%")
    
    # VALIDATION PHASE
    print("\n" + "=" * 50)
    print("VALIDATION PHASE (on separate folder)")
    print("=" * 50)
    
    parser = argparse.ArgumentParser()
    parser.add_argument('--test-dir', type=str, default=val_folder)
    parser.add_argument('--model-path', type=str, default=None)
    parser.add_argument('--output-dir', type=str, default='./evaluation_results')
    args, unknown = parser.parse_known_args()

    output_dir = args.output_dir
    os.makedirs(output_dir, exist_ok=True)
    
    # If model_path is provided, load it
    test_model = model
    final_model_path = args.model_path
    if not final_model_path:
        final_model_path = find_latest_model()
        
    if final_model_path and os.path.exists(final_model_path):
        logger.info(f"Loading weights from {final_model_path}")
        checkpoint = torch.load(final_model_path, map_location=device)
        if isinstance(checkpoint, dict) and 'model_state_dict' in checkpoint:
            test_model.load_state_dict(checkpoint['model_state_dict'])
        elif isinstance(checkpoint, dict) and 'state_dict' in checkpoint:
            test_model.load_state_dict(checkpoint['state_dict'])
        else:
            test_model.load_state_dict(checkpoint)
    elif args.model_path:
        logger.error(f"Model path {args.model_path} not found.")
        exit(1)
    else:
        logger.warning("No pre-trained model found. Using the model from current training phase.")
    
    test_model.eval()
    val_correct = 0
    val_total = 0
    all_preds = []
    all_targets = []
    
    with torch.no_grad():
        for images, labels in val_loader:
            images, labels = images.to(device), labels.to(device)
            outputs = test_model(images)
            _, predicted = torch.max(outputs.data, 1)
            val_total += labels.size(0)
            val_correct += (predicted == labels).sum().item()
            all_preds.extend(predicted.cpu().numpy())
            all_targets.extend(labels.cpu().numpy())
    
    val_accuracy = 100 * val_correct / val_total
    print(f"Validation Accuracy on {val_folder}: {val_accuracy:.2f}%")
    print(f"Correctly classified: {val_correct}/{val_total}")
    
    # Generate Results Dict
    classes = [str(i) for i in range(10)]
    per_class_acc = {}
    cm = confusion_matrix(all_targets, all_preds)
    for i in range(10):
        class_total = np.sum(cm[i, :])
        if class_total > 0:
            per_class_acc[str(i)] = 100 * cm[i, i] / class_total
        else:
            per_class_acc[str(i)] = 0.0

    results = {
        "timestamp": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
        "accuracy": val_accuracy,
        "total_samples": val_total,
        "per_class_accuracy": per_class_acc
    }
    
    # Save Reports
    save_detailed_report(results, output_dir)
    
    # Plot CM
    plot_confusion_matrix(all_targets, all_preds, classes, os.path.join(output_dir, 'confusion_matrix.png'))
    
    # Latest Results feature
    latest_dir = os.path.join(os.path.dirname(output_dir), 'latest_results')
    if os.path.exists(latest_dir):
        shutil.rmtree(latest_dir)
    shutil.copytree(output_dir, latest_dir)
    logger.info(f"Latest results updated at: {os.path.abspath(latest_dir)}")
    
    print("=" * 50)