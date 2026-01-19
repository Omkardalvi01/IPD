import os
import numpy as np
import torch
from torchvision import datasets, transforms
from PIL import Image


def download_mnist_as_jpg(output_dir='mnist_jpg', image_size=(28, 28), train=True, max_images=2000):
    """
    Download MNIST dataset and save as JPG files.

    Args:
        output_dir (str): Directory to save the images
        image_size (tuple): Desired size of the output images (width, height)
        train (bool): If True, download training set, else test set
        max_images (int, optional): Maximum number of images to download. If None, download all.
    """
    # Create output directory if it doesn't exist
    os.makedirs(output_dir, exist_ok=True)

    # Define transformations
    transform = transforms.Compose([
        transforms.Resize(image_size, interpolation=transforms.InterpolationMode.BICUBIC),
        transforms.ToTensor(),
    ])

    # Download dataset
    dataset = datasets.MNIST(
        root='./data',
        train=train,
        download=True,
        transform=transform
    )

    # Limit number of images if specified
    if max_images is not None:
        total_images = min(max_images, len(dataset))
        print(f"Downloading {total_images} images...")
    else:
        total_images = len(dataset)
        print(f"Downloading all {total_images} images...")

    # Save images
    for idx in range(total_images):
        image, label = dataset[idx]

        # Convert tensor to PIL Image
        img = transforms.ToPILImage()(image)

        # Save as JPG
        filename = f"{output_dir}/mnist_{'train' if train else 'test'}_{idx:05d}_label_{label}.jpg"
        img.save(filename, 'JPEG', quality=95)

        # Print progress
        if (idx + 1) % 100 == 0 or (idx + 1) == total_images:
            print(f"Processed {idx + 1}/{total_images} images ({(idx + 1) / total_images * 100:.1f}%)")

    print(f"\nSuccessfully saved {total_images} images to {output_dir}/")


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description='Download MNIST dataset as JPG files')
    parser.add_argument('--output_dir', type=str, default='mnist_jpg',
                        help='Directory to save the images (default: mnist_jpg)')
    parser.add_argument('--width', type=int, default=28,
                        help='Width of output images (default: 28)')
    parser.add_argument('--height', type=int, default=28,
                        help='Height of output images (default: 28)')
    parser.add_argument('--test', action='store_true',
                        help='Download test set instead of training set')
    parser.add_argument('--max_images', type=int, default=None,
                        help='Maximum number of images to download (default: all)')

    args = parser.parse_args()

    download_mnist_as_jpg(
        output_dir=args.output_dir,
        image_size=(args.width, args.height),
        train=not args.test,
        max_images=args.max_images
    )

