package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// y2k is the modification time given to every test file. As a Windows file time it is
// 125911584001234567.
var y2k = time.Date(2000, 1, 1, 0, 0, 0, 123456700, time.UTC)

// writeFiles creates files (slash-separated path -> content) under dir, modified at y2k.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, y2k, y2k); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// layoutPaths returns the paths listed in dir/layout.json, in order.
func layoutPaths(t *testing.T, dir string) []string {
	t.Helper()
	var l layout
	if err := json.Unmarshal([]byte(readFile(t, dir, "layout.json")), &l); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, c := range l.Content {
		paths = append(paths, c.Path)
	}
	return paths
}

func generate(t *testing.T, dir string) Result {
	t.Helper()
	res, err := Generate(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestGenerateWritesLayoutInOriginalFormat(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"manifest.json":                 "{}",
		"SimObjects/A&B 中文 (1)/tex.dds": "abc",
	})
	generate(t, dir)

	want := `{
  "content": [
    {
      "path": "SimObjects/A&B 中文 (1)/tex.dds",
      "size": 3,
      "date": 125911584001234567
    }
  ]
}`
	if got := readFile(t, dir, "layout.json"); got != want {
		t.Errorf("layout.json =\n%s\nwant\n%s", got, want)
	}
}

func TestGenerateKeepsFileNamesAsTheyAreOnDisk(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"manifest.json":          "{}",
		"textures/a#1.dds":       "x",
		"textures/b%41c%20d.dds": "x",
	})
	generate(t, dir)

	want := []string{"textures/a#1.dds", "textures/b%41c%20d.dds"}
	if got := layoutPaths(t, dir); !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
}

func TestGenerateListsFilesBeforeSubfoldersCaseInsensitively(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"manifest.json": "{}",
		"b.txt":         "x",
		"A.txt":         "x",
		"a b.txt":       "x",
		"_x.txt":        "x",
		"c/f.txt":       "x",
		"B/f.txt":       "x",
		"B/a/f.txt":     "x",
	})
	generate(t, dir)

	want := []string{"a b.txt", "A.txt", "b.txt", "_x.txt", "B/f.txt", "B/a/f.txt", "c/f.txt"}
	if got := layoutPaths(t, dir); !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
}

func TestGenerateSkipsPackageFilesAndTheGenerator(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"manifest.json":             "{}",
		"layout.json":               "{}",
		"MSFSLayoutGenerator.exe":   "old copy",
		"_CVT_/cache.bin":           "x",
		"_cvt_notes.txt":            "x",
		"renamed.exe":               "self",
		"other.exe":                 "SELF",
		"SimObjects/layout.json":    "{}",
		"SimObjects/_CVT_/keep.bin": "x",
	})
	self, err := os.Stat(filepath.Join(dir, "renamed.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(dir, self); err != nil {
		t.Fatal(err)
	}

	want := []string{"other.exe", "SimObjects/layout.json", "SimObjects/_CVT_/keep.bin"}
	if got := layoutPaths(t, dir); !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
}

func TestExcludedIgnoresCase(t *testing.T) {
	for _, rel := range []string{"Layout.JSON", "MANIFEST.json", "msfslayoutgenerator.EXE", "_Cvt_/a.dds"} {
		if !excluded(rel) {
			t.Errorf("excluded(%q) = false, want true", rel)
		}
	}
	for _, rel := range []string{"a/layout.json", "a/_CVT_/b.dds", "CVT_x", "_CV"} {
		if excluded(rel) {
			t.Errorf("excluded(%q) = true, want false", rel)
		}
	}
}

func TestGenerateSetsTotalPackageSizeAndKeepsManifestFormatting(t *testing.T) {
	dir := t.TempDir()
	manifest := "{\r\n    \"title\": \"Test\",\r\n    \"total_package_size\": \"00000000000000000001\",\r\n" +
		"    \"nested\": { \"total_package_size\": 5 }\r\n}\r\n"
	writeFiles(t, dir, map[string]string{
		"manifest.json":  manifest,
		"a.bin":          strings.Repeat("a", 1000),
		"sub/b.bin":      "bb",
		"_CVT_/skip.bin": strings.Repeat("c", 50),
	})
	res := generate(t, dir)

	// The total covers every file in the package except the _CVT_ folder.
	var want int64
	for _, rel := range []string{"manifest.json", "layout.json", "a.bin", "sub/b.bin"} {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		want += info.Size()
	}
	if res.Manifest != manifestUpdated || res.TotalSize != want {
		t.Errorf("Manifest = %v, TotalSize = %d, want %v, %d", res.Manifest, res.TotalSize, manifestUpdated, want)
	}
	wantManifest := strings.Replace(manifest, `"00000000000000000001"`, fmt.Sprintf(`"%020d"`, want), 1)
	if got := readFile(t, dir, "manifest.json"); got != wantManifest {
		t.Errorf("manifest.json =\n%q\nwant\n%q", got, wantManifest)
	}
}

func TestGenerateLeavesManifestWithoutTotalPackageSizeAlone(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"manifest.json": `{"title":"Test"}`, "a.txt": "x"})
	res := generate(t, dir)

	if res.Manifest != manifestNoTotalSize {
		t.Errorf("Manifest = %v, want %v", res.Manifest, manifestNoTotalSize)
	}
	if got := readFile(t, dir, "manifest.json"); got != `{"title":"Test"}` {
		t.Errorf("manifest.json = %q, want it unchanged", got)
	}
}

func TestGenerateWritesLayoutEvenIfManifestIsInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"manifest.json": "{ not json", "a.txt": "x"})
	res, err := Generate(dir, nil)

	if err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("err = %v, want a manifest.json error", err)
	}
	if res.Files != 1 {
		t.Errorf("Files = %d, want 1", res.Files)
	}
	if got := layoutPaths(t, dir); !slices.Equal(got, []string{"a.txt"}) {
		t.Errorf("paths = %q, want [a.txt]", got)
	}
}

func TestGenerateRefusesPackageWithoutContent(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"manifest.json": "{}", "layout.json": "keep"})
	if _, err := Generate(dir, nil); !errors.Is(err, errNoFiles) {
		t.Fatalf("err = %v, want errNoFiles", err)
	}
	if got := readFile(t, dir, "layout.json"); got != "keep" {
		t.Errorf("layout.json = %q, want it unchanged", got)
	}
}

func TestGenerateHandlesPathsLongerThan260Characters(t *testing.T) {
	dir := t.TempDir()
	rel := strings.Repeat(strings.Repeat("d", 50)+"/", 6) + "texture.dds"
	if full := filepath.Join(dir, filepath.FromSlash(rel)); len(full) <= 260 {
		t.Fatalf("test path is only %d characters long", len(full))
	}
	writeFiles(t, dir, map[string]string{"manifest.json": "{}", rel: "x"})
	generate(t, dir)

	if got := layoutPaths(t, dir); !slices.Equal(got, []string{rel}) {
		t.Errorf("paths = %q, want [%s]", got, rel)
	}
}

func TestGenerateFollowsLinkedPackageFolder(t *testing.T) {
	target := t.TempDir()
	writeFiles(t, target, map[string]string{"manifest.json": "{}", "a/b.txt": "x"})
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
	generate(t, link)

	if got := layoutPaths(t, target); !slices.Equal(got, []string{"a/b.txt"}) {
		t.Errorf("paths = %q, want [a/b.txt]", got)
	}
}

func TestGenerateStopsAtLinksBackIntoThePackage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"manifest.json": "{}", "a/b.txt": "x"})
	if err := os.Symlink(dir, filepath.Join(dir, "a", "loop")); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
	generate(t, dir)

	if got := layoutPaths(t, dir); !slices.Equal(got, []string{"a/b.txt"}) {
		t.Errorf("paths = %q, want [a/b.txt]", got)
	}
}

func TestSetTotalPackageSize(t *testing.T) {
	tests := []struct {
		name, in, want string
		found          bool
	}{
		{"string value", `{"total_package_size": "1"}`, `{"total_package_size": "00000000000000000042"}`, true},
		{"number value", `{"total_package_size":7, "a": 1}`, `{"total_package_size":"00000000000000000042", "a": 1}`, true},
		{"after other fields", `{"a": [1, {"b": 2}], "total_package_size" : "1" }`, `{"a": [1, {"b": 2}], "total_package_size" : "00000000000000000042" }`, true},
		{"byte order mark", "\xef\xbb\xbf{\"total_package_size\": \"1\"}", "\xef\xbb\xbf{\"total_package_size\": \"00000000000000000042\"}", true},
		{"nested only", `{"a": {"total_package_size": "1"}}`, `{"a": {"total_package_size": "1"}}`, false},
		{"missing", `{"a": 1}`, `{"a": 1}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, found, err := setTotalPackageSize([]byte(tt.in), 42)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.want || found != tt.found {
				t.Errorf("got %q, %v; want %q, %v", out, found, tt.want, tt.found)
			}
		})
	}
	for _, in := range []string{"", "{", "[1]", `{"a": 1} x`} {
		if _, _, err := setTotalPackageSize([]byte(in), 42); err == nil {
			t.Errorf("setTotalPackageSize(%q) succeeded, want an error", in)
		}
	}
}
