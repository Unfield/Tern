#!/usr/bin/env bash
# compare.sh BASE: renders the benchmark app's pages at BASE and at the
# working tree, alternating runs on this machine, and compares them with
# benchstat. It fails when a page is significantly slower: by more than
# THRESHOLD percent (default 10), with benchstat's p < 0.05.
set -euo pipefail
base_ref=${1:?usage: compare.sh BASE}
threshold=${THRESHOLD:-10}
runs=${RUNS:-8}
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'git -C "$root" worktree remove --force "$work/base" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

build() { # tree out
	local tree=$1 out=$2
	(cd "$tree" && pnpm install --frozen-lockfile --silent >/dev/null && pnpm --filter @ternkit/tern build >/dev/null)
	# The base may be older than the benchmarks: use this tree's.
	if [ "$tree" != "$root" ]; then cp "$root/benchmarks/vs-sveltekit/tern/render_bench_test.go" "$tree/benchmarks/vs-sveltekit/tern/"; fi
	(cd "$tree/benchmarks/vs-sveltekit/tern" && npm ci --silent >/dev/null && go tool tern build -o build/ternbench >/dev/null &&
		go test -gcflags=-l -c -o "$out" .)
}

git -C "$root" worktree add --detach "$work/base" "$base_ref" >/dev/null
echo "building $base_ref"
build "$work/base" "$work/base.test"
echo "building the working tree"
build "$root" "$work/head.test"

for i in $(seq "$runs"); do
	for side in base head; do
		(cd "$root/benchmarks/vs-sveltekit/tern" && "$work/$side.test" -test.run '^$' -test.bench . -test.benchtime 2s -test.count 1 | grep '^Benchmark' >>"$work/$side.txt")
	done
done

benchstat() { (cd "$work" && go run golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e "$@" base.txt head.txt); }
benchstat | tee "$work/report.txt"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	{ echo "### Render benchmarks against $base_ref"; echo '```'; cat "$work/report.txt"; echo '```'; } >>"$GITHUB_STEP_SUMMARY"
fi

# A slower page: a sec/op line with a significant change above the threshold.
benchstat -format csv 2>/dev/null |
	awk -F, -v t="$threshold" '
		/^,sec\/op/ { timing = 1; next }
		/^,/ { timing = 0 }
		timing && $1 != "geomean" && $6 ~ /%$/ {
			d = $6; sub(/%/, "", d); p = $7; sub(/p=/, "", p)
			if (d + 0 > t && p + 0 < 0.05) { print "slower: " $1 " " $6; bad = 1 }
		}
		END { exit bad }'
