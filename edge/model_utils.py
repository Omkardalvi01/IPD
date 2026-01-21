"""
Utility functions for model training and evaluation.
"""
import os
import torch
import torch.nn as nn
import torch.optim as optim
import torchvision.transforms as transforms
from torch.utils.data import DataLoader, Dataset
from torchvision.datasets import ImageFolder
from PIL import Image
import logging
from pathlib import Path
from typing import Tuple, Dict, List, Optional, Union

import re

def natural_sort_key(s):
    """Key for natural/alphanumeric sorting (e.g., '2' comes before '10')"""
    return [int(text) if text.isdigit() else text.lower()
            for text in re.split('([0-9]+)', str(s))]

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class CustomImageDataset(Dataset):
    """Custom dataset for handling images with class names in filenames."""
    
    def __init__(self, data_dir: Union[str, Path], transform=None):
        """
        Initialize the dataset.
        
        Args:
            data_dir: Directory containing images with filenames like "class#image.jpg"
            transform: Optional transform to be applied on images
        """
        self.data_dir = Path(data_dir)
        self.transform = transform
        self.image_paths = []
        self.labels = []
        self.class_to_idx = {}
        
        self._load_data()
    
    def _load_data(self):
        """Load image paths and extract labels from filenames."""
        # Find all image files
        valid_extensions = {'.jpg', '.jpeg', '.png', '.bmp', '.tiff', '.tif'}
        image_files = []
        
        for file_path in self.data_dir.iterdir():
            if file_path.is_file() and file_path.suffix.lower() in valid_extensions:
                filename = file_path.name
                # Handle both filename formats:
                # 1. class#image.jpg
                # 2. mnist_train_XXXXX_label_Y.jpg
                if 'label_' in filename:
                    # MNIST format: extract the label after 'label_'
                    class_name = filename.split('label_')[-1].split('.')[0]
                    image_files.append((file_path, class_name))
                elif '#' in filename:
                    # Original format: class#image.jpg
                    class_name = filename.split('#')[0]
                    image_files.append((file_path, class_name))
        
        if not image_files:
            raise ValueError(f"No valid images found in {self.data_dir}")
        
        # Create class_to_idx mapping with Alphanumeric (Natural) sorting
        unique_classes = sorted(set(class_name for _, class_name in image_files), key=natural_sort_key)
        self.class_to_idx = {cls_name: idx for idx, cls_name in enumerate(unique_classes)}
        
        # Create image paths and labels lists
        for file_path, class_name in image_files:
            if validate_image_file(file_path):  # Only add valid images
                self.image_paths.append(file_path)
                self.labels.append(self.class_to_idx[class_name])
        
        logger.info(f"Loaded {len(self.image_paths)} valid images from {len(unique_classes)} classes")
        logger.info(f"Classes: {list(self.class_to_idx.keys())}")
    
    def __len__(self):
        return len(self.image_paths)
    
    def __getitem__(self, idx):
        """Get an image and its label."""
        image_path = self.image_paths[idx]
        label = self.labels[idx]
        
        # Load image
        try:
            image = Image.open(image_path).convert('RGB')
        except Exception as e:
            logger.error(f"Error loading image {image_path}: {e}")
            # Return a black image as fallback
            image = Image.new('RGB', (224, 224), (0, 0, 0))
        
        # Apply transform if provided
        if self.transform:
            image = self.transform(image)
        
        return image, label


def get_device() -> torch.device:
    """Get the available device (GPU if available, else CPU)."""
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    logger.info(f"Using device: {device}")
    return device


def get_model(num_classes: int, device: torch.device, pretrained: bool = False) -> torch.nn.Module:
    """
    Get a MobileNetV2 model with the specified number of output classes.
    
    Args:
        num_classes: Number of output classes
        device: Device to move the model to
        pretrained: Whether to use pretrained weights
        
    Returns:
        Initialized MobileNetV2 model
    """
    from torchvision import models
    
    logger.info(f"Creating MobileNetV2 model with {num_classes} classes (pretrained={pretrained})")
    model = models.mobilenet_v2(pretrained=pretrained)
    
    # Replace the last fully connected layer
    in_features = model.classifier[1].in_features
    model.classifier[1] = nn.Linear(in_features, num_classes)
    
    # Move model to device
    model = model.to(device)
    logger.info(f"Model moved to {device}")
    
    return model


def create_data_loaders(
    data_dir: Union[str, Path],
    batch_size: int = 32,
    img_size: int = 224,
    num_workers: int = 2
) -> Tuple[DataLoader, Dict[str, int]]:
    """
    Create data loaders for training.
    
    Args:
        data_dir: Path to the data directory
        batch_size: Batch size for data loaders
        img_size: Size to resize images to
        num_workers: Number of workers for data loading
        
    Returns:
        Tuple of (data_loader, class_to_idx)
    """
    logger.info(f"Creating data loaders for directory: {data_dir}")
    
    # Define data transformations
    transform = transforms.Compose([
        transforms.Resize((img_size, img_size)),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
    ])
    
    # Create custom dataset
    dataset = CustomImageDataset(data_dir, transform=transform)
    
    if len(dataset) == 0:
        raise ValueError(f"No valid images found in {data_dir}")
    
    # Create data loader
    loader = DataLoader(
        dataset,
        batch_size=batch_size,
        shuffle=True,
        num_workers=num_workers,
        pin_memory=torch.cuda.is_available(),
        drop_last=False  # Don't drop last batch even if it's smaller
    )
    
    logger.info(f"Created data loader with {len(dataset)} images, batch size {batch_size}")
    return loader, dataset.class_to_idx


def save_model(
    model: torch.nn.Module,
    optimizer: torch.optim.Optimizer,
    epoch: int,
    loss: float,
    class_to_idx: Dict[str, int],
    path: Union[str, Path]
) -> None:
    """
    Save the model checkpoint.
    
    Args:
        model: Model to save
        optimizer: Optimizer state to save
        epoch: Current epoch
        loss: Current loss
        class_to_idx: Mapping from class names to indices
        path: Path to save the checkpoint
    """
    checkpoint = {
        'model_state_dict': model.state_dict(),
        'optimizer_state_dict': optimizer.state_dict(),
        'class_to_idx': class_to_idx,
        'epoch': epoch,
        'loss': loss,
        'num_classes': len(class_to_idx)
    }
    
    # Create directory if it doesn't exist
    os.makedirs(os.path.dirname(path), exist_ok=True)
    
    torch.save(checkpoint, path)
    logger.info(f"Model checkpoint saved to {path}")


def load_model(
    path: Union[str, Path],
    device: Optional[torch.device] = None
) -> Tuple[torch.nn.Module, torch.optim.Optimizer, Dict[str, int], Dict[str, object]]:
    """
    Load a model checkpoint.
    
    Args:
        path: Path to the checkpoint file
        device: Device to load the model to
        
    Returns:
        Tuple of (model, optimizer, class_to_idx, checkpoint)
    """
    if device is None:
        device = get_device()
    
    logger.info(f"Loading model from {path}")
    checkpoint = torch.load(path, map_location=device)
    
    # Get number of classes from checkpoint
    num_classes = checkpoint.get('num_classes', len(checkpoint['class_to_idx']))
    
    # Initialize model
    model = get_model(num_classes=num_classes, device=device, pretrained=False)
    model.load_state_dict(checkpoint['model_state_dict'])
    
    # Initialize optimizer
    optimizer = optim.Adam(model.parameters())
    if 'optimizer_state_dict' in checkpoint:
        optimizer.load_state_dict(checkpoint['optimizer_state_dict'])
    
    logger.info(f"Model loaded successfully with {num_classes} classes")
    return model, optimizer, checkpoint['class_to_idx'], checkpoint


def validate_image_file(file_path: Union[str, Path]) -> bool:
    """
    Validate if a file is a valid image file.
    
    Args:
        file_path: Path to the image file
        
    Returns:
        bool: True if the file is a valid image, False otherwise
    """
    try:
        with Image.open(file_path) as img:
            img.verify()
        # Re-open the image to actually check if it can be loaded
        with Image.open(file_path) as img:
            img.load()
        return True
    except (IOError, SyntaxError, OSError) as e:
        logger.warning(f"Invalid image file {file_path}: {e}")
        return False
    except Exception as e:
        logger.warning(f"Unexpected error validating image {file_path}: {e}")
        return False