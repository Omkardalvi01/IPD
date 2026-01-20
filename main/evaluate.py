#!/usr/bin/env python3
"""
Global Model Evaluation Script - Dataset Agnostic Version
Evaluates the latest global model with test data
"""
import argparse
import torch
import torch.nn as nn
import numpy as np
import json
import os
import sys
import glob
import shutil
import logging
from datetime import datetime
from pathlib import Path
from typing import Dict, Tuple, Optional, Any, List, Union
from collections import defaultdict
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import seaborn as sns
from sklearn.metrics import confusion_matrix
from PIL import Image
import torchvision
import torchvision.transforms as transforms
from torch.utils.data import DataLoader, Dataset, TensorDataset

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('global_evaluation.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

sys.path.append('../edge')
sys.path.append('../aggregator')

class DatasetConfig:
    """Configuration class for dataset-specific parameters"""
    
    def __init__(self, config_path: Optional[str] = None):
        self.num_classes = 10
        self.input_channels = 3
        self.input_size = (224, 224)
        self.normalization_mean = [0.485, 0.456, 0.406]
        self.normalization_std = [0.229, 0.224, 0.225]
        self.class_names = []
        self.dataset_name = "unknown"
        
        if config_path and os.path.exists(config_path):
            self.load_from_file(config_path)
    
    def load_from_file(self, config_path: str):
        """Load configuration from JSON file"""
        try:
            with open(config_path, 'r') as f:
                config = json.load(f)
            
            self.num_classes = config.get('num_classes', self.num_classes)
            self.input_channels = config.get('input_channels', self.input_channels)
            self.input_size = tuple(config.get('input_size', self.input_size))
            self.normalization_mean = config.get('normalization_mean', self.normalization_mean)
            self.normalization_std = config.get('normalization_std', self.normalization_std)
            self.class_names = config.get('class_names', self.class_names)
            self.dataset_name = config.get('dataset_name', self.dataset_name)
            
            logger.info(f"Loaded dataset config from {config_path}")
            logger.info(f"Dataset: {self.dataset_name}, Classes: {self.num_classes}")
        except Exception as e:
            logger.warning(f"Failed to load config from {config_path}: {e}")
    
    def save_to_file(self, config_path: str):
        """Save configuration to JSON file"""
        config = {
            'num_classes': self.num_classes,
            'input_channels': self.input_channels,
            'input_size': list(self.input_size),
            'normalization_mean': self.normalization_mean,
            'normalization_std': self.normalization_std,
            'class_names': self.class_names,
            'dataset_name': self.dataset_name
        }
        
        with open(config_path, 'w') as f:
            json.dump(config, f, indent=2)
        
        logger.info(f"Saved dataset config to {config_path}")
    
    def auto_detect_from_data(self, data_dir: str) -> bool:
        """Auto-detect dataset configuration from data directory"""
        try:
            data_path = Path(data_dir)
            
            # Check for metadata
            metadata_dir = data_path / "_metadata"
            if metadata_dir.exists():
                labels_file = metadata_dir / "labels.json"
                if labels_file.exists():
                    with open(labels_file, 'r') as f:
                        labels = json.load(f)
                    
                    unique_labels = sorted(set(str(v) for v in labels.values()))
                    self.num_classes = len(unique_labels)
                    self.class_names = unique_labels
                    logger.info(f"Detected {self.num_classes} classes from labels.json")
            
            if not self.class_names:
                self._detect_classes_from_filenames(data_dir)
            
            self._detect_image_properties(data_dir)
            self.dataset_name = data_path.name
            
            logger.info(f"Auto-detected dataset config: {self.num_classes} classes, "
                       f"{self.input_channels} channels, size {self.input_size}")
            
            return True
            
        except Exception as e:
            logger.error(f"Failed to auto-detect dataset config: {e}")
            return False
    
    def _detect_classes_from_filenames(self, data_dir: str):
        """Detect classes from filename patterns - FIXED to match training logic"""
        classes = set()
        image_extensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp']
        
        for file in Path(data_dir).iterdir():
            if file.is_file() and file.suffix.lower() in image_extensions:
                filename = file.name
                
                # FIXED: Prioritize label_ pattern (MNIST format) - same as training code
                if 'label_' in filename:
                    # MNIST format: extract the label after 'label_'
                    label = filename.split('label_')[-1].split('.')[0]
                    classes.add(label)
                elif '#' in filename:
                    # Original format: class#image.jpg
                    class_name = filename.split('#')[0]
                    classes.add(class_name)
                # REMOVED: The buggy '_' fallback that was extracting wrong parts
        
        if classes:
            self.class_names = sorted(list(classes))
            self.num_classes = len(self.class_names)
            logger.info(f"Detected {self.num_classes} classes from filenames: {self.class_names[:5]}...")
    
    def _detect_image_properties(self, data_dir: str):
        """Detect image properties from sample images"""
        image_extensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp']
        
        for file in Path(data_dir).iterdir():
            if file.is_file() and file.suffix.lower() in image_extensions:
                try:
                    img = Image.open(file)
                    
                    if img.mode == 'L' or img.mode == '1':
                        detected_channels = 1
                    elif img.mode == 'RGB':
                        detected_channels = 3
                    elif img.mode == 'RGBA':
                        detected_channels = 4
                    else:
                        detected_channels = 3
                    
                    # Always use 3 channels for model compatibility
                    self.input_channels = 3
                    self.normalization_mean = [0.485, 0.456, 0.406]
                    self.normalization_std = [0.229, 0.224, 0.225]
                    
                    width, height = img.size
                    if width == height and width in [28, 32, 64, 128, 224, 256]:
                        self.input_size = (width, height)
                    
                    logger.info(f"Detected image properties: {detected_channels} native channels, "
                               f"mode: {img.mode}, original size: {img.size}")
                    logger.info(f"Using 3 channels for model compatibility")
                    break
                    
                except Exception as e:
                    logger.warning(f"Failed to read image {file}: {e}")
                    continue

class ImageDataset(Dataset):
    """Custom dataset for loading images from directory"""
    
    def __init__(self, data_dir: str, transform=None, config: Optional[DatasetConfig] = None):
        self.data_dir = Path(data_dir)
        self.transform = transform
        self.config = config or DatasetConfig()
        
        self.image_extensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.webp']
        self.samples = []
        
        self._load_samples()
        
        logger.info(f"Loaded {len(self.samples)} samples from {data_dir}")
    
    def _load_samples(self):
        """Load image files and labels - FIXED to ensure class_to_idx is never empty"""
        labels_map = {}
        metadata_dir = self.data_dir / "_metadata"
        if metadata_dir.exists():
            labels_file = metadata_dir / "labels.json"
            if labels_file.exists():
                with open(labels_file, 'r') as f:
                    labels_map = json.load(f)
        
        # FIXED: Ensure class_to_idx is NEVER empty
        if self.config.class_names:
            class_to_idx = {cls_name: idx for idx, cls_name in enumerate(self.config.class_names)}
        else:
            # Auto-detect classes from filenames if not provided
            logger.info("No class_names in config, auto-detecting from filenames...")
            classes = set()
            for file in self.data_dir.iterdir():
                if file.is_file() and file.suffix.lower() in self.image_extensions:
                    if 'label_' in file.name:
                        label = file.name.split('label_')[-1].split('.')[0]
                        classes.add(label)
                    elif '#' in file.name:
                        class_name = file.name.split('#')[0]
                        classes.add(class_name)
            
            if classes:
                self.config.class_names = sorted(list(classes))
                class_to_idx = {cls_name: idx for idx, cls_name in enumerate(self.config.class_names)}
                logger.info(f"Auto-detected {len(classes)} classes: {sorted(classes)}")
            else:
                class_to_idx = {}
                logger.warning("Could not auto-detect classes, using empty mapping")
        
        for file in self.data_dir.iterdir():
            if file.is_file() and file.suffix.lower() in self.image_extensions:
                filename = file.name
                
                label = None
                if filename in labels_map:
                    label_str = str(labels_map[filename])
                    label = class_to_idx.get(label_str, int(label_str) if label_str.isdigit() else None)
                else:
                    label = self._extract_label_from_filename(filename, class_to_idx)
                
                if label is not None:
                    self.samples.append((str(file), label))
                else:
                    logger.warning(f"Skipping file with no valid label: {filename}")
    
    def _extract_label_from_filename(self, filename: str, class_to_idx: Dict[str, int]) -> Optional[int]:
        """Extract label from filename - FIXED to match training logic exactly"""
        # FIXED: Use EXACT same logic as model_utils.py CustomImageDataset
        if 'label_' in filename:
            # MNIST format: extract the label after 'label_'
            class_name = filename.split('label_')[-1].split('.')[0]
            # Use class_to_idx mapping if available, otherwise convert to int
            if class_name in class_to_idx:
                return class_to_idx[class_name]
            elif class_name.isdigit():
                return int(class_name)
            else:
                logger.warning(f"Could not extract numeric label from {filename}")
                return None
        
        if '#' in filename:
            # Original format: class#image.jpg
            class_name = filename.split('#')[0]
            return class_to_idx.get(class_name, None)
        
        # FIXED: Removed buggy fallbacks - return None instead of random hash
        logger.warning(f"Could not extract label from filename: {filename}")
        return None
    
    def __len__(self):
        return len(self.samples)
    
    def __getitem__(self, idx):
        img_path, label = self.samples[idx]
        
        try:
            image = Image.open(img_path)
            
            # Always convert to RGB for model compatibility
            image = image.convert('RGB')
            
            if self.transform:
                image = self.transform(image)
            
            return image, label
            
        except Exception as e:
            logger.warning(f"Error loading image {img_path}: {e}")
            return torch.zeros(3, *self.config.input_size), label

class GlobalModelEvaluator:
    """Evaluates global model performance - dataset agnostic"""
    
    def __init__(
        self, 
        controller_data_dir: str = "./test_data",
        results_dir: str = "./evaluation_results",
        config_path: Optional[str] = None
    ):
        self.controller_data_dir = controller_data_dir
        self.results_dir = results_dir
        self.device = 'cuda' if torch.cuda.is_available() else 'cpu'
        
        os.makedirs(results_dir, exist_ok=True)
        os.makedirs(os.path.join(results_dir, "plots"), exist_ok=True)
        
        self.config = DatasetConfig(config_path)
        
        if not config_path and os.path.exists(controller_data_dir):
            logger.info("Auto-detecting dataset configuration...")
            self.config.auto_detect_from_data(controller_data_dir)
            
            config_save_path = os.path.join(results_dir, "dataset_config.json")
            self.config.save_to_file(config_save_path)
        
        logger.info(f"Global Model Evaluator initialized")
        logger.info(f"Controller data: {controller_data_dir}")
        logger.info(f"Results directory: {results_dir}")
        logger.info(f"Device: {self.device}")
        logger.info(f"Dataset: {self.config.dataset_name}, Classes: {self.config.num_classes}")
    
    def get_data_transforms(self, augment: bool = False):
        """Get data transforms based on configuration"""
        transform_list = [
            transforms.Resize(self.config.input_size),
        ]
        
        if augment:
            transform_list.extend([
                transforms.RandomHorizontalFlip(),
                transforms.RandomRotation(10),
            ])
        
        transform_list.append(transforms.ToTensor())
        
        # Always use standard ImageNet normalization for RGB
        transform_list.append(
            transforms.Normalize(
                mean=[0.485, 0.456, 0.406],
                std=[0.229, 0.224, 0.225]
            )
        )
        
        return transforms.Compose(transform_list)
    
    def load_test_data(self) -> Tuple[Optional[DataLoader], int]:
        """Load controller's test data - dataset agnostic"""
        try:
            if not os.path.exists(self.controller_data_dir):
                logger.warning(f"Controller data directory not found: {self.controller_data_dir}")
                logger.info("Using synthetic test data instead")
                return self._create_synthetic_test_data()
            
            image_files = []
            for ext in ['.jpg', '.jpeg', '.png', '.bmp', '.tiff', '.gif', '.webp']:
                image_files.extend(list(Path(self.controller_data_dir).glob(f"*{ext}")))
            
            if not image_files:
                logger.warning("No image files found in controller data directory")
                logger.info("Using synthetic test data instead")
                return self._create_synthetic_test_data()
            
            logger.info(f"Found {len(image_files)} image files in controller data")
            
            transform = self.get_data_transforms(augment=False)
            dataset = ImageDataset(
                self.controller_data_dir,
                transform=transform,
                config=self.config
            )
            
            test_loader = DataLoader(
                dataset,
                batch_size=64,
                shuffle=False,
                num_workers=2,
                pin_memory=True if self.device == 'cuda' else False
            )
            
            logger.info(f"Test data loaded: {len(dataset)} samples")
            return test_loader, len(dataset)
            
        except Exception as e:
            logger.error(f"Failed to load test data: {e}", exc_info=True)
            logger.info("Using synthetic test data instead")
            return self._create_synthetic_test_data()
    
    def _create_synthetic_test_data(self) -> Tuple[DataLoader, int]:
        """Create synthetic test data for evaluation"""
        logger.info("Creating synthetic test data...")
        num_samples = 1000
        
        images_tensor = torch.randn(
            num_samples,
            3,
            *self.config.input_size
        )
        labels_tensor = torch.randint(0, self.config.num_classes, (num_samples,))
        
        dataset = TensorDataset(images_tensor, labels_tensor)
        test_loader = DataLoader(dataset, batch_size=64, shuffle=False)
        
        logger.info(f"Synthetic test data created: {num_samples} samples")
        return test_loader, num_samples
    
    def evaluate_model(
        self,
        model: nn.Module,
        test_loader: DataLoader,
        model_name: str = "Model"
    ) -> Dict[str, Any]:
        """Evaluate a model on test data"""
        model.eval()
        model.to(self.device)
        
        test_loss = 0.0
        correct = 0
        total = 0
        criterion = nn.CrossEntropyLoss()
        
        class_correct = [0] * self.config.num_classes
        class_total = [0] * self.config.num_classes
        
        all_preds = []
        all_targets = []
        
        with torch.no_grad():
            for batch_idx, (data, target) in enumerate(test_loader):
                try:
                    data, target = data.to(self.device), target.to(self.device)
                    output = model(data)
                    loss = criterion(output, target)
                    
                    test_loss += loss.item() * data.size(0)
                    _, predicted = output.max(1)
                    total += target.size(0)
                    correct += predicted.eq(target).sum().item()
                    
                    all_preds.extend(predicted.cpu().numpy())
                    all_targets.extend(target.cpu().numpy())
                    
                    for i in range(len(target)):
                        label = target[i].item()
                        if label < self.config.num_classes:
                            class_total[label] += 1
                            if predicted[i] == label:
                                class_correct[label] += 1
                    
                    if (batch_idx + 1) % 10 == 0:
                        logger.info(f"  Processed {batch_idx + 1}/{len(test_loader)} batches | "
                                  f"Acc: {100. * correct / total:.2f}%")
                        
                except Exception as e:
                    logger.error(f"Error evaluating batch {batch_idx}: {e}")
                    continue
        
        accuracy = correct / total if total > 0 else 0
        avg_loss = test_loss / total if total > 0 else float('inf')
        
        per_class_accuracy = {}
        for i in range(self.config.num_classes):
            if class_total[i] > 0:
                class_acc = class_correct[i] / class_total[i]
                class_name = self.config.class_names[i] if i < len(self.config.class_names) else f"Class_{i}"
                per_class_accuracy[class_name] = float(class_acc)
        
        conf_matrix = None
        if all_preds and all_targets:
            try:
                conf_matrix = confusion_matrix(all_targets, all_preds)
                self._plot_confusion_matrix(
                    conf_matrix, 
                    class_names=self.config.class_names if self.config.class_names else 
                               [f'Class {i}' for i in range(self.config.num_classes)],
                    output_path=os.path.join(self.results_dir, 'confusion_matrix.png')
                )
            except Exception as e:
                logger.warning(f"Could not generate confusion matrix: {e}")
        
        results = {
            'model_name': model_name,
            'accuracy': float(accuracy),
            'loss': float(avg_loss),
            'correct_predictions': int(correct),
            'total_samples': int(total),
            'per_class_accuracy': per_class_accuracy,
            'dataset_config': {
                'num_classes': self.config.num_classes,
                'input_channels': self.config.input_channels,
                'input_size': list(self.config.input_size),
                'dataset_name': self.config.dataset_name,
                'class_names': self.config.class_names
            },
            'timestamp': datetime.now().isoformat(),
            'device': str(self.device),
            'confusion_matrix': conf_matrix.tolist() if conf_matrix is not None else None
        }
        
        logger.info(f"{model_name} Results:")
        logger.info(f"  Accuracy: {accuracy:.4f} ({correct}/{total})")
        logger.info(f"  Loss: {avg_loss:.4f}")
        
        if per_class_accuracy:
            logger.info(f"  Per-class accuracy (top 5):")
            for i, (class_name, acc) in enumerate(list(per_class_accuracy.items())[:5]):
                logger.info(f"    {class_name}: {acc:.4f}")
        
        return results
    
    def load_model_from_checkpoint(
        self,
        checkpoint_path: str,
        model_class: Optional[type] = None
    ) -> Optional[nn.Module]:
        """Load model from checkpoint - flexible for different architectures"""
        try:
            logger.info(f"Loading model from: {checkpoint_path}")
            
            if not os.path.exists(checkpoint_path):
                logger.error(f"Checkpoint file not found: {checkpoint_path}")
                return None
                
            try:
                checkpoint = torch.load(checkpoint_path, map_location=self.device)
            except Exception as e:
                logger.error(f"Failed to load checkpoint: {e}")
                return None
            
            if isinstance(checkpoint, dict):
                logger.info(f"Checkpoint keys: {list(checkpoint.keys())[:10]}...")
            
            if isinstance(checkpoint, nn.Module):
                logger.info("Loaded complete model from checkpoint")
                return checkpoint
            
            if model_class is not None:
                try:
                    model = model_class(
                        num_classes=self.config.num_classes,
                        pretrained=False
                    )
                    
                    state_dict = None
                    if isinstance(checkpoint, dict):
                        for key in ['model_state_dict', 'state_dict', 'model']:
                            if key in checkpoint:
                                state_dict = checkpoint[key]
                                logger.info(f"Found state dict with key: {key}")
                                break
                    
                    if state_dict is None and not isinstance(checkpoint, dict):
                        state_dict = checkpoint
                    
                    if state_dict is not None:
                        if all(k.startswith('module.') for k in state_dict.keys()):
                            state_dict = {k.replace('module.', ''): v for k, v in state_dict.items()}
                        
                        model.load_state_dict(state_dict, strict=False)
                        logger.info(f"Successfully loaded weights into {model_class.__name__}")
                        return model
                    
                except Exception as e:
                    logger.error(f"Error loading model with provided class: {e}")
            
            logger.warning("Could not determine model architecture from checkpoint. Attempting to infer...")
            
            try:
                if isinstance(checkpoint, dict):
                    state_dict = checkpoint.get('state_dict', checkpoint)
                else:
                    state_dict = checkpoint
                
                layer_keys = list(state_dict.keys())
                
                # Try MobileNetV2 first
                if any('features' in k and 'conv' in k for k in layer_keys):
                    logger.info("Detected MobileNetV2-like architecture")
                    import torchvision.models as models
                    model = models.mobilenet_v2(pretrained=False)
                    num_ftrs = model.classifier[1].in_features
                    model.classifier[1] = nn.Linear(num_ftrs, self.config.num_classes)
                    
                    try:
                        model.load_state_dict(state_dict, strict=False)
                        logger.info("Successfully loaded weights into MobileNetV2")
                        return model
                    except Exception as e:
                        logger.warning(f"Failed to load into MobileNetV2: {e}")
                
                # Try ResNet
                elif any('layer' in k and 'conv' in k for k in layer_keys):
                    logger.info("Detected ResNet-like architecture")
                    import torchvision.models as models
                    model = models.resnet18(pretrained=False)
                    num_ftrs = model.fc.in_features
                    model.fc = nn.Linear(num_ftrs, self.config.num_classes)
                    
                    try:
                        model.load_state_dict(state_dict, strict=False)
                        logger.info("Successfully loaded weights into ResNet18")
                        return model
                    except Exception as e:
                        logger.warning(f"Failed to load into ResNet: {e}")
                
                logger.info("Using default CNN architecture")
                class SimpleCNN(nn.Module):
                    def __init__(self, num_classes=10):
                        super(SimpleCNN, self).__init__()
                        self.features = nn.Sequential(
                            nn.Conv2d(3, 32, kernel_size=3, padding=1),
                            nn.ReLU(inplace=True),
                            nn.MaxPool2d(kernel_size=2, stride=2),
                            nn.Conv2d(32, 64, kernel_size=3, padding=1),
                            nn.ReLU(inplace=True),
                            nn.MaxPool2d(kernel_size=2, stride=2),
                        )
                        self.classifier = nn.Sequential(
                            nn.Linear(64 * 56 * 56, 512),
                            nn.ReLU(inplace=True),
                            nn.Dropout(),
                            nn.Linear(512, num_classes)
                        )
                    
                    def forward(self, x):
                        x = self.features(x)
                        x = x.view(x.size(0), -1)
                        x = self.classifier(x)
                        return x
                
                model = SimpleCNN(num_classes=self.config.num_classes)
                
                try:
                    model.load_state_dict(state_dict, strict=False)
                    logger.info("Successfully loaded weights into SimpleCNN")
                    return model
                except Exception as e:
                    logger.error(f"Failed to load into SimpleCNN: {e}")
                    return None
                    
            except Exception as e:
                logger.error(f"Failed to infer model architecture: {e}")
                return None
                
        except Exception as e:
            logger.error(f"Error loading model from checkpoint: {e}", exc_info=True)
            return None
    
    def _plot_confusion_matrix(self, cm, class_names, output_path, title='Confusion Matrix'):
        """Plot and save confusion matrix"""
        plt.figure(figsize=(12, 10))
        
        cm_norm = cm.astype('float') / cm.sum(axis=1)[:, np.newaxis]
        
        sns.heatmap(
            cm_norm, 
            annot=True, 
            fmt='.2f', 
            cmap='Blues',
            xticklabels=class_names,
            yticklabels=class_names,
            cbar=True
        )
        
        plt.title(title)
        plt.ylabel('True Label')
        plt.xlabel('Predicted Label')
        plt.xticks(rotation=45, ha='right')
        plt.yticks(rotation=0)
        plt.tight_layout()
        
        plt.savefig(output_path, dpi=300, bbox_inches='tight')
        plt.close()
        logger.info(f"Confusion matrix saved to: {output_path}")

    def _plot_class_distribution(self, data_loader, output_path):
        """Plot the distribution of classes in the test set"""
        try:
            class_counts = defaultdict(int)
            dataset = data_loader.dataset
            
            # Determine how to iterate
            if hasattr(dataset, 'targets'):
                # TensorDataset or torchvision datasets often have .targets
                for label in dataset.targets:
                    if isinstance(label, torch.Tensor):
                        label = label.item()
                    class_counts[label] += 1
            elif hasattr(dataset, 'samples'):
                # ImageFolder or our ImageDataset has .samples [(path, label), ...]
                for _, label in dataset.samples:
                    class_counts[label] += 1
            else:
                # Fallback: iterate (slow)
                logger.info("Iterating dataset to count classes (no .targets or .samples found)...")
                for _, label in data_loader:
                     for l in label:
                         class_counts[l.item()] += 1
            
            # Prepare plotting data
            labels = sorted(class_counts.keys())
            counts = [class_counts[l] for l in labels]
            
            class_names = self.config.class_names
            x_labels = [class_names[i] if i < len(class_names) else f"Class {i}" for i in labels]
            
            plt.figure(figsize=(12, 6))
            bars = plt.bar(x_labels, counts, color='skyblue')
            
            plt.title("Test Data Class Distribution", fontsize=14, fontweight='bold')
            plt.xlabel("Classes")
            plt.ylabel("Number of Samples")
            plt.xticks(rotation=45, ha='right')
            
            # Add count labels
            for bar in bars:
                height = bar.get_height()
                plt.text(bar.get_x() + bar.get_width()/2., height,
                        f'{int(height)}',
                        ha='center', va='bottom')
            
            plt.tight_layout()
            plt.savefig(output_path, dpi=300)
            plt.close()
            logger.info(f"Class distribution plot saved to: {output_path}")
            
        except Exception as e:
            logger.warning(f"Failed to plot class distribution: {e}")
    
    def generate_comparison_plots(self, results: Dict[str, Any]):
        """Generate comparison plots for evaluation results"""
        try:
            if not results.get('federated_model'):
                logger.info("No federated model results to plot")
                return
            
            fig = plt.figure(figsize=(16, 10))
            
            has_initial = results.get('initial_model') is not None
            
            if not has_initial:
                gs = fig.add_gridspec(2, 2, hspace=0.4, wspace=0.3)
                
                ax1 = fig.add_subplot(gs[0, 0])
                models = [results['federated_model']['model_name']]
                accuracies = [results['federated_model']['accuracy']]
                
                bars1 = ax1.bar(models, accuracies, color='lightblue')
                ax1.set_title('Model Accuracy', fontsize=12, fontweight='bold')
                ax1.set_ylabel('Accuracy')
                ax1.set_ylim(0, 1)
                ax1.grid(axis='y', alpha=0.3)
                
                for bar in bars1:
                    height = bar.get_height()
                    ax1.text(bar.get_x() + bar.get_width()/2., height + 0.01,
                            f'{height:.4f}', ha='center', va='bottom', fontweight='bold')
                
                ax2 = fig.add_subplot(gs[0, 1])
                losses = [results['federated_model']['loss']]
                
                bars2 = ax2.bar(models, losses, color='lightgreen')
                ax2.set_title('Model Loss', fontsize=12, fontweight='bold')
                ax2.set_ylabel('Loss')
                ax2.grid(axis='y', alpha=0.3)
                
                for bar in bars2:
                    height = bar.get_height()
                    ax2.text(bar.get_x() + bar.get_width()/2., height + 0.01,
                            f'{height:.4f}', ha='center', va='bottom', fontweight='bold')
                
                ax3 = fig.add_subplot(gs[1, :])
                per_class = results['federated_model'].get('per_class_accuracy', {})
                
                if per_class:
                    sorted_classes = sorted(per_class.items(), key=lambda x: x[1], reverse=True)
                    classes = [x[0] for x in sorted_classes[:10]]
                    accs = [x[1] for x in sorted_classes[:10]]
                    
                    bars3 = ax3.barh(classes, accs, color='lightblue')
                    ax3.set_title('Top 10 Classes by Accuracy', fontsize=12, fontweight='bold')
                    ax3.set_xlabel('Accuracy')
                    ax3.set_xlim(0, 1.1)
                    ax3.grid(axis='x', alpha=0.3)
                    
                    for i, acc in enumerate(accs):
                        ax3.text(acc + 0.01, i, f'{acc:.3f}', va='center', fontweight='bold')
                
                dataset_name = results.get('dataset_config', {}).get('dataset_name', 'Unknown Dataset')
                fig.suptitle(f'Model Evaluation - {dataset_name}', 
                            fontsize=14, fontweight='bold')
            
            else:
                gs = fig.add_gridspec(3, 2, hspace=0.3, wspace=0.3)
                
                ax1 = fig.add_subplot(gs[0, 0])
                models = ['Initial Model', 'Federated Model']
                accuracies = [
                    results['initial_model']['accuracy'],
                    results['federated_model']['accuracy']
                ]
                colors = ['lightcoral', 'lightblue']
                
                bars1 = ax1.bar(models, accuracies, color=colors)
                ax1.set_title('Model Accuracy Comparison', fontsize=12, fontweight='bold')
                ax1.set_ylabel('Accuracy')
                ax1.set_ylim(0, 1)
                ax1.grid(axis='y', alpha=0.3)
                
                for bar, acc in zip(bars1, accuracies):
                    height = bar.get_height()
                    ax1.text(bar.get_x() + bar.get_width()/2., height + 0.01,
                            f'{acc:.3f}', ha='center', va='bottom', fontweight='bold')
                
                ax2 = fig.add_subplot(gs[0, 1])
                losses = [
                    results['initial_model']['loss'],
                    results['federated_model']['loss']
                ]
                
                bars2 = ax2.bar(models, losses, color=colors)
                ax2.set_title('Model Loss Comparison', fontsize=12, fontweight='bold')
                ax2.set_ylabel('Loss')
                ax2.grid(axis='y', alpha=0.3)
                
                for bar, loss in zip(bars2, losses):
                    height = bar.get_height()
                    ax2.text(bar.get_x() + bar.get_width()/2., height + 0.01,
                            f'{loss:.3f}', ha='center', va='bottom', fontweight='bold')
                
                if results['initial_model'].get('per_class_accuracy'):
                    ax3 = fig.add_subplot(gs[1, 0])
                    per_class = results['initial_model']['per_class_accuracy']
                    classes = list(per_class.keys())[:10]
                    values = [per_class[c] for c in classes]
                    
                    ax3.barh(classes, values, color='lightcoral')
                    ax3.set_title('Initial Model - Per-Class Accuracy', fontsize=11, fontweight='bold')
                    ax3.set_xlabel('Accuracy')
                    ax3.set_xlim(0, 1)
                    ax3.grid(axis='x', alpha=0.3)
                
                ax4 = fig.add_subplot(gs[1, 1])
                if results['federated_model'].get('per_class_accuracy'):
                    per_class = results['federated_model']['per_class_accuracy']
                    classes = list(per_class.keys())[:10]
                    values = [per_class[c] for c in classes]
                    
                    ax4.barh(classes, values, color='lightblue')
                    ax4.set_title('Federated Model - Per-Class Accuracy', fontsize=11, fontweight='bold')
                    ax4.set_xlabel('Accuracy')
                    ax4.set_xlim(0, 1)
                    ax4.grid(axis='x', alpha=0.3)
                
                if results.get('improvement'):
                    ax5 = fig.add_subplot(gs[2, :])
                    
                    metrics = ['Accuracy\nImprovement', 'Loss\nImprovement', 'Relative Accuracy\nImprovement (%)']
                    values = [
                        results['improvement']['accuracy_improvement'],
                        results['improvement']['loss_improvement'],
                        results['improvement']['relative_accuracy_improvement']
                    ]
                    colors_imp = ['green' if v > 0 else 'red' for v in values]
                    
                    bars5 = ax5.bar(metrics, values, color=colors_imp, alpha=0.7)
                    ax5.set_title('Performance Improvement Metrics', fontsize=12, fontweight='bold')
                    ax5.set_ylabel('Improvement Value')
                    ax5.axhline(y=0, color='black', linestyle='-', linewidth=0.5)
                    ax5.grid(axis='y', alpha=0.3)
                    
                    for bar, val in zip(bars5, values):
                        height = bar.get_height()
                        ax5.text(bar.get_x() + bar.get_width()/2., 
                                height + (0.01 if height > 0 else -0.01),
                                f'{val:.3f}', ha='center', 
                                va='bottom' if height > 0 else 'top',
                                fontweight='bold')
                
                dataset_name = results.get('dataset_config', {}).get('dataset_name', 'Unknown')
                num_classes = results.get('dataset_config', {}).get('num_classes', 0)
                fig.suptitle(f'Global Model Evaluation - {dataset_name} ({num_classes} classes)', 
                            fontsize=14, fontweight='bold')
            
            plots_dir = os.path.join(self.results_dir, "plots")
            os.makedirs(plots_dir, exist_ok=True)
            
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            plot_path = os.path.join(plots_dir, f"evaluation_{timestamp}.png")
            plt.savefig(plot_path, dpi=300, bbox_inches='tight')
            logger.info(f"Evaluation plot saved to: {plot_path}")
            
            latest_path = os.path.join(plots_dir, "latest_evaluation.png")
            shutil.copy2(plot_path, latest_path)
            
            plt.close()
            
        except Exception as e:
            logger.error(f"Error generating plots: {e}", exc_info=True)

def parse_arguments():
    """Parse command line arguments"""
    parser = argparse.ArgumentParser(description='Evaluate a global model with test data')
    
    parser.add_argument('--data-dir', '--test-dir', dest='test_dir', type=str, required=True,
                      help='Path to the test data directory')
    parser.add_argument('--model-path', type=str, default=None,
                      help='Path to a specific model checkpoint (optional)')
    parser.add_argument('--output-dir', type=str, default='./evaluation_results',
                      help='Directory to save evaluation results')
    parser.add_argument('--config', type=str, default=None,
                      help='Path to dataset config file (optional)')
    parser.add_argument('--batch-size', type=int, default=64,
                      help='Batch size for evaluation')
    parser.add_argument('--cpu', action='store_true',
                      help='Force CPU usage even if CUDA is available')
    parser.add_argument('--num-clients', type=int, default=0,
                      help='Number of clients participated in aggregation')
    
    return parser.parse_args()

def find_latest_model(model_dir: str = "./global_models") -> Optional[str]:
    """Find the latest model checkpoint"""
    if not os.path.exists(model_dir):
        logger.warning(f"Model directory not found: {model_dir}")
        return None
    
    model_files = []
    for ext in ['.pth', '.pt', '.pth.tar', '.ckpt']:
        model_files.extend(glob.glob(os.path.join(model_dir, '**', f'*{ext}'), recursive=True))
    
    if not model_files:
        logger.warning(f"No model files found in {model_dir}")
        return None
    
    model_files.sort(key=os.path.getmtime, reverse=True)
    latest_model = model_files[0]
    logger.info(f"Found {len(model_files)} model files, using latest: {os.path.basename(latest_model)}")
    return latest_model

def main():
    """Main evaluation function"""
    args = parse_arguments()
    
    device = 'cpu' if args.cpu else ('cuda' if torch.cuda.is_available() else 'cpu')
    logger.info(f"Using device: {device}")
    
    model_path = args.model_path
    if model_path is None:
        model_path = find_latest_model()
        if model_path is None:
            logger.error("No model found and no model path provided")
            return 1
    
    if not os.path.isdir(args.test_dir):
        logger.error(f"Test directory not found: {args.test_dir}")
        return 1
    
    os.makedirs(args.output_dir, exist_ok=True)
    
    # Create timestamped run directory
    timestamp_str = datetime.now().strftime("%Y%m%d_%H%M%S")
    model_name_clean = os.path.basename(model_path).replace('.', '_')
    run_dir = os.path.join(args.output_dir, f"{timestamp_str}_{model_name_clean}")
    os.makedirs(run_dir, exist_ok=True)
    
    logger.info(f"Saving run results to: {os.path.abspath(run_dir)}")
    
    try:
        evaluator = GlobalModelEvaluator(
            controller_data_dir=args.test_dir,
            results_dir=run_dir,  # Use run_dir
            config_path=args.config
        )
        
        logger.info(f"Loading model from: {model_path}")
        model = evaluator.load_model_from_checkpoint(model_path)
        if model is None:
            logger.error("Failed to load model")
            return 1
        
        model = model.to(device)
        
        test_loader, _ = evaluator.load_test_data()
        if test_loader is None:
            logger.error("Failed to load test data")
            return 1
        
        # Plot Test Data Distribution
        evaluator._plot_class_distribution(test_loader, os.path.join(run_dir, "class_distribution.png"))

        import time
        start_time = time.time()
        logger.info("Starting evaluation...")
        results = evaluator.evaluate_model(model, test_loader, "Global Model")
        end_time = time.time()
        duration = end_time - start_time
        
        if results:
            # Enrich results with metadata
            results['evaluation_duration_seconds'] = duration
            results['num_clients'] = args.num_clients
            results['evaluation_timestamp'] = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            
            results_file = os.path.join(run_dir, 'evaluation_results.json')
            with open(results_file, 'w') as f:
                json.dump(results, f, indent=2)
            
            # Generate Text Summary Report
            summary_path = os.path.join(run_dir, 'evaluation_summary.txt')
            with open(summary_path, 'w') as f:
                f.write("="*60 + "\n")
                f.write(f"FEDERATED LEARNING EVALUATION REPORT\n")
                f.write(f"Timestamp: {results['evaluation_timestamp']}\n")
                f.write("="*60 + "\n\n")
                
                f.write(f"Model File:      {os.path.basename(model_path)}\n")
                f.write(f"Test Dataset:    {args.test_dir}\n")
                f.write(f"Test Samples:    {results['total_samples']}\n")
                f.write(f"Clients:         {args.num_clients}\n")
                f.write(f"Duration:        {duration:.2f} seconds\n\n")
                
                f.write("-" * 30 + "\n")
                f.write(f"GLOBAL METRICS\n")
                f.write("-" * 30 + "\n")
                f.write(f"Accuracy:        {results['accuracy']:.2%}\n")
                f.write(f"Loss:            {results['loss']:.4f}\n")
                f.write(f"Correct:         {results['correct_predictions']}/{results['total_samples']}\n\n")
                
                if results['per_class_accuracy']:
                    f.write("-" * 30 + "\n")
                    f.write(f"PER-CLASS PERFORMANCE\n")
                    f.write("-" * 30 + "\n")
                    # Sort by class name
                    for cls_name in sorted(results['per_class_accuracy'].keys()):
                         acc = results['per_class_accuracy'][cls_name]
                         f.write(f"{cls_name:<20}: {acc:.2%}\n")
            
            logger.info("\n" + "="*60)
            logger.info("EVALUATION COMPLETED SUCCESSFULLY")
            logger.info("="*60)
            logger.info(f"Model: {os.path.basename(model_path)}")
            logger.info(f"Clients: {args.num_clients}")
            logger.info(f"Test Accuracy: {results['accuracy']:.4f}")
            logger.info(f"Test Loss: {results['loss']:.4f}")
            logger.info(f"Time Taken: {duration:.2f}s")
            logger.info(f"Results saved to: {os.path.abspath(run_dir)}") # Log run_dir
            logger.info(f"Summary Report: {os.path.abspath(summary_path)}")
            
            # Helper to link latest
            try:
                latest_link = os.path.join(args.output_dir, "latest_results")
                if os.path.islink(latest_link) or os.path.exists(latest_link):
                    os.unlink(latest_link)
                os.symlink(os.path.basename(run_dir), latest_link)
            except:
                pass
            
            evaluator.generate_comparison_plots({
                'test_samples': len(test_loader.dataset),
                'federated_model': results,
                'dataset_config': results.get('dataset_config', {})
            })
            
            evaluator.generate_comparison_plots({
                'test_samples': len(test_loader.dataset),
                'federated_model': results,
                'dataset_config': results.get('dataset_config', {})
            })
            
            return 0
        else:
            logger.error("Evaluation failed")
            return 1
            
    except Exception as e:
        logger.error(f"Fatal error during evaluation: {e}", exc_info=True)
        return 1

if __name__ == "__main__":
    exit_code = main()
    sys.exit(exit_code)