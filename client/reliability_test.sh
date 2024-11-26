#!/bin/bash

# Number of runs
num_runs=100

# Run `go run .` for the specified number of times
for ((i=1; i<=num_runs; i++))
do
    echo "Run #$i"
    go run .
    if [ $? -ne 0 ]; then
        echo "Error: Execution failed on run #$i"
        exit 1
    fi
done

echo "Completed $num_runs runs successfully."
