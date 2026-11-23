package main

import (
	"errors"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
)

// menuLabels lists a menu's item labels, with "---" for separators.
func menuLabels(m *fyne.Menu) []string {
	var got []string
	for _, item := range m.Items {
		if item.IsSeparator {
			got = append(got, "---")
			continue
		}
		got = append(got, item.Label)
	}
	return got
}

func TestOpenWithMenuOrderAndLabels(t *testing.T) {
	stubLaunchers(t)
	appsForPath = func(string) ([]appChoice, error) {
		return []appChoice{
			{Name: "Zed", ID: "/Applications/Zed.app"},
			{Name: "Xcode", ID: "/Applications/Xcode-16.app", Detail: "16.0"},
			{Name: "bbedit", ID: "/Applications/BBEdit.app"},
			{Name: "TextEdit", ID: "/System/Applications/TextEdit.app", Default: true},
			{Name: "Xcode", ID: "/Applications/Xcode.app", Detail: "15.4"},
		}, nil
	}
	item := openWithMenuItem("/tmp/a.txt")
	if item.Label != openWithLabel || item.ChildMenu == nil {
		t.Fatalf("item = %q with child %v, want %q with a submenu", item.Label, item.ChildMenu, openWithLabel)
	}
	want := []string{
		"TextEdit (default)", "---",
		"bbedit", "Xcode (15.4)", "Xcode (16.0)", "Zed", "---",
		otherAppLabel,
	}
	if got := menuLabels(item.ChildMenu); !reflect.DeepEqual(got, want) {
		t.Errorf("submenu = %q, want %q", got, want)
	}
}

func TestOpenWithMenuWithoutDefault(t *testing.T) {
	stubLaunchers(t)
	appsForPath = func(string) ([]appChoice, error) {
		return []appChoice{{Name: "B", ID: "b"}, {Name: "A", ID: "a"}}, nil
	}
	want := []string{"A", "B", "---", otherAppLabel}
	if got := menuLabels(openWithMenuItem("/tmp/a.txt").ChildMenu); !reflect.DeepEqual(got, want) {
		t.Errorf("submenu = %q, want %q", got, want)
	}
}

func TestOpenWithMenuOnlyOtherWhenNoApps(t *testing.T) {
	stubLaunchers(t)
	for name, apps := range map[string]func(string) ([]appChoice, error){
		"none":  func(string) ([]appChoice, error) { return nil, nil },
		"error": func(string) ([]appChoice, error) { return nil, errors.New("boom") },
	} {
		appsForPath = apps
		want := []string{otherAppLabel}
		if got := menuLabels(openWithMenuItem("/tmp/a.txt").ChildMenu); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: submenu = %q, want %q", name, got, want)
		}
	}
}

func TestOpenWithMenuActions(t *testing.T) {
	calls := stubLaunchers(t)
	appsForPath = func(string) ([]appChoice, error) {
		return []appChoice{
			{Name: "Viewer", ID: "viewer", Default: true},
			{Name: "Editor", ID: "editor"},
		}, nil
	}
	path := tempFile(t, "photo.jpg")
	items := openWithMenuItem(path).ChildMenu.Items

	items[0].Action()
	expectLaunch(t, calls, "open with viewer", path)
	items[2].Action()
	expectLaunch(t, calls, "open with editor", path)
	items[len(items)-1].Action()
	expectLaunch(t, calls, "choose app", path)
}
