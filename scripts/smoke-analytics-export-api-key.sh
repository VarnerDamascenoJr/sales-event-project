#!/usr/bin/env bash
set -euo pipefail

api_base_url="${API_BASE_URL:-http://localhost:8080}"
sales_event_id="${SALES_EVENT_ID:-11111111-1111-1111-1111-111111111111}"
support_api_key="${SALES_SUPPORT_API_KEY:-dev-support-key}"
invalid_api_key="${SALES_INVALID_API_KEY:-invalid-support-key}"
limit="${SALES_ANALYTICS_LIMIT:-10}"

url="${api_base_url%/}/analytics/export?salesEventId=${sales_event_id}&limit=${limit}"

require_status() {
  local label="$1"
  local api_key="$2"
  local expected_status="$3"

  local response_file
  response_file="$(mktemp)"
  local status
  status="$(
    curl -sS -o "$response_file" -w '%{http_code}' \
      -H "X-API-Key: ${api_key}" \
      "$url"
  )"

  if [[ "$status" != "$expected_status" ]]; then
    echo "${label}: expected HTTP ${expected_status}, got ${status}" >&2
    cat "$response_file" >&2
    rm -f "$response_file"
    exit 1
  fi

  echo "${label}: HTTP ${status}"
  rm -f "$response_file"
}

require_status "valid support key" "$support_api_key" "200"
require_status "invalid key" "$invalid_api_key" "401"
