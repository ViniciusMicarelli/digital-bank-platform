#!/bin/bash
# Run database migrations for all services

set -e

echo "Running database migrations..."

# Set environment based on context
if [ -f .env ]; then
    export $(grep -E '^(DB_|DATABASE_|SQL_)' .env | xargs)
fi

# Run migrations in parallel for each service
for service in \
    services/ledger/migrations \
    services/payment-gateway/migrations \
    services/identity/migrations \
    services/accounts/migrations; do
    
    if [ -d "$service" ]; then
        echo "Running migrations for: $(basename $service)"
        cd "$(dirname $service)"
        node -e "
            const fs = require('fs');
            const { exec } = require('child_process');
            
            const files = fs.readdirSync('$service').filter(f => f.endsWith('.sql'));
            
            files.forEach(file => {
                const query = fs.readFileSync(file, 'utf-8').trim();
                console.log('Running:', file);
                exec(query, (err, stdout, stderr) => {
                    if (stderr) console.error(stderr);
                });
            });
        "
        cd - > /dev/null
    fi
done

echo "Migrations complete!"
