#!/usr/bin/env bash
# Creates a project with `tern new`, points it at this checkout of Tern
# (the Go module and the @ternkit/tern package), then type-checks, builds
# and runs it. Used by CI to keep the starter template working.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'kill "${server:-0}" 2>/dev/null || true; rm -rf "$work"' EXIT

echo "==> tern new"
(cd "$repo" && go run ./cmd/tern new -skip-go-get "$work/app")
cd "$work/app"

echo "==> use this checkout"
printf '\nrequire github.com/Unfield/Tern v0.0.0\n\nreplace github.com/Unfield/Tern => %s\n' "\"$repo\"" >> go.mod
go get -tool github.com/Unfield/Tern/cmd/tern
go mod tidy
(cd "$repo/packages/tern" && pnpm build >/dev/null)
node -e '
const fs = require("fs");
const p = JSON.parse(fs.readFileSync("package.json"));
p.devDependencies["@ternkit/tern"] = "file:" + process.argv[1] + "/packages/tern";
fs.writeFileSync("package.json", JSON.stringify(p, null, 2));
' "$repo"
npm install --no-audit --no-fund --loglevel=error

echo "==> check"
npm run check

echo "==> build"
npm run build

echo "==> run"
port=4321
PORT=$port ./build/app &
server=$!
for _ in $(seq 1 50); do curl -fs "http://localhost:$port/" >/dev/null && break; sleep 0.2; done

html="$(curl -fs "http://localhost:$port/")"
grep -q 'data-tern-ssr' <<<"$html" || { echo "page was not server-rendered"; exit 1; }
grep -q 'Hello from Go!' <<<"$html" || { echo "load data missing"; exit 1; }
curl -fs -X POST -d name=CI "http://localhost:$port/" | grep -q 'Nice to meet you, CI!' || { echo "form action failed"; exit 1; }
[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST -d name= "http://localhost:$port/")" = 400 ] || { echo "validation failed"; exit 1; }
curl -fsI "http://localhost:$port/" | grep -qi '^x-frame-options: DENY' || { echo "hook header missing"; exit 1; }

echo "==> template OK"
