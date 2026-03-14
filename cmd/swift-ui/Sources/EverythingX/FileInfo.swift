import Foundation

/// Utility helpers mirroring internal/shared/utils.go.
enum FileInfo {
    static func getSizeMod(path: String, isDir: Bool) -> (size: String, modified: String) {
        guard let attrs = try? FileManager.default.attributesOfItem(atPath: path) else {
            return ("", "")
        }
        let modDate = (attrs[.modificationDate] as? Date) ?? Date()
        if isDir {
            return ("--", formatDate(modDate))
        }
        let bytes = (attrs[.size] as? Int64) ?? 0
        return (formatSize(bytes), formatDate(modDate))
    }

    /// Returns an ls-style formatted info string for use in tooltips.
    static func getFileInfo(path: String) -> String {
        guard let attrs = try? FileManager.default.attributesOfItem(atPath: path) else {
            return "No access"
        }
        let bytes = (attrs[.size] as? Int64) ?? 0
        let modDate = (attrs[.modificationDate] as? Date) ?? Date()
        let permissions = (attrs[.posixPermissions] as? Int) ?? 0
        let owner = (attrs[.ownerAccountName] as? String) ?? ""
        let group = (attrs[.groupOwnerAccountName] as? String) ?? ""

        let modeStr = formatPermissions(permissions)
        let sizeStr = formatSize(bytes)
        let dateStr = formatDate(modDate)

        let h = String(format: "%-13@ | %-12@ | %-12@ | %-12@ | %-20@",
                       "Mode" as NSString, "Owner" as NSString, "Group" as NSString,
                       "Size" as NSString, "Last Modified" as NSString)
        let sep = String(repeating: "-", count: 74)
        let v = String(format: "%-13@ | %-12@ | %-12@ | %-12@ | %-20@",
                       modeStr as NSString, owner as NSString, group as NSString,
                       sizeStr as NSString, dateStr as NSString)
        return "\(path)\n\n\(h)\n\(sep)\n\(v)"
    }

    /// Splits `filename` into (before, matchedPart, after) for bold highlighting.
    static func splitFileName(_ filename: String, searchTerm: String) -> (before: String, match: String, after: String) {
        let lower = filename.lowercased()
        let termLower = searchTerm.lowercased()
        guard let range = lower.range(of: termLower) else {
            return (filename, "", "")
        }
        let before = String(filename[filename.startIndex..<range.lowerBound])
        let match = String(filename[range])
        let after = String(filename[range.upperBound...])
        return (before, match, after)
    }

    // MARK: - Internal Helpers

    static func formatSize(_ bytes: Int64) -> String {
        if bytes >= 1024 * 1024 * 1024 * 1024 {
            return String(format: "%.1fT", Double(bytes) / (1024.0 * 1024.0 * 1024.0 * 1024.0))
        } else if bytes >= 1024 * 1024 * 1024 {
            return String(format: "%.1fG", Double(bytes) / (1024.0 * 1024.0 * 1024.0))
        } else if bytes >= 1024 * 1024 {
            return String(format: "%.1fM", Double(bytes) / (1024.0 * 1024.0))
        } else {
            return String(format: "%.1fK", Double(bytes) / 1024.0)
        }
    }

    static func formatDate(_ date: Date) -> String {
        let fmt = DateFormatter()
        fmt.dateFormat = "MMM d yyyy HH:mm"
        return fmt.string(from: date)
    }

    static func formatPermissions(_ permissions: Int) -> String {
        let bits: [(Int, Character)] = [
            (0o400, "r"), (0o200, "w"), (0o100, "x"),
            (0o040, "r"), (0o020, "w"), (0o010, "x"),
            (0o004, "r"), (0o002, "w"), (0o001, "x"),
        ]
        return bits.reduce("-") { acc, pair in
            acc + ((permissions & pair.0) != 0 ? String(pair.1) : "-")
        }
    }
}
