# UI Improvements Plan — EverythingX GUI

> **Status: executed.** Features 1, 2, 3 and 4 are implemented in full. Feature 6 shipped only as a minimal menu bar (File → Close Window, Help → About) — the theme toggle and sort submenu described in its section were **not** built, as both would add behaviour the app does not otherwise have. Feature 5 is blocked (see the note in its section). Two findings changed the plan during execution:
> - `everythingxTheme.Icon` returned the app logo for *every* icon name, so all file-type icons would have been identical. Fixed first; pinned by `TestThemeIconsAreDistinct`.
> - Table cells are recycled across columns, so showing the icon after `CreateCell` needs `inner.Refresh()` to re-run the border layout — without it the icons occupied space but never painted.

**Audience:** an engineering agent implementing this end-to-end.
**Scope:** `cmd/everythingx/` only (Fyne GUI). No DB/schema changes required for the core work.
**Goal:** close the perceived gap with *Everything for Windows* and *Windows File Explorer* by adding file-type icons, sortable columns, and a right-click context menu (replacing today's surprising left-click-to-copy). Keyboard navigation, resizable columns, and a menu bar are optional follow-ons.

> Read `CLAUDE.md` first. Honor its conventions: no added abstraction/flexibility/fallbacks, keep comments short, use `shared.ObjectType` constants (never raw ints), GUI opens the DB read-only, and every UI mutation off the main thread **must** be wrapped in `fyne.Do` (the `fyneDo` migration is on, so wrong-thread bugs will *not* warn — they will just race).

---

## Current state (as of this plan)

`cmd/everythingx/ui.go` (432 lines) builds the whole UI:

- `makeTable()` creates a `widget.NewTableWithHeaders` with 4 columns: Name(0) / Path(1) / Size(2) / Last Modified(3). Column widths 400/600/70/190.
- Each cell is a single `*tooltipCell` (embeds `ttwidget.RichText`) — **text only, no icon support today.**
- `RowData` holds `Name []string` (3 parts: before/match/after for highlight), `Path`, `Size`, `Modified`, and `*shared.SearchResult`.
- Search results live entirely in the package global `tableData []RowData` (capped at `maxSearchResults = 1000`), so **all rows are in memory** — client-side sorting is trivial and needs no DB round-trip.
- `tooltipCell.Tapped` (left-click) currently **copies the path to the clipboard** and flashes "✓ Copied!" in the status bar. This is non-standard and should be replaced (see Feature 3).
- `handleOpenFile(path)` exists in `open_darwin.go` (`open -R`) and `open_linux.go` (`xdg-open` on the parent dir) but is **currently never called** — there is no wired-up open/reveal action anywhere. Wiring it is part of Feature 3.
- Sorting is fixed: `ffdb.PrefixSearch` orders by `filename ASC` in SQL. Do not change SQL for sorting; sort `tableData` in memory.
- Theme (`theme.go`) forces monospace (Roboto Mono), `SizeNamePadding = 0`, `SizeNameSeparatorThickness = 0`, `SizeNameLineSpacing = 4`.
- Available icon resources today: only `resourceFolderWhiteOrange5122xPng` and `resourceFolderOrangeBlack5122xPng` (the app logo). Use Fyne's built-in `theme.*Icon()` set for file-type icons rather than shipping new assets.

Build/verify with `make build` then run `bin/everythingx` (needs the daemon's DB at `/var/lib/everythingx/files.db`, or pass `-db_path`). Run `make test` for unit tests.

---

## Feature 1 — File-type icons in the Name column (highest visual impact)

Add an icon to the left of each filename, chosen from `ObjectType` + file extension. Mirrors both reference apps.

### Approach
1. Change the Name column cell from a bare `*tooltipCell` to a horizontal container `icon + tooltipCell`. The other three columns stay text-only. Two clean options — pick one and keep it consistent:
   - **(Preferred)** Make `CreateCell` return a cell type that contains both a `*widget.Icon` and the RichText, and only show/populate the icon when `id.Col == 0`. Because Fyne reuses one CreateCell factory for all columns, the simplest robust structure is a small custom cell widget wrapping an `*widget.Icon` (hidden for cols 1–3) beside the existing RichText. Keep the tooltip behavior working on the text.
   - Alternative: keep `tooltipCell` as-is and prepend the icon via a `container.NewHBox(icon, richText)` inside `CreateCell`, casting appropriately in `UpdateCell`. Whichever is chosen, the icon must be hidden/blank for cols 1–3 so widths and alignment are unaffected.
2. Add an icon-picker helper in `ui.go`:
   ```go
   // iconForRow returns the theme icon for a result based on its object type
   // and, for regular files, its extension.
   func iconForRow(r *shared.SearchResult) fyne.Resource
   ```
   Mapping (use `shared.ObjectType` constants, never raw ints):
   - `ItemIsDir` → `theme.FolderIcon()`
   - `ItemIsSymlink` → `theme.ContentLinkIcon()` (or `theme.FolderOpenIcon()` if the target is a dir — do **not** stat here; keep it cheap, base it on `ObjectType` only)
   - `ItemIsFile` → by lowercased `filepath.Ext`:
     - images (`.png .jpg .jpeg .gif .webp .bmp .svg .heic .tiff`) → `theme.FileImageIcon()`
     - audio (`.mp3 .wav .flac .aac .ogg .m4a`) → `theme.FileAudioIcon()`
     - video (`.mp4 .mov .avi .mkv .webm`) → `theme.FileVideoIcon()`
     - text/code (`.txt .md .go .py .js .ts .json .yaml .yml .toml .c .h .cpp .rs .java .sh .html .css .xml .csv .log`) → `theme.FileTextIcon()`
     - archives (`.zip .tar .gz .tgz .bz2 .xz .7z .rar`) → `theme.FileApplicationIcon()`
     - apps/binaries (`.app .exe .dmg .pkg .deb .rpm`) → `theme.FileApplicationIcon()`
     - default → `theme.FileIcon()`
   - Everything else (pipe/socket/char/block device) → `theme.FileIcon()`
   - Keep the extension→category tables as small package-level maps in `ui.go`.
3. In `UpdateCell` for `id.Col == 0`, set the icon resource from `iconForRow(row.SearchResult)` and make the icon visible; for other columns hide it (or don't create it). Keep icon size small (it will follow row height; `theme.SizeNameLineSpacing` is 4 and padding 0, so verify visual density — a tight ~16px icon).

### Acceptance
- Folders, images, text/code, and generic files show visibly distinct icons; directories still get the trailing `/` on the name.
- Columns 1–3 render exactly as before (no phantom icon, no width shift).
- Hover tooltip still works on Name cells; match highlight (orange bold) still works.
- `make build` clean; app runs and a search shows icons.

---

## Feature 2 — Sortable columns (click header to sort, with ▲/▼ indicator)

Clicking a column header sorts the in-memory results by that column; clicking again reverses. Show a sort-direction indicator in the active header.

### Approach
1. Add sort state as package globals in `ui.go`:
   ```go
   var sortCol int = -1   // -1 = no user sort (default DB order)
   var sortAsc bool = true
   ```
2. `widget.Table` reports header clicks via its `OnSelected`-style header interaction. Use the table's header tap: set `t.OnSelected` is for cells; for headers use `t.Header`… — concretely, wire header clicks through the table's `CreateHeader`/`UpdateHeader` label by making the header a tappable widget, **or** use `widget.Table`'s built-in `OnSelected` on header cells if the installed Fyne version exposes it. Check the vendored Fyne API first (`go doc fyne.io/fyne/v2/widget.Table`) and use whatever header-click hook that version provides. Do not upgrade Fyne for this.
3. On header click for column `c`:
   - if `sortCol == c`, flip `sortAsc`; else set `sortCol = c`, `sortAsc = true`.
   - Sort `tableData` with `sort.SliceStable`. Comparators:
     - Name (0): compare the reconstructed base name (`Name[0]+Name[1]+Name[2]`), case-insensitive, `strings.ToLower`.
     - Path (1): compare `Path` case-insensitive.
     - Size (2): **do not sort the formatted string** ("3.4M" < "155.9K" would be wrong). Sort by a numeric byte size. Add a numeric field to `RowData` (e.g. `SizeBytes int64`, `-1` for dirs) populated in `handleAutoCompleteEntryChanged`. `shared.GetFileSizeMod` returns only formatted strings, so either add a sibling that also returns raw bytes or `os.Stat` once in the build loop (you already stat there via `GetFileSizeMod` — prefer extending that path over a second stat; if adding a shared func, follow CLAUDE.md and keep it single-purpose).
     - Modified (3): sort by a real timestamp, not the formatted string. Same treatment — carry a `time.Time` (or unix int64) in `RowData`.
   - `t.Refresh()` and `t.ScrollToTop()` after sorting; update the header indicator.
4. Header indicator: in `UpdateHeader`, append `" ▲"` / `" ▼"` to the active column's label text (keep bold). Leave others plain.
5. Sorting must be applied **on the UI thread** (it mutates `tableData` which the table reads). The header click handler already runs on the UI thread — no `fyne.Do` needed there. But note new searches rebuild `tableData` off-thread: after a new search completes, re-apply the current sort inside the existing final `fyne.Do` closure in `handleAutoCompleteEntryChanged` (so results stay sorted as the user types), or reset `sortCol = -1` on each new search — **pick the "persist sort across searches" behavior**, it matches Everything.

### Acceptance
- Clicking Name/Path/Size/Modified sorts correctly; Size and Modified sort by real magnitude/time, not string.
- Clicking the same header again reverses; indicator ▲/▼ reflects direction.
- Sort persists as you keep typing (new results come back in the chosen order).
- No data race (verify by scrolling while typing; sorting happens on UI thread only).

---

## Feature 3 — Right-click context menu + fix left-click behavior

Today left-click copies the path (surprising) and `handleOpenFile` is dead code. Align with both reference apps: left-click selects, right-click opens a context menu, double-click/Enter reveals in the file manager.

### Approach
1. **Remove** the copy-on-left-click from `tooltipCell.Tapped`. Left-click should just select the row (Fyne table selection). Keep the "✓ Copied!" status-flash helper but move it to the copy menu actions.
2. Implement `TappedSecondary(*fyne.PointEvent)` on `tooltipCell` to show a `widget.NewPopUpMenu` (or `fyne.NewMenu` in a `widget.PopUp`) anchored at the event position, with items:
   - **Open / Reveal in Finder** (macOS) / **Open containing folder** (Linux) → call `handleOpenFile(c.path)` (finally wiring the existing per-OS function).
   - **Copy full path** → clipboard = `c.path`, flash "✓ Copied!".
   - **Copy name** → clipboard = `filepath.Base(c.path)`, flash.
   - (optional) **Copy containing folder** → clipboard = `filepath.Dir(c.path)`.
   - Use `mainWindow.Clipboard()` as today. Reuse the status-bar flash pattern already in `Tapped` (the `time.AfterFunc` + `fyne.Do` restore of `lastResultText`).
3. **Double-click to reveal:** implement `DoubleTapped(*fyne.PointEvent)` on `tooltipCell` → `handleOpenFile(c.path)`. (Everything/Explorer both open on double-click.)
4. Keep tooltip-on-hover unchanged.

### Acceptance
- Left-click no longer copies; it selects the row (visible selection highlight).
- Right-click shows a menu; each action works on macOS (verify `open -R`) — Linux paths compile (guarded by build tags already).
- Double-click reveals the file in Finder/file manager.
- `handleOpenFile` is now referenced (no dead-code).

---

## Feature 4 (optional) — Keyboard navigation

Arrow keys move the selected row, Enter reveals it, Esc refocuses the search box, and typing stays in the search entry.

### Approach
- The search `entry` holds focus by default (`w.Canvas().Focus(entry)`). Add key handling via `w.Canvas().SetOnTypedKey` (or a custom entry) for `fyne.KeyDown`/`KeyUp` to move a selection index, `fyne.KeyReturn`/`KeyEnter` → `handleOpenFile` on the selected row, `fyne.KeyEscape` → clear/refocus entry.
- Maintain a `selectedRow int` global; drive `t.Select(widget.TableCellID{Row: selectedRow, Col: 0})` and `t.ScrollTo(...)` to keep it visible.
- Guard against typing conflicts: ↑/↓ must not insert into the entry. Keep it simple; do not add a mode system.

### Acceptance
- ↑/↓ move selection and keep it on screen; Enter reveals; Esc returns focus to search. Typing still filters.

---

## Feature 5 (optional) — Resizable columns

Paths are frequently ellipsized. Enable header drag-resize.

### Outcome: enabled by raising the theme padding

**Resolved.** `theme.go` now sets `SizeNamePadding = 2` (was `0`), which restores column resizing. Each header also carries a thin trailing rule (`ColorNameSeparator`) as the grab affordance — added in `CreateHeader`/`UpdateHeader` rather than via `SizeNameSeparatorThickness`, because Fyne's table draws dividers for rows *and* columns, so raising the thickness would rule every row as well. Raise it further if the 2px drag target feels too narrow; `4` is comfortable but costs roughly 17% of the visible rows and breaks the row highlight into per-column segments. Original diagnosis below.



Fyne 2.8 *does* support header divider drag-resize natively (`Table.Dragged`/`DragEnd`, `dragCol`, `HResizeCursor`) — no flag needed. But it is unreachable in this app:

`Table.columnAt` only reports a divider hit inside the inter-column gap, and that gap is `theme.SizeNamePadding` wide. `theme.go` sets `SizeNamePadding = 0`, so the gap is zero-width and `hoverHeaderCol` is never set — the drag can never start. The same zero padding (with `SizeNameSeparatorThickness = 0`) is why the table shows no column separator lines.

Enabling resize therefore requires a non-zero `SizeNamePadding`, which loosens spacing across every widget in the app and would soften the deliberately dense, Everything-like layout. **That is a design call for the owner, so it was left alone.** If wanted, the change is one line in `theme.go` (`SizeNamePadding` → e.g. `2`), and it would also restore visible column dividers.

Side effect worth knowing: because divider drag can never start, the tappable sort headers added in Feature 2 cannot conflict with column resizing.

---

## Feature 6 (optional) — Native menu bar

Add `w.SetMainMenu(...)` mirroring Everything's structure to house existing/adjacent actions:
- **File**: Close/Hide.
- **View**: theme toggle (light/dark), sort submenu (delegates to Feature 2 state).
- **Help**: About… → existing `showAbout()`.
Keep it minimal; only wire actions that already exist or are added above. Don't invent features behind menu items.

---

## Out of scope (roadmap, do NOT implement here)

- **Everything-style search syntax/filters** (`ext:`, `folder:`, `size:>1mb`, regex, wildcards). This needs a query parser and changes in `internal/ffdb` + SQL. Large; separate plan.
- **Details/preview pane.**
- **Alternating row (zebra) striping** — would require a custom `Color`/cell background in `theme.go`; nice-to-have, only if trivial.

---

## Suggested execution order & PR breakdown

1. **PR 1 — Feature 3** (context menu + fix click + wire `handleOpenFile`). Smallest, removes surprising behavior, no `RowData` changes. Land first.
2. **PR 2 — Feature 1** (icons). Self-contained cell refactor.
3. **PR 3 — Feature 2** (sortable columns). Requires adding `SizeBytes`/`ModTime` to `RowData`; touches the search build loop.
4. **PR 4+ — Features 4/5/6** as capacity allows.

Each PR: `make build` clean, `make test` passing, manual smoke test with a live DB (or `-db_path` to a test DB). Do a Linux cross-compile sanity check for anything touching `open_*.go` or platform-guarded code.

## Files likely touched

- `cmd/everythingx/ui.go` — all core work (cells, icons, sort state, context menu, key handling).
- `cmd/everythingx/theme.go` — only if zebra striping / density tweaks are attempted.
- `internal/shared/utils.go` — only if a raw-bytes/timestamp accessor is added for sorting (keep it single-purpose per CLAUDE.md).
- No changes to `internal/ffdb`, the daemon, or the CLI.
