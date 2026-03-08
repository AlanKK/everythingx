import AppKit
import SwiftUI

struct ResultsTableView: View {
    @ObservedObject var viewModel: SearchViewModel
    @State private var selection = Set<RowData.ID>()

    var body: some View {
        Table(viewModel.rows, selection: $selection) {
            TableColumn("Name") { row in
                HighlightedNameCell(parts: row.nameParts)
                    .nsTooltip(viewModel.tooltipText(for: row))
                    .onTapGesture {
                        viewModel.copyToClipboard(path: row.fullpath)
                    }
            }
            .width(min: 150, ideal: 400)

            TableColumn("Path") { row in
                Text(row.path)
                    .font(.system(.body, design: .monospaced))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.head)
                    .allowsTightening(true)
                    .nsTooltip(viewModel.tooltipText(for: row))
            }
            .width(min: 150, ideal: 600)

            TableColumn("Size") { row in
                Text(row.size)
                    .font(.system(.body, design: .monospaced))
                    .frame(maxWidth: .infinity, alignment: .trailing)
                    .lineLimit(1)
                    .nsTooltip(viewModel.tooltipText(for: row))
            }
            .width(70)

            TableColumn("Last Modified") { row in
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

/// Name cell with bold orange highlighting on the matched search term.
private struct HighlightedNameCell: View {
    let parts: NameParts

    var body: some View {
        Text(attributedName)
            .font(.system(.body, design: .monospaced))
            .lineLimit(1)
            .truncationMode(.tail)
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

// MARK: - NSView tooltip shim

/// Sets NSView.toolTip directly, bypassing SwiftUI's .help() which does not
/// reliably propagate through Table cell views on macOS.
private struct NSTooltipModifier: ViewModifier {
    let text: String
    func body(content: Content) -> some View {
        content.background(TooltipNSView(text: text))
    }
}

private struct TooltipNSView: NSViewRepresentable {
    let text: String
    func makeNSView(context: Context) -> NSView { NSView() }
    func updateNSView(_ nsView: NSView, context: Context) {
        nsView.toolTip = text
    }
}

extension View {
    func nsTooltip(_ text: String) -> some View {
        modifier(NSTooltipModifier(text: text))
    }
}

