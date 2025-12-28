#!/bin/bash

echo "=== Testing Graceful Shutdown ==="
echo ""

echo ""
echo "4. Sending SIGTERM to discovery-service (graceful shutdown)..."
echo "   Watch the logs below - you should see:"
echo "   - 'Shutting down gracefully...'"
echo "   - 'Shutting down discovery service, waiting up to 10s...'"
echo "   - 'All processing goroutines completed successfully' (or timeout message)"
echo ""

# Send SIGTERM to trigger graceful shutdown
# Using docker stop which sends SIGTERM and waits before SIGKILL
docker stop vigil-discovery-service

echo ""
echo "5. Following logs (Ctrl+C to stop)..."
echo "   The service should shut down gracefully within 10 seconds"

echo ""
echo "6. Checking if container stopped..."
sleep 2
docker-compose ps discovery-service

echo ""
echo "=== Test Complete ==="
echo ""
echo "To restart: docker-compose up -d"

