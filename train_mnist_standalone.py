"""
Standalone script to train and test MobileNetV2 on MNIST JPG dataset.
Trains on 1000 randomly selected images for 5 epochs.
"""

import os
import sys
import random
import logging
from pathlib import Path
from typing import Tuple, List

import torch
import torch.nn as nn
import torch.optim as optim
import torchvision.transforms as transforms
from torch.utils.data import DataLoader, Dataset, Subset
from torchvision import models
from PIL import Image
from tqdm import tqdm

# Add edge directory to path for imports
sys.path.append(str(Path(__file__).parent / 'edge'))

from edge.model_utils import CustomImageDataset, get_device, save_model

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('mnist_training.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)


def get_mobilenetv2_model(num_classes: int, device: torch.device) -> nn.Module:
    """
    Create MobileNetV2 model with specified number of classes.
    
    Args:
        num_classes: Number of output classes
        device: Device to move model to
        
    Returns:
        MobileNetV2 model
    """
    logger.info(f"Creating MobileNetV2 model with {num_classes} classes")
    model = models.mobilenet_v2(pretrained=True)
    
    # Replace the last fully connected layer
    in_features = model.classifier[1].in_features
    model.classifier[1] = nn.Linear(in_features, num_classes)
    
    model = model.to(device)
    logger.info(f"Model created and moved to {device}")
    
    return model


def create_random_subset_loader(
    data_dir: str,
    num_samples: int = 1000,
    batch_size: int = 32,
    img_size: int = 224,
    train_split: float = 0.8,
    seed: int = 42
) -> Tuple[DataLoader, DataLoader, dict]:
    """
    Create train and test data loaders with random subset of images.
    
    Args:
        data_dir: Path to directory containing MNIST JPG images
        num_samples: Total number of samples to use
        batch_size: Batch size for data loaders
        img_size: Size to resize images to
        train_split: Fraction of data to use for training
        seed: Random seed for reproducibility
        
    Returns:
        Tuple of (train_loader, test_loader, class_to_idx)
    """
    logger.info(f"Creating data loaders from {data_dir}")
    logger.info(f"Using {num_samples} random samples with {train_split*100}% for training")
    
    # Set random seed for reproducibility
    random.seed(seed)
    torch.manual_seed(seed)
    
    # Define transforms with data augmentation for training
    train_transform = transforms.Compose([
        transforms.Resize((img_size, img_size)),
        transforms.RandomHorizontalFlip(),
        transforms.RandomRotation(10),
        transforms.ColorJitter(brightness=0.2, contrast=0.2),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
    ])
    
    # Define transforms for testing (no augmentation)
    test_transform = transforms.Compose([
        transforms.Resize((img_size, img_size)),
        transforms.ToTensor(),
        transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
    ])
    
    # Create full dataset
    full_dataset = CustomImageDataset(data_dir, transform=None)
    
    if len(full_dataset) < num_samples:
        logger.warning(f"Dataset has only {len(full_dataset)} images, using all of them")
        num_samples = len(full_dataset)
    
    # Randomly select indices
    all_indices = list(range(len(full_dataset)))
    random.shuffle(all_indices)
    selected_indices = all_indices[:num_samples]
    
    # Split into train and test
    num_train = int(num_samples * train_split)
    train_indices = selected_indices[:num_train]
    test_indices = selected_indices[num_train:]
    
    logger.info(f"Train samples: {len(train_indices)}, Test samples: {len(test_indices)}")
    
    # Create train dataset with augmentation
    train_dataset = CustomImageDataset(data_dir, transform=train_transform)
    train_subset = Subset(train_dataset, train_indices)
    
    # Create test dataset without augmentation
    test_dataset = CustomImageDataset(data_dir, transform=test_transform)
    test_subset = Subset(test_dataset, test_indices)
    
    # Create data loaders
    train_loader = DataLoader(
        train_subset,
        batch_size=batch_size,
        shuffle=True,
        num_workers=2,
        pin_memory=torch.cuda.is_available()
    )
    
    test_loader = DataLoader(
        test_subset,
        batch_size=batch_size,
        shuffle=False,
        num_workers=2,
        pin_memory=torch.cuda.is_available()
    )
    
    return train_loader, test_loader, full_dataset.class_to_idx


def train_model(
    model: nn.Module,
    train_loader: DataLoader,
    criterion: nn.Module,
    optimizer: optim.Optimizer,
    device: torch.device,
    num_epochs: int = 5
) -> List[float]:
    """
    Train the model.
    
    Args:
        model: Model to train
        train_loader: Training data loader
        criterion: Loss function
        optimizer: Optimizer
        device: Device to train on
        num_epochs: Number of epochs to train
        
    Returns:
        List of average losses per epoch
    """
    logger.info(f"Starting training for {num_epochs} epochs")
    model.train()
    epoch_losses = []
    
    for epoch in range(num_epochs):
        running_loss = 0.0
        correct = 0
        total = 0
        
        # Progress bar for batches
        pbar = tqdm(train_loader, desc=f"Epoch {epoch+1}/{num_epochs}")
        
        for batch_idx, (images, labels) in enumerate(pbar):
            images = images.to(device)
            labels = labels.to(device)
            
            # Forward pass
            optimizer.zero_grad()
            outputs = model(images)
            loss = criterion(outputs, labels)
            
            # Backward pass and optimization
            loss.backward()
            optimizer.step()
            
            # Statistics
            running_loss += loss.item()
            _, predicted = torch.max(outputs.data, 1)
            total += labels.size(0)
            correct += (predicted == labels).sum().item()
            
            # Update progress bar
            pbar.set_postfix({
                'loss': f'{loss.item():.4f}',
                'acc': f'{100 * correct / total:.2f}%'
            })
        
        # Calculate epoch statistics
        epoch_loss = running_loss / len(train_loader)
        epoch_acc = 100 * correct / total
        epoch_losses.append(epoch_loss)
        
        logger.info(f"Epoch {epoch+1}/{num_epochs} - Loss: {epoch_loss:.4f}, Accuracy: {epoch_acc:.2f}%")
    
    return epoch_losses


def test_model(
    model: nn.Module,
    test_loader: DataLoader,
    criterion: nn.Module,
    device: torch.device
) -> Tuple[float, float]:
    """
    Test the model.
    
    Args:
        model: Model to test
        test_loader: Test data loader
        criterion: Loss function
        device: Device to test on
        
    Returns:
        Tuple of (test_loss, test_accuracy)
    """
    logger.info("Starting model evaluation on test set")
    model.eval()
    
    test_loss = 0.0
    correct = 0
    total = 0
    
    with torch.no_grad():
        pbar = tqdm(test_loader, desc="Testing")
        
        for images, labels in pbar:
            images = images.to(device)
            labels = labels.to(device)
            
            # Forward pass
            outputs = model(images)
            loss = criterion(outputs, labels)
            
            # Statistics
            test_loss += loss.item()
            _, predicted = torch.max(outputs.data, 1)
            total += labels.size(0)
            correct += (predicted == labels).sum().item()
            
            # Update progress bar
            pbar.set_postfix({
                'loss': f'{loss.item():.4f}',
                'acc': f'{100 * correct / total:.2f}%'
            })
    
    test_loss = test_loss / len(test_loader)
    test_accuracy = 100 * correct / total
    
    logger.info(f"Test Results - Loss: {test_loss:.4f}, Accuracy: {test_accuracy:.2f}%")
    
    return test_loss, test_accuracy


def main():
    """Main function to run training and testing."""
    # Configuration
    DATA_DIR = "./mnist_jpg"
    NUM_SAMPLES = 1000
    BATCH_SIZE = 32
    NUM_EPOCHS = 5
    LEARNING_RATE = 0.001
    MODEL_SAVE_PATH = "./saved_models/mnist_mobilenetv2_1000samples.pth"
    
    logger.info("="*60)
    logger.info("MNIST MobileNetV2 Training Script")
    logger.info("="*60)
    logger.info(f"Data directory: {DATA_DIR}")
    logger.info(f"Number of samples: {NUM_SAMPLES}")
    logger.info(f"Batch size: {BATCH_SIZE}")
    logger.info(f"Number of epochs: {NUM_EPOCHS}")
    logger.info(f"Learning rate: {LEARNING_RATE}")
    logger.info("="*60)
    
    # Check if data directory exists
    if not os.path.exists(DATA_DIR):
        logger.error(f"Data directory {DATA_DIR} does not exist!")
        return
    
    # Get device
    device = get_device()
    
    # Create data loaders
    logger.info("\n" + "="*60)
    logger.info("STEP 1: Creating Data Loaders")
    logger.info("="*60)
    train_loader, test_loader, class_to_idx = create_random_subset_loader(
        DATA_DIR,
        num_samples=NUM_SAMPLES,
        batch_size=BATCH_SIZE,
        train_split=0.8
    )
    
    num_classes = len(class_to_idx)
    logger.info(f"Number of classes: {num_classes}")
    logger.info(f"Classes: {sorted(class_to_idx.keys())}")
    
    # Create model
    logger.info("\n" + "="*60)
    logger.info("STEP 2: Creating Model")
    logger.info("="*60)
    model = get_mobilenetv2_model(num_classes, device)
    
    # Define loss function and optimizer
    criterion = nn.CrossEntropyLoss()
    optimizer = optim.Adam(model.parameters(), lr=LEARNING_RATE)
    
    # Train model
    logger.info("\n" + "="*60)
    logger.info("STEP 3: Training Model")
    logger.info("="*60)
    epoch_losses = train_model(
        model, train_loader, criterion, optimizer, device, NUM_EPOCHS
    )
    
    # Test model
    logger.info("\n" + "="*60)
    logger.info("STEP 4: Testing Model")
    logger.info("="*60)
    test_loss, test_accuracy = test_model(model, test_loader, criterion, device)
    
    # Save model
    logger.info("\n" + "="*60)
    logger.info("STEP 5: Saving Model")
    logger.info("="*60)
    os.makedirs(os.path.dirname(MODEL_SAVE_PATH), exist_ok=True)
    save_model(
        model, optimizer, NUM_EPOCHS, epoch_losses[-1], 
        class_to_idx, MODEL_SAVE_PATH
    )
    
    # Print summary
    logger.info("\n" + "="*60)
    logger.info("TRAINING SUMMARY")
    logger.info("="*60)
    logger.info(f"Total epochs trained: {NUM_EPOCHS}")
    logger.info(f"Final training loss: {epoch_losses[-1]:.4f}")
    logger.info(f"Test loss: {test_loss:.4f}")
    logger.info(f"Test accuracy: {test_accuracy:.2f}%")
    logger.info(f"Model saved to: {MODEL_SAVE_PATH}")
    logger.info("="*60)
    logger.info("Training completed successfully!")
    logger.info("="*60)


if __name__ == "__main__":
    main()
