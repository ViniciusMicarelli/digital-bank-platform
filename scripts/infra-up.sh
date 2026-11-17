#!/bin/bash
# Start infrastructure (Docker Compose)

set -e

echo "Starting infrastructure..."

# Check for Docker
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is not installed"
    exit 1
fi

# Check for Docker Compose
if ! command -v docker-compose &> /dev/null; then
    echo "Warning: docker-compose not found, trying 'docker compose'..."
    if ! command -v docker compose &> /dev/null; then
        echo "Error: Docker Compose is not installed"
        exit 1
    fi
fi

# Create .env file from template if it doesn't exist
if [ ! -f .env ]; then
    echo "Creating .env file..."
    cp .env.example . 2>/dev/null || {
        cat > .env << ENVFILE
# Database configuration
POSTGRES_PASSWORD=your-secure-password
DATABASE_URL=postgres://postgres:your-secure-password@localhost:5432/postgres

# Redis configuration
REDIS_PASSWORD=your-secure-password
REDIS_URL=redis://localhost:6379

# API keys
JWT_SECRET=your-jwt-secret-key

# Service configurations
API_PORT=3000
ENVFILE
    }
fi

# Load environment variables
export $(grep -E '^(DB_|DATABASE_|SQL_|REDIS|JWT_|API_)' .env | xargs)

# Start infrastructure
docker-compose -f infrastructure/docker-compose.yml up -d

# Wait for services to be ready
echo "Waiting for services to be ready..."
dockerize -wait tcp://localhost:5432 tcp://localhost:6379 --timeout 30s || true

# Print status
echo ""
echo "Infrastructure started!"
echo "Services running:"
docker-compose -f infrastructure/docker-compose.yml ps

echo ""
echo "To stop infrastructure: docker-compose -f infrastructure/docker-compose.yml down"
