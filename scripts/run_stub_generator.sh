#!/bin/bash 

# Navigate to the generators directory
cd $(dirname "$0")/../generators

# Run the stub generator to generate stubs for client and server
echo "Running the stub generator..."
go run stub_generator.go

# Check if the generation was successful
if [ $? -eq 0 ]; then
    echo "Stub generation successful!"
else
    echo "Stub generation failed."
    exit 1
fi

# Navigate back to the root directory
cd ../

echo "Done."
