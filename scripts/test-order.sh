#!/usr/bin/env bash
# Places a test order via the Order Service API.
set -euo pipefail

ORDER_SERVICE_URL="${ORDER_SERVICE_URL:-http://localhost:8080}"

echo "📦 Placing test order..."
curl -s -X POST "${ORDER_SERVICE_URL}/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "customer_name": "Alice",
    "product": "Mechanical Keyboard",
    "quantity": 1
  }' | jq .

echo ""
echo "✅ Order placed. Check the relay and notification service logs."
