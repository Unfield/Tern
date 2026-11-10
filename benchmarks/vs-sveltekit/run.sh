#!/usr/bin/env bash
# Runs every scenario against Tern, SvelteKit on one Node process, and
# SvelteKit on a Node cluster, one at a time, and writes results.jsonl.
# Build both apps first: (cd tern && npm run build) and (cd sveltekit && npm run build).
set -euo pipefail
cd "$(dirname "$0")"
(cd load && go build -o load .)
: > results.jsonl
DUR=${DUR:-10s}
WARM=${WARM:-3s}
CONNS=${CONNS:-32}

rss() { # total resident memory of the processes matching $1, in MB
	ps -axo rss=,command= | grep -F -- "$1" | grep -v grep | awk '{s+=$1} END {printf "%d", s/1024}'
}

bench() { # name port pattern
	local name=$1 port=$2 pattern=$3
	until curl -fs "localhost:$port/" >/dev/null; do sleep 0.2; done
	for scenario in "home:/:false" "home-unique:/:true" "items:/items:false" "items-unique:/items:true" "data:/items/__data.json:false"; do
		IFS=: read -r label path unique <<<"$scenario"
		./load/load -url "http://localhost:$port$path" -c "$CONNS" -d "$DUR" -warmup "$WARM" -unique="$unique" |
			jq -c --arg n "$name" --arg s "$label" --arg m "$(rss "$pattern")" '. + {server: $n, scenario: $s, rss_mb: ($m | tonumber)}' | tee -a results.jsonl
	done
}

PORT=5101 ./tern/build/ternbench >/dev/null 2>&1 &
bench tern 5101 "ternbench"
kill %1; wait %1 2>/dev/null || true

(cd sveltekit && PORT=5102 exec node build) >/dev/null 2>&1 &
bench sveltekit-1 5102 "node build"
kill %1; wait %1 2>/dev/null || true

(cd sveltekit && WORKERS=${WORKERS:-10} PORT=5103 exec node cluster.js) >/dev/null 2>&1 &
bench sveltekit-cluster 5103 "cluster.js"
pkill -f "node cluster.js" || true
