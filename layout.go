package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Content is one file entry in layout.json.
type Content struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Date int64  `json:"date"`
}

type layout struct {
	Content []Content `json:"content"`
}

// Result describes what Generate changed in a package.
type Result struct {
	Files       int   // entries written to layout.json
	ContentSize int64 // combined size of those files
	Manifest    manifestStatus
	TotalSize   int64 // new total_package_size, set when Manifest is manifestUpdated
}

type manifestStatus int

const (
	manifestMissing manifestStatus = iota
	manifestNoTotalSize
	manifestUpdated
)

// fileTimeUnixOffset is the number of 100-nanosecond intervals between the Windows file time
// epoch (1601-01-01 UTC) and the Unix epoch.
const fileTimeUnixOffset = 116444736000000000

var errNoFiles = errors.New("no files found, so layout.json was not updated")

// Generate rewrites layout.json in the package folder dir so that it lists every file in the
// package, then updates total_package_size in manifest.json if the field exists. self is the
// running executable, or nil; it is left out of the layout when it is inside the package.
func Generate(dir string, self fs.FileInfo) (Result, error) {
	entries, err := collect(dir, self)
	if err != nil {
		return Result{}, err
	}
	if len(entries) == 0 {
		return Result{}, errNoFiles
	}
	data, err := encodeLayout(entries)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "layout.json"), data, 0o644); err != nil {
		return Result{}, err
	}
	res := Result{Files: len(entries)}
	for _, e := range entries {
		res.ContentSize += e.Size
	}

	manifestPath := filepath.Join(dir, "manifest.json")
	manifest, err := os.ReadFile(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("layout.json was updated, but manifest.json could not be read: %w", err)
	}
	// total_package_size is always written with 20 digits, so the size of the updated
	// manifest.json is known before the total is.
	updated, found, err := setTotalPackageSize(manifest, 0)
	if err != nil {
		return res, fmt.Errorf("layout.json was updated, but manifest.json is not valid JSON: %w", err)
	}
	if !found {
		res.Manifest = manifestNoTotalSize
		return res, nil
	}
	total := res.ContentSize + int64(len(data)) + int64(len(updated))
	updated, _, _ = setTotalPackageSize(manifest, total)
	if err := os.WriteFile(manifestPath, updated, 0o644); err != nil {
		return res, fmt.Errorf("layout.json was updated, but manifest.json could not be written: %w", err)
	}
	res.Manifest = manifestUpdated
	res.TotalSize = total
	return res, nil
}

// collect lists the files in the package folder root in the order the original tool produced
// on NTFS: each folder's files first, then its subfolders, both sorted case-insensitively.
// Symbolic links and junctions are followed, except those leading back into a folder that is
// already being listed.
func collect(root string, self fs.FileInfo) ([]Content, error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	var entries []Content
	var walk func(dir, rel string, ancestors []fs.FileInfo) error
	walk = func(dir, rel string, ancestors []fs.FileInfo) error {
		items, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		slices.SortFunc(items, func(a, b fs.DirEntry) int { return compareNames(a.Name(), b.Name()) })
		type subdir struct {
			name string
			info fs.FileInfo
			link bool
		}
		var subdirs []subdir
		for _, item := range items {
			relPath := path.Join(rel, item.Name())
			if excluded(relPath) {
				continue
			}
			info, err := os.Stat(filepath.Join(dir, item.Name()))
			if err != nil {
				return err
			}
			switch {
			case info.IsDir():
				link := item.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0
				subdirs = append(subdirs, subdir{item.Name(), info, link})
			case info.Mode().IsRegular() && !isSelf(info, self):
				entries = append(entries, Content{Path: relPath, Size: info.Size(), Date: fileTime(info.ModTime())})
			}
		}
		for _, sub := range subdirs {
			if sub.link && slices.ContainsFunc(ancestors, func(a fs.FileInfo) bool { return os.SameFile(a, sub.info) }) {
				continue
			}
			if err := walk(filepath.Join(dir, sub.name), path.Join(rel, sub.name), append(slices.Clip(ancestors), sub.info)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, "", []fs.FileInfo{rootInfo}); err != nil {
		return nil, err
	}
	return entries, nil
}

// excluded reports whether the entry at the slash-separated path rel is left out of layout.json.
// Like the original tool, this skips the package's own layout.json and manifest.json, the
// generator, and top-level entries starting with _CVT_.
func excluded(rel string) bool {
	for _, name := range []string{"layout.json", "manifest.json", "MSFSLayoutGenerator.exe"} {
		if strings.EqualFold(rel, name) {
			return true
		}
	}
	return len(rel) >= len("_CVT_") && strings.EqualFold(rel[:len("_CVT_")], "_CVT_")
}

// isSelf reports whether info describes the running executable self.
func isSelf(info, self fs.FileInfo) bool {
	return self != nil && info.Size() == self.Size() && os.SameFile(info, self)
}

// compareNames orders file names the way NTFS lists them: by code point after upper-casing.
func compareNames(a, b string) int {
	if c := strings.Compare(strings.ToUpper(a), strings.ToUpper(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// fileTime converts t to a Windows file time: 100-nanosecond intervals since 1601-01-01 UTC.
func fileTime(t time.Time) int64 {
	return t.UnixNano()/100 + fileTimeUnixOffset
}

// encodeLayout serializes the layout exactly like the original tool: two-space indentation,
// LF line endings, no HTML escaping and no trailing newline.
func encodeLayout(entries []Content) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(layout{Content: entries}); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

var utf8BOM = []byte("\xef\xbb\xbf")

// setTotalPackageSize returns manifest with the value of its top-level total_package_size field
// replaced by total, written as a 20-digit zero-padded string the way MSFS writes it. All other
// bytes are kept as they are. found is false when the field does not exist.
func setTotalPackageSize(manifest []byte, total int64) (out []byte, found bool, err error) {
	body := bytes.TrimPrefix(manifest, utf8BOM)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, false, err
	}
	if _, ok := fields["total_package_size"]; !ok {
		return manifest, false, nil
	}
	// Walk the top-level fields again to find where the value starts and ends.
	dec := json.NewDecoder(bytes.NewReader(body))
	if _, err := dec.Token(); err != nil {
		return nil, false, err
	}
	for {
		key, err := dec.Token()
		if err != nil {
			return nil, false, err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false, err
		}
		if key == "total_package_size" {
			end := len(manifest) - len(body) + int(dec.InputOffset())
			start := end - len(value)
			out = append(out, manifest[:start]...)
			out = fmt.Appendf(out, `"%020d"`, total)
			return append(out, manifest[end:]...), true, nil
		}
	}
}
