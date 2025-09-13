"""
Incremental training script for MobileNetV2 on edge devices.
Monitors a directory for new images and performs incremental training.
"""

import os
import time
import threading
import logging
from pathlib import Path
from typing import Optional, Dict, Any
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import DataLoader
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler
from tqdm import tqdm

from model_utils import (
    get_model, get_device, create_data_loaders, 
    save_model, load_model, validate_image_file
)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('training.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

class ImageHandler(FileSystemEventHandler):
    """File system event handler for monitoring image directory."""
    
    def __init__(self, trainer_instance):
        self.trainer = trainer_instance
        self.last_activity = time.time()
        self.cooldown_period = 10  # seconds
        self.check_interval = 2  # seconds
        self._stop_checking = False
        
    def on_created(self, event):
        """Handle file creation events."""
        if not event.is_directory and self._is_image_file(event.src_path):
            logger.info(f"New image detected: {event.src_path}")
            self.last_activity = time.time()
            
            # Start or restart the cooldown timer
            if not hasattr(self, '_timer') or not self._timer.is_alive():
                self._start_cooldown_timer()
    
    def on_moved(self, event):
        """Handle file move events (for completed transfers)."""
        if not event.is_directory and self._is_image_file(event.dest_path):
            logger.info(f"Image transfer completed: {event.dest_path}")
            self.last_activity = time.time()
            
            # Start or restart the cooldown timer
            if not hasattr(self, '_timer') or not self._timer.is_alive():
                self._start_cooldown_timer()
    
    def _is_image_file(self, file_path: str) -> bool:
        """Check if file is a valid image."""
        valid_extensions = {'.jpg', '.jpeg', '.png', '.bmp', '.tiff', '.tif'}
        return any(file_path.lower().endswith(ext) for ext in valid_extensions)
    
    def _start_cooldown_timer(self):
        """Start cooldown timer to wait for batch completion."""
        self._timer = threading.Timer(self.cooldown_period, self._trigger_training)
        self._timer.start()
        logger.info(f"Started cooldown timer ({self.cooldown_period}s)")
    
    def _trigger_training(self):
        """Trigger training after cooldown period."""
        if time.time() - self.last_activity >= self.cooldown_period:
            logger.info("Cooldown period completed, triggering training...")
            self.trainer.trigger_training()
        else:
            # Restart timer if there was recent activity
            self._start_cooldown_timer()

class IncrementalTrainer:
    """Main trainer class for incremental learning."""
    
    def __init__(self, data_dir: str = "./received_images", 
                 model_save_path: str = "./saved_models/mobilenetv2_custom.pth",
                 batch_size: int = 32,
                 learning_rate: float = 0.001,
                 num_epochs: int = 10):
        """
        Initialize the incremental trainer.
        
        Args:
            data_dir: Directory to monitor for new images
            model_save_path: Path to save/load model
            batch_size: Batch size for training
            learning_rate: Learning rate for optimizer
            num_epochs: Number of epochs per training session
        """
        self.data_dir = Path(data_dir)
        self.model_save_path = model_save_path
        self.batch_size = batch_size
        self.learning_rate = learning_rate
        self.num_epochs = num_epochs
        
        # Create directories if they don't exist
        self.data_dir.mkdir(parents=True, exist_ok=True)
        os.makedirs(os.path.dirname(model_save_path), exist_ok=True)
        
        # Initialize device and model
        self.device = get_device()
        self.model = None
        self.optimizer = None
        self.criterion = nn.CrossEntropyLoss()
        self.class_to_idx = {}
        self.training_epoch = 0
        
        # Training state
        self.is_training = False
        self.training_lock = threading.Lock()
        
        # Load existing model if available
        self._load_or_create_model()
        
        logger.info(f"Trainer initialized. Monitoring: {self.data_dir}")
        logger.info(f"Model save path: {self.model_save_path}")
    
    def _load_or_create_model(self):
        """Create new model for each training session (start from ImageNet weights)."""
        logger.info("Creating new model from ImageNet pre-trained weights for each session")
        self.model = None
        self.optimizer = None
        self.class_to_idx = {}
        self.training_epoch = 0
    
    def _discover_classes(self) -> Dict[str, int]:
        """Discover classes from filenames."""
        classes = set()
        for file in self.data_dir.iterdir():
            if file.is_file() and self._is_image_file(file.name):
                filename = file.name
                class_name = filename.split('#')[0]
                classes.add(class_name)
        
        classes = sorted(list(classes))
        return {cls_name: idx for idx, cls_name in enumerate(classes)}
    
    def _count_images(self) -> int:
        """Count total number of images in data directory."""
        count = 0
        for file in self.data_dir.iterdir():
            if file.is_file() and self._is_image_file(file.name):
                count += 1
        return count
    
    def _validate_images(self) -> int:
        """Validate images and return count of valid images."""
        valid_count = 0
        invalid_files = []
        
        for file in self.data_dir.iterdir():
            if file.is_file() and self._is_image_file(file.name):
                file_path = file
                if validate_image_file(file_path):
                    valid_count += 1
                else:
                    invalid_files.append(file_path)
        
        if invalid_files:
            logger.warning(f"Found {len(invalid_files)} invalid image files:")
            for file in invalid_files[:5]:  # Show first 5
                logger.warning(f"  - {file}")
            if len(invalid_files) > 5:
                logger.warning(f"  ... and {len(invalid_files) - 5} more")
        
        return valid_count
    
    def _is_image_file(self, filename: str) -> bool:
        """Check if the file has a valid image extension."""
        valid_extensions = {'.jpg', '.jpeg', '.png', '.bmp', '.tiff', '.tif'}
        return any(filename.lower().endswith(ext) for ext in valid_extensions)
    
    def trigger_training(self):
        """Trigger training session (thread-safe)."""
        with self.training_lock:
            if self.is_training:
                logger.info("Training already in progress, skipping...")
                return
            
            self.is_training = True
        
        try:
            self._train()
        except Exception as e:
            logger.error(f"Training failed: {e}")
        finally:
            with self.training_lock:
                self.is_training = False
    
    def _train(self):
        """Perform training session."""
        logger.info("Starting training session...")
        
        # Check if we have any images
        total_images = self._count_images()
        if total_images == 0:
            logger.info("No images found, skipping training")
            return
        
        valid_images = self._validate_images()
        if valid_images == 0:
            logger.error("No valid images found, skipping training")
            return
        
        logger.info(f"Found {valid_images} valid images out of {total_images} total")
        
        # Discover classes from filenames
        self.class_to_idx = self._discover_classes()
        if not self.class_to_idx:
            logger.error("No valid classes found in image filenames")
            return
            
        logger.info(f"Discovered {len(self.class_to_idx)} classes: {list(self.class_to_idx.keys())}")
        
        # Create fresh model with the correct number of classes
        num_classes = len(self.class_to_idx)
        self.model = get_model(num_classes, self.device)
        self.optimizer = optim.Adam(self.model.parameters(), lr=self.learning_rate)
        self.training_epoch = 0
        
        # Create data loaders
        try:
            dataloader, _ = create_data_loaders(
                str(self.data_dir),
                batch_size=self.batch_size,
                num_workers=2
            )
        except Exception as e:
            logger.error(f"Error creating data loaders: {e}")
            return
        
        # Training loop
        self.model.train()
        total_loss = 0.0
        num_batches = 0
        
        logger.info(f"Training for {self.num_epochs} epoch(s)...")
        
        for epoch in range(self.num_epochs):
            epoch_loss = 0.0
            epoch_batches = 0
            
            progress_bar = tqdm(dataloader, desc=f"Epoch {epoch + 1}/{self.num_epochs}")
            
            for batch_idx, (images, labels) in enumerate(progress_bar):
                try:
                    images = images.to(self.device)
                    labels = labels.to(self.device)
                    
                    # Forward pass
                    self.optimizer.zero_grad()
                    outputs = self.model(images)
                    loss = self.criterion(outputs, labels)
                    
                    # Backward pass
                    loss.backward()
                    self.optimizer.step()
                    
                    # Update statistics
                    epoch_loss += loss.item()
                    epoch_batches += 1
                    total_loss += loss.item()
                    num_batches += 1
                    
                    # Update progress bar
                    progress_bar.set_postfix({
                        'Loss': f'{loss.item():.4f}',
                        'Avg Loss': f'{epoch_loss / max(epoch_batches, 1):.4f}'
                    })
                    
                except Exception as e:
                    logger.error(f"Error in training batch {batch_idx}: {e}")
                    continue
            
            avg_epoch_loss = epoch_loss / max(epoch_batches, 1)
            logger.info(f"Epoch {epoch + 1} completed. Average loss: {avg_epoch_loss:.4f}")
        
        # Save model
        if num_batches > 0:
            avg_loss = total_loss / num_batches
            self.training_epoch += self.num_epochs
            
            # Save model checkpoint
            save_model(
                self.model, self.optimizer, self.training_epoch, 
                avg_loss, self.class_to_idx, self.model_save_path
            )
            
            # Save model weights for federated learning
            weights_path = os.path.join(os.path.dirname(self.model_save_path), 'federated_weights.pth')
            torch.save(self.model.state_dict(), weights_path)
            
            # Save model parameters in the required format
            self._save_model_parameters()
            
            logger.info(f"Training completed. Average loss: {avg_loss:.4f}")
            logger.info(f"Model saved to {self.model_save_path}")
            logger.info(f"Model weights saved to {weights_path} for federated learning")
        else:
            logger.error("No batches were processed successfully")
    
    def _save_model_parameters(self) -> str:
        """Save model parameters to a text file in the root directory.
        
        Returns:
            str: Path to the saved parameters file
        """
        try:
            # Get the root directory (where the script is located)
            root_dir = Path(__file__).parent
            
            # Try to get edge ID from environment variable, fallback to timestamp
            edge_id = os.environ.get('EDGE_ID')
            if not edge_id:
                edge_id = f"{int(time.time())}"
                logger.warning(f"No EDGE_ID environment variable found, using timestamp as filename: {edge_id}")
            
            # Create the output filename in the root directory
            output_path = root_dir / f"{edge_id}.txt"
            
            with open(output_path, 'w') as f:
                # Iterate through all parameters in the model
                for name, param in self.model.state_dict().items():
                    # Skip num_batches_tracked in BatchNorm layers
                    if 'num_batches_tracked' in name:
                        continue
                        
                    # Get parameter values as a flat list of strings
                    values = ['{:.8f}'.format(x.item()) for x in param.view(-1)]
                    
                    # Write layer name and shape
                    shape = ' '.join(map(str, param.shape))
                    f.write(f"{name} {shape}\n")
                    
                    # Write parameter values
                    f.write(' '.join(values) + '\n')
            
            logger.info(f"Model parameters saved to {output_path}")
            return str(output_path)
            
        except Exception as e:
            logger.error(f"Error saving model parameters: {e}")
            raise
    
    def start_monitoring(self):
        """Start monitoring the data directory."""
        logger.info(f"Starting directory monitoring for {self.data_dir}")
        
        # Check for existing images and trigger training if any found
        if self._count_images() > 0:
            logger.info("Existing images found, starting initial training...")
            self.trigger_training()
        
        # Create event handler
        event_handler = ImageHandler(self)
        
        # Create observer
        observer = Observer()
        observer.schedule(event_handler, str(self.data_dir), recursive=False)
        observer.start()
        
        try:
            logger.info("Directory monitoring started. Press Ctrl+C to stop.")
            while True:
                time.sleep(1)
        except KeyboardInterrupt:
            observer.stop()
            logger.info("Stopping directory monitoring...")
        
        observer.join()
        logger.info("Directory monitoring stopped.")

def main():
    """Main function to run the trainer."""
    import argparse
    
    parser = argparse.ArgumentParser(description='Incremental MobileNetV2 Trainer')
    parser.add_argument('--data-dir', type=str, default='./received_images',
                       help='Directory to monitor for images (default: ./received_images)')
    parser.add_argument('--model-path', type=str, default='./saved_models/mobilenetv2_custom.pth',
                       help='Path to save/load model (default: ./saved_models/mobilenetv2_custom.pth)')
    parser.add_argument('--batch-size', type=int, default=16,
                       help='Batch size for training (default: 16)')
    parser.add_argument('--learning-rate', type=float, default=0.001,
                       help='Learning rate (default: 0.001)')
    parser.add_argument('--epochs', type=int, default=10,
                       help='Number of epochs per training session (default: 10)')
    parser.add_argument('--train-once', action='store_true',
                       help='Train once and exit (for testing)')
    
    args = parser.parse_args()
    
    # Create trainer
    trainer = IncrementalTrainer(
        data_dir=args.data_dir,
        model_save_path=args.model_path,
        batch_size=args.batch_size,
        learning_rate=args.learning_rate,
        num_epochs=args.epochs
    )
    
    if args.train_once:
        logger.info("Training once and exiting...")
        trainer.trigger_training()
    else:
        # Start monitoring
        trainer.start_monitoring()

if __name__ == "__main__":
    main()