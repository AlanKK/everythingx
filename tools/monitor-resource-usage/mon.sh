#!/bin/bash

LOG_FILE="findfilesd_usage.log"
PROCESS_NAME="findfilesd"

echo "Timestamp,CPU%,Memory(MB)" > $LOG_FILE

while true; do
    PID=$(pgrep -x $PROCESS_NAME)
    if [ -z "$PID" ]; then
        echo "Process $PROCESS_NAME not found. Exiting."
        exit 1
    fi

    TIMESTAMP=$(date "+%Y-%m-%d %H:%M:%S")
    CPU_USAGE=$(ps -p $PID -o %cpu | tail -n 1 | tr -d ' ')
    MEM_USAGE_KB=$(ps -p $PID -o rss | tail -n 1 | tr -d ' ')
    MEM_USAGE_MB=$(echo "scale=2; $MEM_USAGE_KB/1024" | bc)

    echo "$TIMESTAMP,$CPU_USAGE,$MEM_USAGE_MB" >> $LOG_FILE

    sleep 15
done
