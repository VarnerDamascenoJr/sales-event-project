#!/usr/bin/env sh
set -eu

script_dir="$(cd "$(dirname "$0")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

cd "$repo_root"
exec go run ./cmd/analytics-demo-data "$@"
