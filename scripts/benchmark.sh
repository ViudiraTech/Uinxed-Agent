#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
OUT="${1:-benchmark.txt}"
BIN="$(mktemp -t ux-agent-bench.XXXXXX)"
trap 'rm -f "$BIN"' EXIT
{
  echo "# Uinxed-Agent benchmark"
  date -u '+date_utc=%Y-%m-%dT%H:%M:%SZ'
  echo "go=$(go version)"
  echo "os=$(go env GOOS)"
  echo "arch=$(go env GOARCH)"
  echo
  echo "## build"
  /usr/bin/time -f 'build_wall=%e build_maxrss_kb=%M' go build -o "$BIN" ./cmd/ux-agent
  wc -c "$BIN" | awk '{print "binary_bytes=" $1}'
  echo
  echo "## binary startup baseline"
  for i in 1 2 3 4 5; do /usr/bin/time -f 'wall=%e maxrss_kb=%M' "$BIN" --version >/dev/null; done
  echo
  echo "## package benchmarks"
  go test -run '^$' -bench . -benchmem ./internal/markdown ./internal/indexer ./internal/storage
} 2>&1 | tee "$OUT"
