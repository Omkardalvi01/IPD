import sys
import os
import shutil
import unittest
from pathlib import Path
import torch
from PIL import Image

# Add parent directory to path
sys.path.append(str(Path(__file__).parent.parent))

from edge.model_utils import CustomImageDataset

class TestClassMapping(unittest.TestCase):
    def setUp(self):
        self.test_dir = Path("temp_test_data")
        os.makedirs(self.test_dir, exist_ok=True)
        
        # Create dummy images for Class 0 and Class 5 ONLY
        # This simulates a client having only a subset of classes
        self._create_dummy_image("0#img1.jpg")
        self._create_dummy_image("5#img2.jpg")
        
        # Define Global Mapping
        self.global_mapping = {str(i): i for i in range(10)}
        
    def tearDown(self):
        if hasattr(self, 'test_dir') and self.test_dir.exists():
            shutil.rmtree(self.test_dir)

    def _create_dummy_image(self, name):
        img = Image.new('RGB', (32, 32), color='red')
        img.save(self.test_dir / name)

    def test_global_mapping_enforcement(self):
        print("\nTesting Global Mapping Enforcement...")
        
        # Initialize dataset with GLOBAL mapping
        dataset = CustomImageDataset(self.test_dir, class_to_idx=self.global_mapping)
        
        # Gather loaded labels
        loaded_labels = []
        for i in range(len(dataset)):
            _, label = dataset[i]
            loaded_labels.append(label)
            
        print(f"Loaded Labels: {loaded_labels}")
        
        # Verification:
        # We MUST have labels 0 and 5.
        # We MUST NOT have label 1 (which would happen if we used local sorting: 0->0, 5->1)
        self.assertIn(0, loaded_labels)
        self.assertIn(5, loaded_labels)
        self.assertNotIn(1, loaded_labels)
        
        print("SUCCESS: Labels are correctly mapped to global indices (0 and 5), not locally compressed (0 and 1).")

if __name__ == '__main__':
    unittest.main()
