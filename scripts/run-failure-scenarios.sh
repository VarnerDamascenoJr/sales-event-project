#!/usr/bin/env bash
set -euo pipefail

api_base_url="${API_BASE_URL:-http://localhost:8080}"
rabbit_base_url="${RABBITMQ_MANAGEMENT_URL:-http://localhost:15672}"
sales_event_id="${SALES_EVENT_ID:-11111111-1111-1111-1111-111111111111}"
ticket_id="${TICKET_ID:-22222222-2222-2222-2222-222222222222}"
ticket_price="${TICKET_PRICE:-10000}"
payment_webhook_secret="${PAYMENT_WEBHOOK_SECRET:-change-me-payment-webhook-secret}"

scenario="${1:-all}"

usage() {
  cat <<'EOF'
Usage: scripts/run-failure-scenarios.sh [scenario]

Scenarios:
  all                  Run every controlled failure scenario.
  duplicate-payment    Send the same payment webhook twice and expect conflict.
  delayed-consumer     Stop the worker, queue SALE_CREATED, then restart it.
  email-failure        Force SMTP failure and drive email retry to DEAD_LETTER.
  outbox-retry         Publish SALE_FAILED without a binding and drive outbox retry.

The script starts the local compose stack if needed and writes observations to stdout.
EOF
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

compose() {
  docker compose "$@"
}

psql() {
  compose exec -T postgres psql -U sales -d sales_event -Atq -c "$1"
}

json_get() {
  local field="$1"
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$field"
}

unique_id() {
  python3 -c 'import uuid; print(uuid.uuid4())'
}

wait_for_sql() {
  local sql="$1"
  local want="$2"
  local label="$3"

  for _ in $(seq 1 60); do
    got="$(psql "$sql" || true)"
    if [[ "$got" == "$want" ]]; then
      echo "$label: $got"
      return 0
    fi
    sleep 1
  done

  echo "timed out waiting for $label to become $want; last value: ${got:-<empty>}" >&2
  exit 1
}

wait_for_sql_contains() {
  local sql="$1"
  local needle="$2"
  local label="$3"

  for _ in $(seq 1 60); do
    got="$(psql "$sql" || true)"
    if [[ "$got" == *"$needle"* ]]; then
      echo "$label: $got"
      return 0
    fi
    sleep 1
  done

  echo "timed out waiting for $label to contain $needle; last value: ${got:-<empty>}" >&2
  exit 1
}

request_json() {
  local method="$1"
  local url="$2"
  local body_file="$3"
  local expected_status="$4"
  shift 4

  local response_file
  response_file="$(mktemp)"
  local status
  status="$(
    curl -sS -o "$response_file" -w '%{http_code}' \
      -X "$method" "$url" \
      -H 'Content-Type: application/json' \
      "$@" \
      --data-binary "@$body_file"
  )"

  if [[ "$status" != "$expected_status" ]]; then
    echo "expected HTTP $expected_status from $method $url, got $status" >&2
    cat "$response_file" >&2
    rm -f "$response_file"
    exit 1
  fi

  cat "$response_file"
  rm -f "$response_file"
}

create_sale() {
  local customer_id
  customer_id="customer-$(unique_id)"
  local body_file
  body_file="$(mktemp)"
  cat >"$body_file" <<EOF
{
  "salesEventId": "$sales_event_id",
  "customerId": "$customer_id",
  "customerName": "Failure Scenario Buyer",
  "customerEmail": "$customer_id@example.com",
  "items": [
    {
      "ticketId": "$ticket_id",
      "quantity": 1,
      "unitPrice": $ticket_price
    }
  ]
}
EOF

  local response
  response="$(request_json POST "$api_base_url/sales" "$body_file" 202)"
  rm -f "$body_file"
  local sale_id
  sale_id="$(printf '%s' "$response" | json_get saleId)"
  echo "$sale_id"
}

create_payment_intent() {
  local sale_id="$1"
  local body_file
  body_file="$(mktemp)"
  cat >"$body_file" <<EOF
{
  "amount": $ticket_price,
  "provider": "failure_scenario"
}
EOF

  local response
  response="$(request_json POST "$api_base_url/sales/$sale_id/payment-intents" "$body_file" 202)"
  rm -f "$body_file"
  printf '%s' "$response" | json_get id
}

payment_webhook_body() {
  local payment_intent_id="$1"
  local sale_id="$2"
  local status="$3"

  python3 - "$payment_intent_id" "$sale_id" "$status" "$ticket_price" <<'PY'
import json
import sys
from datetime import datetime, timezone

payment_intent_id, sale_id, status, amount = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])
payload = {
    "paymentIntentId": payment_intent_id,
    "saleId": sale_id,
    "provider": "failure_scenario",
    "status": status,
    "amount": amount,
    "occurredAt": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    "providerReference": "failure-scenario-" + sale_id,
}
print(json.dumps(payload, separators=(",", ":")))
PY
}

post_payment_webhook() {
  local payload_file="$1"
  local expected_status="$2"
  local signature
  signature="$(openssl dgst -sha256 -hmac "$payment_webhook_secret" -binary "$payload_file" | xxd -p -c 256)"
  request_json POST "$api_base_url/webhooks/payments" "$payload_file" "$expected_status" \
    -H "X-Webhook-Signature: $signature" >/dev/null
}

approve_sale() {
  local sale_id="$1"
  wait_for_sql "SELECT status FROM sales WHERE id = '$sale_id';" "PENDING_PAYMENT" "sale $sale_id status"
  local payment_intent_id
  payment_intent_id="$(create_payment_intent "$sale_id")"
  local payload_file
  payload_file="$(mktemp)"
  payment_webhook_body "$payment_intent_id" "$sale_id" APPROVED >"$payload_file"
  post_payment_webhook "$payload_file" 202
  rm -f "$payload_file"
  wait_for_sql "SELECT status FROM sales WHERE id = '$sale_id';" "COMPLETED" "sale $sale_id status"
}

start_stack() {
  docker network inspect operational-observability-network >/dev/null 2>&1 || docker network create operational-observability-network >/dev/null
  compose up --build -d postgres rabbitmq migrate api worker email-retry-worker >/dev/null
}

scenario_duplicate_payment() {
  echo "== duplicate-payment =="
  local sale_id
  sale_id="$(create_sale)"
  wait_for_sql "SELECT status FROM sales WHERE id = '$sale_id';" "PENDING_PAYMENT" "sale $sale_id status"
  local payment_intent_id
  payment_intent_id="$(create_payment_intent "$sale_id")"
  local payload_file
  payload_file="$(mktemp)"
  payment_webhook_body "$payment_intent_id" "$sale_id" APPROVED >"$payload_file"

  post_payment_webhook "$payload_file" 202
  post_payment_webhook "$payload_file" 202
  rm -f "$payload_file"

  echo "duplicate webhook returned 202 as an idempotent replay"
  wait_for_sql "SELECT COUNT(*) FROM payments WHERE sale_id = '$sale_id';" "1" "payment rows for duplicate webhook"
  wait_for_sql "SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = '$sale_id' AND event_type = 'SALE_COMPLETED';" "1" "SALE_COMPLETED outbox rows for duplicate webhook"
  wait_for_sql "SELECT COUNT(*) FROM issued_tickets WHERE sale_id = '$sale_id';" "1" "issued tickets after duplicate webhook"
  psql "SELECT status, amount, provider FROM payments WHERE sale_id = '$sale_id';"
  psql "SELECT event_type, status, attempts FROM outbox_events WHERE aggregate_id = '$sale_id' ORDER BY created_at;"
  echo 'Metrics to inspect: payments_processed_total{status="APPROVED",provider="failure_scenario"}'
  echo
}

scenario_delayed_consumer() {
  echo "== delayed-consumer =="
  compose stop worker >/dev/null
  local sale_id
  sale_id="$(create_sale)"
  sleep 2

  local persisted_before_restart
  persisted_before_restart="$(psql "SELECT COUNT(*) FROM sales WHERE id = '$sale_id';")"
  if [[ "$persisted_before_restart" != "0" ]]; then
    echo "expected sale $sale_id to wait for the stopped worker, but it was already persisted" >&2
    exit 1
  fi
  echo "sale rows while worker is stopped: $persisted_before_restart"

  local queue_file
  queue_file="$(mktemp)"
  curl -sS -u guest:guest "$rabbit_base_url/api/queues/%2F/sales.created.queue" >"$queue_file"
  python3 - "$queue_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as fh:
    queue = json.load(fh)
ready = queue.get("messages_ready", 0)
unacked = queue.get("messages_unacknowledged", 0)
total = queue.get("messages", 0)
print(f"queue while worker is stopped: ready={ready} unacked={unacked} total={total}")
PY
  rm -f "$queue_file"

  compose start worker >/dev/null
  wait_for_sql "SELECT status FROM sales WHERE id = '$sale_id';" "PENDING_PAYMENT" "sale $sale_id status after worker restart"
  echo 'Logs to inspect: docker compose logs worker | grep "worker consuming queues"'
  echo 'Metrics to inspect: worker_messages_processed_total{queue="sales.created.queue",status="acked"}'
  echo
}

scenario_email_failure() {
  echo "== email-failure =="
  SMTP_HOST=127.0.0.1 SMTP_PORT=1 EMAIL_RETRY_INTERVAL=1s compose up -d --force-recreate worker email-retry-worker >/dev/null

  local sale_id
  sale_id="$(create_sale)"
  approve_sale "$sale_id"
  wait_for_sql "SELECT status FROM email_notifications WHERE sale_id = '$sale_id';" "FAILED" "email status before accelerated retry"

  psql "UPDATE email_notifications SET attempts = 4, next_retry_at = NOW() - INTERVAL '1 second' WHERE sale_id = '$sale_id';" >/dev/null
  wait_for_sql "SELECT status FROM email_notifications WHERE sale_id = '$sale_id';" "DEAD_LETTER" "email status after retry"
  psql "SELECT status, attempts, COALESCE(error_message, '') FROM email_notifications WHERE sale_id = '$sale_id';"
  echo 'Logs to inspect: docker compose logs email-retry-worker | grep "email notification moved to dead letter"'
  echo 'Metrics to inspect: ticket_delivery_total{status="retry_failed"}'
  echo

  SMTP_HOST= SMTP_PORT=587 EMAIL_RETRY_INTERVAL=1m compose up -d --force-recreate worker email-retry-worker >/dev/null
}

scenario_outbox_retry() {
  echo "== outbox-retry =="
  local sale_id
  sale_id="$(create_sale)"
  wait_for_sql "SELECT status FROM sales WHERE id = '$sale_id';" "PENDING_PAYMENT" "sale $sale_id status"
  local payment_intent_id
  payment_intent_id="$(create_payment_intent "$sale_id")"
  local payload_file
  payload_file="$(mktemp)"
  payment_webhook_body "$payment_intent_id" "$sale_id" FAILED >"$payload_file"
  post_payment_webhook "$payload_file" 202
  rm -f "$payload_file"

  wait_for_sql_contains "SELECT status || ':' || attempts FROM outbox_events WHERE aggregate_id = '$sale_id' AND event_type = 'SALE_FAILED' ORDER BY created_at DESC LIMIT 1;" "FAILED:" "SALE_FAILED outbox state before accelerated retry"
  psql "UPDATE outbox_events SET attempts = 4, next_attempt_at = NOW() - INTERVAL '1 second' WHERE aggregate_id = '$sale_id' AND event_type = 'SALE_FAILED';" >/dev/null
  wait_for_sql "SELECT status FROM outbox_events WHERE aggregate_id = '$sale_id' AND event_type = 'SALE_FAILED' ORDER BY created_at DESC LIMIT 1;" "DEAD_LETTER" "SALE_FAILED outbox state after retry"
  psql "SELECT event_type, status, attempts, COALESCE(last_error, '') FROM outbox_events WHERE aggregate_id = '$sale_id' ORDER BY created_at;"
  echo 'Logs to inspect: docker compose logs worker | grep "outbox event publish failed"'
  echo 'Metrics to inspect: outbox_events_processed_total{event_type="SALE_FAILED",status="DEAD_LETTER"}'
  echo
}

run() {
  case "$scenario" in
    -h|--help|help)
      usage
      return 0
      ;;
  esac

  require_command docker
  require_command curl
  require_command openssl
  require_command python3
  require_command xxd
  start_stack

  case "$scenario" in
    all)
      scenario_duplicate_payment
      scenario_delayed_consumer
      scenario_outbox_retry
      scenario_email_failure
      ;;
    duplicate-payment) scenario_duplicate_payment ;;
    delayed-consumer) scenario_delayed_consumer ;;
    email-failure) scenario_email_failure ;;
    outbox-retry) scenario_outbox_retry ;;
    *)
      usage
      echo "unknown scenario: $scenario" >&2
      exit 1
      ;;
  esac
}

run
