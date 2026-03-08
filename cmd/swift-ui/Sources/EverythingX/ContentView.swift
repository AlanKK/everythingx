import SwiftUI

struct ContentView: View {
    @ObservedObject var viewModel: SearchViewModel
    @State private var searchText = ""
    @FocusState private var searchFocused: Bool

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Image(systemName: "magnifyingglass")
                    .foregroundStyle(.secondary)
                TextField("Enter filename...", text: $searchText)
                    .textFieldStyle(.plain)
                    .font(.system(size: 15))
                    .focused($searchFocused)
                    .onChange(of: searchText) { newValue in
                        viewModel.search(term: newValue)
                    }
                if !searchText.isEmpty {
                    Button {
                        searchText = ""
                        viewModel.search(term: "")
                    } label: {
                        Image(systemName: "xmark.circle.fill")
                            .foregroundStyle(.secondary)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .background(Color(nsColor: .controlBackgroundColor))

            Divider()

            ResultsTableView(viewModel: viewModel)

            Divider()

            HStack {
                Text(viewModel.statusText)
                    .font(.system(.caption, design: .monospaced).bold())
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                Spacer()
            }
            .background(Color(nsColor: .windowBackgroundColor))
        }
        .frame(minWidth: 800, idealWidth: 1300, minHeight: 400, idealHeight: 800)
        .onAppear { searchFocused = true }
    }
}
