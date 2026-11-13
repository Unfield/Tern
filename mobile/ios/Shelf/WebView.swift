import SwiftUI
import WebKit

/// WebView shows the app's pages, from the server on the device, with the
/// native bridge: window.webkit.messageHandlers.tern.
struct WebView: UIViewRepresentable {
    let url: URL

    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.userContentController.addScriptMessageHandler(Bridge(), contentWorld: .page, name: "tern")
        // The page knows it's in the app: src/lib/native.ts checks for this.
        config.applicationNameForUserAgent = "TernNative/1"
        let view = WKWebView(frame: .zero, configuration: config)
        view.allowsBackForwardNavigationGestures = true
        view.scrollView.contentInsetAdjustmentBehavior = .automatic
        view.isInspectable = true // Safari's Web Inspector, in development
        view.load(URLRequest(url: url))
        return view
    }

    func updateUIView(_ view: WKWebView, context: Context) {}
}
