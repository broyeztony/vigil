#!/bin/bash

echo "=== Testing Graceful Shutdown ==="
echo ""

# Start services
echo "1. Starting services..."
docker-compose up -d --build

echo ""
echo "2. Waiting 10 seconds for services to start and process some emails..."
sleep 10

echo ""
echo "3. Checking current status..."
docker-compose ps

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

# Follow logs to see the shutdown process
docker-compose logs -f discovery-service

echo ""
echo "6. Checking if container stopped..."
sleep 2
docker-compose ps discovery-service

echo ""
echo "=== Test Complete ==="
echo ""
echo "To restart: docker-compose up -d"

