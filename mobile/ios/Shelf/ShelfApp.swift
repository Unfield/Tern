import Shelfapp // the Go server, built by gomobile into Shelfapp.xcframework
import SwiftUI

/// Shelf on the phone: the Tern app's Go server runs in-process, and a web
/// view shows its pages. See mobile/SPIKE.md.
@main
struct ShelfApp: App {
    @StateObject private var server = Server()

    var body: some Scene {
        WindowGroup {
            Group {
                switch server.state {
                case .starting:
                    ProgressView().controlSize(.large)
                case .ready(let url):
                    WebView(url: url).ignoresSafeArea(.container, edges: .bottom)
                case .failed(let message):
                    Text(message).padding().multilineTextAlignment(.center)
                }
            }
            .task { await server.start() }
        }
    }
}

/// Server starts the Go server once, off the main thread.
@MainActor
final class Server: ObservableObject {
    enum State {
        case starting
        case ready(URL)
        case failed(String)
    }

    @Published var state = State.starting

    func start() async {
        guard case .starting = state else { return }
        let started = Date()
        let result: Result<String, Error> = await Task.detached {
            do {
                let dir = try FileManager.default.url(
                    for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
                var err: NSError?
                let addr = ShelfappStart(dir.path, &err)
                if let err { return .failure(err) }
                return .success(addr)
            } catch {
                return .failure(error)
            }
        }.value
        switch result {
        case .success(let addr):
            print("tern: server started in \(Int(Date().timeIntervalSince(started) * 1000)) ms at \(addr)")
            state = .ready(URL(string: addr)!)
        case .failure(let error):
            state = .failed("Shelf couldn't start: \(error.localizedDescription)")
        }
    }
}
