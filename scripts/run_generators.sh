#!/bin/bash

# Navigate to the server generator directory
cd $(dirname "$0")/../generator_server_stub

# Run the server generator to generate server stub
echo "Running the server stub generator..."
go run generator_server_stub.go

# Check if the generation was successful
if [ $? -eq 0 ]; then
    echo "Server stub generation successful!"
else
    echo "Server stub generation failed."
    exit 1
fi

# Navigate to the client generator directory
cd $(dirname "$0")/../generator_client_stub

# Run the client generator to generate client stub
echo "Running the client stub generator..."
go run generator_client_stub.go

# Check if the generation was successful
if [ $? -eq 0 ]; then
    echo "Client stub generation successful!"
else
    echo "Client stub generation failed."
    exit 1
fi

# Navigate back to the root directory
cd ../

echo "Done."