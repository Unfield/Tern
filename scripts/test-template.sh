#!/usr/bin/env bash
# Creates a project with `tern new`, points it at this checkout of Tern
# (the Go module and the @ternkit/tern package), then type-checks, builds
# and runs it. Used by CI to keep the starter template working.
set -euo pipefail

template="${1:-minimal}"
shift || true # the rest are flags for tern new, such as -ui
repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
cleanup() {
	status=$?
	# Only the server: "kill 0" would end the whole process group.
	if [ -n "${server:-}" ]; then kill "$server" 2>/dev/null || true; fi
	# A failed check prints what the server logged, to find flaky ones.
	if [ "$status" -ne 0 ] && [ -f "$work/server.log" ]; then
		echo "==> server log"
		tail -n 50 "$work/server.log"
	fi
	rm -rf "$work"
}
trap cleanup EXIT

echo "==> tern new -template $template $*"
(cd "$repo" && go run ./cmd/tern new -skip-go-get -template "$template" "$@" "$work/app")
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

echo "==> go test"
npm test

echo "==> run"
port=4321
PORT=$port DATABASE_PATH="$work/app.db" ./build/app >"$work/server.log" 2>&1 &
server=$!
for _ in $(seq 1 50); do curl -fs "http://localhost:$port/" >/dev/null && break; sleep 0.2; done

html="$(curl -fs "http://localhost:$port/")"
grep -q 'data-tern-ssr' <<<"$html" || { echo "page was not server-rendered"; exit 1; }

case "$template" in
minimal)
	grep -q 'Hello from Go!' <<<"$html" || { echo "load data missing"; exit 1; }
	grep -q 'Nice to meet you, CI!' <<<"$(curl -fs -X POST -d name=CI "http://localhost:$port/")" || { echo "form action failed"; exit 1; }
	[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST -d name= "http://localhost:$port/")" = 400 ] || { echo "validation failed"; exit 1; }
	grep -qi '^x-frame-options: DENY' <<<"$(curl -fsI "http://localhost:$port/")" || { echo "hook header missing"; exit 1; }
	;;
blog)
	grep -q 'Writing posts' <<<"$html" || { echo "the post list is missing"; exit 1; }
	grep -q '<h2 id="what-you-get">' <<<"$(curl -fs "http://localhost:$port/posts/hello-world")" || { echo "the post was not rendered"; exit 1; }
	grep -q '<title>Hello, world</title>' <<<"$(curl -fs "http://localhost:$port/rss.xml")" || { echo "the feed is missing"; exit 1; }
	[ -f .tern/output/client/posts/hello-world/index.html ] || { echo "the post was not prerendered"; exit 1; }
	;;
api)
	grep -q '"title":"From CI"' <<<"$(curl -fs -X POST -H 'Content-Type: application/json' -d '{"title":"From CI"}' "http://localhost:$port/api/todos")" || { echo "POST failed"; exit 1; }
	grep -q 'From CI' <<<"$(curl -fs "http://localhost:$port/api/todos?done=false")" || { echo "GET failed"; exit 1; }
	[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"title":" "}' "http://localhost:$port/api/todos")" = 400 ] || { echo "validation failed"; exit 1; }
	grep -q '"/api/todos/{id}"' <<<"$(curl -fs "http://localhost:$port/api/openapi.json")" || { echo "the OpenAPI document is missing"; exit 1; }
	;;
saas)
	grep -q 'Your product, ready on day one' <<<"$html" || { echo "the landing page is missing"; exit 1; }
	[ "$(curl -s -o /dev/null -w '%{redirect_url}' "http://localhost:$port/dashboard")" = "http://localhost:$port/login?next=%2Fdashboard" ] || { echo "the dashboard isn't protected"; exit 1; }
	jar="$work/cookies"
	origin="Origin: http://localhost:$port"
	[ "$(curl -s -o /dev/null -w '%{http_code}' -c "$jar" -H "$origin" -d 'name=CI&email=ci@example.com&password=long+enough+password' "http://localhost:$port/signup")" = 303 ] || { echo "sign-up failed"; exit 1; }
	grep -q 'Getting started' <<<"$(curl -fs -b "$jar" "http://localhost:$port/dashboard")" || { echo "the dashboard didn't load after sign-up"; exit 1; }
	created="$(curl -s -o /dev/null -w '%{http_code} %{redirect_url}' -b "$jar" -H "$origin" -d 'name=From+CI&description=made+by+the+template+test' "http://localhost:$port/projects?/create")"
	case "$created" in 2??*|303*/projects*) ;; *) echo "creating a project failed: $created"; exit 1 ;; esac
	grep -q 'From CI' <<<"$(curl -fs -b "$jar" "http://localhost:$port/projects")" || { echo "the project isn't listed"; exit 1; }
	grep -q 'Two-step sign-in' <<<"$(curl -fs -b "$jar" "http://localhost:$port/settings/security")" || { echo "the security page is missing"; exit 1; }
	grep -qE 'link"?[=:]"?http' "$work/server.log" || { echo "the verification email's link wasn't logged"; exit 1; }
	;;
esac

echo "==> template OK"
