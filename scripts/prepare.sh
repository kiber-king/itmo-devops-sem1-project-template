#!/bin/bash

set -e

DB_HOST="${POSTGRES_HOST:-localhost}"
DB_PORT="${POSTGRES_PORT:-5432}"
DB_NAME="${POSTGRES_DB:-project-sem-1}"
DB_USER="${POSTGRES_USER:-validator}"
DB_PASSWORD="${POSTGRES_PASSWORD:-val1dat0r}"

go mod download
go mod tidy

for i in {1..30}; do
    if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c '\q' 2>/dev/null; then
        break
    fi
    sleep 1
done

PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME <<EOF
DROP TABLE IF EXISTS prices;

CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(255) NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    create_date TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_prices_category ON prices(category);
CREATE INDEX IF NOT EXISTS idx_prices_create_date ON prices(create_date);
EOF
