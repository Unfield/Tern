#!/bin/bash
# summary.sh RESULTS [PREVIOUS]: the benchmark's results as a Markdown
# table, with Tern's change since PREVIOUS (the last run on main) when it
# exists. A drop of more than 10% in Tern's requests per second, or a
# binary 5% larger, is flagged with a GitHub warning.
set -euo pipefail
cur=$1
prev=${2:-}
[ -f "$prev" ] || prev=

echo "### Tern vs SvelteKit (requests per second, p50 / p99 ms, memory)"
echo
echo "| Scenario | Tern | SvelteKit, 1 process | SvelteKit, cluster |"
echo "|---|---|---|---|"
jq -rs 'map(select(.rps)) | group_by(.scenario)[] | [.[0].scenario, (map({(.server): "\(.rps | floor) · \(.p50_ms) / \(.p99_ms) · \(.rss_mb) MB"}) | add | .tern, .["sveltekit-1"], .["sveltekit-cluster"])] | "| " + join(" | ") + " |"' "$cur"

size=$(jq -rs 'map(select(.scenario == "binary"))[0].binary_bytes // empty' "$cur")
if [ -n "$size" ]; then
	echo
	echo "Tern's binary: $(awk -v b="$size" 'BEGIN { printf "%.1f MB", b / 1e6 }')"
fi

[ -n "$prev" ] || exit 0
echo
echo "#### Tern since the last run on main"
echo
echo "| Scenario | Before | Now | Change |"
echo "|---|---|---|---|"
jq -rs --slurpfile p "$prev" '
	def tern($rows): $rows | map(select(.server == "tern")) | map({(.scenario): .}) | add // {};
	tern(.) as $now | tern($p) as $was
	| $now | keys[] | . as $s | select($was[$s])
	| ($was[$s] | .rps // .binary_bytes) as $a | ($now[$s] | .rps // .binary_bytes) as $b
	| [$s, ($a | floor | tostring), ($b | floor | tostring), (if $a > 0 then (($b - $a) / $a * 100 * 10 | round / 10 | tostring) + "%" else "" end)]
	| "| " + join(" | ") + " |"' "$cur"

jq -rs --slurpfile p "$prev" '
	def tern($rows): $rows | map(select(.server == "tern")) | map({(.scenario): .}) | add // {};
	tern(.) as $now | tern($p) as $was
	| $now | keys[] | . as $s | select($was[$s])
	| if $now[$s].rps and $was[$s].rps and $now[$s].rps < $was[$s].rps * 0.9 then
		"::warning title=Tern benchmark::\($s): \($was[$s].rps | floor) → \($now[$s].rps | floor) requests per second"
	elif $now[$s].binary_bytes and $was[$s].binary_bytes and $now[$s].binary_bytes > $was[$s].binary_bytes * 1.05 then
		"::warning title=Tern binary::\($was[$s].binary_bytes) → \($now[$s].binary_bytes) bytes"
	else empty end' "$cur" >&2
