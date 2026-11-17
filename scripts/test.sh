#!/bin/bash
# Run tests for all services and packages

set -e

echo "Running tests..."

# Test infrastructure
echo "Testing infrastructure..."
docker-compose -f infrastructure/docker-compose.yml -f infrastructure/docker-compose.test.yml up -d test

# Wait for test containers
sleep 5

# Test all services
echo "Running service tests..."
for service in services/*; do
    if [ -f "$service/package.json" ]; then
        cd "$service"
        npm run test 2>&1 | tee "tests/logs/$service-test.log"
        cd - > /dev/null
    elif [ -f "$service/pyproject.toml" ] || [ -f "$service/setup.py" ]; then
        cd "$service"
        python -m pytest tests/ -v 2>&1 | tee "tests/logs/$service-test.log"
        cd - > /dev/null
    fi
done

# Run contract tests
echo "Running contract tests..."
npm run test:contracts

# Stop test containers
docker-compose -f infrastructure/docker-compose.yml -f infrastructure/docker-compose.test.yml down

echo ""
echo "All tests complete!"
