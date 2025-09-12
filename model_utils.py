"""
Model utilities for MobileNetV2-based image classification.
Handles model creation, loading, saving, and data preprocessing.
"""

import os
import torch
import torch.nn as nn
import torchvision.models as models
import torchvision.transforms as transforms
from torch.utils.data import Dataset, DataLoader
from PIL import Image
import numpy as np
from typing import List, Tuple, Dict, Optional
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class ImageDataset(Dataset):
    """Custom dataset for loading images with class in filename (class#image.jpg)."""
    
    def __init__(self, data_dir: str, transform=None, class_to_idx: Optional[Dict[str, int]] = None):
        """
        Initialize dataset.
        
        Args:
            data_dir: Directory containing images with filenames in format 'class#image.jpg'
            transform: Image transformations to apply
            class_to_idx: Optional mapping from class names to indices
        """
        self.data_dir = data_dir
        self.transform = transform
        self.images = []
        self.labels = []
        
        # Discover classes and create mapping
        if class_to_idx is None:
            self.class_to_idx = self._discover_classes()
        else:
            self.class_to_idx = class_to_idx
            
        self.idx_to_class = {v: k for k, v in self.class_to_idx.items()}
        
        # Load all images
        self._load_images()
        
        logger.info(f"Loaded {len(self.images)} images from {len(self.class_to_idx)} classes")
        logger.info(f"Classes: {list(self.class_to_idx.keys())}")
    
    def _discover_classes(self) -> Dict[str, int]:
        """Discover class names from filenames (format: class#image.jpg)."""
        classes = set()
        
        # First pass: discover all unique classes
        for filename in os.listdir(self.data_dir):
            if not self._is_image_file(filename):
                continue
                
            # Extract class name from filename (format: class#image.jpg)
            if '#' in filename:
                class_name = filename.split('#')[0]
                classes.add(class_name)
        
        classes = sorted(list(classes))  # Ensure consistent ordering
        return {cls_name: idx for idx, cls_name in enumerate(classes)}
    
    def _load_images(self):
        """Load all images and their corresponding labels from filenames."""
        for filename in os.listdir(self.data_dir):
            if not self._is_image_file(filename):
                continue
                
            # Extract class name from filename (format: class#image.jpg)
            if '#' in filename:
                class_name = filename.split('#')[0]
                if class_name in self.class_to_idx:
                    img_path = os.path.join(self.data_dir, filename)
                    self.images.append(img_path)
                    self.labels.append(self.class_to_idx[class_name])
    
    def _is_image_file(self, filename: str) -> bool:
        """Check if file is a valid image."""
        valid_extensions = {'.jpg', '.jpeg', '.png', '.bmp', '.tiff', '.tif'}
        return any(filename.lower().endswith(ext) for ext in valid_extensions)
    
    def __len__(self):
        return len(self.images)
    
    def __getitem__(self, idx):
        img_path = self.images[idx]
        label = self.labels[idx]
        
        try:
            img = Image.open(img_path).convert('RGB')
            
            if self.transform:
                img = self.transform(img)
                
            return img, label
            
        except Exception as e:
            logger.error(f"Error loading image {img_path}: {e}")
            # Return a random image of the same size on error
            img = torch.randn(3, 224, 224)  # Default size for the model
            return img, label
    
    def get_class_names(self) -> List[str]:
        """Get list of class names."""
        return list(self.class_to_idx.keys())
    
    def get_num_classes(self) -> int:
        """Get number of classes."""
        return len(self.class_to_idx)

def get_model(num_classes: int, device: torch.device, pretrained: bool = True) -> nn.Module:
    """
    Create MobileNetV2 model for transfer learning.
    
    Args:
        num_classes: Number of output classes
        device: Device to place model on
        pretrained: Whether to use pretrained weights
        
    Returns:
        MobileNetV2 model with custom classifier
    """
    model = models.mobilenet_v2(pretrained=pretrained)
    
    # Replace the classifier head
    in_features = model.classifier[1].in_features
    model.classifier[1] = nn.Linear(in_features, num_classes)
    
    model = model.to(device)
    logger.info(f"Created MobileNetV2 model with {num_classes} classes")
    return model

def get_transforms() -> Tuple[transforms.Compose, transforms.Compose]:
    """
    Get training and validation transforms.
    
    Returns:
        Tuple of (train_transform, val_transform)
    """
    train_transform = transforms.Compose([
        transforms.Resize(256),
        transforms.RandomCrop(224),
        transforms.RandomHorizontalFlip(p=0.5),
        transforms.RandomRotation(degrees=10),
        transforms.ColorJitter(brightness=0.2, contrast=0.2, saturation=0.2, hue=0.1),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
    ])
    
    val_transform = transforms.Compose([
        transforms.Resize(256),
        transforms.CenterCrop(224),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
    ])
    
    return train_transform, val_transform

def save_model(model: nn.Module, optimizer: torch.optim.Optimizer, 
               epoch: int, loss: float, class_to_idx: Dict[str, int], 
               save_path: str):
    """
    Save model checkpoint.
    
    Args:
        model: Model to save
        optimizer: Optimizer state
        epoch: Current epoch
        loss: Current loss
        class_to_idx: Class to index mapping
        save_path: Path to save checkpoint
    """
    checkpoint = {
        'model_state_dict': model.state_dict(),
        'optimizer_state_dict': optimizer.state_dict(),
        'epoch': epoch,
        'loss': loss,
        'class_to_idx': class_to_idx,
        'num_classes': len(class_to_idx)
    }
    
    os.makedirs(os.path.dirname(save_path), exist_ok=True)
    torch.save(checkpoint, save_path)
    logger.info(f"Model saved to {save_path}")

def load_model(load_path: str, device: torch.device) -> Tuple[nn.Module, torch.optim.Optimizer, 
                                                             int, float, Dict[str, int]]:
    """
    Load model checkpoint.
    
    Args:
        load_path: Path to checkpoint
        device: Device to load model on
        
    Returns:
        Tuple of (model, optimizer, epoch, loss, class_to_idx)
    """
    if not os.path.exists(load_path):
        raise FileNotFoundError(f"Checkpoint not found at {load_path}")
    
    checkpoint = torch.load(load_path, map_location=device)
    
    # Create model with correct number of classes
    num_classes = checkpoint['num_classes']
    model = get_model(num_classes, device, pretrained=False)
    model.load_state_dict(checkpoint['model_state_dict'])
    
    # Create optimizer (will be properly initialized in trainer)
    optimizer = torch.optim.Adam(model.parameters(), lr=0.001)
    optimizer.load_state_dict(checkpoint['optimizer_state_dict'])
    
    epoch = checkpoint['epoch']
    loss = checkpoint['loss']
    class_to_idx = checkpoint['class_to_idx']
    
    logger.info(f"Model loaded from {load_path}")
    logger.info(f"Epoch: {epoch}, Loss: {loss:.4f}, Classes: {list(class_to_idx.keys())}")
    
    return model, optimizer, epoch, loss, class_to_idx

def create_data_loaders(data_dir: str, batch_size: int = 32, 
                       num_workers: int = 2) -> Tuple[DataLoader, Dict[str, int]]:
    """
    Create data loaders for training.
    
    Args:
        data_dir: Directory containing images
        batch_size: Batch size for training
        num_workers: Number of worker processes
        
    Returns:
        Tuple of (dataloader, class_to_idx)
    """
    train_transform, _ = get_transforms()
    
    dataset = ImageDataset(data_dir, transform=train_transform)
    
    if len(dataset) == 0:
        raise ValueError(f"No images found in {data_dir}")
    
    dataloader = DataLoader(
        dataset, 
        batch_size=batch_size, 
        shuffle=True, 
        num_workers=num_workers,
        pin_memory=True
    )
    
    return dataloader, dataset.class_to_idx

def validate_image_file(file_path: str) -> bool:
    """
    Validate if a file is a valid image.
    
    Args:
        file_path: Path to image file
        
    Returns:
        True if valid image, False otherwise
    """
    try:
        with Image.open(file_path) as img:
            img.verify()
        return True
    except Exception:
        return False

def get_device() -> torch.device:
    """Get the best available device (CUDA if available, else CPU)."""
    if torch.cuda.is_available():
        device = torch.device('cuda')
        logger.info(f"Using CUDA device: {torch.cuda.get_device_name()}")
    else:
        device = torch.device('cpu')
        logger.info("Using CPU device")
    
    return device
