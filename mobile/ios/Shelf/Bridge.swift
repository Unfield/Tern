import Security
import UIKit
import WebKit

/// Bridge answers calls from the page's JavaScript with native features:
/// window.webkit.messageHandlers.tern.postMessage({method, ...}) returns a
/// promise of the reply. src/lib/native.ts is its typed side.
final class Bridge: NSObject, WKScriptMessageHandlerWithReply {
    func userContentController(
        _ controller: WKUserContentController, didReceive message: WKScriptMessage,
        replyHandler: @escaping (Any?, String?) -> Void
    ) {
        guard let body = message.body as? [String: Any], let method = body["method"] as? String else {
            replyHandler(nil, "tern: a call needs a method")
            return
        }
        switch method {
        case "haptic":
            haptic(body["style"] as? String ?? "light")
            replyHandler(true, nil)
        case "store.get":
            replyHandler(Keychain.get(body["key"] as? String ?? ""), nil)
        case "store.set":
            Keychain.set(body["key"] as? String ?? "", body["value"] as? String)
            replyHandler(true, nil)
        default:
            replyHandler(nil, "tern: no native method \(method)")
        }
    }

    private func haptic(_ style: String) {
        switch style {
        case "success": UINotificationFeedbackGenerator().notificationOccurred(.success)
        case "warning": UINotificationFeedbackGenerator().notificationOccurred(.warning)
        case "error": UINotificationFeedbackGenerator().notificationOccurred(.error)
        case "heavy": UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
        case "medium": UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        default: UIImpactFeedbackGenerator(style: .light).impactOccurred()
        }
    }
}

/// Keychain keeps small secrets, such as tokens, encrypted by the system.
enum Keychain {
    static func get(_ key: String) -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: key,
            kSecReturnData as String: true, kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess, let data = item as? Data else {
            return nil
        }
        return String(data: data, encoding: .utf8)
    }

    static func set(_ key: String, _ value: String?) {
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrAccount as String: key]
        SecItemDelete(query as CFDictionary)
        guard let value else { return }
        var add = query
        add[kSecValueData as String] = Data(value.utf8)
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        SecItemAdd(add as CFDictionary, nil)
    }
}
