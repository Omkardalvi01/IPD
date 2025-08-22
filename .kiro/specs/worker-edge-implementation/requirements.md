# Requirements Document

## Introduction

This feature implements a worker edge node for federated learning that receives image batches from a master edge, executes Python ML training scripts, and sends trained weights back to the master. The worker edge maintains persistent bidirectional WebRTC connections throughout the entire training lifecycle and coordinates with the master edge for federated learning rounds.

## Requirements

### Requirement 1

**User Story:** As a federated learning system operator, I want worker edge nodes that can receive image batches from the master edge, so that distributed training can be performed across multiple edge devices.

#### Acceptance Criteria

1. WHEN a worker edge starts THEN it SHALL establish a WebRTC connection with the master edge using a unique connection ID
2. WHEN the master edge sends image batches THEN the worker edge SHALL receive and store them locally in a designated directory
3. WHEN all images in a batch are received THEN the worker edge SHALL signal batch completion to the master edge
4. IF connection fails during image transfer THEN the worker edge SHALL attempt to reconnect and resume transfer

### Requirement 2

**User Story:** As a federated learning system operator, I want worker edges to execute Python ML training scripts on received data, so that model training can be performed locally on each edge device.

#### Acceptance Criteria

1. WHEN a complete image batch is received THEN the worker edge SHALL execute a specified Python training script
2. WHEN the Python script is executed THEN it SHALL be passed the path to the image batch directory as an argument
3. WHEN Python training completes THEN it SHALL generate a weights file in a predefined location
4. IF Python script execution fails THEN the worker edge SHALL log the error and notify the master edge of training failure
5. WHEN training is in progress THEN the worker edge SHALL maintain the WebRTC connection with the master edge

### Requirement 3

**User Story:** As a federated learning system operator, I want worker edges to send trained weights back to the master edge, so that federated aggregation can be performed.

#### Acceptance Criteria

1. WHEN Python training completes successfully THEN the worker edge SHALL read the generated weights file
2. WHEN weights file is read THEN the worker edge SHALL send the weights data back to the master edge via the same WebRTC data channel
3. WHEN weights transfer is complete THEN the worker edge SHALL send a training completion signal to the master edge
4. IF weights file is not found or corrupted THEN the worker edge SHALL notify the master edge of training failure
5. WHEN weights are sent THEN the worker edge SHALL wait for the next training round or connection termination signal

### Requirement 4

**User Story:** As a federated learning system operator, I want worker edges to maintain persistent connections throughout training cycles, so that multiple federated learning rounds can be executed efficiently.

#### Acceptance Criteria

1. WHEN a WebRTC connection is established THEN it SHALL remain open throughout the entire training lifecycle
2. WHEN one training round completes THEN the worker edge SHALL be ready to receive the next batch without reconnecting
3. WHEN the master edge signals end of training THEN the worker edge SHALL gracefully close the connection and terminate
4. IF connection is lost during training THEN the worker edge SHALL attempt to reconnect and resume from the last known state
5. WHEN idle between training rounds THEN the worker edge SHALL send periodic heartbeat messages to maintain connection

### Requirement 5

**User Story:** As a federated learning system operator, I want worker edges to handle errors gracefully and provide status updates, so that the system can recover from failures and provide visibility into training progress.

#### Acceptance Criteria

1. WHEN any error occurs THEN the worker edge SHALL log detailed error information locally
2. WHEN training fails THEN the worker edge SHALL send failure notification to the master edge with error details
3. WHEN training progresses THEN the worker edge SHALL send periodic status updates to the master edge
4. IF disk space is insufficient for image storage THEN the worker edge SHALL reject the batch and notify the master edge
5. WHEN worker edge starts THEN it SHALL validate that Python environment and required dependencies are available