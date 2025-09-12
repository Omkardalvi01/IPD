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
OUTPUT_DIR = BASE_DIR / "output"
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
        print("Validating edge weight files...")
        success, client_weights = validate_edge_weights(weight_files)
        if not success:
            return jsonify({"error": "Weight validation failed"}), 400

        # Perform aggregation
        global_weights = fedavg_aggregate(client_weights)

        # Create timestamped directory for this aggregation
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        aggregation_dir = os.path.join(GLOBAL_WEIGHTS_DIR, f"aggregation_{timestamp}")
        os.makedirs(aggregation_dir, exist_ok=True)

        # Save the aggregated weights
        save_global_model(global_weights, aggregation_dir)

        return jsonify({
            "status": "success",
            "message": "Aggregation completed successfully",
            "timestamp": timestamp,
            "num_clients": len(client_weights)
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

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
