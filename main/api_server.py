from flask import Flask, request, jsonify
import os
import sys
from datetime import datetime
from pathlib import Path

# Add parent directories to path to allow both direct and module imports
project_root = Path(__file__).parent.parent.parent
sys.path.append(str(project_root))

# Import local modules
try:
    from IPD.aggregator.aggregate import (
        load_client_payloads,
        fedavg_aggregate,
        save_global_model
    )
    from IPD.aggregator.validate_weights import validate_edge_weights
except ImportError:
    # Fallback for direct script execution
    from aggregate import (
        load_client_payloads,
        fedavg_aggregate,
        save_global_model
    )
    from validate_weights import validate_edge_weights

app = Flask(__name__)

# Directory configuration
BASE_DIR = Path(__file__).parent
OUTPUT_DIR = BASE_DIR / "outputs"
GLOBAL_WEIGHTS_DIR = BASE_DIR / "global_models"

# Create directories if they don't exist
os.makedirs(OUTPUT_DIR, exist_ok=True)
os.makedirs(GLOBAL_WEIGHTS_DIR, exist_ok=True)

@app.route('/check_status', methods=['GET'])
def check_status():
    """Health check endpoint"""
    return jsonify({
        "status": "ok",
        "message": "API server is running",
        "output_dir": str(OUTPUT_DIR),
        "global_weights_dir": str(GLOBAL_WEIGHTS_DIR)
    }), 200

@app.route('/receive_uids', methods=['POST'])
def receive_uids():
    """
    Receive UIDs of clients and check if their weight files exist
    """
    try:
        data = request.get_json()
        if not data or 'uids' not in data:
            return jsonify({
                "status": "error",
                "message": "Missing uids in request",
                "count": 0
            }), 400

        uids = data['uids']
        
        # Check which weight files exist
        existing_files = []
        missing_files = []
        for uid in uids:
            weight_file = os.path.join(OUTPUT_DIR, f"{uid}.txt")
            if os.path.exists(weight_file):
                existing_files.append(uid)
            else:
                missing_files.append(uid)

        # Get weight files for the specified UIDs
        weight_files = [os.path.join(OUTPUT_DIR, f"{uid}.txt") for uid in uids]
        
        # Validate all weight files
        # print("Validating edge weight files...") # logging suppressed
        success, client_weights = validate_edge_weights(weight_files)
        if not success:
            return jsonify({"error": "Weight validation failed"}), 400

        # Augment client_weights with 'factor' (percentage) from the files
        # validate_edge_weights only returns tensors, we need to re-read headers
        augmented_weights = {}
        for uid in uids:
            if uid in client_weights:
                weight_file = os.path.join(OUTPUT_DIR, f"{uid}.txt")
                percentage = 1.0
                try:
                    with open(weight_file, 'r') as f:
                        first_line = f.readline()
                        if first_line.startswith("# WEIGHT_PERCENTAGE:"):
                            percentage = float(first_line.split(":")[1].strip())
                except:
                    pass # Keep default
                
                augmented_weights[uid] = {
                    'weights': client_weights[uid],
                    'factor': percentage
                }

        # Perform aggregation
        global_weights = fedavg_aggregate(augmented_weights)

        # Create timestamped directory for this aggregation
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        aggregation_dir = os.path.join(GLOBAL_WEIGHTS_DIR, f"aggregation_{timestamp}")
        os.makedirs(aggregation_dir, exist_ok=True)

        # Save the aggregated weights and get the saved paths
        model_info = save_global_model(global_weights, aggregation_dir)
        
        # Get the full path to the saved model file
        model_path = model_info.get('model_path', '')
        
        return jsonify({
            "status": "success",
            "message": "Aggregation completed successfully",
            "timestamp": timestamp,
            "num_clients": len(client_weights),
            "model_path": model_path,
            "model_files": {
                "pytorch_model": model_path,
                "metadata": model_path.replace('.pth', '.json'),
                "directory": os.path.dirname(model_path)
            }
        }), 200

    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/available_clients', methods=['GET'])
def get_available_clients():
    """
    Get list of available client weights
    """
    try:
        clients = [f.replace('.txt', '') for f in os.listdir(OUTPUT_DIR) if f.endswith('.txt')]
        return jsonify({
            "clients": clients,
            "count": len(clients)
        }), 200
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/global_model/latest', methods=['GET'])
def get_latest_global_model():
    """
    Get the path to the most recent global model
    """
    try:
        # Check if global_models directory exists and has files
        if not os.path.exists(GLOBAL_WEIGHTS_DIR) or not os.listdir(GLOBAL_WEIGHTS_DIR):
            return jsonify({
                "status": "not_found",
                "message": "No global models available"
            }), 404
        
        # Get all model directories and sort by modification time (newest first)
        model_dirs = [
            os.path.join(GLOBAL_WEIGHTS_DIR, d) 
            for d in os.listdir(GLOBAL_WEIGHTS_DIR) 
            if os.path.isdir(os.path.join(GLOBAL_WEIGHTS_DIR, d)) and d.startswith('aggregation_')
        ]
        
        if not model_dirs:
            return jsonify({
                "status": "not_found",
                "message": "No aggregation directories found"
            }), 404
            
        # Sort by modification time (newest first)
        model_dirs.sort(key=lambda x: os.path.getmtime(x), reverse=True)
        latest_dir = model_dirs[0]
        
        # Find model files in the directory
        model_files = [
            f for f in os.listdir(latest_dir) 
            if f.endswith(('.pth', '.pt', '.bin', '.json'))
        ]
        
        if not model_files:
            return jsonify({
                "status": "not_found",
                "message": "No model files found in the latest directory"
            }), 404
            
        # Create full paths
        model_paths = {
            'directory': latest_dir,
            'files': [os.path.join(latest_dir, f) for f in model_files],
            'timestamps': {
                'created': os.path.getctime(latest_dir),
                'modified': os.path.getmtime(latest_dir)
            }
        }
        
        return jsonify({
            "status": "success",
            "model": model_paths,
            "message": "Latest global model location retrieved successfully"
        }), 200
        
    except Exception as e:
        return jsonify({
            "status": "error",
            "error": str(e)
        }), 500

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000)
