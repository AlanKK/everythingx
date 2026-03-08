import SwiftUI

@main
struct EverythingXApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var viewModel: SearchViewModel

    init() {
        let dbPath = Self.resolveDBPath()
        if let db = try? DatabaseManager(path: dbPath) {
            _viewModel = StateObject(wrappedValue: SearchViewModel(db: db))
        } else {
            _viewModel = StateObject(wrappedValue: SearchViewModel(error: "Cannot open database at \(dbPath)"))
        }
    }

    var body: some Scene {
        WindowGroup("EverythingX") {
            ContentView(viewModel: viewModel)
        }
        .windowStyle(.titleBar)
        .defaultSize(width: 1300, height: 800)
        .commands {
            CommandGroup(replacing: .appInfo) {
                Button("About EverythingX") {
                    appDelegate.showAbout()
                }
            }
        }
    }

    private static func resolveDBPath() -> String {
        // Allow overriding via --db-path=<path> launch argument.
        for arg in CommandLine.arguments.dropFirst() {
            if arg.hasPrefix("--db-path=") {
                return String(arg.dropFirst("--db-path=".count))
            }
        }
        return "/var/lib/everythingx/files.db"
    }
}
