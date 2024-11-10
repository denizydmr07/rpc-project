#!/usr/bin/env python3

import os
import subprocess
import sys

def run_command(command, success_message, failure_message):
    try:
        result = subprocess.run(command, check=True, capture_output=True, text=True)
        print(success_message)
    except subprocess.CalledProcessError as e:
        print(failure_message)
        print(e.stderr)
        sys.exit(1)

# Get the base directory of the script
base_dir = os.path.dirname(os.path.abspath(__file__))

# Navigate to the server generator directory and run the server stub generator
server_stub_dir = os.path.join(base_dir, "../generator_server_stub")
os.chdir(server_stub_dir)
print("Running the server stub generator...")
run_command(["go", "run", "generator_server_stub.go"], 
            "Server stub generation successful!", 
            "Server stub generation failed.")

# Navigate to the client generator directory and run the client stub generator
client_stub_dir = os.path.join(base_dir, "../generator_client_stub")
os.chdir(client_stub_dir)
print("Running the client stub generator...")
run_command(["go", "run", "generator_client_stub.go"], 
            "Client stub generation successful!", 
            "Client stub generation failed.")

# Navigate back to the root directory
os.chdir(os.path.join(base_dir, "../"))
print("Done.")
