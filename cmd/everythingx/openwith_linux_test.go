//go:build linux

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// writeFiles creates each file under root, with its parent directories.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const testGlobs2 = `# comment
50:text/plain:*.txt
50:application/gzip:*.gz
50:application/x-compressed-tar:*.tar.gz
50:text/x-csrc:*.c
50:text/x-c++src:*.C:cs
50:text/x-makefile:makefile
50:text/x-go:*.go
40:text/x-readme:readme*
60:text/markdown:readme.md
50:text/x-ignored:__NOGLOBS__
`

// xdgFixture is a set of XDG directories under a temp dir, with the
// environment pointed at them.
type xdgFixture struct {
	root, dataHome, sysData, configHome string
}

func newXDGFixture(t *testing.T) *xdgFixture {
	t.Helper()
	root := t.TempDir()
	f := &xdgFixture{
		root:       root,
		dataHome:   filepath.Join(root, "home", "share"),
		sysData:    filepath.Join(root, "usr", "share"),
		configHome: filepath.Join(root, "home", "config"),
	}
	t.Setenv("XDG_DATA_HOME", f.dataHome)
	t.Setenv("XDG_DATA_DIRS", f.sysData)
	t.Setenv("XDG_CONFIG_HOME", f.configHome)
	t.Setenv("XDG_CONFIG_DIRS", filepath.Join(root, "etc", "xdg"))
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "en_US.UTF-8")
	writeFiles(t, f.sysData, map[string]string{
		"mime/globs2":     testGlobs2,
		"mime/subclasses": "application/x-shellscript text/plain\ntext/x-go text/x-csrc\n",
		"mime/aliases":    "text/x-golang text/x-go\n",
	})
	return f
}

func testDesktopFile(name, exec, extra string) string {
	return "[Desktop Entry]\nType=Application\nName=" + name + "\nExec=" + exec + "\n" + extra
}

func TestGlobType(t *testing.T) {
	newXDGFixture(t)
	db := currentMimeDB()
	tests := map[string]string{
		"notes.txt":   "text/plain",
		"NOTES.TXT":   "text/plain",
		"a.tar.gz":    "application/x-compressed-tar",
		"a.gz":        "application/gzip",
		"main.c":      "text/x-csrc",
		"main.C":      "text/x-c++src",
		"Makefile":    "text/x-makefile",
		"README":      "text/x-readme",
		"README.md":   "text/markdown",
		"noextension": "",
	}
	for name, want := range tests {
		if got := db.globType(name); got != want {
			t.Errorf("globType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTypeChain(t *testing.T) {
	newXDGFixture(t)
	db := currentMimeDB()
	tests := map[string][]string{
		"text/x-golang":             {"text/x-go", "text/x-csrc", "text/plain", "application/octet-stream"},
		"application/x-shellscript": {"application/x-shellscript", "text/plain", "application/octet-stream"},
		"image/png":                 {"image/png", "application/octet-stream"},
		"application/octet-stream":  {"application/octet-stream"},
		"inode/directory":           {"inode/directory"},
	}
	for in, want := range tests {
		if got := db.typeChain(in); !reflect.DeepEqual(got, want) {
			t.Errorf("typeChain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileMimeType(t *testing.T) {
	newXDGFixture(t)
	db := currentMimeDB()
	orig := sniffMimeType
	t.Cleanup(func() { sniffMimeType = orig })
	dir := t.TempDir()

	sniffMimeType = func(string) string { t.Error("sniffed a file with a glob match"); return "" }
	if got := fileMimeType(db, tempFile(t, "a.txt")); got != "text/plain" {
		t.Errorf("a.txt: %q, want text/plain", got)
	}
	if got := fileMimeType(db, dir); got != mimeDirectory {
		t.Errorf("directory: %q, want %q", got, mimeDirectory)
	}

	sniffMimeType = func(string) string { return "text/x-golang" }
	if got := fileMimeType(db, tempFile(t, "script")); got != "text/x-go" {
		t.Errorf("sniffed alias: %q, want text/x-go", got)
	}
	sniffMimeType = func(string) string { return "" }
	if got := fileMimeType(db, tempFile(t, "blob")); got != mimeOctetStream {
		t.Errorf("unknown: %q, want %q", got, mimeOctetStream)
	}
}

func TestXDGAppsFor(t *testing.T) {
	f := newXDGFixture(t)
	t.Setenv("LANG", "de_DE.UTF-8")
	writeFiles(t, f.sysData, map[string]string{
		"applications/mimeinfo.cache": "[MIME Cache]\n" +
			"text/plain=gedit.desktop;vim.desktop;hidden.desktop;kate.desktop;mousepad.desktop;gnome-editor.desktop;unwanted.desktop;kde-okular.desktop;\n" +
			"text/x-go=code.desktop;\n",
		"applications/gedit.desktop":        testDesktopFile("Gedit", "gedit %U", ""),
		"applications/vim.desktop":          testDesktopFile("Vim", "vim %F", "Terminal=true\n"),
		"applications/hidden.desktop":       testDesktopFile("Hidden", "hidden %f", "Hidden=true\n"),
		"applications/kate.desktop":         testDesktopFile("Kate", "kate %U", "OnlyShowIn=KDE;\n"),
		"applications/mousepad.desktop":     testDesktopFile("Mousepad", "mousepad %f", "TryExec=/nonexistent/mousepad\n"),
		"applications/gnome-editor.desktop": testDesktopFile("Text Editor", "gnome-text-editor %U", "Name[de]=Texteditor\nOnlyShowIn=GNOME;\n"),
		"applications/unwanted.desktop":     testDesktopFile("Unwanted", "unwanted %f", ""),
		"applications/kde/okular.desktop":   testDesktopFile("Okular", "okular %U", ""),
		"applications/code.desktop":         testDesktopFile("Code", "code %F", ""),
		"applications/added.desktop":        testDesktopFile("Added", "added %f", ""),
	})
	writeFiles(t, f.dataHome, map[string]string{
		// No mimeinfo.cache here, so MimeType= lines are read. This gedit
		// overrides the system one.
		"applications/gedit.desktop":  testDesktopFile("Gedit (user)", "gedit %U", "MimeType=text/plain;\n"),
		"applications/golang.desktop": testDesktopFile("GoLand", "goland %f", "MimeType=text/x-golang;\n"),
	})
	writeFiles(t, f.configHome, map[string]string{
		"mimeapps.list": "[Default Applications]\ntext/plain=missing.desktop;gedit.desktop;\n" +
			"[Added Associations]\ntext/x-go=added.desktop;\n" +
			"[Removed Associations]\ntext/plain=unwanted.desktop;\n",
		"gnome-mimeapps.list": "[Default Applications]\ntext/x-go=code.desktop\n",
	})
	db := currentMimeDB()

	ids := func(apps []*desktopEntry) []string {
		var out []string
		for _, a := range apps {
			out = append(out, a.ID)
		}
		return out
	}

	t.Run("go file on GNOME", func(t *testing.T) {
		apps, def := xdgAppsFor(db, db.typeChain("text/x-go"))
		want := []string{"code.desktop", "added.desktop", "golang.desktop", "gedit.desktop", "gnome-editor.desktop", "kde-okular.desktop"}
		if got := ids(apps); !reflect.DeepEqual(got, want) {
			t.Errorf("apps = %q, want %q", got, want)
		}
		if def == nil || def.ID != "code.desktop" {
			t.Errorf("default = %v, want code.desktop from gnome-mimeapps.list", def)
		}
		names := map[string]string{}
		for _, a := range apps {
			names[a.ID] = a.Name
		}
		if names["gedit.desktop"] != "Gedit (user)" {
			t.Errorf("gedit name = %q, want the user override", names["gedit.desktop"])
		}
		if names["gnome-editor.desktop"] != "Texteditor" {
			t.Errorf("localised name = %q, want Texteditor", names["gnome-editor.desktop"])
		}
	})

	t.Run("text file on KDE", func(t *testing.T) {
		t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
		apps, def := xdgAppsFor(db, db.typeChain("text/plain"))
		want := []string{"gedit.desktop", "kate.desktop", "kde-okular.desktop"}
		if got := ids(apps); !reflect.DeepEqual(got, want) {
			t.Errorf("apps = %q, want %q", got, want)
		}
		if def == nil || def.ID != "gedit.desktop" {
			t.Errorf("default = %v, want gedit.desktop, the first installed default", def)
		}
	})

	t.Run("no default set", func(t *testing.T) {
		apps, def := xdgAppsFor(db, db.typeChain("image/png"))
		if len(apps) != 0 || def != nil {
			t.Errorf("apps = %q, default %v; want none", ids(apps), def)
		}
	})
}

func TestParseDesktopEntry(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("LC_ALL", "sr_RS.UTF-8@latin")
	e := parseDesktopEntry("# c\n[Desktop Entry]\nType=Application\nName=Viewer\nName[sr]=Pregled\n"+
		"Name[sr@latin]=Pregled (lat)\nExec=view\\sit %f\nPath=/opt/v\nMimeType=image/png;image/jpeg;\n"+
		"[Desktop Action new]\nName=Ignored\n", "/x/view.desktop", "view.desktop")
	if !e.usable() || e.Name != "Pregled (lat)" || e.Exec != "view it %f" || e.WorkDir != "/opt/v" ||
		!reflect.DeepEqual(e.MimeTypes, []string{"image/png", "image/jpeg"}) {
		t.Errorf("entry = %+v", e)
	}
	if e := parseDesktopEntry("[Desktop Entry]\nType=Link\nName=L\nExec=x\n", "/x/l.desktop", "l.desktop"); e.usable() {
		t.Error("a Link entry is usable")
	}
	if e := parseDesktopEntry("[Desktop Entry]\nType=Application\nName=N\n", "/x/n.desktop", "n.desktop"); e.usable() {
		t.Error("an entry without Exec is usable")
	}
}

func TestExecArgs(t *testing.T) {
	const path = "/tmp/my file.txt"
	const uri = "file:///tmp/my%20file.txt"
	tests := []struct {
		exec string
		want []string
	}{
		{"gedit %U", []string{"gedit", uri}},
		{"vlc --started-from-file %F", []string{"vlc", "--started-from-file", path}},
		{"plain", []string{"plain", path}},
		{"app %i %f", []string{"app", "--icon", "app-icon", path}},
		{`"/opt/my app/bin" %f`, []string{"/opt/my app/bin", path}},
		{`sh -c "echo \"%f\" \$HOME \\ \` + "`" + `x" sh %f`, []string{"sh", "-c", `echo "%f" $HOME \ ` + "`x", "sh", path}},
		{"app --file=%f", []string{"app", "--file=" + path}},
		{"app %d %D %n %N %v %m %f 100%%", []string{"app", path, "100%"}},
		{"app --title %c --entry %k %u", []string{"app", "--title", "My App", "--entry", "/apps/app.desktop", uri}},
		{"  app\t%f  ", []string{"app", path}},
	}
	for _, tc := range tests {
		e := &desktopEntry{ID: "app.desktop", Path: "/apps/app.desktop", Name: "My App", Icon: "app-icon", Exec: tc.exec}
		got, err := execArgs(e, path)
		if err != nil {
			t.Errorf("execArgs(%q): %v", tc.exec, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("execArgs(%q) = %q, want %q", tc.exec, got, tc.want)
		}
	}

	for _, bad := range []string{`app "unterminated %f`, "%d", "   "} {
		if got, err := execArgs(&desktopEntry{ID: "bad.desktop", Exec: bad}, path); err == nil {
			t.Errorf("execArgs(%q) = %q, want an error", bad, got)
		}
	}
}

// End to end: the default app is found from the real databases and its Exec
// line runs on the file.
func TestPlatformOpenWithRunsDefaultApp(t *testing.T) {
	f := newXDGFixture(t)
	writeFiles(t, f.dataHome, map[string]string{
		"applications/touch.desktop": testDesktopFile("Toucher", `sh -c "touch \"\$1.opened\"" sh %f`, "MimeType=text/plain;\n"),
	})
	writeFiles(t, f.configHome, map[string]string{
		"mimeapps.list": "[Default Applications]\ntext/plain=touch.desktop\n",
	})
	path := tempFile(t, "a.txt")

	apps, err := platformAppsFor(path)
	if err != nil || len(apps) != 1 || apps[0].ID != "touch.desktop" || !apps[0].Default {
		t.Fatalf("platformAppsFor = %+v, %v; want touch.desktop as the default", apps, err)
	}
	if err := platformOpenWith(apps[0], path); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path + ".opened"); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the app did not run on the file")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
