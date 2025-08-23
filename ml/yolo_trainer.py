import sys
import json
import time
import numpy as np
import os
import shutil
import yaml
import torch
from ultralytics import YOLO
from PIL import Image
import base64
import pickle
import glob

class YOLOTrainer:
    def __init__(self, batch_size=8, learning_rate=0.001, epochs_per_batch=1):
        # Load pre-trained YOLOv8n model
        self.model = YOLO('yolov8n.pt')  # Lightweight, fast, accurate
        
        # Setup training parameters
        self.batch_size = batch_size
        self.learning_rate = learning_rate
        self.epochs_per_batch = epochs_per_batch
        
        # Create training directories
        self.setup_training_dirs()
        
        # Initialize batch storage
        self.current_batch = []
        self.batch_annotations = []
    
    def setup_training_dirs(self):
        """Create directory structure for YOLO training"""
        # Create main training directories
        os.makedirs('yolo_training/images/train', exist_ok=True)
        os.makedirs('yolo_training/images/val', exist_ok=True)
        os.makedirs('yolo_training/labels/train', exist_ok=True)
        os.makedirs('yolo_training/labels/val', exist_ok=True)
        
        # Create data.yaml file with proper YOLO format
        data_config = {
            'train': 'yolo_training/images/train',
            'val': 'yolo_training/images/val',
            'nc': 80,  # number of classes (COCO)
            'names': [
                'person', 'bicycle', 'car', 'motorcycle', 'airplane', 'bus', 'train', 'truck', 'boat',
                'traffic light', 'fire hydrant', 'stop sign', 'parking meter', 'bench', 'bird', 'cat',
                'dog', 'horse', 'sheep', 'cow', 'elephant', 'bear', 'zebra', 'giraffe', 'backpack',
                'umbrella', 'handbag', 'tie', 'suitcase', 'frisbee', 'skis', 'snowboard', 'sports ball',
                'kite', 'baseball bat', 'baseball glove', 'skateboard', 'surfboard', 'tennis racket',
                'bottle', 'wine glass', 'cup', 'fork', 'knife', 'spoon', 'bowl', 'banana', 'apple',
                'sandwich', 'orange', 'broccoli', 'carrot', 'hot dog', 'pizza', 'donut', 'cake',
                'chair', 'couch', 'potted plant', 'bed', 'dining table', 'toilet', 'tv', 'laptop',
                'mouse', 'remote', 'keyboard', 'cell phone', 'microwave', 'oven', 'toaster', 'sink',
                'refrigerator', 'book', 'clock', 'vase', 'scissors', 'teddy bear', 'hair drier', 'toothbrush'
            ]
        }
        
        with open('yolo_training/data.yaml', 'w') as f:
            yaml.dump(data_config, f, default_flow_style=False)
    
    def add_image_to_batch(self, image_path, image_id):
        """Add image to current batch and trigger training if batch is full"""
        # Copy image to training directory
        train_image_path = os.path.join('yolo_training/images/train', image_id)
        shutil.copy2(image_path, train_image_path)
        
        # Generate pseudo-annotations using current model
        annotation_content = self.generate_pseudo_annotations(image_path)
        
        # Create corresponding annotation file
        label_filename = os.path.splitext(image_id)[0] + '.txt'
        label_path = os.path.join('yolo_training/labels/train', label_filename)
        
        with open(label_path, 'w') as f:
            f.write(annotation_content)
        
        # Add to current batch
        image_info = {
            'path': train_image_path,
            'id': image_id,
            'label_path': label_path
        }
        self.current_batch.append(image_info)
        
        # If batch is full, trigger training
        if len(self.current_batch) >= self.batch_size:
            return self.train_current_batch()
        
        return None
    
    def generate_pseudo_annotations(self, image_path):
        """Generate pseudo-annotations using current model inference"""
        try:
            # Run current model inference
            results = self.model.predict(image_path, verbose=False)
            
            annotation_lines = []
            
            for result in results:
                if result.boxes is not None:
                    boxes = result.boxes.xyxyn  # Normalized coordinates
                    classes = result.boxes.cls
                    
                    for box, cls in zip(boxes, classes):
                        # Convert to YOLO format: class x_center y_center width height (normalized 0-1)
                        x1, y1, x2, y2 = box.cpu().numpy()
                        x_center = (x1 + x2) / 2.0
                        y_center = (y1 + y2) / 2.0
                        width = x2 - x1
                        height = y2 - y1
                        
                        # Ensure values are within 0-1 range
                        x_center = max(0.0, min(1.0, x_center))
                        y_center = max(0.0, min(1.0, y_center))
                        width = max(0.0, min(1.0, width))
                        height = max(0.0, min(1.0, height))
                        
                        annotation_lines.append(f"{int(cls)} {x_center:.6f} {y_center:.6f} {width:.6f} {height:.6f}")
            
            return '\n'.join(annotation_lines)
            
        except Exception as e:
            # Return empty annotations if inference fails
            return ""
    
    def train_current_batch(self):
        """Train on current batch of images"""
        start_time = time.time()
        
        # Validate batch has minimum required images
        if len(self.current_batch) < 4:
            return {
                'error': 'Batch too small for training',
                'batch_size': len(self.current_batch),
                'min_required': 4
            }
        
        try:
            # Create validation split (20% of batch)
            val_count = max(1, len(self.current_batch) // 5)
            val_indices = np.random.choice(len(self.current_batch), val_count, replace=False)
            
            # Move validation images and labels
            for idx in val_indices:
                image_info = self.current_batch[idx]
                
                # Move image to validation
                val_image_path = os.path.join('yolo_training/images/val', os.path.basename(image_info['path']))
                shutil.move(image_info['path'], val_image_path)
                
                # Move label to validation
                val_label_path = os.path.join('yolo_training/labels/val', os.path.basename(image_info['label_path']))
                shutil.move(image_info['label_path'], val_label_path)
            
            # Call YOLO training
            results = self.model.train(
                data='yolo_training/data.yaml',
                epochs=self.epochs_per_batch,
                batch=len(self.current_batch),
                imgsz=640,
                device='cpu',  # Use CPU for edge compatibility
                verbose=False,
                save=True,
                project='yolo_training',
                name='edge_training'
            )
            
            training_time = time.time() - start_time
            
            # Save updated model weights
            weights_filename = f'yolo_weights_{int(time.time())}.pt'
            weights_path = self.model.save(weights_filename)
            
            # Extract training metrics
            loss = float(results.results_dict.get('train/box_loss', 0.0))
            mAP = float(results.results_dict.get('metrics/mAP50(B)', 0.0))
            
            # Encode model state for transmission
            model_state = base64.b64encode(pickle.dumps(self.model.state_dict())).decode()
            
            # Clear current batch
            self.current_batch = []
            
            return {
                'batch_completed': True,
                'training_loss': loss,
                'validation_map': mAP,
                'training_time': training_time,
                'model_weights': model_state,
                'weights_file_path': weights_path,
                'batch_size': len(self.current_batch),
                'error': None
            }
            
        except Exception as e:
            return {
                'error': str(e),
                'batch_completed': False,
                'training_time': time.time() - start_time
            }
    
    def process_single_image(self, image_path, image_id):
        """Process a single image and add to batch"""
        result = self.add_image_to_batch(image_path, image_id)
        
        if result is None:
            # Still collecting batch
            return {
                'batch_completed': False,
                'current_batch_size': len(self.current_batch),
                'max_batch_size': self.batch_size,
                'status': 'Image added to batch'
            }
        else:
            # Batch was completed and training occurred
            return result

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(json.dumps({'error': 'Usage: python yolo_trainer.py <image_path> <image_id>'}))
        sys.exit(1)
    
    image_path = sys.argv[1]
    image_id = sys.argv[2]
    
    # Create YOLOTrainer instance with optimal parameters for edge training
    trainer = YOLOTrainer(batch_size=8, learning_rate=0.001, epochs_per_batch=1)
    
    # Process the image
    result = trainer.process_single_image(image_path, image_id)
    
    # Print JSON result to stdout
    print(json.dumps(result)) 