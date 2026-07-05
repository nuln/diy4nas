#!/bin/sh
# Mock easytier-core for testing
# Just sleeps and responds to SIGTERM
echo "MOCK easytier-core started (pid $$)" >&2
echo "Listening on 127.0.0.1:11010" >&2
# Run forever until killed
while true; do
    sleep 1
done