#!/bin/bash

# Find the PID of everythingxd process
PID=$(pgrep -f "/usr/local/bin/everythingxd")

if [ -z "$PID" ]; then
    echo "Error: everythingxd process not found"
    exit 1
fi

# Check if process exists
if ! kill -0 "$PID" 2>/dev/null; then
    echo "Error: Process $PID does not exist"
    exit 1
fi

# Send SIGUSR1
echo "Sending SIGUSR1 to process $PID"
kill -SIGUSR1 "$PID"
if [ $? -ne 0 ]; then
    echo "Error: Failed to send SIGUSR1"
    exit 1
fi

# Wait briefly
sleep 1

# Send SIGUSR2
echo "Sending SIGUSR2 to process $PID"
kill -SIGUSR2 "$PID"
if [ $? -ne 0 ]; then
    echo "Error: Failed to send SIGUSR2"
    exit 1
fi

echo "Signals sent successfully"

