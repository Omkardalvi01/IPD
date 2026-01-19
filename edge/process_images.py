"""
WebSocket-based Incremental training script for MobileNetV2 on edge devices.
Receives image directory paths via WebSocket and performs incremental training.
"""

import asyncio
import json
import logging
import os
import sys
import time
import uuid
from pathlib import Path
from typing import Dict, Any, Optional

import torch
import torch.nn as nn
import torch.optim as optim
import torchvision.transforms as transforms
import websockets
from torch.utils.data import DataLoader
from torchvision.models import mobilenet_v2
from tqdm import tqdm

# Add parent directory to path to allow imports from parent
sys.path.append(str(Path(__file__).parent.parent))

from edge.model_utils import (
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


class WebSocketTrainer:
    """WebSocket-based trainer class for incremental learning."""
    
    def __init__(self, model_save_path: str = "./saved_models/mobilenetv2_custom.pth",
                 batch_size: int = 32,
                 learning_rate: float = 0.001,
                 num_epochs: int = 5):
        """
        Initialize the WebSocket trainer.
        
        Args:
            model_save_path: Path to save/load model
            batch_size: Batch size for training
            learning_rate: Learning rate for optimizer
            num_epochs: Number of epochs per training session
        """
        self.model_save_path = model_save_path
        self.batch_size = batch_size
        self.learning_rate = learning_rate
        self.num_epochs = num_epochs
        
        # Create directories if they don't exist
        os.makedirs(os.path.dirname(model_save_path), exist_ok=True)
        
        # Initialize device and model
        self.device = get_device()
        self.model = None
        self.optimizer = None
        self.criterion = nn.CrossEntropyLoss()
        self.class_to_idx = {}
        self.training_epoch = 0
        
        logger.info("WebSocket trainer initialized")
        logger.info(f"Model save path: {self.model_save_path}")
    
    def _discover_classes(self, data_dir: Path) -> Dict[str, int]:
        """Discover classes from filenames."""
        classes = set()
        for file in data_dir.iterdir():
            if file.is_file() and self._is_image_file(file.name):
                filename = file.name
                # Handle both formats:
                # 1. class#image.jpg
                # 2. mnist_train_XXXXX_label_Y.jpg
                if 'label_' in filename:
                    # MNIST format: extract the label after 'label_'
                    label = filename.split('label_')[-1].split('.')[0]
                    classes.add(label)
                elif '#' in filename:
                    # Original format: class#image.jpg
                    class_name = filename.split('#')[0]
                    classes.add(class_name)
        
        classes = sorted(list(classes))
        logger.info(f"Discovered classes: {classes}")
        return {cls_name: idx for idx, cls_name in enumerate(classes)}
    
    def _count_images(self, data_dir: Path) -> int:
        """Count total number of images in data directory."""
        count = 0
        for file in data_dir.iterdir():
            if file.is_file() and self._is_image_file(file.name):
                count += 1
        return count
    
    def _validate_images(self, data_dir: Path) -> int:
        """Validate images and return count of valid images."""
        valid_count = 0
        invalid_files = []
        
        for file in data_dir.iterdir():
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
    
    async def train_on_images(self, data_dir: str) -> Dict[str, Any]:
        """
        Perform training session on images in the given directory.
        
        Args:
            data_dir: Path to directory containing images
            
        Returns:
            Dict containing training results and output file path
        """
        try:
            logger.info(f"Starting training session on directory: {data_dir}")
            data_path = Path(data_dir)
            
            if not data_path.exists():
                error_msg = f"Directory {data_dir} does not exist"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
            
            # Check if we have any images
            total_images = self._count_images(data_path)
            if total_images == 0:
                error_msg = "No images found in directory"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
            
            valid_images = self._validate_images(data_path)
            if valid_images == 0:
                error_msg = "No valid images found in directory"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
            
            logger.info(f"Found {valid_images} valid images out of {total_images} total")
            
            # Discover classes from filenames
            self.class_to_idx = self._discover_classes(data_path)
            if not self.class_to_idx:
                error_msg = "No valid classes found in image filenames"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
                
            logger.info(f"Discovered {len(self.class_to_idx)} classes: {list(self.class_to_idx.keys())}")
            
            # Create fresh model or load existing one
            num_classes = len(self.class_to_idx)
            
            if os.path.exists(self.model_save_path):
                try:
                    logger.info(f"Loading existing model from {self.model_save_path}")
                    self.model, self.optimizer, loaded_class_to_idx, _ = load_model(self.model_save_path, self.device, freeze_features=True)
                    
                    current_out_features = self.model.classifier[1].out_features
                    if current_out_features != num_classes:
                        logger.warning(f"Model class count mismatch (Saved: {current_out_features}, New: {num_classes}). Resetting model.")
                        self.model = get_model(num_classes, self.device, freeze_features=True)
                        self.optimizer = optim.Adam(filter(lambda p: p.requires_grad, self.model.parameters()), lr=self.learning_rate)
                    else:
                        projected_class_names = sorted(list(self.class_to_idx.keys()))
                        loaded_class_names = sorted(list(loaded_class_to_idx.keys()))
                        if projected_class_names != loaded_class_names:
                             logger.warning(f"Class names mismatch. Resetting model to avoid label confusion.")
                             self.model = get_model(num_classes, self.device, freeze_features=True)
                             self.optimizer = optim.Adam(filter(lambda p: p.requires_grad, self.model.parameters()), lr=self.learning_rate)
                        
                except Exception as e:
                    logger.error(f"Failed to load existing model: {e}. Starting fresh.")
                    self.model = get_model(num_classes, self.device, freeze_features=True)
                    self.optimizer = optim.Adam(filter(lambda p: p.requires_grad, self.model.parameters()), lr=self.learning_rate)
            else:
                logger.info("No existing model found. Starting fresh.")
                self.model = get_model(num_classes, self.device, freeze_features=True)
                self.optimizer = optim.Adam(filter(lambda p: p.requires_grad, self.model.parameters()), lr=self.learning_rate)

            self.training_epoch = 0 # In a real scenario, we might want to continue epoch count

            
            # Create data loaders
            try:
                dataloader, _ = create_data_loaders(
                    str(data_path),
                    batch_size=self.batch_size,
                    num_workers=2,
                    augment=True
                )
            except Exception as e:
                error_msg = f"Error creating data loaders: {e}"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
            
            # Training loop
            self.model.train()
            total_loss = 0.0
            num_batches = 0
            
            logger.info(f"Training for {self.num_epochs} epoch(s)...")
            
            for epoch in range(self.num_epochs):
                epoch_loss = 0.0
                epoch_batches = 0
                
                logger.info(f"Starting epoch {epoch + 1}/{self.num_epochs}")
                
                for batch_idx, (images, labels) in enumerate(dataloader):
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
                        
                        # Yield control to event loop to keep WebSocket connection alive
                        if batch_idx % 5 == 0:
                            await asyncio.sleep(0)
                        
                        if batch_idx % 10 == 0:  # Log every 10 batches
                            logger.info(f"Epoch {epoch + 1}, Batch {batch_idx}: Loss = {loss.item():.4f}")
                        
                    except Exception as e:
                        logger.error(f"Error in training batch {batch_idx}: {e}")
                        continue
                
                avg_epoch_loss = epoch_loss / max(epoch_batches, 1)
                logger.info(f"Epoch {epoch + 1} completed. Average loss: {avg_epoch_loss:.4f}")
            
            # Save model and return results
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
                output_file_path = self._save_model_parameters()
                
                logger.info(f"Training completed. Average loss: {avg_loss:.4f}")
                logger.info(f"Model saved to {self.model_save_path}")
                logger.info(f"Model weights saved to {weights_path}")
                logger.info(f"Model parameters saved to {output_file_path}")
                
                return {
                    "success": True,
                    "output_file_path": output_file_path,
                    "average_loss": avg_loss,
                    "epochs_trained": self.num_epochs,
                    "total_batches": num_batches,
                    "num_classes": num_classes,
                    "valid_images": valid_images
                }
            else:
                error_msg = "No batches were processed successfully"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
                
        except Exception as e:
            error_msg = f"Training failed with exception: {str(e)}"
            logger.error(error_msg, exc_info=True)
            return {"success": False, "error": error_msg}
    
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
                # Write Weight Percentage Header if available
                percentage = os.environ.get('ALLOCATED_PERCENTAGE')
                if percentage:
                    f.write(f"# WEIGHT_PERCENTAGE: {percentage}\n")
                
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


async def handle_client(websocket, path=None):
    """Handle incoming WebSocket connections.
    
    Args:
        websocket: The WebSocket connection object
        path: The WebSocket path (optional, for compatibility with older versions)
    """
    # Try to get path from different sources depending on websockets version
    try:
        # For newer versions, try to get from request
        if hasattr(websocket, 'request'):
            connection_path = websocket.request.path
        elif hasattr(websocket, 'path'):
            connection_path = websocket.path
        elif path is not None:
            connection_path = path
        else:
            connection_path = "/"  # default path
    except AttributeError:
        connection_path = "/"  # fallback
    
    client_info = {"type": "trainer", "client_id": None, "path": connection_path}
    trainer = WebSocketTrainer()
    logger.info(f"New connection on path: {connection_path}")
    
    try:
        logger.info(f"New client connected from {websocket.remote_address}")
        
        async for message in websocket:
            try:
                # Parse the message as JSON containing both directory and edge_id
                try:
                    data = json.loads(message)
                    data_dir = data.get('data_dir', '').strip()
                    edge_id = data.get('edge_id', '').strip()
                except json.JSONDecodeError:
                    # Fallback: treat as plain text directory path
                    data_dir = message.strip()
                    edge_id = None
                
                logger.info(f"Received directory path: {data_dir}, edge_id: {edge_id}")
                
                # Set edge_id in environment if provided
                if edge_id:
                    os.environ['EDGE_ID'] = edge_id
                elif not os.environ.get('EDGE_ID'):
                    # If no edge_id provided and none in environment, generate one
                    edge_id = str(uuid.uuid4())
                    os.environ['EDGE_ID'] = edge_id
                    logger.warning(f"No edge_id provided, generated new one: {edge_id}")

                # Handle percentage
                percentage = data.get('percentage')
                if percentage is not None:
                    os.environ['ALLOCATED_PERCENTAGE'] = str(percentage)
                    logger.info(f"Set allocated percentage: {percentage}%")
                
                if not data_dir:
                    error_msg = "ERROR: Empty directory path received"
                    logger.error(error_msg)
                    await websocket.send(json.dumps({"error": error_msg, "success": False}))
                    continue
                
                # Perform training
                logger.info(f"Starting training on directory: {data_dir}")
                result = await trainer.train_on_images(data_dir)
                
                # Add edge_id to the result
                result["edge_id"] = os.environ.get('EDGE_ID')
                
                # Send response as JSON
                await websocket.send(json.dumps(result))
                    
            except Exception as e:
                error_msg = f"ERROR: Internal error - {str(e)}"
                logger.error(f"Error processing message: {e}", exc_info=True)
                await websocket.send(error_msg)
                
    except websockets.exceptions.ConnectionClosedOK:
        logger.info(f"Client {client_info} disconnected gracefully")
    except websockets.exceptions.ConnectionClosedError as e:
        logger.warning(f"Client {client_info} disconnected with error: {e}")
    except Exception as e:
        logger.error(f"Unexpected error with client {client_info}: {e}", exc_info=True)
    finally:
        logger.info(f"Client {client_info} connection closed")


async def main():
    """Main function to start the WebSocket server."""
    import argparse
    
    parser = argparse.ArgumentParser(description='WebSocket Incremental MobileNetV2 Trainer')
    parser.add_argument('--host', type=str, default='localhost',
                       help='Host to bind server to (default: localhost)')
    parser.add_argument('--port', type=int, default=8765,
                       help='Port to bind server to (default: 8765)')
    parser.add_argument('--model-path', type=str, default='./saved_models/mobilenetv2_custom.pth',
                       help='Path to save/load model (default: ./saved_models/mobilenetv2_custom.pth)')
    
    args = parser.parse_args()
    
    logger.info(f"Starting WebSocket trainer server on {args.host}:{args.port}")
    logger.info(f"Model will be saved to: {args.model_path}")
    
    # Start the WebSocket server with proper exception handling
    try:
        # Increase timeouts to handle long training times (default is usually ~20s)
        async with websockets.serve(handle_client, args.host, args.port, ping_interval=60, ping_timeout=60):
            logger.info("WebSocket server started. Waiting for connections...")
            logger.info("Press Ctrl+C to stop the server")
            
            # Keep the server running
            try:
                await asyncio.Future()  # run forever
            except KeyboardInterrupt:
                logger.info("Received keyboard interrupt, shutting down...")
                
    except Exception as e:
        logger.error(f"Failed to start WebSocket server: {e}")
        raise


if __name__ == "__main__":
    asyncio.run(main())