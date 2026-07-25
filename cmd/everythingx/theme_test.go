package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The theme must resolve each icon name to its own artwork. Returning a single
// resource for every name would make every row in the results table look the
// same.
func TestThemeIconsAreDistinct(t *testing.T) {
	var tt everythingxTheme

	names := []fyne.ThemeIconName{
		theme.IconNameFolder,
		theme.IconNameFile,
		theme.IconNameFileImage,
		theme.IconNameFileText,
		theme.IconNameFileAudio,
		theme.IconNameFileVideo,
		theme.IconNameFileApplication,
	}

	seen := make(map[string]fyne.ThemeIconName, len(names))
	for _, name := range names {
		icon := tt.Icon(name)
		if icon == nil {
			t.Fatalf("theme returned no icon for %q", name)
		}
		if other, dup := seen[icon.Name()]; dup {
			t.Errorf("icons %q and %q both resolve to %s", name, other, icon.Name())
			continue
		}
		seen[icon.Name()] = name
	}
}
