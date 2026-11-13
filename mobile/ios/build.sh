#!/bin/bash
# build.sh builds Shelf for the iOS Simulator: the Go server with gomobile,
# the Xcode project with XcodeGen, then the app. Needs Xcode, and:
#   go install golang.org/x/mobile/cmd/gomobile@latest (gobind is a tool of mobile/shelf's go.mod)
#   brew install xcodegen
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
shelf="$here/../shelf"

echo "==> tern build (client, renderer compiled to Go)"
(cd "$shelf" && go tool tern build -o build/app >/dev/null)

echo "==> gomobile bind"
mkdir -p "$here/build"
(cd "$shelf" && gomobile bind -target iossimulator,ios -tags tern_nojs -o "$here/build/Shelfapp.xcframework" .)

echo "==> xcodegen"
(cd "$here" && xcodegen generate --quiet)

echo "==> xcodebuild"
xcodebuild -project "$here/Shelf.xcodeproj" -scheme Shelf -sdk iphonesimulator -configuration Debug \
	-derivedDataPath "$here/build/derived" build | tail -3
echo "$here/build/derived/Build/Products/Debug-iphonesimulator/Shelf.app"
