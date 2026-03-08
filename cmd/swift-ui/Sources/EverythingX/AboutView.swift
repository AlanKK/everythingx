import SwiftUI

struct AboutView: View {
    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "folder.fill")
                .resizable()
                .scaledToFit()
                .frame(width: 80, height: 80)
                .foregroundStyle(.orange)

            Text("EverythingX")
                .font(.title.bold())

            Text("Version \(appVersion)")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Divider()

            VStack(alignment: .leading, spacing: 6) {
                LabeledContent("Author", value: "Alan Keister")
                LabeledContent("License", value: "MIT")
            }
            .font(.body)

            Link("View on GitHub", destination: URL(string: "https://github.com/AlanKK/everythingx")!)
                .font(.body)

            Spacer()
        }
        .padding(24)
        .frame(width: 360, height: 320)
    }

    private var appVersion: String {
        Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev"
    }
}
