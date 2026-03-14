import AppKit
import SwiftUI

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem?
    private var aboutWindow: NSWindow?

    func applicationDidFinishLaunching(_ notification: Notification) {
        // When running as a plain binary (not a .app bundle), macOS won't
        // automatically grant foreground/keyboard focus. Explicitly request it.
        NSApp.setActivationPolicy(.regular)
        if let url = Bundle.module.url(forResource: "app-icon", withExtension: "png"),
           let img = NSImage(contentsOf: url) {
            NSApp.applicationIconImage = img
        }
        setupStatusItem()
        NSApp.activate(ignoringOtherApps: true)
        DispatchQueue.main.async {
            NSApp.windows.first(where: { !($0 is NSPanel) })?.makeKeyAndOrderFront(nil)
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        // Keep app alive in the tray when the main window is closed.
        return false
    }

    private func setupStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = statusItem?.button {
            if let url = Bundle.module.url(forResource: "menubar-icon", withExtension: "png"),
               let img = NSImage(contentsOf: url) {
                img.isTemplate = true  // adapts to light/dark menu bar
                button.image = img
            } else {
                button.image = NSImage(systemSymbolName: "folder.fill", accessibilityDescription: "EverythingX")
            }
        }

        let menu = NSMenu()
        menu.addItem(
            NSMenuItem(title: "Show EverythingX", action: #selector(showMainWindow), keyEquivalent: ""))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(
            NSMenuItem(title: "Settings…", action: #selector(showSettings), keyEquivalent: ","))
        menu.addItem(
            NSMenuItem(title: "About EverythingX", action: #selector(showAbout), keyEquivalent: ""))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(
            NSMenuItem(title: "Quit EverythingX", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q"))
        statusItem?.menu = menu
    }

    @objc private func showMainWindow() {
        NSApp.activate(ignoringOtherApps: true)
        if let win = NSApp.windows.first(where: { !($0 is NSPanel) }) {
            win.makeKeyAndOrderFront(nil)
        }
    }

    @objc private func showSettings() {
        // Placeholder — open Settings window
    }

    @objc func showAbout() {
        if aboutWindow == nil {
            let view = NSHostingView(rootView: AboutView())
            let window = NSPanel(
                contentRect: NSRect(x: 0, y: 0, width: 360, height: 320),
                styleMask: [.titled, .closable, .miniaturizable],
                backing: .buffered,
                defer: false
            )
            window.title = "About EverythingX"
            window.contentView = view
            window.center()
            window.isReleasedWhenClosed = false
            aboutWindow = window
        }
        aboutWindow?.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }
}
