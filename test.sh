#!/bin/bash

echo "Starting Vigil system..."

docker-compose down -v 2>/dev/null || true
docker-compose up -d --build

echo ""
echo "Waiting 15 seconds for services to start..."
sleep 15

echo ""
echo "Services:"
docker ps --filter "name=vigil" --format "{{.Names}}: {{.Status}}"

echo ""
echo "Mock server:"
curl -s http://localhost:8080/health || echo "Not ready"

echo ""
echo "Discovery logs (last 10 lines):"
docker-compose logs --tail=10 discovery-service

echo ""
echo "Done. Watch logs with: docker-compose logs -f discovery-service"
