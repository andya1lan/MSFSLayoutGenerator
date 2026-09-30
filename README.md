# MSFS Layout Generator

[![CI](https://github.com/andya1lan/MSFSLayoutGenerator/actions/workflows/ci.yml/badge.svg)](https://github.com/andya1lan/MSFSLayoutGenerator/actions/workflows/ci.yml)

Creates or updates the `layout.json` of a Microsoft Flight Simulator package, so that files you add or change by hand (liveries, textures, configs) are picked up by the sim. If the package's `manifest.json` has a `total_package_size` field, that is updated as well.

This is a Go rewrite of [MSFS Layout Generator](https://github.com/HughesMDflyer4/MSFSLayoutGenerator) by Brandon Filer (HughesMDflyer4). It writes the same `layout.json` as the original and adds:

- **Double-click to update:** put the exe in a package folder and double-click it.
- **Drag and drop folders:** drop one or more package folders, or their `layout.json`, onto the exe.
- **Long paths:** paths longer than 260 characters work without changing any Windows setting.
- **Visible results:** the window stays open and shows what was updated.
- **No runtime to install:** a single 3 MB exe, no .NET required.

## Download

1. Open the [latest successful build](https://github.com/andya1lan/MSFSLayoutGenerator/actions/workflows/ci.yml?query=branch%3Amaster+is%3Asuccess) and select the top run. You need to be signed in to GitHub.
2. Under **Artifacts**, download `MSFSLayoutGenerator` and unzip it.

Requires Windows 10 or 11 (x64). Because the exe is not code-signed, Windows may show a SmartScreen warning the first time; choose **More info**, then **Run anyway**.

## Usage

Any of these updates a package:

- Put `MSFSLayoutGenerator.exe` in the package folder, next to `manifest.json`, and double-click it.
- Drag the package folder, or its `layout.json`, onto `MSFSLayoutGenerator.exe`. You can drop several packages at once.
- From a terminal, pass package folders or `layout.json` files as absolute or relative paths:
  ```
  MSFSLayoutGenerator.exe "G:\MSFS\Community\my-livery" "G:\MSFS\Community\other-package\layout.json"
  ```
  With no arguments, it updates the folder the exe is in. The exit code is 0 when every package was updated and 1 otherwise.

The window then shows the result:

```
MSFS Layout Generator

[OK] G:\MSFS\Community\my-livery
    layout.json: 12 files, 45.2 MB
    manifest.json: total_package_size set to 47402250 bytes (45.2 MB)

Press any key to exit...
```

Start the sim to see your changes.

## What gets written

- Every file in the package folder and its subfolders is listed with its path relative to the package (separated by `/`), its size in bytes and its last modified time as a Windows [file time](https://learn.microsoft.com/windows/win32/sysinfo/file-times). Symbolic links and junctions are followed.
- These are not listed: `layout.json` and `manifest.json` in the package root, the exe itself (even if renamed), `MSFSLayoutGenerator.exe`, and anything in the package root whose name starts with `_CVT_`.
- The existing `layout.json` is replaced. In `manifest.json`, only the value of `total_package_size` changes: it becomes the combined size of `layout.json`, `manifest.json` and every listed file, written as 20 digits the way MSFS writes it. The rest of `manifest.json` stays exactly as it was.
- A folder is only updated if it contains a `manifest.json` or a `layout.json`, so running the exe in an unrelated folder such as the Desktop changes nothing. A package with no other files keeps its `layout.json` unchanged.
- Files are listed in the order the original tool produced on NTFS: each folder's files first, then its subfolders, sorted case-insensitively. The order does not affect how MSFS loads the package.

## FAQ

**What is layout.json?**
The virtual file system in Microsoft Flight Simulator reads the `layout.json` in the root of each package to determine which files it is allowed to load, and to perform a basic file integrity check.

**Who is this for?**
Anyone who builds packages by hand rather than with the Project Editor in Developer Mode. Livery artists in particular: adding every texture to `layout.json` manually is tedious.

**Does it work with MSFS 2024?**
Possible differences between MSFS 2020 and MSFS 2024 `layout.json` files have not been evaluated, but the updated files are expected to work in MSFS 2024.

**How does the output compare to the original tool?**
`layout.json` has the same entries, sizes, dates and formatting. `manifest.json` differs in one way: the original rewrote the whole file, while this version changes only the `total_package_size` value.

## Building from source

Requires [Go](https://go.dev/dl/) 1.24 or later. The Windows exe can be built on Windows, macOS or Linux:

```
make build      # dist/MSFSLayoutGenerator.exe
make test       # run the tests on this machine
make test-exe   # dist/MSFSLayoutGenerator.test.exe; run it on Windows with -test.v
```

Without make: `GOOS=windows GOARCH=amd64 go build -o dist/MSFSLayoutGenerator.exe .`

On every push, [GitHub Actions](.github/workflows/ci.yml) runs the tests on Windows and builds the exe, which is attached to the run as an artifact.

The icon, version information and Windows manifest are defined in `winres/winres.json`. After changing it, run `make winres` to regenerate `rsrc_windows_amd64.syso`.

## Credits and license

Based on [MSFS Layout Generator](https://github.com/HughesMDflyer4/MSFSLayoutGenerator) by Brandon Filer. Released under the MIT license; see [LICENSE](LICENSE).
