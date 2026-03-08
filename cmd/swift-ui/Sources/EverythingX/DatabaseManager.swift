import Foundation
import SQLite3

struct SearchResult: Sendable {
    let fullpath: String
    let objectType: ObjectType
}

enum ObjectType: Int, Sendable {
    case file = 0
    case directory = 1
    case symlink = 2
    case namedPipe = 3
    case socket = 4
    case charDevice = 5
    case blockDevice = 6

    var isDirectory: Bool { self == .directory }
}

enum DatabaseError: Error {
    case openFailed(String)
    case prepareFailed(String)
    case notFound
}

/// Thread-safe SQLite read-only access to the everythingx file index.
actor DatabaseManager {
    // nonisolated(unsafe) allows deinit to finalize these C pointers
    // without triggering Swift 6 actor-isolation errors.
    nonisolated(unsafe) private var db: OpaquePointer?
    nonisolated(unsafe) private var prefixSearchStmt: OpaquePointer?

    init(path: String) throws {
        let flags = SQLITE_OPEN_READONLY | SQLITE_OPEN_URI | SQLITE_OPEN_NOMUTEX
        let uri = "file:\(path)?mode=ro"
        guard sqlite3_open_v2(uri, &db, flags, nil) == SQLITE_OK else {
            throw DatabaseError.openFailed("Cannot open database at \(path)")
        }
        let sql = """
            SELECT fullpath, object_type
            FROM files
            WHERE filename LIKE ? COLLATE BINARY
            ORDER BY filename ASC
            LIMIT ?
            """
        guard sqlite3_prepare_v2(db, sql, -1, &prefixSearchStmt, nil) == SQLITE_OK else {
            throw DatabaseError.prepareFailed("Cannot prepare search statement")
        }
    }

    deinit {
        sqlite3_finalize(prefixSearchStmt)
        sqlite3_close(db)
    }

    func prefixSearch(term: String, limit: Int = 1000) -> [SearchResult] {
        guard let stmt = prefixSearchStmt else { return [] }
        let searchTerm = "%\(term)%"
        sqlite3_bind_text(stmt, 1, (searchTerm as NSString).utf8String, -1, nil)
        sqlite3_bind_int(stmt, 2, Int32(limit))

        var results: [SearchResult] = []
        results.reserveCapacity(limit)
        while sqlite3_step(stmt) == SQLITE_ROW {
            guard let cStr = sqlite3_column_text(stmt, 0) else { continue }
            let fullpath = String(cString: cStr)
            let typeRaw = Int(sqlite3_column_int(stmt, 1))
            let objectType = ObjectType(rawValue: typeRaw) ?? .file
            results.append(SearchResult(fullpath: fullpath, objectType: objectType))
        }
        sqlite3_reset(stmt)
        return results
    }
}
