# All targets work on macOS, Linux and Windows; the Windows binaries are cross-compiled.
EXE      := dist/MSFSLayoutGenerator.exe
TEST_EXE := dist/MSFSLayoutGenerator.test.exe
WINRES   := github.com/tc-hib/go-winres@v0.3.3

.PHONY: build test test-exe winres clean

build:
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $(EXE) .

test:
	go test ./...

# Test suite as a Windows program, to run on Windows: MSFSLayoutGenerator.test.exe -test.v
test-exe:
	GOOS=windows GOARCH=amd64 go test -c -o $(TEST_EXE) .

# Regenerates rsrc_windows_amd64.syso (icon, version info, manifest) from winres/winres.json.
winres:
	go run $(WINRES) make --in winres/winres.json --arch amd64 --out rsrc

clean:
	rm -rf dist
