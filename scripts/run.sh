#!/bin/bash

set -e

export POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_DB="${POSTGRES_DB:-project-sem-1}"
export POSTGRES_USER="${POSTGRES_USER:-validator}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-val1dat0r}"
export SERVER_PORT="${SERVER_PORT:-8080}"

go build -o server main.go

./server &

for i in {1..30}; do
    if curl -s http://localhost:$SERVER_PORT/api/v0/prices > /dev/null 2>&1; then
        exit 0
    fi
    sleep 1
done

exit 1
