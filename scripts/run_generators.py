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

# Absolute paths for the stub directories
server_stub_dir = os.path.abspath(os.path.join(base_dir, "..", "generator_server_stub"))
client_stub_dir = os.path.abspath(os.path.join(base_dir, "..", "generator_client_stub"))

print("Running the server stub generator...")
os.chdir(server_stub_dir)
run_command(["go", "run", "generator_server_stub.go"], 
            "Server stub generation successful!", 
            "Server stub generation failed.")

os.chdir(base_dir)
print("Running the client stub generator...")
os.chdir(client_stub_dir)
run_command(["go", "run", "generator_client_stub.go"], 
            "Client stub generation successful!", 
            "Client stub generation failed.")

print("Done.")