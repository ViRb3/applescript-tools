package applescript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFixedPointDifference(t *testing.T) {
	if err := fixedPointDifference("return 1\n", "return 1\n"); err != nil {
		t.Fatalf("stable source rejected: %v", err)
	}
	err := fixedPointDifference("one\nreturn 1\n", "one\nreturn 2\n")
	if err == nil || !strings.Contains(err.Error(), "line 2") ||
		!strings.Contains(err.Error(), "return 1") || !strings.Contains(err.Error(), "return 2") {
		t.Fatalf("unstable source error = %v", err)
	}
}

func TestOracleArtifactNamesAndCleanup(t *testing.T) {
	oracle := &compilerOracle{directory: t.TempDir()}
	first, err := oracle.newArtifacts("probe")
	if err != nil {
		t.Fatal(err)
	}
	second, err := oracle.newArtifacts("probe")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first.source == second.source || first.script == second.script || first.log == second.log {
		t.Fatalf("artifact names are not unique: first=%#v second=%#v", first, second)
	}
	for _, artifacts := range []oracleArtifacts{first, second} {
		for _, path := range []string{artifacts.source, artifacts.script, artifacts.log} {
			if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		artifacts.cleanup()
		for _, path := range []string{artifacts.source, artifacts.script, artifacts.log} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("cleanup retained %s: %v", path, err)
			}
		}
	}
}

func TestOracleCompileRegistersArtifactCleanup(t *testing.T) {
	oracle := &compilerOracle{directory: t.TempDir()}
	var artifacts oracleArtifacts
	service := make(chan error, 1)

	t.Run("compile", func(t *testing.T) {
		go func() {
			sourcePath, err := waitForOracleSource(oracle.directory)
			if err != nil {
				service <- err
				return
			}
			stem := strings.TrimSuffix(sourcePath, ".applescript")
			artifacts = oracleArtifacts{
				source: sourcePath,
				script: stem + ".scpt",
				log:    stem + ".log",
			}
			if err := os.WriteFile(artifacts.script, []byte("compiled"), 0o644); err != nil {
				service <- err
				return
			}
			service <- os.WriteFile(artifacts.log, nil, 0o644)
		}()

		if script := oracle.compile(t, "return 1\n", "cleanup"); script == "" {
			t.Fatal("compile returned no script")
		}
		if err := <-service; err != nil {
			t.Fatal(err)
		}
	})

	for _, path := range []string{artifacts.source, artifacts.script, artifacts.log} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("registered cleanup retained %s: %v", path, err)
		}
	}
}

func TestOracleCompileWaitsForLogCompletion(t *testing.T) {
	oracle := &compilerOracle{directory: t.TempDir()}
	type result struct {
		script    string
		artifacts oracleArtifacts
		err       error
	}
	results := make(chan result, 1)
	go func() {
		script, artifacts, err := oracle.compileSource("return 1\n", "wait")
		results <- result{script: script, artifacts: artifacts, err: err}
	}()

	sourcePath, err := waitForOracleSource(oracle.directory)
	if err != nil {
		t.Fatal(err)
	}
	stem := strings.TrimSuffix(sourcePath, ".applescript")
	scriptPath := stem + ".scpt"
	logPath := stem + ".log"
	if err := os.WriteFile(scriptPath, []byte("compiled"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-results:
		got.artifacts.cleanup()
		t.Fatalf("compile completed before log signal: %#v", got)
	case <-time.After(125 * time.Millisecond):
	}
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-results:
		defer got.artifacts.cleanup()
		if got.err != nil || got.script != scriptPath {
			t.Fatalf("compile result = %#v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("compile did not observe log signal")
	}
}

func TestOracleCompileReportsLogWithoutScript(t *testing.T) {
	oracle := &compilerOracle{directory: t.TempDir()}
	type result struct {
		artifacts oracleArtifacts
		err       error
	}
	results := make(chan result, 1)
	go func() {
		_, artifacts, err := oracle.compileSource("one\ntwo\nthree\n", "failure")
		results <- result{artifacts: artifacts, err: err}
	}()

	sourcePath, err := waitForOracleSource(oracle.directory)
	if err != nil {
		t.Fatal(err)
	}
	logPath := strings.TrimSuffix(sourcePath, ".applescript") + ".log"
	if err := os.WriteFile(logPath, []byte("/tmp/probe:2: error: syntax error\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-results:
		defer got.artifacts.cleanup()
		if got.err == nil || !strings.Contains(got.err.Error(), "syntax error") ||
			!strings.Contains(got.err.Error(), "    2  two") {
			t.Fatalf("compiler error = %v", got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("compile did not report failure log")
	}
}

func TestOracleDirectoryAvailability(t *testing.T) {
	directory := t.TempDir()
	if !oracleDirectoryAvailable(directory) {
		t.Fatal("existing oracle directory reported unavailable")
	}
	if oracleDirectoryAvailable(filepath.Join(directory, "missing")) {
		t.Fatal("missing oracle directory reported available")
	}
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if oracleDirectoryAvailable(file) {
		t.Fatal("ordinary file reported as an oracle directory")
	}
}

func TestOracleBackendSelection(t *testing.T) {
	externalDirectory := t.TempDir()
	lookup := func(name string) (string, error) {
		if name != "osacompile" {
			t.Fatalf("lookup = %q", name)
		}
		return "/usr/bin/osacompile", nil
	}

	external, err := resolveCompilerOracle(externalDirectory, "linux", lookup)
	if err != nil || external.backend != oracleExternal || external.directory != externalDirectory {
		t.Fatalf("external selection = %#v, %v", external, err)
	}
	preferredExternal, err := resolveCompilerOracle(externalDirectory, "darwin", func(string) (string, error) {
		t.Fatal("explicit external directory should win")
		return "", nil
	})
	if err != nil || preferredExternal.backend != oracleExternal {
		t.Fatalf("preferred external selection = %#v, %v", preferredExternal, err)
	}
	local, err := resolveCompilerOracle("", "darwin", lookup)
	if err != nil || local.backend != oracleLocal || local.executable != "/usr/bin/osacompile" {
		t.Fatalf("local selection = %#v, %v", local, err)
	}
	if _, err := resolveCompilerOracle("", "linux", lookup); err == nil ||
		!strings.Contains(err.Error(), "APPLESCRIPT_COMPILER_ORACLE_DIR") {
		t.Fatalf("unconfigured non-macOS selection error = %v", err)
	}
	if _, err := resolveCompilerOracle(filepath.Join(externalDirectory, "missing"), "linux", lookup); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unavailable external selection error = %v", err)
	}
}

func TestLocalOracleCompile(t *testing.T) {
	directory := t.TempDir()
	compiler := filepath.Join(directory, "fake-osacompile")
	const program = "#!/bin/sh\n" +
		"if [ \"$1\" != \"-o\" ]; then exit 2; fi\n" +
		"cp \"$3\" \"$2\"\n"
	if err := os.WriteFile(compiler, []byte(program), 0o755); err != nil {
		t.Fatal(err)
	}
	oracle := &compilerOracle{
		backend:    oracleLocal,
		directory:  directory,
		executable: compiler,
	}
	const source = "on run\n    return 1\nend run\n"
	script, artifacts, err := oracle.compileSource(source, "local_success")
	if err != nil {
		artifacts.cleanup()
		t.Fatal(err)
	}
	defer artifacts.cleanup()
	compiled, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if string(compiled) != source {
		t.Fatalf("local compiler output = %q", compiled)
	}
}

func TestLocalOracleCompilerError(t *testing.T) {
	directory := t.TempDir()
	compiler := filepath.Join(directory, "failing-osacompile")
	const program = "#!/bin/sh\n" +
		"echo \"$3:2: error: syntax error\" >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(compiler, []byte(program), 0o755); err != nil {
		t.Fatal(err)
	}
	oracle := &compilerOracle{
		backend:    oracleLocal,
		directory:  directory,
		executable: compiler,
	}
	_, artifacts, err := oracle.compileSource("one\ntwo\nthree\n", "local_failure")
	defer artifacts.cleanup()
	if err == nil || !strings.Contains(err.Error(), "syntax error") ||
		!strings.Contains(err.Error(), "    2  two") {
		t.Fatalf("local compiler error = %v", err)
	}
	log, readErr := os.ReadFile(artifacts.log)
	if readErr != nil || !strings.Contains(string(log), "syntax error") {
		t.Fatalf("local compiler log = %q, %v", log, readErr)
	}
}

func waitForOracleSource(directory string) (string, error) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		paths, err := filepath.Glob(filepath.Join(directory, "*.applescript"))
		if err != nil {
			return "", err
		}
		if len(paths) == 1 {
			return paths[0], nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return "", fmt.Errorf("oracle source did not appear in %s", directory)
}
