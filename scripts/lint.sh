#!/bin/bash
# Lint code for all services and packages

set -e

echo "Linting code..."

# Lint infrastructure
echo "Linting infrastructure..."
docker-compose -f infrastructure/docker-compose.yml exec infrastructure \
    bash -c "which pylint || pip install pylint"
docker-compose -f infrastructure/docker-compose.yml exec infrastructure \
    bash -c "pylint src --rcfile=pylintrc"

# Lint Node.js services and packages
echo "Linting Node.js code..."
for service in services/* packages/*; do
    if [ -f "$service/package.json" ]; then
        cd "$service"
        npm run lint 2>&1
        cd - > /dev/null
    fi
done

# Lint Python services
echo "Linting Python code..."
for service in services/*; do
    if [ -f "$service/pyproject.toml" ] || [ -f "$service/setup.py" ]; then
        cd "$service"
        python -m flake8 src tests 2>&1
        cd - > /dev/null
    fi
done

# Lint contract files
echo "Linting contracts..."
find contracts -name "*.proto" | xargs protoc --proto_path=contracts/proto \
    --pyi_out=. --mypy_out=.

echo ""
echo "Linting complete!"
