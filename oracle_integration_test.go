package applescript

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"applescript-tools/passes"
)

type compilerOracle struct {
	backend    oracleBackend
	directory  string
	executable string
}

type oracleBackend uint8

const (
	oracleExternal oracleBackend = iota
	oracleLocal
)

type oracleArtifacts struct {
	source string
	script string
	log    string
}

func (a oracleArtifacts) cleanup() {
	for _, path := range []string{a.source, a.script, a.log} {
		if path != "" {
			_ = os.Remove(path)
		}
	}
}

func oracleDirectoryAvailable(directory string) bool {
	info, err := os.Stat(directory)
	return err == nil && info.IsDir()
}

func newCompilerOracle(t *testing.T) *compilerOracle {
	t.Helper()
	oracle, err := resolveCompilerOracle(
		os.Getenv("APPLESCRIPT_COMPILER_ORACLE_DIR"),
		runtime.GOOS,
		exec.LookPath,
	)
	if err != nil {
		t.Skipf("AppleScript compiler oracle unavailable: %v", err)
	}
	if oracle.backend == oracleLocal {
		oracle.directory = t.TempDir()
	}
	return oracle
}

func resolveCompilerOracle(
	configuredDirectory, goos string,
	lookPath func(string) (string, error),
) (*compilerOracle, error) {
	if configuredDirectory != "" {
		if !oracleDirectoryAvailable(configuredDirectory) {
			return nil, fmt.Errorf(
				"external directory %s is unavailable",
				configuredDirectory,
			)
		}
		return &compilerOracle{
			backend:   oracleExternal,
			directory: configuredDirectory,
		}, nil
	}
	if goos == "darwin" {
		executable, err := lookPath("osacompile")
		if err != nil {
			return nil, fmt.Errorf("find osacompile: %w", err)
		}
		return &compilerOracle{backend: oracleLocal, executable: executable}, nil
	}
	return nil, fmt.Errorf(
		"no compiler oracle configured: set APPLESCRIPT_COMPILER_ORACLE_DIR",
	)
}

func (o *compilerOracle) newArtifacts(label string) (oracleArtifacts, error) {
	tokenBytes := make([]byte, 12)
	if _, err := rand.Read(tokenBytes); err != nil {
		return oracleArtifacts{}, err
	}
	stem := "go_decompiler_" + label + "_" + hex.EncodeToString(tokenBytes)
	return oracleArtifacts{
		source: filepath.Join(o.directory, stem+".applescript"),
		script: filepath.Join(o.directory, stem+".scpt"),
		log:    filepath.Join(o.directory, stem+".log"),
	}, nil
}

func (o *compilerOracle) compileSource(source, label string) (string, oracleArtifacts, error) {
	if o.backend == oracleLocal {
		return o.compileLocalSource(source, label)
	}
	return o.compileExternalSource(source, label)
}

func (o *compilerOracle) compileExternalSource(source, label string) (string, oracleArtifacts, error) {
	artifacts, err := o.newArtifacts(label)
	if err != nil {
		return "", artifacts, err
	}
	if err := os.WriteFile(artifacts.source, []byte(source), 0o644); err != nil {
		return "", artifacts, err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(artifacts.log); err == nil {
			if _, err := os.Stat(artifacts.script); err == nil {
				return artifacts.script, artifacts, nil
			}
			log, _ := os.ReadFile(artifacts.log)
			return "", artifacts, compilerFailure(source, label, log)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", artifacts, fmt.Errorf("compiler timed out for %s", label)
}

func (o *compilerOracle) compileLocalSource(source, label string) (string, oracleArtifacts, error) {
	artifacts, err := o.newArtifacts(label)
	if err != nil {
		return "", artifacts, err
	}
	if err := os.WriteFile(artifacts.source, []byte(source), 0o644); err != nil {
		return "", artifacts, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, o.executable, "-o", artifacts.script, artifacts.source)
	output, commandErr := command.CombinedOutput()
	if commandErr != nil {
		_ = os.WriteFile(artifacts.log, output, 0o644)
		if ctx.Err() != nil {
			return "", artifacts, fmt.Errorf("compiler timed out for %s: %w", label, ctx.Err())
		}
		return "", artifacts, compilerFailure(source, label, output)
	}
	if _, err := os.Stat(artifacts.script); err != nil {
		return "", artifacts, fmt.Errorf("local compiler produced no script for %s: %w", label, err)
	}
	return artifacts.script, artifacts, nil
}

func compilerFailure(source, label string, log []byte) error {
	var context strings.Builder
	match := regexp.MustCompile(`:(\d+): error`).FindSubmatch(log)
	if len(match) == 2 {
		line, _ := strconv.Atoi(string(match[1]))
		lines := strings.Split(source, "\n")
		start := max(line-3, 0)
		end := min(line+2, len(lines))
		for i := start; i < end; i++ {
			context.WriteString(fmt.Sprintf("%5d  %s\n", i+1, lines[i]))
		}
	}
	return fmt.Errorf("compiler rejected %s:\n%s%s", label, log, context.String())
}

func (o *compilerOracle) compile(t *testing.T, source, label string) string {
	t.Helper()
	script, artifacts, err := o.compileSource(source, label)
	t.Cleanup(artifacts.cleanup)
	if err != nil {
		t.Fatal(err)
	}
	return script
}

func decompilePath(t *testing.T, path string) string {
	return decompilePathWithOptions(t, path, DecompileOptions{})
}

func decompilePathWithOptions(t *testing.T, path string, options DecompileOptions) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result, err := Decompile(context.Background(), f, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("decompilation diagnostics: %#v", result.Diagnostics)
	}
	return result.Source
}

func TestDemoCompilerFixedPoint(t *testing.T) {
	if testing.Short() {
		t.Skip("compiler integration disabled in short mode")
	}
	oracle := newCompilerOracle(t)
	source := decompilePath(t, "testdata/demo.scpt")
	requireLocalCompilerDialect(t, oracle, source, "demo", "legacy load-script syntax")
	assertOracleFixedPoint(t, oracle, source, "demo", DecompileOptions{})
}

func TestSecconCompilerFixedPoint(t *testing.T) {
	if testing.Short() {
		t.Skip("compiler integration disabled in short mode")
	}
	oracle := newCompilerOracle(t)
	source := decompilePath(t, "testdata/seccon.scpt")
	requireLocalCompilerDialect(t, oracle, source, "seccon", "legacy Standard Additions terminology")
	assertOracleFixedPoint(t, oracle, source, "seccon", DecompileOptions{})
}

var localCompilerDialectLimitations = map[string]string{
	"ascii_recovery":             "legacy ASCII character Standard Addition",
	"classic_enumeration":        "legacy folder-domain terminology",
	"copy_data":                  "iTunes dictionary unavailable on current macOS",
	"enumeration_known":          "legacy display-alert Standard Addition",
	"global_references":          "legacy display-dialog Standard Addition",
	"legacy_application_running": "System Preferences dictionary unavailable on current macOS",
	"sdef_boolean_arguments":     "legacy Standard Additions terminology",
}

func requireLocalCompilerDialect(t *testing.T, oracle *compilerOracle, source, label, limitation string) {
	t.Helper()
	if oracle.backend != oracleLocal {
		return
	}
	_, artifacts, err := oracle.compileSource(source, label+"_dialect_probe")
	artifacts.cleanup()
	if err != nil {
		t.Skipf("local compiler lacks %s: %v", limitation, err)
	}
}

func TestPathToMeByteRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("compiler integration disabled in short mode")
	}
	oracle := newCompilerOracle(t)
	original := oracle.compile(t, "on run\n    return path to me\nend run\n", "path_to_me_original")
	source := decompilePathWithOptions(t, original, DecompileOptions{Strict: true})
	if !strings.Contains(source, "path to me") {
		t.Fatalf("decompiled source lost explicit receiver:\n%s", source)
	}
	recompiled := oracle.compile(t, source, "path_to_me_recompiled")
	assertScriptBytesEqual(t, original, recompiled)
}

func TestTransactionAndContinueSemanticRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("compiler integration disabled in short mode")
	}
	oracle := newCompilerOracle(t)
	for _, test := range []struct {
		label  string
		source string
		want   string
	}{
		{
			label:  "transaction",
			source: "on run\n    with transaction\n        set x to 1\n    end transaction\nend run\n",
			want:   "with transaction",
		},
		{
			label:  "continue_open",
			source: "on open xs\n    continue open xs\nend open\n",
			want:   "continue open xs",
		},
	} {
		t.Run(test.label, func(t *testing.T) {
			decompiled := assertOracleFixedPoint(t, oracle, test.source, test.label, DecompileOptions{Strict: true})
			if !strings.Contains(decompiled, test.want) {
				t.Fatalf("decompiled source missing %q:\n%s", test.want, decompiled)
			}
		})
	}
}

func TestAuthoritativeLanguageTermsByteRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("compiler integration disabled in short mode")
	}
	oracle := newCompilerOracle(t)
	originalSource := "on run\n" +
		"    set plainString to \"value\" as string\n" +
		"    set richText to \"value\" as text\n" +
		"    set supportFolder to path to application support\n" +
		"    return {plainString, richText, supportFolder, character id 65, tab, return}\n" +
		"end run\n"
	original := oracle.compile(t, originalSource, "authoritative_language_terms_original")
	source := decompilePathWithOptions(t, original, DecompileOptions{Strict: true})
	for _, fragment := range []string{
		`as string`,
		`as text`,
		`path to application support`,
		`character id 65`,
		`tab`,
		`return`,
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("decompiled source missing %q:\n%s", fragment, source)
		}
	}
	recompiled := oracle.compile(t, source, "authoritative_language_terms_recompiled")
	assertScriptBytesEqual(t, original, recompiled)
}

func assertOracleFixedPoint(t *testing.T, oracle *compilerOracle, source, label string, options DecompileOptions) string {
	t.Helper()
	first := oracle.compile(t, source, label+"_first")
	firstSource := decompilePathWithOptions(t, first, options)
	second := oracle.compile(t, firstSource, label+"_second")
	secondSource := decompilePathWithOptions(t, second, options)
	if err := fixedPointDifference(firstSource, secondSource); err != nil {
		t.Fatal(err)
	}
	return firstSource
}

func fixedPointDifference(firstSource, secondSource string) error {
	if firstSource == secondSource {
		return nil
	}
	firstLines, secondLines := strings.Split(firstSource, "\n"), strings.Split(secondSource, "\n")
	for i := 0; i < len(firstLines) && i < len(secondLines); i++ {
		if firstLines[i] != secondLines[i] {
			start := max(i-3, 0)
			end := min(i+4, len(firstLines))
			secondEnd := min(i+4, len(secondLines))
			return fmt.Errorf("fixed point differs at line %d:\nfirst:\n%s\nsecond:\n%s", i+1, strings.Join(firstLines[start:end], "\n"), strings.Join(secondLines[start:secondEnd], "\n"))
		}
	}
	return fmt.Errorf("fixed point differs:\nfirst length %d, second length %d", len(firstSource), len(secondSource))
}

func TestFocusedOracleFixtures(t *testing.T) {
	if testing.Short() {
		t.Skip("compiler integration disabled in short mode")
	}
	oracle := newCompilerOracle(t)
	paths, err := filepath.Glob("testdata/oracle/*.applescript")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != compiledFixtureCount {
		t.Fatalf("found %d fixtures, want %d", len(paths), compiledFixtureCount)
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(name, func(t *testing.T) {
			sourceBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if name == "root_reference_library" && oracle.backend == oracleLocal {
				_, artifacts, err := oracle.compileSource(
					string(sourceBytes),
					name+"_availability",
				)
				artifacts.cleanup()
				if err != nil {
					// A named script library is resolved from the host's Script
					// Libraries folders. The committed .scpt fixture still
					// exercises decompilation on hosts without that optional
					// development dependency.
					if strings.Contains(err.Error(), "(-1728)") {
						t.Skip("Kevin's Library is not installed")
					}
					t.Fatal(err)
				}
			}
			options := DecompileOptions{Strict: true}
			switch name {
			case "ascii_recovery", "naive_list_edges", "naive_empty_concat":
				options.Passes = []passes.Pass{passes.Strings{}}
			}
			if limitation, ok := localCompilerDialectLimitations[name]; ok {
				requireLocalCompilerDialect(t, oracle, string(sourceBytes), name, limitation)
			}
			got := assertOracleFixedPoint(
				t,
				oracle,
				string(sourceBytes),
				name,
				options,
			)
			expected, err := os.ReadFile(filepath.Join(
				"testdata",
				"compiled",
				name+".expected.applescript",
			))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(expected) {
				t.Fatalf(
					"oracle source decompiles differently from committed golden",
				)
			}
		})
	}
}

func assertScriptBytesEqual(t *testing.T, wantPath, gotPath string) {
	t.Helper()
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(want, got) {
		return
	}
	offset := min(len(want), len(got))
	for i := 0; i < offset; i++ {
		if want[i] != got[i] {
			offset = i
			break
		}
	}
	t.Fatalf("compiled bytecode differs at offset %d: want=%d bytes, got=%d bytes", offset, len(want), len(got))
}
