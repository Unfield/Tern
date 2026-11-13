# Spike: Tern apps on phones

Branch `spike/mobile`. A spike, not a feature: it answers whether a Tern app can become a good iOS and Android app, and at what cost, before any ADR.

## The idea

Not a React Native–style renderer: Svelte 5 compiles to DOM calls and has no custom-renderer API, so driving native widgets would mean a fake DOM or a fork of the compiler, and native versions of Tern UI.

Instead, Expo's developer experience around the app as it is:

- **A native shell:** the app's client runs in the system web view (WKWebView, Android WebView), with native APIs through a bridge.
- **The Go server on the device:** Go builds for iOS and Android with gomobile, and since v1.13 pages render without a JavaScript engine. The app's own server (loads, actions, SQLite, typed SQL, jobs) can run in-process on the phone, so the app works offline, and syncs with the real server when it's online. Nothing like it exists for Expo or SvelteKit.
- **Expo's workflow:** `tern mobile dev` shows a QR code that a dev app on the phone opens, with hot reload; `tern mobile build` makes signed builds, which keel.run could host the way EAS does for Expo.

## What the spike builds

Shelf, the tutorial's app, as an iOS app (Android second):

1. The Go server built as a library with gomobile (`gomobile bind`), started by the app on a loopback port or a custom URL scheme handler, with its SQLite database in the app's data directory.
2. A Swift shell: a WKWebView that loads the app from the in-process server.
3. One native API over the bridge, such as haptics or secure storage, called from Svelte with a typed function.
4. Running in the iOS Simulator.

## What it measures

- **Feel:** start time, scrolling, the keyboard, gestures, navigation between pages. The web view is where this succeeds or fails.
- **Size:** the app's download size with the Go server and native renderer in it.
- **Offline:** loads, actions and the database with no network.
- **Effort:** what's generic, for `tern mobile`, and what's per app.
- **Store review:** what keeps it clear of App Store guideline 4.2 (apps that are only a website).

## Needs

- **iOS:** Xcode (the full app, not only the Command Line Tools), and `go install golang.org/x/mobile/cmd/gomobile@latest`.
- **Android:** the Android NDK, a system image and an emulator device (`sdkmanager`), for the second half.

## Findings

- **A Tern app works as a library.** Code generation already put `tern.gen.go` in the root package by its name; the native renderer assumed `main`, now it doesn't. `mobile/shelf` is Shelf with its root package as `shelfapp`: `Start(dataDir)` serves it on a loopback port (127.0.0.1:0), with its database in the app's data directory, which is the shape gomobile binds (strings and an error). Its test passes on macOS: signed out, `/` redirects to sign in, and `/login` is rendered in Go.
- **The shell is small:** `mobile/ios/Shelf` is three Swift files: the app starts the server off the main thread and shows a WKWebView; the bridge answers `postMessage` calls with promises (haptics, Keychain). `src/lib/native.ts` is the typed side, and does nothing in a browser.
- **Security defaults hold on the phone:** the server is on loopback only, so other devices on the network can't reach it; the rate limit leaves loopback alone; the Content-Security-Policy doesn't apply to the native bridge.
- Still to measure, with Xcode: start time, feel, size, offline.
