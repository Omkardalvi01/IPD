import asyncio
import websockets
import json
import logging
import os

# --- Configuration ---
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# --- In-Memory State ---
rooms = {}
room_locks = {}


async def unregister_client(room_id, edge_id=None):
    """Safely unregisters a controller or an edge and cleans up the room if necessary."""
    if room_id not in room_locks:
        return

    async with room_locks[room_id]:
        if room_id not in rooms:
            return

        if edge_id:  # Unregistering an edge
            if edge_id in rooms[room_id]["connected_edges"]:
                del rooms[room_id]["connected_edges"][edge_id]
                logging.info(f"Edge '{edge_id}' disconnected from room '{room_id}'.")
        else:  # Unregistering a controller
            logging.info(f"Controller for room '{room_id}' disconnected. Closing room.")
            del rooms[room_id]
            del room_locks[room_id]


async def check_and_notify_controller(room_id):
    """Checks if a room is full. If so, calculates allocation and sends it to the controller."""
    if room_id not in rooms:
        return

    room = rooms[room_id]
    num_edges_expected = room["num_edges"]
    num_edges_connected = len(room["connected_edges"])

    logging.info(f"Room '{room_id}' status: {num_edges_connected}/{num_edges_expected} edges connected.")

    if num_edges_connected == num_edges_expected:
        logging.info(f"Room '{room_id}' is full. Sending allocation to controller.")
        
        percentage = 100 // num_edges_expected
        remainder = 100 % num_edges_expected
        
        allocation_data = {}
        edge_ids = list(room["connected_edges"].keys())

        for i, edge_id in enumerate(edge_ids):
            alloc = percentage
            if i < remainder:
                alloc += 1
            allocation_data[edge_id] = alloc

        payload = {"allocation": allocation_data}

        try:
            controller_socket = room["controller_socket"]
            await controller_socket.send(json.dumps(payload))
            logging.info(f"Successfully sent allocation for room '{room_id}': {payload}")
        except websockets.exceptions.ConnectionClosed:
            logging.warning(f"Controller for room '{room_id}' disconnected before allocation could be sent.")
        except Exception as e:
            logging.error(f"Failed to send allocation to controller for room '{room_id}': {e}")


async def handler(ws):
    """Main WebSocket connection handler."""
    client_info = {"type": None, "room_id": None, "edge_id": None}
    try:
        initial_msg = await ws.recv()
        data = json.loads(initial_msg)

        role = data.get("role")
        room_id = data.get("room_id")

        if not role or not room_id:
            await ws.close(1008, "Role and room_id are required.")
            return

        client_info["room_id"] = room_id

        # --- Handle Controller Registration ---
        if role == "C":
            client_info["type"] = "controller"
            num_edges = int(data.get("num_edges", 0))

            if room_id not in room_locks:
                room_locks[room_id] = asyncio.Lock()
            
            async with room_locks[room_id]:
                if room_id in rooms:
                    await ws.close(1008, f"Room '{room_id}' already exists.")
                    return
                
                rooms[room_id] = {
                    "controller_socket": ws,
                    "num_edges": num_edges,
                    "connected_edges": {},
                }
            logging.info(f"Controller registered for room '{room_id}', expecting {num_edges} edges.")

        # --- Handle Edge Registration ---
        elif role == "E":
            client_info["type"] = "edge"
            edge_id = data.get("edge_id")
            if not edge_id:
                await ws.close(1008, "edge_id is required for role 'E'.")
                return
            
            client_info["edge_id"] = edge_id

            if room_id not in room_locks:
                await ws.send(json.dumps({"error": f"Room '{room_id}' not found."}))
                await ws.close()
                return

            async with room_locks[room_id]:
                if room_id not in rooms:
                     await ws.send(json.dumps({"error": f"Room '{room_id}' not found."}))
                     await ws.close()
                     return

                room = rooms[room_id]
                if len(room["connected_edges"]) >= room["num_edges"]:
                    await ws.send(json.dumps({"error": f"Room '{room_id}' is already full."}))
                    await ws.close()
                    return
                
                room["connected_edges"][edge_id] = ws
                await ws.send(json.dumps({"status": "successfully joined room"}))
            
            await check_and_notify_controller(room_id)
        
        else:
            await ws.close(1008, f"Unknown role: {role}")
            return
        
        async for message in ws:
            logging.info(f"Received message from {client_info}: {message}")

    except websockets.exceptions.ConnectionClosedOK:
        logging.info(f"Client {client_info} disconnected gracefully.")
    except websockets.exceptions.ConnectionClosedError as e:
        logging.warning(f"Client {client_info} disconnected with error: {e}")
    except json.JSONDecodeError:
        logging.error("Failed to decode JSON from client.")
    except Exception as e:
        logging.error(f"An unexpected error occurred with {client_info}: {e}", exc_info=True)
    finally:
        if client_info["room_id"]:
            if client_info["type"] == "controller":
                await unregister_client(client_info["room_id"])
            elif client_info["type"] == "edge":
                await unregister_client(client_info["room_id"], client_info["edge_id"])


async def main():
    port = int(os.environ.get("PORT", 10000))  # Render will set $PORT
    logging.info(f"Starting WebSocket server on ws://0.0.0.0:{port}")
    async with websockets.serve(handler, "0.0.0.0", port):
        await asyncio.Future()  # run forever

if __name__ == "__main__":
    asyncio.run(main())
