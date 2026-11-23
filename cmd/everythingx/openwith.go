package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
)

// appChoice is one application offered in the Open With submenu.
type appChoice struct {
	Name    string        // display name
	ID      string        // macOS: the .app path; Linux: the desktop-file id
	Detail  string        // tells apart two apps with the same Name: a version or an id
	Icon    fyne.Resource // nil when there is none
	Default bool          // the app Open would use
}

// The platform work behind the Open With submenu. Tests swap them for stubs.
var (
	// appsForPath lists the applications registered for the item's type.
	appsForPath = platformAppsFor
	// openWithPath opens the item in the given application.
	openWithPath = platformOpenWith
	// chooseAppPath asks the user for an application and opens the item in it.
	chooseAppPath = platformChooseApp
)

const (
	openWithLabel = "Open With"
	otherAppLabel = "Other…"
)

// openWithMenuItem is the Open With entry of the context menu: the apps
// registered for the item, the default one first, then Other… to pick any app.
// The lookup is done now, as the menu opens, so the list reflects any change
// in the user's file associations.
func openWithMenuItem(path string) *fyne.MenuItem {
	apps, err := appsForPath(path)
	if err != nil {
		fmt.Println("Open With:", err)
		apps = nil
	}
	sortApps(apps)
	labels := appLabels(apps)

	var items []*fyne.MenuItem
	for i, app := range apps {
		if i > 0 && apps[i-1].Default {
			items = append(items, fyne.NewMenuItemSeparator())
		}
		item := fyne.NewMenuItem(labels[i], func() {
			launch(func(p string) error { return openWithPath(app, p) }, path)
		})
		item.Icon = app.Icon
		items = append(items, item)
	}
	if len(items) > 0 {
		items = append(items, fyne.NewMenuItemSeparator())
	}
	items = append(items, fyne.NewMenuItem(otherAppLabel, func() {
		launch(chooseAppPath, path)
	}))

	item := fyne.NewMenuItem(openWithLabel, nil)
	item.ChildMenu = fyne.NewMenu("", items...)
	return item
}

// sortApps puts the default app first and the rest in name order, as Finder
// and the Linux file managers do. At most one app stays marked default.
func sortApps(apps []appChoice) {
	slices.SortStableFunc(apps, func(a, b appChoice) int {
		if a.Default != b.Default {
			if a.Default {
				return -1
			}
			return 1
		}
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.Detail, b.Detail),
		)
	})
	for i := 1; i < len(apps); i++ {
		apps[i].Default = false
	}
}

// appLabels gives each app its menu label: the name, with the detail added
// when another app shares the name, and "(default)" on the default one.
func appLabels(apps []appChoice) []string {
	count := make(map[string]int, len(apps))
	for _, a := range apps {
		count[strings.ToLower(a.Name)]++
	}
	labels := make([]string, len(apps))
	for i, a := range apps {
		label := a.Name
		if count[strings.ToLower(a.Name)] > 1 && a.Detail != "" {
			label += " (" + a.Detail + ")"
		}
		if a.Default {
			label += " (default)"
		}
		labels[i] = label
	}
	return labels
}
