#!/bin/bash
set -e
set -x


echo "Initializing RPC project..."

# Initialize client stub generator
cd generator_client_stub
go mod tidy
cd ..

# Initialize server stub generator
cd generator_server_stub
go mod tidy
cd ..

# Run stub generators
python3 scripts/run_generators.py

# Initialize and start load balancer
cd loadbalancer
go mod tidy
cd ..

# Initialize and start server
cd server
go mod tidy
cd ..

# Initialize and start client
cd client
go mod tidy
cd ..

echo "Setup complete!"