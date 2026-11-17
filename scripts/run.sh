#!/bin/bash
# Start all services

set -e

echo "Starting all services..."

# Start infrastructure (Docker Compose)
echo "Starting infrastructure containers..."
docker-compose -f infrastructure/docker-compose.yml up -d

# Wait for services to be ready
echo "Waiting for services to be ready..."
sleep 10

# Start Node.js services
echo "Starting Node.js services..."
for service in packages/*; do
    if [ -f "$service/package.json" ] && [ -d "$service/src" ]; then
        cd "$service"
        npm start &
        cd - > /dev/null
    fi
done

# Start Python services
echo "Starting Python services..."
for service in services/*; do
    if [ -f "$service/pyproject.toml" ] || [ -f "$service/setup.py" ]; then
        cd "$service"
        python -m uvicorn src.main:app --host 0.0.0.0 --port 8000 &
        cd - > /dev/null
    fi
done

# Print running services
echo ""
echo "Services starting in background..."
echo "Check logs with: docker-compose logs -f"
echo ""
echo "To stop all services: ./stop.sh"
