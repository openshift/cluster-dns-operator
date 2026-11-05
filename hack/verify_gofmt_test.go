package hack

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type gofmtFixture struct {
	root      string
	temporary string
	bin       string
	script    string
	formatter string
	env       []string
}

func writeGofmtFixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func newGofmtFixture(t *testing.T) *gofmtFixture {
	t.Helper()
	directory := t.TempDir()
	script, err := filepath.Abs("verify-gofmt.sh")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &gofmtFixture{
		root:      filepath.Join(directory, "source"),
		temporary: filepath.Join(directory, "temporary"),
		bin:       filepath.Join(directory, "bin"),
		script:    script,
		formatter: filepath.Join(runtime.GOROOT(), "bin", "gofmt"),
	}
	for _, path := range []string{fixture.root, fixture.temporary, fixture.bin} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	fixture.env = []string{
		"PATH=" + fixture.bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TMPDIR=" + fixture.temporary,
		"GOFMT=" + fixture.formatter,
	}
	t.Cleanup(func() {
		entries, err := os.ReadDir(fixture.temporary)
		if err != nil {
			t.Error(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "verify-gofmt.") || entry.Name() == "forced" {
				t.Errorf("verifier temporary directory was not cleaned up: %s", entry.Name())
			}
		}
	})
	return fixture
}

func (fixture *gofmtFixture) run(t *testing.T, program string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, program, args...)
	command.Dir = fixture.root
	command.Env = append(os.Environ(), fixture.env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("command did not complete: %v", ctx.Err())
	}
	return stdout.String(), stderr.String(), err
}

func (fixture *gofmtFixture) fakeFormatter(t *testing.T, mode string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture.bin, "formatter with spaces")
	writeGofmtFixtureFile(t, path, "#!/bin/bash\nexec \"$VERIFY_GOFMT_TEST_BINARY\" -test.run=^TestVerifyGofmtFormatter$ -- \"$@\"\n", 0755)
	record := filepath.Join(fixture.bin, "arguments")
	fixture.env = append(fixture.env,
		"GOFMT="+path,
		"VERIFY_GOFMT_TEST_BINARY="+executable,
		"VERIFY_GOFMT_TEST_MODE="+mode,
		"VERIFY_GOFMT_TEST_RECORD="+record,
		"GORACE=atexit_sleep_ms=0",
	)
	return record
}

func TestVerifyGofmtFormatter(t *testing.T) {
	mode := os.Getenv("VERIFY_GOFMT_TEST_MODE")
	if mode == "" {
		return
	}
	if mode == "fail" || mode == "partial" {
		if mode == "partial" {
			fmt.Println("partial.go")
		}
		fmt.Fprintln(os.Stderr, "formatter execution failure")
		os.Exit(3)
	}
	var arguments []string
	for index, argument := range os.Args {
		if argument == "--" {
			arguments = os.Args[index+1:]
			break
		}
	}
	if len(arguments) < 3 || arguments[0] != "-s" || arguments[1] != "-l" || len(arguments) > 102 {
		fmt.Fprintln(os.Stderr, "unexpected formatter arguments")
		os.Exit(4)
	}
	record, err := os.OpenFile(os.Getenv("VERIFY_GOFMT_TEST_RECORD"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(5)
	}
	_, writeError := record.WriteString(strings.Join(arguments[2:], "\x00") + "\x00")
	closeError := record.Close()
	if writeError != nil || closeError != nil {
		fmt.Fprintln(os.Stderr, writeError, closeError)
		os.Exit(6)
	}
	os.Exit(0)
}

func TestVerifyGofmtSource(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name    string
		source  string
		wantErr bool
		message string
	}{
		{name: "clean", source: "package fixture\n"},
		{name: "unformatted", source: "package fixture;var value=1\n", wantErr: true, message: "fixture.go"},
		{name: "invalid syntax", source: "package {\n", wantErr: true, message: "expected"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newGofmtFixture(t)
			path := filepath.Join(fixture.root, "fixture.go")
			writeGofmtFixtureFile(t, path, testCase.source, 0644)
			stdout, stderr, err := fixture.run(t, "bash", fixture.script)
			if (err != nil) != testCase.wantErr || !strings.Contains(stdout+stderr, testCase.message) {
				t.Fatalf("error=%v, stdout=%q, stderr=%q", err, stdout, stderr)
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || string(unchanged) != testCase.source {
				t.Fatalf("source was modified: %q, %v", unchanged, err)
			}
		})
	}
}

func TestVerifyGofmtFailures(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"formatter", "partial output", "missing formatter", "selection", "temporary directory", "output file"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			fixture := newGofmtFixture(t)
			writeGofmtFixtureFile(t, filepath.Join(fixture.root, "fixture.go"), "package fixture\n", 0644)
			switch failure {
			case "formatter":
				fixture.fakeFormatter(t, "fail")
			case "partial output":
				fixture.fakeFormatter(t, "partial")
			case "missing formatter":
				fixture.env = append(fixture.env, "GOFMT="+filepath.Join(fixture.bin, "missing"))
			case "selection":
				writeGofmtFixtureFile(t, filepath.Join(fixture.bin, "find"), "#!/bin/bash\nprintf './fixture.go\\0'\necho 'selection failure' >&2\nexit 9\n", 0755)
			case "temporary directory":
				fixture.env = append(fixture.env, "TMPDIR="+filepath.Join(fixture.temporary, "missing"))
			case "output file":
				forced := filepath.Join(fixture.temporary, "forced")
				if err := os.MkdirAll(filepath.Join(forced, "gofmt.out"), 0755); err != nil {
					t.Fatal(err)
				}
				fixture.env = append(fixture.env, "VERIFY_GOFMT_FORCED_TMP="+forced)
				writeGofmtFixtureFile(t, filepath.Join(fixture.bin, "mktemp"), "#!/bin/bash\nprintf '%s\\n' \"$VERIFY_GOFMT_FORCED_TMP\"\n", 0755)
			}
			stdout, stderr, err := fixture.run(t, "bash", fixture.script)
			if err == nil || stderr == "" {
				t.Fatalf("failure was not reported: error=%v, stdout=%q, stderr=%q", err, stdout, stderr)
			}
		})
	}
}

func TestVerifyGofmtEmpty(t *testing.T) {
	t.Parallel()
	fixture := newGofmtFixture(t)
	record := fixture.fakeFormatter(t, "record")
	stdout, stderr, err := fixture.run(t, "bash", fixture.script)
	if err != nil || stdout != "" || stderr != "" {
		t.Fatalf("empty selection failed: %v, %q, %q", err, stdout, stderr)
	}
	if _, err := os.Stat(record); !os.IsNotExist(err) {
		t.Fatalf("formatter was invoked on empty input: %v", err)
	}
}

func TestVerifyGofmtFilenames(t *testing.T) {
	t.Parallel()
	fixture := newGofmtFixture(t)
	record := fixture.fakeFormatter(t, "record")
	paths := []string{"space name.go", "tab\tname.go", "line\nname.go", "glob[?]*.go", "-leading.go"}
	for _, path := range paths {
		writeGofmtFixtureFile(t, filepath.Join(fixture.root, path), "package fixture\n", 0644)
	}
	stdout, stderr, err := fixture.run(t, "bash", fixture.script)
	if err != nil {
		t.Fatalf("filename verification failed: %v, %q, %q", err, stdout, stderr)
	}
	assertGofmtArguments(t, record, paths)
}

func assertGofmtArguments(t *testing.T, record string, paths []string) {
	t.Helper()
	content, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	arguments := strings.Split(strings.TrimSuffix(string(content), "\x00"), "\x00")
	counts := make(map[string]int)
	for _, argument := range arguments {
		counts[argument]++
	}
	if len(arguments) != len(paths) {
		t.Fatalf("received %d paths, want %d", len(arguments), len(paths))
	}
	for _, path := range paths {
		if counts["./"+filepath.ToSlash(path)] != 1 {
			t.Fatalf("path %q was not passed exactly once", path)
		}
	}
}

func TestVerifyGofmtPolicy(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"vendor/fixture.go", "tests-extension/vendor/fixture.go", "other/vendor/fixture.go",
		"_output/fixture.go", "other/_output/fixture.go", "tests-extension/cmd/fixture.go",
		"tests-extension/test/qe/bindata.go", "generated.go", "vendorish/fixture.go", "_outputish/fixture.go",
		"component.go/vendor/fixture.go", "component.go/_output/fixture.go", "component.go/fixture.go",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			fixture := newGofmtFixture(t)
			writeGofmtFixtureFile(t, filepath.Join(fixture.root, path), "// Code generated by fixture. DO NOT EDIT.\npackage fixture;var value=1\n", 0644)
			excluded := strings.Contains("/"+path, "/vendor/") || strings.Contains("/"+path, "/_output/")
			stdout, stderr, err := fixture.run(t, "bash", fixture.script)
			if (err == nil) != excluded || (!excluded && !strings.Contains(stdout, path)) {
				t.Fatalf("excluded=%v, error=%v, stdout=%q, stderr=%q", excluded, err, stdout, stderr)
			}
		})
	}
}

func TestVerifyGofmtDefaultFormatter(t *testing.T) {
	t.Parallel()
	fixture := newGofmtFixture(t)
	writeGofmtFixtureFile(t, filepath.Join(fixture.root, "fixture.go"), "package fixture\n", 0644)
	if err := os.Symlink(fixture.formatter, filepath.Join(fixture.bin, "gofmt")); err != nil {
		t.Fatal(err)
	}
	fixture.env = append(fixture.env, "GOFMT=")
	stdout, stderr, err := fixture.run(t, "bash", fixture.script)
	if err != nil {
		t.Fatalf("default formatter failed: %v, %q, %q", err, stdout, stderr)
	}
}

func TestVerifyGofmtArgumentLimit(t *testing.T) {
	fixture := newGofmtFixture(t)
	output, err := exec.Command("getconf", "ARG_MAX").Output()
	if err != nil {
		t.Skipf("cannot measure argument limit: %v", err)
	}
	limit, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || limit <= 0 || limit > 8*1024*1024 {
		t.Skipf("unsupported argument limit %q: %v", output, err)
	}
	record := fixture.fakeFormatter(t, "record")
	var paths []string
	totalBytes := 0
	for index := 0; totalBytes <= limit; index++ {
		path := filepath.Join(strings.Repeat("segment", 25), fmt.Sprintf("fixture-%06d.go", index))
		writeGofmtFixtureFile(t, filepath.Join(fixture.root, path), "package fixture\n", 0644)
		paths = append(paths, path)
		totalBytes += len("./"+path) + 1
	}
	stdout, stderr, err := fixture.run(t, "bash", fixture.script)
	if err != nil {
		t.Fatalf("argument-limit verification failed: %v, %q, %q", err, stdout, stderr)
	}
	assertGofmtArguments(t, record, paths)
	t.Logf("processed %d paths containing %d argument bytes, exceeding ARG_MAX=%d", len(paths), totalBytes, limit)
}

func TestVerifyGofmtMakeFailure(t *testing.T) {
	t.Parallel()
	fixture := newGofmtFixture(t)
	repository := filepath.Dir(filepath.Dir(fixture.script))
	if err := os.Symlink(filepath.Join(repository, "vendor"), filepath.Join(fixture.root, "vendor")); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(fixture.script)
	if err != nil {
		t.Fatal(err)
	}
	writeGofmtFixtureFile(t, filepath.Join(fixture.root, "hack", "verify-gofmt.sh"), string(script), 0755)
	writeGofmtFixtureFile(t, filepath.Join(fixture.root, "fixture.go"), "package fixture;var value=1\n", 0644)
	stdout, stderr, err := fixture.run(t, "make", "-f", filepath.Join(repository, "Makefile"), "-o", "verify-generated", "verify")
	if err == nil || !strings.Contains(stdout, "fixture.go") || !strings.Contains(stderr, "gofmt needs to be run") {
		t.Fatalf("Make did not propagate the formatter failure: %v, %q, %q", err, stdout, stderr)
	}
}
