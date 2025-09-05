#!/usr/bin/env python3
"""
Sample image processing script for IPD system.
This script processes images from input directory and saves results to output directory.
"""

import os
import sys
import json
import time
from pathlib import Path

def process_images(input_dir, output_dir):
    """
    Process all images in the input directory.
    
    Args:
        input_dir (str): Path to input directory
        output_dir (str): Path to output directory
    
    Returns:
        dict: Processing results
    """
    input_path = Path(input_dir)
    output_path = Path(output_dir)
    
    # Create output directory if it doesn't exist
    output_path.mkdir(parents=True, exist_ok=True)
    
    processed_files = []
    errors = []
    
    # Process each file in input directory
    for file_path in input_path.iterdir():
        if file_path.is_file():
            try:
                # Simulate image processing
                print(f"Processing: {file_path.name}")
                
                # Read input file
                with open(file_path, 'rb') as f:
                    data = f.read()
                
                # Simulate processing time
                time.sleep(0.1)
                
                # Create processed output (in real scenario, this would be actual image processing)
                processed_data = data + b" [PROCESSED]"
                
                # Write to output directory
                output_file = output_path / f"processed_{file_path.name}"
                with open(output_file, 'wb') as f:
                    f.write(processed_data)
                
                processed_files.append({
                    "input_file": str(file_path),
                    "output_file": str(output_file),
                    "size": len(processed_data)
                })
                
                print(f"✅ Processed: {file_path.name} -> {output_file.name}")
                
            except Exception as e:
                error_msg = f"Error processing {file_path.name}: {str(e)}"
                errors.append(error_msg)
                print(f"❌ {error_msg}")
    
    # Create processing summary
    summary = {
        "input_directory": str(input_path),
        "output_directory": str(output_path),
        "total_files": len(list(input_path.iterdir())),
        "processed_files": len(processed_files),
        "errors": len(errors),
        "files": processed_files,
        "error_messages": errors,
        "processing_time": time.time()
    }
    
    # Save summary to output directory
    summary_file = output_path / "processing_summary.json"
    with open(summary_file, 'w') as f:
        json.dump(summary, f, indent=2)
    
    return summary

def main():
    """Main entry point for the processing script."""
    if len(sys.argv) != 3:
        print("Usage: python process.py <input_directory> <output_directory>")
        sys.exit(1)
    
    input_dir = sys.argv[1]
    output_dir = sys.argv[2]
    
    print(f"🚀 Starting image processing...")
    print(f"📂 Input directory: {input_dir}")
    print(f"📁 Output directory: {output_dir}")
    
    start_time = time.time()
    
    try:
        results = process_images(input_dir, output_dir)
        
        end_time = time.time()
        duration = end_time - start_time
        
        print(f"\n📊 Processing Summary:")
        print(f"   Total files: {results['total_files']}")
        print(f"   Processed: {results['processed_files']}")
        print(f"   Errors: {results['errors']}")
        print(f"   Duration: {duration:.2f} seconds")
        
        if results['errors'] > 0:
            print(f"\n⚠️  Errors occurred:")
            for error in results['error_messages']:
                print(f"   - {error}")
            sys.exit(1)
        else:
            print(f"\n✅ Processing completed successfully!")
            sys.exit(0)
            
    except Exception as e:
        print(f"❌ Fatal error: {str(e)}")
        sys.exit(1)

if __name__ == "__main__":
    main()