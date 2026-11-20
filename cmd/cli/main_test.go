package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlanKK/everythingx/internal/shared"
)

func paths(results []*shared.SearchResult) []string {
	out := []string{}
	for _, r := range results {
		out = append(out, r.Fullpath)
	}
	return out
}

func TestFilterUnderDir(t *testing.T) {
	results := []*shared.SearchResult{
		{Fullpath: "/home/alan"},
		{Fullpath: "/home/alan/.bashrc"},
		{Fullpath: "/home/alan/src/a.go"},
		{Fullpath: "/home/al/notes.txt"},
		{Fullpath: "/home/alanx/notes.txt"},
		{Fullpath: "/etc/bashrc"},
	}
	want := []string{"/home/alan", "/home/alan/.bashrc", "/home/alan/src/a.go"}

	for _, dir := range []string{"/home/alan", "/home/alan/"} {
		in := append([]*shared.SearchResult(nil), results...)
		got := paths(filterUnderDir(in, dir))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("filterUnderDir(%q) = %v, want %v", dir, got, want)
		}
	}
}

func TestFilterExecutable(t *testing.T) {
	dir := t.TempDir()
	execFile := filepath.Join(dir, "run.sh")
	plainFile := filepath.Join(dir, "notes.txt")
	subDir := filepath.Join(dir, "bin")
	linkToExec := filepath.Join(dir, "run-link")
	linkToPlain := filepath.Join(dir, "notes-link")
	missing := filepath.Join(dir, "gone")

	if err := os.WriteFile(execFile, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plainFile, []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(execFile, linkToExec); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(plainFile, linkToPlain); err != nil {
		t.Fatal(err)
	}

	results := []*shared.SearchResult{
		{Fullpath: execFile, ObjectType: shared.ItemIsFile},
		{Fullpath: plainFile, ObjectType: shared.ItemIsFile},
		{Fullpath: subDir, ObjectType: shared.ItemIsDir},
		{Fullpath: linkToExec, ObjectType: shared.ItemIsSymlink},
		{Fullpath: linkToPlain, ObjectType: shared.ItemIsSymlink},
		{Fullpath: missing, ObjectType: shared.ItemIsFile},
	}
	want := []string{execFile, linkToExec}

	got := paths(filterExecutable(results))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterExecutable() = %v, want %v", got, want)
	}
}

func TestShellExpansionHint(t *testing.T) {
	if got := shellExpansionHint("a", nil); got != "" {
		t.Errorf("no extra args: hint %q, want none", got)
	}
	got := shellExpansionHint("uninstall-old.sh", []string{"uninstall.sh"})
	for _, want := range []string{"2 file names", `searched for "uninstall-old.sh"`, `ignored "uninstall.sh"`, "ev 'uninstall*.sh'", "README"} {
		if !strings.Contains(got, want) {
			t.Errorf("hint %q does not contain %q", got, want)
		}
	}
	got = shellExpansionHint("a.go", []string{"b.go", "c.go", "d.go", "e.go"})
	if !strings.Contains(got, "and 1 more") {
		t.Errorf("hint %q should shorten a long list", got)
	}
}

func TestGuessPattern(t *testing.T) {
	cases := []struct {
		names []string
		want  string
	}{
		{[]string{"uninstall-old.sh", "uninstall.sh"}, "uninstall*.sh"},
		{[]string{"a.go", "main.go"}, "*.go"},
		{[]string{"lib1", "lib22"}, "lib*"},
		{[]string{"aa", "aaa"}, "aa*"}, // prefix and suffix must not overlap
		{[]string{"x.c", "y.h"}, ""},
	}
	for _, c := range cases {
		if got := guessPattern(c.names); got != c.want {
			t.Errorf("guessPattern(%q) = %q, want %q", c.names, got, c.want)
		}
	}
}
