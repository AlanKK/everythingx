import AppKit
import SwiftUI

struct ResultsTableView: View {
    @ObservedObject var viewModel: SearchViewModel
    @State private var selection = Set<RowData.ID>()
    @State private var sortOrder: [KeyPathComparator<RowData>] = []

    private var sortedRows: [RowData] {
        viewModel.rows.sorted(using: sortOrder)
    }

    var body: some View {
        Table(sortedRows, selection: $selection, sortOrder: $sortOrder) {
            TableColumn("Name", value: \RowData.filename) { row in
                HighlightedNameCell(parts: row.nameParts, fullpath: row.fullpath)
                    .nsTooltip(viewModel.tooltipText(for: row))
                    .onTapGesture {
                        viewModel.copyToClipboard(path: row.fullpath)
                    }
            }
            .width(min: 150, ideal: 400)

            TableColumn("Path", value: \RowData.path) { row in
                Text(row.path)
                    .font(.system(.body, design: .monospaced))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.head)
                    .allowsTightening(true)
                    .nsTooltip(viewModel.tooltipText(for: row))
            }
            .width(min: 150, ideal: 600)

            TableColumn("Size", value: \RowData.sizeBytes) { row in
                Text(row.size)
                    .font(.system(.body, design: .monospaced))
                    .frame(maxWidth: .infinity, alignment: .trailing)
                    .lineLimit(1)
                    .nsTooltip(viewModel.tooltipText(for: row))
            }
            .width(70)

            TableColumn("Last Modified", value: \RowData.modifiedDate) { row in
                Text(row.modified)
                    .font(.system(.body, design: .monospaced))
                    .lineLimit(1)
                    .nsTooltip(viewModel.tooltipText(for: row))
            }
            .width(190)
        }
        .contextMenu(forSelectionType: RowData.ID.self) { ids in
            if let id = ids.first, let row = viewModel.rows.first(where: { $0.id == id }) {
                Button("Copy Full Path") {
                    viewModel.copyToClipboard(path: row.fullpath)
                }
                Button("Reveal in Finder") {
                    viewModel.openInFinder(path: row.fullpath)
                }
            }
        } primaryAction: { ids in
            // Double-click → reveal in Finder
            if let id = ids.first, let row = viewModel.rows.first(where: { $0.id == id }) {
                viewModel.openInFinder(path: row.fullpath)
            }
        }
    }
}

/// Lazy-loads the file's Finder icon when the cell appears.
private struct FileIconView: View {
    let path: String
    @State private var icon: NSImage? = nil

    var body: some View {
        Group {
            if let icon {
                Image(nsImage: icon)
                    .resizable()
                    .interpolation(.high)
                    .frame(width: 18, height: 18)
                    .shadow(color: .black.opacity(0.25), radius: 0.5, x: 0, y: 0.5)
            } else {
                Color.clear.frame(width: 18, height: 18)
            }
        }
        .onAppear {
            guard icon == nil else { return }
            let clean = path.hasSuffix("/") ? String(path.dropLast()) : path
            let img = NSWorkspace.shared.icon(forFile: clean)
            img.size = NSSize(width: 32, height: 32)
            icon = img
        }
    }
}

/// Name cell with bold orange highlighting on the matched search term.
private struct HighlightedNameCell: View {
    let parts: NameParts
    let fullpath: String

    var body: some View {
        HStack(spacing: 4) {
            FileIconView(path: fullpath)
            Text(attributedName)
                .font(.system(.body, design: .monospaced))
                .lineLimit(1)
                .truncationMode(.tail)
        }
    }

    private var attributedName: AttributedString {
        var result = AttributedString()
        if !parts.before.isEmpty {
            result += AttributedString(parts.before)
        }
        if !parts.match.isEmpty {
            var highlight = AttributedString(parts.match)
            highlight.font = .system(.body, design: .monospaced).bold()
            highlight.foregroundColor = .orange
            result += highlight
        }
        if !parts.after.isEmpty {
            result += AttributedString(parts.after)
        }
        return result
    }
}

// MARK: - Custom monospace tooltip

private let kTooltipDelay: TimeInterval = 0.25

/// NSView subclass that shows a custom floating tooltip panel with monospace font
/// and a short appearance delay (kTooltipDelay), bypassing the slow system tooltip.
private final class MonoTooltipView: NSView {
    var tooltipText: String {
        didSet { cancelTooltip() }
    }
    private var trackingArea: NSTrackingArea?
    private weak var tooltipPanel: NSPanel?
    private var showTimer: Timer?

    init(text: String) {
        self.tooltipText = text
        super.init(frame: .zero)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let t = trackingArea { removeTrackingArea(t) }
        trackingArea = NSTrackingArea(
            rect: bounds,
            options: [.mouseEnteredAndExited, .activeInActiveApp, .inVisibleRect],
            owner: self,
            userInfo: nil
        )
        addTrackingArea(trackingArea!)
    }

    override func mouseEntered(with event: NSEvent) {
        showTimer?.invalidate()
        showTimer = Timer.scheduledTimer(withTimeInterval: kTooltipDelay, repeats: false) { [weak self] _ in
            DispatchQueue.main.async { self?.presentTooltip() }
        }
    }

    override func mouseExited(with event: NSEvent) {
        cancelTooltip()
    }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        if window == nil { cancelTooltip() }
    }

    private func cancelTooltip() {
        showTimer?.invalidate()
        showTimer = nil
        tooltipPanel?.close()
        tooltipPanel = nil
    }

    private func presentTooltip() {
        guard tooltipPanel == nil, window != nil else { return }

        let padding: CGFloat = 10
        let font = NSFont.monospacedSystemFont(ofSize: 11, weight: .regular)

        // Measure text using cell sizing for reliable multi-line result
        let field = NSTextField(wrappingLabelWithString: tooltipText)
        field.font = font
        let cellSize = field.cell!.cellSize(forBounds: CGRect(x: 0, y: 0, width: 1200, height: 10000))
        let textSize = CGSize(width: ceil(cellSize.width), height: ceil(cellSize.height))
        let contentSize = CGSize(width: textSize.width + padding * 2, height: textSize.height + padding * 2)

        field.isBezeled = false
        field.isEditable = false
        field.drawsBackground = false
        field.frame = CGRect(x: padding, y: padding, width: textSize.width, height: textSize.height)

        let container = NSVisualEffectView(frame: CGRect(origin: .zero, size: contentSize))
        container.material = .toolTip
        container.blendingMode = .behindWindow
        container.state = .active
        container.wantsLayer = true
        container.layer?.cornerRadius = 5
        container.layer?.masksToBounds = true
        container.addSubview(field)

        let panel = NSPanel(
            contentRect: CGRect(origin: .zero, size: contentSize),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: false
        )
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.level = .floating
        panel.hasShadow = true
        panel.contentView = container

        var origin = NSEvent.mouseLocation
        origin.y -= contentSize.height + 16
        origin.x += 10
        if let screen = NSScreen.main {
            let sv = screen.visibleFrame
            if origin.x + contentSize.width > sv.maxX { origin.x = sv.maxX - contentSize.width - 4 }
            if origin.y < sv.minY { origin.y = NSEvent.mouseLocation.y + 20 }
        }
        panel.setFrameOrigin(origin)
        panel.orderFront(nil)
        tooltipPanel = panel
    }
}

private struct NSTooltipModifier: ViewModifier {
    let text: String
    func body(content: Content) -> some View {
        content.background(TooltipNSView(text: text))
    }
}

private struct TooltipNSView: NSViewRepresentable {
    let text: String
    func makeNSView(context: Context) -> MonoTooltipView { MonoTooltipView(text: text) }
    func updateNSView(_ nsView: MonoTooltipView, context: Context) {
        nsView.tooltipText = text
    }
}

extension View {
    func nsTooltip(_ text: String) -> some View {
        modifier(NSTooltipModifier(text: text))
    }
}

