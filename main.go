// MSFSLayoutGenerator updates layout.json, and total_package_size in manifest.json, for
// Microsoft Flight Simulator packages.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const usage = `Usage:
  - Put MSFSLayoutGenerator.exe in a package folder (next to manifest.json) and double-click it.
  - Drag package folders, or their layout.json files, onto MSFSLayoutGenerator.exe.
  - From a terminal: MSFSLayoutGenerator.exe <package folder | layout.json> ...
`

var errNotPackage = errors.New("no manifest.json or layout.json here, so this is not an MSFS package folder")

func main() {
	exe, _ := os.Executable()
	// When started from Explorer, the console window closes as soon as the program exits,
	// so keep it open until the results have been read.
	pause := ownsConsole()
	code := run(os.Stdout, os.Args[1:], exe)
	if pause {
		fmt.Print("Press any key to exit...")
		waitForKey()
	}
	os.Exit(code)
}

// run updates each package named in args, or the package containing the executable exe when
// args is empty. It writes a report to w and returns the process exit code.
func run(w io.Writer, args []string, exe string) int {
	fmt.Fprint(w, "MSFS Layout Generator\n\n")
	if len(args) == 1 && isHelpFlag(args[0]) {
		fmt.Fprint(w, usage)
		return 0
	}
	if len(args) == 0 {
		if exe == "" {
			fmt.Fprint(w, "Cannot find the folder this program is in.\n\n", usage)
			return 1
		}
		args = []string{filepath.Dir(exe)}
	}
	var self fs.FileInfo
	if exe != "" {
		self, _ = os.Stat(exe)
	}

	code := 0
	showUsage := false
	done := map[string]bool{}
	for _, arg := range args {
		dir, err := packageDir(arg)
		if err != nil {
			code, showUsage = 1, true
			fmt.Fprintf(w, "[FAILED] %s\n    %v\n\n", arg, err)
			continue
		}
		if done[dir] {
			continue
		}
		done[dir] = true
		res, err := Generate(dir, self)
		if err != nil {
			code = 1
		}
		report(w, dir, res, err)
	}
	if showUsage {
		fmt.Fprint(w, usage)
	}
	return code
}

// packageDir returns the package folder that a command-line argument refers to: the folder
// itself, or the folder containing a layout.json or manifest.json file.
func packageDir(arg string) (string, error) {
	path, err := filepath.Abs(arg)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("not found")
	}
	if err != nil {
		return "", err
	}
	dir := path
	if !info.IsDir() {
		name := filepath.Base(path)
		if !strings.EqualFold(name, "layout.json") && !strings.EqualFold(name, "manifest.json") {
			return "", fmt.Errorf("%s is not a package folder, layout.json or manifest.json", name)
		}
		dir = filepath.Dir(path)
	}
	if !isFile(filepath.Join(dir, "manifest.json")) && !isFile(filepath.Join(dir, "layout.json")) {
		return "", errNotPackage
	}
	return dir, nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isHelpFlag(arg string) bool {
	switch arg {
	case "-h", "-help", "--help", "/?":
		return true
	}
	return false
}

func report(w io.Writer, dir string, res Result, err error) {
	status := "[OK]"
	if err != nil {
		status = "[FAILED]"
	}
	fmt.Fprintf(w, "%s %s\n", status, dir)
	if res.Files > 0 {
		fmt.Fprintf(w, "    layout.json: %d files, %s\n", res.Files, formatSize(res.ContentSize))
	}
	switch {
	case err != nil:
		fmt.Fprintf(w, "    %v\n", err)
	case res.Manifest == manifestUpdated:
		fmt.Fprintf(w, "    manifest.json: total_package_size set to %d bytes (%s)\n", res.TotalSize, formatSize(res.TotalSize))
	case res.Manifest == manifestNoTotalSize:
		fmt.Fprintln(w, "    manifest.json: no total_package_size field, left unchanged")
	default:
		fmt.Fprintln(w, "    manifest.json: not found")
	}
	fmt.Fprintln(w)
}

// formatSize formats n bytes in binary units, like Explorer does.
func formatSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	value, unit := float64(n)/1024, 0
	for value >= 1024 && unit < 4 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f %cB", value, "KMGTP"[unit])
}
