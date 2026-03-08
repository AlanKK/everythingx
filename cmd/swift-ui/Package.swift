// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "EverythingX",
    platforms: [
        .macOS(.v13)
    ],
    targets: [
        .executableTarget(
            name: "EverythingX",
            path: "Sources/EverythingX",
            linkerSettings: [
                .linkedLibrary("sqlite3")
            ]
        )
    ]
)
