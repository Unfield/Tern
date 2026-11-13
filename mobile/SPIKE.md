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
- **Open question, other apps on the same phone:** while Shelf runs, another app could connect to its loopback port. `tern mobile` should close that: serve the pages through a `WKURLSchemeHandler` (an `app://` scheme, no port at all), or require a secret made at each launch that the shell sets as a cookie.

### In the iOS Simulator (Xcode 27.2, iPhone 18 Pro, iOS 27.2)

- **It works.** Shelf runs with its Go server inside the app: sign-up (argon2 hashing, sessions, the `kit.SignedIn` guard, a redirect after the form post), client-side navigation and server rendering in Go, with no network at all: everything the app needs is on the device, so it works offline as it is.
- **Build:** `gomobile bind` takes 40 s; `build.sh` does Tern, gomobile, XcodeGen and Xcode in one go. The Swift side built on the first try.
- **Size:** the Release app for devices is 28 MB installed and 10.6 MB zipped, about what the App Store downloads: the Go server, SQLite, accounts, search and the compiled renderer included. A small Expo app is in the same range.
- **Start time:** 0.5 to 3.2 s at first, measured from the Swift side. Most of it was `accounts` hashing a dummy password with argon2 (64 MB, ~70 ms on a Mac, far more on the Simulator) when the package loads, and gomobile runs package initialisation on the first call. Made lazy, the server starts in **40 to 165 ms**: the loading spinner barely shows. The fix helps every Tern app with accounts, and belongs on main.
- **The port changes each launch,** which makes the page a new origin every time: browser storage (localStorage, IndexedDB) starts empty, and session cookies end when the app closes, so Sam was signed out after a relaunch. With the loopback-port question above, this settles it: `tern mobile` should serve the app through a `WKURLSchemeHandler` on a fixed origin, such as `app://shelf`, and keep users signed in.
- **Focusing a field zooms the page,** as iOS does for inputs under 16 px. The shell should set the viewport's `maximum-scale=1` (or Tern UI inputs use 16 px on phones).
- Not tried yet: haptics on a real device (the Simulator can't), scrolling and gestures on a long list, the keyboard covering fields, Android.
