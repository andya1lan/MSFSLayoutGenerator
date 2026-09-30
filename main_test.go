package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPackageDir(t *testing.T) {
	pkg := t.TempDir()
	writeFiles(t, pkg, map[string]string{"manifest.json": "{}", "layout.json": "{}", "readme.txt": "x"})
	layoutOnly := t.TempDir()
	writeFiles(t, layoutOnly, map[string]string{"layout.json": "{}"})
	notPackage := t.TempDir()
	writeFiles(t, notPackage, map[string]string{"sub/manifest.json": "{}"})

	for _, tt := range []struct{ arg, want string }{
		{pkg, pkg},
		{filepath.Join(pkg, "layout.json"), pkg},
		{filepath.Join(pkg, "manifest.json"), pkg},
		{layoutOnly, layoutOnly},
	} {
		if got, err := packageDir(tt.arg); got != tt.want || err != nil {
			t.Errorf("packageDir(%q) = %q, %v; want %q", tt.arg, got, err, tt.want)
		}
	}
	for _, arg := range []string{
		notPackage,
		filepath.Join(notPackage, "sub", "missing"),
		filepath.Join(pkg, "readme.txt"),
	} {
		if got, err := packageDir(arg); err == nil {
			t.Errorf("packageDir(%q) = %q, want an error", arg, got)
		}
	}
}

func TestRunUsesExecutableFolderWithoutArguments(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"manifest.json": "{}", "tex/a.dds": "x", "layoutgen.exe": "binary"})
	var out bytes.Buffer
	if code := run(&out, nil, filepath.Join(dir, "layoutgen.exe")); code != 0 {
		t.Fatalf("exit code %d, output:\n%s", code, &out)
	}

	if got := layoutPaths(t, dir); !slices.Equal(got, []string{"tex/a.dds"}) {
		t.Errorf("paths = %q, want [tex/a.dds]", got)
	}
}

func TestRunRejectsExecutableFolderThatIsNotAPackage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"notes.txt": "x", "layoutgen.exe": "binary"})
	var out bytes.Buffer
	code := run(&out, nil, filepath.Join(dir, "layoutgen.exe"))

	if code != 1 || !strings.Contains(out.String(), errNotPackage.Error()) || !strings.Contains(out.String(), "Usage:") {
		t.Errorf("exit code %d, output:\n%s", code, &out)
	}
	if _, err := os.Stat(filepath.Join(dir, "layout.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("layout.json was created: %v", err)
	}
}

func TestRunUpdatesEachPackageOnceAndContinuesAfterFailures(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeFiles(t, a, map[string]string{"manifest.json": "{}", "a.txt": "x"})
	writeFiles(t, b, map[string]string{"layout.json": "{}", "b.txt": "x"})
	var out bytes.Buffer
	args := []string{filepath.Join(t.TempDir(), "missing"), a, filepath.Join(a, "layout.json"), b}
	code := run(&out, args, "")

	if code != 1 || strings.Count(out.String(), "[OK]") != 2 || strings.Count(out.String(), "[FAILED]") != 1 {
		t.Errorf("exit code %d, output:\n%s", code, &out)
	}
	if got := layoutPaths(t, b); !slices.Equal(got, []string{"b.txt"}) {
		t.Errorf("paths = %q, want [b.txt]", got)
	}
}

func TestRunPrintsUsageForHelpFlag(t *testing.T) {
	var out bytes.Buffer
	if code := run(&out, []string{"--help"}, ""); code != 0 || !strings.Contains(out.String(), "Usage:") {
		t.Errorf("exit code %d, output:\n%s", code, &out)
	}
}

func TestFormatSize(t *testing.T) {
	for n, want := range map[int64]string{
		0:                 "0 bytes",
		1023:              "1023 bytes",
		1024:              "1.0 KB",
		1536:              "1.5 KB",
		5 << 20:           "5.0 MB",
		3 << 30:           "3.0 GB",
		int64(1.5 * 1e12): "1.4 TB",
	} {
		if got := formatSize(n); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", n, got, want)
		}
	}
}
