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
    save_model, load_model, validate_image_file,
    natural_sort_key
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
    
    def _is_image_file(self, filename: str) -> bool:
        """Check if the file has a valid image extension."""
        valid_extensions = {'.jpg', '.jpeg', '.png', '.bmp', '.tiff', '.tif'}
        return any(filename.lower().endswith(ext) for ext in valid_extensions)
    
    async def train_on_images(self, data_dir: str, hyperparams: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Perform training session on images in the given directory.
        
        Args:
            data_dir: Path to directory containing images
            hyperparams: Optional dictionary of hyperparameters (epochs, batch_size, lr, model_path)
            
        Returns:
            Dict containing training results and output file path
        """
        try:
            # Update hyperparameters if provided
            if hyperparams:
                if hyperparams.get('batch_size') is not None:
                    self.batch_size = int(hyperparams['batch_size'])
                if hyperparams.get('learning_rate') is not None:
                    self.learning_rate = float(hyperparams['learning_rate'])
                if hyperparams.get('epochs') is not None:
                    self.num_epochs = int(hyperparams['epochs'])
                if hyperparams.get('num_classes') is not None:
                    logger.info(f"Using forced num_classes from hyperparams: {hyperparams['num_classes']}")
                
                logger.info(f"Using hyperparams: Batch={self.batch_size}, LR={self.learning_rate}, Epochs={self.num_epochs}")
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
            
            # Initialize model
            # Use num_classes from hyperparams if provided, otherwise discover from local data
            forced_classes = hyperparams.get('num_classes') if hyperparams else None
            num_classes = int(forced_classes) if forced_classes is not None else len(self.class_to_idx)
            
            logger.info(f"INITIALIZING MODEL with {num_classes} classes")
            self.model = get_model(num_classes, self.device)
            
            # Load initial weights if provided
            if hyperparams and 'model_path' in hyperparams and hyperparams['model_path']:
                model_path = hyperparams['model_path']
                if os.path.exists(model_path):
                    logger.info(f"LOADING GLOBAL WEIGHTS from: {model_path}")
                    try:
                        checkpoint = torch.load(model_path, map_location=self.device)
                        
                        # Handle different checkpoint formats
                        if isinstance(checkpoint, dict):
                            if 'model_state_dict' in checkpoint:
                                state_dict = checkpoint['model_state_dict']
                            elif 'state_dict' in checkpoint:
                                state_dict = checkpoint['state_dict']
                            else:
                                state_dict = checkpoint
                        else:
                            state_dict = checkpoint
                            
                        # Enforce STRICT loading for global weights
                        self.model.load_state_dict(state_dict, strict=True)
                        logger.info("✅ SUCCESSFUL GLOBAL WEIGHT SYNC: Model is now using the latest broadcasted weights (STRICT MODE).")
                        
                    except Exception as e:
                        logger.warning(f"Failed to load initial weights: {e}")
                else:
                    logger.warning(f"Initial model path provided but not found: {model_path}")

            self.optimizer = optim.Adam(self.model.parameters(), lr=self.learning_rate)

            self.training_epoch = 0 # In a real scenario, we might want to continue epoch count

            
            # Create data loaders
            try:
                dataloader, _ = create_data_loaders(
                    str(data_path),
                    batch_size=self.batch_size,
                    num_workers=2
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
    
    async def train_on_images(self, data_dir: str, hyperparams: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Perform training session on images in the given directory.
        
        Args:
            data_dir: Path to directory containing images
            hyperparams: Optional dictionary of hyperparameters (epochs, batch_size, lr, model_path)
            
        Returns:
            Dict containing training results and output file path
        """
        try:
            # Update hyperparameters if provided
            if hyperparams:
                if hyperparams.get('batch_size') is not None:
                    self.batch_size = int(hyperparams['batch_size'])
                if hyperparams.get('learning_rate') is not None:
                    self.learning_rate = float(hyperparams['learning_rate'])
                if hyperparams.get('epochs') is not None:
                    self.num_epochs = int(hyperparams['epochs'])
                
                logger.info(f"Using hyperparams: Batch={self.batch_size}, LR={self.learning_rate}, Epochs={self.num_epochs}")
            
            logger.info(f"Starting training session on directory: {data_dir}")
            data_path = Path(data_dir)
            
            if not data_path.exists():
                error_msg = f"Directory {data_dir} does not exist"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
            
            # [Optimization] Consolidated scanning: count, validate, and discover classes in ONE pass
            logger.info(f"Scanning dataset in {data_dir}...")
            valid_images = 0
            total_images = 0
            discovered_classes = set()
            
            for file in data_path.iterdir():
                if file.is_file() and self._is_image_file(file.name):
                    total_images += 1
                    if validate_image_file(file):
                        valid_images += 1
                        filename = file.name
                        if 'label_' in filename:
                            label = filename.split('label_')[-1].split('.')[0]
                            discovered_classes.add(label)
                        elif '#' in filename:
                            class_name = filename.split('#')[0]
                            discovered_classes.add(class_name)
            
            if valid_images == 0:
                error_msg = "No valid images found in directory"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
            
            logger.info(f"Found {valid_images} valid images out of {total_images} total")
            
            # Sort classes naturally
            sorted_classes = sorted(list(discovered_classes), key=natural_sort_key)
            self.class_to_idx = {cls_name: idx for idx, cls_name in enumerate(sorted_classes)}
            
            if not self.class_to_idx:
                error_msg = "No valid classes found in image filenames"
                logger.error(error_msg)
                return {"success": False, "error": error_msg}
                
            logger.info(f"Discovered {len(self.class_to_idx)} classes: {list(self.class_to_idx.keys())}")
            
            # Initialize model
            forced_classes = hyperparams.get('num_classes') if hyperparams else None
            num_classes = int(forced_classes) if forced_classes is not None else len(self.class_to_idx)
            
            logger.info(f"INITIALIZING MODEL with {num_classes} classes")
            self.model = get_model(num_classes, self.device)
            
            # Load initial weights if provided
            if hyperparams and 'model_path' in hyperparams and hyperparams['model_path']:
                model_path = hyperparams['model_path']
                if os.path.exists(model_path):
                    logger.info(f"LOADING GLOBAL WEIGHTS from: {model_path}")
                    try:
                        checkpoint = torch.load(model_path, map_location=self.device)
                        state_dict = checkpoint.get('model_state_dict', checkpoint.get('state_dict', checkpoint))
                        self.model.load_state_dict(state_dict, strict=True)
                        logger.info("✅ SUCCESSFUL GLOBAL WEIGHT SYNC (STRICT MODE).")
                    except Exception as e:
                        logger.warning(f"Failed to load initial weights: {e}")
            
            self.optimizer = optim.Adam(self.model.parameters(), lr=self.learning_rate)
            self.model.train()
            
            # Create data loaders
            dataloader, _ = create_data_loaders(str(data_path), batch_size=self.batch_size, num_workers=2)
            
            # Training loop
            total_loss = 0.0
            num_batches = 0
            for epoch in range(self.num_epochs):
                epoch_loss = 0.0
                epoch_batches = 0
                logger.info(f"Starting epoch {epoch + 1}/{self.num_epochs}")
                
                for batch_idx, (images, labels) in enumerate(dataloader):
                    images, labels = images.to(self.device), labels.to(self.device)
                    self.optimizer.zero_grad()
                    outputs = self.model(images)
                    loss = self.criterion(outputs, labels)
                    loss.backward()
                    self.optimizer.step()
                    
                    epoch_loss += loss.item()
                    epoch_batches += 1
                    total_loss += loss.item()
                    num_batches += 1
                    
                    if batch_idx % 5 == 0:
                        await asyncio.sleep(0)  # Keep WebSocket alive
                
                logger.info(f"Epoch {epoch + 1} completed. Avg loss: {epoch_loss/max(epoch_batches, 1):.4f}")
            
            # Save results
            if num_batches > 0:
                avg_loss = total_loss / num_batches
                output_file_path = self._save_model_parameters()
                return {
                    "success": True, 
                    "output_file_path": output_file_path,
                    "average_loss": avg_loss,
                    "num_classes": num_classes,
                    "valid_images": valid_images
                }
            return {"success": False, "error": "No batches processed"}
                
        except Exception as e:
            logger.error(f"Training failed: {e}", exc_info=True)
            return {"success": False, "error": str(e)}

    def _save_model_parameters(self) -> str:
        """Save model parameters to a text file using memory-efficient streaming."""
        import gc
        try:
            root_dir = Path(__file__).parent
            edge_id = os.environ.get('EDGE_ID', f"{int(time.time())}")
            output_path = root_dir / f"{edge_id}.txt"
            
            logger.info("Serializing model parameters to disk (streaming mode)...")
            with open(output_path, 'w') as f:
                percentage = os.environ.get('ALLOCATED_PERCENTAGE')
                if percentage:
                    f.write(f"# WEIGHT_PERCENTAGE: {percentage}\n")
                
                for name, param in self.model.state_dict().items():
                    if 'num_batches_tracked' in name:
                        continue
                    
                    shape_str = ' '.join(map(str, param.shape))
                    f.write(f"{name} {shape_str}\n")
                    
                    # Stream values in chunks to keep RAM usage constant
                    param_flat = param.view(-1).cpu().numpy()
                    chunk_size = 5000 
                    for i in range(0, len(param_flat), chunk_size):
                        chunk = param_flat[i : i + chunk_size]
                        f.write(' '.join(['{:.8f}'.format(x) for x in chunk]) + ' ')
                    f.write('\n')
            
            # Clean up memory immediately
            gc.collect()
            if torch.cuda.is_available():
                torch.cuda.empty_cache()
                
            logger.info(f"Model parameters successfully saved to {output_path}")
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
                    
                    # Extract hyperparameters safely
                    hyperparams = {}
                    for key in ['batch_size', 'learning_rate', 'epochs', 'model_path', 'num_classes']:
                        val = data.get(key)
                        if val is not None:
                            hyperparams[key] = val
                except json.JSONDecodeError:
                    # Fallback: treat as plain text directory path
                    data_dir = message.strip()
                    edge_id = None
                    hyperparams = None
                
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
                result = await trainer.train_on_images(data_dir, hyperparams)
                
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
        async with websockets.serve(handle_client, args.host, args.port, ping_interval=300, ping_timeout=300):
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