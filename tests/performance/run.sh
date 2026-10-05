#!/usr/bin/env bash
# Local k6 performance test (not run in CI). Generates SBOMs into the
# gitignored data/ directory if needed, serves a fresh database, then runs
# ingest.js followed by query.js against it.
#
# Environment:
#   SBOM_COUNT       number of SBOMs to generate (default 100)
#   SBOM_COMPONENTS  components per SBOM (default 500)
#   SBOM_NAMES       distinct package names; lower = more overlap (default 5000)
#   INGEST_VUS       concurrent ingest clients (default 4)
#   QUERY_VUS        concurrent query clients (default 8)
#   QUERY_DURATION   query phase length (default 30s)
#   ADDR             server address (default 127.0.0.1:18080)
set -euo pipefail

perf_dir=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$perf_dir/../.." && pwd)
data=$perf_dir/data
addr=${ADDR:-127.0.0.1:18080}
db=$data/perf.db

(cd "$root" && go run ./tests/performance/generate -out "$data" \
  -count "${SBOM_COUNT:-100}" -components "${SBOM_COMPONENTS:-500}" -names "${SBOM_NAMES:-5000}")

rm -f "$db" "$db-wal" "$db-shm"
"$root/bin/sbom-cli" serve --db "$db" --addr "$addr" &
server=$!
trap 'kill "$server" 2>/dev/null; wait "$server" 2>/dev/null || true' EXIT

for _ in $(seq 50); do
  curl -sf "http://$addr/healthz" >/dev/null && break
  kill -0 "$server" 2>/dev/null || { echo "server exited during startup" >&2; exit 1; }
  sleep 0.1
done
curl -sf "http://$addr/healthz" >/dev/null || { echo "server not ready at $addr" >&2; exit 1; }

cd "$perf_dir"
k6 run -e BASE_URL="http://$addr" -e VUS="${INGEST_VUS:-4}" ingest.js
k6 run -e BASE_URL="http://$addr" -e VUS="${QUERY_VUS:-8}" -e DURATION="${QUERY_DURATION:-30s}" query.js

# Stop the server first so SQLite folds the write-ahead log into the db file.
kill "$server"
wait "$server" || true
trap - EXIT
echo "database: $(du -h "$db" | cut -f1) at $db"
