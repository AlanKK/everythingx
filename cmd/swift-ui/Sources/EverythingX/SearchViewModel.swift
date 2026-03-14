import AppKit
import SwiftUI

struct RowData: Identifiable, Sendable {
    let id: UUID = UUID()
    let nameParts: NameParts
    let filename: String
    let path: String
    let size: String
    let sizeBytes: Int64
    let modified: String
    let modifiedDate: Date
    let fullpath: String
    let objectType: ObjectType
}

struct NameParts: Sendable {
    let before: String
    let match: String
    let after: String
}

@MainActor
final class SearchViewModel: ObservableObject {
    @Published var rows: [RowData] = []
    @Published var statusText = "0 objects"

    private let db: DatabaseManager?
    private let dbError: String?
    private let maxResults = 1000
    private var searchTask: Task<Void, Never>?

    init(db: DatabaseManager) {
        self.db = db
        self.dbError = nil
    }

    init(error: String) {
        self.db = nil
        self.dbError = error
        self.statusText = "Error: \(error)"
    }

    func search(term: String) {
        searchTask?.cancel()
        guard let db, !term.isEmpty else {
            rows = []
            statusText = dbError.map { "Error: \($0)" } ?? "0 objects"
            return
        }

        searchTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: 120_000_000) // 120 ms debounce
            guard !Task.isCancelled else { return }

            let limit = maxResults
            let results = await db.prefixSearch(term: term, limit: limit)
            guard !Task.isCancelled else { return }

            rows = results.map { r in
                let base: String
                let fp: String
                if r.objectType.isDirectory {
                    base = (r.fullpath as NSString).lastPathComponent + "/"
                    fp = r.fullpath + "/"
                } else {
                    base = (r.fullpath as NSString).lastPathComponent
                    fp = r.fullpath
                }
                let dir = (r.fullpath as NSString).deletingLastPathComponent + "/"
                let attrs = try? FileManager.default.attributesOfItem(atPath: r.fullpath)
                let sizeBytes = r.objectType.isDirectory ? Int64(0) : ((attrs?[.size] as? Int64) ?? 0)
                let modDate = (attrs?[.modificationDate] as? Date) ?? Date.distantPast
                let size = r.objectType.isDirectory ? "--" : FileInfo.formatSize(sizeBytes)
                let modified = FileInfo.formatDate(modDate)
                let (before, match, after) = FileInfo.splitFileName(base, searchTerm: term)
                return RowData(
                    nameParts: NameParts(before: before, match: match, after: after),
                    filename: base,
                    path: dir,
                    size: size,
                    sizeBytes: sizeBytes,
                    modified: modified,
                    modifiedDate: modDate,
                    fullpath: fp,
                    objectType: r.objectType
                )
            }

            if results.count == limit {
                statusText = "Showing first \(limit) objects"
            } else {
                statusText = "\(results.count) objects"
            }
        }
    }

    func copyToClipboard(path: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(path, forType: .string)
        let previous = statusText
        statusText = "✓ Copied!"
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            statusText = previous
        }
    }

    func openInFinder(path: String) {
        let cleanPath = path.hasSuffix("/") ? String(path.dropLast()) : path
        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: cleanPath)])
    }

    func tooltipText(for row: RowData) -> String {
        FileInfo.getFileInfo(path: row.fullpath)
    }
}
