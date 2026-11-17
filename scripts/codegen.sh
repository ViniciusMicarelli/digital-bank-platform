#!/bin/bash
# Generate code from protobuf, SQL, API specs, etc.

set -e

echo "Running code generation..."

# Protobuf generation
if command -v protoc &> /dev/null; then
    echo "Generating protobuf code..."
    protoc --proto_path=contracts/proto \
        --proto_path=packages/core/proto \
        --proto_path=packages/auth/proto \
        --proto_path=services/ledger/proto \
        --proto_path=services/payment-gateway/proto \
        --js_out=import_style=commonjs,binary:packages/core/src \
        --ts_out=services/ledger/src \
        --python_out=packages/core/src \
        contracts/proto/**/*.proto
else
    echo "Warning: protoc not found, skipping protobuf generation"
fi

# SQL schema generation (if sqlc or similar is used)
if command -v sqlc &> /dev/null; then
    echo "Generating SQL code..."
    sqlc generate
fi

# OpenAPI/API spec generation
if command -v openapi-generator-cli &> /dev/null; then
    echo "Generating API client code..."
    openapi-generator-cli generate -i contracts/api/openapi.json -g typescript-node \
        -o services/api-gateway/src
fi

# Contract generation from specs
echo "Generating contracts..."
npm run contracts:generate 2>/dev/null || echo "Contract generation skipped (optional dependency)"

echo "Code generation complete!"
