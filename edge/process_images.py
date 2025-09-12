#!/usr/bin/env python3
import os
import sys
import logging
import datetime
from pathlib import Path
from PIL import Image

def setup_logging():
    """Set up basic logging configuration."""
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(levelname)s - %(message)s',
        handlers=[
            logging.StreamHandler(sys.stdout)
        ]
    )

def process_image(image_path):
    """Process a single image."""
    try:
        with Image.open(image_path) as img:
            # Example: Convert to grayscale
            # grayscale_img = img.convert('L')
            # grayscale_img.save(f"processed_{Path(image_path).name}")
            
            # For now, just log the image info
            logging.info(f"Processing image: {image_path}")
            logging.info(f"  - Size: {img.size}")
            logging.info(f"  - Mode: {img.mode}")
            logging.info(f"  - Format: {img.format if img.format else 'Unknown'}")
            
            # Add your image processing logic here
            
    except Exception as e:
        logging.error(f"Error processing {image_path}: {str(e)}")

def main():
    if len(sys.argv) < 3:
        print("Usage: python process_images.py <directory_path> <edge_id>")
        sys.exit(1)
    
    dir_path = sys.argv[1]
    edge_id = sys.argv[2]
    output_dir = os.path.join(os.path.dirname(dir_path), 'outputs')
    os.makedirs(output_dir, exist_ok=True)
    setup_logging()
    
    if not os.path.isdir(dir_path):
        logging.error(f"Directory not found: {dir_path}")
        sys.exit(1)
    
    logging.info(f"Starting to process images in directory: {dir_path}")
    
    # Get all image files in the directory
    image_extensions = {'.jpg', '.jpeg', '.png', '.bmp', '.tiff', '.webp'}
    image_files = [
        os.path.join(dir_path, f) for f in os.listdir(dir_path)
        if os.path.isfile(os.path.join(dir_path, f)) and 
           os.path.splitext(f)[1].lower() in image_extensions
    ]
    
    if not image_files:
        logging.warning("No image files found in the directory")
        return
    
    logging.info(f"Found {len(image_files)} image(s) to process")
    
    # Process each image
    processed_count = 0
    for image_path in image_files:
        process_image(image_path)
        processed_count += 1
    
    # Create a summary file with edge ID
    output_filename = f'output-{edge_id}.txt'
    output_path = os.path.join(output_dir, output_filename)
    
    with open(output_path, 'w') as f:
        f.write(f'Edge ID: {edge_id}\n')
        f.write(f'Total files processed: {processed_count}\n')
        f.write(f'Processing completed at: {datetime.datetime.now()}\n')
    
    logging.info(f"Image processing completed. Processed {processed_count} files.")
    logging.info(f"Summary saved to: {output_path}")
    
    # Print the output path for the Go code to capture
    print(f"OUTPUT_FILE:{output_path}")

if __name__ == "__main__":
    main()