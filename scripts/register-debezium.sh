#!/usr/bin/env bash
# Registers the Debezium outbox connector with Kafka Connect.
set -euo pipefail

CONNECT_URL="${CONNECT_URL:-http://localhost:8083}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "⏳ Waiting for Kafka Connect to be ready..."
until curl -sf "${CONNECT_URL}/connectors" > /dev/null 2>&1; do
  sleep 2
done

echo "✅ Kafka Connect is ready. Registering outbox connector..."
curl -s -X POST "${CONNECT_URL}/connectors" \
  -H "Content-Type: application/json" \
  -d @"${SCRIPT_DIR}/../debezium/register-connector.json" | jq .

echo "✅ Connector registered. Current connectors:"
curl -s "${CONNECT_URL}/connectors" | jq .
